package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Section 12, "A toast plays a sound" (Boris, 2026-09-27): a drawn toast
// sounds as it appears. The sound is rig's own synthesised hail unless
// --toast-sound names a file of his, and "off" silences it. The daemon holds
// no audio device (section 22), so the renderer plays through the first of
// section 12's player chain it finds; no player is a warning, never an error,
// and the toast is drawn either way. A toast held back by Do Not Disturb never
// reaches the renderer, so it is silent without a check here.

// soundOff is the --toast-sound value that silences toasts.
const soundOff = "off"

// soundPlayers is section 12's player chain, in order.
var soundPlayers = []string{"pw-play", "paplay", "aplay", "play"}

// soundGap is the least time between two sounds, so a burst of toasts is one
// hail rather than a stutter.
const soundGap = 2 * time.Second

// soundPlayTimeout bounds one playback; a player that hangs is killed.
const soundPlayTimeout = 10 * time.Second

// checkToastSound validates the flag's value: empty for the built-in hail,
// "off", or an absolute path to a regular file.
func checkToastSound(v string) error {
	if v == "" || v == soundOff {
		return nil
	}
	if !filepath.IsAbs(v) {
		return fmt.Errorf("--toast-sound %q: want an absolute path, %q, or nothing for the built-in hail", v, soundOff)
	}
	fi, err := os.Stat(v)
	if err != nil {
		return fmt.Errorf("--toast-sound: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("--toast-sound %q is not a regular file", v)
	}
	return nil
}

// toastSound plays one sound per burst of toasts.
type toastSound struct {
	path string // the file played; empty means silent
	play func(ctx context.Context, path string) error
	warn func(string)
	now  func() time.Time

	mu   sync.Mutex
	last time.Time
}

// newToastSound resolves the flag's value to a file and a player. Anything
// that stops a sound warns once and yields a silent toastSound.
func newToastSound(v string, warn func(string)) *toastSound {
	s := &toastSound{warn: warn, now: time.Now}
	if v == soundOff {
		return s
	}
	player, err := findPlayer()
	if err != nil {
		warn(err.Error() + "; toasts are silent")
		return s
	}
	s.play = func(ctx context.Context, path string) error {
		return exec.CommandContext(ctx, player, path).Run()
	}
	if v != "" {
		s.path = v
		return s
	}
	path, err := writeHail()
	if err != nil {
		warn("the built-in hail could not be written, toasts are silent: " + err.Error())
		return s
	}
	s.path = path
	return s
}

// ring plays the sound unless one played within soundGap. It does not wait
// for the player.
func (s *toastSound) ring() {
	if s.path == "" || s.play == nil {
		return
	}
	s.mu.Lock()
	now := s.now()
	if !s.last.IsZero() && now.Sub(s.last) < soundGap {
		s.mu.Unlock()
		return
	}
	s.last = now
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), soundPlayTimeout)
		defer cancel()
		if err := s.play(ctx, s.path); err != nil {
			s.warn("playing the toast sound: " + err.Error())
		}
	}()
}

// findPlayer is the first of section 12's players on PATH.
func findPlayer() (string, error) {
	for _, p := range soundPlayers {
		if path, err := exec.LookPath(p); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no audio player found (pw-play, paplay, aplay or play)")
}

// writeHail puts the built-in hail in the user's cache directory and returns
// its path. It is rewritten on every renderer start, through a rename, so a
// changed hail in a new build replaces the old one.
func writeHail() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "rig")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "hail-*.wav")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(hailWAV())
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	path := filepath.Join(dir, "hail.wav")
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}

// hailRate is the built-in hail's sample rate, and hailSamples its length:
// 0.55 seconds, the last note's tail included.
const (
	hailRate    = 44100
	hailSamples = hailRate * 55 / 100
)

// hailNote is one rising whistle of the hail.
type hailNote struct {
	start, length float64 // seconds
	from, to      float64 // Hz, swept on a curve that rises fast then settles
}

// hailNotes is rig's own hail: two rising whistles, the second higher and
// longer, with a light warble. It is synthesised here rather than sampled, so
// nothing in it is a recording.
var hailNotes = []hailNote{
	{start: 0, length: 0.11, from: 1150, to: 1750},
	{start: 0.15, length: 0.30, from: 1500, to: 2350},
}

// hailWAV is the built-in hail as a 16-bit mono PCM WAV file.
func hailWAV() []byte {
	const n = hailSamples
	pcm := make([]float64, n)
	for _, note := range hailNotes {
		first := int(note.start * hailRate)
		count := int(note.length * hailRate)
		phase := 0.0
		for i := 0; i < count && first+i < n; i++ {
			t := float64(i) / hailRate
			x := t / note.length
			sweep := 1 - math.Exp(-5*x) // most of the rise early
			f := note.from + (note.to-note.from)*sweep/(1-math.Exp(-5))
			f *= 1 + 0.012*math.Sin(2*math.Pi*28*t) // warble
			phase += 2 * math.Pi * f / hailRate
			env := math.Min(1, t/0.006) * math.Exp(-2.2*x) // fast attack, soft decay
			if tail := note.length - t; tail < 0.02 {
				env *= tail / 0.02
			}
			pcm[first+i] += env * (math.Sin(phase) + 0.25*math.Sin(2*phase))
		}
	}
	var body bytes.Buffer
	for _, v := range pcm {
		s := math.Max(-1, math.Min(1, 0.32*v))
		_ = binary.Write(&body, binary.LittleEndian, int16(s*math.MaxInt16))
	}
	var out bytes.Buffer
	const data = uint32(hailSamples * 2)
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, 36+data)
	out.WriteString("WAVEfmt ")
	_ = binary.Write(&out, binary.LittleEndian, struct {
		size                   uint32
		format, channels       uint16
		rate, byteRate         uint32
		blockAlign, sampleBits uint16
	}{16, 1, 1, hailRate, hailRate * 2, 2, 16}) // PCM, mono
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, data)
	out.Write(body.Bytes())
	return out.Bytes()
}
