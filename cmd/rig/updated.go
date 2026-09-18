package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/boris-milner/rig/internal/paths"
)

// ⛔ THE FIRST `rig` AFTER A REDEPLOYMENT SAYS SO, ONCE. PLAN.md section 28,
// Boris 2026-09-18: "When first run after a redeployment `rig` should show a
// toast informing the user of the update."
//
// ⛔ WHY A DESKTOP NOTIFICATION AND NOT SECTION 12's OWN SURFACE. §12's toast
// is a frameless webview with a notification centre behind it and none of it
// is built. §12 ALSO specifies the fallback - "falls back to
// `org.freedesktop.Notifications` where the frameless window is unavailable" -
// so this is the planned capability's lower half rather than a new mechanism.
// When §12 lands, this sends through it instead.
//
// ⛔ WHY `notify-send` AND NOT THE D-BUS LIBRARY, which is section 38's
// name-the-candidates rule answered. `github.com/godbus/dbus/v5` is already in
// this module, INDIRECTLY, through systray and wails - both of which the
// WINDOW links and `rig` does not. Importing it here would promote it to a
// direct dependency of the CLI and put it in the `rig` binary, against §17's
// size ratchet and §22's rule that `rig` links no daemon internals. A
// notification is not worth a megabyte in the binary a human runs fifty times
// a day. `notify-send` is the reference client for the same freedesktop
// interface and costs nothing when it is absent.
//
// ⛔ AND IT CANNOT FAIL A COMMAND. Every error here is swallowed on purpose: a
// notice about an update must never be the reason `rig record put` returns
// non-zero, and a machine with no desktop at all is not a broken one.
const updateMarker = "last-announced-version"

// noticeDeadline bounds the notifier. It is not allowed to hold up a command.
const noticeDeadline = 3 * time.Second

type updateNotice struct {
	version string
	marker  string
	notify  func(ctx context.Context, title, body string)
}

// announceUpdate is main's call, wired to the real state directory and the
// real notifier.
func announceUpdate(ctx context.Context) {
	// ⛔ AN UNSTAMPED BUILD ANNOUNCES NOTHING. `go run ./cmd/rig` is "dev" on
	// every run, so a marker written from one would make the next real binary
	// look like an update and the one after that look like a downgrade.
	if version == "dev" || version == "" {
		return
	}
	dir, err := paths.StateDir()
	if err != nil {
		return
	}
	updateNotice{
		version: version,
		marker:  filepath.Join(dir, updateMarker),
		notify:  desktopNotify,
	}.announce(ctx)
}

// announce writes the running version down and says so if it moved.
func (u updateNotice) announce(ctx context.Context) {
	if u.version == "" || u.marker == "" {
		return
	}
	previous := ""
	if b, err := os.ReadFile(u.marker); err == nil {
		previous = strings.TrimSpace(string(b))
	}
	if previous == u.version {
		return
	}
	// ⛔ THE MARKER IS WRITTEN WHETHER OR NOT ANYTHING IS SHOWN, and that is
	// what makes it "first run" rather than "every run": a notice that cannot
	// record itself repeats forever.
	if err := os.MkdirAll(filepath.Dir(u.marker), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(u.marker, []byte(u.version+"\n"), 0o644); err != nil {
		return
	}
	// A first run on a machine that has never run rig is not an update. There
	// is nothing it moved FROM, and saying "updated" would be a claim about a
	// past this machine does not have.
	if previous == "" {
		return
	}
	if u.notify != nil {
		u.notify(ctx, "rig was updated", previous+"  →  "+u.version)
	}
}

// desktopNotify sends one freedesktop notification and cares about nothing.
func desktopNotify(ctx context.Context, title, body string) {
	// --app-name and the icon are what make it read as rig's own notice rather
	// than an anonymous system message, and `low` urgency is section 28's
	// clause that it must not wear the shape of an error.
	cmd := exec.CommandContext(ctx, "notify-send",
		"--app-name=rig", "--icon=rig", "--urgency=low", title, body)
	_ = cmd.Run()
}
