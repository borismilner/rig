package record

import (
	"errors"
	"math"
	"sort"
	"strings"
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
	if _, err := s.Put(tctx, PutRequest{
		ID: idea, Kind: "work-item", Project: "rig", Body: "which store backs the record",
		Fields:  map[string]string{"title": "which store backs the record", "status": "idea"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(tctx, idea, LinkBlocks, blocked); err != nil {
		t.Fatal(err)
	}

	b, err := s.Brief(tctx, "rig")
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

// ⛔ THE SIGNED COLUMN AND THE UNSIGNED FIELD, AND THE WIRE MADE IT REACHABLE.
//
// SQLite's INTEGER is int64 and Version and Epoch are uint64, so every crossing
// can wrap. That was unreachable while rig's own callers were the only ones -
// a version counts up from 1 - and rig 05a3ceb serves these verbs over a socket,
// so GetVersion takes its version from a remote caller now. A wrapped write is
// the worst available shape: it succeeds, and the record reads back under a
// version nobody asked for.
func TestAVersionTooLargeForTheColumnIsRefusedByNameRatherThanWrapped(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	item := workItem(t, s, "b41", "the CLI roster verb")

	// math.MaxInt64 + 1 is the first value that wraps NEGATIVE in the column.
	_, err := s.GetVersion(tctx, item, 1<<63)
	if err == nil {
		t.Fatal("a version past the column's range was accepted; it wraps negative and matches the wrong row")
	}
	// ⛔ AND IT MUST BE THE RANGE CHECK TALKING, NOT THE LOOKUP. Both are
	// reachable from this call and err != nil cannot tell them apart - the
	// defect shape this package has already shipped three times.
	if !strings.Contains(err.Error(), "too large for the store") {
		t.Fatalf("refused with %q, which is not the range check; a NotFoundError here "+
			"would mean the conversion happened and simply missed", err)
	}

	// A version inside the range is a plain miss, and says so differently.
	_, err = s.GetVersion(tctx, item, 99)
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("an in-range version that does not exist gave %v, want a NotFoundError", err)
	}
}

func TestANegativeColumnIsReportedAsCorruptionRatherThanAHugeNumber(t *testing.T) {
	if _, err := fromColumn("version", -1); err == nil {
		t.Fatal("a negative version was widened to a very large one instead of being refused")
	} else if !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("refused with %q; a negative column is not a caller's mistake and the message should say so", err)
	}
	for _, v := range []int64{0, 1, math.MaxInt64} {
		if got, err := fromColumn("version", v); err != nil || got != uint64(v) {
			t.Fatalf("fromColumn(%d) = %d, %v; want %d, nil", v, got, err, v)
		}
	}
	if _, err := toColumn("epoch", math.MaxInt64); err != nil {
		t.Fatalf("the largest representable epoch was refused: %v", err)
	}
}

// ⛔ THE STREAM IS ORDERED BY id, AND A CLOCK THAT GOES BACKWARDS IS WHAT
// PROVES IT HAS TO BE.
//
// lateststeps.go argues that created_at is not usable as an ordering and that a
// UUIDv7 id is both the order and the tie-break. Stream ordered by
// `created_at, id` until rig 0833eca's successor - correct, because id caught
// every tie, and contradicting that argument in print.
//
// A FROZEN CLOCK DOES NOT PROVE IT AND THAT WAS THE FIRST ATTEMPT. With every
// created_at identical, SQLite returns rows in rowid order, which is insertion
// order, which agrees with the ids - so `ORDER BY created_at` passes. The two
// forms cannot disagree while the clock only ever moves forward.
//
// SO THE CLOCK GOES BACKWARDS HERE, which is what an NTP correction does to a
// long-running daemon. The ids stay monotonic because v7 mints them in
// sequence, the stamps do not, and the two orderings finally disagree.
func TestTheStreamIsOrderedByIdEvenWhenTheClockGoesBackwards(t *testing.T) {
	base := time.Date(2026, 9, 16, 21, 0, 0, 0, time.UTC)
	// Each step is stamped EARLIER than the one before it.
	offsets := []time.Duration{
		50 * time.Second, 40 * time.Second, 30 * time.Second,
		20 * time.Second, 10 * time.Second,
	}
	var n int
	old := now
	now = func() time.Time {
		d := offsets[min(n, len(offsets)-1)]
		n++
		return base.Add(d)
	}
	t.Cleanup(func() { now = old })

	name := estate(t, "development")
	s := openStore(t, name)
	item := workItem(t, s, "b41", "the CLI roster verb")

	want := []string{"started", "blocked", "started", "done"}
	for _, st := range want {
		step(t, s, item, st)
	}

	stream, err := s.Stream(tctx, item)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream) != len(want) {
		t.Fatalf("the stream has %d steps, want %d", len(stream), len(want))
	}

	// ⛔ THE PREMISE IS CHECKED INDEPENDENTLY OF THE THING UNDER TEST.
	//
	// Reading the premise off `stream` in the order Stream returned it cannot
	// tell "the clock did not go backwards" from "Stream ordered by the clock" -
	// the two produce the same failure, which is this package's own err != nil
	// defect wearing a different coat. So the stamps are checked against a copy
	// sorted by id, which is true whatever order Stream chose.
	byID := append([]Record(nil), stream...)
	sort.Slice(byID, func(i, j int) bool { return byID[i].ID < byID[j].ID })
	for i := 1; i < len(byID); i++ {
		if !byID[i].Prov.CreatedAt.Before(byID[i-1].Prov.CreatedAt) {
			t.Fatalf("in id order, step %d is stamped %v and is not before step %d's %v - "+
				"the clock did not go backwards and this test proves nothing",
				i, byID[i].Prov.CreatedAt, i-1, byID[i-1].Prov.CreatedAt)
		}
	}

	for i, w := range want {
		if got := stream[i].Fields["state"]; got != w {
			t.Fatalf("step %d is %q, want %q - the stream followed the clock instead of the id",
				i, got, w)
		}
	}
}
