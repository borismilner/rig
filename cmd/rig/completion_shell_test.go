package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The completion scripts are the only shell code rig ships, and until this
// file they were checked by looking for substrings in them.
//
// That is not nothing - it catches a script that forgot to call back at all -
// but it cannot catch the thing that actually goes wrong in a completion
// script, which is the SLICE. Every one of the three has to hand
// `rig __complete` everything after `rig` and before the word under the
// cursor: one word too many and the candidates are for the wrong position,
// one too few and they are for the previous one. Both mistakes leave
// "rig __complete" in the source and both are invisible to a substring test.
//
// So these run the real script in the real shell, with a stub `rig` on PATH
// that records the argv it was handed. No daemon, no network, and the shell
// is skipped rather than failed when it is not installed.

// stubRig writes a fake `rig` into its own directory and returns the
// directory and the file the stub records its arguments in.
func stubRig(t *testing.T) (dir, argsFile string) {
	t.Helper()
	dir = t.TempDir()
	argsFile = filepath.Join(dir, "argv")
	stub := "#!/bin/sh\nprintf '%s' \"$*\" > \"" + argsFile + "\"\n" +
		"printf 'ping\\npurge\\nreindex\\n'\n"
	path := filepath.Join(dir, "rig")
	if err := os.WriteFile(path, []byte(stub), 0o755); err != nil {
		t.Fatalf("writing the stub: %v", err)
	}
	return dir, argsFile
}

// runShell runs one snippet with only the stub directory on PATH.
func runShell(t *testing.T, shell, dir, snippet string) string {
	t.Helper()
	cmd := exec.Command(shell, "-c", snippet)
	cmd.Env = append(os.Environ(), "PATH="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", shell, err, out)
	}
	return string(out)
}

func shellOrSkip(t *testing.T, shell string) {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s is not installed here, so the %s script is UNVERIFIED "+
			"on this machine", shell, shell)
	}
}

// Bash, with the word under the cursor empty: the callback must receive the
// words before it and NOT the empty one.
func TestTheBashCompletionScriptHandsBackTheRightWords(t *testing.T) {
	shellOrSkip(t, "bash")
	dir, argsFile := stubRig(t)

	out := runShell(t, "bash", dir, completionScripts["bash"]+`
COMP_WORDS=(rig fakeapp "")
COMP_CWORD=2
_rig_complete
printf '%s\n' "${COMPREPLY[@]}"`)

	got := readArgs(t, argsFile)
	if got != "__complete fakeapp" {
		t.Errorf("the callback got %q, want %q", got, "__complete fakeapp")
	}
	// And the candidates have to come back, which is the half the recipe in
	// the handoff never checked: an empty COMPREPLY prints a blank line and
	// reads exactly like success.
	for _, want := range []string{"ping", "purge", "reindex"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not in the completions:\n%q", want, out)
		}
	}
}

// Bash again, with a partial word. compgen has to filter on it, and the
// callback must still not be given it - candidates are for the position, and
// the shell does the filtering.
func TestTheBashScriptFiltersOnThePartialWordWithoutSendingIt(t *testing.T) {
	shellOrSkip(t, "bash")
	dir, argsFile := stubRig(t)

	out := runShell(t, "bash", dir, completionScripts["bash"]+`
COMP_WORDS=(rig fakeapp "re")
COMP_CWORD=2
_rig_complete
printf '%s\n' "${COMPREPLY[@]}"`)

	if got := readArgs(t, argsFile); got != "__complete fakeapp" {
		t.Errorf("the partial word reached the callback: %q", got)
	}
	if !strings.Contains(out, "reindex") {
		t.Errorf("reindex was filtered out:\n%q", out)
	}
	for _, gone := range []string{"ping", "purge"} {
		if strings.Contains(out, gone) {
			t.Errorf("%q survived filtering on \"re\":\n%q", gone, out)
		}
	}
}

// zsh slices with ${words[2,CURRENT-1]}, which is one-indexed and inclusive -
// a different expression from bash's for the same contract, so it is a
// different chance to be off by one.
func TestTheZshCompletionScriptHandsBackTheRightWords(t *testing.T) {
	shellOrSkip(t, "zsh")
	dir, argsFile := stubRig(t)

	// compadd only exists inside a real completion, so it is stubbed to print
	// what it was given. compdef likewise: the script's last line registers
	// the function and there is nothing to register against here.
	out := runShell(t, "zsh", dir, `
compdef() { : }
compadd() { while [[ $1 == -* ]]; do shift; done; print -l -- "$@" }
`+completionScripts["zsh"]+`
words=(rig fakeapp "")
CURRENT=3
_rig`)

	if got := readArgs(t, argsFile); got != "__complete fakeapp" {
		t.Errorf("the callback got %q, want %q", got, "__complete fakeapp")
	}
	for _, want := range []string{"ping", "purge", "reindex"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not in the completions:\n%q", want, out)
		}
	}
}

// The same slice with a NON-empty word under the cursor, and it is a separate
// test rather than a case because of what it catches.
//
// zsh drops empty elements from an unquoted array expansion, so a script that
// slices one word too far sends the identical argv when the cursor is on an
// empty word - the mutation is invisible and the test above passes. A partial
// word is the only shape that can see it.
func TestTheZshScriptDoesNotSendThePartialWord(t *testing.T) {
	shellOrSkip(t, "zsh")
	dir, argsFile := stubRig(t)

	runShell(t, "zsh", dir, `
compdef() { : }
compadd() { while [[ $1 == -* ]]; do shift; done; print -l -- "$@" }
`+completionScripts["zsh"]+`
words=(rig fakeapp "re")
CURRENT=3
_rig`)

	if got := readArgs(t, argsFile); got != "__complete fakeapp" {
		t.Errorf("the partial word reached the callback: %q", got)
	}
}

// fish drops the command itself from `commandline -opc`, and the word under
// the cursor is not in that list at all - a third expression for the same
// contract.
func TestTheFishCompletionScriptHandsBackTheRightWords(t *testing.T) {
	shellOrSkip(t, "fish")
	dir, argsFile := stubRig(t)

	out := runShell(t, "fish", dir, `
function commandline; printf '%s\n' rig fakeapp; end
`+completionScripts["fish"]+`
__rig_complete`)

	if got := readArgs(t, argsFile); got != "__complete fakeapp" {
		t.Errorf("the callback got %q, want %q", got, "__complete fakeapp")
	}
	if !strings.Contains(out, "reindex") {
		t.Errorf("the candidates did not come back:\n%q", out)
	}
}

func readArgs(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the script never called rig at all: %v", err)
	}
	return strings.TrimSpace(string(b))
}
