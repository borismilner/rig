package coord

import (
	"errors"
	"testing"
)

// The happy path: push, claim oldest first, heartbeat with Renew, complete.
func TestATaskIsPushedClaimedHeartbeatAndCompleted(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	for _, k := range []string{"build-1", "build-2"} {
		if _, dup, err := s.Push("builds", k, []byte(`{"ref":"`+k+`"}`)); err != nil || dup {
			t.Fatalf("push %s: dup=%v err=%v", k, dup, err)
		}
	}

	task, h, err := s.Claim("builds", "worker-a", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	if task.Key != "build-1" || task.State != TaskClaimed || task.Attempts != 1 {
		t.Fatalf("claimed %+v, want build-1 CLAIMED on its first attempt", task)
	}

	clock.advance(ttl / 2)
	if h, err = s.Renew(h, ttl); err != nil {
		t.Fatalf("the heartbeat is lease.renew and it was refused: %v", err)
	}
	clock.advance(ttl / 2)

	done, err := s.Complete(h)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != TaskDone || done.DoneBy != "worker-a" {
		t.Fatalf("completed %+v, want DONE by worker-a", done)
	}

	left, nDone, err := s.Tasks("builds")
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Key != "build-2" || left[0].State != TaskReady || nDone != 1 {
		t.Fatalf("after one completion the queue is %+v with %d done", left, nDone)
	}
}

// ⛔ THE IDEMPOTENCY KEY IS MANDATORY, AND A KEY PUSHED AGAIN IS THE SAME TASK,
// even after that task is done. A key over different work is refused.
func TestTheIdempotencyKeyIsMandatoryAndDeduplicates(t *testing.T) {
	s, _, proc := setup(t)
	if _, _, err := s.Push("q", "", []byte("x")); err == nil {
		t.Fatal("a task without an idempotency key was accepted")
	}
	first, _, err := s.Push("q", "k", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	again, dup, err := s.Push("q", "k", []byte("x"))
	if err != nil || !dup || again.ID != first.ID {
		t.Fatalf("the same key pushed twice: dup=%v id=%s want %s err=%v", dup, again.ID, first.ID, err)
	}
	var conflict *KeyConflictError
	if _, _, err := s.Push("q", "k", []byte("y")); !errors.As(err, &conflict) {
		t.Fatalf("one key over different work was not refused: %v", err)
	}

	_, h, err := s.Claim("q", "w", witnessFor(t, proc, 101, 7), ttl)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(h); err != nil {
		t.Fatal(err)
	}
	after, dup, err := s.Push("q", "k", []byte("x"))
	if err != nil || !dup || after.State != TaskDone {
		t.Fatalf("a key pushed after its task finished must be the finished task, "+
			"not new work: %+v dup=%v err=%v", after, dup, err)
	}
	if _, _, err := s.Claim("q", "w", witnessFor(t, proc, 101, 7), ttl); !errors.As(err, new(*EmptyError)) {
		t.Fatalf("a done task was offered again: %v", err)
	}
}

// ⛔ REQUEUE IS THE TWO-STEP MACHINE. Past the deadline with the worker alive,
// the task is ORPHANED and not offered; once the worker is observed dead it is
// READY again and the next claim takes it.
func TestATaskIsRequeuedOnlyWhenItsWorkerIsObservedDead(t *testing.T) {
	s, clock, proc := setup(t)
	if _, _, err := s.Push("q", "k", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Claim("q", "worker-a", witnessFor(t, proc, 101, 7), ttl); err != nil {
		t.Fatal(err)
	}

	clock.advance(ttl * 3)
	var empty *EmptyError
	_, _, err := s.Claim("q", "worker-b", witnessFor(t, proc, 202, 9), ttl)
	if !errors.As(err, &empty) || empty.Orphaned != 1 {
		t.Fatalf("a task whose worker is only slow was handed to a second worker: %v", err)
	}
	tasks, _, _ := s.Tasks("q")
	if tasks[0].State != TaskOrphaned {
		t.Fatalf("the slow worker's task reads %s, want ORPHANED", tasks[0].State)
	}

	reap(t, proc, 101)
	task, _, err := s.Claim("q", "worker-b", witnessFor(t, proc, 202, 9), ttl)
	if err != nil {
		t.Fatalf("a dead worker's task was not requeued: %v", err)
	}
	if task.Attempts != 2 {
		t.Fatalf("the requeued task says %d attempts, want 2 so the consumer knows it may be a replay", task.Attempts)
	}
}

// ⛔ A CLAIMED TASK RUN TWICE, section 16's adversarial row: the stalled worker
// wakes after its task went to another and completes it. It is fenced by the
// token, and only the current claim completes.
func TestAStaleWorkerCannotCompleteATaskThatMovedOn(t *testing.T) {
	s, clock, proc := setup(t)
	if _, _, err := s.Push("q", "k", nil); err != nil {
		t.Fatal(err)
	}
	_, stale, err := s.Claim("q", "worker-a", witnessFor(t, proc, 101, 7), ttl)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 2)
	reap(t, proc, 101)
	_, current, err := s.Claim("q", "worker-b", witnessFor(t, proc, 202, 9), ttl)
	if err != nil {
		t.Fatal(err)
	}

	var fenced *FencedError
	if _, err := s.Complete(stale); !errors.As(err, &fenced) || fenced.What != "token" {
		t.Fatalf("the stale worker completed a task another worker holds: %v", err)
	}
	if _, err := s.Complete(current); err != nil {
		t.Fatalf("the current claim could not complete: %v", err)
	}
}

// A release without Complete gives the task back at once, and a second worker
// of the same seat in the same epoch is never handed the first one's claim.
func TestAReleasedClaimIsReadyAndASeatDoesNotFenceItself(t *testing.T) {
	s, _, proc := setup(t)
	for _, k := range []string{"a", "b"} {
		if _, _, err := s.Push("q", k, nil); err != nil {
			t.Fatal(err)
		}
	}
	w := witnessFor(t, proc, 101, 7)
	first, h1, err := s.Claim("q", "seat", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := s.Claim("q", "seat", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("one seat claimed the same task twice in one epoch, fencing its own first claim")
	}
	if err := s.Release(h1); err != nil {
		t.Fatal(err)
	}
	again, _, err := s.Claim("q", "other", witnessFor(t, proc, 202, 9), ttl)
	if err != nil || again.ID != first.ID {
		t.Fatalf("a released claim was not offered again: %+v %v", again, err)
	}
}

// After a daemon restart every handle is fenced by its epoch, and the worker
// gets its own claimed task back by claiming again.
func TestAWorkerReclaimsItsTaskAfterARestart(t *testing.T) {
	name := estate(t, "queues")
	fakeClock(t)
	fakeBoot(t)
	proc := fakeProc(t)
	s1, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s1.Push("q", "k", nil); err != nil {
		t.Fatal(err)
	}
	w := witnessFor(t, proc, 101, 7)
	_, old, err := s1.Claim("q", "seat", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()

	s2 := openStore(t, name)
	if _, err := s2.Complete(old); !errors.As(err, new(*FencedError)) {
		t.Fatalf("a handle from before the restart completed a task: %v", err)
	}
	if _, _, err := s2.Claim("q", "someone-else", witnessFor(t, proc, 202, 9), ttl); !errors.As(err, new(*EmptyError)) {
		t.Fatalf("a live worker's task went to another seat after a restart: %v", err)
	}
	task, h, err := s2.Claim("q", "seat", w, ttl)
	if err != nil || task.Key != "k" {
		t.Fatalf("the worker could not take its own task back: %+v %v", task, err)
	}
	if _, err := s2.Complete(h); err != nil {
		t.Fatal(err)
	}
}

func TestQueueBoundsAreRefused(t *testing.T) {
	s, _, _ := setup(t)
	for name, push := range map[string]func() error{
		"empty queue":   func() error { _, _, err := s.Push("", "k", nil); return err },
		"control char":  func() error { _, _, err := s.Push("a\x00b", "k", nil); return err },
		"long key":      func() error { _, _, err := s.Push("q", string(make([]byte, MaxTaskKey+1)), nil); return err },
		"large payload": func() error { _, _, err := s.Push("q", "k", make([]byte, MaxTaskPayload+1)); return err },
	} {
		if push() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, _, err := s.Claim("never-pushed", "w", NoWitness(), ttl); !errors.As(err, new(*EmptyError)) {
		t.Errorf("claiming from a queue nobody pushed to: %v, want empty", err)
	}
	if _, err := s.Complete(Handle{Name: "deploy", Epoch: s.Epoch()}); !errors.As(err, new(*NotClaimError)) {
		t.Errorf("completing a lease that is not a claim: %v", err)
	}
}

func TestAClaimNameRoundTripsAQueueWithASlash(t *testing.T) {
	q, seq, ok := parseClaim(claimName("team/builds", 42))
	if !ok || q != "team/builds" || seq != 42 {
		t.Fatalf("parsed %q %d %v", q, seq, ok)
	}
	for _, bad := range []string{"deploy", "queue/", "queue/q/", "queue/q/x", "queue//1"} {
		if _, _, ok := parseClaim(bad); ok {
			t.Errorf("%q parsed as a claim", bad)
		}
	}
}
