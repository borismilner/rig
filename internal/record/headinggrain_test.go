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
