package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
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
//     the right-hand corner of the work area on the edge the panel is on,
//     and the newest bubble sits nearest it. The edge is read from the
//     work area rather than assumed: Boris's panel is at the BOTTOM, and the
//     first build's top-right anchor pointed away from his tray.
//   - NO SHAKE (Boris, 2026-09-27): the window is placed ONCE, as tall as
//     the work area, and never resized. Resizing it to the bubbles was a
//     resize then a move on every frame of a leaving bubble, and the stack
//     jumped between the two. The bubbles alone take the pointer: the page
//     reports where they are and the window's input region is set to that,
//     so a click anywhere else goes to what is underneath.

//go:embed toast.html
var toastPage []byte

const (
	toastWidth  = 410 // the window's width; the page's bubbles are 360 plus room for shadows
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
	spawn toastSpawn
	// fallback puts a toast on the desktop and answers the id it was given
	// there; closeDesktop takes one down by that id.
	fallback     func(*registryv1.Toast) (uint32, error)
	closeDesktop func(uint32) error
	warn         func(string)
	// grace is how soon an exit still counts as dying at start; zero means
	// five seconds.
	grace time.Duration

	mu      sync.Mutex
	running bool
	// onDesktop is what the fallback put up, newest last, and gone is what was
	// taken back; each keeps the newest desktopKept.
	onDesktop []desktopToast
	gone      []string
}

// desktopKept bounds what the tray remembers about the desktop's toasts. A
// withdrawal comes while its toast may still be up, so the newest suffice.
const desktopKept = 64

type desktopToast struct {
	record string
	id     uint32
}

// deliver is called with each batch the tray's wait returns. With no renderer
// alive it starts one from the first toast's cursor; the renderer then waits
// on the daemon itself. A renderer that cannot start, or dies before it
// could draw, sends the batch to the desktop's notification service.
func (w *toastWatcher) deliver(batch []*registryv1.Toast) {
	// A withdrawal starts nothing (plan/53 slice 7): a running renderer reads
	// it from the ring itself, and with none there is no bubble to take down.
	// What the desktop shows for it is taken down here.
	for _, t := range batch {
		if t.GetRetracted() {
			w.withdraw(t.GetRecordId())
		}
	}
	batch = slices.DeleteFunc(slices.Clone(batch), (*registryv1.Toast).GetRetracted)
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
		w.mu.Lock()
		taken := slices.Contains(w.gone, t.GetRecordId())
		w.mu.Unlock()
		if taken {
			continue // taken back before a renderer that died at start was replaced
		}
		id, err := w.fallback(t)
		if err != nil {
			w.warn("the desktop's notification service refused too; the toast is in the record only: " + err.Error())
			return
		}
		w.mu.Lock()
		w.onDesktop = keepNewest(append(w.onDesktop, desktopToast{t.GetRecordId(), id}))
		w.mu.Unlock()
	}
}

// withdraw takes down what the desktop shows for a toast taken back, and
// remembers it so a fallback still to come does not put it up.
func (w *toastWatcher) withdraw(record string) {
	w.mu.Lock()
	w.gone = keepNewest(append(w.gone, record))
	var ids []uint32
	w.onDesktop = slices.DeleteFunc(w.onDesktop, func(d desktopToast) bool {
		if d.record == record {
			ids = append(ids, d.id)
			return true
		}
		return false
	})
	w.mu.Unlock()
	for _, id := range ids {
		if err := w.closeDesktop(id); err != nil {
			w.warn("the desktop would not close a toast taken back, or had closed it already: " + err.Error())
		}
	}
}

func keepNewest[T any](s []T) []T {
	if len(s) > desktopKept {
		return slices.Delete(s, 0, len(s)-desktopKept)
	}
	return s
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
		After: after, TimeoutMs: uint32(timeout.Milliseconds()), //nolint:gosec // zero or toastPoll
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
func notifyDesktop(t *registryv1.Toast) (uint32, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	sev := severityWord(t.GetSeverity())
	var id uint32
	err = desktopNotifications(conn).Call("org.freedesktop.Notifications.Notify", 0,
		"rig", uint32(0), "", "["+sev+"] "+t.GetTitle(), t.GetBody(), []string{},
		map[string]dbus.Variant{"urgency": dbus.MakeVariant(freedesktopUrgency(t.GetSeverity()))},
		int32(-1)).Store(&id)
	return id, err
}

// closeDesktop takes down a notification notifyDesktop put up. The spec lets
// the service answer an error for one already dismissed or expired.
func closeDesktop(id uint32) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return desktopNotifications(conn).Call("org.freedesktop.Notifications.CloseNotification", 0, id).Err
}

func desktopNotifications(conn *dbus.Conn) dbus.BusObject {
	return conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
}

func severityWord(s registryv1.Severity) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "SEVERITY_"))
}

// --- the renderer's half -----------------------------------------------------

// toastJSON is one toast as the page draws it.
type toastJSON struct {
	Seq       uint64   `json:"seq"`
	RecordID  string   `json:"record_id"`
	Severity  string   `json:"severity"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Sender    string   `json:"sender"`
	Replies   []string `json:"replies,omitempty"`
	ReplyText bool     `json:"reply_text,omitempty"`
}

// answerJSON is a reply that arrived for a toast on the page, from this
// page or from anywhere else.
type answerJSON struct {
	RecordID  string `json:"record_id"`
	Reply     string `json:"reply"`
	Text      string `json:"text"`
	Dismissed bool   `json:"dismissed"`
	Withdrawn bool   `json:"withdrawn,omitempty"`
	By        string `json:"by"`
}

// pollJSON is what one poll hands the page, with the panel's edge on every
// poll: a one-off ExecJS can land before the page has loaded, and then the
// stack hugs the wrong edge of a window that never moves again.
type pollJSON struct {
	Toasts  []toastJSON  `json:"toasts"`
	Answers []answerJSON `json:"answers"`
	Edge    string       `json:"edge,omitempty"`
	MaxH    int          `json:"max_h,omitempty"`
}

// inputRegion is the band of the window the bubbles occupy, in the page's
// pixels from the window's top; everything outside it lets clicks through.
type inputRegion struct{ Top, Height int }

// toastFeed is the renderer's state between the daemon and the page: what has
// arrived and not yet been handed to the page, and what the page last said
// about itself.
type toastFeed struct {
	mu      sync.Mutex
	pending []toastJSON
	answers []answerJSON
	shown   int         // bubbles on the page, as it last reported
	region  inputRegion // where they are, as it last reported
	emptyAt time.Time   // when the page last went from some bubbles to none
	edge    string      // the panel's edge, once the window is placed
	maxH    int         // the window's height, once placed
	started bool        // a bubble has been handed to the page

	copyText func(string)                               // puts text on the clipboard; nil refuses
	reply    func(*registryv1.ToastReplyRequest) error  // files a reply; nil refuses
	watch    func(id string, answered func(answerJSON)) // follows a toast's answer; nil does nothing
}

// maxCopy bounds what one copy may put on the clipboard, and so what one
// request may make this process hold.
const maxCopy = 1 << 20

func (f *toastFeed) add(ts []*registryv1.Toast) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range ts {
		if t.GetRetracted() {
			f.withdraw(t)
			continue
		}
		f.pending = append(f.pending, toastJSON{
			Seq: t.GetSeq(), RecordID: t.GetRecordId(), Severity: severityWord(t.GetSeverity()),
			Title: t.GetTitle(), Body: t.GetBody(), Sender: t.GetSender(),
			Replies: t.GetReplies(), ReplyText: t.GetReplyText(),
		})
		if (len(t.GetReplies()) > 0 || t.GetReplyText()) && f.watch != nil {
			go f.watch(t.GetRecordId(), f.answered)
		}
	}
}

// withdraw takes down a toast its sender took back: one not yet handed to
// the page never reaches it, and one on the page is told.
func (f *toastFeed) withdraw(t *registryv1.Toast) {
	before := len(f.pending)
	f.pending = slices.DeleteFunc(f.pending, func(p toastJSON) bool { return p.RecordID == t.GetRecordId() })
	if len(f.pending) == before {
		f.answers = append(f.answers, answerJSON{RecordID: t.GetRecordId(), Withdrawn: true, By: t.GetSender()})
	}
}

// answered queues a reply for the page, wherever it was given.
func (f *toastFeed) answered(a answerJSON) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, a)
}

// poll records the page's report and hands it whatever is new.
func (f *toastFeed) poll(shown int, region inputRegion, now time.Time) pollJSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := pollJSON{Toasts: f.pending, Answers: f.answers, Edge: f.edge, MaxH: f.maxH}
	f.pending, f.answers = nil, nil
	if len(out.Toasts) > 0 {
		f.started = true
	}
	if f.shown > 0 && shown == 0 {
		f.emptyAt = now
	}
	f.shown, f.region = shown, region
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

// input is the band that takes the pointer, and whether any bubble is up.
// With none, the band is empty and every click goes through.
func (f *toastFeed) input() (inputRegion, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.shown == 0 {
		return inputRegion{}, false
	}
	return f.region, true
}

// ServeHTTP is the page's only door to Go: the page itself, and one poll.
func (f *toastFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/toast.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(toastPage)
	case "/toast/poll":
		q := r.URL.Query()
		shown, _ := strconv.Atoi(q.Get("shown"))
		top, _ := strconv.Atoi(q.Get("top"))
		height, _ := strconv.Atoi(q.Get("h"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.poll(max(shown, 0), inputRegion{max(top, 0), max(height, 0)}, time.Now()))
	case "/toast/reply":
		if r.Method != http.MethodPost || f.reply == nil {
			http.Error(w, "reply is POST, and only in the renderer", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			RecordID  string `json:"record_id"`
			Reply     string `json:"reply"`
			Text      string `json:"text"`
			Dismissed bool   `json:"dismissed"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCopy)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := f.reply(&registryv1.ToastReplyRequest{
			RecordId: req.RecordID, Reply: req.Reply, Text: req.Text, Dismissed: req.Dismissed,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusConflict) // the page shows it in the bubble
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "/toast/copy":
		if r.Method != http.MethodPost || f.copyText == nil {
			http.Error(w, "copy is POST, and only in the renderer", http.StatusMethodNotAllowed)
			return
		}
		text, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCopy))
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		f.copyText(string(text))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// runToasts is the renderer process: one frameless, transparent,
// always-on-top window at the tray's corner, sized to its bubbles, gone when
// they are.
func runToasts(after uint64) error {
	//rig:allow nocontextfree: the renderer lives until its last bubble leaves, so its end is app.Quit rather than a deadline
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed := &toastFeed{
		reply: replyToast,
		watch: func(id string, answered func(answerJSON)) { watchAnswer(ctx, id, answered) },
	}
	app := application.New(application.Options{
		Name:     "rig toasts",
		Assets:   application.AssetOptions{Handler: feed},
		LogLevel: slog.LevelWarn,
	})
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Rig toasts",
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
	feed.copyText = func(s string) { application.InvokeSync(func() { app.Clipboard.SetText(s) }) }

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
		placed := false
		last := inputRegion{-1, -1}
		for range tick.C {
			if feed.done(time.Now()) {
				app.Quit()
				return
			}
			band, up := feed.input()
			if up && !placed {
				placed = placeToasts(app, win, feed)
			}
			if placed && band != last {
				last = band
				application.InvokeSync(func() { setInputBand(win, band) })
			}
		}
	}()
	return app.Run()
}

// placeToasts puts the window, as tall as the work area, in its right-hand
// corner on the panel's edge. It runs once: the window never moves or
// resizes after, which is what keeps the stack still.
func placeToasts(app *application.App, win *application.WebviewWindow, feed *toastFeed) bool {
	scr := app.Screen.GetPrimary()
	if scr == nil {
		return false
	}
	wa := scr.WorkArea
	if x11, ok := application.InvokeSyncWithResult(func() workArea {
		r, ok := x11WorkArea()
		return workArea{r, ok}
	}).get(); ok && x11.Width > 0 && x11.Height > 0 {
		wa = x11
	}
	edge := panelEdge(scr.Bounds, wa)
	h := wa.Height - 2*toastMargin
	x, y := wa.X+wa.Width-toastWidth-toastMargin, wa.Y+toastMargin
	win.SetSize(toastWidth, h)
	themeOnce.Do(func() { application.InvokeSync(clearThemeBackground) })
	feed.mu.Lock()
	feed.edge, feed.maxH = edge, h
	feed.mu.Unlock()
	win.Show()
	// After Show as well as before it: GTK drops a move asked of a window
	// that is not yet mapped, which put the stack top-left under Xvfb.
	win.SetPosition(x, y)
	return true
}

// replyToast files a reply from a bubble. The renderer's connection is the
// replier, so rig records the person at this screen, not the sender.
func replyToast(req *registryv1.ToastReplyRequest) error {
	c, err := client.Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	return c.Call(ctx, "rig.toast.reply", req, &registryv1.ToastReplyResponse{})
}

// watchAnswer follows one asking toast until somebody answers it, here or
// anywhere else, so a bubble answered at a terminal does not stay up.
func watchAnswer(ctx context.Context, id string, answered func(answerJSON)) {
	for ctx.Err() == nil {
		c, err := client.Connect()
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, toastPoll+readDeadline)
		var resp registryv1.ToastAnswerResponse
		err = c.Call(cctx, "rig.toast.answer", &registryv1.ToastAnswerRequest{
			RecordId: id, TimeoutMs: uint32(toastPoll / time.Millisecond),
		}, &resp)
		cancel()
		c.Close()
		a := resp.GetAnswer()
		var ce *client.CallError
		switch {
		case errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND:
			return // a restarted daemon forgot the question; the bubble can still be closed
		case err != nil && ctx.Err() == nil:
			time.Sleep(time.Second)
		case a.GetAnswered():
			answered(answerJSON{RecordID: id, Reply: a.GetReply(), Text: a.GetText(), Dismissed: a.GetDismissed(), Withdrawn: a.GetWithdrawn(), By: a.GetBy()})
			return
		}
	}
}

var themeOnce sync.Once

type workArea struct {
	r  application.Rect
	ok bool
}

func (w workArea) get() (application.Rect, bool) { return w.r, w.ok }

// panelEdge says which edge of the screen the panel, and so the tray, is on:
// the side where the work area gives up the most room. With no panel at all
// it answers "top", GNOME's default.
func panelEdge(bounds, wa application.Rect) string {
	top := wa.Y - bounds.Y
	bottom := (bounds.Y + bounds.Height) - (wa.Y + wa.Height)
	if bottom > top {
		return "bottom"
	}
	return "top"
}

// toastAfter parses --after for the renderer.
func toastAfter(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("--after %q: %w", s, err)
	}
	return v, nil
}

// menuDND is the tray's Do Not Disturb row, a checkbox that shows the
// DAEMON's state: the daemon holds it, so the terminal and the tray cannot
// disagree for longer than one poll.
var menuDND *systray.MenuItem

// toastDND asks the daemon to change or report Do Not Disturb. A daemon that
// cannot be reached answers nil, and the row is left as it was.
func toastDND(change registryv1.DndChange) *registryv1.ToastDndResponse {
	c, err := client.Connect()
	if err != nil {
		return nil
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	resp := &registryv1.ToastDndResponse{}
	if err := c.Call(ctx, "rig.toast.dnd", &registryv1.ToastDndRequest{Change: change}, resp); err != nil {
		return nil
	}
	return resp
}

// showDND sets the row from the daemon's answer.
func showDND(resp *registryv1.ToastDndResponse) {
	if menuDND == nil || resp == nil {
		return
	}
	if resp.GetOn() {
		menuDND.Check()
		menuDND.SetTitle(fmt.Sprintf("Do Not Disturb (%d held)", resp.GetSuppressed()))
	} else {
		menuDND.Uncheck()
		menuDND.SetTitle("Do Not Disturb")
	}
}
