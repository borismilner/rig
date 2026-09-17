package record

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ⛔ THIS FILE EXISTS BECAUSE THE PARSER COULD NOT SAY WHAT IT DID NOT TAKE.
//
// `ParseBacklog` answered 68 with no error over a document that addresses 70
// ids and holds eleven ordered rows it never saw. Both of this package's
// instruments key on the same id shape, so the absence was invisible BY
// CONSTRUCTION - and an absence a machine cannot report is one that survives
// every review. The specification these tests enforce is one sentence: a
// projection carries a thing only if an exact whole-set equality between the
// store and the source document can be computed for it by machine, in one
// command, with no human judging.
//
// ⛔ NO `t.Helper()` ANYWHERE IN THIS FILE, AND THAT IS DELIBERATE. A helper
// marked `t.Helper()` re-attributes its failure to the CALLER's line, so a
// mutation pass over a helper with six assertions reports all six as whichever
// call site happened to run and cannot tell the six apart. Every assertion
// here either stands in its own test or carries a `what` string that names it
// in the message.

// sameSet compares two sets of strings and names what differs in both
// directions, because a count equality hides a swap.
func sameSet(t *testing.T, what string, got, want []string) {
	g, w := append([]string(nil), got...), append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if strings.Join(g, "\n") == strings.Join(w, "\n") {
		return
	}
	in := func(s []string, x string) bool {
		for _, y := range s {
			if y == x {
				return true
			}
		}
		return false
	}
	var extra, absent []string
	for _, x := range g {
		if !in(w, x) {
			extra = append(extra, x)
		}
	}
	for _, x := range w {
		if !in(g, x) {
			absent = append(absent, x)
		}
	}
	t.Errorf("%s: the sets differ\n  IN THE PARSE, NOT EXPECTED (%d): %v\n"+
		"  EXPECTED, NOT IN THE PARSE (%d): %v", what, len(extra), extra, len(absent), absent)
}

// render is one unimported thing as a single comparable line, so a set
// comparison can carry every field rather than just the id.
//
// ⛔ IT CARRIES `section` AS WELL AS `under`, AND THE TWO DIFFERING ON THE
// SAME LINE IS THE POINT. Under is the nearest ID-BEARING heading and is
// empty on all eleven ordered rows; Section is the nearest heading of any
// level and says which table they came from. A render that printed one of
// them would let the other go wrong with no signal, which is this file's own
// subject.
//
// ⛔ AND IT CARRIES `title` BESIDE `label` FOR THE SAME REASON. On the eleven
// ordered rows Label is the RANK and Title is the work, and the live store had
// eleven records titled `0`..`9` because only one of the two existed. A render
// printing Label alone would go green over that again.
//
// `cells` is a COUNT rather than the cells, because a row's cells are the
// document's own prose and a set of expected lines carrying them would be
// unreadable. The count separates "the row is here" from "the row arrived
// empty", which is the failure that happened; the cells themselves are pinned
// by value in TestAnOrderedRowCarriesTheWholeRowAndNotJustItsRank.
func render(u Unimported) string {
	return fmt.Sprintf("%s id=%q label=%q title=%q under=%q section=%q cells=%d",
		u.Kind, u.ID, u.Label, u.Title, u.Under, u.Section, len(u.Cells))
}

func renderAll(us []Unimported) []string {
	out := make([]string, 0, len(us))
	for _, u := range us {
		out = append(out, render(u))
	}
	return out
}

// ⛔ STAGE A. This assertion was written to go RED against the parser as it
// stood and the red is recorded in the agent-work notes before anything moved.
const irregularIDDoc = `| # | Work | Seat | State |
|---|---|---|---|
| B60-2 | **an id nobody has agreed the shape of** | lead | **OPEN** |
`

func TestAnIdShapeTheParserCannotReadIsNeverReadAsAPrefix(t *testing.T) {
	items, err := ParseBacklog(strings.NewReader(irregularIDDoc))
	if err != nil {
		t.Fatalf("the parse must not die on an irregular id: %v", err)
	}
	for _, it := range items {
		if it.ID == "B60" {
			t.Errorf("a row whose id cell says B60-2 was read as the record B60, "+
				"and B60 is an OCCUPIED row in rig's own backlog; title %q", it.Title)
		}
	}
	if len(items) != 0 {
		t.Errorf("an id shape this parser cannot read must yield NO record, got %d", len(items))
	}
}

// ⛔ THE OCCUPIED-ROW CASE, WHICH IS THE ONE THE OLD COMMENT GOT WRONG.
// `B60-2` read as `B60` when B60 already exists is swallowed by the duplicate
// rule, so the promised "red from the set guard" never fires and the row is
// gone without a trace. The repair is that it is REPORTED.
const irregularAfterRealDoc = `| # | Work | Seat | State |
|---|---|---|---|
| B60 | **the real row, and it is occupied** | lead | **OPEN** |
| B60-2 | **the invented shape, which must not vanish into B60** | lead | **OPEN** |
`

func TestAnIrregularIdBesideAnOccupiedRowIsReportedRatherThanDropped(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(irregularAfterRealDoc))
	if err != nil {
		t.Fatalf("the parse must not die on an irregular id: %v", err)
	}
	var ids []string
	for _, it := range p.Items {
		ids = append(ids, it.ID)
	}
	sameSet(t, "the records read", ids, []string{"B60"})
	sameSet(t, "what the parser reports it did not take", renderAll(p.Unimported),
		[]string{
			"irregular-id id=\"\" label=\"B60-2\" title=\"the invented shape, which must not vanish into B60\" under=\"\" section=\"\" cells=4",
		})
	for _, it := range p.Items {
		if it.ID == "B60" && strings.Contains(it.Title, "invented") {
			t.Errorf("B60's record took the INVENTED row's title, which is a silent "+
				"mis-id rather than a silent drop: %q", it.Title)
		}
	}
}

// ⛔ THE CRITICAL PATH'S SHAPE, AND THE SEPARATOR THAT MUST NOT BE REPORTED
// WITH IT. A detector keying on "a row in a work-item table with no id" that
// does not exclude separators reports three false positives against rig's own
// backlog, which is a report nobody would trust twice.
const orderedRowsDoc = `## The critical path to the gate

| # | Work | Seat | State |
|---|---|---|---|
| 0 | **the first rank** | lead | **BUILT** |
| 6a | **a rank that is not a number** | either | **DONE** |
| B60 | **a row that does carry an id** | lead | **OPEN** |
`

func TestARowInsideAWorkItemTableWithNoIdIsReportedAndSeparatorsAreNot(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(orderedRowsDoc))
	if err != nil {
		t.Fatalf("an id-less row must be reported, never refused: %v", err)
	}
	var ids []string
	for _, it := range p.Items {
		ids = append(ids, it.ID)
	}
	sameSet(t, "the records read", ids, []string{"B60"})
	sameSet(t, "the ordered rows the parser reports it did not take", renderAll(p.Unimported),
		[]string{
			"row-without-id id=\"\" label=\"0\" title=\"the first rank\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"6a\" title=\"a rank that is not a number\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
		})
	for _, u := range p.Unimported {
		if strings.HasPrefix(u.Label, "-") || u.Label == "---" {
			t.Errorf("a table separator was reported as an unimported row: %+v", u)
		}
	}
}

// ⛔ THE HEADING GRAIN. B46 - the MVP acceptance test - is a heading, and a
// parser whose unit is a table row cannot say it exists. Reporting it is the
// minimum. `under` on a heading is heading-to-heading NESTING, which is the
// document's own structure; it is not the same thing as a row's position
// inside a table, which the test below shows is not safe to read as part-of.
const headingGrainDoc = `## ⛔ B46 - THE MVP ACCEPTANCE TEST

| # | Work | Seat | State |
|---|---|---|---|
| B46a | **a child row** | record | **DONE** |

### ⛔ B46d - A CHILD THAT IS ITSELF A HEADING

prose, so the table is over.
`

func TestAHeadingBorneIdIsReportedWithTheParentTheDocumentNestsItUnder(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(headingGrainDoc))
	if err != nil {
		t.Fatalf("a heading-borne id must be reported, never refused: %v", err)
	}
	sameSet(t, "the heading-borne ids the parser reports it did not take", renderAll(p.Unimported),
		[]string{
			"heading id=\"B46\" label=\"⛔ B46 - THE MVP ACCEPTANCE TEST\" title=\"THE MVP ACCEPTANCE TEST\" under=\"\" section=\"\" cells=0",
			"heading id=\"B46d\" label=\"⛔ B46d - A CHILD THAT IS ITSELF A HEADING\" title=\"A CHILD THAT IS ITSELF A HEADING\" under=\"B46\" section=\"b46-the-mvp-acceptance-test\" cells=0",
		})
	if len(p.Items) != 1 || p.Items[0].ID != "B46a" {
		t.Fatalf("the row grain must be unchanged, got %d items: %+v", len(p.Items), p.Items)
	}
	if got := p.Items[0].PartOf; got != "B46" {
		t.Errorf("B46a's own id carries the sub-letter, so part-of is inside the id "+
			"and owes nothing to which heading the row landed under; got %q", got)
	}
}

// ⛔ THE FIVE IDS IN THE ADOPTER TABLE WERE TRACKED AND NEVER REPORTED.
// `notWorkItems` existed only to keep the set guard quiet; nothing could read
// it, so "this parser decided that is not a work item" and "this parser never
// saw it" were the same silence.
const otherTableDoc = `| Row | Adopter, as a ROLE |
|---|---|
| B6 | ` + "`backend-record`" + ` |
`

func TestAnIdInATableThatIsNotAWorkItemTableIsReported(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(otherTableDoc))
	if err != nil {
		t.Fatalf("an id in another table must be reported, never refused: %v", err)
	}
	if len(p.Items) != 0 {
		t.Errorf("a two-column adopter row is not a work item, got %+v", p.Items)
	}
	sameSet(t, "the other-table ids the parser reports it did not take", renderAll(p.Unimported),
		[]string{
			"other-table id=\"B6\" label=\"B6\" title=\"\" under=\"\" section=\"\" cells=2",
		})
	// ⛔ THE SECOND CELL IS THE FACT THIS TABLE EXISTS TO STATE, and Label -
	// the first cell - is the id it is stated ABOUT. A grain keeping only the
	// first cell of an adopter table keeps the subject and drops the claim.
	if got := p.Unimported[0].Cells; len(got) != 2 || got[1] != "`backend-record`" {
		t.Errorf("the adopter cell is what this table says and it is not carried: %q", got)
	}
}

// backlogDocument reads rig's REAL document through the same gitignored
// symlink and the same skip-with-an-off-switch as the acceptance
// demonstration. It reuses `backlogPath` and `requireBacklog` rather than
// restating them: two copies of one path is the drift this package spends its
// time correcting.
func backlogDocument(t *testing.T) BacklogParse {
	raw, err := os.ReadFile(backlogPath)
	if err != nil {
		if os.Getenv(requireBacklog) != "" {
			t.Fatalf("%s is set, so this measurement was DEMANDED, and rig's own backlog "+
				"is not at %s from here: %v", requireBacklog, backlogPath, err)
		}
		t.Skipf("rig's own backlog is not at %s from here: %v\n"+
			"⛔ THIS IS A SKIP, NOT A PASS. Set %s=1 to make it a failure.",
			backlogPath, err, requireBacklog)
	}
	p, err := ParseBacklogDocument(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRigsOwnBacklogSaysExactlyWhatItDidNotImport pins the SET, never a total.
//
// ⛔ IT MOVES WHEN THE DOCUMENT MOVES, AND THAT IS THE POINT. This is the one
// command the specification asks for: an exact whole-set equality between what
// the parser takes and what the document addresses, with no human judging. A
// red here means Boris added a heading, an ordered row or an id shape - and
// the answer is to read the diff and update the pin in a commit that says so,
// which is a thing that can be DONE. Before this test, the same edit produced
// no signal at all.
func TestRigsOwnBacklogSaysExactlyWhatItDidNotImport(t *testing.T) {
	p := backlogDocument(t)

	sameSet(t, "every id and ordered row rig's own backlog addresses and no record carries",
		renderAll(p.Unimported), []string{
			"row-without-id id=\"\" label=\"0\" title=\"M2's agent front door\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"1\" title=\"§37.6 a named estate refuses a held name\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"2\" title=\"§37.1 estate identity in the protocol\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"3\" title=\"§37.3 build and semantic skew detected\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"4\" title=\"§37.2 storage keyed by estate\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"5\" title=\"§37.5 systemd manages production only\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"6\" title=\"M7 Peers - THE MINIMUM SET ONLY\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"6a\" title=\"§38b: the library search for rows 2, 3 and the WAL\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"7\" title=\"The FIRST cutover, and the gate\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"8\" title=\"The remaining cutovers, one capability at a time\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"row-without-id id=\"\" label=\"9\" title=\"§39 THE CONTINUITY RECORD - and this row is BORIS'S ORDER, not a seat's reading\" under=\"\" section=\"the-critical-path-to-the-gate\" cells=4",
			"heading id=\"B46\" label=\"⛔ B46 - THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT ALL\" title=\"THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT ALL\" under=\"\" section=\"rig-s-development-plan\" cells=0",
			"heading id=\"B46d\" label=\"⛔ B46d - THE CYCLE IS CONSTRUCTED, AND THE RELAY SAID OTHERWISE\" title=\"THE CYCLE IS CONSTRUCTED, AND THE RELAY SAID OTHERWISE\" under=\"B46\" section=\"b46-the-mvp-acceptance-test-and-it-existed-in-no-document-at-all\" cells=0",
			"other-table id=\"B6\" label=\"**B6** two shipped-output changes, one migration\" title=\"\" under=\"\" section=\"the-adopter-column-holds-a-role-never-a-session-name\" cells=2",
			"other-table id=\"B10\" label=\"**B10** `--json` renders in the daemon\" title=\"\" under=\"\" section=\"the-adopter-column-holds-a-role-never-a-session-name\" cells=2",
			"other-table id=\"B18\" label=\"**B18** the session token has no consumer\" title=\"\" under=\"\" section=\"the-adopter-column-holds-a-role-never-a-session-name\" cells=2",
			"other-table id=\"B20\" label=\"**B20** the stub surface\" title=\"\" under=\"\" section=\"the-adopter-column-holds-a-role-never-a-session-name\" cells=2",
			"other-table id=\"B21\" label=\"**B21** `wire.proto`'s `request_id` comment\" title=\"\" under=\"\" section=\"the-adopter-column-holds-a-role-never-a-session-name\" cells=2",
		})

	// ⛔ THE ELEVEN ARE ORDERED AND THE ORDER IS THE DOCUMENT'S OWN.
	// `BACKLOG.md:109` calls them "Ordered", row 9 is Boris's own. Nothing in
	// section 39's model expresses a total order, so the parser reports them in
	// DOCUMENT ORDER and invents no mapping - which is a ruling, not a parse.
	var ranks []string
	for _, u := range p.Unimported {
		if u.Kind == UnimportedRowWithoutID {
			ranks = append(ranks, u.Label)
		}
	}
	if got, want := strings.Join(ranks, " "), "0 1 2 3 4 5 6 6a 7 8 9"; got != want {
		t.Errorf("the critical path must be reported in the document's own order\n  want: %s\n  got:  %s", want, got)
	}
}

// TestEveryBacklogIdInTheDocumentIsEitherARecordOrReported is the whole-set
// equality itself, and it is the assertion the specification actually names.
//
// ⛔ ITS INSTRUMENT SHARES NO CODE PATH WITH THE PARSER. It is a token scan
// over the whole file with no notion of a cell, a table, a header or a
// heading, so it cannot agree with the parser by sharing an assumption - which
// is exactly how both of this package's existing instruments went blind to the
// same class at once.
func TestEveryBacklogIdInTheDocumentIsEitherARecordOrReported(t *testing.T) {
	p := backlogDocument(t)
	raw, err := os.ReadFile(backlogPath)
	if err != nil {
		t.Fatalf("the document was readable a moment ago: %v", err)
	}

	accounted := map[string]bool{}
	for _, it := range p.Items {
		accounted[it.ID] = true
	}
	for _, u := range p.Unimported {
		if u.ID != "" {
			accounted[u.ID] = true
		}
	}

	tok := regexp.MustCompile(`\bB\d+[a-z]?\b`)
	seenTok := map[string]bool{}
	var unaccounted []string
	for _, m := range tok.FindAllString(string(raw), -1) {
		if seenTok[m] {
			continue
		}
		seenTok[m] = true
		if !accounted[m] {
			unaccounted = append(unaccounted, m)
		}
	}
	sort.Strings(unaccounted)

	// POSITIVE CONTROL: the scan must actually find ids, or an empty
	// `unaccounted` proves nothing. 70 tokens on 2026-09-17.
	if len(seenTok) < 60 {
		t.Fatalf("the token scan found only %d backlog ids in %d bytes, which means "+
			"the INSTRUMENT is broken and this test's green would be meaningless",
			len(seenTok), len(raw))
	}
	if len(unaccounted) > 0 {
		t.Errorf("%d backlog ids appear in the document and the parser can neither "+
			"import them nor name them: %v\n"+
			"⛔ THAT IS THE DEFECT THIS FILE EXISTS FOR - an absence no instrument "+
			"can report is one that survives every review.", len(unaccounted), unaccounted)
	}
}

// ⛔ HEADING ENCLOSURE IS NOT A SAFE SOURCE OF `part-of` IN THIS DOCUMENT, AND
// THIS TEST IS THE MEASUREMENT THAT SAYS SO.
//
// The specification this repair was written against recommends both
// derivations - "`B46a -> B46` is already inside the id the parser splits, and
// a row under `## B46` is enclosed by it. Exact, derivable, checkable." ⛔ THE
// SECOND HALF IS FALSIFIED BY THE DOCUMENT: the table under `## ⛔ B46` has
// accumulated twenty-four rows that are not B46's children at all - B61 is
// "SECTION 6's CONFIGURATION SYSTEM IS UNBUILT" and has nothing to do with the
// acceptance test. Deciding which of them belongs to B46 needs a human
// reading, which is exactly what the projection criterion forbids.
//
// So the sub-letter is the ONLY source. It is inside the id, so it cannot
// drift from the row's position in a file somebody is still editing.
func TestPartOfComesFromTheIdAndNeverFromWhichHeadingARowLandedUnder(t *testing.T) {
	p := backlogDocument(t)
	got := map[string]string{}
	var withParent []string
	for _, it := range p.Items {
		got[it.ID] = it.PartOf
		if it.PartOf != "" {
			withParent = append(withParent, it.ID+"->"+it.PartOf)
		}
	}
	sameSet(t, "every row whose parent the document states inside its own id", withParent,
		[]string{
			"B46a->B46", "B46b->B46", "B46c->B46",
			"B46e->B46", "B46f->B46", "B46g->B46",
		})
	for _, id := range []string{"B61", "B58", "B70"} {
		if got[id] != "" {
			t.Errorf("%s sits in the table under `## ⛔ B46` and is not a sub-task of "+
				"it; heading enclosure gave it part-of %q, which is an edge the "+
				"document never states", id, got[id])
		}
	}
}

// ⛔ THE TERMINAL WORD, AND THE FIXTURE FOR IT IS INLINE ON PURPOSE.
// `testdata/backlog-shapes.md` is not a `_test.go` file and is not this
// change's to edit while other seats are in the tree; every shape needed here
// is three lines, so nothing is gained by putting them in a shared file.
//
// The four rows are the four ways this document closes one:
//   - a struck title with the word in a SECOND bold run - B19's shape, and the
//     only one `boldLead` cannot reach;
//   - a terminal lead in the item cell - B21's;
//   - a terminal lead in the STATE cell with the work open - B55's;
//   - a struck title with no word after it at all - B7's and B11's, where the
//     document puts the word in the Evidence cell, which is a different axis.
const dispositionDoc = `| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B1 | ~~**a struck title**~~ **RETRACTED 2026-09-16 by the lead** | evidence | a seat | **argued** |
| B2 | **CLOSED 2026-09-16 late, and the lead is in the item cell** | evidence | a seat | **argued** |
| B3 | **work nobody finished** | evidence | a seat | **REJECTED by a ruling** |
| B4 | ~~**a struck title and nothing after it**~~ | **DONE, but in the EVIDENCE cell** | a seat | **argued** |
| B5 | **an open row** | evidence | nobody | **OPEN** |
`

func TestTheTerminalWordSurvivesTheParseOnEveryShapeThatClosesARow(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(dispositionDoc))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range p.Items {
		got[it.ID] = it.Disposition
	}
	for _, c := range []struct{ id, want, why string }{
		{"B1", "RETRACTED", "the word is a SECOND bold run after the strike closes, " +
			"which is the run `boldLead` cannot reach and the whole reason this is " +
			"not an export of something already computed"},
		{"B2", "CLOSED", "a terminal lead in the item cell"},
		{"B3", "REJECTED", "a terminal lead in the state cell, work still open"},
		{"B4", "", "the document names no word where the strike closed the row - " +
			"the Evidence cell is a different axis and reading it would be a guess"},
		{"B5", "", "an open row has no disposition"},
	} {
		if got[c.id] != c.want {
			t.Errorf("%s disposition is %q, want %q - %s", c.id, got[c.id], c.want, c.why)
		}
	}
}

// ⛔ A STRUCK TITLE CONTAINING A TERMINAL WORD MUST NOT BECOME THE ROW'S
// DISPOSITION. This is the reason the struck search is bounded to `afterStrike`
// rather than run over the whole cell: rig's own backlog has titles that quote
// the words they are about, and an unbounded scan would read the title.
const strikeTrapDoc = `| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B1 | ~~**DONE was printed over a set that was not done**~~ | evidence | a seat | **argued** |
`

func TestAWordInsideAStruckTitleIsNotTheRowsDisposition(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(strikeTrapDoc))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 {
		t.Fatalf("want one row, got %d", len(p.Items))
	}
	if d := p.Items[0].Disposition; d != "" {
		t.Errorf("the struck TITLE's own words were read as the row's disposition: %q", d)
	}
}

// TestRigsOwnBacklogSaysWhichWordClosedEachRow pins the sets, and the last of
// them is the one worth the most: rows the document closed without naming a
// word anywhere the closing clause could read.
func TestRigsOwnBacklogSaysWhichWordClosedEachRow(t *testing.T) {
	p := backlogDocument(t)
	by := map[string][]string{}
	var closedWithNoWord []string
	for _, it := range p.Items {
		if it.Disposition != "" {
			by[it.Disposition] = append(by[it.Disposition], it.ID)
		}
		if (it.Done || it.ClaimsDone) && it.Disposition == "" {
			closedWithNoWord = append(closedWithNoWord, it.ID)
		}
	}
	sameSet(t, "RETRACTED", by["RETRACTED"], []string{"B19"})
	sameSet(t, "CLOSED", by["CLOSED"], []string{"B15", "B21", "B48", "B55", "B56"})
	// B65 joins 2026-09-17: the field predicate landed at rig `1c3a8c4` and
	// the row was struck with DONE after the strike. B64 joins the same day
	// at `22faf89`, the same shape.
	//
	// ⛔ THIS IS THE THIRD PIN A CLOSURE MOVES AND IT IS THE ONE THE ENTRYPOINT
	// DID NOT NAME. It said two - the counts and the struck SET - and this
	// closing-WORD set is a third, found by generation 11 running it. Closing a
	// row therefore touches backlogpin_test.go TWICE and this file ONCE.
	// B75 and B76 join 2026-09-17 later: the write path no longer discards
	// piped prose and a brief for a missing project is named and exits 1.
	// Both were struck with DONE the same day.
	// B66 joins 2026-09-17: the heading grain reached the store with its title
	// and status, `rigseed --check` computes the whole-set equality the row's
	// own bar names, and B46 is in the live brief. Struck, closing word DONE.
	// B68, B87 and B88 join 2026-09-17 later, all three with DONE. B68 is the
	// thirteenth brief section; B87 and B88 were FILED AND CLOSED in the same
	// session, which is why two rows arrived already struck.
	sameSet(t, "DONE", by["DONE"],
		[]string{
			"B20", "B22", "B24", "B25", "B31", "B33", "B44", "B46a", "B64",
			"B65", "B66", "B68", "B75", "B76", "B87", "B88", "B9",
		})
	sameSet(t, "REJECTED", by["REJECTED"], nil)

	// ⛔ B19 IS THE ROW THE WHOLE FIELD EXISTS FOR. It was RETRACTED as
	// falsified, the store could only say `closed`, and closed is what a
	// finished item says too. One word is the difference between "this was
	// done" and "this was withdrawn because it was wrong".
	sameSet(t, "the rows this document closes without naming a word where the "+
		"closing clause reads", closedWithNoWord, []string{"B11", "B7"})
}

// ⛔ THE GRAIN KEPT ONE CELL OUT OF FOUR AND THREW THE ROW AWAY.
//
// Measured over rig's own critical path on 2026-09-17, before this test
// existed: 34 bytes kept - the eleven ranks - and 4,656 bytes dropped. The
// live production store held all eleven records titled `0`..`9` and `6a`,
// body and all, because the RANK was the only part of the row that survived
// the parse. `rig record get the-critical-path-to-the-gate/0` answered
// `title 0` and `body 0`.
//
// ⛔ AND THE ASYMMETRY WAS ALREADY WRITTEN DOWN IN THE CONSUMER. `cmd/rigseed`
// calls its `row record.BacklogItem` field "the EVIDENCE; fields is the
// projection of it, and the report that names irregular rows reads the
// evidence rather than re-deriving it" - and for these eleven there WAS no
// evidence, so that sentence was false for every one of them.
func TestAnOrderedRowCarriesTheWholeRowAndNotJustItsRank(t *testing.T) {
	p := backlogDocument(t)

	var ranked int
	for _, u := range p.Unimported {
		if u.Kind != UnimportedRowWithoutID {
			continue
		}
		ranked++
		// `| # | Work | Seat | State |` - four content cells, and the parser
		// declined the row at the id rather than at the shape.
		if len(u.Cells) != 4 {
			t.Errorf("rank %q (line %d) carries %d cells, want 4 - the row this "+
				"grain declined has four and a seeder cannot re-derive them "+
				"without opening the document again", u.Label, u.Line, len(u.Cells))
			continue
		}
		if u.Cells[0] != u.Label {
			t.Errorf("rank %q (line %d): Cells[0] is %q, and Label's contract is to "+
				"BE the first cell verbatim", u.Label, u.Line, u.Cells[0])
		}
		// ⛔ THE STATE CELL IS THE ONE THE BRIEF IS BLOCKED ON. Every one of
		// the eleven leads its State cell with a terminal word, and nothing
		// could read it. Asserted as non-empty rather than parsed, because
		// what a state cell MEANS on a rank-keyed row is a ruling this parser
		// does not own.
		if strings.TrimSpace(u.Cells[3]) == "" {
			t.Errorf("rank %q (line %d) has an empty State cell: %q", u.Label, u.Line, u.Cells)
		}
		// ⛔ BOTH HALVES TOGETHER. A Title equal to the Label is the defect
		// that reached the store, and a Title that is merely non-empty would
		// pass over it.
		if u.Title == "" || u.Title == u.Label {
			t.Errorf("rank %q (line %d) has Title %q - the rank is not the work, and "+
				"a record titled with its own rank is what shipped", u.Label, u.Line, u.Title)
		}
	}
	// The positive control. Without it this passes over a parse that found no
	// ranked rows at all, which is the absence the whole grain exists for.
	if ranked != 11 {
		t.Fatalf("the critical path has %d ranked rows, want 11 - either the document "+
			"moved, in which case update this number in a commit that says so, or "+
			"nothing above was checked", ranked)
	}

	// ⛔ ONE ROW PINNED BY VALUE, because the loop above checks SHAPE and a
	// grain that carried four cells of the wrong row would satisfy every
	// assertion in it. `6a` is the row whose rank is not a number, which is
	// the one a rank-keyed instrument is most likely to mishandle.
	var six []string
	for _, u := range p.Unimported {
		if u.Kind == UnimportedRowWithoutID && u.Label == "6a" {
			six = u.Cells
		}
	}
	if len(six) != 4 {
		t.Fatalf("rank 6a was not found with four cells: %q", six)
	}
	if want := "**§38b: the library search for rows 2, 3 and the WAL**"; six[1] != want {
		t.Errorf("rank 6a's Work cell is %q, want %q", six[1], want)
	}
	if want := "either worker"; six[2] != want {
		t.Errorf("rank 6a's Seat cell is %q, want %q", six[2], want)
	}
	if !strings.HasPrefix(six[3], "**DONE 2026-09-12, and row 6 is unblocked.**") {
		t.Errorf("rank 6a's State cell is %q, and it is supposed to lead with its "+
			"terminal word the way every row of this table does", six[3])
	}
}

// ⛔ A ROW'S CELLS ARE CARRIED VERBATIM, DECORATION AND ALL, and that is the
// same contract Label already has. The test is written over a fixture because
// no row of rig's own document exercises the interesting edge: a cell that is
// empty, and a cell that holds a pipe inside code markers.
const cellsVerbatimDoc = "## a section\n" +
	"\n" +
	"| # | Work | Seat | State |\n" +
	"|---|---|---|---|\n" +
	"| 0 | **the work** ⛔ *emphasis kept* | | **OPEN** |\n"

func TestARowsCellsAreCarriedVerbatimIncludingTheEmptyOnes(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(cellsVerbatimDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.Unimported) != 1 {
		t.Fatalf("want one reported row, got %d: %+v", len(p.Unimported), p.Unimported)
	}
	u := p.Unimported[0]
	want := []string{"0", "**the work** ⛔ *emphasis kept*", "", "**OPEN**"}
	if strings.Join(u.Cells, "|") != strings.Join(want, "|") {
		t.Errorf("Cells = %q, want %q\n⛔ AN EMPTY CELL IS A CELL: dropping it "+
			"shifts every cell after it, which is the defect B21 already cost "+
			"this parser once", u.Cells, want)
	}
	// The leading and trailing pipes are not cells and must not be carried as
	// two empty strings a consumer has to know to skip.
	if len(u.Cells) != 4 {
		t.Errorf("Cells has %d entries and the row has four cells: %q", len(u.Cells), u.Cells)
	}
}

// ⛔ THE TRUE BRANCH OF `Struck` ON A ROW, WHICH THE LIVE DOCUMENT NEVER
// EXERCISES. `grep -c '~~'` over the eleven ordered rows returns 0, so without
// this fixture `Struck` would be pinned only where it is false - and false
// means both "the document did not strike it" and "nothing looked", which is
// B46's failure verbatim one kind over.
const struckRowDoc = "## a section\n" +
	"\n" +
	"| # | Work | Seat | State |\n" +
	"|---|---|---|---|\n" +
	"| 0 | ~~**the closed work**~~ **DONE** | lead | **DONE** |\n" +
	"| 1 | **the open work** | lead | **OPEN** |\n"

func TestARowWithoutAnIdReportsTheDocumentsOwnStrikethrough(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(struckRowDoc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := map[string]bool{}
	titles := map[string]string{}
	for _, u := range p.Unimported {
		got[u.Label] = u.Struck
		titles[u.Label] = u.Title
	}
	if len(got) != 2 {
		t.Fatalf("want both rows reported, got %d: %+v", len(got), p.Unimported)
	}
	if !got["0"] {
		t.Errorf("a struck row reports Struck=false, so a closed row reads as open")
	}
	if got["1"] {
		t.Errorf("an unstruck row reports Struck=true")
	}
	// ⛔ AND THE TITLE COMES OFF THE SAME CELL BY THE SAME RULE AN IMPORTED
	// ROW USES. `titleOf` takes the first bolded run, struck or not, so a
	// struck row's title is its work and not its terminal word.
	if want := "the closed work"; titles["0"] != want {
		t.Errorf("the struck row's Title is %q, want %q", titles["0"], want)
	}
}
