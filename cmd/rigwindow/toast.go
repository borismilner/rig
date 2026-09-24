package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 12's toasts, the screen's half. Three rulings shape this file
// (the lead, relaying the owner, 2026-09-24):
//
//   - FOOTPRINT: the renderer is an on-demand child process, the window's
//     pattern. The tray holds one long poll on rig.toast.wait and nothing
//     else; the first toast starts `rigwindow --toasts`, which exits when its
//     last bubble leaves, so the tray's idle footprint does not change. When
//     the renderer cannot start, the toast goes to
//     org.freedesktop.Notifications instead.
//   - RECORD: the daemon files every notification before any of this runs,
//     so nothing here is the only copy.
//   - POSITION: Linux gives no tray icon position, so the bubbles anchor at
//     the corner of the monitor's work area where the tray sits (top-right
//     on GNOME), and the first bubble's tail points into that corner. A tail
//     is never aimed at the icon on a guess.

//go:embed toast.html
var toastPage []byte

const (
	toastWidth  = 400 // the window's width; the page's bubbles are 360 plus the tail's room
	toastMargin = 8   // from the work area's corner
	toastLinger = 1500 * time.Millisecond
	toastPoll   = 55 * time.Second
)

// --- the tray's half ---------------------------------------------------------

// toastSpawn starts the renderer for toasts after the given cursor.
type toastSpawn func(after uint64) (exited <-chan error, err error)

// toastWatcher is the tray's whole toast state: a cursor, and whether a
// renderer is alive.
type toastWatcher struct {
	spawn    toastSpawn
	fallback func(*registryv1.Toast) error
	warn     func(string)
	// grace is how soon an exit still counts as dying at start; zero means
	// five seconds.
	grace time.Duration

	mu      sync.Mutex
	running bool
}

// deliver is called with each batch the tray's wait returns. With no renderer
// alive it starts one from the first toast's cursor; the renderer then waits
// on the daemon itself. A renderer that cannot start, or dies before it
// could draw, sends the batch to the desktop's notification service.
func (w *toastWatcher) deliver(batch []*registryv1.Toast) {
	if len(batch) == 0 {
		return
	}
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	exited, err := w.spawn(batch[0].GetSeq() - 1)
	if err != nil {
		w.mu.Unlock()
		w.warn("the toast renderer would not start, using the desktop's notifications: " + err.Error())
		w.fallbackAll(batch)
		return
	}
	w.running = true
	w.mu.Unlock()
	started := time.Now()
	go func() {
		err := <-exited
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		grace := w.grace
		if grace == 0 {
			grace = 5 * time.Second
		}
		if err != nil && time.Since(started) < grace {
			w.warn("the toast renderer died at start, using the desktop's notifications: " + err.Error())
			w.fallbackAll(batch)
		}
	}()
}

func (w *toastWatcher) fallbackAll(batch []*registryv1.Toast) {
	for _, t := range batch {
		if err := w.fallback(t); err != nil {
			w.warn("the desktop's notification service refused too; the toast is in the record only: " + err.Error())
			return
		}
	}
}

// watchToasts is the tray's long poll. It starts at the daemon's latest
// cursor, so a tray that starts late does not replay the backlog as toasts:
// those are in the record.
func watchToasts(w *toastWatcher) {
	var after uint64
	primed := false
	for {
		resp, err := toastWait(after, primed)
		if err != nil {
			time.Sleep(trayRefresh) // detached; the estate poll says so
			continue
		}
		if !primed {
			after, primed = resp.GetLatest(), true
			continue
		}
		w.deliver(resp.GetToasts())
		after = max(after, resp.GetLatest())
	}
}

// toastWait is one rig.toast.wait. Unprimed, it asks with no wait at all,
// only to learn the latest cursor.
func toastWait(after uint64, wait bool) (*registryv1.ToastWaitResponse, error) {
	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	timeout := time.Duration(0)
	if wait {
		timeout = toastPoll
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+readDeadline)
	defer cancel()
	resp := &registryv1.ToastWaitResponse{}
	err = c.Call(ctx, "rig.toast.wait", &registryv1.ToastWaitRequest{
		After: after, TimeoutMs: uint32(timeout.Milliseconds()),
	}, resp)
	return resp, err
}

// spawnToasts runs this binary again as the renderer.
func spawnToasts(after uint64) (<-chan error, error) {
	exe, err := selfExecutable()
	if err != nil {
		return nil, err
	}
	//rig:allow nocontextfree: the renderer ends itself when its last bubble leaves, so its end is the exit channel
	cmd := exec.CommandContext(context.Background(), exe, "--toasts", "--after", strconv.FormatUint(after, 10))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

// freedesktopUrgency maps the five severities onto the spec's three levels:
// low 0, normal 1, critical 2. Critical stays until dismissed, which is what
// error and urgent ask for.
func freedesktopUrgency(s registryv1.Severity) byte {
	switch s {
	case registryv1.Severity_SEVERITY_ERROR, registryv1.Severity_SEVERITY_URGENT:
		return 2
	case registryv1.Severity_SEVERITY_INFO:
		return 0
	default:
		return 1
	}
}

// notifyDesktop is the fallback: org.freedesktop.Notifications on the session
// bus, the service GNOME, KDE and every notification daemon implement.
func notifyDesktop(t *registryv1.Toast) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	sev := severityWord(t.GetSeverity())
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	return obj.Call("org.freedesktop.Notifications.Notify", 0,
		"rig", uint32(0), "", "["+sev+"] "+t.GetTitle(), t.GetBody(), []string{},
		map[string]dbus.Variant{"urgency": dbus.MakeVariant(freedesktopUrgency(t.GetSeverity()))},
		int32(-1)).Err
}

func severityWord(s registryv1.Severity) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "SEVERITY_"))
}

// --- the renderer's half -----------------------------------------------------

// toastJSON is one toast as the page draws it.
type toastJSON struct {
	Seq      uint64 `json:"seq"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Sender   string `json:"sender"`
}

// toastFeed is the renderer's state between the daemon and the page: what has
// arrived and not yet been handed to the page, and what the page last said
// about itself.
type toastFeed struct {
	mu      sync.Mutex
	pending []toastJSON
	shown   int       // bubbles on the page, as it last reported
	height  int       // the page's content height, as it last reported
	emptyAt time.Time // when the page last went from some bubbles to none
	started bool      // a bubble has been handed to the page
}

func (f *toastFeed) add(ts []*registryv1.Toast) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range ts {
		f.pending = append(f.pending, toastJSON{
			Seq: t.GetSeq(), Severity: severityWord(t.GetSeverity()),
			Title: t.GetTitle(), Body: t.GetBody(), Sender: t.GetSender(),
		})
	}
}

// poll records the page's report and hands it whatever is new.
func (f *toastFeed) poll(shown, height int, now time.Time) []toastJSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.pending
	f.pending = nil
	if len(out) > 0 {
		f.started = true
	}
	if f.shown > 0 && shown == 0 {
		f.emptyAt = now
	}
	f.shown, f.height = shown, height
	return out
}

// done is true once every bubble has left, nothing is waiting, and the page
// has been empty for a moment - long enough for a toast sent right after the
// last one to land in the same renderer rather than start another.
func (f *toastFeed) done(now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started && f.shown == 0 && len(f.pending) == 0 &&
		!f.emptyAt.IsZero() && now.Sub(f.emptyAt) > toastLinger
}

// size is the height to give the window: the page's, while it shows any
// bubble. An empty page keeps its last size until the renderer quits.
func (f *toastFeed) size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.shown == 0 {
		return 0
	}
	return f.height
}

// ServeHTTP is the page's only door to Go: the page itself, and one poll.
func (f *toastFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/toast.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(toastPage)
	case "/toast/poll":
		shown, _ := strconv.Atoi(r.URL.Query().Get("shown"))
		height, _ := strconv.Atoi(r.URL.Query().Get("h"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.poll(max(shown, 0), max(height, 0), time.Now()))
	default:
		http.NotFound(w, r)
	}
}

// runToasts is the renderer process: one frameless, transparent,
// always-on-top window at the tray's corner, sized to its bubbles, gone when
// they are.
func runToasts(after uint64) error {
	feed := &toastFeed{}
	app := application.New(application.Options{
		Name:     "rig toasts",
		Assets:   application.AssetOptions{Handler: feed},
		LogLevel: slog.LevelWarn,
	})
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "rig toasts",
		Width:          toastWidth,
		Height:         1,
		Hidden:         true,
		Frameless:      true,
		AlwaysOnTop:    true,
		DisableResize:  true,
		BackgroundType: application.BackgroundTypeTransparent,
		Linux:          application.LinuxWindow{WindowIsTranslucent: true},
		URL:            "/toast.html",
	})

	//rig:allow nocontextfree: the renderer lives until its last bubble leaves, so its end is app.Quit rather than a deadline
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		cursor := after
		for ctx.Err() == nil {
			resp, err := toastWait(cursor, true)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			feed.add(resp.GetToasts())
			for _, t := range resp.GetToasts() {
				cursor = max(cursor, t.GetSeq())
			}
		}
	}()
	go func() {
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		lastH := -1
		for range tick.C {
			if feed.done(time.Now()) {
				app.Quit()
				return
			}
			if h := feed.size(); h != lastH && h > 0 {
				lastH = h
				placeToasts(app, win, h)
			}
		}
	}()
	return app.Run()
}

// placeToasts sizes the window to the bubbles and puts it in the work area's
// top-right corner, where GNOME's tray sits.
func placeToasts(app *application.App, win *application.WebviewWindow, height int) {
	scr := app.Screen.GetPrimary()
	if scr == nil {
		return
	}
	wa := scr.WorkArea
	h := min(height, wa.Height-2*toastMargin)
	x, y := wa.X+wa.Width-toastWidth-toastMargin, wa.Y+toastMargin
	win.SetSize(toastWidth, h)
	if !win.IsVisible() {
		win.Show()
	}
	// After Show as well as before it: GTK drops a move asked of a window
	// that is not yet mapped, which put the stack top-left under Xvfb.
	win.SetPosition(x, y)
}

// toastAfter parses --after for the renderer.
func toastAfter(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("--after %q: %w", s, err)
	}
	return v, nil
}
