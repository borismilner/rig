package daemon

import (
	"context"
	"errors"
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
)

// Section 16's leases over the real wire. internal/coord's own tests prove the
// mechanism - two-step expiry, witness death, fencing across a restart. These
// prove what the wire adds: that the verbs are reachable, that the holder and
// the witness are the CONNECTION'S, and that coord's refusals arrive as
// codes a caller can branch on.

// upLeaseDaemon stands up a named estate with its lease store, the way rigd
// does: the store is opened once, outside the daemon, and handed in with its
// epoch.
func upLeaseDaemon(t *testing.T) (string, *coord.Store) {
	t.Helper()
	const estate = "leasewire"
	dir, err := os.MkdirTemp("", "rigl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	st, err := coord.Open(estate)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{
		Version: "test", Wire: "v1", Lock: lock, Estate: estate,
		Epoch: st.Epoch(), Leases: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if d.records != nil {
			_ = d.records.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock, st
}

func leaseList(ctx context.Context, t *testing.T, c *client.Client) map[string]*rigv1.Lease {
	t.Helper()
	var resp rigv1.LeaseListResponse
	if err := c.Call(ctx, "rig.lease.list", &rigv1.LeaseListRequest{}, &resp); err != nil {
		t.Fatalf("rig.lease.list: %v", err)
	}
	out := map[string]*rigv1.Lease{}
	for _, l := range resp.GetLeases() {
		out[l.GetName()] = l
	}
	return out
}

func wantCode(t *testing.T, err error, code rigv1.Code, what string) {
	t.Helper()
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("%s gave %v, want a %s refusal", what, err, code)
	}
	if ce.Code() != code {
		t.Fatalf("%s was refused with %s, want %s: %s", what, ce.Code(), code, ce.Status.GetMessage())
	}
}

// ⛔ THE HOLDER IS THE SEAT AND THE WITNESS IS THE CALLER'S OWN PID, and a
// second seat is refused with the incumbent named. The whole life of a lease
// - acquire, renew, a stale token fenced, release - over the socket.
func TestALeaseIsHeldBySeatAndWitnessedByTheCallersProcess(t *testing.T) {
	sock, st := upLeaseDaemon(t)
	ctx := recordCtx(t)
	a := seated(t, sock, "seat-a")
	b := seated(t, sock, "seat-b")

	var got rigv1.LeaseAcquireResponse
	if err := a.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{
		Name: "deploy", TtlMs: 60_000,
	}, &got); err != nil {
		t.Fatalf("rig.lease.acquire: %v", err)
	}
	h := got.GetHandle()
	if h.GetHolder() != "seat-a" || h.GetEpoch() != st.Epoch() || h.GetToken() == 0 {
		t.Fatalf("the handle is %+v, want holder seat-a on epoch %d with a token", h, st.Epoch())
	}
	if h.GetRemainingMs() <= 0 || h.GetRemainingMs() > 60_000 {
		t.Errorf("the handle says %d ms left of a 60000 ms lease", h.GetRemainingMs())
	}

	l := leaseList(ctx, t, a)["deploy"]
	if l.GetState() != rigv1.LeaseState_LEASE_STATE_HELD || l.GetHolder() != "seat-a" {
		t.Fatalf("the list shows %+v, want deploy HELD by seat-a", l)
	}
	if want := "pid " + strconv.Itoa(os.Getpid()); l.GetWitness() != want {
		t.Errorf("the lease is witnessed by %q, want %q: the witness is the "+
			"pid the socket reports for the caller", l.GetWitness(), want)
	}
	if l.GetLiveness() != rigv1.Liveness_LIVENESS_ALIVE || l.GetOwnerGone() {
		t.Errorf("a live holder reads as %s, owner_gone=%v", l.GetLiveness(), l.GetOwnerGone())
	}

	err := b.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{
		Name: "deploy", TtlMs: 60_000,
	}, &rigv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a second seat acquiring a held lease")

	if err := a.Call(ctx, "rig.lease.renew", &rigv1.LeaseRenewRequest{
		Name: "deploy", Token: h.GetToken(), Epoch: h.GetEpoch(), TtlMs: 60_000,
	}, &rigv1.LeaseRenewResponse{}); err != nil {
		t.Fatalf("renewing with the handle: %v", err)
	}
	err = a.Call(ctx, "rig.lease.renew", &rigv1.LeaseRenewRequest{
		Name: "deploy", Token: h.GetToken() + 1, Epoch: h.GetEpoch(), TtlMs: 60_000,
	}, &rigv1.LeaseRenewResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "renewing with a token that is not the current one")
	err = a.Call(ctx, "rig.lease.renew", &rigv1.LeaseRenewRequest{
		Name: "deploy", Token: h.GetToken(), Epoch: h.GetEpoch() + 1, TtlMs: 60_000,
	}, &rigv1.LeaseRenewResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "renewing with another epoch's handle")

	if err := a.Call(ctx, "rig.lease.release", &rigv1.LeaseReleaseRequest{
		Name: "deploy", Token: h.GetToken(), Epoch: h.GetEpoch(),
	}, &rigv1.LeaseReleaseResponse{}); err != nil {
		t.Fatalf("releasing: %v", err)
	}
	if l := leaseList(ctx, t, a)["deploy"]; l.GetState() != rigv1.LeaseState_LEASE_STATE_FREE {
		t.Fatalf("after release the lease is %s, want FREE", l.GetState())
	}
	err = a.Call(ctx, "rig.lease.release", &rigv1.LeaseReleaseRequest{
		Name: "deploy", Token: h.GetToken(), Epoch: h.GetEpoch(),
	}, &rigv1.LeaseReleaseResponse{})
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "releasing a free lease")
}

// ⛔ AN UNWITNESSED LEASE PAST ITS DEADLINE IS ORPHANED AND ASKS FOR A BREAK,
// and the break records the breaking SEAT and the reason. This is the
// two-step expiry reaching a reader: not free, and saying why.
func TestAnUnwitnessedOrphanNeedsARecordedBreak(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)
	a := seated(t, sock, "seat-a")
	b := seated(t, sock, "seat-b")

	if err := a.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{
		Name: "vm", TtlMs: 1, Unwitnessed: true,
	}, &rigv1.LeaseAcquireResponse{}); err != nil {
		t.Fatalf("rig.lease.acquire: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	l := leaseList(ctx, t, b)["vm"]
	if l.GetState() != rigv1.LeaseState_LEASE_STATE_ORPHANED || !l.GetNeedsBreak() {
		t.Fatalf("an unwitnessed lease past its deadline reads %+v, want "+
			"ORPHANED and needs_break", l)
	}
	if l.GetWitness() != "unwitnessed" || l.GetLiveness() != rigv1.Liveness_LIVENESS_UNKNOWN {
		t.Errorf("witness %q liveness %s, want unwitnessed and UNKNOWN", l.GetWitness(), l.GetLiveness())
	}
	if l.GetRemainingMs() >= 0 {
		t.Errorf("remaining_ms is %d for a lease past its deadline, want negative", l.GetRemainingMs())
	}

	err := b.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{
		Name: "vm", TtlMs: 60_000,
	}, &rigv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "acquiring an orphan")

	err = b.Call(ctx, "rig.lease.break", &rigv1.LeaseBreakRequest{Name: "vm"}, &rigv1.LeaseBreakResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a break with no reason")

	if err := b.Call(ctx, "rig.lease.break", &rigv1.LeaseBreakRequest{
		Name: "vm", Reason: "the holder's VM was torn down",
	}, &rigv1.LeaseBreakResponse{}); err != nil {
		t.Fatalf("rig.lease.break: %v", err)
	}
	l = leaseList(ctx, t, a)["vm"]
	if l.GetState() != rigv1.LeaseState_LEASE_STATE_FREE {
		t.Fatalf("after the break the lease is %s, want FREE", l.GetState())
	}
	if l.GetBrokenBy() != "seat-b" || l.GetBrokenReason() != "the holder's VM was torn down" {
		t.Errorf("the break recorded by=%q reason=%q, want seat-b and the reason given",
			l.GetBrokenBy(), l.GetBrokenReason())
	}
	if l.GetHolder() != "seat-a" {
		t.Errorf("a broken lease names holder %q, want the seat that abandoned it", l.GetHolder())
	}
}

// A connection with no seat cannot hold or break a lease, because the lease
// could not say who. A zero TTL is refused: there is no infinite hold.
func TestALeaseNeedsASeatAndATTL(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)

	// A program connection: scoped, with no terminal fallback seat.
	p := program(t, sock, "shelf")
	err := p.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{
		Name: "x", TtlMs: 1000,
	}, &rigv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "an acquire from a connection with no seat")

	a := seated(t, sock, "seat-a")
	err = a.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{Name: "x"}, &rigv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "an acquire with no TTL")

	long := strings.Repeat("n", maxLeaseText+1)
	err = a.Call(ctx, "rig.lease.acquire", &rigv1.LeaseAcquireRequest{Name: long, TtlMs: 1000}, &rigv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a lease name over the bound")
	err = a.Call(ctx, "rig.lease.break", &rigv1.LeaseBreakRequest{Name: "x", Reason: long}, &rigv1.LeaseBreakResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a break reason over the bound")
}

// An estate with no lease store refuses by name rather than answering empty.
func TestAnEstateWithNoLeaseStoreRefusesTheLeaseVerbs(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "seat-a")
	err := c.Call(recordCtx(t), "rig.lease.list", &rigv1.LeaseListRequest{}, &rigv1.LeaseListResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "rig.lease.list with no lease store")
}
