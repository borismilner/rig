package instance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The behaviours the existing tests do not reach, and they are the ones that
// only run on a bad day: an unreadable pidfile, a runtime directory that cannot
// be made, and a shutdown path that releases twice. Each is a decision recorded
// in lock.go as a comment and nowhere else.

func TestAHeldLockWithAnUnreadablePidfileStillSaysSomebodyIsThere(t *testing.T) {
	// The recorded decision: "the lock is what decides, not the file's
	// contents ... reporting no pid is better than reporting a wrong one." So
	// an empty or garbled pidfile under a held lock must still refuse, and must
	// say the pid is unknown rather than inventing one or claiming to be alone.
	for _, tc := range []struct {
		name     string
		contents string
	}{
		{"empty", ""},
		{"not a number", "rigd\n"},
		{"a number with rubbish after it", "1234 and then some\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rigd.pid")
			held, err := Acquire(path)
			if err != nil {
				t.Fatalf("first acquire: %v", err)
			}
			defer func() { _ = held.Close() }()

			// Overwrite what Acquire wrote, without touching the lock.
			if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
				t.Fatalf("rewrite pidfile: %v", err)
			}

			_, err = Acquire(path)
			var he *HeldError
			if !errors.As(err, &he) {
				t.Fatalf("second acquire returned %v, want a *HeldError: an "+
					"unreadable pidfile must not read as an empty estate", err)
			}
			if he.Incumbent != 0 {
				t.Errorf("Incumbent is %d, want 0: the pid was not readable "+
					"and a wrong pid is worse than none", he.Incumbent)
			}
			if !strings.Contains(he.Error(), "pid unknown") {
				t.Errorf("the message is %q, and it has to say the pid is "+
					"unknown rather than omit the question", he.Error())
			}
			if !strings.Contains(he.Error(), path) {
				t.Errorf("the message is %q and does not name the lock, so a "+
					"person cannot go and look", he.Error())
			}
		})
	}
}

func TestTheRefusalNamesTheIncumbentWhenItCan(t *testing.T) {
	// The other half of the same decision, asserted on the message rather than
	// on the struct: a readable pid has to reach the text, because "another
	// rigd is running" with no pid is the version that cannot be acted on.
	he := &HeldError{Path: "/run/user/1000/rig/rigd.pid", Incumbent: 4242}
	msg := he.Error()
	if !strings.Contains(msg, "4242") {
		t.Errorf("the message is %q and drops the pid it was given", msg)
	}
	if strings.Contains(msg, "unknown") {
		t.Errorf("the message is %q and says unknown for a pid it has", msg)
	}
}

func TestAcquireSaysWhichStepFailedWhenTheRuntimeDirCannotBeMade(t *testing.T) {
	// A regular file where the runtime directory should be. This is the shape
	// of the XDG_RUNTIME_DIR problems that actually happen, and the message has
	// to say the directory failed rather than blaming the lock.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "rig")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err := Acquire(filepath.Join(blocker, "rigd.pid"))
	if err == nil {
		t.Fatal("acquire succeeded with a file where its directory should be")
	}
	var he *HeldError
	if errors.As(err, &he) {
		t.Fatalf("reported %v as another rigd holding the lock, which sends "+
			"the reader looking for a process that does not exist", err)
	}
	if !strings.Contains(err.Error(), "runtime dir") {
		t.Errorf("the error is %q and does not say the runtime directory was "+
			"the step that failed", err)
	}
}

func TestReleasingTwiceIsSafeAndSoIsReleasingNothing(t *testing.T) {
	// Both are written into Close as a nil guard, and both are reachable: a
	// shutdown path that runs its deferred release after an explicit one, and
	// a startup that failed before it ever had a lock.
	path := filepath.Join(t.TempDir(), "rigd.pid")
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if got := l.Path(); got != path {
		t.Errorf("Path is %q, want %q", got, path)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("second close returned %v, want nil", err)
	}

	var absent *Lock
	if err := absent.Close(); err != nil {
		t.Errorf("closing a nil lock returned %v, want nil", err)
	}

	// And the claim really is gone rather than merely marked gone: the whole
	// point of releasing is that the next daemon can start.
	next, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire after release: %v, so a closed lock still holds", err)
	}
	_ = next.Close()
}
