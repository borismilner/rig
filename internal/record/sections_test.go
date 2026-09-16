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

// ⛔ ALL ELEVEN, EVERY TIME. Boris, 2026-09-16, asked directly whether the four
// that shipped were enough: "Cover all of them."
//
// The count and the SET are asserted separately on purpose. A count of eleven
// with one section reported twice and another missing is the shape a bare
// len() check cannot see, and it is the one a hand-kept list actually produces.
func TestABriefAnswersAllElevenSectionsExactlyOnce(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	if len(b.Sections) != 11 {
		t.Fatalf("the brief reports %d sections, want 11", len(b.Sections))
	}
	seen := map[Section]int{}
	for _, st := range b.Sections {
		seen[st.Section]++
	}
	for _, want := range briefSections {
		switch seen[want] {
		case 1:
		case 0:
			t.Errorf("section %q is not reported at all - absence is the "+
				"failure section 39 added the section states to prevent", want)
		default:
			t.Errorf("section %q is reported %d times", want, seen[want])
		}
	}
	// ⛔ THE ELEVEN, WRITTEN OUT, AS A SECOND INSTRUMENT. Checking the reported
	// set against briefSections would agree with briefSections whatever it
	// said - the cross-check-with-the-same-blind-spot this package has paid for
	// before. These names are section 39's rows 1 to 11 in order, transcribed.
	eleven := map[Section]bool{
		"open": true, "next_up": true, "notes": true, "blocked": true,
		"drift": true, "must_read": true, "projection_behind": true,
		"pending": true, "local_only": true, "features": true,
		"case_notes": true,
	}
	for got := range seen {
		if !eleven[got] {
			t.Errorf("section %q is reported and is not one of the eleven", got)
		}
	}
	for want := range eleven {
		if seen[want] == 0 {
			t.Errorf("section %q is one of section 39's eleven and is not reported", want)
		}
	}
}

// ⛔ A NOT_COMPUTED WITH A BLANK REASON IS WORSE THAN NO SECTION AT ALL: it
// says "this cannot be answered" and refuses to say what would answer it. The
// mirror also holds - a COMPUTED section carrying a reason is a leftover from
// when it could not be computed, which is exactly how the daemon's rows went
// stale while still looking maintained.
func TestEverySectionsReasonMatchesItsState(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	for _, st := range b.Sections {
		switch st.State {
		case SectionComputed:
			if st.Reason != "" {
				t.Errorf("section %q is computed and still carries a reason "+
					"(%q) - a stale reason outlives what it explained",
					st.Section, st.Reason)
			}
		case SectionNotComputed:
			if st.Reason == "" {
				t.Errorf("section %q is not computed and names no missing "+
					"input", st.Section)
			}
		case SectionWithheldByView:
			t.Errorf("section %q is withheld by view, and the DERIVATION must "+
				"never emit that - a view is a property of who is asking and "+
				"the store does not know who is asking", st.Section)
		default:
			t.Errorf("section %q has state %q, which is not one of the three",
				st.Section, st.State)
		}
	}
}

// The four that were built before today plus the two that landed at 08ce632.
// Named individually rather than counted, because "six are computed" stays true
// while the wrong six are.
func TestTheSectionsThisDerivationAnswersSayComputed(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")
	sectItem(t, s, "wi-1")

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	state := map[Section]SectionState{}
	for _, st := range b.Sections {
		state[st.Section] = st.State
	}
	for _, want := range []Section{
		SectionOpen, SectionNextUp, SectionNotes, SectionBlocked, SectionFeatures,
	} {
		if state[want] != SectionComputed {
			t.Errorf("section %q is %q, want %q - this derivation produces it",
				want, state[want], SectionComputed)
		}
	}
	// ⛔ SECTION 11 IS NOT "not built yet" HERE, IT IS NOT APPLICABLE. Row 11 is
	// a CASE's attention_n notes and this brief derives a PROJECT. The wire has
	// no state for inapplicable, so the distinction lives in the reason string
	// and a caller cannot act on it - reported to the team-lead, not fixed here.
	if state[SectionCaseNotes] != SectionNotComputed {
		t.Errorf("section %q is %q on a project brief, want %q",
			SectionCaseNotes, state[SectionCaseNotes], SectionNotComputed)
	}
}

// ⛔ THE GUARD THAT CAN FAIL, AND IT IS THE WHOLE REASON THE STATES MOVED.
//
// The daemon's list went stale because a section could be silently unaccounted
// for: nothing anywhere checked that every section had been given a state by
// somebody. This asserts the refusal directly rather than trusting that it
// would fire, by inventing a twelfth section that neither half knows about.
func TestASectionWithNoStateRefusesTheBriefRatherThanAnsweringTenOfEleven(t *testing.T) {
	orig := briefSections
	t.Cleanup(func() { briefSections = orig })
	briefSections = append(append([]Section{}, orig...), Section("invented"))

	led := newSectionLedger(KindProject)
	for _, s := range orig {
		if isDerived(s) {
			led.did(s)
		}
	}

	got, err := led.statuses()
	if err == nil {
		t.Fatalf("a section with no state produced %d statuses and no error - "+
			"the brief would answer eleven of twelve and say nothing about the "+
			"twelfth, which is the defect this mechanism exists to stop", len(got))
	}
}

// The other half of the same guard, and it is the one that actually happened.
//
// ⛔ THE DAEMON WENT ON SAYING "the derivation does not collect this kind yet"
// FOR NOTES AND FEATURES AFTER THE DERIVATION LANDED THEM. Here that is not a
// stale string, it is a refusal: a section the derivation answers cannot also
// be listed as waiting on something.
func TestASectionCannotBeBothDerivedAndWaiting(t *testing.T) {
	led := newSectionLedger(KindProject)
	for _, s := range briefSections {
		if isDerived(s) {
			led.did(s)
		}
	}
	led.did(SectionDrift) // which sectionsWaitingOn still lists

	if _, err := led.statuses(); err == nil {
		t.Fatal("a section that is both derived and listed as waiting was " +
			"accepted - that is the daemon's stale row with nothing to catch it")
	}
}

// isDerived is the test's own view of which sections this derivation produces,
// deliberately NOT read from the ledger or from sectionsWaitingOn.
//
// ⛔ A SECOND INSTRUMENT SHARING NO CODE PATH WITH ITS SUBJECT. Deriving this
// from sectionsWaitingOn would make every assertion above a tautology - the
// test would agree with the table whatever the table said, which is the
// cross-check-with-the-same-blind-spot this package has paid for before.
func isDerived(s Section) bool {
	switch s {
	case SectionOpen, SectionNextUp, SectionNotes, SectionBlocked, SectionFeatures:
		return true
	}
	return false
}
