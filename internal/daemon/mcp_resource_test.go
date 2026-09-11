package daemon

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/boris-milner/rig/client"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/mcpserver"
	"github.com/boris-milner/rig/internal/meta"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// TestSliceFourDemoAcrossTheTwoSockets is M2 slice 4's demo, corrected.
//
//	"A new command registered by a running program changes the map with no
//	 agent restart and no rig change; a scoped caller and an introspecting one
//	 read different maps at the same instant."
//
// THE SECOND CLAUSE IS CROSS-SOCKET AND CANNOT BE ANYTHING ELSE, which is a
// fact about the slice rather than about this test. newPrincipal sets
// Introspect true unconditionally and asProgram is the one transition that
// clears it; registration is asProgram's only caller and an MCP connection
// never registers. So every caller on the MCP socket introspects, and the
// only pair the clause describes is a REGISTERED PROGRAM on the main socket
// against an AGENT on the MCP socket. Read as "two MCP callers" the clause is
// undemonstrable, and it was written that way from the specification rather
// than from the code.
//
// The MCP transport is in-memory for the same reason slice 3's demo makes it
// so: everything above the transport is the shipping path, and the real
// second socket is exercised by the live demonstration rather than here.
func TestSliceFourDemoAcrossTheTwoSockets(t *testing.T) {
	sock, d := upDaemon(t, nil)
	indexing(t, sock) // "shelf", registered over the real socket
	ctx := ctx5(t)

	// THE SCOPED CALLER: a program on the MAIN socket, scoped to itself by
	// the registration that made it a program at all.
	grabbit := program(t, sock, "grabbit")

	// THE INTROSPECTING CALLER: an agent on the MCP socket.
	agentSession := upAgent(ctx, t, d)

	// It is offered the resource having written nothing.
	listed, err := agentSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	var found *sdk.Resource
	for _, r := range listed.Resources {
		if r.URI == mcpserver.CapabilityURI {
			found = r
		}
	}
	if found == nil {
		t.Fatalf("the capability map is not offered as a resource; got %v", listed.Resources)
	}
	if found.MIMEType != "application/json" {
		t.Errorf("the resource announces %q, not JSON", found.MIMEType)
	}

	// AT THE SAME INSTANT: both callers read.
	wide := readMap(ctx, t, agentSession)
	narrow := scopedEstate(ctx, t, grabbit)

	if got := programIDs(t, wide); len(got) != 2 {
		t.Fatalf("the introspecting caller's map holds %v, want both programs", got)
	}
	if len(narrow) != 1 || narrow[0] != "grabbit" {
		t.Fatalf("the scoped caller saw %v, want just itself", narrow)
	}

	// The map states its own depth rather than leaving it to be inferred.
	if wide["depth"] != "commands" {
		t.Errorf("the resource does not say what depth it answered at: %v", wide["depth"])
	}
	version, _ := wide["version"].(string)
	if version == "" {
		t.Fatal("the map carries no version, so an agent cannot tell whether it changed")
	}

	// AND THE COVERAGE WARNING TRAVELS WITH IT. Section 5k: a surface that
	// answers without saying what it does not know implies completeness.
	partial, _ := wide["partial"].([]any)
	if len(partial) == 0 {
		t.Errorf("shelf declares partial coverage and the map does not say so: %v", wide)
	}

	// FIRST CLAUSE: the estate changes under a connected agent, and the agent
	// neither restarts nor reconnects - it reads the same URI again.
	//
	// The variant where one running program GAINS a command is asserted at
	// kernel level by TestSliceFourDemo; the daemon refuses a second hello on
	// a live connection, so over a socket the honest form of "the estate
	// changed" is another program registering.
	_ = program(t, sock, "docket")

	after := readMap(ctx, t, agentSession)
	if got := programIDs(t, after); len(got) != 3 {
		t.Fatalf("a program registered and the agent's next read did not see "+
			"it: %v", got)
	}
	if after["version"] == version {
		t.Error("the estate changed and the map's version did not, so an " +
			"agent holding the old version would never learn to re-read")
	}
}

// TestTheResourceIsBuiltForTheCallerInFrontOfIt is the scope assertion with
// the estate held still, so a difference cannot be a timing artefact.
//
// Two servers over one kernel at one instant, differing only in the principal
// each was built for. If these two ever agree, the resource is answering from
// something other than the caller's own view.
func TestTheResourceIsBuiltForTheCallerInFrontOfIt(t *testing.T) {
	sock, d := upDaemon(t, nil)
	indexing(t, sock)
	program(t, sock, "grabbit")
	ctx := ctx5(t)

	// Built by hand rather than through principalOfKind, which grants
	// Introspect - and introspect is exactly what a scoped caller does not
	// have. A "scoped" principal that still introspects sees everything, and
	// the test then passes for the wrong reason.
	scoped := kernel.Principal{
		UID: os.Getuid(), Kind: kernel.KindProgram,
		ClientID: "grabbit", SessionID: "s-grabbit", PID: 1,
		Scoped: true, Scopes: []string{"grabbit"},
	}

	wide := readMap(ctx, t, connectAs(ctx, t, d, principalOfKind(kernel.KindAgent, "an-agent")))
	narrow := readMap(ctx, t, connectAs(ctx, t, d, scoped))

	if len(programIDs(t, wide)) == len(programIDs(t, narrow)) {
		t.Fatalf("one URI served two principals the same map: %v vs %v",
			programIDs(t, wide), programIDs(t, narrow))
	}
	if wide["version"] == narrow["version"] {
		t.Error("two different maps share one version, so a cache keyed on " +
			"the version serves the scoped caller the whole estate - which " +
			"is the leak the digest exists to close")
	}
}

// upAgent connects an introspecting agent to its own MCP server.
func upAgent(ctx context.Context, t *testing.T, d *Daemon) *sdk.ClientSession {
	t.Helper()
	return connectAs(ctx, t, d, principalOfKind(kernel.KindAgent, "an-agent"))
}

// connectAs stands up one MCP server for one principal, the way the MCP
// socket does it: one server per connection, one principal per server.
func connectAs(ctx context.Context, t *testing.T, d *Daemon,
	who kernel.Principal,
) *sdk.ClientSession {
	t.Helper()
	server := mcpserver.New(meta.New(d.kernel, d), who, "test")
	clientT, serverT := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("the MCP server would not serve: %v", err)
	}
	c := sdk.NewClient(&sdk.Implementation{Name: "a-client", Version: "1"}, nil)
	session, err := c.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("could not connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// readMap reads the capability resource and unwraps its object.
func readMap(ctx context.Context, t *testing.T, s *sdk.ClientSession) map[string]any {
	t.Helper()
	res, err := s.ReadResource(ctx, &sdk.ReadResourceParams{URI: mcpserver.CapabilityURI})
	if err != nil {
		t.Fatalf("read the capability map: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("the resource answered with %d contents, want 1", len(res.Contents))
	}
	// A TTL of anything but zero would tell a client it may reuse a map built
	// for whoever asked last.
	if res.TTLMs != 0 {
		t.Errorf("the map is served with a TTL of %d, so a client may reuse "+
			"a projection built for a different principal", res.TTLMs)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &out); err != nil {
		t.Fatalf("the resource is not JSON: %v\n%s", err, res.Contents[0].Text)
	}
	return out
}

// scopedEstate asks rig.programs on the MAIN socket, as the registered
// program itself, which is the only caller in the system that is scoped.
func scopedEstate(ctx context.Context, t *testing.T, c *client.Client) []string {
	t.Helper()
	var resp rigv1.ProgramsResponse
	if err := c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{
		Depth: rigv1.Depth_DEPTH_COMMANDS,
	}, &resp); err != nil {
		t.Fatalf("rig.programs as the program itself: %v", err)
	}
	out := make([]string, 0, len(resp.GetPrograms()))
	for _, p := range resp.GetPrograms() {
		out = append(out, p.GetIdentity().GetId())
	}
	return out
}

func programIDs(t *testing.T, m map[string]any) []string {
	t.Helper()
	programs, ok := m["programs"].([]any)
	if !ok {
		t.Fatalf("the map has no programs list: %v", m)
	}
	out := make([]string, 0, len(programs))
	for _, p := range programs {
		obj, _ := p.(map[string]any)
		id, _ := obj["identity"].(map[string]any)
		s, _ := id["id"].(string)
		out = append(out, s)
	}
	return out
}
