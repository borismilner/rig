package main

import (
	"strings"
	"testing"
)

// The tray's one line shows Needs you once per "needs", ignores anything
// else a terminal might type, and leaves the ask for the page to take once.
func TestTheTraysLineAsksForNeedsYouAndThePageTakesItOnce(t *testing.T) {
	wantNeeds.Store(false)
	shown := 0
	readTray(strings.NewReader("hello\n needs \nneedsx\n"), func() { shown++ })
	if shown != 1 {
		t.Fatalf("shown %d times, want 1", shown)
	}
	if !(RigService{}).TakeNeeds() {
		t.Fatal("the page found no ask")
	}
	if (RigService{}).TakeNeeds() {
		t.Fatal("the ask was taken twice")
	}
}

func TestTheBadgeIsTheEstatesOwnGlyphAndOnlyWhileSomethingWaits(t *testing.T) {
	for _, c := range []struct {
		estate string
		n      int
		want   string
	}{
		{"production.png", 0, "production.png"},
		{"production.png", 2, "production-ask.png"},
		{"development.png", 1, "development-ask.png"},
	} {
		if got := trayIconFor(c.estate, c.n); got != c.want {
			t.Errorf("trayIconFor(%q, %d) = %q, want %q", c.estate, c.n, got, c.want)
		}
		if iconBytes(c.want) == nil {
			t.Errorf("%s is not embedded", c.want)
		}
	}
	if needsTitle(3) != "Needs you (3)" {
		t.Errorf("row reads %q", needsTitle(3))
	}
}
