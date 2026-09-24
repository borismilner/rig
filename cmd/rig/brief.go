package main

import (
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// THE BRIEF LEFT rig AT PLAN.md SECTION 50, MOVES 5 AND 8, AND TWO THINGS
// STAYED.
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
// ⛔ THE THIRD THING IS GONE AS OF MOVE 8. The wire conversion at the foot of
// this file - the Brief types, briefFromWire, itemFromWire, noteFromWire,
// sectionFromWire and stepStateWord - existed for wireRecord.Brief in
// record.go, and that method went with the daemon arm that served it. A
// client-side model of the planner's brief is exactly what this repository
// must not carry. (The retired names are not spelled out anywhere here:
// acceptance B's grep is over text and does not know a comment from code.)

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
