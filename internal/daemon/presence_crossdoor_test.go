package daemon

import (
	"testing"
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
