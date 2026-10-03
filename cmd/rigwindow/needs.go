package main

// Needs you, as the tray carries it (plan/55 requirement 25): while a
// question waits on him or a supervised program is parked, the icon is
// badged and a menu row counts them, and the click takes him to Needs you
// instead of toggling the window.
//
// Two processes, one line between them. The tray counts; the window is told
// "needs" on its stdin (supervisor.go) and shows the tab.

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/systray"
)

const (
	needsLine  = "needs"
	needsEvent = "rig:needs"
)

// ── the window's half ──────────────────────────────────────────────────

// wantNeeds is set when the tray asked and cleared when the page took it,
// so an ask that lands before the page is listening is not lost.
var wantNeeds atomic.Bool

// TakeNeeds reports whether the tray asked for Needs you since the page
// last looked, and forgets the ask.
func (RigService) TakeNeeds() bool { return wantNeeds.Swap(false) }

// readTray takes the tray's lines until the pipe closes. Anything but
// "needs" is ignored: a window started by hand reads a terminal.
func readTray(r io.Reader, show func()) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == needsLine {
			wantNeeds.Store(true)
			show()
		}
	}
}

// ── the tray's half ────────────────────────────────────────────────────

var (
	needsMu sync.Mutex
	// asksWaiting is recounted on every notification change; parked on the
	// estate poll, because a program parking files no notification.
	asksWaiting, parkedNow int
	menuNeeds              *systray.MenuItem
)

// needsCount is what the badge and the row say.
func needsCount() int {
	needsMu.Lock()
	defer needsMu.Unlock()
	return asksWaiting + parkedNow
}

// recountAsks reads the notifications again and counts what still waits.
// A failed read keeps the last count: the estate poll badges a dead daemon.
func recountAsks() {
	list, err := RigService{}.Notifications()
	if err != nil {
		return
	}
	n := 0
	for _, x := range list.Notes {
		if x.Waiting && x.Answer == nil {
			n++
		}
	}
	needsMu.Lock()
	asksWaiting = n
	needsMu.Unlock()
	paintNeeds()
}

// recountParked is the estate poll's share: one rig.health a tick.
func recountParked() {
	n := 0
	for _, r := range (RigService{}).Supervision() {
		if r.Parked != "" {
			n++
		}
	}
	needsMu.Lock()
	parkedNow = n
	needsMu.Unlock()
}

// needsTitle is the menu row's words.
func needsTitle(n int) string { return "Needs you (" + strconv.Itoa(n) + ")" }

// askIcon is the badged variant for an estate glyph while something waits.
func askIcon(estate string) string {
	return strings.TrimSuffix(estate, ".png") + "-ask.png"
}

// needsKick asks the estate poll to run now, so the badge follows a
// notification at once. The icon is only ever set on that goroutine, which
// owns the estate state the badge depends on.
var needsKick = make(chan struct{}, 1)

// paintNeeds puts the count on the row and asks the poll for the icon.
func paintNeeds() {
	paintNeedsRow(true)
	select {
	case needsKick <- struct{}{}:
	default:
	}
}

// paintNeedsRow shows the row with the count, or hides it. A daemon that
// does not answer has no count, so the detached state hides it.
func paintNeedsRow(attached bool) {
	if menuNeeds == nil {
		return
	}
	if n := needsCount(); attached && n > 0 {
		menuNeeds.SetTitle(needsTitle(n))
		menuNeeds.Show()
	} else {
		menuNeeds.Hide()
	}
}

// trayIconFor is the estate's glyph, badged while something waits.
func trayIconFor(estate string, n int) string {
	if n > 0 {
		return askIcon(estate)
	}
	return estate
}
