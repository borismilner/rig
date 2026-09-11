package kernel

import "fmt"

// Depth is how much of the estate a read wants (PLAN.md section 9).
//
// Section 9's whole argument for four meta tools is a context budget: fifteen
// programs with twenty commands each is three hundred tools, and handing an
// agent all of it "destroys its context before it has done anything". `list`
// therefore "returns the estate at whatever depth is asked for" and `describe`
// "returns one thing in full". Depth is that knob, and it lives in the kernel
// rather than in a surface so every surface asks the same question and gets
// the same answer.
//
// THE LINE BETWEEN THE DEPTHS IS ONE SENTENCE: DepthCommands carries what an
// agent picks a command BY; DepthFull carries what it calls the command WITH.
// Scalars and the one-line summary are how you choose; schemas, examples,
// preconditions and prose are how you invoke. Any other split invites a
// per-field argument every time a property is added.
//
// The zero value means "not said" and is refused, which is section 21's rule
// about enum zeros applied to a Go enum the proto gate does not reach. A depth
// that defaulted to one of the real values would make an unset field decode as
// a decision, and here the cheapest default would silently hide commands while
// the most complete one would silently defeat the budget.
type Depth uint8

const (
	DepthUnspecified Depth = iota

	// DepthPrograms is the estate itself: who is registered, how much of rig
	// each has adopted, and nothing about what they can do.
	DepthPrograms

	// DepthCommands adds every command's scalar properties and its summary -
	// enough to choose one, and nothing that costs a schema.
	DepthCommands

	// DepthFull is everything this principal may see, including the preamble
	// and each command's argument schema, examples and preconditions.
	DepthFull
)

var depthNames = map[Depth]string{
	DepthUnspecified: unspecifiedName,
	DepthPrograms:    "programs",
	DepthCommands:    "commands",
	DepthFull:        "full",
}

func (d Depth) String() string {
	if n, ok := depthNames[d]; ok {
		return n
	}
	return fmt.Sprintf("Depth(%d)", uint8(d))
}

// Valid refuses the zero and anything past the last depth.
func (d Depth) Valid() error {
	if d == DepthUnspecified {
		return fmt.Errorf("kernel: no depth was asked for; %s, %s and %s are "+
			"the depths, and an absent one is not the same as the cheapest",
			DepthPrograms, DepthCommands, DepthFull)
	}
	if d > DepthFull {
		return fmt.Errorf("kernel: %s is not a depth", d)
	}
	return nil
}

// ParseDepth reads a depth by the name a surface would be given.
func ParseDepth(s string) (Depth, error) {
	for d, n := range depthNames {
		if d != DepthUnspecified && n == s {
			return d, nil
		}
	}
	return DepthUnspecified, fmt.Errorf(
		"kernel: %q is not a depth; the depths are %s, %s and %s",
		s, DepthPrograms, DepthCommands, DepthFull)
}

// Estate is `list`: what this principal may reach, at the depth asked for.
//
// The scope filter is exactly the one Programs uses, and deliberately so - a
// depth decides how much is said about a program, never whether the program is
// mentioned. A cheaper read that quietly showed MORE of the estate would be a
// scope hole wearing a performance argument.
func (v View) Estate(d Depth) ([]Program, error) {
	if err := d.Valid(); err != nil {
		return nil, err
	}
	full := v.Programs()
	out := make([]Program, 0, len(full))
	for _, p := range full {
		out = append(out, atDepth(p, d))
	}
	return out, nil
}

// THERE IS NO Describe METHOD, AND THAT IS THE POINT OF PUTTING THE PREAMBLE
// ON Program. `describe` on a program is View.Program, which already returns
// one thing in full and now carries the preamble; `describe` on a command is
// View.Command, which already returns the whole declaration with its examples
// and effects. Adding Describe and DescribeCommand as aliases was the first
// shape of this file and it was wrong: two names for one read is how a later
// change gets made to one of them.

// atDepth trims one program to a depth. Full is the identity, so the
// projection has one source of truth and the depths only ever subtract.
func atDepth(p Program, d Depth) Program {
	if d >= DepthFull {
		return p
	}
	// The preamble is the one document an agent reads before touching a
	// program (section 9). It is prose, it can be long, and fifteen of them in
	// one list is the context cost this whole mechanism exists to avoid. It
	// belongs to describe.
	p.Preamble = ""
	if d == DepthPrograms {
		p.Commands = nil
		return p
	}
	p.Commands = commandsAtDepth(p.Commands)
	return p
}

// commandsAtDepth keeps what a command is chosen BY and drops what it is
// called WITH.
func commandsAtDepth(in []Command) []Command {
	if in == nil {
		return nil
	}
	out := make([]Command, len(in))
	for i, c := range in {
		c.Args = nil
		c.Examples = nil
		c.Preconditions = nil
		c.Sensitive = nil
		c.Description = ""
		c.Returns = ""
		out[i] = c
	}
	return out
}
