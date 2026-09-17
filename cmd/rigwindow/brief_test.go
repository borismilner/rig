package main

import (
	"strings"
	"testing"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The brief's section states are the one thing the view may not get wrong, so
// they are the one thing tested hardest here. Seven of the eleven sections are
// NOT COMPUTED against the live daemon; a mapping that flattened those to
// "computed" would make the window claim this project has no drift when the
// truth is that rig cannot yet know.
func TestSectionStateSurvivesTheMapping(t *testing.T) {
	in := &rigv1.ProjectBriefResponse{
		Project: "rig", Kind: "project", Title: "rig", Status: "active",
		Sections: []*rigv1.BriefSectionStatus{
			{Section: rigv1.BriefSection_BRIEF_SECTION_OPEN, State: rigv1.SectionState_SECTION_STATE_COMPUTED},
			{Section: rigv1.BriefSection_BRIEF_SECTION_DRIFT, State: rigv1.SectionState_SECTION_STATE_NOT_COMPUTED, Reason: "the standards register does not exist"},
		},
	}
	got := briefFrom(in)
	if len(got.Sections) != 2 {
		t.Fatalf("sections: want 2, got %d", len(got.Sections))
	}
	if got.Sections[0].Section != "OPEN" || got.Sections[0].State != "computed" {
		t.Errorf("computed section mapped wrong: %+v", got.Sections[0])
	}
	if got.Sections[1].State != "not computed" {
		t.Errorf("an UNBUILT section must not read as computed, got %q", got.Sections[1].State)
	}
	if got.Sections[1].Reason == "" {
		t.Error("an unbuilt section without its reason is the defect the field exists to prevent")
	}
}

// A new enum member must NOT render as a known state. An unrecognised value
// reported as "unspecified" is a new fact wearing an old name, which is the
// wire-that-lies shape this repository refuses elsewhere.
func TestAnUnknownEnumNamesItsNumber(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{stepStateName(rigv1.StepState(99)), "unknown step state 99"},
		{sectionStateName(rigv1.SectionState(98)), "unknown section state 98"},
		{sectionName(rigv1.BriefSection(97)), "unknown section 97"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

// A blocker carries its own state, because "blocked by B12" and "blocked by
// B12, which is itself blocked" are different situations for a reader deciding
// what to pick up.
func TestABlockerKeepsItsState(t *testing.T) {
	got := briefFrom(&rigv1.ProjectBriefResponse{
		Blocked: []*rigv1.Blockage{{
			Item: "B5", Title: "five",
			Blockers: []*rigv1.Blocker{{Id: "B12", Title: "twelve", State: rigv1.StepState_STEP_STATE_BLOCKED}},
		}},
	})
	if len(got.Blocked) != 1 || len(got.Blocked[0].Blockers) != 1 {
		t.Fatalf("blockage lost in the mapping: %+v", got.Blocked)
	}
	if b := got.Blocked[0].Blockers[0]; b.ID != "B12" || b.State != "blocked" {
		t.Errorf("blocker mapped wrong: %+v", b)
	}
}

// Brief refuses an empty project rather than asking the daemon a question with
// no subject, and says so in words a person can act on.
//
// ⛔ IT ASSERTS THE EXACT SENTENCE, AND THE FIRST VERSION OF THIS TEST DID NOT.
// It checked only that some error came back and that the word "project"
// appeared in it. Mutation-tested by disabling the guard: the test still
// PASSED, because the call then reached the daemon, which refused it for its
// own reasons and returned an error mentioning the project anyway. A check
// that could not go red had not passed - it had not run. It would also have
// been green in CI, where the connect fails and there is no daemon at all.
func TestBriefRefusesAnEmptyProject(t *testing.T) {
	_, err := RigService{}.Brief("")
	if err == nil {
		t.Fatal("an empty project must be refused")
	}
	const want = "a brief needs a project to be about"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("this must be OUR refusal, not the daemon's or the dialler's.\n got: %q\nwant it to contain: %q", err, want)
	}
}
