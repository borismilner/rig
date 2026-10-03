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
// the call that needs it. A resident's declaration is kept the same way, so
// it is listed while down, and a call to it is told why it is not answered.

// onCallState is what the daemon knows about its on-call programs' kept
// declarations. Its own lock: it is read on every call to one of them.
type onCallState struct {
	mu sync.Mutex
	// kept is the binary identity each kept declaration was read from.
	kept map[string]string
	// reading is the binary identity a run is reading now, so one identity
	// is read once, and a binary that fails to start is not relaunched by
	// every listing.
	reading map[string]read
	// helloOf is the connection whose hello was kept, so a scan never takes
	// a program that registered a moment ago, its hello not yet kept, for
	// one running an old binary.
	helloOf map[string]*conn
	// stale is a quarantined resident whose binary changed: what is listed
	// was read from the binary before.
	stale map[string]bool

	// scanning is a background scan under way, and again that another was
	// asked for meanwhile, so a burst of listings costs at most two scans.
	scanning bool
	again    bool
}

// read is one run reading a binary's declaration. A declare run is stopped
// once its hello is kept; a reload of a running resident stays up.
type read struct {
	bin     string
	declare bool
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
// that binary's identity when it was. scanned is whether a scan found it,
// which is what rigd may declare again from the file alone: a programs.json
// row taken out must not be started by what was kept for it.
type kept struct {
	hello   *rigv1.HelloRequest
	bin     string
	path    string
	scanned bool
}

// keptFromConfig is the origin field of a kept file whose program a
// programs.json row declared. A file with none predates the field, when
// only scanned and on-call programs were kept.
const keptFromConfig = "config"

// readKept reads the kept file at file. Its first line is the binary's
// identity, its path and its origin, tab-separated; the rest is the hello.
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
	var origin string
	k.bin, k.path, _ = strings.Cut(head, "\t")
	k.path, origin, _ = strings.Cut(k.path, "\t")
	k.scanned = origin != keptFromConfig
	k.hello = &rigv1.HelloRequest{}
	if err := proto.Unmarshal([]byte(body), k.hello); err != nil {
		return kept{}, false
	}
	return k, true
}

// saveKept writes the hello, behind the identity it was read from, through a
// temporary file so a reader never sees half of one.
func (d *Daemon) saveKept(id, bin string, spec supervise.Spec, req *rigv1.HelloRequest) error {
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
	origin := "scan"
	if !spec.Scanned {
		origin = keptFromConfig
	}
	head := bin + "\t" + spec.Path + "\t" + origin + "\n"
	if err := os.WriteFile(tmp, append([]byte(head), body...), 0o600); err != nil {
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
		if !ok || k.path == "" || !k.scanned {
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
// programs.json set one: on call is put at rest; resident is started, or
// left running when a declare run found it (decision 0265). Either way its
// declaration is listed while it is down.
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
	if d.kernel.Rest(decl) != nil {
		return
	}
	if !onCall {
		if !d.connected(id) {
			_, _ = d.super.Up(id)
		}
		return
	}
	d.restIfDown(id)
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
	delete(d.oncall.helloOf, id)
	delete(d.oncall.stale, id)
	d.oncall.mu.Unlock()
	d.log.Info("program removed: its binary is gone", "program", id)
}

// refreshKept makes sure id's kept declaration was read from the binary on
// disk now: kept as it is, loaded from the file, or read by running it.
// One stat when nothing changed.
func (d *Daemon) refreshKept(id string) {
	spec, ok := d.super.Spec(id)
	if !ok || spec.Path == "" {
		return
	}
	bin, err := binaryID(spec.Path)
	if err != nil {
		// The binary is gone. What was kept stays listed; a call will start
		// nothing and say why.
		if spec.OnCall {
			d.restIfDown(id)
		}
		return
	}
	d.oncall.mu.Lock()
	switch {
	case d.oncall.kept[id] == bin, d.oncall.reading[id].bin == bin:
		d.oncall.mu.Unlock()
		return
	}
	d.oncall.mu.Unlock()

	k, had := d.readKeptFile(id)
	if had && k.bin == bin {
		if decl, err := declarationFromWire(k.hello); err == nil && d.kernel.Rest(decl) == nil {
			d.oncall.mu.Lock()
			d.oncall.kept[id] = bin
			delete(d.oncall.stale, id)
			d.oncall.mu.Unlock()
			if spec.OnCall {
				d.restIfDown(id)
			}
			return
		}
	}
	if !spec.OnCall {
		d.rereadResident(id, bin, k, had)
		return
	}
	if d.connected(id) {
		// Its next hello is read from whatever binary it is.
		return
	}
	d.reread(id, spec, read{bin: bin, declare: true})
}

// readKeptFile is id's kept file, whichever binary it was read from.
func (d *Daemon) readKeptFile(id string) (kept, bool) {
	path := d.keptPath(id)
	if path == "" {
		return kept{}, false
	}
	return readKept(path)
}

// rereadResident is a resident whose binary is not the one its kept
// declaration was read from (plan/54, closing the two gaps). Running, it is
// restarted on the new binary; stopped, it is run once to read it; while
// it restarts on its own, its next hello reads it; quarantined, it waits
// for a human and is listed stale. A resident never kept and not running is
// left alone: it is listed from its first run.
func (d *Daemon) rereadResident(id, bin string, k kept, had bool) {
	if had {
		// What it declared before stays listed until the new one is read.
		if decl, err := declarationFromWire(k.hello); err == nil {
			_ = d.kernel.Rest(decl)
		}
	}
	spec, _ := d.super.Spec(id)
	d.mu.RLock()
	running := d.programs[id]
	d.mu.RUnlock()
	if running != nil {
		d.oncall.mu.Lock()
		ours := d.oncall.helloOf[id] == running && d.oncall.kept[id] != ""
		d.oncall.mu.Unlock()
		if ours {
			// Its hello was kept, from another binary: restart it.
			d.reread(id, spec, read{bin: bin})
		}
		return
	}
	st, err := d.super.Health(id)
	if err != nil || len(st) != 1 {
		return
	}
	switch st[0].State {
	case supervise.StateQuarantined:
		if had {
			d.oncall.mu.Lock()
			d.oncall.stale[id] = true
			d.oncall.mu.Unlock()
		}
	case supervise.StateUnspecified:
		if had {
			d.reread(id, spec, read{bin: bin, declare: true})
		}
	default:
		// Starting or restarting: its own hello is read.
	}
}

// reread runs id to read its binary: a declare run starts it, and a reload
// restarts the running one.
func (d *Daemon) reread(id string, spec supervise.Spec, r read) {
	d.oncall.mu.Lock()
	d.oncall.reading[id] = r
	d.oncall.mu.Unlock()
	if !r.declare {
		d.log.Info("restarting a program: its binary changed", "program", id, "binary", spec.Path)
		if err := d.super.Reload(id); err != nil {
			d.log.Warn("the restart did not happen", "program", id, "err", err)
		}
		return
	}
	d.log.Info("reading a program's declaration", "program", id, "binary", spec.Path)
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

// keptHello is a supervised program's hello: its declaration is kept,
// whether this start was a declare run, a call or a resident's run, and a
// declare run ends here.
func (d *Daemon) keptHello(c *conn, req *rigv1.HelloRequest, decl kernel.Declaration) {
	id, ok := d.supervisedChild(c)
	if !ok {
		return
	}
	spec, ok := d.super.Spec(id)
	if !ok {
		return
	}
	bin, err := binaryID(spec.Path)
	if err == nil {
		err = d.saveKept(id, bin, spec, req)
	}
	if err != nil {
		d.log.Warn("a program's declaration was not kept on disk", "program", id, "err", err)
	}
	d.oncall.mu.Lock()
	d.oncall.kept[id] = bin
	d.oncall.helloOf[id] = c
	declareRun := d.oncall.reading[id].declare
	delete(d.oncall.reading, id)
	delete(d.oncall.stale, id)
	d.oncall.mu.Unlock()

	onCall := spec.OnCall
	if spec.FromBinary {
		onCall = decl.Load.OnCall()
		_ = d.super.SetLoad(id, onCall)
	}
	if err := d.kernel.Rest(decl); err != nil {
		d.log.Warn("a program's declaration was not kept", "program", id, "err", err)
		return
	}
	if !declareRun {
		return
	}
	if onCall {
		// Stopped unless a call came for it meanwhile.
		_ = d.super.Rest(id)
		return
	}
	// A resident run only to read it. One a scan found was held on call
	// until now, so this run is its start and stays up; one that was
	// already resident was stopped, and a human's stop stands.
	if !spec.OnCall {
		_ = d.super.EndRead(id)
	}
}

// startOnCall is the call's half of section 54. For an on-call program it
// counts the call for as long as it runs, which is what tells an idle exit
// from a crash, and starts the program if it is at rest. It answers nil
// for any other program.
func (d *Daemon) startOnCall(ctx context.Context, program string) (done func(), bad *callFailure) {
	spec, ok := d.onCallSpec(program)
	if !ok {
		return func() {}, d.downFailure(program)
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

// downFailure is the answer to a call to a resident that is down, nil for
// any other program: a call never starts a resident, so it says why it is
// down and what starts it (plan/54, closing the two gaps).
func (d *Daemon) downFailure(program string) *callFailure {
	if !d.down(program) {
		return nil
	}
	st := &rigv1.Status{
		Code:         rigv1.Code_CODE_UNAVAILABLE,
		Message:      program + " is not running, and a call does not start a resident program",
		Precondition: "the program is running",
		Fix:          "start it",
		FixCommand:   "rig up " + program,
	}
	if h, err := d.super.Health(program); err == nil && len(h) == 1 {
		switch h[0].State {
		case supervise.StateUnspecified:
			st.Actual = "it is stopped"
		case supervise.StateQuarantined:
			st.Actual = "it is quarantined"
			st.Fix, st.FixCommand = "read why, then restart it by hand", "rig restart "+program
		default:
			st.Actual = "it is " + h[0].State.String()
			st.Fix, st.FixCommand = "wait for it, or read its health", "rig health "+program
		}
	}
	return &callFailure{status: st}
}

// down says a listed program is a supervised resident with no process
// behind it: what is listed is its kept declaration.
func (d *Daemon) down(id string) bool {
	if d.super == nil {
		return false
	}
	spec, ok := d.super.Spec(id)
	return ok && !spec.OnCall && !d.connected(id)
}

// stale says what is listed for id was read from a binary since changed.
func (d *Daemon) stale(id string) bool {
	d.oncall.mu.Lock()
	defer d.oncall.mu.Unlock()
	return d.oncall.stale[id]
}

// atRest says a listed program is an on-call one with no process behind it.
func (d *Daemon) atRest(id string) bool {
	_, ok := d.onCallSpec(id)
	return ok && !d.connected(id)
}
