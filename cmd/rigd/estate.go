package main

import (
	"fmt"
	"slices"
	"strings"
)

// estateNames is the closed set of names a rig estate may be started under.
//
// PLAN.md section 37: "A user runs AT MOST TWO NAMED ESTATES: production and
// development. Not one, and not an open namespace of N." Boris, 2026-09-11,
// ruling the shape: "I don't think we should allow more than one production
// and one development."
//
// THE SET IS CLOSED HERE, AT THE FLAG, AND DELIBERATELY NOT IN THE CLAIM
// MECHANISM. internal/instance and internal/paths are name-agnostic on purpose:
// what they enforce is section 37's precondition 6, "a named estate refuses a
// name already held", which is a DIFFERENT rule from the two-name set. Putting
// the set inside the lock would bury a policy where nobody reviews it as one,
// and would couple a mechanism that is correct for any name to a list that is
// a product decision and reversible by Boris.
//
// Section 37's ephemeral-estates clause is untouched by this: the rule binds
// NAMED estates, an estate started without --estate claims no name, and every
// test in this repository starts one of those.
var estateNames = []string{"production", "development"}

// checkEstateName refuses a name outside the closed set, naming what is
// permitted rather than only what is wrong.
//
// The empty name is not an error: it is an unnamed estate, which claims nothing
// and collides with nothing.
func checkEstateName(name string) error {
	if name == "" || slices.Contains(estateNames, name) {
		return nil
	}
	return fmt.Errorf(
		"estate %q is not a permitted name: rig runs at most two named estates, "+
			"%s (PLAN.md section 37). Start it without --estate for an unnamed "+
			"estate, which claims no name and collides with nothing",
		name, strings.Join(estateNames, " and "))
}
