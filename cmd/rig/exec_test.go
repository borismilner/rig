package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	// Private state and runtime roots for the whole package, before any test
	// resolves either; isolation_test.go says what was measured.
	unisolate, err := isolateRoots()
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec test: %v\n", err)
		os.Exit(1)
	}
	dir, err := os.MkdirTemp("", "rigexec")
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec test: no temp dir: %v\n", err)
		unisolate()
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
	unisolate()
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
		// describe --json asks the daemon for its object, so with none it
		// is the no-daemon refusal, rendered as JSON because JSON was asked.
		{name: "describe-json-with-no-daemon", argv: []string{"describe", "fakeapp", "--json"}},

		// Refused before anything is dialled: these are rig's own argument
		// handling, and every one of them is a failure transcript.
		{name: "no-command-at-all", argv: []string{}},
		{name: "not-a-verb", argv: []string{"--notaverb"}},
		{name: "completion-refuses-json", argv: []string{"completion", "bash", "--json"}},
		{name: "apps-refuses-an-unknown-depth", argv: []string{"apps", "list", "--depth", "wat"}},
		{name: "down-refuses-a-positional", argv: []string{"down", "somewhere"}},
		{name: "estate-refuses-a-positional", argv: []string{"estate", "somewhere"}},
		{name: "knowledge-needs-a-subcommand", argv: []string{"knowledge"}},
		{name: "worknote-needs-a-subcommand", argv: []string{"worknote"}},
		{name: "message-needs-a-subcommand", argv: []string{"message"}},
		{name: "queue-needs-a-subcommand", argv: []string{"queue"}},
		{name: "notify-refuses-an-unknown-severity", argv: []string{"notify", "loud", "x"}},
		{name: "dnd-needs-on-off-or-status", argv: []string{"dnd", "maybe"}},

		// ⛔ SECTION 46's TWO VERBS, AND ALL FOUR OF THESE ARE REFUSED WITH
		// THE DISK UNTOUCHED. `rig restore` is the only verb in this table
		// that can WRITE to an estate, so its transcripts are deliberately
		// the ones that stop before the claim is taken and before any path is
		// resolved: this harness gives each case its own XDG_RUNTIME_DIR and
		// NOT its own XDG_STATE_HOME, and the record store resolves through
		// the second one.
		{name: "backup-refuses-a-positional", argv: []string{"backup", "/tmp/somewhere.tar.gz"}},
		{name: "restore-usage", argv: []string{"restore"}},
		{name: "restore-refuses-a-bad-estate-name", argv: []string{"restore", "--estate", "../../etc", "x.tar.gz"}},
		{name: "restore-refuses-two-archives", argv: []string{"restore", "--estate", "b", "one.tar.gz", "two.tar.gz"}},
		// B114: `b` is a lexically perfect name that no rigd will ever open,
		// and this transcript is the refusal a person now meets instead of a
		// successful restore that ends "start it with: rigd --estate b".
		{name: "restore-refuses-an-estate-no-daemon-opens", argv: []string{"restore", "--estate", "b", "x.tar.gz"}},

		// ⛔ SECTION 39's THREE VERBS, WHOSE ABSENCE THIS FILE'S OWN OPENING
		// SENTENCE DENIED. "Every verb run() dispatches appears here, and so
		// does the way each one fails" was true of TEN OF THIRTEEN: record,
		// progress, brief and mcp had no transcript at all, in the test file
		// whose job is to stop exactly that claim being made without a check.
		// All four are below, and so is `peers`, which nobody had noticed was
		// missing either - so the sentence is true again rather than weakened
		// to match what was there. Checked by grepping run()'s switch against
		// the argv in this table, not by reading down it.
		{name: "record-usage", argv: []string{"record"}},
		{name: "record-unknown-subcommand", argv: []string{"record", "stamp"}},
		{name: "record-put-needs-kind-and-project", argv: []string{"record", "put", "--project", "rig"}},
		// The --if-version guard, which is the one refusal in this surface that
		// a caller meets by forgetting something rather than by typing it.
		{name: "record-put-id-without-if-version", argv: []string{"record", "put", "--id", "01927-abc", "--kind", "note", "--project", "rig"}},
		{name: "record-link-usage", argv: []string{"record", "link", "a", "cites"}},
		{name: "record-refs-depth-zero", argv: []string{"record", "refs", "x", "--depth", "0"}},
		// --cross-project had no transcript at all, and it is the one record
		// flag whose NAME does not say what it costs: section 39 makes project
		// scoping a performance rule, so this asks the daemon for a wider walk.
		// The transcript pins that it PARSES and reaches the dial - a flag
		// refused at the parser and a flag the daemon never answers are the
		// same silence from a terminal.
		{name: "record-refs-cross-project", argv: []string{"record", "refs", "x", "--cross-project"}},
		{name: "record-duplicate-field", argv: []string{"record", "put", "--kind", "note", "--project", "rig", "--field", "a=1", "--field", "a=2"}},
		{name: "progress-usage", argv: []string{"progress", "step"}},
		{name: "progress-unknown-subcommand", argv: []string{"progress", "stamp"}},
		// ⛔ THE BRIEF LEFT rig AND THE ARM DID NOT (PLAN.md section 50,
		// move 5 and decision 4). The transcript is what proves the
		// difference: a verb that had been DELETED from run's switch would
		// fall through to the default branch and be answered as a missing
		// PROGRAM, which names neither the move nor what replaced it. One
		// row, because a refusal that ignores its arguments has one shape.
		{name: "brief-moved", argv: []string{"brief", "rig"}},

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
		// `rig backup` dials like `rig estate` does, and an unreachable
		// socket is not a form of the answer: nothing was archived, and
		// naming the directory rig WOULD have written to is the guess
		// decision 6 exists to remove.
		{name: "backup-no-daemon", argv: []string{"backup"}},
		{name: "describe", argv: []string{"describe", "fakeapp"}},
		{name: "declared-command", argv: []string{"fakeapp", "reindex", "--since", "7d"}},
		{name: "declared-command-json", argv: []string{"fakeapp", "reindex", "--since", "7d", "--json"}},

		// ⛔ THE RECORD SEAM'S NO-DAEMON REFUSAL, IN BOTH MODES, AND IT COULD
		// NOT HAVE BEEN RECORDED BEFORE THE WIRE LANDED. Until then
		// openRecordAPI returned notWired() - "rigd serves nothing to call" -
		// so these three transcripts would have held a refusal about a missing
		// wire rather than a missing daemon. They are what pins that the seam
		// now reports the daemon's own cause, and that the object and the prose
		// are the same failure rendered two ways.
		{name: "record-get-no-daemon", argv: []string{"record", "get", "01927-abc"}},
		{name: "record-get-no-daemon-json", argv: []string{"record", "get", "01927-abc", "--json"}},

		// ⛔ A FIFTH ABSENTEE, FOUND BY COUNTING RATHER THAN BY READING. The
		// four above were the ones a reader notices; `peers` was missing too,
		// and only `grep`ping run()'s own switch against this table found it.
		// That is the difference between believing a completeness claim and
		// checking one, which is this file's whole subject.
		{name: "peers-no-daemon", argv: []string{"peers"}},

		// THE FOURTH ABSENTEE. `rig mcp` is not a command to run by hand and
		// that is exactly why it needed a transcript: its stdout IS the MCP
		// stream, so the ONE thing a person ever sees from it is this refusal.
		// It is safe to record because it fails before it pumps - with nothing
		// listening it never reaches the byte loop, and stdin here is closed.
		{name: "mcp-no-daemon", argv: []string{"mcp"}},

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

// ⛔ EVERY VERB run() DISPATCHES HAS A TRANSCRIPT, AND THIS IS THE CHECK
// BEHIND THE SENTENCE AT THE TOP OF THIS FILE RATHER THAN THE SENTENCE ITSELF.
//
// That sentence - "Every verb run() dispatches appears here, and so does the
// way each one fails" - was TRUE OF TEN OF THIRTEEN when it was written down,
// and nothing anywhere could tell anyone. `record`, `progress`, `brief` and
// `mcp` were absent, and so was `peers`.
//
// ⛔ THE FIFTH ONE IS THE ARGUMENT FOR THIS TEST EXISTING. Four passes of
// reading found four; `peers` came out of a set difference and nothing else.
// AN ABSENCE IS THE ONE THING READING DOES NOT FIND - a missing row looks
// exactly like a covered one, which is COORDINATION.md's rule about its own
// ownership table arriving in a test file. So a completeness claim a human
// re-checks by reading is a claim that will be false again within a week.
//
// IT WALKS BOTH SIDES AND INVENTS NEITHER. `dispatchedVerbs` is
// refusal_verbs_test.go's, parsing run()'s own switch; the covered set is
// parsed out of the table above rather than listed here, because a hand-kept
// list of what is covered is a second source of truth that rots exactly like
// the comment it replaced. TestNoRefusalCitesAVerbRigDoesNotDispatch and
// briefRenderedSections are the same shape, and this makes three.
func TestEveryVerbRunDispatchesHasAGoldenTranscript(t *testing.T) {
	dispatched := dispatchedVerbs(t)
	covered := map[string]bool{}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "exec_test.go", nil, 0)
	if err != nil {
		t.Fatalf("exec_test.go did not parse, so this test proved NOTHING "+
			"and must not be read as a pass: %v", err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != "argv" {
			return true
		}
		lit, ok := kv.Value.(*ast.CompositeLit)
		if !ok || len(lit.Elts) == 0 {
			// `argv: []string{}` is the no-command-at-all case and covers no
			// verb. Not an error: it is a real transcript of a real failure.
			return true
		}
		first, ok := lit.Elts[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		if s, err := strconv.Unquote(first.Value); err == nil {
			covered[s] = true
		}
		return true
	})

	// ⛔ THE POSITIVE CONTROL, AND WITHOUT IT THIS WHOLE TEST IS AN ABSENCE
	// CHECKED AGAINST AN EMPTY SET. A parse that matched nothing, or a
	// `dispatchedVerbs` that answered about the wrong function, both produce a
	// clean pass below. Two verbs that certainly exist on both sides are named
	// so the instrument has to prove it ran before its result means anything.
	for _, control := range []string{"estate", "record"} {
		if !dispatched[control] {
			t.Fatalf("the dispatch walk did not find %q, so it answered about "+
				"something other than run() and every absence below is "+
				"meaningless. it found: %v", control, sortedKeys(dispatched))
		}
		if !covered[control] {
			t.Fatalf("the case-table walk did not find %q, so it parsed "+
				"something other than the transcripts and every absence below "+
				"is meaningless. it found: %v", control, sortedKeys(covered))
		}
	}

	for _, verb := range sortedKeys(dispatched) {
		if !covered[verb] {
			t.Errorf("run() dispatches %q and no transcript runs it. This "+
				"file's opening sentence claims every dispatched verb appears "+
				"here, and an absent row reads exactly like a covered one - "+
				"which is how `peers` stayed missing through four readings.",
				verb)
		}
	}
}
