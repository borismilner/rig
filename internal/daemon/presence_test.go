package daemon

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

func announce(t *testing.T, c *client.Client, seat, purpose, activity string) *rigv1.AnnounceResponse {
	t.Helper()
	resp := &rigv1.AnnounceResponse{}
	err := c.Call(ctx5(t), "rig.announce", &rigv1.AnnounceRequest{
		Seat: seat, Purpose: purpose, Activity: activity,
	}, resp)
	if err != nil {
		t.Fatalf("rig.announce(%q): %v", seat, err)
	}
	return resp
}

func roster(t *testing.T, c *client.Client) *rigv1.PeersResponse {
	t.Helper()
	resp := &rigv1.PeersResponse{}
	if err := c.Call(ctx5(t), "rig.peers", &rigv1.PeersRequest{}, resp); err != nil {
		t.Fatalf("rig.peers: %v", err)
	}
	return resp
}

// A seat is an address that outlives its occupants, and the generation is the
// only thing that tells one tenancy from the next.
func TestASeatKeepsItsNameAndCountsItsOccupants(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	first := dial(t, sock)
	got := announce(t, first, "backend-1", "porting the settings surface", "reading")
	if got.GetYou().GetSeat() != "backend-1" {
		t.Fatalf("seat = %q, want backend-1", got.GetYou().GetSeat())
	}
	if g := got.GetYou().GetGeneration(); g != 1 {
		t.Fatalf("first occupancy is generation %d, want 1", g)
	}

	// The occupant leaves. Presence is connection state, so closing IS the
	// expiry: nothing is scheduled and nothing has to notice.
	_ = first.Close()

	// The seat must be free, and the next tenancy must be DISTINGUISHABLE from
	// the one before it. Generation 1 twice would mean a briefing addressed to
	// "backend-1 generation 1" could not tell which session it reached, which
	// is the defect this whole mechanism exists to close.
	second := dial(t, sock)
	var got2 *rigv1.AnnounceResponse
	for range 50 {
		got2 = announce(t, second, "backend-1", "the successor", "taking over")
		if got2.GetYou().GetGeneration() == 2 {
			break
		}
		// The first connection's teardown is concurrent with this call.
		// Re-announcing on the SAME connection is idempotent, so this loop
		// cannot manufacture the answer it is waiting for.
	}
	if g := got2.GetYou().GetGeneration(); g != 2 {
		t.Fatalf("second occupancy is generation %d, want 2 - a seat that "+
			"does not count its tenancies cannot answer 'am I still talking "+
			"to the session I was briefed by'", g)
	}
}

// THE MUTATION THIS BITES ON: delete the "other != c" guard in announce and
// this test fails. Letting the newcomer win would make every announce succeed
// and would hide two sessions believing they are the same seat.
func TestAHeldSeatIsRefusedAndTheRefusalNamesTheHolder(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	held := dial(t, sock)
	announce(t, held, "team-lead", "sequencing the cutover", "writing the plan")

	intruder := dial(t, sock)
	err := intruder.Call(ctx5(t), "rig.announce", &rigv1.AnnounceRequest{
		Seat: "team-lead", Purpose: "also sequencing the cutover",
	}, &rigv1.AnnounceResponse{})
	if err == nil {
		t.Fatal("a second live peer took a held seat; both now believe they " +
			"are team-lead and neither will find out")
	}
	// Section 9: the message has to carry the state actually found, not just
	// the refusal, or the caller cannot act on it.
	for _, want := range []string{"team-lead", "generation 1", "sequencing the cutover"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q; a caller cannot tell who holds "+
				"the seat.\ngot: %v", want, err)
		}
	}
}

// Re-announcing into a seat this connection already holds is how a successor
// replaces the row that still says "successor in a warm handoff". It must NOT
// count as a new tenancy: a peer holding generation 1 is still correctly
// addressing this occupant, and bumping would invalidate it for nothing.
func TestRestatingAPurposeInTheSameSeatDoesNotMoveTheGeneration(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)

	first := announce(t, c, "gui", "successor in a warm handoff", "reading")
	again := announce(t, c, "gui", "porting the tray to the new theme", "building")

	if first.GetYou().GetGeneration() != again.GetYou().GetGeneration() {
		t.Fatalf("generation moved from %d to %d on a re-announce into the "+
			"same seat", first.GetYou().GetGeneration(), again.GetYou().GetGeneration())
	}
	if p := again.GetYou().GetPurpose(); p != "porting the tray to the new theme" {
		t.Fatalf("purpose = %q; the re-announce did not replace it, which is "+
			"the whole reason a successor calls it", p)
	}
}

// HANDING_OFF has to survive the activity lines that follow it, or it is a
// state nobody can hold for the length of an actual handoff.
func TestHandingOffSurvivesLaterActivityLines(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)
	announce(t, c, "backend-2", "the session token", "building")

	set := func(activity string, state rigv1.SeatState) *rigv1.Seat {
		t.Helper()
		resp := &rigv1.ActivityResponse{}
		err := c.Call(ctx5(t), "rig.activity", &rigv1.ActivityRequest{
			Activity: activity, State: state,
		}, resp)
		if err != nil {
			t.Fatalf("rig.activity: %v", err)
		}
		return resp.GetYou()
	}

	if s := set("briefing my successor", rigv1.SeatState_SEAT_STATE_HANDING_OFF).GetState(); s != rigv1.SeatState_SEAT_STATE_HANDING_OFF {
		t.Fatalf("state = %v, want HANDING_OFF", s)
	}
	// UNSPECIFIED means "leave it alone". A state that resets itself whenever
	// a peer says what it is doing is a state nobody can hold.
	got := set("answering its questions", rigv1.SeatState_SEAT_STATE_UNSPECIFIED)
	if got.GetState() != rigv1.SeatState_SEAT_STATE_HANDING_OFF {
		t.Fatalf("state fell back to %v after an ordinary activity call; "+
			"UNSPECIFIED must leave the state alone", got.GetState())
	}
	if got.GetActivity() != "answering its questions" {
		t.Fatalf("activity = %q, not updated", got.GetActivity())
	}
}

// An activity line with no purpose above it is exactly the unsupervisable row
// the seat mechanism exists to prevent, so it is not created by side effect.
func TestActivityWithoutAnnounceIsRefused(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	err := dial(t, sock).Call(ctx5(t), "rig.activity",
		&rigv1.ActivityRequest{Activity: "working"}, &rigv1.ActivityResponse{})
	if err == nil {
		t.Fatal("activity created a row with no purpose")
	}
	if !strings.Contains(err.Error(), "announce") {
		t.Errorf("the refusal does not say what to do instead: %v", err)
	}
}

// A blank purpose is indistinguishable from a session nobody is supervising,
// which is the observed defect rather than a hypothetical one.
func TestAnnounceRefusesABlankPurpose(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	err := dial(t, sock).Call(ctx5(t), "rig.announce",
		&rigv1.AnnounceRequest{Seat: "backend-1"}, &rigv1.AnnounceResponse{})
	if err == nil {
		t.Fatal("a row with no purpose was accepted")
	}
}

// A peer may be present without claiming a seat, and it must be visibly
// distinct from an occupant rather than rendered as generation 0 of something.
func TestAnUnseatedPeerIsPresentAndCarriesNoGeneration(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	c := dial(t, sock)
	got := announce(t, c, "", "a one-off check of the ratchet", "measuring")

	if got.GetYou().GetSeat() != "" {
		t.Fatalf("seat = %q, want empty", got.GetYou().GetSeat())
	}
	if g := got.GetYou().GetGeneration(); g != 0 {
		t.Fatalf("generation = %d for an unseated peer, want 0", g)
	}
	if len(got.GetCrew()) != 1 {
		t.Fatalf("crew = %d, want 1 - the caller must be in its own answer, "+
			"or a peer has to merge two calls to learn the roster", len(got.GetCrew()))
	}
}

// The roster is read by a human scanning for a hung session, so two reads in
// the same instant must not reorder.
func TestTheRosterIsOrderedSeatsFirstThenByName(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	announce(t, dial(t, sock), "", "an unseated probe", "measuring")
	announce(t, dial(t, sock), "team-lead", "sequencing", "writing")
	c := dial(t, sock)
	announce(t, c, "backend-1", "the front door", "building")

	crew := roster(t, c).GetCrew()
	if len(crew) != 3 {
		t.Fatalf("crew = %d, want 3", len(crew))
	}
	if crew[0].GetSeat() != "backend-1" || crew[1].GetSeat() != "team-lead" {
		t.Fatalf("seated peers are not first and sorted: %q, %q, %q",
			crew[0].GetSeat(), crew[1].GetSeat(), crew[2].GetSeat())
	}
	if crew[2].GetSeat() != "" {
		t.Fatalf("the unseated peer is not last: %q", crew[2].GetSeat())
	}
}

// Presence is connection state: a peer that is listed is a peer whose
// connection is open. Nothing else has to be true for that to hold.
func TestADroppedConnectionLeavesTheRoster(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "watching", "watching")

	gone := dial(t, sock)
	announce(t, gone, "backend-1", "about to die", "dying")
	if n := len(roster(t, watcher).GetCrew()); n != 2 {
		t.Fatalf("crew = %d before the drop, want 2", n)
	}
	_ = gone.Close()

	for range 50 {
		if len(roster(t, watcher).GetCrew()) == 1 {
			return
		}
	}
	t.Fatal("a dropped peer is still on the roster; presence is supposed to " +
		"expire with the connection and nothing else")
}
