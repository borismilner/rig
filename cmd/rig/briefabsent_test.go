package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// B76: A BRIEF FOR A CONTAINER THAT DOES NOT EXIST.
//
// ⛔ THIS IS THE ABSENT-VERSUS-EMPTY ARGUMENT THE WHOLE BRIEF IS BUILT ON,
// INVERTED AT THE TOP LEVEL. Every section in brief.go is written to
// distinguish "nothing to report" from "this build cannot answer" - and the
// container itself made no such distinction. Measured 2026-09-17 against the
// live production daemon: `rig brief zzz-no-such-project-42` exited 0, printed
// 45 lines of brief, and reported sections 1-4 computed. A seat resuming on a
// mistyped slug was told, in rig's own voice, that there was nothing to do.
//
// ⛔ EVERY TEST HERE HAS A CONTROL THAT USES THE SAME ASSERTION ON A CONTAINER
// THAT DOES EXIST. Without one, a renderer that printed the missing-container
// sentence on EVERY brief would pass the whole file - which is the failure
// mode this seat's predecessor recorded: a collapse guard asserted over a
// whole rendering stayed green against the defect it was written for.

// ---- the heading ----------------------------------------------------------

// The kind word, the status word and the title all have their own "nothing was
// said" spelling, and stacking the three produced a heading that looked like a
// project whose metadata was thin rather than an id that names nothing.
func TestAMissingContainerIsNamedInTheHeadingAndNotSpelledAsAbsentMetadata(t *testing.T) {
	got := briefText(brief(func(b *Brief) {
		b.Project = "zzz-no-such-project-42"
		b.ContainerFound = rigv1.Tristate_TRISTATE_NO
		b.Kind = ""
		b.Title = ""
		b.Status = ""
		b.Semver = ""
	}), now, briefStyle{})

	head := firstLineOf(got)
	if !strings.Contains(head, "NO RECORD UNDER THIS ID") {
		t.Errorf("the first line does not say the id names nothing, so a typo "+
			"is indistinguishable from a project with no work:\n%s", head)
	}
	if !strings.Contains(head, "zzz-no-such-project-42") {
		t.Errorf("the heading does not name the id that was asked for:\n%s", head)
	}
	// ⛔ THE OLD SPELLINGS MUST BE GONE FROM THE HEADING, not merely joined by
	// a new sentence. "(not said)" is section 21's zero and is a fact about a
	// FIELD; here there was no record to read a field from, and printing both
	// teaches a reader that a missing container is a kind of thin metadata.
	if strings.Contains(head, "not said") || strings.Contains(head, "no status") {
		t.Errorf("the heading still spells the missing container as absent "+
			"metadata:\n%s", head)
	}
}

// THE CONTROL. A real container must not acquire the sentence, or the
// assertion above passes against a renderer that prints it unconditionally.
func TestAContainerThatExistsKeepsItsOrdinaryHeading(t *testing.T) {
	got := firstLineOf(briefText(brief(), now, briefStyle{}))
	if strings.Contains(got, "NO RECORD UNDER THIS ID") {
		t.Errorf("a project that exists was reported as missing, so the "+
			"missing-container heading is unconditional:\n%s", got)
	}
	if !strings.Contains(got, "project") || !strings.Contains(got, "active") {
		t.Errorf("the ordinary heading lost its kind or its status:\n%s", got)
	}
}

// ⛔ THE SECTIONS STAY. A record carries its own `project` field, so work
// items, decisions and notes can name an id that was never created as a
// container - `a0-survey` in the live production store is exactly that shape,
// measured 2026-09-17. A fix that emptied the page would replace one wrong
// answer with another, and this is the guard that stops it.
func TestAMissingContainerStillRendersTheRecordsThatNameIt(t *testing.T) {
	got := briefText(brief(func(b *Brief) {
		b.ContainerFound = rigv1.Tristate_TRISTATE_NO
		b.Kind = ""
	}), now, briefStyle{})
	if !strings.Contains(got, "wire the record verbs") {
		t.Errorf("the work recorded under this id vanished with the "+
			"container, which hides real records behind a naming defect:\n%s", got)
	}
	if !strings.Contains(got, "NEXT UP") {
		t.Errorf("the sections were suppressed entirely:\n%s", got)
	}
}

// ---- the predicate --------------------------------------------------------

// ⛔ ContainerMissing READS `container_found` AND DOES NOT INFER, AND THE ROWS
// THAT PROVE IT ARE THE ONES WHERE THE FIELD AND THE OLD INFERENCE DISAGREE.
//
// The old predicate was `b.Kind == ""`, which was correct and was not the
// fact: the derivation has known directly since rig 072aea4
// (record.Brief.ContainerFound) and the wire dropped it until field 22. A test
// that only ever showed the two AGREEING would pass against either predicate
// and could not tell which one this build uses - which is why the first two
// rows below set a kind that contradicts the field.
func TestTheMissingContainerTestIsTheWireFieldAndNotTheKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*Brief)
		want bool
	}{
		// ⛔ THE TWO DISCRIMINATING ROWS. Each is green under the field and
		// RED under `Kind == ""`, so between them they pin the mechanism.
		{"said missing, and still carries a kind", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_NO
			b.Kind = "project"
		}, true},
		{"said found, and carries no kind", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_YES
			b.Kind = ""
		}, false},

		{"said missing", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_NO
			b.Kind = ""
		}, true},
		{"a project", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_YES
			b.Kind = "project"
		}, false},
		{"a case", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_YES
			b.Kind = "case"
		}, false},

		// ⛔ AN EMPTY BRIEF ABOUT A REAL PROJECT IS NOT MISSING. This is the
		// pair the whole defect turns on: emptiness and absence are different
		// answers, and a predicate keyed on emptiness would say true here.
		{"a real project with nothing in it", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_YES
			b.Kind = "project"
			b.NextUp, b.Open, b.Notes = nil, nil, nil
			b.Title, b.Status, b.Semver = "", "", ""
		}, false},
		// And the mirror: a container that is missing but carries work.
		{"missing, with work under its name", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_NO
		}, true},

		// ---- UNSPECIFIED: A DAEMON THAT DOES NOT CARRY FIELD 22 ------------
		//
		// ⛔ THE FALLBACK IS DELIBERATE AND IS NOT DEAD CODE. Against a daemon
		// between rig af7715d and field 22, `kind` IS served and the old
		// inference IS correct, so spending the zero on "found" would re-open
		// B76 for exactly that range, quietly. Spending it on "missing" would
		// refuse every brief from every older daemon. These two rows are the
		// only place the inference is still reachable.
		{"not said, and a kind - the old inference stands", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_UNSPECIFIED
			b.Kind = "project"
		}, false},
		{"not said, and no kind - the old inference stands", func(b *Brief) {
			b.ContainerFound = rigv1.Tristate_TRISTATE_UNSPECIFIED
			b.Kind = ""
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := brief(tc.mut)
			if got := b.ContainerMissing(); got != tc.want {
				t.Errorf("ContainerMissing() = %v, want %v "+
					"(container_found=%v, kind=%q)",
					got, tc.want, b.ContainerFound, b.Kind)
			}
		})
	}
}

// ---- the exit status ------------------------------------------------------

// ⛔ THE EXIT STATUS IS THE HALF AN AGENT READS, AND IT WAS 0.
//
// This drives cmdBrief through the RecordAPI seam rather than asserting on a
// renderer, because the defect was never in the rendering: the command
// computed a correct brief about nothing and reported success.
func TestABriefForAnIdWithNoRecordFailsInsteadOfReportingSuccess(t *testing.T) {
	serving(t, &fakeRecord{brief: func(p string) (Brief, error) {
		return Brief{Project: p}, nil
	}})

	out, err := captureStdout(t, func() error { return cmdBrief([]string{"ghost"}) })
	if err == nil {
		t.Fatalf("rig brief on an id with no record succeeded, so an agent "+
			"reading the exit status is told the project is clear; it printed:\n%s", out)
	}
	obj, _, structured := shape(err)
	if !structured {
		t.Fatalf("the failure is prose and carries no code: %v", err)
	}
	if obj.Code != codeNoSuchContainer {
		t.Errorf("code is %q, want %q", obj.Code, codeNoSuchContainer)
	}
	if !strings.Contains(obj.Message, "ghost") {
		t.Errorf("the refusal does not name the id that was asked for: %q", obj.Message)
	}
	// Section 9: a precondition, the actual state, and a command that fixes
	// it. A caller who mistyped a slug needs the list of what is there.
	for _, f := range []struct{ label, value string }{
		{"precondition", obj.Precondition},
		{"actual", obj.Actual},
		{"fix", obj.Fix},
		{"fix command", obj.FixCommand},
	} {
		if strings.TrimSpace(f.value) == "" {
			t.Errorf("the refusal has no %s, so it is prose wearing a shape", f.label)
		}
	}
	if !strings.Contains(obj.FixCommand, "rig record query") {
		t.Errorf("the fix command does not list what the store holds: %q", obj.FixCommand)
	}
	// AND THE BRIEF WAS STILL PRINTED. stdout and stderr are two channels so
	// an answer and a complaint need not displace each other.
	if !strings.Contains(out, "NEXT UP") {
		t.Errorf("human mode printed no brief at all, so the records that DO "+
			"name this id are now unreachable:\n%s", out)
	}
}

// THE CONTROL, and it is the one that matters most here: a container that
// exists must still exit 0, or the fix has made every brief fail.
func TestABriefForAContainerThatExistsStillSucceeds(t *testing.T) {
	serving(t, &fakeRecord{brief: func(p string) (Brief, error) {
		return Brief{Project: p, Kind: "project", Status: "active"}, nil
	}})

	out, err := captureStdout(t, func() error { return cmdBrief([]string{"rig"}) })
	if err != nil {
		t.Fatalf("a brief for a real project failed: %v", err)
	}
	if !strings.Contains(out, "rig (project) active") {
		t.Errorf("the ordinary heading is gone:\n%s", out)
	}
}

// ⛔ --json RETURNS THE REFUSAL AND NOT THE BRIEF, and the reason is
// refusal.go's own rule: in that mode the object IS the answer, and two JSON
// documents on one stdout is not something any consumer can parse.
func TestJSONModeAnswersAMissingContainerWithTheObjectAndNotAnEmptyBrief(t *testing.T) {
	serving(t, &fakeRecord{brief: func(p string) (Brief, error) {
		return Brief{Project: p}, nil
	}})

	out, err := captureStdout(t, func() error {
		return cmdBrief([]string{"ghost", "--json"})
	})
	if err == nil {
		t.Fatal("rig brief --json on an id with no record succeeded")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("--json printed a brief object AND will print a status "+
			"object, so stdout carries two JSON documents:\n%s", out)
	}
	// The mode has to survive to the renderer or the object never reaches
	// stdout, which is the one promise --json makes about failures.
	var l *rigError
	if !errors.As(err, &l) || !l.asJSON {
		t.Errorf("the refusal did not keep --json, so it renders as prose "+
			"on stderr: %#v", err)
	}
}

// ---- the code -------------------------------------------------------------

// rig's own codes must never collide with the daemon's, and the guarantee is
// structural: every wire code is spelled CODE_*, so a RIG_* code is disjoint
// from all of them and from every one added later. refusal_test.go proves this
// over a hand-kept list that this code is not on, so it is proved here too.
func TestTheMissingContainerCodeIsRigsOwnAndCannotCollide(t *testing.T) {
	if !strings.HasPrefix(codeNoSuchContainer, codeLocal) {
		t.Errorf("%q does not start with %q, so the disjointness from the "+
			"daemon's CODE_* set is no longer structural",
			codeNoSuchContainer, codeLocal)
	}
	// It is its own code and not one of the six already there, because the
	// retry an agent should make differs: argv did not parse is not the same
	// failure as the store holds nothing under that id.
	for _, c := range []string{
		codeNoDaemon, codeNoSuchProgram, codeNoSuchCommand,
		codeBadArgument, codeTimeout, codeBadResult,
	} {
		if codeNoSuchContainer == c {
			t.Errorf("the missing-container failure reuses %q", c)
		}
	}
}

// firstLineOf is the heading, which is the span every assertion above is
// about. Asserting over a whole rendering is how this file's predecessor
// shipped two blind guards: a sentence that appears anywhere is not evidence
// that it appears where a reader looks.
func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var _ = time.Time{}

// ---- the seam -------------------------------------------------------------

// ⛔ `briefFromWire` CARRIES `container_found`, AND THIS TEST EXISTS BECAUSE
// NOTHING ELSE IN THIS PACKAGE NOTICES IF IT STOPS.
//
// MEASURED, not assumed: with `ContainerFound: r.GetContainerFound()` deleted
// from briefFromWire, `go test ./cmd/rig -count=1` was GREEN over the whole
// package. Every other guard on this condition builds a `Brief` directly and
// never crosses the wire, so the one line that joins the two ends was covered
// by nothing.
//
// ⛔ IT IS THE SEAM THIS TEAM KEEPS LOSING THINGS IN, THIRD INSTANCE.
// internal/record derives, internal/daemon maps, cmd/rig renders, and a field
// added at one end and dropped at another is invisible from both. Notes,
// Features and the four header fields were each exactly that.
//
// ⛔ THE ZERO IS A ROW. A wire that did not set the field must arrive as
// UNSPECIFIED and not as a translated false, because UNSPECIFIED is the only
// thing that still licenses the legacy inference in ContainerMissing.
func TestTheClientCarriesWhatTheWireSaidAboutTheContainer(t *testing.T) {
	for _, want := range []rigv1.Tristate{
		rigv1.Tristate_TRISTATE_YES,
		rigv1.Tristate_TRISTATE_NO,
		rigv1.Tristate_TRISTATE_UNSPECIFIED,
	} {
		t.Run(want.String(), func(t *testing.T) {
			got := briefFromWire(&rigv1.ProjectBriefResponse{
				Project:        "rig",
				Kind:           "project",
				ContainerFound: want,
			})
			if got.ContainerFound != want {
				t.Errorf("the wire said container_found=%v and the client "+
					"holds %v. The one line joining the daemon's fact to this "+
					"client's predicate is gone, and B76 is back to being an "+
					"inference", want, got.ContainerFound)
			}
		})
	}
}
