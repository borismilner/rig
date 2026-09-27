package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// app is the program: one connection to rig, the state its page shows, and
// the log of every call it made, which the page lists so each click can be
// seen turning into rig verbs.
type app struct {
	c     *client.Client
	id    string
	seat  string
	queue string

	budget float64       // a run costing more than this parks for an answer
	step   time.Duration // how long one simulated step takes

	mu       sync.Mutex
	calls    []callRow  // newest last, at most maxCalls
	toasts   []toastRow // newest last, at most maxToasts
	worker   workerView
	export   exportView
	answers  map[string]chan bool // a parked run waits on its channel
	health   string               // why health.report is not accepted, if it is not
	reported string               // the last health report, so a repeat is not logged
}

const (
	maxCalls  = 60
	maxToasts = 20

	// replyPoll is one toast.answer long poll; Rig caps it at 60 s.
	replyPoll = 55 * time.Second
)

type callRow struct {
	At     string `json:"at"`
	Verb   string `json:"verb"`
	Note   string `json:"note"`
	Failed bool   `json:"failed"`
}

type toastRow struct {
	At       string `json:"at"`
	Severity string `json:"severity"`
	Title    string `json:"title"`

	// A toast that asks carries its record id, what it offers (the buttons,
	// or "free text"), and the answer once one arrives.
	ID     string `json:"id,omitempty"`
	Asks   string `json:"asks,omitempty"`
	Answer string `json:"answer,omitempty"`
}

// workerView is what the worker is doing, for the page.
type workerView struct {
	Activity string `json:"activity"`
	Run      string `json:"run"`
	Claim    string `json:"claim"` // the queue claim's lease name and token
	Lease    string `json:"lease"` // the gpu lease, when held
	Done     uint64 `json:"done"`  // the health marker
	Parked   string `json:"parked"`
}

type exportView struct {
	Commit   string `json:"commit"`
	Dir      string `json:"dir"`
	Snapshot string `json:"snapshot"`
	Note     string `json:"note"`
}

func newApp(c *client.Client, id, seat string, budget float64, step time.Duration) *app {
	return &app{
		c: c, id: id, seat: seat, queue: id + "-runs",
		budget: budget, step: step, answers: map[string]chan bool{},
	}
}

// call is every rig call this program makes, logged for the page. note
// describes the call in a few words; the result is logged as it came.
func (a *app) call(ctx context.Context, verb, note string, in, out proto.Message) error {
	err := a.c.Call(ctx, "rig."+verb, in, out)
	if ctx.Value(quietKey{}) != nil {
		return err
	}
	row := callRow{At: time.Now().Format("15:04:05"), Verb: verb, Note: note}
	if err != nil {
		row.Failed = true
		row.Note = note + ": " + short(err.Error())
	}
	a.logRow(row)
	return err
}

// logRow keeps the newest maxCalls rows.
func (a *app) logRow(row callRow) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, row)
	if len(a.calls) > maxCalls {
		a.calls = a.calls[len(a.calls)-maxCalls:]
	}
}

// quietKey marks the page's own polling reads, which are not logged: every
// two seconds of them would push the calls a click made off the list. Each
// tab names the verbs its view is read with instead.
type quietKey struct{}

func quietly(ctx context.Context) context.Context {
	return context.WithValue(ctx, quietKey{}, true)
}

func short(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > 140 {
		return s[:140] + "..."
	}
	return s
}

// code is the refusal's code, or unspecified when err is not a refusal.
func code(err error) rigv1.Code {
	var ce *client.CallError
	if errors.As(err, &ce) {
		return ce.Code()
	}
	return rigv1.Code_CODE_UNSPECIFIED
}

// notify puts a toast in the tray (rig.notify) and remembers it for the page.
func (a *app) notify(ctx context.Context, sev registryv1.Severity, title, body string) {
	a.ask(ctx, sev, title, body, nil, false)
}

// ask files a toast that takes a reply: a button per entry in replies, and
// a text field when text is set. It answers the toast's record id, which
// awaitReply and closeAsk name it by, or "" if Rig refused it. With no
// replies and no text it is an ordinary toast.
func (a *app) ask(ctx context.Context, sev registryv1.Severity, title, body string, replies []string, text bool) string {
	word := strings.ToLower(strings.TrimPrefix(sev.String(), "SEVERITY_"))
	var resp registryv1.NotifyResponse
	err := a.call(ctx, "notify", word+": "+title, &registryv1.NotifyRequest{
		Severity: sev, Title: title, Body: body, Replies: replies, ReplyText: text,
	}, &resp)
	row := toastRow{At: time.Now().Format("15:04:05"), Title: title, Severity: word}
	if err == nil && (len(replies) > 0 || text) {
		row.ID = resp.GetToast().GetRecordId()
		row.Asks = strings.Join(replies, " / ")
		if text {
			row.Asks = strings.TrimPrefix(row.Asks+" / free text", " / ")
		}
	}
	a.mu.Lock()
	a.toasts = append(a.toasts, row)
	if len(a.toasts) > maxToasts {
		a.toasts = a.toasts[len(a.toasts)-maxToasts:]
	}
	a.mu.Unlock()
	return row.ID
}

// awaitReply waits for the answer to a toast that asked (toast.answer, a
// long poll), and notes it on the toast's row. It answers nil when ctx ends
// first or Rig no longer knows the toast.
func (a *app) awaitReply(ctx context.Context, id string) *registryv1.ToastAnswer {
	if id == "" {
		return nil
	}
	for ctx.Err() == nil {
		var resp registryv1.ToastAnswerResponse
		err := a.c.Call(ctx, "rig.toast.answer", &registryv1.ToastAnswerRequest{
			RecordId: id, TimeoutMs: uint32(replyPoll / time.Millisecond),
		}, &resp)
		if err != nil {
			if ctx.Err() == nil {
				a.logRow(callRow{At: time.Now().Format("15:04:05"), Verb: "toast.answer", Note: id + ": " + short(err.Error()), Failed: true})
			}
			return nil
		}
		ans := resp.GetAnswer()
		if !ans.GetAnswered() {
			continue
		}
		said := "closed without an answer"
		switch {
		case ans.GetReply() != "":
			said = ans.GetReply()
		case ans.GetText() != "":
			said = "\"" + ans.GetText() + "\""
		}
		a.logRow(callRow{At: time.Now().Format("15:04:05"), Verb: "toast.answer", Note: id + ": " + said + " by " + ans.GetBy()})
		a.mu.Lock()
		for i := range a.toasts {
			if a.toasts[i].ID == id {
				a.toasts[i].Answer = said + ", by " + ans.GetBy()
			}
		}
		a.mu.Unlock()
		return ans
	}
	return nil
}

// closeAsk answers a toast on the person's behalf (toast.reply) when they
// answered somewhere else, so the bubble says so and leaves. Rig takes the
// first answer only, so when the toast itself was answered first this is
// refused, which is the right outcome and is not logged as a failure.
func (a *app) closeAsk(ctx context.Context, id, reply string) {
	if id == "" {
		return
	}
	err := a.c.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{RecordId: id, Reply: reply}, &registryv1.ToastReplyResponse{})
	if err == nil {
		a.logRow(callRow{At: time.Now().Format("15:04:05"), Verb: "toast.reply", Note: id + ": answered elsewhere, " + reply})
	}
}

// setWorker changes what the page shows the worker doing, and tells rig the
// seat's activity when it changed.
func (a *app) setWorker(ctx context.Context, f func(*workerView)) {
	a.mu.Lock()
	before := a.worker.Activity
	f(&a.worker)
	after := a.worker.Activity
	a.mu.Unlock()
	if after != before && after != "" {
		_ = a.call(ctx, "activity", after, &verbsv1.ActivityRequest{
			Activity: after, State: verbsv1.SeatState_SEAT_STATE_ACTIVE,
		}, &verbsv1.ActivityResponse{})
	}
}

// report is rig.health.report: the marker moves with every finished run, and
// parked carries the question waiting for Boris. It is accepted only from a
// program rig started (rig up); run by hand, the page says why.
func (a *app) report(ctx context.Context, waiting string) {
	a.mu.Lock()
	marker, parked := a.worker.Done, a.worker.Parked
	said := fmt.Sprint(marker, waiting, parked)
	if said == a.reported {
		ctx = quietly(ctx) // the same report again, sent so rig sees it alive
	}
	a.reported = said
	a.mu.Unlock()
	err := a.call(ctx, "health.report", fmt.Sprintf("marker %d", marker), &rigv1.HealthReportRequest{
		Marker: marker, Waiting: waiting, Parked: parked,
	}, &rigv1.HealthReportResponse{})
	a.mu.Lock()
	a.health = ""
	if err != nil {
		a.health = "not supervised, so rig keeps no health for it: start it with `rig up " + a.id + "`"
	}
	a.mu.Unlock()
}

// run is one assignment, as stored in the runs collection.
type run struct {
	Title      string  `json:"title"`
	Prompt     string  `json:"prompt"`
	State      string  `json:"state"`
	Cost       float64 `json:"cost"`
	ReplyTo    string  `json:"reply_to,omitempty"`
	Question   string  `json:"question,omitempty"`
	Transcript string  `json:"transcript,omitempty"`
	Created    string  `json:"created"`
	Finished   string  `json:"finished,omitempty"`
}

// The run states, in the order a run moves through them.
const (
	stateQueued  = "queued"
	stateRunning = "running"
	stateWaiting = "waiting"
	stateDone    = "done"
	stateFailed  = "failed"
	stateDenied  = "denied"
)

var states = []string{stateQueued, stateRunning, stateWaiting, stateDone, stateFailed, stateDenied}

// getRun reads one run and its version (store.get).
func (a *app) getRun(ctx context.Context, id string) (run, uint64, error) {
	var resp verbsv1.StoreGetResponse
	if err := a.call(ctx, "store.get", "runs/"+id, &verbsv1.StoreGetRequest{
		Collection: "runs", Ids: []string{id},
	}, &resp); err != nil {
		return run{}, 0, err
	}
	if len(resp.GetDocuments()) == 0 {
		return run{}, 0, fmt.Errorf("no run %q", id)
	}
	d := resp.GetDocuments()[0]
	var r run
	if err := json.Unmarshal([]byte(d.GetDocument()), &r); err != nil {
		return run{}, 0, err
	}
	return r, d.GetVersion(), nil
}

// putRun writes a run at the version read (store.put, compare-and-swap). A
// conflict means somebody else wrote it first, and the caller re-reads.
func (a *app) putRun(ctx context.Context, id string, version uint64, r run, note string) (uint64, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return 0, err
	}
	var resp verbsv1.StorePutResponse
	err = a.call(ctx, "store.put", fmt.Sprintf("runs/%s v%d: %s", id, version, note), &verbsv1.StorePutRequest{
		Collection: "runs", Id: id, ExpectedVersion: version, Document: string(b),
	}, &resp)
	return resp.GetVersion(), err
}

// update reads, changes and writes a run, re-reading on a conflict: the
// page, the terminal and the worker all write runs.
func (a *app) update(ctx context.Context, id, note string, f func(*run)) (run, error) {
	for range 5 {
		r, v, err := a.getRun(ctx, id)
		if err != nil {
			return run{}, err
		}
		f(&r)
		_, err = a.putRun(ctx, id, v, r, note)
		if code(err) == rigv1.Code_CODE_CONFLICT {
			continue
		}
		return r, err
	}
	return run{}, fmt.Errorf("run %s kept changing under five writes", id)
}
