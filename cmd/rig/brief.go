package main

import (
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// THE BRIEF LEFT rig AT PLAN.md SECTION 50, MOVE 5, AND TWO THINGS STAYED.
//
// What is left in this file is those two, and nothing about deriving a brief:
//
//   - the refusal arm below, which is all `rig brief` is now (decision 4);
//   - the COLUMN MACHINERY the record verbs render their own tables with -
//     briefStyle, briefStyleFor, briefTableAround, briefFitAround, briefElide,
//     briefWrap and briefCell. They were written here and cmd/rig/record.go
//     calls every one of them, so they are shared rendering rather than the
//     planner's, and the names keep their spelling because their callers do.
//
// The wire conversion at the foot of the file is the third thing, and it is
// the only part still waiting: it exists for wireRecord.Brief in record.go and
// goes when that method does.

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

	// DescriptionShort and DescriptionLong are the container's own
	// description. Section 39's field table gives both names to `project`,
	// and its case table repeats both for `case`, so neither is new
	// vocabulary and neither is a work-item field borrowed upward.
	//
	// ⛔ THIS IS WHAT BORIS ASKED THE OPENING OF A PROJECT TO CARRY.
	// 2026-09-17: "Each project should start with the name of the project,
	// the overall state something similar to what there is now, some
	// description of the project to remind what it is about." The name is
	// Title, the state is the counts, and the REMINDER is DescriptionLong.
	//
	// ⛔ DescriptionLong BELONGS ON THIS HEADING AND NOWHERE IN A LIST. The
	// compact item card is explicitly never description_long - a list meant
	// to be scanned in one pass fails its own readability requirement the
	// moment it carries a paragraph per row. A heading is read once, at the
	// top, and is the one place the prose is not a cost.
	DescriptionShort string
	DescriptionLong  string

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

	// Governing and GoverningCounts are SECTION 12, B64: the decisions, the
	// requirements and the artefacts recorded against this project.
	//
	// ⛔ THE ROW'S OWN KIND IS RENDERED AND NOT INFERRED FROM ITS POSITION. The
	// rows arrive grouped, so a renderer could print a heading per run and drop
	// the field - and would be right until the derivation's order changed, at
	// which point every row would carry the wrong label and no test would see
	// it. The wire puts the kind on the row; this prints what it was given.
	Governing       []BriefGoverning
	GoverningCounts []BriefKindCount

	// Closed, ClosedCounts and Unlisted are SECTION 13, B68, ruled by Boris
	// 2026-09-17: the work items the open lists drop because something closed
	// them, each carrying the word that closed it.
	//
	// ⛔ THE OPEN LISTS DO NOT CHANGE AND A RENDERER MAY NOT MERGE THESE BACK
	// INTO THEM. Marking the closed rows inside the open list is the shape he
	// was shown and rejected, on the grounds that the open list is already the
	// part of the brief he has called hard to read.
	//
	// ⛔ Unlisted IS NOT A REMAINDER TO BE IGNORED WHEN IT IS INCONVENIENT. It
	// is how many work items are in NEITHER list - an `idea` nobody picked up,
	// or a record whose status nobody wrote - and without it a reader adds the
	// two lists up and is wrong with no way to find out.
	Closed       []BriefClosedItem
	ClosedCounts []BriefWordCount
	Unlisted     uint64

	// Sections is the state of all ELEVEN of section 39's sections.
	//
	// ⛔ RENDERING A SECTION WITHOUT ITS STATE IS THE DEFECT BORIS RULED OUT,
	// ONE LAYER DOWN. An empty notes list means "no notes" or "the derivation
	// does not collect them yet", and a reader of this brief has no other way
	// to tell. Whatever this client does with the rest, it must not present a
	// section as answered when the daemon said it was not.
	Sections []BriefSectionState

	// ContainerFound is B76 AS THE DAEMON STATED IT, and its three values are
	// three different answers rather than a boolean with a spare.
	//
	// ⛔ UNSPECIFIED IS "THIS DAEMON DOES NOT CARRY THE FIELD" AND IS THE ONLY
	// REASON THE INFERENCE BELOW STILL EXISTS. Read ContainerMissing, never
	// this field: the rule for reading it is written once, down there.
	ContainerFound rigv1.Tristate
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

// BriefGoverning is one row of section 12: a decision, a requirement or an
// artefact recorded against this project.
type BriefGoverning struct {
	ID    string
	Title string

	// Kind is WHICH of the three. EMPTY IS A DEFECT rather than a kind: it is
	// the only thing separating one section that carries three kinds from a
	// fold that loses which is which, and the renderer says so rather than
	// leaving the cell blank.
	Kind string
}

// BriefKindCount is how many governing records of one kind exist.
type BriefKindCount struct {
	Kind  string
	Count uint64
}

// BriefClosedItem is one work item that is NOT open, and THE WORD THAT CLOSED
// IT. Section 13, B68.
type BriefClosedItem struct {
	ID    string
	Title string

	// Word is what closed it: a status somebody wrote, or `done` from the
	// progress stream when the status still reads `active`.
	//
	// ⛔ EMPTY IS A DEFECT rather than a word, for BriefGoverning.Kind's
	// reason. The derivation puts an item with no closing word in Unlisted, so
	// a blank cell here means the wire dropped the field - and a blank cell
	// reads as a closure called nothing.
	Word string
}

// BriefWordCount is how many closed items carry one closing word.
type BriefWordCount struct {
	Word  string
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
	ID    string
	Title string

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

// ---- the verb that left ----------------------------------------------------

// codeMoved: rig still dispatches this word and no longer carries what it used
// to do.
//
// It is not codeNoSuchCommand, which is about a PROGRAM's declaration not
// naming a command, and not codeBadArgument, which is about argv: the command
// line was fine and the capability is somewhere else. An agent that retries
// on a bad argument and reports on a moved capability needs the two apart.
const codeMoved = codeLocal + "MOVED"

// cmdBrief is what is left of `rig brief` after the brief moved out of rig
// (PLAN.md section 50, move 5 and decision 4): a refusal that names where it
// went.
//
// THE ARM STAYS IN run's SWITCH AND THAT IS THE POINT. Anything the switch
// does not match falls through to the default branch, which reads the first
// token as a PROGRAM name - so deleting the case would answer `rig brief`
// with "no such program", naming neither the move nor what replaced it.
//
// THE REPLACEMENT IS NAMED IN PROSE AND NOT IN A MARKDOWN CITATION, AND NOT IN
// FixCommand. refusal_verbs_test.go reads the first word after a backticked
// "rig " and checks it against run's switch; the replacement's first word is a
// PROGRAM, which the default branch routes and the switch can never name. A
// backticked citation would therefore read as a dead verb. The guard is left
// exactly as it is rather than taught to accept a shape it has no source of
// truth for - see the seat's FINDINGS for the measurement.
func cmdBrief([]string) error {
	return local(jsonStatus{
		Code: codeMoved,
		Message: "the brief is not rig's to derive: it moved to the docket " +
			"program, which reaches this store over the socket like every " +
			"other program here",
		Precondition: "rig carries the command being asked for",
		Actual: "rig carries the record store and the record verbs; the " +
			"brief is derived on top of them by a program",
		Fix: "ask the program that owns it, through rig: " +
			"rig docket brief <project>",
	})
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

// briefMinCell is the narrowest a column may be squeezed to when the terminal
// cannot hold the natural layout. Below this a cell is an ellipsis and a
// letter, which carries less than the space it costs.
const briefMinCell = 8

// briefTableAround lays out a header and its rows in aligned columns, FITS THE
// RESULT TO THE TERMINAL, and leaves the pinned columns at their full width.
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
func briefTableAround(sb *strings.Builder, st briefStyle, header []string,
	rows [][]string, pinned map[int]bool,
) {
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
	briefFitAround(width, st.Width, pinned)

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

// briefFitAround shrinks the widest column until the row fits the budget, IN
// PLACE, and never touches a column the caller pinned.
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
//
// ⛔ A KEY COLUMN IS NOT A WIDE COLUMN, AND TREATING IT AS ONE IS WHAT THE PIN
// EXISTS TO STOP. Unpinned, it takes the width out of the widest column, which is
// right for prose and wrong for an identifier: measured 2026-09-17 at 80
// columns against the live production store, FIVE of the six GOVERNING rows
// rendered the identical stub `yes-one-mcp-session-is-one-wire-c…`. An elided
// title still identifies the row and can still be read; an elided id
// identifies nothing, cannot be told from its neighbours, and cannot be pasted
// into `rig record get` - which is the single thing section 12 exists to make
// possible.
//
// A pin can make the row unfittable, and that is allowed for the reason the
// floor above already gives: an overrun wraps and stays readable, and every
// alternative here destroys the key.
func briefFitAround(width []int, budget int, pinned map[int]bool) {
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
			if !pinned[i] && w > widest {
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

		DescriptionShort: r.GetDescriptionShort(),
		DescriptionLong:  r.GetDescriptionLong(),

		// ⛔ CARRIED THROUGH UNTRANSLATED, INCLUDING THE ZERO. A daemon that
		// does not set field 22 is a fact about that daemon, and folding it
		// into a bool here would throw it away at the only point where it can
		// still be seen. ContainerMissing is the one place the three values
		// are turned into an answer.
		ContainerFound: r.GetContainerFound(),
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
	for _, g := range r.GetGoverning() {
		b.Governing = append(b.Governing, BriefGoverning{
			ID: g.GetId(), Kind: g.GetKind(), Title: g.GetTitle(),
		})
	}
	for _, c := range r.GetGoverningCounts() {
		b.GoverningCounts = append(b.GoverningCounts, BriefKindCount{
			Kind: c.GetKind(), Count: c.GetCount(),
		})
	}
	for _, c := range r.GetClosed() {
		b.Closed = append(b.Closed, BriefClosedItem{
			ID: c.GetId(), Title: c.GetTitle(), Word: c.GetClosingWord(),
		})
	}
	for _, c := range r.GetClosedCounts() {
		b.ClosedCounts = append(b.ClosedCounts, BriefWordCount{
			Word: c.GetWord(), Count: c.GetCount(),
		})
	}
	// ⛔ READ EVEN THOUGH ITS ZERO IS INDISTINGUISHABLE FROM AN UNSERVED FIELD.
	// An older daemon is told apart by the ABSENCE of the closed section from
	// `sections`, which briefUnavailableSection already reports; dropping the
	// count here would instead make a brief that HAS the section silently
	// under-report by every statusless item.
	b.Unlisted = r.GetUnlistedItems()
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
		Title:    n.GetTitle(),
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
