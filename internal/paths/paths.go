// Package paths resolves rig's runtime locations.
//
// One socket for the whole daemon at $XDG_RUNTIME_DIR/rig/rigd.sock, mode 0600,
// and the pidfile beside it (PLAN.md section 5f). There is deliberately no
// per-program path: a socket named after a program turns the runtime directory
// into a second copy of the registry that ls enumerates and stat polls for
// presence, and a mode bit is a uid instrument being asked to enforce a client
// boundary (section 14).
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// MaxSocketPath is the longest usable unix socket path.
//
// Linux's sockaddr_un.sun_path is 108 bytes including the terminating NUL, so
// 107 is the real ceiling. Exceeding it fails bind and connect with EINVAL -
// "invalid argument" - which names neither the path nor the limit, so the
// check is here and the error says what is wrong.
const MaxSocketPath = 107

// RuntimeDir is $XDG_RUNTIME_DIR/rig.
//
// XDG_RUNTIME_DIR is required, not defaulted. The spec says it is owned by the
// user, mode 0700 and cleared at logout; /tmp guarantees none of that, and a
// silent fallback would put the socket somewhere with different security
// properties than the ones section 14 reasons about.
func RuntimeDir() (string, error) {
	d := os.Getenv("XDG_RUNTIME_DIR")
	if d == "" {
		return "", fmt.Errorf("paths: XDG_RUNTIME_DIR is not set; rig will not " +
			"guess a runtime directory, because the socket's security depends on its mode and owner")
	}
	return filepath.Join(d, "rig"), nil
}

// Socket is the daemon's one socket path.
func Socket() (string, error) {
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(d, "rigd.sock")
	if len(p) > MaxSocketPath {
		return "", fmt.Errorf(
			"paths: socket path is %d bytes, over the %d-byte kernel limit: %s\n"+
				"       set XDG_RUNTIME_DIR to something shorter; the kernel reports "+
				"this only as EINVAL, which names neither the path nor the limit",
			len(p), MaxSocketPath, p)
	}
	return p, nil
}

// PIDFile is the single-instance lock path (section 5f).
func PIDFile() (string, error) {
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "rigd.pid"), nil
}
