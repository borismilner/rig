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

// fyne.io/systray, not Wails' own application.SystemTray: the Wails beta.19
// implementation exports the StatusNotifierItem correctly (properties answer
// over D-Bus, confirmed with dbus-send) but its RegisterStatusNotifierItem
// call never lands in org.kde.StatusNotifierWatcher's own
// RegisteredStatusNotifierItems list, so nothing ever draws it - live-checked
// on this machine's GNOME session, no error logged either side. AgentBox
// registers fine with fyne.io/systray in the same session, so this package
// uses that instead of chasing the beta bug.
func runTraySupervisor(win application.Window) {
	systray.Run(func() {
		systray.SetTooltip("rig")
		systray.SetOnTapped(func() { toggleWindow(win) })
		go pollEstate()
	}, nil)
}

func toggleWindow(win application.Window) {
	if win.IsVisible() && !win.IsMinimised() {
		win.Hide()
		return
	}
	win.Show()
	win.Focus()
}

// pollEstate keeps the icon honest for the life of the process. Section 11 is
// explicit that an UNNAMED estate gets no tray at all - every test and
// reproduction recipe starts one, and a third icon appearing during `make ci`
// is the failure this is meant to prevent - so this only calls SetIcon once a
// named estate answers, and quits the tray if that ever reverts.
//
// Detached (rigd unreachable) is different: the window is still up, so the
// tray stays up too, on its last-known icon. A dedicated detached glyph is
// one of the three dimensions section 11 still owes and is not decided here.
func pollEstate() {
	named := false
	for {
		role, connected := estateRole()
		switch {
		case connected && role == rigv1.EstateRole_ESTATE_ROLE_PRODUCTION:
			setTrayIcon("production.png", "rig - production")
			named = true
		case connected && role == rigv1.EstateRole_ESTATE_ROLE_DEVELOPMENT:
			setTrayIcon("development.png", "rig - development")
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
			// Detached: keep whatever was last shown.
		}
		time.Sleep(trayRefresh)
	}
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

// estateRole reuses RigService's own dial-per-call shape (see readDeadline in
// service.go) rather than holding a connection, for the same reason: no
// socket to go stale, so the next tick reconnects on its own.
func estateRole() (rigv1.EstateRole, bool) {
	c, err := client.Connect()
	if err != nil {
		return rigv1.EstateRole_ESTATE_ROLE_UNSPECIFIED, false
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.EstateResponse{}
	if err := c.Call(ctx, "rig.estate", &rigv1.EstateRequest{}, resp); err != nil {
		return rigv1.EstateRole_ESTATE_ROLE_UNSPECIFIED, false
	}
	return resp.GetRole(), true
}
