package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/daemon"
)

// Section 10 promises --json on everything. Go's flag package stops at the
// first positional, so this is the test that keeps the promise independent of
// argument order.
func TestPartitionAcceptsFlagsAnywhere(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         []string
		wantFlags  []string
		wantPosArg []string
	}{
		{
			"flag after positional",
			[]string{"fakeapp", "--json"},
			[]string{"--json"},
			[]string{"fakeapp"},
		},
		{
			"flag before positional",
			[]string{"--json", "fakeapp"},
			[]string{"--json"},
			[]string{"fakeapp"},
		},
		{
			"valued flag after",
			[]string{"fakeapp", "--timeout", "9s"},
			[]string{"--timeout", "9s"},
			[]string{"fakeapp"},
		},
		{
			"valued flag with equals",
			[]string{"fakeapp", "--timeout=9s"},
			[]string{"--timeout=9s"},
			[]string{"fakeapp"},
		},
		{
			"bool flag does not eat the positional",
			[]string{"--json", "fakeapp"},
			[]string{"--json"},
			[]string{"fakeapp"},
		},
		{
			"double dash ends flags",
			[]string{"--json", "--", "-weird-name"},
			[]string{"--json"},
			[]string{"-weird-name"},
		},
		{"nothing", nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotFlags, gotPos := partition(tc.in)
			if !slices.Equal(gotFlags, tc.wantFlags) {
				t.Errorf("flags = %q, want %q", gotFlags, tc.wantFlags)
			}
			if !slices.Equal(gotPos, tc.wantPosArg) {
				t.Errorf("positional = %q, want %q", gotPos, tc.wantPosArg)
			}
		})
	}
}

// `rig ping ""` must be refused before anything is dialled.
//
// It used to fail at the daemon, because ".ping" is not a
// <program>.<command>. Once the target moved into rig.ping's argument the
// daemon would have read an empty name as "probe rig itself", so the command
// would have answered about rig and looked like it had worked.
func TestPingRefusesAnEmptyProgramName(t *testing.T) {
	err := cmdPing([]string{""})
	if err == nil {
		t.Fatal(`rig ping "" was accepted`)
	}
	if !strings.Contains(err.Error(), "the program name is empty") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// The client must outlast the daemon, or rig's own diagnosis never reaches
// the user.
//
// Section 18 makes the daemon's deadline a supervision decision and rig
// answers a hang itself, naming the program and the deadline it missed. If
// the CLI gives up first the user sees `context deadline exceeded` from their
// own process instead. `rig ping` defaulted to 5s against a 10s CallTimeout
// and did exactly that.
//
// This test imports the daemon so the two numbers cannot drift apart in
// silence. It is a test-only import: the CLI does not link the daemon.
func TestTheClientOutlastsTheDaemonsOwnDeadline(t *testing.T) {
	if defaultCallTimeout <= daemon.CallTimeout {
		t.Fatalf("the CLI waits %s and the daemon answers a hang at %s, so the "+
			"client gives up first and rig's own message never lands",
			defaultCallTimeout, daemon.CallTimeout)
	}
}
