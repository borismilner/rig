package main

import (
	"context"
	"embed"
	"io/fs"
	"time"

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

// runTraySupervisor owns the tray for the life of the window. It creates and
// destroys the tray rather than just repainting it, because section 11 is
// explicit that an UNNAMED estate gets no tray at all - every test and
// reproduction recipe starts one, and a third icon appearing during `make ci`
// is the failure this is meant to prevent.
//
// Detached (rigd unreachable) is different: the window is still up, so the
// tray stays up too, on its last-known icon, with the tooltip saying so. A
// dedicated detached glyph is one of the three dimensions section 11 still
// owes and is not decided here.
func runTraySupervisor(app *application.App, win application.Window) {
	var tray *application.SystemTray
	for {
		role, connected := estateRole()
		switch {
		case connected && role == rigv1.EstateRole_ESTATE_ROLE_PRODUCTION:
			tray = ensureTray(app, win, tray, "production.png", "rig - production")
		case connected && role == rigv1.EstateRole_ESTATE_ROLE_DEVELOPMENT:
			tray = ensureTray(app, win, tray, "development.png", "rig - development")
		case connected:
			// Unnamed or unspecified: no tray, by section 11's own rule.
			if tray != nil {
				tray.Destroy()
				tray = nil
			}
		default:
			// Detached: keep whatever was last shown, say so in the tooltip.
			if tray != nil {
				tray.SetTooltip("rig - detached (rigd not reachable)")
			}
		}
		time.Sleep(trayRefresh)
	}
}

func ensureTray(app *application.App, win application.Window, tray *application.SystemTray, icon, tooltip string) *application.SystemTray {
	if tray == nil {
		tray = app.SystemTray.New()
		tray.AttachWindow(win).WindowOffset(4)
		tray.Run()
	}
	if b := iconBytes(icon); b != nil {
		tray.SetIcon(b)
	}
	tray.SetTooltip(tooltip)
	return tray
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
