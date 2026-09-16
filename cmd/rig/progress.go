package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
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
	// ⛔ THE THREE SPELLINGS ARE HELP TEXT, NOT A CHECK. rigd refuses an
	// unknown state by name and quotes the value back; this client does not
	// pre-validate, because two validators drift and one does not. What a
	// caller cannot do is discover the set from a daemon they have not called
	// yet, which is what this string is for.
	p.state = p.fs.String("state", "", "started, blocked or done")
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
		return badArgumentf("usage: rig progress step <item> --project " +
			"<project> --state <started|blocked|done> [--note <text>]\n" +
			"       the item is the work item's RECORD ID, which " +
			"`rig record query <project> work-item` lists")
	}
	item := positional[1]

	api, release, err := recordAPI()
	if err != nil {
		return err
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), *pf.timeout)
	defer cancel()

	// ⛔ NOTHING IS CHECKED HERE BEYOND ARGUMENT SHAPE, AND THAT IS A RULING
	// RATHER THAN AN OMISSION. An empty --state, an empty --project and an
	// unknown state are all refused by rigd, each in a sentence naming what
	// was wrong. A client that refuses them first is a second copy of section
	// 39's rules in a file nobody reads them from, and the day the two
	// disagree the caller is told something that is not true of the daemon.
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
		// A STEP IS A RECORD AND EMITS THE RECORD OBJECT. One shape for one
		// noun: a consumer that reads `rig record get` does not need a second
		// parser to read what a step wrote, and the fields that make it a
		// step - `state` and `item` - are typed fields inside it, exactly as
		// internal/record writes them.
		return json.NewEncoder(os.Stdout).Encode(recordJSON(step, time.Now()))
	}
	fmt.Print(stepText(step, time.Now()))
	return nil
}

// stepStateSpellings is the set, for a usage line. It is the one place this
// file writes them down; see the --state flag's comment for why it is not a
// check.
func stepStateSpellings() []string {
	return []string{"started", "blocked", "done"}
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
