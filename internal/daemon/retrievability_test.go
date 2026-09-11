package daemon

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// notRetrievable is every declaration field a caller is deliberately NOT given
// back, with the reason. A field is either in this map or it is expected to
// survive the round trip; there is no third state, which is the point.
//
// Adding an entry here is a DECISION and should be argued in the commit that
// adds it. Adding a proto field and forgetting to plumb it is not - and that
// is the difference this test exists to make visible.
var notRetrievable = map[string]string{
	"scope": "how rig decides who may see this program, not something the " +
		"program said about itself. A program that declares a scope other " +
		"than its own id is refused outright (Declaration.validate).",
}

// everyFieldSet is a declaration with EVERY field of Declaration and of
// Command set to a distinctive non-zero value.
//
// It is deliberately maximal. A fixture that left a field at its zero value
// would make the round-trip assertion below vacuous for that field, which is
// exactly how the preamble stayed dead through a year of green test runs.
func everyFieldSet(id string) *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id: id, Name: "Shelf", Version: "1.2.0",
			Icon: "book", Description: "The shelf.",
		},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "search is adopted; storage is not",
		SemanticsGen: 3,
		Services:     []string{"search"},
		Preamble:     "Read this before touching " + id + ".",
		Scope:        id,
		Hosted:       true,
		PaneUrl:      "http://127.0.0.1:8731/pane",
		Elements:     []string{"rigTable"},
		Commands: []*rigv1.Command{{
			Id:            "reindex",
			Title:         "Reindex",
			Args:          []byte(`{"type":"object"}`),
			Examples:      []string{"rig " + id + " reindex --since 7d"},
			Effects:       rigv1.Effects_EFFECTS_WRITES_FILES,
			Idempotent:    rigv1.Tristate_TRISTATE_YES,
			Sensitive:     &rigv1.SensitiveFields{Pointers: []string{"/token"}},
			Interactive:   rigv1.Tristate_TRISTATE_NO,
			Streams:       rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay:  rigv1.Tristate_TRISTATE_NO,
			Duration:      rigv1.Duration_DURATION_SECONDS,
			Confirms:      rigv1.Tristate_TRISTATE_YES,
			Shape:         rigv1.Shape_SHAPE_UNARY,
			Summary:       "Rebuild the index",
			Description:   "Rebuilds the index from the tree.",
			Returns:       "The number of items indexed.",
			DryRun:        true,
			Cost:          "seconds, no money",
			Preconditions: []string{id + ".index.path exists"},
			Promote:       true,
		}},
	}
}

// TestTheFixtureSetsEveryDeclaredField is the half that keeps the OTHER test
// honest, and it has to come first.
//
// The round-trip assertions below can only catch a dropped field if the
// fixture actually set it. So this walks Declaration and Command by
// reflection and fails on any field left at its zero value. Add a field to
// the proto and this reddens immediately, telling the author to set it -
// before the plumbing question is even asked.
func TestTheFixtureSetsEveryDeclaredField(t *testing.T) {
	d := everyFieldSet("shelf")
	unset(t, "Declaration", d.ProtoReflect())
	if len(d.GetCommands()) != 1 {
		t.Fatalf("the fixture declares %d commands", len(d.GetCommands()))
	}
	unset(t, "Command", d.GetCommands()[0].ProtoReflect())
}

func unset(t *testing.T, name string, m protoreflect.Message) {
	t.Helper()
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if !m.Has(f) {
			t.Errorf("%s.%s is not set by everyFieldSet, so any assertion "+
				"that it survives a round trip proves nothing. Set it.",
				name, f.Name())
		}
	}
}

// TestEveryDeclaredFieldIsRetrievable is section 35's rule as a test: a field
// the declaration accepts must be reachable by some caller on some surface,
// or it is not a field - it is a validated place to put something that goes
// nowhere. Acceptance is not a contract; retrievability is.
//
// TWO INSTANCES IN ONE DAY IS WHY THIS EXISTS. The preamble was accepted at
// hello, validated, stored, and carried by no message on any path. `confirms`
// was declared, rendered to a human as a guarantee, and consumed by nothing.
// Neither was going to be caught by reading, and both had been green for as
// long as they had existed.
//
// It compares BY FIELD NAME through reflection rather than by a hand-written
// list, so a proto field added tomorrow is covered tomorrow. A field that is
// deliberately withheld goes in notRetrievable, with its reason, which makes
// withholding a decision somebody wrote down.
func TestEveryDeclaredFieldIsRetrievable(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	c := dial(t, sock)
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, everyFieldSet("shelf")); err != nil {
		t.Fatalf("the maximal declaration was refused: %v", err)
	}

	got := find(estate(t, dial(t, sock), rigv1.Depth_DEPTH_FULL), "shelf")
	if got == nil {
		t.Fatal("shelf is not in the estate")
	}

	// Every Declaration field either comes back on Program under the same
	// name, or is listed as withheld.
	decl := everyFieldSet("shelf").ProtoReflect().Descriptor().Fields()
	program := got.ProtoReflect()
	pfields := program.Descriptor().Fields()
	for i := range decl.Len() {
		name := string(decl.Get(i).Name())
		if why, withheld := notRetrievable[name]; withheld {
			if pfields.ByName(protoreflect.Name(name)) != nil {
				t.Errorf("Declaration.%s is listed as withheld (%s) but Program "+
					"has a field of that name; one of the two is stale", name, why)
			}
			continue
		}
		f := pfields.ByName(protoreflect.Name(name))
		if f == nil {
			t.Errorf("a program may declare %q and NO caller can read it back: "+
				"Program has no such field and it is not listed as withheld. "+
				"Either carry it or say in notRetrievable why not.", name)
			continue
		}
		if !program.Has(f) {
			t.Errorf("a program declared %q and the caller got it empty: the "+
				"field exists on Program and nothing fills it", name)
		}
	}

	// And every Command field comes back, because Command is one message in
	// both directions - so a field dropped by either translator is dead.
	if len(got.GetCommands()) != 1 {
		t.Fatalf("the caller got %d commands", len(got.GetCommands()))
	}
	back := got.GetCommands()[0].ProtoReflect()
	cfields := back.Descriptor().Fields()
	for i := range cfields.Len() {
		f := cfields.Get(i)
		if !back.Has(f) {
			t.Errorf("a program declared Command.%s and the caller got it "+
				"empty: one of commandFromWire or commandToWire drops it",
				f.Name())
		}
	}
}
