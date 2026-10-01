package daemon

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// eventProgram registers name, declaring kinds under events.
func eventProgram(t *testing.T, sock, name string, kinds ...string) *client.Client {
	t.Helper()
	c := dial(t, sock)
	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: name, Version: "1.0"}, nil
	})
	decl := testDeclaration(name)
	decl.Events = kinds
	if _, err := c.Hello(ctx5(t), decl); err != nil {
		t.Fatal(err)
	}
	return c
}

// E3, E4: a program publishes what it declared and nothing else, and a
// payload is JSON within its bound or refused whole.
func TestAProgramPublishesOnlyWhatItDeclared(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	graft := eventProgram(t, sock, "graft", "graft.job.done")
	ctx := ctx5(t)

	var pub registryv1.EventsPublishResponse
	if err := graft.Call(ctx, "rig.events.publish", &registryv1.EventsPublishRequest{
		Kind: "graft.job.done", PayloadJson: `{"job":7}`,
	}, &pub); err != nil {
		t.Fatal(err)
	}
	if ev := pub.GetEvent(); ev.GetSeq() == 0 || ev.GetSource() != "graft" || ev.GetPayloadJson() != `{"job":7}` {
		t.Fatalf("published %v", ev)
	}

	refused := []struct {
		who  *client.Client
		req  *registryv1.EventsPublishRequest
		code rigv1.Code
		why  string
	}{
		{graft, &registryv1.EventsPublishRequest{Kind: "graft.job.lost"}, rigv1.Code_CODE_DENIED, "a kind it did not declare"},
		{graft, &registryv1.EventsPublishRequest{Kind: "hand.changed"}, rigv1.Code_CODE_DENIED, "one of rig's kinds"},
		{graft, &registryv1.EventsPublishRequest{Kind: "graft.job.done", PayloadJson: "{not json"}, rigv1.Code_CODE_INVALID, "a payload that is not JSON"},
		{graft, &registryv1.EventsPublishRequest{Kind: "graft.job.done", PayloadJson: `"` + strings.Repeat("x", maxEventPayload) + `"`}, rigv1.Code_CODE_INVALID, "a payload over 16 KiB"},
		{dial(t, sock), &registryv1.EventsPublishRequest{Kind: "graft.job.done"}, rigv1.Code_CODE_DENIED, "a terminal publishing"},
	}
	for _, r := range refused {
		err := r.who.Call(ctx, "rig.events.publish", r.req, &registryv1.EventsPublishResponse{})
		wantCode(t, err, r.code, r.why)
	}

	// A program waits on its own kinds, and the event is there from the ring.
	var got registryv1.EventsWaitResponse
	if err := graft.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{
		Kinds: []string{"graft.*"}, TimeoutMs: 1000,
	}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.GetEvents()) != 1 || got.GetEvents()[0].GetKind() != "graft.job.done" {
		t.Fatalf("waited and got %v", &got)
	}
}

// A declaration naming a kind outside its own name is refused at hello.
func TestADeclarationOfAnotherOwnersKindIsRefused(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	for _, kind := range []string{"shelf.done", "graft", "graft.Done", "graft..x"} {
		c := dial(t, sock)
		decl := testDeclaration("graft")
		decl.Events = []string{kind}
		if _, err := c.Hello(ctx5(t), decl); err == nil {
			t.Errorf("declaring %q was accepted", kind)
		}
	}
}

// E5 until section 13's grant: rig's kinds are open, another program's are
// not, and a pattern that names nothing is invalid.
func TestWhoMayWaitOnWhat(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	term := dial(t, sock)
	ctx := ctx5(t)
	cases := []struct {
		kind string
		code rigv1.Code
	}{
		{"graft.*", rigv1.Code_CODE_DENIED},
		{"graft.job.done", rigv1.Code_CODE_DENIED},
		{"hand", rigv1.Code_CODE_INVALID},
		{"*", rigv1.Code_CODE_INVALID},
		{"hand.Changed", rigv1.Code_CODE_INVALID},
		{"", rigv1.Code_CODE_INVALID},
	}
	for _, c := range cases {
		err := term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{c.kind}}, &registryv1.EventsWaitResponse{})
		wantCode(t, err, c.code, "waiting on "+c.kind)
	}
	err := term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{}, &registryv1.EventsWaitResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "waiting on nothing")

	var got registryv1.EventsWaitResponse
	if err := term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{
		Kinds: []string{"hand.*", "toast.posted", "system.*", "timer.fired"},
	}, &got); err != nil {
		t.Fatalf("rig's own kinds: %v", err)
	}
}

// The first kind with a user: a hand change wakes a waiter on hand.*.
func TestAHandChangeWakesAWaiterOnTheBus(t *testing.T) {
	sock, d := upDaemon(t, nil)
	term := dial(t, sock)
	ctx := ctx5(t)

	var first registryv1.EventsWaitResponse
	if err := term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{"hand.*"}}, &first); err != nil {
		t.Fatal(err)
	}
	done := make(chan *registryv1.EventsWaitResponse, 1)
	go func() {
		var got registryv1.EventsWaitResponse
		_ = term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{
			After: first.GetLatest(), Epoch: first.GetEpoch(), Kinds: []string{"hand.*"}, TimeoutMs: 5000,
		}, &got)
		done <- &got
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := d.hand.ask("prog", "righand", "a test", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		evs := got.GetEvents()
		if len(evs) != 1 || evs[0].GetKind() != "hand.changed" || evs[0].GetSource() != "rig" ||
			!strings.Contains(evs[0].GetPayloadJson(), "HAND_PHASE_ASKING") || got.GetGap() {
			t.Fatalf("woken with %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter was never woken")
	}
}

// E1: a cursor from another epoch is from before a restart, and says so.
func TestACursorFromAnotherEpochIsAGap(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	var got registryv1.EventsWaitResponse
	if err := dial(t, sock).Call(ctx5(t), "rig.events.wait", &registryv1.EventsWaitRequest{
		After: 40, Epoch: 999_999, Kinds: []string{"hand.*"}, TimeoutMs: 5000,
	}, &got); err != nil {
		t.Fatal(err)
	}
	if !got.GetGap() || got.GetEpoch() == 999_999 {
		t.Fatalf("a stale epoch answered %v", &got)
	}
}

// E6: a wait with nothing to say answers empty with the cursor at its bound.
func TestAnIdleWaitAnswersEmptyAtItsTimeout(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	start := time.Now()
	var got registryv1.EventsWaitResponse
	if err := dial(t, sock).Call(ctx5(t), "rig.events.wait", &registryv1.EventsWaitRequest{
		Kinds: []string{"timer.fired"}, TimeoutMs: 200,
	}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.GetEvents()) != 0 || got.GetGap() || time.Since(start) < 150*time.Millisecond {
		t.Fatalf("an idle wait answered %v after %s", &got, time.Since(start))
	}
}

// E7: past the ring, the answer is a gap, never a silent skip.
func TestFallingBehindTheRingIsAGap(t *testing.T) {
	var b eventBus
	for range eventRingSize + 10 {
		b.publish("graft.tick", "graft", "")
	}
	all := eventMatcher([]string{"graft.*"})
	got, latest, gap, _ := b.after(0, all)
	if !gap || len(got) != eventRingSize || latest != eventRingSize+10 || got[0].GetSeq() != 11 {
		t.Fatalf("behind the ring: %d events, latest %d, gap %v", len(got), latest, gap)
	}
	if _, _, gap, _ := b.after(10, all); gap {
		t.Fatal("a cursor at the oldest dropped seq lost nothing, and was told it did")
	}
	if _, _, gap, _ := b.after(9, all); !gap {
		t.Fatal("a cursor before a dropped seq was not told")
	}
}

func TestPatternsMatchAKindOrEverythingBelowAPrefix(t *testing.T) {
	m := eventMatcher([]string{"hand.*", "toast.posted"})
	for kind, want := range map[string]bool{
		"hand.changed": true, "hand.x.y": true, "toast.posted": true,
		"handy.changed": false, "toast.posted.late": false, "hand": false,
	} {
		if m(kind) != want {
			t.Errorf("%s: got %v", kind, !want)
		}
	}
}

// A deadline is a change when it comes, not when somebody next asks: with
// no waiter at all, the countdown still starts the run and says so.
func TestADeadlinePublishesWithNobodyAsking(t *testing.T) {
	h := newHandDesk(time.Now)
	seen := make(chan registryv1.HandPhase, 8)
	h.published = func(st *registryv1.HandState) { seen <- st.GetPhase() }
	if _, err := h.ask("prog", "righand", "a test", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case p := <-seen:
			if p == registryv1.HandPhase_HAND_PHASE_DRIVING {
				return
			}
		case <-deadline:
			t.Fatal("the countdown ran out and nothing was published")
		}
	}
}
