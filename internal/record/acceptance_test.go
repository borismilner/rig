package record

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// backlogPath is rig's REAL development plan. Not a fixture and not a copy.
//
// Section 39's MVP test, in section 39's own words: rig's own project and
// cases are managed IN rig - not on a fixture, not on a synthetic second
// project, but on rig itself, which is the only project there is. A test that
// seeds invented rows demonstrates the code runs. Only the real document
// demonstrates the MVP.
//
// ⛔ THAT SENTENCE IS SECTION 39's PROSE AND IS NOT A QUOTATION FROM BORIS.
// It stood here as `Boris: "..."` in quote marks until 2026-09-16, relayed into
// this comment from a message asserting a verbatim. Every one of his verbatims
// in section 39 is a blockquote and this is not one of them. The project's rule
// is in records.go: a quotation attributed to him that cannot be traced is a
// paraphrase until proved otherwise, and a code comment is the same surface as
// a document.
//
// ⛔ IT REACHES THE FILE THROUGH THE REPO-ROOT SYMLINK, WHICH IS GITIGNORED, SO
// THIS TEST SKIPS WHEREVER THE LOGBOOK IS NOT CHECKED OUT BESIDE rig - a fresh
// clone, a detached gate worktree, CI. THAT IS DELIBERATE AND IT IS ALSO A
// LIMITATION WORTH STATING: `make ci` does NOT run this test, so a green gate
// is not evidence the MVP demonstration passes. Run it in the working tree.
// The alternative - committing a copy of the backlog as a fixture - fails the
// only requirement this test exists to meet.
const backlogPath = "../../BACKLOG.md"

// requireBacklog turns this test's skip into a failure. Set by `make mvp-demo`,
// which exists so that "has the MVP acceptance demonstration passed" has ONE
// command whose green cannot be a skip. B46e.
const requireBacklog = "RIG_RECORD_REQUIRE_BACKLOG"

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
// shares no code path with the scanner above - not the buffer bound, not the
// loop, not the split. The guard asserts the two agree as SETS.
//
// ⛔ IT EARNED ITS KEEP ON ITS FIRST RUN. It found B21, which FOUR separate
// hand-counts had missed - the old regex, two python cross-checks, and the
// `grep -cE '^\| B[0-9]+ \|'` that produced the 44 reported to the lead and
// into READINESS.txt. Every one of them required a pipe after the id, and B21's
// row does not have one. THE TRUE ROW COUNT IS 45, NOT 44.
var backlogRowAnywhere = regexp.MustCompile(`(?m)^\|\s*(B\d+)\b`)

// cells splits a markdown table row and trims every cell. The table is
// | # | Item | Evidence | Adopter | State |, so cells[1] is the id, cells[2]
// the item and cells[5] the state.
func cells(line string) []string {
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// titleOf takes the item cell's title, past any strikethrough and bold, and
// stops at whichever marker closes it. The rows are handwritten and not all of
// them close the bold before the pipe.
func titleOf(cell string) string {
	s := strings.TrimPrefix(cell, "~~")
	s = strings.TrimPrefix(s, "**")
	for _, cut := range []string{"**", "~~"} {
		if i := strings.Index(s, cut); i > 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
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

type backlogItem struct {
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
	// is a question about the document, and this test does not own the
	// document - so it hands the count up rather than deciding.
	ClaimsDone bool

	// Struck records WHICH of Done's two clauses closed this row: the
	// strikethrough, or a terminal lead in the item cell.
	//
	// ⛔ IT EXISTS BECAUSE THE LABEL LIED. classify printed "%d closed (title
	// struck)" over a set that is not all struck - 8 of 9 - so two seats
	// measured honestly and got different numbers. The label asserted a
	// one-clause test the code does not run, which is the same family as a
	// bound that cannot notice itself going stale: a caption is a claim, and an
	// unchecked one goes wrong exactly where nobody looks.
	Struck bool

	// Malformed is a row whose markdown is irregular enough that the id cell
	// and the item cell ran together. Counted and named in the report so the
	// document's owner can fix it; never a reason to drop the row.
	Malformed bool
}

// readBacklog pulls the real rows out of the real file.
func readBacklog(t *testing.T) []backlogItem {
	t.Helper()
	f, err := os.Open(backlogPath)
	if err != nil {
		// ⛔ THE SKIP IS THE HOLE, SO IT HAS AN OFF SWITCH. B46e.
		//
		// This test reaches rig's real backlog through a gitignored symlink, so
		// it cannot run in a fresh clone, a detached gate worktree or CI - and
		// a skip and a pass are the same colour to everything that reads a
		// gate. That is how "a green make ci means the MVP demonstration
		// passed" became sayable when it was never true.
		//
		// Committing a fixture copy was considered and rejected: it fails the
		// one requirement this test exists to meet. What CAN be fixed is the
		// silence. With requireBacklog set, a missing backlog is a FAILURE,
		// so a caller that means to demand the demonstration gets an answer
		// that cannot be mistaken for one. `make mvp-demo` sets it.
		if os.Getenv(requireBacklog) != "" {
			t.Fatalf("%s is set, so this demonstration was DEMANDED, and rig's own backlog "+
				"is not at %s from here: %v\n\nIt reaches the backlog through a gitignored "+
				"symlink, so it needs the logbook checked out beside rig - run it in the "+
				"working tree, not in a gate worktree or a fresh clone.",
				requireBacklog, backlogPath, err)
		}
		t.Skipf("rig's own backlog is not at %s from here: %v\n"+
			"⛔ THIS IS A SKIP, NOT A PASS - B46e. Set %s=1 to make it a failure, "+
			"or run `make mvp-demo` in the working tree.", backlogPath, err, requireBacklog)
	}
	defer func() { _ = f.Close() }()
	return parseBacklog(t, f)
}

// readBacklogFrom parses any file in the backlog's table shape.
//
// It exists so the parser can be pinned against a fixture that holds one row of
// every SHAPE, without waiting for somebody to edit rig's real backlog. A
// missing fixture is a FAILURE and never a skip: the skip above is about rig's
// own document being unreachable from a gate worktree, which is a fact about
// the checkout, and a fixture that has gone missing is a fact about this test.
func readBacklogFrom(t *testing.T, path string) []backlogItem {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("the parser fixture is not at %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	return parseBacklog(t, f)
}

// parseBacklog is the parse itself, with the reading of the file taken off it.
//
// ⛔ ONE PARSE, TWO CALLERS, AND THAT IS THE WHOLE POINT OF THE SPLIT. The
// acceptance demonstration must read rig's real document and the pin must read
// a fixed one, and two parsers of one table shape is the drift this package
// spends its time correcting elsewhere.
func parseBacklog(t *testing.T, f io.Reader) []backlogItem {
	t.Helper()
	raw, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	var out []backlogItem
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		m := backlogRow.FindStringSubmatch(line)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		c := cells(line)
		if len(c) < 6 {
			t.Fatalf("%s has %d cells, want at least 6 (| # | Item | Evidence | Adopter | State |): %.120s",
				m[1], len(c), line)
		}

		// ⛔ ONE ROW IS MISSING THE PIPE AFTER ITS ID, SO ITS ID CELL ABSORBED
		// THE FRONT OF ITS ITEM CELL - INCLUDING THE CLOSURE MARKER.
		//
		// B21, and it is why the second instrument exists. Reconstructing the
		// item cell is the honest repair: the row is real, it is closed, and
		// every count anyone ran missed it because every one required the pipe.
		// It is FLAGGED rather than silently normalised - section 39's ruling
		// on the migration is import everything and report what is irregular,
		// and a row this test quietly tidied would be a row nobody ever fixes.
		item, malformed := c[2], false
		if rest := strings.TrimSpace(strings.TrimPrefix(c[1], m[1])); rest != "" {
			item, malformed = rest+" "+item, true
		}

		out = append(out, backlogItem{
			ID:    m[1],
			Title: titleOf(c[2]),
			// The document's own closure mark, in the item cell: a struck
			// title, or a terminal bold lead where the strikethrough would be.
			Struck:    strings.HasPrefix(item, "~~"),
			Done:      strings.HasPrefix(item, "~~") || terminalDispositions[firstWord(boldLead(item))],
			Malformed: malformed,
		})
		out[len(out)-1].ClaimsDone = !out[len(out)-1].Done &&
			terminalDispositions[firstWord(boldLead(c[5]))]
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	// ⛔ THE GUARD IS AN EQUALITY BETWEEN TWO INSTRUMENTS, NOT A BOUND.
	//
	// This was `if len(items) < 30 { t.Fatalf("... expected the real file's
	// ~44", len(items)) }`. Over a 44-row file it read 36, and 36 >= 30, so it
	// passed and the message naming 44 never printed. A BOUND CANNOT NOTICE
	// ITSELF GOING STALE - it is the same defect as a check that cannot tell
	// "nothing is wrong" from "the check did not run", and this repo has now
	// recorded seven of those.
	//
	// So the file is scanned a second time by a pattern that shares no code
	// path with the scanner above - a multiline match over the whole contents,
	// no bufio, no buffer bound, no per-line loop - and the two must agree AS
	// SETS. A count equality would still hide a swap; the set difference names
	// the rows that went missing, which is the failure that actually happened.
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
		t.Fatalf("the row scanner read %d rows and the whole-file scan finds %d; it never saw %v",
			len(out), len(out)+len(missing), missing)
	}
	return out
}

// ⛔ THE MVP ACCEPTANCE DEMONSTRATION. B46b.
//
// rig's own backlog, in rig, answered by the brief - and EVERY LINE OF THE
// OUTPUT IS DERIVED. Section 39's slice-2 sentence is that a work item is
// driven start to finish and the report is read off the brief "with NO SEAT
// HAVING WRITTEN A SENTENCE OF PROSE", and that is the thing under test here
// rather than any single function.
func TestRigsOwnBacklogIsManagedInRigAndTheBriefAnswersIt(t *testing.T) {
	items := readBacklog(t)

	classify(t, items)

	name := estate(t, "development")
	s := openStore(t, name)

	// The project record. Its id is the SLUG, section 39's id-scheme exception.
	if _, err := s.Put(tctx, PutRequest{
		ID: "rig", Kind: "project", Project: "rig",
		Body:    "rig itself, the only project there is",
		Fields:  map[string]string{"title": "rig", "status": "active", "next_up_n": "5"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	}); err != nil {
		t.Fatal(err)
	}

	open := 0
	for _, it := range items {
		if _, err := s.Put(tctx, PutRequest{
			ID: it.ID, Kind: "work-item", Project: "rig", Body: it.Title,
			Fields:  map[string]string{"title": it.Title, "status": "active"},
			Session: "record", Seat: "backend-record", Epoch: 6,
		}); err != nil {
			t.Fatalf("seeding %s: %v", it.ID, err)
		}
		// THE CLOSED ONES GET THEIR TERMINAL STEP. Several of these closed
		// today; showing them open would be false on the first read.
		if it.Done {
			step(t, s, it.ID, "done")
			continue
		}
		step(t, s, it.ID, "started")
		open++
	}

	// REAL blocks EDGES, each defensible from the document's own text.
	edges := [][2]string{
		// B45: plan/22's multi-dependency rows are WHY depscheck stops
		// comparing, which is B2. The rows must be split before the report
		// can be trusted.
		{"B45", "B2"},
		// Row 9 of the critical path: section 39 is "Blocked on B28" for
		// slice 1 - which store backs the record.
		{"B28", "B29"},
	}
	for _, e := range edges {
		if err := s.Link(tctx, e[0], LinkBlocks, e[1]); err != nil {
			t.Fatalf("linking %s blocks %s: %v", e[0], e[1], err)
		}
	}

	b, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Cycles) != 0 {
		t.Fatalf("rig's real backlog reports a cycle it does not have: %v", b.Cycles)
	}
	if len(b.NextUp) != 5 {
		t.Fatalf("next up has %d, want the project's next_up_n of 5", len(b.NextUp))
	}
	if len(b.NextUp)+len(b.Open) != open {
		t.Fatalf("the brief carries %d items, want the %d that are not done", len(b.NextUp)+len(b.Open), open)
	}
	if len(b.Blocked) != 2 {
		t.Fatalf("blocked list is %+v, want the two real edges", b.Blocked)
	}
	// A BLOCKER PRECEDES WHAT IT BLOCKS. The real assertion, on real edges.
	all := briefIDs(b)
	if pos(all, "B45") > pos(all, "B2") {
		t.Fatalf("B45 does not precede B2 in execution order: %v", all)
	}
	if pos(all, "B28") > pos(all, "B29") {
		t.Fatalf("B28 does not precede B29: %v", all)
	}

	report(t, "rig's own backlog, as rig answers it", b, open, len(items))

	// ⛔ NOW THE CYCLE, ASSERTED ON TWO REAL ITEMS, AND WATCHED FIRING.
	//
	// rig's backlog has NO naturally occurring blocks cycle - checked, and the
	// "blocks cycle is reported" in rig 15d9846 is a SPECIFICATION finding
	// about the derivation having no cycle handling, not a report of one in
	// this backlog. So the edge that closes the loop is ASSERTED, and it is
	// asserted between two items that really are entangled: B2 is that
	// deps-check reports success without comparing, B45 is that the rows are
	// why. Each can be argued to need the other first.
	if err := s.Link(tctx, "B2", LinkBlocks, "B45"); err != nil {
		t.Fatal(err)
	}
	cyc, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(cyc.Cycles) != 1 {
		t.Fatalf("the asserted cycle did not fire: %v", cyc.Cycles)
	}
	want := []string{"B2", "B45"}
	if len(cyc.Cycles[0]) != 2 || cyc.Cycles[0][0] != want[0] || cyc.Cycles[0][1] != want[1] {
		t.Fatalf("the cycle names %v, want %v", cyc.Cycles[0], want)
	}
	// AND IT STILL ANSWERS: every item is still carried, nothing dropped.
	if len(cyc.NextUp)+len(cyc.Open) != open {
		t.Fatalf("the cycle cost the brief %d items", open-len(cyc.NextUp)-len(cyc.Open))
	}
	// AND NOTHING WAS RESOLVED: both edges survive.
	for _, e := range [][2]string{{"B45", "B2"}, {"B2", "B45"}} {
		out, err := s.LinksFrom(tctx, e[0], LinkBlocks)
		if err != nil {
			t.Fatal(err)
		}
		if !contains(out, e[1]) {
			t.Fatalf("rig broke %s -> %s to resolve the cycle", e[0], e[1])
		}
	}
	report(t, "the same backlog with one asserted edge closing a loop", cyc, open, len(items))

	// ⛔ AND THE HALF NOBODY RUNS: WATCH THE REPORT CLEAR.
	//
	// A cycle report that fires is half the evidence. Two assertions in this
	// package have already passed for a reason other than the one they named,
	// both caught by mutation, and "the report fired" is that same shape if
	// nobody watches it stop.
	if err := s.Unlink(tctx, "B2", LinkBlocks, "B45"); err != nil {
		t.Fatal(err)
	}
	back, err := s.Brief(tctx, "rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Cycles) != 0 {
		t.Fatalf("the cycle report did not clear after the edge was removed: %v", back.Cycles)
	}
	if pos(briefIDs(back), "B45") > pos(briefIDs(back), "B2") {
		t.Fatal("execution order did not restore after the cycle cleared")
	}
}

// classify prints how every row was read, and NAMES the rows this test
// deliberately declines to decide about.
//
// ⛔ THE SPLIT IS THE POINT. A row struck through is closed - the document says
// so itself, in B7's own state cell: "done, struck not deleted". A row whose
// STATE cell leads with a terminal disposition while its title is NOT struck is
// a different thing, and whether it is closed is a question about the document
// rather than about this code. So it is counted and named, never folded in.
// Deciding it here would be this test writing the backlog's semantics into Go,
// where nobody reviews it as a decision.
func classify(t *testing.T, items []backlogItem) {
	t.Helper()
	var done, struck, byLead, claims, malformed []string
	for _, it := range items {
		switch {
		case it.Done:
			done = append(done, it.ID)
			if it.Struck {
				struck = append(struck, it.ID)
			} else {
				byLead = append(byLead, it.ID)
			}
		case it.ClaimsDone:
			claims = append(claims, it.ID)
		}
		if it.Malformed {
			malformed = append(malformed, it.ID)
		}
	}
	fmt.Printf("\n  === how rig's backlog was read ===\n")
	// ⛔ THE CAPTION NAMES THE RULE IT ACTUALLY RAN. Done has TWO clauses and
	// this line said "title struck", which is one of them - so a reader
	// checking it by hand counts 8 where the program says 9, and both are
	// right. A label is a claim and it gets the same treatment as any other.
	fmt.Printf("  %d rows   %d closed (%d struck, %d by a terminal lead in the item cell)   %d open\n",
		len(items), len(done), len(struck), len(byLead), len(items)-len(done))
	fmt.Printf("  closed, struck:          %v\n", struck)
	fmt.Printf("  closed, terminal lead:   %v\n", byLead)
	fmt.Printf("  ⚠ %d claim a terminal state without being struck, SEEDED OPEN: %v\n",
		len(claims), claims)
	fmt.Printf("  ⚠ %d malformed row(s), parsed and seeded anyway: %v\n\n",
		len(malformed), malformed)
}

func report(t *testing.T, headline string, b Brief, open, seeded int) {
	t.Helper()
	fmt.Printf("\n  === %s ===\n", headline)
	fmt.Printf("  project %s   %d open of %d seeded   next_up_n=%d\n",
		b.Project, open, seeded, len(b.NextUp))
	fmt.Printf("  NEXT UP (execution order)\n")
	for i, it := range b.NextUp {
		fmt.Printf("    %d. %-5s %-8s %s\n", i+1, it.ID, "["+it.State+"]", trunc(it.Title, 58))
	}
	fmt.Printf("  BLOCKED (%d)\n", len(b.Blocked))
	for _, bl := range b.Blocked {
		fmt.Printf("    %-5s waits on %v\n", bl.Item, bl.BlockedBy)
	}
	if len(b.Cycles) == 0 {
		fmt.Printf("  CYCLES   none\n")
	} else {
		for _, c := range b.Cycles {
			fmt.Printf("  ⛔ CYCLE  %v - reported, ordered around, NOT resolved\n", c)
		}
	}
	fmt.Printf("  coarse citations %d   (0 until the migration runs)\n", b.CoarseCitations)
	fmt.Printf("  and %d more open items beyond next-up\n\n", len(b.Open))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func pos(ids []string, id string) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
