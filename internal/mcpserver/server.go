// Package mcpserver exposes rig's four meta tools on one MCP server.
//
// It is a RENDERER and nothing else. Every decision - what this caller may
// see, what it may run, what the estate looks like at a depth, which programs
// admit their picture is partial - belongs to internal/meta and the kernel
// underneath it. Section 9's argument for the four tools is that an agent
// drives the whole estate through one server having written nothing; a server
// that decided anything of its own would be a second place for that to be
// true or false.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// Server is rig's MCP server: the four meta tools, plus one tool per PROMOTED
// command (PLAN.md section 9, M2 slice 5).
//
// It wraps the SDK's server rather than being one because promotion is state -
// which commands are promoted depends on what is registered right now, and a
// program that registers later must be able to change the tool list without
// the agent restarting.
type Server struct {
	mcp  *mcp.Server
	meta *meta.Server
	who  kernel.Principal

	// mu guards promoted, which is the set of tool names this server has
	// added for promoted commands. It is the SDK's tool table that is
	// authoritative; this is what lets Sync work out the difference without
	// removing the four.
	mu       sync.Mutex
	promoted map[string]bool
}

// Connect serves one MCP session.
func (s *Server) Connect(ctx context.Context, t mcp.Transport,
	opts *mcp.ServerSessionOptions,
) (*mcp.ServerSession, error) {
	return s.mcp.Connect(ctx, t, opts)
}

// Run serves until the transport closes.
func (s *Server) Run(ctx context.Context, t mcp.Transport) error {
	return s.mcp.Run(ctx, t)
}

// New builds the MCP server for one estate, answering as one principal.
//
// THE PRINCIPAL IS ONE FOR THE WHOLE SERVER AT M2, AND THAT IS A KNOWN LIMIT
// RATHER THAN A DESIGN. Section 14's table gives an agent's connection
// everything, so at M2 there is exactly one principal to hold and no scoping
// to lose. An MCP server holding ONE connection for MANY callers is the case
// section 14's per-connection principal cannot express - the principal is
// minted at accept and never changes - and the first caller that needs a
// narrower one is the HTTP surface at slice 6, which section 14 specifies as
// "its bearer principal's scopes, never introspect".
//
// Section 13a's ruling is what decides the shape when that lands: every
// function on a path to invocation carries the principal. This takes one
// because it has one; it does not hold it anywhere the answer could go stale
// for the caller in front of it.
func New(m *meta.Server, who kernel.Principal, version string) *Server {
	srv := &Server{
		mcp:      mcp.NewServer(&mcp.Implementation{Name: "rig", Version: version}, nil),
		meta:     m,
		who:      who,
		promoted: map[string]bool{},
	}
	s := srv.mcp

	mcp.AddTool(s, &mcp.Tool{
		Name: "list",
		Description: "List the programs this caller may reach, at a depth. " +
			"Returns the estate, the capability map's version, and which " +
			"programs report that their adoption of rig is partial.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a listArgs) (*mcp.CallToolResult, any, error) {
		depth, err := parseDepth(a.Depth)
		if err != nil {
			return refusal(err), nil, nil
		}
		return answer(m.Answer(ctx, who, meta.Request{Tool: meta.List, Depth: depth}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "describe",
		Description: "Describe one program in full, or one of its commands. " +
			"A program's answer carries its preamble; a command's carries the " +
			"argument schema, the examples and the declared effects.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a describeArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.Describe, Program: a.Program, Command: a.Command,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "invoke",
		Description: "Run one declared command. The answer carries the " +
			"program's own result AND, in the same answer, which programs " +
			"report their picture is partial.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a invokeArgs) (*mcp.CallToolResult, any, error) {
		raw, err := encodeArgs(a.Args)
		if err != nil {
			return refusal(err), nil, nil
		}
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.Invoke, Program: a.Program, Command: a.Command, Args: raw,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "query",
		Description: "Ask about rig itself. At M2 this reads the registry and " +
			"the live views only, and it NAMES the sources it cannot reach " +
			"yet rather than omitting them.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a queryArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{Tool: meta.Query, Subject: a.Subject}))
	})

	return srv
}

// Sync brings the promoted tools into line with the estate as it is now.
//
// SLICE 5, AND THE WHOLE OF IT: a command a program marked `promote` becomes a
// tool beside the four, while every other command stays behind `describe` and
// `invoke`. Section 9's tiering argument is what promotion is for - an agent
// should not have to spend context describing fifteen commands to find the one
// it uses constantly - so promotion ADDS a shortcut and removes nothing.
//
// It is a reconciliation rather than an append because the estate moves. A
// program that disconnects takes its promoted tools with it, and one that
// registers while an agent is connected adds them; the SDK tells the client
// its tool list changed, so no agent restarts. Calling it repeatedly is safe
// and is how it is meant to be used.
//
// A promoted tool NEVER bypasses anything. It routes to the same
// meta.Server.Answer that `invoke` routes to, so the visibility check, the
// authorization floor and the coverage warning are identical - the only
// difference is that the agent did not have to name the program and command
// itself. A shortcut that skipped a check would be a second surface, which is
// the thing section 13a's floor is placed in the kernel to prevent.
func (s *Server) Sync(ctx context.Context) error {
	// DepthFull, because a promoted tool needs what the command is CALLED
	// with - its argument schema - not merely what it is picked by.
	a, err := s.meta.Answer(ctx, s.who,
		meta.Request{Tool: meta.List, Depth: kernel.DepthFull})
	if err != nil {
		return fmt.Errorf("mcpserver: could not read the estate to promote from: %w", err)
	}

	want := map[string]promotion{}
	for _, p := range a.Estate {
		for _, c := range p.Commands {
			if !c.Promote {
				continue
			}
			want[toolName(p.Identity.ID, c.ID)] = promotion{program: p.Identity.ID, command: c}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var gone []string
	for name := range s.promoted {
		if _, still := want[name]; !still {
			gone = append(gone, name)
		}
	}
	if len(gone) > 0 {
		// Sorted so a log line or a test sees a stable order rather than a
		// map's.
		sort.Strings(gone)
		s.mcp.RemoveTools(gone...)
		for _, name := range gone {
			delete(s.promoted, name)
		}
	}

	for name, p := range want {
		if s.promoted[name] {
			continue
		}
		s.mcp.AddTool(s.toolFor(name, p), s.promotedHandler(p))
		s.promoted[name] = true
	}
	return nil
}

// promotion is one command a program asked to have promoted.
type promotion struct {
	program string
	command kernel.Command
}

// toolName is <program>.<command>, which is the same name the wire's method
// uses. One spelling for one command, so an agent reading a trace and an agent
// reading a tool list are looking at the same string.
func toolName(program, command string) string { return program + "." + command }

func (s *Server) toolFor(name string, p promotion) *mcp.Tool {
	description := p.command.Summary
	if p.command.Description != "" {
		description = p.command.Description
	}
	if p.command.Returns != "" {
		description += " Returns: " + p.command.Returns
	}
	return &mcp.Tool{
		Name:        name,
		Title:       p.command.Title,
		Description: description,
		InputSchema: schemaOrEmpty(p.command.Args),
	}
}

func (s *Server) promotedHandler(p promotion) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// The arguments go through as the JSON they arrived as. rig does not
		// interpret them (section 5d), and the kernel validates them against
		// what the program declared - the same validation `invoke` gets,
		// because it is literally the same call.
		res, err := s.meta.Answer(ctx, s.who, meta.Request{
			Tool:    meta.Invoke,
			Program: p.program,
			Command: p.command.ID,
			Args:    req.Params.Arguments,
		})
		out, _, _ := answer(res, err)
		return out, nil
	}
}

// schemaOrEmpty is the command's declared argument schema, or the empty object
// schema for a command that declares none.
//
// A tool with no inputSchema at all is not valid MCP, and inventing a
// permissive one would tell an agent it may pass anything. An empty object
// says truthfully: this command takes nothing.
func schemaOrEmpty(args []byte) any {
	if len(args) == 0 {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	var schema any
	if err := json.Unmarshal(args, &schema); err != nil {
		// A declaration whose schema is not JSON cannot have been registered:
		// the kernel validates it. Reaching here means the registry is
		// inconsistent, and refusing arguments is the safe read.
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}

type listArgs struct {
	Depth string `json:"depth,omitempty" jsonschema:"how much to return: programs, commands or full. Defaults to commands."`
}

type describeArgs struct {
	Program string `json:"program" jsonschema:"the program id"`
	Command string `json:"command,omitempty" jsonschema:"a command id; omit to describe the program itself"`
}

type invokeArgs struct {
	Program string         `json:"program" jsonschema:"the program id"`
	Command string         `json:"command" jsonschema:"the command id"`
	Args    map[string]any `json:"args,omitempty" jsonschema:"the command's arguments, shaped by the schema describe returns"`
}

type queryArgs struct {
	Subject string `json:"subject,omitempty" jsonschema:"what to ask about; omit for everything reachable"`
}

// parseDepth defaults to DepthCommands, which is the depth an agent choosing
// a command needs: section 9's line is that DepthCommands carries what an
// agent picks a command BY, and DepthFull what it calls the command WITH.
func parseDepth(s string) (kernel.Depth, error) {
	if s == "" {
		return kernel.DepthCommands, nil
	}
	return kernel.ParseDepth(s)
}

// encodeArgs turns the tool's argument object back into the JSON the program
// declared a schema for. rig does not interpret it (section 5d).
func encodeArgs(args map[string]any) ([]byte, error) {
	if len(args) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("the arguments are not encodable as JSON: %w", err)
	}
	return b, nil
}

// answer renders one meta.Answer, or the refusal that replaced it.
func answer(a meta.Answer, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return refusal(err), nil, nil
	}
	b, err := meta.MarshalAnswer(a)
	if err != nil {
		return nil, nil, fmt.Errorf("rig could not render its own answer: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// refusal returns a failure the AGENT can act on, which is the whole of
// section 9's "errors an agent can act on" reaching this surface.
//
// It is a tool result rather than a protocol error deliberately: a protocol
// error is the transport saying the call did not happen, and this call
// happened and was answered with a no. The four structured fields are carried
// when the refusal has them, so the agent is told the precondition that
// failed, the state actually found, the fix, and the exact command that
// applies it - rather than a sentence it has to parse.
func refusal(err error) *mcp.CallToolResult {
	out := map[string]string{"error": err.Error()}
	if f, ok := kernel.AsRefusal(err); ok {
		for k, v := range map[string]string{
			"precondition": f.Precondition,
			"actual":       f.Actual,
			"fix":          f.Fix,
			"fixCommand":   f.FixCommand,
		} {
			if v != "" {
				out[k] = v
			}
		}
	}
	b, mErr := json.Marshal(out)
	if mErr != nil {
		b = []byte(`{"error":"rig could not render its own refusal"}`)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}
}
