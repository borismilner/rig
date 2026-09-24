package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeWindow is a window process the test ends by hand.
type fakeWindow struct {
	stops  int
	exited chan struct{}
}

func (f *fakeWindow) stop() { f.stops++ }

type fakeSpawner struct {
	mu      sync.Mutex
	started []*fakeWindow
	fail    error
}

func (f *fakeSpawner) spawn() (windowProcess, <-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, nil, f.fail
	}
	w := &fakeWindow{exited: make(chan struct{})}
	f.started = append(f.started, w)
	return w, w.exited, nil
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in 2s")
}

func TestTheFirstClickStartsOneWindowAndTheSecondAsksItToClose(t *testing.T) {
	f := &fakeSpawner{}
	changes := 0
	s := newSupervisor(f.spawn, func(string) { t.Fatal("no warning expected") })
	s.onChange = func() { changes++ }

	s.toggle()
	if len(f.started) != 1 || !s.open() {
		t.Fatalf("after one click: %d windows started, open=%v", len(f.started), s.open())
	}
	s.toggle()
	if len(f.started) != 1 {
		t.Fatalf("the second click started another window: %d", len(f.started))
	}
	if f.started[0].stops != 1 {
		t.Fatalf("the second click should ask the window to stop once, got %d", f.started[0].stops)
	}
	// Still open: the process has not gone yet, and the menu must say so.
	if !s.open() {
		t.Fatal("open() went false before the process ended")
	}
	close(f.started[0].exited)
	waitUntil(t, func() bool { return !s.open() })
	if changes < 2 {
		t.Fatalf("onChange fired %d times, want at least the start and the end", changes)
	}
}

func TestAWindowClosedFromItsOwnTitleBarIsNoticed(t *testing.T) {
	// The user closes the window with the WM; nobody clicked the tray. The
	// child exits, and the tray has to learn it without being told.
	f := &fakeSpawner{}
	s := newSupervisor(f.spawn, func(string) {})
	s.toggle()
	close(f.started[0].exited)
	waitUntil(t, func() bool { return !s.open() })
	s.toggle()
	if len(f.started) != 2 {
		t.Fatalf("after the window closed itself the next click must open a new one, started=%d", len(f.started))
	}
}

func TestAWindowThatWillNotStartLeavesTheTrayUpAndSaysWhy(t *testing.T) {
	f := &fakeSpawner{fail: errors.New("no such file")}
	var warned string
	s := newSupervisor(f.spawn, func(msg string) { warned = msg })
	s.toggle()
	if s.open() {
		t.Fatal("a failed start must not count as an open window")
	}
	if !strings.Contains(warned, "no such file") {
		t.Fatalf("the warning must carry the cause, got %q", warned)
	}
}

func TestTheMenuRowSaysWhatTheClickWillDo(t *testing.T) {
	if got := windowTitle(false); got != "Show rig" {
		t.Fatalf("closed: %q", got)
	}
	if got := windowTitle(true); got != "Close rig" {
		t.Fatalf("open: %q", got)
	}
}

func TestSelfExecutableSurvivesAnInstallUnderneathIt(t *testing.T) {
	exe, err := selfExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(exe, " (deleted)") {
		t.Fatalf("the deleted suffix must be stripped: %q", exe)
	}
	if exe == "" {
		t.Fatal("empty executable path")
	}
}
