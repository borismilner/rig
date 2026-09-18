package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/boris-milner/rig/internal/kernel"
)

// ⛔ B86: A RECORD'S `session` IS THE DERIVED UNIX SESSION, NOT A FRESH RANDOM
// TOKEN PER WRITE. Generation 15 measured 783 sessions over 451 decisions and
// the 2026-09-18 re-seed wrote 380 requirement records under 380 DISTINCT
// values, with `seat` and `epoch` both constant - the one field claiming to
// group the act was the only one disagreeing with every row.
func TestARecordsSessionIsTheDerivedUnixSession(t *testing.T) {
	p := kernel.Principal{Token: "sess-deadbeef", UnixSession: 7254}
	if got, want := recordSession(p), "unix:7254"; got != want {
		t.Errorf("recordSession = %q, want %q", got, want)
	}
}

// ⛔ THE FALLBACK IS TODAY'S ANSWER AND NOT AN EMPTY FIELD. `peerPID` returns 0
// when SO_PEERCRED fails, so there is no unix session to read; a record with an
// EMPTY session would be a new silence where there used to be a useless value.
func TestAnUnreadableUnixSessionFallsBackToTheToken(t *testing.T) {
	for name, p := range map[string]kernel.Principal{
		"no pid was readable": {Token: "sess-deadbeef"},
		"a negative sid":      {Token: "sess-deadbeef", UnixSession: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if got, want := recordSession(p), "sess-deadbeef"; got != want {
				t.Errorf("recordSession = %q, want %q - an empty session field is a "+
					"new silence, not a fix", got, want)
			}
		})
	}
}

// ⛔ ZERO PID IS NOT SESSION ZERO, AND `Getsid(0)` IS THE TRAP. The syscall
// reads 0 as "my own session", which is the DAEMON's - a confident wrong answer
// about the caller, and one that would make every unreadable connection look
// like it shared a session with every other.
func TestAZeroPidYieldsNoUnixSessionRatherThanTheDaemonsOwn(t *testing.T) {
	mine, err := unix.Getsid(0)
	if err != nil {
		t.Skipf("Getsid(0): %v", err)
	}
	for _, pid := range []int{0, -1} {
		if got := unixSession(pid); got != 0 {
			t.Errorf("unixSession(%d) = %d, want 0; the daemon's own session is %d",
				pid, got, mine)
		}
	}
}

// ⛔ THE DERIVATION IS REAL AND NOT A CONSTANT. A test that only ever sees the
// zero path would pass with `unixSession` hard-wired to return 0, which is the
// shape this repository's mutation rule exists to catch.
func TestAReadablePidYieldsItsRealUnixSession(t *testing.T) {
	want, err := unix.Getsid(os.Getpid())
	if err != nil {
		t.Skipf("Getsid: %v", err)
	}
	if got := unixSession(os.Getpid()); got != want {
		t.Errorf("unixSession(self) = %d, want %d", got, want)
	}
	if want <= 0 {
		t.Fatalf("this process reports session %d, so the test proves nothing", want)
	}
}

// ⛔ THE REGISTRY'S TWO KEYS ARE UNTOUCHED, AND THIS IS THE ASSERTION B86's ROW
// WOULD HAVE BROKEN. Its row said the fix was `newPrincipal`'s `SessionID`.
// `Deregister` and the succession rule in `internal/kernel/registry.go` both
// key on `SessionID` naming exactly ONE CONNECTION; two shells in one unix
// session sharing it means closing one deregisters the other's programs.
func TestTwoConnectionsNeverShareASessionIdOrAToken(t *testing.T) {
	a, b := newPrincipal(nil), newPrincipal(nil)
	if a.SessionID == b.SessionID {
		t.Error("two connections share a SessionID; the registry keys succession " +
			"and Deregister on it naming exactly one connection")
	}
	if a.Token == b.Token {
		t.Error("two connections share a Token; section 5f's session token dies " +
			"with an occupancy and must not be derived from anything shared")
	}
	if a.ClientID == b.ClientID {
		t.Error("two connections share a ClientID")
	}
}

// ⛔ A REAL CONNECTION CARRIES A REAL UNIX SESSION, AND THIS TEST EXISTS
// BECAUSE THE OTHERS DID NOT CATCH IT. Every assertion above is satisfied by a
// `newPrincipal` that hard-wires `UnixSession: 0` - the fallback is correct,
// the zero-pid guard is correct, and `recordSession` is correct, so the whole
// derivation could be dead and the package would stay green. Measured: that
// mutation survived four tests.
//
// ⛔ THE SOCKET PATH IS SHORT ON PURPOSE. A unix socket path is bounded at 107
// bytes by the kernel, which reports the overrun only as EINVAL; a `t.TempDir()`
// under this project's scratchpad is already past it.
func TestAPrincipalFromARealSocketCarriesARealUnixSession(t *testing.T) {
	want, err := unix.Getsid(os.Getpid())
	if err != nil || want <= 0 {
		t.Skipf("Getsid: %v (sid %d)", err, want)
	}

	dir, err := os.MkdirTemp("/tmp", "rigsess")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		c, derr := net.Dial("unix", sock)
		if derr == nil {
			defer func() { _ = c.Close() }()
			<-time.After(time.Second)
		}
	}()

	nc, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer func() { _ = nc.Close() }()

	p := newPrincipal(nc)
	if p.PID != os.Getpid() {
		t.Fatalf("PID = %d, want this process %d - SO_PEERCRED did not answer, so "+
			"the rest of this test proves nothing", p.PID, os.Getpid())
	}
	if p.UnixSession != want {
		t.Errorf("UnixSession = %d, want %d - the derivation is not running on a "+
			"real connection", p.UnixSession, want)
	}
	if got := recordSession(p); got != "unix:"+strconv.Itoa(want) {
		t.Errorf("a record written on this connection would carry session %q", got)
	}
}
