package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// `rig progress` - a work item's stream (PLAN.md section 39).
//
// IT IS ITS OWN COMMAND RATHER THAN AN EIGHTH `rig record` SUBCOMMAND, and
// section 39 names it that way: "ON THE CLI: rig record, rig progress, rig
// standard, rig brief." The grouping is not cosmetic. A step is not a record a
// caller writes - internal/record REFUSES a put of kind `progress`, in as many
// words - because "the latest step is the live state" stops being true the
// first time anybody supersedes one. Two words for two different operations is
// the surface telling a reader that before they find out.
//
// ONE SUBCOMMAND TODAY AND THE SHAPE STILL EARNS ITS KEEP. `rig progress step`
// reads as what it does; `rig step` would read as a rig-wide verb and would
// have to be learned separately from the record it belongs to.

// progressSubcommands are what `rig progress` takes. One, and the plural stays
// because the dispatcher and the completion both read this list.
var progressSubcommands = []string{"step"}

// progressFlags is `rig progress step`'s flag set and what it parsed.
//
// Built in a function rather than inline so a test can WALK it: a verb whose
// flags are built inside its own body is invisible to the check that every
// non-boolean flag is declared to the partitioner, and that check is the only
// thing coupling a new flag to valuedFlags.
type progressFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	project *string
	state   *string
	note    *string
}

func progressFlagSet() *progressFlags {
	p := &progressFlags{fs: flag.NewFlagSet("progress step", flag.ContinueOnError)}
	p.asJSON = p.fs.Bool("json", false, "emit JSON")
	p.timeout = p.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	p.project = p.fs.String("project", "", "the project or case the item is in")
	// ⛔ THIS STRING IS HELP TEXT AND stepStateOnTheWire IS THE CHECK, AND THE
	// TWO ARE NOT THE SAME LIST.
	//
	// This comment used to say there was no check here at all, because rigd
	// "refuses an unknown state by name and quotes the value back". THE ENUM
	// ENDED THAT: `--state banana` has no value on the wire, arrives as
	// UNSPECIFIED, and the daemon maps UNSPECIFIED to the empty string - so
	// the store's refusal quotes "" and the caller's word is gone. The check
	// moved here because this is the last place that word exists.
	//
	// Neither is a second source of truth: stepStateOnTheWire and
	// stepStateSpellings both WALK the enum's descriptor, so this help text
	// names the set a caller cannot discover from a daemon they have not
	// called yet without writing it down a second time.
	p.state = p.fs.String("state", "", stepStateHelp())
	p.note = p.fs.String("note", "", "one line of what happened")
	return p
}

// cmdProgress is `rig progress`.
func cmdProgress(args []string) (err error) {
	flags, positional := partition(args)

	pf := progressFlagSet()
	if err := pf.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *pf.asJSON) }()

	if len(positional) == 0 {
		return badArgumentf("usage: rig progress step <item> --project "+
			"<project> --state <%s>", strings.Join(stepStateSpellings(), "|"))
	}
	if positional[0] != "step" {
		return badArgumentf("%q is not a progress subcommand; there is one: %s",
			positional[0], strings.Join(progressSubcommands, ", "))
	}
	if len(positional) != 2 {
		return badArgumentf("usage: rig progress step <item> --project "+
			"<project> --state <%s> [--note <text>]\n"+
			"       the item is the work item's RECORD ID, which "+
			"`rig record query <project> work-item` lists",
			strings.Join(stepStateSpellings(), "|"))
	}
	item := positional[1]

	// THE ARGUMENT SHAPE IS SETTLED ABOVE, BEFORE ANYTHING IS OPENED. Every
	// verb in this package refuses a malformed command before it dials, and a
	// caller with a bad command must be told about the command rather than
	// about the daemon.
	return withRecordAPI(*pf.timeout, func(ctx context.Context, api RecordAPI) error {
		// ⛔ AN EMPTY --state AND AN EMPTY --project ARE rigd's TO REFUSE, AND
		// AN UNSPELLABLE STATE IS NOT. That line moved, and where it moved to
		// is the wire.
		//
		// rigd refuses an empty state and an empty project each in a sentence
		// naming what was wrong, and a client that refused them first would be
		// a second copy of section 39's rules in a file nobody reads them
		// from. That still holds for both. It stopped holding for an UNKNOWN
		// state the moment the field became an enum: the word does not survive
		// the wire, so rigd cannot name it and this client is the last place
		// that can. stepStateOnTheWire in record.go does it, off the enum's
		// own descriptor, which is why it is not the second validator this
		// paragraph exists to refuse.
		step, err := api.Step(ctx, StepArgs{
			Item:    item,
			State:   *pf.state,
			Note:    *pf.note,
			Project: *pf.project,
		})
		if err != nil {
			return err
		}

		if *pf.asJSON {
			// A STEP IS A RECORD AND EMITS THE RECORD OBJECT. One shape for
			// one noun: a consumer that reads `rig record get` needs no
			// second parser to read what a step wrote, and the fields that
			// make it a step - `state` and `item` - are typed fields inside
			// it, exactly as internal/record writes them.
			return json.NewEncoder(os.Stdout).Encode(recordJSON(step, time.Now()))
		}
		fmt.Print(stepText(step, time.Now()))
		return nil
	})
}

// stepStateSpellings is the set, for a usage line, read off the wire enum's
// descriptor the same way stepStateOnTheWire reads it. The enum is the
// vocabulary (internal/record/progress.go has the ruling), so this file
// writes none of the words down.
func stepStateSpellings() []string {
	values := rigv1.StepState_STEP_STATE_UNSPECIFIED.Descriptor().Values()
	var out []string
	for i := range values.Len() {
		if v := values.Get(i); v.Number() != 0 {
			out = append(out, enumLabel(string(v.Name()), "STEP_STATE_"))
		}
	}
	return out
}

// stepStateHelp is the set as a sentence, for the --state flag's help.
func stepStateHelp() string {
	s := stepStateSpellings()
	if len(s) < 2 {
		return strings.Join(s, "")
	}
	return strings.Join(s[:len(s)-1], ", ") + " or " + s[len(s)-1]
}

// stepText is the confirmation a person reads.
//
// ⛔ IT RENDERS WHAT WAS RECORDED, NOT WHAT WAS ASKED. Echoing the request
// would report success in the caller's own words, which is the one rendering
// that cannot fail: a daemon that stored a different state, or dropped the
// item edge, would print exactly the same line. Every value below comes off
// the record that came back.
func stepText(step Record, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s on %s\n", stepField(step, "state"), stepField(step, "item"))

	// The note is the one optional part, and its absence is stated rather
	// than left as a blank line. Section 39: a step with a state and no note
	// is still the signal that something moved.
	if strings.TrimSpace(step.Body) == "" {
		b.WriteString("(no note)\n")
	} else {
		b.WriteString(step.Body + "\n")
	}

	fmt.Fprintf(&b, "%s in %s\n", step.ID, step.Project)
	b.WriteString(provenanceLine(step.Prov, now))
	return b.String()
}

// stepField reads one of the two typed fields that make a record a step.
//
// `state` and `item` are the keys internal/record/progress.go writes, and an
// ABSENT ONE IS A DEFECT RATHER THAN A BLANK. A step without a state cannot be
// written - rigd refuses it - so a missing key here means it did not survive
// the wire, and section 21's rule applies: nothing was said is never a fact
// about anything, and it must not render as something a reader could mistake
// for a value.
func stepField(step Record, key string) string {
	if v := step.Fields[key]; v != "" {
		return v
	}
	return "(not said: " + key + ")"
}
