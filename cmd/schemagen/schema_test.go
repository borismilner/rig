package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// compiled builds the emitted schema and compiles it, so every test below is
// checking the real artefact rather than a copy of it.
func compiled(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f := Files()[0]
	body, err := f.JSON()
	if err != nil {
		t.Fatalf("building the schema: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("the emitted schema is not JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(f.Name, doc); err != nil {
		t.Fatalf("the emitted schema is not a schema: %v", err)
	}
	s, err := c.Compile(f.Name)
	if err != nil {
		t.Fatalf("the emitted schema does not compile: %v", err)
	}
	return s
}

// instance turns a declaration into what would actually cross the wire, so a
// test can never pass against a hand-written shape the daemon never sees.
func instance(t *testing.T, d *rigv1.Declaration) any {
	t.Helper()
	body, err := protojson.Marshal(d)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	return v
}

// valid is a declaration registration would accept: every mandatory property
// said, and nothing optional.
func valid() *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity:     &rigv1.Identity{Id: "probe", Version: "v1"},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		SemanticsGen: 1,
		Commands: []*rigv1.Command{{
			Id:           "ping",
			Effects:      rigv1.Effects_EFFECTS_READ_ONLY,
			Idempotent:   rigv1.Tristate_TRISTATE_YES,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Confirms:     rigv1.Tristate_TRISTATE_NO,
			Shape:        rigv1.Shape_SHAPE_UNARY,
			Summary:      "Round-trip a nonce",
			Description:  "Sends a nonce and reads it back.",
			Returns:      "The nonce.",
		}},
	}
}

func TestADeclarationRegistrationAcceptsAlsoPassesTheSchema(t *testing.T) {
	if err := compiled(t).Validate(instance(t, valid())); err != nil {
		t.Fatalf("the schema refused a declaration the daemon accepts:\n%v", err)
	}
}

// The schema must never be stricter than registration. A property the kernel
// treats as optional has to stay optional here, and an empty sensitive list is
// the one that is easiest to get wrong: absent and empty are different things.
func TestAnEmptySensitiveListIsAcceptedBecauseAbsenceIsTheOtherThing(t *testing.T) {
	d := valid()
	d.Commands[0].Sensitive = &rigv1.SensitiveFields{}
	if err := compiled(t).Validate(instance(t, d)); err != nil {
		t.Fatalf("a present but empty sensitive list was refused:\n%v", err)
	}
}

func TestTheSchemaRefusesAnEnumTheProgramDidNotSay(t *testing.T) {
	d := valid()
	d.Coverage = rigv1.Coverage_COVERAGE_UNSPECIFIED
	// protojson drops a zero enum rather than writing UNSPECIFIED, which is
	// the same thing on the wire and is exactly what registration refuses.
	if err := compiled(t).Validate(instance(t, d)); err == nil {
		t.Fatal("an unsaid coverage passed the schema; registration refuses it")
	}
}

func TestTheSchemaRefusesAMandatoryPropertyLeftOut(t *testing.T) {
	for _, drop := range []struct {
		name string
		f    func(*rigv1.Declaration)
	}{
		{"summary", func(d *rigv1.Declaration) { d.Commands[0].Summary = "" }},
		{"returns", func(d *rigv1.Declaration) { d.Commands[0].Returns = "" }},
		{"sensitive", func(d *rigv1.Declaration) { d.Commands[0].Sensitive = nil }},
		{"identity", func(d *rigv1.Declaration) { d.Identity = nil }},
		{"semanticsGen", func(d *rigv1.Declaration) { d.SemanticsGen = 0 }},
	} {
		t.Run(drop.name, func(t *testing.T) {
			d := valid()
			drop.f(d)
			if err := compiled(t).Validate(instance(t, d)); err == nil {
				t.Fatalf("%s was left out and the schema accepted it", drop.name)
			}
		})
	}
}

// A negative generation is refused by the kernel, so the schema carries the
// floor rather than only the presence check.
func TestTheSchemaCarriesTheGenerationFloor(t *testing.T) {
	d := valid()
	d.SemanticsGen = -1
	if err := compiled(t).Validate(instance(t, d)); err == nil {
		t.Fatal("semantics_gen of -1 passed; the kernel refuses anything below 1")
	}
}

// This is the drift lock, and it is the reason the mandatory set may be
// written by hand at all.
//
// proto descriptors cannot say which properties are required, so mandatory.go
// mirrors the kernel by hand. A mandatory property added to the kernel and not
// here would ship a schema that accepts what registration then refuses. So:
// drive the kernel's own refusal of an empty declaration and require that
// every refusal is one this generator already knows about.
func TestTheMandatorySetMatchesWhatRegistrationRefuses(t *testing.T) {
	d := kernel.Declaration{Commands: []kernel.Command{{}}}
	err := d.Validate()
	if err == nil {
		t.Fatal("an empty declaration was accepted; this test has nothing to lock")
	}

	var refusals []string
	for _, line := range strings.Split(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			refusals = append(refusals, strings.TrimPrefix(line, "- "))
		}
	}

	// Every name this generator calls mandatory, in the wording the kernel
	// uses when it refuses that name - DERIVED from the mandatory map rather
	// than listed again here. A second copy of the set would let the copy and
	// the map drift apart, which is the very failure this test exists to catch.
	known := expectedRefusals(t)
	if len(refusals) != len(known) {
		t.Fatalf("the kernel refuses %d things and this generator knows %d.\n"+
			"A mandatory property has been added or removed in internal/kernel "+
			"and mandatory.go has not followed, so the emitted schema now "+
			"disagrees with registration.\nRefusals were:\n  %s",
			len(refusals), len(known), strings.Join(refusals, "\n  "))
	}
	for _, want := range known {
		found := false
		for _, got := range refusals {
			if strings.Contains(got, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("nothing in the kernel's refusals mentions %q, so this "+
				"generator is requiring a property the kernel does not", want)
		}
	}
}

// The file is regenerated by `make schema` and committed, so unstable output
// would show as a diff on every run and train everyone to ignore it.
func TestTheOutputIsByteStable(t *testing.T) {
	f := Files()[0]
	first, err := f.JSON()
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	for i := range 5 {
		again, err := f.JSON()
		if err != nil {
			t.Fatalf("pass %d: %v", i+2, err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("pass %d differs from the first; map iteration is "+
				"reaching the output", i+2)
		}
	}
}

// json2ts names the generated TypeScript interface after the schema's title,
// so a missing or renamed title silently renames the frontend's type.
func TestTheRootCarriesATitleForTheTypeGenerator(t *testing.T) {
	f := Files()[0]
	body, err := f.JSON()
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if doc["title"] != "Declaration" {
		t.Fatalf("title is %v, and json2ts names the frontend's interface "+
			"after it", doc["title"])
	}
}

// expectedRefusals turns the mandatory map into the wording the kernel uses.
//
// Two adjustments, both rules rather than exceptions:
//   - A mandatory field whose message has mandatory fields of its own is
//     refused through those, not by name: the kernel says "identity.id is
//     empty", never "identity is missing". SensitiveFields deliberately has
//     none, so `sensitive` keeps its own refusal.
//   - A nested message's fields are refused under their parent's name.
func expectedRefusals(t *testing.T) []string {
	t.Helper()
	root := (&rigv1.Declaration{}).ProtoReflect().Descriptor()

	delegates := func(parent protoreflect.MessageDescriptor, name protoreflect.Name) (protoreflect.MessageDescriptor, bool) {
		fd := parent.Fields().ByName(name)
		if fd == nil {
			t.Fatalf("mandatory.go names %q on %s and the proto has no such "+
				"field", name, parent.Name())
		}
		if fd.Kind() != protoreflect.MessageKind {
			return nil, false
		}
		md := fd.Message()
		return md, len(mandatory[md.Name()]) > 0
	}

	var out []string
	for _, n := range mandatory[root.Name()] {
		md, delegated := delegates(root, n)
		if !delegated {
			out = append(out, string(n))
			continue
		}
		for _, inner := range mandatory[md.Name()] {
			out = append(out, string(n)+"."+string(inner))
		}
	}

	cmd := root.Fields().ByName("commands").Message()
	for _, n := range mandatory[cmd.Name()] {
		if n == "id" {
			// The kernel words this one as "commands[0] has no id", because an
			// unnamed command cannot be quoted back at its author.
			out = append(out, "has no id")
			continue
		}
		out = append(out, string(n))
	}
	return out
}
