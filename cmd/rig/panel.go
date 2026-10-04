package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig panel put|list|act` - plan/55's board at the prompt. The owner and
// the presser are the terminal's seat, which rigd names; nothing here says
// who is calling.

const panelUsage = "usage: rig panel put [--card ID] --title T [--status S] [--severity info|success|warning|error]\n" +
	"                     [--progress 0..1 | --no-progress] [--busy|--idle] [--fact LABEL=VALUE]... [--action A]...\n" +
	"                     [--project P] [--body B] [--close]\n" +
	"       rig panel list [--from SENDER]\n" +
	"       rig panel act <card> <action>"

type panelFlags struct {
	fs                                     *flag.FlagSet
	asJSON                                 *bool
	card, title, status, severity, project *string
	body, progress, from                   *string
	noProgress, busy, idle, closeIt        *bool
	facts, actions                         listFlag
}

func panelFlagSet() *panelFlags {
	p := &panelFlags{fs: flag.NewFlagSet("panel", flag.ContinueOnError)}
	p.asJSON = p.fs.Bool("json", false, "emit JSON")
	p.card = p.fs.String("card", "", "put: the card to change; none puts a new one")
	p.title = p.fs.String("title", "", "put: the card's title")
	p.status = p.fs.String("status", "", "put: one line under the title")
	p.severity = p.fs.String("severity", "", "put: info, success, warning or error")
	p.project = p.fs.String("project", "", "put: the project the card is about")
	p.body = p.fs.String("body", "", "put: the card's longer text")
	p.progress = p.fs.String("progress", "", "put: how far along, 0 to 1")
	p.noProgress = p.fs.Bool("no-progress", false, "put: take the progress bar off")
	p.busy = p.fs.Bool("busy", false, "put: show it working, with no known progress")
	p.idle = p.fs.Bool("idle", false, "put: no longer busy")
	p.closeIt = p.fs.Bool("close", false, "put: close the card; it stays on the board a day")
	p.fs.Var(&p.facts, "fact", "put: LABEL=VALUE, up to 6; any one replaces them all")
	p.fs.Var(&p.actions, "action", "put: a button label, up to 3; any one replaces them all")
	p.from = p.fs.String("from", "", "list: only this sender's cards")
	return p
}

func cmdPanel(args []string) (err error) {
	p := panelFlagSet()
	flags, positional := partition(args)
	if err := p.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *p.asJSON) }()
	if len(positional) == 0 {
		return badArgumentf(panelUsage)
	}
	var req, resp proto.Message
	switch {
	case positional[0] == "put" && len(positional) == 1:
		put, err := p.putRequest()
		if err != nil {
			return err
		}
		req, resp = put, &verbsv1.PanelPutResponse{}
	case positional[0] == subList && len(positional) == 1:
		req, resp = &verbsv1.PanelListRequest{From: *p.from}, &verbsv1.PanelListResponse{}
	case positional[0] == "act" && len(positional) == 3:
		req, resp = &verbsv1.PanelActRequest{Card: positional[1], Action: positional[2]}, &verbsv1.PanelActResponse{}
	default:
		return badArgumentf(panelUsage)
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultCallTimeout)
	defer cancel()
	if err := call(ctx, c, "rig.panel."+positional[0], req, resp); err != nil {
		return err
	}
	if *p.asJSON {
		return json.NewEncoder(os.Stdout).Encode(resp)
	}
	switch r := resp.(type) {
	case *verbsv1.PanelPutResponse:
		printCard(r.GetCard())
	case *verbsv1.PanelListResponse:
		for _, card := range r.GetCards() {
			printCard(card)
		}
		if n := r.GetOmitted(); n > 0 {
			fmt.Printf("and %d more\n", n)
		}
	case *verbsv1.PanelActResponse:
		fmt.Printf("%s pressed %q on %s, sent to %s\n", r.GetBy(), r.GetAction(), r.GetCard(), r.GetTo())
	}
	return nil
}

// putRequest carries only the flags that were given, so a change touches
// only what it names.
func (p *panelFlags) putRequest() (*verbsv1.PanelPutRequest, error) {
	given := map[string]bool{}
	p.fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	req := &verbsv1.PanelPutRequest{Card: *p.card, ClearProgress: *p.noProgress, Close: *p.closeIt}
	opt := func(name string, v *string) *string {
		if given[name] {
			return proto.String(*v)
		}
		return nil
	}
	req.Title, req.Status, req.Severity = opt("title", p.title), opt("status", p.status), opt("severity", p.severity)
	req.Project, req.Body = opt("project", p.project), opt("body", p.body)
	if given["progress"] {
		var v float64
		if _, err := fmt.Sscanf(*p.progress, "%g", &v); err != nil {
			return nil, badArgumentf("--progress is a number from 0 to 1, not %q", *p.progress)
		}
		req.Progress = proto.Float64(v)
	}
	switch {
	case *p.busy && *p.idle:
		return nil, badArgumentf("--busy and --idle together say two things")
	case *p.busy:
		req.Busy = proto.Bool(true)
	case *p.idle:
		req.Busy = proto.Bool(false)
	}
	if given["fact"] {
		req.Facts = &verbsv1.PanelFacts{}
		for _, f := range p.facts.v {
			label, value, ok := strings.Cut(f, "=")
			if !ok {
				return nil, badArgumentf("--fact is LABEL=VALUE, not %q", f)
			}
			req.Facts.Facts = append(req.Facts.Facts, &verbsv1.PanelFact{Label: label, Value: value})
		}
	}
	if given["action"] {
		req.Actions = &verbsv1.PanelActions{Labels: p.actions.v}
	}
	return req, nil
}

func printCard(c *verbsv1.PanelCard) {
	state := c.GetSeverity()
	if state == "" {
		state = "info"
	}
	if c.GetBusy() {
		state += ", busy"
	}
	if c.GetClosed() {
		state += ", closed"
	}
	at := time.Unix(0, c.GetUpdatedUnixNano()).Format("15:04:05")
	fmt.Printf("%s  %s  %s  [%s]  from %s\n", c.GetId(), at, c.GetTitle(), state, c.GetFrom())
	if s := c.GetStatus(); s != "" {
		fmt.Printf("    %s\n", s)
	}
	if c.GetHasProgress() {
		fmt.Printf("    %.0f%%\n", c.GetProgress()*100)
	}
	for _, f := range c.GetFacts() {
		fmt.Printf("    %s: %s\n", f.GetLabel(), f.GetValue())
	}
	if as := c.GetActions(); len(as) > 0 {
		fmt.Printf("    actions: %s\n", strings.Join(as, " | "))
	}
}
