package record_test

import (
	"os"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// theDecisionsDocument is rig's own DECISIONS.md, reached through the
// repo-root symlink for backlogPath's reason: the symlink is gitignored, so
// the test skips wherever the logbook is not checked out beside rig - a fresh
// clone, a detached gate worktree, CI. RIG_RECORD_REQUIRE_DECISIONS=1 turns
// that skip into a failure, because a pin that silently skips is a pin that
// stopped being one. B46e.
const theDecisionsDocument = "../../DECISIONS.md"

func openTheDecisionsDocument(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(theDecisionsDocument)
	if err != nil {
		if os.Getenv("RIG_RECORD_REQUIRE_DECISIONS") == "1" {
			t.Fatalf("RIG_RECORD_REQUIRE_DECISIONS=1 and the document is unreadable: %v", err)
		}
		t.Skipf("the real document is not here: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestASectionIsNotADecisionAndBothAreCarried(t *testing.T) {
	const doc = `# rig - decisions

## What rig is

rig is a thing.

## Naming

Names matter.

## 2026-09-10 - the first ruling

It was ruled.

### 1. the sub-ruling

A sub-ruling.

## Decided: a later thing with no date

Decided.
`
	p, err := record.ParseDecisionsDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	byKey := map[string]record.DecisionEntry{}
	for _, d := range p.Decisions {
		byKey[d.Key] = d
	}
	if len(byKey) != len(p.Decisions) {
		t.Fatalf("keys are not unique: %d entries, %d keys", len(p.Decisions), len(byKey))
	}

	want := map[string]record.EntryKind{
		"what-rig-is":                 record.EntrySection,
		"naming":                      record.EntrySection,
		"2026-09-10-the-first-ruling": record.EntryDecision,
		"2026-09-10-the-first-ruling/1-the-sub-ruling": record.EntryDecision,
		"decided-a-later-thing-with-no-date":           record.EntryDecision,
	}
	if len(p.Decisions) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(p.Decisions), len(want), keysOf(p.Decisions))
	}
	for k, kind := range want {
		got, ok := byKey[k]
		if !ok {
			t.Fatalf("key %q is missing; got %v", k, keysOf(p.Decisions))
		}
		if got.Kind != kind {
			t.Errorf("%q: kind %q, want %q", k, got.Kind, kind)
		}
	}

	// ⛔ THE DATE IS READ, NOT GUESSED. An undated heading carries no date and
	// must not borrow one from the heading above it.
	if d := byKey["2026-09-10-the-first-ruling"]; d.Date != "2026-09-10" {
		t.Errorf("dated heading: Date %q, want 2026-09-10", d.Date)
	}
	if d := byKey["decided-a-later-thing-with-no-date"]; d.Date != "" {
		t.Errorf("undated heading: Date %q, want empty", d.Date)
	}
}

func TestASubHeadingIsPartOfItsParentAndNotOfTheOneBefore(t *testing.T) {
	const doc = `# d

## 2026-09-10 - alpha

### The ruling

one

## 2026-09-11 - beta

### The ruling

two
`
	p, err := record.ParseDecisionsDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// ⛔ "### The ruling" APPEARS THREE TIMES IN THE REAL DOCUMENT, so a key
	// that is the child's own text alone collides and one of them is lost with
	// no error. The parent qualifies it.
	got := map[string]string{}
	for _, d := range p.Decisions {
		got[d.Key] = d.PartOf
	}
	for k, parent := range map[string]string{
		"2026-09-10-alpha/the-ruling": "2026-09-10-alpha",
		"2026-09-11-beta/the-ruling":  "2026-09-11-beta",
	} {
		if got[k] != parent {
			t.Errorf("%q: PartOf %q, want %q (all: %v)", k, got[k], parent, got)
		}
	}
	if len(p.Decisions) != 4 {
		t.Errorf("got %d entries, want 4: %v", len(p.Decisions), keysOf(p.Decisions))
	}
}

func TestTheBodyStopsAtTheNextHeadingAndNotAtTheNextBlankLine(t *testing.T) {
	const doc = `# d

## 2026-09-10 - alpha

first paragraph

second paragraph

## 2026-09-11 - beta

other
`
	p, err := record.ParseDecisionsDocument(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, d := range p.Decisions {
		if d.Key != "2026-09-10-alpha" {
			continue
		}
		if !strings.Contains(d.Body, "first paragraph") || !strings.Contains(d.Body, "second paragraph") {
			t.Errorf("body lost a paragraph: %q", d.Body)
		}
		if strings.Contains(d.Body, "other") {
			t.Errorf("body ran into the next heading: %q", d.Body)
		}
		return
	}
	t.Fatalf("alpha is missing: %v", keysOf(p.Decisions))
}

func TestEveryKeyInTheRealDocumentIsUnique(t *testing.T) {
	f := openTheDecisionsDocument(t)
	p, err := record.ParseDecisionsDocument(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.Decisions) == 0 {
		t.Fatalf("the real document parsed to nothing")
	}

	// ⛔ THE KEY IS THE WHOLE INSTRUMENT. Set equality against the store is
	// computed on it, so a collision is not an inconvenience: it is two
	// document facts sharing one record, silently, forever.
	at := map[string]int{}
	for _, d := range p.Decisions {
		if prev, ok := at[d.Key]; ok {
			t.Errorf("key %q is at line %d and line %d", d.Key, prev, d.Line)
		}
		at[d.Key] = d.Line
	}

	for _, d := range p.Decisions {
		if d.Title == "" {
			t.Errorf("line %d: an entry with no title", d.Line)
		}
		if d.PartOf == "" {
			continue
		}
		if _, ok := at[d.PartOf]; !ok {
			t.Errorf("line %d: %q is part-of %q, which is not an entry", d.Line, d.Key, d.PartOf)
		}
	}
}

func keysOf(ds []record.DecisionEntry) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Key)
	}
	return out
}
