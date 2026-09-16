package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/boris-milner/rig/internal/mcpserver"
)

// THIS FILE EXISTS BECAUSE TWO MUTATIONS SURVIVED A GREEN GATE.
//
// The row 0 mutation matrix broke the front door's mechanism seven ways and
// watched which breakages a test noticed. Five bit. Two did not, and both
// survivors have the same root: every other MCP test builds its server with
// mcpserver.New and connects it over an in-memory transport, so the wiring
// BETWEEN the daemon and that server is never on the path.
//
//	MUTATION A - make Daemon.resyncMCP return immediately. The whole suite
//	stays green, because no test ever put a server into d.mcps for it to
//	iterate. resyncMCP, addMCP, removeMCP, the ServeMCP accept loop and
//	serveMCPConn's pre-first-request Sync had zero coverage between them.
//	TestPromotionFollowsTheEstate proves Sync reconciles when it is CALLED;
//	nothing proved rigd ever calls it.
//
//	MUTATION B - rename the CapabilityURI constant to anything else. Green,
//	because "rig://capabilities" appears exactly once in the repository and
//	every test reads the constant rather than the string. The one URI an
//	outside agent has to hard-code could be changed without a failure.
//
// So the tests below drive the REAL path: a real unix listener, ServeMCP's
// own accept loop, a real client dialling it, and the capability URI written
// out as a literal rather than referenced.
//
// The cost of the real path is the reason the other files do not pay it: it
// is a socket, two goroutines and a debounced notification per test. That is
// worth paying once, here, for the wiring, and not worth paying again in
// every test about what the surface ANSWERS.

// TestTheMCPSocketServesTheEstateAnAgentArrivesInTo is serveMCPConn's
// pre-first-request Sync, which is the half of promotion an agent notices
// first: the tools it already has when it opens the connection.
//
// The program registers BEFORE the agent connects, so nothing here depends on
// a notification. If the Sync in serveMCPConn were dropped, an agent's first
// tools/list would carry the four meta tools and nothing else, and promotion
// would appear to work only for programs that registered after it arrived.
func TestTheMCPSocketServesTheEstateAnAgentArrivesInTo(t *testing.T) {
	sock, d := upDaemon(t, nil)
	promoting(t, sock) // "shelf", with search promoted
	ctx := ctx5(t)

	// The agent's VERY FIRST request over the socket.
	tools := toolNames(ctx, t, dialMCP(ctx, t, upMCP(t, d)))

	if !tools["shelf.search"] {
		t.Fatalf("an agent connecting to an estate that already holds a "+
			"promoted command was served without it: %v", tools)
	}
	for _, want := range []string{"list", "describe", "invoke", "query"} {
		if !tools[want] {
			t.Fatalf("the socket served an agent without %q: %v", want, tools)
		}
	}
}

// TestTheDaemonPushesPromotionToAnAgentItIsHolding is MUTATION A's
// conformance case, and it is deliberately stated as a claim about the
// DAEMON rather than about the server: a program registering on the main
// socket reaches an agent on the MCP socket, with the agent doing nothing.
//
// Two things are asserted and they are not the same thing. That the tool list
// CHANGED is the projection being correct. That the agent was TOLD is the
// projection being live - without the notification an agent has no way to
// learn it should re-list, and a correct list nobody knows to read is the
// same as a stale one.
//
// Both directions are covered, because they fail differently. A registration
// that does not reach the agent is a tool it cannot use; a deregistration
// that does not reach it is a tool that resolves to nothing, which surfaces
// as a refusal with no relationship to the cause.
func TestTheDaemonPushesPromotionToAnAgentItIsHolding(t *testing.T) {
	sock, d := upDaemon(t, nil)
	ctx := ctx5(t)

	// Buffered and non-blocking, because this handler runs on the session's
	// own goroutine: a test that blocked here would deadlock the connection
	// it is asserting about rather than fail.
	changed := make(chan struct{}, 8)
	session := dialMCPWith(ctx, t, upMCP(t, d), &sdk.ClientOptions{
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) {
			select {
			case changed <- struct{}{}:
			default:
			}
		},
	})

	if toolNames(ctx, t, session)["shelf.search"] {
		t.Fatal("a tool was promoted for a program that is not registered")
	}

	// A PROGRAM REGISTERS ON THE OTHER SOCKET. The agent is not touched.
	drain(changed)
	stop := promoting(t, sock)
	waitToldToRelist(ctx, t, changed, "a program registered")
	if !toolNames(ctx, t, session)["shelf.search"] {
		t.Fatal("the agent was told its tool list changed and the promoted " +
			"tool is still not in it")
	}

	// AND IT DISCONNECTS. Same path, opposite direction.
	drain(changed)
	stop()
	waitDeregistered(ctx, t, d, "shelf")
	waitToldToRelist(ctx, t, changed, "a program disconnected")
	if toolNames(ctx, t, session)["shelf.search"] {
		t.Fatal("a disconnected program left its promoted tool in a live " +
			"agent's list, where it resolves to nothing")
	}
}

// TestTheCapabilityURIIsTheStringAnAgentHardCodes is MUTATION B's
// conformance case.
//
// THE LITERAL IS WRITTEN OUT ON PURPOSE AND MUST NOT BE REPLACED BY THE
// CONSTANT. Section 9 gives the capability map ONE fixed URI, which means the
// string is part of rig's contract with every agent that ever reads it, in
// the same way a wire field number is. A test that reads CapabilityURI
// asserts that rig agrees with itself, which it always will.
//
// If this fails, the question is not whether the new URI is nicer. It is
// whether every agent holding the old one is expected to find out, and that
// is a decision above a rename.
func TestTheCapabilityURIIsTheStringAnAgentHardCodes(t *testing.T) {
	const asAnAgentWouldWriteIt = "rig://capabilities"

	if mcpserver.CapabilityURI != asAnAgentWouldWriteIt {
		t.Fatalf("the capability map moved from %q to %q, so every agent "+
			"holding the published URI now reads nothing",
			asAnAgentWouldWriteIt, mcpserver.CapabilityURI)
	}

	_, d := upDaemon(t, nil)
	ctx := ctx5(t)
	session := dialMCP(ctx, t, upMCP(t, d))

	// OFFERED under that URI, so an agent that lists finds it.
	listed, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources over the MCP socket: %v", err)
	}
	offered := false
	for _, r := range listed.Resources {
		if r.URI == asAnAgentWouldWriteIt {
			offered = true
		}
	}
	if !offered {
		t.Fatalf("the socket does not offer %q; it offers %v",
			asAnAgentWouldWriteIt, listed.Resources)
	}

	// AND READABLE under it, so an agent that hard-coded it and never listed
	// is served too. Those are different requests and only one of them is
	// what an agent with the URI in its prompt actually sends.
	res, err := session.ReadResource(ctx,
		&sdk.ReadResourceParams{URI: asAnAgentWouldWriteIt})
	if err != nil {
		t.Fatalf("read %q over the MCP socket: %v", asAnAgentWouldWriteIt, err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("the map answered with %d contents, want 1", len(res.Contents))
	}
	if got := res.Contents[0].URI; got != asAnAgentWouldWriteIt {
		t.Errorf("the map was read as %q and answers as %q, so a client "+
			"keying its cache on the answer keys it on the wrong thing",
			asAnAgentWouldWriteIt, got)
	}
}

// upMCP starts the MCP surface on a real unix listener and returns its path.
//
// Its own directory rather than the daemon's, because upDaemon does not hand
// one back, and short for the reason upDaemonLogged states: sun_path is 108
// bytes and a temp dir under a long TMPDIR fails as EINVAL.
func upMCP(t *testing.T, d *Daemon) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "m")

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.ServeMCP(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock
}

// dialMCP connects an agent the way a real one does: a unix dial and an MCP
// client over it, with no reference to the server object on the other side.
func dialMCP(ctx context.Context, t *testing.T, sock string) *sdk.ClientSession {
	t.Helper()
	return dialMCPWith(ctx, t, sock, nil)
}

func dialMCPWith(ctx context.Context, t *testing.T, sock string,
	opts *sdk.ClientOptions,
) *sdk.ClientSession {
	t.Helper()
	nc, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial the MCP socket: %v", err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "an-agent", Version: "1"}, opts).
		Connect(ctx, &sdk.IOTransport{Reader: nc, Writer: nc}, nil)
	if err != nil {
		_ = nc.Close()
		t.Fatalf("connect over the MCP socket: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// waitToldToRelist waits for the server to tell this session its tool list
// changed.
//
// The SDK debounces the notification by 10ms and sends it AFTER the change,
// so a notification arriving means the change has already landed and the
// following list is not a race.
func waitToldToRelist(ctx context.Context, t *testing.T, changed <-chan struct{},
	after string,
) {
	t.Helper()
	select {
	case <-changed:
	case <-ctx.Done():
		t.Fatalf("%s and the agent holding the socket was never told its "+
			"tool list changed, so it would go on using the old one", after)
	}
}

// drain empties the notification channel before an action, so what the test
// waits for afterwards cannot be the previous action's notification.
func drain(c <-chan struct{}) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}
