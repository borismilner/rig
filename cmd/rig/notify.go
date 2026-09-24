package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig notify <severity> <title> [--body B]` - section 12's toast from a shell.
// The severity is one of the wire enum's five, read off its descriptor.

type notifyFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	body    *string
}

func notifyFlagSet() *notifyFlags {
	n := &notifyFlags{fs: flag.NewFlagSet("notify", flag.ContinueOnError)}
	n.asJSON = n.fs.Bool("json", false, "emit JSON")
	n.timeout = n.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	n.body = n.fs.String("body", "", "the detail under the title")
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
		Severity: sev, Title: positional[1], Body: *n.body,
	}, &resp); err != nil {
		return err
	}
	t := resp.GetToast()
	if *n.asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"record_id": t.GetRecordId(), "severity": positional[0], "title": t.GetTitle(), "sender": t.GetSender(),
			"suppressed": t.GetSuppressed(),
		})
	}
	fmt.Printf("%s: %s (filed as %s, from %s)\n", enumLabel(t.GetSeverity().String(), "SEVERITY_"), t.GetTitle(), t.GetRecordId(), t.GetSender())
	if t.GetSuppressed() {
		fmt.Println("not shown: do not disturb is on. It is in the record; `rig dnd off` to see toasts again")
	}
	return nil
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
