package daemon

import (
	"context"
	"strconv"
	"time"

	"github.com/borismilner/rig/internal/observe"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Time-boxed grants (plan/05 section 5m, G1-G8). Boris approves ONE window
// for a seat, and that seat's righand scripts run inside it without a
// countdown each. The grant lives beside the run, not in its phase: it is live
// between two scripts too, which is the whole point of it.
//
//	none -> asking -> live -> none
//
// G2. Only he opens it: hand.answer takes grant30, grant60 or grant5m, and
// that verb is refused to a program and an agent (H5). The seat asking has
// no verb that makes its grant live, longer, or renewed.
// G3. Silence declines. A countdown may default to consent because the
// run is bounded by his Stop; a window he never saw is not.
// G4. It ends at its deadline, on his stop or pause (at once), when the seat
// gives it back, or when the session that asked for it is gone.
// G5. A script running when it ends is not cut mid-step: the run is marked,
// and the next hand.step gate, the one before each step, ends it.
//
// As the countdown's timed transitions are, every one of these is made by the
// caller blocked on it or by changed's timer; nothing here is a goroutine.

// handAuditClient is whose usage-log lines the desk's events are.
const handAuditClient = "rigd"

const (
	handGrantAsk   = 60 * time.Second
	handGrantLeast = time.Second
	handGrantMost  = 5 * time.Minute
)

// How a grant ends, in his words and in the refusal its seat reads.
const (
	handGrantDeclined = "declined"
	handGrantLapsed   = "not answered, so declined"
	handGrantExpired  = "ran out"
	handGrantStopped  = "stopped"
	handGrantPaused   = "revoked by pause"
	handGrantReleased = "given back"
	handGrantLeft     = "the session left"
)

// handGrant is one seat's window. owner is what its end is tied to: the
// session's occupancy for an agent, nil for a terminal, which has no session
// to outlive.
type handGrant struct {
	owner         any
	seat          string
	reason, scope string
	asked         time.Duration
	live          bool
	deadline      time.Time
	window        time.Duration
	ended         string
}

// matches is true when the grant is live and is seat's.
func (g *handGrant) matches(seat string) bool {
	return g != nil && g.live && seat != "" && g.seat == seat
}

// fillGrant puts the grant into a snapshot.
func (h *handDesk) fillGrant(st *registryv1.HandState) {
	g := h.grant
	if g == nil {
		st.GrantPhase = registryv1.HandGrantPhase_HAND_GRANT_PHASE_NONE
		return
	}
	st.GrantPhase = registryv1.HandGrantPhase_HAND_GRANT_PHASE_ASKING
	if g.live {
		st.GrantPhase = registryv1.HandGrantPhase_HAND_GRANT_PHASE_LIVE
	}
	st.GrantSeat, st.GrantReason, st.GrantScope = g.seat, g.reason, g.scope
	st.GrantDeadlineUnixNano, st.GrantWindowMs = g.deadline.UnixNano(), ms(g.window)
}

// auditf hands an event to the daemon's usage log. Called with mu held.
func (h *handDesk) auditf(op string, attrs map[string]string) {
	if h.audit != nil {
		h.audit(op, attrs)
	}
}

func (g *handGrant) attrs() map[string]string {
	return map[string]string{
		"seat": g.seat, "reason": g.reason, "scope": g.scope,
		"window_s": strconv.FormatInt(int64(g.window/time.Second), 10),
	}
}

// endGrant closes the grant. cut says what happens to a run under it: true
// marks the run to end at its next step (G5), false lets it carry on as an
// ordinary run, which is what he gets when he pauses and then resumes it.
// Called with mu held.
func (h *handDesk) endGrant(why string, cut bool) {
	g := h.grant
	if g == nil {
		return
	}
	g.ended = why
	h.lastGrant, h.grant = g, nil
	if h.run == nil {
		// The strip lingers on how it ended, as it does after a run.
		h.st.Holder, h.st.Ended = g.seat, "the grant "+why
	}
	a := g.attrs()
	a["how"] = why
	h.auditf("grant ended", a)
	if r := h.run; r != nil && r.underGrant {
		if cut {
			r.cut = "the grant " + why
		} else {
			r.underGrant = false
		}
	}
	h.changed()
}

// expireGrant makes the transition the grant's deadline owes. Called with mu
// held.
func (h *handDesk) expireGrant() {
	g := h.grant
	if g == nil || h.now().Before(g.deadline) {
		return
	}
	if g.live {
		h.endGrant(handGrantExpired, true)
	} else {
		h.endGrant(handGrantLapsed, false)
	}
}

// grantAsk opens the ask, or refuses while the desktop is in use.
func (h *handDesk) grantAsk(owner any, seat, reason, scope string, asked time.Duration) (*handGrant, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expire()
	switch {
	case seat == "":
		return nil, handRefused(rigv1.Code_CODE_DENIED,
			"rig.hand.grant: this connection holds no seat, so the grant could not say whose it is")
	case h.grant != nil:
		// A live grant is never extended by asking again (G2): the next
		// window is a new ask that he sees.
		return nil, handRefused(rigv1.Code_CODE_CONFLICT,
			"rig.hand.grant: a grant for "+h.grant.seat+" is already "+h.grantWord()+
				"; one at a time, and a live one is never renewed - ask again once it has ended")
	case h.run != nil:
		return nil, handRefused(rigv1.Code_CODE_CONFLICT,
			"rig.hand.grant: the desktop is already asked for by "+h.st.GetHolder())
	}
	g := &handGrant{
		owner: owner, seat: seat, reason: reason, scope: scope, asked: asked,
		deadline: h.now().Add(handGrantAsk), window: handGrantAsk,
	}
	h.grant = g
	h.auditf("grant asked", g.attrs())
	h.changed()
	return g, nil
}

func (h *handDesk) grantWord() string {
	if h.grant != nil && h.grant.live {
		return "live"
	}
	return "asked for"
}

// grantAnswered blocks through the ask and answers once g is live, or why it
// never will. A caller that goes away while it waits withdraws its ask.
func (h *handDesk) grantAnswered(ctx context.Context, g *handGrant) (*registryv1.HandState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		h.expire()
		switch {
		case g.ended != "":
			return nil, handRefused(rigv1.Code_CODE_DENIED, "rig.hand.grant: the grant was "+g.ended)
		case g.live:
			return h.snapshot(), nil
		}
		if !h.await(ctx) {
			if h.grant == g {
				h.endGrant(handGrantLeft, false)
			}
			return nil, ctx.Err()
		}
	}
}

// grantChosen is his answer to an ask: the window he picks, never longer
// than five minutes whatever was asked.
func (h *handDesk) grantChosen(d time.Duration) {
	g := h.grant
	g.live, g.window = true, d
	g.deadline = h.now().Add(d)
	h.auditf("grant live", g.attrs())
	h.changed()
}

// ungrant gives seat's own grant back.
func (h *handDesk) ungrant(seat string) (*registryv1.HandState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expire()
	if g := h.grant; g != nil {
		if g.seat != seat {
			return nil, handRefused(rigv1.Code_CODE_DENIED,
				"rig.hand.ungrant: the grant is "+g.seat+"'s; only that seat gives it back, and he can stop it")
		}
		h.endGrant(handGrantReleased, true)
	}
	return h.snapshot(), nil
}

// dropGrant ends a grant tied to a session that is gone (G4).
func (h *handDesk) dropGrant(owner any) {
	if owner == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.grant != nil && h.grant.owner == owner {
		h.endGrant(handGrantLeft, true)
	}
}

// grantWindow is what a button of his chooses, or zero for any other action.
func grantWindow(a registryv1.HandAction) time.Duration {
	switch a {
	case registryv1.HandAction_HAND_ACTION_GRANT_30S:
		return 30 * time.Second
	case registryv1.HandAction_HAND_ACTION_GRANT_60S:
		return 60 * time.Second
	case registryv1.HandAction_HAND_ACTION_GRANT_5M:
		return handGrantMost
	}
	return 0
}

// serveHandGrant is hand.grant: ask, and block through the ask.
func (d *Daemon) serveHandGrant(ctx context.Context, c *conn, req *registryv1.HandGrantRequest) (*registryv1.HandState, error) {
	asked := time.Duration(req.GetDurationMs()) * time.Millisecond
	if asked < handGrantLeast || asked > handGrantMost {
		return nil, handRefused(rigv1.Code_CODE_INVALID,
			"rig.hand.grant: ask for 1 second to 5 minutes; he picks 30 seconds, 60 seconds or five minutes")
	}
	_, seat, _, _ := d.provenance(c)
	// An agent's session outlives one call, so the grant ends with the
	// session; a terminal has nothing to outlive.
	var owner any
	if c.agentDoor {
		owner = c.occ
	}
	g, err := d.hand.grantAsk(owner, seat, clip(req.GetReason()), clip(req.GetScope()), asked)
	if err != nil {
		return nil, err
	}
	return d.hand.grantAnswered(ctx, g)
}

// serveHandUngrant is hand.ungrant.
func (d *Daemon) serveHandUngrant(c *conn) (*registryv1.HandState, error) {
	_, seat, _, _ := d.provenance(c)
	return d.hand.ungrant(seat)
}

// auditHand writes a desk event as a usage-log line, shape only: who, why, how
// long, never typed text.
func (d *Daemon) auditHand(op string, attrs map[string]string) {
	if d.logs == nil {
		return
	}
	d.logs.Append(observe.Record{
		At: d.hand.now().UnixNano(), Client: handAuditClient, Message: "hand " + op, Attrs: attrs,
	})
}

// answerGrant applies his answer to a grant that waits for it, or reports that
// the answer is not about a grant. Called with mu held.
func (h *handDesk) answerGrant(a registryv1.HandAction) bool {
	asked := h.grant != nil && !h.grant.live
	switch {
	case asked && grantWindow(a) != 0:
		h.grantChosen(grantWindow(a))
	case asked && a == registryv1.HandAction_HAND_ACTION_DECLINE:
		h.endGrant(handGrantDeclined, false)
	case h.grant != nil && a == registryv1.HandAction_HAND_ACTION_PAUSE && h.run == nil:
		h.endGrant(handGrantPaused, false)
	default:
		return false
	}
	return true
}

// serveGrantVerb is hand.grant and hand.ungrant. It answers its own request
// and returns the error to the caller's common refusal.
func (d *Daemon) serveGrantVerb(ctx context.Context, c *conn, f *rigv1.Frame, command string) (*registryv1.HandState, error) {
	if command == "hand.ungrant" {
		st, err := d.serveHandUngrant(c)
		if err == nil {
			c.reply(f.GetStreamId(), &registryv1.HandUngrantResponse{State: st})
		}
		return st, err
	}
	var req registryv1.HandGrantRequest
	if !unmarshalOr(c, f, command, &req) {
		return nil, nil
	}
	st, err := d.serveHandGrant(ctx, c, &req)
	if err == nil {
		c.reply(f.GetStreamId(), &registryv1.HandGrantResponse{State: st})
	}
	return st, err
}
