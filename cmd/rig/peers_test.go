package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// now is a fixed clock, so an age is a value this file can assert rather than
// a race against the test's own runtime.
var now = time.Unix(1_700_000_000, 0)

// ago builds a timestamp d before that clock.
func ago(d time.Duration) int64 { return now.Add(-d).UnixNano() }

func seat(mut ...func(*verbsv1.Seat)) *verbsv1.Seat {
	s := &verbsv1.Seat{
		Seat:              "backend-1",
		Generation:        2,
		Epoch:             7,
		Estate:            "production",
		Purpose:           "the continuity record",
		Activity:          "sizing the slices",
		State:             verbsv1.SeatState_SEAT_STATE_ACTIVE,
		AnnouncedUnixNano: ago(90 * time.Minute),
		ActivityUnixNano:  ago(30 * time.Second),
	}
	for _, m := range mut {
		m(s)
	}
	return s
}

// ---- the empty roster, which is an ANSWER and not a blank -------------------

// AN EMPTY ROSTER IS THE ORDINARY FIRST CALL and it must not read as a broken
// command. `rig peers` on a daemon nothing has announced to is what a person
// runs before anything is up.
func TestAnEmptyRosterIsASentenceAndNotABlankTable(t *testing.T) {
	got := peersText(&verbsv1.PeersResponse{}, now)

	if strings.Contains(got, "SEAT") {
		t.Errorf("an empty roster printed a table header, which reads as a "+
			"broken command:\n%s", got)
	}
	if !strings.Contains(got, "nobody has announced") {
		t.Errorf("an empty roster did not say so in words:\n%s", got)
	}
	// The reader of an empty roster is usually somebody who expected not to be
	// alone, so the answer has to say what WOULD put a row here.
	if !strings.Contains(got, "announce") {
		t.Errorf("an empty roster did not say what puts a row on it:\n%s", got)
	}
}

// `crew` MUST BE [] AND NEVER null. A null is indistinguishable from a field
// this build failed to set, and a consumer branching on it reads "the daemon
// is broken" out of "nobody is here".
func TestAnEmptyRosterEmitsAnEmptyArrayRatherThanNull(t *testing.T) {
	b, err := json.Marshal(peersJSON(&verbsv1.PeersResponse{}, now))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"crew":[]`) {
		t.Errorf("want an empty crew array, got: %s", b)
	}
}

// ---- the unseated peer -----------------------------------------------------

// AN UNSEATED PEER IS A FACT THE WIRE ALLOWS ON PURPOSE - a one-off session is
// present and addressable without pretending to be a seat somebody inherits.
// Rendered as an empty cell it reads as a daemon that failed to send the name.
func TestAPeerWithNoSeatRendersAsAFactRatherThanABlank(t *testing.T) {
	row := peersSeatCell(seat(func(s *verbsv1.Seat) {
		s.Seat = ""
		s.Generation = 0
	}))

	if strings.TrimSpace(row) == "" {
		t.Fatal("an unseated peer rendered as an empty cell, which reads as a " +
			"daemon that failed to send the name")
	}
	if !strings.Contains(row, "(") {
		t.Errorf("an unseated peer rendered as %q, which could be mistaken for "+
			"a seat name; it must be a shape a seat name cannot take", row)
	}
}

// THE GENERATION TRAVELS WITH THE NAME. `backend-1` at generation 2 is a
// different occupancy from `backend-1` at generation 1, and a reader scanning
// a column of bare names sees one seat where there were two.
func TestASeatCarriesItsGenerationSoTwoOccupanciesDoNotReadAsOne(t *testing.T) {
	first := peersSeatCell(seat(func(s *verbsv1.Seat) { s.Generation = 1 }))
	second := peersSeatCell(seat(func(s *verbsv1.Seat) { s.Generation = 2 }))

	if first == second {
		t.Fatalf("two generations of one seat rendered identically as %q, so a "+
			"reader cannot tell the occupancy changed", first)
	}
}

// ---- the age, which is the whole reason the column exists ------------------

// A ZERO TIMESTAMP IS NOT 1970. Computed straight it reports roughly 56 years,
// which a reader will believe less than they believe a dash.
func TestATimestampThatWasNeverSetIsNotRenderedAsAnAge(t *testing.T) {
	if got := peersAgeCell(0, now); got != "-" {
		t.Errorf("an unset timestamp rendered as %q, want a dash: computed "+
			"straight it reads as decades", got)
	}
	if got := ageSeconds(0, now); got != -1 {
		t.Errorf("ageSeconds(0) = %d, want -1: a real age cannot be negative, "+
			"so -1 cannot be mistaken for a measurement", got)
	}
}

// THE AGE IS THE BOARD'S ONLY SIGNAL THAT A SESSION IS REPEATING ITSELF. The
// daemon deliberately does not reset it when an unchanged line is re-sent, so
// a renderer that collapsed every age would delete the signal the rule buys.
func TestTheAgeColumnDistinguishesAFreshLineFromAStaleOne(t *testing.T) {
	fresh := peersAgeCell(ago(5*time.Second), now)
	stale := peersAgeCell(ago(4*time.Hour), now)

	if fresh == stale {
		t.Fatalf("a 5-second line and a 4-hour line both rendered as %q, which "+
			"deletes the activity-age signal entirely", fresh)
	}
	if !strings.HasSuffix(fresh, "s") || !strings.HasSuffix(stale, "h") {
		t.Errorf("want seconds and hours to carry their units, got %q and %q",
			fresh, stale)
	}
}

// ---- `partial`, the field with three meanings ------------------------------

// ⛔ THE FIELD HAS THREE MEANINGS ACROSS TWO PRODUCTS AND THIS IS THE ONE
// PLACE A PERSON READS IT. AgentBox's `partial` is a general disclaimer; rig's
// is one specific, checkable claim. Printing the flag invites the wrong one.
func TestPartialSaysWhatItMeansRatherThanPrintingTheFlag(t *testing.T) {
	got := peersText(&verbsv1.PeersResponse{
		Crew:    []*verbsv1.Seat{seat()},
		Partial: true,
	}, now)

	if !strings.Contains(got, "another estate") {
		t.Errorf("partial did not name what it actually means - another estate "+
			"running on this machine:\n%s", got)
	}
	if strings.Contains(got, "partial: true") {
		t.Errorf("partial printed as a raw flag, which invites AgentBox's "+
			"general-disclaimer reading:\n%s", got)
	}
}

// FALSE GETS A LINE TOO. Silence on the negative case teaches a reader to treat
// absence as a value, and "this is everybody" is what somebody deciding whether
// they are alone actually needs.
func TestACompleteRosterSaysSoRatherThanStayingSilent(t *testing.T) {
	got := peersText(&verbsv1.PeersResponse{Crew: []*verbsv1.Seat{seat()}}, now)

	if !strings.Contains(got, "everybody") {
		t.Errorf("a complete roster did not say it was complete, so a reader "+
			"cannot tell it from one that forgot to mention it:\n%s", got)
	}
}

// ---- the enum's zero and a newer daemon ------------------------------------

// SECTION 21: the zero means nothing was said and is never a fact about a
// seat. A reader who sees it is looking at a defect, and the cell has to be
// the thing that tells them - it must not render as ACTIVE.
// ⛔ ASSERTING unset != active IS TOO WEAK AND THIS TEST USED TO DO ONLY THAT.
// A MUTATION PROVED IT: deleting the special case renders the bare enum word
// "UNSPECIFIED", which differs from "ACTIVE", so the test stayed green against
// the exact defect it was written for. The cell must say the state was NOT
// SAID, in a shape no enum word can take - a reader has to be told they are
// looking at a defect, not handed a fourth state spelling to interpret.
func TestAnUnsetStateDoesNotRenderAsAnActiveSeat(t *testing.T) {
	unset := peersStateCell(seat(func(s *verbsv1.Seat) {
		s.State = verbsv1.SeatState_SEAT_STATE_UNSPECIFIED
	}))
	active := peersStateCell(seat())

	if unset == active {
		t.Fatalf("an unset state rendered identically to an active one (%q), "+
			"so a defect reads as a working seat", unset)
	}
	if !strings.HasPrefix(unset, "(") {
		t.Fatalf("an unset state rendered as %q, which is a bare state "+
			"spelling. Section 21's zero means NOTHING WAS SAID and is never a "+
			"fact about a seat, so it must render in a shape no enum word can "+
			"take - otherwise it reads as a fourth state rather than a defect",
			unset)
	}
	if word, _ := stateLabel(verbsv1.SeatState_SEAT_STATE_UNSPECIFIED); strings.Contains(unset, word) {
		t.Errorf("the unset cell %q contains the enum's own word %q, which is "+
			"the spelling a reader will carry away as the state's name",
			unset, word)
	}
}

// A STATE THIS BUILD HAS NO NAME FOR IS A NEWER DAEMON ON THE SAME WIRE MAJOR,
// and it must NOT fall back to the zero's spelling - that would report a skew
// as "nothing was said" and send the reader looking in the wrong place.
func TestAnUnknownStateReadsAsSkewAndNotAsSilence(t *testing.T) {
	future := peersStateCell(seat(func(s *verbsv1.Seat) { s.State = verbsv1.SeatState(99) }))
	unset := peersStateCell(seat(func(s *verbsv1.Seat) {
		s.State = verbsv1.SeatState_SEAT_STATE_UNSPECIFIED
	}))

	if future == unset {
		t.Fatalf("an unrecognised state rendered as the zero's spelling (%q), "+
			"which reports a skew as 'nothing was said'", future)
	}
}

// state_number rides beside state ALWAYS, because a key that appears only when
// something is wrong is a key nobody's parser has a branch for at the moment it
// first appears.
func TestTheStateNumberIsEmittedEvenWhenTheStateIsUnderstood(t *testing.T) {
	obj := seatJSON(seat(), now)

	if _, ok := obj["state_number"]; !ok {
		t.Fatal("state_number is absent on a state this build understands, so " +
			"a consumer meeting a fourth state has no branch for it")
	}
}

// ---- every field reaches the row -------------------------------------------

// NOTHING HERE IS ENFORCED BY THE COMPILER. seatJSON is a keyed literal, so a
// field added to Seat and forgotten here compiles and the row arrives with a
// zero no reader can tell from a real value. This is the same guard
// TestEverySeatFieldReachesTheRosterRow gives the door, on the CLI's own
// renderer, because the two surfaces are rendered by different code.
func TestEverySeatFieldReachesTheJSONRow(t *testing.T) {
	base := seatJSON(&verbsv1.Seat{}, now)

	for _, tc := range []struct {
		field string
		mut   func(*verbsv1.Seat)
	}{
		{"seat", func(s *verbsv1.Seat) { s.Seat = "backend-9" }},
		{"generation", func(s *verbsv1.Seat) { s.Generation = 3 }},
		{"epoch", func(s *verbsv1.Seat) { s.Epoch = 11 }},
		{"estate", func(s *verbsv1.Seat) { s.Estate = "development" }},
		{"purpose", func(s *verbsv1.Seat) { s.Purpose = "a purpose" }},
		{"activity", func(s *verbsv1.Seat) { s.Activity = "an activity" }},
		{"state", func(s *verbsv1.Seat) { s.State = verbsv1.SeatState_SEAT_STATE_HANDING_OFF }},
		{"announced_unix_nano", func(s *verbsv1.Seat) { s.AnnouncedUnixNano = ago(time.Hour) }},
		{"activity_unix_nano", func(s *verbsv1.Seat) { s.ActivityUnixNano = ago(time.Minute) }},
	} {
		s := &verbsv1.Seat{}
		tc.mut(s)
		got := seatJSON(s, now)

		changed := false
		for k, v := range got {
			if base[k] != v {
				changed = true
				break
			}
		}
		if !changed {
			t.Errorf("setting Seat.%s alone changed nothing in the JSON row, so "+
				"that field reaches no reader of `rig peers --json`", tc.field)
		}
	}
}

// ---- the table -------------------------------------------------------------

// THE LAST COLUMN IS NEVER PADDED. Every row here ends in free text, and a
// padded final cell drags a run of trailing spaces across the terminal - which
// is why this does not reach for text/tabwriter.
func TestTheLastColumnCarriesNoTrailingPadding(t *testing.T) {
	got := peersText(&verbsv1.PeersResponse{Crew: []*verbsv1.Seat{
		seat(func(s *verbsv1.Seat) { s.Activity = "short" }),
		seat(func(s *verbsv1.Seat) { s.Activity = "a considerably longer activity line" }),
	}}, now)

	for _, line := range strings.Split(got, "\n") {
		if line != strings.TrimRight(line, " ") {
			t.Errorf("a row carries trailing padding: %q", line)
		}
	}
}

// ---- leases, section 16 ----------------------------------------------------

// The two facts a reader acts on are words in the NOTE column, and a refused
// lease list is reported under the roster rather than failing it.
func TestLeasesTextSaysWhatAReaderActsOn(t *testing.T) {
	got := leasesText(&verbsv1.LeaseListResponse{Leases: []*verbsv1.Lease{
		{
			Name: "deploy", State: verbsv1.LeaseState_LEASE_STATE_HELD,
			Holder: "seat-a", Witness: "pid 42", RemainingMs: 30_000,
			OwnerGone: true, Liveness: verbsv1.Liveness_LIVENESS_DEAD,
		},
		{
			Name: "vm", State: verbsv1.LeaseState_LEASE_STATE_ORPHANED,
			Holder: "seat-b", Witness: "unwitnessed", RemainingMs: -5000,
			NeedsBreak: true, Liveness: verbsv1.Liveness_LIVENESS_UNKNOWN,
		},
		{
			Name: "db", State: verbsv1.LeaseState_LEASE_STATE_FREE,
			Holder: "seat-c", Witness: "unwitnessed",
			BrokenBy: "seat-a", BrokenReason: "torn down",
		},
	}}, "")
	for _, want := range []string{
		"LEASE", "deploy", "held", "seat-a (pid 42)", "30s",
		"holder observed dead: frees at its deadline",
		"vm", "orphaned", "needs a recorded break",
		"db", "free", "broken by seat-a: torn down",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the lease section does not say %q:\n%s", want, got)
		}
	}

	if got := leasesText(&verbsv1.LeaseListResponse{}, ""); !strings.Contains(got, "No lease has ever been taken") {
		t.Errorf("an empty lease list renders as %q, want a sentence", got)
	}
	if got := leasesText(nil, "rig.lease.list: CODE_UNAVAILABLE: no store"); !strings.Contains(got, "not available") ||
		!strings.Contains(got, "no store") {
		t.Errorf("a refused lease list renders as %q, want the refusal named", got)
	}
}

// Every key on every row, and an empty array rather than null.
func TestLeasesJSONCarriesEveryKey(t *testing.T) {
	b, err := json.Marshal(leasesJSON(&verbsv1.LeaseListResponse{}))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Errorf("no leases marshals as %s, want []", b)
	}
	rows := leasesJSON(&verbsv1.LeaseListResponse{Leases: []*verbsv1.Lease{{
		Name: "deploy", State: verbsv1.LeaseState_LEASE_STATE_HELD,
		Liveness: verbsv1.Liveness_LIVENESS_ALIVE,
	}}})
	for _, k := range []string{
		"name", "state", "holder", "token", "epoch", "witness", "remaining_ms",
		"owner_gone", "liveness", "needs_break", "broken_by", "broken_reason",
	} {
		if _, ok := rows[0][k]; !ok {
			t.Errorf("a lease row has no %q key", k)
		}
	}
	if rows[0]["state"] != "held" || rows[0]["liveness"] != "alive" {
		t.Errorf("state %v liveness %v, want held and alive", rows[0]["state"], rows[0]["liveness"])
	}
}
