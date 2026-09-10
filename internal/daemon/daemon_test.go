package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/client"
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
	if _, err := c.Hello(ctx, name, "1.0"); err != nil {
		t.Fatal(err)
	}
	return c
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
	resp, err := c.Hello(ctx5(t), "someprog", "0.1")
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
	_, err := other.Hello(ctx5(t), "fakeapp", "1.0")
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_DENIED {
		t.Fatalf("want DENIED, got %v", err)
	}
}

// "rig" is the daemon's own namespace; a program taking it would shadow
// rig.ping and rig.hello.
func TestRigIsAReservedProgramName(t *testing.T) {
	c := dial(t, up(t))
	_, err := c.Hello(ctx5(t), "rig", "1.0")
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INVALID {
		t.Fatalf("want INVALID, got %v", err)
	}
}

func TestHelloRefusesAnEmptyProgramID(t *testing.T) {
	c := dial(t, up(t))
	_, err := c.Hello(ctx5(t), "", "1.0")
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
	if _, err := hung.Hello(ctx5(t), "hung", "1.0"); err != nil {
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
