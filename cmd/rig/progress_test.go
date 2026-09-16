package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// ---- one validator, not two ------------------------------------------------

// ⛔ THE CLIENT DOES NOT VALIDATE THE STATE, AND THIS TEST IS WHAT STOPS
// SOMEBODY ADDING THAT.
//
// rigd refuses an unknown state by name and quotes the value back. A second
// copy of the set {started, blocked, done} in this binary is a second thing to
// keep in step with section 39, and the day the two disagree the caller is
// told something that is not true of the daemon. Two validators drift; one
// does not.
//
// So an empty state and a nonsense state both REACH the API, and the refusal
// the caller reads is the daemon's.
func TestTheClientDoesNotSecondGuessTheStepState(t *testing.T) {
	for _, state := range []string{"", "finished", "DONE"} {
		t.Run("state "+state, func(t *testing.T) {
			f := serving(t, &fakeRecord{})

			if _, err := captureStdout(t, func() error {
				return run([]string{
					"progress", "step", "01927-item",
					"--project", "rig", "--state", state,
				})
			}); err != nil {
				t.Fatalf("the client refused state %q itself: %v\n"+
					"rigd names the states it accepts and quotes the bad "+
					"value back; a client that refuses first is a second "+
					"validator that can drift from it", state, err)
			}
			if !slices.Contains(f.calls, "step") {
				t.Fatal("the step never reached the API")
			}
			if f.lastStep.State != state {
				t.Errorf("the API saw state %q, want %q unchanged: rewriting "+
					"it here is the same defect as validating it here",
					f.lastStep.State, state)
			}
		})
	}
}

// AND AN EMPTY PROJECT REACHES IT TOO, for the same reason: rigd says "a step
// needs a project".
func TestAStepWithNoProjectStillReachesTheDaemon(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if _, err := captureStdout(t, func() error {
		return run([]string{"progress", "step", "01927-item", "--state", "done"})
	}); err != nil {
		t.Fatalf("the client refused an empty --project itself: %v", err)
	}
	if f.lastStep.Project != "" {
		t.Errorf("the API saw project %q, want it empty and refused there",
			f.lastStep.Project)
	}
}

// EVERY ARGUMENT REACHES THE CALL UNCHANGED. Nothing here is enforced by the
// compiler: StepArgs is a keyed literal, so a field dropped at the call site
// compiles and the step is written without it.
func TestEveryStepArgumentReachesTheCall(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if _, err := captureStdout(t, func() error {
		return run([]string{
			"progress", "step", "01927-item",
			"--project", "rig", "--state", "blocked",
			"--note", "waiting on the wire",
		})
	}); err != nil {
		t.Fatal(err)
	}
	want := StepArgs{
		Item: "01927-item", State: "blocked",
		Note: "waiting on the wire", Project: "rig",
	}
	if f.lastStep != want {
		t.Errorf("the API saw %+v, want %+v", f.lastStep, want)
	}
}

// ---- the argument shape ----------------------------------------------------

// THE ITEM IS REQUIRED AND THE USAGE SAYS WHERE TO GET ONE. A work item's id
// is a UUIDv7 nobody types from memory, so a usage line that only names
// `<item>` tells a caller what is missing and not how to find it.
func TestAStepWithNoItemIsRefusedAndSaysWhereItemsComeFrom(t *testing.T) {
	f := serving(t, &fakeRecord{})

	err := run([]string{"progress", "step", "--project", "rig", "--state", "done"})
	if err == nil {
		t.Fatal("a step with no item was accepted")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
	if !strings.Contains(err.Error(), "rig record query") {
		t.Errorf("the usage does not say how to find an item id:\n%s", err)
	}
}

// AN UNKNOWN SUBCOMMAND NAMES THE ONE THAT EXISTS.
func TestAnUnknownProgressSubcommandIsRefused(t *testing.T) {
	f := serving(t, &fakeRecord{})

	err := run([]string{"progress", "stream", "01927-item"})
	if err == nil {
		t.Fatal("`rig progress stream` was accepted")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
	if !strings.Contains(err.Error(), "step") {
		t.Errorf("the refusal does not name the subcommand that exists:\n%s", err)
	}
}

// A SECOND POSITIONAL IS REFUSED RATHER THAN IGNORED. `rig progress step item
// "did the thing"` is the shape somebody will try, and silently dropping the
// second word would record a step with no note and report success.
func TestAStepRefusesASecondPositionalRatherThanDroppingIt(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if err := run([]string{"progress", "step", "01927-item", "did the thing"}); err == nil {
		t.Fatal("a stray positional was accepted; the note has to come " +
			"through --note or it is silently lost")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
}

// ---- rendering -------------------------------------------------------------

// ⛔ THE CONFIRMATION RENDERS WHAT WAS RECORDED, NOT WHAT WAS ASKED.
//
// Echoing the request is the one rendering that cannot fail: a daemon that
// stored a different state would print exactly the same line as one that
// stored the right one. So the fake answers with a state the caller did not
// send, and the output has to show the daemon's.
func TestTheStepConfirmationPrintsWhatCameBackAndNotWhatWasSent(t *testing.T) {
	serving(t, &fakeRecord{
		step: func(StepArgs) (Record, error) {
			return step(func(r *Record) {
				r.Fields = map[string]string{"state": "blocked", "item": "01927-other"}
			}), nil
		},
	})

	out, err := captureStdout(t, func() error {
		return run([]string{
			"progress", "step", "01927-item",
			"--project", "rig", "--state", "done",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "01927-other") {
		t.Errorf("the confirmation does not carry what the daemon recorded:\n%s",
			out)
	}
	if strings.Contains(out, "done") {
		t.Errorf("the confirmation echoes the state the CALLER sent, so a "+
			"daemon that stored something else prints as success:\n%s", out)
	}
}

// A STATE OR ITEM MISSING FROM THE ANSWER IS A DEFECT AND SAYS SO. rigd cannot
// write a step without either, so a missing key means it did not survive the
// wire - and section 21's rule binds: nothing was said is never a fact.
func TestAStepMissingItsOwnFieldsRendersThemAsMissing(t *testing.T) {
	got := stepText(step(func(r *Record) { r.Fields = nil }), now)

	for _, key := range []string{"state", "item"} {
		if !strings.Contains(got, "(not said: "+key+")") {
			t.Errorf("a step with no %s field did not say so:\n%s", key, got)
		}
	}
}

// A STEP WITH NO NOTE IS STILL A SIGNAL. Section 39: "a step with a state and
// no note is still the signal that something moved." The absence is stated
// rather than left as a blank line, which would read as a truncated answer.
func TestAStepWithNoNoteSaysSoRatherThanPrintingABlank(t *testing.T) {
	got := stepText(step(func(r *Record) { r.Body = "" }), now)

	if !strings.Contains(got, "(no note)") {
		t.Errorf("a step with no note left a blank where the note goes:\n%s", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("the confirmation has a blank line in it, which reads as a "+
			"section that failed to render:\n%s", got)
	}
}

// THE PROVENANCE IS PRINTED, because a stream is only evidence if a reader can
// see which seat wrote each step.
func TestTheStepConfirmationSaysWhoWroteIt(t *testing.T) {
	got := stepText(step(), now)

	if !strings.Contains(got, "cli") {
		t.Errorf("the confirmation does not say which seat wrote the step:\n%s", got)
	}
	// ⛔ AND IT MUST NOT SAY THE SESSION. Ruled 2026-09-17 off a live
	// measurement: sixty puts at one terminal in one act wrote sixty distinct
	// sessions, because a session IS a connection today. To a human the field
	// asserts a grouping that does not exist. See provenanceLine in record.go
	// for the reopen condition - this is not a field that was forgotten.
	if strings.Contains(got, "s-4f2") {
		t.Errorf("the confirmation prints the session, which is minted per "+
			"INVOCATION and groups nothing a reader would mean:\n%s", got)
	}
}

// A STEP IS A RECORD AND --json EMITS THE RECORD OBJECT. One shape for one
// noun: a consumer that reads `rig record get --json` needs no second parser,
// and the two fields that make it a step are typed fields inside it.
func TestTheStepObjectIsTheRecordObject(t *testing.T) {
	serving(t, &fakeRecord{})

	out, err := captureStdout(t, func() error {
		return run([]string{
			"progress", "step", "01927-item",
			"--project", "rig", "--state", "done", "--json",
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("--json did not emit an object: %v\n%s", err, out)
	}
	for _, key := range []string{
		"id", "version", "kind", "project", "body",
		"fields", "provenance",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("the step object has no %q, so it is not the record "+
				"object a consumer already parses:\n%s", key, out)
		}
	}
	fields, ok := got["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields is not an object: %v", got["fields"])
	}
	if fields["state"] != "done" || fields["item"] != "01927-item" {
		t.Errorf("the two fields that make a record a step did not survive "+
			"into the object: %v", fields)
	}
}
