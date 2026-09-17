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
// preamble is rig's own document an agent reads before touching rig.
//
// SECTION 9 REQUIRES A PREAMBLE PER PROGRAM, AND RIG WAS THE ONE THING ON ITS
// OWN SURFACE WITHOUT ONE. describe carries every registered program's
// preamble at DepthFull; the agent surface itself carried nothing, because
// this constructor passed nil options and ServerOptions.Instructions is where
// the protocol puts exactly this. An agent therefore met four tools with no
// orientation and learned the shape by making the mistakes below.
//
// WHAT IS IN IT IS RULED, 2026-09-16, and the ruling is worth recording
// because the obvious contents are the wrong ones. It is NOT a restatement of
// section 9 and NOT four tool descriptions - the tools carry those already.
// It is the four things an agent cannot recover by reading the tool list:
// that rig is not an invoke target, that depth costs, that absent can mean
// withheld, and what vocabulary query actually understands. Each one is here
// because an agent without it reasons confidently from a wrong model rather
// than failing visibly.
//
// If it grows past roughly forty lines it has become the specification and
// should be cut back to the orientation.
const preamble = `rig is a coordination daemon. These tools read and drive the programs
registered with it, and carry rig's own roster.

THE SEVEN TOOLS ARE TWO SHAPES, NOT SEVEN FEATURES.
  list, describe  the map. list is every program you may reach, at a depth;
                  describe is one program in full.
  invoke          the only one of the four that ACTS on a program.
  query           how you ask rig ABOUT RIG rather than about a program.
  announce        take a seat on the roster and say what you are FOR. A SEAT
                  NAME IS REQUIRED. Do this first; the next two need it.
  set_activity    say what you are doing now, and keep it current.
  list_agents     read the roster: who else is here, and doing what.

THE CONTINUITY RECORD - this project's own memory. Section 39.

  record_put      write a record: a decision, a requirement, a working note.
                  YOU MUST announce FIRST - every record carries the seat that
                  wrote it, taken from your roster row, never from the request.
  record_get      read one record, at its current version or an older one.
  record_query    find records by project and kind, and by field.
  record_history  every version of one record, oldest first. Nothing is
                  overwritten here; a superseded wording is still the record
                  of what was believed.
  record_link     write a typed edge between two records.
  record_unlink   remove one.
  record_refs     what points at this record, and through what.
  project_brief   the whole project in twelve sections. START HERE ON RESUME.
                  It tells you whether the project EXISTS, so a typo in a slug
                  is not reported as a project with nothing to do.
  progress_step   say a work item started, blocked or finished.

rig itself is not in the program map, so invoke and describe cannot reach it.
Asking invoke for program "rig" is the common first mistake; ask query with
subject "estate". Any tool beyond the sixteen above is a promoted program
command.

IF set_activity SAYS YOU HAVE NO ROW, YOU ARE NOT WHERE YOU THINK YOU ARE. A
row lives exactly as long as the connection that took it, so if you announced
earlier and this call says otherwise, that daemon is gone and something
re-dialled you without saying so. Announce again; do not retry.

DEPTH COSTS. ASK FOR THE ONE YOU NEED.
  programs  who is registered, and how much of rig each has adopted.
  commands  adds what you PICK a command by: effects, idempotency, whether it
            confirms, one line of summary. Usually the right answer.
  full      adds what you CALL it with: argument schemas and the program's
            own preamble. Long - ask it per program through describe.

ABSENT CAN MEAN WITHHELD. What you cannot see may have been filtered rather
than missing, and the basis field says which: "complete" is the whole estate,
"scoped" means something may have been filtered away and rig will not say what.

READ partial AND coverageNote BEFORE CONCLUDING ANYTHING. A program reporting
partial coverage has adopted only some of rig and the note names which part:
"the wire only" means its config is unreadable from here, not absent.

QUERY UNDERSTANDS registry, programs AND estate. The first two return the
estate's programs; estate returns which rig this is. Any other subject is
accepted, not refused, and answered with what query cannot reach - so an
answer that is mostly "unavailable" means the subject was not understood.

ONE WORD, TWO MEANINGS, AND IT WILL CATCH YOU. In list's answer the key
"estate" holds the PROGRAMS. Which estate you are connected to is
estateIdentity, and only query with subject "estate" returns it.
`

func New(m *meta.Server, who kernel.Principal, version string) *Server {
	srv := &Server{
		mcp: mcp.NewServer(&mcp.Implementation{Name: "rig", Version: version},
			&mcp.ServerOptions{Instructions: preamble}),
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

	// THE THREE ROSTER TOOLS, AND THEY ARE FIRST-CLASS RATHER THAN PROMOTED.
	//
	// announce, activity and peers are fully declared commands of program
	// `rig` and have been for as long as self.go has existed. Promotion looks
	// like the obvious route to them and is closed at four independent points,
	// the decisive one being that it would put rig.down - EffectsDestructive
	// with Confirms:No, whose protection IS unreachability - onto the agent
	// invoke surface. So these are declared here, and the route under them is
	// meta's Roster rather than Invoke.
	//
	// THE NAMES ARE THE OTHER COORDINATOR'S, NOT rig'S OWN, and that is the
	// point rather than a slip. The caller this surface exists for is an agent
	// moving its calls off a coordinator that names them announce,
	// set_activity and list_agents. A 1:1 map is what stops a silent porting
	// error, and two renames on the one surface the 1:1 rule was written to
	// keep rename-free would be self-defeating. The wire keeps its own names.
	mcp.AddTool(s, &mcp.Tool{
		Name: "announce",
		Description: "Take a seat on this estate's roster and say what this " +
			"session is FOR. Returns the whole roster, so one call both " +
			"registers you and tells you who else is here. A SEAT NAME IS " +
			"REQUIRED: it is the address a supervisor and your peers use for " +
			"you, and announcing into a seat a live peer already holds is " +
			"refused with the holder named rather than silently taking it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a announceArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.Announce, Seat: a.Seat, Purpose: a.Purpose,
			Activity: a.Activity,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "set_activity",
		Description: "Say what you are doing RIGHT NOW, in one line, and keep " +
			"it current as the work changes. Cheap - call it whenever you " +
			"move on to something else. Re-sending an unchanged line " +
			"deliberately does NOT reset its age, because repeating yourself " +
			"is not progress. It refuses if this connection has no row, which " +
			"is how you find out you are talking to a different daemon than " +
			"the one you announced to.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a activityArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.SetActivity, Activity: a.Activity,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "list_agents",
		Description: "Read this estate's roster: every seat, what each one is " +
			"for, what it is doing now, and how long its line has been " +
			"standing. Takes no arguments - rig scopes by estate and nothing " +
			"finer. It also names any other estate this daemon knows of and " +
			"cannot see into, so an incomplete picture says which part is " +
			"missing instead of implying there is none.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{Tool: meta.ListAgents}))
	})

	// THE NINE RECORD TOOLS, AND THEY ARE FIRST-CLASS FOR THE ROSTER TOOLS'
	// REASON RATHER THAN A NEW ONE.
	//
	// B54 asked the question this answers and said in as many words that nobody
	// had: "how does a WRITE to rig's own record reach an agent when rig is
	// deliberately not an invoke target". The three candidate routes were
	// already closed before this surface was written. `query` is READ-ONLY and
	// four of these nine are writes. `invoke` is refused for rig on purpose and
	// pinned by TestRigIsNotAnInvokeTargetOnAnySurface, because rig declares
	// `down` as EffectsDestructive and a regular surface would hand `rig.down`
	// to every agent in the estate - and it could not execute anyway, since
	// Daemon.call routes through a program's own connection and rig has no
	// connection to itself. Promotion is closed at four independent points.
	//
	// So the route is first-class tools, which is exactly where announce,
	// set_activity and list_agents landed for the same reason. Second
	// application of a ruling this tree already made rather than a new one.
	//
	// THE NAMES ARE THE WIRE'S OWN, with the dot replaced. `record.put` on the
	// wire is `record_put` here, and that 1:1 map is what stops a silent
	// porting error between a seat's CLI habits and its agent surface.

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_put",
		Description: "Write a record - a decision, a requirement, a working " +
			"note, anything this project must not lose. ANNOUNCE FIRST: the " +
			"seat that wrote it is taken from your roster row and cannot be " +
			"supplied here, so a write without a seat is refused rather than " +
			"stored anonymously. Supply if_version to replace a record you " +
			"have read; omit it to create, which refuses if the id exists.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recordPutArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordPutTool, RecordID: a.ID, Project: a.Project,
			Kind: a.Kind, Body: a.Body, Fields: a.Fields, IfVersion: a.IfVersion,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_get",
		Description: "Read one record. Omit version for the current one; give " +
			"a version to read what it said before. The answer carries who " +
			"wrote it and when.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recordGetArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordGetTool, RecordID: a.ID, Version: a.Version,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_query",
		Description: "Find records by project and kind, and narrow by field. " +
			"Both are matched EXACTLY, so a kind spelled differently is a " +
			"different kind; query with neither to list everything and find " +
			"out how a thing is actually spelled. An empty result means " +
			"nothing matched - it is an answer, not a failure.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recordQueryArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordQueryTool, Project: a.Project, Kind: a.Kind, Fields: a.Fields,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_history",
		Description: "Every version of one record, oldest first. The record is " +
			"append-only: a change writes a new version and keeps the old, so " +
			"'what did this say before' is a question rather than an " +
			"archaeology.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recordIDArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordHistoryTool, RecordID: a.ID,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_link",
		Description: "Write a typed edge from one record to another, such as " +
			"part-of or supersedes. ANNOUNCE FIRST: an edge is a claim about " +
			"two records and carries who made it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a linkArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordLinkTool, From: a.From, To: a.To, LinkKind: a.Kind,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "record_unlink",
		Description: "Remove one typed edge between two records. ANNOUNCE FIRST.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a linkArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordUnlinkTool, From: a.From, To: a.To, LinkKind: a.Kind,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_refs",
		Description: "What points at this record, and through what edge. The " +
			"answer says whether it was truncated, so a partial picture never " +
			"arrives looking complete.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a recordIDArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.RecordRefsTool, RecordID: a.ID,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "project_brief",
		Description: "The whole project in twelve sections: what is next, what " +
			"is blocked, what governs it, what has moved. START HERE WHEN YOU " +
			"RESUME. It reports whether the project EXISTS as its own fact, so " +
			"a mistyped slug is never answered as a project with nothing to do.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a projectArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.ProjectBriefTool, Project: a.Project,
		}))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "progress_step",
		Description: "Say a work item started, blocked or finished, with a " +
			"note in your own words. ANNOUNCE FIRST. This is what makes a " +
			"brief show movement rather than a static list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, a stepArgs) (*mcp.CallToolResult, any, error) {
		return answer(m.Answer(ctx, who, meta.Request{
			Tool: meta.ProgressStepTool, Item: a.Item, Project: a.Project,
			State: a.State, Body: a.Note,
		}))
	})

	// THE CAPABILITY MAP, AS ONE RESOURCE. Section 9, and M2 slice 4.
	//
	// The handler closes over `who`, which is what makes one URI serve every
	// principal its own map with no extra mechanism: there is one Server per
	// connection and one principal per Server, minted at accept. The scope
	// filter is the same one every other read goes through.
	s.AddResource(&mcp.Resource{
		Name:     "capabilities",
		Title:    "The estate's capability map",
		URI:      CapabilityURI,
		MIMEType: "application/json",
		Description: "Everything this caller may reach, in one object: every " +
			"program, its commands, how much of rig it has adopted, and a " +
			"version that changes when any of that does. Read it once " +
			"instead of describing programs one at a time.",
	}, srv.readCapabilities)

	return srv
}

// CapabilityURI is the one URI the capability map is served at.
//
// ONE FIXED URI WHOSE CONTENT VARIES BY THE CONNECTION'S PRINCIPAL, ruled
// 2026-09-11, and the alternatives were live rather than impossible - the SDK
// expresses one resource per depth and a parameterised template equally well,
// and both were tried against a running client before the ruling.
//
// The reasons it is this one: section 9's purpose is "read it once and know
// everything that exists", which a URI per program defeats by turning
// discovery back into enumeration; and a URI per depth makes an agent choose
// a depth before it knows what exists, which is discovery backwards.
// capability.go's rule that one estate at two depths must never share a
// VERSION is about identity and is satisfied by the version - depth is a
// parameter of the request, not part of the address.
const CapabilityURI = "rig://capabilities"

// capabilityDepth is the depth the resource answers at.
//
// It has to be a constant rather than a choice, because a resource read
// carries a URI and nothing else: the MCP read has no argument channel the
// way a tool call does. DepthCommands is the same default `list` takes and
// for the same sentence - DepthCommands carries what an agent picks a command
// BY, DepthFull what it calls the command WITH - and picking is what
// discovery is. An agent that has chosen then calls describe for the rest.
const capabilityDepth = kernel.DepthCommands

// readCapabilities answers one read of the capability map.
//
// An error here is a protocol error rather than the IsError tool result a
// refusal gets, and the difference is real: a tool result says the call
// happened and the answer was no, while a resource that could not be built
// was not read at all.
func (s *Server) readCapabilities(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	m, err := s.meta.CapabilityMap(s.who, capabilityDepth)
	if err != nil {
		return nil, fmt.Errorf("rig could not build the capability map: %w", err)
	}
	b, err := meta.MarshalCapabilityMap(m)
	if err != nil {
		return nil, fmt.Errorf("rig could not render its own capability map: %w", err)
	}

	res := &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      CapabilityURI,
			MIMEType: "application/json",
			Text:     string(b),
		}},
	}

	// SET EXPLICITLY, THOUGH ZERO IS ALREADY THE VALUE. A TTL of 0 means the
	// response should be treated as immediately stale, which is the only safe
	// hint for an object whose contents depend on who asked. Leaving it to
	// the zero value would make the safe behaviour an accident that survives
	// exactly until someone sets a TTL for a reason that seems good.
	//
	// THE OTHER HALF OF THIS IS NOT DEFENDED, AND THAT WAS SEEN RATHER THAN
	// MISSED. go-sdk v1.7.0 stamps cacheScope="public" on every resource
	// result AFTER the handler returns, overwriting whatever the handler set
	// - setDefaultCacheableValues assigns unconditionally, so its name says
	// default and its behaviour is an override. A per-principal map announced
	// as publicly cacheable is exactly the scope-leak-through-a-cache-key
	// section 9's digest exists to close, arriving from outside rig. It is
	// unreachable today because every caller on this socket introspects, so
	// there is only one map to serve; it arms the moment a narrower principal
	// reaches this surface. There is no supported way to opt out and a forged
	// one would be a second surface, so it is left alone deliberately.
	res.TTLMs = 0

	return res, nil
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

// announceArgs is announce's, and it is 1:1 with the other coordinator's
// EXCEPT for seat, which rig requires and that one has no concept of.
//
// area AND tags ARE DELIBERATELY ABSENT rather than accepted and ignored. The
// other coordinator scopes a roster by them; rig scopes by ESTATE and nothing
// finer, so there is nothing here for them to mean. Undeclared, a caller that
// passes them gets a schema error it can read; accepted, it would get silence
// and believe it had scoped something.
type announceArgs struct {
	Seat     string `json:"seat" jsonschema:"the seat you are taking - the address a supervisor and your peers use for you, such as backend-1. Required."`
	Purpose  string `json:"purpose" jsonschema:"one line saying what this session is FOR, in terms the person supervising would recognise. Required."`
	Activity string `json:"activity,omitempty" jsonschema:"optional: what you are doing right now. set_activity carries this from then on."`
}

// noArgs is list_agents', and it is a named empty struct rather than an
// anonymous one so the generated schema says "this tool takes nothing" in a
// place a reader can find.
type noArgs struct{}

type activityArgs struct {
	Activity string `json:"activity" jsonschema:"one short line saying what is happening right now. It replaces the previous line and resets its age - unless it is identical, which deliberately does not."`
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

// THE RECORD TOOLS' ARGUMENTS.
//
// ⛔ EVERY FIELD CARRIES A jsonschema DESCRIPTION AND THAT IS NOT DECORATION.
// The survey's finding 5.2 measured argument validation here as schema-driven
// with actionable errors, and finding 2.6 measured the opposite failure on
// `query`: its subject vocabulary is not in the description, so an agent cannot
// discover what it may ask for. These structs are the place that gap either
// exists or does not.

type recordPutArgs struct {
	ID        string            `json:"id" jsonschema:"the record id. Supply your own - a stable key you can find again beats a minted uuid nobody can guess."`
	Project   string            `json:"project" jsonschema:"the project this record belongs to"`
	Kind      string            `json:"kind" jsonschema:"what this record IS: decision, requirement, work-item, note, artefact, progress, feature, standard, project. Matched exactly."`
	Body      string            `json:"body,omitempty" jsonschema:"the prose. Optional, and it is the half a typed field cannot carry."`
	Fields    map[string]string `json:"fields,omitempty" jsonschema:"typed fields beside the prose, such as title or status. These are what a brief renders and a query filters on - prefer them to prose an agent has to parse back out."`
	IfVersion uint64            `json:"if_version,omitempty" jsonschema:"the version you believe is current, for compare-and-swap. Omit to CREATE, which refuses if the id already exists. This is the only thing standing between two agents and a lost write."`
}

type recordGetArgs struct {
	ID      string `json:"id" jsonschema:"the record id"`
	Version uint64 `json:"version,omitempty" jsonschema:"omit for the current version; give one to read what the record said before"`
}

type recordQueryArgs struct {
	Project string            `json:"project,omitempty" jsonschema:"the project. Matched exactly; omit to search every project."`
	Kind    string            `json:"kind,omitempty" jsonschema:"the kind. Matched exactly; omit for every kind. Query with neither project nor kind to see how things are actually spelled."`
	Fields  map[string]string `json:"fields,omitempty" jsonschema:"narrow to records whose fields all match these exactly, such as status=active"`
}

type recordIDArgs struct {
	ID string `json:"id" jsonschema:"the record id"`
}

type linkArgs struct {
	From string `json:"from" jsonschema:"the record the edge starts at"`
	To   string `json:"to" jsonschema:"the record the edge points at"`
	Kind string `json:"kind" jsonschema:"the edge type, such as part-of, supersedes, blocks or notes-about"`
}

type projectArgs struct {
	Project string `json:"project" jsonschema:"the project to brief"`
}

type stepArgs struct {
	Item    string `json:"item" jsonschema:"the record id of the work item this step is about"`
	Project string `json:"project" jsonschema:"the project the item belongs to"`
	State   string `json:"state" jsonschema:"started, blocked or done"`
	Note    string `json:"note,omitempty" jsonschema:"optional: what happened, in your own words. A state with no note is still the signal that something moved."`
}
