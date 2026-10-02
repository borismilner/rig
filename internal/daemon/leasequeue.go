package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/rig/internal/coord"
)

// plan/53 slice 2: lease.acquire may queue, which is what AgentBox's
// acquire_lock gives an agent and rig's lease did not. A waiter parks in a
// FIFO queue per lease; a lease that frees goes to the queue's head, never
// to whoever happens to ask next; a wait that would close a cycle is refused
// by name.
//
// What it keeps from section 16: the lease itself is still coord's, durable
// and fenced, and the queue is only who is waiting. That is connection state,
// so it lives here in memory and dies with its waiter, as a seat does.
//
// What needs a clock: expiry is derived on read (coord), but a waiter, and
// anybody watching lease.changed, must be told when a lease frees by itself.
// So rigd looks at each lease it knows to be taken at its deadline, and then
// at an orphan's witness once a second until it is observed dead.

const (
	// maxLeaseWait bounds one park. AgentBox's bound, for its reason: an MCP
	// client abandons a call it has heard nothing about for 30 minutes.
	maxLeaseWait = 25 * time.Minute
	// orphanPoll is how often an orphan's witness is looked at.
	orphanPoll = time.Second
	// maxCycleDepth bounds the deadlock walk.
	maxCycleDepth = 32
)

// Why a lease became the caller's, as granted_because says it.
const (
	grantFree     = "free"
	grantReleased = "released"
	grantBroken   = "broken"
	grantExpired  = "expired"
	grantYours    = "already yours"
)

type leaseWaiter struct {
	seat  string
	w     coord.Witness
	ttl   time.Duration
	grant chan leaseGrant // buffered: a grant never blocks whoever frees it
}

type leaseGrant struct {
	h       coord.Handle
	because string
	err     error
}

// leaseSeen is what the watcher last knew of a lease.
type leaseSeen struct {
	state  coord.State
	holder string
}

type leaseQueue struct {
	mu    sync.Mutex
	q     map[string][]*leaseWaiter
	known map[string]leaseSeen
	next  map[string]time.Time // when the watcher looks again
	kick  chan struct{}
}

func newLeaseQueue() *leaseQueue {
	return &leaseQueue{
		q:     map[string][]*leaseWaiter{},
		known: map[string]leaseSeen{},
		next:  map[string]time.Time{},
		kick:  make(chan struct{}, 1),
	}
}

// wallTime is a boot-clock deadline as a wall-clock instant.
func wallTime(at coord.Instant) time.Time {
	now, err := coord.Now()
	if err != nil {
		return time.Now()
	}
	return time.Now().Add(at.Sub(now))
}

// noteLocked records what a verb or the watcher left a lease as, and when to
// look at it next. A free lease is not looked at again until somebody takes it.
func (q *leaseQueue) noteLocked(st coord.Status) {
	q.known[st.Name] = leaseSeen{state: st.State, holder: st.Holder}
	switch {
	case st.State == coord.Free:
		delete(q.next, st.Name)
	case st.State == coord.Held:
		q.next[st.Name] = wallTime(st.Deadline)
	case st.NeedsBreak:
		// Unwitnessed and orphaned: only a renew or a break changes it, and
		// both are verbs that note it themselves.
		delete(q.next, st.Name)
	default:
		q.next[st.Name] = time.Now().Add(orphanPoll)
	}
	select {
	case q.kick <- struct{}{}:
	default:
	}
}

func (q *leaseQueue) waitingLocked(name string) []string {
	ws := q.q[name]
	if len(ws) == 0 {
		return nil
	}
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.seat
	}
	return out
}

func (q *leaseQueue) waiting(name string) []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.waitingLocked(name)
}

// removeLocked takes w out of name's queue, reporting whether it was there.
// False means it was granted, and its grant is in its channel.
func (q *leaseQueue) removeLocked(name string, w *leaseWaiter) bool {
	ws := q.q[name]
	for i, x := range ws {
		if x == w {
			q.q[name] = append(ws[:i:i], ws[i+1:]...)
			if len(q.q[name]) == 0 {
				delete(q.q, name)
			}
			return true
		}
	}
	return false
}

// handOverLocked gives a free lease to its queue's head, and to the next if
// the head cannot take it. It stops at the first grant, or when the lease is
// held after all.
func (d *Daemon) handOverLocked(name, because string) {
	for len(d.lq.q[name]) > 0 {
		head := d.lq.q[name][0]
		h, err := d.leases.Acquire(name, head.seat, head.w, head.ttl)
		var held *coord.HeldError
		if errors.As(err, &held) {
			d.lq.noteLocked(held.Status)
			return
		}
		d.lq.removeLocked(name, head)
		head.grant <- leaseGrant{h: h, because: because, err: err}
		if err == nil {
			d.lq.noteLocked(coord.Status{Name: name, State: coord.Held, Holder: h.Holder, Deadline: h.Deadline})
			d.publishJSON("lease.changed", leaseChange{Name: name, Change: "acquired", Holder: h.Holder, Token: h.Token, Because: because})
			return
		}
	}
}

// lookLocked evaluates one lease as the watcher does: it tells lease.changed
// what happened by itself since a verb last touched it, and hands a lease
// that freed to its queue.
func (d *Daemon) lookLocked(name string) {
	st, err := d.leases.Inspect(name)
	if err != nil {
		// The store is closing or unreadable; try again shortly rather than
		// forget a lease somebody may be waiting on.
		d.lq.next[name] = time.Now().Add(orphanPoll)
		return
	}
	was := d.lq.known[name]
	switch st.State {
	case coord.Orphaned:
		if was.state != coord.Orphaned {
			d.publishJSON("lease.changed", leaseChange{Name: name, Change: "orphaned", Holder: st.Holder, Token: st.Token, NeedsBreak: st.NeedsBreak})
		}
		// A wrapped run's lease ran out with its holder alive but silent: a
		// stalled or stopped `rig peers run`. Its work is stopped before the
		// lease can pass on, which is the fence (plan/53 slice 6), and the
		// next look finds the holder dead and frees it as expired.
		if st.Fence != nil {
			run, holder := st.Fence.Kill(true), st.Witness.Kill(false)
			if !run && !holder {
				break
			}
			d.publishJSON("lease.changed", leaseChange{Name: name, Change: "fenced", Holder: st.Holder, Token: st.Token})
		}
	case coord.Free:
		if was.state == coord.Held || was.state == coord.Orphaned {
			d.publishJSON("lease.changed", leaseChange{Name: name, Change: "expired", Holder: st.Holder, Token: st.Token})
		}
	case coord.Held:
		if was.state != "" && was.state != coord.Free && was.holder != st.Holder {
			// Somebody took it over between two looks: it expired first.
			d.publishJSON("lease.changed", leaseChange{Name: name, Change: "expired", Holder: was.holder})
		}
	}
	d.lq.noteLocked(st)
	if st.State == coord.Free {
		d.handOverLocked(name, grantExpired)
	}
}

// startLeaseWatch seeds the watcher from the store and runs it while serving.
func (d *Daemon) startLeaseWatch(ctx context.Context) {
	if d.leases == nil {
		return
	}
	if all, err := d.leases.Leases(); err == nil {
		d.lq.mu.Lock()
		for _, st := range all {
			if st.State != coord.Free && !coord.IsClaimLease(st.Name) {
				d.lq.noteLocked(st)
			}
		}
		d.lq.mu.Unlock()
	}
	go d.leaseWatch(ctx)
}

func (d *Daemon) leaseWatch(ctx context.Context) {
	t := time.NewTimer(time.Hour)
	defer t.Stop()
	for {
		d.lq.mu.Lock()
		now := time.Now()
		for name, at := range d.lq.next {
			if !at.After(now) {
				d.lookLocked(name)
			}
		}
		wait := time.Hour
		for _, at := range d.lq.next {
			wait = min(wait, max(time.Until(at), 10*time.Millisecond))
		}
		d.lq.mu.Unlock()
		t.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-d.lq.kick:
			if !t.Stop() {
				<-t.C
			}
		}
	}
}

// cycleLocked names the chain when seat queueing on name, held by holder,
// would wait on itself through the leases others are queued on. Empty when
// there is no cycle.
func (d *Daemon) cycleLocked(seat, name, holder string) string {
	chain := []string{seat + " would wait on " + name + ", held by " + holder}
	seen := map[string]bool{}
	var walk func(cur string, depth int) bool
	walk = func(cur string, depth int) bool {
		if cur == seat {
			return true
		}
		if seen[cur] || depth >= maxCycleDepth {
			return false
		}
		seen[cur] = true
		for lease, ws := range d.lq.q {
			for _, w := range ws {
				if w.seat != cur {
					continue
				}
				st, err := d.leases.Inspect(lease)
				if err != nil || st.State == coord.Free {
					continue
				}
				chain = append(chain, cur+" waits on "+lease+", held by "+st.Holder)
				if walk(st.Holder, depth+1) {
					return true
				}
				chain = chain[:len(chain)-1]
			}
		}
		return false
	}
	if walk(holder, 0) {
		return strings.Join(chain, "; ")
	}
	return ""
}
