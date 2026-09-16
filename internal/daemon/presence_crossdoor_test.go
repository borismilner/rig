package daemon

import (
	"testing"

	"github.com/boris-milner/rig/client"
)

// CONDITION A: AN OCCUPANT IS RELEASED WHEN ITS MCP CONNECTION DIES, AND THE
// SEAT IS SEEN TO GO FREE FROM THE OTHER DOOR.
//
// NOTHING ASSERTED THIS AND IT IS WHAT THE CUTOVER RESTS ON. Three cases cover
// the leak on the WIRE side - TestADroppedConnectionLeavesTheRoster,
// TestASeatKeepsItsNameAndCountsItsOccupants and
// TestOneSeatIsOneRowAcrossThreeSuccessiveSessions - measured by deleting the
// release from the wire handler's defer and watching exactly those three go
// red. Every one of them has the wire on BOTH sides. So a second door that
// allocates an occupancy and forgets to release it leaks occupants forever,
// the roster only grows, seats are never freed, and all three stay green
// because they are about the door that remembered.
//
// THE WATCHER IS ON THE OTHER DOOR DELIBERATELY. A case that announced and
// observed over MCP would prove the MCP door consistent with itself, which is
// not the claim. The claim is that both doors are ONE roster.
//
// THE DROP IS ABRUPT AND THE SESSION IS LEFT ALONE. `dialMCPConn` returns the
// raw conn beside the session; closing the CONN and never the SESSION is what
// the kernel does to a bridge that has just been SIGKILLed. That this is not
// DISTINGUISHABLE daemon-side is measured and is not the point - both endings
// return the same non-nil error from server.Run, because the transport IS the
// connection. The value is that the drop does not depend on the client library
// shutting down politely: a case that can only reach teardown by asking the
// SDK to behave well has made its instrument the thing it is declining to
// trust.
//
// ONE ARM, AND THAT IS A MEASURED DECISION RATHER THAN BREVITY. A second arm
// for a graceful close cannot go red - there is nothing for a door to branch
// on - so it would look like twice the evidence and be the same evidence. The
// leak shape this one arm catches is the realistic one: a release wired to the
// tidy path under an `err == nil` guard, which leaks on BOTH endings. The
// mutations are pre-registered in logbook/projects/rig/condition-a-red-cases.md.
//
// XDG_STATE_HOME IS SET DELIBERATELY AND IS NOT BOILERPLATE. `otherEstates`
// enumerates $XDG_STATE_HOME/rig/estates/*.pid for claims held by a LIVE
// daemon and skips its own name - but an unnamed estate has no name to skip,
// and every test in this repository runs unnamed. Left unset this reads the
// developer's real state directory, so it passes in CI and answers differently
// on a laptop with a second rigd up. This case does not assert `partial`, and
// that is exactly why the line is at risk of being tidied away by somebody who
// checks what is asserted rather than what is read.
func TestAnMCPOccupantIsReleasedWhenItsConnectionDies(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// ONE daemon, TWO doors: upMCP puts the MCP listener on the daemon
	// upDaemon already returns, so the two sockets share one presence.
	sock, d := upDaemon(t, nil)
	msock := upMCP(t, d)

	// The watcher is a WIRE client and announces first, so the roster is never
	// empty and a count of zero cannot be confused with a daemon that has not
	// started serving.
	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "watching the roster from the other door", "watching")

	ctx := ctx5(t)
	session, nc := dialMCPConn(ctx, t, msock, nil)

	// The agent takes a seat through the MCP door. `seat` is REQUIRED there -
	// that is condition C, and it is the one deliberate asymmetry with the
	// program socket, which keeps serving unseated Go programs.
	//
	// A SEAT NO WIRE CASE USES, deliberately. The wire-side roster cases all
	// take `backend-1`, so `awaitRows`'s seat argument had never been called
	// with a second value and `unparam` said so the moment this file arrived.
	// The linter was right: an argument every caller pins to one constant is
	// not exercised. This is the first case with a reason to differ - the
	// occupant arrives through the OTHER door - so it takes its own seat and
	// the parameter becomes load-bearing rather than decorative.
	callTool(ctx, t, session, "announce", map[string]any{
		"seat":     "backend-3",
		"purpose":  "an agent arriving through the door",
		"activity": "working",
	})

	// It is on the roster the OTHER door reads. If this fails, the two doors
	// are not one roster and nothing below it means anything.
	rows := awaitRows(t, watcher, "backend-3", 1)
	if got := rows[0].GetPurpose(); got != "an agent arriving through the door" {
		t.Fatalf("the wire door renders the MCP occupant's purpose as %q, want "+
			"%q. The two doors are serving different rows for one seat, so "+
			"they are not one roster", got, "an agent arriving through the door")
	}

	// THE DEATH. The conn goes and the session does not: no protocol shutdown,
	// no goodbye, nothing a door could have hung a tidy release on.
	if err := nc.Close(); err != nil {
		t.Fatalf("close the MCP connection: %v", err)
	}

	// And the seat is free, read from the other door. The wait is BOUNDED
	// because the release runs on the serving goroutine's defer: a single read
	// after the close races it and would flake rather than fail.
	awaitRows(t, watcher, "backend-3", 0)
}

// THE ACTIVITY AGE RULE THROUGH THE DOOR, READ FROM THE OTHER ONE.
//
// THE DOOR PROMISES THIS IN ITS OWN TOOL DESCRIPTION - "Re-sending an unchanged
// line deliberately does NOT reset its age, because repeating yourself is not
// progress" - and nothing tested it through the door. The four committed cases
// all call announce and setActivity DIRECTLY on the presence struct, so none of
// them goes through a door at all. The rule lives in one place, `setLine`, and
// nothing anywhere asserted that a DOOR reaches it rather than writing the
// fields itself or synthesising a line when the caller supplied none.
//
// TWO ARMS, AND THAT DOES NOT CONTRADICT CONDITION A'S ONE. The test is whether
// each assertion can be made to fail on its own: deleting setLine's equality
// guard reds the first arm and leaves the second green; making setLine a no-op
// reds the second and leaves the first green. Both are load-bearing. The rule
// was never "one arm" - it is that an arm which cannot independently go red is
// weight rather than evidence, and condition A had one such arm where this has
// none. Red cases pre-registered in logbook/projects/rig/age-rule-red-cases.md.
//
// NO FAKE CLOCK, AND NONE IS NEEDED. The committed cases inject `p.now` and
// move it a day; a door-level case runs through a real daemon on a real clock.
// The assertion is that two reads either side of a set_activity carrying the
// SAME line are byte-identical, and real time advances between two socket round
// trips - equality by accident would need both inside one nanosecond, which a
// unix round trip does not do.
//
// THE AGE IS READ FROM THE WIRE WATCHER, not from the door's own reply, for the
// same reason the release is: a door agreeing with itself is not the claim. It
// is also the only way a third party ever sees these fields, which is the gap
// B9 found - nothing in this repository read State, Generation, Purpose or
// Activity off somebody ELSE's row.
func TestTheActivityAgeRuleSurvivesTheDoor(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	sock, d := upDaemon(t, nil)
	msock := upMCP(t, d)

	watcher := dial(t, sock)
	announce(t, watcher, "team-lead", "watching the age from the other door", "watching")

	ctx := ctx5(t)
	session, _ := dialMCPConn(ctx, t, msock, nil)

	const line = "waiting for the gate to finish"
	callTool(ctx, t, session, "announce", map[string]any{
		"seat":     "backend-3",
		"purpose":  "an agent that will repeat itself",
		"activity": line,
	})
	began := ageOf(t, watcher, "backend-3")

	// ARM 1: THE SAME LINE AGAIN. This is the call the door's description makes
	// a promise about, and it is the one a session looping on one line makes.
	callTool(ctx, t, session, "set_activity", map[string]any{"activity": line})

	if got := ageOf(t, watcher, "backend-3"); got != began {
		t.Fatalf("re-sending an UNCHANGED line through the door moved its age "+
			"from %d to %d. The door's own description promises it does not, "+
			"and the age is the only thing on this roster that can say a "+
			"session is repeating itself - a board reading this cannot tell a "+
			"working session from one looping on one line, which is the "+
			"supervision property the cutover exists to keep rather than lose",
			began, got)
	}

	// ARM 2: A DIFFERENT LINE. Without this, a door that never moved the age at
	// all would pass arm 1 - and a frozen age is the same board defect wearing
	// the opposite coat.
	const moved = "gating the change in a detached worktree"
	callTool(ctx, t, session, "set_activity", map[string]any{"activity": moved})

	rows := seatRows(roster(t, watcher).GetCrew(), "backend-3")
	if len(rows) != 1 {
		t.Fatalf("seat backend-3 has %d rows, want 1", len(rows))
	}
	if got := rows[0].GetActivityUnixNano(); got <= began {
		t.Errorf("changing the line left its age at %d, want later than %d. A "+
			"door that never moves the age passes the arm above for the wrong "+
			"reason", got, began)
	}
	// DECISION 6's other half: a WRONG NON-EMPTY value. Asserting the line came
	// back EMPTY would assert nothing, because protojson omits the empty string
	// and absent reads identically to unserved. This asserts the exact bytes.
	if got := rows[0].GetActivity(); got != moved {
		t.Errorf("the watcher reads the line as %q, want %q. The door is "+
			"serving a line other than the one it was handed", got, moved)
	}
}

// ageOf reads one seat's activity age off the roster, through the given client.
func ageOf(t *testing.T, c *client.Client, seat string) int64 {
	t.Helper()
	rows := seatRows(roster(t, c).GetCrew(), seat)
	if len(rows) != 1 {
		t.Fatalf("seat %q has %d rows on the roster, want 1", seat, len(rows))
	}
	return rows[0].GetActivityUnixNano()
}
