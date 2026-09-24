package daemon

import (
	"strings"
	"testing"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/mcpserver"
	"github.com/borismilner/rig/internal/meta"
)

// Section 40's lessons through the agent door, end to end: an agent that has
// not announced is refused a write, an announced one writes under its own
// seat, and search then get finds what it wrote.
func TestAnAgentWritesALessonAndAnotherSearchFindsIt(t *testing.T) {
	_, d := upDaemon(t, nil)
	if d.records == nil {
		t.Skip("this build gives an unnamed estate no scratch store")
	}
	ctx := ctx5(t)
	// The MCP socket mints a session token at accept; the roster helper's
	// principal has none, and a lesson refuses to be written without one. The
	// caller carries it the way the socket hands it over.
	who := principalOfKind(kernel.KindAgent, "an-agent")
	who.Token = "session-agent"
	agent := connectTo(ctx, t, mcpserver.New(
		meta.New(d.kernel, &mcpCaller{Daemon: d, occ: &occupancy{}, who: who}), who, "test"))

	refused := refusedTool(ctx, t, agent, "knowledge_add", map[string]any{
		"title": "t", "summary": "s", "body": "b",
	})
	if !strings.Contains(refused["error"]+refused["fix"], "seat") {
		t.Errorf("an unseated write was refused without naming the seat: %v", refused)
	}

	callTool(ctx, t, agent, "announce", map[string]any{
		"seat": "backend-1", "purpose": "learning",
	})
	added := callTool(ctx, t, agent, "knowledge_add", map[string]any{
		"title":   "WAL checkpoints stall under a long reader",
		"summary": "a reader held open across a checkpoint grows the WAL",
		"body":    "Close readers before a checkpoint, or the WAL file keeps growing.",
		"tags":    []string{"sqlite"},
	})
	lesson, _ := added["record"].(map[string]any)["lesson"].(map[string]any)
	if lesson["seat"] != "backend-1" {
		t.Fatalf("the lesson carries seat %v, want the announced one: %v", lesson["seat"], added)
	}
	id, _ := lesson["id"].(string)

	found := callTool(ctx, t, agent, "knowledge_search", map[string]any{"query": "checkpoint reader"})
	hits, _ := found["record"].(map[string]any)["hits"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["id"] != id {
		t.Fatalf("search did not find the lesson just written: %v", found)
	}
	if _, has := hits[0].(map[string]any)["body"]; has {
		t.Error("a search hit carried the body; hits are for choosing, get is for reading")
	}

	got := callTool(ctx, t, agent, "knowledge_get", map[string]any{"id": id})
	whole, _ := got["record"].(map[string]any)["lesson"].(map[string]any)
	if !strings.Contains(whole["body"].(string), "Close readers") {
		t.Fatalf("get did not return the whole lesson: %v", got)
	}
}
