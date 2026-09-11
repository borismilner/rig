package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// unitPath is the systemd --user unit, which lives in packaging/ rather than
// here: a cmd/ tree builds a binary and the unit is an artefact the build
// installs rather than compiles (PLAN.md section 5l, ruled 2026-09-12).
const unitPath = "../../packaging/rigd.service"

// systemdAnalyse is the verifier's real name. The house spelling is British
// and misspell enforces it, but this is a PROPER NOUN: the binary on disk is
// spelled with a z and renaming it is not available to us.
//
// THE SUPPRESSION IS ONE IDENTIFIER WIDE, on this line alone rather than on
// each use, and every message below reads from it rather than repeating the
// literal. Splitting the string to dodge misspell was tried first and is
// worse: it defeats the linter without recording that an exception was made,
// and nolintlint correctly rejected the now-unused directive that came with
// it. A suppression that does nothing is the thing to avoid, not the
// suppression.
const systemdAnalyse = "systemd-analyze" //nolint:misspell // the tool's actual name

func unitText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("the unit is missing: %v. Section 5l specifies it and "+
			"COORDINATION.md owns the path; if it moved, this test moves with it", err)
	}
	return string(b)
}

// directives returns the unit's actual settings with comments and blank lines
// dropped, so an assertion about what the unit DOES cannot be satisfied or
// broken by what the unit SAYS. The comment block explaining the missing
// ExecStop names ExecStop repeatedly, and a naive grep over the whole file
// would match its own explanation.
//
// ONLY A WHOLE-LINE COMMENT IS DROPPED, AND THAT IS MEASURED RATHER THAN
// ASSUMED. systemd has no inline comment: a trailing # on a directive is part
// of the VALUE. Checked against systemd 255 on 2026-09-12 -
// `Restart=on-failure # trailing` is reported as "Failed to parse service
// restart specifier, ignoring: on-failure # trailing", and on an ExecStart the
// # and the words after it simply become ARGUMENTS to the command, which is
// why that case passes verification while doing something nobody intended.
//
// So keeping the trailing text is correct rather than sloppy: this function
// reads a line the way systemd reads it. A future unit carrying a trailing
// comment is a BUG IN THE UNIT, and an assertion here firing on one is the
// right answer rather than a false positive.
func directives(t *testing.T, s string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// TestTheUnitCarriesNoExecStop is the enforcement section 5l asks for by name.
//
// Section 5l's own words: "Do not give it an ExecStop, and this is not a style
// note." The scar is recorded and was paid for on this machine: AgentBox's
// ExecStop was `agentbox quit`; agentbox is single-instance by flock and
// auto-spawns, so its start command exits 0 when one is already up, systemd
// decided the service had finished, ran ExecStop, and killed the healthy
// daemon. rig has the identical shape - section 5f's flock, section 5g's
// auto-spawn, section 18's SIGTERM drain.
//
// THE RULE IS NOT "NO SIGNAL TO A PID". A whole precondition was written
// against the wrong lesson because its author read section 5l quoted inside
// another section and never opened it: `rig down` is the same shape as
// `agentbox quit`, and `agentbox quit` was not a signal either. So this
// asserts on the DIRECTIVE, whatever value anybody gives it.
func TestTheUnitCarriesNoExecStop(t *testing.T) {
	for _, d := range directives(t, unitText(t)) {
		key, _, found := strings.Cut(d, "=")
		if !found {
			continue // a [Section] header
		}
		if strings.EqualFold(strings.TrimSpace(key), "ExecStop") {
			t.Fatalf("the unit carries %q. Section 5l forbids an ExecStop in "+
				"ANY form, including `rig down`: single-instance-by-flock plus "+
				"auto-spawn makes the start command exit 0, systemd concludes "+
				"the service finished, and the stop command kills the healthy "+
				"daemon that is already serving. That has happened on this "+
				"machine once already", d)
		}
	}
}

// TestTheUnitManagesProductionOnly pins section 37's precondition 5.
//
// production takes the DEFAULT XDG_RUNTIME_DIR and development is always
// placed explicitly, so a unit that sets neither reaches production and cannot
// reach development BY CONSTRUCTION rather than by a flag somebody might omit.
// Setting either here would hand the unit the development estate by accident,
// which is the one outcome precondition 5 is shaped to prevent.
func TestTheUnitManagesProductionOnly(t *testing.T) {
	for _, d := range directives(t, unitText(t)) {
		if strings.HasPrefix(d, "Environment") && strings.Contains(d, "XDG_RUNTIME_DIR") {
			t.Errorf("the unit sets XDG_RUNTIME_DIR (%q). Precondition 5 wants it "+
				"to inherit the ordinary environment, so that it reaches "+
				"production and cannot reach development", d)
		}
		if strings.HasPrefix(d, "ExecStart") && strings.Contains(d, "--estate") {
			t.Errorf("the unit passes --estate (%q). production is the DEFAULT "+
				"estate; naming one here is the flag precondition 5 exists to "+
				"avoid depending on", d)
		}
	}
}

// TestSystemdItselfAcceptsTheUnit is the real demonstration section 5l asks
// for, and it reads the OUTPUT rather than the exit code.
//
// MEASURED 2026-09-12, AND THIS IS WHY THE ASSERTION IS SHAPED LIKE THIS:
// THE VERIFIER EXITS 0 FOR EVERY DIRECTIVE-VALUE ERROR. Measured
// against systemd 255 on this machine, each of these exits 0 while printing
// one "ignoring" line:
//
//	Restart=on-nonsense      Failed to parse service restart specifier, ignoring
//	Type=bogus               Failed to parse service type, ignoring
//	RestartSec=not-a-time    Failed to parse sec value, ignoring
//	NoSuchKey=1              Unknown key name ... ignoring
//
// It exits non-zero only when the unit becomes structurally unloadable (no
// ExecStart at all) or names a command that does not exist. THE CONSEQUENCE IS
// NOT THEORETICAL: a typo in Restart= is ignored, the setting falls back to the
// default of no restart, and the daemon then does NOT come back after a crash -
// which is the entire reason section 5l specifies Restart=on-failure. An exit
// code alone would call that unit good.
//
// So the gate is exit 0 AND no output. A check that cannot tell "nothing is
// wrong" from "the check did not run" reads as a clean result.
func TestSystemdItselfAcceptsTheUnit(t *testing.T) {
	bin, err := exec.LookPath(systemdAnalyse)
	if err != nil {
		t.Skip(systemdAnalyse + " is not on this machine, so the unit's syntax " +
			"is unverified here. The assertions in the other tests in this " +
			"file are textual and ran regardless")
	}

	// ExecStart names an installed path that need not exist on a build
	// machine, and a missing command is the one thing verify does fail on.
	// Point it at a binary that certainly exists so the tool is checking the
	// UNIT rather than this machine's install state.
	src := unitText(t)
	var rewritten []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "ExecStart=") {
			line = "ExecStart=" + mustExist(t)
		}
		rewritten = append(rewritten, line)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rigd.service")
	if err := os.WriteFile(path, []byte(strings.Join(rewritten, "\n")), 0o600); err != nil {
		t.Fatalf("could not stage the unit for verification: %v", err)
	}

	out, err := exec.Command(bin, "--user", "verify", path).CombinedOutput()
	if err != nil {
		t.Fatalf("%s verify refused the unit: %v\n%s", systemdAnalyse, err, out)
	}
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("%s verify EXITED 0 AND STILL COMPLAINED, which is "+
			"how a typo'd directive passes: it is parsed, rejected, ignored, and "+
			"the setting silently falls back to its default. Output:\n%s", systemdAnalyse, out)
	}
}

// mustExist returns a path to a real executable, for the ExecStart rewrite.
func mustExist(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no stand-in executable to point ExecStart at: %v", err)
	}
	return p
}
