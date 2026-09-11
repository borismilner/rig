package daemon

import (
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// TestRigIsNotAnInvokeTargetOnAnySurface pins the boundary that keeps rig's
// own commands off every `invoke` surface, and it exists because a live run
// walked into it rather than because anybody reasoned about it.
//
// FOUND BY DEMONSTRATION, 2026-09-12. The real-process demonstration of the
// session token tried to read the token from the MCP socket, since
// mint-at-accept covers that listener too. It could not: `invoke(program:
// "rig", command: "session")` came back `meta: no program "rig" is visible to
// this caller`. The mechanism is deliberate and documented at
// registry.go's `command` - rig's declaration is held BESIDE the program map
// rather than in it, so the invoker resolves rig's commands to declared
// effects while View.Programs() never lists rig. meta's invoke reaches the
// program map and only the program map (meta.go: `v.Program(r.Program)`).
//
// WHY IT IS WORTH A TEST RATHER THAN A COMMENT. rig declares `down`, and it
// declares it EffectsDestructive. If rig ever became visible through the
// program projection - by someone "fixing" the asymmetry, or by folding self
// into the map to simplify a lookup - then every MCP agent would acquire
// rig.down as an invoke target in the same change, silently, with no line of
// the diff mentioning it. Nothing in the tree asserted this before now.
//
// THE ASSERTION IS DELIBERATELY ON THE INTROSPECTING PRINCIPAL, which is the
// WIDEST caller rig has: section 14 gives it "everything", and every MCP
// connection is one (newPrincipal sets Introspect unconditionally). If the
// widest caller cannot reach rig through the projection, no caller can.
func TestRigIsNotAnInvokeTargetOnAnySurface(t *testing.T) {
	k := kernel.New()
	if err := declareSelf(k); err != nil {
		t.Fatalf("rig could not declare itself: %v", err)
	}

	// The widest caller in section 14's table: an agent Boris runs, which is
	// what every MCP connection is minted as.
	widest := kernel.Principal{
		UID: 1000, Kind: kernel.KindAgent,
		ClientID: "an-agent", SessionID: "s-an-agent", PID: 1,
		Introspect: true,
	}
	if err := widest.Valid(); err != nil {
		t.Fatalf("the test's own principal is not a valid one: %v", err)
	}

	if _, ok := k.See(widest).Program(kernel.SelfID); ok {
		t.Fatalf("the introspecting caller can resolve program %q through the "+
			"projection, so rig's own commands are now an invoke target on "+
			"every surface that reaches the program map - including the MCP "+
			"socket, and including rig.down, which is declared destructive",
			kernel.SelfID)
	}

	// The other half, so the test fails the day somebody "fixes" this by
	// removing rig's declaration altogether rather than by exposing it: the
	// invoker must still have declared effects to match a house rule on.
	// self.go states that getting `effects` wrong here is what would let a
	// rule denying destructive calls fail to catch the most destructive call
	// rig has.
	var destructive []string
	for _, c := range selfDeclaration().Commands {
		if c.Effects == kernel.EffectsDestructive {
			destructive = append(destructive, c.ID)
		}
	}
	if len(destructive) == 0 {
		t.Fatal("rig's own declaration carries no destructive command, so this " +
			"test is no longer guarding what it was written to guard - if that " +
			"is deliberate, the reasoning above needs rewriting rather than " +
			"this assertion deleting")
	}
	t.Logf("held off the invoke surface while rig declares destructive %v", destructive)
}
