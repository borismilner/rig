package main

import (
	"io/fs"
	"testing"
)

// The embed path and the Makefile's copy step are two halves of one
// arrangement written in two files, and nothing connects them: `make
// build-ledger` copies design/kit into cmd/ledger/kit, this package embeds
// all:kit, and .gitignore has to keep the directory visible while ignoring its
// contents. Change any one of those and the binary still builds - it serves a
// page whose stylesheet and modules 404, which looks like an unstyled program
// rather than a build failure. This is the test that notices.
//
// Same shape as cmd/rigwindow/embed_test.go, and for the same reason: go:embed
// cannot reach above its own package, so the source of truth is copied in.
func TestKitIsEmbedded(t *testing.T) {
	entries, err := fs.ReadDir(kitFS, "kit")
	if err != nil {
		t.Fatalf("the embed did not resolve at all: %v", err)
	}

	var names []string
	for _, e := range entries {
		if e.Name() == ".gitkeep" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Skip("kit holds only .gitkeep, so nothing has been copied yet: run make build-ledger")
	}

	for _, want := range []string{"kit/kit.css", "kit/kit.js", "kit/pane.js"} {
		if _, err := fs.Stat(kitFS, want); err != nil {
			t.Errorf("the kit has been copied (%v) but %s is missing, so the "+
				"page would load it as a 404: %v", names, want, err)
		}
	}
}
