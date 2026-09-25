package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/rig/internal/supervise"
)

// The tray is a supervisor and the window is its child process (PLAN.md
// section 17, "the window, when closed: zero"). Section 11's scope call put
// the tray INTO the window process so the icon would be up whenever the
// graphical session is, and that quietly made the window's whole webview a
// permanent resident: measured 2026-09-24 on the installed unit, hidden and
// idle, 202 MB in the cgroup - the renderer alone 173 MB RSS - for an icon.
//
// BORIS, 2026-09-24, VERBATIM, AND IT IS THE RULING THIS FILE IMPLEMENTS:
// "We can't afford any unnecessary costs! The footprint of both AgentBox and
// rig must be absolutely minimal - you are allowed to do anything for this
// cause!"
//
// So: this process is the tray and nothing else. It links Wails because it is
// the same binary, but never calls into it, so no GTK, no webview and no
// renderer exist until somebody clicks. The click starts `rigwindow --window`;
// that process shows the window and EXITS when the window closes, which is
// what returns every byte - the kernel closes the renderer's IPC socket with
// the process and the sandboxed WebKit processes go with it (measured: 0 of 7
// alive four seconds after SIGTERM or SIGKILL, three trials). Requirements 1,
// 4 and 5 of section 11 hold as before: the icon is up whenever the process
// is, a close never touches the icon, and no window opens uninvited.

// stopGrace is how long a window process gets to leave after SIGTERM before
// it is killed. Wails installs no signal handler, so the default action ends
// the process at once; the grace only matters if that ever changes.
const stopGrace = 3 * time.Second

// windowProcess is one running window: the only thing the tray can do to it
// is ask it to go. How it went comes back on the spawn's exit channel.
type windowProcess interface {
	stop()
}

// spawnFunc starts one window process. exited closes when that process has
// ended, however it ended: a close, a crash, a stop.
type spawnFunc func() (proc windowProcess, exited <-chan struct{}, err error)

// supervisor is the tray's knowledge of the window: a process that exists
// while the window is on screen and does not otherwise.
type supervisor struct {
	spawn    spawnFunc
	onChange func() // after every change of open(); the menu retitles here
	warn     func(string)

	mu   sync.Mutex
	proc windowProcess // nil while no window process is alive
}

func newSupervisor(spawn spawnFunc, warn func(string)) *supervisor {
	return &supervisor{spawn: spawn, onChange: func() {}, warn: warn}
}

// open reports whether a window process is alive right now.
func (s *supervisor) open() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proc != nil
}

// toggle is the one gesture the tray offers: open the window if there is
// none, close it if there is. A second click while the window is still
// leaving asks it to leave again, which is harmless; the menu says "Close"
// until the process is actually gone, because that is the truth.
func (s *supervisor) toggle() {
	s.mu.Lock()
	if p := s.proc; p != nil {
		s.mu.Unlock()
		p.stop()
		return
	}
	proc, exited, err := s.spawn()
	if err != nil {
		s.mu.Unlock()
		// The tray stays. A window that will not start is a fact for the
		// journal, not a reason to lose the icon (section 11 requirement 1).
		s.warn("the window would not start: " + err.Error())
		return
	}
	s.proc = proc
	s.mu.Unlock()
	go func() {
		<-exited
		s.mu.Lock()
		if s.proc == proc {
			s.proc = nil
		}
		s.mu.Unlock()
		s.onChange()
	}()
	s.onChange()
}

// spawnWindow runs this same binary again with --window. Same binary on
// purpose: one install, one unit, one version stamp, and the child reads the
// same XDG_RUNTIME_DIR as the tray so it reaches the same estate.
func spawnWindow() (windowProcess, <-chan struct{}, error) {
	exe, err := selfExecutable()
	if err != nil {
		return nil, nil, err
	}
	//rig:allow nocontextfree: the window child lives until the user closes it, so its end is the exit channel rather than a deadline
	cmd := exec.CommandContext(context.Background(), exe, "--window")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	// The mechanism is internal/supervise's, shared with rigd: SIGTERM, then
	// SIGKILL after the grace, the reap and the exit. The command, and so the
	// environment, stay the tray's - StartCommand says why.
	proc, ended, err := supervise.StartCommand(cmd)
	if err != nil {
		return nil, nil, err
	}
	exited := make(chan struct{})
	go func() {
		if e := <-ended; !e.OK() {
			// A close is exit 0. Anything else is the window dying, and the
			// journal is where that is read.
			fmt.Fprintln(os.Stderr, "rigwindow: the window process ended: "+e.String())
		}
		close(exited)
	}()
	return windowChild{proc}, exited, nil
}

// windowChild is the shared Process behind the tray's one verb.
type windowChild struct{ supervise.Process }

func (w windowChild) stop() { w.Stop(stopGrace) }

// selfExecutable is the path this process runs from, made usable after an
// install replaced the file underneath it. /proc/self/exe then reads
// "<path> (deleted)": the path is still where the NEW binary sits, and
// starting that one is right - the screen should run what is installed
// (tools/restart-window.sh is about exactly this defect, from the other side).
func selfExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(exe, " (deleted)"), nil
}
