package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/borismilner/rig/internal/paths"
)

// privateStatePrefix and privateRuntimePrefix name the roots TestMain makes,
// and TestNoTestHereCanReachTheRootsTheShellHandedUs recognises them by
// these names rather than by what the shell had: without TestMain there is
// nothing left that remembers the shell's values, and the guard must still
// go red then.
const (
	privateStatePrefix   = "rigd-test-state-"
	privateRuntimePrefix = "rigd-test-"
)

// TestMain puts every test in this package under a state root and a runtime
// root of its own, before any test can resolve either.
//
// ⛔ MEASURED 2026-09-24, FROM `make ci` IN THE SHARED TREE ON THE TEAM'S OWN
// MACHINE: eight tests here opened the LIVE production estate under the daemon
// serving it, and the development estate beside it. The path is
// New(Config{Estate: "production"}) -> record.Open -> paths.EstateStateDir,
// which reads XDG_STATE_HOME and falls back to $HOME. Nothing was written; a
// schema bump would have migrated his store from a test process. The account
// is in COORDINATION.md, "a private XDG_RUNTIME_DIR isolates the socket and
// not the store".
//
// A t.Setenv per test is the right shape for a test that needs a particular
// root and the wrong shape for a guarantee: the next test written without it
// inherits the shell again. This runs before every test in the package, and
// the guard below fails the package the day it stops holding.
func TestMain(m *testing.M) {
	state, err := os.MkdirTemp("", privateStatePrefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon tests: no private state root: %v\n", err)
		os.Exit(1)
	}
	// Short deliberately, and inside the shell's own runtime directory when it
	// has one: sun_path is 108 bytes and a socket under a long TMPDIR fails as
	// a bare EINVAL. upEstate makes the same choice for the same reason.
	runtime, err := os.MkdirTemp(existingDir(os.Getenv("XDG_RUNTIME_DIR")), privateRuntimePrefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon tests: no private runtime root: %v\n", err)
		_ = os.RemoveAll(state)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_STATE_HOME", state); err != nil {
		fmt.Fprintf(os.Stderr, "daemon tests: XDG_STATE_HOME: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_RUNTIME_DIR", runtime); err != nil {
		fmt.Fprintf(os.Stderr, "daemon tests: XDG_RUNTIME_DIR: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	// A goroutine still running after every test has returned is a leak in
	// the daemon or in a test, and rigd is long-lived, so either accumulates.
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Fprintf(os.Stderr, "daemon tests: %v\n", err)
			code = 1
		}
	}
	_ = os.RemoveAll(state)
	_ = os.RemoveAll(runtime)
	os.Exit(code)
}

// existingDir is dir if it names a directory, else "" so MkdirTemp falls back
// to the system temp directory.
func existingDir(dir string) string {
	if dir == "" {
		return ""
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return dir
}

// TestNoTestHereCanReachTheRootsTheShellHandedUs asks paths - the same
// functions record.Open and the listener ask - and fails unless both answers
// lie under roots TestMain made. Delete TestMain and this goes red.
func TestNoTestHereCanReachTheRootsTheShellHandedUs(t *testing.T) {
	state, err := paths.StateDir()
	if err != nil {
		t.Fatalf("paths.StateDir: %v", err)
	}
	runtime, err := paths.RuntimeDir()
	if err != nil {
		t.Fatalf("paths.RuntimeDir: %v", err)
	}

	// paths appends "rig" to each root, so the root is the parent.
	if root := filepath.Base(filepath.Dir(state)); !strings.HasPrefix(root, privateStatePrefix) {
		t.Errorf("the state root every test here resolves is %s, which TestMain "+
			"did not make: a test that names an estate opens whatever store the "+
			"shell's XDG_STATE_HOME or HOME points at", state)
	}
	if root := filepath.Base(filepath.Dir(runtime)); !strings.HasPrefix(root, privateRuntimePrefix) {
		t.Errorf("the runtime root is %s, which TestMain did not make: a test "+
			"that dials reaches whatever daemon the shell's XDG_RUNTIME_DIR serves", runtime)
	}

	// The fallback paths resolves to when XDG_STATE_HOME is unset, named
	// outright so the message says where the real store is.
	if home, err := os.UserHomeDir(); err == nil {
		machines := filepath.Join(home, ".local", "state", "rig")
		if state == machines || strings.HasPrefix(state, machines+string(filepath.Separator)) {
			t.Errorf("the state root is %s, the machine's own", state)
		}
	}
	if _, err := os.Stat(filepath.Dir(state)); err != nil {
		t.Errorf("the private state root %s is not there: %v", filepath.Dir(state), err)
	}
}
