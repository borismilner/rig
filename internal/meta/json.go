package meta

import (
	"encoding/json"

	"github.com/boris-milner/rig/internal/kernel"
)

// MarshalAnswer renders an Answer as THE one object both agent-facing
// surfaces return.
//
// Section 10 requires `--json` to emit "exactly what the MCP tool returns".
// That is satisfied by one function, not by two renderers held in agreement:
// two renderings of one answer is the shape that reads as identical on the
// day it is written and diverges on the first field either one gains.
//
// It is NOT the socket wire's rendering, and that is deliberate rather than
// an oversight. The wire carries rigv1.Program through the daemon's own
// converters and is described by schema/declaration.schema.json. Section 10
// binds the CLI to the MCP tool; it does not bind either to the frame format,
// and an agent reading this object never sees a frame. Collapsing the two
// would make the socket's field numbering an agent-facing contract.
//
// The enums render as their names. A number would be smaller and is the wrong
// trade for a surface whose whole audience is a reader deciding what to do
// next - and section 21 gives every enum a zero meaning "nothing was said",
// which is legible as "unspecified" and silently wrong as 0.
func MarshalAnswer(a Answer) ([]byte, error) {
	return json.Marshal(answerJSON{
		Tool:        string(a.Tool),
		Partial:     incompleteListJSON(a.Partial),
		Depth:       a.Depth.String(),
		Version:     a.Version,
		Estate:      programListJSON(a.Estate),
		Program:     programPtrJSON(a.Program),
		Command:     commandPtrJSON(a.Command),
		Result:      rawOrNil(a.Result),
		Unavailable: a.Unavailable,
	})
}

// MarshalCapabilityMap renders the capability map as the content of the one
// MCP resource section 9 specifies ("Discovery is a resource, not a guess").
//
// IT SHARES THE PROGRAM AND COMMAND RENDERERS WITH MarshalAnswer RATHER THAN
// DESCRIBING A PROGRAM A SECOND TIME. This repository already carries two
// structural descriptions of a Program - programToWire/commandToWire in the
// daemon, and the view structs here - and nothing ties them together, so a
// field added to kernel.Command lands in one and not the other and presents
// weeks later as "the CLI shows it and MCP does not". A third description
// would be a third place for that. The top-level envelope differs because the
// two things genuinely differ: an Answer says which tool produced it, and a
// resource was not produced by a tool.
func MarshalCapabilityMap(m kernel.CapabilityMap) ([]byte, error) {
	programs := programListJSON(m.Programs)
	if programs == nil {
		// Never nil, for the reason Partial is never nil, and section 9 names
		// the case: "an empty estate is every fresh daemon, so it is the
		// first case a user meets". A null here is a third thing an agent has
		// to interpret, and an ABSENT programs cannot be told apart from a
		// server too old to have the field.
		programs = []programJSON{}
	}
	return json.Marshal(capabilityMapJSON{
		Version:  m.Version,
		Depth:    m.Depth.String(),
		Partial:  incompleteListJSON(partialOf(m.Programs...)),
		Programs: programs,
	})
}

// capabilityMapJSON is the resource's object. Every field is emitted always,
// and that is the whole of its shape argument.
type capabilityMapJSON struct {
	// Version is the map's identity: two maps with the same version are the
	// same map. It is a digest of the projection THIS principal was given,
	// never a counter on the registry - section 9 - so it is meaningless to
	// omit and always present.
	Version string `json:"version"`

	// Depth is emitted always, unspecified included, because the map must
	// STATE its depth rather than leave it inferable from what happens to be
	// in it. A reader inferring depth from content cannot tell "commands, and
	// this program has none" from "programs, so no commands were asked for".
	Depth string `json:"depth"`

	// Partial is emitted always, empty included, for the reason
	// answerJSON.Partial is: section 5k forbids a surface implying
	// completeness, and an absent partial cannot be told apart from a server
	// too old to have the field.
	Partial []incompleteJSON `json:"partial"`

	// Programs is emitted always, empty included. It is what the resource IS,
	// and an empty estate is a real and common answer rather than a missing
	// one.
	Programs []programJSON `json:"programs"`
}

type answerJSON struct {
	Tool string `json:"tool"`

	// Partial is ALWAYS emitted, empty included, and that is the one field
	// here worth arguing about.
	//
	// Section 5k forbids a surface implying completeness. An absent `partial`
	// cannot be told apart from a server too old to have the field, so an
	// agent reading one has to guess which it is - and the safe guess and the
	// useful guess point opposite ways. Emitted always, `[]` means "nothing
	// here is incomplete" and says so.
	Partial []incompleteJSON `json:"partial"`

	// Depth is ALWAYS emitted, unspecified included, for the same reason
	// Partial is.
	//
	// Section 10 binds --json to exactly what the MCP tool returns, and
	// --depth is a knob an agent turns to control its own context budget. An
	// object that cannot say how much was asked for leaves the agent to infer
	// it from which fields happen to be populated, and an ABSENT depth cannot
	// be told apart from a server too old to have the field. Emitted always,
	// "unspecified" says "no depth was involved" and says it out loud.
	Depth string `json:"depth"`

	Version     string          `json:"version,omitempty"`
	Estate      []programJSON   `json:"estate,omitempty"`
	Program     *programJSON    `json:"program,omitempty"`
	Command     *commandJSON    `json:"command,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Unavailable []string        `json:"unavailable,omitempty"`
}

// incompleteJSON is one program admitting how much of rig it has adopted.
type incompleteJSON struct {
	Program  string `json:"program"`
	Coverage string `json:"coverage"`
	Note     string `json:"note,omitempty"`
}

type identityJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
}

type programJSON struct {
	Identity     identityJSON  `json:"identity"`
	Coverage     string        `json:"coverage"`
	CoverageNote string        `json:"coverageNote,omitempty"`
	SemanticsGen int32         `json:"semanticsGen,omitempty"`
	Services     []string      `json:"services,omitempty"`
	Elements     []string      `json:"elements,omitempty"`
	Hosted       bool          `json:"hosted,omitempty"`
	PaneURL      string        `json:"paneUrl,omitempty"`
	Preamble     string        `json:"preamble,omitempty"`
	Commands     []commandJSON `json:"commands,omitempty"`
}

type commandJSON struct {
	ID       string          `json:"id"`
	Title    string          `json:"title,omitempty"`
	Args     json.RawMessage `json:"args,omitempty"`
	Examples []string        `json:"examples,omitempty"`

	Effects      string   `json:"effects"`
	Idempotent   string   `json:"idempotent"`
	Sensitive    []string `json:"sensitive,omitempty"`
	Interactive  string   `json:"interactive"`
	Streams      string   `json:"streams"`
	NeedsDisplay string   `json:"needsDisplay"`
	Duration     string   `json:"duration"`
	Confirms     string   `json:"confirms"`
	Shape        string   `json:"shape"`
	Summary      string   `json:"summary,omitempty"`
	Description  string   `json:"description,omitempty"`
	Returns      string   `json:"returns,omitempty"`

	DryRun        bool     `json:"dryRun,omitempty"`
	Cost          string   `json:"cost,omitempty"`
	Preconditions []string `json:"preconditions,omitempty"`
	Promote       bool     `json:"promote,omitempty"`
}

func incompleteListJSON(in []Incomplete) []incompleteJSON {
	// Never nil: a nil slice marshals as null, and null is a third thing an
	// agent then has to interpret. See answerJSON.Partial.
	out := make([]incompleteJSON, 0, len(in))
	for _, i := range in {
		out = append(out, incompleteJSON{
			Program:  i.Program,
			Coverage: i.Coverage.String(),
			Note:     i.Note,
		})
	}
	return out
}

func programListJSON(in []kernel.Program) []programJSON {
	if len(in) == 0 {
		return nil
	}
	out := make([]programJSON, 0, len(in))
	for _, p := range in {
		out = append(out, oneProgramJSON(p))
	}
	return out
}

func programPtrJSON(p *kernel.Program) *programJSON {
	if p == nil {
		return nil
	}
	out := oneProgramJSON(*p)
	return &out
}

func commandPtrJSON(c *kernel.Command) *commandJSON {
	if c == nil {
		return nil
	}
	out := oneCommandJSON(*c)
	return &out
}

func oneProgramJSON(p kernel.Program) programJSON {
	out := programJSON{
		Identity: identityJSON{
			ID:          p.Identity.ID,
			Name:        p.Identity.Name,
			Version:     p.Identity.Version,
			Icon:        p.Identity.Icon,
			Description: p.Identity.Description,
		},
		Coverage:     p.Coverage.String(),
		CoverageNote: p.CoverageNote,
		SemanticsGen: p.SemanticsGen,
		Services:     p.Services,
		Elements:     p.Elements,
		Hosted:       p.Hosted,
		PaneURL:      p.PaneURL,
		Preamble:     p.Preamble,
	}
	for _, c := range p.Commands {
		out.Commands = append(out.Commands, oneCommandJSON(c))
	}
	return out
}

func oneCommandJSON(c kernel.Command) commandJSON {
	return commandJSON{
		ID:            c.ID,
		Title:         c.Title,
		Args:          rawOrNil(c.Args),
		Examples:      c.Examples,
		Effects:       c.Effects.String(),
		Idempotent:    c.Idempotent.String(),
		Sensitive:     c.Sensitive,
		Interactive:   c.Interactive.String(),
		Streams:       c.Streams.String(),
		NeedsDisplay:  c.NeedsDisplay.String(),
		Duration:      c.Duration.String(),
		Confirms:      c.Confirms.String(),
		Shape:         c.Shape.String(),
		Summary:       c.Summary,
		Description:   c.Description,
		Returns:       c.Returns,
		DryRun:        c.DryRun,
		Cost:          c.Cost,
		Preconditions: c.Preconditions,
		Promote:       c.Promote,
	}
}

// rawOrNil keeps a declared JSON document as JSON.
//
// A []byte marshals to base64 through encoding/json, which would turn a
// command's argument schema - the thing an agent needs in order to CALL it -
// into an opaque string. Empty stays nil so the field is omitted rather than
// emitted as invalid JSON.
func rawOrNil(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}
