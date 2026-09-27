package main

import (
	"context"

	"fyne.io/systray"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// The tray's sound rows (plan/12, S1-S4): a Sounds switch that mutes the
// toast sound and all speech, which sound a toast plays, and how much of it
// is read aloud. Like Do Not Disturb, every row shows the DAEMON's state,
// which holds the settings and keeps them across a restart; the tray only
// asks and shows.

// soundMenu is the tray's sound rows.
type soundMenu struct {
	on    *systray.MenuItem
	sound map[registryv1.ToastSound]*systray.MenuItem
	read  map[registryv1.ReadAloud]*systray.MenuItem
}

var menuSound *soundMenu

// addSoundMenu adds the rows under the Do Not Disturb row and starts their
// click handlers.
func addSoundMenu() {
	m := &soundMenu{
		on:    systray.AddMenuItemCheckbox("Sounds", "toast sounds and reading aloud; off silences both", true),
		sound: map[registryv1.ToastSound]*systray.MenuItem{},
		read:  map[registryv1.ReadAloud]*systray.MenuItem{},
	}
	parent := systray.AddMenuItem("Toast sound", "the sound a toast plays as it appears")
	for _, row := range []struct {
		v     registryv1.ToastSound
		title string
	}{
		{registryv1.ToastSound_TOAST_SOUND_HAIL, "Hail"},
		{registryv1.ToastSound_TOAST_SOUND_BADGE, "Badge chirp"},
		{registryv1.ToastSound_TOAST_SOUND_FILE, "My sound file"},
	} {
		item := parent.AddSubMenuItemCheckbox(row.title, "", false)
		m.sound[row.v] = item
		go func() {
			for range item.ClickedCh {
				m.show(soundCall(&registryv1.SoundRequest{ToastSound: row.v}))
			}
		}()
	}
	m.sound[registryv1.ToastSound_TOAST_SOUND_FILE].Hide() // until the daemon says it has one
	parent = systray.AddMenuItem("Read toasts aloud", "how much of a toast is read aloud")
	for _, row := range []struct {
		v     registryv1.ReadAloud
		title string
	}{
		{registryv1.ReadAloud_READ_ALOUD_OFF, "Off"},
		{registryv1.ReadAloud_READ_ALOUD_TITLE, "Title"},
		{registryv1.ReadAloud_READ_ALOUD_TITLE_AND_BODY, "Title and body"},
	} {
		item := parent.AddSubMenuItemCheckbox(row.title, "", false)
		m.read[row.v] = item
		go func() {
			for range item.ClickedCh {
				m.show(soundCall(&registryv1.SoundRequest{ReadAloud: row.v}))
			}
		}()
	}
	go func() {
		for range m.on.ClickedCh {
			mute := registryv1.SoundMute_SOUND_MUTE_ON
			if !m.on.Checked() {
				mute = registryv1.SoundMute_SOUND_MUTE_OFF
			}
			m.show(soundCall(&registryv1.SoundRequest{Mute: mute}))
		}
	}()
	menuSound = m
}

// soundCall asks the daemon to change or report the sound settings. A daemon
// that cannot be reached answers nil, and the rows are left as they were.
func soundCall(req *registryv1.SoundRequest) *registryv1.SoundResponse {
	c, err := client.Connect()
	if err != nil {
		return nil
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	resp := &registryv1.SoundResponse{}
	if err := c.Call(ctx, "rig.sound", req, resp); err != nil {
		return nil
	}
	return resp
}

// show sets the rows from the daemon's answer.
func (m *soundMenu) show(resp *registryv1.SoundResponse) {
	if m == nil || resp == nil {
		return
	}
	if resp.GetMuted() {
		m.on.Uncheck()
		m.on.SetTitle("Sounds (off)")
	} else {
		m.on.Check()
		m.on.SetTitle("Sounds")
	}
	if resp.GetProblem() != "" {
		m.on.SetTooltip("silent: " + resp.GetProblem())
	}
	for v, item := range m.sound {
		setChecked(item, v == resp.GetToastSound())
	}
	if resp.GetFileSet() {
		m.sound[registryv1.ToastSound_TOAST_SOUND_FILE].Show()
	}
	for v, item := range m.read {
		setChecked(item, v == resp.GetReadAloud())
	}
}

func setChecked(item *systray.MenuItem, on bool) {
	if on {
		item.Check()
	} else {
		item.Uncheck()
	}
}
