package daemon

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The daemon is what runs a command for the four meta tools. Asserted here so
// the interface and the implementation cannot drift apart silently: the
// signature that matters is the one carrying the principal, and a compile
// error is the only thing that notices it changing.
var _ meta.Invoker = (*Daemon)(nil)

// And the optional half of it, for the same reason: the daemon is the thing
// that knows which estate this is, and query is where an agent asks.
var _ meta.EstateIdentity = (*Daemon)(nil)

// EstateIdentity answers which estate this daemon serves.
//
// IT IS THE SAME ANSWER rig.estate GIVES ON THE WIRE, BUILT FROM THE SAME
// FIELDS, and that is a requirement rather than a convenience: two surfaces
// answering "which estate am I in" differently is worse than one surface not
// answering at all, because a wrong answer to this question is what routes an
// agent's write into the wrong estate. If a field is added to EstateResponse,
// it is added here in the same change.
//
// UNSCOPED, like rig.estate, and the reason is stated at that case in
// daemon.go: every field is a fact about this daemon and none is data
// belonging to another principal, so there is nothing to filter. That is why
// this takes no principal. A later field could quietly destroy it.
func (d *Daemon) EstateIdentity() meta.Estate {
	return meta.Estate{
		Name:          d.estate,
		Role:          roleWord(estateRole(d.estate)),
		DaemonVersion: d.version,
		Wire:          d.wire,
		SemanticsGen:  selfDeclaration().SemanticsGen,
		Epoch:         d.epoch,
	}
}

// roleWord is the wire's role enum as the word an agent reads.
//
// Derived FROM the enum rather than from the estate name a second time, so a
// role added to the proto arrives on the agent surface without a second table
// to remember. ESTATE_ROLE_PRODUCTION becomes "production".
func roleWord(r rigv1.EstateRole) string {
	return strings.ToLower(strings.TrimPrefix(r.String(), "ESTATE_ROLE_"))
}

// Invoke runs one declared command as one principal, and it is the whole of
// meta.Invoker.
//
// It goes down Daemon.call, which is the path the wire goes down, so a caller
// arriving through the MCP server meets section 13a's floor rather than a
// second copy of it. That is what call was extracted for: "a new surface
// SPLITS the existing path and reuses its floor; it never grows a second one."
//
// The principal is an argument rather than daemon state because the daemon
// answers every caller. An MCP server holds ONE socket connection for MANY
// callers, so a principal held anywhere but the call itself is a principal
// that can be stale for the caller in front of it.
func (d *Daemon) Invoke(
	ctx context.Context, who kernel.Principal, program, command string, args []byte,
) ([]byte, error) {
	payload, err := proto.Marshal(&rigv1.CallRequest{Args: args})
	if err != nil {
		return nil, fmt.Errorf(
			"daemon: could not encode arguments for %s.%s: %w", program, command, err)
	}

	reply, bad := d.call(ctx, caller{
		who:    who,
		method: program + "." + command,
		args:   payload,
		// requestID is deliberately empty, and the reason survives the
		// correction below: dedup keys on a CLIENT-generated id that is
		// stable across a retry, and an in-process caller has none to offer.
		// Minting one here would produce an id unique per attempt, which is
		// the opposite of what a window needs - it would make every retry
		// look like a new call.
		//
		// THE CITATION WAS WRONG AND THE TENSE WAS WRONG. This said "Section
		// 4's dedup window", which asserts a live mechanism: there is no
		// dedup anywhere in this tree. And section 4 is "Measured: what the
		// wire actually costs" - it contains no dedup, no request id and no
		// replay. Section 5f is the section, and it puts the window
		// "persisted in the WAL", which lands at M7.
	}, program, command)
	if bad != nil {
		return nil, bad.error()
	}

	// A program's own refusal is not rig's, and it arrives as an ERROR frame
	// rather than as a failure of the call. It still has to reach the agent
	// with section 9's structure intact, so it is rebuilt rather than
	// flattened to a sentence.
	if reply.GetKind() == rigv1.FrameKind_FRAME_KIND_ERROR {
		return nil, statusError(program, command, reply.GetStatus())
	}

	var res rigv1.CallResponse
	if err := proto.Unmarshal(reply.GetPayload(), &res); err != nil {
		return nil, fmt.Errorf(
			"daemon: %s.%s answered with something that is not a CallResponse: %w",
			program, command, err)
	}
	return res.GetResult(), nil
}

// statusError turns a Status back into an error, keeping section 9's four
// fields so an in-process caller is told what the wire would have told it.
//
// Section 35: acceptance is not a contract, retrievability is. A refusal that
// reaches one surface complete and another as a bare sentence is the same
// defect one level down.
func statusError(program, command string, st *rigv1.Status) error {
	if st == nil {
		return fmt.Errorf("%s.%s failed and said nothing about why", program, command)
	}
	msg := st.GetMessage()
	if msg == "" {
		msg = fmt.Sprintf("%s.%s failed with %s", program, command, st.GetCode())
	}
	return &kernel.RefusalError{
		Err:          errors.New(msg),
		Precondition: st.GetPrecondition(),
		Actual:       st.GetActual(),
		Fix:          st.GetFix(),
		FixCommand:   st.GetFixCommand(),
	}
}

// mcpCaller is the estate as ONE MCP connection reaches it.
//
// IT EXISTS BECAUSE AN OCCUPANT'S IDENTITY IS ITS CONNECTION AND THE DAEMON IS
// NOT ONE. presence keys its roster on a per-connection token and releases a
// seat from that connection's own defer, so a roster interface implemented on
// *Daemon would have to be handed a token on every call - and the only honest
// type for a token crossing a package boundary is `any`, which is the shape
// that panics as a map key. Wrapping the daemon per connection keeps the token
// where its type is understood and makes the roster's scope structural: this
// value cannot name a connection other than its own.
//
// THE EMBEDDING IS WHAT MAKES IT CHEAP. *Daemon already satisfies meta.Invoker
// and meta.EstateIdentity, so an MCP surface built on this one gets both
// unchanged and adds a roster. Nothing about the other four tools moves.
//
// It is also why nothing in daemon.go changes for this door: the per-connection
// state lives on a value the connection's own handler creates, not in a field
// on the shared daemon.
type mcpCaller struct {
	*Daemon

	// occ is this connection's tenancy of a seat. It is created before the
	// server is and released on the same defer stack as the connection, which
	// is the whole expiry mechanism - no TTL, no reaper.
	occ *occupancy
}

// The roster is the optional half of the meta surface, and this is where it is
// satisfied. A compile-time assertion rather than a comment, because the
// assertion is what fails when a method drifts.
var _ meta.Roster = (*mcpCaller)(nil)

// Announce takes a seat on this estate's roster, for this connection.
//
// A SEAT NAME IS REQUIRED HERE AND NOWHERE ELSE. The program socket keeps
// serving unseated Go programs - cmd/fakeapp and the conformance suite among
// them - and moving this rule into presence.announce would change behaviour for
// every one of them to fix a problem only agents have. An agent gets no
// unseated case: an unseated peer holds an empty seat at generation 0, which is
// not an identity, so its supervision would be lost silently rather than
// refused.
func (m *mcpCaller) Announce(seat, purpose, activity string) (meta.Crew, error) {
	if seat == "" {
		return meta.Crew{}, &kernel.RefusalError{
			Err: errors.New("announce: a seat name is required at this door"),
			Precondition: "seat names the row you are taking, so a supervisor " +
				"can address you and a peer can tell you from the last " +
				"session to hold it",
			Actual: "seat was empty",
			Fix: "announce again with a seat name - the one your briefing " +
				"gave you, such as backend-1",
		}
	}
	// One vocabulary, one validator: a seat is an address other peers type, so
	// it is held to the same shape as an estate name rather than to a looser
	// rule invented at this door.
	if err := paths.ValidEstateName(seat); err != nil {
		return meta.Crew{}, &kernel.RefusalError{
			Err:          fmt.Errorf("announce: invalid seat name: %w", err),
			Precondition: "a seat name is shaped like an estate name",
			Actual:       strconv.Quote(seat),
		}
	}
	if purpose == "" {
		return meta.Crew{}, &kernel.RefusalError{
			Err: errors.New("announce: a purpose is required at this door"),
			Precondition: "purpose is the headline of your row on the " +
				"supervisor's board, in terms they would recognise",
			Actual: "purpose was empty",
			Fix:    "announce again saying what this session is FOR, in one line",
		}
	}

	o, err := m.presence.announce(m.occ, seat, purpose, activity)
	if err != nil {
		var held *seatHeldError
		if asSeatHeld(err, &held) {
			// The fix is deliberately not "try again": the seat is held by
			// something alive, so retrying is the wrong action and naming the
			// holder is the right one.
			return meta.Crew{}, &kernel.RefusalError{
				Err: fmt.Errorf("announce: seat %s is already held by a live "+
					"peer at generation %d whose purpose is %s",
					held.Seat, held.Generation, strconv.Quote(held.Purpose)),
				Precondition: "a seat holds one occupant at a time",
				Actual: fmt.Sprintf("%s is held at generation %d",
					held.Seat, held.Generation),
				Fix: "call list_agents to read the roster, then take a seat " +
					"that is free - or agree with the holder that it should " +
					"stand down first",
			}
		}
		return meta.Crew{}, fmt.Errorf("announce: %w", err)
	}
	return m.crew(&o), nil
}

// SetActivity moves this connection's activity line.
//
// ⛔ ITS REFUSAL IS THE REATTACH CARRIER AND THAT IS THIS METHOD'S SECOND JOB.
// A seat whose daemon dies and whose host silently re-dials to a new one keeps
// working with no sign anything happened: the next call succeeds with a
// byte-identical payload. The daemon cannot tell that seat from one that never
// announced - new connection, fresh token, no occupant, identical - and it does
// not need to, because BOTH NEED THE SAME ACTION. This fires on the call a
// seated agent makes most often, so the reattach is caught in one turn.
//
// THE REFUSAL NAMES THE ESTATE AND THE EPOCH, which the wire's equivalent does
// not. A seat reading only this error then knows WHICH daemon refused it, and
// the line is self-describing in a log without a second read.
func (m *mcpCaller) SetActivity(activity string) (meta.Crew, error) {
	// UNSPECIFIED, NOT ACTIVE, AND THAT IS THE OMISSION MADE STRUCTURAL.
	//
	// state is deliberately not an argument of this tool: it has no adopter at
	// this door, and a field a porting caller never sets is a field nobody
	// tests. Passing ACTIVE would be the door WRITING the field it claims not
	// to carry - an assertion rather than an abstention - and presence reads
	// UNSPECIFIED as "leave it alone" precisely so a caller does not have to
	// restate a transition it did not make.
	//
	// It is harmless today, because an MCP occupant's state can only ever be
	// ACTIVE: a seat is one connection and an MCP connection is not a wire
	// connection, so there is no HANDING_OFF here to overwrite. It stops being
	// harmless the moment state IS added - a caller would set HANDING_OFF and
	// the next set_activity would quietly reset it, which is the failure
	// presence's own comment describes: a state that resets itself every time
	// a peer says what it is doing is a state nobody can hold, and HANDING_OFF
	// must survive several activity lines while a successor is briefed.
	//
	// Raised by the presence owner reviewing this file. Kept as one token
	// rather than a comment on the risk.
	o, ok := m.presence.setActivity(m.occ, activity,
		rigv1.SeatState_SEAT_STATE_UNSPECIFIED)
	if !ok {
		return meta.Crew{}, &kernel.RefusalError{
			Err: fmt.Errorf("set_activity: this connection has no row on %s, "+
				"so there is nothing to update", m.whichDaemon()),
			Precondition: "a row is created by announce and lives exactly as " +
				"long as the connection that took it",
			Actual: "this connection has not announced",
			Fix: "call announce with your seat and purpose. If you announced " +
				"earlier and this is the first you have heard of it, you are " +
				"talking to a DIFFERENT daemon than you were - the one you " +
				"announced to is gone and your host re-dialled without " +
				"telling you. Announcing again is the whole fix",
		}
	}
	return m.crew(&o), nil
}

// Peers reads the roster.
//
// NOT REFUSED FOR A CALLER WITH NO ROW, deliberately. It is an unscoped read on
// the wire and it is the one call a confused seat uses to find out what
// happened; refusing it would break an unseated reader and take away the answer
// at exactly the moment it is needed.
//
// ⛔ AND IT SERVES `you`, WHICH IT ONCE DID NOT. This returned crew(nil)
// unconditionally and therefore answered `"you": null` to a SEATED caller -
// which is this door's own vocabulary for "you have no row". So a seated seat
// was told, in the surface's own words, that it was not on the roster, on the
// one call a confused seat makes after its host has silently re-dialled it to
// a different daemon. The wrong answer, in the single moment the answer matters
// most.
//
// It survived six mutations because no test entered the state: every one of
// them asserted `you` on announce and set_activity, where it was right. A live
// run found it. A mutation measures whether an assertion bites and says nothing
// about whether the assertions cover the states a caller reaches.
func (m *mcpCaller) Peers() meta.Crew {
	if o, ok := m.presence.occupantOf(m.occ); ok {
		return m.crew(&o)
	}
	return m.crew(nil)
}

// whichDaemon names this estate and its epoch, for a refusal that has to be
// legible on its own.
//
// An unnamed estate has no name and no epoch - it opens no store - so it says
// so rather than rendering an empty name and a zero as though they were values.
func (m *mcpCaller) whichDaemon() string {
	if m.estate == "" {
		return "this unnamed estate"
	}
	return fmt.Sprintf("estate %s at epoch %d", m.estate, m.epoch)
}

// crew renders the roster as the agent surface reads it, with this caller's own
// row when it has one.
func (m *mcpCaller) crew(you *occupant) meta.Crew {
	rows := m.presence.crew()
	out := meta.Crew{
		Everyone:     make([]meta.Occupant, 0, len(rows)),
		OtherEstates: m.presence.others(),
	}
	for _, r := range rows {
		out.Everyone = append(out.Everyone, seatToOccupant(r))
	}
	if you != nil {
		o := seatToOccupant(you.proto())
		out.You = &o
	}
	if out.OtherEstates == nil {
		out.OtherEstates = []string{}
	}
	return out
}

// seatToOccupant maps the wire's row onto the agent surface's.
//
// THE MAPPING EXISTS SO internal/meta NEVER LEARNS THE PROTO, which is the same
// direction meta.Estate already travels.
//
// NOTHING HERE IS ENFORCED BY THE COMPILER, AND THIS COMMENT USED TO CLAIM IT
// WAS. It said the mapping is "deliberately total ... a compile-time gap rather
// than a silently thinner row". MEASURED FALSE 2026-09-16: this is a KEYED
// literal, so a field added to either side and forgotten here compiles, and the
// row arrives with a zero value that no reader can tell from a real one.
//
// The neighbouring claim IS true and is easy to confuse with this one:
// meta.occupantToJSON is a type CONVERSION, so Occupant and occupantJSON really
// are a compile-time contract. That guard stops at meta's own boundary and says
// nothing about what this function populates. Obeying the compile error it
// raises - adding the field to occupantJSON - leaves this function untouched
// and the build green.
//
// WHAT GUARDS IT INSTEAD is TestEverySeatFieldReachesTheRosterRow, which sets
// each wire field alone and requires the row to change. Add a field to Seat and
// that test goes red and names it; there is no compiler to catch you.
func seatToOccupant(s *rigv1.Seat) meta.Occupant {
	return meta.Occupant{
		Seat:              s.GetSeat(),
		Generation:        s.GetGeneration(),
		Epoch:             s.GetEpoch(),
		Estate:            s.GetEstate(),
		Purpose:           s.GetPurpose(),
		Activity:          s.GetActivity(),
		State:             stateWord(s.GetState()),
		AnnouncedUnixNano: s.GetAnnouncedUnixNano(),
		ActivityUnixNano:  s.GetActivityUnixNano(),
	}
}

// stateWord is the wire's seat-state enum as the word an agent reads, derived
// FROM the enum for the reason roleWord is: a state added to the proto arrives
// on the agent surface without a second table to remember.
func stateWord(s rigv1.SeatState) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "SEAT_STATE_"))
}
