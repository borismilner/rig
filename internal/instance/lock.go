// Package instance enforces exactly one rigd per user.
//
// PLAN.md section 5f: "Exactly one rigd per user, and it is enforced rather
// than assumed." rigd takes an exclusive flock on the pidfile BEFORE it binds,
// and a second instance exits naming the incumbent's pid rather than unlinking
// the socket and taking over.
//
// This is the mechanism section 16's entire argument rests on - "every
// coordination operation passes through a single serialisation point, which
// makes them linearizable by construction". Two daemons over one state tree
// give two serialisation points, two WALs and two lock namespaces, silently,
// and every property section 16 proves is false for as long as it lasts.
package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// HeldError means another process holds the lock. Inspect Incumbent for its pid.
type HeldError struct {
	Path      string
	Incumbent int // 0 when the pidfile could not be read or held no number
}

func (e *HeldError) Error() string {
	if e.Incumbent > 0 {
		return fmt.Sprintf("another rigd is running (pid %d, lock %s)", e.Incumbent, e.Path)
	}
	// The lock is what decides, not the file's contents: a pidfile that is
	// empty or unreadable while the lock is held still means "somebody is
	// there", and reporting no pid is better than reporting a wrong one.
	return fmt.Sprintf("another rigd is running (pid unknown, lock %s)", e.Path)
}

// Lock is a held single-instance claim. Release it with Close.
type Lock struct {
	f    *os.File
	path string
}

// Path is the pidfile this lock is held on.
func (l *Lock) Path() string { return l.path }

// Acquire takes the exclusive lock on path, creating it 0600.
//
// It returns *HeldError when another process already holds it. The lock lives on
// the open descriptor, so it is released by Close and by the process dying -
// including a SIGKILL, which is why this survives the `kill -9` loop in M6's
// demo without leaving a stale claim behind.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("instance: runtime dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("instance: open %s: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		// Read the incumbent's pid for the message, but never act on it: the
		// lock is the authority. Checking liveness by pid would race a
		// restart and could hand the estate to a second daemon.
		pid := readPID(f)
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, &HeldError{Path: path, Incumbent: pid}
		}
		return nil, fmt.Errorf("instance: flock %s: %w", path, err)
	}

	// Only now is it ours to write. Truncate first: a shorter pid must not
	// leave a longer one's trailing digits behind.
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("instance: truncate %s: %w", path, err)
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("instance: write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("instance: sync %s: %w", path, err)
	}
	return &Lock{f: f, path: path}, nil
}

// Close releases the lock.
//
// The pidfile is deliberately NOT unlinked. Unlinking races a second daemon
// that has the file open: it would hold a lock on an unlinked inode, believe
// it is alone, and a third would then create a new file and agree. The file
// is cheap to leave and its contents are never trusted.
func (l *Lock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}

func readPID(f *os.File) int {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	if n <= 0 {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return 0
	}
	return pid
}
