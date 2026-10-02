package daemon

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/coord"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// plan/53 slice 4: the sync rider over the real MCP door.

// rideOf calls a tool and answers the rider's line, empty when none rode.
func rideOf(ctx context.Context, t *testing.T, s *sdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := s.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call tool %s: %v", name, err)
	}
	switch len(res.Content) {
	case 1:
		return ""
	case 2:
		return res.Content[1].(*sdk.TextContent).Text
	}
	t.Fatalf("%s answered %d content blocks", name, len(res.Content))
	return ""
}

// ⛔ WHAT HAPPENED TO YOUR WORK REACHES YOU MID-TASK, ONCE, ON WHATEVER YOU
// CALL NEXT: a peer arriving, a peer queued behind your lease, your claim
// taken over, your lease expired, a signal for you. AgentBox's rider said
// only the first.
func TestTheRiderTellsASeatWhatHappenedToItsWork(t *testing.T) {
	ctx := ctx5(t)
	d := mailDaemon(t)
	watch, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	d.startLeaseWatch(watch) // what notices a lease ran out
	d.serving.Store(&watch)  // what stops the watch on a departed owner
	sock := upMCP(t, d)
	a := dialMCP(ctx, t, sock)
	b := dialMCP(ctx, t, sock)

	if line := rideOf(ctx, t, a, "lease_list", nil); line != "" {
		t.Fatalf("an unseated caller was handed news: %s", line)
	}
	callTool(ctx, t, a, "announce", map[string]any{"seat": "seat-a", "purpose": "building"})
	if line := rideOf(ctx, t, a, "lease_list", nil); line != "" {
		t.Fatalf("news before anything happened: %s", line)
	}
	// seat-a's lease, taken by a process of its that then died, as a build
	// it ran would: rig frees a lease at its deadline only when its holder
	// is gone.
	proc := exec.Command("sleep", "30")
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	w, err := coord.WitnessProcess(proc.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.leases.Acquire("build", "seat-a", w, 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	_ = proc.Process.Kill()
	_ = proc.Wait()
	claim := callTool(ctx, t, a, "shared_set", map[string]any{"key": "claims.x", "value": 1, "own": true})
	version := resultOf(t, claim)["value"].(map[string]any)["version"]
	callTool(ctx, t, b, "announce", map[string]any{"seat": "seat-b", "purpose": "reviewing"})
	if line := rideOf(ctx, t, a, "lease_list", nil); !strings.Contains(line, `seat-b arrived ("reviewing")`) {
		t.Fatalf("an arrival was not told: %q", line)
	}

	// seat-a's lease runs out while seat-b queues for it, and seat-b does two
	// more things to seat-a's work. seat-a asks about none of them.
	if line := rideOf(ctx, t, b, "lease_acquire", map[string]any{"name": "build", "ttlMs": 5000, "waitMs": 2000}); line != "" {
		t.Fatalf("seat-b was told seat-a's own doings: %s", line)
	}
	callTool(ctx, t, b, "shared_set", map[string]any{"key": "claims.x", "value": 2, "own": true, "expectedVersion": version})
	callTool(ctx, t, b, "events_publish", map[string]any{"kind": "signal.review.ready", "payloadJson": "{}", "toSeat": "seat-a"})
	callTool(ctx, t, b, "events_publish", map[string]any{"kind": "signal.everyone", "payloadJson": "{}"})

	line := rideOf(ctx, t, a, "set_activity", map[string]any{"activity": "still building"})
	for _, want := range []string{
		"seat-b is waiting on your lease build",
		"your claim claims.x was taken by seat-b",
		"your lease build expired",
		"signal.review.ready for you from seat-b",
		"events_wait after ",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the rider lacks %q:\n%s", want, line)
		}
	}
	for _, never := range []string{"arrived", "signal.everyone"} {
		if strings.Contains(line, never) {
			t.Errorf("the rider repeated or broadcast %q:\n%s", never, line)
		}
	}
	if again := rideOf(ctx, t, a, "lease_list", nil); again != "" {
		t.Fatalf("news was told twice: %s", again)
	}
}

// events_wait answers only the kinds it was asked for, so it neither rides
// nor spends the news the next call owes.
func TestEventsWaitDoesNotSpendTheNews(t *testing.T) {
	ctx := ctx5(t)
	sock := upMCP(t, mailDaemon(t))
	a := dialMCP(ctx, t, sock)
	callTool(ctx, t, a, "announce", map[string]any{"seat": "seat-a", "purpose": "building"})
	b := dialMCP(ctx, t, sock)
	callTool(ctx, t, b, "announce", map[string]any{"seat": "seat-b", "purpose": "reviewing"})

	if line := rideOf(ctx, t, a, "events_wait", map[string]any{"patterns": []string{"signal.none"}, "timeoutMs": 1}); line != "" {
		t.Fatalf("events_wait carried a rider: %s", line)
	}
	if line := rideOf(ctx, t, a, "lease_list", nil); !strings.Contains(line, "seat-b arrived") {
		t.Fatalf("events_wait spent the arrival: %q", line)
	}
}

func TestTheRiderSaysHowMuchItLeftOut(t *testing.T) {
	items := make([]string, riderMax+3)
	for i := range items {
		items[i] = "x"
	}
	if line := riderLine(items, 7, false); !strings.Contains(line, "and 3 more") || strings.Count(line, "x") != riderMax {
		t.Fatalf("a long line: %s", line)
	}
	if line := riderLine(nil, 7, true); !strings.Contains(line, "lost") {
		t.Fatalf("a gap with nothing else was not told: %q", line)
	}
	if riderLine(nil, 7, false) != "" {
		t.Fatal("nothing to say said something")
	}
}

// A lease another seat broke is told to its holder by name: the holder's own
// process still lives, so only a break frees it, and the breaker is who
// answers for it.
func TestTheRiderTellsAHolderWhoBrokeItsLease(t *testing.T) {
	ctx := ctx5(t)
	d := mailDaemon(t)
	watch, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	d.startLeaseWatch(watch)
	d.serving.Store(&watch)
	sock := upMCP(t, d)
	a := dialMCP(ctx, t, sock)
	b := dialMCP(ctx, t, sock)
	callTool(ctx, t, a, "announce", map[string]any{"seat": "seat-a", "purpose": "deploying"})
	callTool(ctx, t, b, "announce", map[string]any{"seat": "seat-b", "purpose": "on call"})
	rideOf(ctx, t, a, "lease_list", nil) // seat-b's arrival, told and gone

	callTool(ctx, t, a, "lease_acquire", map[string]any{"name": "deploy", "ttlMs": 300})
	time.Sleep(600 * time.Millisecond) // past the deadline, its witness alive: orphaned
	if line := rideOf(ctx, t, b, "lease_break", map[string]any{"name": "deploy", "reason": "seat-a is stuck"}); strings.Contains(line, "broken") {
		t.Fatalf("the breaker was told its own break: %s", line)
	}
	line := rideOf(ctx, t, a, "set_activity", map[string]any{"activity": "deploying"})
	if !strings.Contains(line, "your lease deploy was broken by seat-b") {
		t.Fatalf("the holder was not told who broke its lease:\n%s", line)
	}
	if strings.Contains(line, "expired") {
		t.Fatalf("a break was told as an expiry:\n%s", line)
	}
	if again := rideOf(ctx, t, a, "lease_list", nil); strings.Contains(again, "broken") {
		t.Fatalf("the break was told twice: %s", again)
	}
}
