package logbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doc builds a document with every shape the splitter handles: an about
// part, small sections, one section over Big made of `###` subsections, and
// one over Big that is a single table.
func doc() string {
	var b strings.Builder
	b.WriteString("# Title\n\nabout text\n\n")
	for i := range 5 {
		fmt.Fprintf(&b, "## 2026-10-0%d - entry %d\n\nbody %d\n\n", i+1, i, i)
	}
	b.WriteString("## Big prose\n\n")
	for i := range 4 {
		fmt.Fprintf(&b, "### Sub %d\n\n%s\n\n", i, strings.Repeat("word ", 1500))
	}
	b.WriteString("## Big table\n\n| id | what |\n|---|---|\n")
	for i := range 300 {
		fmt.Fprintf(&b, "| B%d | %s |\n", i, strings.Repeat("x", 100))
	}
	b.WriteString("\n## Last\n\nend\n")
	return b.String()
}

func split(t *testing.T, name, text string) Doc {
	t.Helper()
	dir := t.TempDir()
	d := Open(filepath.Join(dir, name))
	if err := os.WriteFile(d.Index, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.SplitFile(); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSplitReassemblesByteForByte(t *testing.T) {
	text := doc()
	d := split(t, "DECISIONS.md", text)
	parts, err := d.Parts()
	if err != nil {
		t.Fatal(err)
	}
	if Join(parts) != text {
		t.Fatal("the parts do not reassemble to the document")
	}
	leaves, err := d.Leaves()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range leaves {
		if size(l.Text) > Big {
			t.Errorf("%s is %d characters, over Big", l.Path, size(l.Text))
		}
	}
	if !d.Generated() {
		t.Error("the index is not recognised as generated")
	}
	if stale, err := d.Stale(); err != nil || stale {
		t.Errorf("a fresh split is stale: %v %v", stale, err)
	}
}

func TestTxtSplitsOnBanners(t *testing.T) {
	rule := strings.Repeat("-", 30) + "\n"
	text := "intro\n\n" + rule + "ONE\n" + rule + "a\n\n" + rule + "TWO\n" + rule + "b\n"
	parts := Plan(text, ".txt")
	if len(parts) != 3 || Title(parts[1].Text) != "ONE" || Join(parts) != text {
		t.Fatalf("got %d parts, %q", len(parts), Title(parts[1].Text))
	}
}

// Every line of the document is found again, by At, in the part that holds
// it: how a citation by line number still resolves after the split.
func TestAtMapsEveryLine(t *testing.T) {
	text := doc()
	d := split(t, "BACKLOG.md", text)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for n, want := range lines {
		leaf, at, err := d.At(n + 1)
		if err != nil {
			t.Fatalf("line %d: %v", n+1, err)
		}
		if got := strings.Split(leaf.Text, "\n")[at-1]; got != want {
			t.Fatalf("line %d: got %q want %q", n+1, got, want)
		}
	}
	if _, _, err := d.At(len(lines) + 1); err == nil {
		t.Error("a line past the end resolved")
	}
}

// An entry appended below the marker the old way is seen as pending, then
// moved into a part by Reindex; both markers, this one and the Python
// tool's, guard what sits below them.
func TestReindexMovesInWhatWasAppended(t *testing.T) {
	for _, old := range []bool{false, true} {
		d := split(t, "DECISIONS.md", doc())
		idx, _ := os.ReadFile(d.Index)
		if old {
			idx = []byte(markRE.ReplaceAllString(string(idx),
				"<!-- logsplit: add an entry below this line, or as a new file in decisions/; then run python3 x index DECISIONS.md -->"))
		}
		idx = append(idx, "\n## one\n\nfirst\n\n## two\n\nsecond\n"...)
		if err := os.WriteFile(d.Index, idx, 0o600); err != nil {
			t.Fatal(err)
		}
		if p, _ := d.Pending(); !strings.Contains(p, "second") {
			t.Fatalf("pending is %q", p)
		}
		n, err := d.Reindex()
		if err != nil || n != 2 {
			t.Fatalf("moved %d: %v", n, err)
		}
		if p, _ := d.Pending(); p != "" {
			t.Errorf("still pending: %q", p)
		}
		parts, _ := d.Parts()
		if !strings.HasSuffix(Join(parts), "## one\n\nfirst\n\n## two\n\nsecond\n\n") {
			t.Error("the entries are not at the end of the document")
		}
	}
}

func TestAddNumbersAfterTheLastAndNeverOverwrites(t *testing.T) {
	d := split(t, "DECISIONS.md", doc())
	p, err := d.Add("## 2026-10-01 - a new one\n\ntext")
	if err != nil {
		t.Fatal(err)
	}
	if want := "0009-2026-10-01-a-new-one.md"; filepath.Base(p) != want {
		t.Errorf("got %s want %s", filepath.Base(p), want)
	}
	if _, err := d.Add("  \n"); err == nil {
		t.Error("an empty entry was written")
	}
}

func TestReindexRefusesAFileThatIsNotAnIndex(t *testing.T) {
	d := split(t, "DECISIONS.md", doc())
	if err := os.WriteFile(d.Index, []byte("# hand written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Reindex(); err == nil {
		t.Fatal("a hand-written file was overwritten")
	}
	if d.Generated() {
		t.Error("a hand-written file passed as generated")
	}
}

func TestSlugAndTitle(t *testing.T) {
	for in, want := range map[string]string{
		"## 2026-10-01 - `rig` *is* ⛔ here": "2026-10-01-rig-is-here",
		"| ⛔ **B107** | the bus | x |":      "b107-the-bus",
		"":                                  "part",
	} {
		if got := Slug(Title(in)); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
