package kernel_test

import (
	"testing"
	"time"

	"github.com/boris-milner/rig/internal/kernel"
)

// introspecting is the principal every MCP connection gets: newPrincipal sets
// Introspect unconditionally and only registering as a program clears it, so
// an agent on the front door is always this.
func introspecting() kernel.Principal {
	p := agent()
	p.Introspect = true
	return p
}

// leaves registers shelf and then ends its connection, which is the only way a
// program departs: there is no goodbye on the wire.
func leaves(t *testing.T, k *kernel.Kernel) {
	t.Helper()
	const id = "shelf"
	p, err := k.Register(programPrincipal(id), good(id))
	if err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	k.Deregister(p.SessionID)
}

func TestAProgramThatLeftIsRememberedAsGoneRatherThanMissing(t *testing.T) {
	k := kernel.New()
	leaves(t, k)

	got, ok := k.See(introspecting()).Departed("shelf")
	if !ok {
		t.Fatal("a program registered and then left, and rig reports nothing " +
			"about it, so an agent cannot tell a crash it must react to from " +
			"an access boundary it must request through")
	}
	if got.ID != "shelf" {
		t.Errorf("the departure names %q, want shelf", got.ID)
	}
	if got.Reason != kernel.DepartureConnectionEnded {
		t.Errorf("the reason is %v; the wire carries no farewell, so the only "+
			"thing rig can honestly say is that the connection ended",
			got.Reason)
	}
	if got.At.IsZero() {
		t.Error("the departure carries no time, so an agent cannot tell a " +
			"program that left a second ago from one that left ten minutes ago")
	}
}

// TestATombstoneIsFilteredByTheScopeTheProgramHad is THE INVARIANT, and it is
// the reason this feature is safe to have at all.
//
// A tombstone is a projection and it inherits the dead program's scope. An
// unfiltered one turns the front door into an ENUMERATION ORACLE: register
// nothing, wait, and read off every program that ever ran in this estate.
// Section 14 already refuses to let "you may not see shelf" tell a caller that
// shelf exists, and a tombstone outside the filter undoes that in one step -
// while looking like a helpfulness feature.
func TestATombstoneIsFilteredByTheScopeTheProgramHad(t *testing.T) {
	k := kernel.New()
	leaves(t, k)

	if _, ok := k.See(agent("shelf")).Departed("shelf"); !ok {
		t.Error("a caller that could have seen shelf alive is not told it " +
			"departed, so the filter is refusing what it should allow")
	}

	if _, ok := k.See(agent("elsewhere")).Departed("shelf"); ok {
		t.Fatal("A CALLER THAT COULD NEVER HAVE SEEN SHELF IS TOLD SHELF " +
			"DEPARTED. That is an enumeration oracle: wait, ask, and read off " +
			"every program that ever ran here. Section 14 reports a program " +
			"you may not see as MISSING for exactly this reason")
	}

	if _, ok := k.See(agent()).Departed("shelf"); ok {
		t.Fatal("an unscoped caller is told shelf departed, so the tombstone " +
			"is reachable by anyone who can open a connection")
	}
}

func TestAProgramThatTookItsIDBackIsNotReportedGone(t *testing.T) {
	k := kernel.New()
	leaves(t, k)
	if _, err := k.Register(programPrincipal("shelf"), good("shelf")); err != nil {
		t.Fatalf("re-register: %v", err)
	}

	if _, ok := k.See(introspecting()).Departed("shelf"); ok {
		t.Fatal("shelf is registered and reachable right now, and rig says it " +
			"was here and left; an agent would stop calling a live program")
	}
}

func TestARolledBackRegistrationLeavesNoTombstone(t *testing.T) {
	k := kernel.New()
	p, err := k.Register(programPrincipal("shelf"), good("shelf"))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	// The daemon registers, finds another connection already holding the name,
	// and undoes it in the same breath. Nothing departed.
	k.Rollback(p.SessionID)

	if _, ok := k.See(introspecting()).Departed("shelf"); ok {
		t.Fatal("a registration that was rolled back before it was ever " +
			"reachable is reported as a departure, so rig states something " +
			"that did not happen; a tombstone that can be wrong is worse " +
			"than no tombstone")
	}
	if _, ok := k.See(introspecting()).Program("shelf"); ok {
		t.Error("the rollback did not remove the program")
	}
}

func TestADepartureStopsBeingReadableOnceTheWindowHasPassed(t *testing.T) {
	k := kernel.New()
	k.RememberDeparturesFor(time.Millisecond)
	leaves(t, k)

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, ok := k.See(introspecting()).Departed("shelf")
		if !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the window passed and the departure is still readable, " +
				"so departures accumulate for the life of the daemon")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestTheWindowIsReadableSoAnAnswerCanCarryIt is more of the clause-2 fix than
// the record is.
//
// Without the window in the answer, absence is ambiguous all over again: no
// tombstone means either nothing ever left under that id or the tombstone
// expired, and the agent is back where it started. "rig remembers departures
// for the last N" is what makes silence past N interpretable rather than
// evidence.
func TestTheWindowIsReadableSoAnAnswerCanCarryIt(t *testing.T) {
	k := kernel.New()
	if got := k.See(introspecting()).Remember(); got != kernel.DefaultRemember {
		t.Errorf("the window reads as %v, want the default %v",
			got, kernel.DefaultRemember)
	}

	k.RememberDeparturesFor(90 * time.Second)
	if got := k.See(introspecting()).Remember(); got != 90*time.Second {
		t.Errorf("the window was set to 90s and reads as %v, so an answer "+
			"quoting it would quote the wrong number", got)
	}
}

// TestAnEstateWhereNothingLeavesRemembersNothing is the cost argument, stated
// as behaviour rather than as a comment: a feature that does nothing in the
// steady state must cost nothing in it.
func TestAnEstateWhereNothingLeavesRemembersNothing(t *testing.T) {
	k := kernel.New()
	for _, id := range []string{"shelf", "pilot", "docket"} {
		if _, err := k.Register(programPrincipal(id), good(id)); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	v := k.See(introspecting())
	for _, id := range []string{"shelf", "pilot", "docket", "never-existed"} {
		if _, ok := v.Departed(id); ok {
			t.Errorf("nothing has left this estate and %q is reported as "+
				"departed", id)
		}
	}
}
