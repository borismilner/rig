package coord

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const ttl = 30 * time.Second

// setup gives a test a store, a clock and a process table.
func setup(t *testing.T) (*Store, *testClock, string) {
	t.Helper()
	name := estate(t, "development")
	fakeBoot(t, "boot-one")
	clock := fakeClock(t)
	proc := fakeProc(t)
	return openStore(t, name), clock, proc
}

func witnessFor(t *testing.T, proc string, pid int, ticks uint64) Witness {
	t.Helper()
	spawn(t, proc, pid, ticks)
	w, err := WitnessProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestALeaseIsHeldRenewedAndReleased(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)

	h, err := s.Acquire("deploy", "seat-a", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	if h.Epoch != s.Epoch() {
		t.Fatalf("the handle carries epoch %d and the daemon published %d: "+
			"EVERY handle carries the epoch", h.Epoch, s.Epoch())
	}

	clock.advance(ttl / 2)
	h, err = s.Renew(h, ttl)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl / 2)
	st, err := s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Held {
		t.Fatalf("a renewed lease is %s, want HELD", st.State)
	}

	if err := s.Release(h); err != nil {
		t.Fatal(err)
	}
	st, err = s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free {
		t.Fatalf("a released lease is %s, want FREE", st.State)
	}
}

func TestASecondHolderIsRefusedAndTheIncumbentIsNamed(t *testing.T) {
	s, _, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	if _, err := s.Acquire("deploy", "seat-a", w, ttl); err != nil {
		t.Fatal(err)
	}

	_, err := s.Acquire("deploy", "seat-b", witnessFor(t, proc, 202, 9), ttl)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("want *HeldError, got %T: %v", err, err)
	}
	if held.Status.Holder != "seat-a" {
		t.Fatalf("the refusal names %q as the holder, want seat-a", held.Status.Holder)
	}
	if !strings.Contains(err.Error(), "seat-a") || !strings.Contains(err.Error(), "pid 101") {
		t.Fatalf("the refusal should name the incumbent and its witness; got: %v", err)
	}
}

// THE CLAUSE THE FOUR-CLAUSE BAR IS ABOUT, AND THE ONE A TTL ALONE GETS WRONG.
//
// A holder stalls past its deadline and is STILL RUNNING. A plain TTL frees the
// lease and hands the resource to a second actor while the first is alive and
// about to write. Two-step expiry refuses: the lease is ORPHANED, not FREE.
func TestAStalledHolderKeepsItsLeaseBecauseTheWitnessIsAlive(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	if _, err := s.Acquire("deploy", "seat-a", w, ttl); err != nil {
		t.Fatal(err)
	}

	clock.advance(ttl * 10) // long past the deadline, and seat-a never renewed

	st, err := s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Orphaned {
		t.Fatalf("an expired lease whose holder is still running is %s, want ORPHANED: "+
			"a TTL cannot tell a dead holder from a slow one, and freeing it hands "+
			"one resource to two actors", st.State)
	}
	if st.OwnerGone {
		t.Fatal("owner_gone is true for a holder that is still running")
	}
	if st.Liveness != Alive {
		t.Fatalf("the witness was observed %s, want alive", st.Liveness)
	}

	_, err = s.Acquire("deploy", "seat-b", witnessFor(t, proc, 202, 9), ttl)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("a second holder TOOK an orphaned lease whose witness is alive; "+
			"got %v", err)
	}
	if !strings.Contains(err.Error(), "ORPHANED") || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("the refusal should say it is orphaned and that a stalled holder "+
			"still has it; got: %v", err)
	}
}

// ORPHANED IS NOT FREE, AND THAT IS WHY THE HOLDER GETS IT BACK. A stalled
// holder that wakes up is exactly who the middle state was kept for.
func TestAStalledHolderCanRenewItsOwnOrphanedLease(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	h, err := s.Acquire("deploy", "seat-a", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 10)

	h, err = s.Renew(h, ttl)
	if err != nil {
		t.Fatalf("a stalled holder could not recover its own orphaned lease: %v", err)
	}
	st, err := s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Held || st.Holder != "seat-a" || st.Token != h.Token {
		t.Fatalf("after recovery the lease is %s held by %q token %d, want HELD seat-a token %d",
			st.State, st.Holder, st.Token, h.Token)
	}
}

// ORPHANED becomes FREE only when the witness is OBSERVED DEAD.
func TestAnOrphanBecomesFreeWhenItsWitnessIsObservedDead(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	if _, err := s.Acquire("deploy", "seat-a", w, ttl); err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 2)
	reap(t, proc, 101)

	st, err := s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free {
		t.Fatalf("the lease is %s after its witness died, want FREE", st.State)
	}
	if _, err := s.Acquire("deploy", "seat-b", witnessFor(t, proc, 202, 9), ttl); err != nil {
		t.Fatalf("a dead holder's lease was not grantable: %v", err)
	}
}

// owner_gone is reported by a READ, which is what makes abandoned work visible
// rather than indistinguishable from healthy work.
func TestAReadReportsOwnerGone(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	if _, err := s.Acquire("deploy", "seat-a", w, ttl); err != nil {
		t.Fatal(err)
	}

	// Alive and inside the deadline.
	st, _ := s.Inspect("deploy")
	if st.OwnerGone {
		t.Fatal("owner_gone on a healthy lease")
	}

	// Dead, but still inside the deadline: the state is HELD until the
	// deadline passes, and the liveness is what a reader watches.
	reap(t, proc, 101)
	clock.advance(ttl * 2)
	st, _ = s.Inspect("deploy")
	if !st.OwnerGone {
		t.Fatalf("owner_gone is false for a lease whose holder is gone: state %s", st.State)
	}
}

// AN UNWITNESSED LEASE NEVER FREES ITSELF, and needs a recorded human break.
// AgentBox already works this way and discarding it would be a regression.
func TestAnUnwitnessedOrphanNeedsARecordedBreak(t *testing.T) {
	s, clock, _ := setup(t)
	if _, err := s.Acquire("deploy", "a-human", NoWitness(), ttl); err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 100)

	st, err := s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Orphaned || !st.NeedsBreak {
		t.Fatalf("an expired unwitnessed lease is %s needs_break=%v, want ORPHANED needs_break=true",
			st.State, st.NeedsBreak)
	}
	if st.OwnerGone {
		t.Fatal("owner_gone is true for a holder rig was told it cannot check: " +
			"that is an observation rig never made")
	}
	if _, err := s.Acquire("deploy", "seat-b", NoWitness(), ttl); err == nil {
		t.Fatal("an unwitnessed orphan was taken without a break")
	}

	// A break with no author and no reason is refused.
	if err := s.Break("deploy", "", "because"); err == nil {
		t.Fatal("a break with no author was accepted")
	}
	if err := s.Break("deploy", "boris", ""); err == nil {
		t.Fatal("a break with no reason was accepted")
	}

	if err := s.Break("deploy", "boris", "the laptop was closed for the night"); err != nil {
		t.Fatal(err)
	}
	st, err = s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free {
		t.Fatalf("the lease is %s after a break, want FREE", st.State)
	}
	if st.BrokenBy != "boris" || !strings.Contains(st.BrokenReason, "laptop") {
		t.Fatalf("the break did not survive: by=%q reason=%q", st.BrokenBy, st.BrokenReason)
	}
	if _, err := s.Acquire("deploy", "seat-b", NoWitness(), ttl); err != nil {
		t.Fatalf("a broken lease was not grantable: %v", err)
	}
}

// Breaking a HELD lease is taking a resource from a working holder, not a
// recovery.
func TestBreakingALiveLeaseIsRefused(t *testing.T) {
	s, _, proc := setup(t)
	if _, err := s.Acquire("deploy", "seat-a", witnessFor(t, proc, 101, 7), ttl); err != nil {
		t.Fatal(err)
	}
	err := s.Break("deploy", "boris", "I want it")
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("a live lease was broken; want *HeldError, got %T: %v", err, err)
	}
}

// THE CLASSIC BUG, END TO END: A stalls, its lease expires, A dies, B acquires,
// A wakes and writes anyway. The fencing token is what refuses A's write.
func TestAWokenHolderIsFencedByTheToken(t *testing.T) {
	s, clock, proc := setup(t)
	a, err := s.Acquire("deploy", "seat-a", witnessFor(t, proc, 101, 7), ttl)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 2)
	reap(t, proc, 101) // A is gone, so the lease is genuinely free

	b, err := s.Acquire("deploy", "seat-b", witnessFor(t, proc, 202, 9), ttl)
	if err != nil {
		t.Fatal(err)
	}
	if b.Token <= a.Token {
		t.Fatalf("the second grant's token %d is not above the first's %d: the "+
			"token is monotonic PER LEASE", b.Token, a.Token)
	}

	_, err = s.Renew(a, ttl)
	var fenced *FencedError
	if !errors.As(err, &fenced) || fenced.What != "token" {
		t.Fatalf("the first holder's handle was accepted after the lease moved on; "+
			"got %T: %v", err, err)
	}
	if err := s.Release(a); err == nil {
		t.Fatal("the first holder released the second holder's lease")
	}
}

// A RELEASE DOES NOT RESET THE TOKEN. Deleting the record would restart it at
// 1 and make an ancient handle match again.
func TestTheTokenIsMonotonicAcrossARelease(t *testing.T) {
	s, _, proc := setup(t)
	w := witnessFor(t, proc, 101, 7)
	first, err := s.Acquire("deploy", "seat-a", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Release(first); err != nil {
		t.Fatal(err)
	}
	second, err := s.Acquire("deploy", "seat-a", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token <= first.Token {
		t.Fatalf("token went %d -> %d across a release", first.Token, second.Token)
	}
}

// A FREED LEASE STILL SAYS WHO ABANDONED IT. owner_gone exists so abandoned
// work is visible rather than indistinguishable from healthy work (section 16),
// and a read that reports somebody is gone without saying who does not do that.
func TestAFreedLeaseStillNamesTheHolderThatDied(t *testing.T) {
	s, clock, proc := setup(t)
	if _, err := s.Acquire("deploy", "worker-a", witnessFor(t, proc, 101, 7), ttl); err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 2)
	reap(t, proc, 101)

	st, err := s.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free || !st.OwnerGone {
		t.Fatalf("a dead holder's lease is %s with owner_gone=%v, want FREE and true",
			st.State, st.OwnerGone)
	}
	if st.Holder != "worker-a" {
		t.Fatalf("the freed lease names holder %q, want worker-a: reporting that "+
			"SOMEBODY is gone does not make the abandoned work visible", st.Holder)
	}

	// A lease nobody has ever taken has no last holder to name, which is the
	// one case where the field is empty.
	fresh, err := s.Inspect("never-taken")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.State != Free || fresh.Holder != "" {
		t.Fatalf("an untaken lease reads %s holder %q, want FREE and empty",
			fresh.State, fresh.Holder)
	}
}

// INSPECT AND LEASES ARE THE SAME READ. They were not: Inspect blanked the
// holder of a free lease and Leases did not, so the same record described
// itself differently depending on which call asked.
func TestInspectAndLeasesAgreeAboutTheSameRecord(t *testing.T) {
	s, clock, proc := setup(t)
	if _, err := s.Acquire("held", "worker-a", witnessFor(t, proc, 101, 7), ttl); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Acquire("orphaned", "worker-b", NoWitness(), ttl); err != nil {
		t.Fatal(err)
	}
	gone, err := s.Acquire("freed", "worker-c", witnessFor(t, proc, 303, 11), ttl*4)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Release(gone); err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl * 2)

	all, err := s.Leases()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("Leases returned %d records, want 3", len(all))
	}
	for _, want := range all {
		got, err := s.Inspect(want.Name)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("the lease %q reads as\n  Inspect: %+v\n  Leases:  %+v",
				want.Name, got, want)
		}
	}
}

// A holder that dies INSIDE its deadline is visible at once, not a TTL later.
//
// The lease stays HELD, because the deadline is the contract and nobody else
// may take it yet. What must not happen is the read calling the holder alive:
// section 16 asks for the owner's liveness, and answering from the deadline
// instead of from /proc is an assumption wearing an observation's name.
func TestAHolderThatDiesInsideItsDeadlineIsVisibleAtOnce(t *testing.T) {
	s, clock, proc := setup(t)
	w := witnessFor(t, proc, 808, 12)

	if _, err := s.Acquire("build", "worker-a", w, ttl); err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Second)
	reap(t, proc, 808) // it dies with 29 of its 30 seconds left

	st, err := s.Inspect("build")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Held {
		t.Fatalf("the lease is %s inside its deadline, want HELD: the deadline "+
			"is the contract and a death does not shorten it", st.State)
	}
	if st.Liveness != Dead {
		t.Fatalf("the holder is gone from /proc and the read reports %s: nothing "+
			"looked", st.Liveness)
	}
	if !st.OwnerGone {
		t.Fatal("owner_gone is false for a holder observed dead, so abandoned " +
			"work is indistinguishable from healthy work for a whole TTL")
	}

	// Still nobody else's, and the refusal says why rather than hiding it.
	_, err = s.Acquire("build", "worker-b", NoWitness(), ttl)
	var held *HeldError
	if !errors.As(err, &held) {
		t.Fatalf("a second holder got %v, want a HeldError", err)
	}
	for _, want := range []string{"OBSERVED DEAD", "worker-a", "needs no break"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q:\n%s", want, err)
		}
	}

	// And it frees itself at the deadline, with no break and no sweeper.
	clock.advance(ttl)
	st, err = s.Inspect("build")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free {
		t.Fatalf("past the deadline with a dead witness the lease is %s, want FREE", st.State)
	}
}

// An unwitnessed lease reports UNKNOWN while it is held, never ALIVE.
//
// It is the declared literal from section 16: rig cannot poll this holder. A
// read that called it alive would be inventing the one answer this kind of
// witness exists to refuse to give.
func TestAnUnwitnessedHeldLeaseReportsUnknownRatherThanAlive(t *testing.T) {
	s, _, _ := setup(t)

	if _, err := s.Acquire("release", "a human at a terminal", NoWitness(), ttl); err != nil {
		t.Fatal(err)
	}
	st, err := s.Inspect("release")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Held {
		t.Fatalf("an unwitnessed lease inside its deadline is %s, want HELD", st.State)
	}
	if st.Liveness != LivenessUnknown {
		t.Fatalf("an unwitnessed holder is reported %s: rig cannot poll it, so "+
			"the only honest answer is unknown", st.Liveness)
	}
	if st.OwnerGone {
		t.Fatal("owner_gone is true for a holder rig never observed")
	}
}
