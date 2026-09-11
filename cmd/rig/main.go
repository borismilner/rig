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
	"syscall"
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
		report(os.Stdout, os.Stderr, err)
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
	"depth":   true,
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: rig <command> [flags]

  <app> <cmd>      run a command a program declared, with its declared flags
  apps list        what every program declared, as this client may see it
  describe <app> [<cmd>]
                   one thing in full: a program's preamble, or a command's
                   whole declaration
  ping <program>   round-trip a program through rigd ("rig" pings the daemon)
  estate           which estate this shell reached, and what it is for
  down             stop the daemon serving this XDG_RUNTIME_DIR
  version          print every version this build carries
  completion <sh>  a completion script for bash, zsh or fish

Every command that answers takes --json, with two exceptions. completion
writes a shell script for eval, which is not an answer to put in an object.
describe does not have it YET: section 10 binds its object to the one the
MCP tool returns, and that object is rendered by the daemon rather than
here, so the flag arrives with the renderer rather than ahead of it.

A declared command also takes --timeout and --args '<json>', the second
being the exact argument object when a flag will not do.

rig <app> --help and rig <app> <cmd> --help are generated from what the
program declared, so they list what it actually has.
`)
}

// verbAt is the index of the command word, so rig's own flags may come BEFORE
// it as well as after.
//
// partition already lets a flag follow the verb, and the comment on it says
// why: section 10 promises --json on everything and a promise that depends on
// argument order is not one. This is the other half of that promise and it
// was missing. PLAN.md:1512 names `rig --json list` as the agent affordance
// BY EXAMPLE, and that exact line failed with `no such command "--json"`.
// Found by running the plan's own example.
//
// Returns len(args) when there is no verb at all, which is what keeps
// `rig -h` and `rig --help` reaching the help case rather than being stripped
// down to nothing.
func verbAt(args []string) int {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return i + 1
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			return i
		}
		// `--timeout 5s ping` - the value is not the verb.
		if !strings.Contains(a, "=") && takesValue(a) && i+1 < len(args) {
			i++
		}
	}
	return len(args)
}

// with puts the flags that preceded the verb back at the END of what the
// handler sees, rather than reordering argv in place.
//
// Appending is what makes `rig --json fakeapp reindex --since 7d` work:
// the program and the command have to stay in positions 0 and 1, so a flag
// moved to just after the verb would be read as the command.
func with(args, lead []string) []string {
	if len(lead) == 0 {
		return args
	}
	out := make([]string, 0, len(args)+len(lead))
	return append(append(out, args...), lead...)
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return badArgumentf("no command given")
	}

	var lead []string
	if v := verbAt(args); v > 0 && v < len(args) {
		lead, args = args[:v], args[v:]
	}

	switch args[0] {
	case verbVersion:
		return cmdVersion(with(args[1:], lead))
	case "ping":
		return cmdPing(with(args[1:], lead))
	case "apps":
		return cmdApps(with(args[1:], lead))
	case "down":
		return cmdDown(with(args[1:], lead))
	case "estate":
		return cmdEstate(with(args[1:], lead))
	case "describe":
		return cmdDescribe(with(args[1:], lead))
	case "completion":
		return cmdCompletion(args[1:])
	case "__complete":
		// Hidden: what the shell scripts call. Not secret, just not
		// something a person types.
		return cmdComplete(args[1:])
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
			return badArgumentf("no such command %q", args[0])
		}
		// Help is generated from the declaration, so it is answered here
		// rather than from a string in this file (slice 6).
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			if len(args) == 1 || asksForHelp(args[1:]) {
				return helpForProgram(args[0])
			}
			return fmt.Errorf("usage: rig %s <command> [flags]\n"+
				"       rig %s --help  lists what it declares",
				args[0], args[0])
		}
		if asksForHelp(args[2:]) {
			return helpForCommand(args[0], args[1])
		}
		return cmdCall(args[0], args[1], with(args[2:], lead))
	}
}

// cmdVersion prints all three versions plus the build, because they are not the
// same number and section 28 says so.
func cmdVersion(args []string) error {
	fs := flag.NewFlagSet(verbVersion, flag.ContinueOnError)
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

func cmdPing(args []string) (err error) {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) != 1 {
		return badArgumentf(
			"usage: rig ping <program> [--json] [--timeout=30s]")
	}
	program := positional[0]
	// An empty name used to fail at the daemon, because ".ping" is not a
	// <program>.<command>. Now that the target travels as an argument the
	// daemon would read it as "probe rig itself", so `rig ping ""` would
	// answer about rig and look like it worked. Refuse it here instead.
	if program == "" {
		return badArgumentf(
			"usage: rig ping <program>: the program name is empty")
	}

	sock, err := paths.Socket()
	if err != nil {
		return err
	}
	c, err := client.Dial(sock)
	if err != nil {
		// The one error a user hits constantly, so it says what to do.
		return noDaemon(err)
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
	// rig.ping with the program as an argument, not <program>.ping: the probe
	// is rig's method and never was one of the program's own commands.
	if err := call(ctx, c, "rig.ping",
		&rigv1.PingRequest{Nonce: nonce, Program: program}, resp); err != nil {
		return err
	}
	elapsed := time.Since(start)

	if !bytes.Equal(resp.GetNonce(), nonce) {
		return local(jsonStatus{
			Code:         codeBadResult,
			Message:      program + " answered with the wrong nonce: the round trip is not ours",
			Precondition: "a probe's answer carries back the nonce rig sent",
			Actual:       "it carried a different one",
			Fix:          "the answer is not this probe's: suspect a proxy or a stale connection",
		})
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

// cmdDown stops the daemon serving this XDG_RUNTIME_DIR.
//
// A wire call, not a signal. The cheap version would read paths.PIDFile() and
// SIGTERM the pid, and it loses twice: it skips the house-rules floor at the
// most destructive call rig has, and rigd.pid outlives its own process - a
// SIGTERM'd daemon exits 0 and removes the socket but leaves the pid file
// behind pointing at nothing, so the file is not evidence that anything is
// running.
//
// Scoped for free: it talks to the socket in this XDG_RUNTIME_DIR, which
// internal/paths refuses to guess. Two estates are two runtime directories,
// so a stop needs no estate name and there is nothing to get wrong.
func cmdDown(args []string) (err error) {
	fs := flag.NewFlagSet("down", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) != 0 {
		return badArgumentf("usage: rig down [--json] [--timeout=30s]")
	}

	c, err := client.Connect()
	if err != nil {
		// STOPPING A STOPPED DAEMON SUCCEEDS, and this is where that is
		// decided. Every other verb treats an unreachable socket as the
		// error it is; for this one the caller's goal is already true, and
		// `make down` is a script that should not have to special-case it.
		//
		// Only the two errors that mean "nothing is listening" count:
		// ENOENT for no socket file, ECONNREFUSED for one left behind by a
		// daemon that died without removing it. Anything else - a permission
		// error, an unreadable runtime dir - is a real failure and must not
		// be reported as a successful stop.
		if notRunning(err) {
			if *asJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"stopped": false, "reason": "not running",
				})
			}
			fmt.Println("nothing to stop")
			return nil
		}
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resp := &rigv1.DownResponse{}
	if err := call(ctx, c, "rig.down", &rigv1.DownRequest{}, resp); err != nil {
		return err
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"stopped": true,
			"pid":     resp.GetPid(),
			"version": resp.GetVersion(),
		})
	}
	// The pid is in the answer because two estates run on this machine under
	// different XDG_RUNTIME_DIRs and `pgrep -x rigd` cannot tell them apart.
	fmt.Printf("stopped rigd %s (pid %d)\n", resp.GetVersion(), resp.GetPid())
	return nil
}

// notRunning reports whether a dial error means no daemon is listening, as
// opposed to something being wrong with reaching one that is.
func notRunning(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}

// cmdApps reads the registry back (PLAN.md section 5e, section 14).
//
// What it prints is a projection, not the registry: the daemon answers
// rig.programs through this connection's own principal, so a program sees
// itself and a client of the owner's sees all of it. Coverage travels with
// every row because section 5k forbids a surface that implies completeness -
// "shelf declared 3 of its 20 commands" has to be visible here, not inferred.
// appsFlagSet is `apps list`'s flags, built here rather than inline so a test
// can WALK them.
//
// partition() splits flags from positionals before flag.Parse sees them, and
// it can only know that `--depth full` consumes its next argument by asking
// valuedFlags. Every flag on this command was a boolean until --depth, so the
// set had one entry and nothing coupled it to anything. A non-boolean flag
// missing from it does not fail loudly - `--depth full` reads `full` as a
// positional and the command dies with "flag needs an argument", which points
// at the flag rather than at the set. Found by RUNNING it against a live
// estate; every unit test in this package passed, because they all call the
// renderer or the parser and none of them go through argv.
func appsFlagSet() (fs *flag.FlagSet, asJSON, verbose *bool, depth *string) {
	fs = flag.NewFlagSet("apps", flag.ContinueOnError)
	asJSON = fs.Bool("json", false, "emit JSON")
	verbose = fs.Bool("commands", false, "list each program's commands")
	depth = fs.String("depth", "", "how much to say about each program: "+
		strings.Join(depthSpellings(), ", "))
	return fs, asJSON, verbose, depth
}

func cmdApps(args []string) (err error) {
	fs, asJSON, verbose, depthName := appsFlagSet()
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) == 0 || positional[0] != "list" {
		return badArgumentf("usage: rig apps list [--commands] [--depth %s] "+
			"[--json]", strings.Join(depthSpellings(), "|"))
	}

	// asked is what goes on the wire and shown is what comes back. They differ
	// for exactly one value: an absent --depth sends the zero, which the
	// daemon's boundary restores to DEPTH_FULL as a compatibility rule
	// (wire.proto, ProgramsRequest), so the renderer below has to assume a
	// full answer even though nothing asked for one.
	asked, err := parseDepth(*depthName)
	if err != nil {
		return err
	}
	shown := asked
	if shown == rigv1.Depth_DEPTH_UNSPECIFIED {
		shown = rigv1.Depth_DEPTH_FULL
	}

	c, err := client.Connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := &rigv1.ProgramsResponse{}
	req := &rigv1.ProgramsRequest{Depth: asked}
	if err := call(ctx, c, "rig.programs", req, resp); err != nil {
		return err
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(appsJSON(resp.GetPrograms(), shown))
	}
	if len(resp.GetPrograms()) == 0 {
		fmt.Println("no programs are registered")
		return nil
	}
	// Section 5k: a surface may not imply completeness. At a shallow depth the
	// daemon withheld something, so the listing says so before it is read -
	// and says which flag chose it, because the fix is the reader's to make.
	//
	// Only when something WAS withheld. A full listing has nothing to
	// disclose, so today's output is unchanged. This is deliberately NOT the
	// rule the --json object wants: a terminal listing is read by whoever
	// typed the flag, and the JSON object travels to a reader who did not.
	if note := withheldNote(shown); note != "" {
		fmt.Println(note)
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
		fmt.Printf("%-*s  %-*s  %-8s  %ssemantics gen %d\n",
			wID, p.GetIdentity().GetId(),
			wVer, p.GetIdentity().GetVersion(),
			coverageLabel(p),
			commandCountCell(p, shown),
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

// appsJSON renders the estate AT THE DEPTH THAT WAS ANSWERED, and the depth is
// a parameter because it decides what this object may say anything about.
//
// A field the depth did not fetch is OMITTED, never rendered as an empty one.
// Every empty rendering here already means something: `sensitive: []` is
// deliberately not null because null would read as "the program never
// considered the question", and an absent `args` means "this command takes no
// arguments". At a shallow depth both of those would be false, so the honest
// object leaves them out rather than asserting a declaration nobody read.
//
// THE OBJECT CANNOT YET SAY WHICH DEPTH PRODUCED IT, and that is RULED and
// SEQUENCED rather than accepted. ProgramsResponse carries no depth echo and
// meta.Answer does not either, so a reader who did not type the flag cannot
// tell a cheap question from an empty answer - and under section 37 rig's own
// agents become those readers.
//
// The ruling is a `depth` key emitted ALWAYS, with no omitempty, in BOTH
// renderers, on the argument answerJSON already writes down for `partial`:
// an absent field cannot be told apart from a server too old to have it, so
// an agent reading one has to guess, and the safe guess and the useful guess
// point opposite ways. internal/meta gains it FIRST because the daemon-side
// object is the contract section 10 binds this one to; this renderer follows.
// Landing it in both now rather than with B10 is deliberate: B10 changes
// WHERE the rendering happens and not WHAT the object says, so a converged
// object would inherit the ambiguity instead of ending it.
func appsJSON(ps []*rigv1.Program, d rigv1.Depth) []map[string]any {
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

				// The declared argument schema is added below, embedded as
				// JSON rather than as a string. Section 9 says an agent gets
				// everything it needs to use a command well, and without it
				// an agent can read that reindex exists and still not be
				// able to construct a call.
			}
			// Both of these are dropped by the kernel's projection below
			// DEPTH_FULL, so below it they are omitted here. Emitting them
			// would report what the projection removed as what the program
			// declared.
			if carriesCommandDetail(d) {
				// Present and empty, never null: null reads as "this program
				// never considered the question", which is the one thing
				// section 5e refuses to let a declaration mean by accident.
				// The daemon always sends the wrapper, so absence cannot
				// reach here - and if it ever does, [] is still the honest
				// rendering of what was stored.
				row["sensitive"] = pointers(c.GetSensitive())

				// Omitted rather than null when the command declares none: a
				// command that takes no arguments has no schema, which is not
				// the same as a schema nobody wrote down.
				if schema := rawSchema(c.GetArgs()); schema != nil {
					row["args"] = schema
				}
			}
			cmds = append(cmds, row)
		}
		row := map[string]any{
			"id":            p.GetIdentity().GetId(),
			"version":       p.GetIdentity().GetVersion(),
			"coverage":      coverageLabel(p),
			"coverage_note": p.GetCoverageNote(),
			"semantics_gen": p.GetSemanticsGen(),
		}
		// At DEPTH_PROGRAMS the daemon sent no commands at all, so an empty
		// list here would say the program declares none.
		if carriesCommands(d) {
			row["commands"] = cmds
		}
		out = append(out, row)
	}
	return out
}

// depthSpellings is what --depth accepts, read off the wire's own enum rather
// than carried as a table here.
//
// kernel.ParseDepth takes exactly these spellings and cmd/rig MUST NOT CALL
// IT: the kernel links the JSON Schema validator, so importing it pays section
// 17's 1.45 MB in the one binary the plan says must not pay it, and
// TestTheClientLinksNeitherTheDaemonNorItsValidator fails naming
// santhosh-tekuri rather than the kernel. The descriptor is the same contract
// without the dependency, and it cannot drift when a depth is added.
func depthSpellings() []string {
	values := rigv1.Depth(0).Descriptor().Values()
	out := make([]string, 0, values.Len())
	for i := range values.Len() {
		v := values.Get(i)
		// Section 21: the zero means "nothing was said", so no surface may
		// let a caller ask for it. It is offered nowhere and refused by name
		// below if somebody spells it anyway.
		if v.Number() == 0 {
			continue
		}
		out = append(out, enumLabel(string(v.Name()), "DEPTH_"))
	}
	return out
}

// parseDepth maps the flag onto the wire enum. An empty string is not an error
// and not a default: it is the absent flag, which sends the zero and lets the
// daemon's boundary restore the old wire's meaning.
func parseDepth(s string) (rigv1.Depth, error) {
	if s == "" {
		return rigv1.Depth_DEPTH_UNSPECIFIED, nil
	}
	values := rigv1.Depth(0).Descriptor().Values()
	for i := range values.Len() {
		v := values.Get(i)
		if v.Number() == 0 {
			continue
		}
		if enumLabel(string(v.Name()), "DEPTH_") == s {
			return rigv1.Depth(v.Number()), nil
		}
	}
	return rigv1.Depth_DEPTH_UNSPECIFIED, badArgumentf(
		"%q is not a depth; the depths are %s",
		s, strings.Join(depthSpellings(), ", "))
}

// carriesCommands is false only at DEPTH_PROGRAMS, where kernel.atDepth sets
// Commands to nil.
func carriesCommands(d rigv1.Depth) bool {
	return d != rigv1.Depth_DEPTH_PROGRAMS
}

// carriesCommandDetail is true only at DEPTH_FULL. kernel.commandsAtDepth
// drops Args, Examples, Preconditions, Sensitive, Description and Returns
// below it; of those this renderer carries Args and Sensitive.
func carriesCommandDetail(d rigv1.Depth) bool {
	return d == rigv1.Depth_DEPTH_FULL
}

// commandCountCell is the "N commands, " run in a listing row, and it is EMPTY
// when the depth did not fetch commands.
//
// "0 commands" for a program with twenty is not a cheaper answer, it is a
// false one, and a human reading a listing has no other signal that the number
// was never asked for. It is a function of its own so it can be tested without
// a live daemon: cmdApps needs one and this does not.
func commandCountCell(p *rigv1.Program, d rigv1.Depth) string {
	if !carriesCommands(d) {
		return ""
	}
	n := len(p.GetCommands())
	return fmt.Sprintf("%d command%s, ", n, plural(n))
}

// withheldNote says what a shallow depth left out, for a human reading the
// listing. Empty at full depth, where nothing was withheld and there is
// nothing to disclose.
func withheldNote(d rigv1.Depth) string {
	switch {
	case !carriesCommands(d):
		return "showing the estate only: no commands were asked for. " +
			"--depth commands or --depth full says more"
	case !carriesCommandDetail(d):
		return "showing each command without its arguments or sensitive " +
			"fields: --depth full says more"
	}
	return ""
}
