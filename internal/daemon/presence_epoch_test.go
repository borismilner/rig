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

// THE ADDRESSABLE IDENTITY IS (seat, epoch, generation), AND THIS FILE IS
// WHERE THAT RULING BITES.
//
// The finding this replaces was recorded, not ruled on: a seat reached
// generation 3, rigd was killed and restarted, and the next peer to take THE
// SAME SEAT was handed generation 1 - a number an earlier tenancy already
// held. The old case locked that behaviour and asserted nothing about whether
// it was acceptable.
//
// THE RULING, by the team-lead 2026-09-16, and it needed no new mechanism: a
// generation is unique WITHIN AN EPOCH. Section 37 precondition 4's epoch is
// durable, is bumped unconditionally at every start, and reached the agent
// surface in ed1dfd7. So the pair already exists and nothing has to persist a
// counter - which is what a globally unique generation would have cost.
//
// WHY A WIRE TEST AND NOT A UNIT ONE. Clause 2 asks what a READER WAS TOLD,
// and the whole question is what travels. The reader is where the identity is
// read, so the reader is where it is tested.
//
// THE TRIPLE COMES OFF ONE ROW AND THAT IS THE ASSERTION UNDERNEATH THE
// ASSERTION. An earlier draft of this file took the generation from the row
// and the epoch from a second rig.estate call - which is the test performing
// the straddle in order to test the thing the straddle breaks. A restart
// between those two calls builds a reference to a tenancy that never existed,
// demonstrated on a live daemon and now written into `wire.proto` at the
// field. Seat.epoch is what removed the second call.

// upDaemonIn is upDaemon with the estate and its epoch both chosen, which are
// the two axes these cases turn on. The shared helper can set neither and is
// not this seat's file, so the wiring is repeated here rather than reached
// into.
//
// THE TWO ARGUMENTS ARE PASSED TOGETHER BECAUSE THEY ARE ONE FACT. A named
// estate has a store and therefore an epoch of at least 1; an unnamed one has
// no store and reports 0. `wire.proto` says the mixed pair cannot happen, so a
// helper that let a caller set the epoch alone would let this package's own
// tests build the state the wire says is impossible - which the epoch cases
// below did until the estate landed.
func upDaemonIn(t *testing.T, estate string, epoch uint64) string {
	t.Helper()
	// sun_path is 108 bytes and t.TempDir under a long TMPDIR silently
	// exceeds it, failing as EINVAL. Kept short for the same reason.
	dir, err := os.MkdirTemp("", "rige")
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
		Version: "test", Wire: "v1", Lock: lock,
		Estate: estate, Epoch: epoch,
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

// reference is what a peer writes down when it means to address a seat later -
// row 5's directed messaging is the consumer that makes this bite.
type reference struct {
	seat       string
	epoch      uint64
	generation uint64
}

// TestTheEpochIsWhatFencesAGenerationAcrossARestart is the case that RULES.
//
// It asserts both halves of the ruling in the one place a caller meets them:
// (seat, generation) REPEATS across a restart, and (seat, epoch, generation)
// does not. The first half alone is a description. The pair is the rule.
//
// The failure it prevents is the one Boris raised himself - that peers "may be
// contacted wrongly thinking they are the successor". A message stamped before
// a restart and checked after one validates against a session that is not the
// one it was written for, and the check below is exactly the check row 5 has
// to make.
func TestTheEpochIsWhatFencesAGenerationAcrossARestart(t *testing.T) {
	// The daemon before the restart.
	before := dial(t, upDaemonIn(t, "production", 6))
	was := announce(t, before, "backend-1", "the tenancy before the restart", "working").GetYou()
	wrote := reference{seat: was.GetSeat(), epoch: was.GetEpoch(), generation: was.GetGeneration()}

	// THE RESTART. A new daemon with the next epoch, the same seat name, and a
	// generation counter that is in memory and only in memory.
	after := dial(t, upDaemonIn(t, "production", 7))
	is := announce(t, after, "backend-1", "the tenancy after the restart", "working").GetYou()
	live := reference{seat: is.GetSeat(), epoch: is.GetEpoch(), generation: is.GetGeneration()}

	if wrote.epoch == live.epoch {
		t.Fatalf("both daemons report epoch %d. The epoch is what distinguishes "+
			"one run from the next, and a restart that does not move it leaves "+
			"the pair no better than the generation alone", wrote.epoch)
	}

	// HALF ONE: the generation repeats, and that is recorded rather than fixed.
	// Presence holds no persistent counter by design - section 37 row 1 was
	// built before the write-ahead log exists precisely because it holds none.
	if wrote.generation != live.generation {
		t.Fatalf("generation %d before the restart and %d after it. This case "+
			"asserts they REPEAT, because the ruling accepts the repeat and "+
			"fences it with the epoch instead of spending a persisted counter "+
			"on it. If somebody has made generations survive a restart, this "+
			"test is the wrong shape and should be rewritten against what they "+
			"decided", wrote.generation, live.generation)
	}

	// HALF TWO, AND IT IS THE RULE. The pair a peer actually addresses with
	// must not validate across the restart.
	staleBySeatAndGeneration := wrote.seat == live.seat && wrote.generation == live.generation
	if !staleBySeatAndGeneration {
		t.Fatal("(seat, generation) did not match across the restart, so this " +
			"case is no longer demonstrating the trap it exists to demonstrate")
	}
	staleByFullIdentity := wrote.seat == live.seat &&
		wrote.epoch == live.epoch &&
		wrote.generation == live.generation
	if staleByFullIdentity {
		t.Fatalf("a reference written before the restart (%+v) still validates "+
			"against the occupant after it (%+v) on the FULL identity. The "+
			"epoch has stopped fencing, and a message addressed to a dead "+
			"tenancy is delivered to whoever holds the seat now", wrote, live)
	}
}

// TestEverySeatServedCarriesTheEpochItWasCountedIn is what the construction
// site buys, and it is the case a mutation can actually catch.
//
// `wire.proto` states the invariant as a construction guarantee: EVERY ROW IN
// ONE RESPONSE CARRIES THE SAME VALUE, so a reader must never branch on the
// possibility of two. A guarantee phrased that way is only worth the thing
// that enforces it, and the failure it prevents is quiet: a row served with
// epoch 0 is not an obvious blank, it is a generation nobody can interpret,
// and it reads as "the estate with no durable state" - a real answer that
// happens to be false here.
//
// FOUR SITES, which is why the epoch is set where an occupant is BORN rather
// than where a response is built. Stamping in the handlers was the first shape
// and it leaves a fifth site free to forget; carrying it on the occupant makes
// a Seat without an epoch unconstructable. This case is the proof of that, and
// it fails on any site that regresses.
func TestEverySeatServedCarriesTheEpochItWasCountedIn(t *testing.T) {
	const epoch = 9
	sock := upDaemonIn(t, "production", epoch)

	seated := dial(t, sock)
	// An unseated peer too: it holds no seat and carries generation 0, but it
	// is still present in THIS run, so its row belongs to this epoch like any
	// other. Zero there would be the same unreadable answer.
	loose := dial(t, sock)
	announce(t, loose, "", "a one-off session holding no seat", "reading")

	// SITE 1 and SITE 2: announce answers with the caller's own row and the
	// whole crew.
	got := announce(t, seated, "backend-1", "the seated peer", "working")
	if e := got.GetYou().GetEpoch(); e != epoch {
		t.Errorf("announce told the caller its own epoch is %d, want %d. A peer "+
			"cannot quote an identity it was never given", e, epoch)
	}
	if n := len(got.GetCrew()); n != 2 {
		t.Fatalf("crew = %d, want 2", n)
	}
	for _, s := range got.GetCrew() {
		if e := s.GetEpoch(); e != epoch {
			t.Errorf("announce served seat %q with epoch %d, want %d",
				s.GetSeat(), e, epoch)
		}
	}

	// SITE 3: activity answers with the caller's row, and it is the site a
	// per-response field would have missed entirely.
	act := &rigv1.ActivityResponse{}
	if err := seated.Call(ctx5(t), "rig.activity", &rigv1.ActivityRequest{
		Activity: "still working",
	}, act); err != nil {
		t.Fatal(err)
	}
	if e := act.GetYou().GetEpoch(); e != epoch {
		t.Errorf("rig.activity served a row with epoch %d, want %d. This is the "+
			"call a peer makes most often and the one that would have needed a "+
			"third envelope field", e, epoch)
	}

	// SITE 4: the roster, which is what a third party reads.
	crew := roster(t, seated).GetCrew()
	if len(crew) != 2 {
		t.Fatalf("roster crew = %d, want 2", len(crew))
	}
	for _, s := range crew {
		if e := s.GetEpoch(); e != epoch {
			t.Errorf("rig.peers served seat %q (generation %d) with epoch %d, "+
				"want %d", s.GetSeat(), s.GetGeneration(), e, epoch)
		}
	}
}
