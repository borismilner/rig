package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borismilner/rig/internal/paths"
)

// Capabilities is plan/55 requirement 28, Boris 2026-10-03: "kind of
// Swagger UI but for our needs" - every capability of rig and of each
// registered program, in its own words, with a way to try it.
//
// ⛔ RIG'S OWN LIST COMES FROM THE RUNNING DAEMON, NEVER FROM THIS BINARY.
// The window could import rig's declaration at build time, and would then
// describe the rig it was built beside rather than the one answering: B89's
// skew, moved into a panel. The daemon's MCP surface already answers
// tools/list with each verb's words, argument schema and effects, so that is
// the source, and no new verb was needed (plan/55 build order, slice 4).
//
// The client is the official MCP Go SDK, which the daemon already serves
// with (go.mod); the window adds a client of the same library rather than a
// second hand-written JSON-RPC.

// tryDeadline is long because a command the person runs may take a while;
// the page shows it is waiting, so a slow answer is not a hang.
const tryDeadline = 30 * time.Second

// Capability is one thing a person can call from the Capabilities panel.
type Capability struct {
	// Owner is "rig" for rig's own verbs, else the program's id.
	Owner       string `json:"owner"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Returns     string `json:"returns"`
	// Effects is "read-only", "destructive", a program's declared effect,
	// "changes state" for a rig verb that is neither, or empty when nothing
	// was declared. Anything but read-only is confirmed before it is sent.
	Effects string `json:"effects"`
	// Args is the JSON Schema of the arguments, as JSON text; "" for none.
	Args string `json:"args"`
}

// TryResult is what came back from one call, said as rig said it.
type TryResult struct {
	OK   bool   `json:"ok"`
	Text string `json:"text"`
	// Call is the request as it went on the wire, so the panel can show
	// exactly what was sent.
	Call string `json:"call"`
}

func mcpSession(ctx context.Context) (*mcp.ClientSession, error) {
	sock, err := paths.MCPSocket()
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	nc, err := d.DialContext(ctx, "unix", sock)
	if err != nil {
		return nil, err
	}
	c := mcp.NewClient(&mcp.Implementation{Name: "rigwindow", Version: version}, nil)
	s, err := c.Connect(ctx, &mcp.IOTransport{Reader: nc, Writer: nc}, nil)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	return s, nil
}

// Capabilities lists rig's verbs from the daemon and every registered
// program's declared commands from the registry, rig first, each group in
// the order its source gave it.
func (s RigService) Capabilities() ([]Capability, error) {
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	sess, err := mcpSession(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var out []Capability
	for t, err := range sess.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		// A dotted name is a program's command promoted to a tool; it is
		// listed below from the registry, with its declared effects.
		if strings.Contains(t.Name, ".") {
			continue
		}
		out = append(out, verbCapability(t))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	progs, err := s.Programs()
	if err != nil {
		return out, nil // rig's own list is still worth showing
	}
	for _, p := range progs {
		for _, c := range p.Commands {
			out = append(out, Capability{
				Owner: p.ID, ID: c.ID, Title: c.Title, Summary: c.Summary,
				Description: c.Description, Returns: c.Returns,
				Effects: c.Effects, Args: c.Args,
			})
		}
	}
	return out, nil
}

func verbCapability(t *mcp.Tool) Capability {
	c := Capability{Owner: "rig", ID: t.Name, Title: t.Title, Description: t.Description}
	// The first sentence is the summary the list shows; the rest is said
	// in the detail.
	c.Summary = firstSentence(t.Description)
	if a := t.Annotations; a != nil {
		switch {
		case a.ReadOnlyHint:
			c.Effects = "read-only"
		case a.DestructiveHint != nil && *a.DestructiveHint:
			c.Effects = "destructive"
		default:
			c.Effects = "changes state"
		}
	}
	if t.InputSchema != nil {
		if b, err := json.Marshal(t.InputSchema); err == nil && string(b) != "null" {
			c.Args = string(b)
		}
	}
	return c
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// Try calls one capability. args is the JSON object the panel's form built.
// A refusal from rig is an answer, not an error: it comes back with OK false
// and rig's own words, so the panel shows it where the result goes.
func (RigService) Try(owner, id, args string) (TryResult, error) {
	if owner == "" || id == "" {
		return TryResult{}, fmt.Errorf("no capability named")
	}
	var a map[string]any
	if strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &a); err != nil {
			return TryResult{}, fmt.Errorf("the arguments are not a JSON object: %w", err)
		}
	}
	params := &mcp.CallToolParams{Name: id, Arguments: a}
	if owner != "rig" {
		// A program's command goes through invoke, which validates the
		// arguments against what the program declared.
		params = &mcp.CallToolParams{Name: "invoke", Arguments: map[string]any{
			"program": owner, "command": id, "args": a,
		}}
	}
	call, _ := json.MarshalIndent(map[string]any{"tool": params.Name, "arguments": params.Arguments}, "", "  ")
	res := TryResult{Call: string(call)}

	ctx, cancel := context.WithTimeout(context.Background(), tryDeadline)
	defer cancel()
	sess, err := mcpSession(ctx)
	if err != nil {
		return res, err
	}
	defer sess.Close()

	r, err := sess.CallTool(ctx, params)
	if err != nil {
		return res, err
	}
	res.OK = !r.IsError
	res.Text = toolText(r)
	return res, nil
}

// toolText is the result as text: the structured answer pretty-printed when
// there is one, else every text part joined.
func toolText(r *mcp.CallToolResult) string {
	if r.StructuredContent != nil {
		if b, err := json.MarshalIndent(r.StructuredContent, "", "  "); err == nil {
			return string(b)
		}
	}
	var parts []string
	for _, c := range r.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, "\n")
}
