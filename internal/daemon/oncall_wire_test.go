package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/supervise"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 54 over the wire. The starter "launches" the stub by dialling the
// socket as it, from this process, so the hello comes from the pid rig
// started: the same trick as upSupervised.

// stubProc is one launch of the stub: a connection that said hello, ended
// by rig's stop or by the test as an idle exit.
type stubProc struct {
	once sync.Once
	done chan supervise.Exit
	conn atomic.Pointer[client.Client]
}

func (p *stubProc) PID() int { return os.Getpid() }

func (p *stubProc) Stop(time.Duration) { p.exit(supervise.Exit{Signal: "SIGTERM"}) }

func (p *stubProc) exit(e supervise.Exit) {
	p.once.Do(func() {
		if c := p.conn.Load(); c != nil {
			_ = c.Close()
		}
		e.At = time.Now()
		p.done <- e
		close(p.done)
	})
}

type onCallRig struct {
	sock string
	bin  string
	d    *Daemon

	starts  atomic.Int32
	fail    atomic.Bool  // the next launch exits 1 before its hello
	load    atomic.Int32 // the rigv1.Load the stub declares
	mu      sync.Mutex
	running *stubProc

	stop    func()
	stopped sync.Once
}

// stopOnce ends the daemon, once, so a test may end it early.
func (r *onCallRig) stopOnce() {
	if r.stop != nil {
		r.stopped.Do(r.stop)
	}
}

// start "launches" the stub: it dials the socket as the program and says
// hello, from this process, so the pid matches the child rig started.
func (r *onCallRig) start(spec supervise.Spec, _ map[string]string) (supervise.Process, <-chan supervise.Exit, error) {
	r.starts.Add(1)
	p := &stubProc{done: make(chan supervise.Exit, 1)}
	if r.fail.Load() {
		go p.exit(supervise.Exit{Code: 1})
		return p, p.done, nil
	}
	r.mu.Lock()
	r.running = p
	r.mu.Unlock()
	go func() {
		c, err := client.Dial(r.sock)
		if err != nil {
			p.exit(supervise.Exit{Code: 2})
			return
		}
		p.conn.Store(c)
		c.Handle(func(_ string, payload []byte) (proto.Message, error) {
			var req rigv1.PingRequest
			if err := proto.Unmarshal(payload, &req); err != nil {
				return nil, err
			}
			return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: spec.ID, Version: "1.0"}, nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		decl := testDeclaration(spec.ID)
		decl.Load = rigv1.Load(r.load.Load())
		if _, err := c.Hello(ctx, decl); err != nil {
			p.exit(supervise.Exit{Code: 3})
		}
	}()
	return p, p.done, nil
}

// upOnCall is a daemon supervising one stub declared on call, or resident
// when onCall is false, which is the red control for "nothing runs at rest".
func upOnCall(t *testing.T, onCall bool) *onCallRig {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigoc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	r := &onCallRig{sock: filepath.Join(dir, "s"), bin: filepath.Join(dir, "stub")}
	if err := os.WriteFile(r.bin, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	sup := supervise.New(supervise.Options{Start: r.start})
	spec := supervise.Spec{ID: "stub", Path: r.bin, OnCall: onCall, Autostart: !onCall}
	if err := sup.Declare([]supervise.Spec{spec}); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", r.sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	r.d, err = New(Config{
		Version: "test", Wire: "v1", Lock: lock, Supervisor: sup,
		Declarations: filepath.Join(dir, "declarations"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !onCall {
		if _, err := sup.Up("stub"); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = r.d.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		<-done
		sup.StopAll()
		if r.d.records != nil {
			_ = r.d.records.Close()
		}
	})
	return r
}

// settle waits until the stub's health state is want.
func (r *onCallRig) settle(t *testing.T, want verbsv1.ProgramState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		// Not healthOf: until a background scan has declared the stub, the
		// answer is NOT_FOUND, and that is "not yet" here.
		var resp verbsv1.HealthResponse
		err := dial(t, r.sock).Call(ctx5(t), "rig.health", &verbsv1.HealthRequest{Programs: []string{"stub"}}, &resp)
		var got verbsv1.ProgramState
		if err == nil && len(resp.GetPrograms()) == 1 {
			got = resp.GetPrograms()[0].GetState()
			if got == want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("stub is %s (%v), want %s", got, err, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *onCallRig) listed(t *testing.T) *registryv1.Program {
	t.Helper()
	var resp registryv1.ProgramsResponse
	err := dial(t, r.sock).Call(ctx5(t), "rig.programs",
		&registryv1.ProgramsRequest{Depth: registryv1.Depth_DEPTH_COMMANDS}, &resp)
	if err != nil {
		t.Fatalf("rig.programs: %v", err)
	}
	for _, p := range resp.GetPrograms() {
		if p.GetIdentity().GetId() == "stub" {
			return p
		}
	}
	return nil
}

// At rest nothing runs, and the stub is still listed with its commands.
// The red control is the same stub declared resident, which is running.
func TestAnOnCallProgramAtRestIsListedAndNotRunning(t *testing.T) {
	r := upOnCall(t, true)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	if n := r.starts.Load(); n != 1 {
		t.Fatalf("%d launches before any call, want the one declare run", n)
	}
	p := r.listed(t)
	if p == nil || !p.GetAtRest() || len(p.GetCommands()) == 0 {
		t.Fatalf("at rest the stub is listed as %v, want at_rest with its commands", p)
	}

	resident := upOnCall(t, false)
	resident.settle(t, verbsv1.ProgramState_PROGRAM_STATE_HEALTHY)
	if p := resident.listed(t); p == nil || p.GetAtRest() {
		t.Fatalf("a resident stub is listed as %v, want running", p)
	}
}

// A call starts it and is answered, and an idle exit puts it back at rest
// rather than counting as a crash.
func TestACallStartsAnOnCallProgramAndIsAnswered(t *testing.T) {
	r := upOnCall(t, true)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)

	var pong rigv1.PingResponse
	if err := dial(t, r.sock).Call(ctx5(t), "stub.ping", &rigv1.PingRequest{Nonce: []byte("n")}, &pong); err != nil {
		t.Fatalf("a call to a program at rest: %v", err)
	}
	if string(pong.GetNonce()) != "n" || r.starts.Load() != 2 {
		t.Fatalf("answered %q after %d launches, want n after 2", pong.GetNonce(), r.starts.Load())
	}

	r.mu.Lock()
	run := r.running
	r.mu.Unlock()
	run.exit(supervise.Exit{Code: 0})
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	if h := healthOf(ctx5(t), t, r.sock, "stub"); h.GetRestarts() != 0 {
		t.Fatalf("an idle exit counted %d restarts, want none", h.GetRestarts())
	}
}

// A command the stub never declared is refused from the kept declaration,
// and does not start it. INVALID, the answer a running program's caller gets
// too: plan/54 said NOT_FOUND, and the as-built note corrects it.
func TestAnUndeclaredCommandDoesNotStartIt(t *testing.T) {
	r := upOnCall(t, true)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	before := r.starts.Load()
	wantCode(t, dial(t, r.sock).Call(ctx5(t), "stub.nothing", &rigv1.PingRequest{}, &rigv1.PingResponse{}),
		rigv1.Code_CODE_INVALID, "an undeclared command")
	if n := r.starts.Load(); n != before {
		t.Fatalf("an undeclared command launched the stub (%d launches, was %d)", n, before)
	}
}

// A start that fails answers UNAVAILABLE naming the exit, not "not
// connected".
func TestAFailedStartNamesItsExit(t *testing.T) {
	r := upOnCall(t, true)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	r.fail.Store(true)
	err := dial(t, r.sock).Call(ctx5(t), "stub.ping", &rigv1.PingRequest{}, &rigv1.PingResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "a call whose start failed")
	if !strings.Contains(err.Error(), "exit 1") {
		t.Fatalf("the failure does not name the exit: %v", err)
	}
}

// A rebuilt binary is read again at the next listing. The red control is
// the unchanged binary, listed again, which is not.
func TestAChangedBinaryIsReadAgain(t *testing.T) {
	r := upOnCall(t, true)
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
	r.listed(t)
	if n := r.starts.Load(); n != 1 {
		t.Fatalf("listing an unchanged binary launched it: %d launches", n)
	}

	if err := os.WriteFile(r.bin, []byte("v2, a different size"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.listed(t)
	deadline := time.Now().Add(3 * time.Second)
	for r.starts.Load() != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("a rebuilt binary was not read again: %d launches", r.starts.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.settle(t, verbsv1.ProgramState_PROGRAM_STATE_AT_REST)
}
