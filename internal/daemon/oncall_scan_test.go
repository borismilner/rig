package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/supervise"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Decision 0265 and plan/54's 2026-10-03 requirement over the wire: rig
// finds the stub in a scan directory, the stub's own declaration says how
// it is loaded, what rig learned is kept and served at the next start
// before any scan, and a deleted binary's program is removed.

// scanRig is a daemon that scans dir and keeps declarations in kept. Two of
// them over the same kept directory are two rigd runs.
func scanRig(t *testing.T, dir, kept string, load rigv1.Load, overrides ...supervise.Spec) *onCallRig {
	t.Helper()
	return scanRigHeld(t, dir, kept, load, nil, overrides...)
}

func scanRigHeld(t *testing.T, dir, kept string, load rigv1.Load, hold chan struct{}, overrides ...supervise.Spec) *onCallRig {
	t.Helper()
	run, err := os.MkdirTemp("", "rigsc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(run) })
	r := &onCallRig{sock: filepath.Join(run, "s"), bin: filepath.Join(dir, "stub")}
	r.load.Store(int32(load))
	sup := supervise.New(supervise.Options{Start: r.start})
	l, err := net.Listen("unix", r.sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(run, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	r.d, err = New(Config{
		Version: "test", Wire: "v1", Lock: lock, Supervisor: sup,
		Declarations: kept, Scan: []string{dir}, Overrides: overrides,
	})
	if err != nil {
		t.Fatal(err)
	}
	r.d.scanHold = hold
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = r.d.Serve(ctx, l) }()
	stop := func() {
		cancel()
		<-done
		sup.StopAll()
		if r.d.records != nil {
			_ = r.d.records.Close()
		}
	}
	r.stop = stop
	t.Cleanup(r.stopOnce)
	return r
}

func scanDirs(t *testing.T) (dir, kept string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stub"), []byte("v1"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir, filepath.Join(t.TempDir(), "declarations")
}

// A binary in a scan directory is a program with no programs.json row,
// and declaring on call is enough for rig to keep it at rest.
func TestAScannedBinaryDeclaringOnCallIsKeptAtRest(t *testing.T) {
	dir, kept := scanDirs(t)
	r := scanRig(t, dir, kept, rigv1.Load_LOAD_ON_CALL)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	if p := r.listed(t); p == nil || !p.GetAtRest() || p.GetLoad() != rigv1.Load_LOAD_ON_CALL {
		t.Fatalf("listed %v, want at rest and on call", p)
	}
	if _, err := os.Stat(filepath.Join(kept, "stub.hello")); err != nil {
		t.Fatalf("the declaration was not kept: %v", err)
	}
}

// The red control: the same binary declaring resident is left running by
// its declare run, which was its first run.
func TestAScannedBinaryDeclaringResidentStaysUp(t *testing.T) {
	dir, kept := scanDirs(t)
	r := scanRig(t, dir, kept, rigv1.Load_LOAD_RESIDENT)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	time.Sleep(50 * time.Millisecond)
	if h := healthOf(ctx5(t), t, r.sock, "stub"); h.GetState() != verbsv1.ProgramState_PROGRAM_STATE_HEALTHY || r.starts.Load() != 1 {
		t.Fatalf("resident stub is %s after %d launches, want HEALTHY after one", h.GetState(), r.starts.Load())
	}
}

// programs.json's override wins over what the binary declares.
func TestAnOverrideWinsOverTheBinarysLoadMode(t *testing.T) {
	dir, kept := scanDirs(t)
	r := scanRig(t, dir, kept, rigv1.Load_LOAD_RESIDENT, supervise.Spec{ID: "stub", OnCall: true})
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
}

// What rig learned is served at its next start before any scan: the
// second daemon lists the stub, at rest, without launching it. The red
// control is the same second start with nothing kept, which must read it.
func TestWhatWasLearnedIsServedWithoutARead(t *testing.T) {
	dir, kept := scanDirs(t)
	first := scanRig(t, dir, kept, rigv1.Load_LOAD_ON_CALL)
	first.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	first.stopOnce()

	hold := make(chan struct{})
	second := scanRigHeld(t, dir, kept, rigv1.Load_LOAD_ON_CALL, hold)
	if p := second.listed(t); p == nil || !p.GetAtRest() {
		t.Fatalf("before any scan the second start listed %v, want the kept stub at rest", p)
	}
	close(hold)
	time.Sleep(100 * time.Millisecond) // the background scan has run
	if n := second.starts.Load(); n != 0 {
		t.Fatalf("the second start launched the stub %d times; what was kept should have served", n)
	}

	cold := scanRig(t, dir, filepath.Join(t.TempDir(), "declarations"), rigv1.Load_LOAD_ON_CALL)
	cold.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	if cold.starts.Load() != 1 {
		t.Fatalf("with nothing kept the stub was launched %d times, want one declare run", cold.starts.Load())
	}
}

// A deleted binary's program is removed, from the listing and from what
// is kept, at the next scan.
func TestADeletedBinaryIsForgotten(t *testing.T) {
	dir, kept := scanDirs(t)
	r := scanRig(t, dir, kept, rigv1.Load_LOAD_ON_CALL)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	if err := os.Remove(r.bin); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for r.listed(t) != nil {
		if time.Now().After(deadline) {
			t.Fatal("a deleted binary's program is still listed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(kept, "stub.hello")); !os.IsNotExist(err) {
		t.Fatalf("its kept declaration is still on disk: %v", err)
	}
}

// An unreadable scan directory is not a deleted binary: nothing is
// forgotten on a pass that could not read one. The red control is the
// deleted-binary test above.
func TestAnUnreadableScanDirectoryForgetsNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a directory whatever its mode")
	}
	dir, kept := scanDirs(t)
	r := scanRig(t, dir, kept, rigv1.Load_LOAD_ON_CALL)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	r.listed(t)
	time.Sleep(100 * time.Millisecond) // the background scan has run
	if r.listed(t) == nil {
		t.Fatal("an unreadable scan directory made rig forget the stub")
	}
}
