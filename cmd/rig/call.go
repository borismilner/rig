package main

import (
	"context"
	"encoding/json"
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

// defaultCallTimeout is how long the CLI waits, and it is deliberately LONGER
// than the daemon's own CallTimeout.
//
// Section 18 makes the daemon's deadline a supervision decision - "a program
// that misses them is degraded, then restarted" - and rig answers a hang
// itself, naming the program and the deadline it missed. The client's number
// is not that. It is patience, and if it is the smaller of the two the client
// gives up first: the user gets `context deadline exceeded` from their own
// process instead of rig's diagnosis, on the commonest failure there is.
//
// `rig ping` used to default to 5s against a 10s CallTimeout and did exactly
// that, needing --timeout 30s to see what rig had to say. One number now, and
// TestTheClientOutlastsTheDaemonsOwnDeadline stops the two drifting apart.
const defaultCallTimeout = 30 * time.Second

// callFlags is what rig itself takes on a call.
type callFlags struct {
	asJSON  bool
	timeout time.Duration
	args    string // a whole JSON object, given verbatim
	hasArgs bool
}

// cmdCall runs one declared command.
func cmdCall(program, command string, argv []string) (err error) {
	var own callFlags
	var rest []string
	own, rest, err = splitOwnFlags(argv)
	if err != nil {
		return err
	}
	// Everything below can fail, and all of it is rendered the way the caller
	// asked. --json was not known until the line above.
	defer func() { err = inMode(err, own.asJSON) }()

	c, err := client.Connect()
	if err != nil {
		return noDaemon(err)
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
	if err := call(ctx, c, program+"."+command,
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
	own := callFlags{timeout: defaultCallTimeout}
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
				return own, nil, badArgumentf("--%s needs a value", name)
			}
			i++
			value = argv[i]
		}
		switch name {
		case "json":
			if inline {
				b, err := strconv.ParseBool(value)
				if err != nil {
					return own, nil, badArgumentf("--json takes true or false: %s", err)
				}
				own.asJSON = b
				continue
			}
			own.asJSON = true
		case "timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return own, nil, badArgumentf("--timeout: %s", err)
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
	if err := call(ctx, c, "rig.programs", &rigv1.ProgramsRequest{}, &resp); err != nil {
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
		coverage := strings.ToLower(strings.TrimPrefix(p.GetCoverage().String(), "COVERAGE_"))
		return nil, local(jsonStatus{
			Code:         codeNoSuchCommand,
			Message:      fmt.Sprintf("%s declares no command %q", program, command),
			Precondition: fmt.Sprintf("%s declares a command named %q", program, command),
			Actual: fmt.Sprintf("it declares: %s\ncoverage is %s, so this may be a "+
				"command it has not adopted yet",
				strings.Join(commandIDs(p), ", "), coverage),
			Fix:        "run one it declares, or see what else it has",
			FixCommand: "rig " + program + " --help",
		})
	}

	sort.Strings(names)
	connected := "nothing is connected"
	if len(names) > 0 {
		connected = "connected: " + strings.Join(names, ", ")
	}
	return nil, local(jsonStatus{
		Code:         codeNoSuchProgram,
		Message:      fmt.Sprintf("no program %q is connected", program),
		Precondition: program + " is registered with rig",
		Actual:       connected,
		// No fix command: rig cannot start a program, and section 9 would
		// rather say nothing than offer one that does not run.
		Fix: "start " + program + ", or check the name",
	})
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
		return nil, badArgumentf("--args and the flag %s cannot both be "+
			"given: --args is the whole argument object", rest[0])
	}
	if own.hasArgs {
		if !json.Valid([]byte(own.args)) {
			return nil, badArgumentf("--args is not valid JSON")
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
		return nil, badArgumentf(
			"%s declares no arguments, so %s is not one of them",
			decl.GetId(), rest[0])
	}

	out := map[string]any{}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if !strings.HasPrefix(a, "-") {
			return nil, badArgumentf("unexpected argument %q: every "+
				"declared argument is a flag, and this command takes %s",
				a, strings.Join(schema.names(), ", "))
		}
		name, value, inline := strings.Cut(strings.TrimLeft(a, "-"), "=")

		prop, ok := schema.property(name)
		if !ok {
			return nil, local(jsonStatus{
				Code:    codeBadArgument,
				Message: fmt.Sprintf("%s takes no --%s", decl.GetId(), name),
				Precondition: fmt.Sprintf("%s declares an argument named %q",
					decl.GetId(), name),
				Actual: "it takes: " + strings.Join(schema.names(), ", "),
				Fix:    "see what it declares, with its types and examples",
			})
		}
		// A flag given twice is a mistake, not a preference. Taking the last
		// one silently is how `--since 1d --since 2d` in a script reindexes
		// the wrong window and nothing says so.
		if _, already := out[prop.name]; already {
			return nil, badArgumentf("--%s was given twice", prop.flag())
		}
		if prop.kind() == "boolean" && !inline {
			out[prop.name] = true
			continue
		}
		if !inline {
			if i+1 >= len(rest) {
				return nil, badArgumentf("--%s needs a value", name)
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
