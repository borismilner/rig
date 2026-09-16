package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/boris-milner/rig/internal/instance"
)

// THE FIELD THIS FILE COVERS IS THE REASON PRESENCE IS RANK 1, and until this
// file existed nothing in the repository tested it in the direction that
// matters.
//
// Section 37's minimum beneficial set names presence row 1 and gives the
// reason in one clause: "`partial: true` is what stopped a seat concluding it
// was alone". The backlog's measured table says the same thing in the same
// words. So `partial` is not one field among seven on the roster - it is the
// justification for the row.
//
// MEASURED, by mutating the mechanism and running the whole repository's
// suite. Three mutations of `partial` were applied one at a time and every
// one of them left `go test ./...` GREEN:
//
//   - `partial()` hardcoded to false, so a seat is always told the roster is
//     complete
//   - `partial()` hardcoded to true
//   - both wire handlers sending `Partial: false` regardless of what presence
//     computed
//
// The suite did catch the mechanism UNDERNEATH it - counting claim files
// instead of testing the lock fails TestAStaleEstateClaimIsNotCountedAsAPeer -
// but that test calls `otherEstates` directly. Nothing connected the answer to
// the wire, so every mutation between the two was free.
//
// THE FIRST MUTATION IS THE ONE THAT MATTERS AND THE ASYMMETRY IS THE POINT.
// A `partial` stuck at true is a nuisance: a seat coordinates when it did not
// have to. A `partial` stuck at false is the exact defect the row exists to
// prevent, it is silent, and it is indistinguishable from an honest empty
// estate. A conformance case that only pins the loud direction has not tested
// the mechanism.

// TestAnEmptyRosterIsNotAClaimThatNobodyElseIsThere is the case the mutations
// above all escaped.
//
// The crew is EMPTY and `partial` is TRUE, and that pair is the whole
// mechanism: an empty list means "nobody I can see", and the flag is what
// stops a reader turning that into "nobody". A peer that reads only the crew
// length reaches the wrong conclusion on exactly the estate where another
// daemon is live beside it.
//
// It drives the REAL path rather than injecting an answer: a genuinely held
// claim under a temporary XDG_STATE_HOME, taken with the same
// `instance.Acquire` section 37 precondition 6 uses. Injecting `others` would
// have tested the plumbing and skipped the mechanism.
func TestAnEmptyRosterIsNotAClaimThatNobodyElseIsThere(t *testing.T) {
	lockOtherEstate(t, "development")

	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)

	empty := roster(t, c)
	if n := len(empty.GetCrew()); n != 0 {
		t.Fatalf("crew = %d on a daemon nobody has announced to, want 0", n)
	}
	if !empty.GetPartial() {
		t.Fatal("an EMPTY roster came back with partial=false while another " +
			"named estate is claimed by a live daemon. That is the answer " +
			"section 37 row 1 exists to prevent: a seat reading this is told " +
			"it can see everybody, and it cannot")
	}

	// And on `announce` too, because that is the call a peer makes when it is
	// finding out who is here - it is the one that must not lie.
	a := announce(t, c, "backend-1", "checking whether I am alone", "checking")
	if !a.GetPartial() {
		t.Fatal("rig.announce came back with partial=false while another " +
			"named estate is live. peers and announce must agree; a peer " +
			"that learns it is not alone only by making a second call is a " +
			"peer that will not make it")
	}
}

// TestPartialIsFalseWhenNoOtherEstateIsClaimed is the other direction, and it
// is here so the flag cannot be made honest by making it useless. A mechanism
// that says "incomplete" unconditionally is not reporting anything.
func TestPartialIsFalseWhenNoOtherEstateIsClaimed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)

	if roster(t, c).GetPartial() {
		t.Fatal("partial=true with no other estate claimed anywhere. The " +
			"flag has stopped being a report and become a constant")
	}
	if announce(t, c, "backend-1", "the only estate here", "working").GetPartial() {
		t.Fatal("rig.announce says partial=true with no other estate claimed")
	}
}

// TestAStaleClaimDoesNotMakeTheWireSayPartial carries the 5e865bb finding
// across to the wire.
//
// `TestAStaleEstateClaimIsNotCountedAsAPeer` already pins it inside
// `otherEstates`. This one pins the CONSEQUENCE a caller sees, which is the
// half that was free to break: the regression it guards against was found by
// demonstrating against real state, and a demonstration reads the wire.
func TestAStaleClaimDoesNotMakeTheWireSayPartial(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	claims := filepath.Join(state, "rig", "estates")
	if err := os.MkdirAll(claims, 0o700); err != nil {
		t.Fatal(err)
	}
	// A claim file whose daemon is gone. instance.Close never unlinks one, so
	// this is the designed residue of every estate that has ever run here.
	if err := os.WriteFile(filepath.Join(claims, "development.pid"),
		[]byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)

	if roster(t, c).GetPartial() {
		t.Fatal("the wire reports partial=true against a claim file whose " +
			"daemon is dead. A file outliving its daemon is designed " +
			"behaviour, so this reads every estate that ever ran here as a " +
			"live peer")
	}
}

// lockOtherEstate points XDG_STATE_HOME at a temporary directory and leaves a
// named estate claim genuinely HELD there for the life of the test, which is
// what `otherEstates` tests for with a shared flock.
func lockOtherEstate(t *testing.T, name string) {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	claims := filepath.Join(state, "rig", "estates")
	if err := os.MkdirAll(claims, 0o700); err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(claims, name+".pid"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
}
