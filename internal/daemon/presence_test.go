package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/instance"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

func announce(t *testing.T, c *client.Client, seat, purpose, activity string) *verbsv1.AnnounceResponse {
	t.Helper()
	resp := &verbsv1.AnnounceResponse{}
	err := c.Call(ctx5(t), "rig.announce", &verbsv1.AnnounceRequest{
		Seat: seat, Purpose: purpose, Activity: activity,
	}, resp)
	if err != nil {
		t.Fatalf("rig.announce(%q): %v", seat, err)
	}
	return resp
}

func roster(t *testing.T, c *client.Client) *verbsv1.PeersResponse {
	t.Helper()
	resp := &verbsv1.PeersResponse{}
	if err := c.Call(ctx5(t), "rig.peers", &verbsv1.PeersRequest{}, resp); err != nil {
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
	got2 := &verbsv1.AnnounceResponse{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// The first connection's teardown is concurrent with this call, so
		// until it lands the seat is still held and the announce is refused
		// as DENIED. That refusal is the old tenancy still being torn down,
		// not an answer, and it is retried; any other error is fatal.
		// Re-announcing on the SAME connection is idempotent, so this loop
		// cannot manufacture the answer it is waiting for.
		err := second.Call(ctx5(t), "rig.announce", &verbsv1.AnnounceRequest{
			Seat: "backend-1", Purpose: "the successor", Activity: "taking over",
		}, got2)
		var ce *client.CallError
		if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_DENIED {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if err != nil {
			t.Fatalf("rig.announce(backend-1): %v", err)
		}
		if got2.GetYou().GetGeneration() == 2 {
			break
		}
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
	err := intruder.Call(ctx5(t), "rig.announce", &verbsv1.AnnounceRequest{
		Seat: "team-lead", Purpose: "also sequencing the cutover",
	}, &verbsv1.AnnounceResponse{})
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

	set := func(activity string, state verbsv1.SeatState) *verbsv1.Seat {
		t.Helper()
		resp := &verbsv1.ActivityResponse{}
		err := c.Call(ctx5(t), "rig.activity", &verbsv1.ActivityRequest{
			Activity: activity, State: state,
		}, resp)
		if err != nil {
			t.Fatalf("rig.activity: %v", err)
		}
		return resp.GetYou()
	}

	if s := set("briefing my successor", verbsv1.SeatState_SEAT_STATE_HANDING_OFF).GetState(); s != verbsv1.SeatState_SEAT_STATE_HANDING_OFF {
		t.Fatalf("state = %v, want HANDING_OFF", s)
	}
	// UNSPECIFIED means "leave it alone". A state that resets itself whenever
	// a peer says what it is doing is a state nobody can hold.
	got := set("answering its questions", verbsv1.SeatState_SEAT_STATE_UNSPECIFIED)
	if got.GetState() != verbsv1.SeatState_SEAT_STATE_HANDING_OFF {
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
		&verbsv1.ActivityRequest{Activity: "working"}, &verbsv1.ActivityResponse{})
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
		&verbsv1.AnnounceRequest{Seat: "backend-1"}, &verbsv1.AnnounceResponse{})
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

// A STALE CLAIM IS NOT A PEER, and this test exists because the first version
// of otherEstates counted files. It reported partial=true against two estates
// whose daemons had died the previous evening, which is a specific false claim
// rather than a cautious one: `instance.Close` never unlinks a pidfile, so
// every estate that has ever run leaves one behind for good.
//
// FOUND BY DEMONSTRATING, NOT BY TESTING. The unit tests all run against a
// temporary state directory that no daemon has ever claimed, so nothing here
// could have caught it. That is the gap this case closes.
func TestAStaleEstateClaimIsNotCountedAsAPeer(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	claims := filepath.Join(state, "rig", "estates")
	if err := os.MkdirAll(claims, 0o700); err != nil {
		t.Fatal(err)
	}

	// A claim whose daemon is gone: the file is left behind by design.
	stale := filepath.Join(claims, "production.pid")
	if err := os.WriteFile(stale, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := otherEstates(""); len(got) != 0 {
		t.Fatalf("otherEstates = %v against a stale claim; a file that "+
			"outlives its daemon is the designed behaviour and must not read "+
			"as a live peer", got)
	}

	// The same file, now genuinely held. instance.Acquire is the authority
	// section 37 precondition 6 already uses, so the test drives that rather
	// than a second mechanism.
	lock, err := instance.Acquire(stale)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	got := otherEstates("")
	if len(got) != 1 || got[0] != "production" {
		t.Fatalf("otherEstates = %v while production is held; want "+
			"[production]", got)
	}
	// And an estate never reports itself.
	if own := otherEstates("production"); len(own) != 0 {
		t.Fatalf("otherEstates = %v from production's own daemon", own)
	}
}

// TestReadingYourOwnRowDoesNotTouchIt is `occupantOf`, and the case is about
// what it must NOT do rather than what it returns.
//
// THE DOOR NEEDS THIS READ AND THE ONLY EXISTING WAY TO GET THE ROW WAS A
// WRITER. `list_agents` served `you: null` to a SEATED caller, which is the
// shape the surface uses for "you have no row" - so a seat was told, in the
// surface's own vocabulary, that it was not on the roster, on the one call the
// lead ruled must ANSWER rather than refuse because it is what a confused seat
// uses to find out what happened.
//
// `setActivity(c, "", UNSPECIFIED)` would have worked: setLine returns early
// on an empty line, so today it is a no-op that happens to return the row.
// **That is exactly why it was not used.** The moment setLine's early return
// changes, a reader built on it becomes a writer and nothing at the call site
// says so - and the mutation that would catch it lives in a different function,
// so no test of the reader could bite.
//
// So this case asserts the read is a READ: the age, the state and the
// generation are all where they were afterwards.
func TestReadingYourOwnRowDoesNotTouchIt(t *testing.T) {
	p := newPresence("production", 1)
	at := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return at }

	const line = "waiting for the gate to finish"
	var holder occupancy
	seated, err := p.announce(&holder, "backend-2", "presence", line)
	if err != nil {
		t.Fatal(err)
	}
	began := at

	// A DAY LATER the connection reads its own row. Reading is not an event in
	// the tenancy and not work being done.
	at = at.Add(24 * time.Hour)

	got, ok := p.occupantOf(&holder)
	if !ok {
		t.Fatal("a seated connection could not read its own row, which is the " +
			"defect this exists to close: the door then has to serve `you` as " +
			"null, and null is how it says you have no row at all")
	}
	if got.seat != "backend-2" || got.generation != seated.generation {
		t.Fatalf("read back seat %q at generation %d, want backend-2 at %d",
			got.seat, got.generation, seated.generation)
	}
	if got.proto().GetActivityUnixNano() != began.UnixNano() {
		t.Errorf("reading the row set its activity age to %d, want %d. A read "+
			"that dates the line to now is the stale-line failure arriving "+
			"through the one call a confused seat makes to orient itself",
			got.proto().GetActivityUnixNano(), began.UnixNano())
	}
	if got.proto().GetActivity() != line {
		t.Errorf("activity = %q after a read, want %q kept",
			got.proto().GetActivity(), line)
	}

	// And the roster itself is untouched, not merely the copy handed back.
	crew := p.crew()
	if len(crew) != 1 {
		t.Fatalf("crew = %d after a read, want 1", len(crew))
	}
	if crew[0].GetActivityUnixNano() != began.UnixNano() {
		t.Errorf("the ROSTER's age moved to %d after a read, want %d. The copy "+
			"handed back was clean and the row behind it was not",
			crew[0].GetActivityUnixNano(), began.UnixNano())
	}

	// A connection that never announced has no row, and must be told so rather
	// than handed a blank one that reads as a real peer.
	var stranger occupancy
	if _, ok := p.occupantOf(&stranger); ok {
		t.Error("a connection that never announced was given a row. An empty " +
			"occupant reported as present is a row with no purpose on it, " +
			"which is the defect the seat mechanism exists to surface")
	}
}
