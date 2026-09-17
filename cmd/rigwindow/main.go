// Command rigwindow is the window, and being its own process is the point.
//
// The third binary (PLAN.md section 17, section 22). The daemon links no Wails
// and no webview, and neither does the CLI: `rig window` starts this one, so
// closing the window returns every byte and a beta webview can crash without
// touching anything that matters. It carries its own row in size-ratchet.json
// for the same reason - Wails plus a webview is not a rounding error against
// the CLI's six megabytes.
package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The built frontend, not its sources. Vite writes into dist/ beside this
// package while frontend/ stays at the repo root, where the Makefile's four
// `cd frontend` targets already expect it. dist/.gitkeep is what keeps this
// embed resolvable on a fresh clone, where nothing has been built yet.
//
//go:embed all:dist
var assets embed.FS

var (
	version = "dev"
	wire    = "v1"
	sha     = "none"
	date    = "unknown"
)

// The dark default's --bg, computed by design/theme.js rather than chosen here
// (`tokens(DEFAULTS, 'dark')['--bg']`). The window paints before the webview
// does, so a value that disagrees with the theme is a flash of the wrong
// colour on every single start. Section 6 makes the theme live config, which is
// why this constant is a first-paint fallback and not the source of truth.
var background = application.NewRGB(0x12, 0x1a, 0x23)

func main() {
	showVersion := flag.Bool("version", false, "print every version this build carries and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("product %s\nwire    %s\ncommit  %s\nbuilt   %s\n", version, wire, sha, date)
		return
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rigwindow: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	app := application.New(application.Options{
		Name:        "rig",
		Description: "The platform every in-house program runs on",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},

		// One service, and it is the only route to the daemon. The frontend
		// gets typed bindings for it; it never opens a socket of its own.
		Services: []application.Service{
			application.NewService(&RigService{}),
		},
		// Wails' own chatter, not the product's. Section 8 owns rig's logging,
		// and an INFO line per asset request from a webview is not it.
		LogLevel: slog.LevelWarn,
	})

	// One window (section 11). Sized for a rail, a pane and a status strip
	// rather than for a demo, and floored so the rail cannot be squeezed off
	// screen.
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "rig",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: background,
		URL:              "/",

		// HIDDEN AT BIRTH, AND IT IS A REQUIREMENT RATHER THAN A DEFAULT.
		// Boris, 2026-09-17, on seeing the unit come back after a reboot: "the
		// window should be hidden by default, it is usually a background
		// worker and shown ad-hoc." Section 11 requirement 5. The unit starts
		// this PROCESS at login so the tray is up whenever the graphical
		// session is; the window is one entry on that tray's menu and must
		// wait to be asked for.
		//
		// It is honoured on this platform, which is worth stating because the
		// field is a no-op on some: webview_window_linux.go:453 guards
		// `w.show()` on `!options.Hidden`. Note what else that guard skips -
		// applyScreenPlacement - so a window first shown from the tray takes
		// GTK's placement rather than any StartState this struct asks for. We
		// ask for none, so there is nothing to lose today; a future
		// StartState here would be silently dropped until the first show.
		Hidden: true,

		// No zoom keybinding here, and it is not an omission. Wails cannot
		// deliver one on Linux at beta.19: keys_linux.go's VirtualKeyCodes
		// table holds letters but no digits and none of = + - 0, and
		// getKeyboardState drops any key it cannot name, so a printable-key
		// accelerator never reaches Go at all. Written down because the option
		// exists and compiles, so it reads as working. Tried it: the callbacks
		// never fire. setZoom also clamps at 1.0, so webkit cannot zoom out
		// below 100% either. Zoom belongs in the frontend against the theme's
		// type scale (section 6), which is where step 2 puts it.
	})

	// The tray is the access point (section 11), the window a toggle on it -
	// so closing the window must hide it, not tear it down. Wails' own
	// default WindowClosing listener destroys the window and, once none
	// remain, quits the whole process (application_linux_gtk3.go's
	// unregisterWindow calling a.destroy()) - taking the tray down with it.
	// Cancelling here runs before that listener and skips it entirely; it
	// is skipped rather than overridden, so Hide is this handler's job too.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		win.Hide()

		// RETITLE HERE TOO, AND IT IS A MEASURED DEFECT RATHER THAN TIDINESS.
		// `[ran it]` 2026-09-17 against the installed unit: close the window
		// with _NET_CLOSE_WINDOW and the tray entry still reads "Hide rig" for
		// an already-hidden window. Clicking it then SHOWS the window, so the
		// label says the opposite of what the click does. pollEstate does
		// self-correct it - retitleWindowItem is on its every-5s path - so the
		// window is bounded at trayRefresh and never permanent, which is why
		// nobody caught it by reading. toggleWindow already retitles on its own
		// click path for exactly this reason; this hook is the other way the
		// window's visibility changes and it was the one that did not.
		retitleWindowItem(win)
	})

	// The tray (section 11) is a separate goroutine because both it and
	// app.Run() block. fyne.io/systray, not Wails' own SystemTray - see the
	// comment on runTraySupervisor.
	go runTraySupervisor(app, win)

	return app.Run()
}
