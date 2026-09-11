package main

import (
	"errors"
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

// Section 10 promises --json on everything, and PLAN.md:1512 names
// `rig --json list` as the agent affordance BY EXAMPLE. partition covers a
// flag AFTER the verb; this is the half that was missing, and the plan's own
// example failed with `no such command "--json"` until it existed.
func TestRigsOwnFlagsMayPrecedeTheVerb(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
		want int
	}{
		{"a flag before the verb", []string{"--json", "apps", "list"}, 1},
		{"no flag at all", []string{"apps", "list", "--json"}, 0},
		{
			"a valued flag takes its value with it",
			[]string{"--timeout", "5s", "ping", "fakeapp"},
			2,
		},
		{
			"a valued flag written with equals does not",
			[]string{"--timeout=5s", "ping"},
			1,
		},
		{"two flags", []string{"--json", "--timeout=5s", "apps"}, 2},
		{"everything after -- is positional", []string{"--", "fakeapp"}, 1},
		{"a bare dash is not a flag", []string{"-"}, 0},

		// No verb at all. These must come back as len(args), because that is
		// what leaves `rig -h` and `rig --help` reaching the help case
		// instead of being stripped down to no command at all.
		{"help alone", []string{"--help"}, 1},
		{"short help alone", []string{"-h"}, 1},
		{"a flag and nothing else", []string{"--json"}, 1},
		{"nothing", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := verbAt(tc.in); got != tc.want {
				t.Errorf("verbAt(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// The flags that preceded the verb go back at the END, not just after the
// verb. `rig --json fakeapp reindex --since 7d` only works if the program and
// the command stay in positions 0 and 1 - a flag moved to just after the verb
// would be read as the command.
func TestPrecedingFlagsGoBackAtTheEnd(t *testing.T) {
	argv := []string{"fakeapp", "reindex", "--since", "7d"}
	got := with(argv[2:], []string{"--json"})
	want := []string{"--since", "7d", "--json"}
	if !slices.Equal(got, want) {
		t.Errorf("with = %q, want %q", got, want)
	}
	// And it must not write through the slice it was given: argv is a window
	// onto os.Args, and appending in place would overwrite whatever follows.
	// Spare capacity is the only condition under which that can happen, so
	// the check has to create it rather than hope for it - argv[2:] above has
	// none, which would make this assertion pass on a broken with().
	backing := make([]string, 4, 8)
	copy(backing, argv)
	backing[4:8][0] = "MUST NOT BE OVERWRITTEN"
	_ = with(backing[2:4], []string{"--json"})
	if backing[4:8][0] != "MUST NOT BE OVERWRITTEN" {
		t.Errorf("with appended through its input and clobbered what followed")
	}
	if got := with(argv, nil); !slices.Equal(got, argv) {
		t.Errorf("with(args, nil) = %q, want it unchanged", got)
	}
}

// The flags that preceded the verb have to REACH the handler, and the path
// that matters most is a declared command, because that is what an agent
// invokes. Nothing short of running the dispatch can see this go wrong: with()
// stays correct in isolation while the call site that uses it is missing, and
// a mutation removing exactly that call passed every other test in this
// package.
//
// No daemon is needed, which is what makes it cheap. The call fails at the
// socket, and the failure carries the output mode the handler parsed - which
// is precisely the thing under test.
func TestTheFlagsBeforeTheVerbReachTheHandler(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	for _, tc := range []struct {
		name string
		argv []string
		want bool
	}{
		{
			"--json before the program",
			[]string{"--json", "fakeapp", "reindex", "--since", "7d"},
			true,
		},
		{
			"--json after the command",
			[]string{"fakeapp", "reindex", "--since", "7d", "--json"},
			true,
		},
		{
			"--json before, with a valued flag beside it",
			[]string{"--timeout", "9s", "--json", "fakeapp", "reindex"},
			true,
		},
		{
			"no --json at all",
			[]string{"fakeapp", "reindex", "--since", "7d"},
			false,
		},

		// Every verb that parses --json, not only the declared-command path.
		// A missing with() on any one of them gives that one surface prose
		// where section 10 promises an object, and nothing else would notice.
		{"ping", []string{"--json", "ping", "fakeapp"}, true},
		{"apps", []string{"--json", "apps", "list"}, true},

		// `rig down` is deliberately absent: with no daemon it SUCCEEDS,
		// because the caller's goal is already true, so there is no failure
		// to read the mode off. down_test.go's notRunning set is what covers
		// that path instead.
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.argv)
			if err == nil {
				t.Fatal("the call reached a daemon that should not be there")
			}
			var l *rigError
			if !errors.As(err, &l) {
				t.Fatalf("want a shaped failure, got %T: %v", err, err)
			}
			if l.object.Code != codeNoDaemon {
				t.Fatalf("want %s, got %s: %v", codeNoDaemon, l.object.Code, err)
			}
			if l.asJSON != tc.want {
				t.Errorf("the handler saw --json as %v, want %v", l.asJSON, tc.want)
			}
		})
	}
}
