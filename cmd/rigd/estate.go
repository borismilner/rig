package main

import (
	"fmt"

	"github.com/borismilner/rig/internal/estate"
)

// checkEstateName refuses a name outside section 37's closed set, naming what
// is permitted rather than only what is wrong.
//
// ⛔ THE SET ITSELF IS internal/estate's, AND IT MOVED THERE FOR A REASON
// (B114). It used to live in this file, at this flag, which was one enforcer
// for a rule with two entry points: `rig restore --estate <name>` checked only
// that the name was a safe path component, so an archive could be restored
// into an estate this binary would refuse to open. The package doc carries the
// full argument, including why the set is still not in internal/paths.
//
// What stays here is the remedy, because it is this binary's: a daemon may be
// started with no --estate at all. `rig restore` cannot, so it adds its own.
func checkEstateName(name string) error {
	if err := estate.CheckName(name); err != nil {
		return fmt.Errorf("%w. Start it without --estate for an unnamed "+
			"estate, which claims no name and collides with nothing", err)
	}
	return nil
}
