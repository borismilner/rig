package kernel_test

import (
	"testing"

	"github.com/borismilner/rig/internal/kernel"
)

// Section 54: a program at rest is listed, filtered and resolved from its
// kept declaration exactly as a running one is.
func TestAKeptDeclarationIsListedAndResolvesWithNoProcess(t *testing.T) {
	k := kernel.New()
	d := good("beacon")
	d.Commands[0].ID = "ask"
	if err := k.Rest(d); err != nil {
		t.Fatal(err)
	}
	all := agent()
	all.Introspect = true
	if got := k.See(all).Programs(); len(got) != 1 || got[0].Identity.ID != "beacon" {
		t.Fatalf("listed %v", ids(got))
	}
	if _, ok := k.See(all).Command("beacon", "ask"); !ok {
		t.Fatal("a command at rest did not resolve")
	}
	if err := k.ValidateArgs("beacon", "ask", nil); err != nil {
		t.Fatalf("a call at rest was refused before the start: %v", err)
	}
	if err := k.ValidateArgs("beacon", "nope", nil); err == nil {
		t.Fatal("an undeclared command at rest was accepted")
	}
	if k.DeclaredDuration("beacon", "ask") != kernel.DurationSeconds {
		t.Fatal("the deadline of a command at rest is unknown")
	}

	// Section 14 is the same filter: a client not in beacon's scope does not
	// see it at rest either.
	if _, ok := k.See(agent("other")).Program("beacon"); ok {
		t.Fatal("a resting program was seen outside its scope")
	}

	// A live registration is read first and lists once.
	if _, err := k.Register(programPrincipal("beacon"), good("beacon")); err != nil {
		t.Fatal(err)
	}
	got := k.See(all).Programs()
	if len(got) != 1 || got[0].Commands[0].ID != "reindex" {
		t.Fatalf("with a live registration it listed %+v", got)
	}

	k.Unrest("beacon")
	k.Deregister("s-beacon")
	if _, ok := k.See(all).Program("beacon"); ok {
		t.Fatal("an unrested, deregistered program is still listed")
	}
}
