package record

import (
	"errors"
	"fmt"
	"testing"
)

// B116's store half: Find's answer had no bound, so `record.query --kind
// requirement` over the whole plan grew to 1,123,924 bytes, past
// wire.MaxFrameSize, and the query died. FindPage is the bounded read.
//
// EVERY TEST HERE COMPARES A PAGED WALK AGAINST Find's OWN ANSWER, never
// against a hand-written list. A paging bug is a SKIP or a REPEAT at a page
// boundary, and the only thing that can prove neither happened is the
// unbounded answer the pages must reconstruct exactly - same records, same
// order, same length.

// pagingFixture writes 300 records across two projects and three kinds, so a
// page boundary falls inside a project, inside a kind, and on the seam between
// two of each.
//
// 300 with a small limit means dozens of boundaries rather than one, which is
// what separates "the cursor works" from "the cursor works at the one place
// the fixture happened to break".
func pagingFixture(t *testing.T, s *Store) {
	t.Helper()
	projects := []string{"alpha", "beta"}
	kinds := []string{"decision", "note", "work-item"}

	// ⛔ THE OWNER CYCLES ON A DIFFERENT PERIOD FROM THE KIND, and the first
	// version of this fixture did not: both cycled on i%3, so every `note`
	// carried owner=lead and the kind-and-field shape answered NOTHING. A
	// fixture whose two predicates are the same predicate cannot tell a
	// working conjunction from a broken one.
	owners := []string{"boris", "lead", ""}
	for i := range 300 {
		id := fmt.Sprintf("R%03d", i)
		if _, err := s.Put(tctx, PutRequest{
			ID:      id,
			Kind:    kinds[i%len(kinds)],
			Project: projects[i%len(projects)],
			Body:    "body of " + id,
			Fields:  map[string]string{"owner": owners[(i/3)%len(owners)]},
			Session: "s",
			Seat:    "test",
		}); err != nil {
			t.Fatalf("writing %s: %v", id, err)
		}
	}
}

// pageAll walks every page of a filter and returns what the walk saw.
//
// ⛔ IT STOPS ON A SHORT PAGE, NOT ON AN EMPTY ONE, and the guard counts the
// iterations: a cursor that fails to advance is an infinite loop, which is the
// one paging defect a union assertion cannot see because the test never
// finishes to make it.
func pageAll(t *testing.T, s *Store, f QueryFilter, limit int) []Record {
	t.Helper()
	var out []Record
	var cur Cursor
	for i := 0; ; i++ {
		if i > 1000 {
			t.Fatalf("the paging walk did not end after %d pages of %d: "+
				"the cursor is not advancing", i, limit)
		}
		page, err := s.FindPage(tctx, f, cur, limit)
		if err != nil {
			t.Fatalf("page %d of %s: %v", i, f.describe(), err)
		}
		if len(page) > limit {
			t.Fatalf("page %d carried %d records, over the limit of %d",
				i, len(page), limit)
		}
		out = append(out, page...)
		if len(page) < limit {
			return out
		}
		cur = CursorAt(page[len(page)-1])
	}
}

// sameRecords asserts two answers are the same records in the same order.
//
// IT NAMES THE FIRST DIFFERENCE rather than dumping both lists: at 300 records
// a diff of two slices is unreadable, and the index of the first divergence is
// what says whether a page boundary skipped or repeated.
func sameRecords(t *testing.T, what string, got, want []Record) {
	t.Helper()
	for i := range min(len(got), len(want)) {
		if got[i].ID != want[i].ID || got[i].Version != want[i].Version {
			t.Fatalf("%s: at position %d the walk saw %s v%d, Find has %s v%d",
				what, i, got[i].ID, got[i].Version,
				want[i].ID, want[i].Version)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%s: the walk saw %d records, Find answers %d - the "+
			"difference is at the end, so a page was dropped or repeated",
			what, len(got), len(want))
	}
}

// TestPagesUnionToTheWholeAnswer is the acceptance line from plan/50: union
// complete, no duplicate, no gap.
func TestPagesUnionToTheWholeAnswer(t *testing.T) {
	s := openStore(t, estate(t, "paging"))
	pagingFixture(t, s)

	whole, err := s.Find(tctx, QueryFilter{})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(whole) != 300 {
		t.Fatalf("the fixture holds %d records, want 300", len(whole))
	}

	// Several limits, because a bug that only shows when the last page is
	// exactly full is a real one: 300 divides by 10 and not by 7 or 13.
	for _, limit := range []int{1, 7, 10, 13, 299, 300, 301} {
		got := pageAll(t, s, QueryFilter{}, limit)
		sameRecords(t, fmt.Sprintf("walking in pages of %d", limit), got, whole)

		seen := make(map[string]int, len(got))
		for _, r := range got {
			seen[r.ID]++
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("in pages of %d, %s came back %d times", limit, id, n)
			}
		}
	}
}

// TestEveryQueryShapePagesTheSameAnswerItFinds walks all eight shapes.
//
// ⛔ THIS IS THE ARGUMENT-ORDER TEST, and it is the reason it covers all eight
// rather than a representative one. The cursor's three values are bound AFTER
// the filter's, so every shape binds a different number of values before them;
// a shape whose arguments are one position out compares the cursor against the
// wrong column and answers something plausible rather than failing.
func TestEveryQueryShapePagesTheSameAnswerItFinds(t *testing.T) {
	s := openStore(t, estate(t, "pagingshapes"))
	pagingFixture(t, s)

	for _, f := range []QueryFilter{
		{},
		{Project: "alpha"},
		{Kind: "note"},
		{Project: "alpha", Kind: "note"},
		{Field: "owner", Value: "boris"},
		{Project: "alpha", Field: "owner", Value: "boris"},
		{Kind: "note", Field: "owner", Value: "boris"},
		{Project: "alpha", Kind: "note", Field: "owner", Value: "boris"},

		// The empty VALUE is a real predicate, not a wildcard, and it must
		// survive paging like any other.
		{Field: "owner", Value: ""},
	} {
		want, err := s.Find(tctx, f)
		if err != nil {
			t.Fatalf("Find %s: %v", f.describe(), err)
		}
		if len(want) == 0 {
			t.Fatalf("the fixture answers nothing for %s, so paging it "+
				"proves nothing", f.describe())
		}
		got := pageAll(t, s, f, 4)
		sameRecords(t, "paging "+f.describe(), got, want)
	}
}

// TestACursorPositionsWithoutLookingTheRowUp is plan/50's "one that decodes
// but names a row no longer present still positions".
func TestACursorPositionsWithoutLookingTheRowUp(t *testing.T) {
	s := openStore(t, estate(t, "pagingghost"))
	pagingFixture(t, s)

	whole, err := s.Find(tctx, QueryFilter{})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	// A cursor at a row that never existed, sorting between two that do: same
	// project and kind as whole[0], an id one byte below whole[1]'s.
	ghost := Cursor{
		Project: whole[0].Project,
		Kind:    whole[0].Kind,
		ID:      whole[0].ID + "~",
	}
	page, err := s.FindPage(tctx, QueryFilter{}, ghost, 3)
	if err != nil {
		t.Fatalf("paging from a cursor naming no row: %v", err)
	}
	if len(page) == 0 {
		t.Fatal("a cursor naming a row that does not exist answered nothing: " +
			"the store is looking the row up instead of comparing against it")
	}
	if page[0].ID != whole[1].ID {
		t.Errorf("a cursor just past %s resumed at %s, want %s",
			whole[0].ID, page[0].ID, whole[1].ID)
	}

	// And the same after a row is really gone: retract whole[1], then resume
	// from a cursor that still names it.
	if _, err := s.Retract(tctx, RetractRequest{
		ID: whole[1].ID, Reason: "paging test", Session: "s", Seat: "test",
	}); err != nil {
		t.Fatalf("retracting %s: %v", whole[1].ID, err)
	}
	page, err = s.FindPage(tctx, QueryFilter{}, CursorAt(whole[1]), 3)
	if err != nil {
		t.Fatalf("paging from a cursor at a retracted row: %v", err)
	}
	if len(page) == 0 || page[0].ID != whole[2].ID {
		t.Fatalf("a cursor at the retracted %s resumed at %v, want %s",
			whole[1].ID, recIDs(t, page), whole[2].ID)
	}
}

// TestAPagedReadWithNoBoundIsRefused: the limit is the whole point of the
// method, so losing it must fail rather than answer everything.
func TestAPagedReadWithNoBoundIsRefused(t *testing.T) {
	s := openStore(t, estate(t, "pagingnobound"))
	pagingFixture(t, s)

	for _, limit := range []int{0, -1} {
		if _, err := s.FindPage(tctx, QueryFilter{}, Cursor{}, limit); !errors.Is(err, ErrPageLimit) {
			t.Errorf("a paged read with limit %d returned %v, want ErrPageLimit",
				limit, err)
		}
	}
}

// TestAPagedReadRefusesAValueWithNoField: the guard Find carries is not lost
// by going through the other method. Dropping the predicate would widen the
// answer silently, and a paged widening is harder to notice than a whole one.
func TestAPagedReadRefusesAValueWithNoField(t *testing.T) {
	s := openStore(t, estate(t, "pagingvalue"))

	_, err := s.FindPage(tctx, QueryFilter{Value: "boris"}, Cursor{}, 10)
	if !errors.Is(err, ErrValueWithoutField) {
		t.Errorf("a paged read naming a value with no field returned %v, "+
			"want ErrValueWithoutField", err)
	}
}
