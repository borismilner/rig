package meta

import "github.com/borismilner/rig/internal/kernel"

// Roster is the optional half of Invoker: the thing that can call into an
// estate usually also knows WHO IS IN IT.
//
// OPTIONAL RATHER THAN PART OF Invoker, and it is EstateIdentity's shape on
// purpose - the second use of a pattern this tree already chose for this exact
// inversion. meta.New's invoker may be nil, and a surface that can read the
// estate but not call into it is a real configuration; a surface that can call
// into it but has no roster under it is the same kind of real. Resolving the
// roster PER CALL by type assertion means a server built with a nil invoker and
// one built with a daemon are the same code path, and neither needs a second
// constructor.
//
// WHY THIS EXISTS AT ALL, because "put presence behind the MCP door" hides the
// actual shape: announce, activity and peers are fully declared commands of
// program `rig` and have been since self.go was written. They are unreachable
// from an agent not because they are missing but because rig is held BESIDE the
// program map and invoke reaches the map. Promotion was the obvious route and
// is closed at four independent points, the decisive one being that it would
// put rig.down - EffectsDestructive with Confirms:No, whose protection IS
// unreachability - on the agent invoke surface. So the route is not Invoke under
// any surface choice, and this interface is what replaces it.
//
// NO METHOD HERE TAKES A PRINCIPAL, AND THAT IS A DECISION RATHER THAN AN
// OVERSIGHT, because every other interface in this package takes one.
//
// An occupant's identity is its CONNECTION, not its caller: presence keys its
// map on a per-connection token and `leave` runs from that connection's own
// defer. So the implementation of this interface is itself per-connection - the
// daemon builds one around the connection's token and hands THAT to meta.New -
// and the receiver already carries everything the roster needs to name the
// caller. A principal passed alongside it would be a second identity for the
// same connection, read by nothing.
//
// Section 13a's rule is that every function ON A PATH TO INVOCATION carries the
// principal, and none of these is: they reach presence, which authorises by
// occupancy. Carrying one anyway would put a parameter on the surface that no
// implementation reads, which is the invented-field failure one level up - a
// reader would take its presence as evidence that something checks it.
//
// The alternative that was NOT taken: passing the occupancy token through as an
// `any`. presence keys its map on that token, and an interface carrying what
// presence needs of a key has no methods - which is `any`, which panics as a
// map key on an uncomparable type. Keeping the token inside the daemon keeps
// that hazard where the type is understood.
type Roster interface {
	// Announce takes a seat and returns the roster as this caller now sees it.
	//
	// A SEAT NAME IS REQUIRED AND THAT IS THIS DOOR'S RULE, NOT PRESENCE'S. The
	// program socket keeps serving unseated Go programs - cmd/fakeapp and the
	// conformance suite among them - because an unseated program is a real
	// case there. An agent gets no unseated case: an unseated peer has an empty
	// seat and generation 0, which is not an identity, so supervision of it
	// would be lost silently rather than refused. Enforcing it in presence
	// would change behaviour for every Go caller to fix a problem only agents
	// have.
	Announce(seat, purpose, activity string) (Crew, error)

	// SetActivity moves this caller's activity line.
	//
	// IT REFUSES A CONNECTION THAT HAS NO ROW, and that refusal is the carrier
	// for a failure nothing else on this surface catches: a seat whose daemon
	// died and whose host silently re-dialled to a new one. The daemon cannot
	// tell a reattached seat from a seat that never announced - new connection,
	// fresh token, no occupant, identical - and it does not need to, because
	// both need the same action. This fires on the call a seated agent makes
	// most often, so the reattach is caught in one turn rather than never.
	SetActivity(activity string) (Crew, error)

	// Peers reads the roster, and is deliberately NOT refused for a caller with
	// no row. It is the one call a confused seat uses to find out what
	// happened, and an unseated reader is a legitimate one.
	Peers() Crew
}

// Crew is the estate's roster as one caller reads it.
//
// ITS OWN TYPE RATHER THAN THE WIRE'S Seat, for the reason Estate is its own
// type: this package renders the agent surface and must not acquire a
// dependency on the proto to do it. The daemon maps one to the other, which is
// the same direction Estate already travels.
type Crew struct {
	// You is this caller's own row, when it has one. Nil for a reader that
	// has not announced, which is a case rather than an error.
	You *Occupant

	// Everyone is the whole roster, this caller included.
	//
	// NOT NAMED Crew, because Crew.Crew reads as though the two were different
	// things and the wire's own name for this field has already confused one
	// reader.
	Everyone []Occupant

	// OtherEstates names the estates this daemon knows about and cannot see
	// into. It is emitted ALWAYS, empty included.
	//
	// ⛔ THIS IS WHERE THE WORD `partial` WOULD HAVE GONE, AND IT MUST NOT.
	// There are THREE things called partial, not two: AgentBox's bool (the
	// roster cannot see everybody), rig presence's bool (another named estate
	// exists, unseen), and rig's own meta answers' LIST (which subjects
	// answered partially), emitted on every answer per section 5k. Same name,
	// different TYPE - `if (partial)` on an empty array is truthy in
	// JavaScript and falsy in Python, so one wrong port behaves differently
	// per language.
	//
	// AND rig NEVER SERVES AgentBox'S MEANING AT ALL. Within one estate the
	// roster is complete by construction: one estate is one daemon, presence
	// is connection state on that daemon, so there is no "sessions we cannot
	// see" case. It would be a constant false, and a constant dressed as a
	// live field is a lie by shape.
	//
	// So the door serves NAMES. A caller that wants a bool derives it in one
	// expression and rig keeps the thing that makes its answer better: it can
	// say WHICH.
	OtherEstates []string
}

// Occupant is one row of the roster.
//
// EVERY FIELD HERE IS PART OF THE SUPERVISION CONTRACT AND NONE IS DECORATION.
// B9 closed the gap that nothing in this repository read State, Generation,
// Purpose or Activity off SOMEBODY ELSE'S row, and HANDING_OFF exists solely
// for that third-party reader. A door serving rows without them regresses that
// on a new surface while every wire test stays green.
type Occupant struct {
	// Seat, Generation, Epoch and Estate are the identity, and it takes all
	// four. The epoch store is PER ESTATE and each estate counts from 1, so
	// two estates that have each restarted once are both at epoch 2 and
	// (seat, epoch, generation) is not unique without the estate on it.
	Seat       string
	Generation uint64
	Epoch      uint64
	Estate     string

	Purpose  string
	Activity string

	// State is ACTIVE or HANDING_OFF, lowercased the way Estate.Role is.
	//
	// A SEAT CANNOT SET THIS THROUGH THIS DOOR AND CAN SEE IT ON A PEER'S ROW,
	// which is coherent rather than lopsided: the setter has no adopter in this
	// cutover, and the FACT has one in every reader of the board.
	State string

	// The two timestamps, and they are not optional either. An age is the only
	// thing that distinguishes a working session from a hung one, and the rule
	// that an unchanged activity line does not reset its age is worthless at
	// this door if the age is not served.
	AnnouncedUnixNano int64
	ActivityUnixNano  int64
}

// roster asks the invoker for the estate's roster, if it can give one.
//
// The type assertion is where the optional half is resolved, and it is done per
// call rather than at construction for the reason estate() states: a surface
// built with a nil invoker is the same code path as one built with a daemon.
func (s *Server) roster() (Roster, bool) {
	r, ok := s.invoker.(Roster)
	if !ok {
		return nil, false
	}
	return r, true
}

// announce takes a seat on this estate's roster.
func (s *Server) announce(who kernel.Principal, r Request) (Answer, error) {
	out, err := s.rosterAnswer(who, Announce)
	if err != nil {
		return Answer{}, err
	}
	rs, ok := s.roster()
	if !ok {
		out.Unavailable = append(out.Unavailable, unavailableRoster)
		return out, nil
	}
	crew, err := rs.Announce(r.Seat, r.Purpose, r.Activity)
	if err != nil {
		return Answer{}, err
	}
	out.Crew = &crew
	return out, nil
}

// setActivity moves this caller's activity line.
func (s *Server) setActivity(who kernel.Principal, r Request) (Answer, error) {
	out, err := s.rosterAnswer(who, SetActivity)
	if err != nil {
		return Answer{}, err
	}
	rs, ok := s.roster()
	if !ok {
		out.Unavailable = append(out.Unavailable, unavailableRoster)
		return out, nil
	}
	crew, err := rs.SetActivity(r.Activity)
	if err != nil {
		return Answer{}, err
	}
	out.Crew = &crew
	return out, nil
}

// listAgents reads the roster.
func (s *Server) listAgents(who kernel.Principal, _ Request) (Answer, error) {
	out, err := s.rosterAnswer(who, ListAgents)
	if err != nil {
		return Answer{}, err
	}
	rs, ok := s.roster()
	if !ok {
		out.Unavailable = append(out.Unavailable, unavailableRoster)
		return out, nil
	}
	crew := rs.Peers()
	out.Crew = &crew
	return out, nil
}

// unavailableRoster is what the three tools report when there is no daemon
// under this surface.
//
// UNAVAILABLE RATHER THAN A REFUSAL, and the distinction is the one query
// already draws: a refusal says the caller asked for something wrong, and an
// unavailable says rig could not reach the source. A surface built without a
// daemon is a real configuration and the caller did nothing wrong, so telling
// it that its call was invalid would send it to fix the wrong thing.
const unavailableRoster = "the estate's roster"

// rosterAnswer is the part of a roster answer that is not the roster: the
// tool's name, and what rig could not see while answering it.
//
// IT READS THE CAPABILITY MAP FOR ONE FIELD AND THAT IS DELIBERATE. `partial`
// is emitted on EVERY answer, empty included, because section 5k forbids a
// surface implying completeness and `[]` has to mean "nothing here is
// incomplete" rather than "nobody looked". A roster answer that left it nil
// would render as `[]` and make exactly the claim it had not checked - which
// is worse than the cost of the read, because it is a lie that looks like
// diligence.
//
// Depth is left unspecified, which is section 21's zero meaning "nothing was
// said". No depth is involved in returning a roster.
func (s *Server) rosterAnswer(who kernel.Principal, tool Tool) (Answer, error) {
	m, err := s.kernel.See(who).CapabilityMap(kernel.DepthPrograms)
	if err != nil {
		return Answer{}, err
	}
	return Answer{Tool: tool, Partial: partialOf(m.Programs...)}, nil
}
