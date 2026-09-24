package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/borismilner/rig/internal/instance"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// upIncarnation is upEstate with an epoch, which is the one thing that tells
// two runs of the same estate apart.
//
// The daemon does not open the store and must not: rigd opens it under the
// name claim, before anything binds, and a second opener of a file that must
// have exactly one is the failure the claim exists to prevent. So the epoch
// arrives on Config, exactly as it does in main, and this helper is the test's
// way of saying "a daemon that started for the Nth time".
func upIncarnation(t *testing.T, name string, epoch uint64) string {
	t.Helper()
	// Short deliberately: sun_path is 108 bytes and t.TempDir under a long
	// TMPDIR silently exceeds it, failing as EINVAL.
	dir, err := os.MkdirTemp("", "rigi")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{
		Version: "test-build", Wire: "v1", Estate: name, Epoch: epoch, Lock: lock,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock
}

// TestARestartIsVisibleOnTheWire is the wire half of the defect demonstrated
// 2026-09-16: one daemon restarted over a shared XDG_STATE_HOME and every path
// an agent could read was byte-identical before and after.
//
// Both halves are asserted on purpose. That the epoch MOVED is the easy one.
// That NOTHING ELSE moved is what keeps this an identity: a restart is not a
// different estate, and an agent that re-derived its routing from a changed
// name or role would be worse off than one that could not tell at all.
func TestARestartIsVisibleOnTheWire(t *testing.T) {
	before := estateOf(t, upIncarnation(t, "production", 1))
	after := estateOf(t, upIncarnation(t, "production", 2))

	if before.GetEpoch() != 1 || after.GetEpoch() != 2 {
		t.Fatalf("the wire reported epoch %d then %d, want 1 then 2",
			before.GetEpoch(), after.GetEpoch())
	}
	if before.GetName() != after.GetName() ||
		before.GetRole() != after.GetRole() ||
		before.GetDaemonVersion() != after.GetDaemonVersion() ||
		before.GetWire() != after.GetWire() ||
		before.GetSemanticsGen() != after.GetSemanticsGen() {
		t.Errorf("a restart changed more than the epoch, so it is being "+
			"reported as a different estate:\n  before %v\n  after  %v", before, after)
	}
}

// TestTheTwoSurfacesAgreeAboutTheEpoch turns a comment into a test.
//
// EstateIdentity in meta.go says it out loud: "IT IS THE SAME ANSWER
// rig.estate GIVES ON THE WIRE, BUILT FROM THE SAME FIELDS... If a field is
// added to EstateResponse, it is added here in the same change." The epoch is
// the field where breaking that would be worst - an agent on the MCP surface
// learning it was restarted under while an agent on the wire does not, same
// daemon, same instant - so the rule gets an assertion rather than a paragraph.
func TestTheTwoSurfacesAgreeAboutTheEpoch(t *testing.T) {
	const epoch = 9

	dir, err := os.MkdirTemp("", "rigi")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{
		Version: "test-build", Wire: "v1", Estate: "production", Epoch: epoch, Lock: lock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := d.EstateIdentity().Epoch; got != epoch {
		t.Errorf("the MCP surface reports epoch %d, the daemon published %d", got, epoch)
	}

	onTheWire := estateOf(t, upIncarnation(t, "production", epoch))
	if onTheWire.GetEpoch() != d.EstateIdentity().Epoch {
		t.Errorf("the two surfaces disagree: wire %d, MCP %d",
			onTheWire.GetEpoch(), d.EstateIdentity().Epoch)
	}
}

// TestAnUnnamedEstateReportsEpochZero is the case that has no store to bump.
//
// Zero is an answer rather than a hole, and it cannot collide with a real one:
// the store bumps before it publishes, so the first epoch any daemon can carry
// is 1. proto3 cannot tell an unset scalar from a zero one, which is the same
// reason the role enum keeps UNSPECIFIED as its zero - but here the two
// readings agree, because a daemon that failed to set the field really does
// have no epoch to report.
func TestAnUnnamedEstateReportsEpochZero(t *testing.T) {
	got := estateOf(t, upIncarnation(t, "", 0))
	if got.GetEpoch() != 0 {
		t.Errorf("an unnamed estate reported epoch %d, and it opens no store "+
			"to have got one from", got.GetEpoch())
	}
	if got.GetRole() != rigv1.EstateRole_ESTATE_ROLE_UNNAMED {
		t.Errorf("role is %v, want UNNAMED", got.GetRole())
	}
}
