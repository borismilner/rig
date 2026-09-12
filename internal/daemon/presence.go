package daemon

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// presence is the estate's roster: who is here, what they are for, and which
// seat each one holds.
//
// IT IS CONNECTION STATE AND IT IS NOT PERSISTED, which is the decision that
// let section 37's row 1 be built before the write-ahead log exists. An
// occupant lives exactly as long as its connection: there is no TTL to tune,
// no reaper to schedule and no orphan to detect, because the operating system
// already tells us when a peer is gone. Everything in section 16 that DOES
// need durability - leases, the blackboard, the signal log - is a later row
// resting on a store, and none of it is here.
//
// The one fact that outlives a connection is the GENERATION, and it is a
// counter per seat NAME rather than per occupant. That asymmetry is the whole
// mechanism: a seat is an address that persists, an occupancy is a tenancy of
// it, and the counter is what lets a peer tell one tenancy from the next.
type presence struct {
	mu   sync.Mutex
	by   map[*conn]*occupant
	gens map[string]uint64

	// now is injectable because every field this produces is a timestamp and
	// a test that cannot fix the clock can only assert that time passed.
	// There are no timers here, so section 20's synctest mandate does not
	// reach this file: nothing waits.
	now func() time.Time

	// others names the estates this daemon knows about and cannot see into.
	// Injectable for the same reason - the real one reads a directory.
	others func() []string
}

type occupant struct {
	seat       string
	generation uint64
	purpose    string
	activity   string
	state      rigv1.SeatState
	announced  time.Time
	moved      time.Time
}

func newPresence(estate string) *presence {
	return &presence{
		by:     make(map[*conn]*occupant),
		gens:   make(map[string]uint64),
		now:    time.Now,
		others: func() []string { return otherEstates(estate) },
	}
}

// seatHeldError is returned when a seat is already occupied by a live peer.
//
// REFUSING IS THE POINT AND THE OPPOSITE WAS THE TEMPTING DEFAULT. Letting the
// newcomer take the seat would make every announce succeed, and the failure it
// would hide is exactly the one this mechanism exists to surface: two sessions
// both believing they are `backend-1`, both writing, neither finding out. A
// refusal is a peer discovering the truth at the cheapest possible moment.
type seatHeldError struct {
	Seat       string
	Generation uint64
	Purpose    string
}

func (e *seatHeldError) Error() string {
	return "seat " + e.Seat + " is held by a live peer"
}

// announce records this connection as present, taking a seat if one is named.
//
// Re-announcing on the same connection is allowed and is the documented way to
// restate a purpose after a handoff - the roster defect Boris observed was two
// rows still reading "successor in a warm handoff" because nobody did. Keeping
// the SAME seat keeps the generation; moving to a different one takes a new
// tenancy and therefore a new generation.
func (p *presence) announce(c *conn, seat, purpose, activity string) (occupant, error) {
	seat = strings.TrimSpace(seat)
	p.mu.Lock()
	defer p.mu.Unlock()

	if seat != "" {
		for other, o := range p.by {
			if other != c && o.seat == seat {
				return occupant{}, &seatHeldError{
					Seat: seat, Generation: o.generation, Purpose: o.purpose,
				}
			}
		}
	}

	t := p.now()
	prev, had := p.by[c]

	o := &occupant{
		seat:      seat,
		purpose:   purpose,
		activity:  activity,
		state:     rigv1.SeatState_SEAT_STATE_ACTIVE,
		announced: t,
		moved:     t,
	}
	switch {
	case seat == "":
		// No seat, so no tenancy and no generation. Zero here is a fact and
		// not a missing value, which is why the proto says so.
		o.generation = 0
	case had && prev.seat == seat:
		// Same connection restating itself in the same seat. This is not a
		// new tenancy, so the generation must NOT move: a peer holding a
		// reference to generation 3 is still correctly addressing this
		// occupant, and bumping here would invalidate it for nothing.
		o.generation = prev.generation
		o.announced = prev.announced
	default:
		p.gens[seat]++
		o.generation = p.gens[seat]
	}

	p.by[c] = o
	return *o, nil
}

// setActivity replaces what an occupant is doing, and optionally its state.
//
// UNSPECIFIED LEAVES THE STATE ALONE rather than clearing it, so the ordinary
// call does not have to restate a transition it did not make. A state that
// resets itself every time a peer says what it is doing is a state nobody can
// hold, and HANDING_OFF is precisely a state that must survive several
// activity lines while a successor is briefed.
func (p *presence) setActivity(c *conn, activity string, state rigv1.SeatState) (occupant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	o, ok := p.by[c]
	if !ok {
		return occupant{}, false
	}
	o.activity = activity
	o.moved = p.now()
	if state != rigv1.SeatState_SEAT_STATE_UNSPECIFIED {
		o.state = state
	}
	return *o, true
}

// leave forgets a connection. Called from the handler's own defer, so a
// dropped peer empties its seat with nothing scheduled and nothing to expire.
func (p *presence) leave(c *conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.by, c)
}

// crew is every occupant, ordered so two callers reading the same roster in
// the same instant get the same bytes. Seated peers first, then by seat name,
// then by when they announced - an unstable roster is one a human cannot scan.
func (p *presence) crew() []*rigv1.Seat {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*rigv1.Seat, 0, len(p.by))
	for _, o := range p.by {
		out = append(out, o.proto())
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.GetSeat() == "") != (b.GetSeat() == "") {
			return a.GetSeat() != ""
		}
		if a.GetSeat() != b.GetSeat() {
			return a.GetSeat() < b.GetSeat()
		}
		return a.GetAnnouncedUnixNano() < b.GetAnnouncedUnixNano()
	})
	return out
}

// partial says whether this roster is known to be incomplete.
//
// READ THE FALSE CASE CAREFULLY, because it is weaker than it looks and the
// proto says so too. TRUE means rig can NAME another estate whose peers are
// not in this list. FALSE means no OTHER NAMED ESTATE is claimed - it does not
// mean the caller is alone, because an unnamed estate claims nothing by
// construction (section 37's ephemeral clause) and therefore cannot be
// enumerated by anybody, including this daemon.
//
// That is still strictly more than AgentBox reports, which is an unqualified
// boolean a reader cannot act on. Here a true answer comes with the reason.
func (p *presence) partial() bool {
	return len(p.others()) > 0
}

func (o *occupant) proto() *rigv1.Seat {
	return &rigv1.Seat{
		Seat:              o.seat,
		Generation:        o.generation,
		Purpose:           o.purpose,
		Activity:          o.activity,
		State:             o.state,
		AnnouncedUnixNano: o.announced.UnixNano(),
		ActivityUnixNano:  o.moved.UnixNano(),
	}
}

// otherEstates lists named estate claims that are not this daemon's own.
//
// It reads the claim directory section 37 precondition 6 already maintains
// rather than introducing a second registry, because a second place to write
// the same fact is how the two come to disagree. A missing directory means no
// named estate has ever run on this machine, which is the common case in every
// test in this repository and is not an error.
func otherEstates(mine string) []string {
	dir, err := paths.StateDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, "estates"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".pid")
		if name != e.Name() && name != "" && name != mine {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
