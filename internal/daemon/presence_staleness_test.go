package daemon

import (
	"testing"
	"time"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// THE AGE OF AN ACTIVITY LINE IS THE ONLY THING ON THIS ROSTER THAT CAN SAY A
// SESSION IS REPEATING ITSELF, AND UNTIL THIS FILE EXISTED NOTHING DEFENDED
// IT.
//
// `presence_death_test.go` already rests its own honest limit on this field.
// Its SIGSTOP result says presence detects DEATH and not a HANG, and excuses
// that with "the roster already carries `activity_unix_nano`, which is the raw
// material for judging staleness". That excuse is only good while the field
// means what it says. Both entry points moved it on EVERY call, so a session
// restating itself renewed its own freshness: the one piece of raw material
// the hang case was handed off to was manufactured by the hang.
//
// AGENTBOX REFUSES THE SAME RESET ON PURPOSE and says why in its own tool
// description: "Re-sending an unchanged line deliberately does not reset its
// age, because repeating yourself is not progress." `cutover-gap.md` names
// this as the ONE place where porting straight across LOSES a supervision
// property rather than gaining one.
//
// FOUR CASES, TWO PER DOOR, AND NONE OF THEM IS PADDING. Each names the
// mutation it exists to catch, because a case the suite around it already
// fails is weight rather than evidence:
//
//	1  wire    restating through rig.activity        removing the guard
//	2  wire    a CHANGED line still moves the age    freezing the field
//	3  unit    re-announcing the SAME line           announce's own reset
//	4  unit    re-announcing with NO line            the silent wipe
//
// Cases 1 and 2 are at the wire because clause 2 asks what a READER WAS TOLD,
// and a freshness signal only its own occupant can see has not signalled
// anything. Cases 3 and 4 are units with the clock held still, because "a day
// later, unchanged" is the shape of the failure and a wire test can only
// assert that some time passed.

// stalenessSeat is the one seat these cases put under load. It is a constant
// rather than an argument because every case here is about ONE row being
// misread, and a helper that takes a seat it is only ever given one of is a
// parameter pretending to be a choice - `unparam` says so and is right.
const stalenessSeat = "backend-1"

// activityAge returns the activity timestamp a BOARD READS off that row.
func activityAge(t *testing.T, watcher *client.Client) int64 {
	t.Helper()
	rows := seatRows(roster(t, watcher).GetCrew(), stalenessSeat)
	if len(rows) != 1 {
		t.Fatalf("seat %q has %d rows on the roster, want exactly 1",
			stalenessSeat, len(rows))
	}
	return rows[0].GetActivityUnixNano()
}

// setActivityLine sends one activity line and returns what the occupant itself
// was told, so both readers of the timestamp can be held to the same answer.
func setActivityLine(t *testing.T, c *client.Client, line string) *rigv1.Seat {
	t.Helper()
	resp := &rigv1.ActivityResponse{}
	err := c.Call(ctx5(t), "rig.activity", &rigv1.ActivityRequest{Activity: line}, resp)
	if err != nil {
		t.Fatalf("rig.activity(%q): %v", line, err)
	}
	return resp.GetYou()
}

// TestRestatingAnActivityLineDoesNotRefreshItsAge is the rule, run.
//
// The line is set at ANNOUNCE and then restated, which is the shape a ported
// caller actually has: AgentBox's announce takes an activity and its own
// documentation says "set_activity carries this from then on". So the first
// restatement is the one most likely to happen by accident, and it must not
// move the clock either.
func TestRestatingAnActivityLineDoesNotRefreshItsAge(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "reading the board", "watching")

	const line = "waiting for the gate to finish"
	holder := dial(t, sock)
	announce(t, holder, stalenessSeat, "the cutover", line)

	first := activityAge(t, watcher)

	// Five restatements, because one is a coincidence and the LOOP is what is
	// being defended against. A session stuck in a retry sends this line every
	// time round, and every one of them used to buy it another clean bill.
	for i := range 5 {
		you := setActivityLine(t, holder, line)

		if got := activityAge(t, watcher); got != first {
			t.Fatalf("restatement %d of an UNCHANGED activity line moved the "+
				"roster's activity age from %d to %d. Repeating yourself is "+
				"not progress: a session looping on one line now renews its "+
				"own freshness, and activity_unix_nano is the only signal a "+
				"board has for a session that is stuck. The SIGSTOP limit in "+
				"presence_death_test.go is handed off to exactly this field",
				i+1, first, got)
		}
		if got := you.GetActivityUnixNano(); got != first {
			t.Fatalf("restatement %d: the roster held the age at %d but the "+
				"occupant was told %d. Two readers of one row disagreeing "+
				"about staleness is worse than either answer alone",
				i+1, first, got)
		}
	}
}

// TestChangingTheActivityLineRefreshesItsAge is the other half, and without it
// the case above is passed by a field that never moves at all.
//
// A frozen age is not a conservative failure. It reads as a session that has
// said nothing since it announced, which is the exact accusation the rule
// above exists to make truthfully.
func TestChangingTheActivityLineRefreshesItsAge(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "reading the board", "watching")

	holder := dial(t, sock)
	announce(t, holder, stalenessSeat, "the cutover", "reading the brief")

	announced := activityAge(t, watcher)

	setActivityLine(t, holder, "writing the red test")
	moved := activityAge(t, watcher)
	if moved <= announced {
		t.Fatalf("a NEW activity line left the age at %d (announce was %d). "+
			"An age that never moves says the session has done nothing since "+
			"it arrived, which is the same lie as an age that always moves, "+
			"told the other way round", moved, announced)
	}

	setActivityLine(t, holder, "gating it")
	again := activityAge(t, watcher)
	if again <= moved {
		t.Fatalf("the second new line left the age at %d, having been %d. "+
			"The rule is that the age tracks the LINE, not the call", again, moved)
	}
}

// TestReAnnouncingTheSameLineDoesNotRefreshItsAge is the same rule at the
// OTHER door, and announce is the door it was missed at.
//
// Re-announcing is the documented way to restate a purpose after a handoff -
// `presence.go` says so, and the roster defect Boris counted was rows still
// reading "successor in a warm handoff" because nobody did. So this call is
// one the team is actively encouraged to make, and it must not launder a
// stale activity line into a fresh-looking one on the way past.
//
// The clock is held still and then moved a day, because the failure is not
// "the number changed" but "a day-old line reads as current".
func TestReAnnouncingTheSameLineDoesNotRefreshItsAge(t *testing.T) {
	p := newPresence("production", 1)
	at := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return at }

	const line = "waiting for the gate to finish"
	var holder occupancy
	if _, err := p.announce(&holder, stalenessSeat, "the cutover", line); err != nil {
		t.Fatal(err)
	}
	began := at

	// A DAY LATER the seat restates itself, word for word, and nothing it is
	// doing has moved.
	at = at.Add(24 * time.Hour)
	o, err := p.announce(&holder, stalenessSeat, "the cutover", line)
	if err != nil {
		t.Fatal(err)
	}

	if got := o.proto().GetActivityUnixNano(); got != began.UnixNano() {
		t.Fatalf("re-announcing an UNCHANGED line a day later set the age to "+
			"%d, want the original %d. The generation and the announcement "+
			"are both deliberately preserved on this path because a restate "+
			"is not a new tenancy - the activity age is the third thing that "+
			"did not happen, and it was the one moving",
			got, began.UnixNano())
	}
	if got := o.proto().GetAnnouncedUnixNano(); got != began.UnixNano() {
		t.Fatalf("announced_unix_nano = %d on a restate, want %d. This one "+
			"was already right; it is asserted here so a fix to the activity "+
			"age cannot quietly take it with it", got, began.UnixNano())
	}
}

// TestReAnnouncingWithNoLineKeepsTheOneItHas is the silent wipe, and it is the
// case a porting caller walks into without doing anything wrong.
//
// AGENTBOX TAKES THE ACTIVITY AS OPTIONAL ON ANNOUNCE. Its own description
// says set_activity "carries this from then on", so a seat that announces with
// a line and later restates only its PURPOSE supplies no activity at all -
// which is the ordinary shape, not an exotic one. rig read that empty string
// as "I am doing nothing" and blanked the row.
//
// A BLANK ACTIVITY IS THE UNSUPERVISABLE ROW BY ANOTHER DOOR. `serveAnnounce`
// already refuses an empty PURPOSE at the boundary, in as many words: a blank
// one "is indistinguishable from a session nobody is supervising". A row whose
// activity can be blanked by a caller that said nothing about it is that same
// defect, reached without a refusal to trip over.
func TestReAnnouncingWithNoLineKeepsTheOneItHas(t *testing.T) {
	p := newPresence("production", 1)
	at := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return at }

	const line = "waiting for the gate to finish"
	var holder occupancy
	if _, err := p.announce(&holder, stalenessSeat, "the cutover", line); err != nil {
		t.Fatal(err)
	}
	began := at

	// The successor restates the PURPOSE after a handoff and supplies no
	// activity, exactly as the AgentBox shape allows.
	at = at.Add(24 * time.Hour)
	o, err := p.announce(&holder, stalenessSeat, "the cutover, restated", "")
	if err != nil {
		t.Fatal(err)
	}

	if got := o.proto().GetActivity(); got != line {
		t.Fatalf("re-announcing without an activity left the line as %q, want "+
			"%q kept. There is no way on this wire for a peer to mean \"I am "+
			"now doing nothing\", so an absent line is NOT SUPPLIED - and a "+
			"blank one is the row serveAnnounce refuses to create when the "+
			"purpose is blank, arriving by a door nobody guarded", got, line)
	}
	if got := o.proto().GetActivityUnixNano(); got != began.UnixNano() {
		t.Fatalf("the kept line's age is %d, want %d. Keeping the line and "+
			"then dating it to now is the worse half of both answers: the "+
			"board reads a day-old line as this minute's work", got, began.UnixNano())
	}
	if got := o.proto().GetPurpose(); got != "the cutover, restated" {
		t.Fatalf("purpose = %q; restating the purpose is the whole reason "+
			"this call exists and it must still take effect", got)
	}
}

// TWO MORE CASES, ADDED WHEN THE RULE WAS FOUND TO HOLD FOR SEATED PEERS AND
// NOT FOR THE REST.
//
// The carry-over used to be gated on `restating`, which is a TENANCY test -
// same connection, same non-empty seat. So the line and its age only survived
// for a seated peer restating its own seat, and reset for everybody else. A
// tenancy and a line are two facts with two lifetimes, and one condition was
// answering for both.
//
// THE SPLIT IS NOW `restating` FOR THE TENANCY AND `had` FOR THE LINE, and
// these two cases are the halves that gate could not see. Each names a
// mutation ONLY IT goes red under, because an arm that cannot independently go
// red is weight rather than evidence.

// TestAnUnseatedPeerRestatingOneLineDoesNotRefreshItsAge is the case the old
// gate excluded by construction, and it is NOT a case that ages out.
//
// Requiring a seat AT THE DOOR removes it for agents and leaves it exactly
// where it was for everything on the program socket. `cmd/fakeapp` and the
// conformance suite are PERMANENT unseated peers - that is the whole reason
// enforcement went to the door rather than into announce - and their rows sit
// on the roster `crew()` hands a board, sorted last but present.
//
// MUTATION THIS ONE ALONE CATCHES: `if had && seat != ""`. The two seated
// cases still carry their line and stay green; only this reds.
func TestAnUnseatedPeerRestatingOneLineDoesNotRefreshItsAge(t *testing.T) {
	p := newPresence("production", 1)
	at := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return at }

	const line = "waiting for the gate to finish"
	var holder occupancy
	if _, err := p.announce(&holder, "", "a one-off session holding no seat", line); err != nil {
		t.Fatal(err)
	}
	began := at

	// A DAY LATER, word for word, and nothing it is doing has moved.
	at = at.Add(24 * time.Hour)
	o, err := p.announce(&holder, "", "a one-off session holding no seat", line)
	if err != nil {
		t.Fatal(err)
	}

	if got := o.proto().GetActivityUnixNano(); got != began.UnixNano() {
		t.Fatalf("an UNSEATED peer restating an unchanged line a day later "+
			"set its age to %d, want the original %d. The rule is about the "+
			"LINE and this peer's line did not change - it has no tenancy to "+
			"preserve, which is true of its GENERATION and says nothing "+
			"about its activity. A board cannot tell this row from one doing "+
			"fresh work, and unseated rows do not go away: the door requires "+
			"a seat, the program socket does not",
			got, began.UnixNano())
	}
	if got := o.proto().GetGeneration(); got != 0 {
		t.Errorf("generation = %d on an unseated peer, want 0. Carrying the "+
			"LINE across must not carry a tenancy with it - that is the "+
			"split this case exists to hold", got)
	}
}

// TestReSeatingCarriesTheLineItDidNotChange is the third case, and it is the
// one that changed behaviour rather than fixing it.
//
// A connection that announces seat A and then announces a different FREE seat
// B takes a new tenancy: new generation, new `announced`. Its LINE is not part
// of that. It used to reset, and now it carries.
//
// THAT IS DELIBERATE AND IT IS THE STRICTER ANSWER. A session looping on one
// line that also re-seats would otherwise refresh its own age on the way
// through - the freshness a stuck session manufactures for itself, which is
// the exact failure `setLine` exists to stop, arriving through the seat change
// instead of through the repeat. Nothing is lost by it: `announced` separately
// says when this tenancy began and is served on the same row, so a reader has
// both facts and they are not the same fact.
//
// REACHABLE, not theoretical: the refusal loop fires only when ANOTHER
// connection holds the seat, so taking a different free seat is allowed.
//
// MUTATION THIS ONE ALONE CATCHES: `if had && (seat == "" || prev.seat == seat)`.
// Unseated still carries, seated-same-seat still carries, only this reds.
func TestReSeatingCarriesTheLineItDidNotChange(t *testing.T) {
	p := newPresence("production", 1)
	at := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return at }

	const line = "waiting for the gate to finish"
	var holder occupancy
	if _, err := p.announce(&holder, "backend-1", "the door", line); err != nil {
		t.Fatal(err)
	}
	began := at

	// A DAY LATER the same connection moves to a different, free seat and is
	// still saying the same thing it was saying yesterday.
	at = at.Add(24 * time.Hour)
	o, err := p.announce(&holder, "backend-2", "presence now", line)
	if err != nil {
		t.Fatal(err)
	}

	if got := o.proto().GetActivityUnixNano(); got != began.UnixNano() {
		t.Fatalf("re-seating set the unchanged line's age to %d, want the "+
			"original %d. Taking a new seat is a new TENANCY and not new "+
			"WORK: a session repeating one line across a re-seat would "+
			"otherwise renew its own freshness, which is the failure setLine "+
			"exists to stop arriving by another door",
			got, began.UnixNano())
	}
	if got := o.proto().GetAnnouncedUnixNano(); got != at.UnixNano() {
		t.Errorf("announced_unix_nano = %d after re-seating, want %d. The "+
			"TENANCY did restart and must say so - that is the field which "+
			"keeps the line's age from hiding it", got, at.UnixNano())
	}
	if got := o.proto().GetGeneration(); got != 1 {
		t.Errorf("generation = %d in the new seat, want 1. A different seat "+
			"is a different tenancy with its own counter", got)
	}
	if got := o.proto().GetSeat(); got != "backend-2" {
		t.Errorf("seat = %q, want backend-2", got)
	}
}
