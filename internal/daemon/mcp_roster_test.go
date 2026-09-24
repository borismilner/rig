package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/mcpserver"
	"github.com/borismilner/rig/internal/meta"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// TestTheDoorRequiresASeatAndTheProgramSocketStillDoesNot is condition C, and
// it is ONE test because the two halves are one decision: the rule belongs to
// the door and must not have moved into presence.
//
// WHY BOTH HALVES. An unseated peer holds an empty seat at generation 0, which
// is not an identity, so an unseated AGENT would be unsupervisable - and the
// coordinator this cutover is porting from has no unseated case at all. But
// unseated is a real and ordinary case for a Go PROGRAM: cmd/fakeapp and the
// conformance suite are both unseated, and enforcing the rule in
// presence.announce would break every one of them to fix a problem only agents
// have. So the assertion that the program socket STILL accepts an unseated
// announce is not a courtesy - it is what proves the rule landed at the door
// rather than underneath it.
func TestTheDoorRequiresASeatAndTheProgramSocketStillDoesNot(t *testing.T) {
	sock, d := upDaemon(t, nil)
	ctx := ctx5(t)

	// THE DOOR REFUSES, and as a tool ERROR rather than a cheerful answer
	// carrying sad text. A successful answer IS the failure mode here: an
	// agent reading a success does not re-read the body.
	//
	// TWO WAYS TO ARRIVE UNSEATED AND BOTH ARE ASSERTED, because they are
	// refused by different things and only one of them is rig's own voice:
	//
	//	seat OMITTED  the declared schema requires the property, so the
	//	              protocol layer refuses before rig sees the call. Cheap,
	//	              and it is what a well-behaved host prevents entirely.
	//	seat EMPTY    the property is present and worthless, which the schema
	//	              cannot express. This is the one that reaches rig, and it
	//	              is the one that owes a fix an agent can act on.
	//
	// Asserting only the first would leave the second unreachable in practice
	// and untested in fact - a required property is not a non-empty one.
	agent := upRosterAgent(ctx, t, d)

	omitted := refusedTool(ctx, t, agent, "announce",
		map[string]any{"purpose": "porting my calls to rig"})
	if !strings.Contains(strings.ToLower(omitted["error"]), "seat") {
		t.Errorf("omitting the seat was refused without naming it: %v", omitted)
	}

	empty := refusedTool(ctx, t, agent, "announce",
		map[string]any{"seat": "", "purpose": "porting my calls to rig"})
	if !strings.Contains(empty["error"], "seat") {
		t.Errorf("the door refused an empty seat without saying the word "+
			"seat, so an agent cannot tell what to add: %v", empty)
	}
	if empty["fix"] == "" {
		t.Error("the empty-seat refusal carries no fix, so an agent is told " +
			"it is wrong and not what to do instead (section 9). This is the " +
			"one of the two refusals that is rig's own, and the only one " +
			"that can carry one")
	}

	// THE PROGRAM SOCKET STILL SERVES ONE. If this ever goes red, the rule has
	// migrated into presence and every unseated Go caller has been broken by a
	// change aimed at agents.
	var got rigv1.AnnounceResponse
	if err := dial(t, sock).Call(ctx, "rig.announce",
		&rigv1.AnnounceRequest{Purpose: "a program with no seat"}, &got); err != nil {
		t.Fatalf("the program socket refused an unseated announce, so the "+
			"door's rule has leaked into presence and every unseated Go "+
			"caller - cmd/fakeapp and the conformance suite among them - is "+
			"now broken: %v", err)
	}
	if got.GetYou().GetSeat() != "" {
		t.Errorf("an unseated program was given seat %q", got.GetYou().GetSeat())
	}
}

// TestTheDoorNamesTheEstatesItCannotSeeIntoAndNeverServesAPartialBoolean is
// condition B.
//
// THE NAME `partial` IS ALREADY TAKEN THREE TIMES, WHICH IS THE WHOLE PROBLEM.
// The coordinator being ported from serves a bool meaning "the roster cannot
// see everybody"; rig presence serves a bool meaning "another named estate
// exists, unseen"; and rig's own meta answers serve a LIST meaning "which
// programs reported incomplete coverage", on every answer including this one.
// Same word, and two of the three have different TYPES - `if (partial)` on an
// empty array is truthy in JavaScript and falsy in Python, so the same wrong
// port misbehaves differently per language.
//
// AND rig NEVER SERVES THE FIRST MEANING AT ALL. Within one estate the roster
// is complete by construction: one estate is one daemon and presence is
// connection state on that daemon, so there is no "sessions we cannot see"
// case. It would be a constant false, and a constant dressed as a live field
// is a lie by shape.
//
// So the door serves NAMES, and this test holds both halves: the names are
// live, and the word is absent.
//
// THE TWO MUTATIONS THIS IS WRITTEN AGAINST, because a repeated field owes
// them for the reason a string field does - an empty list and an unserved one
// would otherwise be the same bytes:
//
//	M-B1  serve otherEstates as always empty. Caught by the first assertion.
//	M-B2  serve otherEstates with a wrong non-empty value. Caught by the
//	      second, which names the estate rather than counting the list.
//
// Both were applied and watched fail before this test was committed.
//
// XDG_STATE_HOME IS NOT SET HERE AND THAT IS DELIBERATE, against a standing
// hazard that says any test touching this must set it. The hazard is real for
// a test that lets otherEstates enumerate the real state directory: an unnamed
// estate has no name to skip and every test in this repository runs unnamed.
// This test INJECTS the estate list instead, so it never reads that directory
// and cannot be perturbed by what is in it. A test here that does not inject
// must set XDG_STATE_HOME.
func TestTheDoorNamesTheEstatesItCannotSeeIntoAndNeverServesAPartialBoolean(t *testing.T) {
	_, d := upDaemon(t, nil)
	d.presence.others = func() []string { return []string{"production", "staging"} }
	ctx := ctx5(t)

	got := callTool(ctx, t, upRosterAgent(ctx, t, d), "list_agents", nil)
	crew, ok := got["crew"].(map[string]any)
	if !ok {
		t.Fatalf("list_agents answered with no crew object: %v", got)
	}

	names, _ := crew["otherEstates"].([]any)
	if len(names) != 2 {
		t.Fatalf("the door reported %d other estates and the daemon knows of "+
			"2, so the field is not reading the roster it claims to: %v",
			len(names), crew)
	}
	// NAMED, NOT COUNTED. A count goes green against a field serving the
	// wrong two names, which is exactly mutation M-B2.
	if names[0] != "production" || names[1] != "staging" {
		t.Errorf("the door names %v and the daemon knows of "+
			"[production staging]", names)
	}

	// THE WORD IS ABSENT FROM THE ROSTER OBJECT. Not "false", not "[]" -
	// absent, because a fourth meaning of this word on a nested object of the
	// same family is how a port goes wrong silently.
	if _, present := crew["partial"]; present {
		t.Error("the roster object carries a field called partial. There are " +
			"already three different things by that name across the two " +
			"coordinators a seat holds at once, two of them different types. " +
			"Serve otherEstates instead - it says the same thing and NAMES them")
	}

	// AND THE ANSWER'S OWN `partial` IS STILL THERE, one level up, because
	// section 5k forbids a surface implying completeness and this answer is
	// not exempt. The two must not be confused, which is why the roster's is
	// absent rather than renamed.
	if _, present := got["partial"]; !present {
		t.Error("the answer does not carry partial at all, so it implies a " +
			"completeness nobody checked (section 5k)")
	}
}

// TestSetActivityRefusesAConnectionThatHasNoRow is the reattach carrier, and
// the refusal is the whole point rather than an edge case.
//
// A seat whose daemon dies and whose host silently re-dials to a new one keeps
// working with NO SIGN ANYTHING HAPPENED: demonstrated end to end, and the
// seat that ran it reported that nothing in either response distinguished the
// two calls. That is the frozen-row failure arriving through the recovery
// path, and a row that looks supervised and is not is worse than an absent one.
//
// The daemon cannot tell a reattached seat from one that never announced - new
// connection, fresh token, no occupant, identical - and it does not need to,
// because BOTH NEED THE SAME ACTION. This fires on the call a seated agent
// makes most often, so one turn is the worst case rather than never.
func TestSetActivityRefusesAConnectionThatHasNoRow(t *testing.T) {
	_, d := upDaemon(t, nil)
	ctx := ctx5(t)

	refused := refusedTool(ctx, t, upRosterAgent(ctx, t, d), "set_activity",
		map[string]any{"activity": "reading the brief"})

	// IT MUST SEND THE SEAT TO announce. A refusal that reads as transient
	// gets retried, and retrying is the one thing that cannot work.
	if !strings.Contains(refused["error"]+refused["fix"], "announce") {
		t.Errorf("the refusal never says announce, so a seat is left to "+
			"retry the call that cannot succeed: %v", refused)
	}
	// AND IT MUST SAY WHICH DAEMON, which the wire's equivalent does not. A
	// seat reading only this line then knows it is talking to a different rig
	// than the one it announced to, without a second call.
	if !strings.Contains(refused["error"], "estate") {
		t.Errorf("the refusal does not name the estate, so a seat reading "+
			"only the error cannot tell WHICH daemon has no row for it: %v",
			refused)
	}
}

// TestTheRosterIsUnavailableRatherThanRefusedWithNothingUnderIt is the optional
// half of the interface doing its job.
//
// A surface built without a daemon under it is a real configuration and the
// CALLER DID NOTHING WRONG, so refusing would send it to fix the wrong thing.
// query already draws this line - it names the sources it cannot read rather
// than refusing a subject - and the roster is the same shape one tool along.
func TestTheRosterIsUnavailableRatherThanRefusedWithNothingUnderIt(t *testing.T) {
	ctx := ctx5(t)
	// meta.New with a nil invoker: no daemon, therefore no roster.
	server := mcpserver.New(meta.New(kernel.New(), nil),
		principalOfKind(kernel.KindAgent, "an-agent"), "test")
	session := connectTo(ctx, t, server)

	for _, tool := range []string{"announce", "set_activity", "list_agents"} {
		args := map[string]any{}
		switch tool {
		case "announce":
			args = map[string]any{"seat": "backend-1", "purpose": "p"}
		case "set_activity":
			args = map[string]any{"activity": "a"}
		}
		got := callTool(ctx, t, session, tool, args)
		if !hasString(got["unavailable"], "the estate's roster") {
			t.Errorf("%s with no daemon under it did not report the roster "+
				"as unavailable, so a caller cannot tell an unreachable "+
				"roster from an empty one: %v", tool, got)
		}
		if _, present := got["crew"]; present {
			t.Errorf("%s answered with a crew object it could not have "+
				"read, which makes an unreachable roster look like a quiet "+
				"one: %v", tool, got)
		}
	}
}

// TestARosterRowCarriesEverythingASupervisorReads locks the row's shape.
//
// EVERY FIELD HERE IS THE SUPERVISION CONTRACT AND NONE IS DECORATION, which
// is a fact this repository had to learn once already: nothing read State,
// Generation, Purpose or Activity off SOMEBODY ELSE'S row until a backlog item
// closed the gap, and HANDING_OFF exists solely for that third-party reader. A
// door serving thinner rows would regress that on a new surface while every
// wire test stayed green - the same shape as a leak, one floor up.
//
// THE TWO TIMESTAMPS ARE NOT OPTIONAL EITHER. An age is the only thing that
// tells a working session from a hung one, and the rule that an unchanged
// activity line does not reset its age is worthless at this door if the age is
// never served.
func TestARosterRowCarriesEverythingASupervisorReads(t *testing.T) {
	_, d := upDaemon(t, nil)
	ctx := ctx5(t)

	got := callTool(ctx, t, upRosterAgent(ctx, t, d), "announce", map[string]any{
		"seat":     "backend-1",
		"purpose":  "the MCP door",
		"activity": "writing the roster tools",
	})
	crew, _ := got["crew"].(map[string]any)
	you, ok := crew["you"].(map[string]any)
	if !ok {
		t.Fatalf("announce did not answer with the caller's own row: %v", got)
	}

	// Named one at a time rather than by reflection, because the assertion
	// worth having is that a HUMAN decided each of these belongs on the wire
	// an agent reads - and a reflective check over the struct would pass
	// itself by construction.
	for _, f := range []string{
		"seat", "generation", "epoch", "estate", "purpose", "activity",
		"state", "announcedUnixNano", "activityUnixNano",
	} {
		if _, present := you[f]; !present {
			t.Errorf("a roster row carries no %q. Every field on this row is "+
				"part of what a supervisor reads, and an absent one cannot "+
				"be told apart from a daemon too old to have it", f)
		}
	}

	if you["seat"] != "backend-1" || you["purpose"] != "the MCP door" {
		t.Errorf("the row does not carry what was announced: %v", you)
	}
	// STATE IS SERVED THOUGH IT CANNOT BE SET HERE, and that is coherent
	// rather than lopsided: the setter has no adopter in this cutover and the
	// FACT has one in every reader of the board.
	if you["state"] != "active" {
		t.Errorf("a freshly announced seat reads as state %v, not active", you["state"])
	}
	if n, _ := you["announcedUnixNano"].(float64); n == 0 {
		t.Error("the row carries no announced timestamp, so its age - the " +
			"only thing distinguishing a working session from a hung one - " +
			"cannot be computed by any reader")
	}
}

// upRosterAgent is one MCP connection with a roster under it.
//
// IT WRAPS THE DAEMON PER CONNECTION, exactly as serveMCPConn does, because an
// occupant's identity IS its connection. connectAs deliberately does not: it
// passes the bare daemon, which satisfies Invoker and EstateIdentity but not
// Roster, and that is the configuration the unavailable test above needs to
// stay reachable.
func upRosterAgent(ctx context.Context, t *testing.T, d *Daemon) *sdk.ClientSession {
	t.Helper()
	session, _ := upRosterAgentOcc(ctx, t, d)
	return session
}

// upRosterAgentOcc is upRosterAgent, plus the occupancy token underneath it,
// for a test that has to reach the row the way another surface would.
func upRosterAgentOcc(ctx context.Context, t *testing.T, d *Daemon) (*sdk.ClientSession, *occupancy) {
	t.Helper()
	who := principalOfKind(kernel.KindAgent, "an-agent")
	occ := &occupancy{}
	server := mcpserver.New(
		meta.New(d.kernel, &mcpCaller{Daemon: d, occ: occ}), who, "test")
	return connectTo(ctx, t, server), occ
}

// connectTo puts a client on an in-memory transport to this server.
func connectTo(ctx context.Context, t *testing.T, server *mcpserver.Server) *sdk.ClientSession {
	t.Helper()
	clientT, serverT := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("the MCP server would not serve: %v", err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "a-client", Version: "1"}, nil).
		Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("could not connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// refusedTool calls a tool that is expected to REFUSE, and returns the refusal
// as its fields.
//
// IT ASSERTS IsError, which is the half a test is most likely to skip. A
// refusal rendered as a successful answer carrying sad text is the failure
// mode itself: an agent reading a success does not re-read the body.
func refusedTool(ctx context.Context, t *testing.T, s *sdk.ClientSession,
	name string, args map[string]any,
) map[string]string {
	t.Helper()
	res, err := s.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call tool %s: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s ANSWERED rather than refusing. A refusal that is not "+
			"marked as one is read as a success by every caller that checks "+
			"the flag before the body", name)
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("the refusal is %T, not text", res.Content[0])
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		// NOT A FAILURE, BECAUSE TWO LAYERS CAN REFUSE AND ONLY ONE OF THEM IS
		// rig. A call rejected against the declared schema never reaches rig
		// at all, so it is refused in the protocol's words rather than in the
		// four structured fields. Both are refusals and a test that insisted
		// on rig's shape would be asserting that the schema layer does not
		// work.
		return map[string]string{"error": text.Text}
	}
	return out
}

// hasString reports whether a decoded JSON array contains this string.
func hasString(v any, want string) bool {
	items, _ := v.([]any)
	for _, it := range items {
		if s, _ := it.(string); s == want {
			return true
		}
	}
	return false
}

// TestSetActivityThroughTheDoorDoesNotOVERWRITETheState locks an ABSTENTION,
// which is a thing tests rarely do and this one has to.
//
// state is deliberately not an argument of set_activity at this door: it has
// no adopter in this cutover, and a field a porting caller never sets is a
// field nobody tests. But "not an argument" is only half of it. The door still
// has to pass SOMETHING to presence, and passing ACTIVE would be the door
// WRITING the field it claims not to carry - an assertion dressed as an
// abstention. UNSPECIFIED is what presence reads as "leave it alone".
//
// WHY IT MATTERS WHEN IT CANNOT BITE TODAY. An MCP occupant's state can only
// ever be ACTIVE right now, because a seat is one connection and an MCP
// connection is not a wire connection, so there is no HANDING_OFF at this door
// to overwrite. This test reaches past that by moving the row's state the way
// another surface would, which is the only way to express the failure before
// the argument exists. The day state IS added, a caller setting HANDING_OFF
// would have it silently reset by its own next activity line - and presence's
// own comment names that: a state that resets itself every time a peer says
// what it is doing is a state nobody can hold, and HANDING_OFF must survive
// several activity lines while a successor is briefed.
//
// MUTATION: pass SEAT_STATE_ACTIVE instead of UNSPECIFIED. Red, and the
// message says the state was overwritten rather than that a number differs.
//
// Raised by the presence owner reviewing the door. Kept as a test rather than
// as a comment on the risk, because a comment cannot go red.
func TestSetActivityThroughTheDoorDoesNotOverwriteTheState(t *testing.T) {
	_, d := upDaemon(t, nil)
	ctx := ctx5(t)
	session, occ := upRosterAgentOcc(ctx, t, d)

	callTool(ctx, t, session, "announce", map[string]any{
		"seat": "backend-1", "purpose": "the MCP door",
	})

	// The row goes into handing-off the way another surface would move it: a
	// state-only transition, empty line, which presence treats as leaving both
	// the line and its age alone.
	if _, ok := d.presence.setActivity(occ, "",
		rigv1.SeatState_SEAT_STATE_HANDING_OFF); !ok {
		t.Fatal("the row could not be moved into handing-off, so this test " +
			"cannot measure whether the door preserves it")
	}

	got := callTool(ctx, t, session, "set_activity",
		map[string]any{"activity": "briefing my successor"})
	crew, _ := got["crew"].(map[string]any)
	you, ok := crew["you"].(map[string]any)
	if !ok {
		t.Fatalf("set_activity did not answer with the caller's row: %v", got)
	}

	if you["state"] != "handing_off" {
		t.Errorf("a set_activity through the door reset the row's state to "+
			"%v. The door does not carry state as an argument, so it must "+
			"ABSTAIN rather than assert - pass UNSPECIFIED, which presence "+
			"reads as leave it alone. HANDING_OFF has to survive several "+
			"activity lines while a successor is briefed, and this is the "+
			"line that would quietly end it", you["state"])
	}
	// And the line it WAS asked to move did move, so the test is not passing
	// because the call did nothing at all.
	if you["activity"] != "briefing my successor" {
		t.Errorf("the activity line did not move: %v", you["activity"])
	}
}

// TestEveryToolAnswersYouTheSameWayInEveryStateACallerReaches is the guard
// against the class of defect that got past six mutations.
//
// ⛔ WHY A STATE TABLE RATHER THAN MORE ASSERTIONS. `list_agents` shipped
// serving `"you": null` to a SEATED caller - the door's own vocabulary for
// "you have no row" - and every one of this file's mutations stayed red-when-
// broken and green-when-fixed throughout, because not one of them was about a
// state no test entered. A mutation measures whether an assertion BITES. It
// says nothing about whether the assertions cover the states a caller REACHES,
// and this project had been reading the first as evidence of the second.
//
// So the axis this table varies is the CALLER'S STATE, not the assertion. Every
// tool is asked in every state, and `you` is checked against one rule that does
// not vary by tool:
//
//	`you` is THIS CONNECTION'S ROW when it has one, and null when it does not.
//	It never means "this tool does not answer that question".
//
// The defect was precisely a third meaning smuggled in as the second.
func TestEveryToolAnswersYouTheSameWayInEveryStateACallerReaches(t *testing.T) {
	ctx := ctx5(t)

	for _, c := range []struct {
		state   string
		seat    string // announced first, or empty for an unseated connection
		tool    string
		args    map[string]any
		wantYou bool
	}{
		{
			state: "unseated", tool: "list_agents", args: map[string]any{},
			wantYou: false,
		},
		{
			state: "seated", seat: "backend-1", tool: "list_agents",
			args: map[string]any{}, wantYou: true,
		},
		{
			state: "seated", seat: "backend-1", tool: "set_activity",
			args: map[string]any{"activity": "working"}, wantYou: true,
		},
		{
			state: "seated", seat: "backend-1", tool: "announce",
			args:    map[string]any{"seat": "backend-1", "purpose": "re-announced"},
			wantYou: true,
		},
		{
			// A connection MOVING seats keeps one row, so `you` follows it.
			state: "seated", seat: "backend-1", tool: "announce",
			args:    map[string]any{"seat": "backend-9", "purpose": "moved"},
			wantYou: true,
		},
	} {
		name := c.tool + "/" + c.state
		t.Run(name, func(t *testing.T) {
			_, d := upDaemon(t, nil)
			session, _ := upRosterAgentOcc(ctx, t, d)
			if c.seat != "" {
				callTool(ctx, t, session, "announce",
					map[string]any{"seat": c.seat, "purpose": "a seat"})
			}

			got := callTool(ctx, t, session, c.tool, c.args)
			crew, ok := got["crew"].(map[string]any)
			if !ok {
				t.Fatalf("%s answered with no crew object: %v", c.tool, got)
			}
			you, present := crew["you"]
			if !present {
				t.Fatalf("%s omits `you` entirely, so an absent key cannot be "+
					"told apart from a daemon too old to have it", c.tool)
			}

			row, isRow := you.(map[string]any)
			switch {
			case c.wantYou && !isRow:
				t.Fatalf("%s served `you: null` to a SEATED caller. That is "+
					"this door's own vocabulary for \"you have no row\", so a "+
					"seated seat is told in the surface's own words that it is "+
					"not on the roster - and list_agents is the one call a "+
					"confused seat makes after a reattach, which is the single "+
					"moment the answer matters most", c.tool)
			case !c.wantYou && isRow:
				t.Fatalf("%s served a row to an UNSEATED caller: %v", c.tool, row)
			}
			if !c.wantYou {
				return
			}
			// AND IT IS THIS CONNECTION'S ROW, not merely some row. A tool
			// returning the first entry of the roster would pass every check
			// above while being wrong for every caller but one.
			wantSeat, _ := c.args["seat"].(string)
			if wantSeat == "" {
				wantSeat = c.seat
			}
			if row["seat"] != wantSeat {
				t.Errorf("%s says this connection holds %v; it holds %q",
					c.tool, row["seat"], wantSeat)
			}
		})
	}
}
