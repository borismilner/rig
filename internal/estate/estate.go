// Package estate carries the closed set of names a rig estate may be started
// under, so that every binary refuses the same names.
//
// PLAN.md section 37: "A user runs AT MOST TWO NAMED ESTATES: production and
// development. Not one, and not an open namespace of N." Boris, 2026-09-11,
// ruling the shape: "I don't think we should allow more than one production
// and one development."
//
// ⛔ THE PACKAGE EXISTS BECAUSE THE RULE HAD ONE ENFORCER AND TWO ENTRY POINTS
// (B114). rigd checked the set at its --estate flag and `rig restore --estate
// <name>` checked only that the name was a safe path component, so a person
// could restore an archive into an estate no daemon would ever open - measured
// with two real binaries 2026-09-23: `rig restore --estate b` exited 0 and
// printed `rigd --estate b`, which exits 1.
//
// THE SET IS HERE AND DELIBERATELY NOT IN internal/paths OR THE CLAIM
// MECHANISM. internal/instance and internal/paths are name-agnostic on purpose:
// what they enforce is section 37's precondition 6, "a named estate refuses a
// name already held", which is a DIFFERENT rule from the two-name set. Putting
// the set inside the lock would bury a policy where nobody reviews it as one,
// and would couple a mechanism that is correct for any name to a list that is
// a product decision and reversible by Boris. There is now a second, harder
// reason: paths.ValidEstateName is also what validates a SEAT name
// (internal/daemon/meta.go, internal/daemon/presence_serve.go), so a set closed
// there would refuse every seat this estate has ever had.
//
// This package IS that policy, reviewed as one list. It depends on nothing but
// the standard library, which is what lets both cmd/rigd and cmd/rig read it
// without cmd/rig linking anything of the daemon's (cmd/rig/layering_test.go).
package estate

import (
	"fmt"
	"slices"
	"strings"
)

// names is section 37's closed set. It is an array rather than a slice so that
// no caller can append to it or rewrite an element through Names.
var names = [...]string{"production", "development"}

// Names is the permitted set, in the order a refusal should name them.
//
// It returns a fresh slice on every call: the set is a rule, and a rule a
// caller can edit in place is not one. A full slice expression over the array
// is NOT enough - it stops an append from reaching the array and lets an
// index assignment rewrite it, which is how the first draft of this function
// was caught by its own test.
func Names() []string {
	return slices.Clone(names[:])
}

// Permitted reports whether name is one of section 37's two.
//
// The empty name is NOT permitted here and is not an error either - see
// CheckName. An unnamed estate has no name to be in the set.
func Permitted(name string) bool {
	return slices.Contains(names[:], name)
}

// CheckName refuses a name outside the closed set, naming what is permitted
// rather than only what is wrong.
//
// The empty name is not an error: it is an unnamed estate, which claims no
// name and collides with nothing. Section 37's ephemeral-estates clause binds
// NAMED estates only, and every test in this repository starts an unnamed one.
// A caller that requires a name - `rig restore` does, because a restore writes
// over one particular estate - refuses the empty string itself, before this.
//
// CheckName does NOT check that the name is a safe path component; that is
// paths.ValidEstateName's job and the two are independent. Both hold.
func CheckName(name string) error {
	if name == "" || Permitted(name) {
		return nil
	}
	return fmt.Errorf(
		"estate %q is not a permitted name: rig runs at most two named "+
			"estates, %s (PLAN.md section 37)",
		name, strings.Join(Names(), " and "))
}
