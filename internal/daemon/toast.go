package daemon

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 12's toasts, the daemon's half. rig.notify writes the notification
// record itself, with the sender as provenance, and appends the toast to a
// small in-memory ring; rig.toast.wait is a long poll on that ring, which is
// what the tray holds so that waiting costs nothing while nothing happens.
//
// THE RING IS NOT THE RECORD. It is bounded and dies with the daemon, and it
// exists only to wake a renderer. Every toast is in the record first, so a
// toast the ring dropped or a restart lost is still findable - section 12's
// "nothing is ever only a toast".

const (
	maxToastTitle = 200
	maxToastBody  = 4 << 10
	toastRingSize = 64
	maxToastWait  = 60 * time.Second

	// notificationKind and notificationProject file every notification in
	// the record, estate-wide and apart from any project's own records.
	notificationKind    = "notification"
	notificationProject = "notifications"
)

// toastRing holds the latest toasts and wakes waiters. Its zero value is
// ready to use.
type toastRing struct {
	mu    sync.Mutex
	seq   uint64
	items []*verbsv1.Toast
	wake  chan struct{}
}

func (r *toastRing) add(t *verbsv1.Toast) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	t.Seq = r.seq
	r.items = append(r.items, t)
	if len(r.items) > toastRingSize {
		r.items = r.items[len(r.items)-toastRingSize:]
	}
	if r.wake != nil {
		close(r.wake)
		r.wake = nil
	}
}

// after answers the toasts newer than seq, the latest seq, and a channel that
// closes when the next toast arrives.
func (r *toastRing) after(seq uint64) ([]*verbsv1.Toast, uint64, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*verbsv1.Toast
	for _, t := range r.items {
		if t.GetSeq() > seq {
			out = append(out, t)
		}
	}
	if r.wake == nil {
		r.wake = make(chan struct{})
	}
	return out, r.seq, r.wake
}

func (d *Daemon) serveNotify(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.NotifyRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "notify: "+err.Error())
		return
	}
	sev := req.GetSeverity()
	if _, known := verbsv1.Severity_name[int32(sev)]; !known || sev == verbsv1.Severity_SEVERITY_UNSPECIFIED {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_INVALID,
			Message:      "rig.notify: a notification needs a severity",
			Precondition: "severity is one of info, success, warning, error, urgent",
			Actual:       "severity " + sev.String(),
			Fix:          "set severity; there is no default, because a default would decide how loudly somebody else's message is drawn",
		})
		return
	}
	if req.GetTitle() == "" || len(req.GetTitle()) > maxToastTitle || len(req.GetBody()) > maxToastBody ||
		!utf8.ValidString(req.GetTitle()) || !utf8.ValidString(req.GetBody()) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"rig.notify: a notification has a title of 1 to 200 bytes and a body of at most 4096, in UTF-8")
		return
	}
	st, ok := d.recordStore(c, f, "notify")
	if !ok {
		return
	}

	// THE SENDER IS THE CONNECTION'S, never the request's: a registered
	// program's id, else the caller's seat. A program has no record-write
	// permission and needs none - the daemon writes this record.
	sender := c.name()
	session, seat, epoch, seated := d.provenance(c)
	if sender == "" {
		if !seated {
			refuseUnseatedLease(c, f, "notify")
			return
		}
		sender = seat
	}
	if session == "" {
		session = recordSession(c.principal())
	}
	rec, err := st.Put(ctx, record.PutRequest{
		Kind: notificationKind, Project: notificationProject, Body: req.GetBody(),
		Fields: map[string]string{
			"severity": strings.ToLower(strings.TrimPrefix(sev.String(), "SEVERITY_")), "title": req.GetTitle(), "sender": sender,
		},
		Session: session, Seat: sender, Epoch: max(epoch, d.epoch),
	})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	t := &verbsv1.Toast{
		RecordId: rec.ID, Severity: sev, Title: req.GetTitle(), Body: req.GetBody(),
		Sender: sender, AtUnixNano: time.Now().UnixNano(),
	}
	d.toasts.add(t)
	c.reply(f.GetStreamId(), &verbsv1.NotifyResponse{Toast: t})
}

func (d *Daemon) serveToastWait(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.ToastWaitRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "toast.wait: "+err.Error())
		return
	}
	wait := min(time.Duration(req.GetTimeoutMs())*time.Millisecond, maxToastWait)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		got, latest, wake := d.toasts.after(req.GetAfter())
		if len(got) > 0 {
			c.reply(f.GetStreamId(), &verbsv1.ToastWaitResponse{Toasts: got, Latest: latest})
			return
		}
		select {
		case <-wake:
		case <-timer.C:
			c.reply(f.GetStreamId(), &verbsv1.ToastWaitResponse{Latest: latest})
			return
		case <-ctx.Done():
			return
		}
	}
}
