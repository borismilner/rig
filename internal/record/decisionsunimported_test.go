package record_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// ⛔ `Unimported` IS SHARED BY TWO GRAINS, AND FOR ONE COMMIT HALF OF IT WAS
// BLIND TO A FIELD THE OTHER HALF ADDED.
//
// `Section` arrived with the backlog grain at `2de8bbd` and was populated on
// the four kinds that file reports. The three kinds `decisions.go` reports -
// duplicate-key, heading-too-deep, heading-no-title - carried the zero value,
// and the field's own doc comment said so. **A field populated on four kinds
// and empty on three is indistinguishable from one that was lost**, which is
// the exact failure this grain keeps paying for.
//
// ⛔ AND NOTHING IN rig's OWN `DECISIONS.md` REPORTS ANY OF THE THREE. The
// parse over the live document returns ZERO Unimported, so the gap could not
// have been caught by a live pin and this fixture is the only instrument there
// is. Said out loud rather than glossed: the assertions below are about a
// document written to exercise them.
const decisionsWithEveryUnimportedKind = `# rig - decisions, in the order they were made

## What rig is

the standing section, before the first dated heading.

## 2026-09-01 the first ruling

the ruling's own prose.

### The ruling

one child.

### The ruling

⛔ a SECOND child with the same text, so parent/child collides and the key is
already taken.

#### a fourth level this document does not write

⛔ heading-too-deep.

## ⛔

⛔ a heading that is nothing once the decoration is stripped.
`

func TestEveryThingTheDecisionsGrainReportsSaysWhereItSits(t *testing.T) {
	p, err := record.ParseDecisionsDocument(strings.NewReader(decisionsWithEveryUnimportedKind))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := map[record.UnimportedKind]record.Unimported{}
	for _, u := range p.Unimported {
		if _, dup := got[u.Kind]; dup {
			t.Fatalf("the fixture reports %s twice, so the map below hides one: %+v", u.Kind, p.Unimported)
		}
		got[u.Kind] = u
	}

	// The positive control, and it is per KIND. A green over a fixture that
	// stopped producing one of the three would say nothing at all, and all
	// three are absent from rig's own document.
	for _, k := range []record.UnimportedKind{
		record.UnimportedDuplicateKey,
		record.UnimportedHeadingTooDeep,
		record.UnimportedHeadingNoTitle,
	} {
		if _, ok := got[k]; !ok {
			t.Fatalf("the fixture produced no %s, so nothing of that kind was checked: %+v",
				k, p.Unimported)
		}
	}

	for _, want := range []struct {
		kind    record.UnimportedKind
		section string
	}{
		// The second `### The ruling` sits under the dated H2, not under
		// itself: a heading does not enclose itself, exactly as on the backlog
		// side.
		{record.UnimportedDuplicateKey, "2026-09-01-the-first-ruling"},
		// The level-4 heading sits under the H3 above it, which is a heading
		// this loop DECLINED to carry - so the stack has to track every
		// heading and not only the ones that become entries.
		{record.UnimportedHeadingTooDeep, "the-ruling"},
		// `## ⛔` sits under the document's own H1, which the loop also
		// declines.
		{record.UnimportedHeadingNoTitle, "rig-decisions-in-the-order-they-were-made"},
	} {
		if g := got[want.kind].Section; g != want.section {
			t.Errorf("%s has Section %q, want %q", want.kind, g, want.section)
		}
	}
}

// ⛔ `Section` IS NOT A SECOND `Under`, AND ON THIS GRAIN THE TWO COME APART ON
// EXACTLY THE HEADINGS THE ENTRY LOOP DECLINES TO CARRY.
//
// `Under` here is the enclosing TOP-LEVEL heading's key, which is the nearest
// heading that became an entry. `Section` is the nearest heading of ANY level,
// entry or not. A test asserting one of them would pass over a `Section`
// quietly widened into a copy of `Under`, which is the mutation that has to
// stay dead.
func TestTheDecisionsGrainsSectionAndUnderAreDifferentFacts(t *testing.T) {
	p, err := record.ParseDecisionsDocument(strings.NewReader(decisionsWithEveryUnimportedKind))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var differed int
	for _, u := range p.Unimported {
		if u.Section == "" {
			t.Errorf("%s %q (line %d) says nothing about where it sits", u.Kind, u.Label, u.Line)
		}
		if u.Section != u.Under {
			differed++
		}
	}
	// heading-too-deep sits under an H3 while Under names the H2, and
	// heading-no-title sits under the H1 while Under still names the H2. Two
	// of the three, and a Section that had become a copy of Under would report
	// none.
	if differed != 2 {
		t.Errorf("Section differs from Under on %d of the %d reported things, want 2 - "+
			"the two fields answer different questions and a copy would report 0",
			differed, len(p.Unimported))
	}
}

// The live document, so the fixture above is not the only thing the pin knows
// about. rig's own `DECISIONS.md` reports NOTHING today, and that is the
// assertion: the day it does, the set moves and somebody reads the diff.
func TestRigsOwnDecisionsDocumentReportsNothingUnimported(t *testing.T) {
	// Reuses decisions_test.go's opener and its RIG_RECORD_REQUIRE_DECISIONS
	// off switch rather than restating the path: two copies of one path is the
	// drift this package spends its time correcting.
	f := openTheDecisionsDocument(t)
	p, err := record.ParseDecisionsDocument(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The positive control: the parse has to have read the document at all.
	if len(p.Decisions) < 100 {
		t.Fatalf("the decisions parse returned %d entries, so the instrument is "+
			"broken and an empty Unimported proves nothing", len(p.Decisions))
	}
	if len(p.Unimported) != 0 {
		var lines []string
		for _, u := range p.Unimported {
			lines = append(lines, string(u.Kind)+" "+u.Label)
		}
		t.Errorf("rig's own decisions document now reports %d unimported things, and "+
			"every Section assertion in this file is a fixture's: %v\n"+
			"Read the diff and pin them here in a commit that says so.",
			len(p.Unimported), lines)
	}
}
