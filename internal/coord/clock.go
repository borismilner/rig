package coord

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Instant is a point on CLOCK_BOOTTIME, in nanoseconds since boot.
//
// EVERY DEADLINE IN THIS PACKAGE IS ABSOLUTE AND ON CLOCK_BOOTTIME (PLAN.md
// section 16), and the three rejected alternatives each fail on this machine
// rather than in theory:
//
//   - A REMAINING TTL is silently extended by a restart. The daemon comes back,
//     reads "45 seconds left", and a lease that should have expired during the
//     outage gets its full TTL again. Precondition 4 exists to make a restart
//     visible, so storing the one quantity a restart erases would defeat it.
//   - THE CLIENT'S CLOCK is a number rig cannot check.
//   - WALL TIME moves when a timezone does, and goes backwards on an NTP step.
//
// And MONOTONIC is not enough, which is the part that is specific to this
// machine. Go's monotonic clock does not advance across suspend and this laptop
// suspends nightly, so two readings of the same deadline differ by hours
// depending on whether rigd restarted across the suspend. BOOTTIME does
// advance across suspend, so a lease taken before a suspend is correctly
// expired after it.
type Instant int64

// Add returns the instant d after i.
func (i Instant) Add(d time.Duration) Instant { return i + Instant(d) }

// Before reports whether i is earlier than j.
func (i Instant) Before(j Instant) bool { return i < j }

// Sub returns the duration from j to i.
func (i Instant) Sub(j Instant) time.Duration { return time.Duration(i - j) }

// now reads CLOCK_BOOTTIME.
//
// It is a variable so tests can drive the clock. Nothing in this package reads
// the wall clock at all, which is what makes an injected clock total rather
// than partial - section 20 mandates testing/synctest for timing, and synctest
// cannot model a suspend, so this class needs a real clock it can move by hand.
var now = func() (Instant, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return 0, fmt.Errorf("coord: reading CLOCK_BOOTTIME: %w", err)
	}
	return Instant(ts.Nano()), nil
}

// Now is the clock every deadline here is on, for a caller turning one into a
// distance. A deadline read against any other clock is a number about nothing.
func Now() (Instant, error) { return now() }

// bootIDPath is the kernel's per-boot identifier, and it is a variable only so
// tests can point it at a file they control.
var bootIDPath = "/proc/sys/kernel/random/boot_id"

// readBootID returns the identifier of the current boot.
//
// IT IS WHAT MAKES A BOOTTIME DEADLINE MEAN ANYTHING. BOOTTIME restarts at zero
// on every boot, so "deadline 4000000000" is 4 seconds after THIS boot and a
// completely different moment after the last one. A deadline is therefore
// stored as a pair - boot id and boottime - and a deadline whose boot id is not
// the current one is not late, it is MEANINGLESS, which is a different fact and
// has a different consequence: every witness recorded against that boot is dead
// by construction, because no process survives a reboot.
func readBootID() (string, error) {
	b, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", fmt.Errorf("coord: reading the boot id from %s: %w\n"+
			"       every lease deadline is stored against a boot id, because "+
			"CLOCK_BOOTTIME restarts at zero on each boot", bootIDPath, err)
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return "", fmt.Errorf("coord: the boot id at %s is empty", bootIDPath)
	}
	return id, nil
}
