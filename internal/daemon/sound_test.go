package daemon

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/rig/internal/audio"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// A daemon built with no audio refuses both verbs, naming why.
func TestASilentDaemonRefusesSoundAndSay(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	c := seated(t, sock, "backend-1")
	err := c.Call(ctx, "rig.sound", &registryv1.SoundRequest{}, &registryv1.SoundResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "rig.sound with no audio")
	err = c.Call(ctx, "rig.say", &registryv1.SayRequest{Text: "hi"}, &registryv1.SayResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "rig.say with no audio")
}

// rig.sound changes and reports the settings, keeps them on disk, and refuses
// what the daemon cannot honour; rig.say refuses while muted.
func TestSoundSettingsOverTheWire(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "sound.json")
	a, err := audio.New(audio.Options{
		SettingsPath: settings, SoundDir: t.TempDir(),
		Look: func(string) (string, error) { return "", exec.ErrNotFound },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	sock, _ := upRecordDaemonWith(t, func(d *Daemon) { d.audio = a })
	ctx := recordCtx(t)
	c := seated(t, sock, "backend-1")
	sound := func(req *registryv1.SoundRequest) *registryv1.SoundResponse {
		t.Helper()
		var r registryv1.SoundResponse
		if err := c.Call(ctx, "rig.sound", req, &r); err != nil {
			t.Fatalf("rig.sound %v: %v", req, err)
		}
		return &r
	}

	got := sound(&registryv1.SoundRequest{})
	if got.GetMuted() || got.GetToastSound() != registryv1.ToastSound_TOAST_SOUND_HAIL ||
		got.GetReadAloud() != registryv1.ReadAloud_READ_ALOUD_TITLE {
		t.Errorf("defaults %+v", got)
	}
	if got.GetPlayer() != "" || got.GetProblem() == "" {
		t.Errorf("no player, yet %+v", got)
	}

	got = sound(&registryv1.SoundRequest{
		Mute:       registryv1.SoundMute_SOUND_MUTE_ON,
		ReadAloud:  registryv1.ReadAloud_READ_ALOUD_TITLE_AND_BODY,
		ToastSound: registryv1.ToastSound_TOAST_SOUND_BADGE,
	})
	if !got.GetMuted() || got.GetToastSound() != registryv1.ToastSound_TOAST_SOUND_BADGE ||
		got.GetReadAloud() != registryv1.ReadAloud_READ_ALOUD_TITLE_AND_BODY {
		t.Errorf("after the change %+v", got)
	}
	if st := sound(&registryv1.SoundRequest{}); !st.GetMuted() {
		t.Error("an empty request changed the settings")
	}

	err = c.Call(ctx, "rig.sound", &registryv1.SoundRequest{ToastSound: registryv1.ToastSound_TOAST_SOUND_FILE}, &registryv1.SoundResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "the file sound with no file given")
	err = c.Call(ctx, "rig.sound", &registryv1.SoundRequest{ReadAloud: 99}, &registryv1.SoundResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "an unknown read-aloud mode")

	err = c.Call(ctx, "rig.say", &registryv1.SayRequest{Text: "hello"}, &registryv1.SayResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "rig.say while muted")
	err = c.Call(ctx, "rig.say", &registryv1.SayRequest{Text: strings.Repeat("a", maxSayText+1)}, &registryv1.SayResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "rig.say past its bound")

	// The notification path reaches the audio and does not break a toast.
	var sent registryv1.NotifyResponse
	if err := c.Call(ctx, "rig.notify", &registryv1.NotifyRequest{
		Severity: registryv1.Severity_SEVERITY_INFO, Title: "still drawn",
	}, &sent); err != nil {
		t.Fatalf("rig.notify with audio: %v", err)
	}
}
