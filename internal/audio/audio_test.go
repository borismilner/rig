package audio

import (
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Both sounds are playable WAVs: a well-formed header, 16-bit mono, a
// sensible length, and neither silent nor clipped.
func TestTheSoundsAreWellFormedWAVs(t *testing.T) {
	for name, w := range map[string][]byte{"hail": Hail(), "badge": Badge()} {
		le := binary.LittleEndian
		if string(w[0:4]) != "RIFF" || string(w[8:16]) != "WAVEfmt " || string(w[36:40]) != "data" {
			t.Fatalf("%s: header is %q", name, w[:44])
		}
		if got := le.Uint32(w[4:8]); int(got) != len(w)-8 {
			t.Errorf("%s: RIFF size %d, file %d", name, got, len(w))
		}
		if f, ch, rate, bits := le.Uint16(w[20:22]), le.Uint16(w[22:24]), le.Uint32(w[24:28]), le.Uint16(w[34:36]); f != 1 || ch != 1 || rate != sampleRate || bits != 16 {
			t.Errorf("%s: format %d, channels %d, rate %d, bits %d", name, f, ch, rate, bits)
		}
		data := le.Uint32(w[40:44])
		if int(data) != len(w)-44 {
			t.Errorf("%s: data size %d, body %d", name, data, len(w)-44)
		}
		if secs := float64(data) / 2 / sampleRate; secs < 0.2 || secs > 1 {
			t.Errorf("%s lasts %.2fs", name, secs)
		}
		var peak int16
		for i := 44; i+1 < len(w); i += 2 {
			peak = max(peak, int16(le.Uint16(w[i:i+2])))
		}
		if peak < 3000 || peak == 32767 {
			t.Errorf("%s: peak sample %d, silent or clipped", name, peak)
		}
	}
	if string(Hail()) == string(Badge()) {
		t.Error("the hail and the badge are the same sound")
	}
}

func TestASoundFileIsAnAbsoluteRegularFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mine.wav")
	if err := os.WriteFile(file, Hail(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckSoundFile(file); err != nil {
		t.Errorf("%q refused: %v", file, err)
	}
	for _, bad := range []string{"", "mine.wav", "../mine.wav", filepath.Dir(file), file + ".missing"} {
		if err := CheckSoundFile(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// fakes puts a stand-in player and engine in a directory and returns a Look
// that finds only them, and the log both append to.
func fakes(t *testing.T, withEngine bool) (look func(string) (string, error), log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	write := func(name, script string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// The player: a file argument is a sound; "-" means PCM on stdin.
	write("pw-play", `last=""; for a in "$@"; do last="$a"; done
if [ "$last" = "-" ]; then while :; do n=$(head -c 4800 | wc -c); [ "$n" -eq 0 ] && break; echo "pcm $n" >> `+log+`; done
else echo "file $(basename "$last")" >> `+log+`; fi
`)
	if withEngine {
		// 4800 bytes of s16 at 24 kHz is 0.1s of audio per line.
		write("kokoro-say", `[ "$1" = "--rate" ] && { echo 24000; exit 0; }
while IFS= read -r line; do echo "say $line" >> `+log+`; head -c 4800 /dev/zero; done
`)
	}
	return func(name string) (string, error) {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			return "", exec.ErrNotFound
		}
		return p, nil
	}, log
}

func newAudio(t *testing.T, look func(string) (string, error), settings string) *Audio {
	t.Helper()
	a, err := New(Options{Look: look, SettingsPath: settings, SoundDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

// waitLog waits for the log to hold want lines, or fails.
func waitLog(t *testing.T, log string, want int) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, _ := os.ReadFile(log)
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(data) > 0 && len(lines) >= want {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("log after 10s: %q, want %d lines", data, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A toast plays its sound, THEN its title is read: one queue, in order.
func TestAToastSoundsThenReadsItsTitle(t *testing.T) {
	look, log := fakes(t, true)
	a := newAudio(t, look, "")
	a.Toast("Deploy finished", "all green")
	lines := waitLog(t, log, 3)
	if lines[0] != "file hail.wav" || lines[1] != "say Deploy finished" || !strings.HasPrefix(lines[2], "pcm ") {
		t.Errorf("played %q", lines)
	}
}

func TestTitleAndBodyReadsBoth(t *testing.T) {
	look, log := fakes(t, true)
	a := newAudio(t, look, "")
	if _, err := a.Change(func(s *Settings) { s.ReadAloud = ReadTitleAndBody; s.Sound = SoundBadge }); err != nil {
		t.Fatal(err)
	}
	a.Toast("Deploy finished!", "all\ngreen")
	lines := waitLog(t, log, 2)
	if lines[0] != "file badge.wav" || lines[1] != "say Deploy finished! all green" {
		t.Errorf("played %q", lines)
	}
	a.Toast("Ready", "go")
	if lines = waitLog(t, log, 5); lines[4] != "say Ready. go" {
		t.Errorf("played %q", lines)
	}
}

func TestReadAloudOffOnlySounds(t *testing.T) {
	look, log := fakes(t, true)
	a := newAudio(t, look, "")
	if _, err := a.Change(func(s *Settings) { s.ReadAloud = ReadOff }); err != nil {
		t.Fatal(err)
	}
	a.Toast("quiet", "")
	waitLog(t, log, 1)
	time.Sleep(200 * time.Millisecond)
	if lines := waitLog(t, log, 1); len(lines) != 1 {
		t.Errorf("played %q", lines)
	}
}

// Muted, nothing sounds and Say refuses; unmuted, Say speaks.
func TestMuteSilencesToastsAndSay(t *testing.T) {
	look, log := fakes(t, true)
	a := newAudio(t, look, "")
	if _, err := a.Change(func(s *Settings) { s.Muted = true }); err != nil {
		t.Fatal(err)
	}
	a.Toast("nobody hears this", "")
	if _, err := a.Say("nor this"); !errors.Is(err, ErrMuted) {
		t.Errorf("Say muted: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if data, _ := os.ReadFile(log); len(data) != 0 {
		t.Errorf("muted, but played %q", data)
	}
	if _, err := a.Change(func(s *Settings) { s.Muted = false }); err != nil {
		t.Fatal(err)
	}
	done, err := a.Say("hello there")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Say never finished")
	}
	if lines := waitLog(t, log, 1); lines[0] != "say hello there" {
		t.Errorf("played %q", lines)
	}
}

// The settings survive a new Audio on the same file, which is a restart.
func TestSettingsSurviveARestart(t *testing.T) {
	look, _ := fakes(t, false)
	path := filepath.Join(t.TempDir(), "sound.json")
	a := newAudio(t, look, path)
	if got := a.Status().Settings; got != Defaults() {
		t.Errorf("fresh settings %+v, want the defaults", got)
	}
	if _, err := a.Change(func(s *Settings) { *s = Settings{Muted: true, ReadAloud: ReadOff, Sound: SoundBadge} }); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b := newAudio(t, look, path)
	if got := b.Status().Settings; got != (Settings{Muted: true, ReadAloud: ReadOff, Sound: SoundBadge}) {
		t.Errorf("after a restart %+v", got)
	}
}

func TestBadSettingsAreRefused(t *testing.T) {
	look, _ := fakes(t, false)
	a := newAudio(t, look, "")
	for name, change := range map[string]func(*Settings){
		"read aloud": func(s *Settings) { s.ReadAloud = "shout" },
		"sound":      func(s *Settings) { s.Sound = "klaxon" },
		"file unset": func(s *Settings) { s.Sound = SoundFile },
	} {
		if _, err := a.Change(change); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if got := a.Status().Settings; got != Defaults() {
		t.Errorf("a refused change stuck: %+v", got)
	}
}

// With no engine, a toast still sounds, and Say says why it cannot.
func TestNoEngineStillSounds(t *testing.T) {
	look, log := fakes(t, false)
	a := newAudio(t, look, "")
	a.Toast("still chirps", "")
	if lines := waitLog(t, log, 1); lines[0] != "file hail.wav" {
		t.Errorf("played %q", lines)
	}
	if _, err := a.Say("x"); !errors.Is(err, errNoEngine) {
		t.Errorf("Say with no engine: %v", err)
	}
}

func TestCleanMakesOneLine(t *testing.T) {
	if got := clean("a\nb\t c\x07", 0); got != "a b c" {
		t.Errorf("clean %q", got)
	}
	if got := clean(strings.Repeat("word ", 200), 50); len([]rune(got)) > 53 || !strings.HasSuffix(got, "...") {
		t.Errorf("capped %q", got)
	}
}
