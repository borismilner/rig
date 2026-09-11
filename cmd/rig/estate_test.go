package main

import (
	"strings"
	"testing"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// THE ONE THING THIS VERB EXISTS TO KEEP APART: the zero and UNNAMED.
//
// ESTATE_ROLE_UNSPECIFIED is 0 and means "nothing was said"; ESTATE_ROLE_UNNAMED
// is 1 and means "this estate deliberately claimed no name". proto3 cannot tell
// an unset scalar from a zero one, so a daemon that carried the field and never
// set it arrives here as the zero - and an unnamed estate reads as ephemeral and
// disposable where a production one does not. A renderer that collapses the two
// reintroduces exactly what the enum was shaped to prevent, in the last place
// anyone would look for it.
//
// Asserted on BOTH surfaces, because they are two renderings and only one of
// them was ever going to be checked by eye.
func TestTheUnsetRoleAndTheUnnamedRoleNeverRenderAsEachOther(t *testing.T) {
	unspecified := &rigv1.EstateResponse{
		Name: "somethingelse",
		Role: rigv1.EstateRole_ESTATE_ROLE_UNSPECIFIED,
	}
	unnamed := &rigv1.EstateResponse{
		Name: "",
		Role: rigv1.EstateRole_ESTATE_ROLE_UNNAMED,
	}

	if a, b := estateRoleCell(unspecified), estateRoleCell(unnamed); a == b {
		t.Fatalf("the unset role and the unnamed role render identically "+
			"to a human: %q", a)
	}
	if a, b := estateJSON(unspecified)["role"], estateJSON(unnamed)["role"]; a == b {
		t.Fatalf("the unset role and the unnamed role emit the same --json "+
			"token: %q", a)
	}

	// Collapsing is not the only way to get this wrong. The zero must not
	// read as a claim about the estate at all.
	cell := estateRoleCell(unspecified)
	if !strings.Contains(cell, "the daemon said nothing") {
		t.Errorf("the zero renders as %q, which states it as though it were "+
			"a fact about the estate. Section 21: the zero means nothing was "+
			"said", cell)
	}
	if strings.Contains(cell, "unnamed") {
		t.Errorf("the zero's rendering contains the word \"unnamed\": %q. That "+
			"is the reading the enum spent a number of its own to prevent", cell)
	}
}

// Every role the wire declares renders as its own distinct token, walked off
// the descriptor so a fifth role added to the proto is covered the day it
// lands rather than the day somebody remembers this test.
func TestEveryDeclaredRoleRendersAsSomethingOfItsOwn(t *testing.T) {
	values := rigv1.EstateRole(0).Descriptor().Values()
	seenJSON := map[string]string{}
	seenText := map[string]string{}

	var checked int
	for i := range values.Len() {
		v := values.Get(i)
		r := rigv1.EstateRole(v.Number())
		resp := &rigv1.EstateResponse{Role: r}
		checked++

		label, ok := roleLabel(r)
		if !ok {
			t.Errorf("%s is in the descriptor and roleLabel does not know it",
				v.Name())
			continue
		}
		if label == roleUnrecognised {
			t.Errorf("%s renders as the unrecognised token, which is reserved "+
				"for a role this build has no name for", v.Name())
		}

		j, _ := estateJSON(resp)["role"].(string)
		if prev, dup := seenJSON[j]; dup {
			t.Errorf("%s and %s both emit --json role %q", v.Name(), prev, j)
		}
		seenJSON[j] = string(v.Name())

		cell := estateRoleCell(resp)
		if prev, dup := seenText[cell]; dup {
			t.Errorf("%s and %s both render to a human as %q",
				v.Name(), prev, cell)
		}
		seenText[cell] = string(v.Name())
	}

	// The positive control. Every assertion above is inside the loop, so a
	// descriptor walk that returned nothing would report a clean run - which
	// is the failure this repository keeps catching in its own instruments.
	if checked < 4 {
		t.Fatalf("walked %d roles; the wire declares four and a walk that saw "+
			"fewer than that did not run", checked)
	}
}

// A ROLE NUMBER THIS BUILD DOES NOT KNOW IS SKEW, AND IT MUST NOT FALL BACK
// ONTO THE ZERO.
//
// Enums are extensible inside a wire major, so a newer daemon can answer with a
// fifth role and this binary is the older half. The generated String() returns
// the DECIMAL for such a value, so the obvious rendering prints a bare digit
// where every other answer is a word - and the dangerous rendering reports the
// skew as "nothing was said", which is a different and false statement.
func TestARoleThisBuildDoesNotKnowIsReportedAsSkewAndNotAsTheZero(t *testing.T) {
	future := &rigv1.EstateResponse{Role: rigv1.EstateRole(99)}

	if _, ok := roleLabel(rigv1.EstateRole(99)); ok {
		t.Fatal("roleLabel claims to know role 99, so this test proves nothing")
	}

	got, _ := estateJSON(future)["role"].(string)
	if got != roleUnrecognised {
		t.Errorf("--json emits role %q for a role this build has no name for; "+
			"want %q", got, roleUnrecognised)
	}
	if n := estateJSON(future)["role_number"]; n != int32(99) {
		t.Errorf("--json emits role_number %v, so a client that could not name "+
			"the role cannot say what it could not name either", n)
	}

	cell := estateRoleCell(future)
	if strings.Contains(cell, "nothing") {
		t.Errorf("a role this build does not know renders as %q, which reports "+
			"skew as the zero's meaning", cell)
	}
	if !strings.Contains(cell, "99") {
		t.Errorf("a role this build does not know renders as %q and never says "+
			"which number it was, which is the only actionable thing in it",
			cell)
	}
}

// AN EMPTY NAME IS THE ANSWER, NOT A MISSING ONE.
//
// An unnamed estate legitimately has no name, so the human rendering must not
// print a blank cell: a reader cannot tell an estate that claimed no name from
// a daemon that failed to send one, which is the same collapse as the role's
// and arrives through the other field.
func TestAnUnnamedEstateSaysSoRatherThanPrintingABlank(t *testing.T) {
	unnamed := &rigv1.EstateResponse{
		Role: rigv1.EstateRole_ESTATE_ROLE_UNNAMED,
	}
	if cell := estateNameCell(unnamed); strings.TrimSpace(cell) == "" {
		t.Fatal("an unnamed estate renders its name as a blank, which reads " +
			"as a field that failed rather than as the answer")
	}

	// It must also not be mistakable for a name. The name set is closed and
	// contains no parentheses, which is what makes the marker safe.
	if cell := estateNameCell(unnamed); !strings.HasPrefix(cell, "(") {
		t.Errorf("the empty-name marker is %q, which could be read as a name",
			cell)
	}
	if got := estateJSON(unnamed)["name"]; got != "" {
		t.Errorf("--json emits name %q for an unnamed estate; the object "+
			"carries the wire's value and the marker is a human rendering", got)
	}
}

// SECTION 10: the object's keys are the wire's own field names, and every one
// of them is present on every answer.
//
// A key that comes and goes teaches a consumer to treat absence as a value,
// which is the distinction the role enum spends its zero to keep. This is
// answerJSON's argument for `partial`, applied to a second object.
func TestTheEstateObjectCarriesEveryKeyOnEveryAnswer(t *testing.T) {
	want := []string{
		"name", "role", "role_number", "daemon_version", "wire", "semantics_gen",
	}

	for _, resp := range []*rigv1.EstateResponse{
		{}, // every field its zero: the shape must not thin out
		{
			Name: "production", Role: rigv1.EstateRole_ESTATE_ROLE_PRODUCTION,
			DaemonVersion: "0.1.0", Wire: "v1", SemanticsGen: 3,
		},
	} {
		obj := estateJSON(resp)
		for _, k := range want {
			if _, ok := obj[k]; !ok {
				t.Errorf("--json dropped %q from %v", k, obj)
			}
		}
		if len(obj) != len(want) {
			t.Errorf("--json emits %d keys, want %d: %v", len(obj), len(want), obj)
		}
	}
}

// ROLE AND NAME TRAVEL TOGETHER OR NEITHER DOES.
//
// The proto carries both on purpose even though role is derivable from name
// today: with only the name, `name == "production"` would live in every
// consumer outside this repository, and reopening the name set would break all
// of them silently. A --json object that emitted one without the other would
// put that derivation back in the consumer, which is the whole cost the wire
// paid a field to avoid.
func TestJSONNeverEmitsANameWithoutARoleOrARoleWithoutAName(t *testing.T) {
	for _, resp := range []*rigv1.EstateResponse{
		{Name: "production", Role: rigv1.EstateRole_ESTATE_ROLE_PRODUCTION},
		{Name: "", Role: rigv1.EstateRole_ESTATE_ROLE_UNNAMED},
		{},
	} {
		obj := estateJSON(resp)
		_, hasName := obj["name"]
		_, hasRole := obj["role"]
		if hasName != hasRole {
			t.Errorf("name present=%v, role present=%v for %v: a client that "+
				"gets one without the other has to derive the missing half",
				hasName, hasRole, resp)
		}
	}
}

// The human block prints every field, and the labels line up in one column.
// The alignment is read off the RENDERED lines rather than computed from
// estateColumn, because a test that derives its expectation from the constant
// it is testing cannot see that constant move.
func TestTheHumanBlockPrintsEveryFieldInOneColumn(t *testing.T) {
	out := estateText(&rigv1.EstateResponse{
		Name: "development", Role: rigv1.EstateRole_ESTATE_ROLE_DEVELOPMENT,
		DaemonVersion: "0.1.0", Wire: "v1", SemanticsGen: 3,
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("rendered %d lines, want 5:\n%s", len(lines), out)
	}

	// valueStart is the index the value begins at, measured on the rendered
	// line rather than derived from estateColumn.
	valueStart := func(l string) int {
		label, _, _ := strings.Cut(l, " ")
		rest := l[len(label):]
		return len(label) + len(rest) - len(strings.TrimLeft(rest, " "))
	}

	column := valueStart(lines[0])
	for _, l := range lines[1:] {
		if at := valueStart(l); at != column {
			t.Errorf("line %q starts its value at %d, the first line at %d",
				l, at, column)
		}
	}

	for _, want := range []string{"development", "0.1.0", "v1", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("the block does not carry %q:\n%s", want, out)
		}
	}
}

// The verb is reachable from argv, which is the thing fifteen unit tests could
// not see about --depth. It cannot reach a daemon here, so what is asserted is
// that argv PARSING refuses what it should and that a positional is not
// silently accepted - the rest needs a live estate and is demonstrated there.
func TestEstateRefusesAPositionalBeforeItDialsAnything(t *testing.T) {
	err := cmdEstate([]string{"production"})
	if err == nil {
		t.Fatal("rig estate production was accepted; the estate is the " +
			"runtime dir and there is nothing to name")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("refused with %q, which does not say how to call it", err)
	}
}
