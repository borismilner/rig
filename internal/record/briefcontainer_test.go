package record

import "testing"

// B76, THE DERIVATION'S HALF.
//
// ⛔ THE BRIEF ANSWERS ELEVEN SECTIONS AND EACH ONE DISTINGUISHES "NOTHING TO
// REPORT" FROM "THIS BUILD CANNOT ANSWER". The CONTAINER made no such
// distinction: briefContainer swallowed NotFound, returned a zero Record, and
// the derivation carried on and answered every section about an id the store
// holds nothing for.
//
// The condition was always recoverable - Kind stayed empty - and recovering it
// required knowing that rule, which is the "reachable only if you already
// know" failure section 12 exists to end. ContainerFound states it.

// briefItemIn writes an active work item into an arbitrary project, so a test
// can build the shape that makes the naive fix wrong: records under an id that
// has no container record of its own.
func briefItemIn(t *testing.T, s *Store, project, id string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: project, Body: id,
		Fields:  map[string]string{"title": id, "status": "active"},
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing %s in %s: %v", id, project, err)
	}
}

func TestABriefForAnIdWithNoRecordSaysSoInsteadOfAnsweringAsAnEmptyProject(t *testing.T) {
	s := openStore(t, estate(t, "development"))

	b, err := s.Brief(tctx, "zzz-no-such-project-42")
	if err != nil {
		t.Fatalf("a missing container is not an error - the brief answers for "+
			"the store as it is: %v", err)
	}
	if b.ContainerFound {
		t.Error("the derivation reports a container it never read")
	}
	if !b.ContainerMissing() {
		t.Error("ContainerMissing disagrees with ContainerFound, so the two " +
			"statements of one fact have already drifted")
	}
	// ⛔ AND IT IS STILL A BRIEF. Refusing here was considered and refused:
	// brief.go's own ruling is that a missing container is not an error.
	if len(b.Sections) == 0 {
		t.Error("the section ledger is empty, so a caller cannot tell which " +
			"sections were computed about the missing container")
	}
}

// THE CONTROL, AND IT IS THE PAIR THE WHOLE DEFECT TURNS ON. A project that
// exists and has no work must report ContainerFound. Without this, a "fix"
// keyed on emptiness passes the test above and reports every quiet project as
// a typo.
func TestARealProjectWithNoWorkInItIsFoundAndNotReportedAsMissing(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	project(t, s, "")

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatal(err)
	}
	if !b.ContainerFound {
		t.Fatal("a project record was written and the brief says the " +
			"container is missing")
	}
	if b.ContainerMissing() {
		t.Error("ContainerMissing is true for a container that exists")
	}
	// The emptiness that a naive predicate would have keyed on, asserted so
	// the control is provably the hard case rather than an easy one.
	if len(b.NextUp) != 0 || len(b.Open) != 0 || len(b.Notes) != 0 {
		t.Fatalf("this control is meant to be an EMPTY project and is not: "+
			"next=%d open=%d notes=%d", len(b.NextUp), len(b.Open), len(b.Notes))
	}
}

// ⛔ THE MIRROR CASE, AND IT IS WHY THE DERIVATION MUST NOT REFUSE. A record
// carries its own `project` field, so work can be written under an id that was
// never created as a container. `a0-survey` in the live production store is
// exactly this shape, measured 2026-09-17: one record, no container.
//
// A brief that emptied itself here would hide real records behind a naming
// defect - one wrong answer traded for another.
func TestAMissingContainerStillCarriesTheWorkRecordedUnderItsName(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	briefItemIn(t, s, "ghost", "g1")

	b, err := s.Brief(tctx, "ghost")
	if err != nil {
		t.Fatal(err)
	}
	if b.ContainerFound {
		t.Error("no container record was ever written for `ghost`")
	}
	if len(b.NextUp) != 1 || b.NextUp[0].ID != "g1" {
		t.Fatalf("the work under a container-less id was dropped: %+v", b.NextUp)
	}
}

// ⛔ THE KIND AND THE FLAG ARE ONE FACT AND MUST NOT DRIFT APART, AND WHAT
// THIS GUARDS HAS NARROWED RATHER THAN GONE.
//
// It used to guard the CLI's whole predicate: Kind carried this condition
// alone, the wire had no field for it, and cmd/rig re-derived the missing
// container from an empty kind. `ProjectBriefResponse.container_found`
// (Tristate, field 22) ended that, so a matched pair now exchanges the fact.
//
// ⛔ IT IS NOT OBSOLETE, BECAUSE THE INFERENCE IS STILL REACHABLE. cmd/rig
// falls back to `Kind == ""` on TRISTATE_UNSPECIFIED - a daemon that predates
// field 22 - and that fallback is only correct while these two agree here.
// The day they disagree, every older daemon starts answering B76 wrongly and
// nothing else in the tree would say so.
func TestAnEmptyKindAndAMissingContainerAreTheSameAnswer(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	project(t, s, "")

	for _, id := range []string{sectProject, "zzz-no-such-project-42"} {
		b, err := s.Brief(tctx, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if (b.Kind == "") != b.ContainerMissing() {
			t.Errorf("%s: kind=%q but ContainerMissing=%v - cmd/rig falls "+
				"back to an empty kind against a daemon with no "+
				"container_found field, and that fallback is now wrong",
				id, b.Kind, b.ContainerMissing())
		}
	}
}
