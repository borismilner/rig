package meta_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// TestEveryCapabilityMapFieldReachesTheResource walks kernel.CapabilityMap by
// reflection and fails on any field the resource's renderer does not carry.
//
// THIS IS THE GUARD THE SECOND DESCRIPTION HAS TO COME WITH. This repository
// already carries two structural descriptions of a Program that nothing ties
// together, and the recorded cost is a field landing in one and not the other
// and presenting weeks later as "the CLI shows it and MCP does not". The
// renderer below is a new description of the MAP, so it arrives with the
// guard the digest's own writer already has: a reflection walk that fails
// when a field is not covered rather than a reviewer who has to notice.
//
// It refuses to pretend, which is the load-bearing half. A field this test
// does not know how to change fails loudly instead of passing over - the same
// property nudge has in the kernel's two version walks, and for the same
// reason: a walk that silently skips an unknown field reports "covered" for a
// field nobody covered.
func TestEveryCapabilityMapFieldReachesTheResource(t *testing.T) {
	// change is one mutation per field of kernel.CapabilityMap. Adding a
	// field to that struct and not to this table is a test failure, by
	// design: that is the whole mechanism.
	change := map[string]func(*kernel.CapabilityMap){
		"Version": func(m *kernel.CapabilityMap) { m.Version = m.Version + "-changed" },
		"Depth":   func(m *kernel.CapabilityMap) { m.Depth = kernel.DepthFull },
		"Basis":   func(m *kernel.CapabilityMap) { m.Basis = kernel.BasisScoped },
		"Programs": func(m *kernel.CapabilityMap) {
			m.Programs = append(m.Programs, kernel.Program{
				Identity: kernel.Identity{ID: "added"},
				Coverage: kernel.CoverageFull,
			})
		},
	}

	typ := reflect.TypeOf(kernel.CapabilityMap{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			mutate, known := change[name]
			if !known {
				t.Fatalf("kernel.CapabilityMap gained a field %q and this "+
					"test does not know how to change it, so NOTHING proves "+
					"the resource carries it. Add a mutation above rather "+
					"than skipping, or the map can gain a field the one MCP "+
					"resource silently drops", name)
			}

			base := mapFixture()
			before, err := meta.MarshalCapabilityMap(base)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			one := mapFixture()
			mutate(&one)
			after, err := meta.MarshalCapabilityMap(one)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			if string(before) == string(after) {
				t.Errorf("changing CapabilityMap.%s did not change the "+
					"resource's content, so the renderer drops it and an "+
					"agent reading the resource cannot see it at all", name)
			}
		})
	}
}

// TestTheResourceStatesItsOwnDepthAndAdmitsAnEmptyEstate is the shape
// argument, asserted rather than described.
//
// An empty estate is every fresh daemon, so it is the first case a user
// meets, and it is the case where "omit what is empty" and "never imply
// completeness" pull in opposite directions. Section 5k decides it: the
// object says programs is empty rather than staying quiet about programs.
func TestTheResourceStatesItsOwnDepthAndAdmitsAnEmptyEstate(t *testing.T) {
	empty, err := meta.New(kernel.New(), nil).CapabilityMap(agent(), kernel.DepthCommands)
	if err != nil {
		t.Fatalf("capability map: %v", err)
	}
	b, err := meta.MarshalCapabilityMap(empty)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("the resource did not emit an object: %v", err)
	}

	for _, key := range []string{"version", "depth", "basis", "partial", "programs"} {
		if _, ok := got[key]; !ok {
			t.Errorf("an empty estate's map omits %q, so a reader cannot "+
				"tell it apart from a daemon too old to have the field: %s",
				key, b)
		}
	}
	if string(got["depth"]) != `"commands"` {
		t.Errorf("the map does not state the depth it was built at: %s", got["depth"])
	}
	if string(got["programs"]) != `[]` || string(got["partial"]) != `[]` {
		t.Errorf("empty lists rendered as something other than []: programs=%s partial=%s",
			got["programs"], got["partial"])
	}
}

// TestTheResourcesFieldsAreEmittedAlways asserts the struct tags directly,
// because nothing behavioural can see the trap.
//
// `depth` renders as an enum NAME and "unspecified" is not the empty string,
// so an omitempty added to it changes nothing on the day it is added and
// starts dropping the key the day somebody renders the zero as "". The same
// is true of every other field here the moment its zero value is reachable.
func TestTheResourcesFieldsAreEmittedAlways(t *testing.T) {
	shape := meta.CapabilityMapJSONShape
	for i := range shape.NumField() {
		f := shape.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" {
			t.Errorf("%s has no json tag, so its name is its Go field name", f.Name)
			continue
		}
		for _, opt := range []string{",omitempty", ",omitzero"} {
			if len(tag) > len(opt) && tag[len(tag)-len(opt):] == opt {
				t.Errorf("%s carries %s. Every field of the capability "+
					"resource is emitted always: an absent one cannot be "+
					"told apart from a daemon too old to have it", f.Name, opt)
			}
		}
	}
}

// TestAScopedCallerAndAnIntrospectingOneGetDifferentMaps is slice 4's second
// demo clause with the transport taken out, so the socket test above it is
// demonstrating the transport rather than the filter.
func TestAScopedCallerAndAnIntrospectingOneGetDifferentMaps(t *testing.T) {
	k := estate(t, kernel.CoveragePartial)
	if _, err := k.Register(kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "grabbit", SessionID: "s-grabbit", PID: 3,
	}, declaring("grabbit", kernel.CoverageFull)); err != nil {
		t.Fatalf("register grabbit: %v", err)
	}
	s := meta.New(k, nil)

	scoped := kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "shelf", SessionID: "s-shelf", PID: 1,
		Scoped: true, Scopes: []string{"shelf"},
	}

	wide, err := s.CapabilityMap(agent(), kernel.DepthCommands)
	if err != nil {
		t.Fatalf("the introspecting map: %v", err)
	}
	narrow, err := s.CapabilityMap(scoped, kernel.DepthCommands)
	if err != nil {
		t.Fatalf("the scoped map: %v", err)
	}

	if len(wide.Programs) != 2 {
		t.Fatalf("the introspecting caller sees %d programs, want 2", len(wide.Programs))
	}
	if len(narrow.Programs) != 1 || narrow.Programs[0].Identity.ID != "shelf" {
		t.Fatalf("the scoped caller sees more than itself: %v", narrow.Programs)
	}
	// THE ASSERTION THE VERSION DESIGN EXISTS FOR. Two different maps at one
	// instant must not carry one version, or anything caching on the version
	// serves one caller the other's estate.
	if wide.Version == narrow.Version {
		t.Error("two different maps share a version, so a cache keyed on it " +
			"would serve the scoped caller the whole estate")
	}
}

// mapFixture is a map with every field populated, so a mutation of any one of
// them is a change from something rather than from nothing.
func mapFixture() kernel.CapabilityMap {
	return kernel.CapabilityMap{
		Version: "v-fixture",
		Depth:   kernel.DepthCommands,
		Basis:   kernel.BasisComplete,
		Programs: []kernel.Program{{
			Identity: kernel.Identity{ID: "shelf", Name: "Shelf"},
			Coverage: kernel.CoveragePartial,
			Commands: []kernel.Command{{ID: "search", Summary: "search it"}},
		}},
	}
}

// TestACallerWithNoGrantGetsAnEmptyMapRatherThanAnError is section 9's other
// half of the granted surface: "a client with no grant gets the map of what
// it may reach".
//
// It is a REACHABLE empty map rather than a fresh daemon's - the estate is
// populated and this caller may see none of it - which is why programs is
// emitted always. Rendered as an absent key, this answer and "the daemon is
// too old to have the field" are the same bytes, and the safe reading of each
// points the opposite way.
func TestACallerWithNoGrantGetsAnEmptyMapRatherThanAnError(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), nil)

	none := kernel.Principal{
		UID: 1000, Kind: kernel.KindTerminal,
		ClientID: "ungranted", SessionID: "s-ungranted", PID: 9,
	}
	m, err := s.CapabilityMap(none, kernel.DepthCommands)
	if err != nil {
		t.Fatalf("a caller with no grant was refused rather than answered: %v", err)
	}
	if len(m.Programs) != 0 {
		t.Fatalf("a caller with no grant saw %d programs", len(m.Programs))
	}
	if m.Version == "" {
		t.Error("the empty map carries no version, so a caller cannot tell " +
			"'nothing for you' from 'nothing answered'")
	}

	b, err := meta.MarshalCapabilityMap(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("not an object: %v", err)
	}
	if string(got["programs"]) != `[]` {
		t.Errorf("a reachable empty map rendered programs as %s, not []", got["programs"])
	}
}

// TestTwoEMPTYMapsAreNotTheSameAnswer is V20's acceptance test, and it is the
// one assertion this whole field exists for.
//
//	"This is everything, and everything is nothing" and "this is what you may
//	 reach, which is nothing" are different answers.
//
// Before the basis they were IDENTICAL BYTES. A scoped caller reading an
// empty map could not tell an empty estate from an estate it may see none of,
// and those two call for opposite actions: one is "there is nothing to use",
// the other is "ask for more access". A projection that returns the same
// answer to both is not merely terse, it is wrong.
//
// The empty case is chosen deliberately over a populated one. With programs
// in the map the two answers differ in their contents anyway, so a test there
// passes whether or not the basis exists - green for the wrong reason. Empty
// is where the field is the only thing carrying the difference.
func TestTwoEMPTYMapsAreNotTheSameAnswer(t *testing.T) {
	s := meta.New(kernel.New(), nil)

	everything, err := s.CapabilityMap(agent(), kernel.DepthCommands)
	if err != nil {
		t.Fatalf("the introspecting map: %v", err)
	}
	bounded, err := s.CapabilityMap(kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "shelf", SessionID: "s-shelf", PID: 1,
		Scoped: true, Scopes: []string{"shelf"},
	}, kernel.DepthCommands)
	if err != nil {
		t.Fatalf("the scoped map: %v", err)
	}

	if len(everything.Programs) != 0 || len(bounded.Programs) != 0 {
		t.Fatal("this test is only meaningful on two empty maps")
	}

	a, err := meta.MarshalCapabilityMap(everything)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b, err := meta.MarshalCapabilityMap(bounded)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if string(a) == string(b) {
		t.Fatalf("two empty maps are byte-identical, so a caller cannot tell "+
			"'the estate is empty' from 'you may see none of it':\n%s", a)
	}
	if everything.Version == bounded.Version {
		t.Error("two different answers share a version, so anything caching " +
			"on it serves one caller the other's answer")
	}
}

// TestAnUnsetBasisNeverReadsAsComplete is the house rule on a zero value that
// would be a false reassurance.
//
// The safe reading and the convenient reading point opposite ways here: a map
// that forgot to set its basis must not tell a scoped caller it has seen
// everything. That is the same asymmetry an unset estate role was ruled on,
// and it is why the zero is a value of its own rather than a default.
func TestAnUnsetBasisNeverReadsAsComplete(t *testing.T) {
	b, err := meta.MarshalCapabilityMap(kernel.CapabilityMap{
		Version: "v", Depth: kernel.DepthCommands,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("not an object: %v", err)
	}
	basis := string(got["basis"])
	if basis == `"complete"` {
		t.Fatal("an unset basis rendered as \"complete\", which tells a " +
			"caller it has seen the whole estate on the strength of a field " +
			"nobody set")
	}
	if basis == `""` || basis == "" {
		t.Errorf("an unset basis rendered as %q, which is absent wearing a "+
			"different hat: a reader cannot tell it from a daemon too old to "+
			"have the field", basis)
	}
}
