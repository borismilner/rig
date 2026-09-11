package main

import (
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// B15: NO SURFACE IN THIS PACKAGE MAY RENDER AN ENUM VALUE AS A BARE DIGIT.
//
// The generated String() returns the DECIMAL for a value outside the
// descriptor, and every label in this package used to go through it. A newer
// daemon's fifth effect therefore printed as "4" in a column of words, and an
// agent parsing --json got "4" where it expected a name.
//
// Enums are extensible WITHIN a wire major, so this is reachable: the major
// rides the proto package path, a client and daemon on different majors cannot
// connect at all, and "same major, newer daemon" is what is left. Section 37's
// precondition 3 wants that skew DETECTED, not discovered by a person squinting
// at a column.
//
// This test is a sweep rather than a list, because a list is the thing that
// goes stale: it walks the renderers and asserts the property, so a label added
// later is covered only if it is added here - which is what the source walk
// below is for.
func TestNoRendererEmitsABareDigitForAnEnumValueItCannotName(t *testing.T) {
	const unknown = 99

	for name, got := range map[string]string{
		"effects":  effectsLabel(&rigv1.Command{Effects: rigv1.Effects(unknown)}),
		"duration": durationLabel(&rigv1.Command{Duration: rigv1.Duration(unknown)}),
		"coverage": coverageLabel(&rigv1.Program{Coverage: rigv1.Coverage(unknown)}),
		"code":     codeWord(rigv1.Code(unknown)),
		"role":     mustSkew(t, rigv1.EstateRole(unknown)),
		"shape":    shapeWord(rigv1.Shape(unknown)),
		"tristate": tristateWord(rigv1.Tristate(unknown)),
	} {
		if _, err := strconv.Atoi(strings.TrimSpace(got)); err == nil {
			t.Errorf("%s renders an unknown value as the bare digit %q, which "+
				"is what String() returns and what B15 exists to remove",
				name, got)
		}
		if !strings.Contains(got, "unrecognised") {
			t.Errorf("%s renders an unknown value as %q, which does not say it "+
				"was not understood", name, got)
		}
		if !strings.Contains(got, strconv.Itoa(unknown)) {
			t.Errorf("%s renders an unknown value as %q and never names the "+
				"number, which is the only actionable thing in it", name, got)
		}
	}
}

func mustSkew(t *testing.T, r rigv1.EstateRole) string {
	t.Helper()
	s, _ := estateJSON(&rigv1.EstateResponse{Role: r})["role"].(string)
	return s
}

// ONE SPELLING, EVERYWHERE. Before B15 three functions in this package walked a
// descriptor and disagreed about how to say "I have no name for this": a bare
// digit in the shipped labels, a sentence in describe, and a bare word plus a
// separate number key in estate. Two of those three were written the same day.
func TestEverySurfaceUsesTheOneSkewSpelling(t *testing.T) {
	want := skewToken(rigv1.Effects(99))
	if !strings.HasPrefix(want, "unrecognised-") || !strings.HasSuffix(want, "99") {
		t.Fatalf("skewToken is %q, which is not the documented shape", want)
	}

	// Every renderer that has room for only a token emits exactly it.
	for name, got := range map[string]string{
		"effects":  effectsLabel(&rigv1.Command{Effects: rigv1.Effects(99)}),
		"duration": durationLabel(&rigv1.Command{Duration: rigv1.Duration(99)}),
		"coverage": coverageLabel(&rigv1.Program{Coverage: rigv1.Coverage(99)}),
		"code":     codeWord(rigv1.Code(99)),
		"role":     mustSkew(t, rigv1.EstateRole(99)),
	} {
		if got != want {
			t.Errorf("%s emits %q for an unknown value; every surface emits "+
				"%q, and two spellings is the drift B15 removed", name, got, want)
		}
	}

	// The ones with room for a sentence still START from the same token, so a
	// reader who has learned one form recognises the other.
	for name, got := range map[string]string{
		"describe shape":    shapeWord(rigv1.Shape(99)),
		"describe tristate": tristateWord(rigv1.Tristate(99)),
	} {
		if !strings.HasPrefix(got, want) {
			t.Errorf("%s renders %q, which does not begin with the token %q",
				name, got, want)
		}
	}
}

// A KNOWN VALUE MUST NEVER RENDER AS SKEW. Every assertion above is about
// absence, and a helper that returned the skew token unconditionally would
// satisfy all of them.
func TestAKnownValueNeverRendersAsSkew(t *testing.T) {
	var checked int
	for _, e := range []protoreflect.Enum{
		rigv1.Effects(0), rigv1.Duration(0), rigv1.Coverage(0),
		rigv1.Code(0), rigv1.EstateRole(0), rigv1.Shape(0),
		rigv1.Tristate(0), rigv1.Depth(0),
	} {
		values := e.Descriptor().Values()
		for i := range values.Len() {
			v := values.Get(i)
			checked++
			word, known := enumWord(shiftTo(e, v.Number()), "")
			if !known {
				t.Errorf("%s is in its own descriptor and enumWord does not "+
					"know it", v.Name())
				continue
			}
			if strings.Contains(word, "unrecognised") {
				t.Errorf("%s renders as skew", v.Name())
			}
		}
	}
	// The control: every assertion is inside two loops, so a walk that saw
	// nothing would read as a clean pass.
	if checked < 20 {
		t.Fatalf("walked %d enum values across eight enums; a walk that saw "+
			"fewer than twenty did not run", checked)
	}
}

// shiftTo returns the same enum type carrying another number, which is what
// lets one walk cover every enum rather than one per test.
func shiftTo(e protoreflect.Enum, n protoreflect.EnumNumber) protoreflect.Enum {
	switch e.(type) {
	case rigv1.Effects:
		return rigv1.Effects(n)
	case rigv1.Duration:
		return rigv1.Duration(n)
	case rigv1.Coverage:
		return rigv1.Coverage(n)
	case rigv1.Code:
		return rigv1.Code(n)
	case rigv1.EstateRole:
		return rigv1.EstateRole(n)
	case rigv1.Shape:
		return rigv1.Shape(n)
	case rigv1.Tristate:
		return rigv1.Tristate(n)
	case rigv1.Depth:
		return rigv1.Depth(n)
	}
	return e
}

// AND THE REFUSAL OBJECT ACTUALLY GOES THROUGH IT.
//
// A MUTATION PROVED THIS WAS NEEDED: reverting the call site in shape() to
// s.GetCode().String() survived, because every other assertion in this file
// calls codeWord directly. A helper being correct and a renderer using it are
// two claims, and only the second one is what an agent reads.
func TestTheRefusalObjectRendersAnUnknownCodeThroughTheSkewSpelling(t *testing.T) {
	err := &refusal{CallError: &client.CallError{
		Method: "fakeapp.reindex",
		Status: &rigv1.Status{Code: rigv1.Code(99), Message: "something"},
	}}
	obj, _, structured := shape(err)
	if !structured {
		t.Fatal("a daemon refusal did not produce a structured object")
	}
	if _, convErr := strconv.Atoi(obj.Code); convErr == nil {
		t.Errorf("the --json object carries code %q - a bare digit an agent "+
			"would match against a code vocabulary and miss", obj.Code)
	}
	if want := skewToken(rigv1.Code(99)); obj.Code != want {
		t.Errorf("the --json object carries code %q, want %q", obj.Code, want)
	}

	// The control: a KNOWN code must still arrive as its own wire name, or
	// this test would pass against a renderer that skewed everything.
	known := &refusal{CallError: &client.CallError{
		Status: &rigv1.Status{Code: rigv1.Code_CODE_DEADLINE},
	}}
	if got, _, _ := shape(known); got.Code != rigv1.Code_CODE_DEADLINE.String() {
		t.Errorf("a known code renders as %q, want its wire name", got.Code)
	}
}
