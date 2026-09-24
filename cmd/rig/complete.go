package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// Shell completion, from the same declaration as everything else (PLAN.md
// section 10, M1 slice 6).
//
// The shell scripts below are static and tiny, and they know nothing: every
// candidate comes back from `rig __complete`, which reads the registry. So a
// program that registers a new command is completable immediately, without
// re-sourcing anything, and a program that disconnects stops being offered.
//
// A completion that fails must fail SILENTLY. Nothing here prints an error or
// exits non-zero: a shell that prints "is rigd running?" into the middle of a
// half-typed command line is worse than one that offers nothing.

// verbVersion is one string in three places that MUST agree: the dispatch
// switch in main.go, the flag set that parses it, and the completion list
// below. If they drift, the completion list and the dispatch switch disagree
// about which verbs exist, and nothing says so.
//
// Only this one verb is hoisted, and the reason is honest rather than tidy:
// goconst fires at six occurrences and `version` is the only verb that
// reaches six, because it is also a JSON output key in three places. The
// other five verbs have exactly the same coupling and deserve the same
// treatment; doing all six is a consistent change on its own rather than
// something to smuggle into a lint fix.
//
// The JSON keys are deliberately NOT this constant. An output key is part of
// a contract with whoever reads `--json`; a verb is an input this file
// dispatches on. They share a spelling and nothing else, and giving them one
// name would couple two things that are free to move apart.
const verbVersion = "version"

// staticVerbs are rig's own, and the only names in this file.
var staticVerbs = []string{
	"apps", "ping", "down", "estate", "peers", "describe", "mcp",
	"record", "progress", "backup", "restore",
	verbVersion, "completion", "help",
}

// answersNothingElse is what a verb that takes NO positional argument offers.
//
// Three verbs share it - down, estate and peers - and each has its own reason
// written at its own case, because the reasons are not the same fact: down and
// estate name no estate because the runtime dir already does, and peers names
// no seat because it does not filter. The ANSWER is identical and the
// arguments for it are not, so the list is shared here and the reasoning stays
// where a reader of that case will find it.
//
// It returns a FRESH slice every call, because callers of complete() append to
// what they are given.
func answersNothingElse() []string {
	return []string{"--json", "--timeout"}
}

// cmdCompletion prints the script for one shell.
func cmdCompletion(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: rig completion <bash|zsh|fish>\n" +
			"       eval \"$(rig completion zsh)\"  to try it in this shell")
	}
	script, ok := completionScripts[args[0]]
	if !ok {
		shells := make([]string, 0, len(completionScripts))
		for s := range completionScripts {
			shells = append(shells, s)
		}
		sort.Strings(shells)
		return fmt.Errorf("no completion for %q: rig has %s",
			args[0], strings.Join(shells, ", "))
	}
	fmt.Print(script)
	return nil
}

// cmdComplete answers one completion request.
//
// argv is everything typed after `rig`, MINUS the partial word the cursor is
// on - filtering by that prefix is the shell's job and every shell already
// does it. What this returns is every candidate for the next position.
func cmdComplete(argv []string) error {
	for _, s := range candidates(argv) {
		fmt.Println(s)
	}
	return nil
}

func candidates(argv []string) []string {
	switch len(argv) {
	case 0:
		return append(programIDs(), staticVerbs...)
	case 1:
		switch argv[0] {
		case "apps":
			return []string{"list"}
		case "ping", "describe":
			return programIDs()
		case "completion":
			return []string{"bash", "zsh", "fish"}
		case "down":
			// No positional argument: the estate is the runtime dir, so
			// there is nothing to name. Offering a program id here would
			// suggest `rig down <program>` is a thing, which is exactly the
			// named-estate concept proposal P5 was refused for wanting.
			return answersNothingElse()
		case "estate":
			// Same shape as down and for the same reason: the estate is the
			// runtime dir, so there is nothing to name. Offering a name here
			// would suggest `rig estate <name>` selects one, when the whole
			// point of the verb is that it reports the one you already
			// reached.
			return answersNothingElse()
		case "peers":
			// Same shape again. The roster belongs to the daemon this shell
			// reached, so there is no name to pass - and offering a SEAT name
			// here would be worse than offering a program id, because a seat
			// name is a real thing a reader would expect to filter on and
			// `rig peers` does not filter.
			return answersNothingElse()
		case "backup":
			// No positional argument, and the reason is section 46's
			// decision 6 rather than down's or estate's: the DAEMON chooses
			// the archive's directory and name, so there is no path a caller
			// could type here. Offering one would suggest `rig backup
			// <path>` writes where it is told, which is the request field
			// that decision exists to refuse.
			return flagNames(backupFlagSet().fs)
		case "restore":
			// ⛔ FLAGS ONLY, AND THE POSITIONAL IS DELIBERATELY LEFT TO THE
			// SHELL. `rig restore` takes an archive path, and a path is the
			// one thing every shell already completes better than rig could:
			// the archives live wherever the person copied them to, not only
			// in the directory `rig backup` wrote. Walked off the flag set so
			// --estate and --force cannot drift out of this list.
			return flagNames(restoreFlagSet().fs)
		case "record":
			// The seven subcommands, read off the dispatcher's own list so
			// this cannot drift from what `rig record` accepts. A fresh
			// slice, because callers of complete() append to what they get.
			return append([]string{}, recordSubcommands...)
		case "progress":
			return append([]string{}, progressSubcommands...)
		case verbVersion, "help":
			return []string{"--json"}
		}
		return commandsOf(argv[0])
	default:
		if argv[0] == "apps" {
			return []string{"--commands", "--depth", "--json"}
		}
		// `rig record put <TAB>` offers that subcommand's own flags. Without
		// this the word falls through to flagsOf, which asks the registry
		// about a PROGRAM called record and offers --args, a flag only a
		// declared command takes.
		if argv[0] == "record" && len(argv) == 2 {
			return flagNames(recordFlagSet(argv[1]).fs)
		}
		if argv[0] == "progress" && len(argv) == 2 {
			return flagNames(progressFlagSet().fs)
		}
		// The same guard for section 46's two verbs. Without it `rig restore
		// --estate a <TAB>` falls through to flagsOf, which asks the registry
		// about a PROGRAM called restore and offers --args - a flag only a
		// declared command takes.
		if argv[0] == "backup" {
			return flagNames(backupFlagSet().fs)
		}
		if argv[0] == "restore" {
			return flagNames(restoreFlagSet().fs)
		}
		// `rig describe <program> <TAB>` offers that program's commands. It
		// is the one static verb whose SECOND position is a program's own
		// namespace rather than a flag list, which is what makes describe
		// reachable by tab from nothing but the verb.
		if argv[0] == "describe" && len(argv) == 2 {
			return commandsOf(argv[1])
		}
		return flagsOf(argv[0], argv[1])
	}
}

// programIDs lists what is registered, or nothing at all.
func programIDs() []string {
	resp, ok := programsQuietly()
	if !ok {
		return nil
	}
	var out []string
	for _, p := range resp.GetPrograms() {
		out = append(out, p.GetIdentity().GetId())
	}
	sort.Strings(out)
	return out
}

// commandsOf lists one program's declared commands.
func commandsOf(program string) []string {
	resp, ok := programsQuietly()
	if !ok {
		return nil
	}
	for _, p := range resp.GetPrograms() {
		if p.GetIdentity().GetId() != program {
			continue
		}
		var out []string
		for _, c := range p.GetCommands() {
			out = append(out, c.GetId())
		}
		sort.Strings(out)
		return out
	}
	return nil
}

// flagsOf lists one command's declared flags, plus rig's own.
func flagsOf(program, command string) []string {
	resp, ok := programsQuietly()
	if !ok {
		return nil
	}
	out := []string{"--json", "--timeout", "--args", "--help"}
	for _, p := range resp.GetPrograms() {
		if p.GetIdentity().GetId() != program {
			continue
		}
		for _, c := range p.GetCommands() {
			if c.GetId() != command {
				continue
			}
			schema, err := parseArgSchema(c.GetArgs())
			if err != nil {
				return out
			}
			for _, name := range schema.names() {
				if name != "nothing" {
					out = append(out, name)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// programsQuietly reads the registry and swallows every failure.
//
// A completion is typed into a live shell. There is no useful way to report
// that rigd is down, and every unhelpful way makes the shell unusable.
func programsQuietly() (*rigv1.ProgramsResponse, bool) {
	c, err := client.Connect()
	if err != nil {
		return nil, false
	}
	defer c.Close()

	// Short, because this runs while someone is holding the tab key.
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	var resp rigv1.ProgramsResponse
	if err := call(ctx, c, "rig.programs", &rigv1.ProgramsRequest{}, &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

// completionScripts are the only shell code rig ships.
//
// Each one strips the partial word the cursor is on before calling back, so
// the contract with cmdComplete is the same in all three.
var completionScripts = map[string]string{
	"bash": `# rig completion for bash. eval "$(rig completion bash)"
_rig_complete() {
  local before out
  # Everything after "rig" and before the word being completed.
  before=("${COMP_WORDS[@]:1:COMP_CWORD-1}")
  out="$(rig __complete "${before[@]}" 2>/dev/null)"
  COMPREPLY=($(compgen -W "${out}" -- "${COMP_WORDS[COMP_CWORD]}"))
}
complete -F _rig_complete rig
`,

	"zsh": `# rig completion for zsh. eval "$(rig completion zsh)"
_rig() {
  local -a candidates
  local -a before
  before=(${words[2,$((CURRENT-1))]})
  candidates=(${(f)"$(rig __complete $before 2>/dev/null)"})
  compadd -- $candidates
}
compdef _rig rig
`,

	"fish": `# rig completion for fish. rig completion fish > ~/.config/fish/completions/rig.fish
function __rig_complete
    set -l typed (commandline -opc)
    # Drop "rig" itself. The word under the cursor is not in -opc, so what is
    # left is exactly the contract cmdComplete expects.
    set -e typed[1]
    rig __complete $typed 2>/dev/null
end
complete -c rig -f -a '(__rig_complete)'
`,
}
