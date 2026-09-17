package record

import (
	"os"
	"strings"
	"testing"
)

// TestMarkdownSectionReadsTheRealREADME is the case that matters: the project
// description is seeded FROM THIS DOCUMENT, so a parser that works on a fixture
// and not on README.md has closed nothing.
func TestMarkdownSectionReadsTheRealREADME(t *testing.T) {
	f, err := os.Open("../../README.md")
	if err != nil {
		t.Skipf("README.md not readable from here: %v", err)
	}
	defer f.Close()
	body, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}

	short, err := MarkdownSection(strings.NewReader(string(body)), "The idea, in one line")
	if err != nil {
		t.Fatalf("idea section: %v", err)
	}
	first := short
	if i := strings.Index(short, "\n\n"); i >= 0 {
		first = short[:i]
	}
	if first == "" {
		t.Fatal("README.md's one-line idea came back empty; the seeder would write nothing")
	}
	if strings.Contains(first, "<") {
		t.Errorf("markup reached the one-line idea: %q", first)
	}
	if strings.HasPrefix(first, "**") {
		t.Errorf("emphasis markers survived: %q", first)
	}
	if strings.Contains(first, "\n") {
		t.Errorf("the ONE-LINE idea is more than one line: %q", first)
	}

	long, err := MarkdownSection(strings.NewReader(string(body)), "What it is")
	if err != nil {
		t.Fatalf("what-it-is section: %v", err)
	}
	if long == "" {
		t.Fatal("README.md's 'What it is' came back empty")
	}
	// It must STOP at the next heading of the same level. "The idea, in one
	// line" follows it, and running the two together is the failure this
	// assertion exists for.
	if strings.Contains(long, first) {
		t.Errorf("the section ran past its own heading into the next one:\n%s", long)
	}
}

func TestMarkdownSectionStopsAndSkips(t *testing.T) {
	doc := "# Top\n\nignored\n\n" +
		"## Wanted\n\nfirst para\n\n<picture>\n<img alt=\"x\">\n</picture>\n\n" +
		"second para\n\n" +
		"### Deeper\n\nkept, a subsection belongs to its section\n\n" +
		"## Next\n\nnot this\n"

	got, err := MarkdownSection(strings.NewReader(doc), "Wanted")
	if err != nil {
		t.Fatalf("MarkdownSection: %v", err)
	}
	for _, want := range []string{"first para", "second para", "kept, a subsection"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q from:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"not this", "ignored", "<picture>", "<img"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("carried %q, which it must not:\n%s", unwanted, got)
		}
	}
}

// ⛔ A `#` INSIDE A FENCE IS NOT A HEADING, and this is the rule plan.go
// records as owed to mdHeading's other callers. This function takes it, so a
// shell comment cannot truncate a description.
func TestMarkdownSectionIgnoresHeadingsInsideFences(t *testing.T) {
	doc := "## Wanted\n\nbefore\n\n```sh\n# Next\nrun --thing\n```\n\nafter\n\n## Next\n\nno\n"
	got, err := MarkdownSection(strings.NewReader(doc), "Wanted")
	if err != nil {
		t.Fatalf("MarkdownSection: %v", err)
	}
	if !strings.Contains(got, "after") {
		t.Errorf("a fenced `#` line truncated the section:\n%s", got)
	}
	if strings.Contains(got, "no") && strings.Contains(got, "## Next") {
		t.Errorf("ran past the real heading:\n%s", got)
	}
}

// A MISSING SECTION IS EMPTY AND NOT AN ERROR, so "no such heading" and "the
// document could not be read" stay different answers.
func TestMarkdownSectionMissingIsEmptyNotAnError(t *testing.T) {
	got, err := MarkdownSection(strings.NewReader("## Something\n\nx\n"), "Absent")
	if err != nil {
		t.Fatalf("a missing section must not error: %v", err)
	}
	if got != "" {
		t.Errorf("a missing section answered %q", got)
	}
	if _, err := MarkdownSection(strings.NewReader("x"), "  "); err == nil {
		t.Error("an empty heading argument must be refused")
	}
}
