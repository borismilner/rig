package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The functions under test here are the ones that turn a declaration into the
// text a person or an agent reads: the labels on a --help page and the object
// behind --json. They were the untested half of this package, and not because
// they are hard to reach - none of them needs a running rigd. What they need
// is someone to say what the rendering must never do.

// An unsaid property must not render as the permissive answer. Section 21's
// rule is that enum zero means "the sender did not say", and every label below
// is a place where reading zero as a decision would be silent.

func TestAnUnsaidCoverageDoesNotReadAsFull(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    *rigv1.Program
		want string
	}{
		{"unspecified", &rigv1.Program{}, "coverage?"},
		{"absent program", nil, "coverage?"},
		{"partial", &rigv1.Program{Coverage: rigv1.Coverage_COVERAGE_PARTIAL}, "partial"},
		{"full", &rigv1.Program{Coverage: rigv1.Coverage_COVERAGE_FULL}, "full"},
	} {
		if got := coverageLabel(tc.p); got != tc.want {
			t.Errorf("%s: coverageLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAnUnsaidIdempotenceReadsAsNotSafe(t *testing.T) {
	// Only an explicit yes may say "safe to re-run". A program that never
	// answered the question and a program that answered no have to read the
	// same way, because the cost of being wrong is one-directional.
	for _, tc := range []struct {
		name string
		c    *rigv1.Command
		want string
	}{
		{"unspecified", &rigv1.Command{}, "NOT safe to re-run"},
		{"absent command", nil, "NOT safe to re-run"},
		{"no", &rigv1.Command{Idempotent: rigv1.Tristate_TRISTATE_NO}, "NOT safe to re-run"},
		{"yes", &rigv1.Command{Idempotent: rigv1.Tristate_TRISTATE_YES}, "safe to re-run"},
	} {
		if got := idempotentLabel(tc.c); got != tc.want {
			t.Errorf("%s: idempotentLabel = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Section 5e writes its vocabulary as read-only, not READ_ONLY and not
// read_only, and a house rule and a --help page are both read against that
// spelling. This walks the generated name maps rather than a list written
// here, so an enum member added later is covered the day it is added.

func TestEveryEffectsValueRendersInThePlansVocabulary(t *testing.T) {
	for v, name := range rigv1.Effects_name {
		got := effectsLabel(&rigv1.Command{Effects: rigv1.Effects(v)})
		assertPlanVocabulary(t, name, got)
	}
}

func TestEveryDurationValueRendersInThePlansVocabulary(t *testing.T) {
	for v, name := range rigv1.Duration_name {
		got := durationLabel(&rigv1.Command{Duration: rigv1.Duration(v)})
		assertPlanVocabulary(t, name, got)
	}
}

func assertPlanVocabulary(t *testing.T, from, got string) {
	t.Helper()
	switch {
	case got == "":
		t.Errorf("%s rendered as the empty string, which reads as a missing field", from)
	case strings.Contains(got, "_"):
		t.Errorf("%s rendered as %q: section 5e hyphenates, it does not use underscores", from, got)
	case got != strings.ToLower(got):
		t.Errorf("%s rendered as %q: section 5e is lower case", from, got)
	case strings.HasPrefix(got, "effects-"), strings.HasPrefix(got, "duration-"):
		t.Errorf("%s rendered as %q with its prefix still attached", from, got)
	}
}

func TestEnumLabelLeavesAStringThatDoesNotCarryThePrefix(t *testing.T) {
	// TrimPrefix is a no-op on a mismatch rather than an error, so a caller
	// that passes the wrong prefix gets a lowered name and not a truncated
	// one. Locked because a truncating version would silently rename values.
	if got := enumLabel("EFFECTS_READ_ONLY", "DURATION_"); got != "effects-read-only" {
		t.Fatalf("enumLabel with a mismatched prefix = %q", got)
	}
}

func TestCommandIDsAreSortedAndAnEmptyProgramSaysNothing(t *testing.T) {
	p := &rigv1.Program{Commands: []*rigv1.Command{
		{Id: "reindex"}, {Id: "purge"}, {Id: "audit"},
	}}
	if got, want := commandIDs(p), []string{"audit", "purge", "reindex"}; !reflect.DeepEqual(got, want) {
		t.Errorf("commandIDs = %v, want %v", got, want)
	}

	// "nothing" is a placeholder for a program that declared no commands, and
	// it has to be one no program could also declare. A bare word here would
	// make an empty program indistinguishable from one command called
	// nothing, so if this ever stops being a placeholder the test should say
	// so rather than the completion list should.
	if got, want := commandIDs(&rigv1.Program{}), []string{"nothing"}; !reflect.DeepEqual(got, want) {
		t.Errorf("commandIDs of an empty program = %v, want %v", got, want)
	}
}

func TestRawSchemaKeepsTheDeclaredBytesExactly(t *testing.T) {
	// The declared schema is what an agent validates against, so key order,
	// number spelling and unknown keywords all have to survive. Re-encoding
	// through a map loses all three, which is why this returns a
	// json.RawMessage. Asserted on the marshalled text, because that is what
	// a caller of --json actually reads.
	decl := []byte(`{"type":"object","properties":{"since":{"type":"string"},"workers":{"type":"integer","minimum":1.0}},"x-vendor":true}`)

	out, err := json.Marshal(map[string]any{"args": rawSchema(decl)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(out, decl) {
		t.Fatalf("the declared schema did not survive verbatim.\n got: %s\nwant it to contain: %s", out, decl)
	}
}

func TestRawSchemaSaysSoRatherThanEmittingBrokenJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want any
	}{
		{"absent", "", nil},
		{"whitespace only", "   \n\t ", nil},
		{"not JSON at all", "{oops", "unreadable: the declared schema is not JSON"},
		{"truncated", `{"type":"obj`, "unreadable: the declared schema is not JSON"},
	} {
		got := rawSchema([]byte(tc.in))
		if tc.want == nil {
			if got != nil {
				t.Errorf("%s: rawSchema = %v, want nil so the key is omitted", tc.name, got)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("%s: rawSchema = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPointersIsEmptyRatherThanNull(t *testing.T) {
	// Section 5e refuses to let an absence carry a meaning by accident, and
	// null reads as "this program never considered the question" where [] says
	// "it considered it and there are none".
	for _, tc := range []struct {
		name string
		in   *rigv1.SensitiveFields
	}{
		{"absent wrapper", nil},
		{"present and empty", &rigv1.SensitiveFields{}},
	} {
		got := pointers(tc.in)
		if got == nil {
			t.Errorf("%s: pointers returned nil, which marshals to null", tc.name)
		}
		if len(got) != 0 {
			t.Errorf("%s: pointers = %v, want empty", tc.name, got)
		}
	}
	if got := pointers(&rigv1.SensitiveFields{Pointers: []string{"token"}}); !reflect.DeepEqual(got, []string{"token"}) {
		t.Errorf("pointers dropped a declared pointer, got %v", got)
	}
}

func TestAppsJSONNeverEmitsNullAndOmitsWhatWasNotDeclared(t *testing.T) {
	ps := []*rigv1.Program{{
		Identity: &rigv1.Identity{Id: "fakeapp", Version: "0.1.0"},
		Coverage: rigv1.Coverage_COVERAGE_PARTIAL,
		Commands: []*rigv1.Command{
			{Id: "purge", Effects: rigv1.Effects_EFFECTS_DESTRUCTIVE},
			{Id: "reindex", Args: []byte(`{"type":"object"}`)},
		},
	}}

	out, err := json.Marshal(appsJSON(ps))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(out)

	// null anywhere in this document is the defect: every field either
	// carries what was declared or is left out, and section 5e's whole point
	// is that those two are not the same thing.
	if strings.Contains(text, "null") {
		t.Errorf("appsJSON emitted null:\n%s", text)
	}
	// sensitive is the field the comment in main.go promises is present and
	// empty rather than absent, on every command including one that declared
	// nothing about it.
	if n := strings.Count(text, `"sensitive":[]`); n != 2 {
		t.Errorf("want sensitive present and empty on both commands, saw %d:\n%s", n, text)
	}
	// args is the opposite promise: omitted for a command that declares no
	// schema, because that is not the same as a schema nobody wrote down.
	if n := strings.Count(text, `"args"`); n != 1 {
		t.Errorf("want args on the one command that declared a schema, saw %d:\n%s", n, text)
	}
}

func TestAppsJSONRendersAnUnsaidTristateAsFalse(t *testing.T) {
	// A tristate has three states and JSON gets two, so the mapping has to be
	// "yes, or not yes". Reading unspecified as true would turn a program
	// that never answered into one that promised something.
	rows := appsJSON([]*rigv1.Program{{
		Identity: &rigv1.Identity{Id: "fakeapp"},
		Commands: []*rigv1.Command{{Id: "purge"}},
	}})
	cmd := rows[0]["commands"].([]map[string]any)[0]

	for _, key := range []string{"idempotent", "needs_display", "interactive", "confirms"} {
		v, ok := cmd[key]
		if !ok {
			t.Errorf("%s is missing, so a reader cannot tell it was not declared", key)
			continue
		}
		if v != false {
			t.Errorf("%s = %v for an undeclared tristate, want false", key, v)
		}
	}
}

func TestPluralAgreesWithItsCount(t *testing.T) {
	for n, want := range map[int]string{0: "s", 1: "", 2: "s", 17: "s"} {
		if got := plural(n); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFirstNonEmptySkipsPastEmptyStrings(t *testing.T) {
	if got := firstNonEmpty("", "", "title", "summary"); got != "title" {
		t.Errorf("firstNonEmpty = %q, want title", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("firstNonEmpty with nothing to pick = %q, want the empty string", got)
	}
}
