package record

import (
	"strings"
	"testing"
)

// ⛔ EVERY SEAT NAME IN THIS PROJECT IS HYPHENATED, AND THIS TEST EXISTS
// BECAUSE THE FIRST VERSION CUT AT THE HYPHEN.
//
// `team-lead` came out as `team` and `backend-record` as `backend` - 52 of
// rig's 106 work items filed under seats that do not exist - and it was found
// by seeding a throwaway store and LOOKING at the census, not by the suite.
// Boris asked for rows "grouped or at least tagged so the user can see what
// relates to what"; grouping 52 rows under a name nobody answers to is worse
// than not grouping them, because it looks like it worked.
func TestAnOwnerCellYieldsANameOrNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		cell string
		want string
	}{
		{"a plain seat", "team-lead", "team-lead"},
		{"⛔ the hyphen is part of the name", "backend-record", "backend-record"},
		{"bold", "**backend-1**", "backend-1"},
		{"backticked", "`rig-record`", "rig-record"},
		{"bold, then the cell explains itself", "**Boris**, who raised it", "Boris"},
		{"a spaced dash IS a separator", "the lead - `plan/` is the lead's", "the lead"},
		{"a parenthetical", "backend-1 (proto + daemon), lead sequences", "backend-1"},

		// ⛔ ABSENT RATHER THAN WRONG. A wrong owner sends a reader to the
		// wrong seat and looks authoritative doing it.
		{"a placeholder is not an owner", "-", ""},
		{"empty", "   ", ""},
		{"a cell that explains instead of naming", "**THE CLIENT** - this cell said `backend-2` and that name now resolves to a DIFFERENT ROLE, so a seat matching its own name takes another role's work", "THE CLIENT"},
		// ⛔ A SHORT PHRASE IS KEPT EVEN WHEN IT IS NOT A SEAT NAME, and that
		// is the cap working rather than failing. "nobody executes this" is 20
		// characters, reads correctly in a column headed OWNER, and groups the
		// rows nobody owns with each other - which is what the field was asked
		// for. The cap rejects SENTENCES, not phrases. This expectation was
		// written the other way first and the CODE was right.
		{"a short phrase is an honest owner", "nobody executes this - it is TRACKED, not owned", "nobody executes this"},
		{"a sentence is not", "every seat that has ever touched this file, and the lead sequences them", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownerOf(tc.cell); got != tc.want {
				t.Errorf("ownerOf(%q) = %q, want %q", tc.cell, got, tc.want)
			}
		})
	}
}

// ⛔ THE STATE CELL IS THE ONLY PLACE A READER LEARNS WHAT A ROW IS ABOUT, AND
// IT WAS PARSED AND THROWN AWAY.
//
// `Done`, `ClaimsDone` and `Disposition` are all read OUT of this cell, so the
// parser opened it every time and kept three booleans. Measured on the live
// production store 2026-09-17: B62's `title`, `description_short` and `body`
// were the SAME 161-character string, which is exactly why clicking a row in
// the window showed nothing new.
func TestARowCarriesItsStateProseAndItsOwner(t *testing.T) {
	const doc = "" +
		"| # | Work | Seat | State |\n" +
		"|---|---|---|---|\n" +
		"| **B90** | **A SHORT TITLE** | team-lead | ⛔ **OPEN.** The long explanation nobody could read, because it reached no store. |\n" +
		"| **B91** | **ANOTHER** | - | |\n"

	p, err := ParseBacklogDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 {
		t.Fatalf("the fixture parsed %d rows, want 2", len(p.Items))
	}

	got := p.Items[0]
	if got.Owner != "team-lead" {
		t.Errorf("owner is %q, want team-lead", got.Owner)
	}
	if got.State == "" {
		t.Fatal("the state cell reached no field, so the row's only " +
			"explanation is still thrown away")
	}
	if got.State == got.Title {
		t.Errorf("the state prose is the title again: %q", got.State)
	}
	if len(got.State) <= len(got.Title) {
		t.Errorf("the state prose (%d chars) is not longer than the title "+
			"(%d), so nothing was actually captured", len(got.State), len(got.Title))
	}

	// An empty state cell and an unowned row are both real shapes here.
	if p.Items[1].State != "" {
		t.Errorf("an empty state cell yielded %q", p.Items[1].State)
	}
	if p.Items[1].Owner != "" {
		t.Errorf("the placeholder owner yielded %q", p.Items[1].Owner)
	}
}
