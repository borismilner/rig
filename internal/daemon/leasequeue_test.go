package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/coord"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/53 slice 2: a lease you can queue for. What these prove is what an
// agent relied on AgentBox's acquire_lock for, over rig's wire.

type acquired struct {
	resp *verbsv1.LeaseAcquireResponse
	err  error
}

// acquireLater parks an acquire with wait_ms and answers on the channel.
func acquireLater(c *client.Client, name string, waitMs uint32) <-chan acquired {
	out := make(chan acquired, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(waitMs)*time.Millisecond+5*time.Second)
		defer cancel()
		var resp verbsv1.LeaseAcquireResponse
		err := c.Call(ctx, "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: name, TtlMs: 60_000, WaitMs: waitMs}, &resp)
		out <- acquired{&resp, err}
	}()
	return out
}

func takeLease(t *testing.T, c *client.Client, name string) *verbsv1.LeaseHandle {
	t.Helper()
	var resp verbsv1.LeaseAcquireResponse
	if err := c.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: name, TtlMs: 60_000}, &resp); err != nil {
		t.Fatalf("acquiring %s: %v", name, err)
	}
	return resp.GetHandle()
}

func giveBack(t *testing.T, c *client.Client, h *verbsv1.LeaseHandle) {
	t.Helper()
	if err := c.Call(recordCtx(t), "rig.lease.release", &verbsv1.LeaseReleaseRequest{
		Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
	}, &verbsv1.LeaseReleaseResponse{}); err != nil {
		t.Fatalf("releasing %s: %v", h.GetName(), err)
	}
}

// queuedOn waits until the lease shows n waiters, so a test orders its
// waiters for certain rather than by sleeping.
func queuedOn(t *testing.T, c *client.Client, name string, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := leaseList(recordCtx(t), t, c)[name].GetWaiting()
		if len(w) == n {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s has %v queued, want %d", name, w, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func notYet(t *testing.T, ch <-chan acquired, who string) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("%s was answered before its turn: %+v %v", who, got.resp, got.err)
	case <-time.After(100 * time.Millisecond):
	}
}

func granted(t *testing.T, ch <-chan acquired, who, because string) *verbsv1.LeaseHandle {
	t.Helper()
	select {
	case got := <-ch:
		if got.err != nil || got.resp.GetHandle() == nil {
			t.Fatalf("%s: %v %+v", who, got.err, got.resp)
		}
		if got.resp.GetGrantedBecause() != because {
			t.Fatalf("%s was granted because %q, want %q", who, got.resp.GetGrantedBecause(), because)
		}
		return got.resp.GetHandle()
	case <-time.After(5 * time.Second):
		t.Fatalf("%s was never granted", who)
	}
	return nil
}

// ⛔ FIRST COME, FIRST SERVED, AND A RELEASE IS A HAND-OVER: the lease goes
// to the queue's head, never to a newcomer who asks in between.
func TestQueuedAcquiresAreServedInOrder(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	b := seated(t, sock, "seat-b")
	c := seated(t, sock, "seat-c")
	newcomer := seated(t, sock, "seat-d")

	h := takeLease(t, a, "deploy")
	bWait := acquireLater(b, "deploy", 10_000)
	queuedOn(t, a, "deploy", 1)
	cWait := acquireLater(c, "deploy", 10_000)
	if got := queuedOn(t, a, "deploy", 2); got[0] != "seat-b" || got[1] != "seat-c" {
		t.Fatalf("queued %v, want seat-b then seat-c", got)
	}

	giveBack(t, a, h)
	hb := granted(t, bWait, "seat-b", grantReleased)
	if hb.GetHolder() != "seat-b" {
		t.Fatalf("handed to %q", hb.GetHolder())
	}
	notYet(t, cWait, "seat-c")

	err := newcomer.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "deploy", TtlMs: 60_000}, &verbsv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a newcomer while seat-c is queued")

	giveBack(t, b, hb)
	granted(t, cWait, "seat-c", grantReleased)
}

// ⛔ A WAIT THAT WOULD DEADLOCK IS REFUSED AT ONCE, naming the chain, instead
// of two agents sitting still until both time out.
func TestAWaitThatWouldDeadlockIsRefusedByName(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	b := seated(t, sock, "seat-b")
	ha := takeLease(t, a, "repo")
	hb := takeLease(t, b, "vm")

	aWait := acquireLater(a, "vm", 10_000)
	queuedOn(t, b, "vm", 1)

	err := b.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "repo", TtlMs: 60_000, WaitMs: 10_000}, &verbsv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "closing a cycle")
	if msg := err.Error(); !strings.Contains(msg, "deadlock") || !strings.Contains(msg, "seat-a waits on vm, held by seat-b") {
		t.Fatalf("the refusal does not name the chain: %s", msg)
	}

	giveBack(t, b, hb)
	granted(t, aWait, "seat-a", grantReleased)
	giveBack(t, a, ha)
}

// ⛔ A TIMEOUT IS A RESULT CARRYING THE WHOLE PICTURE, so the caller decides
// without asking again: who holds it, for what, doing what, for how long.
func TestATimedOutWaitSaysWhoHasItAndWhy(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	b := seated(t, sock, "seat-b")
	takeLease(t, a, "deploy")
	time.Sleep(20 * time.Millisecond)

	got := <-acquireLater(b, "deploy", 300)
	if got.err != nil {
		t.Fatalf("a timeout came back as an error: %v", got.err)
	}
	r := got.resp
	if !r.GetTimedOut() || r.GetHandle() != nil || r.GetWaitedMs() < 300 {
		t.Fatalf("got %+v, want timed_out after 300 ms with no handle", r)
	}
	in := r.GetIncumbent()
	if in.GetHolder() != "seat-a" || in.GetHolderPurpose() != "exercising the record verbs" ||
		in.GetHolderActivity() != "testing" || in.GetHeldMs() < 300 {
		t.Fatalf("the incumbent is %+v", in)
	}
	if len(in.GetWaiting()) != 0 {
		t.Fatalf("a waiter that timed out is still queued: %v", in.GetWaiting())
	}

	// And a refusal without a wait carries the same picture in its words.
	err := b.Call(recordCtx(t), "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{Name: "deploy", TtlMs: 60_000}, &verbsv1.LeaseAcquireResponse{})
	var ce *client.CallError
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a refusal without a wait")
	if !errors.As(err, &ce) || !strings.Contains(ce.Status.GetActual(), "for: exercising the record verbs") {
		t.Fatalf("the refusal does not say what the holder is for: %v", err)
	}
}

// ⛔ A WAITER THAT GOES AWAY GIVES UP ITS PLACE, so the head of a queue is
// never somebody who is not there.
func TestAWaiterThatLeavesIsNotHandedTheLease(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	a := seated(t, sock, "seat-a")
	gone := seated(t, sock, "seat-gone")
	c := seated(t, sock, "seat-c")
	h := takeLease(t, a, "deploy")

	_ = acquireLater(gone, "deploy", 10_000)
	queuedOn(t, a, "deploy", 1)
	cWait := acquireLater(c, "deploy", 10_000)
	queuedOn(t, a, "deploy", 2)
	_ = gone.Close()
	queuedOn(t, a, "deploy", 1)

	giveBack(t, a, h)
	granted(t, cWait, "seat-c", grantReleased)
}

// ⛔ A LEASE WHOSE HOLDER DIED FREES BY ITSELF AT ITS DEADLINE, and rig says
// so: the queue's head is granted it as expired, and lease.changed tells
// anybody watching. Without rig's clock it would wait for a read.
func TestADeadHoldersLeaseGoesToTheQueueAsExpired(t *testing.T) {
	sock, st := upLeaseDaemon(t)
	watch := dial(t, sock)
	cursor := waitOn(t, watch, 0, 1, "lease.*").GetLatest()

	holder := exec.Command("sleep", "30")
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	w, err := coord.WitnessProcess(holder.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Acquire("deploy", "seat-dead", w, 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	_ = holder.Process.Kill()
	_ = holder.Wait()
	b := seated(t, sock, "seat-b")
	bWait := acquireLater(b, "deploy", 10_000)

	granted(t, bWait, "seat-b", grantExpired)

	var seen []string
	for !strings.Contains(strings.Join(seen, " "), "acquired:seat-b") {
		got := waitOn(t, watch, cursor, 5000, "lease.*")
		if len(got.GetEvents()) == 0 {
			t.Fatalf("lease.changed said only %v", seen)
		}
		for _, ev := range got.GetEvents() {
			var p struct{ Change, Holder, Because string }
			_ = json.Unmarshal([]byte(ev.GetPayloadJson()), &p)
			seen = append(seen, p.Change+":"+p.Holder+":"+p.Because)
			cursor = ev.GetSeq()
		}
	}
	want := "queued:seat-dead: expired:seat-dead: acquired:seat-b:expired"
	if got := strings.Join(seen, " "); got != want {
		t.Fatalf("lease.changed said %q, want %q", got, want)
	}
}
