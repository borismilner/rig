package daemon

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// THE M0 GATE (PLAN.md 23), on the method the CLI now sends. `rig ping
// fakeapp` used to travel as "fakeapp.ping", which put a reserved command id
// inside the program's own namespace. It is rig's method with the program as
// an argument.
func TestTheM0GateRunsOnRigsOwnMethod(t *testing.T) {
	sock := up(t)
	program(t, sock, "fakeapp")

	nonce := []byte("abcdefgh")
	resp := &rigv1.PingResponse{}
	if err := dial(t, sock).Call(ctx5(t), "rig.ping",
		&rigv1.PingRequest{Nonce: nonce, Program: "fakeapp"}, resp); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if string(resp.GetNonce()) != string(nonce) {
		t.Fatalf("nonce came back as %q, want %q", resp.GetNonce(), nonce)
	}
	// Answered BY the program, not by rig. A probe that rig answered on the
	// program's behalf would prove nothing about the program being alive.
	if resp.GetProgram() != "fakeapp" {
		t.Fatalf("answered by %q, want fakeapp", resp.GetProgram())
	}
}

// The program must never see rig's own method name. route copies the method
// verbatim, so the delegating frame is rebuilt rather than forwarded; drop
// that and the program is handed "rig.ping" and answers a method it does not
// own.
func TestTheProbedProgramSeesItsOwnMethodName(t *testing.T) {
	sock := up(t)

	seen := make(chan string, 1)
	c := dial(t, sock)
	c.Handle(func(method string, payload []byte) (proto.Message, error) {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		select {
		case seen <- method:
		default:
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: "fakeapp", Version: "1.0"}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, testDeclaration("fakeapp")); err != nil {
		t.Fatal(err)
	}

	if err := dial(t, sock).Call(ctx5(t), "rig.ping",
		&rigv1.PingRequest{Nonce: []byte("n"), Program: "fakeapp"},
		&rigv1.PingResponse{}); err != nil {
		t.Fatalf("round trip: %v", err)
	}

	select {
	case method := <-seen:
		if method != "fakeapp.ping" {
			t.Fatalf("the program was handed %q, want fakeapp.ping", method)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the program was never called")
	}
}

// A probe of another program goes through route, so it meets the same
// authorization floor as any other call. Answering it inside serveSelf would
// have routed a call around the boundary section 13a exists to make
// unforgettable.
func TestAProbeOfAnotherProgramStillMeetsTheAuthorizationFloor(t *testing.T) {
	sock, d := upDaemon(t, nil)
	program(t, sock, "fakeapp")

	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "deny-everything", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsReadOnly, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	err := dial(t, sock).Call(ctx5(t), "rig.ping",
		&rigv1.PingRequest{Nonce: []byte("n"), Program: "fakeapp"},
		&rigv1.PingResponse{})
	if err == nil {
		t.Fatal("a probe of a program slipped past a rule denying read-only calls")
	}
}

// An empty program name, and "rig", both mean rig itself rather than a
// program called "". Under the shipped rules - the two url ones and nothing
// else - rig answers its own probe.
//
// The deny-rule half of this used to live here and has moved to
// TestRigsOwnMethodsMeetTheSameFloorAsEverythingElse: rig now declares its
// own commands, so a broad rule reaches them like anything else.
func TestAnEmptyProgramMeansRigItself(t *testing.T) {
	sock := up(t)
	for _, target := range []string{"", "rig"} {
		resp := &rigv1.PingResponse{}
		if err := dial(t, sock).Call(ctx5(t), "rig.ping",
			&rigv1.PingRequest{Nonce: []byte("z"), Program: target}, resp); err != nil {
			t.Fatalf("rig.ping with program %q: %v", target, err)
		}
		if resp.GetProgram() != "rig" {
			t.Fatalf("rig.ping with program %q was answered by %q", target, resp.GetProgram())
		}
	}
}

// An older rig sends "<program>.ping" and must keep working: the move is in
// what the caller sends, not in what the program answers.
func TestTheOlderProbeMethodStillRoutes(t *testing.T) {
	sock := up(t)
	program(t, sock, "fakeapp")

	resp := &rigv1.PingResponse{}
	if err := dial(t, sock).Call(ctx5(t), "fakeapp.ping",
		&rigv1.PingRequest{Nonce: []byte("old")}, resp); err != nil {
		t.Fatalf("the old method stopped routing: %v", err)
	}
	if resp.GetProgram() != "fakeapp" {
		t.Fatalf("answered by %q, want fakeapp", resp.GetProgram())
	}
}

// A probe naming a program that is not connected fails as NOT_FOUND rather
// than being quietly answered by rig, which would report a dead program as
// alive - the one answer a liveness probe must never give.
func TestAProbeOfAnAbsentProgramIsNotAnsweredByRig(t *testing.T) {
	resp := &rigv1.PingResponse{}
	err := dial(t, up(t)).Call(ctx5(t), "rig.ping",
		&rigv1.PingRequest{Nonce: []byte("n"), Program: "nosuch"}, resp)
	if err == nil {
		t.Fatalf("a probe of an absent program was answered by %q", resp.GetProgram())
	}
}
