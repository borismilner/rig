package main

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/replyfield"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Notifications are plan/55 requirements 7 to 9 and 23 to 26: every
// notification rig filed in the record, newest first, the ones still asking
// on top, and a reply from the window answering the program that asked.
//
// ⛔ NO NEW VERB. rig.notify already files each one as a record and each
// reply as another (internal/daemon/toast.go), so the panel is a read of the
// record, and deciding is rig.toast.reply - the same call a bubble makes.

const (
	notifyProject = "notifications"
	notifyKind    = "notification"
	replyKindName = "notification-reply"

	// keepKey is requirement 8's age, a live config key; keepDefault is his
	// "one week", used when the daemon does not know the key yet.
	keepKey     = "dashboard.notifications.keep.days"
	keepDefault = 7

	// notesEvent tells the page to read again: a toast was filed, or a
	// question it shows was answered somewhere else.
	notesEvent = "rig:notifications"

	// maxNotePages bounds one read. A page is the daemon's byte budget, so
	// this is far past a week of notifications (116 in nine days, measured
	// 2026-10-03, read in one page).
	maxNotePages = 64
)

// Note is one notification, and its answer when it has one.
type Note struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Sender   string `json:"sender"`
	// At is RFC 3339, the moment rig filed it.
	At         string `json:"at"`
	Suppressed bool   `json:"suppressed"`
	// Replies are the options it offered; ReplyText says it takes free text.
	Replies   []string `json:"replies"`
	ReplyText bool     `json:"replyText"`
	Asks      bool     `json:"asks"`
	// Waiting is true while the program is still waiting for the answer.
	// An ask with no answer whose question rig no longer holds (the daemon
	// restarted since) is not waiting, and Forgotten says so.
	Waiting   bool   `json:"waiting"`
	Forgotten bool   `json:"forgotten"`
	Answer    *Reply `json:"answer"`
}

// Reply is how a notification was answered.
type Reply struct {
	Reply     string `json:"reply"`
	Text      string `json:"text"`
	Dismissed bool   `json:"dismissed"`
	By        string `json:"by"`
	At        string `json:"at"`
}

// NoteList is the panel's whole answer.
type NoteList struct {
	Notes []Note `json:"notes"`
	// KeepDays is the age in force, and KeepFrom where it came from:
	// a config layer, or "default".
	KeepDays int    `json:"keepDays"`
	KeepFrom string `json:"keepFrom"`
	// Evicted counts the notifications older than KeepDays. They stay in
	// the record; only the panel lets them go.
	Evicted int `json:"evicted"`
}

// Notifications reads the panel's notifications.
func (RigService) Notifications() (NoteList, error) {
	c, err := client.Connect()
	if err != nil {
		return NoteList{}, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*readDeadline)
	defer cancel()

	days, from := keepDays(ctx, c)
	notes, err := queryAll(ctx, c, notifyKind)
	if err != nil {
		return NoteList{}, err
	}
	replies, err := queryAll(ctx, c, replyKindName)
	if err != nil {
		return NoteList{}, err
	}
	list := assemble(notes, replies, time.Now().Add(-time.Duration(days)*24*time.Hour))
	list.KeepDays, list.KeepFrom = days, from

	// Whether an unanswered ask is still waiting is the daemon's memory, not
	// the record's: one zero-timeout rig.toast.answer each, and there are few.
	for i := range list.Notes {
		n := &list.Notes[i]
		if !n.Asks || n.Answer != nil {
			continue
		}
		var resp registryv1.ToastAnswerResponse
		err := c.Call(ctx, "rig.toast.answer", &registryv1.ToastAnswerRequest{RecordId: n.ID}, &resp)
		var ce *client.CallError
		switch {
		case errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND:
			n.Forgotten = true
		case err != nil:
			return NoteList{}, err
		case resp.GetAnswer().GetAnswered():
			n.Answer = replyOf(resp.GetAnswer())
		default:
			n.Waiting = true
			followAnswer(n.ID)
		}
	}
	return list, nil
}

// Answer replies to a notification that asks: one of its options, free
// text, or a dismissal. The daemon records the person at this screen as the
// one who answered, and the program that asked is told.
func (RigService) Answer(id, reply, text string, dismissed bool) (Reply, error) {
	if id == "" {
		return Reply{}, errors.New("no notification named")
	}
	c, err := client.Connect()
	if err != nil {
		return Reply{}, err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	var resp registryv1.ToastReplyResponse
	err = c.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{
		RecordId: id, Reply: reply, Text: text, Dismissed: dismissed,
	}, &resp)
	var ce *client.CallError
	if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND {
		return Reply{}, errors.New("rig holds no such question: it restarted since this was asked, or never asked it, so nobody is waiting for the answer")
	}
	if err != nil {
		return Reply{}, err
	}
	emitNotes()
	return *replyOf(resp.GetAnswer()), nil
}

// keepDays reads requirement 8's age. Any failure is the default rather than
// an error: a daemon older than the key still has notifications to show.
func keepDays(ctx context.Context, c *client.Client) (int, string) {
	var resp registryv1.ConfigGetResponse
	if err := c.Call(ctx, "rig.config.get", &registryv1.ConfigGetRequest{Prefix: keepKey}, &resp); err != nil {
		return keepDefault, "default"
	}
	for _, v := range resp.GetValues() {
		if v.GetKey() != keepKey {
			continue
		}
		var n int
		if err := json.Unmarshal([]byte(v.GetWinner().GetValueJson()), &n); err != nil || n < 1 {
			return keepDefault, "default"
		}
		return n, v.GetWinner().GetLayer()
	}
	return keepDefault, "default"
}

func queryAll(ctx context.Context, c *client.Client, kind string) ([]*verbsv1.Record, error) {
	var out []*verbsv1.Record
	after := ""
	for range maxNotePages {
		var resp verbsv1.RecordQueryResponse
		if err := c.Call(ctx, "rig.record.query", &verbsv1.RecordQueryRequest{
			Project: notifyProject, Kind: kind, After: after,
		}, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.GetRecords()...)
		if after = resp.GetNext(); after == "" {
			return out, nil
		}
	}
	return out, errors.New("more than " + strconv.Itoa(maxNotePages) + " pages of " + kind + " records; the panel reads no further")
}

// assemble joins notifications to their replies, drops the ones filed before
// since, and orders the rest newest first. What still waits goes on top in
// the page, once Notifications has asked the daemon which ones do.
func assemble(notes, replies []*verbsv1.Record, since time.Time) NoteList {
	answers := map[string]*Reply{}
	for _, r := range replies {
		f := r.GetFields()
		id := f["notification"]
		if id == "" || answers[id] != nil {
			continue // the daemon refuses a second answer; the first stands
		}
		answers[id] = &Reply{
			Reply: f["reply"], Text: r.GetBody(), Dismissed: f["dismissed"] == "yes",
			By: f["by"], At: stampOf(r.GetProv().GetAtUnixNano()),
		}
	}
	list := NoteList{Notes: []Note{}}
	cut := since.UnixNano()
	for _, r := range notes {
		at := r.GetProv().GetAtUnixNano()
		if at < cut {
			list.Evicted++
			continue
		}
		f := r.GetFields()
		n := Note{
			ID: r.GetId(), Severity: f["severity"], Title: f["title"], Body: r.GetBody(),
			Sender: f["sender"], At: stampOf(at), Suppressed: f["suppressed"] != "",
			ReplyText: f["reply_text"] == "yes", Replies: []string{}, Answer: answers[r.GetId()],
		}
		n.Replies = append(n.Replies, replyfield.Decode(f["replies"])...)
		_, asked := f["replies"]
		n.Asks = asked || n.ReplyText
		list.Notes = append(list.Notes, n)
	}
	// RFC 3339 in UTC sorts as text; the id breaks a tie, and UUIDv7 ids
	// rise with time.
	sort.SliceStable(list.Notes, func(i, j int) bool {
		a, b := list.Notes[i], list.Notes[j]
		if a.At != b.At {
			return a.At > b.At
		}
		return a.ID > b.ID
	})
	return list
}

func replyOf(a *registryv1.ToastAnswer) *Reply {
	return &Reply{
		Reply: a.GetReply(), Text: a.GetText(), Dismissed: a.GetDismissed(),
		By: a.GetBy(), At: stampOf(a.GetAtUnixNano()),
	}
}

func stampOf(ns int64) string {
	if ns == 0 {
		return ""
	}
	return time.Unix(0, ns).UTC().Format(time.RFC3339Nano)
}

// The page is told to read again through one Wails event. emit is set by
// runWindow; the tray process and the tests have no page, so it stays a
// no-op there.
var (
	emitMu sync.Mutex
	emit   = func() {}

	followMu  sync.Mutex
	following = map[string]bool{}
)

func setEmit(f func()) {
	emitMu.Lock()
	emit = f
	emitMu.Unlock()
}

func emitNotes() {
	emitMu.Lock()
	f := emit
	emitMu.Unlock()
	f()
}

// followAnswer watches one waiting question until it is answered anywhere -
// a bubble, a terminal, another window - and then tells the page. One
// watcher per question for the life of the window process.
func followAnswer(id string) {
	followMu.Lock()
	defer followMu.Unlock()
	if following[id] {
		return
	}
	following[id] = true
	go func() {
		//rig:allow nocontextfree: the watch ends when the question is answered or the window process exits, so it has no deadline
		watchAnswer(context.Background(), id, func(answerJSON) { emitNotes() })
		followMu.Lock()
		delete(following, id)
		followMu.Unlock()
	}()
}

// watchNotes tells the page about every toast rig files while the window is
// open. The same long poll the tray makes (watchToasts); a detached daemon
// is retried on the tray's cadence.
func watchNotes() {
	var after uint64
	primed := false
	for {
		resp, err := toastWait(after, primed)
		if err != nil {
			time.Sleep(trayRefresh)
			primed = false
			continue
		}
		switch {
		case !primed, resp.GetLatest() < after:
			// The first answer, or the first since rig went away: the
			// cursor is this daemon run's, so a restarted one starts it
			// again, and what was filed meanwhile is read once.
			emitNotes()
			after, primed = resp.GetLatest(), true
		case len(resp.GetToasts()) > 0:
			emitNotes()
			after = max(after, resp.GetLatest())
		}
	}
}
