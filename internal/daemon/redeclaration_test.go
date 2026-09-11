package daemon

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// recorder collects the daemon's log so a test can assert on what was NOT
// written. slog.Handler rather than a buffer: the assertion is about levels
// and attributes, and parsing a rendered line back into those is a second
// thing to get wrong.
type recorder struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *recorder) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *recorder) WithGroup(string) slog.Handler            { return r }

func (r *recorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recs = append(r.recs, rec.Clone())
	return nil
}

// since returns everything logged at or above lvl after mark, and the new
// mark. Taking a mark is what lets one test assert about two separate calls.
func (r *recorder) since(mark int, lvl slog.Level) ([]slog.Record, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []slog.Record
	for _, rec := range r.recs[mark:] {
		if rec.Level >= lvl {
			out = append(out, rec)
		}
	}
	return out, len(r.recs)
}

func (r *recorder) mark() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.recs)
}

// declaringDestroy is one program with one command at the given effects, so a
// test can re-declare the same command at a different level and change
// nothing else.
func declaringDestroy(id string, effects rigv1.Effects) *rigv1.Declaration {
	d := testDeclaration(id)
	destroy := proto.Clone(d.GetCommands()[0]).(*rigv1.Command)
	destroy.Id = "destroy"
	destroy.Title = "Destroy"
	destroy.Effects = effects
	destroy.Summary = "Delete the index"
	destroy.Description = "Deletes the index and everything derived from it."
	destroy.Returns = "Nothing."
	d.Commands = append(d.Commands, destroy)
	return d
}

// TestAReconnectingProgramCanWeakenItsOwnEffects is the daemon-level half of
// the hole internal/kernel/redeclaration_test.go locks. It LOCKS the
// behaviour; it does not close it.
//
// THE PATH IS A RECONNECT, AND THAT IS NARROWER THAN IT FIRST LOOKS. The
// kernel refuses only a DIFFERENT session, so the obvious attack is a second
// hello on a live connection - and the daemon refuses that outright: "one
// handshake per connection", because a second hello used to leave a name
// pointing at a dead connection. So kernel.Register's same-session branch is
// NOT REACHABLE FROM THE DAEMON, and the wire-level hole is the other one:
// the program disconnects, the connection's close deregisters it, and it
// reconnects declaring the same command one level weaker. Nothing compares
// the old declaration to the new, because by then the old one is gone.
//
// That is an ordinary program restart, which is what makes it worth locking.
// It needs no hostile program and no race - a deploy is enough.
//
// THE PART THAT MAKES IT A HOLE RATHER THAN A BUG IS THE SILENCE. A rule
// gating the destructive command stops matching, and nothing is recorded: no
// rule was violated, so there is no denial to log. rig logs decisions, not
// changes of declaration. The assertion on the log is the point of this test
// rather than decoration on it.
//
// The rule is written for any caller deliberately: what moves is the EFFECTS
// half of the pair, and naming a caller kind would leave a reader wondering
// which half changed.
//
// Closing it is a specification question - whether a re-declaration may
// weaken effects at all, and what rig does with the difference if not. A
// refusal written here without that ruling would be inventing the rule.
func TestAReconnectingProgramCanWeakenItsOwnEffects(t *testing.T) {
	rec := &recorder{}
	sock, d := upDaemonLogged(t, nil, slog.New(rec))

	connect := func(effects rigv1.Effects) *client.Client {
		t.Helper()
		c, err := client.Dial(sock)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		c.Handle(func(method string, _ []byte) (proto.Message, error) {
			return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := c.Hello(ctx, declaringDestroy("shelf", effects)); err != nil {
			t.Fatalf("declare at %v: %v", effects, err)
		}
		return c
	}

	first := connect(rigv1.Effects_EFFECTS_DESTRUCTIVE)

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID:      "nothing-destructive-unattended",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	caller := dial(t, sock)

	// Before: the rule matches, the call is refused, and the refusal is
	// recorded at warning - which is what makes the silence afterwards
	// measurable rather than assumed.
	at := rec.mark()
	if err := call(t, caller, "shelf.destroy"); err == nil {
		t.Fatal("a destructive command was allowed under a rule denying destruction")
	}
	warned, at := rec.since(at, slog.LevelWarn)
	if len(warned) == 0 {
		t.Fatal("the refusal was not recorded, so this test cannot measure its absence")
	}
	if !citesRule(warned, "nothing-destructive-unattended") {
		t.Fatalf("the refusal does not cite the rule that fired: %v", messages(warned))
	}

	// A second hello on the LIVE connection is refused, and that is asserted
	// rather than assumed: it is the reason this test takes the long way
	// round, and if it ever stops being true the short path is back.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := first.Hello(ctx,
		declaringDestroy("shelf", rigv1.Effects_EFFECTS_WRITES_FILES)); err == nil {
		t.Fatal("a second hello on a live connection was accepted")
	}

	// The restart. Close, wait for the daemon to forget it, reconnect one
	// level weaker.
	_ = first.Close()
	waitGone(t, d, "shelf")
	connect(rigv1.Effects_EFFECTS_WRITES_FILES)

	// After: the rule no longer matches. The call goes through.
	if err := call(t, caller, "shelf.destroy"); err != nil {
		t.Fatalf("the hole this test locks appears to be closed - a weakened "+
			"re-declaration was still refused (%v). Rewrite this test against "+
			"whatever closed it rather than deleting it", err)
	}

	// And NOTHING was written about the weakening. The log carries the
	// restart - a deregistration and a fresh "program registered" - and
	// neither says that a rule which had been gating this command no longer
	// reaches it. An operator watching for refusals sees a call that was
	// simply allowed.
	after, _ := rec.since(at, slog.LevelWarn)
	if len(after) != 0 {
		t.Errorf("something was recorded at warning after the weakening, so "+
			"the silence this test locks is no longer total: %v", messages(after))
	}
}

// waitGone blocks until the daemon has processed a program's disconnection.
// The close is asynchronous, so a reconnect racing it would be refused as
// "already registered by" and the test would fail for the wrong reason.
func waitGone(t *testing.T, d *Daemon, program string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := d.kernel.See(owner()).Program(program); !ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s was still registered five seconds after its connection closed", program)
}

// owner is a principal that sees the whole registry, for a test that needs to
// ask whether something is registered at all.
func owner() kernel.Principal {
	return kernel.Principal{
		UID: 1000, Kind: kernel.KindTerminal,
		ClientID: "t", SessionID: "s-t", PID: 1, Introspect: true,
	}
}

// citesRule reports whether any record carries rule=id.
func citesRule(recs []slog.Record, id string) bool {
	for _, rec := range recs {
		found := false
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == "rule" && strings.Contains(a.Value.String(), id) {
				found = true
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func messages(recs []slog.Record) []string {
	out := make([]string, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.Message)
	}
	return out
}
