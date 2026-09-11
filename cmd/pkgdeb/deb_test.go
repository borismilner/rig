package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTheLeadingVComesOffBecauseDpkgRefusesIt(t *testing.T) {
	got, err := debVersion("v0.0.0-m0-79-gfb330d8")
	if err != nil {
		t.Fatalf("a normal `git describe` output was refused: %v", err)
	}
	if got != "0.0.0-m0-79-gfb330d8" {
		t.Fatalf("version came out %q", got)
	}
}

func TestAVersionThatCannotStartWithADigitIsRefusedHere(t *testing.T) {
	// dpkg refuses these, and its message names the policy rather than the
	// tag, so the refusal is better made here where the tag is in hand.
	for _, bad := range []string{"", "dev", "vdev", "main-3"} {
		if _, err := debVersion(bad); err == nil {
			t.Errorf("%q was accepted as a Debian version", bad)
		}
	}
}

func TestAnArchitectureGoAndDebianSpellDifferently(t *testing.T) {
	if got, _ := debArch("amd64"); got != "amd64" {
		t.Errorf("amd64 became %q", got)
	}
	if got, _ := debArch("386"); got != "i386" {
		t.Errorf("386 became %q, and Debian calls it i386", got)
	}
	if got, _ := debArch("arm"); got != "armhf" {
		t.Errorf("arm became %q, and Debian calls it armhf", got)
	}
	if _, err := debArch("riscv64"); err == nil {
		t.Error("an architecture with no known Debian name was accepted, so " +
			"the package would claim an architecture it is not")
	}
}

// A literally empty line ends the control stanza, and dpkg then reports the
// rest of the description as a parse error somewhere else entirely.
func TestABlankDescriptionLineBecomesALoneFullStop(t *testing.T) {
	c := control{
		Package: "rig", Version: "1.0", Architecture: "amd64",
		Maintainer: "someone", Summary: "a summary",
		Description: "first\n\nsecond",
	}
	got := c.String()
	if !strings.Contains(got, "\n .\n") {
		t.Fatalf("a blank description line was not written as \" .\":\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if line == "" {
			continue // the trailing newline
		}
		if strings.HasPrefix(line, " ") || strings.Contains(line, ": ") {
			continue
		}
		t.Errorf("line %q is neither a field nor a continuation, so it ends "+
			"the stanza early", line)
	}
}

// The three members are read positionally by dpkg, so their order is part of
// the format.
func TestTheArchiveHasItsThreeMembersInOrder(t *testing.T) {
	body := buildForTest(t)

	if !bytes.HasPrefix(body, []byte("!<arch>\n")) {
		t.Fatal("no ar magic")
	}
	want := []string{"debian-binary", "control.tar.gz", "data.tar.gz"}
	var got []string
	for i := 8; i+60 <= len(body); {
		name := strings.TrimSpace(string(body[i : i+16]))
		var size int
		if _, err := fmt.Sscan(strings.TrimSpace(string(body[i+48:i+58])), &size); err != nil {
			t.Fatalf("member size at %d: %v", i, err)
		}
		got = append(got, name)
		i += 60 + size
		if size%2 == 1 {
			i++
		}
	}
	if len(got) != 3 {
		t.Fatalf("archive has %d members: %v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("member %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// Two builds of one commit must not differ, or nothing can be compared with a
// published artefact.
func TestThePackageIsByteStable(t *testing.T) {
	first := buildForTest(t)
	for i := range 3 {
		again := buildForTest(t)
		if !bytes.Equal(first, again) {
			t.Fatalf("build %d differs from the first", i+2)
		}
	}
}

// dpkg tracks directory ownership, so a data archive naming ./usr/bin/rig
// without naming ./usr/bin leaves the directory owned by no package.
func TestEveryAncestorDirectoryIsNamed(t *testing.T) {
	got := dirsOf([]file{{Path: "usr/bin/rig"}, {Path: "usr/share/doc/rig/x"}})
	for _, want := range []string{"usr", "usr/bin", "usr/share", "usr/share/doc", "usr/share/doc/rig"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is not named in the archive, so it would be unowned", want)
		}
	}
	for _, g := range got {
		if g == "usr/bin/rig" {
			t.Error("a FILE was listed as a directory")
		}
	}
}

func buildForTest(t *testing.T) []byte {
	t.Helper()
	c := control{
		Package: "rig", Version: "1.0", Architecture: "amd64",
		Maintainer: "someone", Summary: "s", Description: "d",
	}
	body, err := buildDeb(c, []file{
		{Path: "usr/bin/rig", Mode: 0o755, Body: []byte("binary one")},
		{Path: "usr/bin/rigd", Mode: 0o755, Body: []byte("binary two")},
	}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	return body
}
