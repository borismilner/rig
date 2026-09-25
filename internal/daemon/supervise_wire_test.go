package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/supervise"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// pidProc stands in for a child with a chosen pid. The daemon matches a
// connection to the child by the socket's peer pid, so a child whose pid is
// the test's own is one the test can speak for, and any other pid is one it
// cannot.
type pidProc struct {
	pid  int
	once sync.Once
	done chan supervise.Exit
}

func (p *pidProc) PID() int { return p.pid }

func (p *pidProc) Stop(time.Duration) {
	p.once.Do(func() { p.done <- supervise.Exit{Signal: "SIGTERM", At: time.Now()}; close(p.done) })
}

func upSupervised(t *testing.T, pids map[string]int) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigsv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sup := supervise.New(supervise.Options{
		Start: func(spec supervise.Spec, _ map[string]string) (supervise.Process, <-chan supervise.Exit, error) {
			p := &pidProc{pid: pids[spec.ID], done: make(chan supervise.Exit, 1)}
			return p, p.done, nil
		},
	})
	var specs []supervise.Spec
	for id := range pids {
		specs = append(specs, supervise.Spec{ID: id, Path: "/bin/true"})
	}
	if err := sup.Declare(specs); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	d, err := New(Config{Version: "test", Wire: "v1", Lock: lock, Supervisor: sup})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		<-done
		sup.StopAll()
		if d.records != nil {
			_ = d.records.Close()
		}
	})
	return sock
}

func healthOf(ctx context.Context, t *testing.T, sock, id string) *verbsv1.ProgramHealth {
	t.Helper()
	var resp verbsv1.HealthResponse
	if err := dial(t, sock).Call(ctx, "rig.health", &verbsv1.HealthRequest{Programs: []string{id}}, &resp); err != nil {
		t.Fatalf("rig.health %s: %v", id, err)
	}
	return resp.GetPrograms()[0]
}

// Section 18 over the wire: rig up launches a declared program, its hello
// completes STARTING only when it comes from the process rig started, a
// health report is taken from that process alone, and stop takes it off the
// table with its history kept.
func TestADeclaredProgramIsSupervisedOverTheWire(t *testing.T) {
	ctx := ctx5(t)
	sock := upSupervised(t, map[string]int{"worker": os.Getpid(), "other": 1})
	cli := dial(t, sock)

	wantCode(t, cli.Call(ctx, "rig.up", &verbsv1.UpRequest{Programs: []string{"nobody"}}, &verbsv1.UpResponse{}),
		rigv1.Code_CODE_NOT_FOUND, "rig up of an undeclared program")

	var up verbsv1.UpResponse
	if err := cli.Call(ctx, "rig.up", &verbsv1.UpRequest{}, &up); err != nil {
		t.Fatalf("rig.up: %v", err)
	}
	if len(up.GetPrograms()) != 2 {
		t.Fatalf("rig up with no ids started %d programs, want every declared one", len(up.GetPrograms()))
	}
	if h := healthOf(ctx, t, sock, "worker"); h.GetState() != verbsv1.ProgramState_PROGRAM_STATE_STARTING {
		t.Fatalf("a launched program is %s before its hello", h.GetState())
	}

	// An unregistered connection has no program to report for.
	wantCode(t, cli.Call(ctx, "rig.health.report", &rigv1.HealthReportRequest{Marker: 1}, &rigv1.HealthReportResponse{}),
		rigv1.Code_CODE_DENIED, "a health report from a terminal")

	// "other" was launched as pid 1, so this process saying hello as it is
	// an impostor: it registers as a program, and supervision ignores it.
	impostor := dial(t, sock)
	if _, err := impostor.Hello(ctx, testDeclaration("other")); err != nil {
		t.Fatalf("hello as other: %v", err)
	}
	if h := healthOf(ctx, t, sock, "other"); h.GetState() != verbsv1.ProgramState_PROGRAM_STATE_STARTING {
		t.Fatalf("a hello from a process rig did not start moved the child to %s", h.GetState())
	}
	wantCode(t, impostor.Call(ctx, "rig.health.report", &rigv1.HealthReportRequest{Marker: 9}, &rigv1.HealthReportResponse{}),
		rigv1.Code_CODE_DENIED, "a health report from a process rig did not start")

	child := dial(t, sock)
	if _, err := child.Hello(ctx, testDeclaration("worker")); err != nil {
		t.Fatalf("hello as worker: %v", err)
	}
	if h := healthOf(ctx, t, sock, "worker"); h.GetState() != verbsv1.ProgramState_PROGRAM_STATE_HEALTHY {
		t.Fatalf("the child's own hello left it %s, want HEALTHY", h.GetState())
	}
	if err := child.Call(ctx, "rig.health.report", &rigv1.HealthReportRequest{
		Marker: 7, Waiting: "the build lock",
	}, &rigv1.HealthReportResponse{}); err != nil {
		t.Fatalf("the child's own report: %v", err)
	}
	if h := healthOf(ctx, t, sock, "worker"); h.GetMarker() != 7 || h.GetWaiting() != "the build lock" {
		t.Fatalf("the report did not reach the surface: %+v", h)
	}

	var stopped verbsv1.StopResponse
	if err := cli.Call(ctx, "rig.stop", &verbsv1.StopRequest{Program: "worker"}, &stopped); err != nil {
		t.Fatalf("rig.stop: %v", err)
	}
	p := stopped.GetProgram()
	if p.GetState() != verbsv1.ProgramState_PROGRAM_STATE_UNSPECIFIED || p.GetPid() != 0 || len(p.GetHistory()) < 3 {
		t.Fatalf("a stopped program should be off the table with its history kept: %+v", p)
	}
}
