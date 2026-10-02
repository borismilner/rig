package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig notify <severity> <title> [--body B] [--reply R]... [--text] [--speak S] [--wait D]`
// - section 12's toast from a shell. The severity is one of the wire enum's
// five, read off its descriptor. --reply and --text make it ask for a reply,
// and --wait waits that long for the answer and prints it.

type notifyFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	body    *string
	replies []string
	text    *bool
	speak   *string
	wait    *time.Duration
}

func notifyFlagSet() *notifyFlags {
	n := &notifyFlags{fs: flag.NewFlagSet("notify", flag.ContinueOnError)}
	n.asJSON = n.fs.Bool("json", false, "emit JSON")
	n.timeout = n.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	n.body = n.fs.String("body", "", "the detail under the title")
	n.fs.Func("reply", "a reply button; repeat for up to three", func(v string) error {
		n.replies = append(n.replies, v)
		return nil
	})
	n.text = n.fs.Bool("text", false, "take a free-text reply")
	n.speak = n.fs.String("speak", "", "a line to read aloud instead of the title or the body")
	n.wait = n.fs.Duration("wait", 0, "wait this long for the reply, and print it")
	return n
}

// severities are the enum's words, lowercased, UNSPECIFIED left out.
func severities() []string {
	values := registryv1.Severity_SEVERITY_UNSPECIFIED.Descriptor().Values()
	var out []string
	for i := range values.Len() {
		if v := values.Get(i); v.Number() != 0 {
			out = append(out, enumLabel(string(v.Name()), "SEVERITY_"))
		}
	}
	return out
}

func cmdNotify(args []string) (err error) {
	if len(args) > 0 && args[0] == "retract" {
		return cmdNotifyRetract(args[1:])
	}
	n := notifyFlagSet()
	flags, positional := partition(args)
	if err := n.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *n.asJSON) }()
	if len(positional) != 2 {
		return badArgumentf("usage: rig notify <%s> <title> [--body B]", strings.Join(severities(), "|"))
	}
	sev := registryv1.Severity(registryv1.Severity_value["SEVERITY_"+strings.ToUpper(positional[0])])
	if sev == registryv1.Severity_SEVERITY_UNSPECIFIED {
		return badArgumentf("%q is not a severity; one of %s", positional[0], strings.Join(severities(), ", "))
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *n.timeout)
	defer cancel()
	var resp registryv1.NotifyResponse
	if err := call(ctx, c, "rig.notify", &registryv1.NotifyRequest{
		Severity: sev, Title: positional[1], Body: *n.body, Replies: n.replies, ReplyText: *n.text,
		Speak: *n.speak,
	}, &resp); err != nil {
		return err
	}
	t := resp.GetToast()
	var answer *registryv1.ToastAnswer
	if *n.wait > 0 {
		if answer, err = awaitAnswer(c, t.GetRecordId(), *n.wait); err != nil {
			return err
		}
	}
	if *n.asJSON {
		out := map[string]any{
			"record_id": t.GetRecordId(), "severity": positional[0], "title": t.GetTitle(), "sender": t.GetSender(),
			"suppressed": t.GetSuppressed(),
		}
		if answer != nil {
			out["answer"] = map[string]any{
				"answered": answer.GetAnswered(), "reply": answer.GetReply(), "text": answer.GetText(),
				"dismissed": answer.GetDismissed(), "withdrawn": answer.GetWithdrawn(), "by": answer.GetBy(),
			}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	fmt.Printf("%s: %s (filed as %s, from %s)\n", enumLabel(t.GetSeverity().String(), "SEVERITY_"), t.GetTitle(), t.GetRecordId(), t.GetSender())
	if t.GetSuppressed() {
		fmt.Println("not shown: do not disturb is on. It is in the record; `rig dnd off` to see toasts again")
	}
	switch {
	case answer == nil:
	case !answer.GetAnswered():
		fmt.Printf("no reply within %s; the toast stays up until answered\n", *n.wait)
	case answer.GetWithdrawn():
		fmt.Printf("taken back by its sender, %s, before anybody answered\n", answer.GetBy())
	case answer.GetDismissed():
		fmt.Printf("closed without a reply, by %s\n", answer.GetBy())
	case answer.GetReply() != "":
		fmt.Printf("reply: %s (by %s)\n", answer.GetReply(), answer.GetBy())
	default:
		fmt.Printf("reply: %q (by %s)\n", answer.GetText(), answer.GetBy())
	}
	return nil
}

// awaitAnswer asks rig.toast.answer until the reply comes or wait passes. One
// call parks for at most a minute, so a longer wait is several.
func awaitAnswer(c *client.Client, id string, wait time.Duration) (*registryv1.ToastAnswer, error) {
	end := time.Now().Add(wait)
	for {
		left := time.Until(end)
		step := min(left, time.Minute)
		ctx, cancel := context.WithTimeout(context.Background(), step+defaultCallTimeout)
		var resp registryv1.ToastAnswerResponse
		err := call(ctx, c, "rig.toast.answer", &registryv1.ToastAnswerRequest{
			RecordId: id, TimeoutMs: uint32(max(step.Milliseconds(), 1)), //nolint:gosec // at most a minute
		}, &resp)
		cancel()
		if err != nil || resp.GetAnswer().GetAnswered() || time.Until(end) <= 0 {
			return resp.GetAnswer(), err
		}
	}
}

// `rig dnd on|off|status` - section 12's Do Not Disturb. While it is on,
// notifications are filed in the record and not drawn; urgent ones still are.
func cmdDND(args []string) (err error) {
	n := notifyFlagSet()
	flags, positional := partition(args)
	if err := n.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *n.asJSON) }()
	changes := map[string]registryv1.DndChange{
		"on": registryv1.DndChange_DND_CHANGE_ON, "off": registryv1.DndChange_DND_CHANGE_OFF,
		"status": registryv1.DndChange_DND_CHANGE_QUERY,
	}
	if len(positional) != 1 || changes[positional[0]] == registryv1.DndChange_DND_CHANGE_UNSPECIFIED {
		return badArgumentf("usage: rig dnd on|off|status")
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *n.timeout)
	defer cancel()
	var resp registryv1.ToastDndResponse
	if err := call(ctx, c, "rig.toast.dnd", &registryv1.ToastDndRequest{Change: changes[positional[0]]}, &resp); err != nil {
		return err
	}
	if *n.asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"on": resp.GetOn(), "suppressed": resp.GetSuppressed()})
	}
	if resp.GetOn() {
		fmt.Printf("do not disturb is on: %d held back so far, all in the record; urgent still shows\n", resp.GetSuppressed())
	} else {
		fmt.Println("do not disturb is off")
	}
	return nil
}

// cmdNotifyRetract is `rig notify retract [RECORD_ID] [--reason R]`
// (plan/53 slice 7): take back a toast this terminal sent, or with no id,
// every one of its still on screen.
func cmdNotifyRetract(args []string) (err error) {
	fs := flag.NewFlagSet("notify retract", flag.ContinueOnError)
	reason := fs.String("reason", "", "why it is taken back, kept with the retraction")
	asJSON := fs.Bool("json", false, "emit JSON")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) > 1 {
		return badArgumentf("usage: rig notify retract [RECORD_ID] [--reason R]")
	}
	req := &registryv1.ToastRetractRequest{Reason: *reason}
	if len(positional) == 1 {
		req.RecordId = positional[0]
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	var resp registryv1.ToastRetractResponse
	if err := call(ctx, c, "rig.toast.retract", req, &resp); err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"record_ids": resp.GetRecordIds(), "note": resp.GetNote()})
	}
	for _, id := range resp.GetRecordIds() {
		fmt.Printf("taken back: %s\n", id)
	}
	if resp.GetNote() != "" {
		fmt.Println(resp.GetNote())
	}
	return nil
}
