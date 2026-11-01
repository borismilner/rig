package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/audio"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

func speechWireDaemon(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, script := range map[string]string{
		"kokoro-say": "#!/bin/sh\n[ \"$1\" = \"--rate\" ] && { echo 24000; exit 0; }\nwhile IFS= read -r line; do head -c 48000 /dev/zero; done\n",
		"pw-play":    "#!/bin/sh\nexec cat >/dev/null\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	a, err := audio.New(audio.Options{SoundDir: dir, Look: func(name string) (string, error) {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			return "", exec.ErrNotFound
		}
		return path, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	sock, _ := upRecordDaemonWith(t, func(d *Daemon) { d.audio = a })
	return sock
}

func TestSpeechLifecycleOverTheWire(t *testing.T) {
	sock := speechWireDaemon(t)
	c := seated(t, sock, "reader")
	other := seated(t, sock, "other")
	ctx := recordCtx(t)
	var started registryv1.SpeechResponse
	if err := c.Call(ctx, "rig.speech.start", &registryv1.SpeechStartRequest{Text: "a reading region"}, &started); err != nil {
		t.Fatal(err)
	}
	if started.GetId() == "" || (started.GetState() != audio.SpeechQueued && started.GetState() != audio.SpeechPlaying) {
		t.Fatal(&started)
	}
	req := &registryv1.SpeechHandleRequest{Id: started.GetId()}
	for _, verb := range []string{"speech.state", "speech.stop"} {
		err := other.Call(ctx, "rig."+verb, req, &registryv1.SpeechResponse{})
		wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "foreign "+verb)
	}
	// Read through the wire until the existing queue really picked the item up.
	deadline := time.Now().Add(time.Second)
	for {
		var st registryv1.SpeechResponse
		if err := c.Call(ctx, "rig.speech.state", req, &st); err != nil {
			t.Fatal(err)
		}
		if st.GetState() == audio.SpeechPlaying {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never playing: %v", &st)
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		var stopped registryv1.SpeechResponse
		if err := c.Call(ctx, "rig.speech.stop", req, &stopped); err != nil {
			t.Fatal(err)
		}
		if stopped.GetId() != started.GetId() || stopped.GetState() != audio.SpeechCancelled {
			t.Fatal(&stopped)
		}
	}
	var settings registryv1.SoundResponse
	if err := c.Call(ctx, "rig.sound", &registryv1.SoundRequest{}, &settings); err != nil {
		t.Fatal(err)
	}
	if settings.GetMuted() {
		t.Fatal("stop changed global mute")
	}
	// Existing Say still waits for the common queue and answers its old shape.
	var legacy registryv1.SayResponse
	if err := c.Call(ctx, "rig.say", &registryv1.SayRequest{Text: "legacy", Wait: true}, &legacy); err != nil {
		t.Fatal(err)
	}
	if !legacy.GetHeard() {
		t.Fatal("legacy Say did not finish")
	}
	fresh := seated(t, speechWireDaemon(t), "reader")
	for _, verb := range []string{"speech.state", "speech.stop"} {
		err := fresh.Call(ctx, "rig."+verb, req, &registryv1.SpeechResponse{})
		wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "prior daemon "+verb)
	}
}

func TestSpeechLifecycleWireValidationAndUnavailable(t *testing.T) {
	c := seated(t, speechWireDaemon(t), "reader")
	ctx := recordCtx(t)
	for _, text := range []string{" ", strings.Repeat("x", maxSayText+1)} {
		err := c.Call(ctx, "rig.speech.start", &registryv1.SpeechStartRequest{Text: text}, &registryv1.SpeechResponse{})
		wantCode(t, err, rigv1.Code_CODE_INVALID, "invalid speech text")
	}
	for _, verb := range []string{"speech.state", "speech.stop"} {
		err := c.Call(ctx, "rig."+verb, &registryv1.SpeechHandleRequest{}, &registryv1.SpeechResponse{})
		wantCode(t, err, rigv1.Code_CODE_INVALID, "empty handle")
	}
	if err := c.Call(ctx, "rig.sound", &registryv1.SoundRequest{Mute: registryv1.SoundMute_SOUND_MUTE_ON}, &registryv1.SoundResponse{}); err != nil {
		t.Fatal(err)
	}
	err := c.Call(ctx, "rig.speech.start", &registryv1.SpeechStartRequest{Text: "muted"}, &registryv1.SpeechResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "muted speech")
	silent := seated(t, upRecordDaemon(t), "reader")
	err = silent.Call(ctx, "rig.speech.start", &registryv1.SpeechStartRequest{Text: "silent"}, &registryv1.SpeechResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "daemon without audio")
}

func TestSpeechLifecycleDeclarationScrubsText(t *testing.T) {
	// Self verbs currently do not produce call records (observe.go). Keep the
	// declaration safe for any renderer or observation consumer that uses it.
	r := (&Daemon{}).compileRedaction(selfDeclaration())["speech.start"]
	if r == nil || r.Empty() {
		t.Fatal("speech text has no declared redaction")
	}
	const payload = `{"text":"private-reading-text"}`
	got, err := r.Apply([]byte(payload))
	if err != nil || string(got) != `{"text":"[redacted]"}` {
		t.Fatalf("speech redaction: %s, %v", got, err)
	}
}
