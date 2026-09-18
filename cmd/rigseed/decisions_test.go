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
	"sort"
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
	}, record.DecisionParse{}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("what-rig-is", "What rig is", record.EntrySection),
		entryFor("2026-09-17-a-ruling", "2026-09-17 a ruling", record.EntryDecision),
	}}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{dated, undated}}, record.PlanParse{})

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
// ⛔ THE PARENT HERE IS A RULING AND NOT A STANDING SECTION, AND THAT IS THE
// FIX RATHER THAN A TIDY-UP. It used to be `what-rig-is`, which is a NOTE, and
// a note parent is now the one shape whose edge is deliberately not asserted -
// see TestARulingUnderAStandingSectionAssertsNoPartOf. What this test is about
// is the 292 edges that DO stand, so it names a parent of that kind.
func TestASubHeadingsParentIsAnEntryAndTheEdgeIsEmitted(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("2026-09-10-the-attack", "2026-09-10 the attack", record.EntryDecision),
		{
			Key: "2026-09-10-the-attack/the-ruling", Title: "The ruling", Kind: record.EntryDecision,
			Level: 3, PartOf: "2026-09-10-the-attack", Line: 12, Body: "prose",
		},
	}}, record.PlanParse{})

	if got := intentFor(t, p, "2026-09-10-the-attack/the-ruling").partOf; got != "2026-09-10-the-attack" {
		t.Errorf("part-of = %q, want 2026-09-10-the-attack", got)
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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("a-ruling", "a ruling", record.EntryDecision),
	}}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("what-rig-is", "What rig is", record.EntrySection),
	}}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("a-ruling", "a ruling", record.EntryDecision),
	}}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("what-rig-is", "What rig is", record.EntrySection),
		entryFor("a-ruling", "a ruling", record.EntryDecision),
	}}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("what-rig-is", "What rig is", record.EntrySection),
		{
			Key: "what-rig-is/the-ruling", Title: "The ruling", Kind: record.EntryDecision,
			Level: 3, PartOf: "what-rig-is", Line: 12, Body: "prose",
		},
	}}, record.PlanParse{})

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
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, dec, record.PlanParse{})

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

// ---------------------------------------------------------------------------
// THE ORDERED TABLE - B73
// ---------------------------------------------------------------------------

// rankedRow is one row of an ordered table as the parser reports it.
func rankedRow(rank, section string, line int) record.Unimported {
	return record.Unimported{
		Kind: record.UnimportedRowWithoutID, Label: rank, Section: section, Line: line,
		// The seeder derives the KEY from the slug and the TAG from the text,
		// so a fixture carrying only the slug tests half the pair.
		SectionTitle: "The critical path to the gate",
	}
}

// ⛔ THE ID NAMES ITS TABLE, AND A BARE RANK NAMESPACE IS THE FAILURE.
//
// RULED 2026-09-17 by the team-lead: the eleven ordered rows are work-items
// keyed on their rank and no `B` id is minted. `0`, `6a` and `9` are unique in
// this document today and unique only by luck, so the key is qualified by the
// enclosing heading - which is the whole reason `Unimported.Section` exists.
func TestARankedRowIsKeyedOnItsTableAndCarriesItsRank(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{
		Items:      []record.BacklogItem{{ID: "B1", Title: "a row"}},
		Unimported: []record.Unimported{rankedRow("9", "the-critical-path-to-the-gate", 135)},
	}, record.DecisionParse{}, record.PlanParse{})

	const want = "the-critical-path-to-the-gate/9"
	in := intentFor(t, p, want)
	if in.kind != record.KindWorkItem {
		t.Errorf("kind = %q, want %q", in.kind, record.KindWorkItem)
	}
	for name, w := range map[string]string{
		fieldRank:    "9",
		fieldSection: "the-critical-path-to-the-gate",
		fieldDocLine: "135",
	} {
		if got := in.fields[name]; got != w {
			t.Errorf("%s = %q, want %q", name, got, w)
		}
	}
	// ⛔ THE TAGS GO THROUGH THE READER. This row was in the table above,
	// comparing the raw string - a writer checked against itself.
	// The field keeps the full slug because the id is built from it; the tag
	// drops the article because a person reads it in a filter.
	if got := record.DecodeTags(in.fields[fieldTags]); len(got) != 2 ||
		got[0] != tagRankOnly || got[1] != tagSection+"critical-path-to-the-gate" {
		t.Errorf("tags decode to %v, want [%s %scritical-path-to-the-gate]",
			got, tagRankOnly, tagSection)
	}
	// ⛔ NO status. `Unimported` does not export the State cell, so `active`
	// would be the store contradicting its own document on every row the table
	// calls built - the B19 inversion, one grain over.
	if got, held := in.fields[fieldStatus]; held {
		t.Errorf("a ranked row was given status=%q, and the document's State cell "+
			"does not reach this grain at all", got)
	}
	// ⛔ NO part-of. `Under` is empty on these rows and heading enclosure alone
	// is the derivation the parser's notes record as falsified.
	if in.partOf != "" {
		t.Errorf("part-of = %q, want empty", in.partOf)
	}
	if got := strings.Join(p.orphaned, " "); got != "" {
		t.Errorf("orphaned = {%s}, want {}", got)
	}
}

// ⛔ TWO UNNUMBERED TABLES MUST NOT COLLIDE, WHICH IS THE ONE PROPERTY THE
// QUALIFIER BUYS.
//
// Keyed on the bare rank, the second table's row 0 would overwrite the first
// table's row 0 and every set in the report would agree about it - a wrong
// answer both halves of the instrument reach together.
func TestTwoOrderedTablesDoNotCollideBecauseTheIdNamesTheTable(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B1", Title: "a row"}},
		Unimported: []record.Unimported{
			rankedRow("0", "the-critical-path-to-the-gate", 125),
			rankedRow("0", "a-second-unnumbered-table", 600),
		},
	}, record.DecisionParse{}, record.PlanParse{})

	for _, want := range []string{"the-critical-path-to-the-gate/0", "a-second-unnumbered-table/0"} {
		if !has(ids(p.want), want) {
			t.Errorf("%s was not planned; the plan holds %v", want, ids(p.want))
		}
	}
	if got := strings.Join(p.collided, " "); got != "" {
		t.Errorf("collided = {%s}, want {} - the two ranks are in different tables", got)
	}
}

// ⛔ A ROW WITH NO SECTION IS REPORTED, NEVER KEYED ON THE RANK ALONE.
//
// The parser's own note says Section is empty on three of the seven kinds and
// calls that a gap rather than a decision. A fallback to the bare rank would
// turn that gap into the silent collision the field was added to prevent, so
// the row stays exactly where it was: reported, unimported, visible.
func TestARankedRowWithNoSectionIsReportedRatherThanKeyedOnItsRankAlone(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{
		Items:      []record.BacklogItem{{ID: "B1", Title: "a row"}},
		Unimported: []record.Unimported{rankedRow("0", "", 125)},
	}, record.DecisionParse{}, record.PlanParse{})

	for _, in := range p.want {
		if in.grain == grainRanked {
			t.Fatalf("%s was keyed from a row whose table has no name", in.id)
		}
	}
	if len(p.unimported) != 1 || p.unimported[0].Label != "0" {
		t.Errorf("the row was not reported; unimported = %v", p.unimported)
	}
	// The control: the same row WITH a section is keyed, so the refusal above
	// is a decision rather than a path nothing reaches.
	q := planFor(o, record.BacklogParse{
		Items:      []record.BacklogItem{{ID: "B1", Title: "a row"}},
		Unimported: []record.Unimported{rankedRow("0", "a-named-table", 125)},
	}, record.DecisionParse{}, record.PlanParse{})

	if !has(ids(q.want), "a-named-table/0") {
		t.Errorf("a row with a section was not keyed either, so this test proves nothing")
	}
}

// rig's OWN eleven, which is the only place the ranks and the adopter table
// exist together.
func TestRigsOwnOrderedTableIsKeyedAndTheAdopterTableIsNot(t *testing.T) {
	const live = "../../BACKLOG.md"
	if _, err := os.Stat(live); err != nil {
		t.Skipf("%s does not resolve on this machine (%v)", live, err)
	}
	doc, err := readBacklog(live)
	if err != nil {
		t.Fatalf("reading %s: %v", live, err)
	}
	o := options{project: "rig", backlog: live, decisions: "DECISIONS.md"}
	p := planFor(o, doc, record.DecisionParse{}, record.PlanParse{})

	var ranked []string
	for _, in := range p.want {
		if in.grain == grainRanked {
			ranked = append(ranked, in.fields[fieldRank])
		}
	}
	sort.Strings(ranked)
	if got := strings.Join(ranked, " "); got != "0 1 2 3 4 5 6 6a 7 8 9" {
		t.Errorf("the ranks planned = {%s}, want {0 1 2 3 4 5 6 6a 7 8 9}", got)
	}
	if !has(ids(p.want), "the-critical-path-to-the-gate/9") {
		t.Errorf("row 9 - Boris's own order - is not keyed on its table; the plan holds %v",
			ids(p.want))
	}

	// ⛔ THE FIVE other-table IDS STAY UNIMPORTED. They are real rows elsewhere
	// in the document, and importing them would supersede five real records
	// with a two-cell shape. Nothing has ruled on them.
	var left []string
	for _, u := range p.unimported {
		left = append(left, string(u.Kind)+" "+unimportedName(u.Unimported))
		if u.Kind == record.UnimportedRowWithoutID {
			t.Errorf("a ranked row is still unimported at %s:%d", u.doc, u.Line)
		}
	}
	if len(left) != 5 {
		t.Errorf("unimported = {%s}; the five other-table ids and nothing else were "+
			"expected to remain", strings.Join(left, ", "))
	}
}

// ⛔ AN ID THE TWO DOCUMENTS BOTH STATE IS REPORTED, NEVER WRITTEN TWICE.
//
// The store keys on the id alone and knows nothing about which document a
// record came from, so two puts against one id in one run supersede each other
// and whichever went last wins - a silent wrong answer decided by the order
// this seeder happens to read in. A backlog id is `B\d+` and a decision key is
// a title slug, so it is not expected; the point is that the day it happens it
// is a line in a report rather than a record nobody can account for.
func TestAnIdBothDocumentsStateIsReportedAndWrittenOnce(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B1", Title: "the row"}},
	}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("B1", "a ruling that took the row's id", record.EntryDecision),
	}}, record.PlanParse{})

	if got := strings.Join(p.collided, " "); got != "B1" {
		t.Errorf("collided = {%s}, want {B1}", got)
	}
	// The BACKLOG's record is the one written, because it was read first.
	in := intentFor(t, p, "B1")
	if in.kind != record.KindWorkItem || in.title != "the row" {
		t.Errorf("B1 was written as %q/%q; the first document read must win and "+
			"nothing may be written twice", in.kind, in.title)
	}
	n := 0
	for _, w := range p.want {
		if w.id == "B1" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("B1 is planned %d times; two puts against one id supersede each other", n)
	}
	if !has(d(p).nonEmpty(), "id-stated-twice") {
		t.Errorf("nonEmpty = %v; an id two documents claim must not exit clean", d(p).nonEmpty())
	}
}

// d is the divergence of a plan against a store that holds exactly it, so a
// test can assert on the sets a collision puts there without building a store.
func d(p plan) divergence {
	store := map[string]held{}
	for _, in := range p.want {
		store[in.id] = heldOf(in)
	}
	return diff(p, store)
}

// ⛔ A NOTE IS `part-of` THE PROJECT, AND NOTHING ELSE PUTS IT IN A BRIEF.
//
// `notesAbout` in internal/record/brief.go selects
// `l.type='part-of' AND n.kind='note' AND d.project=?` with the NOTE as
// `l.src`, and then keeps the row only where `l.dst` is in the subject set -
// which is seeded with the project id. So a note with no outgoing part-of is in
// no brief, and a note that is only ever an edge's DESTINATION is in no brief
// either.
//
// ⛔ THE STORE HELD THE ARROW THE OTHER WAY ROUND FOR FOUR GENERATIONS AND THE
// SPECIFICATION WAS BLAMED THREE TIMES. Measured against the production store
// on 2026-09-17: `rig record refs what-rig-is` answered "nothing points at
// what-rig-is within 4 hops", every note was the source of no edge at all, and
// six RULINGS pointed at one of them. plan/39's derivation was right and the
// data was wrong.
func TestANoteIsPartOfTheProjectSoTheBriefCanReachIt(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("what-rig-is", "What rig is", record.EntrySection),
		entryFor("2026-09-17-a-ruling", "2026-09-17 a ruling", record.EntryDecision),
	}}, record.PlanParse{})

	if got := intentFor(t, p, "what-rig-is").partOf; got != o.project {
		t.Errorf("a note states part-of %q, want %q - a note attached to nothing "+
			"is invisible to every brief", got, o.project)
	}

	// ⛔ AND A TOP-LEVEL RULING STATES NONE. Section 12 lists every decision
	// the project holds without joining on an edge, so a part-of here would be
	// a claim no derivation reads and one more edge to keep true.
	if got := intentFor(t, p, "2026-09-17-a-ruling").partOf; got != "" {
		t.Errorf("a top-level ruling states part-of %q, want none", got)
	}

	// The project record is written by seedProject and is in no intent, so the
	// undefined-parent sweep must know about it or it would strip the edge it
	// was just given and report it as a document defect.
	if got := strings.Join(p.orphaned, " "); got != "" {
		t.Errorf("orphaned = {%s}, want {} - the project record exists", got)
	}
}

// ⛔ A RULING UNDER A STANDING SECTION ASSERTS NO part-of. RULED by the
// team-lead, 2026-09-17, on the six edges the decisions import had written the
// wrong way round: they are to be UNLINKED rather than left beside the note's
// own edge to the project.
//
// ⛔ THE SECOND HALF IS WHAT STOPS THIS BEING A BLANKET RULE. A sub-heading
// under a RULING still states its parent, and that is 292 edges of this
// document: only a parent that is a NOTE loses the edge.
func TestARulingUnderAStandingSectionAssertsNoPartOf(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("the-attack", "Decisions taken during the attack", record.EntrySection),
		{
			Key: "the-attack/5-gap-is-cut", Title: "5. gap is cut", Kind: record.EntryDecision,
			Level: 3, PartOf: "the-attack", Line: 173, Body: "prose",
		},
		entryFor("2026-09-17-a-ruling", "2026-09-17 a ruling", record.EntryDecision),
		{
			Key: "2026-09-17-a-ruling/the-detail", Title: "The detail", Kind: record.EntryDecision,
			Level: 3, PartOf: "2026-09-17-a-ruling", Line: 200, Body: "prose",
		},
	}}, record.PlanParse{})

	if got := intentFor(t, p, "the-attack/5-gap-is-cut").partOf; got != "" {
		t.Errorf("a ruling under a standing section states part-of %q, want none - "+
			"that edge is the one the lead ruled unlinked", got)
	}
	if got := intentFor(t, p, "2026-09-17-a-ruling/the-detail").partOf; got != "2026-09-17-a-ruling" {
		t.Errorf("a sub-heading under a RULING states part-of %q, want "+
			"2026-09-17-a-ruling - only a note parent loses the edge", got)
	}
}

// ⛔ AN EDGE THE DOCUMENT STATES AND THIS SEEDER DOES NOT ASSERT IS NAMED.
//
// It is a RULED exclusion rather than a defect, so it is not in `orphaned` -
// an orphan names a parent nothing defines and wants mending, this names a
// parent that exists and a decision taken about it. Dropping it in silence
// would be the only way a reader could not tell the two apart.
func TestAnEdgeTheSeederDoesNotAssertIsNamedRatherThanDropped(t *testing.T) {
	o := options{project: "rig", decisions: "DECISIONS.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}, record.DecisionParse{Decisions: []record.DecisionEntry{
		entryFor("the-attack", "Decisions taken during the attack", record.EntrySection),
		{
			Key: "the-attack/5-gap-is-cut", Title: "5. gap is cut", Kind: record.EntryDecision,
			Level: 3, PartOf: "the-attack", Line: 173, Body: "prose",
		},
	}}, record.PlanParse{})

	if got := strings.Join(p.detached, " "); got != "the-attack/5-gap-is-cut -part-of-> the-attack" {
		t.Errorf("detached = {%s}, want the one edge the ruling drops", got)
	}
	if got := strings.Join(p.orphaned, " "); got != "" {
		t.Errorf("orphaned = {%s}, want {} - the parent exists, so this is not a "+
			"defect in the document", got)
	}

	// It reaches BOTH reports, because a seeding run and --check are read by
	// the same person asking the same question.
	var seed, check bytes.Buffer
	p.report(&seed)
	d(p).report(&check, o, p, "production")
	for name, b := range map[string]*bytes.Buffer{"the seeding report": &seed, "--check": &check} {
		if !strings.Contains(b.String(), "the-attack/5-gap-is-cut -part-of-> the-attack") {
			t.Errorf("%s does not name the edge it dropped:\n%s", name, b.String())
		}
		if !strings.Contains(b.String(), "DELIBERATELY NOT ASSERTED") {
			t.Errorf("%s does not label it a ruled exclusion:\n%s", name, b.String())
		}
	}
}
