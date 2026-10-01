package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/internal/paths"
)

// cmdMCP pumps bytes between this process's stdin/stdout and the daemon's MCP
// socket, so an agent host that can only spawn a child process can reach a
// surface that only listens on a unix socket.
//
// WHY A VERB RATHER THAN A SECOND BINARY: section 38's do-not-reinvent rule,
// plus the shape already running on this machine - `agentbox mcp` is one
// binary with an `mcp` subcommand and an agent host reaches it with
// command/args. rig's case is thinner than agentbox's, which re-implements an
// MCP server in the child: rigd already serves a complete MCP server on the
// socket, so this stays a pump. It reads only the JSON-RPC framing, to carry
// a session over a rigd restart (mcp_bridge.go), and answers nothing itself
// except an error for a call the lost daemon never answered.
//
// ⛔ IT MUST BE EXEC'D DIRECTLY AND NEVER WRAPPED IN A SHELL. This is not a
// preference and it is not inherited caution - it was measured on 2026-09-16
// with two arms of one probe. `sh -c 'exec socat ...'` dies with its process
// and the daemon logs the teardown. `sh -c 'socat ... & wait'` does not:
// killing the pid the agent host thinks is the server leaves the GRANDCHILD
// holding the socket, and the daemon logs NO TEARDOWN AT ALL. An occupant in
// rig lives exactly as long as its connection, so a wrapped bridge leaves a
// GHOST SITTING IN A SEAT - and the seat mechanism refuses an announce into a
// held seat, so the ghost locks out the session that replaces it. There is no
// reaper to clean it up, by design. Whoever changes the invocation reads this
// comment, which is why it is here and not only in the plan.
//
// STDOUT IS THE PROTOCOL STREAM, so nothing but the daemon's bytes may reach
// it. That is why --json is refused below even though section 10 promises it
// "on everything": the promise already has a stated exception for
// `completion`, which writes a shell script for eval, and this is the same
// exception for the same reason. Every diagnostic here goes to stderr.
func cmdMCP(args []string) error {
	if len(args) > 0 {
		if strings.HasPrefix(args[0], "-") {
			return badArgumentf(
				"mcp takes no flags, and --json in particular is refused: "+
					"stdout carries the MCP stream, so anything else written "+
					"there corrupts it (got %q)", args[0])
		}
		return badArgumentf("mcp takes no arguments (got %q)", args[0])
	}

	sock, err := paths.MCPSocket()
	if err != nil {
		return err
	}
	// BOUNDED, because every wait for a socket in this project is. A stale
	// socket file left by a killed daemon refuses immediately, so the bound
	// is not what usually ends this - but "bound every wait for a socket and
	// say what happened when the bound expires" is the rule, and an
	// unbounded dial is how a refusal presents as a hang.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", sock)
	if err != nil {
		return noDoor(sock, err)
	}

	dial := func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}
	return newBridge(os.Stdin, os.Stdout, dial, redialFor).run(c)
}

// noDoor is the cold-dial failure: nothing is listening on the MCP socket.
//
// It names the MCP socket rather than the program socket, which is what
// distinguishes it from noDaemon. The two are not interchangeable: rigd
// serves BOTH and a caller told the wrong path goes and checks the wrong
// thing. The socket FILE survives a SIGKILL, so `stat` is not a liveness
// test and the dial is - that is why this error can only be produced here,
// after an actual dial.
func noDoor(sock string, cause error) error {
	return &rigError{cause: cause, object: jsonStatus{
		Code:         codeNoDaemon,
		Message:      cause.Error(),
		Precondition: "the rig daemon is running and serving its MCP socket",
		Actual:       "nothing is listening on " + sock,
		Fix:          "start rigd, then run this again",
		FixCommand:   "rigd",
	}}
}

// lostDaemon is the mid-session failure, and it is the one socat cannot
// report.
//
// The cause is usually nil - a clean EOF - so the sentence has to carry the
// meaning on its own. It says what the host needs to distinguish: the server
// did not finish, it lost the thing it was a bridge to.
func lostDaemon(cause error) error {
	msg := "rig mcp: the daemon closed the MCP socket before the agent host " +
		"asked to stop; this session lost its daemon rather than finishing"
	if cause != nil {
		return fmt.Errorf("%s: %w", msg, cause)
	}
	return fmt.Errorf("%s", msg)
}
