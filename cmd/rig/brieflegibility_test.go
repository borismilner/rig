package main

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// What is left of this file is the probe, and it is here because the probe is
// left: briefStyleFor and briefStyle survived the brief's move out of rig
// (PLAN.md section 50, move 5) as the device rule every record table renders
// through. The rest of this file measured the brief's own sections and went
// with them; the write-up it came from is
// logbook/projects/rig/attacks/run-2026-09-17/S6-FINDINGS.md.

// ---- the probe: what the renderer is allowed to know about the device ------

// ⛔ THE ANSWER IS ONE IOCTL AND NOT A GUESS. `TIOCGWINSZ` fails with ENOTTY
// on a pipe, on /dev/null and on a regular file, and succeeds on a terminal
// carrying its size, so the same call answers both questions this renderer
// has: is anybody watching, and how wide.
//
// THE POSITIVE CONTROL IS A REAL PTY. Without it every assertion below is an
// absence, and an absence is also what a probe that always answers "no"
// produces - which is exactly what this returned before the repair.
func TestTheStyleProbeTellsATerminalFromEverythingElse(t *testing.T) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx on this machine, so the positive control "+
			"cannot run and the negatives below would prove nothing: %v", err)
	}
	defer pty.Close()
	if err := unix.IoctlSetWinsize(int(pty.Fd()), unix.TIOCSWINSZ,
		&unix.Winsize{Row: 50, Col: 97}); err != nil {
		t.Fatalf("the control pty would not take a size, so this test "+
			"proved NOTHING: %v", err)
	}

	if st := briefStyleFor(pty); st.Width != 97 || !st.Bold {
		t.Errorf("a 97-column terminal was read as %+v", st)
	}

	// Every way this process can be run without a terminal.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("no pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	file, err := os.CreateTemp(t.TempDir(), "brief")
	if err != nil {
		t.Fatalf("no temp file: %v", err)
	}
	defer file.Close()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("no %s: %v", os.DevNull, err)
	}
	defer devnull.Close()

	for name, f := range map[string]*os.File{
		"a pipe":        w,
		"a file":        file,
		os.DevNull:      devnull,
		"a closed file": mustClosed(t),
	} {
		if st := briefStyleFor(f); st != (briefStyle{}) {
			t.Errorf("%s was read as a terminal: %+v", name, st)
		}
	}
}

// mustClosed is a file handle that is already gone, which is what a caller
// holding stdout after it was closed under them has. The probe must answer
// "no terminal" rather than panicking on it.
func mustClosed(t *testing.T) *os.File {
	f, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatalf("no temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("could not close it: %v", err)
	}
	return f
}

// THE NO-COLOUR CONVENTION TURNS OFF THE ATTRIBUTE AND LEAVES THE WIDTH
// ALONE.
//
// They are two different facts about the device. Somebody who has set that
// variable has said what their terminal should print, not how wide it is, and
// a probe that collapsed the two would throw away the larger of the two
// repairs to honour the smaller.
//
// this repository spells.
//
//nolint:misspell // the variable's actual name is an identifier, not a word
func TestNoColorSuppressesTheAttributeAndKeepsTheWidth(t *testing.T) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx, so this proves nothing: %v", err)
	}
	defer pty.Close()
	if err := unix.IoctlSetWinsize(int(pty.Fd()), unix.TIOCSWINSZ,
		&unix.Winsize{Row: 50, Col: 97}); err != nil {
		t.Fatalf("the control pty would not take a size: %v", err)
	}

	// The control: unset, this same handle is bold.
	t.Setenv("NO_COLOR", "")
	if st := briefStyleFor(pty); !st.Bold {
		t.Fatalf("with NO_COLOR empty the terminal is not emphasised, so the "+
			"assertion below cannot tell the variable from the default: %+v", st)
	}

	t.Setenv("NO_COLOR", "1")
	st := briefStyleFor(pty)
	if st.Bold {
		t.Errorf("NO_COLOR is set and the renderer still emphasises: %+v", st)
	}
	if st.Width != 97 {
		t.Errorf("NO_COLOR took the terminal's width with it: %+v", st)
	}
}
