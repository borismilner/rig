package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/instance"
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
		"lease.changed:released:seat-adeploy",
		"roster.changed:left:seat-a",
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw %v, want %v", seen, want)
		}
	}
}

// serveStore runs a daemon over st until the returned stop is called, the
// way rigd does: one store, opened outside, its epoch handed in.
func serveStore(t *testing.T, st *coord.Store, dir string) (sock string, stop func()) {
	t.Helper()
	sock = filepath.Join(dir, "s"+strconv.FormatUint(st.Epoch(), 10))
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{Version: "test", Wire: "v1", Lock: lock, Estate: "sigwire", Epoch: st.Epoch(), Leases: st})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	return sock, func() {
		cancel()
		<-done
		_ = d.Close()
		_ = lock.Close()
	}
}

// plan/53 slice 1b: a signal outlives rigd. A cursor from before the
// restart finds exactly what came after it, with no gap on signal kinds,
// and a gap the moment the wait also names a kind only the ring held.
func TestASignalOutlivesARestart(t *testing.T) {
	dir, err := os.MkdirTemp("", "rigsig")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	st, err := coord.Open("sigwire")
	if err != nil {
		t.Fatal(err)
	}
	sock, stop := serveStore(t, st, dir)
	a := seated(t, sock, "seat-a")
	first := waitOn(t, a, 0, 1, "signal.*")
	cursor, oldEpoch := first.GetLatest(), first.GetEpoch()
	sent := signal(t, a, &registryv1.EventsPublishRequest{Kind: "signal.tests.green", PayloadJson: `{"run":9}`})
	private := signal(t, a, &registryv1.EventsPublishRequest{Kind: "signal.handoff", ToSeat: "seat-b"})
	_ = a.Close()
	stop()
	_ = st.Close()

	st, err = coord.Open("sigwire")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sock, stop = serveStore(t, st, dir)
	t.Cleanup(stop)
	if st.Epoch() == oldEpoch {
		t.Fatal("the restart did not move the epoch")
	}

	b, c := seated(t, sock, "seat-b"), seated(t, sock, "seat-c")
	ask := func(who *client.Client, kinds ...string) *registryv1.EventsWaitResponse {
		var got registryv1.EventsWaitResponse
		if err := who.Call(ctx5(t), "rig.events.wait", &registryv1.EventsWaitRequest{
			Kinds: kinds, After: cursor, Epoch: oldEpoch, TimeoutMs: 100,
		}, &got); err != nil {
			t.Fatal(err)
		}
		return &got
	}
	got := ask(b, "signal.*")
	if got.GetGap() || len(got.GetEvents()) != 2 ||
		got.GetEvents()[0].GetSeq() != sent.GetSeq() || got.GetEvents()[1].GetSeq() != private.GetSeq() {
		t.Fatalf("seat-b after the restart: gap %v, events %v", got.GetGap(), got.GetEvents())
	}
	if ev := got.GetEvents()[0]; ev.GetSource() != "seat:seat-a" || ev.GetPayloadJson() != `{"run":9}` {
		t.Fatalf("the signal came back as %v", ev)
	}
	// The addressee survives the restart too: seat-c sees the public one only.
	if got := ask(c, "signal.*"); len(got.GetEvents()) != 1 || got.GetEvents()[0].GetSeq() != sent.GetSeq() {
		t.Fatalf("seat-c after the restart saw %v", got.GetEvents())
	}
	// Red control: name a ring-only kind beside it and the restart is a gap.
	if got := ask(b, "signal.*", "lease.*"); !got.GetGap() {
		t.Fatal("a wait naming lease.* across a restart was not told of the gap")
	}
	// And the seqs of the new run order after the old cursor.
	if now := waitOn(t, b, 0, 1, "signal.*").GetLatest(); now <= private.GetSeq() {
		t.Fatalf("the new run's latest %d is not after the old run's %d", now, private.GetSeq())
	}
}

// An answer stays inside the frame, and where it stops, latest is the last
// seq it carries, so the next call picks up the rest.
func TestAWaitAnswerIsBoundedByBytesAndResumesWhereItStopped(t *testing.T) {
	sock, d := upDaemon(t, nil)
	big := `"` + strings.Repeat("x", maxEventPayload-2) + `"`
	for range 100 {
		d.events.publish("toast.posted", eventSourceRig, big)
	}
	c := dial(t, sock)
	seen, after := 0, uint64(0)
	for seen < 100 {
		got := waitOn(t, c, after, 100, "toast.*")
		if n := len(got.GetEvents()); n == 0 || n == 100 && seen == 0 {
			t.Fatalf("an answer carried %d of 100 16 KiB events", n)
		}
		seen += len(got.GetEvents())
		after = got.GetLatest()
		if last := got.GetEvents()[len(got.GetEvents())-1].GetSeq(); last != after {
			t.Fatalf("latest %d is not the last seq carried, %d", after, last)
		}
	}
}

// A first call is handed this run's signals only, never an earlier run's
// backlog; an exact kind is read from the store as a prefix is.
func TestAFirstWaitIsNotHandedAnEarlierRunsSignals(t *testing.T) {
	dir, err := os.MkdirTemp("", "rigsig")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	st, err := coord.Open("sigwire")
	if err != nil {
		t.Fatal(err)
	}
	sock, stop := serveStore(t, st, dir)
	signal(t, seated(t, sock, "seat-a"), &registryv1.EventsPublishRequest{Kind: "signal.old"})
	stop()
	_ = st.Close()

	st, err = coord.Open("sigwire")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sock, stop = serveStore(t, st, dir)
	t.Cleanup(stop)
	b := seated(t, sock, "seat-b")
	now := signal(t, b, &registryv1.EventsPublishRequest{Kind: "signal.new"})
	for _, kinds := range [][]string{{"signal.*"}, {"signal.new", "signal.old"}} {
		got := waitOn(t, b, 0, 100, kinds...)
		if len(got.GetEvents()) != 1 || got.GetEvents()[0].GetSeq() != now.GetSeq() {
			t.Fatalf("a first wait on %v got %v", kinds, got.GetEvents())
		}
	}
}

// A publish says how many waits it reached, counting only those that may
// see it: an addressed signal does not count a third seat's wait.
func TestAPublishSaysHowManyWaitsItReached(t *testing.T) {
	sock, d := upDaemon(t, nil)
	a, b := seated(t, sock, "seat-a"), seated(t, sock, "seat-b")
	start := waitOn(t, b, 0, 1, "signal.*").GetLatest()
	done := make(chan struct{})
	go func() { defer close(done); waitOn(t, b, start, 5000, "signal.*") }()
	for {
		d.events.mu.Lock()
		n := len(d.events.parked)
		d.events.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	publish := func(req *registryv1.EventsPublishRequest) uint32 {
		var pub registryv1.EventsPublishResponse
		if err := a.Call(ctx5(t), "rig.events.publish", req, &pub); err != nil {
			t.Fatal(err)
		}
		return pub.GetDelivered()
	}
	if n := publish(&registryv1.EventsPublishRequest{Kind: "signal.x", ToSeat: "seat-c"}); n != 0 {
		t.Fatalf("a signal for seat-c reached %d", n)
	}
	if n := publish(&registryv1.EventsPublishRequest{Kind: "signal.x"}); n != 1 {
		t.Fatalf("a signal reached %d, want seat-b's one wait", n)
	}
	<-done
}
