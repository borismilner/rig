package main

import (
	"io/fs"
	"testing"
)

// The embed path and vite's outDir are two halves of one arrangement written in
// two files, and nothing connects them: frontend/vite.config.ts writes to
// ../cmd/rigwindow/dist, this package embeds all:dist, and .gitignore has to
// keep the directory visible while ignoring its contents. Change any one of
// those and the binary still builds - it just serves an empty asset FS, which
// looks like a blank window rather than a build failure. This is the test that
// notices.
func TestBuiltFrontendIsEmbedded(t *testing.T) {
	entries, err := fs.ReadDir(assets, "dist")
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
		t.Skip("dist holds only .gitkeep, so nothing has been built yet: run make build-rigwindow")
	}

	if _, err := fs.Stat(assets, "dist/index.html"); err != nil {
		t.Errorf("dist has been built (%v) but carries no index.html, so the "+
			"window would serve nothing: %v", names, err)
	}
}
