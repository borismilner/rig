package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig events wait <kind>...` - section 52's bus at the prompt.
//
// PUBLISHING IS NOT HERE YET. A program publishes under its own name (E3);
// a seat, a terminal included, may publish signal.<words> (plan/53), which
// agents do through events_publish and nothing at the prompt needs today.

const eventsUsage = "usage: rig events wait <kind>... [--after N --epoch E] [--follow]"

type eventsFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	after   *uint64
	epoch   *uint64
	follow  *bool
}

func eventsFlagSet() *eventsFlags {
	e := &eventsFlags{fs: flag.NewFlagSet("events", flag.ContinueOnError)}
	e.asJSON = e.fs.Bool("json", false, "emit JSON")
	e.timeout = e.fs.Duration("timeout", maxEventsWait, "how long one wait parks, at most 60s")
	e.after = e.fs.Uint64("after", 0, "the cursor: answer events after this seq")
	e.epoch = e.fs.Uint64("epoch", 0, "the epoch the cursor is from")
	e.follow = e.fs.Bool("follow", false, "keep waiting and print each event as it comes")
	return e
}

// maxEventsWait is rigd's own bound on one wait (E6).
const maxEventsWait = 60 * time.Second

func cmdEvents(args []string) (err error) {
	e := eventsFlagSet()
	flags, positional := partition(args)
	if err := e.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *e.asJSON) }()
	if len(positional) < 2 || positional[0] != "wait" {
		return badArgumentf(eventsUsage)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()

	out := json.NewEncoder(os.Stdout)
	req := &registryv1.EventsWaitRequest{
		After: *e.after, Epoch: *e.epoch, Kinds: positional[1:],
		TimeoutMs: uint32(min(max(*e.timeout, 0), maxEventsWait).Milliseconds()), //nolint:gosec // at most a minute
	}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.GetTimeoutMs())*time.Millisecond+defaultCallTimeout)
		var resp registryv1.EventsWaitResponse
		err := call(ctx, c, "rig.events.wait", req, &resp)
		cancel()
		if err != nil {
			return err
		}
		if !*e.follow {
			if *e.asJSON {
				return out.Encode(eventsJSON(&resp))
			}
			printEvents(&resp)
			fmt.Printf("latest %d, epoch %d\n", resp.GetLatest(), resp.GetEpoch())
			return nil
		}
		if *e.asJSON {
			if resp.GetGap() {
				_ = out.Encode(map[string]any{"gap": true, "latest": resp.GetLatest(), "epoch": resp.GetEpoch()})
			}
			for _, ev := range resp.GetEvents() {
				_ = out.Encode(eventJSON(ev))
			}
		} else {
			printEvents(&resp)
		}
		req.After, req.Epoch = resp.GetLatest(), resp.GetEpoch()
	}
}

func printEvents(resp *registryv1.EventsWaitResponse) {
	if resp.GetGap() {
		fmt.Println("gap: events were lost, past the ring or across a restart; re-read the state")
	}
	for _, ev := range resp.GetEvents() {
		at := time.Unix(0, ev.GetAtUnixNano()).Format("15:04:05.000")
		fmt.Printf("%d  %s  %s  from %s  %s\n", ev.GetSeq(), at, ev.GetKind(), ev.GetSource(), ev.GetPayloadJson())
	}
}

func eventsJSON(resp *registryv1.EventsWaitResponse) map[string]any {
	evs := make([]map[string]any, 0, len(resp.GetEvents()))
	for _, ev := range resp.GetEvents() {
		evs = append(evs, eventJSON(ev))
	}
	return map[string]any{"events": evs, "latest": resp.GetLatest(), "epoch": resp.GetEpoch(), "gap": resp.GetGap()}
}

func eventJSON(ev *registryv1.Event) map[string]any {
	m := map[string]any{
		"seq": ev.GetSeq(), "kind": ev.GetKind(), "source": ev.GetSource(),
		"at": time.Unix(0, ev.GetAtUnixNano()).UTC().Format(time.RFC3339Nano),
	}
	if p := ev.GetPayloadJson(); p != "" {
		m["payload"] = json.RawMessage(p)
	}
	return m
}
