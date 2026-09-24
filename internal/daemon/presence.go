package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/borismilner/rig/internal/paths"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// occupancy is WHAT PRESENCE COUNTS AN OCCUPANT BY: an identity, and not a
// connection. It carries no data because presence needs none.
//
// IT USED TO BE `*conn` AND THAT WAS A COUPLING NOBODY CHOSE. Nothing in this
// file ever dereferenced it - the map lookups, the `other != c` comparison and
// the delete all used it as an opaque comparable identity and touched no
// field. So the wire connection was standing in for a token, and the cost only
// appeared when a SECOND door arrived: the MCP door hands its transport a raw
// net.Conn and never builds a `conn` at all, which made presence unreachable
// from the surface the cutover is about.
//
// THE LIFETIME CONTRACT IS THE WHOLE OF IT, AND BREAKING IT FAILS SILENTLY.
// A door must allocate exactly one of these per ACCEPTED CONNECTION and must
// `defer leave` it on the same defer stack that closes that connection. It
// cannot be per call, per session or per request: an occupant lives exactly as
// long as its connection, and that is what buys "no TTL, no reaper, no orphan
// to detect".
//
// A DOOR THAT FORGETS THE LEAVE LEAKS OCCUPANTS FOREVER - the roster only
// grows and seats are never released - AND NOTHING GOES RED. The holder-dies
// result stays true for the door that remembered and is false for the one that
// did not, which is one roster, two doors and a green suite. That is why this
// paragraph is at the type rather than in a handoff.
//
// IT MUST NOT BE ZERO-SIZED, AND THE FIRST VERSION OF IT WAS. This padding is
// load-bearing and deleting it silently destroys the roster.
//
// Go gives every heap allocation of a zero-sized type THE SAME ADDRESS -
// runtime.zerobase - so `&occupancy{}` on two connections returns one pointer,
// and a map keyed on it holds ONE entry for both. Measured: two allocations
// compared equal at 0x594fc0 and a two-key map had length 1.
//
// That is not a tidy bug. It is two live peers sharing one roster row, which
// is the two-sessions-one-seat failure the seat refusal exists to prevent,
// arriving through the IDENTITY instead of through the seat name - and the
// refusal cannot fire, because from presence's side there is only one peer.
// presence_identity_test.go holds it.
type occupancy struct{ _ [1]byte }

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
	by   map[*occupancy]*occupant
	gens map[string]uint64

	// epoch is WHICH DAEMON RUN the generations above are being counted in,
	// and it is the half of the identity presence cannot derive. The counter
	// is in memory and restarts at 1 with the daemon; the epoch is durable and
	// is bumped at every start, so only the pair tells one tenancy from
	// another. It is taken once at construction because a daemon answers out
	// of exactly one run: a roster cannot hold two epochs, and a reader must
	// never have to branch on the possibility.
	epoch uint64

	// estate is WHICH ESTATE this roster belongs to, by name, and it is the
	// outermost component of a seat's identity. It is here for exactly the
	// reason the epoch is, one level further out: the triple above is not
	// unique without it, because the epoch store is PER ESTATE and every
	// estate counts its own from 1, so two estates that have each restarted
	// once are both at epoch 2.
	//
	// EMPTY IS A CASE AND NOT A MISSING VALUE. An unnamed estate has no
	// persistent store, so it is empty here and zero above, and that pair is
	// coherent. The pair cannot come apart because ONE caller supplies both
	// and derives the epoch from this estate's own store, which coord.Open
	// refuses to give an unnamed estate at all.
	estate string

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
	epoch      uint64
	estate     string
	purpose    string
	activity   string
	state      rigv1.SeatState
	announced  time.Time
	moved      time.Time
}

func newPresence(estate string, epoch uint64) *presence {
	return &presence{
		by:     make(map[*occupancy]*occupant),
		gens:   make(map[string]uint64),
		epoch:  epoch,
		estate: estate,
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
//
// AND A RESTATE CARRIES THE ACTIVITY LINE ACROSS, which is the third thing
// that does not restart with it. This door was the one the age rule below was
// missed at: restating a purpose is a call the team is actively encouraged to
// make, so laundering a day-old activity line into a fresh-looking one on the
// way past is a failure that arrives through the recommended path.
func (p *presence) announce(c *occupancy, seat, purpose, activity string) (occupant, error) {
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

	// restating is the same connection saying itself again in the SAME seat,
	// and it gates the TENANCY only: the generation and the announcement.
	//
	// THE LINE AND ITS AGE ARE GATED SEPARATELY, ON `had`, AND THE SPLIT IS
	// THE WHOLE POINT. A tenancy and a line are two facts with two lifetimes.
	// The generation and `announced` say when THIS TENANCY began and must
	// restart when it does; `moved` says when THE LINE last changed, and
	// setLine's own rule is that the age tracks the LINE and not the call.
	// Gating both on one condition made the age restart for reasons that had
	// nothing to do with the line.
	//
	// THREE CASES REACH THIS, and the seat is what tells them apart:
	//
	//	unseated re-announce	 had, no seat        tenancy no, line YES
	//	seated, same seat	 had, seat unchanged  tenancy no, line YES
	//	seated, DIFFERENT seat	 had, seat changed    tenancy YES, line YES
	//
	// THE UNSEATED CASE IS NOT A CASE THAT AGES OUT, which is what the
	// previous version of this comment assumed. Requiring a seat AT THE DOOR
	// removes it for agents and leaves it untouched for everything on the
	// program socket - cmd/fakeapp and the conformance suite are permanent
	// unseated peers, and their rows sit on the roster crew() hands a board.
	// Measured: an unseated peer restating one line a day later reported that
	// line as current, while a seated peer restating the same line did not.
	//
	// THE THIRD CASE IS THE ONE TO READ TWICE. A connection that re-announces
	// into a DIFFERENT free seat now carries its line and age across, where it
	// used to reset them. That is deliberate: a session looping on one line
	// that also re-seats would otherwise refresh its own age, which is exactly
	// the freshness a stuck session manufactures for itself - the failure
	// setLine exists to stop, arriving through the seat change instead of
	// through the repeat. Nothing is lost, because `announced` separately
	// carries when the tenancy began and is served on the row beside it.
	restating := had && seat != "" && prev.seat == seat

	o := &occupant{
		seat:      seat,
		epoch:     p.epoch,
		estate:    p.estate,
		purpose:   purpose,
		state:     rigv1.SeatState_SEAT_STATE_ACTIVE,
		announced: t,
		moved:     t,
	}
	switch {
	case seat == "":
		// No seat, so no tenancy and no generation. Zero here is a fact and
		// not a missing value, which is why the proto says so.
		o.generation = 0
	case restating:
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

	// The line and its age come across BEFORE the new one is applied, so that
	// setLine below compares against what this occupant was already saying
	// rather than against a blank. Without this the comparison is trivially
	// true every time and the rule cannot bite at this door at all.
	//
	// `had` AND NOT `restating`, WHICH IS THE DIFFERENCE BETWEEN A LINE AND A
	// TENANCY - see the split above. The condition is "the same connection was
	// already here", because the line belongs to the peer rather than to the
	// seat it is sitting in.
	if had {
		o.activity, o.moved = prev.activity, prev.moved
	}
	o.setLine(activity, t)

	p.by[c] = o
	return *o, nil
}

// setLine records what an occupant is doing and WHEN THAT LINE LAST CHANGED.
// Both doors go through here, because the rule is about the line and not about
// which call carried it.
//
// AN UNCHANGED LINE DOES NOT MOVE THE AGE. AgentBox refuses the same reset and
// gives the reason in its own tool description - "repeating yourself is not
// progress" - and section 37's cutover inherits the property along with the
// callers. A session looping on one line otherwise renews its own freshness,
// which turns the only signal a board has for a stuck session into a signal
// the stuck session manufactures. `presence_death_test.go` is where that bites
// hardest: its honest limit is that presence detects DEATH and not a HANG, and
// it hands the hang off to exactly this field.
//
// AN EMPTY LINE IS NOT SUPPLIED, AND NEVER A CLEARING. Nothing on this wire
// lets a peer mean "I am now doing nothing", and a blank activity is the row
// `serveAnnounce` already refuses when the PURPOSE is blank - "indistinguishable
// from a session nobody is supervising". AgentBox takes the activity as
// OPTIONAL on announce, so a ported caller restating only its purpose supplies
// no line at all: the ordinary shape, not an exotic one.
func (o *occupant) setLine(line string, t time.Time) {
	if line == "" || line == o.activity {
		return
	}
	o.activity = line
	o.moved = t
}

// setActivity replaces what an occupant is doing, and optionally its state.
//
// THE AGE TRACKS THE LINE AND NOT THE CALL - see setLine, which is the whole
// of that rule and is shared with announce. A state-only transition therefore
// reaches here with an empty activity and correctly leaves both the line and
// its age where they were, which is the same answer as the re-announce case
// and not a second rule.
//
// UNSPECIFIED LEAVES THE STATE ALONE rather than clearing it, so the ordinary
// call does not have to restate a transition it did not make. A state that
// resets itself every time a peer says what it is doing is a state nobody can
// hold, and HANDING_OFF is precisely a state that must survive several
// activity lines while a successor is briefed.
func (p *presence) setActivity(c *occupancy, activity string, state rigv1.SeatState) (occupant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	o, ok := p.by[c]
	if !ok {
		return occupant{}, false
	}
	o.setLine(activity, p.now())
	if state != rigv1.SeatState_SEAT_STATE_UNSPECIFIED {
		o.state = state
	}
	return *o, true
}

// occupantOf reads this connection's own row and changes NOTHING.
//
// IT EXISTS BECAUSE THE ONLY EXISTING WAY TO GET THIS ROW IS A WRITER.
// setActivity already returns (occupant, bool) and is a no-op for an empty
// line and an unspecified state, because setLine returns early on an empty
// line - so it would serve as a read today, and the door's list_agents could
// have been built on it. That is a trap rather than a shortcut: the day
// setLine's early return changes, every caller using it as a read silently
// becomes a writer, and nothing at the call site says so.
//
// A reader that is a writer under the covers cannot be mutation-tested either,
// because the mutation that breaks it lives in a different function.
//
// THE VALUE IS COPIED OUT, as announce and setActivity already do, so a caller
// cannot reach back into the roster through the row it was handed.
func (p *presence) occupantOf(c *occupancy) (occupant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	o, ok := p.by[c]
	if !ok {
		return occupant{}, false
	}
	return *o, true
}

// leave forgets a connection. Called from the handler's own defer, so a
// dropped peer empties its seat with nothing scheduled and nothing to expire.
func (p *presence) leave(c *occupancy) {
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
		Epoch:             o.epoch,
		Estate:            o.estate,
		Purpose:           o.purpose,
		Activity:          o.activity,
		State:             o.state,
		AnnouncedUnixNano: o.announced.UnixNano(),
		ActivityUnixNano:  o.moved.UnixNano(),
	}
}

// otherEstates lists named estate claims held by a LIVE daemon, other than
// this one's own.
//
// THE LIVENESS TEST IS THE FLOCK AND NOT THE FILE'S EXISTENCE, and that is not
// a refinement - reading the directory is wrong. `instance.Close` says so in
// as many words: the pidfile is DELIBERATELY never unlinked, because unlinking
// races a second daemon, and "the file is cheap to leave and its contents are
// never trusted". So a claim file outliving its daemon is the designed
// behaviour, and a reader that counts files reports every estate that has ever
// run on this machine as though it were running now.
//
// MEASURED, and it is why this function looks like this: the first version
// read directory entries, and a live demonstration reported `partial: true`
// against two claims whose daemons had been dead since the previous evening.
// The unit tests could not see it because they never touch the real state
// directory. Section 37 precondition 6 already owns this question, so the
// answer is taken from its mechanism rather than from a second one.
//
// The pid inside is still never read. Trying the lock asks the kernel the
// question the pid could only guess at.
func otherEstates(mine string) []string {
	dir, err := paths.StateDir()
	if err != nil {
		return nil
	}
	claims := filepath.Join(dir, "estates")
	entries, err := os.ReadDir(claims)
	if err != nil {
		// No named estate has ever run here. The common case in every test in
		// this repository, and not an error.
		return nil
	}
	var out []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".pid")
		if name == e.Name() || name == "" || name == mine {
			continue
		}
		if claimHeld(filepath.Join(claims, e.Name())) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// claimHeld reports whether a live process holds this claim's lock.
//
// A SHARED lock is enough to answer it and is the weakest thing that can:
// the holder takes LOCK_EX, so LOCK_SH fails with EWOULDBLOCK exactly while
// somebody is there. Taking LOCK_EX to test would be indistinguishable from
// trying to STEAL the claim, and on a file this process has no business
// writing.
func claimHeld(path string) bool {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		// EWOULDBLOCK means held. Any other error means the question could not
		// be asked, and an unanswerable question is not evidence of a peer.
		return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}
