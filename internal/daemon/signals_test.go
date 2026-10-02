package daemon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/53 slice 1: a seat's signals, and rig's lease and roster changes, on
// section 52's bus.

func waitOn(t *testing.T, c *client.Client, after uint64, timeoutMs uint32, kinds ...string) *registryv1.EventsWaitResponse {
	t.Helper()
	var got registryv1.EventsWaitResponse
	if err := c.Call(ctx5(t), "rig.events.wait", &registryv1.EventsWaitRequest{
		Kinds: kinds, After: after, TimeoutMs: timeoutMs,
	}, &got); err != nil {
		t.Fatal(err)
	}
	return &got
}

func signal(t *testing.T, c *client.Client, req *registryv1.EventsPublishRequest) *registryv1.Event {
	t.Helper()
	var pub registryv1.EventsPublishResponse
	if err := c.Call(ctx5(t), "rig.events.publish", req, &pub); err != nil {
		t.Fatal(err)
	}
	return pub.GetEvent()
}

// The sender is the seat the daemon knows, and a parked peer wakes with it.
// A terminal is attributed too, as its terminal seat.
func TestASeatsSignalWakesAPeer(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	a, b := seated(t, sock, "seat-a"), seated(t, sock, "seat-b")
	start := waitOn(t, b, 0, 1, "signal.*").GetLatest()

	got := make(chan *registryv1.EventsWaitResponse, 1)
	go func() { got <- waitOn(t, b, start, 5000, "signal.tests.*") }()
	signal(t, a, &registryv1.EventsPublishRequest{Kind: "signal.tests.green", PayloadJson: `{"run":3}`})

	evs := (<-got).GetEvents()
	if len(evs) != 1 || evs[0].GetSource() != "seat:seat-a" || evs[0].GetPayloadJson() != `{"run":3}` {
		t.Fatalf("seat-b woke with %v", evs)
	}
	if ev := signal(t, dial(t, sock), &registryv1.EventsPublishRequest{Kind: "signal.x"}); !strings.HasPrefix(ev.GetSource(), "seat:terminal:") {
		t.Fatalf("a terminal's signal came from %q", ev.GetSource())
	}
}

// to_seat is that seat's alone: a third seat on the same pattern never sees
// it, and nor does a caller with no seat.
func TestAnAddressedSignalIsSeenByItsSeatAlone(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	a, b, c := seated(t, sock, "seat-a"), seated(t, sock, "seat-b"), seated(t, sock, "seat-c")
	anon := dial(t, sock)
	ev := signal(t, a, &registryv1.EventsPublishRequest{Kind: "signal.handoff", ToSeat: "seat-b"})

	if got := waitOn(t, b, 0, 1000, "signal.*").GetEvents(); len(got) != 1 || got[0].GetSeq() != ev.GetSeq() {
		t.Fatalf("the addressee got %v", got)
	}
	// Red control: the same wait from anyone else answers empty at its
	// timeout, with the cursor past the signal it was not shown.
	for name, who := range map[string]*client.Client{"seat-c": c, "no seat": anon} {
		got := waitOn(t, who, 0, 100, "signal.*")
		if len(got.GetEvents()) != 0 || got.GetLatest() < ev.GetSeq() {
			t.Fatalf("%s saw %v (latest %d)", name, got.GetEvents(), got.GetLatest())
		}
	}
}

func TestASignalIsRefusedWhenItCannotBeAttributedOrIsMisshapen(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	a := seated(t, sock, "seat-a")
	for _, r := range []struct {
		who  *client.Client
		req  *registryv1.EventsPublishRequest
		code rigv1.Code
		why  string
	}{
		{eventProgram(t, sock, "graft"), &registryv1.EventsPublishRequest{Kind: "signal.tests.green"}, rigv1.Code_CODE_DENIED, "a program, which has no seat"},
		{a, &registryv1.EventsPublishRequest{Kind: "signal"}, rigv1.Code_CODE_INVALID, "no words"},
		{a, &registryv1.EventsPublishRequest{Kind: "signal.*"}, rigv1.Code_CODE_INVALID, "a pattern"},
		{a, &registryv1.EventsPublishRequest{Kind: "signal.Tests"}, rigv1.Code_CODE_INVALID, "upper case"},
		{a, &registryv1.EventsPublishRequest{Kind: "hand.changed", ToSeat: "seat-b"}, rigv1.Code_CODE_INVALID, "to_seat off a signal"},
		{a, &registryv1.EventsPublishRequest{Kind: "signal.x", ToSeat: "../b"}, rigv1.Code_CODE_INVALID, "a bad seat"},
		{a, &registryv1.EventsPublishRequest{Kind: "signal.x", PayloadJson: "{"}, rigv1.Code_CODE_INVALID, "bad JSON"},
	} {
		err := r.who.Call(ctx5(t), "rig.events.publish", r.req, &registryv1.EventsPublishResponse{})
		wantCode(t, err, r.code, r.why)
	}
}

// rig posts a seat arriving, a lease changing hands, and the seat leaving,
// in that order, to a waiter that asked for nothing else.
func TestRigPostsLeaseAndRosterChanges(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	watch := dial(t, sock)
	cursor := waitOn(t, watch, 0, 1, "lease.*").GetLatest()

	a := seated(t, sock, "seat-a")
	ctx := recordCtx(t)
	var h verbsv1.LeaseAcquireResponse
	if err := a.Call(ctx, "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "deploy", TtlMs: 60000}, &h); err != nil {
		t.Fatal(err)
	}
	if err := a.Call(ctx, "rig.lease.release", &verbsv1.LeaseReleaseRequest{
		Name: "deploy", Token: h.GetHandle().GetToken(), Epoch: h.GetHandle().GetEpoch(),
	}, &verbsv1.LeaseReleaseResponse{}); err != nil {
		t.Fatal(err)
	}
	_ = a.Close()

	var seen []string
	for len(seen) < 4 {
		got := waitOn(t, watch, cursor, 5000, "lease.*", "roster.*")
		if len(got.GetEvents()) == 0 {
			t.Fatalf("timed out after %v", seen)
		}
		for _, ev := range got.GetEvents() {
			var p struct{ Change, Seat, Holder, Name string }
			if err := json.Unmarshal([]byte(ev.GetPayloadJson()), &p); err != nil {
				t.Fatal(err)
			}
			seen = append(seen, ev.GetKind()+":"+p.Change+":"+p.Seat+p.Holder+p.Name)
			cursor = ev.GetSeq()
		}
	}
	want := []string{
		"roster.changed:announced:seat-a",
		"lease.changed:acquired:seat-adeploy",
		"lease.changed:released:deploy",
		"roster.changed:left:seat-a",
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw %v, want %v", seen, want)
		}
	}
}
