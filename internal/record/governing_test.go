package record

import "testing"

const govProject = "gov"

// govPut writes one record of the given kind into this file's project.
//
// Named apart from the other files' helpers deliberately. sections_test.go's
// sectItem hard-codes an active work-item because that is the shape its
// question needs; a shared helper grown to serve both is how an assertion ends
// up depending on a field a neighbouring test happened to set.
func govPut(t *testing.T, s *Store, id, kind, title string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: kind, Project: govProject, Body: "b",
		Fields:  map[string]string{"title": title},
		Session: "s", Seat: "team-lead",
	}); err != nil {
		t.Fatalf("writing %s (%s): %v", id, kind, err)
	}
}

// ⛔ THE ACCEPTANCE TEST FOR B64, AND THE NEGATIVE HALF IS THE HALF THAT WORKS.
//
// B64's condition is that a decision written through record.put comes back out
// of the brief WITHOUT the reader knowing its id. The obvious test - write a
// decision, assert it appears - is satisfied by a derivation that appends every
// record in the store, which would be a worse brief than the one B64 is fixing.
//
// ⛔ THIS IS GENERATION 11's MUTATION LESSON ARRIVING ONE LEVEL UP. Its field
// predicate survived a `=` -> `LIKE` mutation because every case asked for a
// field name that existed, so every assertion was satisfiable by the broken
// code. A renderer test whose cases all render successfully cannot catch a
// renderer that renders the wrong thing. So four of the seven assertions below
// are about what MUST NOT appear, and they are what make the other three mean
// anything.
func TestSectionTwelveCarriesTheGoverningKindsAndNothingElse(t *testing.T) {
	s := openStore(t, estate(t, govProject))
	govPut(t, s, govProject, KindProject, "the project")
	govPut(t, s, "d-1", KindDecision, "the retention ladder governs observability")
	govPut(t, s, "r-1", KindRequirement, "storage does not reset with each release")
	govPut(t, s, "a-1", KindArtefact, "the attack synthesis")
	// "work-item" is a literal here because rig no longer names the planner's
	// kinds: the constant for it left with the importers at plan/50 move 6, and
	// the declaring program owns the word now (decision 5). Spelling the
	// constant's name even in a comment fails acceptance B's grep, which is why
	// this line says it the long way round.
	govPut(t, s, "wi-1", "work-item", "a work item, which section 1 already carries")
	govPut(t, s, "f-1", KindFeature, "a feature, which section 10 already carries")
	govPut(t, s, "n-1", KindNote, "a note, which section 3 already carries")

	b, err := s.Brief(tctx, govProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	got := map[string]string{}
	for _, g := range b.Governing {
		got[g.ID] = g.Kind
	}

	// THE POSITIVE HALF: the three kinds B64 exists for.
	for _, c := range []struct{ id, kind string }{
		{"d-1", KindDecision},
		{"r-1", KindRequirement},
		{"a-1", KindArtefact},
	} {
		switch k, ok := got[c.id]; {
		case !ok:
			t.Errorf("%s (%s) is in the store and NOT in the brief - it is "+
				"reachable only by a caller who already knows its id, which is "+
				"the whole of B64", c.id, c.kind)
		case k != c.kind:
			t.Errorf("%s renders as kind %q, want %q - a row that cannot say "+
				"WHICH governing kind it is has folded three kinds into one, "+
				"which is the option B64 ruled against", c.id, k, c.kind)
		}
	}

	// ⛔ THE NEGATIVE HALF. Each of these already has its own section, and a
	// derivation that reaches them here is dumping the store rather than
	// answering section 12.
	for _, c := range []struct{ id, why string }{
		{"wi-1", "work-items are sections 1 and 2"},
		{"f-1", "features are section 10"},
		{"n-1", "notes are section 3"},
		{govProject, "the container is the brief's subject, not a row in it"},
	} {
		if k, ok := got[c.id]; ok {
			t.Errorf("section 12 carries %s as %q and it must not - %s. A "+
				"derivation that appends every record passes every positive "+
				"assertion in this test and fails here, which is why this half "+
				"exists", c.id, k, c.why)
		}
	}

	if n := len(b.Governing); n != 3 {
		t.Errorf("section 12 has %d rows, want exactly 3 - a count is the only "+
			"assertion that catches a row nobody thought to name", n)
	}
}

// The counts cover every governing record and the list covers all three kinds,
// so here they agree - but they are derived separately for the reason section
// 10 gives about its own pair, and a test that only ever sees them agree cannot
// tell that they are two answers rather than one.
func TestTheGoverningCountsAreDeterministicAndInARuledOrder(t *testing.T) {
	s := openStore(t, estate(t, govProject))
	govPut(t, s, govProject, KindProject, "the project")
	govPut(t, s, "r-1", KindRequirement, "one")
	govPut(t, s, "r-2", KindRequirement, "two")
	govPut(t, s, "d-1", KindDecision, "a ruling")

	b, err := s.Brief(tctx, govProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	// ⛔ A SLICE IN A FIXED ORDER, NOT A MAP, AND THE REASON IS SECTION 10's:
	// Go randomises map iteration, so a map would render differently on every
	// call and a golden test over the brief would flake.
	want := []KindCount{
		{Kind: KindDecision, Count: 1},
		{Kind: KindRequirement, Count: 2},
	}
	if len(b.GoverningCounts) != len(want) {
		t.Fatalf("counts are %v, want %v - a kind with no records gets no row, "+
			"because a zero is not the same claim as an absence",
			b.GoverningCounts, want)
	}
	for i, w := range want {
		if b.GoverningCounts[i] != w {
			t.Errorf("count %d is %v, want %v - the order is decision, "+
				"requirement, artefact: what was ruled, what is required, what "+
				"exists. Alphabetical reads as nothing", i, b.GoverningCounts[i], w)
		}
	}

	// The list is ordered by that same vocabulary and then by id, so two runs
	// over the same store render identically.
	var ids []string
	for _, g := range b.Governing {
		ids = append(ids, g.ID)
	}
	for i, w := range []string{"d-1", "r-1", "r-2"} {
		if i >= len(ids) || ids[i] != w {
			t.Fatalf("section 12 renders %v, want [d-1 r-1 r-2]", ids)
		}
	}
}

// ⛔ THE SECTION MUST DECLARE ITSELF, AND THIS IS THE ASSERTION THAT STOPS THE
// TWELFTH BEING ADDED HALF-WIRED.
//
// statuses() refuses any brief in which a section is neither derived nor given
// a reason, and Brief returns Brief{}, err on that refusal - so a half-wired
// twelfth does not degrade one section, it takes down EVERY project.brief,
// including the one the window polls. That property is why the twelfth section
// was chosen over folding these kinds into an existing row: a half-done fold is
// silent. This test asserts the section is present and computed, so the guard
// is exercised rather than merely relied upon.
func TestTheTwelfthSectionIsDeclaredAndComputedOnEveryBrief(t *testing.T) {
	s := openStore(t, estate(t, govProject))
	govPut(t, s, govProject, KindProject, "the project")

	b, err := s.Brief(tctx, govProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b.Sections) != len(briefSections) {
		t.Fatalf("the brief carries %d section states and briefSections has %d",
			len(b.Sections), len(briefSections))
	}

	var found bool
	for _, st := range b.Sections {
		if st.Section != SectionGoverning {
			continue
		}
		found = true
		if st.State != SectionComputed {
			t.Errorf("section 12 is %q (%q), want %q - it derives from records "+
				"the store already holds, so it waits on nothing and an empty "+
				"list means there are genuinely no decisions",
				st.State, st.Reason, SectionComputed)
		}
	}
	if !found {
		t.Fatal("section 12 is absent from the brief's own section list, which " +
			"is the failure section 39 rules against: absent hides the gap")
	}

	// An empty answer from a COMPUTED section is a fact, not a gap. A project
	// with no decisions must report zero rows and still say it looked.
	if len(b.Governing) != 0 || len(b.GoverningCounts) != 0 {
		t.Errorf("a project with no governing records rendered %d rows and %d "+
			"counts", len(b.Governing), len(b.GoverningCounts))
	}
}
