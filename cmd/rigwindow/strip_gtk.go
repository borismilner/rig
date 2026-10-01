package main

// The strip must never take the keyboard: the hand types into whatever has
// focus, so a strip that takes it eats the run's typing, and on 2026-10-01 the
// first build did exactly that under native Wayland (its page reported
// document.hasFocus() on map, and X named it the active window). Wayland gives a client no way to refuse focus,
// so the strip runs under XWayland as an override-redirect window, the kind
// a tooltip or a menu is. The pointer still reaches the buttons.

/*
#cgo pkg-config: gtk4 x11
#include <gtk/gtk.h>
#ifdef GDK_WINDOWING_X11
#include <gdk/x11/gdkx.h>
#endif

// rig_strip_nofocus realizes the window and makes it override-redirect at
// x, y, and answers 1; or 0 where the window is not an X11 one. An
// override-redirect window is outside the window manager, which is what
// focuses windows on map and on click, so it never gets the keyboard. The
// input hint alone is not enough: GTK rewrites WM_HINTS with input True when
// it maps the window (seen live, 2026-10-01).
static int rig_strip_nofocus(void *win, int x, int y) {
#ifdef GDK_WINDOWING_X11
	gtk_widget_realize(GTK_WIDGET(win));
	GdkSurface *s = gtk_native_get_surface(GTK_NATIVE(win));
	if (s == NULL || !GDK_IS_X11_SURFACE(s)) return 0;
	Display *d = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
	Window w = gdk_x11_surface_get_xid(s);
	XSetWindowAttributes a;
	a.override_redirect = True;
	XChangeWindowAttributes(d, w, CWOverrideRedirect, &a);
	XMoveWindow(d, w, x, y);
	XFlush(d);
	return 1;
#else
	return 0;
#endif
}

static void rig_strip_move(void *win, int x, int y) {
#ifdef GDK_WINDOWING_X11
	GdkSurface *s = gtk_native_get_surface(GTK_NATIVE(win));
	if (s == NULL || !GDK_IS_X11_SURFACE(s)) return;
	Display *d = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
	XMoveWindow(d, gdk_x11_surface_get_xid(s), x, y);
	XFlush(d);
#endif
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// stripNoFocus makes the strip's window one that never takes the keyboard,
// at x, y, and reports whether it could. Must run on the GTK thread, before
// the window is shown.
func stripNoFocus(win *application.WebviewWindow, x, y int) bool {
	p := win.NativeWindow()
	return p != nil && C.rig_strip_nofocus(p, C.int(x), C.int(y)) == 1
}

// stripMove moves the strip's override-redirect window, which no window
// manager will move for it. Must run on the GTK thread.
func stripMove(win *application.WebviewWindow, x, y int) {
	if p := win.NativeWindow(); p != nil {
		C.rig_strip_move(p, C.int(x), C.int(y))
	}
}
