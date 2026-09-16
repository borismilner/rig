package record

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// backlogPath is rig's REAL development plan. Not a fixture and not a copy.
//
// Section 39's MVP, Boris: "rig's own project and cases are managed IN rig.
// Not demonstrated on a fixture, not on a synthetic second project - on rig
// itself, which is the only project there is." A test that seeds invented rows
// demonstrates the code runs. Only the real document demonstrates the MVP.
//
// ⛔ IT REACHES THE FILE THROUGH THE REPO-ROOT SYMLINK, WHICH IS GITIGNORED, SO
// THIS TEST SKIPS WHEREVER THE LOGBOOK IS NOT CHECKED OUT BESIDE rig - a fresh
// clone, a detached gate worktree, CI. THAT IS DELIBERATE AND IT IS ALSO A
// LIMITATION WORTH STATING: `make ci` does NOT run this test, so a green gate
// is not evidence the MVP demonstration passes. Run it in the working tree.
// The alternative - committing a copy of the backlog as a fixture - fails the
// only requirement this test exists to meet.
const backlogPath = "../../BACKLOG.md"

// A row is "| B<n> | **<title>** | ...". The title is taken up to its closing
// bold or the next cell, whichever comes first - the rows are handwritten and
// not all of them close the bold before the pipe.
var backlogRow = regexp.MustCompile(`^\| (B\d+) \| \*\*(.+)$`)

func titleOf(rest string) string {
	for _, cut := range []string{"**", " | "} {
		if i := strings.Index(rest, cut); i > 0 {
			rest = rest[:i]
		}
	}
	return strings.TrimSpace(rest)
}

type backlogItem struct {
	ID    string
	Title string
	Done  bool
}

// readBacklog pulls the real rows out of the real file.
func readBacklog(t *testing.T) []backlogItem {
	t.Helper()
	f, err := os.Open(backlogPath)
	if err != nil {
		t.Skipf("rig's own backlog is not at %s from here: %v", backlogPath, err)
	}
	defer func() { _ = f.Close() }()

	var out []backlogItem
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		m := backlogRow.FindStringSubmatch(line)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		low := strings.ToLower(line)
		out = append(out, backlogItem{
			ID:    m[1],
			Title: titleOf(m[2]),
			// A row whose state cell says done, or whose evidence says BUILT or
			// DONE with a sha, is finished work. A seeded backlog that shows
			// today's closed items as open is a lie on its first read.
			Done: strings.Contains(low, "| **done") || strings.Contains(low, "**done ") ||
				strings.Contains(low, "**built,") || strings.Contains(low, "**rejected"),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
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
	if len(items) < 30 {
		t.Fatalf("read %d rows from rig's backlog, expected the real file's ~44", len(items))
	}

	name := estate(t, "development")
	s := openStore(t, name)

	// The project record. Its id is the SLUG, section 39's id-scheme exception.
	if _, err := s.Put(PutRequest{
		ID: "rig", Kind: "project", Project: "rig",
		Body:    "rig itself, the only project there is",
		Fields:  map[string]string{"title": "rig", "status": "active", "next_up_n": "5"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	}); err != nil {
		t.Fatal(err)
	}

	open := 0
	for _, it := range items {
		if _, err := s.Put(PutRequest{
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
	real := [][2]string{
		// B45: plan/22's multi-dependency rows are WHY depscheck stops
		// comparing, which is B2. The rows must be split before the report
		// can be trusted.
		{"B45", "B2"},
		// Row 9 of the critical path: section 39 is "Blocked on B28" for
		// slice 1 - which store backs the record.
		{"B28", "B29"},
	}
	for _, e := range real {
		if err := s.Link(e[0], LinkBlocks, e[1]); err != nil {
			t.Fatalf("linking %s blocks %s: %v", e[0], e[1], err)
		}
	}

	b, err := s.Brief("rig")
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

	report(t, "rig's own backlog, as rig answers it", b, open)

	// ⛔ NOW THE CYCLE, ASSERTED ON TWO REAL ITEMS, AND WATCHED FIRING.
	//
	// rig's backlog has NO naturally occurring blocks cycle - checked, and the
	// "blocks cycle is reported" in rig 15d9846 is a SPECIFICATION finding
	// about the derivation having no cycle handling, not a report of one in
	// this backlog. So the edge that closes the loop is ASSERTED, and it is
	// asserted between two items that really are entangled: B2 is that
	// deps-check reports success without comparing, B45 is that the rows are
	// why. Each can be argued to need the other first.
	if err := s.Link("B2", LinkBlocks, "B45"); err != nil {
		t.Fatal(err)
	}
	cyc, err := s.Brief("rig")
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
		out, err := s.LinksFrom(e[0], LinkBlocks)
		if err != nil {
			t.Fatal(err)
		}
		if !contains(out, e[1]) {
			t.Fatalf("rig broke %s -> %s to resolve the cycle", e[0], e[1])
		}
	}
	report(t, "the same backlog with one asserted edge closing a loop", cyc, open)

	// ⛔ AND THE HALF NOBODY RUNS: WATCH THE REPORT CLEAR.
	//
	// A cycle report that fires is half the evidence. Two assertions in this
	// package have already passed for a reason other than the one they named,
	// both caught by mutation, and "the report fired" is that same shape if
	// nobody watches it stop.
	if err := s.Unlink("B2", LinkBlocks, "B45"); err != nil {
		t.Fatal(err)
	}
	back, err := s.Brief("rig")
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

func report(t *testing.T, headline string, b Brief, open int) {
	t.Helper()
	fmt.Printf("\n  === %s ===\n", headline)
	fmt.Printf("  project %s   %d open of %d seeded   next_up_n=%d\n",
		b.Project, open, open, len(b.NextUp))
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
