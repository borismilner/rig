package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/instance"
	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// down calls rig.down and returns the answer, so a test asserts on the reply
// rather than on the shutdown alone. Getting an ANSWER is half the property.
func down(t *testing.T, c *client.Client) (*rigv1.DownResponse, error) {
	t.Helper()
	resp := &rigv1.DownResponse{}
	return resp, c.Call(ctx5(t), "rig.down", &rigv1.DownRequest{}, resp)
}

// stoppedWithin reports whether the socket has stopped accepting, polling
// because the daemon replies BEFORE it stops and the stop is therefore
// asynchronous with respect to the answer the caller already holds.
//
// The window is a parameter because the two directions cost differently. A
// stop that is going to happen happens in microseconds, so the positive case
// only needs a generous ceiling it will never reach. Proving a daemon did NOT
// stop means waiting out the whole window every time, so that one is kept
// short deliberately - a suite pays it on every run.
func stoppedWithin(sock string, window time.Duration) bool {
	deadline := time.Now().Add(window)
	for {
		c, err := client.Dial(sock)
		if err != nil {
			return true
		}
		_ = c.Close()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func stopped(sock string) bool { return stoppedWithin(sock, 2*time.Second) }

// keptServing is the negative, and it is not !stopped(): it uses a short
// window because it always waits the whole of it.
func keptServing(sock string) bool { return !stoppedWithin(sock, 250*time.Millisecond) }

// TestRigDownAnswersAndThenStops is the whole feature in one test: the caller
// gets a real answer, and the daemon is gone afterwards.
func TestRigDownAnswersAndThenStops(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	resp, err := down(t, dial(t, sock))
	if err != nil {
		t.Fatalf("rig.down was refused: %v", err)
	}

	// The pid identifies WHICH daemon agreed to stop. Two estates run on one
	// machine under different XDG_RUNTIME_DIRs and `pgrep -x rigd` cannot
	// tell them apart, so an answer that omitted this would not say what it
	// had acted on.
	if got, want := resp.GetPid(), int32(os.Getpid()); got != want {
		t.Errorf("rig.down answered pid %d, want this process %d", got, want)
	}
	if resp.GetVersion() != "test" {
		t.Errorf("rig.down answered version %q, want the daemon's own", resp.GetVersion())
	}

	if !stopped(sock) {
		t.Fatal("the daemon answered rig.down and kept serving")
	}
}

// TestRigDownAnswersRatherThanDroppingTheCaller checks that the caller gets a
// well-formed answer rather than a dropped connection.
//
// IT DOES NOT LOCK THE REPLY-BEFORE-STOP ORDERING, and the name says so
// because an earlier version of it claimed to. Measured: a mutation that moved
// stop() above the reply passed this test and the whole suite. The wrong order
// is a RACE - closeLive() runs on another goroutine and the reply almost
// always wins - so a single run cannot distinguish the orders, and a test that
// catches a race one run in ten is worse than none.
//
// The ordering is therefore enforced by construction in replyThenStop's defer,
// not here. This test still earns its place: it catches the reply being
// dropped, malformed or never written at all, which are not races.
func TestRigDownAnswersRatherThanDroppingTheCaller(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	resp, err := down(t, dial(t, sock))
	if err != nil {
		t.Fatalf("rig.down did not answer its caller: %v", err)
	}
	if resp.GetPid() == 0 {
		t.Fatal("rig.down answered an empty response")
	}
}

// TestAHouseRuleCanRefuseRigDown is the architectural point of building this
// as a wire method rather than as a pidfile and a signal.
//
// The rejected shape - read rigd.pid, send SIGTERM - would have made this test
// impossible to write, because nothing about it reaches the rules table. That
// is the gap f7b8d3a closed for rig's own methods, and stopping the daemon is
// the worst available place to reopen it.
func TestAHouseRuleCanRefuseRigDown(t *testing.T) {
	sock, d := upDaemon(t, nil)

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID:      "no-unattended-destruction",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	_, err := down(t, dial(t, sock))
	if err == nil {
		t.Fatal("rig.down was allowed under a rule denying destructive calls")
	}
	// The refusal names the pair, because whoever reads it is the person who
	// has to write the next rule.
	for _, want := range []string{"terminal", "destructive", "no-unattended-destruction"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %s", err, want)
		}
	}

	// And the refusal has to have actually refused. A rule that produces a
	// good error message while the daemon stops anyway is worse than no rule.
	if !keptServing(sock) {
		t.Fatal("rig.down was refused and the daemon stopped regardless")
	}
}

// TestARuleDenyingLessThanDestructiveDoesNotStopRigDown is the other side of
// the floor: `effects` on a rule is "at least as dangerous as", so a rule
// pitched at read-only catches down too, and one pitched ABOVE destructive
// must not.
func TestARuleDenyingOnlyDrivesInputDoesNotCatchRigDown(t *testing.T) {
	sock, d := upDaemon(t, nil)

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID:      "no-synthetic-input",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDrivesInput,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	if _, err := down(t, dial(t, sock)); err != nil {
		t.Fatalf("a rule above destructive refused rig.down: %v", err)
	}
}

// TestRigDeclaresDownAsDestructive checks the declaration itself, because the
// declaration is what a house rule matches on. Every property in it is load
// bearing in a way a summary string is not.
func TestRigDeclaresDownAsDestructive(t *testing.T) {
	var cmd *kernel.Command
	decl := selfDeclaration()
	for i := range decl.Commands {
		if decl.Commands[i].ID == "down" {
			cmd = &decl.Commands[i]
		}
	}
	if cmd == nil {
		t.Fatal("rig does not declare down, so no house rule can reach it")
	}
	if cmd.Effects != kernel.EffectsDestructive {
		t.Errorf("down declares effects %v, want destructive", cmd.Effects)
	}
	// Idempotent because the ESTATE ends up stopped either way, which is what
	// lets the CLI report an already-stopped daemon as success.
	if cmd.Idempotent != kernel.Yes {
		t.Errorf("down declares idempotent %v, want yes", cmd.Idempotent)
	}
	// Not Confirms: there is no surface to confirm through at M1 (Asker is
	// nil, so a confirm is denied as an unanswered gating question), and
	// declaring yes would describe a prompt that does not happen.
	if cmd.Confirms != kernel.No {
		t.Errorf("down declares confirms %v, want no", cmd.Confirms)
	}
}

// TestRigDownOnADaemonThatIsNotServingSaysSo covers the nil-stopper branch. A
// Daemon dispatched to directly, without Serve, has no listener to stop, and
// the honest answer is that rather than a panic.
func TestRigDownOnADaemonThatIsNotServingSaysSo(t *testing.T) {
	lock, err := instance.Acquire(filepath.Join(t.TempDir(), "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{Version: "test", Wire: "v1", Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	if d.stopper() != nil {
		t.Fatal("a daemon that was never served reports something to stop")
	}
}
