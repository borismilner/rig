package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// estate is a daemon running alone in a runtime directory of its own.
type estate struct {
	runtimeDir string
	socket     string
	daemon     *exec.Cmd
	programs   []*exec.Cmd
}

// start brings up a daemon and waits for its socket.
func start(ctx context.Context, binary string) (*estate, error) {
	dir, err := os.MkdirTemp("", "footprint-")
	if err != nil {
		return nil, fmt.Errorf("making a runtime directory: %w", err)
	}
	// XDG_RUNTIME_DIR is specified as 0700 and internal/paths reasons about
	// that, so the measurement runs against the same mode the real thing has.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("securing the runtime directory: %w", err)
	}

	e := &estate{
		runtimeDir: dir,
		socket:     filepath.Join(dir, "rig", "rigd.sock"),
	}
	e.daemon = exec.CommandContext(ctx, binary)
	e.daemon.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+dir)
	e.daemon.Stdout = os.Stderr
	e.daemon.Stderr = os.Stderr
	if err := e.daemon.Start(); err != nil {
		e.stop()
		return nil, fmt.Errorf("starting %s: %w", binary, err)
	}
	if err := e.waitForSocket(ctx); err != nil {
		e.stop()
		return nil, err
	}
	return e, nil
}

// waitForSocket blocks until the daemon is listening, rather than sleeping a
// guess: a guess that is too short measures a daemon that is still starting,
// which is exactly the moment its RSS and CPU are least representative.
func (e *estate) waitForSocket(ctx context.Context) error {
	for {
		if _, err := os.Stat(e.socket); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the daemon never created %s: %w", e.socket, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// pid is the daemon's process id.
func (e *estate) pid() int { return e.daemon.Process.Pid }

// addPrograms starts n more programs and waits for each to register.
func (e *estate) addPrograms(ctx context.Context, binary string, n int) error {
	for i := range n {
		// The name is positional rather than meaningful: these are load, not
		// programs anybody is pretending to be.
		name := fmt.Sprintf("load-%d", len(e.programs))
		cmd := exec.CommandContext(ctx, binary, "--name", name)
		cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+e.runtimeDir)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("starting program %d: %w", i, err)
		}
		e.programs = append(e.programs, cmd)
	}
	// Registration is a round trip after the process starts, so give the
	// batch a moment to land before anything is measured.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(n)*20*time.Millisecond + 500*time.Millisecond):
	}
	return nil
}

// stop kills everything this estate started and removes its directory.
func (e *estate) stop() {
	for _, p := range e.programs {
		if p.Process != nil {
			_ = p.Process.Kill()
			_ = p.Wait()
		}
	}
	if e.daemon != nil && e.daemon.Process != nil {
		_ = e.daemon.Process.Kill()
		_ = e.daemon.Wait()
	}
	if e.runtimeDir != "" {
		_ = os.RemoveAll(e.runtimeDir)
	}
}

// watch samples the daemon across a window and reports the rates.
func (e *estate) watch(ctx context.Context, d time.Duration) (window, error) {
	first, err := take(e.pid())
	if err != nil {
		return window{}, err
	}
	select {
	case <-ctx.Done():
		return window{}, ctx.Err()
	case <-time.After(d):
	}
	last, err := take(e.pid())
	if err != nil {
		return window{}, err
	}
	return between(first, last), nil
}
