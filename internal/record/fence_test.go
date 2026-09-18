// The fence rule, tested at the three callers that gained it.
//
// ⛔ EVERY TEST HERE CARRIES ITS POSITIVE CONTROL, because a parser that reads
// no heading at all passes the fenced case for the wrong reason. The control
// is the same document with the fence markers removed: the heading must then
// be read, or the test below is measuring the shape of the line rather than
// the fence.
package record

import (
	"strings"
	"testing"
)

const fencedBacklog = "# Backlog\n\n" +
	"## Open\n\n" +
	"| # | Item | Evidence | Adopter | State |\n" +
	"|---|---|---|---|---|\n" +
	"| B1 | the first row | - | team-lead | open |\n\n" +
	"```sh\n" +
	"# mutate and test in there; the shared tree is never touched\n" +
	"## ⛔ B99 - AND THIS IS NOT A HEADING\n" +
	"```\n\n" +
	"## After the fence\n\n" +
	"| # | Item | Evidence | Adopter | State |\n" +
	"|---|---|---|---|---|\n" +
	"| B2 | a row after the block | - | team-lead | open |\n"

func TestTheBacklogDoesNotReadAHeadingInsideAFence(t *testing.T) {
	p, err := ParseBacklogDocument(strings.NewReader(fencedBacklog))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	for _, u := range p.Unimported {
		if u.ID == "B99" || strings.Contains(u.Label, "B99") {
			t.Errorf("a shell comment inside a ```sh block was read as the heading %q", u.Label)
		}
	}
	// The rows on both sides of the block are still read, so the fence closed
	// and the parse did not simply stop at it.
	var ids []string
	for _, it := range p.Items {
		ids = append(ids, it.ID)
	}
	if got := strings.Join(ids, " "); got != "B1 B2" {
		t.Errorf("items = {%s}; want B1 B2 - the fence swallowed the rest of the document", got)
	}

	// POSITIVE CONTROL: the same text with no fence around it IS a heading.
	loose := strings.ReplaceAll(strings.ReplaceAll(fencedBacklog, "```sh\n", ""), "```\n", "")
	lp, err := ParseBacklogDocument(strings.NewReader(loose))
	if err != nil {
		t.Fatalf("parsing the control: %v", err)
	}
	var found bool
	for _, u := range lp.Unimported {
		if u.ID == "B99" {
			found = true
		}
	}
	if !found {
		t.Error("the control did not read an UNFENCED id-bearing heading, so the test " +
			"above proves nothing about fences")
	}
}

func TestTheBacklogRefusesAnUnclosedFence(t *testing.T) {
	const doc = "# Backlog\n\n## Open\n\n```sh\n# it opens and never closes\n\n## B7 - LOST\n"
	if _, err := ParseBacklogDocument(strings.NewReader(doc)); err == nil {
		t.Fatal("a document with an unclosed fence parsed as though it were complete")
	}
}

const fencedDecisions = "# rig - decisions\n\n" +
	"## What rig is\n\n" +
	"rig is a thing.\n\n" +
	"## 2026-09-10 - the first ruling\n\n" +
	"It was ruled, and the ruling quotes a shell session:\n\n" +
	"```sh\n" +
	"# 2026-09-11 - a shell comment, not a ruling\n" +
	"rig record put\n" +
	"## 2026-09-11 - and neither is this\n" +
	"```\n\n" +
	"## 2026-09-12 - the second ruling\n\n" +
	"It was ruled again.\n"

func TestTheDecisionsParseDoesNotReadAHeadingInsideAFence(t *testing.T) {
	p, err := ParseDecisionsDocument(strings.NewReader(fencedDecisions))
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	var titles []string
	for _, d := range p.Decisions {
		titles = append(titles, d.Title)
	}
	// One section and two rulings; the document's own H1 is not an entry.
	if len(titles) != 3 {
		t.Errorf("entries = %v; want the section and the two real rulings only", titles)
	}
	for _, ti := range titles {
		if strings.Contains(ti, "neither is this") {
			t.Errorf("a `#` line inside a ```sh block became the entry %q", ti)
		}
	}
	// The fenced lines are still the first ruling's body, which is what makes
	// this a fence rule rather than a drop.
	var body string
	for _, d := range p.Decisions {
		if strings.Contains(d.Title, "first") {
			body = d.Body
		}
	}
	if !strings.Contains(body, "rig record put") {
		t.Errorf("the fenced block left the ruling's body: %q", body)
	}

	// POSITIVE CONTROL: unfenced, the same line IS a heading.
	loose := strings.ReplaceAll(strings.ReplaceAll(fencedDecisions, "```sh\n", ""), "```\n", "")
	lp, err := ParseDecisionsDocument(strings.NewReader(loose))
	if err != nil {
		t.Fatalf("parsing the control: %v", err)
	}
	if len(lp.Decisions) != 4 {
		t.Errorf("the control read %d entries; want 4, so the test above is measuring "+
			"the fence and not the shape of the line", len(lp.Decisions))
	}
}

func TestTheDecisionsParseRefusesAnUnclosedFence(t *testing.T) {
	const doc = "# rig - decisions\n\n## 2026-09-10 - a ruling\n\n```sh\n# never closed\n"
	if _, err := ParseDecisionsDocument(strings.NewReader(doc)); err == nil {
		t.Fatal("a document with an unclosed fence parsed as though it were complete")
	}
}

// ⛔ THE THREE-WAY ANSWER IS THE REASON THE MACHINE IS SHARED RATHER THAN
// COPIED, so it is asserted directly: the callers differ on the MARKER line and
// agree on everything else.
func TestTheFenceMachineAnswersMarkerAndCodeSeparately(t *testing.T) {
	var f fenceScan
	for _, c := range []struct {
		line   string
		marker bool
		code   bool
	}{
		{"prose", false, false},
		{"```go", true, true},
		{"# a comment", false, true},
		{"``", false, true}, // too short to be a fence at all
		{"```", true, true}, // closes it
		{"# a heading", false, false},
	} {
		marker, code := f.read(c.line)
		if marker != c.marker || code != c.code {
			t.Errorf("read(%q) = (%v, %v); want (%v, %v)", c.line, marker, code, c.marker, c.code)
		}
	}
	if got := f.unclosed(); got != "" {
		t.Errorf("unclosed = %q after a closed block", got)
	}
}

func TestAShorterFenceDoesNotCloseALongerOne(t *testing.T) {
	var f fenceScan
	f.read("````go")
	if marker, code := f.read("```"); marker || !code {
		t.Errorf("a ``` line closed a ```` block: marker=%v code=%v", marker, code)
	}
	if f.unclosed() != "````" {
		t.Errorf("unclosed = %q; the block is still open", f.unclosed())
	}
}

// ⛔ THE MARKER LINES ARE DROPPED AND THE BLOCK'S CONTENTS ARE KEPT, and until
// this test was written nothing said so: a mutation that turned the fence
// markers into body prose left every other fence test green. A requirement's
// body is rendered to Boris, so a stray ``` in it is visible, and it is the
// only property of `planHeadings` that the shared machine's three-way answer
// exists to preserve.
func TestAPlanBodyKeepsTheFencedCodeAndDropsItsMarkers(t *testing.T) {
	const doc = "## Configuration\n\n```sh\nrig record put\n```\n"

	p, err := ParsePlanSection(strings.NewReader(doc), "plan/06-configuration.md", 6)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(p.Entries) != 1 {
		t.Fatalf("entries = %d; want the one heading", len(p.Entries))
	}
	body := p.Entries[0].Body
	if !strings.Contains(body, "rig record put") {
		t.Errorf("body = %q; the fenced block's contents were dropped with its markers", body)
	}
	if strings.Contains(body, "```") {
		t.Errorf("body = %q; the fence markers reached the record", body)
	}
}
