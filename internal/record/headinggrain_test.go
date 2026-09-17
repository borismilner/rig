package record_test

import (
	"os"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// ⛔ B46 WAS SEEDED WITH ITS OWN DECORATION AND ITS OWN ID IN ITS TITLE.
// Measured 2026-09-17 against the live production store, by the seat that
// built the heading import: the stored title read
// `⛔ B46 - THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT ALL`,
// where every row-grain title goes through `titleOf` and carries neither.
//
// TWO GRAINS RENDERING ONE DOCUMENT TWO WAYS is the failure this file exists
// to stop, and it arrived the first time a second grain shipped.
func TestAHeadingBorneIdGetsATitleAndNotItsOwnName(t *testing.T) {
	const doc = `# b

## ⛔ B46 - THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT ALL

prose

| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B1 | **a row** | e | a | **open** |
`
	p, err := record.ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u := onlyHeading(t, p)

	if want := "THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT ALL"; u.Title != want {
		t.Errorf("Title = %q, want %q", u.Title, want)
	}
	// ⛔ Label's CONTRACT IS TO BE VERBATIM and the fix must not quietly
	// change it: a report saying "the document wrote this here" has to quote
	// what the document wrote.
	if !strings.HasPrefix(u.Label, "⛔ B46") {
		t.Errorf("Label stopped being verbatim: %q", u.Label)
	}
	if u.Struck {
		t.Errorf("an unstruck heading reports Struck: %+v", u)
	}
}

// AND THE CLOSURE MARK IS READ AT THIS GRAIN TOO. The document's convention is
// its own - B7's state cell says "done, struck not deleted" - so a struck
// heading is closed for the same stated reason a struck row is.
//
// ⛔ NO HEADING IN rig's OWN BACKLOG IS STRUCK, so this is the only place the
// true branch is exercised. That is a weaker pin than the row grain's and it
// is named rather than glossed.
func TestAStruckHeadingIsClosedByTheDocumentsOwnConvention(t *testing.T) {
	const doc = `# b

## ⛔ ~~B46 - THE MVP ACCEPTANCE TEST~~

prose
`
	p, err := record.ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u := onlyHeading(t, p)

	if !u.Struck {
		t.Errorf("a struck heading does not report Struck: %+v", u)
	}
	if u.ID != "B46" {
		t.Errorf("the strike swallowed the id: %q", u.ID)
	}
	if want := "THE MVP ACCEPTANCE TEST"; u.Title != want {
		t.Errorf("Title = %q, want %q", u.Title, want)
	}
}

// A heading that states an id and nothing else keeps an empty title rather
// than borrowing its own name, and the seeder is what decides what to do
// about that - reporting the absence is this parser's whole job.
func TestAHeadingThatIsOnlyAnIdHasNoTitleToGive(t *testing.T) {
	const doc = `# b

## B46
`
	p, err := record.ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u := onlyHeading(t, p)
	if u.Title != "" {
		t.Errorf("Title = %q, want empty - there is no title in that heading", u.Title)
	}
}

// THE REAL DOCUMENT, because the three above are fixtures and rig's own
// backlog is what gets seeded.
func TestTheRealBacklogsHeadingsCarryCleanTitles(t *testing.T) {
	f := openTheBacklogDocument(t)
	p, err := record.ParseBacklogDocument(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var heads int
	for _, u := range p.Unimported {
		if u.Kind != record.UnimportedHeading {
			continue
		}
		heads++
		if u.Title == "" {
			t.Errorf("%s (line %d) has no title: %q", u.ID, u.Line, u.Label)
		}
		if strings.Contains(u.Title, u.ID) {
			t.Errorf("%s (line %d) still carries its own id in its title: %q",
				u.ID, u.Line, u.Title)
		}
		if strings.ContainsAny(u.Title, "⛔✅~") {
			t.Errorf("%s (line %d) still carries decoration: %q", u.ID, u.Line, u.Title)
		}
	}
	// The positive control: without it this passes over a parse that found no
	// headings at all, which is the absence B66 is about.
	if heads == 0 {
		t.Fatalf("no heading-borne ids were found, so nothing above was checked")
	}
}

// ⛔ SECTION IS THE FIELD THE SEEDER MINTS AN ID FROM, SO THE CONTRACT IS
// ASSERTED FROM OUTSIDE THE PACKAGE, WHERE A CONSUMER STANDS.
//
// The eleven ranked rows of the critical path sit under `## The critical path
// to the gate`, which carries NO ID - so `Under` is empty for all eleven and
// nothing in the report said which table they came from. An id minted from
// the bare rank `0`, `6a`, `9` is unique in this document by luck, and would
// collide silently the day a second unnumbered table appears.
//
// Both halves are asserted together on purpose: `Section` non-empty while
// `Under` is empty is the whole claim that the two are different facts, and a
// test that checked one of them would pass over a `Section` quietly widened
// into a second `Under`.
func TestTheRankedRowsSayWhichSectionTheyCameFromAndStillNameNoId(t *testing.T) {
	f := openTheBacklogDocument(t)
	p, err := record.ParseBacklogDocument(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	const critical = "the-critical-path-to-the-gate"
	var ranked int
	for _, u := range p.Unimported {
		if u.Kind != record.UnimportedRowWithoutID {
			continue
		}
		ranked++
		if u.Section != critical {
			t.Errorf("rank %q (line %d) has Section %q, want %q - a seeder keying "+
				"on the bare rank has nothing to qualify it with",
				u.Label, u.Line, u.Section, critical)
		}
		if u.Under != "" {
			t.Errorf("rank %q (line %d) has Under %q: that heading carries no id, "+
				"so Under has been widened into Section and the two fields no "+
				"longer say different things", u.Label, u.Line, u.Under)
		}
	}
	// The positive control. Without it this passes over a parse that found no
	// ranked rows at all, which is the absence the whole grain exists for.
	if ranked != 11 {
		t.Fatalf("the critical path has %d ranked rows, want 11 - either the "+
			"document moved, in which case update this number in a commit that "+
			"says so, or nothing above was checked", ranked)
	}
}

// ⛔ EVERY Unimported THIS GRAIN REPORTS CARRIES A SECTION, BECAUSE A FIELD
// PRESENT ON ONE KIND AND EMPTY ON OTHERS IS INDISTINGUISHABLE FROM ONE THAT
// WAS LOST. That is how `Title` and `Struck` were both found missing by a
// CONSUMER rather than by this package.
//
// The claim is about rig's own document, not about markdown: a row before the
// first heading of a file would have no enclosing heading and an empty Section
// would be the honest answer. rig's backlog opens with a level-1 heading, so
// there is no such row and the assertion is exact here.
func TestEveryThingTheBacklogGrainReportsSaysWhereItSits(t *testing.T) {
	f := openTheBacklogDocument(t)
	p, err := record.ParseBacklogDocument(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byKind := map[record.UnimportedKind]int{}
	for _, u := range p.Unimported {
		byKind[u.Kind]++
		if u.Section == "" {
			t.Errorf("%s %q (line %d) says nothing about where it sits", u.Kind, u.Label, u.Line)
		}
	}
	// The positive control, and it is per KIND rather than a total: a total
	// green over a parse that stopped finding one whole kind would say nothing.
	for _, k := range []record.UnimportedKind{
		record.UnimportedHeading, record.UnimportedRowWithoutID, record.UnimportedOtherTable,
	} {
		if byKind[k] == 0 {
			t.Errorf("no %s was found at all, so nothing of that kind was checked", k)
		}
	}
}

// openTheBacklogDocument reaches rig's real backlog through the same
// gitignored repo-root symlink acceptance_test.go uses, and honours the same
// off switch - this file is the external test package, so it cannot share
// that one's unexported helper.
func openTheBacklogDocument(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open("../../BACKLOG.md")
	if err != nil {
		if os.Getenv("RIG_RECORD_REQUIRE_BACKLOG") == "1" {
			t.Fatalf("RIG_RECORD_REQUIRE_BACKLOG=1 and the document is unreadable: %v", err)
		}
		t.Skipf("the real document is not here: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func onlyHeading(t *testing.T, p record.BacklogParse) record.Unimported {
	t.Helper()
	var out []record.Unimported
	for _, u := range p.Unimported {
		if u.Kind == record.UnimportedHeading {
			out = append(out, u)
		}
	}
	if len(out) != 1 {
		t.Fatalf("want exactly one heading-borne id, got %d: %+v", len(out), out)
	}
	return out[0]
}

// ⛔ A HEADING'S RECORD ASSERTED ITS OWN TITLE AS ITS BODY, WHICH IS A RECORD
// SAYING SOMETHING THE DOCUMENT DOES NOT SAY.
//
// `cmd/rigseed`'s `headingIntent` sets `body: title` and there was nothing
// else to give it: the grain reported seven fields about a heading and none of
// the prose underneath it. Under `## ⛔ B46` that prose IS the statement of the
// MVP acceptance test, which is the one thing B46 exists to carry.
//
// ⛔ AND THE BODY IS PROSE, NOT EVERY LINE - the one place this grain parts
// company with `DecisionEntry.Body`, and it is measured rather than preferred.
// A decisions heading encloses paragraphs; a backlog heading encloses TABLES.
// `## ⛔ B46` holds 65,187 bytes to the next heading and 695 of them are prose.
func TestAHeadingsBodyIsTheProseBeneathItAndNotItsTableAndNotItsTitle(t *testing.T) {
	const doc = `# b

## ⛔ B46 - THE MVP ACCEPTANCE TEST

the first paragraph, which is what this heading says.

| # | Work | Seat | State |
|---|---|---|---|
| B46a | **a child row** | record | **DONE** |

the second paragraph, on the far side of the table.

### something else
`
	p, err := record.ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u := onlyHeading(t, p)

	want := "the first paragraph, which is what this heading says.\n\n" +
		"the second paragraph, on the far side of the table."
	if u.Body != want {
		t.Errorf("Body = %q\nwant %q", u.Body, want)
	}
	if u.Body == u.Title {
		t.Errorf("Body is the Title repeated, which is the stand-in this field replaces")
	}
	// ⛔ THE ROW IS NOT LOST BY BEING LEFT OUT. It is carried at the ROW
	// grain, by this same parse, which is the whole argument for excluding it.
	if len(p.Items) != 1 || p.Items[0].ID != "B46a" {
		t.Fatalf("the row under the heading must still be an item: %+v", p.Items)
	}
}

// A heading with nothing but a table under it has no prose, and an empty Body
// is the honest answer rather than the table flattened into one.
func TestAHeadingWithNoProseUnderItHasAnEmptyBody(t *testing.T) {
	const doc = `# b

## ⛔ B46 - THE MVP ACCEPTANCE TEST

| # | Work | Seat | State |
|---|---|---|---|
| B46a | **a child row** | record | **DONE** |
`
	p, err := record.ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u := onlyHeading(t, p); u.Body != "" {
		t.Errorf("Body = %q, want empty - there is no prose under that heading", u.Body)
	}
}

// THE REAL DOCUMENT, because the two above are fixtures and rig's own backlog
// is what gets seeded.
func TestTheRealBacklogsHeadingsCarryTheProseUnderThem(t *testing.T) {
	f := openTheBacklogDocument(t)
	p, err := record.ParseBacklogDocument(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var heads int
	for _, u := range p.Unimported {
		if u.Kind != record.UnimportedHeading {
			continue
		}
		heads++
		if u.Body == "" {
			t.Errorf("%s (line %d) carries no body, and the prose under it is "+
				"carried nowhere else in this parse", u.ID, u.Line)
		}
		if u.Body == u.Title {
			t.Errorf("%s (line %d) has its own title as its body: %q", u.ID, u.Line, u.Body)
		}
		// ⛔ NO TABLE ROW IN THE BODY. Those rows are this same parse's Items
		// and Unimported; copying them in would put the row grain inside the
		// heading grain and grow without bound as the table grows.
		for _, line := range strings.Split(u.Body, "\n") {
			if strings.HasPrefix(line, "|") {
				t.Errorf("%s (line %d) carries a table row in its body: %q", u.ID, u.Line, line)
			}
		}
		// ⛔ AND NO HOLE WHERE THE TABLE WAS. Removing a table from between
		// two paragraphs leaves the blank line before it next to the blank
		// line after it, and B46d in rig's own document is exactly that shape.
		if strings.Contains(u.Body, "\n\n\n") {
			t.Errorf("%s (line %d) has a run of blank lines where a table was removed", u.ID, u.Line)
		}
	}
	// The positive control: without it this passes over a parse that found no
	// headings at all, which is the absence B66 is about.
	if heads == 0 {
		t.Fatalf("no heading-borne ids were found, so nothing above was checked")
	}
	// ⛔ AND A SECOND CONTROL, BY VALUE. Every assertion above is satisfiable
	// by a Body holding one stray word, and B46's prose is the statement of
	// the MVP acceptance test - the sentence this whole field exists for.
	var b46 string
	for _, u := range p.Unimported {
		if u.ID == "B46" {
			b46 = u.Body
		}
	}
	if !strings.Contains(b46, "THIS IS THE TEST BORIS NAMED") {
		t.Errorf("B46's body does not carry the sentence that states the acceptance "+
			"test; got %d bytes: %.200s", len(b46), b46)
	}
}

// ⛔ THE LAST HEADING'S BODY IS CLOSED BY THE END OF THE DOCUMENT AND BY
// NOTHING ELSE, AND THAT PATH WAS UNPINNED UNTIL THIS TEST.
//
// Found by mutation while the change was still uncommitted: deleting the flush
// after the scan loop left every fixture and the live document GREEN, because
// in all of them the id-bearing heading is followed by another heading that
// closes it. A body is a buffer, and a buffer nobody flushes at the end is the
// oldest defect in the trade.
func TestTheLastHeadingsBodyIsClosedByTheEndOfTheDocument(t *testing.T) {
	const doc = `# b

## ⛔ B46 - THE MVP ACCEPTANCE TEST

the prose that runs to the end of the file, with no heading after it.
`
	p, err := record.ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	u := onlyHeading(t, p)
	if want := "the prose that runs to the end of the file, with no heading after it."; u.Body != want {
		t.Errorf("Body = %q, want %q", u.Body, want)
	}
}
