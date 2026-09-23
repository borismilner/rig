package estate

import (
	"slices"
	"strings"
	"testing"
)

// The set itself, asserted as a list rather than through CheckName. A test
// that only exercises the check cannot tell "development was removed" from
// "the check stopped working".
func TestTheSetIsExactlyTheTwoNamesSection37Allows(t *testing.T) {
	want := []string{"production", "development"}
	if got := Names(); !slices.Equal(got, want) {
		t.Fatalf("the permitted set is %v and section 37 allows %v", got, want)
	}
}

// Names hands out a copy, so a caller cannot edit the rule.
func TestNamesCannotBeEditedThroughWhatItReturns(t *testing.T) {
	got := Names()
	got[0] = "staging"
	got = append(got, "qa")
	_ = got

	if !Permitted("production") || Permitted("staging") || Permitted("qa") {
		t.Fatalf("editing the slice Names returned changed the set: %v", Names())
	}
}

func TestTheTwoNamedEstatesArePermitted(t *testing.T) {
	for _, name := range Names() {
		if !Permitted(name) {
			t.Errorf("Permitted(%q) is false for a name in the set", name)
		}
		if err := CheckName(name); err != nil {
			t.Errorf("%q was refused: %v", name, err)
		}
	}
}

// Section 37's ephemeral-estates clause, which is what keeps `make ci`
// working: the rule binds NAMED estates, and an unnamed one claims nothing.
func TestAnUnnamedEstateIsNotAPolicyViolation(t *testing.T) {
	if err := CheckName(""); err != nil {
		t.Fatalf("an unnamed estate was refused: %v\nevery test in this "+
			"repository starts one, and section 37 does not reach them", err)
	}
	// It is still not a MEMBER, and a caller asking that question directly -
	// to print the set, say - must not be told the empty name is one of two.
	if Permitted("") {
		t.Error("the empty name reports as a member of the set")
	}
}

func TestARefusalNamesTheOffenceTheSetAndTheSection(t *testing.T) {
	err := CheckName("b")
	if err == nil {
		t.Fatal("`b` was accepted; section 37 closes the set at two")
	}
	// Naming only the offence leaves the reader to guess the remedy, and the
	// remedy here IS the set: the two names that exist.
	for _, want := range []string{`"b"`, "production", "development", "section 37"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q, got: %v", want, err)
		}
	}
}

// The set is closed, so a near-miss is refused too. This is the case that
// would otherwise be discovered at 3am against a daemon that silently started
// a third estate under a typo - or, since B114, against an archive restored
// into an estate no daemon will open.
func TestNearMissesAreRefused(t *testing.T) {
	for _, name := range []string{
		"Production", "prod", "dev", "development2", " production", "production ",
	} {
		if err := CheckName(name); err == nil {
			t.Errorf("%q was accepted; the set is closed at production and development", name)
		}
	}
}
