package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"syscall"

	"github.com/boris-milner/rig/internal/kernel"
)

// newPrincipal decides who a connection is, at the moment it is made.
//
// Section 14: the connection itself carries the answer, and nothing is
// distributed, minted or inherited. A fresh connection is a client of the
// owner's and reads everything; it becomes a program, and scoped, only by
// completing the program handshake - which is the one transition below.
//
// Introspect starts true and that is deliberate rather than lax. One daemon
// per uid, and this uid is the owner's: section 14 says default deny exists
// so a program does not read another program's events by accident, "not
// because Boris should be kept out of his own machine". The population the
// isolation argument is stated over is programs and unprivileged clients, and
// a program is exactly what stops holding this.
//
// THE SESSION TOKEN IS MINTED HERE, AT ACCEPT, FOR EVERY CONNECTION, AND THAT
// PLACEMENT IS THE POINT RATHER THAN CONVENIENCE. Section 5f says "every
// connection carries a session token". Minting it on demand - when a caller
// gets round to asking - makes that sentence a thing clients opt into, and
// this repository already has that failure shipped: `request_id` is wire
// field 4, plumbed both directions since M0, never once set by any rig
// surface, and asserted as working in two places in PLAN.md. Minting at
// accept makes the sentence true AT THE DAEMON whatever any client does, so
// the only thing adoption can still get wrong is reading it.
//
// Both accept paths reach here - the main socket and the MCP socket - so no
// caller kind can be given a connection without one by someone adding a third
// listener and forgetting.
func newPrincipal(nc net.Conn) kernel.Principal {
	return kernel.Principal{
		UID:        os.Getuid(),
		Kind:       kernel.KindTerminal,
		ClientID:   randomID("c"),
		SessionID:  randomID("s"),
		PID:        peerPID(nc),
		Introspect: true,
		Token:      randomID("sess"),
	}
}

// asProgram is the one transition a principal makes, and registration is what
// makes it. It is never undone: a connection that registered is scoped for
// its whole life, so a program cannot shed the scope by asking again.
func asProgram(p kernel.Principal, id string) kernel.Principal {
	p.Kind = kernel.KindProgram
	p.ClientID = id
	p.Introspect = false
	return p
}

// peerPID reads the pid off the socket.
//
// It is carried because an elevation prompt has to name the caller's process
// (section 14): a question that says only "stop shelf?" is answered by
// whoever is looking at the screen. It is NOT an authorisation input - the
// 2026-09-10 attack's highest-ranked finding was SO_PEERCRED being read as
// one, which made every client an operator.
//
// Zero when it cannot be read, and a zero pid is reported as unknown rather
// than as pid 0.
func peerPID(nc net.Conn) int {
	uc, ok := nc.(*net.UnixConn)
	if !ok {
		return 0
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0
	}
	var pid int
	_ = raw.Control(func(fd uintptr) {
		cred, err := syscall.GetsockoptUcred(int(fd),
			syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err == nil {
			pid = int(cred.Pid)
		}
	})
	return pid
}

func randomID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on Linux, and an id that collides is
		// worse than a daemon that will not start, so this does not paper
		// over it with a counter.
		panic("daemon: crypto/rand: " + err.Error())
	}
	return prefix + "-" + hex.EncodeToString(b)
}
