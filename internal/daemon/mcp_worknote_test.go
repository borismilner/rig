package daemon

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/mcpserver"
	"github.com/borismilner/rig/internal/meta"
)

// Section 09's done-when, through the agent's own door: a seat writes notes,
// its session dies, and the next session in the SAME seat gets them back.
// Another seat does not, and a note attached to a record is found from it.
func TestWorkingNotesComeBackToTheSeatThroughTheMCPDoor(t *testing.T) {
	_, d := upDaemon(t, nil)
	if d.records == nil {
		t.Skip("this build gives an unnamed estate no scratch store")
	}
	ctx := ctx5(t)
	dial := func(token string, occ *occupancy) *sdk.ClientSession {
		who := principalOfKind(kernel.KindAgent, token)
		who.Token = token
		return connectTo(ctx, t, mcpserver.New(
			meta.New(d.kernel, &mcpCaller{Daemon: d, occ: occ, who: who}), who, "test"))
	}

	firstOcc := &occupancy{}
	first := dial("session-1", firstOcc)
	refused := refusedTool(ctx, t, first, "worknote_write", map[string]any{
		"project": "rig", "body": "before announcing",
	})
	if refused["fix"] == "" {
		t.Fatalf("an unseated note was not refused with a fix: %v", refused)
	}
	callTool(ctx, t, first, "announce", map[string]any{"seat": "backend-1", "purpose": "building"})
	callTool(ctx, t, first, "record_put", map[string]any{
		"id": "wi-7", "project": "rig", "kind": "work-item", "body": "the thing",
	})
	wrote := callTool(ctx, t, first, "worknote_write", map[string]any{
		"project": "rig", "body": "tried the WAL fix; it did not hold",
		"tags": []string{"sqlite", "dead-end"}, "part_of": []string{"wi-7", "no-such-id"},
	})
	note, _ := wrote["record"].(map[string]any)["note"].(map[string]any)
	if note["seat"] != "backend-1" {
		t.Fatalf("the note is signed %v, want the announced seat: %v", note["seat"], wrote)
	}
	if m, _ := note["missing"].([]any); len(m) != 1 || m[0] != "no-such-id" {
		t.Fatalf("a missing attachment was not reported: %v", note)
	}
	callTool(ctx, t, first, "worknote_write", map[string]any{"project": "rig", "body": "second thought"})

	// The session dies: its row goes, exactly as the connection's defer does it.
	d.presence.leave(firstOcc)

	next := dial("session-2", &occupancy{})
	callTool(ctx, t, next, "announce", map[string]any{"seat": "backend-1", "purpose": "resuming"})
	mine := callTool(ctx, t, next, "worknote_mine", map[string]any{})
	list, _ := mine["record"].(map[string]any)["notes"].(map[string]any)
	notes, _ := list["notes"].([]any)
	if len(notes) != 2 || list["total"] != float64(2) {
		t.Fatalf("the successor got %d notes (total %v), want both: %v", len(notes), list["total"], mine)
	}
	if notes[0].(map[string]any)["body"] != "second thought" {
		t.Errorf("notes are not newest first: %v", notes)
	}

	other := dial("session-3", &occupancy{})
	callTool(ctx, t, other, "announce", map[string]any{"seat": "frontend-1", "purpose": "elsewhere"})
	theirs := callTool(ctx, t, other, "worknote_mine", map[string]any{})
	if l, _ := theirs["record"].(map[string]any)["notes"].(map[string]any)["notes"].([]any); l == nil || len(l) != 0 {
		t.Fatalf("another seat saw backend-1's notes, or an empty list vanished: %v", theirs)
	}

	about := callTool(ctx, t, other, "worknote_about", map[string]any{"id": "wi-7"})
	on, _ := about["record"].(map[string]any)["notes"].(map[string]any)["notes"].([]any)
	if len(on) != 1 || on[0].(map[string]any)["body"] != "tried the WAL fix; it did not hold" {
		t.Fatalf("the note attached to wi-7 was not found from it: %v", about)
	}
}
