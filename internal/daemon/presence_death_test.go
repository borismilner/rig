package daemon

import (
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// WHAT HAPPENS WHEN THE HOLDER DIES, and the harder half: what happens when
// it does NOT. Section 37's clause 2 asks for failure semantics demonstrated
// rather than specified, and the demonstration is not in this file because it
// cannot be: a Go test closes a socket, and a socket closed by `Close` and a
// socket closed by the kernel on SIGKILL are the same event by the time the
// daemon sees either. So the death was shown with real processes and the
// numbers are recorded here; the case below pins what the demonstration
// found that nothing else did.
//
// DEMONSTRATED 2026-09-16 at acb8015, real rigd, a real holder process, a real
// SIGKILL, and a separate reader process on both sides of it:
//
//	before          crew 0, partial false
//	holder seated   backend-1, generation 1, ACTIVE
//	reader sees     backend-1 / "the seat that is about to die" / ACTIVE / 1
//	kill -9         no goodbye, no protocol, no deferred cleanup in the holder
//	reader sees     crew 0
//	successor       backend-1 at generation 2, granted
//
// THE LATENCY IS THE RESULT WORTH QUOTING, over five runs: 16, 22, 25, 27 and
// 18 ms from the kill to the row being gone, and in every run the FIRST read
// after the kill already returned zero rows. There is no window in which a
// reader sees a ghost. That is the connection-scoped design paying: a TTL
// cannot beat it, because a TTL's floor IS its interval.
//
// AND THE LIMIT, measured the same way and recorded because it is the honest
// half. SIGSTOP instead of SIGKILL: the holder goes to state T, stops
// responding to anything, and its row stays on the roster reading ACTIVE with
// its purpose and activity intact. Presence detects DEATH, not a HANG, because
// the connection is the evidence and a stopped process holds its connection.
// That is not a defect to fix here - the roster already carries
// `activity_unix_nano`, which is the raw material for judging staleness, and
// whether anything renders it is not this mechanism's question.
//
// ONE CASE WAS WRITTEN FOR THIS FILE AND THEN CUT, recorded so nobody writes
// it a third time. It ran the transition a successor actually meets: the seat
// refused while its holder is live, then GRANTED at generation 2 once the
// holder is gone, with the counter still remembering the dead tenancy. It
// passed, and it was cut because it bites nothing. Mutating the refusal to
// consult `gens` (a remembered claim) instead of the live connections already
// fails four committed cases without it - TestASeatKeepsItsNameAndCounts-
// ItsOccupants, TestAHeldSeatIsRefusedAndTheRefusalNamesTheHolder,
// TestOneSeatIsOneRowAcrossThreeSuccessiveSessions and
// TestAGenerationIsNotUniqueAcrossADaemonRestart. A case a mutation cannot
// distinguish from the suite around it is weight, not evidence.

// TestAnIdleSeatIsNeverReapedBecauseThereIsNoClockToReapOn pins the guarantee
// the suite had no case for.
//
// `presence.go` promises "there is no TTL to tune, no reaper to schedule and
// no orphan to detect", and `daemon.go` repeats it. Nothing tested it. Every
// existing case drops a connection and watches the row go, which is the loud
// direction; a reaper added tomorrow passes all of them, because no test in
// this package has ever moved the clock.
//
// The SIGSTOP result above is this test's real subject. A stopped holder is
// indistinguishable from a working one and it MUST keep its seat: the
// alternative is a daemon that hands a seat to a second occupant while the
// first is merely paused, which is the two-sessions-one-seat failure the
// refusal exists to prevent, arriving by a different door.
func TestAnIdleSeatIsNeverReapedBecauseThereIsNoClockToReapOn(t *testing.T) {
	p := newPresence("production", 1)
	at := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return at }

	var holder occupancy
	seated, err := p.announce(&holder, "backend-1", "seated and then silent", "seated")
	if err != nil {
		t.Fatal(err)
	}

	// A DAY LATER, and the occupant has said nothing since. Any TTL anybody
	// would be tempted to add fires somewhere inside this interval.
	at = at.Add(24 * time.Hour)

	crew := p.crew()
	if len(crew) != 1 {
		t.Fatalf("crew = %d a day after the occupant's last word, want 1. "+
			"Something now expires a seat on a clock, and presence is "+
			"connection state: the peer is still connected, so it is still "+
			"there. A seat that vanishes while its holder is alive is worse "+
			"than no roster, because the successor is told the seat is free",
			len(crew))
	}
	row := crew[0]
	if row.GetSeat() != "backend-1" || row.GetGeneration() != seated.generation {
		t.Fatalf("row is %q at generation %d after a day, want backend-1 at %d",
			row.GetSeat(), row.GetGeneration(), seated.generation)
	}
	if row.GetState() != rigv1.SeatState_SEAT_STATE_ACTIVE {
		t.Errorf("state is %v after a day of silence, want ACTIVE. Presence "+
			"reports what a peer SAID it was doing, and nothing here is "+
			"entitled to downgrade that on a timer", row.GetState())
	}
	if row.GetAnnouncedUnixNano() != at.Add(-24*time.Hour).UnixNano() {
		t.Errorf("announced_unix_nano moved to %d. The announcement is when "+
			"the tenancy began and re-reading the roster is not an event in it",
			row.GetAnnouncedUnixNano())
	}
}
