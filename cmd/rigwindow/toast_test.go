package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

func toastsOf(seqs ...uint64) []*verbsv1.Toast {
	var out []*verbsv1.Toast
	for _, s := range seqs {
		out = append(out, &verbsv1.Toast{Seq: s, Severity: verbsv1.Severity_SEVERITY_INFO, Title: "t"})
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
		fallback: func(*verbsv1.Toast) error { t.Error("fell back with a renderer running"); return nil },
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
	fb := func(tt *verbsv1.Toast) error { mu.Lock(); fell = append(fell, tt.GetSeq()); mu.Unlock(); return nil }

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
	if got := f.poll(0, 40, now); len(got) != 1 || got[0].Severity != "info" {
		t.Fatalf("the page was handed %v", got)
	}
	f.poll(1, 120, now)
	if f.size() != 120 || f.done(now.Add(time.Hour)) {
		t.Fatal("a renderer with a bubble up would quit, or is not sized to it")
	}
	f.poll(0, 30, now)
	if f.done(now.Add(toastLinger / 2)) {
		t.Fatal("the renderer quit before the linger")
	}
	if !f.done(now.Add(2 * toastLinger)) {
		t.Fatal("the renderer outlived its last bubble")
	}
}

func TestTheFiveSeveritiesMapOntoFreedesktopUrgency(t *testing.T) {
	want := map[verbsv1.Severity]byte{
		verbsv1.Severity_SEVERITY_INFO: 0, verbsv1.Severity_SEVERITY_SUCCESS: 1,
		verbsv1.Severity_SEVERITY_WARNING: 1, verbsv1.Severity_SEVERITY_ERROR: 2,
		verbsv1.Severity_SEVERITY_URGENT: 2,
	}
	for s, u := range want {
		if got := freedesktopUrgency(s); got != u {
			t.Errorf("%s maps to urgency %d, want %d", s, got, u)
		}
	}
}
