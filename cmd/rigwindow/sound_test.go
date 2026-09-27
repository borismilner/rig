package main

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The built-in hail is a playable WAV: a well-formed header, 16-bit mono at
// hailRate, a little over half a second long, and never clipped silent.
func TestTheHailIsAWellFormedWAV(t *testing.T) {
	w := hailWAV()
	if string(w[0:4]) != "RIFF" || string(w[8:16]) != "WAVEfmt " || string(w[36:40]) != "data" {
		t.Fatalf("header is %q", w[:44])
	}
	le := binary.LittleEndian
	if got := le.Uint32(w[4:8]); int(got) != len(w)-8 {
		t.Errorf("RIFF size %d, file %d", got, len(w))
	}
	if fmt, ch, rate, bits := le.Uint16(w[20:22]), le.Uint16(w[22:24]), le.Uint32(w[24:28]), le.Uint16(w[34:36]); fmt != 1 || ch != 1 || rate != hailRate || bits != 16 {
		t.Errorf("format %d, channels %d, rate %d, bits %d", fmt, ch, rate, bits)
	}
	data := le.Uint32(w[40:44])
	if int(data) != len(w)-44 {
		t.Errorf("data size %d, body %d", data, len(w)-44)
	}
	if secs := float64(data) / 2 / hailRate; secs < 0.4 || secs > 1 {
		t.Errorf("hail lasts %.2fs", secs)
	}
	var peak int16
	for i := 44; i+1 < len(w); i += 2 {
		peak = max(peak, int16(le.Uint16(w[i:i+2])))
	}
	if peak < 3000 || peak == 32767 {
		t.Errorf("peak sample %d: silent or clipped", peak)
	}
}

func TestTheSoundFlagTakesNothingOffOrAnAbsoluteFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "hail.wav")
	if err := os.WriteFile(file, hailWAV(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, ok := range []string{"", soundOff, file} {
		if err := checkToastSound(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"hail.wav", "../hail.wav", filepath.Dir(file), file + ".missing"} {
		if err := checkToastSound(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// A burst of toasts is one sound; a toast after the gap sounds again.
func TestABurstOfToastsRingsOnce(t *testing.T) {
	var mu sync.Mutex
	var played []string
	done := make(chan struct{}, 8)
	clock := time.Unix(1000, 0)
	s := &toastSound{
		path: "/x/hail.wav",
		play: func(_ context.Context, p string) error {
			mu.Lock()
			played = append(played, p)
			mu.Unlock()
			done <- struct{}{}
			return nil
		},
		warn: func(m string) { t.Error(m) },
		now:  func() time.Time { return clock },
	}
	s.ring()
	s.ring()
	clock = clock.Add(soundGap / 2)
	s.ring()
	clock = clock.Add(soundGap)
	s.ring()
	for range 2 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a sound never played")
		}
	}
	select {
	case <-done:
		t.Fatal("a burst rang more than once")
	case <-time.After(50 * time.Millisecond):
	}
	mu.Lock()
	defer mu.Unlock()
	if len(played) != 2 || played[0] != "/x/hail.wav" {
		t.Errorf("played %v", played)
	}
}

func TestOffIsSilent(t *testing.T) {
	s := newToastSound(soundOff, func(m string) { t.Error(m) })
	if s.path != "" || s.play != nil {
		t.Errorf("off still has a sound: %q", s.path)
	}
	s.ring() // must not panic or play
}

// The built-in hail lands in the cache directory, whole.
func TestTheHailIsWrittenToTheCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := writeHail()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(hailWAV()) {
		t.Error("the cached hail differs from the built-in one")
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "hail-*.wav")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}
