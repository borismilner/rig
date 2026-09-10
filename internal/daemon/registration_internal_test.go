package daemon

import (
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The two effects maps are the only place a wire value becomes a kernel value,
// and a mistake in either is silent: the wrong level authorises, the call
// succeeds, and nothing reports a mapping error because there is no error to
// report. `EFFECTS_DRIVES_INPUT` was added by editing both maps by hand, and
// mapping it onto `EffectsDestructive` in one of them passed every test in the
// repository.
//
// So this asserts the property rather than the entries: the maps are exact
// inverses of each other, and between them they cover every named value except
// the unspecified zero, which is refused at the boundary rather than mapped.
// A value added to one map and not the other fails here on the day it is added.

func TestTheEffectsMapsAreExactInverses(t *testing.T) {
	for wire, k := range effectsIn {
		back, ok := effectsOut[k]
		if !ok {
			t.Errorf("%s maps in to %s and %s maps back to nothing", wire, k, k)
			continue
		}
		if back != wire {
			t.Errorf("%s maps in to %s, which maps back out to %s", wire, k, back)
		}
	}
	for k, wire := range effectsOut {
		back, ok := effectsIn[wire]
		if !ok {
			t.Errorf("%s maps out to %s and %s maps back to nothing", k, wire, wire)
			continue
		}
		if back != k {
			t.Errorf("%s maps out to %s, which maps back in to %s", k, wire, back)
		}
	}
}

func TestEveryNamedEffectsValueIsMapped(t *testing.T) {
	// Walks the generated name table, so a value added to the proto and
	// forgotten in the maps is caught by the generator's own output rather
	// than by a list maintained here.
	for v, name := range rigv1.Effects_name {
		wire := rigv1.Effects(v)
		if wire == rigv1.Effects_EFFECTS_UNSPECIFIED {
			if _, ok := effectsIn[wire]; ok {
				t.Errorf("%s is mapped, and a meaningful zero is exactly what "+
					"PLAN.md section 21 bans: an unset field would decode as a "+
					"decision", name)
			}
			continue
		}
		k, ok := effectsIn[wire]
		if !ok {
			t.Errorf("%s is declared in the proto and maps to no kernel value, "+
				"so a program declaring it registers as unspecified", name)
			continue
		}
		if k == kernel.EffectsUnspecified {
			t.Errorf("%s maps to the unspecified kernel value", name)
			continue
		}
		// And it has to be NAMEABLE, which is a separate thing from being
		// mapped and fails silently in a different place. A house rule is
		// written as text (`effects = "drives-input"`) and parsed with
		// ParseEffects, so a level that maps correctly but carries no name
		// renders as Effects(5), refuses to parse in a rule, and the rule the
		// level exists for cannot be written at all.
		if back, err := kernel.ParseEffects(k.String()); err != nil {
			t.Errorf("%s maps to %s, which does not parse back from its own "+
				"name: a house rule cannot name this level", name, k)
		} else if back != k {
			t.Errorf("%s maps to %s, whose name parses back as %s", name, k, back)
		}
	}
}

func TestDrivesInputDoesNotArriveAsDestructive(t *testing.T) {
	// Named on its own because the collapse is the plausible mistake: the two
	// levels sit next to each other, the words are both alarming, and a rule
	// written for one would then silently decide the other. This is the
	// assertion PLAN.md section 5m's whole argument rests on.
	got := commandFromWire(&rigv1.Command{
		Id:      "type",
		Effects: rigv1.Effects_EFFECTS_DRIVES_INPUT,
	})
	if got.Effects != kernel.EffectsDrivesInput {
		t.Fatalf("a command declaring drives-input registered as %s", got.Effects)
	}
	if got.Effects == kernel.EffectsDestructive {
		t.Fatal("drives-input collapsed onto destructive, so a house rule " +
			"about typing into windows would also decide file deletion")
	}
}
