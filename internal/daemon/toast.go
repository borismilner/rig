package daemon

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
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
	maxToastSpeak = 400
	toastRingSize = 64
	maxToastWait  = 60 * time.Second

	// A toast may ask for a reply (Boris, 2026-09-27): up to three buttons of
	// at most 40 bytes, and a free-text field. The daemon keeps the latest
	// maxToastAsks questions in memory; each answer is filed in the record too.
	maxToastReplies = 3
	maxToastReply   = 40
	maxToastAsks    = 256
	replyKind       = "notification-reply"

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
	items []*registryv1.Toast
	wake  chan struct{}

	// asks are the toasts waiting for, or holding, a reply, by record id;
	// order keeps them bounded, oldest first.
	asks  map[string]*toastAsk
	order []string

	// Do Not Disturb. In memory on purpose: a restart turns it off, so a
	// daemon never comes back silent without anyone having said so.
	dnd        bool
	suppressed uint32

	// gone are the toasts their senders took back (toastretract.go).
	gone map[string]bool
}

// toastAsk is one toast's question and, once somebody replied, its answer.
type toastAsk struct {
	replies []string
	text    bool
	answer  *registryv1.ToastAnswer // nil until answered
	wake    chan struct{}           // closed when answered
}

// ask registers a toast that waits for a reply.
func (r *toastRing) ask(id string, replies []string, text bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.asks == nil {
		r.asks = map[string]*toastAsk{}
	}
	r.asks[id] = &toastAsk{replies: replies, text: text, wake: make(chan struct{})}
	r.order = append(r.order, id)
	if len(r.order) > maxToastAsks {
		delete(r.asks, r.order[0])
		r.order = r.order[1:]
	}
}

// question answers the ask for a record id and whether there is one, with
// the answer so far and a channel that closes when it arrives.
func (r *toastRing) question(id string) (a toastAsk, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.asks[id]
	if !ok {
		return toastAsk{}, false
	}
	return *q, true
}

// answer files the first reply. It says false when there was already one.
func (r *toastRing) answer(id string, ans *registryv1.ToastAnswer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.asks[id]
	if !ok || q.answer != nil {
		return false
	}
	q.answer = ans
	close(q.wake)
	return true
}

// setDND applies a change and answers the state after it.
func (r *toastRing) setDND(ch registryv1.DndChange) (on bool, suppressed uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch ch {
	case registryv1.DndChange_DND_CHANGE_ON:
		if !r.dnd {
			r.suppressed = 0
		}
		r.dnd = true
	case registryv1.DndChange_DND_CHANGE_OFF:
		r.dnd = false
	}
	return r.dnd, r.suppressed
}

// suppress reports whether a toast of this severity is held back now, and
// counts it if so. An urgent toast never is: section 12's rule is that Do Not
// Disturb suppresses notices, never what someone is blocked on.
func (r *toastRing) suppress(sev registryv1.Severity) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dnd || sev == registryv1.Severity_SEVERITY_URGENT {
		return false
	}
	r.suppressed++
	return true
}

func (r *toastRing) add(t *registryv1.Toast) {
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
func (r *toastRing) after(seq uint64) ([]*registryv1.Toast, uint64, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*registryv1.Toast
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

// serveToast dispatches the three toast verbs through one arm of the
// daemon's switch, which gocyclo holds under its ceiling.
func (d *Daemon) serveToast(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "notify":
		d.serveNotify(ctx, c, f)
	case "toast.wait":
		d.serveToastWait(ctx, c, f)
	case "toast.dnd":
		d.serveToastDND(c, f)
	case "toast.reply":
		d.serveToastReply(ctx, c, f)
	case "toast.answer":
		d.serveToastAnswer(ctx, c, f)
	case "toast.retract":
		d.serveToastRetract(ctx, c, f)
	case "sound", "say":
		d.serveSound(ctx, c, f, command)
	case "events.publish", "events.wait", "timer.arm", "timer.disarm", "timer.list":
		d.serveEvents(ctx, c, f, command)
	case "config.get", "config.set":
		d.serveConfig(c, f, command)
	case "logs.query":
		d.serveLogs(ctx, c, f)
	default:
		d.serveHand(ctx, c, f, command)
	}
}

func (d *Daemon) serveNotify(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.NotifyRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "notify: "+err.Error())
		return
	}
	sev := req.GetSeverity()
	if _, known := registryv1.Severity_name[int32(sev)]; !known || sev == registryv1.Severity_SEVERITY_UNSPECIFIED {
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
		len(req.GetSpeak()) > maxToastSpeak ||
		!utf8.ValidString(req.GetTitle()) || !utf8.ValidString(req.GetBody()) || !utf8.ValidString(req.GetSpeak()) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"rig.notify: a notification has a title of 1 to 200 bytes, a body of at most 4096 and a speak line of at most 400, in UTF-8")
		return
	}
	if msg := checkReplies(req.GetReplies()); msg != "" {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_INVALID,
			Message:      "rig.notify: " + msg,
			Precondition: "at most 3 distinct replies, each 1 to 40 bytes of UTF-8",
			Fix:          "shorten or drop replies; offer reply_text for anything longer",
		})
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
	suppressed := d.toasts.suppress(sev)
	fields := map[string]string{
		"severity": strings.ToLower(strings.TrimPrefix(sev.String(), "SEVERITY_")), "title": req.GetTitle(), "sender": sender,
	}
	if suppressed {
		fields["suppressed"] = "do not disturb"
	}
	asks := len(req.GetReplies()) > 0 || req.GetReplyText()
	if asks {
		fields["replies"] = strings.Join(req.GetReplies(), " | ")
		if req.GetReplyText() {
			fields["reply_text"] = "yes"
		}
	}
	rec, err := st.Put(ctx, record.PutRequest{
		Kind: notificationKind, Project: notificationProject, Body: req.GetBody(),
		Fields:  fields,
		Session: session, Seat: sender, Epoch: max(epoch, d.epoch),
	})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	t := &registryv1.Toast{
		RecordId: rec.ID, Severity: sev, Title: req.GetTitle(), Body: req.GetBody(),
		Sender: sender, AtUnixNano: time.Now().UnixNano(), Suppressed: suppressed,
		Replies: req.GetReplies(), ReplyText: req.GetReplyText(),
	}
	if asks {
		d.toasts.ask(rec.ID, req.GetReplies(), req.GetReplyText())
	}
	if !suppressed {
		d.toasts.add(t)
		d.events.publishRig("toast.posted", t)
		if d.audio != nil {
			d.audio.Toast(t.GetTitle(), t.GetBody(), req.GetSpeak())
		}
	}
	c.reply(f.GetStreamId(), &registryv1.NotifyResponse{Toast: t})
}

func (d *Daemon) serveToastWait(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.ToastWaitRequest
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
			c.reply(f.GetStreamId(), &registryv1.ToastWaitResponse{Toasts: got, Latest: latest})
			return
		}
		select {
		case <-wake:
		case <-timer.C:
			c.reply(f.GetStreamId(), &registryv1.ToastWaitResponse{Latest: latest})
			return
		case <-ctx.Done():
			return
		}
	}
}

func (d *Daemon) serveToastDND(c *conn, f *rigv1.Frame) {
	var req registryv1.ToastDndRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "toast.dnd: "+err.Error())
		return
	}
	switch req.GetChange() {
	case registryv1.DndChange_DND_CHANGE_QUERY, registryv1.DndChange_DND_CHANGE_ON, registryv1.DndChange_DND_CHANGE_OFF:
	default:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"rig.toast.dnd: change is one of query, on, off; got "+req.GetChange().String())
		return
	}
	on, n := d.toasts.setDND(req.GetChange())
	c.reply(f.GetStreamId(), &registryv1.ToastDndResponse{On: on, Suppressed: n})
}

// checkReplies answers what is wrong with a notification's reply buttons, or
// "" when nothing is.
func checkReplies(replies []string) string {
	if len(replies) > maxToastReplies {
		return "a toast offers at most 3 replies"
	}
	seen := map[string]bool{}
	for _, r := range replies {
		if r == "" || len(r) > maxToastReply || !utf8.ValidString(r) {
			return "each reply is 1 to 40 bytes of UTF-8"
		}
		if seen[r] {
			return "the replies repeat " + strconv.Quote(r)
		}
		seen[r] = true
	}
	return ""
}

// serveToastReply files the answer to a toast that asked for one. Exactly one
// of a button's label, free text, or dismissed; the first reply wins. The
// replier comes off the connection, never the request.
func (d *Daemon) serveToastReply(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.ToastReplyRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "toast.reply: "+err.Error())
		return
	}
	q, ok := d.toasts.question(req.GetRecordId())
	if !ok {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND,
			"rig.toast.reply: no toast "+strconv.Quote(req.GetRecordId())+" is waiting for a reply")
		return
	}
	if q.answer.GetWithdrawn() {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT,
			"rig.toast.reply: its sender "+q.answer.GetBy()+" took the toast back")
		return
	}
	if q.answer != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT,
			"rig.toast.reply: the toast was already answered by "+q.answer.GetBy())
		return
	}
	if msg := checkReply(q, &req); msg != "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.toast.reply: "+msg)
		return
	}
	st, ok := d.recordStore(c, f, "toast.reply")
	if !ok {
		return
	}
	by := c.name()
	session, seat, epoch, _ := d.provenance(c)
	if by == "" {
		by = seat
	}
	if by == "" {
		refuseUnseatedLease(c, f, "toast.reply")
		return
	}
	if session == "" {
		session = recordSession(c.principal())
	}
	fields := map[string]string{"notification": req.GetRecordId(), "by": by}
	switch {
	case req.GetDismissed():
		fields["dismissed"] = "yes"
	case req.GetReply() != "":
		fields["reply"] = req.GetReply()
	}
	if _, err := st.Put(ctx, record.PutRequest{
		Kind: replyKind, Project: notificationProject, Body: req.GetText(), Fields: fields,
		Session: session, Seat: by, Epoch: max(epoch, d.epoch),
	}); err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	ans := &registryv1.ToastAnswer{
		RecordId: req.GetRecordId(), Answered: true, Reply: req.GetReply(), Text: req.GetText(),
		Dismissed: req.GetDismissed(), By: by, AtUnixNano: time.Now().UnixNano(),
	}
	if !d.toasts.answer(req.GetRecordId(), ans) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.toast.reply: the toast was answered meanwhile")
		return
	}
	c.reply(f.GetStreamId(), &registryv1.ToastReplyResponse{Answer: ans})
}

// checkReply answers what is wrong with a reply to the question q, or "".
func checkReply(q toastAsk, req *registryv1.ToastReplyRequest) string {
	given := 0
	for _, b := range []bool{req.GetDismissed(), req.GetReply() != "", req.GetText() != ""} {
		if b {
			given++
		}
	}
	switch {
	case given != 1:
		return "a reply is exactly one of: a button's label, free text, or dismissed"
	case req.GetReply() != "" && !slices.Contains(q.replies, req.GetReply()):
		return "the toast offers no reply " + strconv.Quote(req.GetReply())
	case req.GetText() != "" && !q.text:
		return "the toast does not take a free-text reply"
	case len(req.GetText()) > maxToastBody || !utf8.ValidString(req.GetText()):
		return "free text is at most 4096 bytes of UTF-8"
	}
	return ""
}

// serveToastAnswer waits, up to the timeout, for the reply to a toast.
func (d *Daemon) serveToastAnswer(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.ToastAnswerRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "toast.answer: "+err.Error())
		return
	}
	q, ok := d.toasts.question(req.GetRecordId())
	if !ok {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND,
			"rig.toast.answer: no toast "+strconv.Quote(req.GetRecordId())+" asked for a reply on this daemon's run")
		return
	}
	if q.answer == nil && req.GetTimeoutMs() > 0 {
		timer := time.NewTimer(min(time.Duration(req.GetTimeoutMs())*time.Millisecond, maxToastWait))
		defer timer.Stop()
		select {
		case <-q.wake:
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		q, _ = d.toasts.question(req.GetRecordId())
	}
	ans := q.answer
	if ans == nil {
		ans = &registryv1.ToastAnswer{RecordId: req.GetRecordId()}
	}
	c.reply(f.GetStreamId(), &registryv1.ToastAnswerResponse{Answer: ans})
}
