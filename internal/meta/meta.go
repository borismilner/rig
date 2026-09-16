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

// Estate is WHICH rig this is, as opposed to what is registered in it.
//
// IT IS NOT DERIVABLE FROM ANYTHING ELSE THIS PACKAGE ALREADY RETURNS, and
// that is the defect it was added for. The capability map's version is a
// digest of content alone - deliberately, so it is "comparable between two
// daemons" - which means two estates holding the same programs produce the
// SAME version. An agent comparing digests to work out whether it dialled
// production or development gets a match in exactly the case that matters.
//
// The fields are the ones rig.estate already answers on the wire, in the
// types this package can hold: section 9's data layer knows nothing about the
// transport, so the wire's role enum arrives here as the word an agent reads.
type Estate struct {
	// Name is what this estate claimed, empty when it claimed none.
	Name string

	// Role is production, development, unnamed or unspecified - the wire
	// enum's own word, lowercased, so a role added to the proto arrives here
	// without a second table to update.
	Role string

	DaemonVersion string
	Wire          string
	SemanticsGen  int32
}

// EstateIdentity is the optional half of Invoker: the thing that can call
// into an estate usually also knows which estate it is.
//
// OPTIONAL RATHER THAN PART OF Invoker, and the reason is the constructor.
// meta.New's invoker may be nil - "a surface that can read the estate but not
// call into it is a real configuration" - so a surface with no daemon under it
// must still be able to answer query, and it answers by reporting the estate's
// identity UNAVAILABLE rather than by refusing. Widening Invoker would make
// that configuration unrepresentable instead of honest.
type EstateIdentity interface {
	EstateIdentity() Estate
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

	// Depth is how much of the estate this answer actually carries.
	//
	// IT IS WHAT WAS USED, NOT WHAT WAS ASKED FOR, and the two can differ.
	// list takes its depth from the capability map rather than from the
	// Request, because the map is what normalised it and a map's identity
	// includes its depth - "the same estate at two depths is two different
	// maps and must never share a version". Reporting the request would let
	// an answer claim a depth its own version does not correspond to.
	//
	// A tool that projects nothing leaves it unspecified, which is section
	// 21's zero meaning "nothing was said" and is the honest answer for
	// invoke: no depth was involved in returning a program's own result.
	Depth kernel.Depth

	// Program and Command are describe's, depending on what was addressed.
	Program *kernel.Program
	Command *kernel.Command

	// Result is invoke's, exactly as the program returned it.
	Result []byte

	// Identity is query's answer for subject "estate": which rig this is.
	//
	// IT IS NOT CALLED Estate BECAUSE Estate IS ALREADY TAKEN BY THE PROGRAM
	// LIST, and that is a misnomer this field did not introduce and must not
	// pretend is fine: `Answer.Estate` carries []kernel.Program, so the field
	// named for the estate is the one thing in this type that is not about
	// the estate. Renaming it moves a shipped output shape, so it is a
	// backlog item rather than a drive-by; until then the two live side by
	// side and this comment is what keeps the next reader from swapping them.
	Identity *Estate

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

// CapabilityMap is the whole estate as ONE principal may see it: the content
// of the single MCP resource section 9 specifies.
//
// IT IS HERE RATHER THAN REACHED FOR BY THE SURFACE, and that is the same
// rule the four tools already follow. Every decision about what a caller may
// see belongs to this package and the kernel under it; a surface calling See
// itself would be a second place for the scope filter to be applied - or
// forgotten - and section 13a puts the floor in the kernel precisely so there
// is only ever one.
//
// The principal is an argument for the reason Answer's is: a principal held
// on the server is a principal that can be stale for the caller in front of
// it.
func (s *Server) CapabilityMap(who kernel.Principal, d kernel.Depth) (kernel.CapabilityMap, error) {
	return s.kernel.See(who).CapabilityMap(d)
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
		Depth:   m.Depth,
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
	// describe "returns one thing in full" (section 9), so it reports the
	// depth it actually carries rather than leaving an agent to infer it from
	// which fields happen to be populated.
	out := Answer{Tool: Describe, Depth: kernel.DepthFull, Partial: partialOf(p)}

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

// The subjects query understands at M2. A subject in neither this set nor
// unavailableAtM2 is one query does not understand, and it says so.
const (
	SubjectRegistry = "registry"
	SubjectPrograms = "programs"
	SubjectEstate   = "estate"
)

func (s *Server) query(who kernel.Principal, r Request) (Answer, error) {
	m, err := s.kernel.See(who).CapabilityMap(kernel.DepthCommands)
	if err != nil {
		return Answer{}, err
	}
	out := Answer{
		Tool:        Query,
		Version:     m.Version,
		Depth:       m.Depth,
		Unavailable: append([]string(nil), unavailableAtM2...),
		Partial:     partialOf(m.Programs...),
	}

	switch {
	case r.Subject == "" || r.Subject == SubjectRegistry || r.Subject == SubjectPrograms:
		out.Estate = m.Programs

	case r.Subject == SubjectEstate:
		// WHICH ESTATE THIS IS, AND IT IS THE ONE QUESTION THE AGENT SURFACE
		// COULD NOT ANSWER AT ALL. rig.estate is specified (section 37
		// precondition 1), built, and reachable from a terminal; the agent -
		// which section 37 calls the motivating caller - had no route to it,
		// because rig is held BESIDE the program map and invoke reaches the
		// map. That is not a missing feature, it is an inversion, and the
		// route out is query: section 9's tool for asking rig about rig.
		id, ok := s.estate()
		if !ok {
			out.Unavailable = append(out.Unavailable, "the estate's own identity")
			break
		}
		out.Identity = &id

	default:
		// A SUBJECT query CANNOT REACH IS NOT REFUSED, and the reasoning is
		// pinned by a test rather than left here: "a refusal would tell an
		// agent the subject does not exist. Saying where to look next is the
		// difference." Subjects are free-form prose at M2 - the pinned
		// example is "why did the nightly reindex fail" - so an unmatched one
		// usually means query cannot reach the source yet, not that the agent
		// mistyped a keyword. Unavailable above is the answer.
		//
		// WHAT IS STILL MISSING HERE IS THE OTHER HALF, and it is recorded
		// rather than fixed: the answer says what query CANNOT read and never
		// says what it CAN. An agent asking for the estate in prose lands in
		// this branch and is told about logs. That belongs in the tool's own
		// description, which is the thing an agent reads BEFORE calling.
	}
	return out, nil
}

// estate asks the invoker which estate this is, if it can say.
//
// The type assertion is where the optional half of Invoker is resolved, and
// it is done per call rather than at construction so that a surface built
// with a nil invoker is the same code path as one built with a daemon.
func (s *Server) estate() (Estate, bool) {
	e, ok := s.invoker.(EstateIdentity)
	if !ok {
		return Estate{}, false
	}
	return e.EstateIdentity(), true
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

// notFound says why an address did not resolve, and the SelfID case is why
// this function has a branch that is not about visibility.
//
// A REFUSAL MUST NAME THE RIGHT CAUSE. Asking for rig used to be answered
// `no program "rig" is visible to this caller`, which reads as a permissions
// problem: it sends an agent looking for a grant that does not exist and
// cannot be granted. rig is not hidden from this caller, it is not a program,
// and holding its declaration beside the registry rather than in it is the
// mechanism that keeps rig.down - declared destructive - off every invoke
// surface. So the refusal says that, and points at the tool that does answer.
func notFound(program, command string) error {
	if program == kernel.SelfID {
		return fmt.Errorf("meta: %q is not a program and is not an invoke "+
			"target; its own commands are held beside the registry, not in "+
			"it. Ask %s with subject %q for which estate this is",
			kernel.SelfID, Query, SubjectEstate)
	}
	if command == "" {
		// THREE DIFFERENT FACTS WEAR THIS ONE REFUSAL, AND ONLY ONE OF THEM
		// IS ANSWERED BY ASKING FOR MORE ACCESS. The address may be
		// unregistered, in the registry but outside this caller's scope, or
		// registered and gone since the caller last looked. The old wording
		// - "is not visible to this caller" - picked the middle one and
		// ASSERTED it, which is the case that sends an agent after a grant.
		// For the first it is a typo; for the third the program died and the
		// agent should be reacting to that, not filing an access request.
		//
		// SECTION 36's V20 IS THE RULE: "absent and withheld are different
		// facts and only one of them means ask for more access". meta cannot
		// tell them apart here - there is no tombstone at M2 and the
		// projection does not carry one - so it names the ambiguity instead
		// of resolving it by guess. That is the same discipline as the
		// coverage note: a surface that cannot see says so.
		//
		// IT DOES NOT ENUMERATE, deliberately. V20's basis-marker annotation
		// refuses a withheld LIST - the map says HOW it was filtered, never
		// WHAT was removed - so this says which QUESTION is open and points
		// at the basis, and never at a name the caller may not have.
		return fmt.Errorf("meta: no program %q is reachable by this caller. "+
			"rig cannot say which: unregistered, outside this caller's "+
			"scope, or gone since you last looked. Only the middle one is "+
			"fixed by a grant - %s reports the basis this caller's "+
			"projection was built on", program, List)
	}
	return fmt.Errorf("meta: %q declares no command %q, or it is not visible "+
		"to this caller", program, command)
}
