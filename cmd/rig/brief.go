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

// `rig brief` - the derived answer to "what is going on here" (PLAN.md
// section 39).
//
// IT IS THE CALL EVERY ARRIVING SESSION MAKES, which is what shapes every
// rendering decision below: the reader is a session that knows nothing yet, so
// an empty section is a fact it needs rather than a section to drop, and an
// answer it cannot act on must say so in words rather than by leaving a blank.
//
// ⛔ IT TAKES A PROJECT **OR A CASE** AND THE VERB IS STILL CALLED
// project.brief. Section 39 ruled that deliberately: a case differs from a
// project in three fields, "twelve verbs is not better than eleven", and "the
// name stays and is mildly wrong; the alternative is a second verb whose body
// is the first one's". So `rig brief my-health` is a supported call, and the
// renderer below omits the rows a case does not have rather than printing them
// empty.

// Brief is what `project.brief` derives.
//
// ⛔ PROVISIONAL UNTIL SLICE 2. Section 39 specifies what a brief ANSWERS and
// not the message that carries it, so this struct is shaped by what the
// renderer needs and the lead's wire message replaces it when it lands.
//
// TWO THINGS SECTION 39 PUTS IN A BRIEF ARE DELIBERATELY MISSING, and they are
// missing for the same reason `rig standard` and `rig project gate` are not
// verbs in this build:
//
//   - THE MUST-READ SET. Section 39's read-before-write gate says
//     "project.brief returns the must-read set" - and the gate is SLICE 7,
//     off Boris's MVP. There is no must-read set to return yet, and a section
//     rendering one would be a surface promising a capability rigd does not
//     have.
//   - THE FLAGGED CITATION COUNT. The migration ruling says the 78% of
//     citations that resolve only to a whole section are "COUNTED AND
//     REPORTED AS FAILED ROWS", surfaced here. That is slice 8.
//
// Both are LOCKED rather than closed: the rows arrive with the slices that
// make them true, and this comment exists so the next reader knows they were
// read and left out rather than missed.
type Brief struct {
	// Project is the container's slug - which is also its id, section 39's
	// one exception to the UUIDv7 id scheme, for a project and a case alike.
	Project string

	// Kind is `project` or `case`, and it decides which rows below mean
	// anything. A case has no semver and its status is open/resolved rather
	// than idea/active.
	Kind string

	Title  string
	Status string

	// Semver is the version a PROJECT carries. Empty on a case, which does
	// not ship and so has no version to advance.
	Semver string

	// NextUp is up to the container's `next_up_n` work items, in expected
	// execution order. NEVER PADDED TO N - section 39 says so twice, once for
	// a project's next-up and once for a case's notes.
	NextUp []BriefItem

	// Notes are what Boris attached to this container and its items. A case
	// caps and orders them by `attention_n`; every other kind shows all of
	// them.
	Notes []BriefNote

	// Blocked are the `blocks` cycles the derivation FOUND rather than ran
	// into. See BriefCycle.
	Blocked []BriefCycle
}

// BriefItem is one row of "next up".
type BriefItem struct {
	ID       string
	Title    string
	Owner    string
	Priority string
}

// BriefNote is one thing attached to a record for an agent to read.
//
// IT CARRIES ITS PROVENANCE AND THAT IS A REQUIREMENT RATHER THAN A DETAIL.
// Section 39, on comments: "No status field, no resolution workflow ...
// Provenance already says who wrote it (Boris's session, distinct from an
// agent's), which is enough to tell a directive from routine progress
// narration." A note rendered without its author is a note an agent cannot
// weigh.
type BriefNote struct {
	ID       string
	Priority string
	Body     string
	Prov     Provenance
}

// BriefCycle is one cycle in the `blocks` graph.
//
// ⛔ A CYCLE IS DETECTED AND REPORTED, NEVER RESOLVED. Section 39 rules it, and
// the ruling reaches this renderer rather than stopping at the store: the
// cycle "appears in the brief as a BLOCKED CONDITION, naming the items in
// it", every item outside it keeps its place in the next-up list, and rig does
// not pick an edge to break - choosing which `blocks` edge is wrong is a
// judgement about the work, which is non-goal 1.
//
// So this type holds the ITEMS and nothing that could be read as a
// recommendation.
type BriefCycle struct{ Items []string }

// briefFlags is `rig brief`'s flag set, built here so a test can walk it.
type briefFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
}

func briefFlagSet() *briefFlags {
	b := &briefFlags{fs: flag.NewFlagSet("brief", flag.ContinueOnError)}
	b.asJSON = b.fs.Bool("json", false, "emit JSON")
	b.timeout = b.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	return b
}

// cmdBrief is `rig brief <project>`.
//
// NO SUBCOMMAND, and the positional is the container. `rig brief` with nothing
// is refused rather than defaulting to some project: rig has no notion of the
// project a shell is "in" - a runtime directory names an ESTATE, not a
// project - so guessing would be inventing one.
func cmdBrief(args []string) (err error) {
	flags, positional := partition(args)

	bf := briefFlagSet()
	if err := bf.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *bf.asJSON) }()

	if len(positional) != 1 {
		return badArgumentf("usage: rig brief <project> [--json]\n" +
			"       the argument is a project's or a case's SLUG, which is " +
			"also its record id")
	}

	// The container is settled before anything is opened, the same order
	// every other verb here uses.
	return withRecordAPI(*bf.timeout, func(ctx context.Context, api RecordAPI) error {
		brief, err := api.Brief(ctx, positional[0])
		if err != nil {
			return err
		}

		if *bf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(briefJSON(brief))
		}
		fmt.Print(briefText(brief, time.Now()))
		return nil
	})
}

// ---- rendering -------------------------------------------------------------

// briefJSON is the object --json emits.
//
// EVERY KEY ON EVERY ANSWER, and every list [] rather than null, for the
// argument peersJSON writes down: a null cannot be told from a field this
// build failed to set, and an empty next-up list is the ordinary answer on a
// project whose work is all done or not yet picked up.
//
// `semver` IS PRESENT AND EMPTY ON A CASE rather than omitted. A consumer
// branching on its absence would have to know that a case has no version;
// a consumer reading "" learns it from the answer.
func briefJSON(b Brief) map[string]any {
	next := make([]map[string]any, 0, len(b.NextUp))
	for _, it := range b.NextUp {
		next = append(next, map[string]any{
			"id":       it.ID,
			"title":    it.Title,
			"owner":    it.Owner,
			"priority": it.Priority,
		})
	}
	notes := make([]map[string]any, 0, len(b.Notes))
	for _, n := range b.Notes {
		notes = append(notes, map[string]any{
			"id":       n.ID,
			"priority": n.Priority,
			"body":     n.Body,
			"seat":     n.Prov.Seat,
			"session":  n.Prov.Session,
		})
	}
	blocked := make([]map[string]any, 0, len(b.Blocked))
	for _, c := range b.Blocked {
		items := c.Items
		if items == nil {
			items = []string{}
		}
		blocked = append(blocked, map[string]any{"items": items})
	}
	return map[string]any{
		"project": b.Project,
		"kind":    b.Kind,
		"title":   b.Title,
		"status":  b.Status,
		"semver":  b.Semver,
		"next_up": next,
		"notes":   notes,
		// `blocked` IS A CYCLE REPORT AND NOT A RESOLUTION. It carries the
		// items and nothing that could be read as an edge to break.
		"blocked": blocked,
	}
}

// briefText is the brief a person reads.
func briefText(b Brief, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(briefHeading(b))

	// ⛔ THE BLOCKED CONDITION COMES FIRST, BEFORE THE LIST IT AFFECTS.
	// Section 39 requires the cycle to be reported and the items outside it to
	// keep their place, so both are printed - and a reader who stops after the
	// next-up table must not be the one who misses that the ordering has a
	// hole in it.
	sb.WriteString(briefBlockedSection(b.Blocked))
	sb.WriteString(briefNextUpSection(b.NextUp))
	sb.WriteString(briefNotesSection(b.Notes, now))
	return sb.String()
}

// briefHeading is the container's own line.
//
// THE KIND IS PRINTED BECAUSE THE STATUS VOCABULARY DEPENDS ON IT. `active` is
// a project's and `open` is a case's, and a reader who cannot see which
// container they are looking at cannot tell a status they do not recognise
// from one this build rendered wrong.
func briefHeading(b Brief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s) %s", b.Project, briefKindWord(b.Kind),
		briefStatusWord(b.Status))

	// A CASE HAS NO SEMVER AND MUST NOT PRINT ONE. Section 39: "`semver` -
	// NO - a case does not ship, so it has no version to advance." Rendering
	// an empty one as "v" or as "v0.0.0" would invent a version for a thing
	// that has none.
	if b.Semver != "" {
		fmt.Fprintf(&sb, " v%s", b.Semver)
	}
	sb.WriteString("\n")

	if strings.TrimSpace(b.Title) == "" {
		sb.WriteString("(no title)\n")
	} else {
		sb.WriteString(b.Title + "\n")
	}
	return sb.String()
}

func briefKindWord(kind string) string {
	if kind == "" {
		// Section 21's zero: nothing was said is never a fact, and this one
		// decides how the status below reads.
		return "not said"
	}
	return kind
}

func briefStatusWord(status string) string {
	if status == "" {
		return "(no status)"
	}
	return status
}

// briefNextUpSection is the work in expected execution order.
func briefNextUpSection(items []BriefItem) string {
	var sb strings.Builder
	sb.WriteString("\nNEXT UP\n")

	// AN EMPTY NEXT-UP IS A SENTENCE. It is a real state - everything is done,
	// or everything is still an idea - and it is one an arriving session has
	// to be able to tell from a derivation that failed.
	if len(items) == 0 {
		sb.WriteString("Nothing is next up: no active work item is " +
			"unblocked.\nThis is a state, not an empty answer - an item " +
			"appears here once its status is\nactive and nothing it is " +
			"blocked by is outstanding.\n")
		return sb.String()
	}

	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.ID,
			briefCell(it.Owner, "(nobody)"),
			briefCell(it.Priority, "-"),
			briefCell(it.Title, "(no title)"),
		})
	}
	writeTable(&sb, []string{"ID", "OWNER", "PRIORITY", "TITLE"}, rows)

	// THE COUNT IS NOT COMPARED AGAINST next_up_n, ON PURPOSE. Section 39:
	// "Never padded to N". A line reading "3 of 5" would teach a reader that
	// two rows are missing when the truth is that there are three.
	fmt.Fprintf(&sb, "\n%d item%s, in expected execution order.\n",
		len(items), plural(len(items)))
	return sb.String()
}

// briefNotesSection is what has been attached for an agent to read.
func briefNotesSection(notes []BriefNote, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("\nNOTES\n")
	if len(notes) == 0 {
		sb.WriteString("Nothing has been attached to this one.\n")
		return sb.String()
	}

	rows := make([][]string, 0, len(notes))
	for _, n := range notes {
		rows = append(rows, []string{
			// WHO WROTE IT LEADS THE ROW. Section 39 makes provenance the way
			// an agent tells a directive from an agent's own narration, and a
			// column a reader has to scan back to is a column they read after
			// deciding.
			provWord(n.Prov.Seat),
			briefCell(n.Priority, "-"),
			peersAgeCell(provUnix(n.Prov.CreatedAt), now),
			firstLine(strings.TrimSpace(n.Body)),
		})
	}
	writeTable(&sb, []string{"WHO", "PRIORITY", "AGE", "NOTE"}, rows)
	fmt.Fprintf(&sb, "\n%d note%s.\n", len(notes), plural(len(notes)))
	return sb.String()
}

// briefBlockedSection reports the cycles and REFUSES TO RESOLVE THEM.
//
// It prints nothing at all when there are none, and that is the one section
// here that may be silent: a blocked condition is an exception, and a line
// saying "no cycles" on every brief would train a reader to skip the place the
// real one appears.
func briefBlockedSection(cycles []BriefCycle) string {
	if len(cycles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nBLOCKED CONDITION: the `blocks` graph has a cycle\n")
	for _, c := range cycles {
		if len(c.Items) == 0 {
			// A cycle with no items is a report that reports nothing, and it
			// must not render as a decorative warning. Naming the items is
			// the entire requirement.
			sb.WriteString("  a cycle was found and its items did not reach " +
				"this client\n")
			continue
		}
		// The first item is repeated at the end, because a cycle written as a
		// flat list reads as a chain and the reader has to be told it closes.
		sb.WriteString("  " + strings.Join(append(append([]string{}, c.Items...),
			c.Items[0]), " -> ") + "\n")
	}
	sb.WriteString("rig does not pick an edge to break: which dependency is " +
		"the wrong one is a\njudgement about the work. Every item outside " +
		"the cycle keeps its place below.\n")
	return sb.String()
}

// briefCell renders a value that may legitimately be absent, in a shape that
// cannot be mistaken for one that is present.
func briefCell(value, absent string) string {
	if strings.TrimSpace(value) == "" {
		return absent
	}
	return value
}
