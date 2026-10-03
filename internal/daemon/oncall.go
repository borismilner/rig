package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/supervise"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// Section 54: a program started when it is called. The supervisor holds its
// state and starts it; this file keeps its declaration while it is down, so
// it is listed and called as though it were running, and starts it inside
// the call that needs it.

// onCallState is what the daemon knows about its on-call programs' kept
// declarations. Its own lock: it is read on every call to one of them.
type onCallState struct {
	mu sync.Mutex
	// kept is the binary identity each kept declaration was read from.
	kept map[string]string
	// reading is the binary identity a declare run is reading now, so one
	// identity is read once, and a binary that fails to start is not
	// relaunched by every listing.
	reading map[string]string
}

// binaryID is a binary's identity: device, inode, size and modification
// time. A rebuild, an install and a package upgrade each change one of them,
// and reading it is one stat.
func binaryID(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano())
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		id = fmt.Sprintf("%d:%d:%s", st.Dev, st.Ino, id)
	}
	return id, nil
}

// keptPath is where one program's declaration is kept. The id is a
// supervise.Spec's, which Validate has already refused a separator in.
func (d *Daemon) keptPath(id string) string {
	if d.keptDir == "" {
		return ""
	}
	return filepath.Join(d.keptDir, id+".hello")
}

// loadKept answers the kept hello for id if it was read from this binary.
func (d *Daemon) loadKept(id, bin string) (*rigv1.HelloRequest, bool) {
	path := d.keptPath(id)
	if path == "" {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	head, body, ok := strings.Cut(string(b), "\n")
	if !ok || head != bin {
		return nil, false
	}
	var req rigv1.HelloRequest
	if err := proto.Unmarshal([]byte(body), &req); err != nil {
		return nil, false
	}
	return &req, true
}

// saveKept writes the hello, behind the identity it was read from, through a
// temporary file so a reader never sees half of one.
func (d *Daemon) saveKept(id, bin string, req *rigv1.HelloRequest) error {
	path := d.keptPath(id)
	if path == "" {
		return nil
	}
	body, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.keptDir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append([]byte(bin+"\n"), body...), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// onCallSpec is the spec of a program declared on call, if id is one.
func (d *Daemon) onCallSpec(id string) (supervise.Spec, bool) {
	if d.super == nil {
		return supervise.Spec{}, false
	}
	spec, ok := d.super.Spec(id)
	return spec, ok && spec.OnCall
}

// restOnCall is rigd starting: every on-call program's declaration is taken
// from what was kept, or read by a declare run when the binary has changed.
func (d *Daemon) restOnCall() {
	if d.super == nil {
		return
	}
	for _, id := range d.super.Declared() {
		if _, ok := d.onCallSpec(id); ok {
			d.refreshKept(id)
		}
	}
}

// refreshKept makes sure id's kept declaration was read from the binary on
// disk now: kept as it is, loaded from the file, or read by a declare run.
// One stat when nothing changed.
func (d *Daemon) refreshKept(id string) {
	spec, ok := d.onCallSpec(id)
	if !ok {
		return
	}
	bin, err := binaryID(spec.Path)
	if err != nil {
		// The binary is gone. What was kept stays listed; a call will start
		// nothing and say why.
		d.restIfDown(id)
		return
	}
	d.oncall.mu.Lock()
	switch {
	case d.oncall.kept[id] == bin, d.oncall.reading[id] == bin:
		d.oncall.mu.Unlock()
		return
	}
	d.oncall.mu.Unlock()

	if req, ok := d.loadKept(id, bin); ok {
		if decl, err := declarationFromWire(req); err == nil && d.kernel.Rest(decl) == nil {
			d.oncall.mu.Lock()
			d.oncall.kept[id] = bin
			d.oncall.mu.Unlock()
			d.restIfDown(id)
			return
		}
	}
	if d.connected(id) {
		// Its next hello is read from whatever binary it is.
		return
	}
	d.oncall.mu.Lock()
	d.oncall.reading[id] = bin
	d.oncall.mu.Unlock()
	d.log.Info("reading an on-call program's declaration", "program", id, "binary", spec.Path)
	if _, err := d.super.Up(id); err != nil {
		d.log.Warn("the declare run did not start", "program", id, "err", err)
	}
}

// restIfDown puts a program that is not running at rest. Never one that is:
// Rest stops a running program with no call in flight, which is right for a
// declare run and wrong for a program still showing something.
func (d *Daemon) restIfDown(id string) {
	if !d.connected(id) {
		_ = d.super.Rest(id)
	}
}

func (d *Daemon) connected(id string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.programs[id]
	return ok
}

// keptHello is an on-call program's hello: its declaration is kept, whether
// this start was a declare run or a call, and a declare run ends here.
func (d *Daemon) keptHello(c *conn, req *rigv1.HelloRequest, decl kernel.Declaration) {
	id, ok := d.supervisedChild(c)
	if !ok {
		return
	}
	spec, ok := d.onCallSpec(id)
	if !ok {
		return
	}
	bin, err := binaryID(spec.Path)
	if err == nil {
		err = d.saveKept(id, bin, req)
	}
	if err != nil {
		d.log.Warn("an on-call program's declaration was not kept on disk", "program", id, "err", err)
	}
	if err := d.kernel.Rest(decl); err != nil {
		d.log.Warn("an on-call program's declaration was not kept", "program", id, "err", err)
		return
	}
	d.oncall.mu.Lock()
	d.oncall.kept[id] = bin
	declareRun := d.oncall.reading[id] != ""
	delete(d.oncall.reading, id)
	d.oncall.mu.Unlock()
	if declareRun {
		// Stopped unless a call came for it meanwhile.
		_ = d.super.Rest(id)
	}
}

// startOnCall is the call's half of section 54. For an on-call program it
// counts the call for as long as it runs, which is what tells an idle exit
// from a crash, and starts the program if it is at rest. It answers nil
// for any other program.
func (d *Daemon) startOnCall(ctx context.Context, program string) (done func(), bad *callFailure) {
	spec, ok := d.onCallSpec(program)
	if !ok {
		return func() {}, nil
	}
	d.refreshKept(program)
	ready, done, err := d.super.Call(program)
	if errors.Is(err, supervise.ErrQuarantined) {
		return nil, &callFailure{status: &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      program + " is quarantined, so this call did not start it",
			Actual:       err.Error(),
			Precondition: "the program has restart budget left",
			Fix:          "read why, then restart it by hand",
			FixCommand:   "rig restart " + program,
		}}
	}
	if err != nil {
		return nil, failure(rigv1.Code_CODE_INTERNAL, err.Error())
	}
	// The supervisor ends a start that does not register at this bound, on
	// its interval; the margin covers the interval.
	wait := spec.Health.Register
	if wait <= 0 {
		wait = supervise.DefaultHealth().Register
	}
	timer := time.NewTimer(wait + supervise.DefaultHealth().Interval)
	defer timer.Stop()
	select {
	case err = <-ready:
	case <-ctx.Done():
		done()
		return nil, failure(rigv1.Code_CODE_DEADLINE,
			program+" did not start within this call's deadline")
	case <-timer.C:
		err = &supervise.StartError{Program: program, Reason: "did not register within " + wait.String()}
	}
	if err == nil {
		return done, nil
	}
	done()
	st := &rigv1.Status{
		Code:       rigv1.Code_CODE_UNAVAILABLE,
		Message:    err.Error(),
		Fix:        "read its health and its log",
		FixCommand: "rig health " + program,
	}
	var se *supervise.StartError
	if errors.As(err, &se) {
		st.Actual = "it wrote nothing to stderr"
		if se.Stderr != "" {
			st.Actual = "its last words: " + se.Stderr
		}
	}
	return nil, &callFailure{status: st}
}

// atRest says a listed program is an on-call one with no process behind it.
func (d *Daemon) atRest(id string) bool {
	_, ok := d.onCallSpec(id)
	return ok && !d.connected(id)
}
