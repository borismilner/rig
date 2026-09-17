// The decisions half, tested where it can be wrong.
//
// ⛔ THE FIRST TEST IN THIS FILE IS THE NEGATIVE CONTROL AND IT IS NOT
// CEREMONY. Every assertion below says "the plan holds X because the document
// states X", and that sentence is worthless unless a plan built from a document
// that states nothing holds nothing. A harness that would pass either way
// reports a meaningless result, which this repository has now recorded ten
// times under a different name.
package main

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// sectionNine is section 39's ten kinds, which is the closed set a record may
// carry. It is written out here rather than reached for, because the point of
// the assertion is that `kind` refuses nothing and something has to.
var sectionNine = map[string]bool{
	record.KindProject: true, record.KindCase: true, record.KindWorkItem: true,
	record.KindFeature: true, record.KindDecision: true, record.KindRequirement: true,
	record.KindArtefact: true, record.KindNote: true, record.KindStandard: true,
	record.KindProgress: true,
}

// entryFor is one TOP-LEVEL parsed entry, spelled once.
//
// The level is fixed at 2 rather than taken as an argument: every sub-heading
// in this file is written out in full because it also needs a PartOf and a
// distinct body, and a parameter only ever passed one value is a parameter that
// reads as a choice nobody makes.
func entryFor(key, title string, kind record.EntryKind) record.DecisionEntry {
	return record.DecisionEntry{Key: key, Title: title, Kind: kind, Level: 2, Line: 1, Body: title}
}

// ⛔ THE NEGATIVE CONTROL. A document that states nothing must plan nothing.
func TestADocumentThatStatesNoDecisionPlansNoDecision(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B1", Title: "a row"}},
	}, record.DecisionParse{})

	for _, in := range p.want {
		if in.grain == grainEntry {
			t.Fatalf("%s was planned at the entry grain from an empty decisions "+
				"parse, so every other test in this file is measuring the seeder "+
				"rather than the document", in.id)
		}
		if in.kind != record.KindWorkItem {
			t.Errorf("%s is kind %q and the backlog states only work items", in.id, in.kind)
		}
	}
	if got := strings.Join(p.collided, " "); got != "" {
		t.Errorf("collided = {%s}, want {}", got)
	}
}

// ⛔ A STANDING SECTION IS A `note`, AND NOTHING HERE MAY MINT A KIND.
//
// `kind` is not a closed set and `Put` refuses no value, so an invented
// `section` kind would be written silently and be reachable only by a caller
// who already knew to ask for it. The lead ruled `note`, and the second half of
// this test is the guard that matters: every kind this seeder writes is one of
// section 39's ten.
func TestAStandingSectionIsANoteAndEveryKindIsOneOfTheTen(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("what-rig-is", "What rig is", record.EntrySection),
			entryFor("2026-09-17-a-ruling", "2026-09-17 a ruling", record.EntryDecision),
		}})

	if got := intentFor(t, p, "what-rig-is").kind; got != record.KindNote {
		t.Errorf("a standing section was written as kind %q, want %q", got, record.KindNote)
	}
	if got := intentFor(t, p, "2026-09-17-a-ruling").kind; got != record.KindDecision {
		t.Errorf("a ruling was written as kind %q, want %q", got, record.KindDecision)
	}
	for _, in := range p.want {
		if !sectionNine[in.kind] {
			t.Errorf("%s would be written at kind %q, which is not one of section 39's "+
				"ten - and the store would accept it without a word", in.id, in.kind)
		}
	}
}

// ⛔ THE DOCUMENT'S COORDINATES TRAVEL WITH THE RECORD, AND A DATE IS NEVER
// INVENTED.
//
// The parser refuses to inherit a date from the heading above because a
// fabricated date reads exactly like a measured one. An EMPTY `doc-date` field
// is the same claim in a thinner disguise, so the field is absent instead.
func TestAnEntryCarriesItsCoordinatesAndAnUndatedOneCarriesNoDate(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	dated := record.DecisionEntry{
		Key: "2026-09-17-dated", Title: "2026-09-17 dated", Date: "2026-09-17",
		Kind: record.EntryDecision, Level: 2, Line: 412, Body: "the ruling",
	}
	undated := record.DecisionEntry{
		Key: "undated", Title: "undated", Kind: record.EntryDecision, Level: 2, Line: 9, Body: "prose",
	}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{dated, undated}})

	in := intentFor(t, p, "2026-09-17-dated")
	for name, want := range map[string]string{
		fieldDocKey:  "2026-09-17-dated",
		fieldDocLine: "412",
		fieldDocDate: "2026-09-17",
		fieldLevel:   "2",
		"title":      "2026-09-17 dated",
		"source":     "DECISIONS.md",
	} {
		if got := in.fields[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	// ⛔ THE BODY IS THE RULING AND IS NOT THE TITLE. On a backlog row the two
	// are the same, which is exactly why this would have gone unnoticed.
	if in.body != "the ruling" {
		t.Errorf("body = %q, want the entry's prose", in.body)
	}

	if _, held := intentFor(t, p, "undated").fields[fieldDocDate]; held {
		t.Errorf("an entry the document gives no date carries a %s field anyway, "+
			"which states a blank date as though the document had written one", fieldDocDate)
	}
	// The control: the field is there when the document states one, so its
	// absence above is a decision rather than a field nobody writes.
	if _, held := in.fields[fieldDocDate]; !held {
		t.Errorf("a dated entry carries no %s field, so the assertion above proves nothing", fieldDocDate)
	}
}

// ⛔ A SUB-HEADING'S part-of IS EMITTED, AND ITS PARENT IS ALWAYS AN ENTRY.
//
// The parser carries sections precisely so a child's parent exists, which is
// the thing B66 found missing on the backlog side: six children there named a
// parent that was in no record. An edge whose parent is missing is not written
// at all - the store refuses it and the run aborts half-done.
func TestASubHeadingsParentIsAnEntryAndTheEdgeIsEmitted(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("what-rig-is", "What rig is", record.EntrySection),
			{
				Key: "what-rig-is/the-ruling", Title: "The ruling", Kind: record.EntryDecision,
				Level: 3, PartOf: "what-rig-is", Line: 12, Body: "prose",
			},
		}})

	if got := intentFor(t, p, "what-rig-is/the-ruling").partOf; got != "what-rig-is" {
		t.Errorf("part-of = %q, want what-rig-is", got)
	}
	if got := strings.Join(p.orphaned, " "); got != "" {
		t.Errorf("orphaned = {%s}, want {} - the parent is an entry and the edge stands", got)
	}
}

// ⛔ AN EDITED RULING IS STALE, AND IT WAS INVISIBLE UNTIL THE BODY WAS
// COMPARED.
//
// `staleFields` looked only at the typed fields. On a work item the body IS the
// title, so the hole cost nothing and nobody could see it; on a decision the
// body is the whole ruling, so a rewritten one would have read as clean for
// ever. Measured 2026-09-17 while extending this seeder to the second document.
func TestARewrittenRulingIsStaleBecauseTheBodyIsCompared(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("a-ruling", "a ruling", record.EntryDecision),
		}})

	stored := heldOf(intentFor(t, p, "a-ruling"))
	stored.body = "what it used to say"

	d := diff(p, map[string]held{
		"B1":       heldOf(intentFor(t, p, "B1")),
		"a-ruling": stored,
	})
	if len(d.missing) != 0 || len(d.extra) != 0 {
		t.Fatalf("membership must agree for this test to mean anything: missing=%v extra=%v",
			d.missing, d.extra)
	}
	if got := strings.Join(d.stale, " "); got != "a-ruling(body)" {
		t.Errorf("stale = {%s}, want {a-ruling(body)}", got)
	}
}

// ⛔ AN ID HELD AT THE WRONG KIND IS PRESENT, SO MEMBERSHIP SAYS NOTHING.
//
// `missing` and `extra` are both empty when a record exists under the right id
// at the wrong kind, and every field can agree. That is the equal-size
// divergence one axis over, and the only thing that catches it is comparing the
// kind.
func TestARecordHeldAtTheWrongKindIsStaleRatherThanClean(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("what-rig-is", "What rig is", record.EntrySection),
		}})

	stored := heldOf(intentFor(t, p, "what-rig-is"))
	stored.kind = record.KindDecision

	d := diff(p, map[string]held{
		"B1":          heldOf(intentFor(t, p, "B1")),
		"what-rig-is": stored,
	})
	if len(d.missing) != 0 || len(d.extra) != 0 {
		t.Fatalf("membership must agree for this test to mean anything: missing=%v extra=%v",
			d.missing, d.extra)
	}
	if got := strings.Join(d.stale, " "); got != "what-rig-is(kind)" {
		t.Errorf("stale = {%s}, want {what-rig-is(kind)}", got)
	}
}

// ⛔ THE CHECK GOES BLIND IF IT ONLY LOOKS AT THE BACKLOG.
//
// A ruling in no record must land in `missing`, must be named by nonEmpty so
// the exit line says WHICH set moved, and must be printed. A detector that
// covered one of two documents would print a page of empty sets over a store
// missing several hundred rulings.
func TestARulingInNoRecordIsMissingAndTheCheckRefusesToExitClean(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("a-ruling", "a ruling", record.EntryDecision),
		}})

	d := diff(p, map[string]held{"B1": heldOf(intentFor(t, p, "B1"))})
	if got := strings.Join(d.missing, " "); got != "a-ruling" {
		t.Errorf("missing = {%s}, want {a-ruling}", got)
	}
	if !has(d.nonEmpty(), "missing") {
		t.Errorf("nonEmpty = %v; a ruling in no record must not exit clean", d.nonEmpty())
	}

	var b bytes.Buffer
	d.report(&b, o, p, "production")
	got := b.String()
	if !strings.Contains(got, "a-ruling") {
		t.Errorf("the report never names the missing ruling:\n%s", got)
	}
	// ⛔ AND THE HEADER MUST NAME BOTH DOCUMENTS, or a reader cannot tell a
	// clean run from one that never opened the second file.
	if !strings.Contains(got, "BACKLOG.md + DECISIONS.md") {
		t.Errorf("the check's header does not name both documents:\n%s", got)
	}
}

// ⛔ THE KINDS QUERIED COME FROM THE PLAN, OR A WHOLE KIND IS NEVER COMPARED.
func TestTheStoreIsReadAtEveryKindThePlanWrites(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("what-rig-is", "What rig is", record.EntrySection),
			entryFor("a-ruling", "a ruling", record.EntryDecision),
		}})

	if got := strings.Join(p.kinds(), " "); got != "decision note work-item" {
		t.Errorf("kinds = {%s}, want {decision note work-item}", got)
	}
}

// ⛔ A SECOND RUN MUST WRITE NOTHING, WHICH AT THE SET GRAIN MEANS A STORE
// BUILT FROM THE PLAN DIVERGES FROM IT IN NO WAY AT ALL.
//
// The live proof is a second `--check` against the daemon; this is the half
// that runs without one, and it is the half that catches a field whose value
// this seeder computes differently on two passes - a map iterated into a
// string, a timestamp, anything not derived from the document.
func TestAStoreBuiltFromThePlanDivergesInNoSet(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}},
		record.DecisionParse{Decisions: []record.DecisionEntry{
			entryFor("what-rig-is", "What rig is", record.EntrySection),
			{
				Key: "what-rig-is/the-ruling", Title: "The ruling", Kind: record.EntryDecision,
				Level: 3, PartOf: "what-rig-is", Line: 12, Body: "prose",
			},
		}})

	store := map[string]held{}
	for _, in := range p.want {
		store[in.id] = heldOf(in)
	}
	d := diff(p, store)
	for _, s := range []struct {
		name string
		set  []string
	}{
		{"missing", d.missing}, {"extra", d.extra}, {"stale", d.stale},
	} {
		if len(s.set) != 0 {
			t.Errorf("%s = {%s}, want {} - a second run over the same document must "+
				"ask for nothing", s.name, strings.Join(s.set, " "))
		}
	}
	// POSITIVE CONTROL: the store above must be non-trivial, or {} is free.
	if len(store) < 3 {
		t.Fatalf("the fixture store holds %d records; this test is not looking at "+
			"what it thinks", len(store))
	}
}

// rig's OWN decisions document, which is the only place several hundred entries
// and 292 part-of edges exist at once.
//
// It SKIPS rather than fails when the document is not there: DECISIONS.md is a
// gitignored symlink into the logbook, so a fresh clone and a CI runner have a
// dangling link.
func TestRigsOwnDecisionsDocumentPlansEveryEntry(t *testing.T) {
	const live = "../../DECISIONS.md"
	if _, err := os.Stat(live); err != nil {
		t.Skipf("%s does not resolve on this machine (%v)", live, err)
	}
	dec, err := readDecisions(live)
	if err != nil {
		t.Fatalf("reading %s: %v", live, err)
	}
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: live}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, dec)

	// POSITIVE CONTROL. An empty parse would pass every assertion below in
	// silence, which is how this project's checks have failed ten times.
	if len(dec.Decisions) < 300 {
		t.Fatalf("the parse yielded %d entries and the document held 457 when this "+
			"was written; this test is not looking at what it thinks", len(dec.Decisions))
	}

	planned := map[string]bool{}
	for _, in := range p.want {
		planned[in.id] = true
	}
	for _, e := range dec.Decisions {
		if !planned[e.Key] {
			t.Fatalf("%s is stated at %s:%d and no record was planned for it", e.Key, live, e.Line)
		}
	}

	// ⛔ EVERY EDGE THE DOCUMENT STATES SURVIVES. The parser carries sections so
	// that a child's parent always exists, so an orphan here is a real defect
	// rather than a document the seeder is walking out of.
	if got := strings.Join(p.orphaned, " "); got != "" {
		t.Errorf("orphaned = {%s}, want {} - every parent in this document is itself "+
			"an entry", got)
	}
	// ⛔ AND NOT ONE COLLISION, which would mean two entries silently sharing a
	// record and whichever went last winning.
	if got := strings.Join(p.collided, " "); got != "" {
		t.Errorf("collided = {%s}, want {}", got)
	}

	edges, notes, dated := 0, 0, 0
	for _, in := range p.want {
		if in.grain != grainEntry {
			continue
		}
		if in.partOf != "" {
			edges++
		}
		if in.kind == record.KindNote {
			notes++
		}
		if in.fields[fieldDocDate] != "" {
			dated++
		}
		// The coordinates are what make a record resolve back to a place in the
		// document, and a record that cannot is one nobody will mend.
		if in.fields[fieldDocKey] == "" || in.fields[fieldDocLine] == "" {
			t.Errorf("%s carries no document coordinates", in.id)
		}
		if _, err := strconv.Atoi(in.fields[fieldDocLine]); err != nil {
			t.Errorf("%s has %s = %q, which is not a line number", in.id, fieldDocLine, in.fields[fieldDocLine])
		}
	}
	if edges == 0 || notes == 0 || dated == 0 {
		t.Errorf("edges=%d notes=%d dated=%d; every one of the three is stated by "+
			"this document, so a zero means the projection dropped a whole class",
			edges, notes, dated)
	}
}
