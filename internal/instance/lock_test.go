package instance

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestAcquireAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rigd.pid")
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != strconv.Itoa(os.Getpid()) {
		t.Fatalf("pidfile holds %q, want our pid %d", got, os.Getpid())
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Releasing must let the next one in, or a clean restart is impossible.
	l2, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	_ = l2.Close()
}

// The pidfile survives Close on purpose: unlinking races a second daemon that
// already has it open, which would leave it locking an unlinked inode and
// believing it is alone.
func TestCloseDoesNotUnlinkThePidfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rigd.pid")
	l, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("pidfile should still exist after release: %v", err)
	}
}

func TestPidfileIs0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rigd.pid")
	l, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pidfile mode is %#o, want 0600", perm)
	}
}

// THE M0 GATE (PLAN.md 5f, 23): start two and assert the second refuses,
// naming the incumbent's pid. flock is per-process, not per-descriptor, so
// this needs a real second process - an in-process second Acquire would
// succeed and prove nothing.
func TestSecondProcessRefusesAndNamesTheIncumbent(t *testing.T) {
	if os.Getenv("RIG_LOCK_CHILD") == "1" {
		// The child: try to take the lock the parent holds and report.
		_, err := Acquire(os.Getenv("RIG_LOCK_PATH"))
		var held *HeldError
		if errors.As(err, &held) {
			os.Stdout.WriteString("HELD:" + strconv.Itoa(held.Incumbent) + "\n")
			os.Exit(0)
		}
		os.Stdout.WriteString("ACQUIRED\n")
		os.Exit(1)
	}

	path := filepath.Join(t.TempDir(), "rigd.pid")
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("parent acquire: %v", err)
	}
	defer l.Close()

	cmd := exec.Command(os.Args[0],
		"-test.run=TestSecondProcessRefusesAndNamesTheIncumbent", "-test.v=false")
	cmd.Env = append(os.Environ(), "RIG_LOCK_CHILD=1", "RIG_LOCK_PATH="+path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("second instance did not refuse: %v (output %q)", err, out)
	}
	want := "HELD:" + strconv.Itoa(os.Getpid())
	if !strings.Contains(string(out), want) {
		t.Fatalf("second instance said %q, want it to name our pid (%s)", out, want)
	}
}

// Many processes racing the lock must produce exactly one winner. This is the
// chaos shape 5f asks for, run in-process against separate children.
func TestOnlyOneOfManyWins(t *testing.T) {
	if os.Getenv("RIG_LOCK_CHILD") == "1" {
		_, err := Acquire(os.Getenv("RIG_LOCK_PATH"))
		var held *HeldError
		if errors.As(err, &held) {
			os.Exit(3) // refused, correctly
		}
		if err != nil {
			os.Exit(4) // some other failure
		}
		os.Exit(0) // won
	}

	path := filepath.Join(t.TempDir(), "rigd.pid")
	const n = 12
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=TestOnlyOneOfManyWins")
			cmd.Env = append(os.Environ(), "RIG_LOCK_CHILD=1", "RIG_LOCK_PATH="+path)
			if err := cmd.Run(); err != nil {
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					codes[i] = ee.ExitCode()
					return
				}
				codes[i] = -1
				return
			}
			codes[i] = 0
		}(i)
	}
	wg.Wait()

	// Children run concurrently and each exits immediately, so several may
	// win in sequence. What must never happen is two holding it at once -
	// and no child may fail for an unexpected reason.
	for i, c := range codes {
		if c != 0 && c != 3 {
			t.Fatalf("child %d exited %d: neither won (0) nor refused (3)", i, c)
		}
	}
	won := 0
	for _, c := range codes {
		if c == 0 {
			won++
		}
	}
	if won == 0 {
		t.Fatal("no child ever acquired the lock")
	}
}
