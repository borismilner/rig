package main

import (
	"strings"
	"testing"
)

func TestTheTwoNamedEstatesArePermitted(t *testing.T) {
	for _, name := range []string{"production", "development"} {
		if err := checkEstateName(name); err != nil {
			t.Errorf("%q was refused: %v", name, err)
		}
	}
}

// Section 37's ephemeral-estates clause, which is what keeps `make ci` working:
// the rule binds NAMED estates, and an unnamed one claims nothing.
func TestAnUnnamedEstateIsNotAPolicyViolation(t *testing.T) {
	if err := checkEstateName(""); err != nil {
		t.Fatalf("an unnamed estate was refused: %v\nevery test in this "+
			"repository starts one, and section 37 does not reach them", err)
	}
}

func TestAThirdEstateNameIsRefusedAndTheRefusalSaysWhatIsAllowed(t *testing.T) {
	err := checkEstateName("staging")
	if err == nil {
		t.Fatal("a third estate name was accepted; section 37 closes the set at two")
	}
	// Naming only the offence leaves the reader to guess the remedy.
	for _, want := range []string{"staging", "production", "development", "section 37"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q, got: %v", want, err)
		}
	}
}

// The set is closed, so a near-miss is refused too. This is the case that would
// otherwise be discovered at 3am against a daemon that silently started a third
// estate under a typo.
func TestNearMissesAreRefused(t *testing.T) {
	for _, name := range []string{"Production", "prod", "dev", "development2", " production"} {
		if err := checkEstateName(name); err == nil {
			t.Errorf("%q was accepted; the set is closed at production and development", name)
		}
	}
}
