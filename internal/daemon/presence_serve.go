package daemon

import (
	"errors"
	"strconv"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// serveAnnounce takes a seat, or joins the roster without one.
func (d *Daemon) serveAnnounce(c *conn, f *rigv1.Frame) {
	var req rigv1.AnnounceRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "announce: "+err.Error())
		return
	}
	if req.GetPurpose() == "" {
		// A row with no purpose is the defect this mechanism was built to fix,
		// so it is refused at the boundary rather than rendered as a blank.
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"announce: empty purpose. The purpose is the headline of this "+
				"peer's row and a blank one is indistinguishable from a "+
				"session nobody is supervising")
		return
	}
	if seat := req.GetSeat(); seat != "" {
		// A seat is an address other peers will type, so it is held to the
		// same shape as an estate name rather than to a looser rule invented
		// here. One vocabulary, one validator, one place to change it.
		if err := paths.ValidEstateName(seat); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
				"announce: invalid seat name: "+err.Error())
			return
		}
	}

	o, err := d.presence.announce(c.occ, req.GetSeat(), req.GetPurpose(), req.GetActivity())
	if err != nil {
		var held *seatHeldError
		if ok := asSeatHeld(err, &held); ok {
			// SECTION 9: the failed precondition, the state actually found,
			// and a fix the caller can run. The fix is deliberately not "try
			// again" - the seat is held by something alive, so retrying is
			// the wrong action and naming the holder is the right one.
			//
			// IT USED TO SAY "Run `rig peers` to see the roster" AND THAT
			// VERB DOES NOT EXIST. cmd/rig dispatches version, ping, apps,
			// down, estate, describe, mcp, completion and help; `peers` is a
			// forward citation to the M7 peers service and falls through to
			// the DEFAULT branch, which reads the first token as a PROGRAM
			// name - so a caller following the fix was told its program does
			// not exist, and the failure read as its own fault rather than
			// this message's. `rig apps` lists programs, not peers, so no
			// existing command could satisfy the advice either. The missing
			// verb is tracked as B41 and is not closed by deleting the
			// citation; serving a fix nobody can run is what is fixed here.
			//
			// THE UNSEATED OFFER STAYS, AND THAT IS NOT AN OVERSIGHT. This
			// text is served to a WIRE caller only - the MCP door carries its
			// own refusal, which names `list_agents`, a tool that caller
			// actually holds. Requiring a seat is a DOOR rule precisely so
			// the program socket keeps serving unseated programs, so
			// announcing without a seat is still valid for everything that
			// can reach this message.
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
				"announce: seat "+held.Seat+" is already held by a live peer "+
					"at generation "+strconv.FormatUint(held.Generation, 10)+
					" whose purpose is "+strconv.Quote(held.Purpose)+
					". Announce without a seat if you are not taking this "+
					"one, or agree with the holder that it should stand "+
					"down first")
			return
		}
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, "announce: "+err.Error())
		return
	}

	c.reply(f.GetStreamId(), &rigv1.AnnounceResponse{
		You:     o.proto(),
		Crew:    d.presence.crew(),
		Partial: d.presence.partial(),
	})
}

// serveActivity updates what this peer is doing now.
func (d *Daemon) serveActivity(c *conn, f *rigv1.Frame) {
	var req rigv1.ActivityRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "activity: "+err.Error())
		return
	}

	o, ok := d.presence.setActivity(c.occ, req.GetActivity(), req.GetState())
	if !ok {
		// ANNOUNCE FIRST IS A REAL ORDERING AND NOT A FORMALITY: an activity
		// line with no purpose above it is exactly the unsupervisable row the
		// seat mechanism exists to prevent, so the daemon refuses to create
		// one by side effect.
		// CODE_INVALID rather than a precondition code of its own, because
		// this wire has none and inventing one for a single call site is a
		// wire change section 21 would have to carry forever. The section 9
		// fields say what was wrong; the code says only how wrong.
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"activity: this connection has not announced, so it has no row "+
				"to update. Call rig.announce with a purpose first")
		return
	}
	c.reply(f.GetStreamId(), &rigv1.ActivityResponse{You: o.proto()})
}

// servePeers reads the roster.
func (d *Daemon) servePeers(c *conn, f *rigv1.Frame) {
	c.reply(f.GetStreamId(), &rigv1.PeersResponse{
		Crew:    d.presence.crew(),
		Partial: d.presence.partial(),
	})
}

// asSeatHeld is errors.As for the one error type announce refuses with. A bare
// type assertion was here and errorlint refused it: it misses the error the
// moment anything between presence and this handler wraps it.
func asSeatHeld(err error, target **seatHeldError) bool {
	return errors.As(err, target)
}
