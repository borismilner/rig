package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/54, closing the two gaps: a resident is listed while it is down, and
// a rebuilt resident's binary is read by the scan, not at its next start.

// stopStub stops the stub as a human would, and waits for it to be off the
// table.
func (r *onCallRig) stopStub(t *testing.T) {
	t.Helper()
	if err := dial(t, r.sock).Call(ctx5(t), "rig.stop",
		&verbsv1.StopRequest{Program: "stub"}, &verbsv1.StopResponse{}); err != nil {
		t.Fatalf("rig.stop: %v", err)
	}
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_UNSPECIFIED)
	r.waitGone(t)
}

// waitGone waits until the stub's connection is gone, so "down" is about
// the daemon and not a race with its departure.
func (r *onCallRig) waitGone(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for r.d.connected("stub") {
		if time.Now().After(deadline) {
			t.Fatal("the stub never disconnected")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitStarts waits for the stub's launch count to reach n.
func (r *onCallRig) waitStarts(t *testing.T, n int32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for r.starts.Load() != n {
		if time.Now().After(deadline) {
			t.Fatalf("%d launches, want %d", r.starts.Load(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fixOf is the fix command a refusal carries.
func fixOf(t *testing.T, err error) string {
	t.Helper()
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("%v is not a refusal", err)
	}
	return ce.Status.GetFixCommand()
}

func (r *onCallRig) rebuild(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(r.bin, []byte("v2, a different size"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A stopped resident stays listed, down, with its commands, and a call to
// it says it is stopped and how to start it, and starts nothing. The red
// control is the same stub before it was stopped: listed, not down.
func TestAStoppedResidentIsListedDownAndACallSaysHowToStartIt(t *testing.T) {
	r := upOnCall(t, false)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	if p := r.listed(t); p == nil || p.GetDown() {
		t.Fatalf("a running resident is listed as %v, want not down", p)
	}
	r.stopStub(t)

	p := r.listed(t)
	if p == nil || !p.GetDown() || p.GetAtRest() || len(p.GetCommands()) == 0 {
		t.Fatalf("a stopped resident is listed as %v, want down with its commands", p)
	}
	err := dial(t, r.sock).Call(ctx5(t), "stub.ping", &rigv1.PingRequest{}, &rigv1.PingResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "a call to a stopped resident")
	if fix := fixOf(t, err); fix != "rig up stub" {
		t.Fatalf("the refusal's fix is %q, want rig up stub", fix)
	}
	if n := r.starts.Load(); n != 1 {
		t.Fatalf("a call started a stopped resident: %d launches", n)
	}
}

// A running resident whose binary changed is restarted on it by the scan a
// listing asks for. The red control is the unchanged binary, listed again,
// which restarts nothing.
func TestARunningResidentsRebuiltBinaryRestartsIt(t *testing.T) {
	r := upOnCall(t, false)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	r.listed(t)
	time.Sleep(100 * time.Millisecond)
	if n := r.starts.Load(); n != 1 {
		t.Fatalf("listing an unchanged resident restarted it: %d launches", n)
	}

	r.rebuild(t)
	r.listed(t)
	r.waitStarts(t, 2)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	if p := r.listed(t); p == nil || p.GetDown() || p.GetStale() {
		t.Fatalf("after the restart the stub is listed as %v, want up", p)
	}
	h := healthOf(ctx5(t), t, r.sock, "stub")
	found := false
	for _, e := range h.GetHistory() {
		if e.GetActor() == "rig" && e.GetTrigger() == "its binary changed on disk" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the restart is not in the history as rig's: %v", h.GetHistory())
	}
}

// A stopped resident whose binary changed is run once to read it, and is
// stopped again: a human's stop stands.
func TestAStoppedResidentsRebuiltBinaryIsReadAndStaysStopped(t *testing.T) {
	r := upOnCall(t, false)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	r.stopStub(t)

	r.rebuild(t)
	r.listed(t)
	r.waitStarts(t, 2)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_UNSPECIFIED)
	r.waitGone(t)
	r.listed(t)
	time.Sleep(100 * time.Millisecond)
	if n := r.starts.Load(); n != 2 {
		t.Fatalf("the read binary was read again: %d launches", n)
	}
	if p := r.listed(t); p == nil || !p.GetDown() {
		t.Fatalf("after the read the stub is listed as %v, want down", p)
	}
}

// A quarantined resident whose binary changed is left for a human, and
// listed stale.
func TestAQuarantinedResidentsRebuiltBinaryIsListedStale(t *testing.T) {
	r := upOnCall(t, false)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	for range 2 {
		if err := r.d.super.Panicked("stub", "a test"); err != nil {
			t.Fatal(err)
		}
	}
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_QUARANTINED)
	r.waitGone(t)
	if p := r.listed(t); p == nil || !p.GetDown() || p.GetStale() {
		t.Fatalf("a quarantined resident is listed as %v, want down and not stale", p)
	}

	r.rebuild(t)
	r.listed(t)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if p := r.listed(t); p != nil && p.GetStale() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a quarantined resident's rebuilt binary was never listed stale")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := r.starts.Load(); n != 1 {
		t.Fatalf("rig started a quarantined program: %d launches", n)
	}
	err := dial(t, r.sock).Call(ctx5(t), "stub.ping", &rigv1.PingRequest{}, &rigv1.PingResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "a call to a quarantined resident")
	if fix := fixOf(t, err); fix != "rig restart stub" {
		t.Fatalf("a call to a quarantined resident's fix is %q, want rig restart stub", fix)
	}
}

// A programs.json resident's kept declaration is not a scanned program's:
// a rigd with no row for it does not declare or start it from the file.
// The red control is TestWhatWasLearnedIsServedWithoutARead, where a
// scanned one's is.
func TestAConfigResidentsKeptFileDeclaresNothingWithoutItsRow(t *testing.T) {
	r := upOnCall(t, false)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	kept := filepath.Join(filepath.Dir(r.bin), "declarations")
	if _, err := os.Stat(filepath.Join(kept, "stub.hello")); err != nil {
		t.Fatalf("a programs.json resident's declaration was not kept: %v", err)
	}

	hold := make(chan struct{})
	defer close(hold)
	next := scanRigHeld(t, t.TempDir(), kept, rigv1.Load_LOAD_RESIDENT, hold)
	if p := next.listed(t); p != nil {
		t.Fatalf("without its row the stub was listed as %v", p)
	}
	if n := next.starts.Load(); n != 0 {
		t.Fatalf("without its row the stub was launched %d times", n)
	}
}

// The same for a resident a scan found: stopped by a human, rebuilt, read,
// and stopped again. Its first run, by contrast, stays up
// (TestAScannedBinaryDeclaringResidentStaysUp).
func TestAStoppedScannedResidentsRebuiltBinaryStaysStopped(t *testing.T) {
	dir, kept := scanDirs(t)
	r := scanRig(t, dir, kept, rigv1.Load_LOAD_RESIDENT)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	r.stopStub(t)
	before := r.starts.Load()

	if err := os.WriteFile(r.bin, []byte("v2, a different size"), 0o700); err != nil {
		t.Fatal(err)
	}
	r.listed(t)
	r.waitStarts(t, before+1)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_UNSPECIFIED)
	r.waitGone(t)
	if p := r.listed(t); p == nil || !p.GetDown() {
		t.Fatalf("after the read the scanned stub is listed as %v, want down", p)
	}
}
