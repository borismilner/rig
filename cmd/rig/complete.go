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

// staticVerbs are rig's own, and the only names in this file.
var staticVerbs = []string{"apps", "ping", "down", "version", "completion", "help"}

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
		case "ping":
			return programIDs()
		case "completion":
			return []string{"bash", "zsh", "fish"}
		case "down":
			// No positional argument: the estate is the runtime dir, so
			// there is nothing to name. Offering a program id here would
			// suggest `rig down <program>` is a thing, which is exactly the
			// named-estate concept proposal P5 was refused for wanting.
			return []string{"--json", "--timeout"}
		case "version", "help":
			return []string{"--json"}
		}
		return commandsOf(argv[0])
	default:
		if argv[0] == "apps" {
			return []string{"--commands", "--json"}
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
	if err := c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{}, &resp); err != nil {
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
