package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// rig sound and rig say, section 12's sounds and speech (plan/12, S1-S5).

const soundUsage = "usage: rig sound status | on | off | read off|title|title-and-body | use hail|badge|file"

// soundWords are the CLI's words for the wire's enums.
var (
	readAloudWords = map[string]registryv1.ReadAloud{
		"off": registryv1.ReadAloud_READ_ALOUD_OFF, "title": registryv1.ReadAloud_READ_ALOUD_TITLE,
		"title-and-body": registryv1.ReadAloud_READ_ALOUD_TITLE_AND_BODY,
	}
	toastSoundWords = map[string]registryv1.ToastSound{
		"hail": registryv1.ToastSound_TOAST_SOUND_HAIL, "badge": registryv1.ToastSound_TOAST_SOUND_BADGE,
		"file": registryv1.ToastSound_TOAST_SOUND_FILE,
	}
)

func wordOf[E comparable](words map[string]E, v E) string {
	for w, e := range words {
		if e == v {
			return w
		}
	}
	return "unknown"
}

func cmdSound(args []string) (err error) {
	fs := flag.NewFlagSet("sound", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	var req registryv1.SoundRequest
	switch {
	case len(positional) == 1 && positional[0] == "status":
	case len(positional) == 1 && positional[0] == "on":
		req.Mute = registryv1.SoundMute_SOUND_MUTE_OFF
	case len(positional) == 1 && positional[0] == "off":
		req.Mute = registryv1.SoundMute_SOUND_MUTE_ON
	case len(positional) == 2 && positional[0] == "read" && readAloudWords[positional[1]] != registryv1.ReadAloud_READ_ALOUD_UNSPECIFIED:
		req.ReadAloud = readAloudWords[positional[1]]
	case len(positional) == 2 && positional[0] == "use" && toastSoundWords[positional[1]] != registryv1.ToastSound_TOAST_SOUND_UNSPECIFIED:
		req.ToastSound = toastSoundWords[positional[1]]
	default:
		return badArgumentf(soundUsage)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	var resp registryv1.SoundResponse
	if err := call(ctx, c, "rig.sound", &req, &resp); err != nil {
		return err
	}
	read, sound := wordOf(readAloudWords, resp.GetReadAloud()), wordOf(toastSoundWords, resp.GetToastSound())
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"muted": resp.GetMuted(), "read_aloud": read, "toast_sound": sound, "file_set": resp.GetFileSet(),
			"player": resp.GetPlayer(), "engine": resp.GetEngine(), "problem": resp.GetProblem(),
		})
	}
	if resp.GetMuted() {
		fmt.Println("sounds are off: toasts are silent and nothing is read aloud")
	} else {
		fmt.Printf("sounds are on: toasts play the %s, and read aloud: %s\n", sound, read)
	}
	if p := resp.GetProblem(); p != "" {
		fmt.Println("silent because: " + p)
	}
	return nil
}

func cmdSay(args []string) (err error) {
	fs := flag.NewFlagSet("say", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	wait := fs.Duration("wait", 0, "return once it has been heard, waiting at most this long (the daemon's bound is 2m)")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	text := strings.Join(positional, " ")
	if strings.TrimSpace(text) == "" {
		return badArgumentf(`usage: rig say [--wait D] "some words"`)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	budget := defaultCallTimeout
	if *wait > 0 {
		budget = *wait
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	var resp registryv1.SayResponse
	err = call(ctx, c, "rig.say", &registryv1.SayRequest{Text: text, Wait: *wait > 0}, &resp)
	if *wait > 0 && errors.Is(err, context.DeadlineExceeded) {
		err, resp.Heard = nil, false // still speaking; the line is not lost
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"heard": resp.GetHeard()})
	}
	if *wait > 0 && !resp.GetHeard() {
		fmt.Println("still speaking when the wait ran out")
	}
	return nil
}
