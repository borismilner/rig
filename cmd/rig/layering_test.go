package main

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	// Test-only, and the import is the point.
	//
	// internal/meta is what the MCP server answers through, and section 10
	// binds this client's --json to what that server returns. So the two have
	// to be readable in one place, and a test is the only place they can be
	// without the client linking the package. This file importing it while
	// TestTheClientLinksNeitherTheDaemonNorItsValidator passes is itself the
	// demonstration that a test-only import costs the binary nothing.
	_ "github.com/boris-milner/rig/internal/meta"
)

// The client must not link the daemon's JSON Schema validator, and this is
// the test that stops it happening by accident.
//
// Section 17's budget line says the validator "has to be in the daemon rather
// than the client, because a program cannot trust a caller to have validated
// its own arguments (section 5e)". That sentence is a claim about what this
// binary links, and nothing was checking it: the layering was asserted in a
// handoff document off a manual `go list` run, which is prose.
//
// It is not a size rule, though the size is what makes it visible. Measured
// 2026-09-11 with make's own flags: importing internal/meta into this package
// takes `rig` from 6,119,687 to 7,586,055 bytes, because internal/meta
// depends on internal/kernel and the kernel links santhosh-tekuri/jsonschema
// and twelve golang.org/x/text packages. The bytes are the symptom. The
// contradiction with section 17 is the argument, and it would still be the
// argument if the validator were free.
//
// golang.org/x/text is listed separately rather than left to the jsonschema
// entry, because it arrives as a DIRECT dependency the moment anything here
// reaches for it and a direct dependency needs a section 22 row. That is a
// second, independent reason and it survives the validator being replaced.
func TestTheClientLinksNeitherTheDaemonNorItsValidator(t *testing.T) {
	// `.` rather than ./cmd/rig: a test runs in its own package directory, so
	// the relative path cannot rot if this package is ever moved.
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps . could not run, so this test proved NOTHING "+
			"and must not be read as a pass: %v\n%s", err, out)
	}
	deps := strings.Split(strings.TrimSpace(string(out)), "\n")

	// The positive control, and it is the difference between this test and a
	// test that cannot tell "nothing is wrong" from "the check did not run".
	//
	// Every forbidden-package assertion below is an ABSENCE, and an absence
	// is what an empty, truncated or misdirected `go list` also produces. So
	// the first thing asserted is a package this binary certainly does link.
	// Lose that and the whole file is a clean-looking result over nothing.
	const control = "github.com/boris-milner/rig/client"
	if !slices.Contains(deps, control) {
		t.Fatalf("the control package %q is not in `go list -deps .` output, "+
			"so the command answered about something other than this binary "+
			"and every absence below is meaningless. %d lines came back:\n%s",
			control, len(deps), out)
	}

	// Test-only imports are not package imports, and this is where that is
	// asserted rather than assumed. `go list -deps` without -test reports
	// what the PACKAGE imports, so the internal/meta import at the top of
	// this file is invisible here. If that ever stops being true, this
	// assertion fails and the ratchet row is the next thing to look at.
	for _, forbidden := range []struct{ prefix, why string }{
		{
			"github.com/boris-milner/rig/internal/meta",
			"section 10 binds the CLI's --json to what the MCP tool returns, " +
				"and this file binds the two in a TEST for exactly this reason: " +
				"the call would drag the kernel and its validator in behind it",
		},
		{
			"github.com/boris-milner/rig/internal/daemon",
			"the client is a client. TestTheClientOutlastsTheDaemonsOwnDeadline " +
				"reads the daemon's deadline in a test so the two numbers cannot " +
				"drift, and that is the only way this package may know it",
		},
		{
			"github.com/santhosh-tekuri/jsonschema",
			"section 17: the validator is the daemon's, because a program cannot " +
				"trust a caller to have validated its own arguments",
		},
		{
			"golang.org/x/text",
			"it arrives with the validator and it arrives DIRECT, which needs a " +
				"section 22 row for a message formatter rig does not otherwise use",
		},
	} {
		for _, d := range deps {
			if d == forbidden.prefix || strings.HasPrefix(d, forbidden.prefix+"/") {
				t.Errorf("cmd/rig links %s, and it must not: %s", d, forbidden.why)
			}
		}
	}
}
