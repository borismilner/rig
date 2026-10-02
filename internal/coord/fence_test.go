package coord

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// plan/53 slice 6: what a wrapped run's fence records and what rigd may kill.

func startGroup(t *testing.T, own bool, script string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: own}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

func me(t *testing.T) Witness {
	t.Helper()
	w, err := WitnessProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestAFenceIsTheHoldersAndGoesWithTheHold(t *testing.T) {
	s := openStore(t, estate(t, "fence"))
	h, err := s.Acquire("deploy", "seat-a", me(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	group := Witness{Kind: PID, Pid: 4242, StartTicks: 1, BootID: "b"}
	if err := s.Fence(h, group); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Inspect("deploy"); st.Fence == nil || *st.Fence != group {
		t.Fatalf("the fence was not recorded: %+v", st)
	}
	stale := h
	stale.Token--
	var fenced *FencedError
	if err := s.Fence(stale, group); !errors.As(err, &fenced) {
		t.Fatalf("an old handle fenced the lease: %v", err)
	}
	again, err := s.Acquire("deploy", "seat-a", me(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Inspect("deploy"); st.Fence != nil {
		t.Fatalf("a new hold kept the old run's fence: %+v", st)
	}
	_ = again
	if err := s.Fence(h, Witness{Kind: Unwitnessed}); err == nil {
		t.Fatal("a fence that names no process was taken")
	}
}

// ⛔ rigd KILLS ONLY WHAT ITS CALLER STARTED: a group leader whose parent
// is the caller, as the caller's own user.
func TestOnlyACallersOwnGroupCanBeAFence(t *testing.T) {
	own := startGroup(t, true, "sleep 30")
	if _, err := WitnessGroup(own.Process.Pid, me(t)); err != nil {
		t.Fatalf("the caller's own group was refused: %v", err)
	}
	shared := startGroup(t, false, "sleep 30")
	if _, err := WitnessGroup(shared.Process.Pid, me(t)); err == nil {
		t.Fatal("a process in the caller's own group was taken as a fence")
	}
	init, err := WitnessProcess(1)
	if err != nil {
		t.Skip("cannot witness pid 1 here")
	}
	if _, err := WitnessGroup(own.Process.Pid, init); err == nil {
		t.Fatal("a group somebody else started was taken as a fence")
	}
	if _, err := WitnessGroup(1, me(t)); err == nil {
		t.Fatal("pid 1 was taken as a fence")
	}
}

// ⛔ A KILL REACHES THE WHOLE GROUP, and never a pid now naming another
// process.
func TestAKillStopsTheGroupAndNothingElse(t *testing.T) {
	run := startGroup(t, true, "sleep 30 & wait")
	w, err := WitnessGroup(run.Process.Pid, me(t))
	if err != nil {
		t.Fatal(err)
	}
	recycled := w
	recycled.StartTicks++
	if recycled.Kill(true) {
		t.Fatal("a kill was sent to a pid whose start time does not match")
	}
	if !w.Kill(true) {
		t.Fatal("the group was not signalled")
	}
	done := make(chan error, 1)
	go func() { done <- run.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the run outlived its kill")
	}
	// The sleep was in the group too: nothing is left in it.
	time.Sleep(50 * time.Millisecond)
	if err := syscall.Kill(-w.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("the group still has members: %v", err)
	}
	if w.Kill(true) {
		t.Fatal("a kill was sent to a group already gone")
	}
}
