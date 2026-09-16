package record

import "testing"

const prioProject = "prio"

// ⛔ THE IDS ARE CHOSEN SO THAT SORTING BY ID ALONE CANNOT PASS THESE TESTS.
//
// Ascending by id gives high, low, medium, none. The ruled rank gives high,
// medium, low, none. They differ at the second element, so a leftover
// sort.Strings - the exact thing this change replaces - goes red rather than
// staying green by coincidence. An id set that happened to agree with the rank
// would make every assertion below decorative.
//
// The four values are the three ruled ones plus the EMPTY string, and that is
// load-bearing rather than thorough:
//
//   - two values cannot separate the rank from an ASCENDING alphabetical sort,
//     because over raw strings h < l and the rank also puts high before low.
//     Only medium disagrees, since h < l < m makes ascending high, low, medium.
//   - the empty string cannot separate the rank from a DESCENDING one either,
//     because descending puts "" last too. The never-dropped clause is a COUNT
//     assertion and no ordering assertion reaches it.
//
// So the order needs three values and the count needs the fourth.
var prioOrder = []struct{ id, priority string }{
	{"wi-high", "high"},
	{"wi-medium", "medium"},
	{"wi-low", "low"},
	{"wi-none", ""},
}

// prioItem writes an active work-item carrying a priority. Named apart from the
// helpers in card_test.go and sections_test.go on this package's own rule: one
// helper serving two tests is how an assertion ends up depending on a field a
// neighbouring test needed.
func prioItem(t *testing.T, s *Store, id, priority string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: prioProject, Body: "b",
		Fields: map[string]string{
			"title": id, "status": "active", "priority": priority,
		},
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing %s: %v", id, err)
	}
}

func prioNote(t *testing.T, s *Store, id, priority string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: KindNote, Project: prioProject, Body: "b",
		Fields:  map[string]string{"priority": priority},
		Session: "boris", Seat: "boris",
	}); err != nil {
		t.Fatalf("writing note %s: %v", id, err)
	}
	if err := s.Link(tctx, id, LinkPartOf, prioProject); err != nil {
		t.Fatalf("attaching %s: %v", id, err)
	}
}

func prioProjectRecord(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: prioProject, Kind: "project", Project: prioProject, Body: "b",
		Fields: map[string]string{"title": "the project"},
		// next_up_n is left at the default 5, so all four items land in
		// next_up and the ordering under test is the whole list rather than a
		// prefix of it. A truncated list can hide a misordered tail.
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing the project: %v", err)
	}
}

// ⛔ SECTION 39 BINDS THIS AND THE CODE DID NOT IMPLEMENT IT UNTIL NOW.
//
// plan/39: "a topological sort over the blocks graph among status: active
// work-items, unresolved dependencies excluded, TIES BROKEN BY priority."
// ItemState.Priority was filled and read by nothing, and next-up's ties were
// broken by id - which is deterministic, so no test noticed.
//
// There are no blocks edges here on purpose: with every item ready at once,
// the topological order is ENTIRELY the tie-break, so this test measures the
// one thing it is named for.
func TestNextUpBreaksItsTiesByPriority(t *testing.T) {
	s := openStore(t, estate(t, prioProject))
	prioProjectRecord(t, s)
	for _, p := range prioOrder {
		prioItem(t, s, p.id, p.priority)
	}

	b, err := s.Brief(tctx, prioProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	// ⛔ THE COUNT IS ASSERTED BEFORE THE ORDER, AND IT IS NOT A PRECONDITION
	// CHECK. "An unrecognised or empty priority is NEVER DROPPED" is the half
	// of the ruling that no ordering assertion can reach: a derivation that
	// silently discarded wi-none would produce a perfectly ordered list of
	// three and every order assertion below would still pass.
	if len(b.NextUp) != len(prioOrder) {
		t.Fatalf("next_up has %d items, want %d - an item with an unrecognised "+
			"or empty priority must be RANKED LAST, never dropped: %v",
			len(b.NextUp), len(prioOrder), ids(b.NextUp))
	}

	for i, want := range prioOrder {
		if got := b.NextUp[i].ID; got != want.id {
			t.Errorf("next_up[%d] = %s, want %s (priority %q)\n"+
				" got: %v\nwant: %v",
				i, got, want.id, want.priority, ids(b.NextUp), wantIDs())
			break
		}
	}
}

// Section 3's notes lead with the important ones, and it is the SAME rank.
//
// Section 39 binds no order for a project's notes - row 3 says only that a note
// is rendered in full. This ordering is the seat's choice, taken so that these
// notes and a case's attention_n notes eleven rows later cannot disagree.
//
// ⛔ THE COMMENT ON notesAbout CLAIMED THIS SORT BEFORE THE CODE DID IT. That
// is why this test exists in this shape: the caption named priority over a
// query that ordered by id, and nothing was watching.
func TestTheProjectsNotesLeadWithTheImportantOnes(t *testing.T) {
	s := openStore(t, estate(t, prioProject))
	prioProjectRecord(t, s)
	for _, p := range prioOrder {
		// "wi-high" -> "n-high". The same id ORDER as the work-items above, so
		// this test inherits the property that makes theirs meaningful: sorting
		// by id alone gives high, low, medium, none and cannot pass.
		prioNote(t, s, "n"+p.id[2:], p.priority)
	}

	b, err := s.Brief(tctx, prioProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	if len(b.Notes) != len(prioOrder) {
		var got []string
		for _, n := range b.Notes {
			got = append(got, n.ID+"/"+n.Priority)
		}
		t.Fatalf("section 3 has %d notes, want %d - a note whose priority is "+
			"unrecognised or empty must sort LAST, never vanish: %v",
			len(b.Notes), len(prioOrder), got)
	}

	for i, want := range prioOrder {
		if got := b.Notes[i].Priority; got != want.priority {
			var order []string
			for _, n := range b.Notes {
				order = append(order, n.Priority)
			}
			t.Errorf("notes[%d] has priority %q, want %q\n got: %v",
				i, got, want.priority, order)
			break
		}
	}
}

// ⛔ THE PROPERTY, NOT THE TABLE. Asserting priorityRank("high") == 0 would
// restate the vocabulary in a second place, which is the duplication the
// one-definition ruling exists to prevent - and it would have to be edited
// every time the table legitimately changes.
//
// What must hold whatever the vocabulary is: every known value ranks before
// every unknown one, and an unknown value gets a rank rather than being
// refused. A priorityRank returning -1 for the unknown case would sort it
// FIRST, which is the reassuring-lie shape section 39 names - the least
// important note leading the list a reader scans first.
func TestAnUnrecognisedPriorityRanksAfterEveryKnownOne(t *testing.T) {
	unknown := []string{"", "urgent", "P0", "HIGH", "medium "}

	for _, known := range priorities {
		for _, u := range unknown {
			if priorityRank(known) >= priorityRank(u) {
				t.Errorf("priorityRank(%q)=%d is not before priorityRank(%q)=%d",
					known, priorityRank(known), u, priorityRank(u))
			}
		}
	}

	// Case and whitespace are NOT normalised, deliberately: "HIGH" and
	// "medium " are unrecognised, and the ruling says an unrecognised value is
	// visible rather than silently repaired. A store holding "HIGH" has a data
	// problem the brief should show, not hide.
	for i, a := range priorities {
		for _, b := range priorities[i+1:] {
			if priorityRank(a) >= priorityRank(b) {
				t.Errorf("the vocabulary is out of order: %q ranks %d, %q ranks %d",
					a, priorityRank(a), b, priorityRank(b))
			}
		}
	}
}

func ids(items []ItemState) []string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, i.ID)
	}
	return out
}

func wantIDs() []string {
	out := make([]string, 0, len(prioOrder))
	for _, p := range prioOrder {
		out = append(out, p.id)
	}
	return out
}
