package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/mcpserver"
	"github.com/boris-milner/rig/internal/meta"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// TestSliceThreeDemoThroughARealMCPServer is section 23's own M2 demo, run
// rather than argued:
//
//	"An agent runs a real command in a real program through one MCP server,
//	 having written nothing - and is told, in the same answer, that its
//	 picture of that program is partial."
//
// Every clause is asserted, and the load-bearing ones are the last two.
// HAVING WRITTEN NOTHING means the agent discovers the command through the
// tools rather than being handed its name by the test. IN THE SAME ANSWER
// means the invoke result and the partial-coverage warning come back from ONE
// call - not a second call, not a resource it might have read.
//
// It is wired the way rigd wires it: a real daemon, a real program registered
// over a real socket, the daemon's own meta.Invoker, and a real MCP client
// speaking to a real MCP server. The transport is in-memory so the test needs
// no second process; everything above it is the shipping path.
func TestSliceThreeDemoThroughARealMCPServer(t *testing.T) {
	sock, d := upDaemon(t, nil)
	indexing(t, sock) // registers "shelf", whose coverage is PARTIAL
	ctx := ctx5(t)

	who := principalOfKind(kernel.KindAgent, "an-agent")
	server := mcpserver.New(meta.New(d.kernel, d), who, "test")

	clientT, serverT := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("the MCP server would not serve: %v", err)
	}
	agent := sdk.NewClient(&sdk.Implementation{Name: "an-agent", Version: "1"}, nil)
	session, err := agent.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("the agent could not connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// HAVING WRITTEN NOTHING, part one: the agent is told what tools exist.
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"list", "describe", "invoke", "query"} {
		if !got[want] {
			t.Fatalf("the four meta tools are not all exposed; %q is missing from %v", want, got)
		}
	}

	// HAVING WRITTEN NOTHING, part two: it finds the program and the command
	// by asking, and the command name below is READ from this answer rather
	// than typed by the test.
	listed := callTool(ctx, t, session, "list", map[string]any{"depth": "commands"})
	program, command := firstCommand(t, listed)

	// THE DEMO: a real command, in a real program, through one MCP server.
	ran := callTool(ctx, t, session, "invoke",
		map[string]any{"program": program, "command": command})

	// It really ran, and the program's own result came back as JSON rather
	// than as a base64 string. Section 5d: rig is a dumb pipe, and a result an
	// agent has to decode twice is one rig has reshaped.
	result, ok := ran["result"].(map[string]any)
	if !ok || len(result) == 0 {
		t.Fatalf("the command produced no usable result: %v", ran)
	}
	if result["ran"] != program+"."+command {
		t.Fatalf("a different command ran: %v", result)
	}

	// AND IN THE SAME ANSWER: the agent is told its picture is partial. This
	// is the assertion the whole Answer type exists for - the warning is in
	// the object that carries the result, not somewhere the agent has to
	// think to look.
	partial, ok := ran["partial"].([]any)
	if !ok || len(partial) == 0 {
		t.Fatalf("the invoke answer did not say the picture is partial: %v", ran)
	}
	first, _ := partial[0].(map[string]any)
	if first["program"] != program {
		t.Fatalf("the warning names the wrong program: %v", first)
	}
	if first["coverage"] != "partial" {
		t.Fatalf("the warning does not say partial: %v", first)
	}
}

// indexing registers a program with PARTIAL coverage and one read-only
// command a caller would actually want to run.
//
// It is not the destructive fixture the authorisation tests use. A demo whose
// only runnable command is destructive would be a demo of the rules table,
// and one whose only command is `ping` would be a demo of rig's own liveness
// probe. Section 23's demo is about a command a PROGRAM declared.
func indexing(t *testing.T, sock string) {
	t.Helper()
	const name = "shelf"

	decl := testDeclaration(name)
	search := proto.Clone(decl.GetCommands()[0]).(*rigv1.Command)
	search.Id = "search"
	search.Title = "Search"
	search.Effects = rigv1.Effects_EFFECTS_READ_ONLY
	search.Summary = "Search the index"
	search.Description = "Returns the items matching a query."
	search.Returns = "The matching items and how many there were."
	decl.Commands = append(decl.Commands, search)

	c := dial(t, sock)
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{
			Result: []byte(`{"ran":"` + method + `","hits":3}`),
		}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, decl); err != nil {
		t.Fatal(err)
	}
}

// callTool runs one meta tool and unwraps the object rig returned, failing on
// a refusal rather than asserting against its text.
func callTool(ctx context.Context, t *testing.T, s *sdk.ClientSession,
	name string, args map[string]any,
) map[string]any {
	t.Helper()
	res, err := s.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call tool %s: %v", name, err)
	}
	if len(res.Content) == 0 {
		t.Fatal("the tool answered with no content at all")
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("the tool answered with %T, not text", res.Content[0])
	}
	if res.IsError {
		t.Fatalf("rig refused: %s", text.Text)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, text.Text)
	}
	return out
}

// firstCommand reads a program and one of its commands out of a list answer,
// so the demo's "having written nothing" clause is true of the test too.
func firstCommand(t *testing.T, listed map[string]any) (program, command string) {
	t.Helper()
	estate, _ := listed["estate"].([]any)
	if len(estate) == 0 {
		t.Fatalf("list returned no estate: %v", listed)
	}
	p, _ := estate[0].(map[string]any)
	identity, _ := p["identity"].(map[string]any)
	program, _ = identity["id"].(string)

	commands, _ := p["commands"].([]any)
	for _, c := range commands {
		cm, _ := c.(map[string]any)
		id, _ := cm["id"].(string)
		// Not the liveness probe: `ping` is rig's own and would make this a
		// demo of the probe rather than of a command a program declared.
		// Not the destructive one either - that would make it a demo of the
		// rules table, which section 13a's own tests already cover.
		if id != ProbeCommand && cm["effects"] != "destructive" {
			command = id
			break
		}
	}
	if program == "" || command == "" {
		t.Fatalf("no program/command found in %v", listed)
	}
	return program, command
}
