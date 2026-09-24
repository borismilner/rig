package main

import (
	"io/fs"
	"strings"
	"testing"
)

// ⛔ THE EMBEDDED TIER IS DEFINED BY WHAT IS ABSENT. A pane_url moves a
// program off the generated tier; an empty element list and a page that loads
// neither kit.css nor kit.js keep it off the kit tier. If either slips, lantern
// silently becomes a third kit-tier demo and the embedded tier is undemonstrated
// again.
func TestLanternIsOnTheEmbeddedTier(t *testing.T) {
	d := declaration("lantern", "http://127.0.0.1:7453/pane")
	if d.GetPaneUrl() == "" {
		t.Fatal("no pane_url, so the window would draw the generated tier")
	}
	if n := len(d.GetElements()); n != 0 {
		t.Fatalf("declares %d kit elements (%v); the embedded tier takes none", n, d.GetElements())
	}
	for _, kit := range []string{"kit.css", "kit.js"} {
		if strings.Contains(page, kit) {
			t.Errorf("the page loads %s, which is the kit tier, not the embedded one", kit)
		}
	}
	if !strings.Contains(page, `from "/rig/pane.js"`) {
		t.Error("the page does not load pane.js, so it can never receive the token set")
	}
	if !strings.Contains(page, `html:not([data-rig-painted])`) {
		t.Error("the page does not hold its paint on pane.js's documented marker")
	}
}

// The Makefile copies design/kit/pane.js in and this package embeds it; the
// two halves live in different files and nothing else connects them.
func TestPaneJSIsEmbedded(t *testing.T) {
	entries, err := fs.ReadDir(rigFS, "rig")
	if err != nil {
		t.Fatalf("the embed did not resolve: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != ".gitkeep" {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		t.Skip("rig/ holds only .gitkeep: run make build-lantern")
	}
	if _, err := fs.Stat(rigFS, "rig/pane.js"); err != nil {
		t.Errorf("pane.js is not embedded (%v): %v", names, err)
	}
	for _, n := range names {
		if n != "pane.js" {
			t.Errorf("rig/ holds %s; the embedded tier ships pane.js alone", n)
		}
	}
}
