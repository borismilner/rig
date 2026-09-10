// Command rig is the front door.
//
// The other of the two binaries (PLAN.md section 22). It never links the
// daemon's internals: it reaches rig over the socket the same way every other
// client does, which is also why the CLI cannot become a second implementation
// of anything (section 5d).
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

var (
	version = "dev"
	wire    = "v1"
	sha     = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rig: "+err.Error())
		os.Exit(1)
	}
}

// partition splits flags from positionals so a flag may appear anywhere.
//
// Go's flag package stops at the first non-flag argument, so `rig ping fakeapp
// --json` silently treats --json as a positional and the command fails with a
// usage error. Section 10 promises "--json on everything", and a promise that
// depends on argument order is not one. Found by running it.
func partition(args []string) (flags, positional []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return flags, append(positional, args[i+1:]...)
		case strings.HasPrefix(a, "-") && a != "-":
			flags = append(flags, a)
			// A flag written as `--timeout 5s` takes the next argument, while
			// `--timeout=5s` and a boolean do not.
			if !strings.Contains(a, "=") && i+1 < len(args) &&
				!strings.HasPrefix(args[i+1], "-") && takesValue(a) {
				i++
				flags = append(flags, args[i])
			}
		default:
			positional = append(positional, a)
		}
	}
	return flags, positional
}

// takesValue reports whether a flag consumes the following argument. The set is
// tiny and explicit: guessing from the next token's shape is what makes
// `rig ping --timeout 5s fakeapp` and `rig ping --json fakeapp` disagree.
func takesValue(arg string) bool {
	return valuedFlags[strings.TrimLeft(arg, "-")]
}

// valuedFlags is that set. A map rather than a switch so adding one is a line
// rather than a clause, and so the parameter does not have to be called flag
// and shadow the package of the same name.
var valuedFlags = map[string]bool{
	"timeout": true,
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: rig <command> [flags]

  <app> <cmd>      run a command a program declared, with its declared flags
  apps list        what every program declared, as this client may see it
  ping <program>   round-trip a program through rigd ("rig" pings the daemon)
  version          print every version this build carries

Every command takes --json. A declared command also takes --timeout and
--args '<json>', the second being the exact argument object when a flag will
not do.
`)
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}

	switch args[0] {
	case "version":
		return cmdVersion(args[1:])
	case "ping":
		return cmdPing(args[1:])
	case "apps":
		return cmdApps(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		// Anything rig does not implement itself is a program's command, and
		// the registry decides whether it exists. Nothing here is a list of
		// programs: section 5e's whole point is that adopting rig costs a
		// declaration, not an edit to this file.
		if strings.HasPrefix(args[0], "-") {
			usage()
			return fmt.Errorf("no such command %q", args[0])
		}
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return fmt.Errorf("usage: rig %s <command> [flags]\n"+
				"       rig apps list --commands  lists what %s declares",
				args[0], args[0])
		}
		return cmdCall(args[0], args[1], args[2:])
	}
}

// cmdVersion prints all three versions plus the build, because they are not the
// same number and section 28 says so.
func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	flags, _ := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"product": version, "wire": wire, "commit": sha, "built": date,
		})
	}
	fmt.Printf("product %s\nwire    %s\ncommit  %s\nbuilt   %s\n", version, wire, sha, date)
	return nil
}

func cmdPing(args []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", 5*time.Second, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if len(positional) != 1 {
		return errors.New("usage: rig ping <program> [--json] [--timeout=5s]")
	}
	program := positional[0]

	sock, err := paths.Socket()
	if err != nil {
		return err
	}
	c, err := client.Dial(sock)
	if err != nil {
		// The one error a user hits constantly, so it says what to do.
		return fmt.Errorf("%w\n       is rigd running? start it with: rigd", err)
	}
	defer c.Close()

	// A fresh nonce per call, echoed back, so the round trip is provably this
	// call's and not a cached or crossed reply.
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	start := time.Now()
	resp := &rigv1.PingResponse{}
	if err := c.Call(ctx, program+".ping", &rigv1.PingRequest{Nonce: nonce}, resp); err != nil {
		return err
	}
	elapsed := time.Since(start)

	if !bytes.Equal(resp.GetNonce(), nonce) {
		return fmt.Errorf("%s answered with the wrong nonce: the round trip is not ours", program)
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"program": resp.GetProgram(),
			"version": resp.GetVersion(),
			"rtt_us":  elapsed.Microseconds(),
		})
	}
	fmt.Printf("%s %s, round trip %s\n", resp.GetProgram(), resp.GetVersion(),
		elapsed.Round(time.Microsecond))
	return nil
}

// cmdApps reads the registry back (PLAN.md section 5e, section 14).
//
// What it prints is a projection, not the registry: the daemon answers
// rig.programs through this connection's own principal, so a program sees
// itself and a client of the owner's sees all of it. Coverage travels with
// every row because section 5k forbids a surface that implies completeness -
// "shelf declared 3 of its 20 commands" has to be visible here, not inferred.
func cmdApps(args []string) error {
	fs := flag.NewFlagSet("apps", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	verbose := fs.Bool("commands", false, "list each program's commands")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	if len(positional) == 0 || positional[0] != "list" {
		return errors.New("usage: rig apps list [--commands] [--json]")
	}

	c, err := client.Connect()
	if err != nil {
		return fmt.Errorf("%w\n       is rigd running? start it with: rigd", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := &rigv1.ProgramsResponse{}
	if err := c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{}, resp); err != nil {
		return err
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(appsJSON(resp.GetPrograms()))
	}
	if len(resp.GetPrograms()) == 0 {
		fmt.Println("no programs are registered")
		return nil
	}
	// Column widths come from the widest cell, not from a guess. A version
	// string is git describe output and can be thirty characters, which a
	// fixed %-10s turns into a table with no columns at all.
	wID, wVer := 0, 0
	for _, p := range resp.GetPrograms() {
		wID = max(wID, len(p.GetIdentity().GetId()))
		wVer = max(wVer, len(p.GetIdentity().GetVersion()))
	}
	for _, p := range resp.GetPrograms() {
		fmt.Printf("%-*s  %-*s  %-8s  %d command%s, semantics gen %d\n",
			wID, p.GetIdentity().GetId(),
			wVer, p.GetIdentity().GetVersion(),
			coverageLabel(p),
			len(p.GetCommands()), plural(len(p.GetCommands())),
			p.GetSemanticsGen())
		if !*verbose {
			continue
		}
		wCmd, wEff := 0, 0
		for _, cmd := range p.GetCommands() {
			wCmd = max(wCmd, len(cmd.GetId()))
			wEff = max(wEff, len(effectsLabel(cmd)))
		}
		for _, cmd := range p.GetCommands() {
			fmt.Printf("    %-*s  %-*s  %-8s  %s\n",
				wCmd, cmd.GetId(), wEff, effectsLabel(cmd),
				durationLabel(cmd), cmd.GetSummary())
		}
	}
	return nil
}

// coverageLabel says how much of rig a program adopted, and says it on every
// row. Section 5k: the honest default is the conservative one.
func coverageLabel(p *rigv1.Program) string {
	switch p.GetCoverage() {
	case rigv1.Coverage_COVERAGE_FULL:
		return "full"
	case rigv1.Coverage_COVERAGE_PARTIAL:
		return "partial"
	default:
		return "coverage?"
	}
}

// enumLabel renders a proto enum as section 5e writes it: read-only, not
// READ_ONLY and not read_only. The plan's vocabulary is what a house rule and
// a --help page are read against, so it is the one that ships.
func enumLabel(full, prefix string) string {
	return strings.ReplaceAll(
		strings.ToLower(strings.TrimPrefix(full, prefix)), "_", "-")
}

func effectsLabel(c *rigv1.Command) string {
	return enumLabel(c.GetEffects().String(), "EFFECTS_")
}

func durationLabel(c *rigv1.Command) string {
	return enumLabel(c.GetDuration().String(), "DURATION_")
}

func pointers(s *rigv1.SensitiveFields) []string {
	if p := s.GetPointers(); p != nil {
		return p
	}
	return []string{}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// rawSchema embeds a declared schema in the output as JSON, not as a string.
//
// json.RawMessage rather than re-encoding: what the program declared is what
// an agent has to validate against, and re-marshalling a schema through a
// map is how key order, numbers and unknown keywords quietly change.
func rawSchema(args []byte) any {
	if len(bytes.TrimSpace(args)) == 0 {
		return nil
	}
	if !json.Valid(args) {
		// A schema this malformed cannot have been registered - the kernel
		// compiles it at connect - so reaching here means the wire or the
		// daemon changed it. Say so rather than emitting broken JSON.
		return "unreadable: the declared schema is not JSON"
	}
	return json.RawMessage(args)
}

func appsJSON(ps []*rigv1.Program) []map[string]any {
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		cmds := make([]map[string]any, 0, len(p.GetCommands()))
		for _, c := range p.GetCommands() {
			row := map[string]any{
				"id":            c.GetId(),
				"summary":       c.GetSummary(),
				"effects":       effectsLabel(c),
				"duration":      durationLabel(c),
				"idempotent":    c.GetIdempotent() == rigv1.Tristate_TRISTATE_YES,
				"needs_display": c.GetNeedsDisplay() == rigv1.Tristate_TRISTATE_YES,
				"interactive":   c.GetInteractive() == rigv1.Tristate_TRISTATE_YES,
				"confirms":      c.GetConfirms() == rigv1.Tristate_TRISTATE_YES,
				// Present and empty, never null: null reads as "this program
				// never considered the question", which is the one thing
				// section 5e refuses to let a declaration mean by accident.
				// The daemon always sends the wrapper, so absence cannot
				// reach here - and if it ever does, [] is still the honest
				// rendering of what was stored.
				"sensitive": pointers(c.GetSensitive()),

				// The declared argument schema is added below, embedded as
				// JSON rather than as a string. Section 9 says an agent gets
				// everything it needs to use a command well, and without it
				// an agent can read that reindex exists and still not be
				// able to construct a call.
			}
			// Omitted rather than null when the command declares none: a
			// command that takes no arguments has no schema, which is not
			// the same as a schema nobody wrote down.
			if schema := rawSchema(c.GetArgs()); schema != nil {
				row["args"] = schema
			}
			cmds = append(cmds, row)
		}
		out = append(out, map[string]any{
			"id":            p.GetIdentity().GetId(),
			"version":       p.GetIdentity().GetVersion(),
			"coverage":      coverageLabel(p),
			"coverage_note": p.GetCoverageNote(),
			"semantics_gen": p.GetSemanticsGen(),
			"commands":      cmds,
		})
	}
	return out
}
