package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func brief(mut ...func(*Brief)) Brief {
	b := Brief{
		Project: "rig",
		Kind:    "project",
		Title:   "the swiss knife and the record under it",
		Status:  "active",
		Semver:  "0.4.1",
		NextUp: []BriefItem{
			{ID: "01927-a", Title: "wire the record verbs", Owner: "team-lead", Priority: "1"},
			{ID: "01927-b", Title: "the CLI surface", Owner: "cli", Priority: "2"},
		},
		Notes: []BriefNote{{
			ID:       "01927-n",
			Priority: "1",
			Body:     "do not delete the logbook until slice 5 ships",
			Prov:     Provenance{Session: "boris", Seat: "boris", CreatedAt: now.Add(-time.Hour)},
		}},
	}
	for _, m := range mut {
		m(&b)
	}
	return b
}

// ---- ⛔ a `blocks` cycle is DETECTED AND REPORTED, NEVER RESOLVED ----------

// ⛔ SECTION 39 RULES THIS AND THE RULING REACHES THIS RENDERER RATHER THAN
// STOPPING AT THE STORE.
//
// "detected - the derivation terminates, always. reported - it appears in the
// brief as a BLOCKED CONDITION, naming the items in it. ordered around - every
// item not in the cycle keeps its place. never resolved - rig does not pick an
// edge to break."
//
// The failure this exists for is not a wrong order: it is a brief that prints
// a next-up list with a silent hole in it, which every arriving session then
// works from.
func TestACycleIsReportedByNameAndTheOtherItemsKeepTheirPlace(t *testing.T) {
	got := briefText(brief(func(b *Brief) {
		b.Blocked = []BriefCycle{{Items: []string{"01927-x", "01927-y", "01927-z"}}}
	}), now)

	for _, item := range []string{"01927-x", "01927-y", "01927-z"} {
		if !strings.Contains(got, item) {
			t.Errorf("the cycle report does not name %q. Naming the items in "+
				"it IS the requirement - a warning that a cycle exists is "+
				"not actionable by anybody:\n%s", item, got)
		}
	}
	// Every item outside the cycle keeps its place, so the next-up list is
	// still printed rather than suppressed.
	for _, item := range []string{"01927-a", "01927-b"} {
		if !strings.Contains(got, item) {
			t.Errorf("a cycle suppressed the next-up list, and %q is outside "+
				"it:\n%s", item, got)
		}
	}
	if !strings.Contains(got, "does not pick an edge") {
		t.Errorf("the report does not say rig refuses to resolve the cycle. "+
			"Which dependency is the wrong one is a judgement about the work, "+
			"which is non-goal 1:\n%s", got)
	}
}

// THE CYCLE IS PRINTED AS A CYCLE. A flat list reads as a chain, and a reader
// has to be told that the last item points back at the first.
func TestTheCycleIsPrintedAsClosingRatherThanAsAChain(t *testing.T) {
	got := briefBlockedSection([]BriefCycle{{Items: []string{"a", "b"}}})

	if strings.Count(got, "a") < 2 {
		t.Errorf("the first item does not reappear at the end, so the cycle "+
			"reads as a chain that stops:\n%s", got)
	}
	if !strings.Contains(got, "->") {
		t.Errorf("the cycle is printed without its direction:\n%s", got)
	}
}

// THE BLOCKED CONDITION COMES BEFORE THE LIST IT AFFECTS. A reader who stops
// after the next-up table must not be the one who misses that the ordering has
// a hole in it.
func TestTheBlockedConditionIsPrintedBeforeTheNextUpList(t *testing.T) {
	got := briefText(brief(func(b *Brief) {
		b.Blocked = []BriefCycle{{Items: []string{"01927-x", "01927-y"}}}
	}), now)

	blocked := strings.Index(got, "BLOCKED CONDITION")
	next := strings.Index(got, "NEXT UP")
	if blocked < 0 || next < 0 {
		t.Fatalf("a brief with a cycle is missing one of the two sections:\n%s", got)
	}
	if blocked > next {
		t.Errorf("the next-up list is printed before the condition that puts "+
			"a hole in it:\n%s", got)
	}
}

// AND A BRIEF WITH NO CYCLE SAYS NOTHING ABOUT CYCLES. This is the one section
// allowed to be silent: a "no cycles" line on every brief trains a reader to
// skip the place the real one appears.
func TestABriefWithNoCycleHasNoBlockedSectionAtAll(t *testing.T) {
	got := briefText(brief(), now)

	if strings.Contains(got, "BLOCKED") {
		t.Errorf("a clean brief carries a blocked section, which teaches a "+
			"reader to skip the line a real cycle appears on:\n%s", got)
	}
}

// A CYCLE WHOSE ITEMS DID NOT ARRIVE IS NOT A DECORATIVE WARNING. A report
// that reports nothing is worse than no report: it tells a reader something is
// wrong and gives them nowhere to look.
func TestACycleWithNoItemsSaysTheItemsAreMissing(t *testing.T) {
	got := briefBlockedSection([]BriefCycle{{}})

	if !strings.Contains(got, "did not reach") {
		t.Errorf("an empty cycle rendered as a bare warning:\n%s", got)
	}
}

// ---- a case is not a project ----------------------------------------------

// ⛔ A CASE HAS NO SEMVER AND MUST NOT PRINT ONE. Section 39: "`semver` - NO -
// a case does not ship, so it has no version to advance." Rendering the empty
// string as "v" or "v0.0.0" invents a version for a thing that has none, and
// `project.brief` serves both containers through one renderer, which is
// exactly where that happens.
func TestACaseRendersNoVersionAtAll(t *testing.T) {
	got := briefHeading(brief(func(b *Brief) {
		b.Kind = "case"
		b.Status = "open"
		b.Semver = ""
	}))

	if strings.Contains(got, "v") && strings.Contains(got, " v") {
		t.Errorf("a case printed a version:\n%s", got)
	}
	if !strings.Contains(got, "case") {
		t.Errorf("the heading does not say which container this is, and the "+
			"status vocabulary depends on it - `open` is a case's and "+
			"`active` is a project's:\n%s", got)
	}
	// And the project still prints one, or the assertion above would pass
	// against a renderer that dropped the version for everybody.
	if p := briefHeading(brief()); !strings.Contains(p, "v0.4.1") {
		t.Errorf("a project's semver is not printed either, so the case "+
			"assertion above proves nothing:\n%s", p)
	}
}

// `semver` IS PRESENT AND EMPTY IN THE OBJECT rather than omitted. A consumer
// branching on its absence has to already know that a case has no version; a
// consumer reading "" learns it from the answer.
func TestTheBriefObjectCarriesEverySectionEvenWhenEmpty(t *testing.T) {
	b, err := json.Marshal(briefJSON(Brief{Project: "my-health", Kind: "case"}))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		`"semver":""`, `"title":""`, `"status":""`,
		`"next_up":[]`, `"notes":[]`, `"blocked":[]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the object does not carry %s. An absent key reads as "+
				"\"this was never considered\", and a null cannot be told "+
				"from a field this build failed to set:\n%s", want, got)
		}
	}
}

// ---- the empty answers, which are answers ---------------------------------

// AN EMPTY NEXT-UP IS A SENTENCE. It is a real state - everything done, or
// everything still an idea - and an arriving session has to be able to tell it
// from a derivation that failed.
func TestAnEmptyNextUpIsASentenceAndNotABlankTable(t *testing.T) {
	got := briefNextUpSection(nil)

	if strings.Contains(got, "OWNER") {
		t.Errorf("an empty next-up printed a table header:\n%s", got)
	}
	if !strings.Contains(got, "Nothing is next up") {
		t.Errorf("an empty next-up did not say so in words:\n%s", got)
	}
	// And it says what WOULD put a row there, because the reader of an empty
	// list is usually somebody who expected work.
	if !strings.Contains(got, "active") {
		t.Errorf("an empty next-up did not say what puts an item on it:\n%s", got)
	}
}

func TestAnEmptyNotesSectionSaysSo(t *testing.T) {
	got := briefNotesSection(nil, now)

	if strings.Contains(got, "PRIORITY") {
		t.Errorf("an empty notes section printed a table header:\n%s", got)
	}
	if !strings.Contains(got, "Nothing has been attached") {
		t.Errorf("an empty notes section did not say so:\n%s", got)
	}
}

// THE COUNT IS NEVER COMPARED AGAINST next_up_n. Section 39 says "Never padded
// to N", twice - once for next-up and once for a case's notes - and a line
// reading "3 of 5" teaches a reader that two rows are missing when the truth
// is that there are three.
func TestTheNextUpCountReportsWhatIsThereAndNotAShortfall(t *testing.T) {
	got := briefNextUpSection(brief().NextUp)

	if !strings.Contains(got, "2 items") {
		t.Errorf("the count line does not report the rows it printed:\n%s", got)
	}
	if strings.Contains(got, " of ") {
		t.Errorf("the count is rendered against a target, which reads as rows "+
			"being missing:\n%s", got)
	}
}

// ---- notes carry who wrote them -------------------------------------------

// ⛔ SECTION 39 MAKES PROVENANCE THE WAY AN AGENT TELLS A DIRECTIVE FROM ITS
// OWN NARRATION: "Provenance already says who wrote it (Boris's session,
// distinct from an agent's), which is enough to tell a directive from routine
// progress narration." A note rendered without its author is a note an agent
// cannot weigh, and there is no status field to fall back on - section 39
// refuses one on purpose.
func TestANoteCarriesWhoWroteIt(t *testing.T) {
	got := briefNotesSection(brief().Notes, now)

	if !strings.Contains(got, "boris") {
		t.Errorf("the note does not say who wrote it:\n%s", got)
	}
	if !strings.Contains(got, "do not delete the logbook") {
		t.Errorf("the note's own words are missing:\n%s", got)
	}
}

// A NOTE FROM A SEAT THAT DID NOT REACH THIS CLIENT SAYS SO. The store refuses
// a write without provenance, so a blank author is a defect rather than an
// anonymous note - and an anonymous note is exactly what an agent would then
// weigh wrongly.
func TestANoteWithNoAuthorRendersAsADefect(t *testing.T) {
	got := briefNotesSection([]BriefNote{{ID: "n", Body: "something"}}, now)

	if !strings.Contains(got, "(not said)") {
		t.Errorf("a note with no seat rendered its author as a blank:\n%s", got)
	}
}

// ---- the argument ----------------------------------------------------------

// `rig brief` WITH NOTHING IS REFUSED RATHER THAN DEFAULTED. rig has no notion
// of the project a shell is "in" - a runtime directory names an ESTATE, not a
// project - so guessing one would be inventing it.
func TestBriefRefusesToGuessAProject(t *testing.T) {
	f := serving(t, &fakeRecord{})

	err := run([]string{"brief"})
	if err == nil {
		t.Fatal("`rig brief` with no argument was accepted; there is nothing " +
			"it could be about")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
	// Case-insensitive: the word is what matters and the capitalisation is
	// emphasis, so a rewording that shouts it still passes.
	if !strings.Contains(strings.ToLower(err.Error()), "slug") {
		t.Errorf("the usage does not say what the argument is - a project's "+
			"or a case's slug, which is also its record id:\n%s", err)
	}
}

// THE ARGUMENT REACHES THE CALL. A brief that always asked about the same
// project would pass every renderer test above.
func TestTheBriefAsksAboutTheProjectThatWasNamed(t *testing.T) {
	var asked string
	serving(t, &fakeRecord{
		brief: func(project string) (Brief, error) {
			asked = project
			return brief(func(b *Brief) { b.Project = project }), nil
		},
	})

	out, err := captureStdout(t, func() error { return run([]string{"brief", "my-health"}) })
	if err != nil {
		t.Fatal(err)
	}
	if asked != "my-health" {
		t.Errorf("the API was asked about %q", asked)
	}
	if !strings.Contains(out, "my-health") {
		t.Errorf("the brief does not name the container it is about:\n%s", out)
	}
}

// A BRIEF WITH NOTHING IN IT IS STILL A BRIEF. This is what an arriving
// session gets on a project nobody has recorded anything about yet, and every
// section has to be present and say so - the reader knows nothing and cannot
// tell a missing section from an empty one.
func TestABriefWithNothingInItStillPrintsEverySection(t *testing.T) {
	got := briefText(Brief{Project: "fresh", Kind: "project"}, now)

	for _, want := range []string{
		"fresh", "NEXT UP", "NOTES", "(no title)",
		"(no status)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("an empty brief is missing %q:\n%s", want, got)
		}
	}
}
