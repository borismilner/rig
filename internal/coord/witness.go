package coord

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Liveness is what rig OBSERVED about a witness, and Unknown is a first-class
// answer rather than a failure to produce one.
//
// THE ASYMMETRY IS THE WHOLE POINT. Reporting Alive when the holder is dead
// costs a wait. Reporting Dead when the holder is alive hands one resource to
// two actors, which is the failure the lease exists to prevent. So anything
// this package cannot establish is Unknown, and Unknown never frees a lease.
type Liveness int

const (
	// LivenessUnknown means rig could not establish the answer. An orphaned
	// lease whose witness is Unknown stays orphaned.
	LivenessUnknown Liveness = iota
	// Alive means the witness was observed running.
	Alive
	// Dead means the witness was observed gone. Only this frees an orphan.
	Dead
)

func (l Liveness) String() string {
	switch l {
	case Alive:
		return "alive"
	case Dead:
		return "dead"
	default:
		return "unknown"
	}
}

// WitnessKind names how a holder's liveness can be checked.
type WitnessKind string

const (
	// Unwitnessed is the literal section 16 requires be representable: a lease
	// whose holder rig cannot poll. It is not a missing witness, it is a
	// DECLARED one, and it buys a different expiry rule - an unwitnessed lease
	// never becomes free on its own and needs a recorded human break. AgentBox
	// already works this way and discarding it would be a regression.
	Unwitnessed WitnessKind = "unwitnessed"
	// PID is a process rig can poll on this machine.
	PID WitnessKind = "pid"
)

// Witness is the liveness handle recorded beside a lease.
//
// WHY A PID AND ITS START TIME, AND NOT A PIDFD. Section 16 offers "a pid or
// pidfd rig can poll, a cgroup, or the literal unwitnessed", and for THIS
// precondition the pid is not a compromise - it is the only one of them that
// works. A pidfd is an open descriptor: it dies with the process that opened
// it, so every witness in the estate would evaporate the moment rigd restarted,
// which is the exact event precondition 4 exists to survive. A pidfd is the
// better witness for a lease rig holds in memory and the worse one for a lease
// rig holds on disk.
//
// A BARE PID IS NOT ENOUGH EITHER, AND THE HOLE IS REACHABLE RATHER THAN
// THEORETICAL. Pids are recycled, so a holder that dies and is replaced in the
// table by an unrelated process reads as alive forever and its lease never
// frees. The start time from /proc/<pid>/stat closes it: the pair identifies
// one process for as long as the machine is up, and a reused pid has a later
// start time.
//
// THE BOOT ID CLOSES THE LAST ONE. A pid from before a reboot names nothing,
// and after a reboot it may well name something else entirely. A witness
// recorded against another boot is Dead without asking the kernel anything,
// because no process survives a reboot.
type Witness struct {
	Kind WitnessKind `json:"kind"`
	// PID is the process id, when Kind is PID.
	Pid int `json:"pid,omitempty"`
	// StartTicks is field 22 of /proc/<pid>/stat, the process start time in
	// clock ticks since boot. It is what makes Pid exact.
	StartTicks uint64 `json:"start_ticks,omitempty"`
	// BootID is the boot this pid was observed under.
	BootID string `json:"boot_id,omitempty"`
}

// NoWitness returns the unwitnessed literal.
func NoWitness() Witness { return Witness{Kind: Unwitnessed} }

// WitnessProcess records pid as a witness, reading its start time now.
//
// It fails when the process is already gone, which is deliberate: a lease
// acquired with a witness that was never alive would go straight to orphaned
// and then straight to free, and the caller would rather be told.
func WitnessProcess(pid int) (Witness, error) {
	if pid <= 0 {
		return Witness{}, fmt.Errorf("coord: %d is not a process id", pid)
	}
	ticks, err := startTicks(pid)
	if err != nil {
		return Witness{}, err
	}
	id, err := readBootID()
	if err != nil {
		return Witness{}, err
	}
	return Witness{Kind: PID, Pid: pid, StartTicks: ticks, BootID: id}, nil
}

// Describe renders the witness for a refusal message.
func (w Witness) Describe() string {
	if w.Kind == PID {
		return fmt.Sprintf("pid %d", w.Pid)
	}
	return string(Unwitnessed)
}

// Observe asks the kernel whether this witness is still running.
//
// currentBootID is passed rather than read so one evaluation pass over many
// leases reads /proc once, and so a test can move the machine across a reboot
// without a file in /proc changing under it.
func (w Witness) Observe(currentBootID string) Liveness {
	switch w.Kind {
	case Unwitnessed:
		// NEVER Dead. An unwitnessed lease is one rig was told it cannot
		// check, so reporting it dead would be inventing an observation.
		return LivenessUnknown
	case PID:
		if w.BootID != currentBootID {
			// No process survives a reboot. This needs no syscall and must
			// not make one: the pid may well be in use by something else now.
			return Dead
		}
		st, err := readStat(w.Pid)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return Dead
		case err != nil:
			// Could not tell. Unknown never frees a lease.
			return LivenessUnknown
		case st.ticks != w.StartTicks:
			// The pid was recycled: same number, different process.
			return Dead
		case st.state == zombie:
			// A ZOMBIE IS DEAD, and saying so is not a guess: the entry that
			// remains is the exit status, kept until a parent that is not
			// waiting collects it. Nothing else here reads the state field,
			// because no other value distinguishes a stopped or uninterruptible
			// process from a working one - but this one is the process having
			// already exited, which is exactly the question being asked.
			//
			// It only ever lasts while the holder's PARENT is alive and not
			// reaping; an orphan is reaped by init at once. So without this
			// the lease of a holder leaked by a live supervisor would sit
			// ORPHANED rather than FREE - safe, and needlessly unavailable.
			return Dead
		default:
			return Alive
		}
	default:
		return LivenessUnknown
	}
}

// zombie is /proc/<pid>/stat's state letter for a process that has exited and
// is waiting for its parent to collect the exit status.
const zombie = 'Z'

// procPath is where the process table is read from, a variable so tests can
// point it at a directory they build.
var procPath = "/proc"

// procStat is the part of /proc/<pid>/stat this package reads: the run state
// and the start time.
type procStat struct {
	state byte   // field 3: R, S, D, Z, T ...
	ticks uint64 // field 22: start time, in clock ticks since boot
}

// readStat reads fields 3 and 22 of /proc/<pid>/stat.
//
// FIELD 2 IS THE COMMAND AND IT CAN CONTAIN SPACES AND BRACKETS, so the fields
// cannot simply be split. It is wrapped in parentheses and is the only field
// that may be, so everything after the LAST ')' is safe to split - which is
// what every correct reader of this file does and what a naive one does not.
func readStat(pid int) (procStat, error) {
	b, err := os.ReadFile(fmt.Sprintf("%s/%d/stat", procPath, pid))
	if err != nil {
		return procStat{}, err
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return procStat{}, fmt.Errorf("coord: /proc/%d/stat has no comm field", pid)
	}
	// After the ')' the fields are state (3) onward, so field 22 is index 19.
	f := strings.Fields(string(b[i+1:]))
	const startTimeIndex = 19
	if len(f) <= startTimeIndex {
		return procStat{}, fmt.Errorf("coord: /proc/%d/stat has %d fields after comm, want at least %d",
			pid, len(f), startTimeIndex+1)
	}
	ticks, err := strconv.ParseUint(f[startTimeIndex], 10, 64)
	if err != nil {
		return procStat{}, fmt.Errorf("coord: /proc/%d/stat start time %q: %w", pid, f[startTimeIndex], err)
	}
	return procStat{state: f[0][0], ticks: ticks}, nil
}

// startTicks is readStat's start time alone, for taking a witness.
func startTicks(pid int) (uint64, error) {
	st, err := readStat(pid)
	return st.ticks, err
}
