package main

// The toast window is transparent everywhere the page does not paint, and
// Wails' own transparency does not get there on GTK4, which is what this
// binary links: in v3.0.0-beta.19 its GTK4 setTransparent is an empty stub
// ("Transparency via CSS"), so the desktop theme still paints the window's
// background underneath the webview. Under Yaru dark that was a #2c2c2c
// rectangle around the bubbles (Boris, 2026-09-24: "it has strange
// background rectangle"); under a light theme it turned light grey, which is
// how the theme was proven to be the painter.
//
// The provider is added for the whole display, and that is safe only because
// the renderer is its own process with this one window. Never call this from
// the tray or the main window.

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#ifdef GDK_WINDOWING_X11
#include <gdk/x11/gdkx.h>
#endif

static void rig_toast_clear_theme(void) {
	GtkCssProvider *p = gtk_css_provider_new();
	gtk_css_provider_load_from_string(p,
		"window, window.background, window > *, #webview-box {"
		" background: none; background-color: transparent; box-shadow: none; border: none; }");
	gtk_style_context_add_provider_for_display(gdk_display_get_default(),
		GTK_STYLE_PROVIDER(p), GTK_STYLE_PROVIDER_PRIORITY_USER + 1);
	g_object_unref(p);
}
// rig_toast_workarea fills the primary monitor's work area, the part the
// panels leave free, and answers 1; or 0 where it cannot be known.
static int rig_toast_workarea(int *x, int *y, int *w, int *h) {
#ifdef GDK_WINDOWING_X11
	GdkDisplay *d = gdk_display_get_default();
	if (d == NULL || !GDK_IS_X11_DISPLAY(d)) return 0;
	GdkMonitor *m = gdk_x11_display_get_primary_monitor(d);
	if (m == NULL) return 0;
	GdkRectangle r;
	gdk_x11_monitor_get_workarea(m, &r);
	*x = r.x; *y = r.y; *w = r.width; *h = r.height;
	return 1;
#else
	return 0;
#endif
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// clearThemeBackground must run on the GTK thread, after GTK has started.
func clearThemeBackground() { C.rig_toast_clear_theme() }

// x11WorkArea is the primary monitor's work area as X11 knows it. Wails' own
// Screen.WorkArea on GTK4 is the monitor's full bounds, panel included (seen
// 2026-09-24: 1920x1080 with a 48 px panel at the bottom), so without this
// the bubbles cannot tell which edge the tray is on. Must run on the GTK
// thread; ok is false on Wayland or with no primary monitor.
func x11WorkArea() (r application.Rect, ok bool) {
	var x, y, w, h C.int
	if C.rig_toast_workarea(&x, &y, &w, &h) == 0 {
		return r, false
	}
	return application.Rect{X: int(x), Y: int(y), Width: int(w), Height: int(h)}, true
}
