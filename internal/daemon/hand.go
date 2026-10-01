package daemon

import (
	"cmp"
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// The HANDS OFF strip, the daemon's half (plan/05 section 5m, H1-H5). There
// is one desktop, so there is one run at a time, and its life is:
//
//	idle -> asking -> driving <-> paused -> idle
//	          |  ^
//	          v  |
//	          held
//
// A program asks with hand.request and blocks through the countdown; silence
// is consent (H1). He can decline, or hold the countdown, and a held one never
// turns into driving by itself (H2). While it drives, every step passes the
// gate hand.step, which blocks once he took the desktop back and is refused
// once he stopped the run (H4). Only a connection that is neither a program
// nor an agent answers (H5).
//
// EVERY TIMED TRANSITION IS MADE BY THE CALLER BLOCKED ON IT. The countdown
// and the hold are ended by the hand.request waiting through them, the pause
// by the hand.step waiting through it, so there is no timer goroutine to leak
// and a run whose program died is ended by its connection closing.

const (
	handCountdown      = 20 * time.Second
	handCountdownLeast = 10 * time.Second
	handCountdownMost  = 120 * time.Second
	// How long he may hold a countdown or a run before it ends: AgentBox's
	// pause bound, which nobody has asked to change.
	handHoldMax  = 10 * time.Minute
	handPauseMax = 10 * time.Minute
	maxHandWait  = 60 * time.Second
	maxHandText  = 200
	// How long a step waits for the strip to move off its point (H6), and
	// how far around the strip a point still counts as under it.
	handStripClear = 3 * time.Second
	handStripPad   = 4
)

// How a run ends, in his words on the strip and in the refusal its program
// reads.
const (
	handFinished   = "finished"
	handDeclined   = "declined"
	handStopped    = "stopped"
	handLeft       = "the program left"
	handHeldLong   = "held too long, so declined"
	handPausedLong = "paused too long, so stopped"
)

// handRun is one program's claim on the desktop. owner is the connection that
// asked, compared by identity only.
type handRun struct {
	owner any
	ended string
}

// handDesk is the state machine. Its zero value is not ready: newHandDesk.
type handDesk struct {
	mu   sync.Mutex
	st   *registryv1.HandState
	run  *handRun // nil while idle
	last *handRun // the run that ended last, so its program learns why
	wake chan struct{}
	now  func() time.Time
	// holdMax and pauseMax are handHoldMax and handPauseMax; tests shorten
	// them.
	holdMax, pauseMax time.Duration
	// strip is where the strip last said it is, and stripBy the connection
	// that said so; nil while no strip is up.
	strip   *handRect
	stripBy any
	// stripClear is handStripClear; tests shorten it.
	stripClear time.Duration
}

// handRect is the strip's window on the root, shadow included: all of it
// takes the pointer.
type handRect struct{ x, y, w, h int32 }

// handPoint is where a step will press or release.
type handPoint struct{ x, y int32 }

// covers is true when p is under the strip, or too close to it to trust.
func (r *handRect) covers(p handPoint) bool {
	return r != nil &&
		p.x >= r.x-handStripPad && p.x < r.x+r.w+handStripPad &&
		p.y >= r.y-handStripPad && p.y < r.y+r.h+handStripPad
}

func newHandDesk(now func() time.Time) *handDesk {
	return &handDesk{
		st:  &registryv1.HandState{Seq: 1, Phase: registryv1.HandPhase_HAND_PHASE_IDLE},
		now: now, holdMax: handHoldMax, pauseMax: handPauseMax, stripClear: handStripClear,
	}
}

// handError is a refusal with the code its handler answers.
type handError struct {
	code rigv1.Code
	msg  string
}

func (e *handError) Error() string { return e.msg }

func handRefused(code rigv1.Code, msg string) error { return &handError{code: code, msg: msg} }

// changed publishes the state: a new seq, and every waiter woken. Called with
// mu held.
func (h *handDesk) changed() {
	h.st.Seq++
	if h.wake != nil {
		close(h.wake)
		h.wake = nil
	}
}

// waitChan is closed at the next change. Called with mu held.
func (h *handDesk) waitChan() <-chan struct{} {
	if h.wake == nil {
		h.wake = make(chan struct{})
	}
	return h.wake
}

func (h *handDesk) snapshot() *registryv1.HandState {
	st, _ := proto.Clone(h.st).(*registryv1.HandState)
	return st
}

// end closes the current run. Called with mu held.
func (h *handDesk) end(why string) {
	if h.run == nil {
		return
	}
	h.run.ended = why
	h.last, h.run = h.run, nil
	h.st = &registryv1.HandState{
		Seq: h.st.GetSeq(), Phase: registryv1.HandPhase_HAND_PHASE_IDLE,
		Holder: h.st.GetHolder(), Reason: h.st.GetReason(), Ended: why,
	}
	h.changed()
}

func (h *handDesk) unixNano(d time.Duration) int64 { return h.now().Add(d).UnixNano() }

// ms is a bounded duration in milliseconds; every duration here is at most
// minutes, so the clamp only guards the conversion.
func ms(d time.Duration) uint32 {
	return uint32(min(max(d.Milliseconds(), 0), int64(^uint32(0))))
}

// ask starts the countdown, or refuses while somebody else has the desktop.
func (h *handDesk) ask(owner any, holder, reason string, window time.Duration) (*handRun, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.run != nil {
		return nil, handRefused(rigv1.Code_CODE_CONFLICT,
			"rig.hand.request: the desktop is already asked for by "+h.st.GetHolder())
	}
	h.run = &handRun{owner: owner}
	h.st = &registryv1.HandState{
		Seq: h.st.GetSeq(), Phase: registryv1.HandPhase_HAND_PHASE_ASKING,
		Holder: holder, Reason: reason, AskedUnixNano: h.now().UnixNano(),
		DeadlineUnixNano: h.unixNano(window), WindowMs: ms(window), LeftMs: ms(window),
	}
	h.changed()
	return h.run, nil
}

// expire makes the transitions a deadline owes. Called with mu held.
func (h *handDesk) expire() {
	if h.run == nil || h.now().UnixNano() < h.st.GetDeadlineUnixNano() {
		return
	}
	switch h.st.GetPhase() {
	case registryv1.HandPhase_HAND_PHASE_ASKING:
		h.drive()
	case registryv1.HandPhase_HAND_PHASE_HELD:
		h.end(handHeldLong)
	case registryv1.HandPhase_HAND_PHASE_PAUSED:
		h.end(handPausedLong)
	}
}

// drive starts the run driving. Called with mu held.
func (h *handDesk) drive() {
	h.st.Phase = registryv1.HandPhase_HAND_PHASE_DRIVING
	h.st.DrivingUnixNano = h.now().UnixNano()
	h.st.DeadlineUnixNano, h.st.LeftMs = 0, 0
	h.changed()
}

// await blocks until a deadline or a change, whichever is first, and reports
// false when ctx ended first. Called with mu held; returns with it held.
func (h *handDesk) await(ctx context.Context) bool {
	wake := h.waitChan()
	var timer <-chan time.Time
	if dl := h.st.GetDeadlineUnixNano(); dl != 0 {
		t := time.NewTimer(time.Duration(dl - h.now().UnixNano()))
		defer t.Stop()
		timer = t.C
	}
	h.mu.Unlock()
	defer h.mu.Lock()
	select {
	case <-wake:
	case <-timer:
	case <-ctx.Done():
		return false
	}
	return true
}

// granted blocks through the countdown and answers once run drives, or why
// it never will.
func (h *handDesk) granted(ctx context.Context, run *handRun) (*registryv1.HandState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		h.expire()
		switch {
		case run.ended != "":
			return nil, endedErr("rig.hand.request", run.ended)
		case h.st.GetPhase() == registryv1.HandPhase_HAND_PHASE_DRIVING,
			h.st.GetPhase() == registryv1.HandPhase_HAND_PHASE_PAUSED:
			return h.snapshot(), nil
		}
		if !h.await(ctx) {
			if h.run == run {
				h.end(handLeft)
			}
			return nil, ctx.Err()
		}
	}
}

// endedErr is the refusal a run's program reads once the run is over.
func endedErr(verb, why string) error {
	code := rigv1.Code_CODE_DENIED
	switch why {
	case handLeft:
		code = rigv1.Code_CODE_UNAVAILABLE
	case handHeldLong, handPausedLong:
		code = rigv1.Code_CODE_DEADLINE
	}
	return handRefused(code, verb+": the run is over: "+why)
}

// step is the gate before one step. It answers at once while the run drives,
// blocks while he has the desktop, and refuses once the run is over. A step
// with a point under the strip publishes that point and waits for the strip
// to move off it (H6), and is refused if it does not: a click under the
// strip would press the strip's own buttons.
func (h *handDesk) step(ctx context.Context, owner any, activity, note string, pt *handPoint) (*registryv1.HandState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var clearBy time.Time
	for {
		h.expire()
		if h.run == nil || h.run.owner != owner {
			if h.last != nil && h.last.owner == owner {
				return nil, endedErr("rig.hand.step", h.last.ended)
			}
			return nil, handRefused(rigv1.Code_CODE_NOT_FOUND, "rig.hand.step: this connection holds no run; ask with rig.hand.request")
		}
		switch h.st.GetPhase() {
		case registryv1.HandPhase_HAND_PHASE_DRIVING:
			if (activity != "" && activity != h.st.GetActivity()) || (note != "" && note != h.st.GetNote()) {
				h.st.Activity = cmp.Or(activity, h.st.GetActivity())
				h.st.Note = cmp.Or(note, h.st.GetNote())
				h.changed()
			}
			if pt == nil || !h.strip.covers(*pt) {
				h.aim(nil)
				return h.snapshot(), nil
			}
			if clearBy.IsZero() {
				clearBy = h.now().Add(h.stripClear)
				h.aim(pt)
			} else if !h.now().Before(clearBy) {
				h.aim(nil)
				return nil, handRefused(rigv1.Code_CODE_CONFLICT,
					"rig.hand.step: the HANDS OFF strip did not move off the point this step acts on")
			}
			if !h.awaitUntil(ctx, clearBy) && ctx.Err() != nil {
				h.aim(nil)
				return nil, ctx.Err()
			}
			continue
		case registryv1.HandPhase_HAND_PHASE_ASKING, registryv1.HandPhase_HAND_PHASE_HELD:
			return nil, handRefused(rigv1.Code_CODE_CONFLICT, "rig.hand.step: the run is not granted yet; rig.hand.request answers when it is")
		}
		if !h.await(ctx) {
			return nil, ctx.Err()
		}
	}
}

// awaitUntil is await bounded by t as well as by ctx. Called with mu held.
func (h *handDesk) awaitUntil(ctx context.Context, t time.Time) bool {
	bound, cancel := context.WithDeadline(ctx, t)
	defer cancel()
	return h.await(bound)
}

// aim publishes the point a step waits on, or clears it. Called with mu
// held.
func (h *handDesk) aim(pt *handPoint) {
	has := pt != nil
	if has == h.st.GetHasAim() && (!has || (pt.x == h.st.GetAimX() && pt.y == h.st.GetAimY())) {
		return
	}
	h.st.HasAim, h.st.AimX, h.st.AimY = has, 0, 0
	if has {
		h.st.AimX, h.st.AimY = pt.x, pt.y
	}
	h.changed()
}

// placed records where the strip is, from the strip's own connection.
func (h *handDesk) placed(by any, r handRect) *registryv1.HandState {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.strip, h.stripBy = &r, by
	h.changed()
	return h.snapshot()
}

// release ends owner's run as finished. Releasing nothing is not an error: a
// run he stopped is already over.
func (h *handDesk) release(owner any) *registryv1.HandState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.run != nil && h.run.owner == owner {
		h.end(handFinished)
	}
	return h.snapshot()
}

// drop ends owner's run because its connection closed.
func (h *handDesk) drop(owner any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.run != nil && h.run.owner == owner {
		h.end(handLeft)
	}
	if h.last != nil && h.last.owner == owner {
		h.last = nil
	}
	if h.stripBy == owner {
		h.strip, h.stripBy = nil, nil
		h.changed()
	}
}

// answer applies his answer, or refuses one the phase does not take.
func (h *handDesk) answer(a registryv1.HandAction) (*registryv1.HandState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expire()
	ph := h.st.GetPhase()
	asking := ph == registryv1.HandPhase_HAND_PHASE_ASKING
	held := ph == registryv1.HandPhase_HAND_PHASE_HELD
	switch {
	case a == registryv1.HandAction_HAND_ACTION_ALLOW && (asking || held):
		h.drive()
	case a == registryv1.HandAction_HAND_ACTION_DECLINE && (asking || held):
		h.end(handDeclined)
	case a == registryv1.HandAction_HAND_ACTION_HOLD && asking:
		left := max(time.Duration(h.st.GetDeadlineUnixNano()-h.now().UnixNano()), 0)
		h.st.Phase = registryv1.HandPhase_HAND_PHASE_HELD
		h.st.LeftMs = ms(left)
		h.st.DeadlineUnixNano = h.unixNano(h.holdMax)
		h.changed()
	case a == registryv1.HandAction_HAND_ACTION_RESUME && held:
		// The countdown runs on from where it was held, so letting it run is
		// never a surprise start.
		h.st.Phase = registryv1.HandPhase_HAND_PHASE_ASKING
		h.st.DeadlineUnixNano = h.unixNano(time.Duration(h.st.GetLeftMs()) * time.Millisecond)
		h.changed()
	case a == registryv1.HandAction_HAND_ACTION_PAUSE && ph == registryv1.HandPhase_HAND_PHASE_DRIVING:
		h.st.Phase = registryv1.HandPhase_HAND_PHASE_PAUSED
		h.st.DeadlineUnixNano = h.unixNano(h.pauseMax)
		h.changed()
	case a == registryv1.HandAction_HAND_ACTION_RESUME && ph == registryv1.HandPhase_HAND_PHASE_PAUSED:
		h.st.Phase = registryv1.HandPhase_HAND_PHASE_DRIVING
		h.st.DeadlineUnixNano = 0
		h.changed()
	case a == registryv1.HandAction_HAND_ACTION_STOP && h.run != nil:
		h.end(handStopped)
	default:
		return nil, handRefused(rigv1.Code_CODE_CONFLICT,
			"rig.hand.answer: "+handWord(a.String(), "HAND_ACTION_")+" does not apply while the desktop is "+
				handWord(ph.String(), "HAND_PHASE_"))
	}
	return h.snapshot(), nil
}

// handWord is an enum value as he would say it: "pause", "idle".
func handWord(full, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(full, prefix))
}

// after answers the state once its seq is past seq, waiting up to wait. A
// deadline passing while it waits is a change too, so the strip never shows
// a countdown that has already run out.
func (h *handDesk) after(ctx context.Context, seq uint64, wait time.Duration) (*registryv1.HandState, bool) {
	bound, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		h.expire()
		if h.st.GetSeq() > seq || wait <= 0 {
			return h.snapshot(), true
		}
		if !h.await(bound) {
			if ctx.Err() != nil {
				return nil, false
			}
			return h.snapshot(), true
		}
	}
}

// serveHand dispatches the six hand verbs through one arm of the daemon's
// switch.
func (d *Daemon) serveHand(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	var (
		st  *registryv1.HandState
		err error
	)
	switch command {
	case "hand.request":
		var req registryv1.HandRequestRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		st, err = d.serveHandRequest(ctx, c, &req)
		if err == nil {
			c.reply(f.GetStreamId(), &registryv1.HandRequestResponse{State: st})
		}
	case "hand.step":
		var req registryv1.HandStepRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		var pt *handPoint
		if req.GetHasPoint() {
			pt = &handPoint{req.GetX(), req.GetY()}
		}
		if st, err = d.hand.step(ctx, c, clip(req.GetActivity()), clip(req.GetNote()), pt); err == nil {
			c.reply(f.GetStreamId(), &registryv1.HandStepResponse{State: st})
		}
	case "hand.release":
		c.reply(f.GetStreamId(), &registryv1.HandReleaseResponse{State: d.hand.release(c)})
	case "hand.wait":
		var req registryv1.HandWaitRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		wait := min(time.Duration(req.GetTimeoutMs())*time.Millisecond, maxHandWait)
		if st, ok := d.hand.after(ctx, req.GetAfter(), wait); ok {
			c.reply(f.GetStreamId(), &registryv1.HandWaitResponse{State: st})
		}
	case "hand.strip":
		d.serveHandStrip(c, f, command)
	case "hand.answer":
		var req registryv1.HandAnswerRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		// H5: the answer is his. A program or an agent answering its own
		// countdown would make the countdown decoration.
		if c.scoped.Load() || c.agentDoor {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
				"rig.hand.answer: only the person at the desktop answers, from the strip or a terminal")
			return
		}
		if st, err = d.hand.answer(req.GetAction()); err == nil {
			c.reply(f.GetStreamId(), &registryv1.HandAnswerResponse{State: st})
		}
	}
	if err == nil || ctx.Err() != nil {
		return
	}
	var he *handError
	if errors.As(err, &he) {
		c.fail(f.GetStreamId(), he.code, he.msg)
		return
	}
	c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
}

// serveHandStrip takes where the strip is. Where the strip is decides where
// a click may land, so only the strip says so, and it is a person's process
// as an answer is.
func (d *Daemon) serveHandStrip(c *conn, f *rigv1.Frame, command string) {
	var req registryv1.HandStripRequest
	if !unmarshalOr(c, f, command, &req) {
		return
	}
	if c.scoped.Load() || c.agentDoor {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
			"rig.hand.strip: only the HANDS OFF strip says where it is")
		return
	}
	if req.GetWidth() <= 0 || req.GetHeight() <= 0 {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.hand.strip: the strip has a width and a height")
		return
	}
	st := d.hand.placed(c, handRect{req.GetX(), req.GetY(), req.GetWidth(), req.GetHeight()})
	c.reply(f.GetStreamId(), &registryv1.HandStripResponse{State: st})
}

func (d *Daemon) serveHandRequest(ctx context.Context, c *conn, req *registryv1.HandRequestRequest) (*registryv1.HandState, error) {
	window := handCountdown
	if ms := req.GetCountdownMs(); ms != 0 {
		window = time.Duration(ms) * time.Millisecond
	}
	if window < handCountdownLeast || window > handCountdownMost {
		return nil, handRefused(rigv1.Code_CODE_INVALID,
			"rig.hand.request: the countdown is 10 to 120 seconds, so he always has time to decline")
	}
	// The holder comes off the connection, never the request: the strip
	// names who is asking, and a caller must not choose that name.
	holder := c.name()
	if holder == "" {
		holder = "a terminal"
	}
	run, err := d.hand.ask(c, holder, clip(req.GetReason()), window)
	if err != nil {
		return nil, err
	}
	return d.hand.granted(ctx, run)
}

// clip bounds a line the strip draws, cut on a rune boundary.
func clip(s string) string {
	if len(s) <= maxHandText {
		return s
	}
	i := maxHandText
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i] + "…"
}
