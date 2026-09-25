package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/mcpserver"
	"github.com/borismilner/rig/internal/meta"
)

// mailDaemon is a named estate with a message store and no socket: the MCP
// door is reached in memory, so nothing needs to be served.
func mailDaemon(t *testing.T) *Daemon {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	st, err := coord.Open("mailmcp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	d, err := New(Config{
		Version: "test", Wire: "v1", Lock: lock, Estate: "mailmcp",
		Epoch: st.Epoch(), Leases: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if d.records != nil {
			_ = d.records.Close()
		}
	})
	return d
}

func mailAgent(t *testing.T, d *Daemon, name string) *sdk.ClientSession {
	t.Helper()
	who := principalOfKind(kernel.KindAgent, name)
	return connectTo(ctx5(t), t, mcpserver.New(
		meta.New(d.kernel, &mcpCaller{Daemon: d, occ: &occupancy{}, who: who}), who, "test"))
}

// Section 16's directed messages through the agent door: the sender is the
// caller's seat, the recipient reads its own inbox, acted_on reaches the
// sender through message_list, and a stale pin is refused with the fields an
// agent acts on rather than a bare sentence.
func TestAnAgentSendsAMessageAndThePeerActsOnIt(t *testing.T) {
	d := mailDaemon(t)
	ctx := ctx5(t)
	lead := mailAgent(t, d, "lead")
	worker := mailAgent(t, d, "worker")

	refused := refusedTool(ctx, t, lead, "message_send", map[string]any{
		"to": "backend-1", "subject": "hello",
	})
	if !strings.Contains(refused["error"], "no seat") {
		t.Errorf("an unannounced sender was refused without naming the seat: %v", refused)
	}

	callTool(ctx, t, lead, "announce", map[string]any{"seat": "team-lead", "purpose": "leading"})
	joined := callTool(ctx, t, worker, "announce", map[string]any{"seat": "backend-1", "purpose": "building"})
	you, _ := joined["crew"].(map[string]any)["you"].(map[string]any)

	// A pin to a tenancy that is not the one in the seat is the feature.
	stale := refusedTool(ctx, t, lead, "message_send", map[string]any{
		"to": "backend-1", "subject": "for the old session",
		"to_generation": you["generation"].(float64) + 1, "to_epoch": you["epoch"],
	})
	if !strings.Contains(stale["error"], "somebody else holds the seat") || stale["fix"] == "" {
		t.Fatalf("a stale pin was not refused with a fix an agent can act on: %v", stale)
	}

	sent := callTool(ctx, t, lead, "message_send", map[string]any{
		"to": "backend-1", "subject": "the proto is regenerated", "body": "pull first",
		"to_generation": you["generation"], "to_epoch": you["epoch"],
	})
	msg, _ := sent["mail"].(map[string]any)["sent"].(map[string]any)["message"].(map[string]any)
	if msg["from"] != "team-lead" || msg["state"] != "queued" {
		t.Fatalf("send answered %v", sent)
	}

	box := callTool(ctx, t, worker, "message_inbox", map[string]any{})
	batch, _ := box["mail"].(map[string]any)["batch"].(map[string]any)
	got, _ := batch["messages"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["state"] != "read" {
		t.Fatalf("the inbox answered %v", box)
	}
	if mis, _ := batch["misaddressed"].([]any); len(mis) != 0 {
		t.Fatalf("a message pinned to the reader was flagged misaddressed: %v", mis)
	}

	callTool(ctx, t, worker, "message_ack", map[string]any{
		"id": msg["id"], "state": "acted_on", "outcome": "pulled",
	})
	listed := callTool(ctx, t, lead, "message_list", map[string]any{"seat": "backend-1"})
	all, _ := listed["mail"].(map[string]any)["listed"].(map[string]any)["messages"].([]any)
	if len(all) != 1 || all[0].(map[string]any)["state"] != "acted_on" ||
		all[0].(map[string]any)["outcome"] != "pulled" {
		t.Fatalf("the sender cannot see that its message was acted on: %v", listed)
	}

	empty := callTool(ctx, t, lead, "message_list", map[string]any{"seat": "nobody"})
	if l, _ := empty["mail"].(map[string]any)["listed"].(map[string]any)["messages"].([]any); l == nil {
		t.Fatalf("an empty list vanished instead of answering []: %v", empty)
	}
}
