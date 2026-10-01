package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Speech, plan/12's "Speech: the same notification, said out loud", ported
// from AgentBox's internal/speech rather than written a second time. What
// came across: the engine held open between lines (a model load is ~3s and a
// line ~375ms), the counting copy that says when a line has been HEARD, and
// the player flags. What did not: the reading transport, which rig has no
// caller for.
//
// S5, best quality (Boris, 2026-09-27): Kokoro first at its native 24 kHz,
// piper only when Kokoro is missing, and pw-play's resampler pinned to 15.

// resamplerQuality is pw-play's resampler setting, pinned to its maximum. A
// voice is 22-24 kHz and this machine's sink runs at 48 kHz, so every line is
// resampled; PipeWire's default of 4 is chosen for hour-long streams, and a
// sentence deserves 15.
const resamplerQuality = 15

// engine is a resolved speech engine: argv, and the rate of the PCM it emits.
type engine struct {
	argv []string
	rate int
}

// errNoEngine is speech with nothing to synthesise with. rig without a voice
// still sounds its toasts.
var errNoEngine = errors.New("no speech engine found (kokoro-say, or piper and a voice)")

// findEngine is S5's order: Kokoro, then the best piper voice installed.
func findEngine(look func(string) (string, error)) (engine, error) {
	if bin, err := look("kokoro-say"); err == nil {
		rate := 24000
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, bin, "--rate").Output(); err == nil {
			if r, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && r > 0 {
				rate = r
			}
		}
		return engine{argv: []string{bin}, rate: rate}, nil
	}
	bin, err := look("piper")
	if err != nil {
		return engine{}, errNoEngine
	}
	model := findVoice()
	if model == "" {
		return engine{}, fmt.Errorf("%w: piper is installed but no .onnx voice was found in %s",
			errNoEngine, strings.Join(voiceDirs, ", "))
	}
	return engine{argv: []string{bin, "--model", model, "--output-raw"}, rate: voiceRate(model)}, nil
}

// --- the pipeline -----------------------------------------------------------

// pipeline is the engine and the player, joined by a counting copy: every
// byte of PCM is measured on its way to the player, which is how rig knows
// when a line has finished being heard, and so when the next sound may start.
type pipeline struct {
	synth     *exec.Cmd
	player    *exec.Cmd
	in        io.WriteCloser
	meter     *meter
	synthGone chan struct{}
	playGone  chan struct{}
	// what the player said and how it ended, readable once playGone closes
	playErr  error
	playSaid *tail
}

// tail keeps the last bytes a child wrote to stderr, for the log line that
// says why it stopped.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

const tailMax = 512

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if over := len(t.buf) - tailMax; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// playerDied reports why the player stopped, if it has.
func (p *pipeline) playerDied() (error, bool) { //nolint:revive // the error is the answer, not a failure
	select {
	case <-p.playGone:
		if p.playErr == nil {
			return fmt.Errorf("it exited: %s", p.playSaid), true
		}
		return fmt.Errorf("%w: %s", p.playErr, p.playSaid), true
	default:
		return nil, false
	}
}

// drainGrace is how long a pipeline that is let go may finish its sentence.
const drainGrace = 5 * time.Second

func startPipeline(eng engine, player string) (*pipeline, error) {
	//rig:allow nocontextfree: the engine lives across lines until idle or a cut; close and kill end it
	synth := exec.CommandContext(context.Background(), eng.argv[0], eng.argv[1:]...)
	in, err := synth.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("engine stdin: %w", err)
	}
	// os.Pipe rather than StdoutPipe: Cmd.Wait closes a pipe it handed out,
	// and the reapers below call Wait as soon as a child exits, which would
	// shut the read end while the copy was still draining it.
	synthR, synthW, err := os.Pipe()
	if err != nil {
		_ = in.Close()
		return nil, fmt.Errorf("pipe: %w", err)
	}
	playR, playW, err := os.Pipe()
	if err != nil {
		_ = in.Close()
		_ = synthR.Close()
		_ = synthW.Close()
		return nil, fmt.Errorf("pipe: %w", err)
	}
	closeAll := func() {
		_ = in.Close()
		_ = synthR.Close()
		_ = synthW.Close()
		_ = playR.Close()
		_ = playW.Close()
	}
	synth.Stdout = synthW
	synth.Stderr = nil // piper narrates progress there; nobody reads it

	//rig:allow nocontextfree: as the engine above
	play := exec.CommandContext(context.Background(), player, pcmArgs(player, eng.rate)...)
	play.Stdin = playR
	said := &tail{}
	play.Stderr = said

	if err := synth.Start(); err != nil {
		closeAll()
		return nil, fmt.Errorf("start %s: %w", filepath.Base(eng.argv[0]), err)
	}
	if err := play.Start(); err != nil {
		closeAll()
		_ = synth.Process.Kill()
		_ = synth.Wait()
		return nil, fmt.Errorf("start %s: %w", filepath.Base(player), err)
	}
	// Each child holds its own end now; the parent lets go of those two or
	// nothing ever sees EOF.
	_ = synthW.Close()
	_ = playR.Close()

	p := &pipeline{
		synth: synth, player: play, in: in,
		meter:     &meter{bps: float64(2 * eng.rate)},
		synthGone: make(chan struct{}),
		playGone:  make(chan struct{}),
		playSaid:  said,
	}
	go func() {
		_, _ = io.Copy(counted{w: playW, m: p.meter}, synthR)
		_ = playW.Close()
		_ = synthR.Close()
	}()
	go func() { _ = synth.Wait(); close(p.synthGone) }()
	go func() { p.playErr = play.Wait(); close(p.playGone) }()
	return p, nil
}

func (p *pipeline) say(text string) error {
	_, err := io.WriteString(p.in, text+"\n")
	return err
}

// close lets the pipeline finish what it is saying; only a process that
// outstays drainGrace is killed.
func (p *pipeline) close() {
	_ = p.in.Close()
	waitOrKill(p.synthGone, p.synth)
	waitOrKill(p.playGone, p.player)
}

// kill stops the sound now. The player goes too, because it holds audio the
// meter has already counted.
func (p *pipeline) kill() {
	if p.synth.Process != nil {
		_ = p.synth.Process.Kill()
	}
	if p.player.Process != nil {
		_ = p.player.Process.Kill()
	}
	_ = p.in.Close()
	<-p.synthGone
	<-p.playGone
}

func waitOrKill(gone <-chan struct{}, cmd *exec.Cmd) {
	select {
	case <-gone:
	case <-time.After(drainGrace):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-gone
	}
}

// --- knowing when the sound stopped -------------------------------------------

// meter turns a byte count into the wall-clock instant when everything handed
// to the player so far will have been heard: raw s16 PCM has no container, so
// bytes / (2 x rate) is exact.
type meter struct {
	mu    sync.Mutex
	bps   float64
	bytes int64
	last  time.Time
	until time.Time
}

func (m *meter) wrote(n int) {
	now := time.Now()
	d := time.Duration(float64(n) / m.bps * float64(time.Second))
	m.mu.Lock()
	if now.After(m.until) {
		m.until = now.Add(d)
	} else {
		m.until = m.until.Add(d)
	}
	m.bytes += int64(n)
	m.last = now
	m.mu.Unlock()
}

func (m *meter) read() (bytes int64, last, until time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bytes, m.last, m.until
}

type counted struct {
	w io.Writer
	m *meter
}

func (c counted) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.m.wrote(n)
	}
	return n, err
}

// The drain constants, measured in AgentBox on this machine (kokoro-onnx
// 0.5.0, am_michael, one core: ~0.026 s of synthesis and ~0.067 s of speech
// per character). A neural engine emits nothing until the whole line is
// synthesised, so the bounds scale with the text; both have better than 2x
// headroom, because they are backstops against a dead engine.
const (
	drainQuiet    = 120 * time.Millisecond
	drainPoll     = 20 * time.Millisecond
	drainStart    = 5 * time.Second
	drainCeiling  = 30 * time.Second
	drainSettle   = 400 * time.Millisecond
	synthPerChar  = 60 * time.Millisecond
	speechPerChar = 140 * time.Millisecond
)

func waitBudget(line string) (start, ceiling time.Duration) {
	n := utf8.RuneCountInString(line)
	start = drainStart + time.Duration(n)*synthPerChar
	return start, start + drainCeiling + time.Duration(n)*speechPerChar
}

// drain blocks until the line written after byte count from has been heard:
// the stream quiet for drainQuiet AND the meter's until passed. It reports
// whether cut ended the wait first; the caller then kills the pipeline.
func (p *pipeline) drain(from int64, grace, ceiling time.Duration, cut <-chan struct{}) bool {
	began := time.Now()
	tick := time.NewTicker(drainPoll)
	defer tick.Stop()
	for {
		select {
		case <-cut:
			return true
		case <-tick.C:
			bytes, last, until := p.meter.read()
			now := time.Now()
			if bytes == from {
				if now.Sub(began) >= grace {
					return false // this line synthesised to nothing
				}
				continue
			}
			if now.Sub(last) >= drainQuiet && now.After(until.Add(drainSettle)) {
				return false
			}
			if now.Sub(began) >= ceiling {
				return false
			}
		}
	}
}

func (p *pipeline) written() int64 {
	n, _, _ := p.meter.read()
	return n
}

// pcmArgs are the player's flags for raw mono s16 PCM at rate. The players
// differ in every detail, including the spelling of the sample format.
func pcmArgs(player string, rate int) []string {
	r := strconv.Itoa(rate)
	switch filepath.Base(player) {
	case "pw-play":
		return []string{
			// --raw: pw-play 1.6 reads "-" as a sound file without it, and
			// refuses PCM with "Format not recognised"
			"--raw", "--rate=" + r, "--channels=1", "--format=s16",
			"--quality=" + strconv.Itoa(resamplerQuality), "-",
		}
	case "paplay":
		return []string{"--raw", "--rate=" + r, "--channels=1", "--format=s16le"}
	case "play":
		return []string{"-q", "-t", "raw", "-r", r, "-e", "signed", "-b", "16", "-c", "1", "-"}
	default: // aplay
		return []string{"-q", "-t", "raw", "-f", "S16_LE", "-r", r, "-c", "1"}
	}
}

// fileArgs are the player's flags for a sound file.
func fileArgs(player, path string) []string {
	switch filepath.Base(player) {
	case "pw-play":
		return []string{"--quality=" + strconv.Itoa(resamplerQuality), path}
	case "play":
		return []string{"-q", path}
	case "aplay":
		return []string{"-q", path}
	default:
		return []string{path}
	}
}

// --- the spoken line ----------------------------------------------------------

// clean makes one line an engine can read: the protocol is a line per
// utterance, so a newline would split a sentence and a control character
// confuses it. maxChars zero means no cap.
func clean(text string, maxChars int) string {
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r == utf8.RuneError, unicode.IsControl(r):
			return -1
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if maxChars <= 0 || utf8.RuneCountInString(text) <= maxChars {
		return text
	}
	cut := string([]rune(text)[:maxChars])
	if space := strings.LastIndexByte(cut, ' '); space > len(cut)/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,;:-") + "..."
}

// --- piper, the fallback --------------------------------------------------------

var voiceDirs = []string{
	"~/.local/share/piper-voices",
	"~/piper-voices",
	"/usr/share/piper-voices",
	"/usr/local/share/piper-voices",
}

// findVoice prefers an English voice, then the highest quality tier.
func findVoice() string {
	var found []string
	for _, dir := range voiceDirs {
		if rest, ok := strings.CutPrefix(dir, "~/"); ok {
			home, err := os.UserHomeDir()
			if err != nil {
				continue
			}
			dir = filepath.Join(home, rest)
		}
		matches, err := filepath.Glob(filepath.Join(dir, "*.onnx"))
		if err != nil {
			continue
		}
		found = append(found, matches...)
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool {
		if a, b := voiceScore(found[i]), voiceScore(found[j]); a != b {
			return a > b
		}
		return found[i] < found[j]
	})
	return found[0]
}

var tiers = []string{"x_low", "low", "medium", "high"}

func voiceScore(path string) int {
	name := strings.ToLower(filepath.Base(path))
	score := 0
	if strings.HasPrefix(name, "en") {
		score += 10
	}
	for i := len(tiers) - 1; i >= 0; i-- {
		if strings.Contains(name, "-"+tiers[i]) {
			return score + i + 1
		}
	}
	return score
}

// voiceRate reads the sample rate from the voice's companion JSON; a voice
// played at the wrong rate is a chipmunk, so it is never guessed.
func voiceRate(model string) int {
	const fallback = 22050
	data, err := os.ReadFile(model + ".json")
	if err != nil {
		return fallback
	}
	var cfg struct {
		Audio struct {
			SampleRate int `json:"sample_rate"`
		} `json:"audio"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Audio.SampleRate <= 0 {
		return fallback
	}
	return cfg.Audio.SampleRate
}
