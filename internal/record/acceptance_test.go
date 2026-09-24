package record

import (
	"sort"
	"strings"
	"testing"
)

// What is left in this file after plan/50 move 6, and what left with the
// importers.
//
// ⛔ SECTION 39's MVP ACCEPTANCE DEMONSTRATION (B46b) IS NO LONGER IN rig AND
// NOTHING HERE CAN REBUILD IT. It read the real BACKLOG.md through
// `ParseBacklog`, seeded a store from those rows and printed what `Store.Brief`
// derived. Move 6 deletes the importers and move 8 retires the brief, so BOTH
// ends of the demonstration are the planner's now - the document it reads and
// the derivation it checks both live in the docket program. The `mvp-demo`
// target still names the deleted test by -run; the Makefile is not this seat's
// file, so that is reported to the lead rather than edited here.
//
// ⛔ WHAT THAT COSTS, STATED SO IT IS NOT DISCOVERED LATER: rig no longer has a
// command whose green is evidence the MVP demonstration passed. B46e's point
// was that a skip and a pass are the same colour to whatever reads a gate; the
// demonstration is now owed by docket, which carries the parser (move 3) and
// will carry the brief (move 4).
//
// What left with it, every one of them typed on BacklogItem or on Brief:
// backlogPath, requireBacklog (RIG_RECORD_REQUIRE_BACKLOG), readBacklog,
// readBacklogFrom, parseBacklog, classify, report, trunc. Docket already
// carries the parsing half as its own internal/importers test.
//
// What stayed: pos and contains are plain slice helpers that itemstate_test.go
// calls, and nextUpOrderLabel is a caption rule with its own test below.

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

// nextUpOrderLabel says what the printed order ACTUALLY IS rather than what the
// derivation is capable of.
//
// ⛔ THIS CAPTION WAS `make mvp-demo`'s OUTPUT - the artefact whose whole job
// was to prove section 39 works - AND IT CARRIED A FALSE CAPTION. It printed
// "(execution order)" unconditionally. `sortByPriority` IS a topological sort
// over `blocks` and it is correct; the store holds ZERO `blocks` edges and ZERO
// priorities, so it degenerates to `sort.Strings` and the demonstration
// asserted an ordering it had not performed. Measured 2026-09-17 against live
// production: 0 of 68 records carry a priority.
//
// The fourth of four sites, found by the seat that fixed the other three and
// handed this one back because it is not in `cmd/rig`. The other three were at
// `cmd/rig/brief.go` and left with the renderer at move 5; the daemon's note
// that NO GATE IN THIS REPOSITORY CHECKS A CAPTION goes with the brief arm at
// move 8. This is the last of the four still standing, which is why the rule
// is kept here with its test even though nothing in rig prints it any more.
// Every one of the four was found by a person reading rather than by a test.
//
// Derived, never asserted, so it retires itself the day an ordering exists: it
// compares the order it was handed against those same ids sorted, and reports
// what it sees.
func nextUpOrderLabel(items []ItemState) string {
	if len(items) < 2 {
		return "(order not observable below two items)"
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	byID := append([]string(nil), ids...)
	sort.Strings(byID)
	for i := range ids {
		if ids[i] != byID[i] {
			return "(execution order)"
		}
	}
	return "(id order - nothing here carries a priority or a blocks edge)"
}

// The caption must go to "id order" exactly when the order it was handed is
// the ids sorted, and to "execution order" only when the derivation actually
// moved something. Mutation-tested: inverting the comparison makes both arms
// report the other label and each assertion below names its own case.
func TestNextUpOrderLabelSaysWhatTheOrderIs(t *testing.T) {
	sorted := []ItemState{{ID: "B1"}, {ID: "B10"}, {ID: "B12"}}
	if got := nextUpOrderLabel(sorted); !strings.Contains(got, "id order") {
		t.Errorf("ids already in sorted order must NOT be captioned as execution order, got %q", got)
	}
	moved := []ItemState{{ID: "B45"}, {ID: "B2"}, {ID: "B7"}}
	if got := nextUpOrderLabel(moved); got != "(execution order)" {
		t.Errorf("an order the derivation actually changed IS execution order, got %q", got)
	}
	// Below two items there is nothing to observe, and claiming either label
	// would be a claim the data cannot support.
	if got := nextUpOrderLabel([]ItemState{{ID: "B1"}}); !strings.Contains(got, "not observable") {
		t.Errorf("one item cannot evidence an ordering, got %q", got)
	}
}
