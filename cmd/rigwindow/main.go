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
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "rig",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: background,
		URL:              "/",

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

	return app.Run()
}
