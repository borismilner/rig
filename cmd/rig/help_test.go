package main

import (
	"strings"
	"testing"
)

func TestHelpIsGeneratedFromTheDeclaredSchema(t *testing.T) {
	flags, err := describeFlags(reindexDecl().GetArgs())
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	// One line per declared property, in a stable order, and nothing else.
	if len(flags) != 6 {
		t.Fatalf("got %d flags from a schema with 6 properties: %+v", len(flags), flags)
	}
	var usage []string
	for _, f := range flags {
		usage = append(usage, f.usage)
	}
	got := strings.Join(usage, " | ")
	for _, want := range []string{
		"--dry-run",              // a boolean shows no value, because it takes none
		"--either (--args only)", // two declared types has no flag form
		"--nested (--args only)", // an object has none either
		"--ratio <number>",
		"--since <string>",
		"--workers <integer>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q is missing from %q", want, got)
		}
	}
	// A boolean must not be shown as taking a value.
	if strings.Contains(got, "--dry-run <") {
		t.Fatalf("a boolean flag is shown as taking a value: %q", got)
	}
}

func TestRequiredAndBoundsComeFromTheSchemaNotFromProse(t *testing.T) {
	flags, err := describeFlags([]byte(`{
      "type":"object",
      "required":["since"],
      "properties":{
        "since":{"type":"string","description":"how far back"},
        "workers":{"type":"integer","minimum":1,"maximum":64},
        "mode":{"type":"string","enum":["fast","full"],"default":"fast"}
      }
    }`))
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	var b strings.Builder
	for _, f := range flags {
		b.WriteString(f.usage + " :: " + f.help + "\n")
	}
	joined := b.String()
	for _, want := range []string{
		"how far back (required)", // description plus the required list
		"(1 to 64)",               // the declared bounds
		"--mode fast|full",        // an enum is offered as its own values
		"default fast",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q is missing from:\n%s", want, joined)
		}
	}
}

func TestACommandWithNoSchemaHasNoFlags(t *testing.T) {
	flags, err := describeFlags(nil)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(flags) != 0 {
		t.Fatalf("a command with no schema produced %+v", flags)
	}
	if _, err := describeFlags([]byte(`{"properties":`)); err == nil {
		t.Fatal("an unreadable schema was described anyway")
	}
}

func TestAsksForHelpStopsAtADoubleDash(t *testing.T) {
	if !asksForHelp([]string{"--since", "7d", "--help"}) {
		t.Fatal("--help was not noticed")
	}
	if !asksForHelp([]string{"-h"}) {
		t.Fatal("-h was not noticed")
	}
	// Past `--`, --help is the command's argument, not rig's.
	if asksForHelp([]string{"--", "--help"}) {
		t.Fatal("--help after -- was taken as rig's own")
	}
	if asksForHelp([]string{"--since", "7d"}) {
		t.Fatal("help was assumed")
	}
}

func TestWrapDoesNotLoseOrSplitWords(t *testing.T) {
	in := "Walks everything changed in the window and rebuilds the index for it."
	out := wrap(in, 30)
	if strings.Join(strings.Fields(out), " ") != in {
		t.Fatalf("wrap changed the text:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 30 && !strings.Contains(line, " ") {
			t.Fatalf("a single word was split: %q", line)
		}
	}
}

func TestEveryShellRigOffersHasAScript(t *testing.T) {
	// The list in the completion candidates and the map of scripts have to
	// agree, or `rig completion <TAB>` offers a shell that then errors.
	for _, shell := range candidates([]string{"completion"}) {
		if _, ok := completionScripts[shell]; !ok {
			t.Fatalf("completion offers %q and has no script for it", shell)
		}
	}
	if len(completionScripts) != len(candidates([]string{"completion"})) {
		t.Fatalf("%d scripts, %d offered", len(completionScripts),
			len(candidates([]string{"completion"})))
	}
}

func TestTheCompletionScriptsCallBackTheWayCmdCompleteExpects(t *testing.T) {
	// Each script has to drop `rig` itself and the partial word, and call the
	// hidden verb. A script that passes the partial word would offer
	// candidates for the wrong position.
	for shell, script := range completionScripts {
		if !strings.Contains(script, "rig __complete") {
			t.Fatalf("the %s script does not call rig __complete", shell)
		}
		if !strings.Contains(script, "2>/dev/null") {
			t.Fatalf("the %s script does not silence failures, so a stopped "+
				"rigd would print into the command line", shell)
		}
	}
}

func TestStaticVerbsAreOfferedAtTheTopLevel(t *testing.T) {
	got := strings.Join(candidates(nil), " ")
	// The registry may be unreachable in a test environment, which is
	// exactly the case that must still offer rig's own verbs.
	for _, want := range staticVerbs {
		if !strings.Contains(got, want) {
			t.Fatalf("%q is not offered: %q", want, got)
		}
	}
}
