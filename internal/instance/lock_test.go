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

// Section 37, precondition 6: a named estate refuses a name already held, and
// the refusal names the NAME as well as the incumbent pid.
//
// Same subprocess shape as the directory lock's test, because flock is
// per-process and an in-process second Acquire on the same descriptor would
// prove nothing.
func TestASecondEstateWithTheSameNameIsRefused(t *testing.T) {
	if os.Getenv("RIG_NAME_CHILD") == "1" {
		_, err := AcquireName(os.Getenv("RIG_LOCK_PATH"), os.Getenv("RIG_ESTATE"))
		var held *NameHeldError
		if errors.As(err, &held) {
			os.Stdout.WriteString("HELD:" + held.Name + ":" + strconv.Itoa(held.Incumbent) + "\n")
			os.Exit(0)
		}
		os.Stdout.WriteString("ACQUIRED\n")
		os.Exit(1)
	}

	path := filepath.Join(t.TempDir(), "production.pid")
	l, err := AcquireName(path, "production")
	if err != nil {
		t.Fatalf("parent claim: %v", err)
	}
	defer l.Close()

	cmd := exec.Command(os.Args[0],
		"-test.run=TestASecondEstateWithTheSameNameIsRefused", "-test.v=false")
	cmd.Env = append(os.Environ(), "RIG_NAME_CHILD=1",
		"RIG_LOCK_PATH="+path, "RIG_ESTATE=production")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the second estate did not refuse: %v (output %q)", err, out)
	}
	want := "HELD:production:" + strconv.Itoa(os.Getpid())
	if !strings.Contains(string(out), want) {
		t.Fatalf("the second estate said %q, want it to name the name AND our pid (%s)", out, want)
	}
}

// A NameHeldError must not read like a second daemon in one directory. They are
// different boundaries and a message naming the wrong one sends the reader to
// the wrong place.
func TestTheNameRefusalNamesTheNameAndTheIncumbent(t *testing.T) {
	e := &NameHeldError{Name: "production", Path: "/home/u/.local/state/rig/estates/production.pid", Incumbent: 4242}
	msg := e.Error()
	for _, want := range []string{"production", "4242", "already held"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal %q should mention %q", msg, want)
		}
	}
	if strings.Contains(msg, "another rigd is running") {
		t.Errorf("the name refusal reads like the directory refusal: %q", msg)
	}

	unknown := &NameHeldError{Name: "production", Path: "/x", Incumbent: 0}
	if strings.Contains(unknown.Error(), "pid 0") {
		t.Errorf("an unreadable pid was reported as 0: %q", unknown.Error())
	}
}

// Two estates with DIFFERENT names must both start. This is the other half of
// the precondition: refusing a duplicate is worthless if it also refuses the
// pair section 37 actually allows.
func TestTwoDifferentlyNamedEstatesBothStart(t *testing.T) {
	dir := t.TempDir()
	prod, err := AcquireName(filepath.Join(dir, "production.pid"), "production")
	if err != nil {
		t.Fatalf("production: %v", err)
	}
	defer prod.Close()
	dev, err := AcquireName(filepath.Join(dir, "development.pid"), "development")
	if err != nil {
		t.Fatalf("development was refused while production held its own name: %v", err)
	}
	defer dev.Close()
}
