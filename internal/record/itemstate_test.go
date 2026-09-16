package record

import (
	"testing"
	"time"
)

// ⛔ ItemState.Age SHIPPED WITH NO CALLER AND NO TEST, AND IT IS THE PACKAGE
// HALF OF A DOCUMENTED WIRE PROMISE.
//
// Section 39's eleven-section table, row 1: open items are carried "each with
// its last progress step and its age", because "a stale step beside a live
// session is the signal that a seat is stuck". proto/rig/v1/wire.proto puts the
// consumer's half of it on ItemState.since_unix_nano in as many words - "a
// reader must not render that as 'infinitely stale': an item nobody has stepped
// sorts BELOW every real signal, not above it". Both halves existed; nothing
// exercised either.
func TestAnItemWithNoStepsHasAgeZeroRatherThanInfinitelyStale(t *testing.T) {
	asOf := time.Date(2026, 9, 16, 21, 0, 0, 0, time.UTC)

	// The stream is EMPTY, which is a real state - picked up, not yet reported
	// on - and not missing data.
	empty := ItemState{ID: "b41", Title: "the CLI roster verb"}
	if empty.Since != (time.Time{}) {
		t.Fatalf("a stateless ItemState has Since %v, want the zero time", empty.Since)
	}
	if got := empty.Age(asOf); got != 0 {
		t.Fatalf("an item with no steps has age %v, want 0 - a non-zero age here "+
			"is measured from the zero time and sorts the item above every real signal", got)
	}
}

func TestAgeIsMeasuredFromTheLatestStepAndGrowsWithTheClock(t *testing.T) {
	since := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	it := ItemState{ID: "b41", State: "started", Since: since}

	for _, c := range []struct {
		asOf time.Time
		want time.Duration
	}{
		{since, 0},
		{since.Add(90 * time.Minute), 90 * time.Minute},
		{since.Add(36 * time.Hour), 36 * time.Hour},
	} {
		if got := it.Age(c.asOf); got != c.want {
			t.Fatalf("age at %v is %v, want %v", c.asOf, got, c.want)
		}
	}

	// A STALE ITEM MUST OUTRANK A FRESH ONE, which is the whole purpose of the
	// field, and an item with no steps must outrank NEITHER.
	fresh := ItemState{ID: "b45", Since: since.Add(23 * time.Hour)}
	never := ItemState{ID: "b46"}
	asOf := since.Add(24 * time.Hour)
	if it.Age(asOf) <= fresh.Age(asOf) {
		t.Fatalf("a day-old step (%v) does not outrank an hour-old one (%v)", it.Age(asOf), fresh.Age(asOf))
	}
	if never.Age(asOf) >= fresh.Age(asOf) {
		t.Fatalf("an item nobody has stepped (%v) outranks a real signal (%v)", never.Age(asOf), fresh.Age(asOf))
	}
}

// ⛔ AN ITEM WAITING ON SOMETHING NOBODY HAS PICKED UP IS BLOCKED, AND ITS
// BLOCKER IS IN NEITHER LIST. Ruled by the team-lead, rig fee7580.
//
// The brief used to derive `blocked` from blocksAmong(active), which filters
// BOTH ends to the active set. That is right for a blocker whose latest step is
// `done` and wrong for every other kind: an item whose status is `idea` is also
// outside the active set, so the brief called X ready while the thing X waits on
// had not been started.
func TestAnItemBlockedByAnIdeaIsBlockedAndItsBlockerIsInNeitherList(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	blocked := workItem(t, s, "b29", "where the record lives on disk")
	step(t, s, blocked, "started")

	// The blocker is an IDEA: nobody has picked it up, so it is not active.
	idea := "b28"
	if _, err := s.Put(PutRequest{
		ID: idea, Kind: "work-item", Project: "rig", Body: "which store backs the record",
		Fields:  map[string]string{"title": "which store backs the record", "status": "idea"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(idea, LinkBlocks, blocked); err != nil {
		t.Fatal(err)
	}

	b, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}

	if len(b.Blocked) != 1 {
		t.Fatalf("blocked is %+v, want %s waiting on the idea %s", b.Blocked, blocked, idea)
	}
	if b.Blocked[0].Item != blocked {
		t.Fatalf("blocked names %q, want %q", b.Blocked[0].Item, blocked)
	}
	if len(b.Blocked[0].BlockedBy) != 1 || b.Blocked[0].BlockedBy[0] != idea {
		t.Fatalf("blocked by %v, want just the idea %s", b.Blocked[0].BlockedBy, idea)
	}

	// IT IS NOT NEXT UP. It cannot be started; something has to start its
	// blocker first.
	if pos(briefIDs(b), blocked) < len(b.NextUp) {
		t.Fatalf("%s is in next-up at %v while it waits on an unstarted item: %v",
			blocked, pos(briefIDs(b), blocked), briefIDs(b))
	}

	// ⛔ AND THE BLOCKER IS IN NEITHER LIST, WHICH IS THE CASE THE WIRE HAD TO
	// BE TOLD ABOUT: a renderer cannot resolve it against the brief's own
	// lists, so proto Blockage carries the blocker's title and state itself.
	if contains(briefIDs(b), idea) {
		t.Fatalf("the idea %s appears in next-up or open: %v", idea, briefIDs(b))
	}
}
