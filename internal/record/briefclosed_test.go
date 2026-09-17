package record

import (
	"reflect"
	"testing"
)

// itemWithStatus writes a work item carrying exactly the status given, and an
// EMPTY status writes no status field at all.
//
// ⛔ THE TWO ARE DIFFERENT STORES AND THIS PACKAGE HAS BOTH. Measured in the
// live production store 2026-09-17: 11 of 95 work items carry no `status` key,
// written by rigseed out of a backlog table whose rows have no status column.
// A helper that stamped an empty string instead would test a shape the store
// does not hold.
func itemWithStatus(t *testing.T, s *Store, id, title, status string) string {
	t.Helper()
	fields := map[string]string{"title": title}
	if status != "" {
		fields["status"] = status
	}
	w, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: "rig", Body: title,
		Fields:  fields,
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatalf("writing work item %s with status %q: %v", id, status, err)
	}
	return w.ID
}

// closedWords is the brief's closed section as id -> word, for comparison.
func closedWords(b Brief) map[string]string {
	out := map[string]string{}
	for _, c := range b.Closed {
		out[c.ID] = c.Word
	}
	return out
}

// ⛔ "CLOSED" IS A FAMILY AND NOT A WORD, AND THIS IS THE TEST B68 WAS FILED
// OVER. The live store holds `closed` AND `closed-by-ruling`; a predicate
// written against the literal `"closed"` drops the second silently and leaves
// exactly the invisibility the section was ruled to end.
//
// IT IS ALSO THE MUTATION GUARD. Change the predicate to `== "closed"` and
// this test fails on `B55`; widen it to `!= "active"` and it fails on the idea
// and the statusless row below. Both directions are covered on purpose,
// because the two repairs are each other's failure mode.
func TestTheClosedSectionCarriesEveryClosingWordAndNotOnlyTheWordClosed(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	live := itemWithStatus(t, s, "B90", "still being worked on", "active")
	closed := itemWithStatus(t, s, "B91", "finished and said so", "closed")
	byRuling := itemWithStatus(t, s, "B92", "closed by a ruling of his", "closed-by-ruling")
	retracted := itemWithStatus(t, s, "B93", "a word nobody has written yet", "retracted")
	idea := itemWithStatus(t, s, "B94", "nobody has picked this up", "idea")
	noStatus := itemWithStatus(t, s, "B95", "the seeder wrote no status", "")

	b, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		closed:    "closed",
		byRuling:  "closed-by-ruling",
		retracted: "retracted",
	}
	if got := closedWords(b); !reflect.DeepEqual(got, want) {
		t.Fatalf("the closed section is %v, want %v", got, want)
	}

	// ⛔ NEITHER `idea` NOR A MISSING STATUS IS A CLOSURE. `idea` is section
	// 39's word for "has not been picked up" and an absent status is a defect
	// in whatever wrote the record. Filing either under a heading reading
	// CLOSED states something no document says.
	for _, id := range []string{idea, noStatus} {
		if _, ok := closedWords(b)[id]; ok {
			t.Fatalf("%s is in the closed section; nothing closed it", id)
		}
	}

	// ⛔ AND THE OPEN LIST DOES NOT CHANGE. Boris ruled the shape on exactly
	// this condition - his brief is already the thing he called hard to read -
	// so the closed rows are a section of their own and nothing else moves.
	if len(b.NextUp) != 1 || b.NextUp[0].ID != live {
		t.Fatalf("next up is %+v, want just the one active item %s", b.NextUp, live)
	}
	if len(b.Open) != 0 {
		t.Fatalf("the open list is %+v, want it untouched by the closed section", b.Open)
	}
}

// ⛔ AN ITEM CLOSED BY ITS PROGRESS STREAM IS CLOSED TOO, AND B68's ROW NAMES
// IT: "absence is how this store already expresses closure for the nine
// progress-stepped items". An `active` item whose latest step is `done` is
// dropped from the open list by the SECOND half of the derivation's predicate,
// so a closed section that only reads `status` leaves it invisible - B68
// re-opened for a different reason.
func TestAnItemClosedByADoneStepIsInTheClosedSectionWithThatWord(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	stepped := itemWithStatus(t, s, "B96", "finished but nobody changed its status", "active")
	step(t, s, stepped, "done")
	both := itemWithStatus(t, s, "B97", "closed and stepped done", "closed")
	step(t, s, both, "done")

	b, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{stepped: "done", both: "closed"}
	if got := closedWords(b); !reflect.DeepEqual(got, want) {
		t.Fatalf("the closed section is %v, want %v - the STATUS word wins "+
			"when a record carries one, because it is what a person wrote", got, want)
	}
}

// THE COUNTS ARE PER WORD, which is the census the ruling asks the section to
// show: "12 `closed` and 2 `closed-by-ruling` in the live store".
func TestTheClosedCountsAreKeptPerWordAndOrderedForRepeatability(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	itemWithStatus(t, s, "B90", "one", "closed")
	itemWithStatus(t, s, "B91", "two", "closed")
	itemWithStatus(t, s, "B92", "three", "closed-by-ruling")
	itemWithStatus(t, s, "B93", "four", "active")

	b, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	want := []WordCount{{Word: "closed", Count: 2}, {Word: "closed-by-ruling", Count: 1}}
	if !reflect.DeepEqual(b.ClosedCounts, want) {
		t.Fatalf("the closed counts are %+v, want %+v", b.ClosedCounts, want)
	}
}

// ⛔ A WORK ITEM IN NEITHER LIST IS COUNTED, BECAUSE OPEN + CLOSED NOW READS AS
// THE TOTAL AND IS NOT ONE. 11 of the live store's 95 work items carry no
// status at all, so a reader adding the two lists up is wrong by 11 and has no
// way to find out. The count is the one honest thing a derivation can say
// about a record whose status nobody wrote.
func TestWorkItemsInNeitherListAreCounted(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	itemWithStatus(t, s, "B90", "live", "active")
	itemWithStatus(t, s, "B91", "closed", "closed")
	itemWithStatus(t, s, "B92", "an idea", "idea")
	itemWithStatus(t, s, "B93", "no status at all", "")

	b, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	if b.Unlisted != 2 {
		t.Fatalf("the brief counts %d work items in neither list, want 2 - "+
			"the idea and the one with no status", b.Unlisted)
	}
}

// THE SECTION DECLARES ITS OWN STATE like the other twelve, and the ledger in
// sections.go REFUSES a brief where it does not. This test is what turns that
// refusal into a failure somebody reads.
func TestTheClosedSectionDeclaresItsOwnState(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	itemWithStatus(t, s, "B90", "live", "active")

	b, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range b.Sections {
		if st.Section != SectionClosed {
			continue
		}
		if st.State != SectionComputed {
			t.Fatalf("the closed section reports %q (%s), want computed", st.State, st.Reason)
		}
		return
	}
	t.Fatalf("no section state for %q in %+v - a section that does not "+
		"declare itself is the silent absence the ledger exists to refuse",
		SectionClosed, b.Sections)
}
