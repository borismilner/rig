package daemon

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// toast.retract (plan/53 slice 7): a sender takes back a toast it sent,
// before anybody deals with it. AgentBox's `retract` dismissed the card; this
// also retracts the notification record, so what was said and what was taken
// back both stay findable, and answers an unanswered ask as withdrawn, so a
// seat waiting on the reply learns why none will come.

// maxRetractReason bounds the reason kept with a retraction.
const maxRetractReason = 400

// toastDwell is how long a bubble stays up on its own, and false for one that
// never closes by itself: urgent, or asking. It is cmd/rigwindow/toast.html's
// dwellFor, doubled, since hovering pauses the countdown and rig cannot see
// the pointer.
func toastDwell(t *registryv1.Toast) (time.Duration, bool) {
	if t.GetSeverity() == registryv1.Severity_SEVERITY_URGENT || len(t.GetReplies()) > 0 || t.GetReplyText() {
		return 0, false
	}
	lines := 1
	if t.GetBody() != "" {
		lines += 1 + strings.Count(t.GetBody(), "\n")
	}
	d := 10*time.Second + time.Duration(lines)*5*time.Second
	if t.GetSeverity() == registryv1.Severity_SEVERITY_ERROR {
		d *= 2
	}
	return 2 * d, true
}

// open answers the record ids of sender's toasts that may still be on
// screen: inside their dwell, or never closing and not yet answered.
func (r *toastRing) open(sender string, now time.Time) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, t := range r.items {
		if t.GetSender() != sender || t.GetRetracted() || r.gone[t.GetRecordId()] {
			continue
		}
		if dwell, closes := toastDwell(t); closes {
			if now.Sub(time.Unix(0, t.GetAtUnixNano())) < dwell {
				out = append(out, t.GetRecordId())
			}
		} else if q, asks := r.asks[t.GetRecordId()]; !asks || q.answer == nil {
			out = append(out, t.GetRecordId())
		}
	}
	return out
}

// withdraw marks a toast taken back and puts the withdrawal on the ring for
// renderers. gone is kept to the ids the ring still holds.
func (r *toastRing) withdraw(id, sender string) {
	r.mu.Lock()
	if r.gone == nil {
		r.gone = map[string]bool{}
	}
	r.gone[id] = true
	held := map[string]bool{}
	for _, t := range r.items {
		held[t.GetRecordId()] = true
	}
	for g := range r.gone {
		if !held[g] && g != id {
			delete(r.gone, g)
		}
	}
	r.mu.Unlock()
	r.add(&registryv1.Toast{RecordId: id, Sender: sender, Retracted: true, AtUnixNano: time.Now().UnixNano()})
}

// toastRetraction is toast.retracted's payload.
type toastRetraction struct {
	RecordID string `json:"record_id"`
	Sender   string `json:"sender"`
	Reason   string `json:"reason,omitempty"`
}

func (d *Daemon) serveToastRetract(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.ToastRetractRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "toast.retract: "+err.Error())
		return
	}
	if len(req.GetReason()) > maxRetractReason || !utf8.ValidString(req.GetReason()) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.toast.retract: a reason is at most 400 bytes of UTF-8")
		return
	}
	st, ok := d.recordStore(c, f, "toast.retract")
	if !ok {
		return
	}
	// The sender is the connection's, as notify names it, so a caller can
	// only ever take back its own.
	sender := c.name()
	session, seat, epoch, seated := d.provenance(c)
	if sender == "" {
		if !seated {
			refuseUnseatedLease(c, f, "toast.retract")
			return
		}
		sender = seat
	}
	if session == "" {
		session = recordSession(c.principal())
	}

	targets := d.toasts.open(sender, time.Now())
	if id := req.GetRecordId(); id != "" {
		if status := d.retractable(ctx, st, id, sender); status != nil {
			c.failStatus(f.GetStreamId(), status)
			return
		}
		targets = []string{id}
	}
	reason := "withdrawn by its sender"
	if req.GetReason() != "" {
		reason += ": " + req.GetReason()
	}
	resp := &registryv1.ToastRetractResponse{}
	for _, id := range targets {
		if _, asks := d.toasts.question(id); asks && !d.toasts.answer(id, &registryv1.ToastAnswer{
			RecordId: id, Answered: true, Withdrawn: true, By: sender, AtUnixNano: time.Now().UnixNano(),
		}) {
			continue // answered meanwhile: it was dealt with, so it stays
		}
		got, err := st.Retract(ctx, record.RetractRequest{
			ID: id, Reason: reason, Session: session, Seat: sender, Epoch: max(epoch, d.epoch),
		})
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		}
		if got.Already {
			continue
		}
		d.toasts.withdraw(id, sender)
		d.publishJSON("toast.retracted", toastRetraction{RecordID: id, Sender: sender, Reason: req.GetReason()})
		resp.RecordIds = append(resp.RecordIds, id)
	}
	if len(resp.GetRecordIds()) == 0 {
		resp.Note = "nothing of yours was still on screen"
		if req.GetRecordId() != "" {
			resp.Note = "it was already taken back, or answered meanwhile"
		}
	}
	c.reply(f.GetStreamId(), resp)
}

// retractable answers why sender may not take back the toast id, or nil.
func (d *Daemon) retractable(ctx context.Context, st *record.Store, id, sender string) *rigv1.Status {
	rec, err := st.Get(ctx, id)
	_, missing := errors.AsType[*record.NotFoundError](err)
	switch {
	case missing || (err == nil && rec.Kind != notificationKind):
		return &rigv1.Status{
			Code:    rigv1.Code_CODE_NOT_FOUND,
			Message: "rig.toast.retract: no toast " + strconv.Quote(id),
		}
	case err != nil:
		return &rigv1.Status{Code: rigv1.Code_CODE_INTERNAL, Message: "rig.toast.retract: " + err.Error()}
	case rec.Fields["sender"] != sender:
		return &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig.toast.retract: only a toast's sender may take it back",
			Precondition: "the toast was sent by " + sender,
			Actual:       "it was sent by " + rec.Fields["sender"],
			Fix:          "ask its sender to take it back; closing it is the person's own call",
		}
	}
	if q, ok := d.toasts.question(id); ok && q.answer != nil {
		return &rigv1.Status{
			Code:    rigv1.Code_CODE_CONFLICT,
			Message: "rig.toast.retract: it was already answered by " + q.answer.GetBy() + ", so there is nothing to take back",
		}
	}
	return nil
}
