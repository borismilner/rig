package main

import (
	"testing"
	"time"
)

// ⛔ THE BADGE GOES ON THE ESTATE'S OWN GLYPH, so the tray does not answer
// "rig is not running" by forgetting which estate it was.
//
// Boris, 2026-09-17: "if the daemon is down it can also indicate it with a red
// dot and details when clicked." The estate is the fact the tray exists to
// carry, and a single shared down icon would drop it at the one moment he is
// looking hardest.
func TestTheDownIconKeepsTheEstateItWasShowing(t *testing.T) {
	for _, c := range []struct{ last, want string }{
		{"production.png", "production-down.png"},
		{"development.png", "development-down.png"},

		// ⛔ ALREADY BADGED MUST NOT DOUBLE-BADGE. pollEstate ticks every five
		// seconds and calls this on every tick while detached, so a function
		// that appended each time would ask the embed for
		// `production-down-down.png` on the second tick and the icon would
		// silently stop changing - the exact failure the badge exists to fix.
		{"production-down.png", "production-down.png"},

		// No estate has ever answered: the window started while rigd was
		// already down. Showing nothing is what he complained about.
		{"", "production-down.png"},
	} {
		if got := downIcon(c.last); got != c.want {
			t.Errorf("downIcon(%q) = %q, want %q", c.last, got, c.want)
		}
	}
}

// AND THE DURATION IS READ AT A GRAIN A PERSON USES.
//
// ⛔ THE SECONDS ARE LOAD-BEARING BELOW A MINUTE. `make install` stops rigd and
// starts it again within a few seconds, so "down for 4s" is what tells him he
// is watching a deploy rather than an outage. Above a minute they are a number
// nobody reads on a row that would otherwise change every five seconds.
func TestTheDetachedClockRoundsToSomethingReadable(t *testing.T) {
	for _, c := range []struct {
		ago  time.Duration
		want time.Duration
	}{
		{4200 * time.Millisecond, 4 * time.Second},
		{59 * time.Second, 59 * time.Second},
		{92 * time.Second, 2 * time.Minute},
		{40 * time.Minute, 40 * time.Minute},
	} {
		got := roundedSince(time.Now().Add(-c.ago))
		if got != c.want {
			t.Errorf("roundedSince(%s ago) = %s, want %s", c.ago, got, c.want)
		}
	}
}

// ⛔ A ZERO CLOCK IS NOT "DOWN SINCE THE EPOCH". setFacts resets detachedSince
// when the daemon answers, and a caller that then read it without checking
// would report fifty-six years. This is the guard on that: the zero value has
// to be asked about, never formatted.
func TestTheClockIsZeroWhileTheDaemonAnswers(t *testing.T) {
	detachedSince = time.Time{}
	if !detachedSince.IsZero() {
		t.Fatalf("the fixture is wrong, so nothing below is tested")
	}
	if d := roundedSince(detachedSince); d < 100*time.Hour {
		t.Errorf("roundedSince on a zero time returned %s, which is a "+
			"plausible-looking duration - the callers must test IsZero "+
			"rather than trust this", d)
	}
}
