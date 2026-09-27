package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
)

// The two toast sounds (plan/12, S4). Both are synthesised here rather than
// sampled, so nothing in them is a recording: the originals they are shaped
// after are studio recordings under copyright.

// sampleRate is the rate both sounds are synthesised at.
const sampleRate = 44100

// voice is one tone of a sound: it starts at a time, glides from one pitch
// towards another, and decays.
type voice struct {
	start, length float64 // seconds
	from, to      float64 // Hz
	glide         float64 // seconds for most of the glide; the pitch settles after
	attack        float64 // seconds to full level
	decay         float64 // exponential decay rate, per second
	partials      []partial
}

// partial is an overtone, as a multiple of the tone's pitch and a level.
// Inharmonic multiples are what make a tone sound metallic.
type partial struct{ ratio, level float64 }

// hail is the communicator hail: two rising whistles, the second higher and
// longer, with a light warble.
var hail = []voice{
	{
		start: 0, length: 0.11, from: 1150, to: 1750, glide: 0.022, attack: 0.006, decay: 20,
		partials: []partial{{1, 1}, {2, 0.25}},
	},
	{
		start: 0.15, length: 0.30, from: 1500, to: 2350, glide: 0.06, attack: 0.006, decay: 7.3,
		partials: []partial{{1, 1}, {2, 0.25}},
	},
}

// badge is the chirp of the communicator worn on the body: one tone gliding
// up and ringing. Variant A, his pick by ear of three (plan/12).
var badge = []voice{
	{
		start: 0, length: 0.20, from: 1600, to: 2700, glide: 0.02, attack: 0.002, decay: 12,
		partials: []partial{{1, 1}, {2.76, 0.35}, {5.4, 0.15}},
	},
}

// hailWarble is the hail's vibrato: its depth as a fraction of the pitch, and
// its rate in Hz. The badge has none.
const (
	hailWarbleDepth = 0.012
	hailWarbleRate  = 28
)

// Hail is the hail as a 16-bit mono PCM WAV file.
func Hail() []byte { return wav(render(hail, 0.55, hailWarbleDepth), 0.32) }

// Badge is the badge chirp as a 16-bit mono PCM WAV file.
func Badge() []byte { return wav(render(badge, 0.30, 0), 0.30) }

// render mixes the voices into total seconds of samples.
func render(voices []voice, total, warble float64) []float64 {
	n := int(total * sampleRate)
	out := make([]float64, n)
	for _, v := range voices {
		first := int(v.start * sampleRate)
		phase := 0.0
		for i := 0; i < int(v.length*sampleRate) && first+i < n; i++ {
			t := float64(i) / sampleRate
			f := v.from + (v.to-v.from)*(1-math.Exp(-t/v.glide))
			f *= 1 + warble*math.Sin(2*math.Pi*hailWarbleRate*t)
			phase += 2 * math.Pi * f / sampleRate
			env := math.Min(1, t/v.attack) * math.Exp(-v.decay*t)
			if tail := v.length - t; tail < 0.01 {
				env *= tail / 0.01 // no click at the end
			}
			var s float64
			for _, p := range v.partials {
				s += p.level * math.Sin(p.ratio*phase)
			}
			out[first+i] += env * s
		}
	}
	return out
}

// wav encodes samples, scaled by gain and clipped, as a WAV file.
func wav(samples []float64, gain float64) []byte {
	var body bytes.Buffer
	for _, v := range samples {
		s := math.Max(-1, math.Min(1, gain*v))
		_ = binary.Write(&body, binary.LittleEndian, int16(s*math.MaxInt16))
	}
	data := body.Bytes()
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+len(data))) //nolint:gosec // a few hundred KB at most
	out.WriteString("WAVEfmt ")
	_ = binary.Write(&out, binary.LittleEndian, struct {
		size                   uint32
		format, channels       uint16
		rate, byteRate         uint32
		blockAlign, sampleBits uint16
	}{16, 1, 1, sampleRate, sampleRate * 2, 2, 16}) // PCM, mono
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(data))) //nolint:gosec // as above
	out.Write(data)
	return out.Bytes()
}

// writeSound puts one sound in dir under name, through a rename, so a player
// never reads a half-written file and a new build's sound replaces the old.
func writeSound(dir, name string, content []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(content)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return path, nil
}
