package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestNotRunningMatchesTheTwoWaysNothingIsListening pins the exact set.
//
// `rig down` is the one verb that reports an unreachable socket as SUCCESS,
// because the caller's goal - no daemon - is already true. That makes
// notRunning a silent-failure risk rather than a convenience: anything it
// wrongly matches becomes a real failure reported as a completed stop.
func TestNotRunningMatchesTheTwoWaysNothingIsListening(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"no socket file at all", syscall.ENOENT, true},
		{"a socket left behind by a daemon that died", syscall.ECONNREFUSED, true},

		// Everything below is a real failure. Reporting any of these as
		// "nothing to stop" would tell someone their daemon is down while it
		// is serving.
		{"the runtime dir is not readable", syscall.EACCES, false},
		{"the socket path is too long", syscall.EINVAL, false},
		{"the daemon is there but wedged", syscall.ETIMEDOUT, false},
		{"something else entirely", errors.New("wire: short read"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Wrapped the way client.Dial wraps it, so the test exercises
			// errors.Is through the same chain production has rather than a
			// bare sentinel.
			wrapped := fmt.Errorf("client: dial /run/x/rigd.sock: %w",
				&net.OpError{Op: "dial", Net: "unix", Err: tc.err})
			if got := notRunning(wrapped); got != tc.want {
				t.Errorf("notRunning(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestNotRunningSeesThroughARealDialError is the same property measured
// against an error the operating system actually produced, because a
// hand-built net.OpError is a guess about what net.Dial returns.
func TestNotRunningSeesThroughARealDialError(t *testing.T) {
	// Kept short: sun_path is 108 bytes.
	dir, err := os.MkdirTemp("", "rigd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	absent := filepath.Join(dir, "s")
	_, err = net.Dial("unix", absent)
	if err == nil {
		t.Fatal("dialling a socket that does not exist succeeded")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the OS did not report a missing socket as ENOENT: %v", err)
	}
	if !notRunning(fmt.Errorf("client: dial %s: %w", absent, err)) {
		t.Error("a genuinely absent socket was not read as 'nothing to stop'")
	}

	// A socket file that exists with nobody listening. This is the case the
	// pidfile-and-signal shape got wrong for a different reason: the file
	// outlives its process, so its presence is not evidence of a daemon.
	stale := filepath.Join(dir, "stale")
	l, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	if _, err := os.Stat(stale); err == nil {
		// Go removes the socket file on Close. If a platform ever keeps it,
		// the connect below is ECONNREFUSED and notRunning must still match.
		_, derr := net.Dial("unix", stale)
		if derr != nil && !notRunning(fmt.Errorf("client: dial %s: %w", stale, derr)) {
			t.Errorf("a stale socket with no listener was not read as 'nothing to stop': %v", derr)
		}
	}
}

// TestDownTakesNoPositionalArgument locks the absence of an estate name.
//
// Proposal P5 asked for "a stop scoped to one estate by name" and was refused:
// the scope is XDG_RUNTIME_DIR, which internal/paths will not guess, so two
// estates are already two runtime directories. Accepting an argument here
// would reintroduce the named-estate concept through the back door.
func TestDownTakesNoPositionalArgument(t *testing.T) {
	err := cmdDown([]string{"someestate"})
	if err == nil {
		t.Fatal("rig down accepted a positional argument")
	}
	if !strings.Contains(err.Error(), "usage: rig down") {
		t.Errorf("the refusal %q does not show the usage", err)
	}
}
