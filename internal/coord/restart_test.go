package coord

import (
	"errors"
	"strings"
	"testing"
)

// SECTION 37 PRECONDITION 4, THE HEADLINE: a restart is SURVIVABLE. The
// coordination state is still there afterwards - the whole reason this is on
// disk rather than in a map.
func TestLeasesSurviveARestart(t *testing.T) {
	name := estate(t, "development")
	fakeBoot(t, "boot-one")
	clock := fakeClock(t)
	proc := fakeProc(t)
	w := witnessFor(t, proc, 101, 7)

	first, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Acquire("deploy", "seat-a", w, ttl); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	clock.advance(ttl / 2) // the outage is shorter than the lease

	second, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()

	st, err := second.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Held || st.Holder != "seat-a" {
		t.Fatalf("after a restart the lease is %s held by %q, want HELD seat-a: "+
			"coordination state survives a daemon restart", st.State, st.Holder)
	}
	// AND A SECOND HOLDER IS STILL REFUSED. A restart that quietly forgot who
	// held what would look identical to this test until somebody else asked.
	if _, err := second.Acquire("deploy", "seat-b", witnessFor(t, proc, 202, 9), ttl); err == nil {
		t.Fatal("the restart handed seat-a's live lease to seat-b")
	}
}

// AND IT IS DISTINGUISHABLE FROM A BLIP: every handle issued before the restart
// is fenced by the epoch, whatever its token says.
func TestAHandleFromBeforeTheRestartIsFencedByTheEpoch(t *testing.T) {
	name := estate(t, "development")
	fakeBoot(t, "boot-one")
	clock := fakeClock(t)
	proc := fakeProc(t)
	w := witnessFor(t, proc, 101, 7)

	first, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := first.Acquire("deploy", "seat-a", w, ttl)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	clock.advance(ttl / 4)

	second, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if second.Epoch() == h.Epoch {
		t.Fatal("the epoch did not move across a restart")
	}

	_, err = second.Renew(h, ttl)
	var fenced *FencedError
	if !errors.As(err, &fenced) || fenced.What != "epoch" {
		t.Fatalf("a handle from before the restart was accepted; got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "acquire again") {
		t.Fatalf("the refusal should tell the holder what to do; got: %v", err)
	}
	if err := second.Release(h); err == nil {
		t.Fatal("a handle from before the restart released the lease")
	}

	// THE RECONNECT PATH. The holder presents itself again and gets its work
	// back with a fresh epoch and a fresh token - "clients reconcile on
	// reconnect rather than losing their place".
	back, err := second.Acquire("deploy", "seat-a", w, ttl)
	if err != nil {
		t.Fatalf("the holder could not reconnect to its own live lease: %v", err)
	}
	if back.Epoch != second.Epoch() {
		t.Fatalf("the reissued handle carries epoch %d, want %d", back.Epoch, second.Epoch())
	}
	if back.Token <= h.Token {
		t.Fatalf("reconnecting reused token %d; the old handle inside that same "+
			"holder must still be fenced", back.Token)
	}
	if _, err := second.Renew(h, ttl); err == nil {
		t.Fatal("the pre-restart handle worked again after its holder reconnected")
	}
}

// A LEASE THAT EXPIRED WHILE RIGD WAS DOWN IS EXPIRED THE MOMENT ANYBODY LOOKS.
// There is no catch-up sweep to be late, and no timer for the restart to have
// stopped - which is the reason expiry is derived on read.
func TestALeaseThatExpiredDuringTheOutageIsExpiredOnTheNextRead(t *testing.T) {
	name := estate(t, "development")
	fakeBoot(t, "boot-one")
	clock := fakeClock(t)
	proc := fakeProc(t)

	first, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Acquire("deploy", "seat-a", witnessFor(t, proc, 101, 7), ttl); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	clock.advance(ttl * 5) // rigd was down for longer than the lease
	reap(t, proc, 101)     // and the holder did not survive either

	second, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	st, err := second.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free {
		t.Fatalf("a lease that expired during the outage reads as %s on the first "+
			"look after it, want FREE", st.State)
	}
}

// A REBOOT INVALIDATES EVERY DEADLINE AND EVERY WITNESS, and it must do so
// WITHOUT asking the kernel about a pid that now belongs to something else.
//
// This is the case a monotonic clock gets silently wrong: BOOTTIME restarts at
// zero, so a stored deadline of "one hour in" is in the PAST for the first hour
// after every reboot and in the FUTURE after that.
func TestAllLeasesAreFreeAfterAReboot(t *testing.T) {
	name := estate(t, "development")
	reboot := fakeBoot(t, "boot-one")
	clock := fakeClock(t)
	proc := fakeProc(t)

	first, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Acquire("deploy", "seat-a", witnessFor(t, proc, 101, 7), ttl); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// The machine reboots. BOOTTIME goes back to near zero and pid 101 is
	// handed to something unrelated, which is STILL PRESENT in the table.
	reboot("boot-two")
	clock.at = Instant(1_000_000)
	spawn(t, proc, 101, 7) // same pid, same start ticks, different boot

	second, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if !second.Rebooted() {
		t.Fatal("the store did not notice the reboot")
	}
	st, err := second.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != Free {
		t.Fatalf("a lease from before a reboot is %s, want FREE: no process "+
			"survives a reboot, so its witness is dead by construction", st.State)
	}
	if _, err := second.Acquire("deploy", "seat-b", witnessFor(t, proc, 202, 9), ttl); err != nil {
		t.Fatalf("a lease from before a reboot was not grantable: %v", err)
	}
}
