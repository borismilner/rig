package main

import (
	"io/fs"
	"testing"
)

// The embed path and the Makefile's copy step are two halves of one
// arrangement written in two files, and nothing connects them: `make
// build-rigwindow` copies design/tray into cmd/rigwindow/icons, tray.go embeds
// all:icons, and .gitignore has to keep the directory visible while ignoring
// its contents. Change any one of those and the binary still builds - the
// tray just never gets an icon, which looks like the estate query failing
// rather than a build failure. This is the test that notices.
//
// Same shape as cmd/rigwindow/embed_test.go and cmd/ledger/embed_test.go, and
// for the same reason: go:embed cannot reach above its own package.
func TestTrayIconsAreEmbedded(t *testing.T) {
	entries, err := fs.ReadDir(trayIcons, "icons")
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
		t.Skip("icons holds only .gitkeep, so nothing has been copied yet: run make build-rigwindow")
	}

	// ⛔ THE -down PAIR IS HERE BECAUSE IT FAILS EXACTLY LIKE THE OTHER TWO
	// AND IS LESS LIKELY TO BE NOTICED. A missing estate icon shows up the
	// moment the tray starts; a missing down icon shows up only when the
	// daemon stops, which is the one moment Boris is relying on it - "if the
	// daemon is down it can also indicate it with a red dot", 2026-09-17.
	for _, want := range []string{
		"icons/development.png", "icons/production.png",
		"icons/development-down.png", "icons/production-down.png",
	} {
		if _, err := fs.Stat(trayIcons, want); err != nil {
			t.Errorf("icons has been copied (%v) but %s is missing, so the "+
				"tray would fail to set that estate's icon: %v", names, want, err)
		}
	}
}
