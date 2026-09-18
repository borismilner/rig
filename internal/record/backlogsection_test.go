// ⛔ THE PARSER MUST FILL `Section` FROM THE DOCUMENT, AND A TEST BUILT ON A
// HAND-WRITTEN `BacklogItem` CANNOT SEE WHETHER IT DOES.
//
// `cmd/rigseed`'s round-trip guard constructs its items literally, so removing
// `it.Section = s.heads.section()` from the parse left that guard GREEN - a
// mutation that survived, caught by counting which assertions each mutation
// turned red rather than how many mutations bit. This file is the half that
// reads a real document.
package record

import (
	"strings"
	"testing"
)

// ⛔ ONE FIXTURE, THREE SHAPES THAT DIFFER, AND THE THIRD IS THE ONE THAT
// MATTERS: a row under a heading that states no id at all. That is where
// `Section` and `Under` come apart, and it is the shape rig's own backlog has -
// eleven ordered rows under `## The critical path to the gate`.
const sectionFixture = `# Backlog

## Open

| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B1 | the first row | - | team-lead | open |

### ⛔ B2 - A HEADING THAT CARRIES AN ID

| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B3 | a row under an id-bearing heading | - | team-lead | open |

## Rejected, kept so the argument is not had twice

| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B4 | a row in a different place | - | team-lead | open |
`

func TestARowCarriesTheHeadingItSitsUnder(t *testing.T) {
	items, err := ParseBacklog(strings.NewReader(sectionFixture))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	want := map[string]string{
		"B1": "open",
		"B3": "b2-a-heading-that-carries-an-id",
		"B4": "rejected-kept-so-the-argument-is-not-had-twice",
	}
	if len(items) != len(want) {
		t.Fatalf("parsed %d rows, want %d", len(items), len(want))
	}
	for _, it := range items {
		w, ok := want[it.ID]
		if !ok {
			t.Errorf("%s is not a row this fixture states", it.ID)
			continue
		}
		if it.Section != w {
			t.Errorf("%s sits under %q, want %q", it.ID, it.Section, w)
		}
	}
}

// ⛔ THE NEAREST HEADING, NOT THE NEAREST ID-BEARING ONE, AND THE TWO ARE
// DIFFERENT FACTS. `Unimported.Under` is deliberately the id-bearing one
// because heading enclosure as a `part-of` edge produced twenty-four edges this
// document never states. `Section` never skips, and a row under a heading with
// no id is where that difference is observable at all.
func TestASectionIsTheNearestHeadingEvenWhenItStatesNoID(t *testing.T) {
	const fixture = `# Backlog

## ⛔ B9 - AN ID-BEARING HEADING

### The critical path, which states no id

| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B10 | a row two levels down | - | team-lead | open |
`
	items, err := ParseBacklog(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("parsed %d rows, want 1", len(items))
	}
	if got, want := items[0].Section, "the-critical-path-which-states-no-id"; got != want {
		t.Errorf("Section is %q, want %q - the nearest heading of ANY level", got, want)
	}
}

// A row the document places under no heading at all carries no section, and
// that is an honest empty rather than a stand-in.
func TestARowUnderNoHeadingCarriesNoSection(t *testing.T) {
	const fixture = `| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B11 | a row before any heading | - | team-lead | open |
`
	items, err := ParseBacklog(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("parsed %d rows, want 1", len(items))
	}
	if items[0].Section != "" {
		t.Errorf("Section is %q, want empty", items[0].Section)
	}
}
