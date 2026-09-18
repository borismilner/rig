// The specification's parser, tested where it can be wrong.
//
// ⛔ THE FIRST TEST IS THE NEGATIVE CONTROL AND IT IS NOT CEREMONY. Every
// assertion below reads "the parse holds X because the document states X", and
// that sentence means nothing unless a document that states no heading parses
// to no entry. A harness that would pass either way reports a meaningless
// result, which this repository has recorded ten times under other names.
package record

import (
	"strings"
	"testing"
	"testing/fstest"
)

// planKeys is the key set of a parse, so an assertion reads as a set.
func planKeys(p PlanParse) []string {
	out := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		out = append(out, e.Key)
	}
	return out
}

// planEntry finds one entry by key, failing loudly rather than returning a zero
// value that every later assertion would then be about.
func planEntry(t *testing.T, p PlanParse, key string) PlanEntry {
	t.Helper()
	for _, e := range p.Entries {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("no entry keyed %q; the parse holds {%s}", key, strings.Join(planKeys(p), " "))
	return PlanEntry{}
}

// ⛔ THE NEGATIVE CONTROL.
func TestASectionThatStatesNoHeadingParsesToNoEntry(t *testing.T) {
	p, err := ParsePlanSection(strings.NewReader(
		"just prose, and a line that is not a heading.\n\n#not-a-heading\n"), "plan/07-storage.md", 7)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := strings.Join(planKeys(p), " "); got != "" {
		t.Errorf("entries = {%s}, want {} - every other test here measures the parser "+
			"rather than the document if this one is not empty", got)
	}
	if len(p.Unimported) != 0 {
		t.Errorf("unimported = %v, want none", p.Unimported)
	}
}

// ⛔ THE RULED GRAIN, AND NOTHING WIDER. plan/39, "THE GRAIN IS THE HEADING.
// RULED BY BORIS 2026-09-16 LATE": one record per `##`, `###` and `####`. A
// `#####` is OUTSIDE it and is reported rather than imported, so the thirteen
// that exist today - three of them Boris's own rulings - are visible in
// `--check` until somebody rules on them.
func TestTheGrainIsTwoToFiveAndDeeperIsReportedRatherThanImported(t *testing.T) {
	const doc = `## 39. The continuity record

top prose

### A ruling

what it says

#### The detail

what that says

##### The ruling under the detail

what THAT says. Boris widened the grain to five on 2026-09-17 because three
of his own rulings were sitting at this level and rig could not reach them.

###### Too deep to be a record

deep prose
`
	p, err := ParsePlanSection(strings.NewReader(doc), "plan/39-the-continuity-record.md", 39)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	want := "39/39-the-continuity-record 39/39-the-continuity-record/a-ruling " +
		"39/39-the-continuity-record/a-ruling/the-detail " +
		"39/39-the-continuity-record/a-ruling/the-detail/the-ruling-under-the-detail"
	if got := strings.Join(planKeys(p), " "); got != want {
		t.Errorf("entries = {%s}\n          want {%s}", got, want)
	}

	if len(p.Unimported) != 1 {
		t.Fatalf("unimported = %v, want the one heading below the grain", p.Unimported)
	}
	u := p.Unimported[0]
	if u.Kind != UnimportedHeadingTooDeep {
		t.Errorf("the deep heading is kind %q, want %q", u.Kind, UnimportedHeadingTooDeep)
	}
	if u.Label != "Too deep to be a record" || u.Line != 18 {
		t.Errorf("the report does not resolve to the heading: %+v", u)
	}
	if u.Under != "39/39-the-continuity-record/a-ruling/the-detail/the-ruling-under-the-detail" {
		t.Errorf("the deep heading names %q as its enclosure, want the `#####` above it", u.Under)
	}
}

// ⛔ THE SECTION NUMBER LEADS THE KEY BECAUSE THE SLUGS ALONE COLLIDE ACROSS 42
// FILES. "Where it sits" is a heading several sections use, and a key without
// the number would make one section's heading supersede another's - two puts
// against one id, with file order deciding which survived.
func TestTheKeyCarriesTheSectionNumberAndTheWholeAncestorChain(t *testing.T) {
	const a = "## Where it sits\n\n### The detail\n"
	const b = "## Where it sits\n\n### The detail\n"

	pa, err := ParsePlanSection(strings.NewReader(a), "plan/40-the-knowledge-sharing-section.md", 40)
	if err != nil {
		t.Fatalf("parsing 40: %v", err)
	}
	pb, err := ParsePlanSection(strings.NewReader(b), "plan/41-export-and-import-a-project-or-a-case.md", 41)
	if err != nil {
		t.Fatalf("parsing 41: %v", err)
	}

	if got := strings.Join(planKeys(pa), " "); got != "40/where-it-sits 40/where-it-sits/the-detail" {
		t.Errorf("section 40 keys = {%s}", got)
	}
	if got := strings.Join(planKeys(pb), " "); got != "41/where-it-sits 41/where-it-sits/the-detail" {
		t.Errorf("section 41 keys = {%s}", got)
	}
	for _, x := range planKeys(pa) {
		for _, y := range planKeys(pb) {
			if x == y {
				t.Errorf("%q is the key of a heading in two sections", x)
			}
		}
	}
}

// ⛔ A LEVEL-2 HEADING STATES NO PARENT AND THE FILE IS NOT ONE. A section
// file's `##` headings are siblings in markdown, including the first; making
// the first the parent of the rest would be this parser inventing a hierarchy
// the document does not write. The section number travels as a FIELD instead.
func TestAPartOfIsTheEnclosingHeadingAndALevelTwoStatesNone(t *testing.T) {
	const doc = `## First

### Under the first

#### Under that

## Second

### Under the second
`
	p, err := ParsePlanSection(strings.NewReader(doc), "plan/05-architecture.md", 5)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	for key, want := range map[string]string{
		"5/first":                            "",
		"5/second":                           "",
		"5/first/under-the-first":            "5/first",
		"5/first/under-the-first/under-that": "5/first/under-the-first",
		"5/second/under-the-second":          "5/second",
	} {
		if got := planEntry(t, p, key).PartOf; got != want {
			t.Errorf("%s is part-of %q, want %q", key, got, want)
		}
	}
}

// ⛔ A HEADING INSIDE A FENCED BLOCK IS NOT A HEADING, AND THIS IS THE ONE
// CORRECTNESS FIX IN THIS PARSER. `plan/` has fenced blocks and none of them
// holds a `#` line today, so the rule ships at zero hits; `COORDINATION.md:906`
// is a `#` shell comment inside a ```sh fence, so the defect is measured rather
// than imagined. A parser that gained the rule after the first shell comment
// appeared would already have imported a comment as a requirement.
func TestAHeadingInsideAFencedBlockIsNotAHeading(t *testing.T) {
	const doc = "## Configuration\n\n```sh\n# mutate and test in there\n## and this is not a section\n```\n\n### After the fence\n"

	p, err := ParsePlanSection(strings.NewReader(doc), "plan/06-configuration.md", 6)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := strings.Join(planKeys(p), " "); got != "6/configuration 6/configuration/after-the-fence" {
		t.Errorf("entries = {%s}; a fenced comment was read as a heading", got)
	}
	if len(p.Unimported) != 0 {
		t.Errorf("unimported = %v; a line inside a fence is not an irregular heading, "+
			"it is not a heading at all", p.Unimported)
	}

	// POSITIVE CONTROL: the very same text outside a fence IS read, so the test
	// above is measuring the fence and not the shape of the line.
	loose, err := ParsePlanSection(strings.NewReader(
		"## Configuration\n\n## and this is not a section\n"), "plan/06-configuration.md", 6)
	if err != nil {
		t.Fatalf("parsing the control: %v", err)
	}
	if len(loose.Entries) != 2 {
		t.Errorf("the control parsed %d entries, want 2 - the assertion above is not "+
			"about fences if an unfenced heading is also missed", len(loose.Entries))
	}
}

// ⛔ ONLY A FENCE OF THE SAME CHARACTER, AT LEAST AS LONG, WITH NOTHING AFTER
// IT, CLOSES A BLOCK. That is CommonMark's rule and it is what lets a
// four-backtick block quote a three-backtick one - which is exactly how a
// document about markdown gets written.
func TestAShorterFenceInsideALongerOneDoesNotCloseIt(t *testing.T) {
	const doc = "## Toasts\n\n````md\n```\n## not a heading\n```\n````\n\n### After\n"

	p, err := ParsePlanSection(strings.NewReader(doc), "plan/12-toasts.md", 12)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := strings.Join(planKeys(p), " "); got != "12/toasts 12/toasts/after" {
		t.Errorf("entries = {%s}; the inner fence closed the outer block", got)
	}
}

// ⛔ AN UNCLOSED FENCE IS AN ERROR AND NOT A SHRUG. Everything after it was read
// as code, so the parse is missing an unknown number of entries and `missing`
// would name them all without saying why. Declining to answer is the only
// honest arm of the pass-versus-no-run class.
func TestAnUnclosedFenceIsRefusedRatherThanAnswered(t *testing.T) {
	_, err := ParsePlanSection(strings.NewReader(
		"## Storage\n\n```sh\nnever closed\n\n## a heading nobody will see\n"),
		"plan/07-storage.md", 7)
	if err == nil {
		t.Fatal("a file with an unclosed fence parsed as though it were complete")
	}
}

// ⛔ A COLLISION IS REPORTED AND THE FIRST READ WINS. Two puts against one id in
// one run supersede each other, so which content survived would be decided by
// the order the files were read in - a silent wrong answer nobody could account
// for later.
func TestTwoHeadingsKeyingTheSameWayAreReportedAndWrittenOnce(t *testing.T) {
	p, err := ParsePlanDir(fstest.MapFS{
		"07-storage.md": &fstest.MapFile{Data: []byte("## The one line\n\n## The one line\n")},
	})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := strings.Join(planKeys(p), " "); got != "7/the-one-line" {
		t.Errorf("entries = {%s}, want the first read only", got)
	}
	if len(p.Unimported) != 1 || p.Unimported[0].Kind != UnimportedDuplicateKey {
		t.Fatalf("unimported = %v, want one duplicate-key report", p.Unimported)
	}
	if p.Unimported[0].ID != "7/the-one-line" {
		t.Errorf("the report names %q, want the key that collided", p.Unimported[0].ID)
	}
}

// A heading that is nothing but decoration has no title to key on, and is
// reported rather than keyed on an empty string.
func TestAHeadingWithNoTitleLeftIsReportedRatherThanKeyed(t *testing.T) {
	p, err := ParsePlanSection(strings.NewReader("## ⛔ **~~ ~~**\n\n## Real\n"),
		"plan/03-the-maximals-made-measurable.md", 3)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := strings.Join(planKeys(p), " "); got != "3/real" {
		t.Errorf("entries = {%s}, want the one heading with a title", got)
	}
	if len(p.Unimported) != 1 || p.Unimported[0].Kind != UnimportedHeadingNoTitle {
		t.Fatalf("unimported = %v, want one no-title report", p.Unimported)
	}
}

// ⛔ A FILE THAT IS NOT A SECTION FILE IS AN ERROR AND NOT A SKIP. plansplit
// names every file in that directory, so one that does not match means the
// caller pointed somewhere else or the generator's contract broke. A skip would
// import 41 of 42 sections and report a clean run.
func TestAFileThatIsNotASectionFileStopsTheParseRatherThanBeingSkipped(t *testing.T) {
	_, err := ParsePlanDir(fstest.MapFS{
		"07-storage.md": &fstest.MapFile{Data: []byte("## Storage\n")},
		"README.md":     &fstest.MapFile{Data: []byte("## Not a section\n")},
	})
	if err == nil {
		t.Fatal("a directory holding a file plansplit did not generate parsed clean")
	}
	if !strings.Contains(err.Error(), "README.md") {
		t.Errorf("the error does not name the file: %v", err)
	}
}

// ⛔ THE SECTION NUMBER COMES FROM THE FILE NAME AND NOT FROM THE HEADING, and
// plansplit's own rule - a file's name must match its heading - is what makes
// the name the checked half of that pair.
func TestTheSectionNumberIsReadFromTheFileName(t *testing.T) {
	p, err := ParsePlanDir(fstest.MapFS{
		"09-built-for-agents.md": &fstest.MapFile{Data: []byte("## 9. Built for agents\n")},
	})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	e := planEntry(t, p, "9/9-built-for-agents")
	if e.Section != 9 {
		t.Errorf("section = %d, want 9", e.Section)
	}
	if e.File != "09-built-for-agents.md" {
		t.Errorf("file = %q", e.File)
	}
	if e.Level != 2 || e.Line != 1 {
		t.Errorf("level/line = %d/%d, want 2/1", e.Level, e.Line)
	}
}

// The body is the prose under the heading, up to the next heading of ANY level,
// so a parent's body is its own preamble and not its children's text.
func TestTheBodyStopsAtTheNextHeadingOfAnyLevel(t *testing.T) {
	p, err := ParsePlanSection(strings.NewReader(
		"## Parent\n\nthe parent's own prose\n\n### Child\n\nthe child's prose\n"),
		"plan/05-architecture.md", 5)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := planEntry(t, p, "5/parent").Body; got != "the parent's own prose" {
		t.Errorf("parent body = %q", got)
	}
	if got := planEntry(t, p, "5/parent/child").Body; got != "the child's prose" {
		t.Errorf("child body = %q", got)
	}
}

// ⛔ EVERY ENTRY IN A FILE CARRIES THAT FILE'S SECTION TITLE, INCLUDING THE
// SECTION HEADING ITSELF. The seeder tags a requirement by its section, and a
// tag is `FriendlyTag` of the TEXT - a slug has thrown away the punctuation the
// clause cut needs, which is why this is not read back off the file name.
func TestEveryEntryCarriesItsSectionsOwnTitle(t *testing.T) {
	p, err := ParsePlanDir(fstest.MapFS{
		"40-the-knowledge-sharing-section.md": &fstest.MapFile{Data: []byte(
			"## 40. The knowledge-sharing section\n\n### What it holds\n\n#### The shape\n")},
		"22-tech-stack.md": &fstest.MapFile{Data: []byte("## 22. Tech stack\n\n### Go\n")},
	})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	for id, want := range map[string]string{
		"40/40-the-knowledge-sharing-section":                         "40. The knowledge-sharing section",
		"40/40-the-knowledge-sharing-section/what-it-holds":           "40. The knowledge-sharing section",
		"40/40-the-knowledge-sharing-section/what-it-holds/the-shape": "40. The knowledge-sharing section",
		"22/22-tech-stack":    "22. Tech stack",
		"22/22-tech-stack/go": "22. Tech stack",
	} {
		if got := planEntry(t, p, id).SectionTitle; got != want {
			t.Errorf("%s: SectionTitle = %q, want %q - a child tagged by another "+
				"file's section groups two sections as one", id, got, want)
		}
	}
}

// A file whose first heading is BELOW the grain still states the section's
// title, and the entries under it must carry it: the first heading is the
// section's own by plansplit's contract, whether or not this parser imports it.
func TestTheSectionTitleIsTheFirstHeadingEvenWhereItIsNotAnEntry(t *testing.T) {
	p, err := ParsePlanSection(strings.NewReader(
		"# 30. Name\n\n## Why rig\n"), "30-name.md", 30)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got := planEntry(t, p, "30/why-rig").SectionTitle; got != "30. Name" {
		t.Errorf("SectionTitle = %q, want %q", got, "30. Name")
	}
}
