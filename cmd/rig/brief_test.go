package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

func brief(mut ...func(*Brief)) Brief {
	b := Brief{
		Project: "rig",
		Kind:    "project",
		Title:   "the swiss knife and the record under it",
		Status:  "active",
		Semver:  "0.4.1",

		// ⛔ THE FIXTURE IS WHAT A DAEMON THAT CARRIES FIELD 22 SENDS, and
		// leaving this at the zero would have made every test in the package
		// pass through `ContainerMissing`'s legacy fallback instead of the
		// path production takes. A fixture that exercises the wrong branch is
		// green for the wrong reason.
		ContainerFound: rigv1.Tristate_TRISTATE_YES,
		NextUp: []BriefItem{
			{
				ID:    "01927-a",
				Title: "wire the record verbs",
				State: "started",
				Since: now.Add(-30 * time.Minute),
				Note:  "the seam is next",
			},
			// ⛔ NO STATE AND NO STAMP, DELIBERATELY: it is the item nobody has
			// stepped, which is a real state and the one a blank cell would
			// swallow.
			{ID: "01927-b", Title: "the CLI surface"},
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
		b.Cycles = []BriefCycle{{Items: []string{"01927-x", "01927-y", "01927-z"}}}
	}), now, briefStyle{})

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
	got := briefBlockedSection([]BriefCycle{{Items: []string{"a", "b"}}}, briefStyle{})

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
		b.Cycles = []BriefCycle{{Items: []string{"01927-x", "01927-y"}}}
	}), now, briefStyle{})

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

// AND A BRIEF WITH NO CYCLE SAYS NOTHING ABOUT CYCLES. The cycle report is the
// one thing here allowed to be silent: a "no cycles" line on every brief
// trains a reader to skip the place the real one appears.
//
// ⛔ THE PROBE WAS THE BARE WORD "BLOCKED" AND THAT WAS TOO WIDE, WHICH ONLY
// SHOWED UP WHEN SECTION 4 STARTED PRINTING. "BLOCKED CONDITION" is the cycle
// report; "BLOCKED" alone is now also section 39's row 4 heading, which the
// daemon reports COMPUTED and which therefore MUST render - "nothing is
// blocked" and "blocked was not computed" are different facts and a silent
// section collapses them. Two different claims had been resting on one
// substring, and the narrower probe is the one this test always meant.
func TestABriefWithNoCycleHasNoCycleReportAtAll(t *testing.T) {
	got := briefText(brief(), now, briefStyle{})

	if strings.Contains(got, "BLOCKED CONDITION") {
		t.Errorf("a clean brief carries a cycle report, which teaches a "+
			"reader to skip the line a real cycle appears on:\n%s", got)
	}
	// The other half, and it is the new rule rather than a restatement: row 4
	// is COMPUTED, so it renders even with nothing in it.
	if !strings.Contains(got, "Nothing is blocked") {
		t.Errorf("section 4 rendered nothing at all, which cannot be told "+
			"from a section this build has no renderer for:\n%s", got)
	}
}

// A CYCLE WHOSE ITEMS DID NOT ARRIVE IS NOT A DECORATIVE WARNING. A report
// that reports nothing is worse than no report: it tells a reader something is
// wrong and gives them nowhere to look.
func TestACycleWithNoItemsSaysTheItemsAreMissing(t *testing.T) {
	got := briefBlockedSection([]BriefCycle{{}}, briefStyle{})

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
	}), briefStyle{})

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
	if p := briefHeading(brief(), briefStyle{}); !strings.Contains(p, "v0.4.1") {
		t.Errorf("a project's semver is not printed either, so the case "+
			"assertion above proves nothing:\n%s", p)
	}
}

// `semver` IS PRESENT AND EMPTY IN THE OBJECT rather than omitted. A consumer
// branching on its absence has to already know that a case has no version; a
// consumer reading "" learns it from the answer.
func TestTheBriefObjectCarriesEverySectionEvenWhenEmpty(t *testing.T) {
	b, err := json.Marshal(briefJSON(Brief{Project: "my-health", Kind: "case"}, now))
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
	got := briefNextUpSection(nil, now, briefStyle{})

	if strings.Contains(got, "STATE") {
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
	got := briefNotesSection(nil, now, briefStyle{})

	if strings.Contains(got, "PRIORITY") {
		t.Errorf("an empty notes section printed a table header:\n%s", got)
	}
	if !strings.Contains(got, "Nothing is attached to this one") {
		t.Errorf("an empty notes section did not say so:\n%s", got)
	}
	// ⛔ AND IT SAYS WHAT IT LOOKED FOR, which every other empty section on
	// this page does and this one did not. The live store holds eight notes
	// this section correctly reports none of, because none of them carries the
	// `part-of` edge section 3 is defined over.
	if !strings.Contains(got, "part-of") {
		t.Errorf("the empty notes sentence states what it FOUND and not what "+
			"it LOOKED FOR, so a reader cannot tell an unwritten note from an "+
			"unattached one:\n%s", got)
	}
}

// THE COUNT IS NEVER COMPARED AGAINST next_up_n. Section 39 says "Never padded
// to N", twice - once for next-up and once for a case's notes - and a line
// reading "3 of 5" teaches a reader that two rows are missing when the truth
// is that there are three.
func TestTheNextUpCountReportsWhatIsThereAndNotAShortfall(t *testing.T) {
	got := briefNextUpSection(brief().NextUp, now, briefStyle{})

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
	got := briefNotesSection(brief().Notes, now, briefStyle{})

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
	got := briefNotesSection([]BriefNote{{ID: "n", Body: "something"}}, now, briefStyle{})

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
	got := briefText(Brief{Project: "fresh", Kind: "project"}, now, briefStyle{})

	for _, want := range []string{
		"fresh", "NEXT UP", "NOTES", "(no title)",
		"(no status)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("an empty brief is missing %q:\n%s", want, got)
		}
	}
}

// ---- the wire -----------------------------------------------------------

// ⛔ WITHHELD-BY-VIEW IS NEVER COLLAPSED INTO NOT-COMPUTED, AND THIS BOUNDARY
// IS WHERE COLLAPSING THEM WOULD BE EASIEST.
//
// A section the caller is not being shown and a section nothing can compute
// are different answers: the first says the derivation RAN, the second says
// its input does not exist. Folding the view rule into the capability gap
// would make the human view read as a degraded agent view, which is the exact
// reading SectionState was added to prevent.
func TestASectionWithheldByTheViewIsNotReportedAsUnbuilt(t *testing.T) {
	b := briefFromWire(&rigv1.ProjectBriefResponse{
		Project: "rig",
		Sections: []*rigv1.BriefSectionStatus{
			{
				Section: rigv1.BriefSection_BRIEF_SECTION_NEXT_UP,
				State:   rigv1.SectionState_SECTION_STATE_COMPUTED,
			},
			{
				Section: rigv1.BriefSection_BRIEF_SECTION_MUST_READ,
				State:   rigv1.SectionState_SECTION_STATE_WITHHELD_BY_VIEW,
			},
			{
				Section: rigv1.BriefSection_BRIEF_SECTION_DRIFT,
				State:   rigv1.SectionState_SECTION_STATE_NOT_COMPUTED,
				Reason:  "the standards register is slice 6",
			},
		},
	})

	if len(b.Sections) != 3 {
		t.Fatalf("got %d sections, want 3", len(b.Sections))
	}
	// THE NUMBER IS SECTION 39'S OWN ROW NUMBER. The proto says so: the
	// numbers "are citations rather than an ordinal, and they are never
	// renumbered", which is why this is a conversion and not a lookup table.
	if b.Sections[0].Section != 2 || !b.Sections[0].Computed {
		t.Errorf("next-up came back as %+v, want section 2 computed", b.Sections[0])
	}
	withheld := b.Sections[1]
	if !withheld.Withheld {
		t.Errorf("a section withheld by the view did not say so: %+v", withheld)
	}
	if withheld.Computed {
		t.Errorf("a withheld section reported itself COMPUTED, which would "+
			"render its emptiness as an answer: %+v", withheld)
	}
	// ⛔ AND IT MUST NOT REACH THE NOT-ANSWERED LIST, which is the rendering
	// where the collapse would actually be read by somebody.
	if got := briefUnavailableSection(b.Sections, briefStyle{}); strings.Contains(got, "section 6") {
		t.Errorf("the must-read set was reported as not built, when the "+
			"derivation ran and this caller is simply not being shown it:\n%s", got)
	}
	// The control: a genuinely unbuilt section MUST appear there, or the
	// assertion above passes against a renderer that lists nothing at all.
	if got := briefUnavailableSection(b.Sections, briefStyle{}); !strings.Contains(got, "slice 6") {
		t.Errorf("a section that is not built did not reach the not-answered "+
			"list, so the assertion above proves nothing:\n%s", got)
	}
}

// ⛔ AN ITEM NOBODY HAS STEPPED IS NOT THE STALEST THING IN THE LIST.
//
// The wire says a zero `since` means there are no steps, and that it must not
// render as infinitely stale - such an item sorts BELOW every real signal, not
// above it. A zero stamp turned into time.Unix(0,0) would print as a 56-year
// age, which is the reading that inverts the list.
func TestAnItemNobodyHasSteppedReportsNoAgeRatherThanAHugeOne(t *testing.T) {
	b := briefFromWire(&rigv1.ProjectBriefResponse{
		Project: "rig",
		NextUp: []*rigv1.ItemState{
			{Id: "01927-a", Title: "unstepped"},
			{
				Id: "01927-b", Title: "moving", Note: "seam landed",
				State:         rigv1.StepState_STEP_STATE_STARTED,
				SinceUnixNano: now.Add(-90 * time.Second).UnixNano(),
			},
		},
	})

	if !b.NextUp[0].Since.IsZero() {
		t.Errorf("an item with no steps carries the stamp %s", b.NextUp[0].Since)
	}
	if b.NextUp[0].State != "" {
		t.Errorf("an unstepped item was given the state %q. The wire spends "+
			"its enum ZERO on this case and it is a real state - picked up, "+
			"nobody has reported on it", b.NextUp[0].State)
	}
	if b.NextUp[1].State != "started" {
		t.Errorf("a stepped item came back as %q, so the state is not "+
			"travelling and the assertion above proves nothing",
			b.NextUp[1].State)
	}

	got := briefNextUpSection(b.NextUp, now, briefStyle{})
	if !strings.Contains(got, "not stepped") {
		t.Errorf("the unstepped item rendered as a blank rather than as the "+
			"state it is:\n%s", got)
	}
	// The age column: "-" for the zero, a real age beside it. Without the
	// second half this passes against a renderer that prints "-" for every
	// row.
	if !strings.Contains(got, "90s") && !strings.Contains(got, "1m") {
		t.Errorf("the stepped item shows no age at all:\n%s", got)
	}
	if strings.Contains(got, "OWNER") {
		t.Errorf("the table still prints an OWNER column, and the wire has "+
			"nothing that could ever fill it:\n%s", got)
	}
}

// A BLOCKER NOBODY HAS PICKED UP KEEPS ITS EMPTY STATE, and the renderer is
// what spells it out. Section 39 makes that the `idea` case and it is
// load-bearing: the way forward is for somebody to START it, which is a
// different instruction from waiting on work in progress.
func TestABlockerNobodyHasStartedKeepsTheEmptyStateTheRendererSpellsOut(t *testing.T) {
	b := briefFromWire(&rigv1.ProjectBriefResponse{
		Project: "rig",
		Blocked: []*rigv1.Blockage{{
			Item: "01927-a", Title: "the seeding",
			Blockers: []*rigv1.Blocker{
				{Id: "01927-b", Title: "the CLI seam"},
				{
					Id: "01927-c", Title: "the store",
					State: rigv1.StepState_STEP_STATE_STARTED,
				},
			},
		}},
	})

	if s := b.Blocked[0].Blockers[0].State; s != "" {
		t.Errorf("an unstarted blocker was given the state %q", s)
	}
	if s := b.Blocked[0].Blockers[1].State; s != "started" {
		t.Errorf("a started blocker came back as %q, so the state is not "+
			"travelling", s)
	}
	got := briefBlockageSection(b.Blocked, briefStyle{})
	if !strings.Contains(got, "nobody has picked it up") {
		t.Errorf("the `idea` blocker rendered as a blank:\n%s", got)
	}
}

// ---- ⛔ a section the reader is not seeing says WHICH HALF is at fault -----

// ⛔ A SECTION rigd COMPUTED AND THIS BUILD CANNOT RENDER IS A GAP IN THE
// CLIENT, AND IT MUST NOT READ AS A GAP IN rig.
//
// Both faults leave the reader without the section and they are repaired in
// different files, so a brief that blurs them sends whoever reads it to the
// wrong half of the system. This is the failure that shipped: `sections` would
// report a section COMPUTED while the renderer printed nothing, and nothing
// anywhere said so.
func TestASectionComputedAndNotRenderedIsReportedAsTheClientsGap(t *testing.T) {
	// Section 5 is `drift`. This build has no field for it, so a daemon that
	// starts computing it would silently drop it here.
	got := briefUnavailableSection([]BriefSectionState{
		{Section: 5, Computed: true},
	}, briefStyle{})

	if got == "" {
		t.Fatal("a section the daemon computed and this build cannot render " +
			"was reported nowhere at all, which is the exact defect this list " +
			"exists to prevent")
	}
	if !strings.Contains(got, "section 5") {
		t.Errorf("the report does not name the section:\n%s", got)
	}
	// ⛔ THE SENTENCE HAS TO SAY WHICH HALF. A reader sent to rigd for a
	// client-side gap finds nothing wrong there and concludes the brief is
	// correct.
	if !strings.Contains(got, "rig's CLI") {
		t.Errorf("the report does not say the gap is in this client rather "+
			"than in the derivation:\n%s", got)
	}
	if strings.Contains(got, "not yet built") {
		t.Errorf("a COMPUTED section was reported as unbuilt, which is the "+
			"other fault entirely:\n%s", got)
	}
}

// AND THE TWO LISTS ARE SEPARATE, which is what stops one being read as the
// other when both are present.
func TestTheTwoNotShownReasonsAreReportedAsDifferentLists(t *testing.T) {
	got := briefUnavailableSection([]BriefSectionState{
		{Section: 5, Computed: true},
		{Section: 7, Computed: false, Reason: "the git projection does not exist yet"},
	}, briefStyle{})

	if !strings.Contains(got, "the git projection does not exist yet") {
		t.Errorf("the unbuilt section lost its reason:\n%s", got)
	}
	if !strings.Contains(got, "rig's CLI") {
		t.Errorf("the client-side gap is not reported beside it:\n%s", got)
	}
	// The unbuilt list comes first: it is the one a reader can do nothing
	// about, and the client-side one has an action attached.
	if strings.Index(got, "not yet built") > strings.Index(got, "rig's CLI") {
		t.Errorf("the client-side gap is printed before the capability gap:\n%s", got)
	}
}

// ⛔ EVERY SECTION THIS BUILD CLAIMS TO RENDER ACTUALLY PRINTS SOMETHING WHEN
// IT IS EMPTY.
//
// This is the guard behind the lead's ruling - "a section reported COMPUTED
// must render something, even if that something is none". An empty list and a
// section this build cannot render are otherwise the same output, and the
// whole of SectionState exists to keep them apart.
//
// THE HEADINGS ARE LISTED HERE RATHER THAN WALKED, and that is deliberate: a
// heading is prose and there is nothing in the code to walk. What keeps the
// table honest is the count assertion at the end, which fails the moment
// briefRenderedSections gains a row this test does not.
func TestEverySectionThisBuildRendersPrintsSomethingWhenItIsEmpty(t *testing.T) {
	headings := map[int]string{
		1:  "ALSO OPEN",
		2:  "NEXT UP",
		3:  "NOTES",
		4:  "BLOCKED",
		10: "FEATURES",
		12: "GOVERNING",
		13: "CLOSED",
	}
	// The positive control, and it is what makes every assertion below mean
	// something: a table that has fallen behind briefRenderedSections would
	// otherwise pass by simply not checking the new row.
	if len(headings) != len(briefRenderedSections) {
		t.Fatalf("this table has %d rows and briefRenderedSections has %d. A "+
			"section was added to the renderer's claim without being added "+
			"here, so it is UNCHECKED and this test proves nothing about it",
			len(headings), len(briefRenderedSections))
	}

	for section, heading := range headings {
		if !briefRenderedSections[section] {
			t.Errorf("section %d is in this table and not in "+
				"briefRenderedSections", section)
			continue
		}
		// An EMPTY brief that says all five are computed. Nothing to print,
		// and every one of them must print anyway.
		empty := Brief{Project: "rig", Sections: []BriefSectionState{
			{Section: section, Computed: true},
		}}
		got := briefText(empty, now, briefStyle{})
		if !strings.Contains(got, heading) {
			t.Errorf("section %d is reported COMPUTED and this build claims "+
				"to render it, and %q is nowhere in the output. An empty "+
				"section that prints nothing cannot be told from one this "+
				"build has no renderer for:\n%s", section, heading, got)
		}
		// And it must NOT appear in either not-shown list, because it IS
		// shown.
		if u := briefUnavailableSection(empty.Sections, briefStyle{}); u != "" {
			t.Errorf("section %d rendered AND was reported as not shown:\n%s",
				section, u)
		}
	}
}

// THE OPEN LIST IS ITS OWN SECTION AND IS NOT FOLDED INTO NEXT-UP. The wire
// cuts the two disjoint so an item never gets two incompatible rules for
// rendering its notes; a renderer that merged them would double-count.
func TestOpenAndNextUpAreRenderedAsTwoListsAndNotOne(t *testing.T) {
	b := briefFromWire(&rigv1.ProjectBriefResponse{
		Project: "rig",
		NextUp:  []*rigv1.ItemState{{Id: "01927-n", Title: "the seam"}},
		Open:    []*rigv1.ItemState{{Id: "01927-o", Title: "the render blocks"}},
	})
	if len(b.Open) != 1 || len(b.NextUp) != 1 {
		t.Fatalf("open=%d next_up=%d, want 1 and 1", len(b.Open), len(b.NextUp))
	}

	got := briefText(b, now, briefStyle{})
	if !strings.Contains(got, "01927-o") {
		t.Errorf("the open item is nowhere in the brief:\n%s", got)
	}
	if !strings.Contains(got, "01927-n") {
		t.Errorf("the next-up item is nowhere in the brief:\n%s", got)
	}
	// ⛔ BOTH IDS ARE COUNTED, AND THE ONE-SIDED VERSION OF THIS SURVIVED A
	// MUTATION. Counting only the open id catches open leaking into next-up
	// and is blind to next-up leaking into open, which is the direction a
	// renderer that concatenates the lists actually fails in. A guard that
	// covers one direction of a symmetric property has not run on the other.
	for _, id := range []string{"01927-o", "01927-n"} {
		if n := strings.Count(got, id); n != 1 {
			t.Errorf("%s appears %d times, so the two lists were merged and a "+
				"reader adding them up double-counts:\n%s", id, n, got)
		}
	}
}

// FEATURES AND THEIR STAGE COUNTS ARE BOTH PRINTED. A count on its own cannot
// be acted on - "three at shipped" does not say which three - and a list on
// its own makes a reader tally the stages by eye. Section 39 carries both
// fields, so both are rendered.
func TestTheFeaturesSectionCarriesBothTheListAndTheCounts(t *testing.T) {
	b := briefFromWire(&rigv1.ProjectBriefResponse{
		Project: "rig",
		Features: []*rigv1.Feature{
			{Id: "01927-f", Title: "the continuity record", Stage: "building"},
		},
		FeatureStages: []*rigv1.StageCount{
			{Stage: "building", Count: 1},
			{Stage: "shipped", Count: 4},
		},
	})

	got := briefFeaturesSection(b.Features, b.FeatureStages, briefStyle{})
	for _, want := range []string{
		"01927-f", "the continuity record", "building", "shipped", "4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the features section does not carry %q:\n%s", want, got)
		}
	}

	// AN EMPTY ONE IS A SENTENCE, because row 10 is COMPUTED and a silent
	// section cannot be told from one this build cannot render.
	empty := briefFeaturesSection(nil, nil, briefStyle{})
	if !strings.Contains(empty, "no features recorded") {
		t.Errorf("an empty features section printed no sentence:\n%s", empty)
	}
}

// ⛔ THE BRIEF IS THE THIRD HUMAN SURFACE AND IT HAD NO GUARD AT ALL.
//
// Found by mutation: swapping `displaySeat` back to `provWord` in
// `briefNotesSection` SURVIVED the whole suite. The reason is the fixture -
// its seat is the bare string `boris`, with no kind prefix, so the two
// functions return the same bytes for it and no assertion over that fixture
// can tell them apart. **A test whose input cannot exhibit the property is
// not a weak test, it is not a test of that property at all.**
//
// This one uses a seat shaped the way the daemon actually mints them:
// `internal/daemon/record.go` writes `"terminal:" + u.Username`.
func TestANotesAuthorIsRenderedForAHumanRatherThanAsAStoredSeat(t *testing.T) {
	t.Setenv("RIG_DISPLAY_NAME", "")

	got := briefNotesSection([]BriefNote{{
		ID: "n1", Body: "the note's own words",
		Prov: Provenance{
			Session: "s-1", Seat: "terminal:boris-milner", CreatedAt: now.Add(-time.Hour),
		},
	}}, now, briefStyle{})

	if !strings.Contains(got, "boris-milner") {
		t.Errorf("the note does not say who wrote it:\n%s", got)
	}
	// The absence is the half that bites. "boris-milner" is a substring of
	// "terminal:boris-milner", so the presence check above passes on the
	// UNSTRIPPED rendering too and proves nothing on its own.
	if strings.Contains(got, "terminal:boris-milner") {
		t.Errorf("the brief renders the stored seat with its kind prefix. A "+
			"human reading their own project's notes is shown a name; the "+
			"namespaced value is what --json carries:\n%s", got)
	}
}

// briefWireFieldsNotRendered names every field on `ProjectBriefResponse` that
// `briefJSON` deliberately does not emit, WITH THE REASON.
//
// ⛔ IT IS A TABLE RATHER THAN A COMMENT BECAUSE THE COMMENT ALREADY ROTTED.
// `briefFromWire`'s own doc predicted it - "a comment rots the first time
// somebody adds a field" - and then named five unread fields when the wire
// carried six. `must_read_cleared` was the sixth, and nothing noticed, because
// nothing could: prose is not checked against anything.
var briefWireFieldsNotRendered = map[string]string{
	"coarse_citations": "a count the daemon computes for its own ranking; " +
		"section 39 does not put it in front of a reader, and a number with " +
		"no unit beside it is the kind of field a reader invents a meaning for",
	"must_read": "section 6's set is not built. When it lands it is BARE IDS " +
		"with no title resolver - the lead withdrew the lookup half of that " +
		"ruling 2026-09-17, and the precedent on this wire (Blockage carrying " +
		"item AND title) is that a reference CARRIES what it needs",
	"must_read_cleared": "the flag half of the same unbuilt set. ⛔ THIS IS " +
		"THE FIELD THE PROSE VERSION OF THIS LIST MISSED",
	"drift":      "section 39 row 8, NOT_COMPUTED by today's daemon",
	"health":     "section 39 row 9, NOT_COMPUTED by today's daemon",
	"case_notes": "section 39 row 11, NOT_COMPUTED by today's daemon",
	"container_found": "B76, AND IT IS READ RATHER THAN DROPPED - " +
		"`briefFromWire` carries it and `ContainerMissing` decides on it. It " +
		"is not a KEY because in --json the REFUSAL is the rendering: " +
		"brief.go returns RIG_NO_SUCH_CONTAINER instead of the brief when the " +
		"container is missing, so a key emitted beside a brief that exists " +
		"could only ever read true. A field with one reachable value teaches " +
		"a consumer to stop reading it",
}

// ⛔ EVERY FIELD THE BRIEF WIRE CARRIES IS EITHER RENDERED OR HAS A WRITTEN
// REASON NOT TO BE, AND A NEW ONE IS RED UNTIL SOMEBODY DECIDES WHICH.
//
// This is `TestEveryFieldTheWireCarriesOnARefIsRendered` at the message that
// matters most, and it exists because of a seam this team keeps losing things
// in: `internal/record` derives, `internal/daemon` maps, and this package
// renders, with three different owners and nobody owning the join. A field
// added at one end and never picked up at the other is invisible from both -
// `Notes`/`Features`/`Stages` were exactly that, and so was the brief header.
//
// ⛔ THE SECOND HALF IS THE ONE THAT MAKES IT SAFE: A REASON FOR A FIELD THAT
// NO LONGER EXISTS IS ALSO RED. Otherwise this table becomes its own comment -
// a list of excuses that outlive what they excused, which is precisely the
// property that makes `//rig:allow` safe and a `.golangci.yml` line not.
func TestEveryFieldOnTheBriefWireIsRenderedOrSaysWhyNot(t *testing.T) {
	emitted := map[string]bool{}
	for k := range briefJSON(Brief{}, now) {
		emitted[k] = true
	}

	onTheWire := map[string]bool{}
	fields := (&rigv1.ProjectBriefResponse{}).ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		name := string(fields.Get(i).Name())
		onTheWire[name] = true

		if emitted[name] {
			if why, excused := briefWireFieldsNotRendered[name]; excused {
				t.Errorf("%q is BOTH rendered and excused (%q). One of the two "+
					"is stale, and an excuse beside a rendering is how a "+
					"reader learns to distrust the table", name, why)
			}
			continue
		}
		if _, excused := briefWireFieldsNotRendered[name]; !excused {
			t.Errorf("the wire carries %q and `briefJSON` does not emit it, "+
				"and no reason is recorded.\n"+
				"A field added at the store or the daemon and never picked up "+
				"here is invisible from BOTH ends - it is the defect this "+
				"seam has produced three times.\n"+
				"Render it, or add it to briefWireFieldsNotRendered WITH a "+
				"reason.", name)
		}
	}

	// ⛔ AN EXCUSE THAT OUTLIVED ITS FIELD. Without this the table rots the
	// same way the comment it replaced did, just more slowly and with more
	// authority.
	for name := range briefWireFieldsNotRendered {
		if !onTheWire[name] {
			t.Errorf("briefWireFieldsNotRendered excuses %q, which is not a "+
				"field on ProjectBriefResponse any more. Delete the row: an "+
				"exemption must not outlive what it excused", name)
		}
	}
}
