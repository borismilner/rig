package main

import (
	"strings"
	"testing"
)

// SECTION 12's IDS ARE PROSE, AND AN ALIGNED COLUMN CANNOT HOLD THEM.
//
// §39 derives a record's id from its heading, so a decision imported from a
// document carries the heading as its key. Measured 2026-09-17 against the
// live production store, all three `[ran it]`:
//
//   - the whole brief was 263 columns through a pipe, 233 of them this table;
//   - at 80 columns FIVE of the six rows rendered the identical stub
//     `yes-one-mcp-session-is-one-wire-c…`;
//   - the ID column was padded to 145 characters, so a row whose id was 80
//     characters still started its KIND cell at column 147.
//
// ⛔ EVERY TEST HERE ASSERTS WHAT WAS RENDERED, NOT THAT SOMETHING WAS. A
// mutation setting every row's id to a constant "x" once passed this seat's
// predecessor's whole governing suite, because the rows were keyed on kind and
// nothing ever read an id's value. Each assertion below names a specific id
// and a specific line.

// The two doc-key ids from the live store, verbatim, and the pair that makes
// the defect visible: they agree for 80 characters and differ after.
const (
	govLongA = "yes-one-mcp-session-is-one-wire-connection-and-one-accept-" +
		"and-the-bridge-must-ex/what-this-ruling-does-not-say-cite-the-bound-" +
		"not-the-sentence"
	govLongB = "yes-one-mcp-session-is-one-wire-connection-and-one-accept-" +
		"and-the-bridge-must-ex/the-constraint-and-it-is-measured-rather-" +
		"than-cautionary"
)

func govLongRows() []BriefGoverning {
	return []BriefGoverning{
		{ID: govLongA, Kind: "decision", Title: "WHAT THIS RULING DOES NOT SAY"},
		{ID: govLongB, Kind: "decision", Title: "THE CONSTRAINT, and it is measured"},
		{
			ID: "01a0af22-0fa9-7976-84bb-aecb6a437699", Kind: "artefact",
			Title: "B72's quotation is traced to the transcript",
		},
	}
}

func govShortRows() []BriefGoverning {
	return []BriefGoverning{
		{ID: "d-one", Kind: "decision", Title: "the first ruling"},
		{ID: "d-two", Kind: "decision", Title: "the second ruling"},
	}
}

// ⛔ THE DEFECT ITSELF: two ids that differ only after their 80th character
// must not both render as the same thing.
func TestTwoIdsSharingALongPrefixAreNeverRenderedAsTheSameRow(t *testing.T) {
	got := briefGoverningSection("rig", govLongRows(), nil, briefStyle{Width: 80})

	if !strings.Contains(got, govLongA) {
		t.Errorf("the first id is not in the output whole, so it cannot be "+
			"pasted into `rig record get`:\n%s", got)
	}
	if !strings.Contains(got, govLongB) {
		t.Errorf("the second id is not in the output whole:\n%s", got)
	}
	// And the pair is genuinely confusable, or the assertion above is about
	// nothing. This is the positive control for the CASE, not for the code.
	if govLongA[:80] != govLongB[:80] {
		t.Fatal("the two fixture ids no longer share a prefix, so this test " +
			"proves nothing about the defect it was written for")
	}
}

// ⛔ NO ID IS EVER PASSED THROUGH briefElide, AT ANY WIDTH. A title cut at the
// margin still identifies its row; a key cut at the margin identifies nothing.
func TestNoGoverningIdIsEverElidedAtAnyWidth(t *testing.T) {
	for _, w := range []int{0, 40, 80, 100, 120, 200, 400} {
		got := briefGoverningSection("rig", govLongRows(), nil, briefStyle{Width: w})
		for _, want := range []string{govLongA, govLongB} {
			if !strings.Contains(got, want) {
				t.Errorf("width %d cut an id:\n%s", w, got)
			}
		}
	}
}

// THE CONTROL FOR THE LAYOUT SWITCH. Short ids must keep the three-column
// table, or the fix has replaced one layout with another rather than choosing
// between them from the data.
func TestShortIdsKeepTheThreeColumnTable(t *testing.T) {
	got := briefGoverningSection("rig", govShortRows(), nil, briefStyle{Width: 80})

	head := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "ID") || strings.HasPrefix(line, "KIND") {
			head = line
			break
		}
	}
	if !strings.HasPrefix(head, "ID") {
		t.Errorf("a table of short ids lost its ID column; its header is "+
			"%q:\n%s", head, got)
	}
	// The id and its title are on ONE line, which is what the column form is
	// for and what the keyed form gives up.
	if !strings.Contains(got, "d-one") {
		t.Fatalf("the fixture id is not rendered at all:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "d-one") && strings.Contains(line, "the first ruling") {
			return
		}
	}
	t.Errorf("a short id and its title are on two lines, so the three-column "+
		"form was abandoned for a table that fits:\n%s", got)
}

// ⛔ AND THE LONG CASE DOES SWITCH. Without this, the control above passes
// against a renderer that never uses the keyed form at all.
func TestLongIdsDropTheIdColumnAndSayWhy(t *testing.T) {
	got := briefGoverningSection("rig", govLongRows(), nil, briefStyle{Width: 80})

	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "ID ") {
			t.Errorf("the ID column survived a table it cannot fit:\n%s", got)
		}
	}
	if !strings.Contains(got, "rig record get") {
		t.Errorf("the layout changed and the page does not say why, so a "+
			"reader sees an id floating under a row with no explanation:\n%s", got)
	}
}

// ⛔ WHEN THE IDS DO FIT, THE ID COLUMN IS PINNED AND THE TITLE PAYS. briefFit
// takes width out of the WIDEST column, which is the id - right for prose,
// wrong for a key.
func TestTheTitlePaysForTheSqueezeAndTheIdDoesNot(t *testing.T) {
	// ⛔ THE ID MUST BE THE WIDEST COLUMN OR THIS TEST IS BLIND, and the first
	// version of it was: with a 180-character title beside a 40-character id,
	// briefFit cut the title because the title was widest, and the id survived
	// whether or not it was pinned. The shape that separates the two is a LONG
	// KEY beside a SHORT TITLE, over a budget the pair cannot fit.
	id := strings.Repeat("k", 70)
	rows := []BriefGoverning{
		{ID: id + "-a", Kind: "decision", Title: "a title of ordinary length"},
		{ID: id + "-b", Kind: "decision", Title: "another ordinary title"},
	}
	const width = 100
	got := briefGoverningSection("rig", rows, nil, briefStyle{Width: width})

	// The three-column form is what is under test; if the keyed form fired,
	// the pin is not what kept the id whole and this proves nothing.
	if !strings.Contains(got, "ID ") {
		t.Fatalf("the keyed form fired, so the PIN is untested here:\n%s", got)
	}
	for _, want := range []string{id + "-a", id + "-b"} {
		if !strings.Contains(got, want) {
			t.Errorf("the id was squeezed to make room for a shorter "+
				"title:\n%s", got)
		}
	}
	// And something WAS cut, or the budget never bound and the pin was never
	// asked to do anything.
	if !strings.Contains(got, "…") {
		t.Fatalf("nothing was elided at %d columns, so the fit did not run "+
			"and this test is not exercising the pin:\n%s", width, got)
	}
}

// ⛔ A PIPE WITH SHORT IDS KEEPS THE THREE-COLUMN TABLE. The default width
// CHOOSES the layout, and a renderer that skipped the default would send every
// piped brief through the keyed form - which costs a line per row for nothing,
// on the widest surface rig has.
func TestAPipeWithShortIdsKeepsTheColumnTable(t *testing.T) {
	got := briefGoverningSection("rig", govShortRows(), nil, briefStyle{})

	if !strings.Contains(got, "ID ") {
		t.Errorf("a pipe dropped the ID column for ids five characters "+
			"long:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "d-one") && strings.Contains(line, "the first ruling") {
			return
		}
	}
	t.Errorf("a short id and its title are on two lines in a pipe:\n%s", got)
}

// ⛔ A PIPE STILL LOSES NOTHING. briefStyle's zero means unbounded because a
// pipe's consumer reads every byte. Choosing a LAYOUT for a default width
// takes no bytes away - briefDefaultWrap already governs this file's prose on
// that argument - but CUTTING a cell does, and the first version of this code
// put an ellipsis into a pipe.
func TestAPipeGetsTheKeyedLayoutAndLosesNoBytes(t *testing.T) {
	long := strings.Repeat("a title with real words in it ", 12)
	rows := []BriefGoverning{
		{ID: govLongA, Kind: "decision", Title: long},
		{ID: govLongB, Kind: "decision", Title: long + " and more"},
	}
	got := briefGoverningSection("rig", rows, nil, briefStyle{})

	if strings.Contains(got, "…") {
		t.Errorf("an elision marker was printed into a pipe:\n%s", got)
	}
	if !strings.Contains(got, long) {
		t.Errorf("a title was cut with no terminal to cut it for:\n%s", got)
	}
	// AND the layout still changed, or the width is unfixed in a pipe - which
	// is where the 263 columns were measured.
	if !strings.Contains(got, "rig record get") {
		t.Errorf("a pipe kept the unbounded three-column table:\n%s", got)
	}
	if !strings.Contains(got, govLongA) {
		t.Errorf("the id did not survive into a pipe whole:\n%s", got)
	}
}

// ⛔ THE COUNT LINE WAS THE LAST OVERRUNNING LINE ON THE PAGE. Measured at 80
// columns against the live store it rendered at 84, and the command it carries
// grows with the project slug and the kind, so the overrun is unbounded.
func TestTheGoverningCountLineIsWrappedToTheTerminal(t *testing.T) {
	counts := []BriefKindCount{{Kind: "decision", Count: 451}}
	rows := make([]BriefGoverning, 0, 6)
	for i := range 6 {
		rows = append(rows, BriefGoverning{
			ID: "d-" + strings.Repeat("x", i+1), Kind: "decision", Title: "t",
		})
	}
	got := briefGoverningSection("rig", rows, counts, briefStyle{Width: 80})

	var hint string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "last 5 shown") {
			hint = line
		}
		if n := len([]rune(line)); n > 80 {
			t.Errorf("a line is %d columns wide at a terminal of 80: %q", n, line)
		}
	}
	if hint == "" {
		t.Fatalf("the cap's exit line is not printed at all, so a reader "+
			"cannot reach the other 446:\n%s", got)
	}
	// The count is still aligned with the rows that carry no hint, which is
	// what putting the prefix in the wrapper's LEAD buys.
	if !strings.HasPrefix(hint, "  decision     451") {
		t.Errorf("the count lost its alignment when it gained a wrap: %q", hint)
	}
}
