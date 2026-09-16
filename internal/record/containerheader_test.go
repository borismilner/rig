package record

import "testing"

const hdrProject = "hdr"

// ⛔ THE BRIEF READ ITS CONTAINER AND KEPT ONLY THE KIND, AND THE STORE HELD THE
// REST THE WHOLE TIME.
//
// Measured 2026-09-17 against the seeded production store: `rig record get rig`
// returned a project record with title, description_short and status populated,
// and `rig brief rig` printed "(not said) (no status)" over "(no title)" with
// every row underneath it correct. The record was there; the derivation never
// looked past container.Kind.
//
// ⛔ IT WAS TWO DEFECTS AND NEITHER HALF EXPLAINED THE SCREEN ALONE. The daemon
// carried none of the four header fields, including the Kind this package DID
// compute - fixed at rig af7715d - and this package modelled only that one. A
// seat repairing its own half would have shipped and watched nothing change.
// This is the package half.
func TestTheBriefCarriesItsContainersOwnTitleStatusAndSemver(t *testing.T) {
	s := openStore(t, estate(t, hdrProject))
	if _, err := s.Put(tctx, PutRequest{
		ID: hdrProject, Kind: KindProject, Project: hdrProject, Body: "b",
		Fields: map[string]string{
			"title":  "rig",
			"status": "active",
			"semver": "0.4.1",
		},
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing the project: %v", err)
	}

	b, err := s.Brief(tctx, hdrProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	for _, c := range []struct{ field, got, want string }{
		{"Kind", b.Kind, KindProject},
		{"Title", b.Title, "rig"},
		{"Status", b.Status, "active"},
		{"Semver", b.Semver, "0.4.1"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q - the record carries it and the brief "+
				"dropped it, which renders as a header that knows nothing over "+
				"rows that are all correct", c.field, c.got, c.want)
		}
	}
}

// ⛔ EMPTY AND ABSENT STAY INDISTINGUISHABLE, AND THAT IS A DECISION RATHER THAN
// AN OVERSIGHT.
//
// A container with no record and a container whose record has no title both
// produce an empty Title, so a reader cannot tell them apart. The argument for
// keeping it that way is `cli`'s, taken 2026-09-17 across the seam: "a container
// with no title is a fact about the project; a title that was never read is a
// DEFECT IN THE PIPELINE. Giving the defect its own rendering teaches every
// reader that there are two normal kinds of blank, and normalises it."
//
// The defect is caught by a descriptor-coverage guard over the wire in cmd/rig,
// which goes red when a field nobody renders arrives - not by a second flavour
// of blank on a human's screen.
func TestAContainerWithNoRecordHasAnEmptyHeaderRatherThanAWrongOne(t *testing.T) {
	s := openStore(t, estate(t, "hdrghost"))

	b, err := s.Brief(tctx, "hdrghost")
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	for _, c := range []struct{ field, got string }{
		{"Kind", b.Kind},
		{"Title", b.Title},
		{"Status", b.Status},
		{"Semver", b.Semver},
	} {
		if c.got != "" {
			t.Errorf("%s = %q on a container with no record, want empty - "+
				"there is nothing to read it from", c.field, c.got)
		}
	}
}

// ⛔ A CASE HAS NO SEMVER BECAUSE NOBODY WRITES ONE, NOT BECAUSE THE DERIVATION
// REFUSES TO REPORT IT.
//
// Section 39 is explicit that a case has no `semver` - "a case does not ship, so
// it has no version to advance" - and the wire says field 12 is "empty on a
// case". Both are satisfied by reading the field: a case record has no semver
// field, so the answer is empty without anything enforcing it.
//
// ⛔ THE ALTERNATIVE WAS SUPPRESSING IT BY KIND AND IT WAS REFUSED, recorded so
// it is not re-walked. Suppression is a narrowing no lead ruled, and it would
// make the brief lie about a record that genuinely carried the field. This
// package's own B21 finding is the precedent: reporting what the document says
// beats inventing what it meant. The reopen condition is named - if a case ever
// legitimately carries a semver and the brief must hide it, that is a ruling.
func TestACasesSemverIsEmptyBecauseNothingWroteOneNotBecauseItIsSuppressed(t *testing.T) {
	s := openStore(t, estate(t, "hdrcase"))
	if _, err := s.Put(tctx, PutRequest{
		ID: "hdrcase", Kind: KindCase, Project: "hdrcase", Body: "b",
		Fields:  map[string]string{"title": "a case", "status": "open"},
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing the case: %v", err)
	}

	b, err := s.Brief(tctx, "hdrcase")
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if b.Kind != KindCase {
		t.Fatalf("Kind = %q, want %q", b.Kind, KindCase)
	}
	if b.Title != "a case" {
		t.Errorf("Title = %q, want %q - a case has a title like any container", b.Title, "a case")
	}
	if b.Status != "open" {
		t.Errorf("Status = %q, want %q - a case's status is open/resolved", b.Status, "open")
	}
	if b.Semver != "" {
		t.Errorf("Semver = %q, want empty - nothing wrote one", b.Semver)
	}
}
