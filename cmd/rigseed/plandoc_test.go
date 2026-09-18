// The specification half of the seeder, tested where it can be wrong.
//
// ⛔ THE FIRST TEST IS THE NEGATIVE CONTROL. Every assertion below says "the
// plan holds X because the specification states X", and that is worthless
// unless a plan built from a specification that states nothing holds nothing.
package main

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/boris-milner/rig/internal/record"
)

// planParseOf is one section file parsed, spelled once.
func planParseOf(t *testing.T, name, body string) record.PlanParse {
	t.Helper()
	pp, err := record.ParsePlanDir(fstest.MapFS{name: &fstest.MapFile{Data: []byte(body)}})
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return pp
}

// oneRow is the backlog every test here needs and none of them is about: the
// seeder refuses a zero-row parse, so something has to be there.
func oneRow() record.BacklogParse {
	return record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "a row"}}}
}

// ⛔ THE NEGATIVE CONTROL.
func TestASpecificationThatStatesNothingPlansNoRequirement(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, oneRow(), record.DecisionParse{}, record.PlanParse{})

	for _, in := range p.want {
		if in.kind == record.KindRequirement {
			t.Fatalf("%s was planned as a requirement from an empty plan parse, so every "+
				"other test in this file is measuring the seeder rather than the "+
				"specification", in.id)
		}
	}
}

// ⛔ `status` IS OMITTED, AND IT IS THE FIELD THAT ALREADY BIT ONCE. B66 records
// a heading-borne backlog id seeded with no status, which imported B46 into
// invisibility. The backlog was MENDED because its strikethrough is a
// machine-readable closure mark; `plan/` has no such mark, so a status written
// here would be a fact this seeder invented rather than one it read.
func TestAPlanHeadingCarriesNoStatusAndNoDate(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, oneRow(), record.DecisionParse{},
		planParseOf(t, "07-storage.md", "## 7. Storage\n\n2026-09-16 prose with a date in it\n"))

	in := intentFor(t, p, "7/7-storage")
	for _, banned := range []string{fieldStatus, "date", "doc-date", "priority", "supersedes"} {
		if v, ok := in.fields[banned]; ok {
			t.Errorf("a plan heading carries %s=%q; the document states no such thing and "+
				"a seat that writes one has invented a fact", banned, v)
		}
	}

	want := map[string]string{
		"title":             "7. Storage",
		"source":            "plan/07-storage.md",
		"description_short": "2026-09-16 prose with a date in it",
		fieldSection:        "7",
		fieldLevel:          "2",
		fieldDocLine:        "1",
		fieldTags:           `["section:storage"]`,
	}
	for name, v := range want {
		if in.fields[name] != v {
			t.Errorf("%s = %q, want %q", name, in.fields[name], v)
		}
	}
	if len(in.fields) != len(want) {
		t.Errorf("fields = %v, want exactly %v - a field nobody can account for is a "+
			"field a later reader has to work out the meaning of", in.fields, want)
	}
	if in.kind != record.KindRequirement {
		t.Errorf("kind = %q, want %q", in.kind, record.KindRequirement)
	}
	if in.grain != grainPlanHeading {
		t.Errorf("grain = %q, want %q", in.grain, grainPlanHeading)
	}
}

// ⛔ `source` RESOLVES FROM THE REPOSITORY ROOT AND NOT FROM THE PARSER'S OWN
// ROOT. The parser is handed the directory as an fs.FS so it only ever sees a
// base name; a `source` carrying that would be a file:line that is precise and
// does not resolve, which is worse than none at all.
func TestTheSourceFieldIsThePathAReaderWouldType(t *testing.T) {
	o := options{
		project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md",
		planDir: "some/where/plan",
	}
	p := planFor(o, oneRow(), record.DecisionParse{},
		planParseOf(t, "22-tech-stack.md", "## 22. Tech stack\n"))

	if got := intentFor(t, p, "22/22-tech-stack").fields["source"]; got != "some/where/plan/22-tech-stack.md" {
		t.Errorf("source = %q, want the joined path", got)
	}
}

// The edge the document states travels to the store through the same
// `linkPartOf` every other edge uses, so `compareEdges` covers it for free.
func TestAPlanHeadingsParentIsTheHeadingAboveIt(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, oneRow(), record.DecisionParse{},
		planParseOf(t, "05-architecture.md", "## 5. Architecture\n\n### The dumb pipe\n\n#### What it costs\n"))

	for id, want := range map[string]string{
		"5/5-architecture":                             "",
		"5/5-architecture/the-dumb-pipe":               "5/5-architecture",
		"5/5-architecture/the-dumb-pipe/what-it-costs": "5/5-architecture/the-dumb-pipe",
	} {
		if got := intentFor(t, p, id).partOf; got != want {
			t.Errorf("%s is part-of %q, want %q", id, got, want)
		}
	}
	if got := strings.Join(p.orphaned, " "); got != "" {
		t.Errorf("orphaned = {%s}, want {} - every parent is a heading this run writes", got)
	}
}

// ⛔ THE THIRTEEN `#####` HEADINGS ARE REPORTED WITH THEIR FILE AND LINE, NEVER
// IMPORTED. The bound is Boris's and a heading below it is REPORTED rather than
// dropped, which is section 39's migration rule.
//
// ⛔ THE BOUND MOVED ONCE, AND THIS TEST MOVED WITH IT. Until 2026-09-17 the
// grain was 2-4 and this fixture used `#####`: thirteen real `#####` headings
// were being reported, THREE OF THEM HIS OWN RULINGS, including section 39's
// "fill everything in first". Shown that, he widened the grain to 5. The fixture
// is now `######` so the test still has something outside the bound to catch -
// a test whose case has been legalised has stopped being able to fail.
func TestAHeadingBelowTheRuledGrainIsReportedWithItsFileAndLine(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, oneRow(), record.DecisionParse{},
		planParseOf(t, "39-the-continuity-record.md",
			"## 39. The continuity record\n\n###### DEEPER THAN THE RULED GRAIN.\n"))

	if has(ids(p.want), "39/39-the-continuity-record/deeper-than-the-ruled-grain") {
		t.Fatal("a heading below the ruled grain was IMPORTED; the grain is Boris's")
	}
	if len(p.unimported) != 1 {
		t.Fatalf("unimported = %v, want the one heading below the grain", p.unimported)
	}
	u := p.unimported[0]
	if u.Kind != record.UnimportedHeadingTooDeep {
		t.Errorf("kind = %q, want %q", u.Kind, record.UnimportedHeadingTooDeep)
	}
	if u.doc != "plan/39-the-continuity-record.md" || u.Line != 3 {
		t.Errorf("the report resolves to %s:%d, want plan/39-the-continuity-record.md:3", u.doc, u.Line)
	}
}

// ⛔ THREE DOCUMENTS CANNOT SILENTLY CLAIM ONE ID. The store keys on id alone,
// so two puts against one id in one run supersede each other and document order
// decides the content - a silent wrong answer.
func TestAnIdTheBacklogAndThePlanBothStateIsWrittenOnceAndReported(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o,
		record.BacklogParse{Items: []record.BacklogItem{{ID: "7/7-storage", Title: "a row keyed like a plan heading"}}},
		record.DecisionParse{},
		planParseOf(t, "07-storage.md", "## 7. Storage\n"))

	n := 0
	for _, in := range p.want {
		if in.id == "7/7-storage" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the id is planned %d times; two puts against one id supersede each other", n)
	}
	if got := strings.Join(p.collided, " "); got != "7/7-storage" {
		t.Errorf("collided = {%s}, want the id both documents state", got)
	}
}

// ⛔ THE LIVE SPECIFICATION, WHICH IS THE ONLY PLACE THE REAL NUMBERS ARE.
//
// It SKIPS rather than fails when `plan/` is not reachable from here, the same
// way the backlog pin does, because a CI runner may not have the tree laid out
// this way. ⛔ THAT MAKES A SKIP READ AS A PASS, so the skip says so out loud.
//
// ⛔ AND IT IS A SECOND INSTRUMENT ON PURPOSE. The spec-gap seat counted the
// headings with a shell census on 2026-09-17 and got 56 / 183 / 90 = 329 with
// 13 below the grain. This parser is an independent derivation of the same
// question, and two instruments agreeing is worth more than either alone.
func TestRigsOwnPlanParsesAtTheRuledGrain(t *testing.T) {
	const live = "../../plan"
	if _, err := os.Stat(live); err != nil {
		t.Skipf("rig's own plan/ is not at %s from here: %v\n"+
			"⛔ THIS IS A SKIP, NOT A PASS - the numbers below are the only place the "+
			"live specification is measured.", live, err)
	}
	pp, err := record.ParsePlanDir(os.DirFS(live))
	if err != nil {
		t.Fatalf("parsing rig's own plan/: %v", err)
	}

	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, oneRow(), record.DecisionParse{}, pp)

	byLevel := map[string]int{}
	sections := map[string]bool{}
	for _, in := range p.want {
		if in.kind != record.KindRequirement {
			continue
		}
		byLevel[in.fields[fieldLevel]]++
		sections[in.fields[fieldSection]] = true
	}

	// ⛔ EVERY NUMBER IS PRINTED WHETHER IT PASSES OR FAILS. A census that only
	// speaks when it disagrees cannot be told from one that never ran, and
	// these are the numbers the lead asked to see as a set difference.
	t.Logf("plan/ at the ruled grain: %d requirement records over %d section files "+
		"(## %d, ### %d, #### %d, ##### %d), %d headings below the grain",
		byLevel["2"]+byLevel["3"]+byLevel["4"]+byLevel["5"], len(sections),
		byLevel["2"], byLevel["3"], byLevel["4"], byLevel["5"], len(pp.Unimported))

	if got := byLevel["2"] + byLevel["3"] + byLevel["4"] + byLevel["5"]; got != len(pp.Entries) {
		t.Errorf("%d entries parsed and %d planned; every entry becomes a record", len(pp.Entries), got)
	}
	if len(sections) != 42 {
		t.Errorf("%d section files carried a heading, want 42", len(sections))
	}
	for _, u := range pp.Unimported {
		if u.Kind != record.UnimportedHeadingTooDeep {
			t.Errorf("rig's own plan/ has an unimported heading of kind %q at line %d: %q",
				u.Kind, u.Line, u.Label)
		}
	}

	// The keys are unique across 42 files, which is what the section-number
	// prefix exists for, and it is asserted rather than assumed.
	seen := map[string]string{}
	for _, in := range p.want {
		if in.kind != record.KindRequirement {
			continue
		}
		if first, dup := seen[in.id]; dup {
			t.Errorf("%s is keyed by two headings (%s and %s)", in.id, first, in.fields["source"])
		}
		seen[in.id] = in.fields["source"]
	}
	if got := strings.Join(p.collided, " "); got != "" {
		t.Errorf("collided = {%s}, want {} - no plan key may collide with a backlog id "+
			"or a decision key", got)
	}

	// ⛔ EVERY STATED PARENT IS A RECORD THIS RUN WRITES, or the store refuses
	// the link and a seeding run aborts half-written.
	var missing []string
	for _, in := range p.want {
		if in.kind == record.KindRequirement && in.partOf != "" && !has(ids(p.want), in.partOf) {
			missing = append(missing, in.id+" -> "+in.partOf)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("part-of towards an id the run does not write: %v", missing)
	}
}

// ⛔ AN UNIMPORTED SET THAT IS DELIBERATE AND STILL OPEN MUST SAY SO. The lead
// ruled 2026-09-17: ship at the ruled grain, report the thirteen `#####`
// headings with their paths, let `--check` exit 2 over them until somebody
// rules - and make the line say WHY, so it reads as a question waiting on Boris
// rather than as something broken.
func TestTheUnimportedBlockSaysWhyEachKindIsThere(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, record.BacklogParse{
		Items:      []record.BacklogItem{{ID: "B1", Title: "a row"}},
		Unimported: []record.Unimported{{Kind: record.UnimportedIrregularID, ID: "B60-2", Line: 400}},
	}, record.DecisionParse{},
		planParseOf(t, "39-the-continuity-record.md",
			"## 39. The continuity record\n\n###### DEEPER THAN THE RULED GRAIN.\n"))

	var b bytes.Buffer
	d(p).report(&b, o, p, "production")
	out := b.String()

	for _, want := range []string{
		"heading-too-deep", "BELOW the grain Boris ruled",
		"THREE ARE HIS OWN RULINGS", "plan/39-the-continuity-record.md:3",
		"irregular-id", "id-SHAPED that is not a legal id",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not carry %q:\n%s", want, out)
		}
	}

	// ⛔ AND IT STAYS AN OPEN DIVERGENCE. A `#####` heading is not a ruled
	// exclusion the way an other-table id is - nobody has ruled on it yet, and
	// a check that stopped failing would be the ruling arriving by default.
	if !has(d(p).nonEmpty(), "unimported") {
		t.Error("a heading below the grain stopped failing the check, which is a ruling " +
			"arriving by default")
	}
}

// ⛔ EVERY KIND internal/record DEFINES HAS A STATEMENT HERE, or the report
// prints a kind a reader has to go and look up. The default arm says exactly
// that out loud rather than printing nothing.
func TestEveryUnimportedKindHasAReason(t *testing.T) {
	for _, k := range []record.UnimportedKind{
		record.UnimportedHeading, record.UnimportedRowWithoutID,
		record.UnimportedIrregularID, record.UnimportedOtherTable,
		record.UnimportedHeadingTooDeep, record.UnimportedHeadingNoTitle,
		record.UnimportedDuplicateKey,
	} {
		// `heading` and `other-table` never reach the open block - one is
		// imported and one is ruled - so they are allowed the default.
		if k == record.UnimportedHeading || k == record.UnimportedOtherTable {
			continue
		}
		if strings.Contains(unimportedReason(k), "nothing here was taught what it means") {
			t.Errorf("%q reaches the open block with no statement of what it means", k)
		}
	}
}

// ⛔ THE NEGATIVE CONTROL FOR THE SHORT DESCRIPTION. Every case below says
// "the field carries what the document wrote"; that is worth nothing unless a
// body the rule cannot read yields NO field rather than a wrong one.
func TestABodyWithNoProseToReadCarriesNoShortDescription(t *testing.T) {
	for name, body := range map[string]string{
		"empty":  "",
		"blanks": "\n\n   \n",
		"table":  "| Call | What it does |\n|---|---|\n| `get` | reads |",
		"fence":  "```sh\nrig record get B1\n```",
		"tilde":  "~~~\nnot prose\n~~~",
	} {
		if got := docLead(body); got != "" {
			t.Errorf("%s: docLead = %q, want no field at all - a description the "+
				"document does not state is one this seeder invented", name, got)
		}
	}
}

// ⛔ `description_short` IS THE DOCUMENT'S OWN LEAD AND NEVER A CUT OF THE
// TITLE. GAP 1's defect was `description_short` seeded as the title, which is
// present, non-empty, passes every check and says nothing. On this grain a cut
// of the title would restate it: 116 of the 379 headings SHOUT and 103 run past
// 70 characters.
func TestTheShortDescriptionIsWhatTheDocumentWrote(t *testing.T) {
	for _, c := range []struct{ name, body, want string }{{
		name: "a bold lead that is a whole sentence is the lead",
		body: "**The rule binds NAMED estates.** Every test in this repository and " +
			"every reproduction recipe starts an anonymous estate.",
		want: "The rule binds NAMED estates.",
	}, {
		name: "a one-word bold lead fills on to the sentence beside it",
		body: "**Presence.** What AgentBox does today, kept because it works.",
		want: "Presence. What AgentBox does today, kept because it works.",
	}, {
		name: "a label alone in its paragraph reads the block it introduces",
		body: "**BORIS, 2026-09-17, verbatim:**\n\n> *\"I want everything persisted.\"*",
		want: `"I want everything persisted."`,
	}, {
		name: "a label with its statement beside it is left whole",
		body: "**Measured:** today they are the same thing.",
		want: "Measured: today they are the same thing.",
	}, {
		name: "a blockquote is his words, not the seeder's",
		body: "> *\"MVP is being able to use `rig` to work on `rig`.\"*",
		want: `"MVP is being able to use rig to work on rig."`,
	}, {
		name: "a list yields its FIRST item and not the whole list",
		body: "- **Name:** rig. The CLI.\n- **Wire:** protobuf over a unix socket.",
		want: "Name: rig. The CLI.",
	}, {
		name: "a numbered list loses its marker, which is not part of what it says",
		body: "1. The daemon is the only writer.\n2. Everything else is a client.",
		want: "The daemon is the only writer.",
	}, {
		name: "prose with no terminal punctuation is offered whole",
		body: "2026-09-16 prose with a date in it",
		want: "2026-09-16 prose with a date in it",
	}, {
		name: "a date inside a sentence does not end it",
		body: "Ruled on 2026-09-17. The second sentence.",
		want: "Ruled on 2026-09-17. The second sentence.",
	}, {
		name: "a first sentence past the budget is cut on a word boundary",
		body: "The supervising process sits in a session the harness never shares " +
			"with any of the tool calls it makes on a seat's behalf.",
		want: "The supervising process sits in a session the harness never shares with...",
	}} {
		t.Run(c.name, func(t *testing.T) {
			if got := docLead(c.body); got != c.want {
				t.Errorf("docLead = %q, want %q", got, c.want)
			}
		})
	}
}

// ⛔ THE BUDGET IS A BUDGET AND NOT A HOPE. `fillSentences` adds sentences
// while they fit, so the bound has to hold over the real document rather than
// over the cases above - a rule that overruns on one heading in 379 writes a
// field no list can lay out.
func TestNoShortDescriptionOverrunsTheBudget(t *testing.T) {
	pp := planOfThisRepo(t)
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}

	// ⛔ AND THE FIELD HAS TO BE THERE, over the real document. Every bound
	// below is satisfied by writing nothing at all, so a rule that quietly
	// stopped reading would pass every one of them. 341 of the 379 headings
	// carry a lead today; the floor is set well under that so ordinary
	// editing of `plan/` does not trip it, and a collapse does.
	const floor = 300

	held := 0
	const ellipsis = 3
	for _, e := range pp.Entries {
		short := planEntryIntent(o, e).fields["description_short"]
		if short != "" {
			held++
		}
		if n := len([]rune(short)); n > shortWidth+ellipsis {
			t.Errorf("%s: description_short is %d runes, over the %d budget: %q",
				e.Key, n, shortWidth, short)
		}
		if short == e.Title {
			t.Errorf("%s: description_short is a second copy of the title, which is "+
				"GAP 1's defect restated", e.Key)
		}
	}
	if held < floor {
		t.Errorf("%d of %d requirement records carry a description_short, under the "+
			"floor of %d - the rule has stopped reading the document",
			held, len(pp.Entries), floor)
	}
}

// ⛔ THE SECTION TAG DROPS THE SECTION NUMBER, and the bar is Boris's:
// "the tags as well as the content and the titles must be useful and friendly".
// `## 40. The knowledge-sharing section` tagged `section:40-the-knowledge-sharing-section`
// carries the id into the one field the id was ruled out of.
func TestTheSectionTagCarriesNoSectionNumber(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}
	p := planFor(o, oneRow(), record.DecisionParse{},
		planParseOf(t, "40-the-knowledge-sharing-section.md",
			"## 40. The knowledge-sharing section\n\nprose\n"))

	in := intentFor(t, p, "40/40-the-knowledge-sharing-section")
	if got, want := in.fields[fieldTags], `["section:knowledge-sharing-section"]`; got != want {
		t.Errorf("tags = %q, want %q", got, want)
	}
}

// ⛔ EVERY REQUIREMENT RECORD CARRIES A SECTION TAG, over the real document.
// Until 2026-09-18 this grain carried NO tags at all, so the one document the
// grouping ruling was about was the only one without it.
func TestEveryRequirementCarriesItsSectionTag(t *testing.T) {
	pp := planOfThisRepo(t)
	o := options{project: "rig", backlog: "BACKLOG.md", decisions: "DECISIONS.md", planDir: "plan"}

	sections := map[string]bool{}
	for _, e := range pp.Entries {
		tags := record.DecodeTags(planEntryIntent(o, e).fields[fieldTags])
		if len(tags) != 1 || !strings.HasPrefix(tags[0], tagSection) {
			t.Fatalf("%s: tags = %v, want exactly one %s tag", e.Key, tags, tagSection)
		}
		sections[tags[0]] = true
	}
	if len(sections) != len(planFiles(t)) {
		t.Errorf("%d distinct section tags over %d section files - a file whose tag "+
			"collides with another's groups two sections as one",
			len(sections), len(planFiles(t)))
	}
}

// planOfThisRepo is the real `plan/` directory, because the rules above are
// about a document that exists and a fixture cannot go stale with it.
func planOfThisRepo(t *testing.T) record.PlanParse {
	t.Helper()
	pp, err := record.ParsePlanDir(os.DirFS("../../plan"))
	if err != nil {
		t.Fatalf("parsing the repository's own plan directory: %v", err)
	}
	if len(pp.Entries) == 0 {
		t.Fatal("the repository's own plan directory parsed to no entries at all")
	}
	return pp
}

// planFiles is every section file on disk.
func planFiles(t *testing.T) []string {
	t.Helper()
	names, err := os.ReadDir("../../plan")
	if err != nil {
		t.Fatalf("reading the plan directory: %v", err)
	}
	var out []string
	for _, n := range names {
		out = append(out, n.Name())
	}
	return out
}
