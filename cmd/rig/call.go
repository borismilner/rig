package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// `rig <app> <cmd>` (PLAN.md section 10, M1 slice 5).
//
// Nothing here knows what any command is. The command set comes from the
// registry, the flags come from the declared JSON Schema, and rig validates
// the arguments against that same schema at the boundary - so this file is a
// translator from argv to JSON and back, and it is the only place that has to
// change when a program declares something new. Which is to say: it does not.

// ownFlags are rig's own, on a call. Everything else in argv is the command's.
//
// A command that declares a property with one of these names cannot be given
// it as sugar - rig's flag wins, because a caller typing `--json` on any
// surface of rig means the same thing everywhere. `--args` is the way out and
// the error says so.
var ownFlags = map[string]bool{"json": true, "timeout": true, "args": true}

// callFlags is what rig itself takes on a call.
type callFlags struct {
	asJSON  bool
	timeout time.Duration
	args    string // a whole JSON object, given verbatim
	hasArgs bool
}

// cmdCall runs one declared command.
func cmdCall(program, command string, argv []string) error {
	own, rest, err := splitOwnFlags(argv)
	if err != nil {
		return err
	}

	c, err := client.Connect()
	if err != nil {
		return fmt.Errorf("%w\n       is rigd running? start it with: rigd", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), own.timeout)
	defer cancel()

	decl, err := lookup(ctx, c, program, command)
	if err != nil {
		return err
	}

	args, err := buildArgs(decl, own, rest)
	if err != nil {
		return err
	}

	var resp rigv1.CallResponse
	if err := c.Call(ctx, program+"."+command,
		&rigv1.CallRequest{Args: args}, &resp); err != nil {
		return err
	}
	return printResult(program, command, resp.GetResult(), own.asJSON)
}

// splitOwnFlags takes rig's own flags out of argv and leaves the command's.
//
// It cannot use the shared partition helper: whether a declared flag consumes
// the next argument depends on its declared type, which is not known until
// the registry has been read, and partition's set of value-taking flags is
// fixed at compile time.
func splitOwnFlags(argv []string) (callFlags, []string, error) {
	own := callFlags{timeout: 30 * time.Second}
	var rest []string

	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			rest = append(rest, argv[i+1:]...)
			break
		}
		name, value, inline := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || !ownFlags[name] {
			rest = append(rest, a)
			continue
		}
		// A value-taking own flag reads the next argument when it was not
		// written with an equals sign.
		if !inline && name != "json" {
			if i+1 >= len(argv) {
				return own, nil, fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = argv[i]
		}
		switch name {
		case "json":
			if inline {
				b, err := strconv.ParseBool(value)
				if err != nil {
					return own, nil, fmt.Errorf("--json takes true or false: %w", err)
				}
				own.asJSON = b
				continue
			}
			own.asJSON = true
		case "timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return own, nil, fmt.Errorf("--timeout: %w", err)
			}
			own.timeout = d
		case "args":
			own.args, own.hasArgs = value, true
		}
	}
	return own, rest, nil
}

// lookup finds one command's declaration, and says something useful when it
// is not there.
//
// Section 5k: no surface may imply completeness. So a command that is not
// declared is reported with the program's coverage, because "shelf declares 3
// of its 20 commands" is the difference between a typo and a command that
// exists but has not been adopted yet.
func lookup(ctx context.Context, c *client.Client, program, command string) (*rigv1.Command, error) {
	var resp rigv1.ProgramsResponse
	if err := c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{}, &resp); err != nil {
		return nil, err
	}

	var names []string
	for _, p := range resp.GetPrograms() {
		names = append(names, p.GetIdentity().GetId())
		if p.GetIdentity().GetId() != program {
			continue
		}
		for _, cmd := range p.GetCommands() {
			if cmd.GetId() == command {
				return cmd, nil
			}
		}
		return nil, fmt.Errorf("%s declares no command %q\n       it declares: %s\n"+
			"       coverage is %s, so this may be a command it has not adopted yet",
			program, command, strings.Join(commandIDs(p), ", "),
			strings.ToLower(strings.TrimPrefix(p.GetCoverage().String(), "COVERAGE_")))
	}

	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no program %q is connected, and neither is any other",
			program)
	}
	return nil, fmt.Errorf("no program %q is connected\n       connected: %s",
		program, strings.Join(names, ", "))
}

func commandIDs(p *rigv1.Program) []string {
	var out []string
	for _, c := range p.GetCommands() {
		out = append(out, c.GetId())
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"nothing"}
	}
	return out
}

// buildArgs turns the command's own flags into the JSON object rig will
// validate.
//
// `--args` is exact and the flags are sugar. Both at once is refused rather
// than merged: a caller who gave the object AND a flag has two intentions and
// rig would have to pick one silently.
func buildArgs(decl *rigv1.Command, own callFlags, rest []string) ([]byte, error) {
	if own.hasArgs && len(rest) > 0 {
		return nil, fmt.Errorf("--args and the flag %s cannot both be given: "+
			"--args is the whole argument object", rest[0])
	}
	if own.hasArgs {
		if !json.Valid([]byte(own.args)) {
			return nil, errors.New("--args is not valid JSON")
		}
		return []byte(own.args), nil
	}
	if len(rest) == 0 {
		return nil, nil
	}

	schema, err := parseArgSchema(decl.GetArgs())
	if err != nil {
		return nil, err
	}
	if len(schema.Properties) == 0 {
		return nil, fmt.Errorf("%s declares no arguments, so %s is not one of them",
			decl.GetId(), rest[0])
	}

	out := map[string]any{}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "-") {
			return nil, fmt.Errorf("unexpected argument %q: every declared "+
				"argument is a flag, and this command takes %s",
				a, strings.Join(schema.names(), ", "))
		}
		name, value, inline := strings.Cut(strings.TrimLeft(a, "-"), "=")

		prop, ok := schema.property(name)
		if !ok {
			return nil, fmt.Errorf("%s takes no --%s\n       it takes: %s",
				decl.GetId(), name, strings.Join(schema.names(), ", "))
		}
		// A flag given twice is a mistake, not a preference. Taking the last
		// one silently is how `--since 1d --since 2d` in a script reindexes
		// the wrong window and nothing says so.
		if _, already := out[prop.name]; already {
			return nil, fmt.Errorf("--%s was given twice", prop.flag())
		}
		if prop.kind() == "boolean" && !inline {
			out[prop.name] = true
			continue
		}
		if !inline {
			if i+1 >= len(rest) {
				return nil, fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = rest[i]
		}
		v, err := prop.parse(value)
		if err != nil {
			return nil, err
		}
		out[prop.name] = v
	}
	return json.Marshal(out)
}

// printResult writes what the program answered.
//
// --json prints the program's own JSON, unchanged and unwrapped. Section 5d
// says the client is a dumb pipe, and a CLI that re-shaped a result would
// make its output a second contract nobody declared.
//
// A result that is not JSON is printed as it arrived AND reported as an
// error, in both modes. The program declared what it returns; answering
// something else is its defect, and a CLI that swallows it in human mode and
// reports it in --json mode makes the defect depend on who is looking.
func printResult(program, command string, result []byte, asJSON bool) error {
	if len(result) == 0 {
		if asJSON {
			fmt.Println("null")
			return nil
		}
		fmt.Printf("%s.%s: done\n", program, command)
		return nil
	}

	var v any
	if err := json.Unmarshal(result, &v); err != nil {
		fmt.Println(string(result))
		return fmt.Errorf("%s.%s answered something that is not JSON: %w",
			program, command, err)
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	fmt.Println(human(v))
	return nil
}

// human renders a JSON value for a person, one line per field.
//
// It is deliberately small. Rendering a declared shape properly is a renderer
// against a schema (section 5h) and it is not this file's job; what this has
// to do is not lie about what came back.
func human(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for i, k := range keys {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%-16s %s", k, human(t[k]))
		}
		return b.String()
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, human(e))
		}
		return strings.Join(parts, ", ")
	case nil:
		return "-"
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
