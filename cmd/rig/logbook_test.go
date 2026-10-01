package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/borismilner/rig/internal/logbook"
)

// A project's checkout links one document into its logbook folder and not
// the others. Every document must still resolve from the checkout, through
// the link or not.
func TestLogbookResolvesFromACheckoutWithLinks(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "logbook", "projects", "proj")
	checkout := filepath.Join(root, "proj")
	for _, d := range []string{notes, filepath.Join(checkout, ".git")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"DECISIONS.md", "COORDINATION.md"} {
		d := logbook.Open(filepath.Join(notes, name))
		if err := os.WriteFile(d.Index, []byte("# x\n\n## one\n\nbody\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := d.SplitFile(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(notes, "DECISIONS.md"), filepath.Join(checkout, "DECISIONS.md")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIG_LOGBOOK", filepath.Join(root, "logbook"))
	t.Chdir(checkout)
	lb := logbookIn{}
	for _, name := range []string{"DECISIONS.md", "COORDINATION.md"} {
		d, err := lb.doc(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !d.Split() {
			t.Errorf("%s resolved to %s, which is not split", name, d.Index)
		}
	}
	if docs, err := lb.docs(nil); err != nil || len(docs) != 2 {
		t.Errorf("every document: got %d, %v", len(docs), err)
	}
}
