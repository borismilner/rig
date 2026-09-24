package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// A TRISTATE RENDERS THREE WAYS HERE, AND THAT IS THE WHOLE POINT OF DOING IT
// IN A NEW VERB.
//
// appsJSON renders four tristates as `== TRISTATE_YES`, so NO and UNSPECIFIED
// produce the same token and a reader cannot tell "the program said no" from
// "the program never said" - the one distinction section 21 gives every enum a
// zero to keep. That collapse is in SHIPPED output and is batched into B6.
// describe has no caller to break, so it does not inherit it, and this test is
// what stops it being inherited later by someone tidying three cases into two.
func TestATristateRendersAsThreeDistinctWordsAndNeverTwo(t *testing.T) {
	yes := tristateWord(rigv1.Tristate_TRISTATE_YES)
	no := tristateWord(rigv1.Tristate_TRISTATE_NO)
	unsaid := tristateWord(rigv1.Tristate_TRISTATE_UNSPECIFIED)

	for _, pair := range []struct{ a, b, why string }{
		{yes, no, "yes and no"},
		{no, unsaid, "a declared no and an unsaid field"},
		{yes, unsaid, "yes and an unsaid field"},
	} {
		if pair.a == pair.b {
			t.Errorf("%s render identically as %q", pair.why, pair.a)
		}
	}

	// The unsaid case must not render as a blank or a dash either. A reader
	// skimming a column of yes and no reads an empty cell as a rendering gap
	// rather than as the program's silence.
	switch strings.TrimSpace(unsaid) {
	case "", "-", "--":
		t.Errorf("an unsaid tristate renders as %q, which reads as a hole in "+
			"the renderer rather than as a fact about the program", unsaid)
	}
}

// A PLAIN BOOL IS NOT A TRISTATE AND MUST NOT BORROW ITS WORDS.
//
// dry_run and promote are `bool` on the wire, so false genuinely means false
// and there is no third state. Rendering them with the tristate's vocabulary
// would tell a reader a distinction exists where none does, which is the same
// error as the collapse, pointing the other way.
func TestAPlainBoolDoesNotBorrowTheTristatesVocabulary(t *testing.T) {
	tri := map[string]bool{
		tristateWord(rigv1.Tristate_TRISTATE_YES):         true,
		tristateWord(rigv1.Tristate_TRISTATE_NO):          true,
		tristateWord(rigv1.Tristate_TRISTATE_UNSPECIFIED): true,
	}
	for _, w := range []string{boolWord(true), boolWord(false)} {
		if tri[w] {
			t.Errorf("a bool renders as %q, which is one of the tristate's own "+
				"words and implies a third state that does not exist", w)
		}
	}
	if boolWord(true) == boolWord(false) {
		t.Fatal("both bool values render the same")
	}
}

// Every value of every enum describe renders gets a word of its own, walked
// off the descriptor so a value added to the proto is covered the day it lands.
func TestEveryEnumValueDescribeRendersGetsAWordOfItsOwn(t *testing.T) {
	for _, tc := range []struct {
		name string
		word func(int32) string
		n    int
	}{
		{
			name: "Effects",
			word: func(n int32) string { return effectsWord(rigv1.Effects(n)) },
			n:    rigv1.Effects(0).Descriptor().Values().Len(),
		},
		{
			name: "Duration",
			word: func(n int32) string { return durationWord(rigv1.Duration(n)) },
			n:    rigv1.Duration(0).Descriptor().Values().Len(),
		},
		{
			name: "Shape",
			word: func(n int32) string { return shapeWord(rigv1.Shape(n)) },
			n:    rigv1.Shape(0).Descriptor().Values().Len(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[string]int32{}
			var checked int
			for i := range int32(tc.n) {
				w := tc.word(i)
				checked++
				if prev, dup := seen[w]; dup {
					t.Errorf("%s values %d and %d both render as %q",
						tc.name, prev, i, w)
				}
				seen[w] = i
			}
			// The positive control: every assertion is inside the loop, so a
			// descriptor that reported no values would read as a clean pass.
			if checked < 2 {
				t.Fatalf("walked %d values of %s; a walk that saw fewer than "+
					"two proved nothing", checked, tc.name)
			}
		})
	}
}

// THE ZERO IS "NOTHING WAS SAID" FOR EVERY ENUM SECTION 21 COVERS, and it must
// not read as a value the program chose. EFFECTS_UNSPECIFIED is not an effect.
func TestTheZeroOfEveryEnumReadsAsSilenceRatherThanAsAChoice(t *testing.T) {
	for name, got := range map[string]string{
		"Effects":  effectsWord(rigv1.Effects_EFFECTS_UNSPECIFIED),
		"Duration": durationWord(rigv1.Duration_DURATION_UNSPECIFIED),
		"Shape":    shapeWord(rigv1.Shape_SHAPE_UNSPECIFIED),
	} {
		if !strings.Contains(got, "not said") {
			t.Errorf("%s's zero renders as %q, which reads as something the "+
				"program declared rather than as its silence", name, got)
		}
		// And it must not render as the enum's own spelling, which is what
		// makes "unspecified" look like a value in a column of values.
		if strings.Contains(got, "unspecified") {
			t.Errorf("%s's zero renders as %q, which is the wire's spelling "+
				"rather than a word for a reader", name, got)
		}
	}
}

// A value this build has no word for is SKEW, and it must not fall back onto
// the zero's rendering or onto a bare digit.
//
// The generated String() returns the DECIMAL for such a value, so the obvious
// implementation prints "9" in a column of words - skew DISCOVERED by a person
// squinting at output, where section 37's precondition 3 requires it DETECTED.
func TestAnEnumValueThisBuildDoesNotKnowIsReportedAsSkew(t *testing.T) {
	for name, got := range map[string]string{
		"Effects":  effectsWord(rigv1.Effects(99)),
		"Duration": durationWord(rigv1.Duration(99)),
		"Shape":    shapeWord(rigv1.Shape(99)),
		"Tristate": tristateWord(rigv1.Tristate(99)),
	} {
		if !strings.Contains(got, "unrecognised") {
			t.Errorf("%s value 99 renders as %q, which does not say it was "+
				"not understood", name, got)
		}
		if !strings.Contains(got, "99") {
			t.Errorf("%s value 99 renders as %q and never names the number, "+
				"which is the only actionable thing in it", name, got)
		}
		if strings.Contains(got, "not said") {
			t.Errorf("%s value 99 renders as %q, which reports skew as the "+
				"zero's meaning", name, got)
		}
	}
}

// DESCRIBE ON A PROGRAM RETURNS ITS PREAMBLE. That is the slice's own demo
// clause, and the field was dead on every path until 6c71712.
func TestDescribeOnAProgramReturnsItsPreamble(t *testing.T) {
	const preamble = "read the shelf before reindexing it, because reindex " +
		"rewrites paths in place"
	out := describeProgram(&registryv1.Program{
		Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.2.0"},
		Preamble: preamble,
		Coverage: rigv1.Coverage_COVERAGE_FULL,
	})
	// Wrapped for a terminal, so the whole sentence is not on one line. The
	// words are what must survive.
	for _, word := range strings.Fields(preamble) {
		if !strings.Contains(out, word) {
			t.Fatalf("the preamble word %q is missing from describe's output:\n%s",
				word, out)
		}
	}
}

// A PROGRAM THAT DECLARED NO PREAMBLE SAYS SO. describe exists to deliver it,
// so silence reads as "describe is broken" rather than as "this program
// declared none" - the estate verb's distinction arriving through prose.
func TestAProgramWithNoPreambleSaysSoRatherThanPrintingNothing(t *testing.T) {
	out := describeProgram(&registryv1.Program{
		Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.2.0"},
		Coverage: rigv1.Coverage_COVERAGE_PARTIAL,
	})
	if !strings.Contains(out, "no preamble") {
		t.Errorf("a program with no preamble renders without saying so:\n%s", out)
	}
	// Section 5k: the coverage still has to travel, because this surface
	// renders a whole program and is the one most mistaken for all of it.
	if !strings.Contains(out, "coverage is") {
		t.Errorf("describe on a program does not carry its coverage:\n%s", out)
	}
}

// DESCRIBE ON A COMMAND RETURNS THE FULL DECLARATION WITH EXAMPLES AND
// EFFECTS - the other half of the slice's demo clause, asserted field by field
// rather than by eye.
func TestDescribeOnACommandRendersEveryDeclaredProperty(t *testing.T) {
	out := describeCommand(
		&registryv1.Program{Identity: &rigv1.Identity{Id: "fakeapp"}},
		&rigv1.Command{
			Id:            "reindex",
			Summary:       "rebuild the index",
			Description:   "walks every shelf and rewrites the index in place",
			Args:          []byte(`{"type":"object","properties":{"since":{"type":"string","description":"how far back"}}}`),
			Examples:      []string{"rig fakeapp reindex --since 7d"},
			Preconditions: []string{"the shelf is not being written"},
			Returns:       "the number of entries rewritten",
			Cost:          "one full walk of the shelf",
			Effects:       rigv1.Effects_EFFECTS_WRITES_FILES,
			Duration:      rigv1.Duration_DURATION_MINUTES,
			Shape:         rigv1.Shape_SHAPE_UNARY,
			Idempotent:    rigv1.Tristate_TRISTATE_YES,
			Interactive:   rigv1.Tristate_TRISTATE_NO,
			Confirms:      rigv1.Tristate_TRISTATE_NO,
			DryRun:        true,
			Promote:       true,
			Sensitive:     &rigv1.SensitiveFields{Pointers: []string{"/token"}},
		})

	for _, want := range []string{
		"rig fakeapp reindex",         // the invocation, spelled as it is typed
		"rebuild the index",           // summary
		"rewrites the index in place", // description
		"--since",                     // the declared argument
		"how far back",                // its declared documentation
		"writes-files",                // effects
		"minutes",                     // duration
		"unary",                       // shape
		"idempotent",                  // the property labels themselves
		"interactive",
		"needs display",
		"confirms",
		"dry run",
		"promoted",
		"one full walk of the shelf",      // cost
		"the number of entries rewritten", // returns
		"the shelf is not being written",  // preconditions
		"rig fakeapp reindex --since 7d",  // examples
		"/token",                          // sensitive fields
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe on a command does not carry %q:\n%s", want, out)
		}
	}

	// `streams` and `needs_display` were never set on this fixture, so they
	// are the unsaid case - and describe must still render them, because a
	// field a program left unsaid is a fact about the program.
	if !strings.Contains(out, "not said") {
		t.Errorf("a command that left fields unsaid renders nothing for them, "+
			"so a reader cannot tell an unsaid field from one describe "+
			"forgot:\n%s", out)
	}
}

// A COMMAND WITH NO SENSITIVE FIELDS SAYS SO. "This declares none" and "nobody
// looked" are different facts, and a reader deciding what may be logged needs
// the first out loud rather than inferred from a missing heading.
func TestACommandWithNoSensitiveFieldsSaysSoRatherThanOmittingTheHeading(t *testing.T) {
	out := describeCommand(
		&registryv1.Program{Identity: &rigv1.Identity{Id: "fakeapp"}},
		&rigv1.Command{Id: "status", Summary: "say what is going on"})
	if !strings.Contains(out, "no sensitive fields") {
		t.Errorf("a command with no sensitive fields renders without saying "+
			"so:\n%s", out)
	}
	if !strings.Contains(out, "takes no arguments") {
		t.Errorf("a command with no arguments renders without saying so:\n%s", out)
	}
}

// DESCRIBE IS NOT `--help`, and the difference is testable rather than a
// claim in a comment: help renders the usable subset, describe renders every
// declared property including the ones help has no reason to show.
func TestDescribeCarriesPropertiesThatHelpDoesNotBother(t *testing.T) {
	cmd := &rigv1.Command{
		Id: "reindex", Summary: "rebuild the index",
		Shape: rigv1.Shape_SHAPE_UNARY, Promote: true,
		Streams: rigv1.Tristate_TRISTATE_NO,
	}
	out := describeCommand(
		&registryv1.Program{Identity: &rigv1.Identity{Id: "fakeapp"}}, cmd)

	// These three are declared properties that generated help never prints.
	for _, want := range []string{"shape", "promoted", "streams"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe does not render %q, so it is not carrying more "+
				"than help does:\n%s", want, out)
		}
	}
}

// The argv path, which is what nothing else in this package covers and what
// the --depth defect proved unit tests cannot see.
func TestDescribeRefusesTheArgumentCountsItCannotAnswer(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"nothing to describe", nil},
		{"three positionals", []string{"fakeapp", "reindex", "extra"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := cmdDescribe(tc.argv)
			if err == nil {
				t.Fatalf("rig describe %v was accepted", tc.argv)
			}
			if !strings.Contains(err.Error(), "usage:") {
				t.Errorf("refused with %q, which does not say how to call it", err)
			}
		})
	}
}

// `--json` PRINTS WHAT rig.describe ANSWERED, BYTE FOR BYTE, AND ASKS FOR
// WHAT WAS TYPED. The daemon renders the MCP tool's object (B10), so the
// client's whole job is to send the address and print the answer unchanged:
// a reshaped answer is a second renderer, and a dropped command describes the
// wrong thing.
func TestDescribeJSONPrintsTheDaemonsObjectUnchanged(t *testing.T) {
	const object = `{"tool":"describe","depth":"full","program":{"identity":{"id":"fakeapp"}}}`
	d := startFakeDaemon(t, &verbsv1.DescribeResponse{Answer: []byte(object)})
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := captureStdout(t, func() error {
		return describeJSON(ctx, c, "fakeapp", "reindex")
	})
	if err != nil {
		t.Fatalf("describeJSON: %v", err)
	}
	if out != object+"\n" {
		t.Errorf("printed %q, want the daemon's bytes unchanged:\n%q", out, object+"\n")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) != 1 {
		t.Fatalf("the daemon was sent %d requests, want 1", len(d.requests))
	}
	f := d.requests[0]
	if f.GetMethod() != "rig.describe" {
		t.Errorf("sent %q, want rig.describe", f.GetMethod())
	}
	var req verbsv1.DescribeRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		t.Fatal(err)
	}
	if req.GetProgram() != "fakeapp" || req.GetCommand() != "reindex" {
		t.Errorf("asked for %q/%q, want fakeapp/reindex", req.GetProgram(), req.GetCommand())
	}
}

// A wrapped value's continuation lines are indented to the value column.
// Measured on the rendered output rather than derived from describeColumn, so
// moving the constant cannot move both sides of the assertion at once.
func TestALongValueWrapsIntoItsOwnColumnRatherThanBackToTheMargin(t *testing.T) {
	out := describeCommand(
		&registryv1.Program{Identity: &rigv1.Identity{Id: "fakeapp"}},
		&rigv1.Command{
			Id: "reindex", Summary: "rebuild",
			Returns: "The window it used, how many items it indexed, and " +
				"whether it was a dry run, which is a sentence long enough " +
				"to need more than one line in a terminal.",
		})

	lines := strings.Split(out, "\n")
	var start, wrapped int
	for i, l := range lines {
		if !strings.HasPrefix(l, "returns") {
			continue
		}
		rest := l[len("returns"):]
		start = len("returns") + len(rest) - len(strings.TrimLeft(rest, " "))
		// The next line is the wrapped continuation.
		if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
			wrapped = len(lines[i+1]) - len(strings.TrimLeft(lines[i+1], " "))
		}
		break
	}
	if start == 0 {
		t.Fatalf("no returns row was rendered:\n%s", out)
	}
	if wrapped != start {
		t.Errorf("the returns value starts at column %d and its next line at "+
			"column %d: a continuation at the margin reads as a new label "+
			"rather than as more of the same field:\n%s", start, wrapped, out)
	}
}
