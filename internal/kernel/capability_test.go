package kernel_test

import (
	"reflect"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

func mapOf(t *testing.T, k *kernel.Kernel, p kernel.Principal, d kernel.Depth) kernel.CapabilityMap {
	t.Helper()
	m, err := k.See(p).CapabilityMap(d)
	if err != nil {
		t.Fatalf("capability map at %s: %v", d, err)
	}
	return m
}

// TestSliceFourDemo is M2 slice 4's demo, run rather than described:
//
//	"A new command registered by a running program changes the map with no
//	 agent restart and no rig change; a scoped caller and an introspecting one
//	 read different maps at the same instant."
//
// Both halves, in the order the demo states them.
func TestSliceFourDemo(t *testing.T) {
	k := kernel.New()
	owner := programPrincipal("shelf")
	if _, err := k.Register(owner, withCommands("shelf", map[string]kernel.Effects{
		"search": kernel.EffectsReadOnly,
	})); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := k.Register(programPrincipal("grabbit"),
		withCommands("grabbit", map[string]kernel.Effects{
			"fetch": kernel.EffectsNetwork,
		})); err != nil {
		t.Fatalf("register grabbit: %v", err)
	}

	agent := caller(kernel.KindAgent)
	scoped := programPrincipal("shelf")
	scoped.Scoped = true
	scoped.Scopes = []string{"shelf"}

	// A new command, registered by the running program. No rig change and
	// nothing restarted: the same kernel, the same session.
	before := mapOf(t, k, agent, kernel.DepthCommands)
	if _, err := k.Register(owner, withCommands("shelf", map[string]kernel.Effects{
		"search":  kernel.EffectsReadOnly,
		"reindex": kernel.EffectsWritesFiles,
	})); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	after := mapOf(t, k, agent, kernel.DepthCommands)

	if before.Version == after.Version {
		t.Fatal("a new command did not change the map, so an agent holding " +
			"the old version would never learn the command exists")
	}
	if len(commandsOf(after, "shelf")) != 2 {
		t.Fatalf("the new command is not in the map: %v",
			commandsOf(after, "shelf"))
	}

	// A scoped caller and an introspecting one, at the same instant.
	wide := mapOf(t, k, agent, kernel.DepthCommands)
	narrow := mapOf(t, k, scoped, kernel.DepthCommands)

	if len(wide.Programs) != 2 {
		t.Fatalf("the introspecting caller's map has %d programs, want 2",
			len(wide.Programs))
	}
	if len(narrow.Programs) != 1 || narrow.Programs[0].Identity.ID != "shelf" {
		t.Fatalf("the scoped caller's map is %v, want just shelf",
			ids(narrow.Programs))
	}

	// THE ASSERTION THE WHOLE VERSION DESIGN EXISTS FOR. Two different maps
	// at one instant must not carry one version, or anything caching on it
	// serves one caller the other's estate.
	if wide.Version == narrow.Version {
		t.Fatal("a scoped caller and an introspecting one got DIFFERENT maps " +
			"with the SAME version, so the version is of the registry rather " +
			"than of the projection")
	}
}

// TestTheSameEstateGivesTheSameVersion is the other half of a version being
// useful: it has to be stable, or every read looks like a change and an agent
// re-reads the whole map forever.
func TestTheSameEstateGivesTheSameVersion(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(programPrincipal("shelf"), withPreamble("shelf")); err != nil {
		t.Fatalf("register: %v", err)
	}
	who := caller(kernel.KindAgent)

	first := mapOf(t, k, who, kernel.DepthFull)
	second := mapOf(t, k, who, kernel.DepthFull)
	if first.Version != second.Version {
		t.Fatalf("two reads of one unchanged estate differ:\n%s\n%s",
			first.Version, second.Version)
	}

	// And a re-registration that changes NOTHING changes nothing. This is the
	// ordinary case of a program restarting, and a map that churned on it
	// would train an agent to ignore the version.
	if _, err := k.Register(programPrincipal("shelf"), withPreamble("shelf")); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if third := mapOf(t, k, who, kernel.DepthFull); third.Version != first.Version {
		t.Error("re-declaring the same thing changed the map's version")
	}
}

// TestTheDepthIsPartOfTheMapsIdentity: the same estate at two depths is two
// different maps. Sharing a version would let a cache answer a full read from
// a cheap one.
func TestTheDepthIsPartOfTheMapsIdentity(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(programPrincipal("shelf"), withPreamble("shelf")); err != nil {
		t.Fatalf("register: %v", err)
	}
	who := caller(kernel.KindAgent)

	seen := map[string]kernel.Depth{}
	for _, d := range []kernel.Depth{
		kernel.DepthPrograms, kernel.DepthCommands, kernel.DepthFull,
	} {
		m := mapOf(t, k, who, d)
		if m.Depth != d {
			t.Errorf("a map asked for at %s reports %s", d, m.Depth)
		}
		if other, clash := seen[m.Version]; clash {
			t.Errorf("depths %s and %s share a version", other, d)
		}
		seen[m.Version] = d
	}
	if _, err := k.See(who).CapabilityMap(kernel.Depth(0)); err == nil {
		t.Error("a map was built at an unspecified depth")
	}

	// AND ON AN EMPTY ESTATE, which is where it actually bites. With programs
	// registered the three depths differ in content anyway, so they would
	// have different versions even if the depth were left out of the digest
	// entirely - a mutation removing it left the check above green. An empty
	// estate has identical content at every depth, so only the depth itself
	// can separate them. A fresh daemon is exactly this case.
	empty := kernel.New()
	seenEmpty := map[string]kernel.Depth{}
	for _, d := range []kernel.Depth{
		kernel.DepthPrograms, kernel.DepthCommands, kernel.DepthFull,
	} {
		m := mapOf(t, empty, who, d)
		if len(m.Programs) != 0 {
			t.Fatalf("the empty estate has %d programs", len(m.Programs))
		}
		if other, clash := seenEmpty[m.Version]; clash {
			t.Errorf("on an EMPTY estate, depths %s and %s share a version, "+
				"so the depth is not part of the map's identity", other, d)
		}
		seenEmpty[m.Version] = d
	}
}

// TestCommandOrderDoesNotChangeTheMap: declaration order is what a program
// happened to write, not something it declared. A diff that reported a
// reordering as a change is noise an agent has to learn to ignore, and an
// agent that learns to ignore a version has no version.
func TestCommandOrderDoesNotChangeTheMap(t *testing.T) {
	k := kernel.New()
	d := withCommands("shelf", map[string]kernel.Effects{
		"search":  kernel.EffectsReadOnly,
		"reindex": kernel.EffectsWritesFiles,
		"gc":      kernel.EffectsDestructive,
	})
	owner := programPrincipal("shelf")
	if _, err := k.Register(owner, d); err != nil {
		t.Fatalf("register: %v", err)
	}
	who := caller(kernel.KindAgent)
	first := mapOf(t, k, who, kernel.DepthCommands).Version

	// The same commands, declared in the reverse order.
	reversed := d
	reversed.Commands = make([]kernel.Command, len(d.Commands))
	for i, c := range d.Commands {
		reversed.Commands[len(d.Commands)-1-i] = c
	}
	if _, err := k.Register(owner, reversed); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if second := mapOf(t, k, who, kernel.DepthCommands).Version; second != first {
		t.Error("reordering a program's commands changed the map's version")
	}
}

// TestAdjacentFieldsCannotBeConfused locks the length prefix, which is the
// one thing in the digest that looks like ceremony and is not.
//
// A digest over concatenated fields cannot tell "ab" + "c" from "a" + "bc".
// Two genuinely different estates would then share a version, and an agent
// holding the first would never re-read. The fields below are chosen to be
// exactly that pair.
func TestAdjacentFieldsCannotBeConfused(t *testing.T) {
	// Identity.Name and Identity.Version, because they are written one after
	// the other with nothing between them. The first version of this test
	// used CoverageNote and Preamble, which are separated by five other
	// fields - so removing the length prefix left it green, and it was
	// testing nothing. Adjacency is the whole point.
	one := fullProgram()
	one.Identity.Name, one.Identity.Version = "ab", "c"

	two := fullProgram()
	two.Identity.Name, two.Identity.Version = "a", "bc"

	if kernel.MapVersion(kernel.DepthFull, []kernel.Program{one}) ==
		kernel.MapVersion(kernel.DepthFull, []kernel.Program{two}) {
		t.Fatal("two different estates share a version: the digest runs " +
			"adjacent fields together, so a boundary between them can move " +
			"without the version noticing")
	}
}

// TestEveryProgramFieldChangesTheVersion is the guard that keeps the digest
// honest, and it is the same idiom as the retrievability test in
// internal/daemon: a hand-written list rots, reflection does not.
//
// A field the digest ignores is a field that can change without changing the
// version - a diff that silently reports "no change". That is the same class
// of defect as a declared field no caller can read: invisible to review,
// green for as long as it exists. So this walks kernel.Program by reflection,
// changes each field in turn, and fails on any whose change the version does
// not notice.
func TestEveryProgramFieldChangesTheVersion(t *testing.T) {
	base := fullProgram()
	baseline := kernel.MapVersion(kernel.DepthFull, []kernel.Program{base})

	v := reflect.ValueOf(&base).Elem()
	typ := v.Type()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			one := fullProgram()
			f := reflect.ValueOf(&one).Elem().Field(i)
			if !nudge(f) {
				t.Fatalf("this test does not know how to change a %s, so "+
					"%s is NOT covered. Teach nudge about it rather than "+
					"skipping, or the digest can quietly ignore the field",
					f.Kind(), name)
			}
			if got := kernel.MapVersion(kernel.DepthFull, []kernel.Program{one}); got == baseline {
				t.Errorf("changing %s did not change the version, so the "+
					"digest ignores it and a diff would report no change",
					name)
			}
		})
	}
}

// TestEveryCommandFieldChangesTheVersion is the sibling walk, and it exists
// because the one above does NOT cover it.
//
// Nudging Program.Commands appends a command, which proves only that the
// COUNT participates. A command's own fields could be ignored wholesale and
// that test would still pass - which is the same "green for the wrong reason"
// shape this repository keeps paying for. So this walks kernel.Command.
func TestEveryCommandFieldChangesTheVersion(t *testing.T) {
	baseline := kernel.MapVersion(kernel.DepthFull, []kernel.Program{fullProgram()})

	typ := reflect.TypeOf(kernel.Command{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		t.Run(name, func(t *testing.T) {
			one := fullProgram()
			f := reflect.ValueOf(&one.Commands[0]).Elem().Field(i)
			if !nudge(f) {
				t.Fatalf("this test does not know how to change a %s, so "+
					"Command.%s is NOT covered. Teach nudge about it rather "+
					"than skipping", f.Kind(), name)
			}
			if got := kernel.MapVersion(kernel.DepthFull, []kernel.Program{one}); got == baseline {
				t.Errorf("changing Command.%s did not change the version, so "+
					"the digest ignores it", name)
			}
		})
	}
}

// nudge changes a value in a way its type can express, reporting whether it
// knew how. It refuses to pretend: an unknown kind returns false so the test
// says the field is uncovered rather than passing over it.
func nudge(f reflect.Value) bool {
	switch f.Kind() {
	case reflect.String:
		f.SetString(f.String() + "-changed")
		return true
	case reflect.Bool:
		f.SetBool(!f.Bool())
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt(f.Int() + 1)
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f.SetUint(f.Uint() + 1)
		return true
	case reflect.Slice:
		grown := reflect.Append(f, reflect.New(f.Type().Elem()).Elem())
		f.Set(grown)
		return true
	case reflect.Struct:
		// Identity, and anything else added later: change its first field
		// that can be changed.
		for i := range f.NumField() {
			if nudge(f.Field(i)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// fullProgram is a Program with every field set, for the same reason the
// retrievability fixture is maximal: nudging a field that was already at its
// zero value would prove nothing about a field that is really used.
func fullProgram() kernel.Program {
	return kernel.Program{
		Identity: kernel.Identity{
			ID: "shelf", Name: "Shelf", Version: "1.2.0",
			Icon: "book", Description: "The shelf.",
		},
		Coverage:     kernel.CoveragePartial,
		CoverageNote: "search is adopted; storage is not",
		SemanticsGen: 3,
		Services:     []string{"search"},
		Elements:     []string{"rigTable"},
		Hosted:       true,
		PaneURL:      "http://127.0.0.1:8731/pane",
		Preamble:     "Read this first.",
		Commands: []kernel.Command{{
			ID: "reindex", Title: "Reindex",
			Args: []byte(`{"type":"object"}`), Examples: []string{"rig shelf reindex"},
			Effects: kernel.EffectsWritesFiles, Idempotent: kernel.Yes,
			Sensitive: []string{"/token"}, Interactive: kernel.No,
			Streams: kernel.No, NeedsDisplay: kernel.No,
			Duration: kernel.DurationSeconds, Confirms: kernel.Yes,
			Shape: kernel.ShapeUnary, Summary: "Rebuild",
			Description: "Rebuilds.", Returns: "A count.",
			DryRun: true, Cost: "seconds", Preconditions: []string{"path exists"},
			Promote: true,
		}},
	}
}

func commandsOf(m kernel.CapabilityMap, id string) []string {
	for _, p := range m.Programs {
		if p.Identity.ID != id {
			continue
		}
		out := make([]string, 0, len(p.Commands))
		for _, c := range p.Commands {
			out = append(out, c.ID)
		}
		return out
	}
	return nil
}
