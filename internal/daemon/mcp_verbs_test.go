package daemon

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/mcpserver"
	"github.com/borismilner/rig/internal/meta"
)

// plan/09, "The MCP door covers everything rig does": every verb rig declares
// is a tool on the agent door or on the written exclusion list, and a verb
// that is neither fails here - which is the whole guard, since the next verb
// added to self.go is the one that would otherwise be missing.
func TestEveryRigVerbIsOnTheAgentDoor(t *testing.T) {
	declared := map[string]bool{}
	for _, c := range selfDeclaration().Commands {
		declared[c.ID] = true
		_, b := bridged[c.ID]
		_, s := servedByName[c.ID]
		_, x := notOnTheAgentDoor[c.ID]
		if n := btoi(b) + btoi(s) + btoi(x); n != 1 {
			t.Errorf("rig.%s is on %d of the three lists (bridged, served by name, excluded); "+
				"plan/09 wants exactly one", c.ID, n)
		}
	}
	for _, list := range []map[string]bool{keys(bridged), keys(servedByName), keys(notOnTheAgentDoor)} {
		for id := range list {
			if !declared[id] {
				t.Errorf("rig.%s is listed for the agent door but rig does not declare it", id)
			}
		}
	}

	// And the tools are really there: an agent's tools/list carries every one.
	ctx := ctx5(t)
	agent := mailAgent(t, mailDaemon(t), "lister")
	res, err := agent.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]*sdk.Tool{}
	for _, tl := range res.Tools {
		have[tl.Name] = tl
	}
	for id := range bridged {
		if have[verbTool(id)] == nil {
			t.Errorf("rig.%s has no tool %q on the agent door", id, verbTool(id))
		}
	}
	for id, tool := range servedByName {
		if have[string(tool)] == nil {
			t.Errorf("rig.%s is said to be served by %q, and the agent door has no such tool", id, tool)
		}
	}
	for id := range notOnTheAgentDoor {
		if have[verbTool(id)] != nil {
			t.Errorf("rig.%s is excluded by plan/09 and is on the agent door anyway", id)
		}
	}
	if st := have["stop"]; st == nil || st.Annotations == nil || st.Annotations.DestructiveHint == nil ||
		!*st.Annotations.DestructiveHint || st.Annotations.ReadOnlyHint {
		t.Errorf("stop is destructive and its tool does not say so: %+v", st)
	}
	if h := have["lease_list"]; h == nil || h.Annotations == nil || !h.Annotations.ReadOnlyHint {
		t.Errorf("lease_list is read-only and its tool does not say so: %+v", h)
	}
}

// The bridged verbs run through the daemon's own dispatch AS THE AGENT: a
// lease and a claim are held under the seat it announced, a refusal arrives
// with the fields an agent acts on, and arguments that do not fit the schema
// are refused before they reach the daemon.
func TestAnAgentHoldsALeaseAndWorksAQueueThroughTheDoor(t *testing.T) {
	d := mailDaemon(t)
	ctx := ctx5(t)
	// A terminal principal, because that is what the MCP socket mints
	// (mcp_listen.go). An agent-kind principal here hid the defect this
	// test now pins: the terminal fallback naming an unannounced agent.
	who := principalOfKind(kernel.KindTerminal, "worker")
	agent := connectTo(ctx, t, mcpserver.New(
		meta.New(d.kernel, &mcpCaller{Daemon: d, occ: &occupancy{}, who: who}), who, "test"))

	unseated := refusedTool(ctx, t, agent, "lease_acquire", map[string]any{"name": "build", "ttlMs": 5000})
	if !strings.Contains(unseated["error"], "holds no seat") || unseated["fix"] == "" {
		t.Fatalf("an unannounced agent's lease was not refused with a fix: %v", unseated)
	}

	callTool(ctx, t, agent, "announce", map[string]any{"seat": "backend-1", "purpose": "building"})
	got := callTool(ctx, t, agent, "lease_acquire", map[string]any{"name": "build", "ttl_ms": 5000})
	h, _ := resultOf(t, got)["handle"].(map[string]any)
	if h["holder"] != "backend-1" || h["name"] != "build" {
		t.Fatalf("the lease was not held under the agent's seat: %v", got)
	}

	listed := resultOf(t, callTool(ctx, t, agent, "lease_list", nil))
	ls, _ := listed["leases"].([]any)
	if len(ls) != 1 || ls[0].(map[string]any)["holder"] != "backend-1" {
		t.Fatalf("lease_list answered %v", listed)
	}

	callTool(ctx, t, agent, "queue_push", map[string]any{"queue": "jobs", "idempotencyKey": "job-1", "payload": "aGk="})
	claimed := resultOf(t, callTool(ctx, t, agent, "queue_claim", map[string]any{"queue": "jobs", "ttlMs": 5000}))
	if ch, _ := claimed["handle"].(map[string]any); ch["holder"] != "backend-1" {
		t.Fatalf("the claim was not held under the agent's seat: %v", claimed)
	}

	bad := refusedTool(ctx, t, agent, "lease_acquire", map[string]any{"label": "not a field"})
	if !strings.Contains(bad["error"], "do not fit") {
		t.Fatalf("an unknown argument reached the daemon: %v", bad)
	}

	// A daemon built without a supervisor says so, in the daemon's own words.
	none := refusedTool(ctx, t, agent, "health", nil)
	if !strings.Contains(none["error"], "supervises nothing") {
		t.Fatalf("health on a daemon with no supervisor answered %v", none)
	}
}

func resultOf(t *testing.T, ans map[string]any) map[string]any {
	t.Helper()
	r, ok := ans["result"].(map[string]any)
	if !ok {
		t.Fatalf("the answer carries no result object: %v", ans)
	}
	return r
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func keys[V any](m map[string]V) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}
