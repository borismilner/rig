package daemon

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/internal/mcpserver"
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

// TestTheDoorTearsDownWhenTheConnectionDIESRATHERTHANSAYSGOODBYE is the
// self-check on dialMCPConn, and it is here because a helper nobody has
// watched discriminate is not an instrument.
//
// WHAT IT ASSERTS: serveMCPConn's teardown runs when the connection is closed
// underneath it with NO protocol shutdown - an fd closed and nothing else,
// which is exactly what the kernel does to a SIGKILLed bridge. That is the
// defer stack an occupant's release will hang off, so a door built on it needs
// this to be true before it is built, not after.
//
// AND THE FINDING THAT CAME OUT OF WRITING IT, which is worth more than the
// assertion and is the reason this comment is long:
//
//	AN ABRUPT DROP AND A GRACEFUL CLOSE ARE THE SAME EVENT AT THIS DAEMON.
//
// Measured, not reasoned: both produce `mcp session ended` carrying the same
// error - "use of closed network connection" - and then `mcp session closed`.
// There is no attribute, no error and no ordering that differs. The reason is
// structural rather than incidental: closing an MCP session closes its
// transport, the transport IS the connection, and serveMCPConn reads the end
// of that connection the same way whoever ended it.
//
// SO DO NOT WRITE THIS AS TWO ARMS. The shape it is tempting to write - one
// arm for a tidy release and one for a hard death, on the theory that a door
// could pass the first and leak on the second - has a second arm that CANNOT
// GO RED, because there is nothing for the door to branch on. The failure that
// shape is imagining (release wired to the tidy path only, e.g. after
// server.Run under an `err == nil` guard) leaks on BOTH paths here, since the
// error is non-nil either way. One arm catches it. Two arms would look like
// twice the evidence and be exactly the same evidence.
//
// WHAT dialMCPConn STILL BUYS, given that: the drop does not depend on the
// client library behaving well. A test that can only reach teardown by asking
// the SDK to shut down politely is a test whose instrument is the thing it
// should not be trusting, and condition A is about what happens when nothing
// behaves well.
func TestTheDoorTearsDownWhenTheConnectionDIESRATHERTHANSAYSGOODBYE(t *testing.T) {
	rec := &recorder{}
	_, d := upDaemonLogged(t, nil, slog.New(rec))
	sock := upMCP(t, d)
	ctx := ctx5(t)

	at := rec.mark()
	session, nc := dialMCPConn(ctx, t, sock, nil)

	// A REAL REQUEST FIRST, so the session is genuinely established rather
	// than a dial that has not yet been spoken over. A teardown observed on a
	// connection that never carried a request proves nothing about one that
	// did.
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatalf("list tools before dropping the connection: %v", err)
	}

	opened := waitLogged(t, rec, at, "mcp session opened")
	client := attr(opened, "client")
	if client == "" {
		t.Fatal("the accept carries no client id, so a teardown cannot be " +
			"matched to it and this test cannot tell one session from another")
	}

	// THE DROP. The connection, and NOT the session - session.Close is never
	// called, so nothing in the client library gets the chance to be polite.
	if err := nc.Close(); err != nil {
		t.Fatalf("drop the connection: %v", err)
	}

	closed := waitLogged(t, rec, at, "mcp session closed")
	if got := attr(closed, "client"); got != client {
		t.Fatalf("the teardown names client %q and the accept named %q, so "+
			"the daemon released a different session than the one that died",
			got, client)
	}
}

// waitLogged waits, bounded, for one record with this message after mark.
//
// BOUNDED AND NOT A SINGLE READ, because the teardown runs on the serving
// goroutine's own defer and a single read after the close is a race that
// passes on a fast machine and flakes on a loaded one. The bound is what
// turns "not yet" into a failure with something to read.
func waitLogged(t *testing.T, rec *recorder, mark int, msg string) slog.Record {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		recs, _ := rec.since(mark, slog.LevelDebug)
		for _, r := range recs {
			if r.Message == msg {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the daemon never logged %q. What it did log: %v",
				msg, messages(recs))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// attr reads one attribute off a record, or "" if it carries none.
func attr(r slog.Record, key string) string {
	var out string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			out = a.Value.String()
			return false
		}
		return true
	})
	return out
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
	session, _ := dialMCPConn(ctx, t, sock, opts)
	return session
}

// dialMCPConn is dialMCPWith, and it hands back the connection underneath the
// session as well.
//
// IT EXISTS BECAUSE CLOSING A SESSION IS NOT THE SAME EVENT AS LOSING A
// CONNECTION, and the door has to survive the second one. Until this helper
// existed a test could only reach the first: dialMCPWith dialled the conn,
// handed it to the transport and dropped the reference, so the most abrupt
// thing any test could do was ask the client library politely to shut down.
//
// That gap is not cosmetic. An occupant's life is its connection's life, so
// the release the roster depends on has to run when a bridge is KILLED - the
// kernel closing an fd with no protocol shutdown - and not merely when a
// well-behaved client says goodbye. A door that releases on the tidy path and
// leaks on the abrupt one would have passed every test this file could write.
// Closing the returned conn is that abrupt path: it is exactly what the kernel
// does for a process that has just been SIGKILLed.
//
// WHAT IT STILL CANNOT REACH, said here so nobody reads it as more than it is:
// a bridge that HANGS rather than dies holds its fd open, and no close of any
// kind models that. Nothing in the tree detects it either - there is no TTL
// and no reaper, deliberately - so it is a property of the presence model
// rather than a hole in this helper.
func dialMCPConn(ctx context.Context, t *testing.T, sock string,
	opts *sdk.ClientOptions,
) (*sdk.ClientSession, net.Conn) {
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
	// Both, and in this order, so a test that drops the conn itself is not
	// then torn down twice and a test that does neither leaks no fd. Cleanup
	// is LIFO, so the conn goes first - which is the abrupt order, and the
	// one a test asserting on the tidy path must therefore not rely on.
	t.Cleanup(func() { _ = session.Close() })
	t.Cleanup(func() { _ = nc.Close() })
	return session, nc
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

// TestTheAgentIsOrientedBeforeItMakesTheMistakesTheSurfaceInvites is rig's
// own preamble, on the wire, at the one moment an agent reads it.
//
// Section 9 requires a preamble per program and describe carries every
// registered program's. rig was the one thing on its own surface without one,
// because the MCP server was constructed with nil options and Instructions is
// where the protocol puts it. An agent met four tools with no orientation.
//
// WHAT IS ASSERTED IS THE FACTS, NOT THE PROSE. The preamble may be reworded
// freely; what it may not do is stop carrying one of the four things an agent
// cannot recover from the tool list, each of which is a mistake the surface
// actively invites. Every needle below was a real wrong turn taken against
// the live socket before the preamble existed.
func TestTheAgentIsOrientedBeforeItMakesTheMistakesTheSurfaceInvites(t *testing.T) {
	_, d := upDaemon(t, nil)
	ctx := ctx5(t)

	got := dialMCP(ctx, t, upMCP(t, d)).InitializeResult().Instructions
	if got == "" {
		t.Fatal("an agent connecting to the MCP socket is given no " +
			"instructions at all, so rig is the one thing on its own " +
			"surface with no preamble (section 9)")
	}

	for _, c := range []struct {
		mistake string
		needles []string
	}{
		{
			mistake: "asking invoke for program \"rig\", which is refused " +
				"because rig is held beside the registry rather than in it",
			needles: []string{"invoke", "query", "estate"},
		},
		{
			mistake: "asking for the widest depth by default, which is what " +
				"naming the cost in the preamble exists to stop",
			needles: []string{"depth", "programs", "commands", "full"},
		},
		{
			mistake: "reading a scoped projection as if it were the whole " +
				"estate, which is section 36's V20: absent can mean withheld",
			needles: []string{"basis", "complete", "scoped"},
		},
		{
			mistake: "concluding a partial program's config is absent when " +
				"it is only unreadable from here",
			needles: []string{"partial", "coverageNote"},
		},
		{
			// This case is here because it was MISSED. The first case above
			// already names invoke, query and estate, so deleting the whole
			// vocabulary paragraph left every needle satisfied elsewhere and
			// the test passed. These two words appear nowhere else in the
			// preamble, which is what makes the paragraph's absence visible.
			mistake: "asking query for a subject in prose and being answered " +
				"with what query cannot reach, because the answer names what " +
				"it CANNOT read and never what it can",
			needles: []string{"registry", "unavailable"},
		},
	} {
		for _, n := range c.needles {
			if !strings.Contains(strings.ToLower(got), strings.ToLower(n)) {
				t.Errorf("the preamble never says %q, so an agent is left "+
					"to discover it by %s", n, c.mistake)
			}
		}
	}

	// THE LENGTH IS PART OF THE RULING, 2026-09-16: past roughly forty lines
	// the preamble has become the specification, and an agent that has to
	// read the specification before its first call has been given the cost
	// section 9's tiering exists to avoid.
	if lines := strings.Count(strings.TrimSpace(got), "\n") + 1; lines > 45 {
		t.Errorf("the preamble is %d lines; it is orientation, not the "+
			"specification, and the bar is roughly forty", lines)
	}
}
