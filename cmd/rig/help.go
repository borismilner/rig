package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// Generated help (PLAN.md section 10, M1 slice 6).
//
// Every line below comes from the registry or from the declared JSON Schema.
// Nothing about any program is written down here, and that is the whole test:
// `rig fakeapp --help` lists what fakeapp declared and nothing else, so a
// program that adds a command gets help for it without rig being rebuilt.
//
// It is also why help has to reach the daemon. A CLI that could print its
// help offline would be a CLI with a second, stale copy of every
// declaration.

// asksForHelp reports whether argv is asking for help rather than a call.
func asksForHelp(argv []string) bool {
	for _, a := range argv {
		switch a {
		case "--help", "-h", "help":
			return true
		case "--":
			return false
		}
	}
	return false
}

// helpForProgram lists what one program declared.
func helpForProgram(program string) error {
	c, err := client.Connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p, err := lookupProgram(ctx, c, program)
	if err != nil {
		return err
	}

	fmt.Printf("%s %s - %s\n", p.GetIdentity().GetId(), p.GetIdentity().GetVersion(),
		firstNonEmpty(p.GetIdentity().GetDescription(), p.GetIdentity().GetName()))
	fmt.Printf("\nusage: rig %s <command> [flags]\n\n", program)

	cmds := p.GetCommands()
	if len(cmds) == 0 {
		fmt.Println("it declares no commands")
		return nil
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].GetId() < cmds[j].GetId() })

	w := 0
	for _, cmd := range cmds {
		w = max(w, len(cmd.GetId()))
	}
	for _, cmd := range cmds {
		fmt.Printf("  %-*s  %-12s  %s\n", w, cmd.GetId(),
			effectsLabel(cmd), cmd.GetSummary())
	}

	// Section 5k: no surface may imply completeness. This IS a surface, and
	// it is the one most likely to be mistaken for the whole of a program.
	fmt.Printf("\ncoverage is %s", coverageLabel(p))
	if note := p.GetCoverageNote(); note != "" {
		fmt.Printf(" - %s", note)
	}
	fmt.Printf("\nso this is what %s has declared to rig, not everything it can do.\n",
		program)
	fmt.Printf("\nrig %s <command> --help  what one command takes\n", program)
	return nil
}

// helpForCommand prints one command's declared arguments.
func helpForCommand(program, command string) error {
	c, err := client.Connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decl, err := lookup(ctx, c, program, command)
	if err != nil {
		return err
	}

	fmt.Printf("rig %s %s - %s\n", program, command, decl.GetSummary())
	if d := decl.GetDescription(); d != "" && d != decl.GetSummary() {
		fmt.Printf("\n%s\n", wrap(d, 72))
	}

	flags, err := describeFlags(decl.GetArgs())
	if err != nil {
		return err
	}
	fmt.Println()
	if len(flags) == 0 {
		fmt.Println("it takes no arguments")
	} else {
		fmt.Println("flags:")
		w := 0
		for _, f := range flags {
			w = max(w, len(f.usage))
		}
		for _, f := range flags {
			fmt.Printf("  %-*s  %s\n", w, f.usage, f.help)
		}
		fmt.Printf("\n  --args '<json>'%s  the whole argument object, when a flag will not do\n",
			strings.Repeat(" ", max(0, w-15)))
	}

	// The declared properties, said plainly, because they are what decides
	// whether a command belongs on a surface at all.
	fmt.Printf("\neffects %s, %s, %s\n", effectsLabel(decl), durationLabel(decl),
		idempotentLabel(decl))
	// Wrapped, because `returns` is the program's own prose and nothing has
	// trimmed it for a terminal.
	fmt.Printf("returns %s\n", wrap(decl.GetReturns(), 64))
	if decl.GetConfirms() == rigv1.Tristate_TRISTATE_YES {
		fmt.Println("it declares that it confirms before acting")
	}

	if ex := decl.GetExamples(); len(ex) > 0 {
		fmt.Println("\nexamples:")
		for _, e := range ex {
			fmt.Printf("  %s\n", e)
		}
	}
	return nil
}

// flagHelp is one line of generated flag help.
type flagHelp struct {
	usage string // --since <string>
	help  string // the declared description, plus required
}

// describeFlags turns a declared schema into help lines.
//
// Required comes from the schema's own `required` list and the description
// from each property's `description`, so a program that documents its
// arguments gets that documentation on the command line for free - and one
// that does not gets an honest blank rather than an invented sentence.
func describeFlags(raw []byte) ([]flagHelp, error) {
	if strings.TrimSpace(string(raw)) == "" {
		return nil, nil
	}
	var doc struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type        any    `json:"type"`
			Description string `json:"description"`
			Enum        []any  `json:"enum"`
			Default     any    `json:"default"`
			Minimum     *int   `json:"minimum"`
			Maximum     *int   `json:"maximum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("the declared argument schema is not readable: %w", err)
	}

	required := map[string]bool{}
	for _, r := range doc.Required {
		required[r] = true
	}

	names := make([]string, 0, len(doc.Properties))
	for name := range doc.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]flagHelp, 0, len(names))
	for _, name := range names {
		p := doc.Properties[name]
		prop := propSchema{name: name, Type: p.Type}
		usage := "--" + prop.flag()
		placeholder, supported := prop.flagForm()
		switch {
		case len(p.Enum) > 0:
			// The declared values, which is more useful than the type.
			usage += " " + enumList(p.Enum)
		case supported && placeholder != "":
			usage += " " + placeholder
		case !supported:
			// The parser refuses this as a flag, so help must not offer it
			// as one. Both sides read flagForm.
			usage += " (--args only)"
		}

		var notes []string
		if required[name] {
			notes = append(notes, "required")
		}
		if p.Default != nil {
			notes = append(notes, fmt.Sprintf("default %v", p.Default))
		}
		if p.Minimum != nil && p.Maximum != nil {
			notes = append(notes, fmt.Sprintf("%d to %d", *p.Minimum, *p.Maximum))
		}
		help := p.Description
		if len(notes) > 0 {
			if help != "" {
				help += " "
			}
			help += "(" + strings.Join(notes, ", ") + ")"
		}
		out = append(out, flagHelp{usage: usage, help: help})
	}
	return out, nil
}

func enumList(vals []any) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fmt.Sprint(v))
	}
	return strings.Join(parts, "|")
}

// lookupProgram finds one program, and lists the others when it is not there.
func lookupProgram(ctx context.Context, c *client.Client, program string) (*rigv1.Program, error) {
	var resp rigv1.ProgramsResponse
	if err := call(ctx, c, "rig.programs", &rigv1.ProgramsRequest{}, &resp); err != nil {
		return nil, err
	}
	var names []string
	for _, p := range resp.GetPrograms() {
		if p.GetIdentity().GetId() == program {
			return p, nil
		}
		names = append(names, p.GetIdentity().GetId())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no program %q is connected, and neither is any other",
			program)
	}
	return nil, fmt.Errorf("no program %q is connected\n       connected: %s",
		program, strings.Join(names, ", "))
}

func idempotentLabel(c *rigv1.Command) string {
	if c.GetIdempotent() == rigv1.Tristate_TRISTATE_YES {
		return "safe to re-run"
	}
	return "NOT safe to re-run"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// wrap breaks a declared description at a column, because a program's own
// description is prose and nothing has trimmed it for a terminal.
func wrap(s string, width int) string {
	var b strings.Builder
	col := 0
	for i, word := range strings.Fields(s) {
		switch {
		case i == 0:
		case col+1+len(word) > width:
			b.WriteByte('\n')
			col = 0
		default:
			b.WriteByte(' ')
			col++
		}
		b.WriteString(word)
		col += len(word)
	}
	return b.String()
}
