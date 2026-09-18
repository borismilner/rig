// ⛔ THE PARSER MUST FILL `Section` FROM THE DOCUMENT, AND A TEST BUILT ON A
// HAND-WRITTEN `BacklogItem` CANNOT SEE WHETHER IT DOES. `cmd/rigseed`'s
// guard builds its items literally, so deleting the parser's assignment left
// it green - a mutation that survived. This file reads a real document.
package record

import (
	"strings"
	"testing"
)

// Three shapes, and the third is the one that matters: a row under a heading
// that states no id, which is where `Section` and `Under` come apart.
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
	// ⛔ BOTH FIELDS PER ROW. Asserting only the slug left a mutation alive -
	// deleting the parser's `SectionTitle` assignment stayed green, because
	// every other test builds its items by hand.
	want := map[string]struct{ slug, title string }{
		"B1": {"open", "Open"},
		"B3": {"b2-a-heading-that-carries-an-id", "B2 - A HEADING THAT CARRIES AN ID"},
		"B4": {
			"rejected-kept-so-the-argument-is-not-had-twice",
			"Rejected, kept so the argument is not had twice",
		},
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
		if it.Section != w.slug {
			t.Errorf("%s sits under %q, want %q", it.ID, it.Section, w.slug)
		}
		if it.SectionTitle != w.title {
			t.Errorf("%s SectionTitle = %q, want %q", it.ID, it.SectionTitle, w.title)
		}
	}
}

// ⛔ THE NEAREST HEADING, NOT THE NEAREST ID-BEARING ONE. `Under` is the
// id-bearing one; `Section` never skips. A row under an id-less heading is
// where the difference is observable at all.
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
	if got, want := items[0].SectionTitle, "The critical path, which states no id"; got != want {
		t.Errorf("SectionTitle is %q, want %q", got, want)
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
	if items[0].SectionTitle != "" {
		t.Errorf("SectionTitle is %q, want empty", items[0].SectionTitle)
	}
}
