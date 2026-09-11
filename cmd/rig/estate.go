package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// cmdEstate answers "which estate did I just reach" (PLAN.md section 37).
//
// THE VERB EXISTS BECAUSE A TERMINAL NEVER HANDSHAKES. Every other way to
// learn the estate's identity rides the hello a program sends at connect, and
// a person at a prompt sends none - so rig.estate landed on the wire as a self
// method and was reachable from no binary at all until this file. Two estates
// on one machine are two XDG_RUNTIME_DIRs, and until now the only way to tell
// which one a shell was pointed at was to read the environment and believe it.
//
// EstateRequest is empty on purpose, on rig.programs' precedent: the role is
// derived by the daemon from the name, so a request field a client could write
// a role into would be a second place for the two to disagree from.
func cmdEstate(args []string) (err error) {
	fs, asJSON, timeout := estateFlagSet()
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) != 0 {
		return badArgumentf("usage: rig estate [--json] [--timeout=30s]")
	}

	c, err := client.Connect()
	if err != nil {
		// Unlike `rig down`, an unreachable socket is not a form of the
		// answer. "Which estate did I reach" has no true answer when nothing
		// was reached, and reporting the runtime dir rig WOULD have used is
		// the guess this verb exists to remove.
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resp := &rigv1.EstateResponse{}
	if err := call(ctx, c, "rig.estate", &rigv1.EstateRequest{}, resp); err != nil {
		return err
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(estateJSON(resp))
	}
	fmt.Print(estateText(resp))
	return nil
}

// estateFlagSet is `estate`'s flags, built here rather than inline so a test
// can WALK them - see TestEveryFlagThatTakesAValueIsDeclaredToThePartitioner,
// which walks every flag set this package builds because walking one of them
// could not see a second verb's flags.
func estateFlagSet() (fs *flag.FlagSet, asJSON *bool, timeout *time.Duration) {
	fs = flag.NewFlagSet("estate", flag.ContinueOnError)
	asJSON = fs.Bool("json", false, "emit JSON")
	timeout = fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	return fs, asJSON, timeout
}

// ---- rendering, and the whole design is the unnamed case -------------------

// THE ZERO AND UNNAMED MUST RENDER DIFFERENTLY AND MUST NEVER COLLAPSE.
//
// ESTATE_ROLE_UNSPECIFIED is 0 and ESTATE_ROLE_UNNAMED is 1, and the wire gave
// the identity fact a number of its own for a reason this renderer is the last
// line of: proto3 cannot tell an unset scalar from a zero one, so a daemon
// that carried the field and failed to set it - a bug, or a build where the
// field landed and the setter did not - arrives here indistinguishable from a
// daemon that meant it. An unnamed estate reads as ephemeral and disposable
// and a production one does not, so the safe guess and the useful guess point
// opposite ways and a renderer that picks either is wrong half the time.
//
// So the zero is rendered as what section 21 says it is - nothing was said -
// and never as a fact about the estate.

// roleLabel is the spelling for a role, walked off the wire's own enum
// descriptor rather than carried as a table here.
//
// It returns ok=false for a number this build's descriptor does not contain,
// which is a newer daemon on the same wire major: enums are extensible within
// a major, so a fifth role can arrive here and this binary is the older half.
// The caller renders that as its own case. What it must NOT do is fall back to
// the zero's spelling, which would report a skew as "nothing was said".
//
// The generated String() is deliberately not used: for a number outside the
// descriptor it returns the DECIMAL, so enumLabel would yield "4" and a
// renderer would print a bare digit where every other value is a word.
func roleLabel(r rigv1.EstateRole) (string, bool) {
	v := rigv1.EstateRole(0).Descriptor().Values().ByNumber(r.Number())
	if v == nil {
		return "", false
	}
	return enumLabel(string(v.Name()), "ESTATE_ROLE_"), true
}

// roleUnrecognised is the spelling for a role number this build does not know.
//
// A word rather than a digit, and a word that cannot be mistaken for one of
// the four: an agent branching on this string must fall through to "I do not
// know what I reached" rather than onto any behaviour.
const roleUnrecognised = "unrecognised"

// estateJSON is the object --json emits. Section 10: the proto's own field
// names are the contract, which is why they are spelled as the wire spells
// them rather than in the lowerCamelCase protojson would produce.
//
// EVERY KEY IS PRESENT ON EVERY ANSWER, with no omitempty anywhere, and that
// is the same argument answerJSON writes down for `partial`. An absent key
// reads as "this was never considered", and all five of these were considered
// on every call - an empty name is the ANSWER for an unnamed estate, not a
// missing one. A key that comes and goes is also how a consumer learns to
// treat absence as a value, which is the distinction the role enum spends its
// zero to keep.
//
// role_number rides beside role because it is the only thing that makes
// `unrecognised` actionable: without it a client that meets a fifth role can
// report that it did not understand and cannot say what it did not understand.
// It is emitted ALWAYS rather than only in that case, because a key that
// appears only when something is wrong is a key nobody's parser has a branch
// for at the moment it first appears.
func estateJSON(r *rigv1.EstateResponse) map[string]any {
	label, ok := roleLabel(r.GetRole())
	if !ok {
		label = roleUnrecognised
	}
	return map[string]any{
		"name":           r.GetName(),
		"role":           label,
		"role_number":    int32(r.GetRole().Number()),
		"daemon_version": r.GetDaemonVersion(),
		"wire":           r.GetWire(),
		"semantics_gen":  r.GetSemanticsGen(),
	}
}

// estateText is the human rendering. It is a function of its own, taking the
// response rather than reaching the wire, so every case below is testable
// without a live daemon - including the two this daemon cannot currently
// produce.
func estateText(r *rigv1.EstateResponse) string {
	var b strings.Builder
	row := func(label, value string) {
		fmt.Fprintf(&b, "%-*s%s\n", estateColumn, label, value)
	}
	row("name", estateNameCell(r))
	row("role", estateRoleCell(r))
	row("daemon", r.GetDaemonVersion())
	row("wire", r.GetWire())
	row("semantics", strconv.Itoa(int(r.GetSemanticsGen())))
	return b.String()
}

// estateColumn is the column a value starts in. Nine is the longest label
// ("semantics") plus two spaces of gutter.
const estateColumn = 9 + 2

// estateNameCell renders the name, and an empty one is a FACT rather than a
// blank.
//
// A bare empty cell is the one rendering that loses the distinction this verb
// is built around: a person reading `name` followed by nothing cannot tell an
// estate that claimed no name from a daemon that failed to send one. The
// parenthesised form cannot be a name, because a name is drawn from a closed
// set that contains neither parentheses nor spaces.
func estateNameCell(r *rigv1.EstateResponse) string {
	if r.GetName() == "" {
		return "(none claimed)"
	}
	return r.GetName()
}

// estateRoleCell renders the role, and the two cases that are not facts about
// the estate say so in the line rather than leaving the reader to know.
func estateRoleCell(r *rigv1.EstateResponse) string {
	label, ok := roleLabel(r.GetRole())
	switch {
	case !ok:
		// A newer daemon on this wire major. The number is printed because it
		// is the only actionable thing here, and the sentence says which side
		// is old so the reader does not go looking at the daemon.
		return fmt.Sprintf("%s (%d) - this rig is older than the daemon "+
			"it reached and has no name for that role",
			roleUnrecognised, r.GetRole().Number())
	case r.GetRole() == rigv1.EstateRole_ESTATE_ROLE_UNSPECIFIED:
		// SECTION 21: the zero means nothing was said, and it is never a fact
		// about an estate. It cannot be reached through today's rigd, which
		// refuses a name outside the closed set before the daemon is built -
		// so a reader who sees this line is looking at a defect and the line
		// has to be the thing that tells them.
		// The word this line must NOT contain is the one for role 1. A reader
		// skimming a negation keeps the word and drops the "not", which is the
		// collapse arriving through the sentence instead of through the enum.
		return label + " - the daemon said nothing, so this is a defect " +
			"rather than a fact about the estate"
	default:
		return label
	}
}
