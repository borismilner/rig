package main

import (
	"context"
	"embed"
	"io/fs"
	"time"

	"fyne.io/systray"
	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// design/tray is the source (section 11); cmd/rigwindow/icons is where the
// Makefile's build-rigwindow copies it, because go:embed cannot reach above
// its own package. Same arrangement as dist above it in this package.
//
//go:embed all:icons
var trayIcons embed.FS

// trayRefresh polls rigd for which estate it is rather than being told once,
// because the window survives a daemon restart (section 11) and a tray that
// only asks at launch would freeze on whatever it first saw.
const trayRefresh = 5 * time.Second

// The menu, held package-level because the poll loop retitles it on every
// tick and systray has no way to read an item back.
//
// BORIS, 2026-09-16: "Clicking on the icon reveals the different options and
// it also shows the version and whether it's prod or dev." So the first two
// rows are FACTS AND NOT COMMANDS - disabled, because a menu entry that looks
// clickable and does nothing is worse than a label.
var (
	menuVersion *systray.MenuItem
	menuEstate  *systray.MenuItem
	menuWindow  *systray.MenuItem
)

// fyne.io/systray, not Wails' own application.SystemTray: the Wails beta.19
// implementation exports the StatusNotifierItem correctly (properties answer
// over D-Bus, confirmed with dbus-send) but its RegisterStatusNotifierItem
// call never lands in org.kde.StatusNotifierWatcher's own
// RegisteredStatusNotifierItems list, so nothing ever draws it - live-checked
// on this machine's GNOME session, no error logged either side. AgentBox
// registers fine with fyne.io/systray in the same session, so this package
// uses that instead of chasing the beta bug.
func runTraySupervisor(app *application.App, win application.Window) {
	systray.Run(func() {
		systray.SetTooltip("rig")

		// Pre-daemon text, so the menu is never blank before the first poll
		// answers. The window's OWN build stamp is the honest answer here:
		// nothing has told us the daemon's yet.
		menuVersion = systray.AddMenuItem("rig "+version+" (window)", "the version this window was built at")
		menuVersion.Disable()
		menuEstate = systray.AddMenuItem("looking for a daemon", "which estate this tray is attached to")
		menuEstate.Disable()

		systray.AddSeparator()
		menuWindow = systray.AddMenuItem("Show rig", "Open or hide the rig window")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit rig window", "Close the window and its tray icon. The daemon keeps running")

		// Left-click still toggles, so the gesture that worked before this
		// menu existed keeps working. The menu is an addition, not a
		// replacement: section 11 says the tray is an access point, and
		// taking away the one-click toggle to add options would trade one
		// for the other.
		systray.SetOnTapped(func() { toggleWindow(win) })

		go func() {
			for {
				select {
				case <-menuWindow.ClickedCh:
					toggleWindow(win)
				case <-quit.ClickedCh:
					// Quits the WINDOW PROCESS, which is what owns this tray.
					// rigd is a separate process and is deliberately left
					// running - the menu entry says so, because "Quit rig"
					// next to a tray icon reads as "stop rig" and that is the
					// one thing this must not be mistaken for.
					app.Quit()
					return
				}
			}
		}()

		go pollEstate(win)
	}, nil)
}

func toggleWindow(win application.Window) {
	if win.IsVisible() && !win.IsMinimised() {
		win.Hide()
	} else {
		win.Show()
		win.Focus()
	}
	retitleWindowItem(win)
}

// retitleWindowItem keeps the entry describing what clicking it will DO, the
// way AgentBox's own tray does. Called from the click path for immediacy and
// from the poll loop so it self-corrects when the window is hidden or shown
// by anything other than this menu.
func retitleWindowItem(win application.Window) {
	if menuWindow == nil {
		return
	}
	if win.IsVisible() && !win.IsMinimised() {
		menuWindow.SetTitle("Hide rig")
		return
	}
	menuWindow.SetTitle("Show rig")
}

// pollEstate keeps the icon AND the two fact rows honest for the life of the
// process. Section 11 is explicit that an UNNAMED estate gets no tray at all -
// every test and reproduction recipe starts one, and a third icon appearing
// during `make ci` is the failure this is meant to prevent - so this only
// calls SetIcon once a named estate answers, and quits the tray if that ever
// reverts.
//
// Detached (rigd unreachable) is different: the window is still up, so the
// tray stays up too, on its last-known icon. A dedicated detached glyph is
// one of the three dimensions section 11 still owes and is not decided here.
// The TEXT does not stay on its last-known value, though, and that asymmetry
// is deliberate: a stale icon is ambiguous, a stale VERSION is a lie, so the
// rows say the daemon is gone while the icon holds.
func pollEstate(win application.Window) {
	named := false
	for {
		est, connected := estateSnapshot()
		switch {
		case connected && est.GetRole() == rigv1.EstateRole_ESTATE_ROLE_PRODUCTION:
			setTrayIcon("production.png", "rig - production")
			setFacts(est)
			named = true
		case connected && est.GetRole() == rigv1.EstateRole_ESTATE_ROLE_DEVELOPMENT:
			setTrayIcon("development.png", "rig - development")
			setFacts(est)
			named = true
		case connected:
			// Unnamed or unspecified: no tray, by section 11's own rule. The
			// tray having been created to reach this branch at all, quitting
			// it is the closest fyne.io/systray gets to "never existed" -
			// there is no re-create-on-demand hook like Wails' SystemTray.New.
			if named {
				systray.Quit()
				return
			}
		default:
			// Detached: keep the icon, say so in the text.
			setDetached()
		}
		retitleWindowItem(win)
		time.Sleep(trayRefresh)
	}
}

// setFacts writes the two rows Boris asked for. The version served is the
// DAEMON's, not this window's, because the daemon is the thing actually
// running: a window left open across an upgrade would otherwise report the
// version it was built at while talking to a newer estate.
func setFacts(est *rigv1.EstateResponse) {
	if menuVersion == nil || menuEstate == nil {
		return
	}
	v := est.GetDaemonVersion()
	if v == "" {
		// An empty string on the wire is indistinguishable from unserved, so
		// it is NOT rendered as a version. Same reasoning as decision 6.
		v = "unknown"
	}
	menuVersion.SetTitle("rig " + v)
	menuVersion.SetTooltip("the version of the daemon this tray is attached to")

	name := est.GetName()
	if name == "" {
		// Unnamed reaches here only before the branch above quits the tray.
		name = "unnamed"
	}
	menuEstate.SetTitle("estate: " + name)
	menuEstate.SetTooltip("which estate this tray is attached to")
}

// setDetached says the daemon is gone rather than leaving the last version on
// screen. It falls back to this window's own build stamp and SAYS it is the
// window's, so the row is never a claim about a daemon that is not answering.
func setDetached() {
	if menuVersion == nil || menuEstate == nil {
		return
	}
	menuVersion.SetTitle("rig " + version + " (window)")
	menuEstate.SetTitle("no daemon answering")
}

func setTrayIcon(icon, tooltip string) {
	if b := iconBytes(icon); b != nil {
		systray.SetIcon(b)
	}
	systray.SetTooltip(tooltip)
}

func iconBytes(name string) []byte {
	b, err := fs.ReadFile(trayIcons, "icons/"+name)
	if err != nil {
		return nil
	}
	return b
}

// estateSnapshot reuses RigService's own dial-per-call shape (see readDeadline
// in service.go) rather than holding a connection, for the same reason: no
// socket to go stale, so the next tick reconnects on its own.
//
// It returns the whole response rather than just the role, because the menu
// needs the name and the daemon version too and a second call would be a
// second dial answering about a possibly different instant.
func estateSnapshot() (*rigv1.EstateResponse, bool) {
	c, err := client.Connect()
	if err != nil {
		return nil, false
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.EstateResponse{}
	if err := c.Call(ctx, "rig.estate", &rigv1.EstateRequest{}, resp); err != nil {
		return nil, false
	}
	return resp, true
}
