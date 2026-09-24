package daemon

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 16's leases, served. internal/coord is the whole mechanism - TTL,
// two-step expiry, the liveness witness, epoch fencing - and this file only
// decides the two things the wire adds: WHO is holding, and WHAT witnesses
// them. Both are the connection's and neither is read off the request.
//
//   - The holder is the caller's seat, from the same provenance a record
//     write takes, so a lease and a record written by one seat name it the
//     same way and a reconnecting seat gets its own lease back.
//   - The witness is the pid SO_PEERCRED gave for the caller. A pid on the
//     request would let a caller pin a lease to a process it does not own,
//     and a lease on a pid that never dies never frees.

// serveLease dispatches the five lease verbs.
func (d *Daemon) serveLease(c *conn, f *rigv1.Frame, command string) {
	if d.leases == nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code: rigv1.Code_CODE_UNAVAILABLE,
			Message: "rig." + command + ": this estate has no lease store: an " +
				"unnamed estate keeps no persistent state, and a lease that " +
				"did not survive a restart could not fence one",
			Precondition: "rigd was started with --estate",
			Actual:       "this daemon serves an unnamed estate",
			Fix:          "start rigd with --estate <name>",
		})
		return
	}
	switch command {
	case "lease.list":
		d.serveLeaseList(c, f)
	case "lease.acquire":
		d.serveLeaseAcquire(c, f)
	case "lease.renew":
		d.serveLeaseRenew(c, f)
	case "lease.release":
		d.serveLeaseRelease(c, f)
	case "lease.break":
		d.serveLeaseBreak(c, f)
	case "lease.check":
		d.serveLeaseCheck(c, f)
	}
}

func (d *Daemon) serveLeaseList(c *conn, f *rigv1.Frame) {
	all, err := d.leases.Leases()
	if err != nil {
		c.failErr(f.GetStreamId(), leaseCode(err), err)
		return
	}
	at, err := coord.Now()
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	resp := &verbsv1.LeaseListResponse{}
	for _, st := range all {
		resp.Leases = append(resp.Leases, leaseToWire(st, at))
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) serveLeaseAcquire(c *conn, f *rigv1.Frame) {
	var req verbsv1.LeaseAcquireRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "lease.acquire: "+err.Error())
		return
	}
	if !leaseTextOK(c, f, "lease.acquire", "name", req.GetName()) {
		return
	}
	if coord.IsClaimLease(req.GetName()) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, fmt.Sprintf(
			"rig.lease.acquire: %q is a queue claim's name, and a claim is taken "+
				"with rig.queue.claim; holding it directly would block a task "+
				"nobody is working on", req.GetName()))
		return
	}
	_, seat, _, ok := d.provenance(c)
	if !ok {
		refuseUnseatedLease(c, f, "lease.acquire")
		return
	}
	w := coord.NoWitness()
	if !req.GetUnwitnessed() {
		var err error
		if w, err = coord.WitnessProcess(c.principal().PID); err != nil {
			c.failStatus(f.GetStreamId(), &rigv1.Status{
				Code: rigv1.Code_CODE_INVALID,
				Message: "lease.acquire: rig could not witness this caller's " +
					"process: " + err.Error(),
				Precondition: "the socket reports a live pid for the caller",
				Actual:       "no pid rig can poll",
				Fix: "acquire with unwitnessed set, which means the lease " +
					"will need a recorded break if you vanish",
			})
			return
		}
	}
	h, err := d.leases.Acquire(req.GetName(), seat, w, ttlOf(req.GetTtlMs()))
	if err != nil {
		c.failErr(f.GetStreamId(), leaseCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.LeaseAcquireResponse{Handle: handleToWire(h)})
}

func (d *Daemon) serveLeaseRenew(c *conn, f *rigv1.Frame) {
	var req verbsv1.LeaseRenewRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "lease.renew: "+err.Error())
		return
	}
	h, err := d.leases.Renew(coord.Handle{
		Name: req.GetName(), Token: req.GetToken(), Epoch: req.GetEpoch(),
	}, ttlOf(req.GetTtlMs()))
	if err != nil {
		c.failErr(f.GetStreamId(), leaseCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.LeaseRenewResponse{Handle: handleToWire(h)})
}

func (d *Daemon) serveLeaseRelease(c *conn, f *rigv1.Frame) {
	var req verbsv1.LeaseReleaseRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "lease.release: "+err.Error())
		return
	}
	if err := d.leases.Release(coord.Handle{
		Name: req.GetName(), Token: req.GetToken(), Epoch: req.GetEpoch(),
	}); err != nil {
		c.failErr(f.GetStreamId(), leaseCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.LeaseReleaseResponse{})
}

// serveLeaseBreak records WHO broke a lease as the caller's seat. A break is
// a recorded human action, and a break by nobody in particular cannot be
// argued with afterwards.
func (d *Daemon) serveLeaseBreak(c *conn, f *rigv1.Frame) {
	var req verbsv1.LeaseBreakRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "lease.break: "+err.Error())
		return
	}
	if !leaseTextOK(c, f, "lease.break", "name", req.GetName()) ||
		!leaseTextOK(c, f, "lease.break", "reason", req.GetReason()) {
		return
	}
	_, seat, _, ok := d.provenance(c)
	if !ok {
		refuseUnseatedLease(c, f, "lease.break")
		return
	}
	if err := d.leases.Break(req.GetName(), seat, req.GetReason()); err != nil {
		c.failErr(f.GetStreamId(), leaseCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.LeaseBreakResponse{})
}

// serveLeaseCheck answers whether a fencing token is current. It needs no
// seat: the caller is typically a resource checking somebody else's token.
func (d *Daemon) serveLeaseCheck(c *conn, f *rigv1.Frame) {
	var req verbsv1.LeaseCheckRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "lease.check: "+err.Error())
		return
	}
	if !leaseTextOK(c, f, "lease.check", "name", req.GetName()) {
		return
	}
	st, current, err := d.leases.Check(req.GetName(), req.GetToken())
	if err != nil {
		c.failErr(f.GetStreamId(), leaseCode(err), err)
		return
	}
	at, err := coord.Now()
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.LeaseCheckResponse{Current: current, Lease: leaseToWire(st, at)})
}

func refuseUnseatedLease(c *conn, f *rigv1.Frame, command string) {
	c.failStatus(f.GetStreamId(), &rigv1.Status{
		Code: rigv1.Code_CODE_DENIED,
		Message: "rig." + command + ": this connection holds no seat, so the " +
			"lease could not say who holds or broke it",
		Precondition: "the caller announced into a named seat",
		Actual:       "this connection has not announced, or announced without a seat",
		Fix: "call rig.announce on this connection first, with a seat. The " +
			"holder is the daemon's to name and is never read off the request",
	})
}

// maxLeaseText bounds a lease name and a break reason. Both are stored and
// echoed in every lease.list, so an unbounded one is a caller growing every
// other caller's answer; a name is an identifier and a reason a sentence.
const maxLeaseText = 256

// leaseTextOK refuses a name or reason over maxLeaseText by name.
func leaseTextOK(c *conn, f *rigv1.Frame, command, what, v string) bool {
	if len(v) <= maxLeaseText {
		return true
	}
	c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, fmt.Sprintf(
		"rig.%s: the %s is %d bytes, over the %d-byte bound", command, what, len(v), maxLeaseText))
	return false
}

// ttlOf turns the wire's milliseconds into a duration. Zero stays zero, and
// coord refuses it by name: there is no infinite hold.
func ttlOf(ms uint32) time.Duration { return time.Duration(ms) * time.Millisecond }

// leaseCode maps coord's refusals onto the wire's codes.
func leaseCode(err error) rigv1.Code {
	var held *coord.HeldError
	var fenced *coord.FencedError
	var notHeld *coord.NotHeldError
	switch {
	case errors.As(err, &held), errors.As(err, &fenced):
		return rigv1.Code_CODE_CONFLICT
	case errors.As(err, &notHeld):
		return rigv1.Code_CODE_NOT_FOUND
	case errors.Is(err, coord.ErrClosed):
		return rigv1.Code_CODE_UNAVAILABLE
	default:
		return rigv1.Code_CODE_INVALID
	}
}

func handleToWire(h coord.Handle) *verbsv1.LeaseHandle {
	at, err := coord.Now()
	remaining := int64(0)
	if err == nil {
		remaining = h.Deadline.Sub(at).Milliseconds()
	}
	return &verbsv1.LeaseHandle{
		Name: h.Name, Holder: h.Holder, Token: h.Token, Epoch: h.Epoch,
		RemainingMs: remaining,
	}
}

var leaseStateWire = map[coord.State]verbsv1.LeaseState{
	coord.Held:     verbsv1.LeaseState_LEASE_STATE_HELD,
	coord.Orphaned: verbsv1.LeaseState_LEASE_STATE_ORPHANED,
	coord.Free:     verbsv1.LeaseState_LEASE_STATE_FREE,
}

var livenessWire = map[coord.Liveness]verbsv1.Liveness{
	coord.LivenessUnknown: verbsv1.Liveness_LIVENESS_UNKNOWN,
	coord.Alive:           verbsv1.Liveness_LIVENESS_ALIVE,
	coord.Dead:            verbsv1.Liveness_LIVENESS_DEAD,
}

// leaseToWire renders a status. A lease nobody ever took has no deadline, and
// its distance is left at zero rather than computed from nothing.
func leaseToWire(st coord.Status, at coord.Instant) *verbsv1.Lease {
	out := &verbsv1.Lease{
		Name:         st.Name,
		State:        leaseStateWire[st.State],
		Holder:       st.Holder,
		Token:        st.Token,
		Epoch:        st.Epoch,
		OwnerGone:    st.OwnerGone,
		Liveness:     livenessWire[st.Liveness],
		NeedsBreak:   st.NeedsBreak,
		BrokenBy:     st.BrokenBy,
		BrokenReason: st.BrokenReason,
	}
	if st.Holder != "" {
		out.Witness = st.Witness.Describe()
		out.RemainingMs = st.Deadline.Sub(at).Milliseconds()
	}
	return out
}
