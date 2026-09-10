package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/instance"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// Section 5f puts the flock before the bind. Prose keeps an ordering rule one
// refactor away from being false, so the constructor carries it: no lock, no
// daemon. This is the test that keeps the enforcement rather than the comment.
func TestNewRefusesWithoutTheSingleInstanceLock(t *testing.T) {
	if _, err := New(Config{Version: "t", Wire: "v1"}); err == nil {
		t.Fatal("a daemon was built with no single-instance lock")
	}
}

func TestNewRefusesWithoutAWireVersion(t *testing.T) {
	dir, err := os.MkdirTemp("", "rigt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if _, err := New(Config{Version: "t", Lock: lock}); err == nil {
		t.Fatal("a daemon was built with no wire version")
	}
}

// up starts a daemon on a socket in a temp dir and returns its path.
func up(t *testing.T) string {
	t.Helper()
	// Kept short deliberately: sun_path is 108 bytes and t.TempDir under a
	// long TMPDIR silently exceeds it, failing as EINVAL.
	dir, err := os.MkdirTemp("", "rigt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	// The lock is required to construct a Daemon at all, so every test takes
	// a real one on its own pidfile. That is the point of putting it in the
	// type: the ordering cannot be skipped, here or in main.
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{Version: "test", Wire: "v1", Lock: lock})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock
}

func dial(t *testing.T, sock string) *client.Client {
	t.Helper()
	c, err := client.Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// program connects, answers ping, and says hello under the given name.
func program(t *testing.T, sock, name string) *client.Client {
	t.Helper()
	c := dial(t, sock)
	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: name, Version: "1.0"}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, testDeclaration(name)); err != nil {
		t.Fatal(err)
	}
	return c
}

// testDeclaration is the smallest declaration rig accepts: identity,
// coverage, semantics_gen, and one command with every mandatory property
// said. Anything less is refused, which is what the tests below check.
func testDeclaration(id string) *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity:     &rigv1.Identity{Id: id, Name: id, Version: "1.0"},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		SemanticsGen: 1,
		Commands: []*rigv1.Command{{
			Id:           "ping",
			Title:        "Ping",
			Effects:      rigv1.Effects_EFFECTS_READ_ONLY,
			Idempotent:   rigv1.Tristate_TRISTATE_YES,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Confirms:     rigv1.Tristate_TRISTATE_NO,
			Shape:        rigv1.Shape_SHAPE_UNARY,
			Summary:      "Round-trip a nonce",
			Description:  "Echoes the nonce it was given.",
			Returns:      "The nonce, the program id and its version.",
		}},
	}
}

func ctx5(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

// THE M0 GATE (PLAN.md 23): rig ping fakeapp round-trips.
func TestPingRoundTripsThroughTheDaemon(t *testing.T) {
	sock := up(t)
	program(t, sock, "fakeapp")

	caller := dial(t, sock)
	nonce := []byte("abcdefgh")
	resp := &rigv1.PingResponse{}
	if err := caller.Call(ctx5(t), "fakeapp.ping",
		&rigv1.PingRequest{Nonce: nonce}, resp); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if string(resp.GetNonce()) != string(nonce) {
		t.Fatalf("nonce came back as %q, want %q", resp.GetNonce(), nonce)
	}
	if resp.GetProgram() != "fakeapp" {
		t.Fatalf("answered by %q, want fakeapp", resp.GetProgram())
	}
}

func TestPingTheDaemonItself(t *testing.T) {
	c := dial(t, up(t))
	resp := &rigv1.PingResponse{}
	if err := c.Call(ctx5(t), "rig.ping", &rigv1.PingRequest{Nonce: []byte("x")}, resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetProgram() != "rig" {
		t.Fatalf("rig.ping answered by %q", resp.GetProgram())
	}
}

// Section 14's predicate: a connection that completes the program handshake is
// scoped, and a plain client is not. This is the whole authorisation state a
// connection carries, so it is asserted from the first commit.
func TestHelloScopesTheConnection(t *testing.T) {
	sock := up(t)
	c := dial(t, sock)
	resp, err := c.Hello(ctx5(t), testDeclaration("someprog"))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetScoped() {
		t.Fatal("a program that said hello is not scoped")
	}
	if resp.GetWire() != "v1" {
		t.Fatalf("wire is %q, want v1", resp.GetWire())
	}
}

func TestUnknownProgramIsNotFound(t *testing.T) {
	c := dial(t, up(t))
	err := c.Call(ctx5(t), "nosuch.ping", &rigv1.PingRequest{}, &rigv1.PingResponse{})
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_NOT_FOUND {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestUnknownMethodOnRigIsNotFound(t *testing.T) {
	c := dial(t, up(t))
	err := c.Call(ctx5(t), "rig.nosuchthing", &rigv1.PingRequest{}, &rigv1.PingResponse{})
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_NOT_FOUND {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestMethodMustBeProgramDotCommand(t *testing.T) {
	c := dial(t, up(t))
	for _, m := range []string{"noseparator", ".leading", "trailing."} {
		err := c.Call(ctx5(t), m, &rigv1.PingRequest{}, &rigv1.PingResponse{})
		var ce *client.CallError
		if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INVALID {
			t.Fatalf("method %q: want INVALID, got %v", m, err)
		}
	}
}

// Two programs cannot hold one name, or a ping is routed to whichever
// connected last and the caller cannot tell.
func TestASecondProgramCannotTakeAConnectedName(t *testing.T) {
	sock := up(t)
	program(t, sock, "fakeapp")

	other := dial(t, sock)
	_, err := other.Hello(ctx5(t), testDeclaration("fakeapp"))
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_DENIED {
		t.Fatalf("want DENIED, got %v", err)
	}
}

// "rig" is the daemon's own namespace; a program taking it would shadow
// rig.ping and rig.hello.
func TestRigIsAReservedProgramName(t *testing.T) {
	c := dial(t, up(t))
	_, err := c.Hello(ctx5(t), testDeclaration("rig"))
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INVALID {
		t.Fatalf("want INVALID, got %v", err)
	}
}

func TestHelloRefusesAnEmptyProgramID(t *testing.T) {
	c := dial(t, up(t))
	_, err := c.Hello(ctx5(t), testDeclaration(""))
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INVALID {
		t.Fatalf("want INVALID, got %v", err)
	}
}

// A program that disconnects stops being routable, rather than leaving a name
// that answers with a timeout.
func TestADisconnectedProgramStopsBeingRoutable(t *testing.T) {
	sock := up(t)
	p := program(t, sock, "gone")
	_ = p.Close()

	caller := dial(t, sock)
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := caller.Call(ctx5(t), "gone.ping", &rigv1.PingRequest{}, &rigv1.PingResponse{})
		var ce *client.CallError
		if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("still routable after disconnect: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Section 18: "a hung program can never block a rig goroutine". The daemon must
// answer the caller itself and stay available to everyone else.
func TestAHungProgramDoesNotBlockTheDaemon(t *testing.T) {
	sock := up(t)
	hung := dial(t, sock)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	hung.Handle(func(_ string, _ []byte) (proto.Message, error) {
		<-release
		return &rigv1.PingResponse{}, nil
	})
	if _, err := hung.Hello(ctx5(t), testDeclaration("hung")); err != nil {
		t.Fatal(err)
	}

	// A short client deadline stands in for CallTimeout, which is 10s.
	caller := dial(t, sock)
	short, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := caller.Call(short, "hung.ping", &rigv1.PingRequest{}, &rigv1.PingResponse{}); err == nil {
		t.Fatal("a hung program answered")
	}

	// The daemon is still serving, which is the actual assertion.
	resp := &rigv1.PingResponse{}
	if err := caller.Call(ctx5(t), "rig.ping", &rigv1.PingRequest{Nonce: []byte("y")}, resp); err != nil {
		t.Fatalf("daemon stopped serving after a hang: %v", err)
	}
}

func TestConcurrentCallersAreNotCrossed(t *testing.T) {
	sock := up(t)
	program(t, sock, "fakeapp")
	caller := dial(t, sock)

	const n = 40
	errs := make(chan error, n)
	for i := range n {
		go func(i int) {
			nonce := []byte{byte(i), byte(i >> 8), 0xAB}
			resp := &rigv1.PingResponse{}
			if err := caller.Call(ctx5(t), "fakeapp.ping",
				&rigv1.PingRequest{Nonce: nonce}, resp); err != nil {
				errs <- err
				return
			}
			if string(resp.GetNonce()) != string(nonce) {
				errs <- errors.New("a reply reached the wrong caller")
				return
			}
			errs <- nil
		}(i)
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

// ---- Registration (PLAN.md section 5e) -------------------------------------

// programs asks the registry what this connection may see.
func programs(t *testing.T, c *client.Client) []string {
	t.Helper()
	resp := &rigv1.ProgramsResponse{}
	if err := c.Call(ctx5(t), "rig.programs", &rigv1.ProgramsRequest{}, resp); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range resp.GetPrograms() {
		out = append(out, p.GetIdentity().GetId())
	}
	return out
}

func wantInvalid(t *testing.T, err error, substr string) {
	t.Helper()
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INVALID {
		t.Fatalf("want INVALID, got %v", err)
	}
	if !strings.Contains(ce.Error(), substr) {
		t.Fatalf("the refusal does not mention %q: %v", substr, ce)
	}
}

// Registration is the handshake, not a later call: a connection that
// completed it is a program for its whole life (section 14).
func TestHelloWithoutADeclarationIsRefused(t *testing.T) {
	c := dial(t, up(t))
	err := c.Call(ctx5(t), "rig.hello",
		&rigv1.HelloRequest{Program: "bare", Version: "1.0"},
		&rigv1.HelloResponse{})
	wantInvalid(t, err, "no declaration")
}

// Section 5e: no property has a default that carries a safety meaning, so an
// absent effects is refused rather than read as read-only.
func TestHelloRefusesADeclarationMissingAMandatoryProperty(t *testing.T) {
	sock := up(t)
	for _, tc := range []struct {
		name   string
		breaks func(*rigv1.Declaration)
		says   string
	}{
		{"effects", func(d *rigv1.Declaration) {
			d.Commands[0].Effects = rigv1.Effects_EFFECTS_UNSPECIFIED
		}, "effects"},
		{"sensitive absent", func(d *rigv1.Declaration) {
			d.Commands[0].Sensitive = nil
		}, "sensitive"},
		{"duration", func(d *rigv1.Declaration) {
			d.Commands[0].Duration = rigv1.Duration_DURATION_UNSPECIFIED
		}, "duration"},
		{"coverage", func(d *rigv1.Declaration) {
			d.Coverage = rigv1.Coverage_COVERAGE_UNSPECIFIED
		}, "coverage"},
		{"semantics_gen", func(d *rigv1.Declaration) {
			d.SemanticsGen = 0
		}, "semantics_gen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDeclaration("broken")
			tc.breaks(d)
			_, err := dial(t, sock).Hello(ctx5(t), d)
			wantInvalid(t, err, tc.says)
		})
	}
}

// An empty sensitive list is a declaration; an absent one is not. The wrapper
// message on the wire exists only so those two are different, and a repeated
// field would have made them identical.
func TestAnEmptySensitiveListSurvivesTheWire(t *testing.T) {
	sock := up(t)
	p := program(t, sock, "sens")
	defer p.Close()

	resp := &rigv1.ProgramsResponse{}
	if err := dial(t, sock).Call(ctx5(t), "rig.programs",
		&rigv1.ProgramsRequest{}, resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.GetPrograms()) != 1 {
		t.Fatalf("want one program, got %d", len(resp.GetPrograms()))
	}
	s := resp.GetPrograms()[0].GetCommands()[0].GetSensitive()
	if s == nil {
		t.Fatal("the sensitive wrapper came back nil, so a declared-empty " +
			"list is indistinguishable from one never declared")
	}
	if len(s.GetPointers()) != 0 {
		t.Fatalf("want no pointers, got %v", s.GetPointers())
	}
}

// Two places to say the same thing is two places to disagree.
func TestHelloRefusesAProgramIDThatContradictsTheIdentity(t *testing.T) {
	sock := up(t)
	d := testDeclaration("declared")
	err := dial(t, sock).Call(ctx5(t), "rig.hello",
		&rigv1.HelloRequest{Program: "claimed", Version: "1.0", Declaration: d},
		&rigv1.HelloResponse{})
	wantInvalid(t, err, "does not match identity.id")
}

// Section 14, run 1 of the three-run battery: a program sees itself and
// nothing else, and the other program is registered at the same time so the
// test would fail if the filter were absent rather than merely untested.
func TestAProgramSeesOnlyItself(t *testing.T) {
	sock := up(t)
	alpha := program(t, sock, "alpha")
	defer alpha.Close()
	beta := program(t, sock, "beta")
	defer beta.Close()

	if got := programs(t, alpha); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("alpha sees %v, want [alpha] only", got)
	}
	if got := programs(t, beta); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("beta sees %v, want [beta] only", got)
	}
}

// Section 14, run 2: a scoped answer where a complete one was owed fails as
// hard as a leak. Every other local connection from this uid is a client of
// the owner's and reads everything.
func TestAClientOfTheOwnersSeesEveryProgram(t *testing.T) {
	sock := up(t)
	alpha := program(t, sock, "alpha")
	defer alpha.Close()
	beta := program(t, sock, "beta")
	defer beta.Close()

	got := programs(t, dial(t, sock))
	if len(got) != 2 {
		t.Fatalf("a client of the owner's sees %v, want both programs - a "+
			"partial answer where a complete one was owed fails as hard as a "+
			"leak (section 14)", got)
	}
}

// The registry forgets by session, so a restarted program takes its id back
// rather than colliding with the corpse of the last one.
func TestDisconnectingRemovesTheDeclaration(t *testing.T) {
	sock := up(t)
	p := program(t, sock, "transient")
	if got := programs(t, dial(t, sock)); len(got) != 1 {
		t.Fatalf("want the program registered, got %v", got)
	}
	_ = p.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := programs(t, dial(t, sock)); len(got) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the declaration outlived the connection that made it")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// And the id is free again.
	again := program(t, sock, "transient")
	defer again.Close()
	if got := programs(t, dial(t, sock)); len(got) != 1 {
		t.Fatalf("a restarted program could not take its id back: %v", got)
	}
}

// An unregistered client is not a program, so it cannot register one either
// way round: the kernel refuses a non-program principal outright.
func TestTheDeclarationSurvivesTheRoundTripWhole(t *testing.T) {
	sock := up(t)
	p := program(t, sock, "whole")
	defer p.Close()

	resp := &rigv1.ProgramsResponse{}
	if err := dial(t, sock).Call(ctx5(t), "rig.programs",
		&rigv1.ProgramsRequest{}, resp); err != nil {
		t.Fatal(err)
	}
	got := resp.GetPrograms()[0]
	if got.GetCoverage() != rigv1.Coverage_COVERAGE_PARTIAL {
		t.Fatalf("coverage came back %v", got.GetCoverage())
	}
	if got.GetSemanticsGen() != 1 {
		t.Fatalf("semantics_gen came back %d", got.GetSemanticsGen())
	}
	c := got.GetCommands()[0]
	for _, check := range []struct {
		name string
		ok   bool
	}{
		{"effects", c.GetEffects() == rigv1.Effects_EFFECTS_READ_ONLY},
		{"idempotent", c.GetIdempotent() == rigv1.Tristate_TRISTATE_YES},
		{"interactive", c.GetInteractive() == rigv1.Tristate_TRISTATE_NO},
		{"duration", c.GetDuration() == rigv1.Duration_DURATION_INSTANT},
		{"shape", c.GetShape() == rigv1.Shape_SHAPE_UNARY},
		{"summary", c.GetSummary() == "Round-trip a nonce"},
		{"returns", c.GetReturns() != ""},
	} {
		if !check.ok {
			t.Errorf("%s did not survive the round trip", check.name)
		}
	}
}
