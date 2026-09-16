package coord

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// State is where a lease is in the TWO-STEP expiry (PLAN.md section 16).
//
// THE MIDDLE STATE IS THE WHOLE MECHANISM AND IT IS WHY THERE ARE THREE.
// "Expired" and "free" are the same word in most lock libraries, and collapsing
// them is the classic failure: holder A stalls, its lease expires, B acquires,
// A wakes and writes anyway. A TTL alone cannot tell "the holder is gone" from
// "the holder is slow", and those need opposite answers.
type State string

const (
	// Held: inside its deadline, on this boot. The holder owns it.
	Held State = "HELD"
	// Orphaned: the deadline passed and the witness was NOT observed dead.
	// It is NOT free. Its own holder may still renew it, which is the point
	// of not freeing it - a stalled holder gets its work back rather than
	// discovering a second holder did it meanwhile.
	Orphaned State = "ORPHANED"
	// Free: nobody holds it. Reached from Orphaned only by observing the
	// witness dead, or by a recorded Break.
	Free State = "FREE"
)

// Handle is a lease held by a caller, and it carries the epoch.
//
// EVERY HANDLE CARRIES THE EPOCH (section 37, precondition 4, ruling V15).
// Token fences one holder against the next holder of the same lease; the epoch
// fences every handle against a daemon restart. They are different failures:
// the token catches "somebody else has it now", the epoch catches "the thing
// that granted this is gone, and everything it knew about who holds what came
// off a disk it has not re-checked with you".
type Handle struct {
	Name     string
	Holder   string
	Token    uint64
	Epoch    uint64
	Deadline Instant
}

// Status is what a reader learns about a lease, INCLUDING the owner's liveness.
//
// OwnerGone is section 16's requirement in as many words: "a read reports the
// owner's liveness, not just the value", so a claim whose owner is gone is
// visible as abandoned rather than indistinguishable from a healthy one. It is
// the same fact AgentBox's blackboard publishes under the same name.
type Status struct {
	Name  string
	State State
	// Holder is the LAST holder, not necessarily a current one - State is the
	// authority on whether the lease is available and Holder never is.
	//
	// A FREE LEASE STILL NAMES THE HOLDER THAT ABANDONED IT, and that is the
	// point of OwnerGone rather than an oversight: "abandoned work is visible
	// rather than indistinguishable from healthy work" (section 16) is not
	// served by reporting that SOMEBODY died. It is empty only for a lease
	// nobody has ever taken.
	Holder   string
	Token    uint64
	Epoch    uint64
	Witness  Witness
	Deadline Instant
	// OwnerGone is true when the witness was OBSERVED dead. It is false for
	// an owner rig could not check, which is why NeedsBreak exists beside it.
	OwnerGone bool
	// Liveness is what was observed, so "not gone" and "could not tell" are
	// distinguishable rather than both reading as healthy.
	Liveness Liveness
	// NeedsBreak is true for an orphaned lease rig can never free on its own:
	// an unwitnessed one. It is a request for a recorded human action.
	NeedsBreak bool
	// BrokenBy and BrokenReason survive the break, so a lease that was taken
	// from somebody says who took it and why.
	BrokenBy     string `json:",omitempty"`
	BrokenReason string `json:",omitempty"`
}

// HeldError refuses an acquisition, naming the incumbent and its state.
//
// It is a DIFFERENT type from instance.HeldError, which refuses a second daemon
// in one runtime directory. Collapsing them would produce a message naming the
// wrong boundary - one is "somebody is already in this estate", this one is
// "somebody already holds this name inside it".
type HeldError struct {
	Status Status
}

func (e *HeldError) Error() string {
	switch {
	case e.Status.State == Orphaned && e.Status.NeedsBreak:
		return fmt.Sprintf(
			"the lease %q is ORPHANED, held by %q with no witness rig can poll: "+
				"its deadline passed but rig cannot observe whether the holder is "+
				"gone, so it will not free on its own and needs a recorded break",
			e.Status.Name, e.Status.Holder)
	case e.Status.State == Orphaned:
		return fmt.Sprintf(
			"the lease %q is ORPHANED, held by %q (%s, observed %s): its deadline "+
				"passed but the holder was not observed dead, so it is not free - "+
				"a stalled holder still has it",
			e.Status.Name, e.Status.Holder, e.Status.Witness.Describe(), e.Status.Liveness)
	case e.Status.State == Held && e.Status.OwnerGone:
		// Held and the holder is gone. Saying only "it is held" here would be
		// the Inspect/Leases defect again: rig knows the holder is dead and
		// the refusal would hide it. The wait is bounded and needs no break,
		// so the message says that rather than leaving it to be guessed.
		return fmt.Sprintf(
			"the lease %q is held by %q until its deadline, but the holder (%s) "+
				"was OBSERVED DEAD: the deadline is the contract, so the lease is "+
				"not free yet\n"+
				"       it frees itself on the first read after the deadline, and "+
				"needs no break",
			e.Status.Name, e.Status.Holder, e.Status.Witness.Describe())
	default:
		return fmt.Sprintf("the lease %q is held by %q (%s)",
			e.Status.Name, e.Status.Holder, e.Status.Witness.Describe())
	}
}

// FencedError refuses a handle that is no longer the current one.
type FencedError struct {
	Name string
	// What is "epoch" or "token".
	What string
	Want uint64
	Got  uint64
}

func (e *FencedError) Error() string {
	if e.What == "epoch" {
		return fmt.Sprintf(
			"the handle for %q carries epoch %d and rigd is on epoch %d: rigd has "+
				"restarted since this handle was issued\n"+
				"       acquire again. The epoch is bumped on every start, and nothing "+
				"distinguishes a planned restart from a crash - deliberately, because a "+
				"handle that trusted a restart it was told about would break the first "+
				"time it was told wrong",
			e.Name, e.Got, e.Want)
	}
	return fmt.Sprintf(
		"the handle for %q carries token %d and the current token is %d: this "+
			"lease has been granted again since",
		e.Name, e.Got, e.Want)
}

// NotHeldError means the lease is free and there was nothing to renew, release
// or break.
type NotHeldError struct{ Name string }

func (e *NotHeldError) Error() string { return fmt.Sprintf("the lease %q is not held", e.Name) }

// record is the stored form. The token survives a release on purpose: it is
// monotonic PER LEASE, so a handle from two holders ago can never match again.
type record struct {
	Name         string  `json:"name"`
	Holder       string  `json:"holder"`
	Token        uint64  `json:"token"`
	Epoch        uint64  `json:"epoch"`
	Witness      Witness `json:"witness"`
	BootID       string  `json:"boot_id"`
	Deadline     Instant `json:"deadline"`
	Released     bool    `json:"released"`
	BrokenBy     string  `json:"broken_by,omitempty"`
	BrokenReason string  `json:"broken_reason,omitempty"`
}

// evaluate derives the CURRENT state of a stored lease. It writes nothing.
//
// EXPIRY IS DERIVED ON READ RATHER THAN SWEPT BY A TIMER, and that is not a
// shortcut. A sweeper is a second clock: it can be late, it can be stopped by
// the very restart this package exists to survive, and it makes "expired" mean
// "expired and noticed". Deriving it means a lease that expired while rigd was
// down is expired the moment anybody looks, with no catch-up pass.
func (r *record) evaluate(at Instant, bootID string) (State, Liveness) {
	if r == nil || r.Released {
		return Free, LivenessUnknown
	}
	// THE WITNESS IS OBSERVED ON EVERY READ, INCLUDING A LEASE THAT IS STILL
	// INSIDE ITS DEADLINE, and skipping it here was a defect rather than an
	// optimisation. Section 16 asks a read to report "the owner's liveness",
	// and a lease that answered Alive because its deadline had not passed was
	// reporting an assumption in the field that is supposed to carry an
	// observation. It is the same mistake the zombie clause fixed one layer
	// down: claiming alive without looking.
	//
	// THE STATE AND THE LIVENESS ARE SEPARATE ANSWERS, which is why the fix
	// costs nothing elsewhere. The deadline is the contract and it alone
	// decides Held, so a holder that died one second into a thirty-second
	// lease keeps it for the remaining twenty-nine - nobody else may take it,
	// and the two-step rule is untouched. What changes is that OwnerGone is
	// true immediately instead of a TTL later, so abandoned work is visible
	// the moment anybody looks, which is the whole of what owner_gone is for.
	live := r.Witness.Observe(bootID)

	// A deadline from another boot is not late, it is meaningless: BOOTTIME
	// restarts at zero. Treat it as expired and let the witness decide, which
	// it will do instantly - a pid from a previous boot is dead by
	// construction.
	if r.BootID == bootID && at.Before(r.Deadline) {
		return Held, live
	}
	if live == Dead {
		return Free, Dead
	}
	// Alive, or could not be established. Neither frees it.
	return Orphaned, live
}

func (r *record) status(state State, live Liveness) Status {
	return Status{
		Name:         r.Name,
		State:        state,
		Holder:       r.Holder,
		Token:        r.Token,
		Epoch:        r.Epoch,
		Witness:      r.Witness,
		Deadline:     r.Deadline,
		OwnerGone:    live == Dead,
		Liveness:     live,
		NeedsBreak:   state == Orphaned && r.Witness.Kind == Unwitnessed,
		BrokenBy:     r.BrokenBy,
		BrokenReason: r.BrokenReason,
	}
}

func getRecord(tx *bolt.Tx, name string) (*record, error) {
	raw := tx.Bucket(bucketLeases).Get([]byte(name))
	if raw == nil {
		return nil, nil
	}
	var r record
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("coord: decoding the lease %q: %w", name, err)
	}
	return &r, nil
}

func putRecord(tx *bolt.Tx, r *record) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("coord: encoding the lease %q: %w", r.Name, err)
	}
	return tx.Bucket(bucketLeases).Put([]byte(r.Name), raw)
}

// Acquire takes the lease, or refuses with the incumbent's full status.
//
// A lease is granted when it is Free - which includes an Orphaned lease whose
// witness has been OBSERVED DEAD, and never one whose witness is alive or
// merely unknown.
//
// RE-ACQUIRING A LEASE YOU ALREADY HOLD IS ALLOWED AND IS THE RECONNECT PATH.
// After a restart every handle in the estate carries a stale epoch and is
// fenced, so the holder's only way back is to present itself again; §16 calls
// this "clients reconcile on reconnect rather than losing their place". It
// issues a NEW token, so a stalled goroutine inside that same holder is still
// fenced by the token even though the holder as a whole was readmitted.
func (s *Store) Acquire(name, holder string, w Witness, ttl time.Duration) (Handle, error) {
	if s == nil || s.db == nil {
		return Handle{}, ErrClosed
	}
	if name == "" || holder == "" {
		return Handle{}, fmt.Errorf("coord: a lease needs a name and a holder, got %q and %q", name, holder)
	}
	if ttl <= 0 {
		return Handle{}, fmt.Errorf("coord: the lease %q was asked for with ttl %s; "+
			"every lease has a TTL and there is no infinite hold", name, ttl)
	}
	at, err := now()
	if err != nil {
		return Handle{}, err
	}

	var h Handle
	err = s.db.Update(func(tx *bolt.Tx) error {
		r, err := getRecord(tx, name)
		if err != nil {
			return err
		}
		token := uint64(0)
		if r != nil {
			token = r.Token
			state, live := r.evaluate(at, s.bootID)
			if state != Free && r.Holder != holder {
				return &HeldError{Status: r.status(state, live)}
			}
		}
		next := &record{
			Name:     name,
			Holder:   holder,
			Token:    token + 1,
			Epoch:    s.epoch,
			Witness:  w,
			BootID:   s.bootID,
			Deadline: at.Add(ttl),
		}
		h = Handle{Name: name, Holder: holder, Token: next.Token, Epoch: next.Epoch, Deadline: next.Deadline}
		return putRecord(tx, next)
	})
	if err != nil {
		return Handle{}, err
	}
	return h, nil
}

// Renew extends a lease against the handle that holds it.
//
// AN ORPHANED LEASE IS RENEWABLE BY ITS OWN HOLDER, and that is the reason
// Orphaned is not Free. A holder that stalled past its deadline and woke up is
// exactly who the middle state was kept for: nobody else can have taken it, so
// handing it back is safe and losing the work is not.
func (s *Store) Renew(h Handle, ttl time.Duration) (Handle, error) {
	if s == nil || s.db == nil {
		return Handle{}, ErrClosed
	}
	if ttl <= 0 {
		return Handle{}, fmt.Errorf("coord: the lease %q was renewed with ttl %s; "+
			"every lease has a TTL and there is no infinite hold", h.Name, ttl)
	}
	if h.Epoch != s.epoch {
		return Handle{}, &FencedError{Name: h.Name, What: "epoch", Want: s.epoch, Got: h.Epoch}
	}
	at, err := now()
	if err != nil {
		return Handle{}, err
	}

	var out Handle
	err = s.db.Update(func(tx *bolt.Tx) error {
		r, err := getRecord(tx, h.Name)
		if err != nil {
			return err
		}
		if r == nil || r.Released {
			return &NotHeldError{Name: h.Name}
		}
		if r.Token != h.Token {
			return &FencedError{Name: h.Name, What: "token", Want: r.Token, Got: h.Token}
		}
		r.Deadline = at.Add(ttl)
		r.Epoch = s.epoch
		r.BootID = s.bootID
		out = Handle{Name: r.Name, Holder: r.Holder, Token: r.Token, Epoch: r.Epoch, Deadline: r.Deadline}
		return putRecord(tx, r)
	})
	if err != nil {
		return Handle{}, err
	}
	return out, nil
}

// Release gives the lease back.
//
// The record is kept rather than deleted, with Released set: the token must
// stay monotonic per lease, and a deleted record would restart it at 1 and make
// an ancient handle match again. Section 16 says the same of claims - "values
// are NEVER trimmed" - for the same reason.
func (s *Store) Release(h Handle) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	if h.Epoch != s.epoch {
		return &FencedError{Name: h.Name, What: "epoch", Want: s.epoch, Got: h.Epoch}
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		r, err := getRecord(tx, h.Name)
		if err != nil {
			return err
		}
		if r == nil || r.Released {
			return &NotHeldError{Name: h.Name}
		}
		if r.Token != h.Token {
			return &FencedError{Name: h.Name, What: "token", Want: r.Token, Got: h.Token}
		}
		r.Released = true
		r.Witness = NoWitness()
		return putRecord(tx, r)
	})
}

// Break frees a lease by a recorded human action, and refuses to be silent
// about it.
//
// IT IS THE ONLY WAY AN UNWITNESSED ORPHAN EVER BECOMES FREE. by and reason are
// required because a break that records neither is indistinguishable from an
// expiry afterwards, and the whole point of the unwitnessed rule is that
// somebody took responsibility for the judgement rig could not make.
//
// IT REFUSES A HELD LEASE. Breaking one that is still inside its deadline is
// not a recovery, it is taking a resource from a working holder, and if that is
// wanted the holder should be stopped.
func (s *Store) Break(name, by, reason string) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	if by == "" || reason == "" {
		return fmt.Errorf("coord: breaking the lease %q needs who is breaking it "+
			"and why: a break is a recorded human action, and one that records "+
			"neither cannot be told from an expiry afterwards", name)
	}
	at, err := now()
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		r, err := getRecord(tx, name)
		if err != nil {
			return err
		}
		if r == nil || r.Released {
			return &NotHeldError{Name: name}
		}
		if state, live := r.evaluate(at, s.bootID); state == Held {
			return &HeldError{Status: r.status(state, live)}
		}
		r.Released = true
		r.BrokenBy = by
		r.BrokenReason = reason
		r.Witness = NoWitness()
		return putRecord(tx, r)
	})
}

// Inspect reports a lease's state and its OWNER'S LIVENESS.
func (s *Store) Inspect(name string) (Status, error) {
	if s == nil || s.db == nil {
		return Status{}, ErrClosed
	}
	at, err := now()
	if err != nil {
		return Status{}, err
	}
	var st Status
	err = s.db.View(func(tx *bolt.Tx) error {
		r, err := getRecord(tx, name)
		if err != nil {
			return err
		}
		if r == nil {
			st = Status{Name: name, State: Free}
			return nil
		}
		state, live := r.evaluate(at, s.bootID)
		st = r.status(state, live)
		return nil
	})
	return st, err
}

// Leases reports every lease the store knows about, evaluated now.
//
// It reads /proc once per witness, which is what makes this the call a status
// surface makes rather than a loop over Inspect.
func (s *Store) Leases() ([]Status, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}
	at, err := now()
	if err != nil {
		return nil, err
	}
	var out []Status
	err = s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketLeases).ForEach(func(k, raw []byte) error {
			var r record
			if err := json.Unmarshal(raw, &r); err != nil {
				return fmt.Errorf("coord: decoding the lease %q: %w", k, err)
			}
			state, live := r.evaluate(at, s.bootID)
			out = append(out, r.status(state, live))
			return nil
		})
	})
	return out, err
}
