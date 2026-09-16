package record

import (
	"sort"
	"strings"
	"testing"
)

// ⛔ THESE TWO TESTS EXIST BECAUSE `classify` ONLY PRINTS.
//
// The figures everybody quotes for rig's backlog - 45 rows, 9 closed, 36 open,
// 4 claiming a terminal state unstruck, 1 malformed - are produced by a
// `fmt.Printf` inside `classify` and NOTHING ASSERTS ANY OF THEM. A change to
// the parser can move every one of those numbers and leave the whole suite
// green, which is this package's own "a label is a claim and no gate checks a
// caption" defect standing in the file that keeps finding it.
//
// They are landed BEFORE the parser is promoted out of this file, deliberately
// and in their own commit. A refactor whose test was written in the same commit
// has proved nothing: the test then describes wherever the code ended up.

// backlogShapes is a hand-written file holding one row of every SHAPE the real
// document uses. It is NOT a copy of rig's backlog and must never become one -
// `BACKLOG.md` is reached through a gitignored symlink precisely so its text
// stays out of this repository.
const backlogShapes = "testdata/backlog-shapes.md"

// ids, sorted, so a set comparison cannot be defeated by ordering.
func idsWhere(items []BacklogItem, keep func(BacklogItem) bool) []string {
	var out []string
	for _, it := range items {
		if keep(it) {
			out = append(out, it.ID)
		}
	}
	sort.Strings(out)
	return out
}

func pin(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("%s changed\n  was:  %v\n  now:  %v", what, want, got)
	}
}

// TestTheBacklogParserReadsEveryRowShapeTheSameWay pins the parse of a FIXED
// document, so it answers one question and only one: did the parser change?
//
// ⛔ IT IS THE HALF THAT SURVIVES BORIS EDITING THE BACKLOG. Its companion
// below pins the real file and therefore moves whenever the document does, so
// the two together are discriminating: this one green and that one red means
// the DOCUMENT changed; this one red means the PARSER did. Neither can say that
// alone, and a single instrument that cannot separate those two causes is the
// shape this repository has now recorded eleven times.
func TestTheBacklogParserReadsEveryRowShapeTheSameWay(t *testing.T) {
	items := readBacklogFrom(t, backlogShapes)

	pin(t, "every row read", idsWhere(items, func(BacklogItem) bool { return true }),
		[]string{"B1", "B10", "B2", "B3", "B4", "B5", "B6", "B7", "B8", "B9"})
	pin(t, "closed by a struck title", idsWhere(items, func(i BacklogItem) bool { return i.Done && i.Struck }),
		[]string{"B2", "B7"})
	pin(t, "closed by a terminal lead in the item cell", idsWhere(items, func(i BacklogItem) bool { return i.Done && !i.Struck }),
		[]string{"B3", "B5", "B6", "B9"})
	pin(t, "claims a terminal state unstruck, seeded OPEN", idsWhere(items, func(i BacklogItem) bool { return i.ClaimsDone }),
		[]string{"B4"})
	pin(t, "malformed, parsed and seeded anyway", idsWhere(items, func(i BacklogItem) bool { return i.Malformed }),
		[]string{"B7"})

	// ⛔ THE TITLES ARE PINNED BECAUSE NOTHING ELSE PINS THEM. A mutation that
	// made titleOf stop at the first "~~" instead of at either marker SURVIVED
	// the first pass of this test: every set was still right, because a title
	// is not a set. The promotion moves titleOf, so its answer is pinned here
	// rather than left as the one thing a reader assumes somebody checked.
	titles := map[string]string{}
	for _, it := range items {
		titles[it.ID] = it.Title
	}
	for id, want := range map[string]string{
		"B1": "A plain open row",
		"B2": "A struck row, which is how the document closes one",
		"B6": "REJECTED, the third terminal word",
		"B8": "A row whose bold never closes before the pipe",
	} {
		if got := titles[id]; got != want {
			t.Errorf("%s title changed\n  was:  %q\n  now:  %q", id, want, got)
		}
	}

	// The duplicate id later in the fixture must be DROPPED, not re-read.
	if n := len(items); n != 10 {
		t.Errorf("read %d rows, want 10 - a repeated id must be kept once, not twice", n)
	}

	// ⛔ A DEFECT PINNED AS IT IS, NOT AS IT SHOULD BE. On a row missing the
	// pipe after its id, the title is taken from cells[2] - which for that row
	// is the EVIDENCE cell, because every cell has shifted left. So B7's title
	// reads "evidence" rather than its own words, and B21 in the real document
	// has the same defect. It is REPORTED to the lead rather than repaired
	// here: the promotion must move this parser without changing its answers,
	// and a fix smuggled into a refactor is a change nobody reviewed.
	for _, it := range items {
		if it.ID == "B7" && it.Title != "evidence" {
			t.Errorf("B7's title is %q; the malformed-row title defect was pinned as %q "+
				"and a promotion must not quietly repair it", it.Title, "evidence")
		}
	}
}

// TestRigsOwnBacklogStillParsesAsItDidBeforeTheParserMoved pins the real
// document's figures - the ones READINESS.txt, BACKLOG.md B46b and three
// handoffs all quote - so the promotion cannot move them silently.
//
// ⛔ IT CAN GO RED FOR TWO DIFFERENT REASONS AND SAYS SO IN ITS OWN FAILURE.
// The backlog is a live document and Boris edits it; a red here is only a
// parser regression if the fixture test above is GREEN. Naming both causes in
// the message is the whole difference between this and the `len(items) < 30`
// bound it descends from, which could not notice itself going stale.
func TestRigsOwnBacklogStillParsesAsItDidBeforeTheParserMoved(t *testing.T) {
	items := readBacklog(t) // skips outside the working tree - B46e

	struck := idsWhere(items, func(i BacklogItem) bool { return i.Done && i.Struck })
	byLead := idsWhere(items, func(i BacklogItem) bool { return i.Done && !i.Struck })
	claims := idsWhere(items, func(i BacklogItem) bool { return i.ClaimsDone })
	malformed := idsWhere(items, func(i BacklogItem) bool { return i.Malformed })

	pin(t, "closed, struck", struck,
		[]string{"B11", "B19", "B20", "B22", "B31", "B33", "B7", "B9"})
	pin(t, "closed by a terminal lead in the item cell", byLead, []string{"B21"})
	pin(t, "claims a terminal state unstruck, seeded OPEN", claims,
		[]string{"B15", "B24", "B25", "B44"})
	pin(t, "malformed", malformed, []string{"B21"})

	open := len(items) - len(struck) - len(byLead)
	for _, c := range []struct {
		what      string
		got, want int
	}{
		{"rows", len(items), 45},
		{"closed", len(struck) + len(byLead), 9},
		{"open", open, 36},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, pinned at %d.\n"+
				"⛔ TWO CAUSES AND THIS TEST CANNOT TELL THEM APART ON ITS OWN:\n"+
				"   - if TestTheBacklogParserReadsEveryRowShapeTheSameWay is GREEN, the DOCUMENT changed "+
				"and this pin should be updated in a commit that says so;\n"+
				"   - if that test is RED too, the PARSER changed and the promotion moved an answer.",
				c.what, c.got, c.want)
		}
	}
}
