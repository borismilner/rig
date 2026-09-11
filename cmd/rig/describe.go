package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// Command `describe` - one thing in full (PLAN.md sections 9 and 10, M2 slice 2).
//
// WHAT SEPARATES IT FROM `--help`, because the two read alike and are not:
// help is the USABLE subset and describe is the COMPLETE declaration. Help
// answers "how do I call this" and leaves out everything that does not help a
// person type a command; describe answers "what did this program actually say
// about itself", which is what an agent deciding whether a command belongs on
// a surface needs, and it therefore renders every declared field including the
// ones with nothing in them.
//
// THE PREAMBLE IS THE POINT. Section 9 makes it "the one document an agent
// reads first" and requires it "returned by describe on the program". A
// program has been able to declare one since M1 - accepted at hello,
// validated, stored - and until 6c71712 no message on any path could carry it
// back. It was a dead field for as long as it existed, and this verb is the
// only surface that reads it.
//
// `--json` IS DELIBERATELY ABSENT, and it is OWED rather than dropped. Section
// 10 requires --json to return exactly what the MCP tool returns, and B10
// rules that the daemon renders those bytes with meta.MarshalAnswer and the
// CLI prints them through. A hand-written renderer here would be a second
// implementation of that object, shipped from a verb that has no callers to
// break, and B10 would delete it. Shipping the flag WRONG is worse than
// shipping it later.
func cmdDescribe(args []string) (err error) {
	fs, timeout, asJSON := describeFlagSet()
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	// THE REFUSAL RENDERS IN THE MODE IT WAS ASKED IN, even though this verb
	// has no --json answer. An agent that asked for a machine-readable answer
	// gets a machine-readable refusal; section 10's promise is --json on every
	// ERROR as much as on every result.
	defer func() { err = inMode(err, *asJSON) }()

	// --json IS DECLARED IN ORDER TO BE REFUSED, and that is the whole reason
	// it is here. Left undeclared, `rig describe fakeapp --json` dies with the
	// flag package's own "flag provided but not defined: -json" and a usage
	// dump, which reads as a bug in rig rather than as a deliberate absence -
	// and an agent reading section 10's "--json on every command" will type it
	// on its first attempt. This seat's own sweep already found that shape
	// once, on `rig completion --json`, and a promise the CLI cannot keep is
	// worth one explicit sentence rather than a parser error.
	if *asJSON {
		return local(jsonStatus{
			Code: codeBadArgument,
			Message: "rig describe has no --json yet: its object is the one " +
				"the MCP tool returns, and that object is rendered by the " +
				"daemon rather than by this client",
			Precondition: "the verb being asked for --json emits one",
			Actual:       "describe emits the human rendering only",
			Fix: "read it as text, or use the MCP describe tool for the " +
				"object",
			FixCommand: "rig describe " + strings.Join(positional, " "),
		})
	}

	if len(positional) == 0 || len(positional) > 2 {
		return badArgumentf(
			"usage: rig describe <program> [<command>] [--timeout=30s]")
	}

	c, err := client.Connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// DEPTH_FULL asked for explicitly. describe is defined as "one thing in
	// full", and the preamble travels at DEPTH_FULL only.
	p, err := programAt(ctx, c, positional[0], rigv1.Depth_DEPTH_FULL)
	if err != nil {
		return err
	}

	if len(positional) == 1 {
		fmt.Print(describeProgram(p))
		return nil
	}

	command := positional[1]
	for _, cmd := range p.GetCommands() {
		if cmd.GetId() == command {
			fmt.Print(describeCommand(p, cmd))
			return nil
		}
	}
	return noSuchCommand(p, command)
}

func describeFlagSet() (fs *flag.FlagSet, timeout *time.Duration, asJSON *bool) {
	fs = flag.NewFlagSet("describe", flag.ContinueOnError)
	timeout = fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	asJSON = fs.Bool("json", false,
		"not emitted yet: describe's object is rendered by the daemon")
	return fs, timeout, asJSON
}

// ---- the program ----------------------------------------------------------

// describeProgram renders what a program said about itself, preamble first.
func describeProgram(p *rigv1.Program) string {
	var b strings.Builder
	id := p.GetIdentity()

	fmt.Fprintf(&b, "%s %s", id.GetId(), id.GetVersion())
	if d := firstNonEmpty(id.GetDescription(), id.GetName()); d != "" {
		fmt.Fprintf(&b, " - %s", d)
	}
	fmt.Fprintln(&b)

	// THE PREAMBLE, AND ITS ABSENCE IS STATED RATHER THAN LEFT BLANK.
	//
	// This verb exists to deliver it, so silence here reads as "describe is
	// broken" rather than as "this program declared none" - the same
	// distinction the estate verb spends a whole enum value on, arriving
	// through prose instead of through a field.
	fmt.Fprintln(&b)
	if pre := strings.TrimSpace(p.GetPreamble()); pre != "" {
		fmt.Fprintf(&b, "%s\n", wrap(pre, 72))
	} else {
		fmt.Fprintf(&b, "it declares no preamble, so there is no document to "+
			"read before\nusing %s\n", id.GetId())
	}

	// Section 5k: no surface may imply completeness, and this one renders a
	// whole program.
	fmt.Fprintf(&b, "\ncoverage is %s", coverageLabel(p))
	if note := p.GetCoverageNote(); note != "" {
		fmt.Fprintf(&b, " - %s", note)
	}
	fmt.Fprintf(&b, "\nsemantics generation %d\n", p.GetSemanticsGen())
	if svc := p.GetServices(); len(svc) > 0 {
		fmt.Fprintf(&b, "services %s\n", strings.Join(svc, ", "))
	}

	cmds := append([]*rigv1.Command(nil), p.GetCommands()...)
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].GetId() < cmds[j].GetId() })
	fmt.Fprintln(&b)
	if len(cmds) == 0 {
		fmt.Fprintln(&b, "it declares no commands")
		return b.String()
	}

	fmt.Fprintf(&b, "commands (%d):\n", len(cmds))
	w := 0
	for _, cmd := range cmds {
		w = max(w, len(cmd.GetId()))
	}
	for _, cmd := range cmds {
		fmt.Fprintf(&b, "  %-*s  %-12s  %s\n", w, cmd.GetId(),
			effectsWord(cmd.GetEffects()), cmd.GetSummary())
	}
	fmt.Fprintf(&b, "\nrig describe %s <command>  one command in full\n", id.GetId())
	return b.String()
}

// ---- the command ----------------------------------------------------------

// describeCommand renders one command's whole declaration.
//
// EVERY DECLARED PROPERTY IS RENDERED, INCLUDING THE ONES THE PROGRAM DID NOT
// SET, and that is the difference from `--help`. A field a program left unsaid
// is a fact about the program, and it is the fact section 21 gives every enum
// a zero in order to keep. Help may leave it out because a person is reading
// to type a command; describe may not, because its reader is deciding whether
// the command belongs on a surface at all.
func describeCommand(p *rigv1.Program, c *rigv1.Command) string {
	var b strings.Builder
	program := p.GetIdentity().GetId()

	fmt.Fprintf(&b, "rig %s %s", program, c.GetId())
	if s := firstNonEmpty(c.GetSummary(), c.GetTitle()); s != "" {
		fmt.Fprintf(&b, " - %s", s)
	}
	fmt.Fprintln(&b)
	if d := c.GetDescription(); d != "" && d != c.GetSummary() {
		fmt.Fprintf(&b, "\n%s\n", wrap(d, 72))
	}

	fmt.Fprintln(&b)
	flags, err := describeFlags(c.GetArgs())
	switch {
	case err != nil:
		// An unreadable schema is reported rather than swallowed: describe is
		// the surface that says what a program declared, and "it declared
		// something rig cannot parse" is one of the answers.
		fmt.Fprintf(&b, "arguments: the declared schema is not readable (%v)\n", err)
	case len(flags) == 0:
		fmt.Fprintln(&b, "it takes no arguments")
	default:
		fmt.Fprintln(&b, "arguments:")
		w := 0
		for _, f := range flags {
			w = max(w, len(f.usage))
		}
		for _, f := range flags {
			fmt.Fprintf(&b, "  %-*s  %s\n", w, f.usage, f.help)
		}
	}

	fmt.Fprintln(&b)
	// A wrapped value's continuation lines are indented to the value column.
	// Without it a long `returns` starts its second line at column zero, where
	// it reads as a new label rather than as more of the same field - the same
	// defect refusal.go's detail() already solves, and it looks like a
	// rendering bug rather than like prose.
	row := func(label, value string) {
		indented := strings.ReplaceAll(value, "\n",
			"\n"+strings.Repeat(" ", describeColumn))
		fmt.Fprintf(&b, "%-*s%s\n", describeColumn, label, indented)
	}
	row("effects", effectsWord(c.GetEffects()))
	row("duration", durationWord(c.GetDuration()))
	row("shape", shapeWord(c.GetShape()))
	row("idempotent", tristateWord(c.GetIdempotent()))
	row("interactive", tristateWord(c.GetInteractive()))
	row("streams", tristateWord(c.GetStreams()))
	row("needs display", tristateWord(c.GetNeedsDisplay()))
	row("confirms", tristateWord(c.GetConfirms()))
	row("dry run", boolWord(c.GetDryRun()))
	row("promoted", boolWord(c.GetPromote()))
	if cost := c.GetCost(); cost != "" {
		row("cost", cost)
	}
	if r := c.GetReturns(); r != "" {
		row("returns", wrap(r, 72-describeColumn))
	}

	if pre := c.GetPreconditions(); len(pre) > 0 {
		fmt.Fprintln(&b, "\npreconditions:")
		for _, s := range pre {
			fmt.Fprintf(&b, "  %s\n", s)
		}
	}
	if ex := c.GetExamples(); len(ex) > 0 {
		fmt.Fprintln(&b, "\nexamples:")
		for _, e := range ex {
			fmt.Fprintf(&b, "  %s\n", e)
		}
	}

	// SENSITIVE FIELDS ARE STATED EITHER WAY, and this one is not a tidiness
	// choice. "This command declares no sensitive fields" and "nobody has
	// looked at whether it has any" are different facts, and a reader deciding
	// what may be logged needs the first said out loud rather than inferred
	// from a missing heading.
	fmt.Fprintln(&b)
	if ptr := pointers(c.GetSensitive()); len(ptr) > 0 {
		fmt.Fprintln(&b, "sensitive fields:")
		for _, s := range ptr {
			fmt.Fprintf(&b, "  %s\n", s)
		}
	} else {
		fmt.Fprintln(&b, "it declares no sensitive fields")
	}
	return b.String()
}

// describeColumn is where a property's value starts. Thirteen is the longest
// label ("needs display") plus two spaces of gutter.
const describeColumn = 13 + 2

// ---- words for enums, and none of them may collapse ------------------------

// tristateWord renders a tristate as THREE distinct words.
//
// THIS IS THE COLLAPSE THIS PACKAGE ALREADY PINS, NOT REPEATED. appsJSON
// renders four tristates as `== TRISTATE_YES`, so NO and UNSPECIFIED produce
// the same token and an agent reading `false` cannot tell "the program said
// no" from "the program never said" - the one distinction section 21 gives
// every enum a zero in order to keep. That collapse is in SHIPPED output and
// is batched into B6 rather than fixed piecemeal.
//
// `describe` is a new verb with no caller to break, so it does not inherit it.
// "not said" is a third word, deliberately not a blank and deliberately not a
// dash: a reader skimming a column of yes and no reads an empty cell as a
// rendering gap rather than as the program's silence.
func tristateWord(t rigv1.Tristate) string {
	switch t {
	case rigv1.Tristate_TRISTATE_YES:
		return "yes"
	case rigv1.Tristate_TRISTATE_NO:
		return "no"
	case rigv1.Tristate_TRISTATE_UNSPECIFIED:
		return "not said"
	default:
		return skewWord(t, "tristate")
	}
}

// boolWord renders a plain bool. It is NOT a tristate and says so by using
// different words: `dry_run` and `promote` are `bool` on the wire, so false
// genuinely means false and there is no third state to lose.
func boolWord(v bool) string {
	if v {
		return "declared"
	}
	return "not declared"
}

func effectsWord(e rigv1.Effects) string   { return enumWord(e, "EFFECTS_", "effect") }
func durationWord(d rigv1.Duration) string { return enumWord(d, "DURATION_", "duration") }
func shapeWord(s rigv1.Shape) string       { return enumWord(s, "SHAPE_", "shape") }

// enumWord renders any wire enum as a word off its own descriptor, and a
// number this build has no word for as SKEW rather than as the zero.
//
// The generated String() is deliberately not used. For a value outside the
// descriptor it returns the DECIMAL, so a newer daemon's fifth effect prints
// as "4" in a column of words - skew DISCOVERED by a person squinting at
// output, where section 37's precondition 3 requires it DETECTED. This is the
// same form the estate verb uses for its role, and it is applied here because
// `describe` is new; effectsLabel and durationLabel still carry the bare-digit
// behaviour in SHIPPED surfaces and are a backlog row of their own.
func enumWord(e protoreflect.Enum, prefix, kind string) string {
	v := e.Descriptor().Values().ByNumber(e.Number())
	if v == nil {
		return skewWord(e, kind)
	}
	label := enumLabel(string(v.Name()), prefix)
	// The zero is "nothing was said" for every enum section 21 covers, and it
	// must not read as a value the program chose.
	if e.Number() == 0 {
		return "not said"
	}
	return label
}

// skewWord is what a value this build has no name for renders as. It names the
// number, because that is the only actionable thing in it, and says which side
// is old so the reader does not go looking at the daemon.
func skewWord(e protoreflect.Enum, kind string) string {
	return fmt.Sprintf("unrecognised %s %d - this rig is older than the daemon "+
		"it reached", kind, e.Number())
}

// noSuchCommand is describe's refusal, and it carries the program's coverage
// for the same reason lookup's does: section 5k, "shelf declares 3 of its 20
// commands" is the difference between a typo and a command that exists and has
// not been adopted.
func noSuchCommand(p *rigv1.Program, command string) error {
	program := p.GetIdentity().GetId()
	return local(jsonStatus{
		Code:         codeNoSuchCommand,
		Message:      fmt.Sprintf("%s declares no command %q", program, command),
		Precondition: fmt.Sprintf("%s declares a command named %q", program, command),
		Actual: fmt.Sprintf("it declares: %s\ncoverage is %s, so this may be a "+
			"command it has not adopted yet",
			strings.Join(commandIDs(p), ", "), coverageLabel(p)),
		Fix:        "describe one it declares, or read the program first",
		FixCommand: "rig describe " + program,
	})
}
