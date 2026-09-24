package daemon

import (
	"testing"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// WHAT A THIRD PARTY READS OFF THE ROSTER, which is a different question from
// what a peer is told about itself, and until this file existed only the
// second one was tested.
//
// Backlog B9 is the subject: "the board shows SESSIONS and Boris thinks in
// SEATS". His own experience, 2026-09-11, unprompted - he looked at the Agents
// board and counted THREE rows for one seat, `rig-backend-2`, `rig-client` and
// `rig-client-b`, which are one lineage across three sessions. Two further rows
// still read "successor in a warm handoff" as their PURPOSE because the seats
// never re-announced after taking over.
//
// Both halves of that are ALREADY ANSWERED by row 1 as built - the seat is an
// address with a generation, and `wire.proto` says HANDING_OFF "EXISTS BECAUSE
// IT IS THE STATE THAT WAS OBSERVED MISSING". Nothing here proposes anything.
// What was missing is the evidence that the answer reaches the reader it was
// built for.
//
// MEASURED, and this is the gap these cases close. Every assertion about a
// seat's STATE in this package reads `resp.GetYou()` - the reply handed to the
// occupant that just called. Every assertion about the CREW reads seat names
// and the length of the list. So `crew()` could return SEAT_STATE_ACTIVE for
// every row, or drop the generation, and the whole repository stays green.
//
// THAT MUTATION IS B9's OWN DEFECT. A handing-off state that only the session
// handing off can see is worth exactly as much as the stale purpose it
// replaced: the reader who needed it is the one watching, and the watcher is
// the case nobody asserted.

// seatRows returns the roster rows for one seat. B9 is a COUNT of rows, so the
// count is what this returns to be asserted on.
func seatRows(crew []*verbsv1.Seat, seat string) []*verbsv1.Seat {
	var out []*verbsv1.Seat
	for _, s := range crew {
		if s.GetSeat() == seat {
			out = append(out, s)
		}
	}
	return out
}

// awaitRows blocks until a seat has exactly n rows on the roster, and reports
// what it saw if it never does. Presence expires with the connection, so a
// predecessor's row leaves on the daemon's schedule rather than on the test's.
func awaitRows(t *testing.T, watcher *client.Client, seat string, n int) []*verbsv1.Seat {
	t.Helper()
	var last int
	for range 200 {
		rows := seatRows(roster(t, watcher).GetCrew(), seat)
		if len(rows) == n {
			return rows
		}
		last = len(rows)
	}
	t.Fatalf("seat %q has %d rows on the roster, want %d", seat, last, n)
	return nil
}

// TestABoardReaderSeesAHandoffAsAStateAndNotAsASecondRow is B9 stated as a
// test, and it is the one the mutation above escapes.
//
// The roster carries ONE row for a seat being handed off, and that row says
// HANDING_OFF to somebody who is not its occupant. Those are the two halves of
// what Boris counted: not three rows, and not a purpose nobody rewrote.
func TestABoardReaderSeesAHandoffAsAStateAndNotAsASecondRow(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "reading the board", "watching")

	holder := dial(t, sock)
	announce(t, holder, "backend-1", "the session token", "building")

	resp := &verbsv1.ActivityResponse{}
	err := holder.Call(ctx5(t), "rig.activity", &verbsv1.ActivityRequest{
		Activity: "briefing my successor",
		State:    verbsv1.SeatState_SEAT_STATE_HANDING_OFF,
	}, resp)
	if err != nil {
		t.Fatalf("rig.activity: %v", err)
	}

	rows := awaitRows(t, watcher, "backend-1", 1)
	if s := rows[0].GetState(); s != verbsv1.SeatState_SEAT_STATE_HANDING_OFF {
		t.Fatalf("a WATCHER reads state=%v on a seat that is handing off, "+
			"want HANDING_OFF. The state exists so the roster can say what a "+
			"stale purpose used to say badly; a state only its own occupant "+
			"can see has not replaced anything", s)
	}
	if p := rows[0].GetPurpose(); p != "the session token" {
		t.Fatalf("purpose on the roster = %q, want the holder's. A board that "+
			"renders a blank headline is the unsupervisable row announce "+
			"refuses at the boundary, arriving by the other door", p)
	}
	if a := rows[0].GetActivity(); a != "briefing my successor" {
		t.Fatalf("activity on the roster = %q; the watcher is reading a line "+
			"the occupant has already replaced", a)
	}
}

// TestOneSeatIsOneRowAcrossThreeSuccessiveSessions is B9's count, run.
//
// Three sessions take one seat one after another - the exact shape of
// `rig-backend-2` / `rig-client` / `rig-client-b` - and a watcher counts the
// rows throughout. The answer is ONE at every point, and the generation is how
// the watcher tells which tenancy it is looking at.
//
// THE GENERATION IS ASSERTED OFF THE ROSTER rather than off the occupant's own
// reply, because that is the half that was free to break: a seat that renders
// as one row but cannot say WHICH tenancy is a seat a briefing cannot be
// addressed to, and section 37 row 5 is directed messaging carrying exactly
// that number.
func TestOneSeatIsOneRowAcrossThreeSuccessiveSessions(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "reading the board", "watching")

	for want := uint64(1); want <= 3; want++ {
		occupant := dial(t, sock)
		announce(t, occupant, "backend-1", "one lineage, three sessions", "working")

		rows := awaitRows(t, watcher, "backend-1", 1)
		if g := rows[0].GetGeneration(); g != want {
			t.Fatalf("tenancy %d renders on the roster at generation %d. A "+
				"watcher that cannot number the tenancy is reading the board "+
				"Boris read, where three sessions in one seat were three "+
				"unrelated rows", want, g)
		}

		// The occupant goes. Presence is connection state, so this IS the
		// expiry and the seat is empty until the next one announces.
		_ = occupant.Close()
		awaitRows(t, watcher, "backend-1", 0)
	}
}
