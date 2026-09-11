package daemon

import (
	"context"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/mcpserver"
	"github.com/boris-milner/rig/internal/meta"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// TestSliceFiveDemo is M2 slice 5's demo:
//
//	"A program declares `promote` on one command, and an agent sees that
//	 command as its own tool while the other commands stay behind `describe`
//	 and `invoke`."
//
// BOTH HALVES ARE ASSERTED, and the second is the one that would rot quietly.
// Promotion is a shortcut, so a bug that promoted everything would satisfy
// "the promoted command is a tool" and destroy the whole point of section 9's
// tiering: an agent should not spend its context on fifteen tools it will
// never call.
func TestSliceFiveDemo(t *testing.T) {
	sock, d := upDaemon(t, nil)
	promoting(t, sock)
	ctx := ctx5(t)

	server := mcpserver.New(meta.New(d.kernel, d),
		principalOfKind(kernel.KindAgent, "an-agent"), "test")
	if err := server.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	session := connectAgent(ctx, t, server)

	tools := toolNames(ctx, t, session)

	// The promoted one is a tool of its own.
	if !tools["shelf.search"] {
		t.Fatalf("the promoted command is not a tool: %v", tools)
	}
	// The unpromoted one is NOT, and stays reachable through invoke.
	if tools["shelf.ping"] {
		t.Fatalf("an unpromoted command was promoted anyway: %v", tools)
	}
	// The four are still there. Promotion adds; it never replaces.
	for _, want := range []string{"list", "describe", "invoke", "query"} {
		if !tools[want] {
			t.Fatalf("promotion displaced a meta tool: %q missing from %v", want, tools)
		}
	}

	// The promoted tool runs the real command, and still carries the coverage
	// warning - it is the same call `invoke` makes, not a second path.
	ran := callTool(ctx, t, session, "shelf.search", map[string]any{})
	result, ok := ran["result"].(map[string]any)
	if !ok || result["ran"] != "shelf.search" {
		t.Fatalf("the promoted tool did not run the command: %v", ran)
	}
	if partial, _ := ran["partial"].([]any); len(partial) == 0 {
		t.Fatalf("the promoted tool dropped the coverage warning: %v", ran)
	}

	// The unpromoted command is still reachable the long way round. "Stay
	// behind describe and invoke" is a promise about reachability, not a
	// refusal.
	viaInvoke := callTool(ctx, t, session, "invoke",
		map[string]any{"program": "shelf", "command": "ping"})
	if _, ok := viaInvoke["result"]; !ok {
		t.Fatalf("an unpromoted command became unreachable: %v", viaInvoke)
	}
}

// TestPromotionFollowsTheEstate is the half the demo sentence does not state
// and slice 4's does: the tool list tracks what is registered NOW.
//
// A program that disconnects takes its promoted tools with it. Without this,
// an agent holding a session keeps a tool that resolves to nothing, and the
// failure surfaces as a call refused for a reason that has nothing to do with
// the cause.
func TestPromotionFollowsTheEstate(t *testing.T) {
	sock, d := upDaemon(t, nil)
	ctx := ctx5(t)

	server := mcpserver.New(meta.New(d.kernel, d),
		principalOfKind(kernel.KindAgent, "an-agent"), "test")

	// Nothing registered yet: no promoted tools, and Sync on an empty estate
	// is not an error.
	if err := server.Sync(ctx); err != nil {
		t.Fatalf("sync on an empty estate: %v", err)
	}
	session := connectAgent(ctx, t, server)
	if toolNames(ctx, t, session)["shelf.search"] {
		t.Fatal("a tool was promoted for a program that is not registered")
	}

	// A program registers while the agent is connected.
	stop := promoting(t, sock)
	if err := server.Sync(ctx); err != nil {
		t.Fatalf("sync after registration: %v", err)
	}
	if !toolNames(ctx, t, session)["shelf.search"] {
		t.Fatal("a program registered and its promoted tool did not appear")
	}

	// And it goes away again when the program does.
	stop()
	waitDeregistered(ctx, t, d, "shelf")
	if err := server.Sync(ctx); err != nil {
		t.Fatalf("sync after disconnect: %v", err)
	}
	if toolNames(ctx, t, session)["shelf.search"] {
		t.Fatal("a disconnected program left its promoted tool behind")
	}
}

// promoting registers "shelf" with search PROMOTED and ping not, and returns a
// function that disconnects it.
func promoting(t *testing.T, sock string) (stop func()) {
	t.Helper()
	const name = "shelf"

	decl := testDeclaration(name)
	search := proto.Clone(decl.GetCommands()[0]).(*rigv1.Command)
	search.Id = "search"
	search.Title = "Search"
	search.Effects = rigv1.Effects_EFFECTS_READ_ONLY
	search.Summary = "Search the index"
	search.Description = "Returns the items matching a query."
	search.Returns = "The matching items."
	// The one line this fixture exists for.
	search.Promote = true
	decl.Commands = append(decl.Commands, search)

	c := dial(t, sock)
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, decl); err != nil {
		t.Fatal(err)
	}
	return func() { _ = c.Close() }
}

func connectAgent(ctx context.Context, t *testing.T, server *mcpserver.Server) *sdk.ClientSession {
	t.Helper()
	clientT, serverT := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("serve: %v", err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "an-agent", Version: "1"}, nil).
		Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func toolNames(ctx context.Context, t *testing.T, s *sdk.ClientSession) map[string]bool {
	t.Helper()
	listed, err := s.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	out := map[string]bool{}
	for _, tool := range listed.Tools {
		out[tool.Name] = true
	}
	return out
}

// waitDeregistered waits for the registry to forget a program, so the test asserts
// about a deregistration that has happened rather than racing it.
func waitDeregistered(ctx context.Context, t *testing.T, d *Daemon, id string) {
	t.Helper()
	for {
		if _, still := d.kernel.See(principalOfKind(kernel.KindAgent, "w")).Program(id); !still {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s never deregistered", id)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
