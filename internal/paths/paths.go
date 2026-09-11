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
	"errors"
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
		return "", errors.New("paths: XDG_RUNTIME_DIR is not set; rig will not " +
			"guess a runtime directory, because the socket's security depends on its mode and owner")
	}
	return filepath.Join(d, "rig"), nil
}

// Socket is the daemon's own socket path, which every ordinary client speaks.
func Socket() (string, error) { return socketNamed("rigd.sock") }

// MCPSocket is the MCP surface's own socket (PLAN.md sections 9, 13a, 14).
//
// A SECOND SOCKET, AND SECTION 14'S CALLER TABLE IS THE REASON RATHER THAN
// CONVENIENCE. The alternatives were ruled out on mechanism, not preference:
// stdio is one process per agent and rigd is a singleton, and folding MCP into
// the HTTP surface merges two rows the table exists to keep apart - an HTTP
// client gets "its bearer principal's scopes, never introspect", while MCP
// connecting as an unregistered socket client gets everything. A second socket
// is the only shape that keeps MCP a distinct caller row.
//
// It reaches exactly the set of processes the first socket reaches: same 0600
// mode, same 0700 runtime directory, same uid. So it widens nothing, and the
// isolation argument section 14 makes over the first socket is the same
// argument here.
func MCPSocket() (string, error) { return socketNamed("rigd-mcp.sock") }

// socketNamed joins a socket name to the runtime directory and refuses a path
// the kernel would reject.
//
// The length check is here rather than at each call site because the kernel
// reports this as a bare EINVAL, naming neither the path nor the limit, and a
// second socket is a second chance to spend an afternoon on it.
func socketNamed(name string) (string, error) {
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(d, name)
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
