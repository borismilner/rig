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

	// scanning is a background scan under way, and again that another was
	// asked for meanwhile, so a burst of listings costs at most two scans.
	scanning bool
	again    bool
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

// kept is one kept declaration: the hello, the binary it was read from, and
// that binary's identity when it was.
type kept struct {
	hello *rigv1.HelloRequest
	bin   string
	path  string
}

// readKept reads the kept file at file. Its first line is the binary's
// identity and path, tab-separated; the rest is the hello.
func readKept(file string) (kept, bool) {
	b, err := os.ReadFile(file)
	if err != nil {
		return kept{}, false
	}
	head, body, ok := strings.Cut(string(b), "\n")
	if !ok {
		return kept{}, false
	}
	var k kept
	k.bin, k.path, _ = strings.Cut(head, "\t")
	k.hello = &rigv1.HelloRequest{}
	if err := proto.Unmarshal([]byte(body), k.hello); err != nil {
		return kept{}, false
	}
	return k, true
}

// loadKept answers the kept hello for id if it was read from this binary.
func (d *Daemon) loadKept(id, bin string) (*rigv1.HelloRequest, bool) {
	path := d.keptPath(id)
	if path == "" {
		return nil, false
	}
	k, ok := readKept(path)
	if !ok || k.bin != bin {
		return nil, false
	}
	return k.hello, true
}

// saveKept writes the hello, behind the identity it was read from, through a
// temporary file so a reader never sees half of one.
func (d *Daemon) saveKept(id, bin, binPath string, req *rigv1.HelloRequest) error {
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
	if err := os.WriteFile(tmp, append([]byte(bin+"\t"+binPath+"\n"), body...), 0o600); err != nil {
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

// adoptKept is rigd starting, before any scan (plan/54, 2026-10-03): every
// program a scan found before is declared again from what was kept, so it
// is listed and callable at once. On call goes to rest; resident is
// started, as autostart would.
func (d *Daemon) adoptKept() {
	if d.super == nil || d.keptDir == "" {
		return
	}
	entries, err := os.ReadDir(d.keptDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".hello")
		if !ok {
			continue
		}
		if _, declared := d.super.Spec(id); declared {
			continue // programs.json declares it; refreshKept reads its file
		}
		k, ok := readKept(filepath.Join(d.keptDir, e.Name()))
		if !ok || k.path == "" {
			continue
		}
		spec, _ := supervise.Merge(d.overridesFor(id), map[string]string{id: k.path})
		if len(spec) != 1 || d.super.Declare(spec) != nil {
			continue
		}
		decl, err := declarationFromWire(k.hello)
		if err != nil {
			continue
		}
		d.applyLoad(id, decl)
		d.oncall.mu.Lock()
		d.oncall.kept[id] = k.bin
		d.oncall.mu.Unlock()
	}
}

// overridesFor is programs.json's override row for id, if it has one.
func (d *Daemon) overridesFor(id string) []supervise.Spec {
	for _, o := range d.overrides {
		if o.ID == id {
			return []supervise.Spec{o}
		}
	}
	return nil
}

// applyLoad puts a program in the mode its declaration asks for, unless
// programs.json set one: on call is kept and put at rest; resident is
// started, or left running when a declare run found it (decision 0265).
func (d *Daemon) applyLoad(id string, decl kernel.Declaration) {
	spec, ok := d.super.Spec(id)
	if !ok {
		return
	}
	onCall := spec.OnCall
	if spec.FromBinary {
		onCall = decl.Load.OnCall()
		_ = d.super.SetLoad(id, onCall)
	}
	if !onCall {
		d.kernel.Unrest(id)
		if !d.connected(id) {
			_, _ = d.super.Up(id)
		}
		return
	}
	if d.kernel.Rest(decl) == nil {
		d.restIfDown(id)
	}
}

// rescan asks for a background scan and returns at once: the scan never
// blocks a listing, a describe or a call (plan/54, 2026-10-03).
func (d *Daemon) rescan() {
	if d.super == nil {
		return
	}
	d.oncall.mu.Lock()
	if d.oncall.scanning {
		d.oncall.again = true
		d.oncall.mu.Unlock()
		return
	}
	d.oncall.scanning = true
	d.oncall.mu.Unlock()
	go func() {
		for {
			d.scan()
			d.oncall.mu.Lock()
			if !d.oncall.again {
				d.oncall.scanning = false
				d.oncall.mu.Unlock()
				return
			}
			d.oncall.again = false
			d.oncall.mu.Unlock()
		}
	}()
}

// scan is one pass: a new binary is declared and read, a changed one is
// read again, and a scanned program whose binary is gone is forgotten,
// kept file and all.
func (d *Daemon) scan() {
	if d.scanHold != nil {
		<-d.scanHold
	}
	if ctx := d.serving.Load(); ctx != nil && (*ctx).Err() != nil {
		return // shutting down: a declare run now would outlive rigd
	}
	found, problems := supervise.Scan(d.scanDirs)
	for _, p := range problems {
		d.log.Warn("program scan", "err", p)
	}
	// A directory that could not be read says nothing about what was
	// deleted, so nothing is forgotten on a pass that failed to read one.
	if !unreadable(problems) {
		for _, id := range d.super.Declared() {
			spec, ok := d.super.Spec(id)
			if ok && spec.Scanned && found[id] == "" {
				d.forget(id)
			}
		}
	}
	specs, problems := supervise.Merge(d.overrides, found)
	for _, p := range problems {
		d.log.Warn("program scan", "err", p)
	}
	for _, spec := range specs {
		if _, declared := d.super.Spec(spec.ID); declared || !spec.Scanned {
			continue
		}
		if err := d.super.Declare([]supervise.Spec{spec}); err != nil {
			d.log.Warn("a scanned program was not declared", "program", spec.ID, "err", err)
			continue
		}
		d.log.Info("program found", "program", spec.ID, "binary", spec.Path)
	}
	for _, id := range d.super.Declared() {
		d.refreshKept(id)
	}
}

// unreadable is whether a scan failed to read a directory, as opposed to
// finding something in one it would not declare.
func unreadable(problems []error) bool {
	for _, p := range problems {
		if errors.Is(p, supervise.ErrScanUnreadable) {
			return true
		}
	}
	return false
}

// forget removes a scanned program whose binary is gone: from the
// supervisor, from what is listed, and from what is kept.
func (d *Daemon) forget(id string) {
	d.super.Forget(id)
	d.kernel.Unrest(id)
	if path := d.keptPath(id); path != "" {
		_ = os.Remove(path)
	}
	d.oncall.mu.Lock()
	delete(d.oncall.kept, id)
	delete(d.oncall.reading, id)
	d.oncall.mu.Unlock()
	d.log.Info("program removed: its binary is gone", "program", id)
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
	spec, ok := d.super.Spec(id)
	if !ok || (!spec.OnCall && !spec.Scanned) {
		return
	}
	bin, err := binaryID(spec.Path)
	if err == nil {
		err = d.saveKept(id, bin, spec.Path, req)
	}
	if err != nil {
		d.log.Warn("a program's declaration was not kept on disk", "program", id, "err", err)
	}
	d.oncall.mu.Lock()
	d.oncall.kept[id] = bin
	declareRun := d.oncall.reading[id] != ""
	delete(d.oncall.reading, id)
	d.oncall.mu.Unlock()

	onCall := spec.OnCall
	if spec.FromBinary {
		onCall = decl.Load.OnCall()
		_ = d.super.SetLoad(id, onCall)
	}
	if !onCall {
		// Resident: this run is its first, and it stays up.
		d.kernel.Unrest(id)
		return
	}
	if err := d.kernel.Rest(decl); err != nil {
		d.log.Warn("an on-call program's declaration was not kept", "program", id, "err", err)
		return
	}
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
