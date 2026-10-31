package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/replyfield"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

const (
	askMarker  = "ask"
	askExpired = "expired"
	maxAskTTL  = 7 * 24 * time.Hour
	maxAskRefs = 16
)

func (d *Daemon) serveAsk(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "ask.open":
		d.serveAskOpen(ctx, c, f)
	case "ask.get":
		d.serveAskGet(ctx, c, f)
	case "ask.presented":
		d.serveAskPresented(ctx, c, f)
	case "ask.list":
		d.serveAskList(ctx, c, f)
	case "ask.answer":
		d.serveAskAnswer(ctx, c, f)
	case "ask.withdraw":
		d.serveAskWithdraw(ctx, c, f)
	}
}

func checkAskOpen(q *registryv1.AskOpenRequest) string {
	if q.GetKey() == "" || len(q.GetKey()) > 128 || !utf8.ValidString(q.GetKey()) {
		return "key needs 1 to 128 bytes of UTF-8"
	}
	if !askKind(q.GetKind()) {
		return "kind needs a lower-case letter followed by up to 63 lower-case letters, digits, dots, underscores or hyphens"
	}
	if !slices.Contains([]string{"routine", "normal", "urgent"}, q.GetUrgency()) {
		return "urgency is routine, normal or urgent"
	}
	if q.GetTtlSeconds() == 0 || time.Duration(q.GetTtlSeconds())*time.Second > maxAskTTL {
		return "ttl_seconds needs 1 second to 7 days"
	}
	if q.GetTitle() == "" || len(q.GetTitle()) > maxToastTitle || len(q.GetBody()) > maxToastBody ||
		!utf8.ValidString(q.GetTitle()) || !utf8.ValidString(q.GetBody()) {
		return "title needs 1 to 200 bytes, body at most 4096 bytes, both UTF-8"
	}
	if msg := checkReplies(q.GetOptions()); msg != "" {
		return msg
	}
	if len(q.GetOptions()) == 0 && !q.GetAcceptsText() {
		return "offer an option or free text"
	}
	if len(q.GetRefs()) > maxAskRefs {
		return "at most 16 references"
	}
	for _, ref := range q.GetRefs() {
		if ref == "" || len(ref) > 256 || !utf8.ValidString(ref) {
			return "each reference needs 1 to 256 bytes of UTF-8"
		}
	}
	if q.GetFailureMode() != "" && q.GetFailureMode() != "gating" && q.GetFailureMode() != "disambiguating" && q.GetFailureMode() != askCountdown {
		return "failure_mode is gating, disambiguating or countdown"
	}
	if q.GetFailureMode() == askCountdown && (!slices.Equal(q.GetOptions(), []string{"Stop"}) || q.GetAcceptsText()) {
		return "countdown requires exactly the Stop option and no free text"
	}
	return ""
}

func askKind(kind string) bool {
	if kind == "" || len(kind) > 64 || kind[0] < 'a' || kind[0] > 'z' {
		return false
	}
	for _, c := range kind[1:] {
		if c < 'a' || c > 'z' {
			if c < '0' || c > '9' {
				if c != '.' && c != '_' && c != '-' {
					return false
				}
			}
		}
	}
	return true
}

func askSeverity(urgency string) registryv1.Severity {
	switch urgency {
	case "urgent":
		return registryv1.Severity_SEVERITY_URGENT
	case "normal":
		return registryv1.Severity_SEVERITY_WARNING
	default:
		return registryv1.Severity_SEVERITY_INFO
	}
}

func askMode(mode string) string {
	if mode == "" {
		return "gating"
	}
	return mode
}

func askFingerprint(q *registryv1.AskOpenRequest) string {
	request := proto.CloneOf(q)
	request.FailureMode = askMode(request.GetFailureMode())
	b, _ := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (d *Daemon) serveAskOpen(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.AskOpenRequest
	if !unmarshalOr(c, f, "ask.open", &req) {
		return
	}
	if msg := checkAskOpen(&req); msg != "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.ask.open: "+msg)
		return
	}
	who, ok := d.panelCaller(c, f, "ask.open")
	if !ok {
		return
	}
	st, ok := d.recordStore(c, f, "ask.open")
	if !ok {
		return
	}
	d.askMu.Lock()
	defer d.askMu.Unlock()
	key := who.name + "\x00" + req.GetKey()
	prior, err := st.Find(ctx, record.QueryFilter{
		Project: notificationProject, Kind: notificationKind, Field: "ask_key", Value: key,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	if len(prior) > 0 {
		if prior[0].Fields["ask_fingerprint"] != askFingerprint(&req) {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.ask.open: key already belongs to a different question")
			return
		}
		a, err := d.askFromRecord(ctx, st, prior[0], time.Now())
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		}
		c.reply(f.GetStreamId(), &registryv1.AskOpenResponse{Ask: a})
		return
	}
	refs, _ := json.Marshal(req.GetRefs())
	mode := askMode(req.GetFailureMode())
	to := who.to
	sev := askSeverity(req.GetUrgency())
	now := time.Now()
	delivery, err := d.decideAskDelivery(ctx, st, &req, now)
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	fields := map[string]string{
		"ask_marker": askMarker, "ask_key": key, "ask_fingerprint": askFingerprint(&req),
		"ask_kind": req.GetKind(), "ask_urgency": req.GetUrgency(), "ask_refs": string(refs),
		"ask_mode": mode, "ask_expires": strconv.FormatInt(now.Add(time.Duration(req.GetTtlSeconds())*time.Second).UnixNano(), 10),
		"ask_idempotency": req.GetKey(), "ask_channel": delivery.channel, "ask_delivery_note": delivery.note,
		"severity": strings.ToLower(strings.TrimPrefix(sev.String(), "SEVERITY_")),
		"title":    req.GetTitle(), "sender": who.name, "to": to,
		"replies": replyfield.Encode(req.GetOptions()),
	}
	if req.GetAcceptsText() {
		fields["reply_text"] = fieldYes
	}
	if delivery.suppressed {
		fields["suppressed"] = delivery.note
	}
	if delivery.never {
		fields["ask_policy_never"] = fieldYes
	}
	if mode == askCountdown {
		if err := d.startCountdown(fields, req.GetTtlSeconds()); err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_UNAVAILABLE, err)
			return
		}
		if delivery.suppressed {
			delivery.never = true
			fields["ask_policy_never"] = fieldYes
		}
	}
	rec, err := st.Put(ctx, record.PutRequest{
		Kind: notificationKind, Project: notificationProject, Body: req.GetBody(), Fields: fields,
		Session: who.session, Seat: who.seat, Epoch: who.epoch,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	t := &registryv1.Toast{
		RecordId: rec.ID, Severity: sev, Title: req.GetTitle(), Body: req.GetBody(), Sender: who.name,
		AtUnixNano: rec.Prov.CreatedAt.UnixNano(), Suppressed: delivery.suppressed,
		Replies: req.GetOptions(), ReplyText: req.GetAcceptsText(),
	}
	t.CountdownSeconds = countdownSeconds(rec)
	if delivery.never {
		d.toasts.ask(rec.ID, req.GetOptions(), req.GetAcceptsText(), to)
		d.toasts.answer(rec.ID, &registryv1.ToastAnswer{
			RecordId: rec.ID, Answered: true, Dismissed: true, By: eventSourceRig, AtUnixNano: time.Now().UnixNano(),
		})
	} else {
		d.toasts.ask(rec.ID, req.GetOptions(), req.GetAcceptsText(), to)
		select {
		case d.askWake <- struct{}{}:
		default:
		}
	}
	if !delivery.suppressed && !delivery.never {
		d.toasts.add(t)
		d.events.publishRig("toast.posted", t)
		if d.audio != nil {
			d.audio.Toast(t.GetTitle(), t.GetBody(), "")
		}
	}
	a, err := d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), &registryv1.AskOpenResponse{Ask: a})
}

func (d *Daemon) askRecord(ctx context.Context, st *record.Store, id string) (record.Record, error) {
	rec, err := st.Get(ctx, id)
	if err != nil {
		return rec, err
	}
	if rec.Kind != notificationKind || rec.Project != notificationProject || rec.Fields["ask_marker"] != askMarker {
		return rec, &record.NotFoundError{ID: id}
	}
	return rec, nil
}

func (d *Daemon) askFromRecord(ctx context.Context, st *record.Store, rec record.Record, now time.Time) (*registryv1.Ask, error) {
	var refs []string
	if err := json.Unmarshal([]byte(rec.Fields["ask_refs"]), &refs); err != nil {
		return nil, err
	}
	expires, err := strconv.ParseInt(rec.Fields["ask_expires"], 10, 64)
	if err != nil {
		return nil, err
	}
	a := &registryv1.Ask{
		Id: rec.ID, Key: rec.Fields["ask_idempotency"], Kind: rec.Fields["ask_kind"],
		Urgency: rec.Fields["ask_urgency"], Refs: refs, Options: replyfield.Decode(rec.Fields["replies"]),
		Title: rec.Fields["title"], Body: rec.Body, AcceptsText: rec.Fields["reply_text"] == fieldYes,
		FailureMode: rec.Fields["ask_mode"], ExpiresUnixNano: expires, Sender: rec.Fields["sender"],
		Channel: rec.Fields["ask_channel"], DeliveryNote: rec.Fields["ask_delivery_note"], State: "open",
	}
	replies, err := st.Find(ctx, record.QueryFilter{
		Project: notificationProject, Kind: replyKind, Field: "notification", Value: rec.ID,
	})
	if err != nil {
		return nil, err
	}
	switch {
	case len(replies) > 0:
		r := replies[0]
		a.Answer = &registryv1.ToastAnswer{
			RecordId: rec.ID, Answered: true, Reply: r.Fields["reply"], Text: r.Body,
			Dismissed: r.Fields["dismissed"] == fieldYes,
			Withdrawn: r.Fields["withdrawn"] == fieldYes,
			Expired:   r.Fields[askExpired] == fieldYes,
			By:        r.Fields["by"], AtUnixNano: r.Prov.CreatedAt.UnixNano(),
		}
		switch {
		case r.Fields[askExpired] == fieldYes:
			a.State = askExpired
		case a.GetAnswer().GetWithdrawn():
			a.State = "withdrawn"
		case a.GetAnswer().GetDismissed():
			a.State = "dismissed"
		default:
			a.State = "answered"
		}
	case rec.Fields["ask_policy_never"] == fieldYes:
		a.State = "denied"
		if a.GetFailureMode() == "disambiguating" {
			a.State = "dismissed"
		}
		a.Answer = &registryv1.ToastAnswer{RecordId: rec.ID, Answered: true, Dismissed: true, By: eventSourceRig}
	case now.UnixNano() >= expires:
		a.State = askExpired
	}
	if a.GetState() == askExpired && a.GetFailureMode() == "gating" {
		a.State = "denied"
	}
	if a.GetFailureMode() == askCountdown {
		countdownState(a, rec, replies)
	}
	return a, nil
}

func (d *Daemon) serveAskGet(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.AskGetRequest
	if !unmarshalOr(c, f, "ask.get", &req) {
		return
	}
	st, ok := d.recordStore(c, f, "ask.get")
	if !ok {
		return
	}
	rec, err := d.askRecord(ctx, st, req.GetId())
	if isNotFound(err) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.ask.get: no question "+strconv.Quote(req.GetId()))
		return
	}
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	a, err := d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), &registryv1.AskGetResponse{Ask: a})
}

func (d *Daemon) serveAskList(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.AskListRequest
	if !unmarshalOr(c, f, "ask.list", &req) {
		return
	}
	st, ok := d.recordStore(c, f, "ask.list")
	if !ok {
		return
	}
	recs, err := st.Find(ctx, record.QueryFilter{
		Project: notificationProject, Kind: notificationKind, Field: "ask_marker", Value: askMarker,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	resp := &registryv1.AskListResponse{}
	now := time.Now()
	for _, rec := range recs {
		if req.GetSender() != "" && rec.Fields["sender"] != req.GetSender() ||
			req.GetKind() != "" && rec.Fields["ask_kind"] != req.GetKind() {
			continue
		}
		a, err := d.askFromRecord(ctx, st, rec, now)
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
			return
		}
		if !req.GetOpenOnly() || askIsOpen(a) {
			resp.Asks = append(resp.Asks, a)
		}
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) serveAskAnswer(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.AskAnswerRequest
	if !unmarshalOr(c, f, "ask.answer", &req) {
		return
	}
	st, ok := d.recordStore(c, f, "ask.answer")
	if !ok {
		return
	}
	rec, err := d.askRecord(ctx, st, req.GetId())
	if err != nil {
		if isNotFound(err) {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.ask.answer: no question "+strconv.Quote(req.GetId()))
		} else {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		}
		return
	}
	current, err := d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	if !askIsOpen(current) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.ask.answer: question is "+current.GetState())
		return
	}
	_, ok = d.fileToastReply(ctx, c, f, &registryv1.ToastReplyRequest{
		RecordId: req.GetId(), Reply: req.GetOption(), Text: req.GetText(), Dismissed: req.GetDismissed(),
	})
	if !ok {
		return
	}
	rec, err = d.askRecord(ctx, st, req.GetId())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	a, err := d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), &registryv1.AskAnswerResponse{Ask: a})
}

func (d *Daemon) serveAskWithdraw(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.AskWithdrawRequest
	if !unmarshalOr(c, f, "ask.withdraw", &req) {
		return
	}
	st, ok := d.recordStore(c, f, "ask.withdraw")
	if !ok {
		return
	}
	who, ok := d.panelCaller(c, f, "ask.withdraw")
	if !ok {
		return
	}
	d.askMu.Lock()
	defer d.askMu.Unlock()
	rec, err := d.askRecord(ctx, st, req.GetId())
	if isNotFound(err) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "rig.ask.withdraw: no question "+strconv.Quote(req.GetId()))
		return
	}
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	if rec.Fields["sender"] != who.name {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, "rig.ask.withdraw: only the sender may withdraw this question")
		return
	}
	a, err := d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	if a.GetState() == "withdrawn" {
		c.reply(f.GetStreamId(), &registryv1.AskWithdrawResponse{Ask: a})
		return
	}
	if !askIsOpen(a) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_CONFLICT, "rig.ask.withdraw: question is "+a.GetState())
		return
	}
	_, err = st.Put(ctx, record.PutRequest{
		Kind: replyKind, Project: notificationProject,
		Fields:  map[string]string{"notification": rec.ID, "withdrawn": fieldYes, "by": who.name},
		Session: who.session, Seat: who.seat, Epoch: who.epoch,
	})
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	ans := &registryv1.ToastAnswer{
		RecordId: rec.ID, Answered: true, Withdrawn: true, By: who.name, AtUnixNano: time.Now().UnixNano(),
	}
	d.toasts.answer(rec.ID, ans)
	d.publishAskAnswer(rec, ans)
	d.toasts.withdraw(rec.ID, who.name)
	d.publishJSON("toast.retracted", toastRetraction{RecordID: rec.ID, Sender: who.name})
	a, err = d.askFromRecord(ctx, st, rec, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), &registryv1.AskWithdrawResponse{Ask: a})
}
