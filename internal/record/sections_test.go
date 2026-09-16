package record

import "testing"

const sectProject = "sect"

// sectItem writes an active work-item in this file's project. Named apart from
// brief_test.go's own helper, which builds a different shape for a different
// question - one helper serving two tests is how an assertion ends up depending
// on a field a neighbouring test needed.
func sectItem(t *testing.T, s *Store, id string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: sectProject, Body: "b",
		Fields:  map[string]string{"title": id, "status": "active"},
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing %s: %v", id, err)
	}
}

func note(t *testing.T, s *Store, id, body, priority, about string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: KindNote, Project: sectProject, Body: body,
		Fields:  map[string]string{"priority": priority},
		Session: "boris", Seat: "boris",
	}); err != nil {
		t.Fatalf("writing note %s: %v", id, err)
	}
	if err := s.Link(tctx, id, LinkPartOf, about); err != nil {
		t.Fatalf("attaching %s to %s: %v", id, about, err)
	}
}

func project(t *testing.T, s *Store, nextUpN string) {
	t.Helper()
	f := map[string]string{"title": "the project"}
	if nextUpN != "" {
		f["next_up_n"] = nextUpN
	}
	if _, err := s.Put(tctx, PutRequest{
		ID: sectProject, Kind: "project", Project: sectProject, Body: "b",
		Fields: f, Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing the project: %v", err)
	}
}

// ⛔ SECTION 3 READS THE OPEN LIST AND NOT NEXT-UP, AND THAT IS A CONTRADICTION
// SECTION 39 ALREADY PAID TO RESOLVE.
//
// Rows 2 and 3 gave the same item two incompatible renderings: a next-up item's
// notes as a flag, an open item's in full. The lists were made disjoint so every
// item has exactly one rendering. A derivation collecting notes for BOTH lists
// puts that contradiction straight back, and nothing else here would notice -
// the notes would simply appear, which looks like more information rather than
// like a defect.
//
// next_up_n is 1, so of two items exactly one is next-up and one is open, and
// the two notes are distinguishable by which list their subject landed in.
func TestSectionThreeCarriesNotesOnOpenItemsAndTheProjectButNotOnNextUp(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "1")
	sectItem(t, s, "wi-1")
	sectItem(t, s, "wi-2")
	note(t, s, "n-project", "a question about the whole project", "high", sectProject)
	note(t, s, "n-1", "about the first item", "low", "wi-1")
	note(t, s, "n-2", "about the second item", "low", "wi-2")

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b.NextUp) != 1 || len(b.Open) != 1 {
		t.Fatalf("expected one next-up and one open, got %d and %d", len(b.NextUp), len(b.Open))
	}
	nextUp, open := b.NextUp[0].ID, b.Open[0].ID

	got := map[string]string{}
	for _, n := range b.Notes {
		got[n.About] = n.Body
	}
	if _, ok := got[sectProject]; !ok {
		t.Errorf("the note part-of the project itself is missing from section 3")
	}
	if _, ok := got[open]; !ok {
		t.Errorf("the note on the OPEN item %s is missing from section 3", open)
	}
	if body, ok := got[nextUp]; ok {
		t.Errorf("section 3 carries a note on the NEXT-UP item %s (%q): rows 2 and 3 "+
			"are giving one item two renderings again", nextUp, body)
	}
	if len(b.Notes) != 2 {
		t.Errorf("section 3 has %d notes, want 2", len(b.Notes))
	}
}

// A note is RENDERED IN FULL and carries its provenance, because section 39
// says "never summarised" and because a question with no author is a question
// nobody can answer.
func TestSectionThreeRendersTheNoteInFullWithItsProvenance(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")
	body := "is this still needed, or did the wire change make it moot? " +
		"I could not tell from the backlog row and I did not want to guess."
	note(t, s, "n-1", body, "high", sectProject)

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b.Notes) != 1 {
		t.Fatalf("expected one note, got %d", len(b.Notes))
	}
	n := b.Notes[0]
	if n.Body != body {
		t.Errorf("the note body was not carried whole:\n got %q\nwant %q", n.Body, body)
	}
	if n.Priority != "high" {
		t.Errorf("Priority = %q, want %q - it is the ordering signal a case sorts on", n.Priority, "high")
	}
	if n.Prov.Seat != "boris" || n.Prov.Session != "boris" {
		t.Errorf("provenance = %+v, want the seat and session that wrote it", n.Prov)
	}
	if n.Prov.CreatedAt.IsZero() {
		t.Error("the note has no created_at: section 11 sorts on it")
	}
}

// ⛔ THE COUNTS COVER EVERY STAGE AND THE LIST COVERS ONE, so the counts cannot
// be derived from the list. A project with three shipped features and none
// building would otherwise report nothing at all.
func TestSectionTenListsWhatIsBuildingAndCountsEveryStage(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")
	for _, f := range []struct{ id, title, stage string }{
		{"f-1", "the door", StageBuilding},
		{"f-2", "the record", StageBuilding},
		{"f-3", "presence", StageShipped},
		{"f-4", "the window", StagePlanned},
		{"f-5", "the old bridge", StageDeprecated},
		{"f-6", "a stage nobody ruled", "abandoned"},
	} {
		if _, err := s.Put(tctx, PutRequest{
			ID: f.id, Kind: KindFeature, Project: sectProject, Body: "b",
			Fields:  map[string]string{"title": f.title, "stage": f.stage},
			Session: "s", Seat: "backend-record",
		}); err != nil {
			t.Fatalf("writing %s: %v", f.id, err)
		}
	}

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	if len(b.Features) != 2 {
		t.Fatalf("Features has %d, want the 2 at stage building: %+v", len(b.Features), b.Features)
	}
	for _, f := range b.Features {
		if f.Stage != StageBuilding {
			t.Errorf("%s is in the building list at stage %q", f.ID, f.Stage)
		}
		if f.Title == "" {
			t.Errorf("%s has no title: the list is meant to be read", f.ID)
		}
	}

	// ⛔ THE ORDER IS THE LIFECYCLE, NOT THE ALPHABET, and the unrecognised
	// stage sorts after the known ones rather than being dropped. A value
	// outside section 39's four has to be visible in the one place somebody is
	// looking, whether it is a typo or a fifth stage nobody has ruled yet.
	want := []StageCount{
		{StagePlanned, 1},
		{StageBuilding, 2},
		{StageShipped, 1},
		{StageDeprecated, 1},
		{"abandoned", 1},
	}
	if len(b.Stages) != len(want) {
		t.Fatalf("Stages = %+v, want %+v", b.Stages, want)
	}
	for i := range want {
		if b.Stages[i] != want[i] {
			t.Errorf("Stages[%d] = %+v, want %+v (the whole list was %+v)",
				i, b.Stages[i], want[i], b.Stages)
		}
	}
}

// A project with no features answers with empty sections rather than failing,
// and a project with features at no stage at all still counts them.
func TestSectionTenOnAProjectWithNothingToSay(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")
	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b.Features) != 0 || len(b.Stages) != 0 {
		t.Errorf("a project with no features reported %+v / %+v", b.Features, b.Stages)
	}
	if len(b.Notes) != 0 {
		t.Errorf("a project with no notes reported %+v", b.Notes)
	}
}
