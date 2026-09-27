package daemon

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/audio"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 12's sounds and speech (plan/12, S1-S5). The daemon holds the one
// audio queue, so a toast's sound always ends before its title is read and
// rig.say never talks over a toast. internal/audio has the queue; this file
// is the wire.

// maxSayText bounds what one rig.say may ask the engine to read.
const maxSayText = 10000

// maxSayWait bounds how long rig.say with wait holds its caller.
const maxSayWait = 2 * time.Minute

func (d *Daemon) serveSound(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	if d.audio == nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig." + command + ": this daemon has no sound",
			Precondition: "rigd was started with its audio queue",
			Actual:       "it was not; a daemon built for a test is silent",
			Fix:          "ask the production daemon",
		})
		return
	}
	switch command {
	case "sound":
		d.serveSoundSettings(c, f)
	case "say":
		d.serveSay(ctx, c, f)
	}
}

var (
	readAloudWire = map[registryv1.ReadAloud]audio.ReadAloud{
		registryv1.ReadAloud_READ_ALOUD_OFF:            audio.ReadOff,
		registryv1.ReadAloud_READ_ALOUD_TITLE:          audio.ReadTitle,
		registryv1.ReadAloud_READ_ALOUD_TITLE_AND_BODY: audio.ReadTitleAndBody,
	}
	toastSoundWire = map[registryv1.ToastSound]audio.Sound{
		registryv1.ToastSound_TOAST_SOUND_HAIL:  audio.SoundHail,
		registryv1.ToastSound_TOAST_SOUND_BADGE: audio.SoundBadge,
		registryv1.ToastSound_TOAST_SOUND_FILE:  audio.SoundFile,
	}
)

func (d *Daemon) serveSoundSettings(c *conn, f *rigv1.Frame) {
	var req registryv1.SoundRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.sound: "+err.Error())
		return
	}
	read, readOK := readAloudWire[req.GetReadAloud()]
	sound, soundOK := toastSoundWire[req.GetToastSound()]
	switch {
	case req.GetMute() != registryv1.SoundMute_SOUND_MUTE_UNSPECIFIED &&
		req.GetMute() != registryv1.SoundMute_SOUND_MUTE_ON && req.GetMute() != registryv1.SoundMute_SOUND_MUTE_OFF:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.sound: mute is on or off; got "+req.GetMute().String())
		return
	case req.GetReadAloud() != registryv1.ReadAloud_READ_ALOUD_UNSPECIFIED && !readOK:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.sound: read aloud is off, title or title and body; got "+req.GetReadAloud().String())
		return
	case req.GetToastSound() != registryv1.ToastSound_TOAST_SOUND_UNSPECIFIED && !soundOK:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.sound: the toast sound is hail, badge or file; got "+req.GetToastSound().String())
		return
	}
	st, err := d.audio.Change(func(s *audio.Settings) {
		switch req.GetMute() {
		case registryv1.SoundMute_SOUND_MUTE_ON:
			s.Muted = true
		case registryv1.SoundMute_SOUND_MUTE_OFF:
			s.Muted = false
		}
		if readOK {
			s.ReadAloud = read
		}
		if soundOK {
			s.Sound = sound
		}
	})
	if err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.sound: "+err.Error())
		return
	}
	c.reply(f.GetStreamId(), soundResponse(st))
}

func soundResponse(st audio.Status) *registryv1.SoundResponse {
	r := &registryv1.SoundResponse{
		Muted: st.Muted, FileSet: st.FileSet,
		Player: st.Player, Engine: st.Engine, Problem: st.Problem,
	}
	for w, a := range readAloudWire {
		if a == st.ReadAloud {
			r.ReadAloud = w
		}
	}
	for w, s := range toastSoundWire {
		if s == st.Sound {
			r.ToastSound = w
		}
	}
	return r
}

func (d *Daemon) serveSay(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.SayRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.say: "+err.Error())
		return
	}
	if len(req.GetText()) > maxSayText || !utf8.ValidString(req.GetText()) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.say: text is up to 10000 bytes of UTF-8")
		return
	}
	done, err := d.audio.Say(req.GetText())
	if err != nil {
		code := rigv1.Code_CODE_UNAVAILABLE
		if errors.Is(err, audio.ErrNothingToSay) {
			code = rigv1.Code_CODE_INVALID
		}
		c.fail(f.GetStreamId(), code, "rig.say: "+err.Error())
		return
	}
	if !req.GetWait() {
		c.reply(f.GetStreamId(), &registryv1.SayResponse{})
		return
	}
	timer := time.NewTimer(maxSayWait)
	defer timer.Stop()
	select {
	case <-done:
		c.reply(f.GetStreamId(), &registryv1.SayResponse{Heard: true})
	case <-timer.C:
		c.reply(f.GetStreamId(), &registryv1.SayResponse{})
	case <-ctx.Done():
	}
}
