package main

import (
	"strings"
	"testing"
)

// ⛔ THE CASE THIS EXISTS FOR IS B89, AND IT IS THE FIRST ROW.
//
// On 2026-09-17 Boris ran `make install`, it reported success, and the window he
// was looking at stayed on a build ten commits and thirteen hours behind. Both
// halves were running; nothing anywhere said they disagreed. These are the exact
// two strings that were live on that machine.
func TestTheWindowAndTheDaemonCanDisagreeAndSaySo(t *testing.T) {
	const (
		daemonAfterInstall = "v0.0.0-m0-471-g8c4d195"
		windowLeftBehind   = "v0.0.0-m0-409-g57c99ca-dirty"
	)

	agree, verdict := deploymentVerdict(daemonAfterInstall, windowLeftBehind)
	if agree {
		t.Fatalf("B89's two real versions were reported as agreeing: %q vs %q",
			daemonAfterInstall, windowLeftBehind)
	}
	if verdict == "" {
		t.Fatal("a disagreement with no verdict is the silence B89 was made of")
	}
	// The verdict must name the remedy, because the remedy is the thing nobody
	// knew: `make install` deliberately does not install the window.
	if !strings.Contains(verdict, "make install-window") {
		t.Errorf("the verdict does not name the remedy, so it repeats B89's failure: %q", verdict)
	}
}

func TestTwoIdenticalBuildsAgree(t *testing.T) {
	const v = "v0.0.0-m0-471-g8c4d195"
	agree, verdict := deploymentVerdict(v, v)
	if !agree {
		t.Fatalf("identical versions reported as a skew: %q", verdict)
	}
}

// ⛔ "I CANNOT COMPARE" IS NOT "THEY DISAGREE", AND CONFLATING THEM WOULD SEND A
// PERSON TO REINSTALL A WINDOW THAT IS ALREADY RIGHT.
func TestADaemonThatDoesNotStampItselfIsNotReportedAsASkew(t *testing.T) {
	agree, verdict := deploymentVerdict("", "v0.0.0-m0-471-g8c4d195")
	if agree {
		t.Fatal("an unstamped daemon must not be reported as agreeing either")
	}
	if strings.Contains(verdict, "make install-window") {
		t.Errorf("an unknown version was answered with a reinstall instruction: %q", verdict)
	}
	if !strings.Contains(verdict, "cannot be compared") {
		t.Errorf("the verdict does not say the comparison could not be made: %q", verdict)
	}
}
