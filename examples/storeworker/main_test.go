package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/client/clienttest"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// start registers a storeworker against a real daemon and runs its worker
// until the test ends.
func start(t *testing.T, budget float64) *app {
	t.Helper()
	clienttest.StartStored(t, "development")
	c, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	a := newApp(c, "storeworker", "storeworker", budget, time.Millisecond)
	c.Handle(a.handle)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := c.Hello(ctx, a.declaration("http://127.0.0.1:7454/pane")); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { a.work(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return a
}

// waitFor polls the run until it reaches state.
func waitFor(t *testing.T, a *app, id, state string) run {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		r, _, err := a.getRun(context.Background(), id)
		if err == nil && r.State == state {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	r, _, _ := a.getRun(context.Background(), id)
	t.Fatalf("run %s is %q, want %q", id, r.State, state)
	return run{}
}

// A run goes from the store, through the queue and the gpu lease, to done
// with its transcript written, and the day's count moved with it.
func TestARunGoesFromAssignedToDone(t *testing.T) {
	a := start(t, 1)
	ctx := context.Background()
	out, err := a.assign(ctx, assignArgs{Title: "summarise", Prompt: "three lines"})
	if err != nil {
		t.Fatal(err)
	}
	r := waitFor(t, a, out["run"].(string), stateDone)
	if r.Finished == "" {
		t.Fatalf("a done run has no finish time: %+v", r)
	}
	list, err := a.runs(ctx, runsArgs{State: stateDone})
	if err != nil {
		t.Fatal(err)
	}
	if n := list["counts"].(map[string]uint64)[stateDone]; n != 1 {
		t.Fatalf("counted %d done runs, want 1", n)
	}
}

// A run over the budget parks until it is answered, and a no stops it.
func TestAnExpensiveRunWaitsForItsAnswer(t *testing.T) {
	a := start(t, 0.01)
	ctx := context.Background()
	out, err := a.assign(ctx, assignArgs{Title: "big", Prompt: strings.Repeat("x", 100)})
	if err != nil {
		t.Fatal(err)
	}
	id := out["run"].(string)
	r := waitFor(t, a, id, stateWaiting)
	if !strings.Contains(r.Question, "over the $0.01 budget") {
		t.Fatalf("the question is %q", r.Question)
	}
	if _, err := a.answer(ctx, answerArgs{Run: "r-nobody", Yes: true}); code(err).String() != "CODE_NOT_FOUND" {
		t.Fatalf("answering a run not waiting: %v", err)
	}
	// Through rig, as a terminal would.
	you, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer you.Close()
	w := &web{a: a, addr: "x", you: you}
	if _, err := w.invoke(ctx, "answer", `{"run":"`+id+`","yes":false}`); err != nil {
		t.Fatal(err)
	}
	waitFor(t, a, id, stateDenied)

	// Answered at the terminal, so storeworker told the toast: the bubble
	// would now say "No" and close rather than ask a settled question.
	ans := toastAnswer(t, you, askedToast(t, a))
	if ans.GetReply() != replyNo || ans.GetBy() == "" {
		t.Fatalf("the toast was left saying %+v", ans)
	}
}

// Answering the budget question in the toast alone settles the run: the
// reply reaches storeworker through toast.answer with no page and no
// terminal involved.
func TestAToastReplyAnswersAnExpensiveRun(t *testing.T) {
	a := start(t, 0.01)
	ctx := context.Background()
	out, err := a.assign(ctx, assignArgs{Title: "big", Prompt: strings.Repeat("x", 100)})
	if err != nil {
		t.Fatal(err)
	}
	id := out["run"].(string)
	waitFor(t, a, id, stateWaiting)

	you, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer you.Close()
	if err := you.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{
		RecordId: askedToast(t, a), Reply: replyYes,
	}, &registryv1.ToastReplyResponse{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, a, id, stateDone)
}

// askedToast is the record id of the newest toast storeworker filed that
// asks for a reply. The run is stored as waiting just before the toast goes
// out, so it waits for one.
func askedToast(t *testing.T, a *app) string {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		a.mu.Lock()
		for i := len(a.toasts) - 1; i >= 0; i-- {
			if id := a.toasts[i].ID; id != "" {
				a.mu.Unlock()
				return id
			}
		}
		a.mu.Unlock()
	}
	t.Fatal("storeworker filed no toast that asks")
	return ""
}

func toastAnswer(t *testing.T, c *client.Client, id string) *registryv1.ToastAnswer {
	t.Helper()
	var resp registryv1.ToastAnswerResponse
	if err := c.Call(context.Background(), "rig.toast.answer", &registryv1.ToastAnswerRequest{
		RecordId: id, TimeoutMs: 5000,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.GetAnswer()
}

// The page's API answers only its own page: the served Host and the header
// a cross-site request cannot send without a preflight.
func TestTheAPIRefusesARequestFromElsewhere(t *testing.T) {
	a := start(t, 1)
	w := &web{a: a, addr: "127.0.0.1:7454"}
	h := w.handler(http.Dir(t.TempDir()))
	for _, tc := range []struct {
		name, host, header string
		want               int
	}{
		{"its own page", "127.0.0.1:7454", "1", http.StatusOK},
		{"no header", "127.0.0.1:7454", "", http.StatusForbidden},
		{"a rebound name", "evil.example:7454", "1", http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/tab/store", nil)
		req.Host = tc.host
		if tc.header != "" {
			req.Header.Set("X-Storeworker", tc.header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s: %d, want %d", tc.name, rec.Code, tc.want)
		}
		if tc.want == http.StatusOK {
			var d map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || d["runs"] == nil {
				t.Fatalf("%s: %v %s", tc.name, err, rec.Body.String())
			}
		}
	}
}
