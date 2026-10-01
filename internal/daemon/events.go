package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/kernel"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 52's event bus: one in-memory log, one counter, a cursor per
// waiter. It is §16's watch shape, and the ring and wake that toastRing and
// handDesk each wrote for themselves, once.

const (
	eventRingSize   = 4096
	maxEventPayload = 16 << 10
	maxEventWait    = 60 * time.Second
	maxEventKinds   = 32
	eventSourceRig  = "rig"
)

// eventBus holds the latest events and wakes waiters. Its zero value is
// ready to use.
type eventBus struct {
	mu    sync.Mutex
	seq   uint64
	lost  uint64 // the newest seq the ring has dropped; 0 while it dropped none
	items []*registryv1.Event
	wake  chan struct{}
}

func (b *eventBus) publish(kind, source, payload string) *registryv1.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	ev := &registryv1.Event{
		Seq: b.seq, Kind: kind, AtUnixNano: time.Now().UnixNano(),
		Source: source, PayloadJson: payload,
	}
	b.items = append(b.items, ev)
	if over := len(b.items) - eventRingSize; over > 0 {
		b.lost = b.items[over-1].GetSeq()
		b.items = append(b.items[:0:0], b.items[over:]...)
	}
	if b.wake != nil {
		close(b.wake)
		b.wake = nil
	}
	return ev
}

// publishRig publishes one of rig's own kinds with a proto message as its
// payload. A message that does not render is published with none, because a
// waiter woken without the detail still knows to re-read the state.
func (b *eventBus) publishRig(kind string, m proto.Message) {
	payload := ""
	if m != nil {
		if raw, err := protojson.Marshal(m); err == nil {
			payload = string(raw)
		}
	}
	b.publish(kind, eventSourceRig, payload)
}

// after answers the matching events newer than seq, the latest seq, whether
// some after seq were dropped, and a channel that closes at the next publish.
func (b *eventBus) after(seq uint64, match func(string) bool) (got []*registryv1.Event, latest uint64, gap bool, wake <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.items {
		if ev.GetSeq() > seq && match(ev.GetKind()) {
			got = append(got, ev)
		}
	}
	if b.wake == nil {
		b.wake = make(chan struct{})
	}
	return got, b.seq, seq < b.lost, b.wake
}

// eventMatcher turns patterns into one test. A pattern is a kind, or a
// prefix ending ".*" that matches every kind below it (E2).
func eventMatcher(patterns []string) func(string) bool {
	return func(kind string) bool {
		for _, p := range patterns {
			if prefix, ok := strings.CutSuffix(p, ".*"); ok {
				if strings.HasPrefix(kind, prefix+".") {
					return true
				}
			} else if kind == p {
				return true
			}
		}
		return false
	}
}

// badPattern says why a caller may not wait on pattern, or returns empty.
//
// ⛔ WHO MAY WAIT ON WHAT, until section 13's subscribe grant is built:
// every caller may wait on rig's own kinds, as every caller may call
// hand.wait and toast.wait today, and a program may wait on its own. Waiting
// on another program's kinds needs the grant, so it is refused rather than
// allowed without one (E5).
func badPattern(p, program string) (msg string, denied bool) {
	if len(p) > kernel.MaxEventKind {
		return "is over 128 bytes", false
	}
	kind, prefix := strings.CutSuffix(p, ".*")
	root, words, _ := strings.Cut(kind, ".")
	switch {
	case root == "" || words == "" && !prefix:
		return "names no kind: a pattern is a kind like hand.changed or a prefix like hand.*", false
	case words != "":
		if bad := kernel.BadEventWords(words); bad != "" {
			return bad, false
		}
	}
	switch {
	case kernel.RigEventRoots[root], program != "" && root == program:
		return "", false
	default:
		return "is another program's: waiting on it needs section 13's subscribe grant, which is not built yet. " +
			"rig's own kinds (" + strings.Join(rigRoots(), ".*, ") + ".*) and your own are open", true
	}
}

func rigRoots() []string { return []string{"hand", "system", "timer", "toast"} }

func (d *Daemon) serveEvents(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "events.publish":
		d.serveEventsPublish(c, f)
	case "events.wait":
		d.serveEventsWait(ctx, c, f)
	}
}

func (d *Daemon) serveEventsPublish(c *conn, f *rigv1.Frame) {
	var req registryv1.EventsPublishRequest
	if !unmarshalOr(c, f, "events.publish", &req) {
		return
	}
	id := f.GetStreamId()
	program := c.name()
	if program == "" {
		c.failStatus(id, &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig.events.publish: only a registered program publishes events",
			Precondition: "the caller registered with hello and declared the kind under events",
			Actual:       "this connection is not a registered program",
			Fix:          "publish from the program the event is about; rig publishes its own kinds itself",
		})
		return
	}
	me, ok := d.kernel.See(c.principal()).Program(program)
	if !ok || !me.DeclaresEvent(req.GetKind()) {
		c.failStatus(id, &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig.events.publish: " + program + " did not declare " + quoteKind(req.GetKind()),
			Precondition: "the kind is one this program declared under events at registration (section 52 E3)",
			Actual:       "declared: " + strings.Join(me.Events, ", "),
			Fix:          "declare it under events, as " + program + ".<what happened>, and register again",
		})
		return
	}
	payload := req.GetPayloadJson()
	if len(payload) > maxEventPayload || payload != "" && !json.Valid([]byte(payload)) {
		c.fail(id, rigv1.Code_CODE_INVALID,
			"rig.events.publish: payload_json is one JSON value of at most 16384 bytes, or empty; it is refused, never truncated (E4)")
		return
	}
	c.reply(id, &registryv1.EventsPublishResponse{Event: d.events.publish(req.GetKind(), program, payload)})
}

func (d *Daemon) serveEventsWait(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.EventsWaitRequest
	if !unmarshalOr(c, f, "events.wait", &req) {
		return
	}
	id := f.GetStreamId()
	kinds := req.GetKinds()
	if len(kinds) == 0 || len(kinds) > maxEventKinds {
		c.fail(id, rigv1.Code_CODE_INVALID, "rig.events.wait: name 1 to 32 kinds, each a kind or a prefix ending .*")
		return
	}
	for _, p := range kinds {
		if msg, denied := badPattern(p, c.name()); msg != "" {
			code := rigv1.Code_CODE_INVALID
			if denied {
				code = rigv1.Code_CODE_DENIED
			}
			c.fail(id, code, "rig.events.wait: "+quoteKind(p)+" "+msg)
			return
		}
	}

	// A cursor from another epoch is from before a restart: everything it
	// had not seen is gone, so the answer starts over and says so (E1).
	after, gap := req.GetAfter(), false
	if req.GetEpoch() != 0 && req.GetEpoch() != d.epoch {
		after, gap = 0, true
	}
	match := eventMatcher(kinds)
	timer := time.NewTimer(min(time.Duration(req.GetTimeoutMs())*time.Millisecond, maxEventWait))
	defer timer.Stop()
	for {
		got, latest, lost, wake := d.events.after(after, match)
		resp := &registryv1.EventsWaitResponse{Events: got, Latest: latest, Epoch: d.epoch, Gap: gap || lost}
		if len(got) > 0 || resp.GetGap() {
			c.reply(id, resp)
			return
		}
		select {
		case <-wake:
		case <-timer.C:
			c.reply(id, resp)
			return
		case <-ctx.Done():
			return
		}
	}
}

func quoteKind(k string) string {
	if k == "" {
		return "an empty kind"
	}
	return "kind " + k
}
