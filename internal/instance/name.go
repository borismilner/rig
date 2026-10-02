package instance

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

// THE NAME CLAIM NO ENVIRONMENT CAN MOVE (section 37 precondition 6, reopened
// by decision 0259). The pidfile claim lives under XDG_STATE_HOME, and on
// 2026-10-02 rig-team.service, which sets a private one, ran beside
// rigd.service with both calling themselves production. So a named estate
// also binds an abstract unix socket keyed on its uid and name: one per user
// per machine, whatever the state home, root or runtime directory, and the
// kernel drops it when the process dies, SIGKILL included.

// claimSpace prefixes every abstract name. Only tests move it, through
// SetClaimSpace, so a test's "production" never meets the machine's.
var claimSpace = "rig-estate"

// SetClaimSpace moves the abstract names to space and returns the undo. It
// exists for tests, which name real estates in a process beside real daemons.
func SetClaimSpace(space string) (undo func()) {
	old := claimSpace
	claimSpace = space
	return func() { claimSpace = old }
}

func abstractName(name string) string {
	return "@" + claimSpace + "/" + strconv.Itoa(os.Getuid()) + "/" + name
}

// bindName holds the abstract name for name, or returns *NameHeldError naming
// the holder's pid, which the kernel reports rather than any file.
func bindName(name string) (*net.UnixListener, error) {
	addr := abstractName(name)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: addr, Net: "unix"})
	if err == nil {
		// Nobody is meant to talk to it; accepting and closing keeps the
		// backlog empty, so a refused second estate never waits on it.
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		return ln, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("instance: claim %s: %w", addr, err)
	}
	return nil, &NameHeldError{Name: name, Path: addr, Incumbent: holder(addr)}
}

// holder is the pid bound at addr, 0 when it cannot be read. Like readPID it
// is for the message only; the bind decided.
func holder(addr string) int {
	// addr is abstractName's, built from the uid and a validated name.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := new(net.Dialer).DialContext(ctx, "unix", addr)
	if err != nil {
		return 0
	}
	defer func() { _ = c.Close() }()
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0
	}
	var pid int
	_ = raw.Control(func(fd uintptr) {
		cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err == nil {
			pid = int(cred.Pid)
		}
	})
	return pid
}
