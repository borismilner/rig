package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/53 slice 6: `rig peers run`, the real binary against a real daemon.

var (
	rigOnce sync.Once
	rigPath string
	errRig  error
)

// rigBinary builds cmd/rig once per test run.
func rigBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the rig binary")
	}
	rigOnce.Do(func() {
		dir, err := os.MkdirTemp("", "rigbin")
		if err != nil {
			errRig = err
			return
		}
		rigPath = filepath.Join(dir, "rig")
		out, err := exec.Command("go", "build", "-o", rigPath, "../../cmd/rig").CombinedOutput()
		if err != nil {
			errRig = errors.New(string(out))
		}
	})
	if errRig != nil {
		t.Fatalf("building rig: %v", errRig)
	}
	return rigPath
}

// runUnder starts `rig peers run` against the daemon at sock.
func runUnder(t *testing.T, sock string, args ...string) *exec.Cmd {
	t.Helper()
	rt := t.TempDir()
	if err := os.Mkdir(filepath.Join(rt, "rig"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sock, filepath.Join(rt, "rig", "rigd.sock")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rigBinary(t), append([]string{"peers", "run"}, args...)...)
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+rt)
	return cmd
}

func exitCode(err error) int {
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestARunHoldsItsLeaseAndPassesItsExitOn(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	watch := dial(t, sock)
	out, err := runUnder(t, sock, "--lease=build", "--ttl=3s", "--", "sh", "-c",
		"exit 3").CombinedOutput()
	if exitCode(err) != 3 {
		t.Fatalf("the command's status was not passed on: %v\n%s", err, out)
	}
	if l := leaseList(recordCtx(t), t, watch)["build"]; l != nil && l.GetState() == verbsv1.LeaseState_LEASE_STATE_HELD {
		t.Fatalf("the lease outlived the run: %+v", l)
	}

	a := seated(t, sock, "seat-a")
	if err := a.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "build", TtlMs: 30000},
		&verbsv1.LeaseAcquireResponse{}); err != nil {
		t.Fatal(err)
	}
	held, err := runUnder(t, sock, "--lease=build", "--", "true").CombinedOutput()
	if exitCode(err) != exitLeaseLostForTest || !strings.Contains(string(held), "seat-a") {
		t.Fatalf("a held lease ran the command anyway: %v\n%s", err, held)
	}
}

// exitLeaseLostForTest is cmd/rig's exitLeaseLost, which this package
// cannot import.
const exitLeaseLostForTest = 75

// ⛔ A STALLED HOLDER'S WORK STOPS BEFORE THE NEXT HOLDER STARTS (PLAN.md
// section 16, the fifth gate): the run's `rig` is frozen, so it renews
// nothing; at the deadline rigd kills the fenced group and the frozen
// holder, and only then is the lease handed on. AgentBox's wrapped lock
// would have let both write.
func TestAStalledRunIsStoppedBeforeItsLeasePassesOn(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	watch := dial(t, sock)
	cursor := waitOn(t, watch, 0, 1, "lease.*").GetLatest()

	log := filepath.Join(t.TempDir(), "writes")
	run := runUnder(t, sock, "--lease=deploy", "--ttl=3s", "--", "sh", "-c",
		`while :; do echo w >> "$WRITES"; sleep 0.05; done`)
	run.Env = append(run.Env, "WRITES="+log)
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Process.Kill(); _ = run.Wait() })
	for deadline := time.Now().Add(10 * time.Second); size(log) == 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the run never started writing")
		}
	}
	time.Sleep(300 * time.Millisecond) // past the fence call
	_ = run.Process.Signal(syscall.SIGSTOP)

	b := seated(t, sock, "seat-b")
	var got verbsv1.LeaseAcquireResponse
	if err := b.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{
		Name: "deploy", TtlMs: 5000, WaitMs: 15000,
	}, &got); err != nil || got.GetHandle().GetHolder() != "seat-b" {
		t.Fatalf("the next holder was not granted it: %+v %v", &got, err)
	}
	at := size(log)
	time.Sleep(400 * time.Millisecond)
	if after := size(log); after != at {
		t.Fatalf("the stalled run was still writing after the lease passed on: %d then %d bytes", at, after)
	}
	if err := run.Wait(); err == nil {
		t.Fatal("the frozen holder was not stopped")
	}

	var seen []string
	for !strings.Contains(strings.Join(seen, " "), "acquired:seat-b") {
		evs := waitOn(t, watch, cursor, 5000, "lease.*")
		if len(evs.GetEvents()) == 0 {
			t.Fatalf("lease.changed said only %v", seen)
		}
		for _, ev := range evs.GetEvents() {
			var p leaseChange
			_ = json.Unmarshal([]byte(ev.GetPayloadJson()), &p)
			seen = append(seen, p.Change+":"+p.Holder)
			cursor = ev.GetSeq()
		}
	}
	if s := strings.Join(seen, " "); !strings.Contains(s, "fenced:") || strings.Index(s, "fenced:") > strings.Index(s, "acquired:seat-b") {
		t.Fatalf("the run was not fenced before the lease passed on: %s", s)
	}
}

func size(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// lease.fence names a group rigd will kill, so it is refused unless the
// caller is the process the lease witnessed and the group is one it started.
func TestAFenceIsRefusedForWhatTheCallerDidNotStart(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	var un verbsv1.LeaseAcquireResponse
	if err := a.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "loose", TtlMs: 30000, Unwitnessed: true}, &un); err != nil {
		t.Fatal(err)
	}
	stranger := exec.Command("sleep", "30")
	stranger.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := stranger.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stranger.Process.Kill(); _ = stranger.Wait() })
	fence := func(h *verbsv1.LeaseHandle, pid int) error {
		return a.Call(recordCtx(t), "rig.lease.fence", &verbsv1.LeaseFenceRequest{
			Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(), Pid: uint32(pid),
		}, &verbsv1.LeaseFenceResponse{})
	}
	wantCode(t, fence(un.GetHandle(), stranger.Process.Pid), rigv1.Code_CODE_DENIED, "an unwitnessed lease fenced")

	var w verbsv1.LeaseAcquireResponse
	if err := a.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "mine", TtlMs: 30000}, &w); err != nil {
		t.Fatal(err)
	}
	if err := fence(w.GetHandle(), stranger.Process.Pid); err != nil {
		t.Fatalf("this test's own child was refused: %v", err)
	}
	wantCode(t, fence(w.GetHandle(), 1), rigv1.Code_CODE_INVALID, "pid 1 fenced")
	notLeader := exec.Command("sleep", "30")
	if err := notLeader.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = notLeader.Process.Kill(); _ = notLeader.Wait() })
	wantCode(t, fence(w.GetHandle(), notLeader.Process.Pid), rigv1.Code_CODE_INVALID, "a process in the caller's own group fenced")
}

// ⛔ TWO RUNS FROM ONE TERMINAL USER EXCLUDE EACH OTHER: every terminal of a
// user is the same holder, and the reconnect path handed a running run's
// lease straight to the next, so both wrote. Found live on 2026-10-02.
func TestASecondRunFromTheSameUserWaitsForTheFirst(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	log := filepath.Join(t.TempDir(), "writes")
	first := runUnder(t, sock, "--lease=deploy", "--ttl=3s", "--", "sh", "-c",
		`while :; do echo w >> "$WRITES"; sleep 0.05; done`)
	first.Env = append(first.Env, "WRITES="+log)
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Process.Kill(); _ = first.Wait() })
	for deadline := time.Now().Add(10 * time.Second); size(log) == 0; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the first run never started writing")
		}
	}
	time.Sleep(300 * time.Millisecond)

	out, err := runUnder(t, sock, "--lease=deploy", "--", "true").CombinedOutput()
	if exitCode(err) != exitLeaseLostForTest {
		t.Fatalf("a second run took a lease the first still held: %v\n%s", err, out)
	}

	_ = first.Process.Signal(syscall.SIGSTOP)
	start := time.Now()
	out, err = runUnder(t, sock, "--lease=deploy", "--wait=15s", "--", "true").CombinedOutput()
	if err != nil {
		t.Fatalf("the second run was not handed the lease the frozen one lost: %v\n%s", err, out)
	}
	if time.Since(start) < 2*time.Second {
		t.Fatalf("the second run did not wait for the first's deadline: %s", time.Since(start))
	}
	at := size(log)
	time.Sleep(400 * time.Millisecond)
	if after := size(log); after != at {
		t.Fatalf("the frozen run was still writing after the lease passed on: %d then %d", at, after)
	}
}
