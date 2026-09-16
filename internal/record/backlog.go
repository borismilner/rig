// Backlog parsing: a markdown work-item table read as records.
//
// ⛔ THIS LIVED IN acceptance_test.go UNTIL rig PROMOTED IT, AND THE REASON IT
// MOVED IS THE REASON IT MUST NOT BE COPIED. Seeding rig's real backlog through
// the CLI is the last step of the MVP, and the only thing that could read
// BACKLOG.md was a test helper - so anything written in cmd/ to drive that
// seeding would have been a SECOND parser of one document. Two parsers of one
// file that drift is not a hypothetical here: this package's own history is a
// row regex, two cross-checks and a grep that all required a pipe this document
// does not always have, agreeing on a number that was wrong.
//
// It is promoted UNCHANGED. The answers it gives about rig's backlog today -
// 45 rows, 9 closed, 36 open, 4 claiming a terminal state unstruck, 1 malformed
// - are pinned in backlogpin_test.go, landed in its own commit BEFORE this file
// existed, against both a fixed fixture and the live document.

package record

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// backlogRow matches any table row whose first cell is a backlog id.
//
// ⛔ IT MUST NOT REQUIRE THE TITLE CELL TO OPEN WITH `**`. The pattern here was
// `^\| (B\d+) \| \*\*(.+)$` until 2026-09-16, and a CLOSED row opens `~~**`
// because strikethrough is how this document marks one. So it dropped eight
// rows - B7, B9, B11, B19, B20, B22, B31, B33 - every one of them closed, and
// seeded 36 of 44 while three documents reported 44. The demonstration ran
// against rig's backlog with its history removed, which is the one distortion
// most likely to flatter a next-up list.
var backlogRow = regexp.MustCompile(`^\|\s*(B\d+)\b`)

// backlogRowAnywhere is the SECOND instrument, and it exists to disagree.
//
// It is a multiline match over the whole file rather than a line scan, so it
// shares no code path with the scanner - not the buffer bound, not the loop,
// not the split. The guard asserts the two agree as SETS.
//
// ⛔ IT EARNED ITS KEEP ON ITS FIRST RUN. It found B21, which FOUR separate
// hand-counts had missed - the old regex, two python cross-checks, and the
// `grep -cE '^\| B[0-9]+ \|'` that produced the 44 reported to the lead and
// into READINESS.txt. Every one of them required a pipe after the id, and B21's
// row does not have one. THE TRUE ROW COUNT IS 45, NOT 44.
var backlogRowAnywhere = regexp.MustCompile(`(?m)^\|\s*(B\d+)\b`)

// BacklogItem is one row of a backlog table, read rather than interpreted.
type BacklogItem struct {
	ID    string
	Title string

	// Done is the DOCUMENT'S OWN closure mark: the title struck through.
	//
	// ⛔ THIS IS EVIDENCE RATHER THAN INFERENCE. B7's State cell says it in as
	// many words - "done, struck not deleted" - so the file states its own
	// convention. Striking a title is a deliberate act somebody performed;
	// scanning the row's prose for the word "done" is a lottery, and the
	// pattern this replaced lost B44 to a comma, B15 to the word CLOSED and
	// B26 to a full stop.
	Done bool

	// ClaimsDone is a row whose STATE cell leads with a terminal disposition
	// while its title is NOT struck.
	//
	// ⛔ COUNTED AND REPORTED, NEVER FOLDED INTO Done. cells[2] and cells[5]
	// are different axes: the item cell carries whether the WORK is closed,
	// the state cell carries how the FINDING was disposed of. B9 is struck and
	// done while its state says "argued", and that is not a contradiction.
	// Whether a row that claims done in prose without being struck is closed
	// is a question about the document, and this parser does not own the
	// document - so it hands the fact up rather than deciding.
	ClaimsDone bool

	// Struck records WHICH of Done's two clauses closed this row: the
	// strikethrough, or a terminal lead in the item cell.
	//
	// ⛔ IT EXISTS BECAUSE THE LABEL LIED. A report printed "%d closed (title
	// struck)" over a set that is not all struck - 8 of 9 - so two seats
	// measured honestly and got different numbers. The label asserted a
	// one-clause test the code does not run, which is the same family as a
	// bound that cannot notice itself going stale: a caption is a claim, and an
	// unchecked one goes wrong exactly where nobody looks.
	Struck bool

	// Malformed is a row whose markdown is irregular enough that the id cell
	// and the item cell ran together. Named in the answer so the document's
	// owner can fix it; never a reason to drop the row.
	//
	// ⛔ SECTION 39'S MIGRATION RULING IS IMPORT EVERYTHING AND FLAG WHAT IS
	// IRREGULAR. A row a parser quietly tidies is a row nobody ever fixes.
	//
	// ⛔ IT NO LONGER MEANS "DO NOT TRUST THIS ROW'S TITLE". It did: the title
	// was read from cells[2], which on a shifted row is the EVIDENCE cell, so
	// B21's title in rig's own backlog was the word "evidence". That is fixed -
	// the title is taken from the reconstructed cell - and the fix is its own
	// commit rather than smuggled into the promotion, because it CHANGES AN
	// ANSWER and the pin existed precisely to make that visible.
	//
	// The flag still means the DOCUMENT is irregular and somebody should mend
	// the row. rig imports it either way: section 39's migration ruling is
	// import everything and flag what is irregular.
	Malformed bool
}

// cells splits a markdown table row and trims every cell. The table is
// | # | Item | Evidence | Adopter | State |, so cells[1] is the id, cells[2]
// the item and cells[5] the state.
//
// ⛔ A PIPE INSIDE A BACKTICK CODE SPAN IS NOT A CELL BOUNDARY, AND THIS USED
// TO BE `strings.Split(line, "|")`, WHICH SAID IT WAS. A row whose item cell
// quotes a regex or a shell alternation - `a|b` - split into one cell too many,
// and every cell after it shifted LEFT. The state cell then read as the adopter
// cell, so the row's terminal disposition was decided by a column that never
// held one. B8 in rig's own backlog is the live instance: its ClaimsDone was
// being read from "**PRESENCE** proposed, **the lead** holds the shape".
//
// ⛔ IT MOVED NO PINNED FIGURE THE DAY IT WAS FIXED, AND THAT IS WHY IT IS
// WORTH SAYING OUT LOUD. Neither cell's bold lead happened to be a terminal
// word, so the wrong answer and the right answer agreed. The defect was
// invisible to every count in this package and would have surfaced the first
// time somebody wrote a backticked pipe into a row that closes.
func cells(line string) []string {
	var out []string
	var cur strings.Builder
	inCode := false
	for _, r := range line {
		switch {
		case r == '`':
			inCode = !inCode
			cur.WriteRune(r)
		case r == '|' && !inCode:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, strings.TrimSpace(cur.String()))
}

// titleOf takes the item cell's title: its FIRST BOLDED RUN, which is how every
// row in this table writes one, struck or not.
//
// ⛔ IT USED TO TRIM THE MARKERS OFF THE FRONT AND CUT AT THE NEXT ONE, WHICH
// ASSUMED THE CELL STARTS WITH THEM. B21 in rig's own backlog starts with a
// tick, so trimming matched nothing and the cut landed at the tick: its title
// read "✅". The rows are handwritten and a decoration before the bold is
// ordinary, so the title is located rather than trimmed to.
//
// The fallback matters for the same reason: a cell with no bold at all still
// has to yield something, and the whole trimmed cell is the honest answer.
func titleOf(cell string) string {
	if b := strings.TrimSpace(boldLead(cell)); b != "" {
		return b
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(cell, "~~"), "**"))
}

// boldLead returns a cell's first bolded run, which is how every row in this
// table states its disposition.
func boldLead(cell string) string {
	i := strings.Index(cell, "**")
	if i < 0 {
		return ""
	}
	rest := cell[i+2:]
	if j := strings.Index(rest, "**"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// terminalDispositions are the bold leads that mean the work is over.
var terminalDispositions = map[string]bool{
	"DONE": true, "CLOSED": true, "REJECTED": true, "RETRACTED": true,
}

// firstWord returns a cell's first word, upper-cased and stripped of the
// punctuation these rows attach to it - "DONE," and "DONE." both mean DONE.
func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " ,.;:"); i > 0 {
		s = s[:i]
	}
	return strings.ToUpper(s)
}

// ParseBacklog reads a backlog table and returns one item per row.
//
// It refuses rather than guessing on two conditions, and both have cost this
// project a wrong number: a row with too few cells to be the table's shape, and
// a disagreement between the line scanner and a whole-file scan that shares no
// code path with it.
func ParseBacklog(r io.Reader) ([]BacklogItem, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var out []BacklogItem
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		// ⛔ len RATHER THAN A NIL CHECK, AND IT IS NOT COSMETIC. This read
		// `m == nil` while it lived in a _test.go file, where gocritic does not
		// run. Promoting it into product code is what made the linter reach it:
		// FindStringSubmatch returns nil or a slice of 1+len(groups), so a nil
		// check happens to be sufficient for THIS pattern and stops being so
		// the moment somebody edits the pattern's groups.
		m := backlogRow.FindStringSubmatch(line)
		if len(m) < 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		c := cells(line)
		if len(c) < 6 {
			return nil, fmt.Errorf("%s has %d cells, want at least 6 "+
				"(| # | Item | Evidence | Adopter | State |): %.120s", m[1], len(c), line)
		}

		// ⛔ ONE ROW IS MISSING THE PIPE AFTER ITS ID, SO ITS ID CELL ABSORBED
		// THE FRONT OF ITS ITEM CELL - INCLUDING THE CLOSURE MARKER.
		//
		// B21, and it is why the second instrument exists. Reconstructing the
		// item cell is the honest repair: the row is real, it is closed, and
		// every count anyone ran missed it because every one required the pipe.
		item, malformed := c[2], false
		if rest := strings.TrimSpace(strings.TrimPrefix(c[1], m[1])); rest != "" {
			item, malformed = rest+" "+item, true
		}

		it := BacklogItem{
			ID: m[1],
			// ⛔ THE TITLE COMES FROM THE RECONSTRUCTED ITEM CELL, NOT FROM
			// cells[2]. On a row missing the pipe after its id every cell has
			// shifted left, so cells[2] is the EVIDENCE cell - B21 in rig's own
			// backlog read its title as the word "evidence" for as long as this
			// parser has existed. `item` is already the repaired cell three
			// lines above, and for a well-formed row it IS cells[2], so the
			// normal path is untouched.
			Title: titleOf(item),
			// The document's own closure mark, in the item cell: a struck
			// title, or a terminal bold lead where the strikethrough would be.
			Struck:    strings.HasPrefix(item, "~~"),
			Done:      strings.HasPrefix(item, "~~") || terminalDispositions[firstWord(boldLead(item))],
			Malformed: malformed,
		}
		it.ClaimsDone = !it.Done && terminalDispositions[firstWord(boldLead(c[5]))]
		out = append(out, it)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// ⛔ THE GUARD IS AN EQUALITY BETWEEN TWO INSTRUMENTS, NOT A BOUND.
	//
	// This was `if len(items) < 30 { ... "expected the real file's ~44" }`.
	// Over a 44-row file it read 36, and 36 >= 30, so it passed and the message
	// naming 44 never printed. A BOUND CANNOT NOTICE ITSELF GOING STALE - it is
	// the same defect as a check that cannot tell "nothing is wrong" from "the
	// check did not run".
	//
	// So the file is scanned a second time by a pattern sharing no code path
	// with the scanner above, and the two must agree AS SETS. A count equality
	// would still hide a swap; the set difference names the rows that went
	// missing, which is the failure that actually happened.
	byScan := map[string]bool{}
	for _, it := range out {
		byScan[it.ID] = true
	}
	var missing []string
	for _, m := range backlogRowAnywhere.FindAllStringSubmatch(string(raw), -1) {
		if !byScan[m[1]] {
			missing = append(missing, m[1])
			delete(byScan, m[1])
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("the row scanner read %d rows and the whole-file scan finds %d; "+
			"it never saw %v", len(out), len(out)+len(missing), missing)
	}
	return out, nil
}
