// Package audio is everything rig says out loud: the toast sound and speech,
// through one queue, behind one mute (plan/12, S1-S5).
//
// ONE QUEUE is the design and not a convenience: a toast's chirp always ends
// before its title is read, and two voices never talk over each other,
// because one goroutine plays everything in order and waits for each sound
// to be heard before the next. The daemon holds no audio device (section
// 22); every sound goes through a player subprocess from section 12's chain.
package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ReadAloud is how much of a toast is read aloud (S3).
type ReadAloud string

// Sound is which sound a toast plays (S4).
type Sound string

const (
	ReadOff          ReadAloud = "off"
	ReadTitle        ReadAloud = "title"
	ReadTitleAndBody ReadAloud = "title-and-body"

	SoundHail  Sound = "hail"
	SoundBadge Sound = "badge"
	// SoundFile is the file named by Options.SoundFile, his own clip.
	SoundFile Sound = "file"
)

// Settings are what a person changes, and they survive a restart (his
// ruling, plan/12).
type Settings struct {
	Muted     bool      `json:"muted"`
	ReadAloud ReadAloud `json:"read_aloud"`
	Sound     Sound     `json:"sound"`
}

// Defaults are his, 2026-09-27: the hail, the title read aloud, sound on.
func Defaults() Settings {
	return Settings{ReadAloud: ReadTitle, Sound: SoundHail}
}

func (s Settings) valid(fileSet bool) error {
	switch s.ReadAloud {
	case ReadOff, ReadTitle, ReadTitleAndBody:
	default:
		return fmt.Errorf("read aloud is one of off, title, title-and-body; got %q", s.ReadAloud)
	}
	switch s.Sound {
	case SoundHail, SoundBadge:
	case SoundFile:
		if !fileSet {
			return errors.New("the toast sound is a file only when rigd was started with --toast-sound-file")
		}
	default:
		return fmt.Errorf("the toast sound is one of hail, badge, file; got %q", s.Sound)
	}
	return nil
}

// players is section 12's player chain, in order.
var players = []string{"pw-play", "paplay", "aplay", "play"}

// queueDepth bounds what may wait. Past it the oldest is dropped: twenty
// toasts at once must not become a two-minute monologue.
const queueDepth = 8

// toastChars caps a toast read aloud. A body is up to maxToastBody bytes,
// and reading all of it is a page, not an announcement. Say has no cap.
const toastChars = 400

// Options configure an Audio.
type Options struct {
	Log *slog.Logger
	// SettingsPath is where Settings are kept; empty keeps them in memory.
	SettingsPath string
	// SoundDir is where the synthesised sounds are written for the player.
	SoundDir string
	// SoundFile is his own toast sound: an absolute path, or empty.
	SoundFile string
	// Idle releases the speech engine after this long with nothing said;
	// a voice model is ~100 MB resident. Zero means a minute.
	Idle time.Duration
	// Look finds a binary; nil is exec.LookPath. Tests replace it.
	Look func(string) (string, error)
}

// Status is what Audio reports: the settings, and what it found to play with.
type Status struct {
	Settings
	FileSet bool   // a --toast-sound-file was given, so SoundFile is a choice
	Player  string // empty: nothing can sound
	Engine  string // empty: nothing can speak
	Problem string // why something is silent, in words
}

// item is one thing to play: a sound file, or a line to speak.
type item struct {
	file string
	line string
	done chan struct{} // closed when heard or dropped; nil when nobody waits
}

func (it item) release() {
	if it.done != nil {
		close(it.done)
	}
}

// Audio owns the settings, the queue and the speech engine.
type Audio struct {
	opt    Options
	player string
	sounds map[Sound]string // the file each sound plays

	mu       sync.Mutex
	settings Settings
	eng      *engine // nil until the first line, then found once
	engErr   error
	problem  string
	closed   bool

	items chan item
	cut   chan struct{} // capacity 1: stop what is playing now
	done  chan struct{}
}

// New loads the settings, writes the sounds and starts the queue. Nothing it
// finds missing is fatal: a missing player is silence, reported in Status.
func New(opt Options) (*Audio, error) {
	if opt.Log == nil {
		opt.Log = slog.New(slog.DiscardHandler)
	}
	if opt.Look == nil {
		opt.Look = exec.LookPath
	}
	if opt.Idle == 0 {
		opt.Idle = time.Minute
	}
	if opt.SoundFile != "" {
		if err := CheckSoundFile(opt.SoundFile); err != nil {
			return nil, err
		}
	}
	a := &Audio{
		opt:    opt,
		sounds: map[Sound]string{SoundFile: opt.SoundFile},
		items:  make(chan item, queueDepth),
		cut:    make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	a.settings = a.load()
	for _, p := range players {
		if path, err := opt.Look(p); err == nil {
			a.player = path
			break
		}
	}
	if a.player == "" {
		a.problem = "no audio player found (pw-play, paplay, aplay or play)"
		opt.Log.Warn("rig is silent", "reason", a.problem)
	}
	for name, content := range map[Sound][]byte{SoundHail: Hail(), SoundBadge: Badge()} {
		path, err := writeSound(opt.SoundDir, string(name)+".wav", content)
		if err != nil {
			opt.Log.Warn("a toast sound could not be written", "sound", name, "err", err)
			continue
		}
		a.sounds[name] = path
	}
	go a.run()
	return a, nil
}

// CheckSoundFile validates his own sound: an absolute path to a regular file.
func CheckSoundFile(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("toast sound file %q: want an absolute path", path)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("toast sound file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("toast sound file %q is not a regular file", path)
	}
	return nil
}

// load reads the settings, falling back to the defaults for a missing or
// unreadable file. A file naming the file sound with no file given falls back
// to the default sound rather than to silence.
func (a *Audio) load() Settings {
	s := Defaults()
	if a.opt.SettingsPath == "" {
		return s
	}
	data, err := os.ReadFile(a.opt.SettingsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			a.opt.Log.Warn("sound settings unreadable, using the defaults", "err", err)
		}
		return s
	}
	var got Settings
	if err := json.Unmarshal(data, &got); err != nil {
		a.opt.Log.Warn("sound settings unreadable, using the defaults", "err", err)
		return s
	}
	if got.Sound == SoundFile && a.opt.SoundFile == "" {
		got.Sound = s.Sound
	}
	if err := got.valid(a.opt.SoundFile != ""); err != nil {
		a.opt.Log.Warn("sound settings refused, using the defaults", "err", err)
		return s
	}
	return got
}

// save writes the settings through a rename. Called with a.mu held.
func (a *Audio) save() error {
	if a.opt.SettingsPath == "" {
		return nil
	}
	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return err
	}
	_, err = writeSound(filepath.Dir(a.opt.SettingsPath), filepath.Base(a.opt.SettingsPath), append(data, '\n'))
	return err
}

// Status answers the settings and what was found.
func (a *Audio) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := Status{Settings: a.settings, FileSet: a.opt.SoundFile != "", Player: a.player, Problem: a.problem}
	if a.eng != nil {
		st.Engine = a.eng.argv[0]
	}
	if a.engErr != nil && st.Problem == "" {
		st.Problem = a.engErr.Error()
	}
	return st
}

// Change applies a change to the settings and saves them. Muting stops what
// is playing and drops what waits.
func (a *Audio) Change(change func(*Settings)) (Status, error) {
	a.mu.Lock()
	next := a.settings
	change(&next)
	if err := next.valid(a.opt.SoundFile != ""); err != nil {
		a.mu.Unlock()
		return Status{}, err
	}
	prev := a.settings
	a.settings = next
	err := a.save()
	if err != nil {
		a.settings = prev
	}
	a.mu.Unlock()
	if err != nil {
		return Status{}, fmt.Errorf("saving the sound settings: %w", err)
	}
	if next.Muted && !prev.Muted {
		a.silence()
	}
	return a.Status(), nil
}

// Toast sounds a toast that was drawn: its sound, then as much of it read
// aloud as the settings say. A speak line, written to be heard, is read in
// place of the title or the body; read-aloud off still silences it.
func (a *Audio) Toast(title, body, speak string) {
	a.mu.Lock()
	s := a.settings
	file := a.sounds[s.Sound]
	a.mu.Unlock()
	if s.Muted {
		return
	}
	if file != "" {
		a.enqueue(item{file: file})
	}
	var text string
	switch {
	case s.ReadAloud == ReadOff:
	case strings.TrimSpace(speak) != "":
		text = speak
	case s.ReadAloud == ReadTitle:
		text = title
	case s.ReadAloud == ReadTitleAndBody:
		text = strings.TrimSpace(title)
		if !strings.HasSuffix(text, ".") && !strings.HasSuffix(text, "!") && !strings.HasSuffix(text, "?") {
			text += "." // a pause between the title and the body
		}
		text += " " + body
	}
	if line := clean(text, toastChars); line != "" {
		a.enqueue(item{line: line})
	}
}

// ErrMuted is Say while sounds are off.
var ErrMuted = errors.New("sounds are muted")

// ErrNothingToSay is Say with only blanks and control characters.
var ErrNothingToSay = errors.New("nothing to say")

// Say reads text aloud (S2), and returns a channel closed once it has been
// heard, or dropped by a mute or a full queue.
func (a *Audio) Say(text string) (<-chan struct{}, error) {
	a.mu.Lock()
	muted, player := a.settings.Muted, a.player
	a.mu.Unlock()
	if muted {
		return nil, ErrMuted
	}
	if player == "" {
		return nil, errors.New(a.problem)
	}
	if err := a.findEngine(); err != nil {
		return nil, err
	}
	line := clean(text, 0)
	if line == "" {
		return nil, ErrNothingToSay
	}
	done := make(chan struct{})
	a.enqueue(item{line: line, done: done})
	return done, nil
}

// findEngine resolves the engine once. Called outside the queue so Say can
// refuse at once when there is none.
func (a *Audio) findEngine() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.eng == nil && a.engErr == nil {
		eng, err := findEngine(a.opt.Look)
		if err != nil {
			a.engErr = err
			a.opt.Log.Warn("rig cannot speak", "reason", err)
		} else {
			a.eng = &eng
			a.opt.Log.Info("speech engine found", "engine", eng.argv[0], "rate", eng.rate)
		}
	}
	return a.engErr
}

func (a *Audio) enqueue(it item) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.player == "" {
		it.release()
		return
	}
	for {
		select {
		case a.items <- it:
			return
		default:
		}
		select {
		case dropped := <-a.items:
			dropped.release()
		default:
		}
	}
}

// silence drops everything waiting and stops what is playing.
func (a *Audio) silence() {
	for {
		select {
		case dropped := <-a.items:
			dropped.release()
		default:
			select {
			case a.cut <- struct{}{}:
			default:
			}
			return
		}
	}
}

// Close stops the queue. Safe to call twice.
func (a *Audio) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	close(a.items)
	a.mu.Unlock()
	<-a.done
}

// run plays the queue, one item at a time, each heard before the next.
func (a *Audio) run() {
	defer close(a.done)
	var (
		pipe  *pipeline
		timer *time.Timer
		idle  <-chan time.Time
	)
	release := func(kill bool) {
		if pipe == nil {
			return
		}
		if kill {
			pipe.kill()
		} else {
			pipe.close()
		}
		pipe = nil
	}
	defer release(false)
	for {
		select {
		case it, ok := <-a.items:
			if !ok {
				return
			}
			select { // a cut left from before this item is not for it
			case <-a.cut:
			default:
			}
			if it.file != "" {
				a.playFile(it.file)
			} else if it.line != "" {
				pipe = a.speak(pipe, it.line)
			}
			it.release()
			if timer == nil {
				timer = time.NewTimer(a.opt.Idle)
			} else {
				timer.Reset(a.opt.Idle)
			}
			idle = timer.C
		case <-idle:
			release(false)
			idle = nil
		}
	}
}

// playFile plays one sound file and waits for it, or for a cut.
func (a *Audio) playFile(file string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.player, fileArgs(a.player, file)...)
	if err := cmd.Start(); err != nil {
		a.opt.Log.Warn("playing a toast sound", "err", err)
		return
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			a.opt.Log.Warn("playing a toast sound", "err", err)
		}
	case <-a.cut:
		cancel()
		<-waited
	}
}

// speak says one line on the pipeline, starting it if needed, and waits until
// it has been heard. It returns the pipeline to keep, nil after a failure or
// a cut.
func (a *Audio) speak(pipe *pipeline, line string) *pipeline {
	if err := a.findEngine(); err != nil {
		return pipe
	}
	a.mu.Lock()
	eng := *a.eng
	a.mu.Unlock()
	var err error
	if pipe == nil {
		if pipe, err = startPipeline(eng, a.player); err != nil {
			a.opt.Log.Warn("starting speech", "err", err)
			return nil
		}
	}
	from := pipe.written()
	if err = pipe.say(line); err != nil {
		// The engine went away. One rebuild, then give up on this line.
		pipe.kill()
		if pipe, err = startPipeline(eng, a.player); err == nil {
			from = pipe.written()
			err = pipe.say(line)
		}
		if err != nil {
			a.opt.Log.Warn("speaking", "err", err)
			if pipe != nil {
				pipe.kill()
			}
			return nil
		}
	}
	grace, ceiling := waitBudget(line)
	if pipe.drain(from, grace, ceiling, a.cut) {
		pipe.kill()
		return nil
	}
	// A player that refused the stream leaves the engine running and the
	// line unheard, and nothing else would say so.
	if why, died := pipe.playerDied(); died {
		a.opt.Log.Warn("the player stopped, so the line was not heard",
			"player", filepath.Base(a.player), "err", why)
		pipe.kill()
		return nil
	}
	return pipe
}
