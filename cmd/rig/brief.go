package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"

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

	// Open is every work item that is open. Section 39 row 1.
	//
	// ⛔ Open AND NextUp ARE DISJOINT BY CONSTRUCTION AND ARE NOT TO BE
	// MERGED. The wire cuts them that way deliberately: an item in both got
	// two incompatible rules for rendering its notes, which is why next-up is
	// not a prefix of open. So the two lists below print as two sections and
	// a reader adding them up gets the real total.
	Open []BriefItem

	// NextUp is up to the container's `next_up_n` work items, IN THE ORDER
	// rigd SENT THEM AND NOTHING MORE. NEVER PADDED TO N - section 39 says so
	// twice, once for a project's next-up and once for a case's notes.
	//
	// ⛔ THIS FIELD USED TO PROMISE THE ORDER THE WORK WAS EXPECTED TO BE DONE
	// IN, AND THAT WAS NOT TRUE - the exact wording is gone from this file on
	// purpose, and a test keeps it gone, so a reader grepping for the old
	// claim finds nothing that still makes it.
	//
	// `ItemState` on the wire carries id, title, state, since and note;
	// there is no priority on it, so nothing that reaches this client ranks
	// the work. Measured against the live estate on 2026-09-17, 0 of 68 work
	// items carried a priority, the derivation's priority-then-id sort
	// therefore degenerated to a string sort, and the printed head was
	// `B1 B10 B12 B13 B14` - exactly the first five sorted ids, with B8
	// printing 59th of 59. The doc was the worst of the three sites, because
	// it is where the next reader learns what the field means and writes the
	// printed claim back.
	//
	// THE SORT IS NOT THE DEFECT. It is correct and the store has nothing for
	// it to sort on; inventing an order here would put a ranking in front of
	// a reader that nobody decided. The renderer says what the order IS, and
	// briefOrderLine derives that from the ids it was handed.
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

	// Features and FeatureStages are section 39 row 10: what this project has,
	// and how many sit at each stage.
	//
	// THE COUNTS ARE A LIST AND NOT A MAP, following the wire, which spends a
	// paragraph on why: protobuf map iteration order is unspecified, and a
	// map renders one answer two ways. A list is ordered by construction.
	Features      []BriefFeature
	FeatureStages []BriefStageCount

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

// BriefFeature is one feature of this project. Section 39 row 10.
type BriefFeature struct {
	ID    string
	Title string

	// Stage is where the feature has got to. EMPTY IS A DEFECT rather than a
	// stage: a feature with no stage cannot be placed against the counts
	// below, and the renderer says so rather than leaving the cell blank.
	Stage string
}

// BriefStageCount is how many features sit at one stage.
type BriefStageCount struct {
	Stage string
	Count uint64
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
		fmt.Print(briefText(brief, time.Now(), briefStyleFor(os.Stdout)))
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
	open := briefItemsJSON(b.Open, now)
	features := make([]map[string]any, 0, len(b.Features))
	for _, f := range b.Features {
		features = append(features, map[string]any{
			"id": f.ID, "stage": f.Stage, titleKey: f.Title,
		})
	}
	stages := make([]map[string]any, 0, len(b.FeatureStages))
	for _, c := range b.FeatureStages {
		stages = append(stages, map[string]any{"stage": c.Stage, "count": c.Count})
	}
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
		// ⛔ `open` AND `next_up` ARE DISJOINT AND ARE NOT TO BE MERGED. The
		// wire cuts them that way so an item never gets two incompatible
		// rules for rendering its notes, and a consumer concatenating them
		// gets the real total rather than a double count.
		"open":           open,
		"next_up":        next,
		"notes":          notes,
		"features":       features,
		"feature_stages": stages,
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

// briefItemsJSON is the object form of an item list, shared by `open` and
// `next_up` so one noun cannot acquire two shapes.
func briefItemsJSON(items []BriefItem, now time.Time) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id":     it.ID,
			titleKey: it.Title,
			// An empty state is the item's stream being empty, which is an
			// ANSWER - picked up, not yet reported on - so it is emitted as ""
			// rather than omitted.
			"state": it.State,
			// The timestamp AND the age, for the reason recordJSON gives: a
			// consumer computing against its own clock must not be forced
			// through this one.
			"since_at":    provTime(it.Since),
			"since_age_s": provAge(it.Since, now),
			"note":        it.Note,
		})
	}
	return out
}

// ---- the output device -----------------------------------------------------

// briefStyle is everything about the DEVICE the brief is being painted on,
// and it is a PARAMETER rather than a package variable so one process can
// render the same brief for an 80-column terminal, a 200-column one and a
// pipe without any of the three seeing the others' answer.
//
// ⛔ THE ZERO VALUE IS A PIPE, and that is the safe default. Unbounded width,
// no escapes: every consumer that is not a person - `| grep`, `> file`, a
// capture in another program - gets exactly the bytes it got before this
// type existed.
type briefStyle struct {
	// Width is the terminal's column count, and ZERO MEANS UNBOUNDED rather
	// than zero-wide. Nothing is cut at zero: a pipe has no width to fit, and
	// discarding bytes it was going to read in full is destruction rather
	// than legibility.
	Width int

	// Bold is whether the device can carry SGR attributes.
	//
	// ⛔ IT IS BOLD AND NOT COLOUR, AND THAT IS A MEASUREMENT DECISION RATHER
	// THAN A TASTE ONE. A colour's contrast ratio depends on the terminal's
	// palette and its background, neither of which this process can read, so
	// a colour here is a change nobody can put a number on and this
	// repository does not make those. SGR 1 changes the WEIGHT of a glyph and
	// leaves the foreground pair the terminal already chose, so the ratio is
	// unchanged by construction and the only thing that moves is how many
	// visual levels the page has.
	Bold bool
}

// briefStyleFor asks a file descriptor what it is, in ONE ioctl.
//
// ⛔ `TIOCGWINSZ` ANSWERS BOTH QUESTIONS AT ONCE. It fails with ENOTTY on a
// pipe, on a regular file and on /dev/null, and succeeds on a terminal
// carrying its size - so "is anybody watching" and "how wide are they" come
// from one call and cannot disagree with each other.
//
// THE LIBRARY WAS SEARCHED FOR RATHER THAN SKIPPED, which section 38
// requires. `golang.org/x/term` (`term.GetSize`) and
// `github.com/mattn/go-isatty` both do this, and both would arrive as a NEW
// DIRECT dependency needing a section 22 row. `golang.org/x/sys/unix` is
// already direct, already has its row, and carries the same ioctl wrapper -
// so the syscall plumbing here is the library's and only the policy is ours.
//
// $COLUMNS WAS REJECTED WITH A REASON. Most shells keep it as a shell
// variable and never export it, so a child process reads it as absent on a
// terminal that is plainly there.
//
// (no-color.org). It is an identifier, not a word this repository spells.
//
//nolint:misspell // NO_COLOR is the environment variable's actual name
func briefStyleFor(f *os.File) briefStyle {
	// A closed handle reports ^uintptr(0) as its descriptor, the ioctl comes
	// back EBADF, and this returns the pipe answer rather than panicking.
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return briefStyle{}
	}
	st := briefStyle{
		// NO_COLOR IS HONOURED AND IT DOES NOT TAKE THE WIDTH WITH IT. The
		// convention (no-color.org) is that any non-empty value turns
		// decoration off. It says nothing about layout, and the layout is the
		// larger of the two repairs, so collapsing the two would throw away
		// the bigger fix to honour the smaller one.
		Bold: os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb",
	}
	// A terminal that reports no size is still a terminal. It keeps the
	// attribute and renders unbounded, which is what it did before.
	if ws.Col > 0 {
		st.Width = int(ws.Col)
	}
	return st
}

// strong is the ONE attribute this renderer emits.
//
// It closes with SGR 22 (normal intensity) rather than SGR 0 (reset
// everything): a full reset turns off attributes this renderer never turned
// on, and those belong to whoever set them.
func (s briefStyle) strong(text string) string {
	if !s.Bold {
		return text
	}
	return "\x1b[1m" + text + "\x1b[22m"
}

// briefText is the brief a person reads.
func briefText(b Brief, now time.Time, st briefStyle) string {
	var sb strings.Builder
	sb.WriteString(briefHeading(b))

	// ⛔ THE BLOCKED CONDITION COMES FIRST, BEFORE THE LIST IT AFFECTS.
	// Section 39 requires the cycle to be reported and the items outside it to
	// keep their place, so both are printed - and a reader who stops after the
	// next-up table must not be the one who misses that the ordering has a
	// hole in it.
	sb.WriteString(briefBlockedSection(b.Cycles, st))
	sb.WriteString(briefNextUpSection(b.NextUp, now, st))
	// ⛔ ROW 4 SITS AFTER THE LIST IT EXPLAINS, AND THE CYCLE REPORT ABOVE
	// DOES NOT. They moved apart when row 4 started printing on an empty
	// list: a cycle puts a HOLE in the next-up ordering, so a reader who
	// stops after the table must not be the one who misses it - but an
	// ordinary blockage explains why an item is ABSENT from that table, and
	// it is only readable once the reader has seen the table. Leading every
	// brief with "Nothing is blocked" buries the answer the reader came for.
	sb.WriteString(briefBlockageSection(b.Blocked, st))
	sb.WriteString(briefOpenSection(b.Open, now, st))
	sb.WriteString(briefNotesSection(b.Notes, now, st))
	sb.WriteString(briefFeaturesSection(b.Features, b.FeatureStages, st))
	sb.WriteString(briefUnavailableSection(b.Sections, st))
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

// briefNextUpSection is the work rigd put at the front, in the order it sent.
//
// ⛔ IT MAY NOT PROMISE AN ORDER IT CANNOT SEE. See `Brief.NextUp`: the wire
// carries no priority, so the only order this client knows about is the one
// it was handed, and the caption says exactly that and no more.
func briefNextUpSection(items []BriefItem, now time.Time, st briefStyle) string {
	var sb strings.Builder
	sb.WriteString("\n" + st.strong("NEXT UP") + "\n")

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

	sb.WriteString(briefItemTable(items, now, st))

	// THE COUNT IS NOT COMPARED AGAINST next_up_n, ON PURPOSE. Section 39:
	// "Never padded to N". A line reading "3 of 5" would teach a reader that
	// two rows are missing when the truth is that there are three.
	sb.WriteString("\n" + briefWrap(fmt.Sprintf("%d item%s, %s",
		len(items), plural(len(items)), briefOrderLine(items)), st.Width))
	return sb.String()
}

// briefOrderLine says what the printed order ACTUALLY IS.
//
// ⛔ IT IS DERIVED, NEVER ASSERTED, AND THAT IS THE WHOLE REPAIR. This client
// holds the ids it was handed and can compare their order against those same
// ids sorted. That comparison is a fact it owns; anything about WHY rigd
// chose the order is not, because no field carrying a reason reaches here.
//
// AND IT RETIRES ITSELF. The day work items carry priorities the derivation's
// order stops matching the string sort, the second sentence stops printing,
// and no line of this file has to be found and changed by whoever lands them.
func briefOrderLine(items []BriefItem) string {
	const sent = "in the order rigd sent them"
	if len(items) < 2 {
		return sent + "."
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	if !slices.IsSorted(ids) {
		return sent + "."
	}
	return sent + ", which here is exactly the ids sorted - alphabetical, " +
		"and not a judgement about what to do first."
}

// briefOpenSection is every open work item. Section 39 row 1.
//
// IT IS A SEPARATE SECTION FROM NEXT-UP AND NOT A SUPERSET OF IT. The wire
// cuts the two lists disjoint on purpose, so a reader adding them up gets the
// real total and neither list has to be subtracted from the other.
//
// ⛔ IT PRINTS EVEN WHEN EMPTY, because the daemon reports row 1 COMPUTED and
// a computed section that renders nothing cannot be told from one this build
// has no renderer for.
func briefOpenSection(items []BriefItem, now time.Time, st briefStyle) string {
	var sb strings.Builder
	sb.WriteString("\n" + st.strong("ALSO OPEN") + "\n")
	if len(items) == 0 {
		sb.WriteString("Nothing else is open: every open item is in the " +
			"next-up list above.\n")
		return sb.String()
	}
	sb.WriteString(briefItemTable(items, now, st))
	fmt.Fprintf(&sb, "\n%d open item%s beyond the next-up list.\n",
		len(items), plural(len(items)))
	return sb.String()
}

// briefItemTable is the rows shared by next-up and open, so the two sections
// cannot drift into rendering one noun two ways.
//
// ⛔ THREE OF ITS FIVE COLUMNS HELD ONE DISTINCT VALUE ACROSS 54 ROWS. Against
// the live estate on 2026-09-17: STATE was `not stepped` on every row, AGE was
// `-` on every row, LATEST NOTE was `-` on every row. 29 characters of screen
// per row times 54 rows is 1,566 characters spent repeating three constants,
// on a table whose one discriminating stored field - `tags`, four values on
// eight of those rows - the wire does not carry at all.
//
// So a constant column is now STATED ONCE instead of printed per row, and
// the decision is taken from the DATA on every render rather than from a list
// of columns somebody judged dead. That is what makes it survive the repair
// going on in parallel: the write path currently flattens every terminal
// disposition to `active`, and the day it stops, STATE varies, and the column
// comes back here with no change to this file.
func briefItemTable(items []BriefItem, now time.Time, st briefStyle) string {
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
	// ⛔ ID AND TITLE ARE NOT IN THE COLLAPSIBLE SET, and that is what
	// guarantees a table is left over. The id is the only thing on the row a
	// reader can act on and the title is the only thing that says what it is;
	// neither may vanish because today's rows happen to agree.
	header, rows, constants := briefCollapse(
		[]string{"ID", "STATE", "AGE", "TITLE", "LATEST NOTE"}, rows,
		map[int]bool{1: true, 2: true, 4: true})

	var sb strings.Builder
	// The constants go ABOVE the table. A reader who has already scanned the
	// rows has spent the scan; the sentence has to arrive before the eye
	// reaches the columns it is explaining.
	//
	// ⛔ EACH `COLUMN "value"` PAIR IS ONE WRAPPING UNIT. Broken across a
	// line, `STATE "not` and `stepped"` is neither greppable nor readable,
	// and it was the first thing the test caught.
	if len(constants) > 0 {
		units := strings.Fields(fmt.Sprintf("The same on all %d rows, so "+
			"stated here once rather than %d times:", len(rows), len(rows)))
		for i, c := range constants {
			if i == len(constants)-1 {
				units = append(units, c+".")
				continue
			}
			units = append(units, c+",")
		}
		sb.WriteString(briefWrapUnits(units, "", "", st.Width))
	}
	briefTable(&sb, st, header, rows)
	return sb.String()
}

// briefMinCell is the narrowest a column may be squeezed to when the terminal
// cannot hold the natural layout. Below this a cell is an ellipsis and a
// letter, which carries less than the space it costs.
const briefMinCell = 8

// briefTable lays out a header and its rows in aligned columns AND FITS THE
// RESULT TO THE TERMINAL.
//
// ⛔ IT IS NOT `writeTable` AND THE DUPLICATION IS DELIBERATE, not an
// oversight. `writeTable` in peers.go also serves `rig peers` and three
// `rig record` tables; teaching it about width would change four surfaces at
// once and only one of them has been measured. The two converge the day
// somebody measures the others, and until then the brief carries its own.
//
// text/tabwriter stays rejected for the reason writeTable already gives: it
// pads the final cell, and every row here ends in free text.
//
// ⛔ WIDTH IS COUNTED IN RUNES, WHICH IS NOT THE SAME AS COLUMNS. A
// double-width glyph occupies two cells and is counted here as one, so a row
// carrying one can overrun by a column. Measuring display width properly
// needs `github.com/mattn/go-runewidth` as a new direct dependency and a
// section 22 row; rune counting is strictly better than the byte counting it
// replaces and the residual error is bounded by the number of wide glyphs in
// a title.
func briefTable(sb *strings.Builder, st briefStyle, header []string, rows [][]string) {
	if len(header) == 0 {
		return
	}
	width := make([]int, len(header))
	for i, h := range header {
		width[i] = utf8.RuneCountInString(h)
	}
	for _, r := range rows {
		for i, cell := range r {
			if n := utf8.RuneCountInString(cell); n > width[i] {
				width[i] = n
			}
		}
	}
	briefFit(width, st.Width)

	line := func(cells []string) string {
		var out strings.Builder
		for i, cell := range cells {
			cell = briefElide(cell, width[i])
			// THE LAST COLUMN IS NEVER PADDED, which is what keeps a title
			// from dragging a run of trailing spaces across the terminal.
			if i == len(cells)-1 {
				out.WriteString(cell)
				break
			}
			out.WriteString(cell)
			if pad := width[i] - utf8.RuneCountInString(cell); pad > 0 {
				out.WriteString(strings.Repeat(" ", pad))
			}
			out.WriteString("  ")
		}
		return out.String()
	}

	sb.WriteString(st.strong(line(header)) + "\n")
	for _, r := range rows {
		sb.WriteString(line(r) + "\n")
	}
}

// briefFit shrinks the widest column until the row fits the budget, IN PLACE.
//
// ⛔ THE WIDEST COLUMN PAYS FIRST, which is the whole of the rule. The
// measured table was ID 6, STATE 13, AGE 5, TITLE 223, LATEST NOTE 11: one
// column held 87% of the row and the other four held nothing worth cutting.
// Taking a share from every column would have cut the id to make room for a
// title.
//
// A budget of zero or less is a pipe and nothing is touched. Nothing is ever
// squeezed below briefMinCell, so a budget narrower than the table's floor
// leaves the row overrunning rather than rendering a line of ellipses - an
// overrun wraps and is still readable, and a row of stubs is not.
//
// One column per pass is O(the deficit), which is at most a few hundred
// iterations on a table nobody can read anyway. It is written this way
// because it is obviously right.
func briefFit(width []int, budget int) {
	if budget <= 0 || len(width) == 0 {
		return
	}
	total := 2 * (len(width) - 1)
	for _, w := range width {
		total += w
	}
	for total > budget {
		widest, at := briefMinCell, -1
		for i, w := range width {
			if w > widest {
				widest, at = w, i
			}
		}
		if at < 0 {
			return
		}
		width[at]--
		total--
	}
}

// briefElide cuts a cell to a column count and SAYS THAT IT DID.
//
// The marker is the whole signal: without it a title cut at the terminal's
// edge cannot be told from a title that ends there, which is a worse defect
// than the wrapping it replaces.
func briefElide(cell string, columns int) string {
	if columns <= 0 || utf8.RuneCountInString(cell) <= columns {
		return cell
	}
	if columns == 1 {
		return "…"
	}
	return string([]rune(cell)[:columns-1]) + "…"
}

// briefCollapse drops every COLLAPSIBLE column holding one value on every
// row, and returns the line that states what it dropped and what the value
// was.
//
// ⛔ NOTHING IS DELETED. The constant is printed once instead of once per
// row, so a reader gains room and loses no fact - and the column returns by
// itself the moment two rows disagree, because the test is the data rather
// than a judgement about which columns are dead.
//
// ONE ROW IS NOT A PATTERN. Every column of a one-row table is trivially
// constant and collapsing them all would trade a table for a sentence.
func briefCollapse(header []string, rows [][]string, collapsible map[int]bool) (
	[]string, [][]string, []string,
) {
	if len(rows) < 2 {
		return header, rows, nil
	}
	drop := make([]bool, len(header))
	stated := make([]string, 0, len(header))
	for i := range header {
		if !collapsible[i] {
			continue
		}
		same := true
		for _, r := range rows[1:] {
			if r[i] != rows[0][i] {
				same = false
				break
			}
		}
		if !same {
			continue
		}
		drop[i] = true
		// Quoted, because several of these constants ARE punctuation - `-`
		// on its own in a sentence reads as a dash rather than as a value.
		stated = append(stated, header[i]+" "+strconv.Quote(rows[0][i]))
	}
	if len(stated) == 0 {
		return header, rows, nil
	}

	keep := func(cells []string) []string {
		out := make([]string, 0, len(cells))
		for i, c := range cells {
			if !drop[i] {
				out = append(out, c)
			}
		}
		return out
	}
	trimmed := make([][]string, 0, len(rows))
	for _, r := range rows {
		trimmed = append(trimmed, keep(r))
	}
	return keep(header), trimmed, stated
}

// briefDefaultWrap is where generated prose wraps when there is no terminal
// to wrap it to. It matches the hand-wrapped sentences elsewhere in this
// file, so a piped brief does not have one paragraph running three times the
// width of its neighbours.
const briefDefaultWrap = 76

// briefWrap breaks generated prose on word boundaries.
//
// ⛔ ONLY GENERATED SENTENCES GO THROUGH THIS. Every other paragraph in this
// file is hand-wrapped in the source, where the break points were chosen;
// re-wrapping those would move a break to a worse place at every width.
func briefWrap(text string, columns int) string {
	return briefWrapUnits(strings.Fields(text), "", "", columns)
}

// briefWrapUnits is briefWrap where the CALLER decides three things the
// default cannot know.
//
//   - WHAT MAY NOT BE SPLIT. A column name and its value are one unit,
//     because `STATE "not` on one line and `stepped"` on the next is worse
//     than the repetition it replaced.
//   - WHAT THE LINE OPENS WITH. `  section 6: ` is part of the first line's
//     budget, and a wrapper that does not know about it overruns by exactly
//     the length of the label.
//   - ⛔ WHAT A CONTINUATION LOOKS LIKE. This is the half that is about
//     reading rather than arithmetic: 55 of the lines the old renderer
//     painted at 80 columns began mid-word in column 0, the same column the
//     `Bnn` anchor lives in, so the one landmark on the page competed with
//     wrapped prose for the left margin. An indent says "this is the same
//     thought" without costing a glyph of ink.
func briefWrapUnits(units []string, lead, indent string, columns int) string {
	if columns <= 0 {
		columns = briefDefaultWrap
	}
	var out strings.Builder
	out.WriteString(lead)
	line := utf8.RuneCountInString(lead)
	for i, word := range units {
		n := utf8.RuneCountInString(word)
		switch {
		case i == 0:
			out.WriteString(word)
			line += n
		case line+1+n > columns:
			out.WriteString("\n" + indent + word)
			line = utf8.RuneCountInString(indent) + n
		default:
			out.WriteString(" " + word)
			line += 1 + n
		}
	}
	out.WriteString("\n")
	return out.String()
}

// briefFeaturesSection is what this project has. Section 39 row 10.
//
// THE STAGE COUNTS RIDE BESIDE THE LIST RATHER THAN REPLACING IT. A count on
// its own cannot be acted on - "three at `shipped`" does not say which three -
// and a list on its own makes a reader tally the stages by eye. Section 39
// carries both fields, so both are printed.
func briefFeaturesSection(features []BriefFeature, stages []BriefStageCount, st briefStyle) string {
	var sb strings.Builder
	sb.WriteString("\n" + st.strong("FEATURES") + "\n")
	if len(features) == 0 && len(stages) == 0 {
		sb.WriteString("This one has no features recorded.\n")
		return sb.String()
	}

	if len(features) > 0 {
		rows := make([][]string, 0, len(features))
		for _, f := range features {
			rows = append(rows, []string{
				f.ID,
				// ⛔ A FEATURE WITH NO STAGE IS A DEFECT AND SAYS SO. It cannot
				// be placed against the counts below, and a blank cell reads
				// as a stage called nothing.
				briefCell(f.Stage, "(no stage - which is a defect, not a stage)"),
				briefCell(f.Title, "(no title)"),
			})
		}
		briefTable(&sb, st, []string{"ID", "STAGE", "TITLE"}, rows)
	}

	// ⛔ THE COUNTS PRINT EVEN WHEN THE LIST IS EMPTY, AND THE REVERSE. Either
	// one arriving alone is a fact about the derivation, and collapsing them
	// into one condition would hide whichever half is missing.
	if len(stages) > 0 {
		sb.WriteString("\n")
		for _, c := range stages {
			fmt.Fprintf(&sb, "  %-12s %d\n",
				briefCell(c.Stage, "(no stage)"), c.Count)
		}
	}
	return sb.String()
}

// briefNotesSection is what has been attached for an agent to read.
func briefNotesSection(notes []BriefNote, now time.Time, st briefStyle) string {
	var sb strings.Builder
	sb.WriteString("\n" + st.strong("NOTES") + "\n")
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
			displaySeat(n.Prov.Seat),
			briefCell(n.Priority, "-"),
			peersAgeCell(provUnix(n.Prov.CreatedAt), now),
			firstLine(strings.TrimSpace(n.Body)),
		})
	}
	briefTable(&sb, st, []string{"WHO", "PRIORITY", "AGE", "NOTE"}, rows)
	fmt.Fprintf(&sb, "\n%d note%s.\n", len(notes), plural(len(notes)))
	return sb.String()
}

// briefBlockedSection reports the cycles and REFUSES TO RESOLVE THEM.
//
// It prints nothing at all when there are none, and that is the one section
// here that may be silent: a blocked condition is an exception, and a line
// saying "no cycles" on every brief would train a reader to skip the place the
// real one appears.
func briefBlockedSection(cycles []BriefCycle, st briefStyle) string {
	if len(cycles) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n" + st.strong("BLOCKED CONDITION") +
		": the `blocks` graph has a cycle\n")
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
func briefBlockageSection(blocked []BriefBlockage, st briefStyle) string {
	var sb strings.Builder
	sb.WriteString("\n" + st.strong("BLOCKED") + "\n")

	// ⛔ IT PRINTS WHEN THERE IS NOTHING, AND THAT IS A CORRECTION. This
	// function used to return "" on an empty list, borrowing the cycle
	// section's argument about not training a reader to skip. THE TWO ARE NOT
	// ALIKE: a cycle is an exception condition and is not one of section 39's
	// eleven sections at all, while BLOCKED is row 4 and the daemon reports it
	// COMPUTED. A computed section that renders nothing is indistinguishable
	// from one this build cannot render, which is the whole reason
	// SectionState exists.
	if len(blocked) == 0 {
		sb.WriteString("Nothing is blocked.\n")
		return sb.String()
	}
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

// briefRenderedSections is every section of section 39's eleven that THIS
// BUILD has a renderer for.
//
// ⛔ IT IS WHAT TURNS "the comment says which fields we dropped" INTO A CHECK.
// A comment naming the unread fields rots the first time somebody adds one;
// this list is compared against what the daemon actually SAID on every call,
// so a section that starts being computed while no renderer exists for it is
// reported to the reader rather than silently dropped.
//
// The numbers are section 39's own row numbers, which the wire's BriefSection
// enum also uses - "citations rather than an ordinal, and they are never
// renumbered".
//
//	1  open          2  next-up      3  notes       4  blocked
//	10 features
//
// The six absent from it are absent because this client has no FIELD for them:
// 5 drift, 6 must-read, 7 projection-behind, 8 pending, 9 local-only,
// 11 case-notes. Every one is NOT_COMPUTED by today's daemon, so none of them
// currently reaches the second list below - and that is precisely the state in
// which a gap goes unnoticed, which is why the list exists before the gap does.
var briefRenderedSections = map[int]bool{1: true, 2: true, 3: true, 4: true, 10: true}

// briefUnavailableSection names every section of the brief the reader is not
// seeing, AND WHICH OF THE TWO REASONS APPLIES.
//
// ⛔ IT PRINTS WHAT IS MISSING, WHICH IS THE WHOLE POINT AND IS EASY TO READ AS
// NOISE. Boris ruled all eleven of section 39's sections into the MVP on
// 2026-09-16 - "Cover all of them" - after four shipped and seven were absent.
// The mechanism that makes that true is not a longer brief, it is a brief that
// says which of its own sections mean anything: an empty notes list is "no
// notes" or "the derivation does not collect them yet", and nothing else here
// can tell a reader which.
//
// ⛔ THE SECOND LIST IS THE ONE THIS FUNCTION WAS MISSING, AND IT IS A
// DIFFERENT FAULT WITH THE SAME SYMPTOM. A section the DAEMON could not
// compute is a capability gap in rig; a section the daemon DID compute and
// this build cannot render is a gap in the CLIENT. Both leave the reader
// without the section and they are repaired in different files, so a brief
// that blurs them sends whoever reads it to the wrong half of the system.
//
// IT GOES LAST, DELIBERATELY. The answer a reader came for is the work; this is
// the confidence interval on it. Printing it first would make every brief open
// with an apology.
func briefUnavailableSection(sections []BriefSectionState, st briefStyle) string {
	var notComputed, notRendered []BriefSectionState
	for _, s := range sections {
		switch {
		case s.Withheld:
			// ⛔ NOT LISTED AT ALL, AND THAT IS THE POINT OF THE FIELD. The
			// derivation RAN and this caller is simply not being shown it -
			// section 39's view table drops the must-read set from the human
			// view because it is not a decision he makes. Reporting it here
			// would make the human view read as a degraded agent view.
		case !s.Computed:
			notComputed = append(notComputed, s)
		case !briefRenderedSections[s.Section]:
			notRendered = append(notRendered, s)
		}
	}
	if len(notComputed) == 0 && len(notRendered) == 0 {
		return ""
	}

	var sb strings.Builder
	if len(notComputed) > 0 {
		sb.WriteString("\n" + st.strong("NOT ANSWERED BY THIS BRIEF") +
			" - these sections are specified and not yet built,\nso their " +
			"absence above is NOT a statement that there is nothing to " +
			"report:\n")
		for _, s := range notComputed {
			// ⛔ THE REASON IS THE DAEMON'S PROSE AND IT IS UNBOUNDED. The
			// longest one on the live estate is 302 characters, which painted
			// four screen lines at 80 columns with the last three starting
			// mid-word in column 0.
			sb.WriteString(briefWrapUnits(
				strings.Fields(briefCell(s.Reason,
					"no reason was given, which is itself a defect")),
				"  section "+strconv.Itoa(s.Section)+": ", "    ", st.Width))
		}
	}
	if len(notRendered) > 0 {
		// ⛔ THE SENTENCE NAMES WHICH HALF IS AT FAULT, because the repair is
		// in a different file from the one above and a reader sent to rigd
		// for a client-side gap finds nothing wrong there.
		sb.WriteString("\n⛔ " + st.strong("COMPUTED BY rigd AND NOT SHOWN BY "+
			"THIS BUILD OF rig") + " - the answer EXISTS\nand this client " +
			"has no renderer for it. " +
			"That is a gap in rig's CLI, not in\nthe derivation, and upgrading " +
			"rig is what fixes it:\n")
		for _, s := range notRendered {
			sb.WriteString("  section " + strconv.Itoa(s.Section) + "\n")
		}
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
// THAT IS NOT BUILT.
//
// ⛔ THIS COMMENT USED TO LIST THEM AND IT HAD ALREADY ROTTED, IN THE SENTENCE
// BELOW THAT PREDICTS EXACTLY THAT. It named five - `drift`, `health`,
// `case_notes`, `coarse_citations`, `must_read` - while the wire carried SIX.
// `must_read_cleared` was the one nobody noticed, and nobody could, because
// prose is checked against nothing. THE LIST LIVES IN
// `briefWireFieldsNotRendered` in the test file now, where every entry needs a
// written reason, an unlisted new field is RED, and a reason that outlived its
// field is red too.
//
// ⛔ AND NAMING THEM ANYWHERE IS NOT THE WHOLE MECHANISM. briefRenderedSections
// below is the runtime half: any section the daemon reports COMPUTED that this
// build cannot render is listed to the reader as a CLIENT-SIDE gap, in its own
// sentence, beside the sections the daemon could not compute - checked against
// what the daemon actually said on every call rather than against a list.
func briefFromWire(r *rigv1.ProjectBriefResponse) Brief {
	b := Brief{
		Project: r.GetProject(),
		Kind:    r.GetKind(),
		Title:   r.GetTitle(),
		Status:  r.GetStatus(),
		Semver:  r.GetSemver(),
	}
	for _, it := range r.GetOpen() {
		b.Open = append(b.Open, itemFromWire(it))
	}
	for _, it := range r.GetNextUp() {
		b.NextUp = append(b.NextUp, itemFromWire(it))
	}
	for _, f := range r.GetFeatures() {
		b.Features = append(b.Features, BriefFeature{
			ID: f.GetId(), Title: f.GetTitle(), Stage: f.GetStage(),
		})
	}
	for _, c := range r.GetFeatureStages() {
		b.FeatureStages = append(b.FeatureStages, BriefStageCount{
			Stage: c.GetStage(), Count: c.GetCount(),
		})
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
