package audio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitSpeech(t *testing.T, a *Audio, owner, id, want string) SpeechStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, err := a.SpeechState(owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if st.State == want {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %s, want %s", st.State, want)
		}
		time.Sleep(time.Millisecond)
	}
}
func startReading(t *testing.T, a *Audio, owner, text string) SpeechStatus {
	t.Helper()
	st, err := a.StartSpeech(owner, text)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
func stopReading(t *testing.T, a *Audio, owner, id string) SpeechStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st, err := a.StopSpeech(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSpeechLifecycleStopsOnlyItsItem(t *testing.T) {
	look, log := fakes(t, true)
	a := newAudio(t, look, "")
	first := startReading(t, a, "reader", "first")
	waitLog(t, log, 2) // The actual engine and player have started the first line.
	second := startReading(t, a, "other", "second")
	third := startReading(t, a, "reader", "third")
	if got := stopReading(t, a, "reader", third.ID); got.State != SpeechCancelled {
		t.Fatal(got)
	}
	if _, err := a.StopSpeech(context.Background(), "other", first.ID); !errors.Is(err, ErrSpeechNotFound) {
		t.Fatalf("foreign stop: %v", err)
	}
	if got := stopReading(t, a, "reader", first.ID); got.State != SpeechCancelled {
		t.Fatal(got)
	}
	waitSpeech(t, a, "other", second.ID, SpeechCompleted)
	if got := stopReading(t, a, "other", second.ID); got.State != SpeechCompleted {
		t.Fatal(got)
	}
	if a.Status().Muted {
		t.Fatal("stop changed mute")
	}
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "say third") || !strings.Contains(string(data), "say second") {
		t.Fatalf("played %q", data)
	}
	// Legacy speech and a toast still share the surviving queue.
	done, err := a.Say("legacy")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("legacy Say stuck")
	}
	a.Toast("toast after stop", "", "")
	waitLog(t, log, 9)
}

func TestSpeechLifecycleStopsBlockedEngineWrite(t *testing.T) {
	look, _ := fakes(t, true)
	engine, err := look("kokoro-say")
	if err != nil {
		t.Fatal(err)
	}
	// Once queried for its rate, this engine never reads stdin. A large line
	// fills the OS pipe; Stop must kill the pipeline and join that writer.
	err = os.WriteFile(engine, []byte("#!/bin/sh\n[ \"$1\" = \"--rate\" ] && { echo 24000; exit 0; }\nexec sleep 30\n"), 0700)
	if err != nil {
		t.Fatal(err)
	}
	a := newAudio(t, look, "")
	st := startReading(t, a, "reader", strings.Repeat("word ", 200000))
	waitSpeech(t, a, "reader", st.ID, SpeechPlaying)
	time.Sleep(20 * time.Millisecond)
	if got := stopReading(t, a, "reader", st.ID); got.State != SpeechCancelled {
		t.Fatal(got)
	}
}

func TestSpeechLifecycleDropMuteRetentionAndClose(t *testing.T) {
	look, log := fakes(t, true)
	a := newAudio(t, look, "")
	active := startReading(t, a, "reader", "active")
	waitLog(t, log, 2)
	oldest := startReading(t, a, "reader", "oldest")
	for i := 0; i < queueDepth; i++ {
		startReading(t, a, "reader", "queued")
	}
	waitSpeech(t, a, "reader", oldest.ID, SpeechDropped)
	if _, err := a.Change(func(s *Settings) { s.Muted = true }); err != nil {
		t.Fatal(err)
	}
	waitSpeech(t, a, "reader", active.ID, SpeechDropped)
	if _, err := a.StartSpeech("reader", "muted"); !errors.Is(err, ErrMuted) {
		t.Fatalf("muted start: %v", err)
	}
	if _, err := a.Change(func(s *Settings) { s.Muted = false }); err != nil {
		t.Fatal(err)
	}
	// Produce terminal handles without slowing the test by playing each one.
	first := startReading(t, a, "reader", "retention")
	stopReading(t, a, "reader", first.ID)
	for i := 0; i < speechHistory; i++ {
		s := startReading(t, a, "reader", "retention")
		stopReading(t, a, "reader", s.ID)
	}
	if _, err := a.SpeechState("reader", first.ID); !errors.Is(err, ErrSpeechNotFound) {
		t.Fatalf("old handle retained: %v", err)
	}
	a.mu.Lock()
	n := len(a.speech)
	a.mu.Unlock()
	if n > speechHistory {
		t.Fatalf("unbounded history: %d", n)
	}
	last := startReading(t, a, "reader", "close")
	a.Close()
	waitSpeech(t, a, "reader", last.ID, SpeechCancelled)
	if _, err := a.StartSpeech("reader", "closed"); err == nil {
		t.Fatal("start after Close accepted")
	}
	// A settings call racing shutdown must not spin on the closed queue.
	changed := make(chan error, 1)
	go func() { _, err := a.Change(func(s *Settings) { s.Muted = true }); changed <- err }()
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("mute stuck on closed queue")
	}

}

func TestSpeechLifecycleFailureAndConcurrentStop(t *testing.T) {
	look, _ := fakes(t, true)
	player, _ := look("pw-play")
	if err := os.WriteFile(player, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a := newAudio(t, look, "")
	failed := startReading(t, a, "reader", "failure")
	waitSpeech(t, a, "reader", failed.ID, SpeechFailed)
	look2, log := fakes(t, true)
	b := newAudio(t, look2, "")
	active := startReading(t, b, "reader", "concurrent")
	waitLog(t, log, 2)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			st, err := b.StopSpeech(ctx, "reader", active.ID)
			if err != nil || st.State != SpeechCancelled {
				t.Errorf("stop: %+v %v", st, err)
			}
		}()
	}
	wg.Wait()
	// A replacement Audio cannot report a prior process's utterance completed.
	fresh := newAudio(t, look2, filepath.Join(t.TempDir(), "sound.json"))
	if _, err := fresh.SpeechState("reader", active.ID); !errors.Is(err, ErrSpeechNotFound) {
		t.Fatalf("old process handle: %v", err)
	}
}

// The worker has already observed complete playback, but has not published
// that result when Stop arrives. Completion remains the observed fact.
func TestSpeechLifecycleCompletionWinsLateStop(t *testing.T) {
	j := &speechJob{id: "finished", owner: "reader", state: SpeechPlaying,
		cancel: make(chan struct{}), done: make(chan struct{})}
	a := &Audio{speech: map[string]*speechJob{j.id: j}}
	result := make(chan SpeechStatus, 1)
	stopped := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { st, err := a.StopSpeech(ctx, "reader", j.id); result <- st; stopped <- err }()
	select {
	case <-j.cancel:
	case <-ctx.Done():
		t.Fatal("stop was not requested")
	}
	j.finish(SpeechCompleted)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if st := <-result; st.State != SpeechCompleted {
		t.Fatalf("completed playback relabelled: %+v", st)
	}
}

func TestSpeechLifecycleWaitDeadlineDoesNotUndoStop(t *testing.T) {
	j := &speechJob{id: "stopping", owner: "reader", state: SpeechPlaying,
		cancel: make(chan struct{}), done: make(chan struct{})}
	a := &Audio{speech: map[string]*speechJob{j.id: j}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := a.StopSpeech(ctx, "reader", j.id); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop wait: %v", err)
	}
	select {
	case <-j.cancel:
	default:
		t.Fatal("deadline undid cancellation")
	}
	// The worker subsequently kills the pipeline and publishes its result.
	j.finish(SpeechCancelled)
	st, err := a.SpeechState("reader", j.id)
	if err != nil || st.State != SpeechCancelled {
		t.Fatalf("later state: %+v %v", st, err)
	}
}
