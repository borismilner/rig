package coord

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// THE REAL KERNEL, NOT MY FIXTURE. Every other test in this file reads a
// /proc/<pid>/stat this package wrote itself, so all of them would still pass
// if the field offset were wrong and the fixture matched the mistake. This one
// reads the kernel's own table for a process whose pid is known to be alive.
func TestStartTicksReadsTheRealProc(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc on this machine")
	}
	ticks, err := startTicks(os.Getpid())
	if err != nil {
		t.Fatalf("the kernel's own stat line did not parse: %v", err)
	}
	if ticks == 0 {
		t.Fatal("start ticks read as 0; field 22 of /proc/<pid>/stat is nonzero " +
			"for every process the test runner can be")
	}

	// It is STABLE. A field read out of position would still be nonzero, so
	// "nonzero" alone proves little; the start time is the one number in that
	// line that cannot change while the process lives, and utime/stime either
	// side of it do change.
	again, err := startTicks(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if again != ticks {
		t.Fatalf("start ticks moved from %d to %d within one process; the field "+
			"being read is not the start time", ticks, again)
	}

	// And it AGREES WITH THE KERNEL'S OWN ANSWER for the same process, read a
	// different way. /proc/<pid>/stat field 22 is in clock ticks since boot and
	// so is the `starttime` the kernel reports for our parent's child - so the
	// cross-check is against a second process whose stat we also parse.
	boot, err := readBootID()
	if err != nil {
		t.Fatal(err)
	}
	w, err := WitnessProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if got := w.Observe(boot); got != Alive {
		t.Fatalf("this very process observes as %s", got)
	}
}

// A PID THAT DIED AND WAS REUSED IS DEAD, which is the whole reason the witness
// is not a bare pid. Linux wraps pids at /proc/sys/kernel/pid_max, so on a busy
// machine this is a matter of hours, not of bad luck.
func TestAReusedPidIsObservedDead(t *testing.T) {
	proc := fakeProc(t)
	fakeBoot(t, "boot-one")
	w := witnessFor(t, proc, 4242, 900)
	if got := w.Observe("boot-one"); got != Alive {
		t.Fatalf("a live witness observes as %s", got)
	}

	// The holder exits. The kernel walks round the pid space and hands 4242 to
	// something entirely unrelated, which is running RIGHT NOW.
	reap(t, proc, 4242)
	spawn(t, proc, 4242, 5150)

	if got := w.Observe("boot-one"); got != Dead {
		t.Fatalf("a reused pid observes as %s, want Dead: the start time is what "+
			"distinguishes the holder from its successor", got)
	}
}

// A PROCESS FROM A PREVIOUS BOOT IS DEAD WITHOUT A SYSCALL. Start ticks are
// counted from boot, so after a reboot they are a different origin and can
// collide by coincidence - the boot id is what closes that.
func TestAWitnessFromAnotherBootIsDeadWithoutLookingAtProc(t *testing.T) {
	proc := fakeProc(t)
	fakeBoot(t, "boot-one")
	w := witnessFor(t, proc, 4242, 900)

	// The pid AND the start ticks both match, exactly, in the new boot. Only
	// the boot id differs - so a witness that consulted /proc here would call
	// this process alive.
	if got := w.Observe("boot-two"); got != Dead {
		t.Fatalf("a witness from another boot observes as %s, want Dead", got)
	}

	// And it really did not consult /proc: the entry is present and matching.
	if _, err := os.Stat(filepath.Join(proc, "4242", "stat")); err != nil {
		t.Fatalf("the fixture is not testing what it claims: %v", err)
	}
}

// UNKNOWN IS A FIRST-CLASS ANSWER AND NEVER FREES A LEASE. An unreadable /proc
// is not evidence of death, and treating it as death is how two holders end up
// believing they hold the same lease.
func TestAnUnreadableProcIsUnknownRatherThanDead(t *testing.T) {
	proc := fakeProc(t)
	fakeBoot(t, "boot-one")
	w := witnessFor(t, proc, 4242, 900)

	// The stat file is there but cannot be read. This is the hardened-kernel
	// and container case, not a contrivance: hidepid=2 makes exactly this
	// shape for a process belonging to another user.
	stat := filepath.Join(proc, "4242", "stat")
	if err := os.Chmod(stat, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stat, 0o600) })
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can read it anyway")
	}

	if got := w.Observe("boot-one"); got != LivenessUnknown {
		t.Fatalf("an unreadable /proc entry observes as %s, want Unknown: "+
			"not knowing is not the same as knowing it is dead", got)
	}
}

// AN UNWITNESSED HOLDER IS NEVER OBSERVED DEAD. There is nothing to look at, so
// the answer is Unknown forever - which is what makes section 16's explicit
// human break the ONLY way such a lease is ever freed.
func TestAnUnwitnessedHolderIsAlwaysUnknown(t *testing.T) {
	fakeBoot(t, "boot-one")
	w := NoWitness()
	if got := w.Observe("boot-one"); got != LivenessUnknown {
		t.Fatalf("an unwitnessed holder observes as %s, want Unknown", got)
	}
	if got := w.Observe("boot-two"); got != LivenessUnknown {
		t.Fatalf("an unwitnessed holder observes as %s across a reboot, want "+
			"Unknown: there is no process to have not survived", got)
	}
	if !strings.Contains(w.Describe(), "unwitnessed") {
		t.Fatalf("Describe should say the holder is unwitnessed; got %q", w.Describe())
	}
}

// A PROCESS THIS TEST ACTUALLY KILLS. The fixtures above prove the decision
// logic; this proves the two ends meet - that a witness taken from a real
// process observes Alive while it runs and Dead once it does not.
func TestARealProcessIsWitnessedAliveThenDead(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc on this machine")
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep(1) to hold open")
	}
	cmd := exec.Command(sleep, "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	boot, err := readBootID()
	if err != nil {
		t.Fatal(err)
	}
	w, err := WitnessProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("no witness for a live child pid %d: %v", cmd.Process.Pid, err)
	}
	if w.StartTicks == 0 {
		t.Fatalf("no start ticks for a live child pid %d", cmd.Process.Pid)
	}
	if got := w.Observe(boot); got != Alive {
		t.Fatalf("a running child observes as %s, want Alive", got)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if _, err := cmd.Process.Wait(); err != nil {
		t.Fatal(err)
	}
	// Reaped, so the entry is gone rather than a zombie - which is the state a
	// killed child of this test process is in after Wait.
	if got := w.Observe(boot); got != Dead {
		t.Fatalf("a killed and reaped child observes as %s, want Dead", got)
	}
}

// A ZOMBIE IS DEAD. Its /proc entry is still there and its start ticks still
// match, so everything the witness compares says alive - the run state is the
// only field that does not, and it says the process has already exited.
func TestAZombieHolderIsObservedDead(t *testing.T) {
	proc := fakeProc(t)
	fakeBoot(t, "boot-one")
	w := witnessFor(t, proc, 4242, 900)
	if got := w.Observe("boot-one"); got != Alive {
		t.Fatalf("a running holder observes as %s", got)
	}

	// It exits. Its parent is alive and is not reaping, so the entry stays.
	spawnInState(t, proc, 4242, 900, "Z")
	if got := w.Observe("boot-one"); got != Dead {
		t.Fatalf("a zombie holder observes as %s, want Dead: the entry left "+
			"behind is its exit status, not the process", got)
	}
}

// AND A REAL ONE, KILLED AND DELIBERATELY NOT REAPED. The fixture above proves
// the comparison; this proves the letter the kernel actually writes is the
// letter this package looks for.
func TestARealZombieIsObservedDead(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("no /proc on this machine")
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep(1) to hold open")
	}
	cmd := exec.Command(sleep, "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if !reaped {
			_, _ = cmd.Process.Wait()
		}
	})
	boot, err := readBootID()
	if err != nil {
		t.Fatal(err)
	}
	w, err := WitnessProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	// SIGKILL is delivered asynchronously and this test deliberately never
	// calls Wait, so there is nothing to synchronise on but the state letter.
	deadline := time.Now().Add(5 * time.Second)
	var got Liveness
	for time.Now().Before(deadline) {
		if got = w.Observe(boot); got == Dead {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got != Dead {
		st, _ := readStat(cmd.Process.Pid)
		t.Fatalf("a killed and unreaped child observes as %s with state %q, want Dead",
			got, string(st.state))
	}
	_, _ = cmd.Process.Wait()
	reaped = true
}
