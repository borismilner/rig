package daemon

import (
	"errors"
	"fmt"

	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The wire and the kernel do not share types, and the translation lives here
// because here is the boundary.
//
// Section 21 says rig serves every wire major it has ever shipped, forever.
// If the kernel held rigv1 types, a v2 wire would reach into the kernel and
// the "forever" would cost a kernel change per major. Instead each major gets
// a translation into the same kernel vocabulary, and the kernel never learns
// that more than one exists.

var (
	coverageIn = map[rigv1.Coverage]kernel.Coverage{
		rigv1.Coverage_COVERAGE_PARTIAL: kernel.CoveragePartial,
		rigv1.Coverage_COVERAGE_FULL:    kernel.CoverageFull,
	}
	coverageOut = map[kernel.Coverage]rigv1.Coverage{
		kernel.CoveragePartial: rigv1.Coverage_COVERAGE_PARTIAL,
		kernel.CoverageFull:    rigv1.Coverage_COVERAGE_FULL,
	}
	effectsIn = map[rigv1.Effects]kernel.Effects{
		rigv1.Effects_EFFECTS_READ_ONLY:    kernel.EffectsReadOnly,
		rigv1.Effects_EFFECTS_WRITES_FILES: kernel.EffectsWritesFiles,
		rigv1.Effects_EFFECTS_NETWORK:      kernel.EffectsNetwork,
		rigv1.Effects_EFFECTS_DESTRUCTIVE:  kernel.EffectsDestructive,
		rigv1.Effects_EFFECTS_DRIVES_INPUT: kernel.EffectsDrivesInput,
	}
	effectsOut = map[kernel.Effects]rigv1.Effects{
		kernel.EffectsReadOnly:    rigv1.Effects_EFFECTS_READ_ONLY,
		kernel.EffectsWritesFiles: rigv1.Effects_EFFECTS_WRITES_FILES,
		kernel.EffectsNetwork:     rigv1.Effects_EFFECTS_NETWORK,
		kernel.EffectsDestructive: rigv1.Effects_EFFECTS_DESTRUCTIVE,
		kernel.EffectsDrivesInput: rigv1.Effects_EFFECTS_DRIVES_INPUT,
	}
	durationIn = map[rigv1.Duration]kernel.Duration{
		rigv1.Duration_DURATION_INSTANT: kernel.DurationInstant,
		rigv1.Duration_DURATION_SECONDS: kernel.DurationSeconds,
		rigv1.Duration_DURATION_MINUTES: kernel.DurationMinutes,
		rigv1.Duration_DURATION_HOURS:   kernel.DurationHours,
	}
	durationOut = map[kernel.Duration]rigv1.Duration{
		kernel.DurationInstant: rigv1.Duration_DURATION_INSTANT,
		kernel.DurationSeconds: rigv1.Duration_DURATION_SECONDS,
		kernel.DurationMinutes: rigv1.Duration_DURATION_MINUTES,
		kernel.DurationHours:   rigv1.Duration_DURATION_HOURS,
	}
	shapeIn = map[rigv1.Shape]kernel.Shape{
		rigv1.Shape_SHAPE_UNARY:              kernel.ShapeUnary,
		rigv1.Shape_SHAPE_STREAM:             kernel.ShapeStream,
		rigv1.Shape_SHAPE_INTERACTIVE_STREAM: kernel.ShapeInteractiveStream,
	}
	shapeOut = map[kernel.Shape]rigv1.Shape{
		kernel.ShapeUnary:             rigv1.Shape_SHAPE_UNARY,
		kernel.ShapeStream:            rigv1.Shape_SHAPE_STREAM,
		kernel.ShapeInteractiveStream: rigv1.Shape_SHAPE_INTERACTIVE_STREAM,
	}
	tristateIn = map[rigv1.Tristate]kernel.Tristate{
		rigv1.Tristate_TRISTATE_NO:  kernel.No,
		rigv1.Tristate_TRISTATE_YES: kernel.Yes,
	}
	tristateOut = map[kernel.Tristate]rigv1.Tristate{
		kernel.No:  rigv1.Tristate_TRISTATE_NO,
		kernel.Yes: rigv1.Tristate_TRISTATE_YES,
	}
)

// Each map above deliberately omits its zero. An unknown or unset enum value
// therefore translates to the kernel's own zero, which Declaration.Validate
// refuses by name - so a newer program sending a value this daemon has never
// heard of is refused rather than silently read as the first case.

// declarationFromWire translates a declaration and refuses one rig cannot
// reason about. The refusal is the program's, not the wire's: Validate names
// every missing property at once.
func declarationFromWire(req *rigv1.HelloRequest) (kernel.Declaration, error) {
	w := req.GetDeclaration()
	if w == nil {
		return kernel.Declaration{}, errors.New(
			"hello: no declaration. Registration is the handshake, not a later " +
				"call (section 5e): identity, coverage and semantics_gen are required")
	}
	id := w.GetIdentity()
	// program and version are on HelloRequest as well as inside identity, and
	// two places to say the same thing is two places to disagree.
	if req.GetProgram() != "" && req.GetProgram() != id.GetId() {
		return kernel.Declaration{}, fmt.Errorf(
			"hello: program %q does not match identity.id %q",
			req.GetProgram(), id.GetId())
	}
	if req.GetVersion() != "" && req.GetVersion() != id.GetVersion() {
		return kernel.Declaration{}, fmt.Errorf(
			"hello: version %q does not match identity.version %q",
			req.GetVersion(), id.GetVersion())
	}

	d := kernel.Declaration{
		Identity: kernel.Identity{
			ID:          id.GetId(),
			Name:        id.GetName(),
			Version:     id.GetVersion(),
			Icon:        id.GetIcon(),
			Description: id.GetDescription(),
		},
		Coverage:     coverageIn[w.GetCoverage()],
		CoverageNote: w.GetCoverageNote(),
		SemanticsGen: w.GetSemanticsGen(),
		Services:     w.GetServices(),
		Elements:     w.GetElements(),
		Preamble:     w.GetPreamble(),
		Scope:        w.GetScope(),
		Hosted:       w.GetHosted(),
		PaneURL:      w.GetPaneUrl(),
	}
	for _, c := range w.GetCommands() {
		d.Commands = append(d.Commands, commandFromWire(c))
	}
	if err := d.Validate(); err != nil {
		return kernel.Declaration{}, err
	}
	return d, nil
}

func commandFromWire(c *rigv1.Command) kernel.Command {
	out := kernel.Command{
		ID:            c.GetId(),
		Title:         c.GetTitle(),
		Args:          c.GetArgs(),
		Examples:      c.GetExamples(),
		Effects:       effectsIn[c.GetEffects()],
		Idempotent:    tristateIn[c.GetIdempotent()],
		Interactive:   tristateIn[c.GetInteractive()],
		Streams:       tristateIn[c.GetStreams()],
		NeedsDisplay:  tristateIn[c.GetNeedsDisplay()],
		Duration:      durationIn[c.GetDuration()],
		Confirms:      tristateIn[c.GetConfirms()],
		Shape:         shapeIn[c.GetShape()],
		Summary:       c.GetSummary(),
		Description:   c.GetDescription(),
		Returns:       c.GetReturns(),
		DryRun:        c.GetDryRun(),
		Cost:          c.GetCost(),
		Preconditions: c.GetPreconditions(),
		Promote:       c.GetPromote(),
	}
	// The wrapper is the whole point: a nil message means the program never
	// considered the question, an empty one means it did and the answer was
	// none. Validate refuses the first and accepts the second.
	if s := c.GetSensitive(); s != nil {
		out.Sensitive = s.GetPointers()
		if out.Sensitive == nil {
			out.Sensitive = []string{}
		}
	}
	return out
}

// depthIn reads the depth a caller asked for, and decides what an ABSENT one
// means.
//
// It means DEPTH_FULL, and that is a compatibility rule rather than a default
// worth having on its own. Every caller written before the field existed
// asked for the whole estate, and proto3 hands an old message the zero value
// whether the sender said anything or not - so reading zero as "the cheapest
// depth" would silently empty the commands list of every client that has not
// been recompiled.
//
// The kernel refuses an unspecified depth outright, and the two are meant to
// disagree: the kernel will not guess, and this is the one place that knows
// what the wire used to mean. Section 21's enum rule is about what a zero may
// MEAN in a decision; restoring a previous wire's behaviour at the boundary is
// not a decision the sender is being credited with.
func depthIn(d rigv1.Depth) kernel.Depth {
	switch d {
	case rigv1.Depth_DEPTH_PROGRAMS:
		return kernel.DepthPrograms
	case rigv1.Depth_DEPTH_COMMANDS:
		return kernel.DepthCommands
	default:
		return kernel.DepthFull
	}
}

// programToWire renders one program as a principal was allowed to see it.
func programToWire(p kernel.Program) *rigv1.Program {
	out := &rigv1.Program{
		Identity: &rigv1.Identity{
			Id:          p.Identity.ID,
			Name:        p.Identity.Name,
			Version:     p.Identity.Version,
			Icon:        p.Identity.Icon,
			Description: p.Identity.Description,
		},
		Coverage:     coverageOut[p.Coverage],
		CoverageNote: p.CoverageNote,
		SemanticsGen: p.SemanticsGen,
		Services:     p.Services,
		Elements:     p.Elements,
		Hosted:       p.Hosted,
		PaneUrl:      p.PaneURL,
		Preamble:     p.Preamble,
	}
	for _, c := range p.Commands {
		out.Commands = append(out.Commands, commandToWire(c))
	}
	return out
}

func commandToWire(c kernel.Command) *rigv1.Command {
	return &rigv1.Command{
		Id:            c.ID,
		Title:         c.Title,
		Args:          c.Args,
		Examples:      c.Examples,
		Effects:       effectsOut[c.Effects],
		Idempotent:    tristateOut[c.Idempotent],
		Sensitive:     &rigv1.SensitiveFields{Pointers: c.Sensitive},
		Interactive:   tristateOut[c.Interactive],
		Streams:       tristateOut[c.Streams],
		NeedsDisplay:  tristateOut[c.NeedsDisplay],
		Duration:      durationOut[c.Duration],
		Confirms:      tristateOut[c.Confirms],
		Shape:         shapeOut[c.Shape],
		Summary:       c.Summary,
		Description:   c.Description,
		Returns:       c.Returns,
		DryRun:        c.DryRun,
		Cost:          c.Cost,
		Preconditions: c.Preconditions,
		Promote:       c.Promote,
	}
}
