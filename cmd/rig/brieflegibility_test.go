package main

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// The brief is the surface Boris actually reads, and these are the three
// defects a measured pass over it found on 2026-09-17. Every test here names
// the number it exists to hold.
//
// The write-up they come from is
// logbook/projects/rig/attacks/run-2026-09-17/S6-FINDINGS.md, findings S6-1,
// S6-2 and S6-3.

// ---- S6-1: the caption said an order the wire cannot carry -----------------

// ⛔ `NEXT UP` PRINTED "in expected execution order" OVER A BYTE SORT OF THE
// ID STRING. Measured against the live estate: the printed head was
// `B1 B10 B12 B13 B14`, which is exactly the first five of the sorted ids,
// and 0 of 68 work items carried a priority. B8 printed 59th of 59, B10
// printed 2nd.
//
// THE SORT IS NOT THE DEFECT AND MUST NOT BE "FIXED". `sortByPriority` in the
// derivation is priority-then-id and is correct; the store simply holds no
// priorities, so it degenerates to a string sort. Inventing an order would
// put a ranking in front of a reader that nobody decided. The caption is what
// lied, so the caption is what changes.
func TestTheNextUpCaptionDoesNotClaimAnOrderTheWireCannotCarry(t *testing.T) {
	got := briefNextUpSection(brief().NextUp, now, briefStyle{})

	if strings.Contains(got, "expected execution order") {
		t.Errorf("the caption still claims an expected execution order. "+
			"Nothing on the wire ranks these items - `ItemState` carries id, "+
			"title, state, since and note, and no priority - so this client "+
			"cannot know of any order beyond the one it was handed:\n%s", got)
	}
	// The positive control. Without it this test passes against a renderer
	// that prints no caption at all, which would be a different defect with
	// the same green.
	if !strings.Contains(got, "2 items") {
		t.Errorf("the caption line is gone entirely, so the assertion above "+
			"proved nothing:\n%s", got)
	}
}

// AND IT SAYS WHAT THE ORDER ACTUALLY IS, WHEN IT CAN SEE THAT FOR ITSELF.
//
// The client holds the ids it was handed and can compare their order against
// those same ids sorted. That is a fact it owns rather than an inference, and
// it is the whole of the honest caption: the day priorities exist the order
// stops matching the sort and the sentence retires itself with no code change.
func TestAnIdSortedNextUpSaysSoAndOneThatIsNotDoesNot(t *testing.T) {
	sorted := []BriefItem{
		{ID: "B1", Title: "first"},
		{ID: "B10", Title: "second"},
		{ID: "B2", Title: "third"},
	}
	got := briefNextUpSection(sorted, now, briefStyle{})
	if !strings.Contains(got, "ids sorted") {
		t.Errorf("the ids arrived in exactly sorted order and the caption "+
			"does not say so, so a reader still has no way to tell this list "+
			"is unranked:\n%s", got)
	}

	// ⛔ THE HALF THAT MAKES THE HALF ABOVE MEAN ANYTHING. A renderer that
	// prints "ids sorted" unconditionally passes the first assertion and is
	// as wrong as the caption being replaced.
	ranked := []BriefItem{
		{ID: "B2", Title: "first"},
		{ID: "B1", Title: "second"},
		{ID: "B10", Title: "third"},
	}
	got = briefNextUpSection(ranked, now, briefStyle{})
	if strings.Contains(got, "ids sorted") {
		t.Errorf("the ids arrived in an order that is NOT the sort and the "+
			"caption claimed it was, which is the original defect with a new "+
			"sentence:\n%s", got)
	}
	if !strings.Contains(got, "3 items") {
		t.Errorf("the second case printed no caption at all, so the absence "+
			"asserted above proves nothing:\n%s", got)
	}

	// ONE ITEM IS SORTED AND THE OBSERVATION IS WORTHLESS. A list of one is
	// trivially in id order, so saying so would be a sentence that tells a
	// reader nothing about a list they can see all of.
	got = briefNextUpSection([]BriefItem{{ID: "B1", Title: "alone"}}, now,
		briefStyle{})
	if strings.Contains(got, "ids sorted") {
		t.Errorf("a one-item list was told its order is the id sort:\n%s", got)
	}
	if !strings.Contains(got, "1 item,") {
		t.Errorf("the one-item caption is gone, so the absence above proves "+
			"nothing:\n%s", got)
	}
}

// ⛔ THE CAPTION WAS AT THREE SITES AND THE WORST ONE WAS A DOC COMMENT, which
// no rendering test can reach: `Brief.NextUp`'s own documentation said the
// items arrive "in expected execution order", so the next person to read the
// type learns the wrong thing and writes the printed line back.
//
// This reads the source rather than the output, which is the only instrument
// that covers all three.
func TestNothingInThisFileStillDocumentsAnExpectedExecutionOrder(t *testing.T) {
	src, err := os.ReadFile("brief.go")
	if err != nil {
		t.Fatalf("brief.go could not be read, so this test proved NOTHING "+
			"and must not be read as a pass: %v", err)
	}
	// The positive control, for the same reason the layering test carries
	// one: an absence is also what an empty or misdirected read produces.
	if !strings.Contains(string(src), "func briefText(") {
		t.Fatalf("the file read back does not contain briefText, so it is " +
			"not the renderer and the absence below means nothing")
	}
	if n := strings.Count(string(src), "expected execution order"); n != 0 {
		t.Errorf("%d site(s) in brief.go still document an expected "+
			"execution order. The wire carries no priority, so no part of "+
			"this client may promise one - not the printed line, not a "+
			"function's doc, and least of all the doc on the field itself", n)
	}
}

// ---- S6-2: three columns, one value each, 54 rows --------------------------

// ⛔ MEASURED: STATE, AGE AND LATEST NOTE HELD ONE DISTINCT VALUE EACH ACROSS
// 54 ROWS. 29 characters per row times 54 rows is 1,566 characters of screen
// spent repeating three constants.
//
// THE CONSTANT IS NOT DELETED, IT IS STATED ONCE. A column that is constant
// today may vary tomorrow, so the collapse is decided from the DATA on every
// render rather than from a list of columns somebody judged dead. The write
// path that flattens every disposition to `active` is being repaired in
// parallel; when it lands, STATE starts varying and the column comes back
// with no change here.
func TestAColumnConstantOnEveryRowIsStatedOnceInsteadOfPerRow(t *testing.T) {
	items := []BriefItem{
		{ID: "B1", Title: "the first"},
		{ID: "B2", Title: "the second"},
		{ID: "B3", Title: "the third"},
	}
	got := briefItemTable(items, now, briefStyle{})

	// The header line is the one that decides how wide every row is, so it
	// is the line the assertion has to be about. Testing the whole rendering
	// for the word STATE is the mistake this comment exists to stop: the
	// collapsed constant NAMES its column, so "STATE" is still on the screen
	// and must be.
	header := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "ID") {
			header = line
			break
		}
	}
	if header == "" {
		t.Fatalf("no header line was found at all, so every assertion below "+
			"would be about the wrong thing:\n%s", got)
	}
	for _, col := range []string{"STATE", "AGE", "LATEST NOTE"} {
		if strings.Contains(header, col) {
			t.Errorf("%s holds one value on all three rows and still has a "+
				"column of its own: %q", col, header)
		}
	}
	if n := strings.Count(got, "not stepped"); n != 1 {
		t.Errorf("the constant `not stepped` is printed %d times. It is one "+
			"fact about the whole table and belongs on one line:\n%s", n, got)
	}
	// ⛔ NOTHING IS DELETED. A collapse that drops the value is not a
	// legibility fix, it is information loss with a smaller footprint.
	for _, kept := range []string{"STATE", `"not stepped"`, "AGE", "LATEST NOTE"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%s left the output entirely, so the reader lost a fact "+
				"rather than gaining room:\n%s", kept, got)
		}
	}
	if !strings.Contains(got, "B1") || !strings.Contains(got, "the first") {
		t.Errorf("the id or the title went missing, so the collapse reached "+
			"columns it must never touch:\n%s", got)
	}
	// ⛔ A COLUMN NAME AND ITS VALUE ARE ONE UNIT AND THE WRAP MAY NOT SPLIT
	// THEM. `STATE "not` on one line and `stepped"` on the next is neither
	// greppable nor readable, and it is what the first attempt at this did.
	for _, line := range strings.Split(got, "\n") {
		if strings.Count(line, `"`)%2 != 0 {
			t.Errorf("a quoted constant was wrapped across lines: %q", line)
		}
	}
}

// AND A COLUMN THAT VARIES KEEPS ITS COLUMN.
//
// This one is a CONTROL rather than a new guarantee: it passed before the
// collapse existed and its job is to go red the day the collapse over-applies
// and starts eating a column that carries a signal.
func TestAColumnThatVariesAcrossRowsKeepsItsColumn(t *testing.T) {
	items := []BriefItem{
		{ID: "B1", Title: "the first"},
		{
			ID: "B2", Title: "the second", State: "started",
			Since: now.Add(-90 * time.Second), Note: "the seam landed",
		},
		{ID: "B3", Title: "the third"},
	}
	got := briefItemTable(items, now, briefStyle{})

	if !strings.Contains(got, "STATE") {
		t.Errorf("STATE holds two different values and lost its column:\n%s", got)
	}
	if !strings.Contains(got, "started") || !strings.Contains(got, "not stepped") {
		t.Errorf("one of the two states is not on the screen:\n%s", got)
	}
	if !strings.Contains(got, "the seam landed") {
		t.Errorf("the only note in the table is not rendered:\n%s", got)
	}
}

// ONE ROW IS NOT A PATTERN. Every column of a one-row table is trivially
// constant, and collapsing them all would leave a table with no columns.
//
// A CONTROL, like the test above.
func TestASingleRowKeepsEveryColumnBecauseOneRowIsNotAPattern(t *testing.T) {
	got := briefItemTable([]BriefItem{{ID: "B1", Title: "alone"}}, now, briefStyle{})

	// ⛔ THE HEADER LINE, NOT THE WHOLE RENDERING. A collapsed column NAMES
	// itself in the sentence it collapses into, so "STATE" is present either
	// way and an assertion over the whole string cannot tell the two apart.
	// This test was blind to exactly that until a mutation pass caught it.
	header := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "ID") {
			header = line
			break
		}
	}
	if header == "" {
		t.Fatalf("no header line at all:\n%s", got)
	}
	for _, col := range []string{"STATE", "AGE", "TITLE", "LATEST NOTE"} {
		if !strings.Contains(header, col) {
			t.Errorf("a one-row table lost its %s column, which trades a "+
				"header for nothing: %q", col, header)
		}
	}
	if strings.Contains(got, "The same on all") {
		t.Errorf("a one-row table was told it has a pattern:\n%s", got)
	}
}

// ---- S6-3: half the painted screen carried no information ------------------

// ⛔ MEASURED: THE 80, 120 AND 200 COLUMN CAPTURES WERE BYTE-IDENTICAL. The
// renderer padded TITLE to the widest title in the section - 223 characters -
// and never consulted the terminal at all, so a 248-character row painted
// four screen lines at 80 columns, 55 continuation lines began mid-word in
// the same column as the id anchor, and 39.8% to 52.5% of the painted lines
// carried no ink but a `-`.
func TestTheTableIsElidedToTheTerminalWidth(t *testing.T) {
	const long = "a title long enough to run off any terminal anybody has " +
		"ever sat in front of, and then a good deal further than that, so " +
		"that nothing about this row can fit in eighty columns"
	items := []BriefItem{
		{ID: "B1", Title: long},
		{ID: "B2", Title: long + " and more"},
	}
	got := briefItemTable(items, now, briefStyle{Width: 80})

	for i, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if w := utf8.RuneCountInString(line); w > 80 {
			t.Errorf("line %d is %d columns wide on an 80-column terminal, "+
				"so it wraps and the row stops being one row:\n%q", i, w, line)
		}
	}
	if !strings.Contains(got, "…") {
		t.Errorf("a title was cut to fit and nothing says it was cut, so the "+
			"reader cannot tell a short title from a truncated one:\n%s", got)
	}
}

// AND WITH NO TERMINAL BEHIND IT, NOTHING IS CUT.
//
// A CONTROL. `rig brief | ...` and `rig brief > file` have no width to fit,
// and a renderer that elided anyway would be destroying bytes a pipe was
// going to read in full.
func TestWithNoTerminalNothingIsElided(t *testing.T) {
	const long = "a title long enough to run off any terminal anybody has " +
		"ever sat in front of, and then a good deal further than that"
	got := briefItemTable([]BriefItem{
		{ID: "B1", Title: long},
		{ID: "B2", Title: long + " and more"},
	}, now, briefStyle{})

	if !strings.Contains(got, long) {
		t.Errorf("the title was cut with no terminal to cut it for:\n%s", got)
	}
	if strings.Contains(got, "…") {
		t.Errorf("an elision marker was printed into a pipe:\n%s", got)
	}
}

// THE ID SURVIVES A TERMINAL TOO NARROW FOR ANYTHING. It is the only thing on
// the row a reader can act on, and a renderer that elides it has spent the
// row on nothing.
//
// A CONTROL.
func TestAnAbsurdlyNarrowTerminalStillPrintsTheWholeID(t *testing.T) {
	got := briefItemTable([]BriefItem{
		{ID: "B1", Title: "the first"},
		{ID: "B234", Title: "the second"},
	}, now, briefStyle{Width: 12})

	for _, id := range []string{"B1", "B234"} {
		if !strings.Contains(got, id) {
			t.Errorf("id %s did not survive a 12-column terminal:\n%s", id, got)
		}
	}
}

// ⛔ THE DAEMON'S OWN PROSE IS UNBOUNDED AND IT IS PRINTED, so it is the
// renderer's problem to fit. The longest not-computed reason on the live
// estate is 302 characters: at 80 columns it painted four screen lines and
// three of them began mid-word in column 0, the same column the `Bnn` anchor
// lives in.
//
// THE CONTINUATION IS INDENTED, which is the reading half of the fix. An
// indent says "still the same section" for the price of no ink at all.
func TestTheDaemonsReasonIsWrappedToTheTerminalAndIndented(t *testing.T) {
	long := "the standards register does not exist, and " +
		strings.Repeat("this reason runs on and on and on, ", 8) + "so it does"
	got := briefUnavailableSection([]BriefSectionState{
		{Section: 6, Computed: false, Reason: long},
	}, briefStyle{Width: 80})

	// ⛔ THE LEAD LINE IS THE ONE THAT OVERRUNS, so it is the one that must be
	// measured. An earlier version of this test skipped past it to look at
	// continuations, found none on a 302-character single line, and passed
	// against the exact defect it was written for.
	var block []string
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "  section 6: "):
			block = append(block, line)
		case len(block) > 0 && strings.HasPrefix(line, "    "):
			block = append(block, line)
		case len(block) > 0:
			// The block has ended and everything after it belongs to
			// another section.
		}
	}
	if len(block) == 0 {
		t.Fatalf("the section was not rendered at all, so nothing below was "+
			"checked:\n%s", got)
	}
	if len(block) < 2 {
		t.Errorf("a %d-character reason rendered as ONE line on an "+
			"80-column terminal, so it was not wrapped at all:\n%q",
			utf8.RuneCountInString(long), block[0])
	}
	for i, line := range block {
		if w := utf8.RuneCountInString(line); w > 80 {
			t.Errorf("reason line %d is %d columns wide on an 80-column "+
				"terminal:\n%q", i, w, line)
		}
		if i > 0 && !strings.HasPrefix(line, "    ") {
			t.Errorf("a continuation line starts in column 0, where the "+
				"section labels live:\n%q", line)
		}
	}
	// The control: with no terminal the reason is not re-flowed to 80 either
	// way, but it must still be present in full.
	whole := briefUnavailableSection([]BriefSectionState{
		{Section: 6, Computed: false, Reason: long},
	}, briefStyle{})
	if !strings.Contains(strings.Join(strings.Fields(whole), " "),
		strings.Join(strings.Fields(long), " ")) {
		t.Errorf("the reason lost words on the way through the wrapper:\n%s",
			whole)
	}
}

// ⛔ MEASURED: 0 ANSI ESCAPES ON A REAL PTY, against 23 from
// `ls` with its colour output forced, through the same instrument. Two visual
// levels for 273 screen lines, where the document it replaces has eight or
// more.
//
// IT IS BOLD AND NOTHING ELSE, ON PURPOSE. A colour cannot be checked for
// contrast here: the ratio depends on the terminal's own palette and
// background, which this process cannot read, and an unmeasurable change is
// one this repository does not make. SGR 1 changes the WEIGHT of a glyph and
// leaves the foreground pair the terminal already chose, so the ratio is
// unchanged by construction.
func TestATerminalGetsTheSectionLabelsInBoldAndAPipeGetsNothing(t *testing.T) {
	b := brief()

	plain := briefText(b, now, briefStyle{})
	if strings.Contains(plain, "\x1b") {
		t.Errorf("an escape sequence reached output with no terminal behind "+
			"it:\n%q", plain)
	}

	styled := briefText(b, now, briefStyle{Bold: true})
	if !strings.Contains(styled, "\x1b[1mNEXT UP\x1b[22m") {
		t.Errorf("the section label is not emphasised on a terminal that can "+
			"carry it, so the brief still has two visual levels:\n%q", styled)
	}
	// SGR 22 rather than SGR 0: a full reset would clobber whatever else the
	// terminal was carrying, which is somebody else's state.
	if strings.Contains(styled, "\x1b[0m") {
		t.Errorf("the renderer emits a full reset, which turns off attributes "+
			"it did not turn on:\n%q", styled)
	}
	// ⛔ THE TWO RENDERINGS MUST DIFFER ONLY IN THE ESCAPES. Emphasis that
	// also moves the text is a second change hiding behind a measured one.
	stripped := stripSGR(styled)
	if stripped != plain {
		t.Errorf("styling changed the text itself, not just its attributes\n"+
			"plain:\n%s\nstyled, escapes removed:\n%s", plain, stripped)
	}
}

// stripSGR removes every SGR sequence, so the two renderings can be compared
// as text. Deliberately narrow: it handles the escapes this file emits and
// would fail loudly on anything else by leaving it in place.
func stripSGR(s string) string {
	for _, seq := range []string{"\x1b[1m", "\x1b[22m"} {
		s = strings.ReplaceAll(s, seq, "")
	}
	return s
}

// ---- the probe: what the renderer is allowed to know about the device ------

// ⛔ THE ANSWER IS ONE IOCTL AND NOT A GUESS. `TIOCGWINSZ` fails with ENOTTY
// on a pipe, on /dev/null and on a regular file, and succeeds on a terminal
// carrying its size, so the same call answers both questions this renderer
// has: is anybody watching, and how wide.
//
// THE POSITIVE CONTROL IS A REAL PTY. Without it every assertion below is an
// absence, and an absence is also what a probe that always answers "no"
// produces - which is exactly what this returned before the repair.
func TestTheStyleProbeTellsATerminalFromEverythingElse(t *testing.T) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx on this machine, so the positive control "+
			"cannot run and the negatives below would prove nothing: %v", err)
	}
	defer pty.Close()
	if err := unix.IoctlSetWinsize(int(pty.Fd()), unix.TIOCSWINSZ,
		&unix.Winsize{Row: 50, Col: 97}); err != nil {
		t.Fatalf("the control pty would not take a size, so this test "+
			"proved NOTHING: %v", err)
	}

	if st := briefStyleFor(pty); st.Width != 97 || !st.Bold {
		t.Errorf("a 97-column terminal was read as %+v", st)
	}

	// Every way this process can be run without a terminal.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("no pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	file, err := os.CreateTemp(t.TempDir(), "brief")
	if err != nil {
		t.Fatalf("no temp file: %v", err)
	}
	defer file.Close()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("no %s: %v", os.DevNull, err)
	}
	defer devnull.Close()

	for name, f := range map[string]*os.File{
		"a pipe":        w,
		"a file":        file,
		os.DevNull:      devnull,
		"a closed file": mustClosed(t),
	} {
		if st := briefStyleFor(f); st != (briefStyle{}) {
			t.Errorf("%s was read as a terminal: %+v", name, st)
		}
	}
}

// mustClosed is a file handle that is already gone, which is what a caller
// holding stdout after it was closed under them has. The probe must answer
// "no terminal" rather than panicking on it.
func mustClosed(t *testing.T) *os.File {
	f, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatalf("no temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("could not close it: %v", err)
	}
	return f
}

// THE NO-COLOUR CONVENTION TURNS OFF THE ATTRIBUTE AND LEAVES THE WIDTH
// ALONE.
//
// They are two different facts about the device. Somebody who has set that
// variable has said what their terminal should print, not how wide it is, and
// a probe that collapsed the two would throw away the larger of the two
// repairs to honour the smaller.
//
// this repository spells.
//
//nolint:misspell // the variable's actual name is an identifier, not a word
func TestNoColorSuppressesTheAttributeAndKeepsTheWidth(t *testing.T) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx, so this proves nothing: %v", err)
	}
	defer pty.Close()
	if err := unix.IoctlSetWinsize(int(pty.Fd()), unix.TIOCSWINSZ,
		&unix.Winsize{Row: 50, Col: 97}); err != nil {
		t.Fatalf("the control pty would not take a size: %v", err)
	}

	// The control: unset, this same handle is bold.
	t.Setenv("NO_COLOR", "")
	if st := briefStyleFor(pty); !st.Bold {
		t.Fatalf("with NO_COLOR empty the terminal is not emphasised, so the "+
			"assertion below cannot tell the variable from the default: %+v", st)
	}

	t.Setenv("NO_COLOR", "1")
	st := briefStyleFor(pty)
	if st.Bold {
		t.Errorf("NO_COLOR is set and the renderer still emphasises: %+v", st)
	}
	if st.Width != 97 {
		t.Errorf("NO_COLOR took the terminal's width with it: %+v", st)
	}
}
