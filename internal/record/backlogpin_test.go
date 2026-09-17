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
		[]string{"B1", "B10", "B11", "B2", "B3", "B4", "B5", "B6", "B7", "B8", "B9"})
	pin(t, "closed by a struck title", idsWhere(items, func(i BacklogItem) bool { return i.Done && i.Struck }),
		[]string{"B2", "B7"})
	pin(t, "closed by a terminal lead in the item cell", idsWhere(items, func(i BacklogItem) bool { return i.Done && !i.Struck }),
		[]string{"B3", "B5", "B6", "B9"})
	// ⛔ B11 IS HERE BECAUSE OF A SPLITTER, NOT BECAUSE OF A DISPOSITION. Its item
	// cell quotes a pipe inside a code span, and a `strings.Split(line, "|")`
	// cuts the row there: every later cell shifts LEFT, so cells[5] lands on the
	// ADOPTER cell and this row's terminal state is decided by a column that
	// never held one. It reads OPEN under the old splitter and CLOSED-claiming
	// under the right one, and NOTHING ELSE IN THIS SUITE CAN TELL THEM APART.
	pin(t, "claims a terminal state unstruck, seeded OPEN", idsWhere(items, func(i BacklogItem) bool { return i.ClaimsDone }),
		[]string{"B11", "B4"})
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
		"B1":  "A plain open row",
		"B2":  "A struck row, which is how the document closes one",
		"B6":  "REJECTED, the third terminal word",
		"B8":  "A row whose bold never closes before the pipe",
		"B11": "A row whose item cell holds a pipe inside backticks, `a|b`",
	} {
		if got := titles[id]; got != want {
			t.Errorf("%s title changed\n  was:  %q\n  now:  %q", id, want, got)
		}
	}

	// ⛔ B99 LIVES IN A TWO-COLUMN TABLE THIS PARSER MUST NOT READ, AND IT IS
	// HERE BECAUSE A MUTATION SURVIVED WITHOUT IT. Deleting the not-a-work-item
	// guard left the whole suite green against the real document, because every
	// id in rig's adopter table also exists as a real row ABOVE it and the
	// duplicate rule skipped them. The guard was protecting nothing that any
	// test could see. B99 exists nowhere else, so the guard now has to work.
	for _, it := range items {
		if it.ID == "B99" {
			t.Errorf("B99 is in a `| Row | Adopter, as a ROLE |` table and is not a " +
				"work item; reading it means the parser is matching on the first " +
				"cell rather than on the table's header")
		}
	}

	// The duplicate id later in the fixture must be DROPPED, not re-read.
	if n := len(items); n != 11 {
		t.Errorf("read %d rows, want 11 - a repeated id must be kept once, not twice", n)
	}

	// ⛔ A DEFECT THIS PIN CAUGHT BEING FIXED, WHICH IS WHAT IT IS FOR. On a row
	// missing the pipe after its id every cell shifts left, so the title used to
	// be read from cells[2] - the EVIDENCE cell - and B7 here, like B21 in rig's
	// own backlog, reported its title as the word "evidence". The parser now
	// takes the title from the RECONSTRUCTED cell.
	//
	// The pin went RED on that change and was updated in the same commit as the
	// fix, which is the opposite of the thing it exists to prevent: a refactor
	// claiming to move code while quietly moving an answer. A deliberate change
	// updates its pin and says so; a silent one is caught.
	for _, it := range items {
		if it.ID == "B7" && it.Title != "A row missing the pipe after its id, so the id cell absorbs the title" {
			t.Errorf("B7 is the malformed row and its title must come from the "+
				"RECONSTRUCTED cell, not from cells[2] which is the evidence "+
				"cell on a shifted row; got %q", it.Title)
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

	// ⛔ THE WHOLE SET, NOT THE TOTAL, AND THAT IS THE POINT OF THIS BLOCK.
	// A count equality hides a swap, and 64 arriving because 64 was expected is
	// the least informative outcome available. These ids were derived from the
	// document by a header-aware scan sharing no code path with the parser.
	pin(t, "every work item read", idsWhere(items, func(BacklogItem) bool { return true }),
		[]string{
			"B1", "B10", "B11", "B12", "B13", "B14", "B15", "B16",
			"B17", "B18", "B19", "B2", "B20", "B21", "B22", "B23",
			"B24", "B25", "B26", "B27", "B28", "B29", "B3", "B30",
			"B31", "B32", "B33", "B34", "B35", "B36", "B37", "B38",
			"B39", "B4", "B40", "B41", "B42", "B43", "B44", "B45",
			"B46a", "B46b", "B46c", "B46e", "B46f", "B46g", "B47", "B48",
			"B49", "B5", "B50", "B51", "B52", "B53", "B54", "B55",
			"B56", "B57", "B58", "B59", "B6", "B60", "B61", "B62", "B63",
			// B64-B70 filed 2026-09-17 by the nine-surface attack, all OPEN.
			// B71 filed 2026-09-17 by team-lead generation 11: the contrast
			// gate cannot see a background-image, so it returns green over
			// ground the eye never sees. OPEN.
			"B64", "B65", "B66", "B67", "B68", "B69", "B7", "B70", "B71",
			// B72 filed 2026-09-17 by team-lead generation 12, at Boris's own
			// challenge: two processes both call themselves the production
			// estate and rig's singleton is per runtime dir, so neither is
			// told the other exists. OPEN.
			// B73 filed 2026-09-17 by team-lead generation 13: the eleven
			// ranked rows of the critical path are in no record, and one of
			// them is Boris's own order. OPEN, and it is B66's named
			// remainder rather than a new finding.
			"B72", "B73", "B8", "B9",
		})

	// B65 joins this set 2026-09-17: the field predicate landed at rig
	// `1c3a8c4` and the row was struck, which is this document's own closure
	// mark. The SET is what says which row closed; the count alone would not.
	// B64 joins this set 2026-09-17: the twelfth brief section landed at rig
	// `22faf89` and the row was struck. The SET is what says WHICH row
	// closed; the count alone would have accepted a swap.
	// B66 joins this set 2026-09-17: the heading grain reached the store with
	// its title and status (rig `f970106`), `rigseed --check` computes the
	// whole-set equality the row's own bar names, and B46 is in `rig brief
	// rig` on the live production daemon. The row was struck.
	pin(t, "closed, struck", struck,
		[]string{"B11", "B19", "B20", "B22", "B31", "B33", "B64", "B65", "B66", "B7", "B9"})
	pin(t, "closed by a terminal lead in the item cell", byLead, []string{"B21"})
	pin(t, "claims a terminal state unstruck, seeded OPEN", claims,
		[]string{"B15", "B24", "B25", "B44", "B46a", "B48", "B55", "B56"})
	pin(t, "closed by a RULING - a tick on the id, a terminal state, no work done",
		idsWhere(items, func(i BacklogItem) bool { return i.RuledClosed }),
		[]string{"B55", "B56"})
	pin(t, "malformed", malformed, []string{"B21"})

	// ⛔ THE FIVE IDS IN THE ADOPTER TABLE MUST NOT BE HERE, AND THIS IS THE
	// ASSERTION THAT SAYS SO. `| Row | Adopter, as a ROLE |` puts B6, B10, B18,
	// B20 and B21 in two-column rows that are not work items. All five already
	// exist as real rows elsewhere, so a parser that read them would not add an
	// unknown id - it would SUPERSEDE five real rows with a two-cell shape, and
	// only the titles would show it.
	for _, id := range []string{"B6", "B10", "B18", "B20", "B21"} {
		for _, it := range items {
			if it.ID == id && it.Title == "" {
				t.Errorf("%s has an empty title, which is what reading it from the "+
					"two-column adopter table looks like", id)
			}
		}
	}

	open := len(items) - len(struck) - len(byLead)
	for _, c := range []struct {
		what      string
		got, want int
	}{
		// 68 -> 75 and 59 -> 66 on 2026-09-17: the 9-surface attack filed
		// B64-B70, seven OPEN rows, so `closed` is unmoved and both other
		// numbers rise by exactly seven. The document changed, not the parser -
		// established the way this test's own failure message says to, by
		// checking that TestTheBacklogParserReadsEveryRowShapeTheSameWay was
		// GREEN at the same commit.
		//
		// 75 -> 76 and 66 -> 67 later the same day: B71, one OPEN row, filed by
		// team-lead generation 11. Same discrimination run again - the shape
		// test was GREEN at this commit and the SET pin reported exactly one
		// added id, {B71}, which is the assertion that would have caught a swap
		// and the one a previous generation moved the counts without checking.
		// 9 -> 10 closed and 67 -> 66 open, same day: B65 was struck when the
		// field predicate landed. `rows` is UNMOVED at 76, which is the check
		// that says a row was closed rather than added or removed.
		//
		// 76 -> 77 and 66 open unmoved, 2026-09-17: TWO changes in one commit,
		// which is why the SET pins above matter more than these numbers. B72
		// was FILED (open, +1 row) and B64 was STRUCK (closed, +1). The two
		// move `open` in opposite directions and it lands back on 66 - a count
		// that is unchanged while two rows moved is exactly the shape a count
		// pin cannot see, and the struck SET above is what catches it.
		// 2026-09-17, generation 13, TWO changes and the SET pins are what
		// separate them: B66 was STRUCK (closed 11 -> 12) and B73 was FILED
		// (rows 77 -> 78, open +1). `open` therefore lands back on 66
		// unmoved, which is exactly the shape a count pin cannot see - the
		// struck SET and the every-item SET above are what catch it.
		//
		// ⛔ B73 EXISTS BECAUSE THE INSTRUMENT DEMANDED IT. B66's closing
		// text cited B73 before the row was written, and
		// TestEveryBacklogIdInTheDocumentIsEitherARecordOrReported went RED
		// on a dangling id the same minute - a cross-reference check this
		// project has said twice it does not have, working.
		{"rows", len(items), 78},
		{"closed", len(struck) + len(byLead), 12},
		{"open", open, 66},
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
