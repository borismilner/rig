package daemon

import (
	"os"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// principalOfKind builds a caller that differs from the next one in exactly
// one respect: its kind. Everything else is held constant so a test asserting
// two different answers is asserting about the principal and nothing else.
func principalOfKind(kind kernel.ClientKind, id string) kernel.Principal {
	return kernel.Principal{
		UID: os.Getuid(), Kind: kind,
		ClientID: id, SessionID: "s-" + id, PID: 1,
		Introspect: true,
	}
}

// TestTheInvokerMeetsTheFloorAndTheCALLERDecides is the lock on section 13a's
// rule that no path to an invocation may drop the caller.
//
// THE TWO CALLS ARE THE TEST. One rule is written, and it names a caller KIND
// rather than any caller - so an agent is refused and a terminal is not. If
// meta.Invoker stopped carrying the principal, or the daemon started
// authorising as itself, both calls would get the SAME answer. A test that
// invoked once and asserted a refusal would pass against a daemon that refuses
// everyone, which is the failure mode worth guarding.
//
// This is not a hypothetical. The interface shipped without a principal, the
// layer above used one for its visibility check and then dropped it, and the
// floor was not bypassed - it was made unreachable by a type signature.
func TestTheInvokerMeetsTheFloorAndTheCALLERDecides(t *testing.T) {
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "no-destructive-agents", Caller: kernel.CallerKind(kernel.KindAgent),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	_, err := d.Invoke(ctx5(t), principalOfKind(kernel.KindAgent, "an-agent"),
		"shelf", "destroy", nil)
	if err == nil {
		t.Fatal("an agent ran a destructive command its house rule denies")
	}
	// The refusal names the RULE and the PAIR it matched, and the pair is the
	// assertion that matters: "(agent, destructive)" is only reachable if the
	// caller's kind reached the floor. A refusal naming the command alone
	// would be satisfied by a daemon authorising as itself.
	for _, want := range []string{`"no-destructive-agents"`, "(agent, destructive)", "deny"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not show the floor matched on the caller: "+
				"wanted %q in %v", want, err)
		}
	}

	// Same command, same estate, same instant. Only the caller differs, and
	// the rule does not match it.
	if _, err := d.Invoke(ctx5(t), principalOfKind(kernel.KindTerminal, "a-terminal"),
		"shelf", "destroy", nil); err != nil {
		t.Fatalf("a terminal was refused by a rule written against agents, "+
			"so the caller is not reaching the floor: %v", err)
	}
}

// TestTheInvokerCannotReachAProgramThePrincipalCannotSee is the visibility
// half, which is a DIFFERENT mechanism from the authorisation half above.
//
// Conflating the two is what hid the missing principal: the scope filter was
// locked by a test, and having locked it the "who is calling" question felt
// answered. It was not. Both halves are asserted here, next to each other, so
// the pair is visible to whoever reads this file next.
func TestTheInvokerCannotReachAProgramThePrincipalCannotSee(t *testing.T) {
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)

	// A scoped program that is not shelf and shares no scope with it.
	other := kernel.Principal{
		UID: os.Getuid(), Kind: kernel.KindProgram,
		ClientID: "docket", SessionID: "s-docket", PID: 2,
		Scoped: true,
	}

	_, err := d.Invoke(ctx5(t), other, "shelf", "destroy", nil)
	if err == nil {
		t.Fatal("a scoped program invoked a program it cannot see")
	}
	// Section 14: missing, not refused. A distinguishable refusal here is an
	// existence oracle for every program on the box - "you may not see shelf"
	// tells the caller that shelf exists.
	if !strings.Contains(err.Error(), "no program") {
		t.Fatalf("the refusal distinguishes hidden from absent: %v", err)
	}
}

// TestTheInvokerReturnsWhatTheProgramReturned keeps the daemon a pipe.
//
// Section 5d: the client is a dumb pipe, and a daemon that reshaped a
// program's own result would be the opposite. The in-process path unwraps a
// CallResponse where the wire path relays a frame, so this is the assertion
// that the unwrapping does not become interpretation.
func TestTheInvokerReturnsWhatTheProgramReturned(t *testing.T) {
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)

	out, err := d.Invoke(ctx5(t), principalOfKind(kernel.KindTerminal, "a-terminal"),
		"shelf", "destroy", nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(string(out), "shelf.destroy") {
		t.Fatalf("the program's own result did not survive the invoker: %s", out)
	}
}
