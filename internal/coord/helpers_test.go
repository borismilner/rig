package coord

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// NOTHING IN THIS PACKAGE'S TESTS RUNS IN PARALLEL, and that is a decision
// rather than an omission. The clock, /proc and the boot id are package
// variables so the whole thing can be driven from a test; two tests swapping
// them at once would be reading each other's machine.

type testClock struct{ at Instant }

func (c *testClock) advance(d time.Duration) { c.at = c.at.Add(d) }

// fakeClock replaces CLOCK_BOOTTIME with one the test moves by hand.
//
// Section 20 mandates testing/synctest for timing and says in the same breath
// that synctest CANNOT MODEL SUSPEND, which is the case this package is built
// for. So the clock is injected and moved explicitly, which is the shape that
// section asks for when synctest is structurally blind.
func fakeClock(t *testing.T) *testClock {
	t.Helper()
	c := &testClock{at: Instant(1_000_000_000)}
	old := now
	now = func() (Instant, error) { return c.at, nil }
	t.Cleanup(func() { now = old })
	return c
}

// fakeBoot points the boot id at a file the test owns, and returns a function
// that reboots the machine.
func fakeBoot(t *testing.T, id string) func(string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "boot_id")
	if err := os.WriteFile(p, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := bootIDPath
	bootIDPath = p
	t.Cleanup(func() { bootIDPath = old })
	return func(next string) {
		if err := os.WriteFile(p, []byte(next+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeProc gives the test a process table it controls.
func fakeProc(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := procPath
	procPath = dir
	t.Cleanup(func() { procPath = old })
	return dir
}

// spawn writes a process into the fake table.
//
// THE COMM DELIBERATELY CONTAINS A SPACE AND A CLOSING PARENTHESIS. Field 2 of
// /proc/<pid>/stat is the executable name in parentheses and it is not escaped,
// so a reader that splits the line on whitespace reads the wrong field and gets
// a plausible number. That is the bug this fixture exists to catch.
func spawn(t *testing.T, dir string, pid int, ticks uint64) {
	t.Helper()
	spawnInState(t, dir, pid, ticks, "S")
}

// spawnInState is spawn with the run-state letter chosen, for the one case
// where it is the subject: a zombie.
func spawnInState(t *testing.T, dir string, pid int, ticks uint64, state string) {
	t.Helper()
	d := filepath.Join(dir, fmt.Sprint(pid))
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	// After the last ')' come fields 3 onward; field 22 is index 19 there.
	rest := make([]string, 20)
	rest[0] = state
	for i := 1; i < 19; i++ {
		rest[i] = "0"
	}
	rest[19] = fmt.Sprint(ticks)
	line := fmt.Sprintf("%d (weird) name) %s\n", pid, strings.Join(rest, " "))
	if err := os.WriteFile(filepath.Join(d, "stat"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

// reap removes a process from the fake table.
func reap(t *testing.T, dir string, pid int) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(dir, fmt.Sprint(pid))); err != nil {
		t.Fatal(err)
	}
}

// estate points XDG_STATE_HOME at a temporary tree and returns an estate name.
func estate(t *testing.T, name string) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return name
}

// openStore opens and registers the close.
func openStore(t *testing.T, name string) *Store {
	t.Helper()
	s, err := Open(name)
	if err != nil {
		t.Fatalf("opening the store for %q: %v", name, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
