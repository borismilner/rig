// Package meta is the four meta tools, with no transport under them.
//
// Section 9 solves the scaling problem with four tools - list, describe,
// invoke, query - and section 10 then requires `--json` to return "exactly
// what the MCP tool returns". Two surfaces returning the same answer is only
// achievable if the answer is built once, below both, which is what this
// package is. An MCP server and the CLI's --json are renderers over it.
//
// It deliberately knows nothing about MCP. M1's slice 1 was "no wire, no
// daemon" for the same reason: the data layer before the transport, so the
// transport has something true to carry.
package meta

import (
	"context"
	"errors"
	"fmt"

	"github.com/boris-milner/rig/internal/kernel"
)

// Tool names one of the four. There is no fifth, and adding one is a section
// 9 decision rather than a package change: the whole argument for four is
// that an agent pays for detail only where it needs detail.
type Tool string

const (
	List     Tool = "list"
	Describe Tool = "describe"
	Invoke   Tool = "invoke"
	Query    Tool = "query"
)

// Invoker runs one declared command as one principal. The daemon implements
// it; this package never holds a connection.
//
// It is an interface rather than a function value so an implementation can
// say what it is in a stack trace, and so a test double is a named type
// rather than an anonymous closure nobody can find again.
//
// THE PRINCIPAL IS AN ARGUMENT BECAUSE IT ONCE WAS NOT, and the gap that left
// is the case section 13a was rewritten for. This interface used to take
// (program, command, args). The layer above it had the caller, used it for the
// visibility check below, and then dropped it - so the daemon's implementation
// arrived at the authorization floor with nothing to authorise. The floor was
// not bypassed; it was made UNREACHABLE by a type signature, which is why it
// looked wrong from neither side: the caller believed it had authorised, the
// implementer had nothing to authorise with, and no rule was violated because
// none was consulted.
//
// Section 13a now states it as a rule: every function on a path to invocation
// carries the principal, and a signature that cannot express the caller is a
// defect in the signature.
type Invoker interface {
	Invoke(ctx context.Context, who kernel.Principal,
		program, command string, args []byte) ([]byte, error)
}

// Request is one call to one meta tool.
type Request struct {
	Tool Tool

	// Program and Command address a thing. describe takes a program and
	// optionally a command; invoke takes both.
	Program string
	Command string

	// Depth is list's only knob (section 9: "list returns the estate at
	// whatever depth is asked for").
	Depth kernel.Depth

	// Args is invoke's payload, as the program's declared schema expects it.
	Args []byte

	// Subject is what query is asking about.
	Subject string
}

// Answer is what every meta tool returns, and what both --json and the MCP
// tool serialise without adding to it.
//
// ONE TYPE FOR FOUR TOOLS, with the unused parts empty, and that is
// deliberate. Four result types would let one of them quietly stop carrying
// Partial, which is the one field section 9's demo turns on.
type Answer struct {
	Tool Tool

	// PARTIAL IS THE FIELD THIS WHOLE TYPE EXISTS FOR.
	//
	// Section 9's demo: an agent "runs a real command in a real program ...
	// and is told, IN THE SAME ANSWER, that its picture of that program is
	// partial". Not in a second call, not in a resource it might read, not in
	// a log line. Section 5k's rule is that no surface may imply
	// completeness, and a surface that answers a question without saying what
	// it does not know is implying it.
	Partial []Incomplete

	// Estate is list's answer, with the capability map's version so an agent
	// can tell whether anything has changed since it last looked.
	Estate  []kernel.Program
	Version string

	// Program and Command are describe's, depending on what was addressed.
	Program *kernel.Program
	Command *kernel.Command

	// Result is invoke's, exactly as the program returned it.
	Result []byte

	// Unavailable is query's honesty, and it is not an error.
	//
	// query reaches "logs, traces, the call log, config provenance, schedule
	// history, the audit log" (section 9), and at M2 every one of those lands
	// at M4 or later. So query is real and thin - the registry and the live
	// views - and it SAYS which sources it could not read. That is section
	// 5k's coverage principle applied to rig itself, and it is the difference
	// between a scoped tool and the invented data source section 5h records
	// as a failure.
	Unavailable []string
}

// Incomplete is one program admitting how much of rig it has adopted.
type Incomplete struct {
	Program  string
	Coverage kernel.Coverage
	Note     string
}

// ErrNoSuchTool is returned for a tool name that is not one of the four.
var ErrNoSuchTool = errors.New("meta: not one of the four tools")

// Server answers the four tools for one estate.
type Server struct {
	kernel  *kernel.Kernel
	invoker Invoker
}

// New builds a server. The invoker may be nil, and invoke then refuses rather
// than panicking: a surface that can read the estate but not call into it is
// a real configuration, and it should say so.
func New(k *kernel.Kernel, inv Invoker) *Server {
	return &Server{kernel: k, invoker: inv}
}

// Answer routes one request, as one principal.
//
// The principal is an argument rather than server state because a server
// answers every caller, and a principal held on the server is a principal
// that can be stale for the caller in front of it - which is the shape
// section 14 rewrote itself to forbid.
func (s *Server) Answer(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	switch r.Tool {
	case List:
		return s.list(who, r)
	case Describe:
		return s.describe(who, r)
	case Invoke:
		return s.invoke(ctx, who, r)
	case Query:
		return s.query(who, r)
	default:
		return Answer{}, fmt.Errorf("%w: %q; the four are %s, %s, %s and %s",
			ErrNoSuchTool, r.Tool, List, Describe, Invoke, Query)
	}
}

func (s *Server) list(who kernel.Principal, r Request) (Answer, error) {
	m, err := s.kernel.See(who).CapabilityMap(r.Depth)
	if err != nil {
		return Answer{}, err
	}
	return Answer{
		Tool:    List,
		Estate:  m.Programs,
		Version: m.Version,
		Partial: partialOf(m.Programs...),
	}, nil
}

func (s *Server) describe(who kernel.Principal, r Request) (Answer, error) {
	if r.Program == "" {
		return Answer{}, errors.New("meta: describe needs a program")
	}
	v := s.kernel.See(who)
	p, ok := v.Program(r.Program)
	if !ok {
		return Answer{}, notFound(r.Program, "")
	}
	out := Answer{Tool: Describe, Partial: partialOf(p)}

	if r.Command == "" {
		out.Program = &p
		return out, nil
	}
	c, ok := v.Command(r.Program, r.Command)
	if !ok {
		return Answer{}, notFound(r.Program, r.Command)
	}
	// The program travels with the command, because coverage is a property of
	// the program and an agent describing one command still has to be told
	// its picture of the program is partial.
	out.Program = &p
	out.Command = &c
	return out, nil
}

func (s *Server) invoke(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	if r.Program == "" || r.Command == "" {
		return Answer{}, errors.New("meta: invoke needs a program and a command")
	}
	v := s.kernel.See(who)
	p, ok := v.Program(r.Program)
	if !ok {
		return Answer{}, notFound(r.Program, "")
	}
	if s.invoker == nil {
		return Answer{}, errors.New("meta: this surface can read the estate " +
			"but cannot call into it; no invoker is configured")
	}

	// The coverage is read BEFORE the call and returned WITH its result,
	// which is section 9's demo in one function: an agent runs a real command
	// and is told in the same answer that its picture is partial.
	partial := partialOf(p)

	// The principal travels WITH the call. Visibility was decided above, and
	// it is a different mechanism from authorisation: the two were conflated
	// once here and that is exactly what left the floor unreachable.
	res, err := s.invoker.Invoke(ctx, who, r.Program, r.Command, r.Args)
	if err != nil {
		return Answer{}, err
	}
	return Answer{Tool: Invoke, Result: res, Partial: partial}, nil
}

// unavailableAtM2 is what query cannot read yet, named rather than omitted.
//
// Section 23 puts config and the event stream at M4 and log, trace and metric
// ingest, the call log, the coverage log and compiled redaction spans at M5.
// Section 14 already states the ordering - between M1 and M5 "an
// introspecting client reads the registry and the live views but not a
// recorded transcript" - and an ordering only one section knows is an
// ordering somebody changes.
var unavailableAtM2 = []string{
	"logs", "traces", "the call log", "config provenance",
	"schedule history", "the audit log",
}

func (s *Server) query(who kernel.Principal, r Request) (Answer, error) {
	m, err := s.kernel.See(who).CapabilityMap(kernel.DepthCommands)
	if err != nil {
		return Answer{}, err
	}
	out := Answer{
		Tool:        Query,
		Version:     m.Version,
		Unavailable: append([]string(nil), unavailableAtM2...),
		Partial:     partialOf(m.Programs...),
	}
	// The live views are all query has at M2, so a subject it cannot reach is
	// answered with what IS covered rather than refused. A refusal would tell
	// an agent the subject does not exist; this tells it where to look next.
	if r.Subject == "" || r.Subject == "registry" || r.Subject == "programs" {
		out.Estate = m.Programs
	}
	return out, nil
}

// partialOf reports every program whose coverage is not full.
//
// A program with full coverage produces no entry, so an empty Partial means
// "nothing here is incomplete" rather than "nobody looked" - and the
// distinction is the whole value of the field. Unspecified coverage is
// reported too: registration refuses it, so seeing one means something
// bypassed registration and that is worth saying rather than treating as
// complete.
func partialOf(ps ...kernel.Program) []Incomplete {
	var out []Incomplete
	for _, p := range ps {
		if p.Coverage == kernel.CoverageFull {
			continue
		}
		out = append(out, Incomplete{
			Program:  p.Identity.ID,
			Coverage: p.Coverage,
			Note:     p.CoverageNote,
		})
	}
	return out
}

func notFound(program, command string) error {
	if command == "" {
		return fmt.Errorf("meta: no program %q is visible to this caller", program)
	}
	return fmt.Errorf("meta: %q declares no command %q, or it is not visible "+
		"to this caller", program, command)
}
