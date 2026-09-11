package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Section 10 says --json returns "exactly what the MCP tool returns", and the
// two objects are NOT the same object today. These tests are the honest
// enforcement of that sentence: they cannot assert the two are identical,
// because they are not, so they assert that the gap is EXACTLY this and can
// never widen.
//
// Measured 2026-09-11, one declaration through both renderers: the CLI emits a
// bare array where the MCP tool emits a {tool, partial, estate} envelope, it
// flattens identity, it spells its keys in snake_case where the MCP tool uses
// camelCase, it renders tristates as booleans rather than as names, and it
// carries 10 of a command's 20 declared fields. That is not drift. The two
// were never the same object.
//
// Closing it means changing `rig apps list --json`, which is shipped and
// agent-parsed, so it is a decision about a published contract rather than a
// repair. Until that decision exists, a field added to the wire that the CLI
// does not carry fails the second test below, which is the whole benefit
// section 10 was asking for and costs no change to any output.

// Every declared field has to be retrievable, and the meta object is where
// that is true.
//
// Section 35's rule, from two fields found accepted-and-unreadable on one day:
// acceptance is not a contract, retrievability is. A declaration field no
// surface can return is a defect, so the object both agent-facing surfaces
// answer through has to carry all of them.
func TestEveryDeclaredFieldIsRetrievableThroughTheMetaObject(t *testing.T) {
	got := renderedFacts(t, metaJSONOf(t, kernelFixture(t)))
	want := declaredFacts()

	for _, fact := range want {
		if !slices.Contains(got, fact) {
			t.Errorf("the declaration carries %s and the meta object does not "+
				"return it, so a program can declare it and no agent can ever "+
				"read it (section 35: acceptance is not a contract, "+
				"retrievability is)", fact)
		}
	}
}

// The CLI's --json falls short of the meta object by exactly this much.
//
// Enumerated rather than asserted away. Each name below is a fact a program
// declared, the MCP tool returns, and `rig apps list --json` drops. The list
// is allowed to SHRINK only by someone deciding that the CLI carries the
// field, and it must never grow.
var cliDoesNotCarry = []string{
	// Identity, flattened to id and version. A name, an icon and a one-line
	// description are what a human-facing list would use and the CLI is the
	// surface that has neither.
	"identity.name",
	"identity.icon",
	"identity.description",

	// The program half of section 9's tiering argument. The preamble is the
	// one document an agent reads first, and slice 2 exists to deliver it.
	"preamble",
	"services",
	"elements",
	"hosted",
	"pane_url",

	// The command half. examples is the one that also feeds fix_command, and
	// shape and streams are what an agent needs in order to decide HOW to
	// call something rather than whether it may.
	"commands.title",
	"commands.examples",
	"commands.streams",
	"commands.shape",
	"commands.description",
	"commands.returns",
	"commands.dry_run",
	"commands.cost",
	"commands.preconditions",
	"commands.promote",
}

// cliFlattens is the CLI's identity spelling, mapped rather than counted as a
// gap.
//
// `rig apps list --json` puts a program's id and version at the top level
// where the meta object nests them under identity. The fact IS retrievable, in
// a different place, so it is not a missing field - and the spelling
// difference is one of the divergences named at the top of this file, which is
// where it belongs rather than in a gap list it would misdescribe.
var cliFlattens = map[string]string{
	"id":      "identity.id",
	"version": "identity.version",
}

func TestTheCLIsJSONFallsShortOfTheMetaObjectByExactlyThisMuch(t *testing.T) {
	declared := declaredFacts()

	var cli []string
	for _, fact := range renderedFacts(t, cliJSONOf(t, wireFixture(t))) {
		if nested, ok := cliFlattens[fact]; ok {
			fact = nested
		}
		cli = append(cli, fact)
	}

	for _, fact := range declared {
		if slices.Contains(cli, fact) {
			continue
		}
		if !slices.Contains(cliDoesNotCarry, fact) {
			t.Errorf("the CLI's --json does not carry %s, and the gap just GREW: "+
				"section 10 binds this output to what the MCP tool returns, so "+
				"either the CLI renders it or cliDoesNotCarry says out loud that "+
				"it does not", fact)
		}
	}

	for _, fact := range cliDoesNotCarry {
		switch {
		case slices.Contains(cli, fact):
			t.Errorf("cliDoesNotCarry lists %s and the CLI carries it: the gap "+
				"shrank, which is the good direction, and the list has to say so",
				fact)
		case !slices.Contains(declared, fact):
			// Otherwise a field removed from the wire leaves an entry here
			// forever, and a gap list with dead entries in it is the same
			// rotted document this test exists to prevent.
			t.Errorf("cliDoesNotCarry lists %s and no program can declare it any "+
				"more, so the entry is stale and describes a gap that is gone",
				fact)
		}
	}
}

// The CLI renders four tristates as JSON booleans, and that is only safe
// because the kernel refuses a declaration that leaves one unsaid.
//
// appsJSON compares each against TRISTATE_YES, so NO and UNSPECIFIED produce
// the same token. Measured 2026-09-11: a command with confirms=NO and one with
// confirms=UNSPECIFIED render byte-identically, so an agent reading `false`
// cannot tell "the program said no" from "the program never said" - the one
// thing section 21 gives every enum a zero in order to keep apart.
//
// It is not reachable today, and that is the point of this test rather than a
// reason not to write it. The protection lives entirely in kernel.Command's
// mandatory set, which is another seat's file, and NOTHING in this package
// recorded the dependency. Relax that set - `confirms` is already under a
// ruling of its own - and this client starts losing information with no test
// anywhere noticing. The import is what stops the two drifting in silence,
// and it is test-only: TestTheClientLinksNeitherTheDaemonNorItsValidator
// proves the binary links none of it.
func TestTheCLIsBooleanTristatesAreSafeOnlyBecauseTheKernelRefusesUnsaid(t *testing.T) {
	// The four appsJSON renders as booleans. streams is declared and the CLI
	// does not carry it at all, which is cliDoesNotCarry's business.
	for _, field := range []string{"Idempotent", "NeedsDisplay", "Interactive", "Confirms"} {
		t.Run(field, func(t *testing.T) {
			decl := declarationFixture()

			// The control first: the fixture has to be valid, or a refusal
			// below says nothing about the field under test.
			if err := decl.Validate(); err != nil {
				t.Fatalf("the fixture is not a valid declaration, so this test "+
					"cannot attribute any refusal to %s: %v", field, err)
			}

			cmd := reflect.ValueOf(&decl.Commands[0]).Elem().FieldByName(field)
			if !cmd.IsValid() {
				t.Fatalf("kernel.Command has no field %s any more, so the CLI's "+
					"boolean rendering of it is unchecked", field)
			}
			cmd.Set(reflect.ValueOf(kernel.Unsaid))

			err := decl.Validate()
			if err == nil {
				t.Fatalf("the kernel now ACCEPTS a command with %s unsaid, and "+
					"appsJSON renders it as `false` - identical to a program that "+
					"said no. Either the CLI stops rendering it as a boolean or "+
					"the kernel keeps refusing it (section 21)", field)
			}
			if want := wireName(field); !strings.Contains(err.Error(), want) {
				t.Errorf("the kernel refused the declaration but did not name %q, "+
					"so the refusal may be about something else and this test is "+
					"not measuring %s: %v", want, field, err)
			}
		})
	}
}

// ---- fixtures, and the guards that stop them going blind -------------------

// declaredFacts is every fact a program can declare, named as the wire names
// it, read off the descriptors rather than listed by hand.
//
// Read off the proto because that is the source: a field added to the wire is
// a field an agent must be able to read, and a hand-written list is how this
// test would quietly stop covering one.
func declaredFacts() []string {
	var out []string
	program := (&rigv1.Program{}).ProtoReflect().Descriptor()
	for i := range program.Fields().Len() {
		f := program.Fields().Get(i)
		switch f.Name() {
		case "identity":
			out = append(out, prefixed("identity", (&rigv1.Identity{}).ProtoReflect().Descriptor())...)
		case "commands":
			out = append(out, prefixed("commands", (&rigv1.Command{}).ProtoReflect().Descriptor())...)
		default:
			out = append(out, string(f.Name()))
		}
	}
	return out
}

func prefixed(prefix string, m protoreflect.MessageDescriptor) []string {
	out := make([]string, 0, m.Fields().Len())
	for i := range m.Fields().Len() {
		out = append(out, prefix+"."+string(m.Fields().Get(i).Name()))
	}
	return out
}

// renderedFacts flattens one rendered program into the same wire names
// declaredFacts uses, so the two renderers are comparable despite spelling
// their keys differently. camelCase is folded to snake_case for that reason
// and no other.
func renderedFacts(t *testing.T, program map[string]any) []string {
	t.Helper()
	var out []string
	for k, v := range program {
		name := snake(k)
		switch name {
		case "identity":
			for sub := range v.(map[string]any) {
				out = append(out, "identity."+snake(sub))
			}
		case "commands":
			first, ok := v.([]any)
			if !ok || len(first) == 0 {
				t.Fatalf("the rendered object has no commands, so every "+
					"commands.* fact below would read as missing: %v", v)
			}
			for sub := range first[0].(map[string]any) {
				out = append(out, "commands."+snake(sub))
			}
		default:
			out = append(out, name)
		}
	}
	// The guard: a renderer that answered with nothing useful must not read as
	// a renderer that dropped everything.
	if !slices.Contains(out, "id") && !slices.Contains(out, "identity.id") {
		t.Fatalf("the rendered object carries no program id at all, so it is "+
			"not a program and this comparison proved nothing: %v", out)
	}
	return out
}

func metaJSONOf(t *testing.T, p kernel.Program) map[string]any {
	t.Helper()
	raw, err := meta.MarshalAnswer(meta.Answer{
		Tool: meta.List, Estate: []kernel.Program{p}, Version: "test",
	})
	if err != nil {
		t.Fatalf("the meta object did not render, so nothing was compared: %v", err)
	}
	var envelope struct {
		Estate []map[string]any `json:"estate"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("the meta object is not readable JSON: %v\n%s", err, raw)
	}
	if len(envelope.Estate) != 1 {
		t.Fatalf("the meta object carries %d programs and one was rendered, so "+
			"the envelope is not what this test reads: %s", len(envelope.Estate), raw)
	}
	return envelope.Estate[0]
}

func cliJSONOf(t *testing.T, p *rigv1.Program) map[string]any {
	t.Helper()
	rows := appsJSON([]*rigv1.Program{p})
	if len(rows) != 1 {
		t.Fatalf("appsJSON rendered %d rows from one program", len(rows))
	}
	// Through JSON rather than reading the map directly, so omitempty and any
	// other marshalling behaviour is included in what is measured.
	raw, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatalf("the CLI object did not render: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the CLI object is not readable JSON: %v\n%s", err, raw)
	}
	return out
}

// kernelFixture is one program with every field set, and it REFUSES to be
// incomplete.
//
// Every field has to be non-zero because the meta object omits empties, so an
// unset field and an unrendered field look the same. The guard below is what
// stops that reading as "meta does not carry this": a field added to
// kernel.Program fails here, naming the fixture, rather than producing a false
// accusation against another seat's renderer.
func kernelFixture(t *testing.T) kernel.Program {
	t.Helper()
	p := kernel.Program{
		Identity: kernel.Identity{
			ID: "fakeapp", Name: "Fake App", Version: "1.2.0",
			Icon: "box", Description: "the reference program",
		},
		Coverage:     kernel.CoverageFull,
		CoverageNote: "everything but streams",
		SemanticsGen: 3,
		Services:     []string{"index"},
		Elements:     []string{"rigTable"},
		Hosted:       true,
		PaneURL:      "http://127.0.0.1:9/pane",
		Preamble:     "read this first",
		Commands:     []kernel.Command{kernelCommandFixture()},
	}
	allSet(t, "kernel.Program", p, "Commands")
	allSet(t, "kernel.Identity", p.Identity)
	allSet(t, "kernel.Command", p.Commands[0])
	return p
}

func kernelCommandFixture() kernel.Command {
	return kernel.Command{
		ID: "reindex", Title: "Reindex",
		Args:          []byte(`{"type":"object"}`),
		Examples:      []string{"rig fakeapp reindex --since 7d"},
		Effects:       kernel.EffectsReadOnly,
		Idempotent:    kernel.Yes,
		Sensitive:     []string{"/token"},
		Interactive:   kernel.No,
		Streams:       kernel.No,
		NeedsDisplay:  kernel.No,
		Duration:      kernel.DurationSeconds,
		Confirms:      kernel.No,
		Shape:         kernel.ShapeUnary,
		Summary:       "rebuild the index",
		Description:   "walks the tree and rebuilds",
		Returns:       "a count",
		DryRun:        true,
		Cost:          "cheap",
		Preconditions: []string{"the tree exists"},
		Promote:       true,
	}
}

// declarationFixture is the same program as something registration would
// accept, for the tristate test.
func declarationFixture() kernel.Declaration {
	return kernel.Declaration{
		Identity: kernel.Identity{ID: "fakeapp", Version: "1.2.0"},
		Coverage: kernel.CoverageFull, SemanticsGen: 3,
		Commands: []kernel.Command{kernelCommandFixture()},
	}
}

// wireFixture mirrors kernelFixture on the wire, with the same guard.
func wireFixture(t *testing.T) *rigv1.Program {
	t.Helper()
	p := &rigv1.Program{
		Identity: &rigv1.Identity{
			Id: "fakeapp", Name: "Fake App", Version: "1.2.0",
			Icon: "box", Description: "the reference program",
		},
		Coverage:     rigv1.Coverage_COVERAGE_FULL,
		CoverageNote: "everything but streams",
		SemanticsGen: 3,
		Services:     []string{"index"},
		Elements:     []string{"rigTable"},
		Hosted:       true,
		PaneUrl:      "http://127.0.0.1:9/pane",
		Preamble:     "read this first",
		Commands: []*rigv1.Command{{
			Id: "reindex", Title: "Reindex",
			Args:          []byte(`{"type":"object"}`),
			Examples:      []string{"rig fakeapp reindex --since 7d"},
			Effects:       rigv1.Effects_EFFECTS_READ_ONLY,
			Idempotent:    rigv1.Tristate_TRISTATE_YES,
			Sensitive:     &rigv1.SensitiveFields{Pointers: []string{"/token"}},
			Interactive:   rigv1.Tristate_TRISTATE_NO,
			Streams:       rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay:  rigv1.Tristate_TRISTATE_NO,
			Duration:      rigv1.Duration_DURATION_SECONDS,
			Confirms:      rigv1.Tristate_TRISTATE_NO,
			Shape:         rigv1.Shape_SHAPE_UNARY,
			Summary:       "rebuild the index",
			Description:   "walks the tree and rebuilds",
			Returns:       "a count",
			DryRun:        true,
			Cost:          "cheap",
			Preconditions: []string{"the tree exists"},
			Promote:       true,
		}},
	}
	allPopulated(t, p)
	allPopulated(t, p.GetIdentity())
	allPopulated(t, p.GetCommands()[0])
	return p
}

// allSet fails when a struct field this fixture is meant to populate is still
// zero. except names the ones checked separately.
func allSet(t *testing.T, what string, v any, except ...string) {
	t.Helper()
	rv := reflect.ValueOf(v)
	for i := range rv.NumField() {
		name := rv.Type().Field(i).Name
		if slices.Contains(except, name) {
			continue
		}
		if rv.Field(i).IsZero() {
			t.Fatalf("%s.%s is not set in this test's fixture, so the test "+
				"cannot tell whether a renderer carries it. Set it rather than "+
				"adding it to an exception list", what, name)
		}
	}
}

// allPopulated is allSet for a proto message, where "populated" is the
// question proto reflection answers and zero-vs-unset is the same distinction
// the renderers are being measured on.
func allPopulated(t *testing.T, m proto.Message) {
	t.Helper()
	d := m.ProtoReflect().Descriptor()
	for i := range d.Fields().Len() {
		f := d.Fields().Get(i)
		if !m.ProtoReflect().Has(f) {
			t.Fatalf("%s.%s is not populated in this test's fixture, so the "+
				"test cannot tell whether a renderer carries it",
				d.Name(), f.Name())
		}
	}
}

// snake folds a rendered camelCase key back to the wire's own spelling.
func snake(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// wireName is the Go field name as the kernel's refusal spells it.
func wireName(goField string) string {
	if goField == "NeedsDisplay" {
		return "needs_display"
	}
	return strings.ToLower(goField[:1]) + goField[1:]
}
