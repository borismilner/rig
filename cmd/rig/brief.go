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

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
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
// ⛔ THE PROVISIONAL SHAPE IS GONE. This struct used to say it was shaped by
// what the renderer needed, until "the lead's wire message replaces it when it
// lands". `ProjectBriefResponse` landed; briefFromWire at the bottom of this
// file is that promise kept, field for field.
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

	// Cycles are the `blocks` cycles the derivation FOUND rather than ran
	// into. See BriefCycle.
	//
	// ⛔ THIS FIELD WAS CALLED `Blocked` AND IT HELD CYCLES, WHICH IS NOT WHAT
	// THE WIRE MEANS BY THAT WORD. `ProjectBriefResponse` carries `blocked`
	// (blockages: an item and who it waits on) and `cycles` separately, so a
	// mapping that trusted the name would have rendered every blockage as a
	// cycle and printed "the blocks graph has a cycle" over work that simply
	// has a dependency. Renamed when the wire landed, which is what
	// PROVISIONAL UNTIL SLICE 2 above promised.
	Cycles []BriefCycle

	// Blocked is section 39 row 4 proper: what is blocked, and on whom.
	//
	// IT WAS MISSING ENTIRELY. The renderer could show a cycle and could not
	// show an ordinary blockage, which is the common case and the one row 4 is
	// mostly about.
	Blocked []BriefBlockage

	// Sections is the state of all ELEVEN of section 39's sections.
	//
	// ⛔ RENDERING A SECTION WITHOUT ITS STATE IS THE DEFECT BORIS RULED OUT,
	// ONE LAYER DOWN. An empty notes list means "no notes" or "the derivation
	// does not collect them yet", and a reader of this brief has no other way
	// to tell. Whatever this client does with the rest, it must not present a
	// section as answered when the daemon said it was not.
	Sections []BriefSectionState
}

// BriefBlockage is one item and everything it waits on. Section 39 row 4.
type BriefBlockage struct {
	Item  string
	Title string

	// Blockers are RESOLVED rather than bare ids: once an `idea` item can
	// block, a blocker need not appear anywhere else in the brief, so an id
	// alone would render as a raw UUIDv7 at a human.
	Blockers []BriefBlocker
}

// BriefBlocker is one thing standing in the way.
type BriefBlocker struct {
	ID    string
	Title string

	// State is the blocker's latest progress step, and EMPTY IS MEANINGFUL: it
	// is section 39's `idea` case - nobody has picked this blocker up, so the
	// way forward is for somebody to start it. That is a different instruction
	// from a blocker somebody is already working on.
	State string
}

// BriefSectionState is one section's availability, as the daemon reported it.
type BriefSectionState struct {
	// Section is section 39's own row number, 1 to 11.
	Section int

	// Computed is whether this section's content means anything.
	Computed bool

	// Withheld is the view rule rather than a capability gap: the human view
	// does not carry the must-read set because it is not a decision he makes.
	// ⛔ NEVER FOLD THIS INTO !Computed - a section the caller is not being
	// shown and a section nothing can compute are different answers.
	Withheld bool

	// Reason is why, when it is not computed. Never empty in that case.
	Reason string
}

// BriefItem is one row of "next up". It mirrors `rigv1.ItemState`.
//
// ⛔ IT CARRIED `Owner` AND `Priority` AND THE WIRE HAS NEITHER, WHILE IT DID
// CARRY THE ITEM'S STATE, ITS AGE AND ITS LATEST NOTE - WHICH THIS STRUCT HAD
// NOWHERE TO PUT. So the table printed two columns nothing could ever fill and
// dropped the three that say whether the work is moving. Rendered, that is
// "(nobody)" under OWNER on every row of every brief: a sentence about the
// work, produced by a client that was never told anything about ownership.
type BriefItem struct {
	ID    string
	Title string

	// State is the item's LATEST STEP, and EMPTY IS A REAL STATE rather than
	// missing data: the stream is empty, so the item has been picked up and
	// nobody has reported on it. The wire spends its enum zero on exactly this
	// and the renderer must not collapse it into a blank.
	State string

	// Since is when that step landed, and ZERO MEANS THERE ARE NO STEPS.
	//
	// ⛔ IT MUST NOT RENDER AS "infinitely stale". An item nobody has stepped
	// sorts BELOW every real signal, not above it - the wire says so at the
	// field - and peersAgeCell prints a zero as "-" rather than as an age,
	// which is the behaviour this field relies on.
	Since time.Time

	// Note is the latest step's own line, which is usually the one thing in
	// the row that says WHY the item is where it is.
	Note string
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
			return json.NewEncoder(os.Stdout).Encode(briefJSON(brief, time.Now()))
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
func briefJSON(b Brief, now time.Time) map[string]any {
	next := make([]map[string]any, 0, len(b.NextUp))
	for _, it := range b.NextUp {
		next = append(next, map[string]any{
			"id":     it.ID,
			titleKey: it.Title,
			// An empty state is the item's stream being empty, which is an
			// ANSWER - picked up, not yet reported on - so it is emitted as ""
			// rather than omitted. A consumer branching on absence would have
			// to know that, where one reading "" learns it from the answer.
			"state": it.State,
			// The timestamp AND the age, for the reason recordJSON gives: a
			// consumer computing against its own clock must not be forced
			// through this one.
			"since_at":    provTime(it.Since),
			"since_age_s": provAge(it.Since, now),
			"note":        it.Note,
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
	cycles := make([]map[string]any, 0, len(b.Cycles))
	for _, c := range b.Cycles {
		items := c.Items
		if items == nil {
			items = []string{}
		}
		cycles = append(cycles, map[string]any{"items": items})
	}
	blocked := make([]map[string]any, 0, len(b.Blocked))
	for _, bl := range b.Blocked {
		on := make([]map[string]any, 0, len(bl.Blockers))
		for _, k := range bl.Blockers {
			on = append(on, map[string]any{
				"id": k.ID, titleKey: k.Title,
				// An empty state is the `idea` case and is rendered as such
				// rather than as an absent field, which would read as a
				// serialisation gap.
				"state": briefCell(k.State, "not started"),
			})
		}
		blocked = append(blocked, map[string]any{
			"item": bl.Item, titleKey: bl.Title, "blocked_by": on,
		})
	}
	sections := make([]map[string]any, 0, len(b.Sections))
	for _, sec := range b.Sections {
		row := map[string]any{"section": sec.Section, "computed": sec.Computed}
		if sec.Withheld {
			row["withheld_by_view"] = true
		}
		if !sec.Computed {
			row["reason"] = sec.Reason
		}
		sections = append(sections, row)
	}
	return map[string]any{
		"project": b.Project,
		"kind":    b.Kind,
		titleKey:  b.Title,
		"status":  b.Status,
		"semver":  b.Semver,
		"next_up": next,
		"notes":   notes,
		// `blocked` IS WHAT WAITS ON WHAT. `cycles` IS THE CYCLE REPORT AND
		// NOT A RESOLUTION - it carries the items and nothing that could be
		// read as an edge to break. ⛔ THESE TWO KEYS WERE ONE, UNDER THE
		// CYCLE'S MEANING, WHICH LEFT ROW 4's ORDINARY CASE WITH NO KEY AT
		// ALL.
		"blocked": blocked,
		"cycles":  cycles,
		// ⛔ WITHOUT THIS KEY EVERY EMPTY LIST ABOVE IS AMBIGUOUS between "no
		// such thing" and "not built yet".
		"sections": sections,
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
	sb.WriteString(briefBlockedSection(b.Cycles))
	sb.WriteString(briefBlockageSection(b.Blocked))
	sb.WriteString(briefNextUpSection(b.NextUp, now))
	sb.WriteString(briefNotesSection(b.Notes, now))
	sb.WriteString(briefUnavailableSection(b.Sections))
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
func briefNextUpSection(items []BriefItem, now time.Time) string {
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
			// ⛔ AN EMPTY STATE IS "picked up, nobody has reported on it",
			// WHICH IS A STATE. The wire spends its enum zero on it and says
			// so at the field; a blank cell here would read as a renderer that
			// ran out of things to print.
			briefCell(it.State, "not stepped"),
			// A zero `since` prints "-" rather than an age, because an item
			// nobody has stepped is not infinitely stale - it has no signal at
			// all, and those are different facts.
			peersAgeCell(provUnix(it.Since), now),
			briefCell(it.Title, "(no title)"),
			briefCell(firstLine(strings.TrimSpace(it.Note)), "-"),
		})
	}
	writeTable(&sb, []string{"ID", "STATE", "AGE", "TITLE", "LATEST NOTE"}, rows)

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

// briefBlockageSection is section 39 row 4 proper: what is blocked, and on whom.
//
// SEPARATE FROM THE CYCLE SECTION ABOVE, because they are different news. A
// cycle is a defect in the graph that rig refuses to resolve; a blockage is the
// work behaving normally, and the reader's question is only "on what".
func briefBlockageSection(blocked []BriefBlockage) string {
	if len(blocked) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nBLOCKED\n")
	for _, b := range blocked {
		sb.WriteString("  " + briefCell(b.Title, b.Item) + "\n")
		for _, k := range b.Blockers {
			// ⛔ AN EMPTY STATE IS THE `idea` CASE AND IS SPELLED OUT. Section
			// 39 makes it load-bearing: nobody has picked the blocker up, so
			// the way forward is for somebody to START it - which is a
			// different instruction from waiting on work in progress. Printing
			// an empty cell would collapse the two.
			sb.WriteString("    waits on " + briefCell(k.Title, k.ID) +
				" (" + briefCell(k.State, "not started - nobody has picked it up") +
				")\n")
		}
	}
	return sb.String()
}

// briefUnavailableSection names every section of the brief that could not be
// answered, and what each one waits on.
//
// ⛔ IT PRINTS WHAT IS MISSING, WHICH IS THE WHOLE POINT AND IS EASY TO READ AS
// NOISE. Boris ruled all eleven of section 39's sections into the MVP on
// 2026-09-16 - "Cover all of them" - after four shipped and seven were absent.
// The mechanism that makes that true is not a longer brief, it is a brief that
// says which of its own sections mean anything: an empty notes list is "no
// notes" or "the derivation does not collect them yet", and nothing else here
// can tell a reader which.
//
// IT GOES LAST, DELIBERATELY. The answer a reader came for is the work; this is
// the confidence interval on it. Printing it first would make every brief open
// with an apology.
func briefUnavailableSection(sections []BriefSectionState) string {
	var missing []BriefSectionState
	for _, s := range sections {
		if !s.Computed && !s.Withheld {
			missing = append(missing, s)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nNOT ANSWERED BY THIS BRIEF - " +
		"these sections are specified and not yet built,\nso their absence " +
		"above is NOT a statement that there is nothing to report:\n")
	for _, s := range missing {
		sb.WriteString("  section " + strconv.Itoa(s.Section) + ": " +
			briefCell(s.Reason, "no reason was given, which is itself a defect") +
			"\n")
	}
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

// ---- the wire ---------------------------------------------------------------

// briefFromWire is `ProjectBriefResponse` in this client's vocabulary.
//
// IT IS FIELD FOR FIELD AND THAT IS CHECKABLE, which is the point of keeping
// it beside the struct it fills rather than with the other wire code: a reader
// holding the proto open can walk the two together without a third file.
//
// ⛔ EVERY FIELD ON THAT MESSAGE THIS FUNCTION DOES NOT READ IS A SECTION THE
// DAEMON COMPUTED AND THIS CLIENT DROPPED, WHICH IS NOT THE SAME AS A SECTION
// THAT IS NOT BUILT. The ones still unread are `open`, `drift`, `health`,
// `features`, `feature_stages`, `case_notes`, `coarse_citations` and
// `must_read`. They are named here rather than left to be noticed, because
// their `sections` entries will say COMPUTED while nothing renders them - a
// partial answer that looks complete, on the verb the MVP is judged by.
func briefFromWire(r *rigv1.ProjectBriefResponse) Brief {
	b := Brief{
		Project: r.GetProject(),
		Kind:    r.GetKind(),
		Title:   r.GetTitle(),
		Status:  r.GetStatus(),
		Semver:  r.GetSemver(),
	}
	for _, it := range r.GetNextUp() {
		b.NextUp = append(b.NextUp, itemFromWire(it))
	}
	for _, n := range r.GetNotes() {
		b.Notes = append(b.Notes, noteFromWire(n))
	}
	for _, bl := range r.GetBlocked() {
		out := BriefBlockage{Item: bl.GetItem(), Title: bl.GetTitle()}
		for _, k := range bl.GetBlockers() {
			out.Blockers = append(out.Blockers, BriefBlocker{
				ID:    k.GetId(),
				Title: k.GetTitle(),
				// UNSPECIFIED becomes the empty string and the renderer spells
				// it out as "nobody has picked it up". That is section 39's
				// `idea` case and is LOAD-BEARING - a materially different
				// instruction from waiting on work in progress - so the zero
				// must not acquire a word of its own here.
				State: stepStateWord(k.GetState()),
			})
		}
		b.Blocked = append(b.Blocked, out)
	}
	for _, c := range r.GetCycles() {
		b.Cycles = append(b.Cycles, BriefCycle{Items: c.GetItems()})
	}
	for _, s := range r.GetSections() {
		b.Sections = append(b.Sections, sectionFromWire(s))
	}
	return b
}

func itemFromWire(i *rigv1.ItemState) BriefItem {
	out := BriefItem{
		ID:    i.GetId(),
		Title: i.GetTitle(),
		State: stepStateWord(i.GetState()),
		Note:  i.GetNote(),
	}
	// A ZERO STAYS THE ZERO time.Time RATHER THAN BECOMING 1970. The wire says
	// zero means there are no steps, and peersAgeCell prints the zero as "-"
	// rather than as an age, which is what keeps an unstepped item from
	// reading as the stalest thing in the list.
	if n := i.GetSinceUnixNano(); n != 0 {
		out.Since = time.Unix(0, n).UTC()
	}
	return out
}

func noteFromWire(n *rigv1.BriefNote) BriefNote {
	return BriefNote{
		ID:       n.GetId(),
		Priority: n.GetPriority(),
		Body:     n.GetBody(),
		Prov:     provFromWire(n.GetProv()),
	}
}

// sectionFromWire is one section's own statement about itself.
//
// ⛔ WITHHELD AND NOT-COMPUTED ARE NEVER COLLAPSED, AND THIS IS THE BOUNDARY
// WHERE COLLAPSING THEM WOULD BE EASIEST. A section the caller is not being
// shown and a section nothing can compute are different answers: the first
// says the derivation ran, the second says the input does not exist. Folding
// the view rule into the capability gap would make the human view read as a
// degraded agent view, which is the exact reading SectionState was added to
// prevent.
//
// AN UNSPECIFIED STATE IS NEITHER, deliberately. Section 21: the zero means
// nothing was said, which is never a fact about anything - so it falls through
// to the not-answered list carrying no reason, and briefUnavailableSection
// reports the missing reason as the defect it is.
func sectionFromWire(s *rigv1.BriefSectionStatus) BriefSectionState {
	return BriefSectionState{
		// The enum's NUMBER is section 39's own row number, which is why this
		// is a conversion rather than a lookup: the proto says the numbers
		// "are citations rather than an ordinal, and they are never
		// renumbered".
		Section:  int(s.GetSection().Number()),
		Computed: s.GetState() == rigv1.SectionState_SECTION_STATE_COMPUTED,
		Withheld: s.GetState() == rigv1.SectionState_SECTION_STATE_WITHHELD_BY_VIEW,
		Reason:   s.GetReason(),
	}
}

// stepStateWord is a StepState as a person reads it, and THE ZERO KEEPS ITS
// EMPTINESS.
//
// Every caller of this function treats "" as a real answer with its own
// sentence - "not stepped" in the next-up table, "nobody has picked it up" for
// a blocker - because the wire spends the enum's zero on precisely that. A
// word here would be a third spelling of it, in the one place none of those
// callers could override.
//
// A VALUE THIS BUILD HAS NO NAME FOR RENDERS AS THE SKEW TOKEN, not as the
// zero's emptiness. A newer daemon on the same wire major can send a fourth
// state, and reporting that as "not stepped" would turn a build skew into a
// fact about the work.
func stepStateWord(s rigv1.StepState) string {
	if s == rigv1.StepState_STEP_STATE_UNSPECIFIED {
		return ""
	}
	return wordOrSkew(s, "STEP_STATE_")
}
