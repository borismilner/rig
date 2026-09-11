package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// THE ONLY TEST IN THIS PACKAGE THAT RUNS rig AS A PROCESS.
//
// Everything else here calls run(), a renderer or a parser, and that leaves one
// seam uncovered that no amount of in-process testing can reach: main() itself.
// It is five lines -
//
//	if err := run(os.Args[1:]); err != nil {
//	        report(os.Stdout, os.Stderr, err)
//	        os.Exit(1)
//	}
//
// - and those five lines are where the CHANNEL and the EXIT CODE are chosen.
// report() is well covered in refusal_test.go, but every one of those tests
// passes it its own buffers, so the two real file descriptors are selected here
// and asserted nowhere. Section 10's ratified contract - the --json object goes
// to stdout, human prose keeps stderr, the exit is non-zero either way - was
// therefore structurally untestable.
//
// Measured before this file was written, at 8dc3c01, both in a detached
// worktree: swapping report's two writers so the object lands on stderr, and
// changing os.Exit(1) to os.Exit(0), each SURVIVE `go test -race ./...` with
// zero failures. The calibration that makes those two numbers mean anything is
// a third mutation - valuedFlags["depth"] = false - which fails two named tests
// and exits 1, proving the suite was live and able to go red while blind to the
// other two.
//
// Section 20 asks for "testscript golden transcripts for every command
// including failures". The transcripts and the breadth are the obligation; the
// library was plumbing, and rig's CLI cases are argv, one environment variable,
// an exit code and two streams, with no filesystem fixtures and no multi-step
// setup. The pattern below is the one this repository had already reached twice
// on its own - internal/instance/lock_test.go re-execs a binary and reads its
// exit code, completion_shell_test.go execs a real shell - so it costs no
// dependency at all.
//
// THE BINARY IS BUILT FROM SOURCE ON EVERY RUN, into a temporary directory.
// That is not tidiness, it is the one property this file must have: a check
// that execs a stale binary and reports green is the same defect as sizeratchet
// measuring a previous build and believing it, and this repository has now
// caught that shape often enough to design against it rather than to remember
// it. A stale binary is impossible here by construction rather than by
// discipline.
//
// It is built WITHOUT the Makefile's -trimpath and -ldflags. That is
// deliberate, and it is not the trap the handoff records: measuring a SIZE with
// a bare `go build` is wrong because make strips and this does not. Nothing
// here measures size. What the flags would cost is determinism - LDFLAGS injects
// `git describe`, a short sha and a UTC timestamp - and a transcript containing
// the time it was recorded can never match a golden file. Unstamped, the
// version fields are their compile-time defaults and `rig version` is stable.

// updateGolden rewrites every transcript from what the binary actually printed.
// Regenerating is deliberately a separate act from running: a transcript that
// silently repaired itself would assert only that the code agrees with itself,
// which is the failure mode golden files exist to prevent.
//
//	go test ./cmd/rig/ -run TestTheBinary -update
var updateGolden = flag.Bool("update", false,
	"rewrite the golden transcripts in testdata/exec from what the binary printed")

// rigBin is the binary under test, built by TestMain into a temporary
// directory that does not exist before the run and does not survive it.
var rigBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rigexec")
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec test: no temp dir: %v\n", err)
		os.Exit(1)
	}
	rigBin = filepath.Join(dir, "rig")

	// A build failure must stop the run LOUDLY. A run that greps output for
	// failures scores a build failure as zero failures, which is the reassuring
	// direction and the expensive one.
	build := exec.Command("go", "build", "-o", rigBin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "exec test: the binary under test did not build: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// transcript is one invocation and the file that records what it printed.
type transcript struct {
	// name is the golden file's basename and the subtest's name.
	name string
	argv []string

	// stdoutTo sends the process's stdout to a path instead of capturing it.
	// /dev/full is the only value that matters and it is the reason the field
	// exists: writes to it fail with ENOSPC without the writer closing, which
	// is the one way to reach report's fallback - the branch that prints prose
	// on stderr when the JSON encode fails. In process that branch is
	// unreachable, because a bytes.Buffer does not fail.
	stdoutTo string
}

// record runs the binary with an estate of its own and renders everything an
// observer of the process could see: the exit code and both streams, in one
// document, because the whole point is that the three are chosen together.
func (tc transcript) record(t *testing.T) string {
	t.Helper()

	// An estate of its own, so a peer's running daemon cannot change what this
	// prints and this cannot reach one. No rigd is ever started: every case
	// below is either answerable without a daemon or is the refusal that says
	// there is none, and those refusals are the failure transcripts section 20
	// asks for.
	runtime := t.TempDir()

	cmd := exec.Command(rigBin, tc.argv...)
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtime)
	var stdout, stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if tc.stdoutTo != "" {
		f, err := os.OpenFile(tc.stdoutTo, os.O_WRONLY, 0)
		if err != nil {
			// Skipping rather than failing, and saying which case is not being
			// made: /dev/full is a Linux device and nothing in this repository
			// can conjure one. Same treatment as the fish completion.
			t.Skipf("%s is not available here, so report's fallback branch is "+
				"not exercised by this run: %v", tc.stdoutTo, err)
		}
		defer f.Close()
		cmd.Stdout = f
	}

	exit := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			// Not "the command failed" - the command never ran. A missing or
			// unexecutable binary must not be reported as a transcript
			// mismatch, because that reads as the code being wrong.
			t.Fatalf("rig did not run at all (%v). This test proved nothing.", err)
		}
		exit = ee.ExitCode()
	}

	return render(tc.argv, tc.stdoutTo, exit,
		scrub(stdout.String(), runtime),
		scrub(stderr.String(), runtime))
}

// scrub removes the one thing that legitimately differs between runs. The
// runtime directory is a fresh temporary path and it appears inside the dial
// error, so a transcript holding it could never match twice.
func scrub(s, runtime string) string {
	return strings.ReplaceAll(s, runtime, "$RUNTIME")
}

// render is the transcript format. Both streams are always present, empty
// included, for the reason answerJSON gives for `partial`: an absent section
// and a section that had nothing in it are different facts, and a format that
// omits the empty one cannot tell them apart.
func render(argv []string, stdoutTo string, exit int, stdout, stderr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "$ rig %s", strings.Join(argv, " "))
	if stdoutTo != "" {
		fmt.Fprintf(&b, " > %s", stdoutTo)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "exit %d\n", exit)
	if stdoutTo != "" {
		// Not captured, and saying so rather than printing "(0 bytes)", which
		// would read as the process having printed nothing.
		fmt.Fprintf(&b, "--- stdout redirected to %s, not captured\n", stdoutTo)
	} else {
		fmt.Fprintf(&b, "--- stdout (%d bytes)\n%s", len(stdout), stdout)
		if stdout != "" && !strings.HasSuffix(stdout, "\n") {
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "--- stderr (%d bytes)\n%s", len(stderr), stderr)
	if stderr != "" && !strings.HasSuffix(stderr, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "exec", name+".golden")
}

// TestTheBinary is section 20's "golden transcripts for every command including
// failures". Every verb run() dispatches appears here, and so does the way each
// one fails.
func TestTheBinary(t *testing.T) {
	for _, tc := range []transcript{
		// Answerable with no daemon at all.
		{name: "version", argv: []string{"version"}},
		{name: "version-json", argv: []string{"version", "--json"}},
		{name: "help-long", argv: []string{"--help"}},
		{name: "help-short", argv: []string{"-h"}},
		{name: "help-verb", argv: []string{"help"}},
		{name: "completion-bash", argv: []string{"completion", "bash"}},
		{name: "completion-zsh", argv: []string{"completion", "zsh"}},
		{name: "complete-apps", argv: []string{"__complete", "apps", "list"}},
		{name: "down-with-nothing-to-stop", argv: []string{"down"}},

		// Refused before anything is dialled: these are rig's own argument
		// handling, and every one of them is a failure transcript.
		{name: "no-command-at-all", argv: []string{}},
		{name: "not-a-verb", argv: []string{"--notaverb"}},
		{name: "completion-refuses-json", argv: []string{"completion", "bash", "--json"}},
		{name: "describe-refuses-json", argv: []string{"describe", "fakeapp", "--json"}},
		{name: "apps-refuses-an-unknown-depth", argv: []string{"apps", "list", "--depth", "wat"}},
		{name: "down-refuses-a-positional", argv: []string{"down", "somewhere"}},
		{name: "estate-refuses-a-positional", argv: []string{"estate", "somewhere"}},

		// The no-daemon refusal on every surface that dials. This is the set
		// that proves the channel and the exit code, because the object and the
		// prose are the same failure rendered two ways.
		{name: "apps-list", argv: []string{"apps", "list"}},
		{name: "apps-list-json", argv: []string{"apps", "list", "--json"}},
		{name: "apps-list-depth-full", argv: []string{"apps", "list", "--depth", "full"}},
		{name: "apps-list-depth-programs-json", argv: []string{"apps", "list", "--depth", "programs", "--json"}},
		{name: "ping", argv: []string{"ping", "fakeapp"}},
		{name: "ping-json", argv: []string{"--json", "ping", "fakeapp"}},
		{name: "estate", argv: []string{"estate"}},
		{name: "estate-json", argv: []string{"estate", "--json"}},
		{name: "describe", argv: []string{"describe", "fakeapp"}},
		{name: "declared-command", argv: []string{"fakeapp", "reindex", "--since", "7d"}},
		{name: "declared-command-json", argv: []string{"fakeapp", "reindex", "--since", "7d", "--json"}},

		// report's fallback: when the object cannot be written, prose goes to
		// stderr rather than the caller being told nothing. The comment on that
		// branch has always said so and nothing had ever run it - in process it
		// is unreachable, because the writer a test supplies does not fail.
		// These two are a PAIR and only mean something together: same argv, one
		// with a stdout that errors and one with a stdout that works.
		{
			name:     "json-falls-back-to-prose-when-stdout-fails",
			argv:     []string{"ping", "fakeapp", "--json"},
			stdoutTo: "/dev/full",
		},
		{name: "json-falls-back-control", argv: []string{"ping", "fakeapp", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.record(t)
			path := goldenPath(tc.name)

			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				// A missing transcript FAILS. It must never be created
				// silently by a plain run: a golden file that writes itself on
				// first sight asserts nothing, and the run that created it
				// would report green.
				t.Fatalf("no golden transcript at %s (%v).\n"+
					"If this case is new, record it deliberately:\n"+
					"    go test ./cmd/rig/ -run TestTheBinary -update", path, err)
			}
			if got != string(want) {
				t.Errorf("the process printed something else.\n"+
					"--- recorded (%s)\n%s\n--- printed now\n%s",
					path, want, got)
			}
		})
	}
}

// TestThisFileCanSeeWhatTheProcessPrinted is the control, and it exists
// because every other assertion in this file is a comparison: a test that
// captured nothing would compare "" against a golden holding "" and pass.
//
// Ask of any control what edit would make it fail. This one fails if the build
// in TestMain silently produced nothing runnable, if the streams are wired to
// the wrong writers, or if the exit code is being read off the wrong process -
// three ways this file could be broken while still reporting green.
func TestThisFileCanSeeWhatTheProcessPrinted(t *testing.T) {
	got := transcript{name: "control", argv: []string{"version"}}.record(t)

	if !strings.Contains(got, "--- stdout") || !strings.Contains(got, "--- stderr") {
		t.Fatalf("the transcript has no stream sections at all:\n%s", got)
	}
	if strings.Contains(got, "--- stdout (0 bytes)") {
		t.Errorf("`rig version` printed nothing on stdout, so this file is "+
			"not capturing output and every green above is meaningless:\n%s", got)
	}
	if !strings.Contains(got, "exit 0") {
		t.Errorf("`rig version` did not exit 0, so the exit code this file "+
			"reads is not the one the process returned:\n%s", got)
	}

	// And the failing direction, because a test that can only observe
	// success cannot observe the two mutations this file was written for.
	bad := transcript{name: "control", argv: []string{"--notaverb"}}.record(t)
	if strings.Contains(bad, "exit 0") {
		t.Errorf("a refused invocation reported exit 0, so a non-zero exit is "+
			"invisible here:\n%s", bad)
	}
	if strings.Contains(bad, "--- stderr (0 bytes)") {
		t.Errorf("a refused invocation printed nothing on stderr, so this "+
			"file cannot see the channel it exists to assert:\n%s", bad)
	}
}

// TestTheBinaryUnderTestIsTheOneJustBuilt pins the property the whole file rests on.
// The binary must come from the source in this tree on this run, never from
// build/rig, which is whatever the last `make build` left behind - possibly a
// peer's, possibly hours old. That is sizeratchet's stale-binary defect, and it
// is worth an assertion rather than a comment because the cheap way to write
// this file is to point it at build/rig and the result would look identical
// until the day it lied.
func TestTheBinaryUnderTestIsTheOneJustBuilt(t *testing.T) {
	if rigBin == "" {
		t.Fatal("TestMain did not build a binary")
	}
	if !strings.HasPrefix(rigBin, os.TempDir()) {
		t.Errorf("the binary under test is %s, which is not the freshly built "+
			"one in a temporary directory", rigBin)
	}
	if _, err := os.Stat(rigBin); err != nil {
		t.Errorf("the binary under test is not there: %v", err)
	}
	if abs, err := filepath.Abs(filepath.Join("..", "..", "build", "rig")); err == nil {
		if rigBin == abs {
			t.Error("these tests run build/rig, which is whatever the " +
				"last make build produced rather than this tree's source")
		}
	}
}
