package daemon

import (
	"encoding/json"
	"strings"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/paths"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// plan/53, slice 1: AgentBox's signals on section 52's bus. A seat posts
// signal.<words>, and rig itself posts lease.changed and roster.changed, so
// "wake me when the deploy lease is free" or "when backend-2 leaves" is one
// events.wait rather than a poll with a model turn in each step.
//
// What it improves on AgentBox's post_signal: the poster is the seat the
// daemon knows, never a key the request carries; an addressed signal is
// seen by that seat alone, not by anyone who waits on its topic; and a
// restart is a gap the waiter is told about, by section 52's epoch.

// serveSignalPublish publishes a seat's signal.
func (d *Daemon) serveSignalPublish(c *conn, id uint32, req *registryv1.EventsPublishRequest) {
	kind := req.GetKind()
	root, words, _ := strings.Cut(kind, ".")
	if root != "signal" {
		c.fail(id, rigv1.Code_CODE_INVALID,
			"rig.events.publish: to_seat is for a signal only, and "+quoteKind(kind)+" is not signal.<words>")
		return
	}
	if len(kind) > kernel.MaxEventKind || words == "" || strings.HasSuffix(kind, ".*") {
		c.fail(id, rigv1.Code_CODE_INVALID,
			"rig.events.publish: a signal is signal.<words> of at most 128 bytes, like signal.tests.green; "+
				"a prefix ending .* is for waiting, never for posting")
		return
	}
	if bad := kernel.BadEventWords(words); bad != "" {
		c.fail(id, rigv1.Code_CODE_INVALID, "rig.events.publish: "+quoteKind(kind)+" "+bad)
		return
	}
	_, seat, _, ok := d.provenance(c)
	if !ok {
		c.failStatus(id, &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig.events.publish: a signal needs a seat, so its waiters know who sent it",
			Precondition: "the caller announced into a named seat",
			Actual:       "this connection holds no seat",
			Fix:          "call announce with a seat first; the sender is the daemon's to name, never the request's",
		})
		return
	}
	to := req.GetToSeat()
	if to != "" {
		if err := paths.ValidEstateName(to); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "rig.events.publish: to_seat: "+err.Error())
			return
		}
		to = seatSource(to)
	}
	if !payloadOK(c, id, req.GetPayloadJson()) {
		return
	}
	ev, delivered, err := d.events.publishTo(kind, seatSource(seat), req.GetPayloadJson(), to)
	if err != nil {
		c.fail(id, rigv1.Code_CODE_INTERNAL, "rig.events.publish: the signal could not be stored, so nobody was woken: "+err.Error())
		return
	}
	c.reply(id, &registryv1.EventsPublishResponse{Event: ev, Delivered: delivered})
}

// leaseChange is lease.changed's payload.
type leaseChange struct {
	Name   string `json:"name"`
	Change string `json:"change"` // acquired, released, broken, queued, orphaned, fenced or expired
	Holder string `json:"holder,omitempty"`
	Token  uint64 `json:"token,omitempty"`
	By     string `json:"by,omitempty"` // who broke it, or who queued
	// Because is why an acquired lease became the holder's: free, released,
	// broken or expired (plan/53 slice 2).
	Because    string `json:"because,omitempty"`
	NeedsBreak bool   `json:"needs_break,omitempty"`
}

// rosterChange is roster.changed's payload.
type rosterChange struct {
	Seat    string `json:"seat"`
	Change  string `json:"change"` // announced or left
	Purpose string `json:"purpose,omitempty"`
}

// publishJSON publishes one of rig's kinds with v as its payload. A value
// that does not render goes with none: the waiter still knows to re-read.
func (d *Daemon) publishJSON(kind string, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		raw = nil
	}
	d.events.publish(kind, eventSourceRig, string(raw))
}
