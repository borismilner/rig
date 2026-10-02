package daemon

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
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
	items []busItem
	wake  chan struct{}

	// parked is every wait now blocked, so a publish can say how many it
	// reached.
	parked map[*busWaiter]struct{}

	// durable keeps signal.* past a restart (plan/53 slice 1b); nil on an
	// unnamed estate, whose signals live in the ring like everything else.
	// With it set, a signal is written there and never held in the ring.
	durable *coord.Store
}

// busWaiter is one parked events.wait.
type busWaiter struct {
	match func(string) bool
	v     viewer
}

// park registers w until the returned func is called.
func (b *eventBus) park(w *busWaiter) (unpark func()) {
	b.mu.Lock()
	if b.parked == nil {
		b.parked = map[*busWaiter]struct{}{}
	}
	b.parked[w] = struct{}{}
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.parked, w)
		b.mu.Unlock()
	}
}

// reached counts the parked waits that match kind and may see it. Called
// with mu held.
func (b *eventBus) reached(kind, to string) uint32 {
	var n uint32
	for w := range b.parked {
		if w.match(kind) && w.v.sees(to) {
			n++
		}
	}
	return n
}

// seqBase is where a run's numbering starts: the epoch in the high 32 bits,
// so a cursor from before a restart still orders against every seq after
// it, and a stored signal is found by it.
func seqBase(epoch uint64) uint64 { return epoch << 32 }

// isDurable says which kinds are kept past a restart: a seat's signals.
func isDurable(kind string) bool { return strings.HasPrefix(kind, "signal.") }

// busItem is an event and who may see it: empty is everyone, otherwise one
// program's id, or "seat:<name>" for a signal addressed to one seat. A
// timer's fire is its owner's business alone.
type busItem struct {
	ev *registryv1.Event
	to string
}

// publish publishes one of rig's or a program's kinds, which are never
// durable, so it cannot fail.
func (b *eventBus) publish(kind, source, payload string) *registryv1.Event {
	ev, _, _ := b.publishTo(kind, source, payload, "")
	return ev
}

// publishTo numbers and publishes an event. A durable kind is written to
// the store under the lock, so stored order is seq order; if the write
// fails nobody is woken and the caller is told. delivered is how many
// waits in progress will receive it.
func (b *eventBus) publishTo(kind, source, payload, to string) (_ *registryv1.Event, delivered uint32, _ error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	ev := &registryv1.Event{
		Seq: b.seq, Kind: kind, AtUnixNano: time.Now().UnixNano(),
		Source: source, PayloadJson: payload,
	}
	if b.durable != nil && isDurable(kind) {
		if err := b.durable.PutSignal(coord.Signal{
			Seq: ev.GetSeq(), Kind: kind, At: ev.GetAtUnixNano(), Source: source, To: to, Payload: payload,
		}); err != nil {
			return nil, 0, err
		}
		b.ring()
		return ev, b.reached(kind, to), nil
	}
	b.items = append(b.items, busItem{ev: ev, to: to})
	if over := len(b.items) - eventRingSize; over > 0 {
		b.lost = b.items[over-1].ev.GetSeq()
		b.items = append(b.items[:0:0], b.items[over:]...)
	}
	b.ring()
	return ev, b.reached(kind, to), nil
}

// ring wakes every waiter. Called with mu held.
func (b *eventBus) ring() {
	if b.wake != nil {
		close(b.wake)
		b.wake = nil
	}
}

// publishRig publishes one of rig's own kinds with a proto message as its
// payload. A message that does not render is published with none, because a
// waiter woken without the detail still knows to re-read the state.
func (b *eventBus) publishRig(kind string, m proto.Message) {
	b.publishRigTo(kind, m, "")
}

// publishRigTo is publishRig seen by one program alone.
func (b *eventBus) publishRigTo(kind string, m proto.Message, to string) {
	payload := ""
	if m != nil {
		if raw, err := protojson.Marshal(m); err == nil {
			payload = string(raw)
		}
	}
	_, _, _ = b.publishTo(kind, eventSourceRig, payload, to)
}

// viewer is who is reading: a program's id, a seat's name, either or
// neither.
type viewer struct{ program, seat string }

func (v viewer) sees(to string) bool {
	return to == "" || v.program != "" && to == v.program || v.seat != "" && to == seatSource(v.seat)
}

// seatSource is how a seat is named as an event's source or addressee, so
// a seat and a program of the same name never read as one another.
func seatSource(seat string) string { return "seat:" + seat }

// after answers the matching events newer than seq that v may see, the
// latest seq, whether some after seq were dropped, and a channel that closes
// at the next publish.
func (b *eventBus) after(seq uint64, match func(string) bool, v viewer) (got []*registryv1.Event, latest uint64, gap bool, wake <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, it := range b.items {
		if it.ev.GetSeq() > seq && match(it.ev.GetKind()) && v.sees(it.to) {
			got = append(got, it.ev)
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

func rigRoots() []string {
	roots := make([]string, 0, len(kernel.RigEventRoots))
	for r := range kernel.RigEventRoots {
		roots = append(roots, r)
	}
	slices.Sort(roots)
	return roots
}

func (d *Daemon) serveEvents(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "events.publish":
		d.serveEventsPublish(c, f)
	case "events.wait":
		d.serveEventsWait(ctx, c, f)
	default:
		d.serveTimer(c, f, command)
	}
}

func (d *Daemon) serveEventsPublish(c *conn, f *rigv1.Frame) {
	var req registryv1.EventsPublishRequest
	if !unmarshalOr(c, f, "events.publish", &req) {
		return
	}
	id := f.GetStreamId()
	if root, _, _ := strings.Cut(req.GetKind(), "."); root == "signal" || req.GetToSeat() != "" {
		d.serveSignalPublish(c, id, &req)
		return
	}
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
	if !payloadOK(c, id, req.GetPayloadJson()) {
		return
	}
	ev, delivered, _ := d.events.publishTo(req.GetKind(), program, req.GetPayloadJson(), "")
	c.reply(id, &registryv1.EventsPublishResponse{Event: ev, Delivered: delivered})
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

	// A cursor from another epoch is from before a restart. What the ring
	// held is gone, so a wait on any kind kept only there is a gap (E1). A
	// stored signal is not gone: seqs order across epochs (seqBase), so the
	// cursor still finds exactly what came after it. Unnamed, nothing is
	// stored, and the answer starts over.
	after, gap := req.GetAfter(), false
	if req.GetEpoch() != 0 && req.GetEpoch() != d.epoch {
		gap = d.events.durable == nil || slices.ContainsFunc(kinds, func(k string) bool { return !isDurable(k) })
		if d.events.durable == nil {
			after = 0
		}
	}
	match := eventMatcher(kinds)
	stored := slices.ContainsFunc(kinds, isDurable)
	v := viewer{program: c.name()}
	if _, seat, _, ok := d.provenance(c); ok {
		v.seat = seat
	}
	// Registered for the whole call, so a publish counts every wait that
	// will receive it, not only those asleep at that instant.
	defer d.events.park(&busWaiter{match: match, v: v})()
	timer := time.NewTimer(min(time.Duration(req.GetTimeoutMs())*time.Millisecond, maxEventWait))
	defer timer.Stop()
	for {
		resp, wake, err := d.eventsAfter(after, match, stored, v)
		if err != nil {
			c.fail(id, rigv1.Code_CODE_INTERNAL, "rig.events.wait: the stored signals could not be read: "+err.Error())
			return
		}
		resp.Epoch, resp.Gap = d.epoch, resp.GetGap() || gap
		if len(resp.GetEvents()) > 0 || resp.GetGap() {
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

// payloadOK refuses a payload that is not one JSON value of at most 16 KiB.
func payloadOK(c *conn, id uint32, payload string) bool {
	if len(payload) > maxEventPayload || payload != "" && !json.Valid([]byte(payload)) {
		c.fail(id, rigv1.Code_CODE_INVALID,
			"rig.events.publish: payload_json is one JSON value of at most 16384 bytes, or empty; it is refused, never truncated (E4)")
		return false
	}
	return true
}

// maxEventAnswer keeps a wait's answer inside the 1 MiB frame.
const maxEventAnswer = 768 << 10

// eventsAfter is the ring's events and the stored signals after the cursor,
// in seq order, bounded by count and by bytes. Where it stops early, Latest
// is the last seq it carries, so the next call misses nothing.
//
// stored says the patterns name a signal kind, so the store is read at all.
func (d *Daemon) eventsAfter(after uint64, match func(string) bool, stored bool, v viewer) (*registryv1.EventsWaitResponse, <-chan struct{}, error) {
	// The ring first: it takes the wake channel, and a signal stored after
	// that rings it, so the read below can never miss one and park.
	got, latest, lost, wake := d.events.after(after, match, v)
	resp := &registryv1.EventsWaitResponse{Latest: latest, Gap: lost}
	if st := d.events.durable; st != nil && stored {
		// A first call (after 0) answers what this run holds, as the ring
		// does; earlier runs' signals are reached by a cursor from then, so
		// a fresh waiter is never handed a week of backlog.
		if after == 0 {
			after = seqBase(d.epoch)
		}
		sigs, more, err := st.SignalsAfter(after, coord.MaxSignalBatch, func(sig *coord.Signal) bool {
			return match(sig.Kind) && v.sees(sig.To)
		})
		if err != nil {
			return nil, wake, err
		}
		trimmed, err := st.SignalsTrimmed()
		if err != nil {
			return nil, wake, err
		}
		// Per kind, from what retention recorded taking (coord/signal.go).
		for kind, through := range trimmed {
			if through > after && match(kind) {
				resp.Gap = true
			}
		}
		for i := range sigs {
			got = append(got, signalEvent(&sigs[i]))
		}
		slices.SortFunc(got, func(a, b *registryv1.Event) int { return cmpSeq(a.GetSeq(), b.GetSeq()) })
		if more {
			last := sigs[len(sigs)-1].Seq
			got = slices.DeleteFunc(got, func(ev *registryv1.Event) bool { return ev.GetSeq() > last })
			resp.Latest = last
		}
	}
	size := 0
	for i, ev := range got {
		if size += proto.Size(ev); size > maxEventAnswer && i > 0 {
			got, resp.Latest = got[:i], got[i-1].GetSeq()
			break
		}
	}
	resp.Events = got
	return resp, wake, nil
}

func signalEvent(sig *coord.Signal) *registryv1.Event {
	return &registryv1.Event{
		Seq: sig.Seq, Kind: sig.Kind, AtUnixNano: sig.At, Source: sig.Source, PayloadJson: sig.Payload,
	}
}

func cmpSeq(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func quoteKind(k string) string {
	if k == "" {
		return "an empty kind"
	}
	return "kind " + k
}
