package kernel_test

import (
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// TestAProgramCanWeakenItsOwnEffectsByReDeclaring records section 13a's
// fourth clause as a hole nobody has ruled on, so it is a decision next time
// rather than a discovery. It LOCKS the behaviour; it does not close it.
//
// Register refuses only a DIFFERENT session (internal/kernel/registry.go:
// prev.owner.SessionID != p.SessionID). The same session re-registers freely
// and the entry is replaced wholesale, with no comparison of the old
// declaration to the new. So a program may declare a command destructive,
// be matched by a house rule, and then declare the same command writes-files
// - after which the rule simply stops matching.
//
// THE PART THAT MAKES IT A HOLE RATHER THAN A BUG: no rule is violated and
// no denial is recorded, because a rule that stops matching produces no
// event at all. The failure is silent by construction, which is why it needs
// a test rather than a log line.
//
// What would close it is a specification, not code: whether a re-declaration
// may weaken effects at all, and if not, what rig does with the difference.
// Writing a refusal here without that ruling would be inventing the rule.
func TestAProgramCanWeakenItsOwnEffectsByReDeclaring(t *testing.T) {
	k := kernel.New()
	owner := programPrincipal("pilot")

	if _, err := k.Register(owner, withCommands("pilot",
		map[string]kernel.Effects{"destroy": kernel.EffectsDestructive},
	)); err != nil {
		t.Fatalf("register: %v", err)
	}

	// The house rule from section 13a's own vocabulary: an agent calling
	// anything destructive is gated.
	if err := k.SetRules([]kernel.Rule{{
		ID:      "agents-confirm-destruction",
		Caller:  kernel.CallerKind(kernel.KindAgent),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionConfirm,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	ref := []kernel.Ref{{
		Kind: kernel.RefCommand, Program: "pilot", Command: "destroy",
	}}
	an := agent()

	before, err := k.Authorize(an, ref)
	if err != nil {
		t.Fatalf("authorize before: %v", err)
	}
	if before.Action != kernel.ActionConfirm {
		t.Fatalf("the rule did not match a destructive command, so this test "+
			"proves nothing: %v", before.Action)
	}
	// Quoted: an identified rule is cited as %q so the audit log can tell an
	// id from a position (rules[3]).
	if before.Rule != `"agents-confirm-destruction"` {
		t.Fatalf("a different rule matched: %s", before.Rule)
	}

	// The SAME session re-declares the SAME command, one level weaker.
	// Nothing else about the program changes.
	if _, err := k.Register(owner, withCommands("pilot",
		map[string]kernel.Effects{"destroy": kernel.EffectsWritesFiles},
	)); err != nil {
		t.Fatalf("the same session could not re-register: %v", err)
	}

	after, err := k.Authorize(an, ref)
	if err != nil {
		t.Fatalf("authorize after: %v", err)
	}
	if after.Action != kernel.ActionAllow {
		t.Fatalf("the weakened declaration was still gated as %v; the hole "+
			"this test locks has been closed, and the test should be "+
			"rewritten against whatever closed it", after.Action)
	}
	if after.Rule != "" {
		t.Errorf("a rule is cited for a call nothing matched: %q", after.Rule)
	}
	if after.Pair == before.Pair {
		t.Errorf("the pair did not change, so the effects did not: %v", after.Pair)
	}
}

// The other half of this - that a DIFFERENT session cannot take a registered
// program's name - is already locked by TestATakenIDIsRefusedToADifferentSession
// in this package. It is worth reading beside this one: the guard that exists
// works, and it is scoped to the OWNER rather than to the declaration. That is
// exactly the gap above.
