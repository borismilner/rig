package kernel

import (
	"fmt"
	"slices"
	"strings"
)

// Declaration is what a program declares, once, at connect (PLAN.md section
// 5e). It is data: no rig code runs inside the program to produce it.
//
// Only Identity, Coverage, SemanticsGen and the mandatory command properties
// are required. A program that declares nothing but commands still gets a
// CLI, an MCP tool, an HTTP route, a palette entry, a tray item and a
// schedulable job - each one wherever its declared properties allow it.
type Declaration struct {
	Identity     Identity
	Coverage     Coverage
	CoverageNote string
	SemanticsGen int32
	Services     []string
	Preamble     string
	Commands     []Command
	Scope        string
	Hosted       bool

	// PaneURL is where this program serves its own HTML for the window's
	// pane (section 11). Empty means it declares no pane.
	//
	// Loopback only, and checked here - at registration - rather than when
	// the window comes to draw it. Whatever is in this field is what a
	// webview inside rig will load, so a program that could name any origin
	// could point the window at anything. Section 5h's R7 puts the same class
	// of check at registration, because a refusal at render happens in front
	// of the user.
	PaneURL string
}

// Identity is who the program is.
type Identity struct {
	ID          string
	Name        string
	Version     string
	Icon        string
	Description string
}

// Coverage says how much of rig this program has adopted (section 5k).
// Adoption is per service, so a program is never blocked waiting to adopt
// everything, and it says so rather than being assumed complete.
type Coverage uint8

const (
	CoverageUnspecified Coverage = iota
	CoveragePartial
	CoverageFull
)

var coverageNames = map[Coverage]string{
	CoverageUnspecified: unspecifiedName,
	CoveragePartial:     "partial",
	CoverageFull:        "full",
}

func (c Coverage) String() string {
	if n, ok := coverageNames[c]; ok {
		return n
	}
	return fmt.Sprintf("Coverage(%d)", uint8(c))
}

// Command is one thing a program can be asked to do.
//
// It declares properties and never surfaces. Section 5e calls this the single
// most important correction in the document: a command that named its
// surfaces made the surface set a closed vocabulary inside the registration
// schema, so every surface added later cost every program an edit.
type Command struct {
	ID       string
	Title    string
	Args     []byte // JSON Schema for the arguments
	Examples []string

	// Mandatory. A registration missing any of these is refused.
	Effects      Effects
	Idempotent   Tristate
	Sensitive    []string // JSON pointers into arguments and results
	Interactive  Tristate
	Streams      Tristate
	NeedsDisplay Tristate
	Duration     Duration
	Confirms     Tristate
	Shape        Shape
	Summary      string
	Description  string
	Returns      string

	// Optional planning and promotion hints.
	DryRun        bool
	Cost          string
	Preconditions []string
	Promote       bool
}

// Tristate is a mandatory boolean that has to have been said.
//
// A plain bool cannot express "the program never declared this", and section
// 5e is explicit that no property may have a default carrying a safety
// meaning: a field whose absence means "safe" can never have its default
// changed without lying about every declaration written before the change.
type Tristate uint8

const (
	Unsaid Tristate = iota
	No
	Yes
)

func (t Tristate) String() string {
	switch t {
	case No:
		return "no"
	case Yes:
		return "yes"
	default:
		return "unsaid"
	}
}

// Bool reads a Tristate that has been validated as said.
func (t Tristate) Bool() bool { return t == Yes }

// Effects is what running a command does to the world.
type Effects uint8

const (
	EffectsUnspecified Effects = iota
	EffectsReadOnly
	EffectsWritesFiles
	EffectsNetwork
	EffectsDestructive
	EffectsDrivesInput
)

// EffectsCeiling is the most dangerous level that exists, and it is a name
// rather than a literal on purpose.
//
// Anything rig cannot resolve has to be treated as the worst thing it could be
// (see RefOpaque). That was written as EffectsDestructive while destructive was
// the top of the order, and adding a level above it silently turned "assume the
// worst" into "assume the second worst" - an opaque call would have escaped a
// rule that denied the new level. The ceiling is declared here so the next
// value added cannot reintroduce that, and the test that asserts it names this
// constant rather than a member.
const EffectsCeiling = EffectsDrivesInput

var effectsNames = map[Effects]string{
	EffectsUnspecified: unspecifiedName,
	EffectsReadOnly:    "read-only",
	EffectsWritesFiles: "writes-files",
	EffectsNetwork:     "network",
	EffectsDestructive: "destructive",
	EffectsDrivesInput: "drives-input",
}

func (e Effects) String() string {
	if n, ok := effectsNames[e]; ok {
		return n
	}
	return fmt.Sprintf("Effects(%d)", uint8(e))
}

// ParseEffects reads the name a house rule or a declaration writes.
func ParseEffects(s string) (Effects, error) {
	for e, n := range effectsNames {
		if n == s && e != EffectsUnspecified {
			return e, nil
		}
	}
	return EffectsUnspecified, fmt.Errorf("kernel: %q is not an effects value", s)
}

// AtLeastAsDangerousAs orders effects for house-rule matching, so a rule
// written against one level covers everything above it.
func (e Effects) AtLeastAsDangerousAs(other Effects) bool { return e >= other }

// Duration is an order of magnitude, not an estimate. It is what tells a
// surface whether a command belongs on it at all.
type Duration uint8

const (
	DurationUnspecified Duration = iota
	DurationInstant
	DurationSeconds
	DurationMinutes
	DurationHours
)

var durationNames = map[Duration]string{
	DurationUnspecified: unspecifiedName,
	DurationInstant:     "instant",
	DurationSeconds:     "seconds",
	DurationMinutes:     "minutes",
	DurationHours:       "hours",
}

func (d Duration) String() string {
	if n, ok := durationNames[d]; ok {
		return n
	}
	return fmt.Sprintf("Duration(%d)", uint8(d))
}

// Shape is what a result is.
type Shape uint8

const (
	ShapeUnspecified Shape = iota
	ShapeUnary
	ShapeStream
	ShapeInteractiveStream
)

var shapeNames = map[Shape]string{
	ShapeUnspecified:       unspecifiedName,
	ShapeUnary:             "unary",
	ShapeStream:            "stream",
	ShapeInteractiveStream: "interactive-stream",
}

func (s Shape) String() string {
	if n, ok := shapeNames[s]; ok {
		return n
	}
	return fmt.Sprintf("Shape(%d)", uint8(s))
}

// Validate refuses a declaration rig cannot reason about.
//
// It reports every problem it finds rather than the first, because a program
// author fixing a generated declaration one error per run is a program author
// who stops generating it.
func (d Declaration) Validate() error {
	var bad []string
	add := func(f string, a ...any) { bad = append(bad, fmt.Sprintf(f, a...)) }

	if d.Identity.ID == "" {
		add("identity.id is empty")
	} else if reserved[d.Identity.ID] {
		add("identity.id %q is reserved for rig itself", d.Identity.ID)
	} else if bad := badID(d.Identity.ID); bad != "" {
		add("identity.id %q %s", d.Identity.ID, bad)
	}

	// A program does not choose which scope it is in.
	//
	// Register would otherwise put the declared value straight into the
	// principal's scope set, so a program declaring scope "beta" would read
	// beta's declaration - the seventh view leaking, from attacker-controlled
	// input, with nothing behind it. Joining a scope is what a crew is, and a
	// crew is section 16 at M7. Until then the only scope is a program's own
	// id, and asking for another is refused rather than quietly ignored: a
	// program that asked for something and did not get it should be told.
	if d.Scope != "" && d.Scope != d.Identity.ID {
		add("scope %q is not this program's own id: a program does not choose "+
			"its scope, and joining one is the peers service at M7 (section 16)",
			d.Scope)
	}
	if d.Identity.Version == "" {
		add("identity.version is empty")
	}
	if d.Coverage == CoverageUnspecified {
		add("coverage is unspecified: say partial or full (section 5k)")
	}
	if d.SemanticsGen <= 0 {
		add("semantics_gen is %d: it pins what every declared name means for "+
			"this program's lifetime, so it cannot be absent (section 21)",
			d.SemanticsGen)
	}

	if err := validatePaneURL(d.PaneURL); err != nil {
		add("%s", err)
	}

	seen := map[string]bool{}
	for i, c := range d.Commands {
		where := fmt.Sprintf("commands[%d]", i)
		if c.ID != "" {
			where = fmt.Sprintf("command %q", c.ID)
			if seen[c.ID] {
				add("%s is declared twice", where)
			}
			seen[c.ID] = true
		} else {
			add("%s has no id", where)
		}
		for _, p := range c.missing() {
			add("%s: %s", where, p)
		}
	}
	if len(bad) > 0 {
		slices.Sort(bad)
		return fmt.Errorf("kernel: registration refused:\n  - %s",
			strings.Join(bad, "\n  - "))
	}
	return nil
}

// reserved is rig's own namespace, which no program may take.
var reserved = map[string]bool{"rig": true, "rigd": true}

// badID says why an id cannot be used, or returns empty.
//
// A method is <program>.<command> and the split takes the FIRST dot, so a
// program whose id contains one is a program none of whose commands can ever
// be addressed. It registers happily and is then unreachable, which is worse
// than being refused.
func badID(id string) string {
	if strings.ContainsRune(id, '.') {
		return "contains a dot: a method is <program>.<command> and the split " +
			"takes the first one, so this program's commands would be unaddressable"
	}
	if strings.TrimSpace(id) != id || strings.ContainsAny(id, " \t\n\r") {
		return "has surrounding or embedded whitespace"
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return "contains a control character"
		}
	}
	return ""
}

// missing lists the mandatory properties this command did not declare.
func (c Command) missing() []string {
	var out []string
	if c.Effects == EffectsUnspecified {
		out = append(out, "effects is mandatory and has no safe default")
	}
	if c.Idempotent == Unsaid {
		out = append(out, "idempotent is mandatory: it decides retry, replay "+
			"and schedule coalescing")
	}
	if c.Sensitive == nil {
		out = append(out, "sensitive is mandatory and may be empty, but an "+
			"absent list is not an empty one (section 15)")
	}
	if c.Interactive == Unsaid {
		out = append(out, "interactive is mandatory")
	}
	if c.Streams == Unsaid {
		out = append(out, "streams is mandatory")
	}
	if c.NeedsDisplay == Unsaid {
		out = append(out, "needs_display is mandatory")
	}
	if c.Duration == DurationUnspecified {
		out = append(out, "duration is mandatory")
	}
	if c.Confirms == Unsaid {
		out = append(out, "confirms is mandatory")
	}
	if c.Shape == ShapeUnspecified {
		out = append(out, "shape is mandatory")
	}
	if c.Summary == "" {
		out = append(out, "summary is mandatory: section 9 writes it for a "+
			"reader who has never seen this program")
	}
	if c.Description == "" {
		out = append(out, "description is mandatory")
	}
	if c.Returns == "" {
		out = append(out, "returns is mandatory")
	}
	return out
}
