package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/rig/internal/paths"
)

// privateStatePrefix and privateRuntimePrefix name the roots isolateRoots
// makes. The guard below recognises them by name rather than by what the
// shell had, so it goes red the day TestMain stops calling isolateRoots.
const (
	privateStatePrefix   = "rig-test-state-"
	privateRuntimePrefix = "rig-test-"
)

// isolateRoots points XDG_STATE_HOME and XDG_RUNTIME_DIR at roots of this
// run's own before any test can resolve either, and returns their removal.
//
// Measured 2026-09-24 (BACKLOG B87, its cmd/rig half): a test in this
// package ran under the shell's XDG_RUNTIME_DIR and reached the socket of
// the production daemon during a seat's mutation pass. Every transcript case
// already sets a runtime directory of its own; this is the guarantee for the
// test that does not. internal/daemon carries the same shape.
func isolateRoots() (cleanup func(), err error) {
	state, err := os.MkdirTemp("", privateStatePrefix)
	if err != nil {
		return nil, fmt.Errorf("no private state root: %w", err)
	}
	// Short, and inside the shell's own runtime directory when it has one:
	// sun_path is 108 bytes and a socket under a long TMPDIR fails as a bare
	// EINVAL.
	runtime, err := os.MkdirTemp(existingDir(os.Getenv("XDG_RUNTIME_DIR")), privateRuntimePrefix)
	if err != nil {
		_ = os.RemoveAll(state)
		return nil, fmt.Errorf("no private runtime root: %w", err)
	}
	cleanup = func() {
		_ = os.RemoveAll(state)
		_ = os.RemoveAll(runtime)
	}
	if err := os.Setenv("XDG_STATE_HOME", state); err != nil {
		cleanup()
		return nil, fmt.Errorf("XDG_STATE_HOME: %w", err)
	}
	if err := os.Setenv("XDG_RUNTIME_DIR", runtime); err != nil {
		cleanup()
		return nil, fmt.Errorf("XDG_RUNTIME_DIR: %w", err)
	}
	return cleanup, nil
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

// TestNoTestHereCanReachTheRootsTheShellHandedUs asks paths, the same
// functions the binary under test and the in-process verbs ask, and fails
// unless both answers lie under roots isolateRoots made. Delete the call in
// TestMain and this goes red.
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
			"did not make: a verb that names an estate opens whatever store the "+
			"shell's XDG_STATE_HOME or HOME points at", state)
	}
	if root := filepath.Base(filepath.Dir(runtime)); !strings.HasPrefix(root, privateRuntimePrefix) {
		t.Errorf("the runtime root is %s, which TestMain did not make: a verb "+
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
