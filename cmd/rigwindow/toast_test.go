package main

import (
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

func toastsOf(seqs ...uint64) []*registryv1.Toast {
	var out []*registryv1.Toast
	for _, s := range seqs {
		out = append(out, &registryv1.Toast{Seq: s, Severity: registryv1.Severity_SEVERITY_INFO, Title: "t"})
	}
	return out
}

// One renderer at a time, started from the first toast's cursor; a second
// batch while it lives starts nothing, because the renderer waits itself.
func TestTheTrayStartsOneRendererFromTheFirstToast(t *testing.T) {
	var mu sync.Mutex
	var spawned []uint64
	exit := make(chan error, 1)
	w := &toastWatcher{
		spawn: func(after uint64) (<-chan error, error) {
			mu.Lock()
			defer mu.Unlock()
			spawned = append(spawned, after)
			return exit, nil
		},
		fallback: func(*registryv1.Toast) (uint32, error) { t.Error("fell back with a renderer running"); return 0, nil },
		warn:     func(string) {},
		grace:    time.Millisecond,
	}
	w.deliver(toastsOf(7, 8))
	w.deliver(toastsOf(9))
	mu.Lock()
	if len(spawned) != 1 || spawned[0] != 6 {
		t.Fatalf("spawned %v, want one renderer after cursor 6", spawned)
	}
	mu.Unlock()

	time.Sleep(10 * time.Millisecond) // past the died-at-start window
	exit <- nil
	deadline := time.Now().Add(2 * time.Second)
	for {
		w.mu.Lock()
		running := w.running
		w.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the renderer's exit was not noticed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	w.deliver(toastsOf(10))
	exit <- nil // the second renderer leaves too, so nothing outlives the test
	mu.Lock()
	defer mu.Unlock()
	if len(spawned) != 2 || spawned[1] != 9 {
		t.Fatalf("after the renderer left, spawned %v, want a second from cursor 9", spawned)
	}
}

// A renderer that cannot start, or dies at start, sends the batch to the
// desktop's notification service instead.
func TestARendererThatCannotStartFallsBackToTheDesktop(t *testing.T) {
	var mu sync.Mutex
	var fell []uint64
	fb := func(tt *registryv1.Toast) (uint32, error) {
		mu.Lock()
		fell = append(fell, tt.GetSeq())
		mu.Unlock()
		return 0, nil
	}

	w := &toastWatcher{
		spawn:    func(uint64) (<-chan error, error) { return nil, errors.New("no display") },
		fallback: fb, warn: func(string) {},
	}
	w.deliver(toastsOf(1, 2))

	dead := make(chan error, 1)
	dead <- errors.New("exit status 1")
	w2 := &toastWatcher{
		spawn:    func(uint64) (<-chan error, error) { return dead, nil },
		fallback: fb, warn: func(string) {},
	}
	w2.deliver(toastsOf(3))
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(fell) != 3 {
		t.Fatalf("fell back %v, want 1, 2 and 3", fell)
	}
}

// The renderer quits only once every bubble has left, nothing waits, and the
// page has stayed empty for the linger.
func TestTheRendererLeavesOnlyWhenTheLastBubbleHasGone(t *testing.T) {
	f := &toastFeed{}
	now := time.Now()
	if f.done(now) {
		t.Fatal("a renderer that has drawn nothing yet quit")
	}
	f.add(toastsOf(1))
	if got := f.poll(0, inputRegion{}, now); len(got.Toasts) != 1 || got.Toasts[0].Severity != "info" {
		t.Fatalf("the page was handed %v", got)
	}
	f.poll(1, inputRegion{Top: 900, Height: 120}, now)
	if band, up := f.input(); !up || band.Height != 120 || f.done(now.Add(time.Hour)) {
		t.Fatal("a renderer with a bubble up would quit, or its bubbles do not take the pointer")
	}
	f.poll(0, inputRegion{}, now)
	if _, up := f.input(); up {
		t.Fatal("an empty page still takes the pointer")
	}
	if f.done(now.Add(toastLinger / 2)) {
		t.Fatal("the renderer quit before the linger")
	}
	if !f.done(now.Add(2 * toastLinger)) {
		t.Fatal("the renderer outlived its last bubble")
	}
}

func TestTheFiveSeveritiesMapOntoFreedesktopUrgency(t *testing.T) {
	want := map[registryv1.Severity]byte{
		registryv1.Severity_SEVERITY_INFO: 0, registryv1.Severity_SEVERITY_SUCCESS: 1,
		registryv1.Severity_SEVERITY_WARNING: 1, registryv1.Severity_SEVERITY_ERROR: 2,
		registryv1.Severity_SEVERITY_URGENT: 2,
	}
	for s, u := range want {
		if got := freedesktopUrgency(s); got != u {
			t.Errorf("%s maps to urgency %d, want %d", s, got, u)
		}
	}
}

// The tray is on whichever edge the panel took room from. Boris's panel is at
// the bottom, and the first build assumed GNOME's top bar.
func TestTheBubblesFaceThePanelsEdge(t *testing.T) {
	screen := application.Rect{X: 0, Y: 0, Width: 3000, Height: 1920}
	for _, c := range []struct {
		name string
		wa   application.Rect
		want string
	}{
		{"bottom panel", application.Rect{X: 0, Y: 0, Width: 3000, Height: 1860}, "bottom"},
		{"top bar", application.Rect{X: 0, Y: 48, Width: 3000, Height: 1872}, "top"},
		{"no panel", screen, "top"},
	} {
		if got := panelEdge(screen, c.wa); got != c.want {
			t.Errorf("%s: panelEdge = %q, want %q", c.name, got, c.want)
		}
	}
}

// Copy is a POST with a bounded body, and only the renderer can do it.
func TestCopyPutsTheTextOnTheClipboardAndNothingElse(t *testing.T) {
	var got []string
	f := &toastFeed{copyText: func(s string) { got = append(got, s) }}
	for _, c := range []struct {
		method, body string
		want         int
	}{
		{"POST", "title\n\nbody", 204},
		{"GET", "", 405},
		{"POST", strings.Repeat("x", maxCopy+1), 413},
	} {
		rec := httptest.NewRecorder()
		f.ServeHTTP(rec, httptest.NewRequest(c.method, "/toast/copy", strings.NewReader(c.body)))
		if rec.Code != c.want {
			t.Errorf("%s %d bytes: status %d, want %d", c.method, len(c.body), rec.Code, c.want)
		}
	}
	if len(got) != 1 || got[0] != "title\n\nbody" {
		t.Errorf("clipboard got %q, want exactly the one good copy", got)
	}
	rec := httptest.NewRecorder()
	(&toastFeed{}).ServeHTTP(rec, httptest.NewRequest("POST", "/toast/copy", strings.NewReader("x")))
	if rec.Code != 405 {
		t.Errorf("a feed with no clipboard answered %d, want 405", rec.Code)
	}
}

// A bubble's reply goes to rig as the request it was, a refusal comes back
// for the bubble to show, and an asking toast is followed until it is
// answered, so an answer given anywhere reaches the page.
func TestARepliedToastReachesRigAndItsAnswerReachesThePage(t *testing.T) {
	var sent []*registryv1.ToastReplyRequest
	var watched []string
	f := &toastFeed{
		reply: func(r *registryv1.ToastReplyRequest) error {
			sent = append(sent, r)
			if r.GetReply() == "No" {
				return errors.New("CODE_CONFLICT: already answered")
			}
			return nil
		},
		watch: func(id string, answered func(answerJSON)) {
			watched = append(watched, id)
			answered(answerJSON{RecordID: id, Reply: "Yes", By: "terminal:boris"})
		},
	}
	for _, c := range []struct {
		method, body string
		want         int
	}{
		{"POST", `{"record_id":"n1","reply":"Yes"}`, 204},
		{"POST", `{"record_id":"n1","reply":"No"}`, 409},
		{"GET", "", 405},
		{"POST", `not json`, 400},
	} {
		rec := httptest.NewRecorder()
		f.ServeHTTP(rec, httptest.NewRequest(c.method, "/toast/reply", strings.NewReader(c.body)))
		if rec.Code != c.want {
			t.Errorf("%s %s: status %d, want %d", c.method, c.body, rec.Code, c.want)
		}
	}
	if len(sent) != 2 || sent[0].GetRecordId() != "n1" || sent[0].GetReply() != "Yes" {
		t.Fatalf("rig was sent %v", sent)
	}

	f.add([]*registryv1.Toast{
		{Seq: 1, RecordId: "plain", Severity: registryv1.Severity_SEVERITY_INFO, Title: "no question"},
		{Seq: 2, RecordId: "asks", Severity: registryv1.Severity_SEVERITY_URGENT, Title: "run it?", Replies: []string{"Yes", "No"}},
	})
	time.Sleep(50 * time.Millisecond) // the watch runs on its own goroutine
	got := f.poll(0, inputRegion{}, time.Now())
	if len(got.Toasts) != 2 || got.Toasts[1].RecordID != "asks" || len(got.Toasts[1].Replies) != 2 {
		t.Fatalf("the page was handed %+v", got.Toasts)
	}
	if len(got.Answers) != 1 || got.Answers[0].RecordID != "asks" || len(watched) != 1 {
		t.Fatalf("answers %+v after watching %v; only the asking toast is followed", got.Answers, watched)
	}
}

// plan/53 slice 7: a toast its sender took back. With no renderer running
// there is no bubble to take down, so none is started; a renderer drops one
// it has not shown yet, and tells the page about one it has.
func TestAWithdrawalStartsNothingAndTakesTheBubbleDown(t *testing.T) {
	w := &toastWatcher{
		spawn:        func(uint64) (<-chan error, error) { t.Error("a withdrawal started a renderer"); return nil, nil },
		fallback:     func(*registryv1.Toast) (uint32, error) { t.Error("a withdrawal went to the desktop"); return 0, nil },
		closeDesktop: func(uint32) error { t.Error("closed a desktop toast that was never put up"); return nil },
		warn:         func(string) {},
	}
	w.deliver([]*registryv1.Toast{{Seq: 3, RecordId: "r1", Retracted: true}})

	f := &toastFeed{}
	f.add([]*registryv1.Toast{{Seq: 1, RecordId: "unseen"}, {Seq: 2, RecordId: "unseen", Retracted: true}})
	if got := f.poll(0, inputRegion{}, time.Now()); len(got.Toasts) != 0 || len(got.Answers) != 0 {
		t.Fatalf("a toast taken back before it was shown reached the page: %+v", got)
	}
	f.add([]*registryv1.Toast{{Seq: 3, RecordId: "shown"}})
	f.poll(0, inputRegion{}, time.Now())
	f.add([]*registryv1.Toast{{Seq: 4, RecordId: "shown", Sender: "backend-1", Retracted: true}})
	got := f.poll(1, inputRegion{}, time.Now())
	if len(got.Answers) != 1 || !got.Answers[0].Withdrawn || got.Answers[0].RecordID != "shown" || got.Answers[0].By != "backend-1" {
		t.Fatalf("the page was not told the bubble was withdrawn: %+v", got)
	}
}

// A toast that went to the desktop because no renderer could start is taken
// down there when its sender takes it back, and only that one. One taken back
// before a renderer that died at start was replaced is never put up.
func TestAWithdrawalTakesDownWhatTheDesktopShows(t *testing.T) {
	var mu sync.Mutex
	var closed []uint32
	var shown []string
	w := &toastWatcher{
		spawn: func(uint64) (<-chan error, error) { return nil, errors.New("no display") },
		fallback: func(tt *registryv1.Toast) (uint32, error) {
			mu.Lock()
			defer mu.Unlock()
			shown = append(shown, tt.GetRecordId())
			return uint32(100 + len(shown)), nil
		},
		closeDesktop: func(id uint32) error { mu.Lock(); closed = append(closed, id); mu.Unlock(); return nil },
		warn:         func(string) {},
	}
	w.deliver([]*registryv1.Toast{{Seq: 1, RecordId: "a"}, {Seq: 2, RecordId: "b"}})
	w.deliver([]*registryv1.Toast{{Seq: 3, RecordId: "b", Retracted: true}})
	w.deliver([]*registryv1.Toast{{Seq: 4, RecordId: "b", Retracted: true}})
	mu.Lock()
	if len(closed) != 1 || closed[0] != 102 {
		t.Fatalf("closed %v, want only b's desktop id 102, once", closed)
	}
	mu.Unlock()

	dead := make(chan error, 1)
	w.spawn = func(uint64) (<-chan error, error) { return dead, nil }
	w.deliver([]*registryv1.Toast{{Seq: 5, RecordId: "c"}})
	w.deliver([]*registryv1.Toast{{Seq: 6, RecordId: "c", Retracted: true}})
	dead <- errors.New("exit status 1")
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(shown) != 2 {
		t.Fatalf("the desktop was shown %v; c was taken back before it fell back", shown)
	}
}

// A question a restarted daemon replays is not drawn twice by a renderer that
// already has it; a new toast still is.
func TestAReplayedQuestionIsNotDrawnTwice(t *testing.T) {
	f := &toastFeed{}
	q := &registryv1.Toast{Seq: 1, RecordId: "r-1", Severity: registryv1.Severity_SEVERITY_INFO, Title: "now?", Replies: []string{"Now"}}
	f.add([]*registryv1.Toast{q})
	if got := f.poll(0, inputRegion{}, time.Now()); len(got.Toasts) != 1 {
		t.Fatalf("the page was handed %v", got)
	}
	f.add([]*registryv1.Toast{{Seq: 1, RecordId: "r-1", Title: "now?", Replies: []string{"Now"}}, {Seq: 2, RecordId: "r-2", Title: "new"}})
	if got := f.poll(1, inputRegion{}, time.Now()); len(got.Toasts) != 1 || got.Toasts[0].RecordID != "r-2" {
		t.Fatalf("after the replay the page was handed %v, want the new toast alone", got)
	}
}
