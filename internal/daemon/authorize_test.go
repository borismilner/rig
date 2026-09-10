package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// destructiveDeclaration is the test declaration plus one destructive
// command, so a test can ask for the dangerous one without changing what
// every other test registers.
func destructiveDeclaration(id string) *rigv1.Declaration {
	d := testDeclaration(id)
	destroy := proto.Clone(d.GetCommands()[0]).(*rigv1.Command)
	destroy.Id = "destroy"
	destroy.Title = "Destroy"
	destroy.Effects = rigv1.Effects_EFFECTS_DESTRUCTIVE
	destroy.Summary = "Delete the index"
	destroy.Description = "Deletes the index and everything derived from it."
	destroy.Returns = "Nothing."
	// One destructive command that also takes declared arguments, so a test
	// can prove the question carries them (section 14).
	reindex := proto.Clone(destroy).(*rigv1.Command)
	reindex.Id = "reindex"
	reindex.Title = "Reindex"
	reindex.Args = []byte(`{"type":"object","additionalProperties":false,` +
		`"required":["since"],"properties":{"since":{"type":"string"}}}`)
	reindex.Summary = "Rebuild the index"
	reindex.Description = "Deletes the index and rebuilds it from the tree."
	reindex.Returns = "The number of items indexed."

	d.Commands = append(d.Commands, destroy, reindex)
	return d
}

// dangerous connects the shelf program, which answers anything it is asked
// and declares one read-only command and one destructive one.
//
// It answers a CallResponse, which is what every declared command carries -
// the typed Ping pair is only rig's own probe.
func dangerous(t *testing.T, sock string) {
	t.Helper()
	const name = "shelf"
	c := dial(t, sock)
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The connection stays up for the test: dial registers its own cleanup,
	// so the caller has nothing to hold.
	if _, err := c.Hello(ctx, destructiveDeclaration(name)); err != nil {
		t.Fatal(err)
	}
}

// call invokes one declared command with no arguments and returns the error
// the caller sees.
func call(t *testing.T, c *client.Client, method string) error {
	t.Helper()
	var resp rigv1.CallResponse
	return c.Call(ctx5(t), method, &rigv1.CallRequest{}, &resp)
}

// TestSliceFourDemo is M1 slice 4's demo, over the real wire: one house rule
// is written, and a destructive command is then refused with the refusal
// naming the pair it matched.
//
// The pair is (terminal, destructive): a fresh connection is a terminal
// (section 14), and the effects come from what the program declared.
func TestSliceFourDemo(t *testing.T) {
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID:      "no-unattended-destruction",
		Caller:  kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive,
		Action:  kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	caller := dial(t, sock)

	// The read-only command is untouched: the rule's floor is destructive.
	if err := call(t, caller, "shelf.ping"); err != nil {
		t.Fatalf("a read-only command was refused: %v", err)
	}

	err := call(t, caller, "shelf.destroy")
	if err == nil {
		t.Fatal("a destructive command under a deny rule was allowed through")
	}
	// The refusal has to name the pair, because whoever reads it is the
	// person who has to write the next rule.
	for _, want := range []string{"terminal", "destructive", "no-unattended-destruction"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal %q does not name %s", err, want)
		}
	}
}

func TestARefusalIsDeniedNotNotFound(t *testing.T) {
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "deny", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	err := call(t, dial(t, sock), "shelf.destroy")
	if err == nil {
		t.Fatal("allowed")
	}
	// A refusal that arrives as NOT_FOUND tells the caller the command does
	// not exist, which is a different bug to go and look for.
	if !strings.Contains(err.Error(), "DENIED") && !strings.Contains(err.Error(), "denied") {
		t.Fatalf("a house-rules refusal arrived as %v", err)
	}
}

func TestNothingAlreadyWorkingBreaksWithTheShippedRules(t *testing.T) {
	// The compatibility promise, over the wire: no rule is written, so every
	// caller is unmatched, so even the destructive command goes through.
	sock, _ := upDaemon(t, nil)
	dangerous(t, sock)
	caller := dial(t, sock)

	for _, method := range []string{"shelf.ping", "shelf.destroy"} {
		if err := call(t, caller, method); err != nil {
			t.Fatalf("%s was refused under the shipped defaults: %v", method, err)
		}
	}
}

// answerer is a surface that answers every gating question the same way.
type answerer struct {
	yes   bool
	err   error
	asked []Question
}

func (a *answerer) Ask(_ context.Context, q Question) (bool, error) {
	a.asked = append(a.asked, q)
	return a.yes, a.err
}

func TestAConfirmWithNoSurfaceToAskOnIsDenied(t *testing.T) {
	// Section 13a: a gating ask fails CLOSED. At M1 there is no surface, so
	// this is the path every confirm takes, and it must not be the path that
	// lets a call through.
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "confirm-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionConfirm,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	err := call(t, dial(t, sock), "shelf.destroy")
	if err == nil {
		t.Fatal("a confirm nobody could answer let the call through")
	}
	if !strings.Contains(err.Error(), "no surface could ask") {
		t.Fatalf("the refusal %q does not say why nobody was asked", err)
	}
	if !strings.Contains(err.Error(), "(terminal, destructive)") {
		t.Fatalf("the refusal %q does not name the pair", err)
	}
}

func TestAnAnsweredConfirmAuthorisesThatOneCall(t *testing.T) {
	yes := &answerer{yes: true}
	sock, d := upDaemon(t, yes)
	dangerous(t, sock)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "confirm-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionConfirm,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	caller := dial(t, sock)
	var resp rigv1.CallResponse
	for i := range 2 {
		if err := caller.Call(ctx5(t), "shelf.reindex",
			&rigv1.CallRequest{Args: []byte(`{"since":"7d"}`)}, &resp); err != nil {
			t.Fatalf("call %d was refused after a yes: %v", i, err)
		}
	}
	// Nothing is stored on the connection, so the second call asks again.
	// The answer authorises that one call, not the connection.
	if len(yes.asked) != 2 {
		t.Fatalf("two calls produced %d questions", len(yes.asked))
	}

	// Section 14: the question names the principal, the pid, the command and
	// the arguments as they will be invoked.
	q := yes.asked[0]
	if q.Method != "shelf.reindex" {
		t.Fatalf("the question named method %q", q.Method)
	}
	// Section 14: the arguments AS THEY WILL BE INVOKED. A prompt that shows
	// the command without them is answered by someone who cannot see what
	// they are authorising.
	if !strings.Contains(string(q.Args), "7d") {
		t.Fatalf("the question carried arguments %q", q.Args)
	}
	if q.Principal.Kind != kernel.KindTerminal || q.Principal.PID == 0 {
		t.Fatalf("the question does not name its caller: %s", q.Principal)
	}
	if q.Decision.Pair.String() != "(terminal, destructive)" {
		t.Fatalf("the question carries pair %s", q.Decision.Pair)
	}
}

func TestARefusedConfirmIsARefusal(t *testing.T) {
	no := &answerer{yes: false}
	sock, d := upDaemon(t, no)
	dangerous(t, sock)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "confirm-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionConfirm,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	err := call(t, dial(t, sock), "shelf.destroy")
	if err == nil {
		t.Fatal("a no let the call through")
	}
	if !strings.Contains(err.Error(), "the answer was no") {
		t.Fatalf("the refusal %q does not say the question was answered no", err)
	}
}

func TestASurfaceThatCannotBeReachedDeniesRatherThanAllows(t *testing.T) {
	broken := &answerer{yes: true, err: context.DeadlineExceeded}
	sock, d := upDaemon(t, broken)
	dangerous(t, sock)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "confirm-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionConfirm,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// The answerer says yes AND fails. A boundary that read the bool first
	// would allow the call on a question that was never put.
	err := call(t, dial(t, sock), "shelf.destroy")
	if err == nil {
		t.Fatal("a question that could not be put let the call through")
	}
	if !strings.Contains(err.Error(), "could not be put") {
		t.Fatalf("the refusal %q does not say the question failed", err)
	}
}

// GAP 2 IS CLOSED, and this test is the deliberate change its predecessor
// asked for.
//
// It used to assert that rig.ping still answered under a rule denying every
// read-only call, because rig's own methods had no registry entry and were
// not matched at all. That was the defect, not the behaviour: serveSelf was
// the one surface that never reached the rules table, which is precisely what
// section 13a's "no surface can forget it" forbids.
//
// rig now declares its own commands, so a rule denying every read-only call
// denies rig's read-only commands too. That is the owner's rule doing what it
// says. A refusal is still distinguishable from a dead daemon: it arrives as
// CODE_DENIED over a connection that dialled, where a dead daemon cannot be
// dialled at all - which is the objection this decision turned on.
func TestRigsOwnMethodsMeetTheSameFloorAsEverythingElse(t *testing.T) {
	sock, d := upDaemon(t, nil)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "deny-everything", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsReadOnly, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	for _, m := range []string{"rig.ping", "rig.programs"} {
		err := dial(t, sock).Call(ctx5(t), m, &rigv1.PingRequest{Nonce: []byte("7")},
			&rigv1.PingResponse{})
		if err == nil {
			t.Fatalf("%s answered under a rule denying every read-only call", m)
		}
		if !strings.Contains(err.Error(), "CODE_DENIED") {
			t.Fatalf("%s failed, but not as a refusal: %v", m, err)
		}
	}
}

// The one method that must NOT meet the floor, and it is not an exemption.
//
// hello is the handshake that MINTS the principal, so there is no
// (caller, effects) pair to match before it - `caller` is what hello
// establishes. Authorise it and a broad rule stops any program registering at
// all, which makes the daemon unusable rather than restricted.
func TestHelloIsPriorToTheFloorAndNotSubjectToIt(t *testing.T) {
	sock, d := upDaemon(t, nil)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "deny-everything", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsReadOnly, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	// program() completes the handshake and fails the test if it cannot.
	program(t, sock, "fakeapp")
}

func TestOneProgramCallingAnothersReadOnlyCommandIsNotDestructive(t *testing.T) {
	// The bug the adversarial pass found, over the wire. A program is scoped
	// to itself (section 14), so resolving effects through the caller's view
	// made this call resolve as (program, destructive) and a rule denying
	// destructive calls refused a read-only one.
	//
	// Routing does not check visibility, so this call is reachable whatever
	// the invoker decides - which is why the invoker matching on the target's
	// own declaration is the honest answer rather than the lax one.
	sock, d := upDaemon(t, nil)
	dangerous(t, sock) // shelf, with one read-only and one destructive command
	other := program(t, sock, "grabbit")

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "deny-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	var resp rigv1.CallResponse
	if err := other.Call(ctx5(t), "shelf.ping",
		&rigv1.CallRequest{}, &resp); err != nil {
		t.Fatalf("a read-only cross-program call was refused: %v", err)
	}

	// The destructive one is still refused, and still names the pair.
	err := other.Call(ctx5(t), "shelf.destroy", &rigv1.CallRequest{}, &resp)
	if err == nil {
		t.Fatal("the destructive command was allowed")
	}
	if !strings.Contains(err.Error(), "(program, destructive)") {
		t.Fatalf("the refusal %q does not name the pair", err)
	}
}

// The test that proves rig's declaration is actually READ, rather than its
// methods merely reaching the table.
//
// A rule denying only the top of the danger order must NOT catch rig's own
// read-only commands. Stop consulting rig's declaration and they resolve as
// unresolvable, which is EffectsCeiling, which this rule denies - so rig stops
// answering under a rule about driving the desktop. Every other test here
// passes either way, because a rule with a read-only floor catches both an
// honest read-only and a pessimistic ceiling.
func TestRigsOwnCommandsResolveToWhatTheyDeclare(t *testing.T) {
	sock, d := upDaemon(t, nil)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "no-driving-my-desktop", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsCeiling, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	resp := &rigv1.PingResponse{}
	if err := dial(t, sock).Call(ctx5(t), "rig.ping",
		&rigv1.PingRequest{Nonce: []byte("q")}, resp); err != nil {
		t.Fatalf("rig.ping is read-only and was denied by a rule about the "+
			"top of the danger order, so its declared effects were not read: %v", err)
	}
	if resp.GetProgram() != "rig" {
		t.Fatalf("answered by %q", resp.GetProgram())
	}
}
