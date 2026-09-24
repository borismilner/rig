// Command rigwindow is the tray, and the window is its child process.
//
// The third binary (PLAN.md section 17, section 22). The daemon links no Wails
// and no webview, and neither does the CLI. This binary runs in two modes:
// with no flags it is the TRAY - a systray icon over D-Bus, a poll of the
// daemon, and nothing else resident - and with --window it is the window
// itself, started by the tray on a click and gone when the window closes. So
// closing the window returns every byte, and a beta webview can crash without
// touching the icon or anything that matters. The split is section 17's rule
// ("the window is a separate process") applied one level further after Boris's
// 2026-09-24 ruling on footprint; supervisor.go carries his words and the
// numbers. It carries its own row in size-ratchet.json for the same reason -
// Wails plus a webview is not a rounding error against the CLI's six megabytes.
package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
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
	window := flag.Bool("window", false, "be the window rather than the tray (the tray starts this itself)")
	flag.Parse()
	if *showVersion {
		fmt.Printf("product %s\nwire    %s\ncommit  %s\nbuilt   %s\n", version, wire, sha, date)
		return
	}

	if *window {
		if err := runWindow(); err != nil {
			fmt.Fprintln(os.Stderr, "rigwindow: "+err.Error())
			os.Exit(1)
		}
		return
	}
	runTray()
}

// runTray is the resident process: the icon, the menu, the poll of rigd. It
// never calls into Wails, so no GTK is initialised and no webview exists
// while the window is closed - that is the whole footprint argument in
// supervisor.go, and the reason this function does not take an *application.App.
func runTray() {
	sup := newSupervisor(spawnWindow, func(msg string) {
		fmt.Fprintln(os.Stderr, "rigwindow: "+msg)
	})
	sup.onChange = func() { retitleWindowItem(sup) }
	runTraySupervisor(sup)
}

// runWindow is the window process. It shows one window and returns when that
// window closes: Wails' default WindowClosing listener destroys the window
// and, once none remain, quits the application (application_linux_gtk3.go's
// unregisterWindow calling a.destroy()), which is exactly the exit wanted.
// The tray is another process, so nothing here can take the icon down
// (section 11 requirement 4).
func runWindow() error {
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

		// NOT HIDDEN, AND THAT IS NOT A REVERSAL OF REQUIREMENT 5. Boris,
		// 2026-09-17: "the window should be hidden by default, it is usually a
		// background worker and shown ad-hoc." Section 11 requirement 5. The
		// tray process autostarts at login and shows no window; THIS process
		// exists only because he clicked, so its one window is shown at birth
		// and the requirement is met one process earlier. (Hidden: true here
		// would be a window nobody can reach.) The placement note from the
		// old shape still applies: webview_window_linux.go:453 runs
		// applyScreenPlacement only on this show path, so a StartState asked
		// for here would now be honoured.

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

	// In front, not merely on screen: this process exists because he clicked.
	win.Focus()

	// No WindowClosing hook. The old one cancelled the close and hid the
	// window so the tray in this process would survive; the tray is now the
	// parent process, so the default listener - destroy, and quit on the last
	// window - is the behaviour wanted, and the parent notices the exit and
	// retitles its menu (supervisor.go). Measured 2026-09-24: every WebKit
	// process under this one is gone within four seconds of its exit.

	return app.Run()
}
