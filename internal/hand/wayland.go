package hand

// Driving a GNOME Wayland desktop.
//
// Why a second backend: GNOME 50 (Ubuntu 26.04) has no Xorg session at all.
// DISPLAY still answers, but it is XWayland, and XTEST events synthesised there
// reach XWayland clients only. The shell, GTK4 apps, Terminator and Chrome never
// see them, and nothing reports an error - the worst possible failure for a
// tool whose whole job is to act. So Open detects XWayland and comes here.
//
// Input goes through mutter's own remote-desktop API (org.gnome.Mutter.
// RemoteDesktop), the one gnome-remote-desktop uses. It needs no consent
// prompt from inside the session, and it gives what XTEST gave: absolute
// pointer motion, buttons, the wheel and keys. Absolute motion is expressed
// against a screen-cast stream, so one stream is recorded per monitor and a
// point is sent to the stream of the monitor it lies on. While a script runs
// GNOME shows its screen-sharing indicator in the top bar; that is the price of
// absolute coordinates, and it doubles as a visible "an agent is driving" sign.
//
// Keys are sent as keysyms, not keycodes: mutter resolves them against the
// keymap it actually has, including the Shift level. But only in the ACTIVE
// layout: with Hebrew selected, a Latin "a" has no key and mutter drops it
// without an error (measured 30/09/2026: "tag 2026.7.3 ok" arrived as
// " 2026.7.3 "). AgentBox answered by switching layout with the human's own
// shortcut per run of letters. rig does not switch (Boris, 2026-10-01): every
// switch restarted Espanso's worker and drew on his screen. Text the active
// layout cannot type is pasted instead, through both selections, and his
// clipboard is put back after; a shortcut such as ctrl+c is sent by key
// position, as a physical keyboard sends it, so it works in any layout.
// Hebrew letters are sent as the legacy hebrew_* keysyms, because that is
// what the il layout carries; the Unicode keysyms are not on any key.
//
// Windows come from the window-calls GNOME Shell extension
// (window-calls@domandoman.xyz), because Wayland gives an ordinary client no way
// to list, locate or raise another application's windows. Without it, screen
// scripts still run and `window TITLE` fails saying what to install.
//
// What Wayland takes away, stated so nobody relies on it:
//
//   - The pointer position cannot be read. agentbox knows where it put the
//     pointer and starts from XWayland's last known position, which is stale if
//     the human has since moved over a Wayland window. Coordinates relative to
//     the pointer are relative to where agentbox last put it.
//   - The stacking order is not exposed, so "which window is under the pointer"
//     is judged from the focused window and the target's rectangle rather than
//     read from the server. After a raise the target is on top, which is what
//     makes that judgement sound in practice.
//   - A modal dialog is a separate window with no transient-for visible here, so
//     typing into a target's dialog is refused rather than allowed.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/godbus/dbus/v5"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	rdName  = "org.gnome.Mutter.RemoteDesktop"
	rdPath  = "/org/gnome/Mutter/RemoteDesktop"
	scName  = "org.gnome.Mutter.ScreenCast"
	scPath  = "/org/gnome/Mutter/ScreenCast"
	dcName  = "org.gnome.Mutter.DisplayConfig"
	dcPath  = "/org/gnome/Mutter/DisplayConfig"
	wcName  = "org.gnome.Shell"
	wcPath  = "/org/gnome/Shell/Extensions/Windows"
	wcIface = "org.gnome.Shell.Extensions.Windows"

	windowCallsUUID = "window-calls@domandoman.xyz"

	// evdev button codes (linux/input-event-codes.h).
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
	btnSide   = 0x113
	btnExtra  = 0x114
)

// wlOutput is one monitor in the logical layout, with the stream that absolute
// motion on it is addressed to.
type wlOutput struct {
	Rect
	connector string
	stream    dbus.ObjectPath
}

type wlSession struct {
	bus     *dbus.Conn
	session dbus.BusObject
	outputs []wlOutput
	pos     Pt
}

// onWayland reports whether this desktop must be driven through mutter rather
// than XTEST: the X server is XWayland, or there is no X server but there is a
// Wayland one.
func onWayland() bool {
	conn, err := xgb.NewConn()
	if err != nil {
		return os.Getenv("WAYLAND_DISPLAY") != ""
	}
	defer conn.Close()
	r, err := xproto.QueryExtension(conn, uint16(len("XWAYLAND")), "XWAYLAND").Reply()
	return err == nil && r != nil && r.Present
}

// xwaylandPointer is the best available guess at where the pointer is when a
// script starts: XWayland's last known position.
func xwaylandPointer() (Pt, bool) {
	conn, err := xgb.NewConn()
	if err != nil {
		return Pt{}, false
	}
	defer conn.Close()
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	r, err := xproto.QueryPointer(conn, root).Reply()
	if err != nil || r == nil {
		return Pt{}, false
	}
	return Pt{X: int(r.RootX), Y: int(r.RootY)}, true
}

func openWayland(seed int64) (*Hand, error) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("no session bus to reach the Wayland compositor: %w", err)
	}
	wl := &wlSession{bus: bus}
	if err := wl.start(); err != nil {
		bus.Close()
		return nil, err
	}

	h := &Hand{
		wl:     wl,
		speed:  1,
		wpm:    defaultWPM,
		settle: 90 * time.Millisecond,
	}
	h.screen = wl.bounds()
	h.frame = h.screen
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	h.rnd = rand.New(rand.NewSource(seed)) //nolint:gosec // motion jitter, not a secret
	if p, ok := xwaylandPointer(); ok {
		wl.pos = p
	} else {
		wl.pos = Pt{X: h.screen.X + h.screen.W/2, Y: h.screen.Y + h.screen.H/2}
	}
	return h, nil
}

// start opens a remote-desktop session with one screen-cast stream per monitor
// and starts it. Streams must be recorded before Start; mutter rejects them after.
func (wl *wlSession) start() error {
	outs, err := wl.monitors()
	if err != nil {
		return err
	}
	var path dbus.ObjectPath
	if err := wl.bus.Object(rdName, rdPath).Call(rdName+".CreateSession", 0).Store(&path); err != nil {
		return fmt.Errorf("mutter refused a remote-desktop session: %w", err)
	}
	wl.session = wl.bus.Object(rdName, path)
	v, err := wl.session.GetProperty(rdName + ".Session.SessionId")
	if err != nil {
		return fmt.Errorf("reading the remote-desktop session id: %w", err)
	}
	id, _ := v.Value().(string)

	var sc dbus.ObjectPath
	opts := map[string]dbus.Variant{"remote-desktop-session-id": dbus.MakeVariant(id)}
	if err := wl.bus.Object(scName, scPath).Call(scName+".CreateSession", 0, opts).Store(&sc); err != nil {
		return fmt.Errorf("mutter refused a screen-cast session: %w", err)
	}
	for i := range outs {
		var st dbus.ObjectPath
		// cursor-mode 0 (hidden): nothing reads these streams, they exist only to
		// give absolute motion a coordinate space.
		o := map[string]dbus.Variant{"cursor-mode": dbus.MakeVariant(uint32(0))}
		if err := wl.bus.Object(scName, sc).Call(scName+".Session.RecordMonitor", 0, outs[i].connector, o).Store(&st); err != nil {
			return fmt.Errorf("recording monitor %s: %w", outs[i].connector, err)
		}
		outs[i].stream = st
	}
	wl.outputs = outs
	if err := wl.session.Call(rdName+".Session.Start", 0).Err; err != nil {
		return fmt.Errorf("starting the remote-desktop session: %w", err)
	}
	return nil
}

func (wl *wlSession) close() {
	if wl.session != nil {
		_ = wl.session.Call(rdName+".Session.Stop", 0).Err
		wl.session = nil
	}
	if wl.bus != nil {
		wl.bus.Close()
		wl.bus = nil
	}
}

// monitors reads the logical layout from mutter: where each active monitor sits
// and how big it is in logical pixels, rotation and scale applied.
func (wl *wlSession) monitors() ([]wlOutput, error) {
	type mode struct {
		ID                      string
		Width, Height           int32
		Refresh, PreferredScale float64
		SupportedScales         []float64
		Props                   map[string]dbus.Variant
	}
	type monitor struct {
		Spec  struct{ Connector, Vendor, Product, Serial string }
		Modes []mode
		Props map[string]dbus.Variant
	}
	type logical struct {
		X, Y      int32
		Scale     float64
		Transform uint32
		Primary   bool
		Monitors  []struct{ Connector, Vendor, Product, Serial string }
		Props     map[string]dbus.Variant
	}
	var (
		serial   uint32
		mons     []monitor
		logicals []logical
		props    map[string]dbus.Variant
	)
	call := wl.bus.Object(dcName, dcPath).Call(dcName+".GetCurrentState", 0)
	if err := call.Store(&serial, &mons, &logicals, &props); err != nil {
		return nil, fmt.Errorf("reading the monitor layout from mutter: %w", err)
	}
	current := map[string]mode{}
	for _, m := range mons {
		for _, md := range m.Modes {
			if v, ok := md.Props["is-current"]; ok {
				if b, _ := v.Value().(bool); b {
					current[m.Spec.Connector] = md
				}
			}
		}
	}
	var outs []wlOutput
	for _, l := range logicals {
		if len(l.Monitors) == 0 {
			continue
		}
		c := l.Monitors[0].Connector
		md, ok := current[c]
		if !ok {
			continue
		}
		w, hgt := float64(md.Width), float64(md.Height)
		if l.Transform%2 == 1 { // 90 and 270, flipped or not
			w, hgt = hgt, w
		}
		scale := l.Scale
		if scale <= 0 {
			scale = 1
		}
		outs = append(outs, wlOutput{
			Rect:      Rect{X: int(l.X), Y: int(l.Y), W: int(w / scale), H: int(hgt / scale)},
			connector: c,
		})
	}
	if len(outs) == 0 {
		return nil, errors.New("mutter reports no active monitor to drive")
	}
	return outs, nil
}

// bounds is the rectangle every monitor fits in.
func (wl *wlSession) bounds() Rect {
	minX, minY, maxX, maxY := wl.outputs[0].X, wl.outputs[0].Y, 0, 0
	for _, o := range wl.outputs {
		minX, minY = min(minX, o.X), min(minY, o.Y)
		maxX, maxY = max(maxX, o.X+o.W), max(maxY, o.Y+o.H)
	}
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}

// outputFor is the monitor a point lies on, or the nearest one when the point
// is in a gap of the layout (the bounding box is not always covered).
func (wl *wlSession) outputFor(p Pt) (wlOutput, Pt) {
	best, bestD := wl.outputs[0], -1
	for _, o := range wl.outputs {
		cx := min(max(p.X, o.X), o.X+o.W-1)
		cy := min(max(p.Y, o.Y), o.Y+o.H-1)
		d := (cx-p.X)*(cx-p.X) + (cy-p.Y)*(cy-p.Y)
		if bestD < 0 || d < bestD {
			best, bestD = o, d
		}
	}
	cx := min(max(p.X, best.X), best.X+best.W-1)
	cy := min(max(p.Y, best.Y), best.Y+best.H-1)
	return best, Pt{X: cx, Y: cy}
}

func (wl *wlSession) motion(p Pt) error {
	o, at := wl.outputFor(p)
	err := wl.session.Call(rdName+".Session.NotifyPointerMotionAbsolute", 0,
		string(o.stream), float64(at.X-o.X), float64(at.Y-o.Y)).Err
	if err == nil {
		wl.pos = at
	}
	return err
}

// button maps an X button number onto what mutter wants: evdev codes for real
// buttons, discrete axis steps for the wheel (4/5 vertical, 6/7 horizontal).
// The wheel turns on press only; its release is not an event.
func (wl *wlSession) button(b byte, down bool) error {
	var code int32
	switch b {
	case 1:
		code = btnLeft
	case 2:
		code = btnMiddle
	case 3:
		code = btnRight
	case 8:
		code = btnSide
	case 9:
		code = btnExtra
	case 4, 5, 6, 7:
		if !down {
			return nil
		}
		axis, steps := uint32(0), int32(1)
		if b == 4 || b == 6 {
			steps = -1
		}
		if b >= 6 {
			axis = 1
		}
		return wl.session.Call(rdName+".Session.NotifyPointerAxisDiscrete", 0, axis, steps).Err
	default:
		return fmt.Errorf("button %d has no Wayland equivalent", b)
	}
	return wl.session.Call(rdName+".Session.NotifyPointerButton", 0, code, down).Err
}

func (wl *wlSession) keysym(ks uint32, down bool) error {
	return wl.session.Call(rdName+".Session.NotifyKeyboardKeysym", 0, ks, down).Err
}

func (wl *wlSession) keycode(code uint32, down bool) error {
	return wl.session.Call(rdName+".Session.NotifyKeyboardKeycode", 0, code, down).Err
}

// --- windows, through window-calls ------------------------------------------

type wlWindow struct {
	ID        uint32 `json:"id"`
	Title     string `json:"title"`
	Class     string `json:"wm_class"`
	Focus     bool   `json:"focus"`
	InCurrent bool   `json:"in_current_workspace"`
	Minimised bool   `json:"minimized"` //nolint:misspell // the extension's field name
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func (w wlWindow) rect() Rect { return Rect{X: w.X, Y: w.Y, W: w.Width, H: w.Height} }

func (w wlWindow) info() winInfo { return winInfo{Win: w.ID, Name: w.Title, Class: w.Class} }

func (wl *wlSession) callWindows(method string, out *string, args ...any) error {
	c := wl.bus.Object(wcName, wcPath).Call(wcIface+"."+method, 0, args...)
	if c.Err != nil {
		return fmt.Errorf("listing windows needs the GNOME Shell extension %s "+
			"(gext install %s; gnome-extensions enable %s): %w",
			windowCallsUUID, windowCallsUUID, windowCallsUUID, c.Err)
	}
	if out != nil {
		return c.Store(out)
	}
	return nil
}

// windows lists the windows on the current workspace, with their rectangles.
// List carries no geometry, so each window's Details is read as well.
func (wl *wlSession) windows() ([]wlWindow, error) {
	var raw string
	if err := wl.callWindows("List", &raw); err != nil {
		return nil, err
	}
	var list []wlWindow
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("reading the window list: %w", err)
	}
	var out []wlWindow
	for _, w := range list {
		if !w.InCurrent {
			continue
		}
		d, err := wl.window(w.ID)
		if err != nil {
			continue // closed between the two calls
		}
		out = append(out, d)
	}
	return out, nil
}

func (wl *wlSession) window(id uint32) (wlWindow, error) {
	var raw string
	if err := wl.callWindows("Details", &raw, id); err != nil {
		return wlWindow{}, err
	}
	var w wlWindow
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return wlWindow{}, fmt.Errorf("reading window details: %w", err)
	}
	return w, nil
}

func (wl *wlSession) activate(id uint32) error {
	if err := wl.callWindows("Activate", nil, id); err != nil {
		return fmt.Errorf("cannot ask GNOME to raise the window: %w", err)
	}
	return nil
}

func (wl *wlSession) focused() (wlWindow, bool, error) {
	ws, err := wl.windows()
	if err != nil {
		return wlWindow{}, false, err
	}
	for _, w := range ws {
		if w.Focus {
			return w, true, nil
		}
	}
	return wlWindow{}, false, nil
}

// --- the Hand methods, Wayland side -----------------------------------------

func (h *Hand) windowsWL() ([]Candidate, error) {
	ws, err := h.wl.windows()
	if err != nil {
		return nil, err
	}
	var cands []Candidate
	for i, w := range ws {
		// Details reports a minimised window with its old rectangle; it is not
		// on screen, but UseWindow raises what it picks, so it stays a candidate.
		// The focused window ranks as the top of the stack, which is the only
		// stacking fact Wayland gives away.
		order := i + 1
		if w.Focus {
			order = len(ws) + 1
		}
		if w.Width < 16 || w.Height < 16 || w.Title == "" {
			continue
		}
		cands = append(cands, Candidate{Name: w.Title, Rect: w.rect(), Order: order, Win: w.ID})
	}
	return cands, nil
}

func (h *Hand) viewableWL(id uint32) (Rect, bool) {
	w, err := h.wl.window(id)
	if err != nil || w.Minimised || w.Width < 16 || w.Height < 16 {
		return Rect{}, false
	}
	return w.rect(), true
}

// pointerChainWL is the window the pointer is over, as far as Wayland lets it be
// known: the focused window if the pointer is inside it (it is on top of what it
// overlaps), else the locked target if the pointer is inside that, else the
// first window that contains the pointer.
func (h *Hand) pointerChainWL() ([]winInfo, error) {
	ws, err := h.wl.windows()
	if err != nil {
		return nil, err
	}
	p := h.wl.pos
	inside := func(w wlWindow) bool {
		return !w.Minimised && p.X >= w.X && p.X < w.X+w.Width && p.Y >= w.Y && p.Y < w.Y+w.Height
	}
	for _, w := range ws {
		if w.Focus && inside(w) {
			return []winInfo{w.info()}, nil
		}
	}
	for _, w := range ws {
		if w.ID == uint32(h.targetWin) && inside(w) {
			return []winInfo{w.info()}, nil
		}
	}
	for _, w := range ws {
		if inside(w) && w.Title != "" {
			return []winInfo{w.info()}, nil
		}
	}
	// X always ends a chain at the root, and describeChain relies on a chain
	// never being empty; the bare desktop is this backend's root.
	return []winInfo{{Name: "the desktop", IsRootLike: true}}, nil
}

func (h *Hand) focusChainWL() ([]winInfo, error) {
	w, ok, err := h.wl.focused()
	if err != nil || !ok {
		return nil, err
	}
	return []winInfo{w.info()}, nil
}

// --- input sources -----------------------------------------------------------

var sourceRe = regexp.MustCompile(`\('([^']*)', '([^']*)'\)`)

func gsetting(schema, key string) (string, error) {
	out, err := output(nil, "gsettings", "get", schema, key)
	if err != nil {
		return "", fmt.Errorf("reading %s %s: %w", schema, key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// sources is the human's input sources in MRU order; the first is active.
// GNOME rewrites mru-sources on every switch, which makes it the one readable
// record of which layout is live.
func sources() ([]string, error) {
	v, err := gsetting("org.gnome.desktop.input-sources", "mru-sources")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range sourceRe.FindAllStringSubmatch(v, -1) {
		ids = append(ids, m[2])
	}
	if len(ids) == 0 { // never switched: mru is empty, sources order holds
		v, err = gsetting("org.gnome.desktop.input-sources", "sources")
		if err != nil {
			return nil, err
		}
		for _, m := range sourceRe.FindAllStringSubmatch(v, -1) {
			ids = append(ids, m[2])
		}
	}
	return ids, nil
}

func isHebrewSource(id string) bool { return id == "il" || strings.HasPrefix(id, "il+") }

// sourceFor is the layout a rune must be typed in, or "" for a character every
// layout here carries (digits, punctuation, space). ok is false for a letter no
// configured layout has: mutter would drop it without a word, and a missing
// letter in typed text is a different text, so the caller refuses instead.
func sourceFor(r rune, ids []string) (id string, ok bool) {
	var want func(string) bool
	switch {
	case r >= 0x05d0 && r <= 0x05ea:
		want = isHebrewSource
	case unicode.In(r, unicode.Latin) && unicode.IsLetter(r):
		want = func(id string) bool { return !isHebrewSource(id) }
	case unicode.IsLetter(r):
		return "", false // a script none of the handled layouts carries
	default:
		return "", true
	}
	for _, id := range ids {
		if want(id) {
			return id, true
		}
	}
	return "", false
}

// keysymWL is KeysymFor, except for Hebrew: the il layout carries the legacy
// hebrew_aleph..hebrew_taw keysyms (0xce0..0xcfa), not the Unicode ones.
func keysymWL(r rune) uint32 {
	if r >= 0x05d0 && r <= 0x05ea {
		return 0x0ce0 + uint32(r-0x05d0)
	}
	return KeysymFor(r)
}

// run is a stretch of text typed one way: key by key on the active layout,
// or pasted because the active layout has no key for it.
type run struct {
	text  string
	paste bool
}

// splitRuns cuts text into runs. A digit, a space or punctuation between
// two pasted letters joins their run, so "שלום עולם" is one paste rather
// than two pastes and a typed space. Pure, so it is tested without a desktop.
func splitRuns(text string, ids []string) []run {
	// Layouts unreadable: no letter is known to be on the active layout, and
	// mutter drops a keysym it has no key for without a word, so every letter
	// is pasted. Seen when gsettings read an empty dconf.
	active := ""
	if len(ids) > 0 {
		active = ids[0]
	}
	rs := []rune(text)
	foreign := make([]bool, len(rs))
	neutral := make([]bool, len(rs))
	for i, r := range rs {
		id, ok := sourceFor(r, ids)
		switch {
		case !ok:
			foreign[i] = true // no layout here carries it; a paste can
		case id == "":
			neutral[i] = true
		default:
			foreign[i] = id != active
		}
	}
	// a neutral stretch joins a paste only when pastes lie on both sides
	for i := 0; i < len(rs); {
		if !neutral[i] {
			i++
			continue
		}
		j := i
		for j < len(rs) && neutral[j] {
			j++
		}
		if i > 0 && j < len(rs) && foreign[i-1] && foreign[j] {
			for k := i; k < j; k++ {
				foreign[k] = true
			}
		}
		i = j
	}
	var out []run
	for i, r := range rs {
		if n := len(out); n > 0 && out[n-1].paste == foreign[i] {
			out[n-1].text += string(r)
			continue
		}
		out = append(out, run{text: string(r), paste: foreign[i]})
	}
	return out
}

// activeOnly reports whether the active layout types r, or every layout does.
func activeOnly(r rune, ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	id, ok := sourceFor(r, ids)
	return ok && (id == "" || id == ids[0])
}

// typeWL types with keysyms, paced by the same planner as X, and pastes the
// runs the active layout cannot type (see the file comment).
func (h *Hand) typeWL(text string) error {
	ids, err := sources()
	if err != nil {
		ids = nil
	}
	runs := splitRuns(text, ids)
	for i, r := range runs {
		if r.paste {
			if err := h.parked(); err != nil {
				return err
			}
			h.trace("paste %d characters", len([]rune(r.text)))
			if err := h.pasteWL(r.text); err != nil {
				return err
			}
			continue
		}
		strokes, _ := PlanText(r.text, nil, Typing{WPM: h.wpm, Rand: h.rnd})
		for k, s := range strokes {
			if h.park != nil && h.park.Blocked() {
				if err := h.parked(); err != nil {
					return err
				}
				// He may have changed layout while he had the desktop, so
				// what is left is split again against the layout now live.
				var rest strings.Builder
				rest.WriteString(string([]rune(r.text)[k:]))
				for _, later := range runs[i+1:] {
					rest.WriteString(later.text)
				}
				return h.typeWL(rest.String())
			}
			time.Sleep(s.After)
			ks := keysymWL(s.Rune)
			if err := h.wl.keysym(ks, true); err != nil {
				return fmt.Errorf("typing %q: %w", s.Rune, err)
			}
			time.Sleep(time.Duration(18+h.rnd.Intn(22)) * time.Millisecond)
			if err := h.wl.keysym(ks, false); err != nil {
				return fmt.Errorf("releasing %q: %w", s.Rune, err)
			}
		}
	}
	return nil
}

// pasteSettle is how long the application has to read the selection before
// the human's own clipboard is put back. A paste is a request the
// application makes after the key, so restoring sooner hands it the old text.
var pasteSettle = 400 * time.Millisecond

// pasteWL puts text in BOTH selections and presses Shift+Insert, which GTK,
// Chrome and the terminals all take as paste (some from the primary, the rest
// from the clipboard). Insert is on every layout, which ctrl+v's v is not.
// Whatever the human had in each selection is restored afterwards.
func (h *Hand) pasteWL(text string) error {
	if _, err := exec.LookPath("wl-copy"); err != nil {
		return fmt.Errorf("text the active keyboard layout cannot type is pasted, and that needs wl-clipboard (wl-copy): %w", err)
	}
	clip, prim := saveSelection(false), saveSelection(true)
	defer func() {
		clip.restore(false)
		prim.restore(true)
	}()
	for _, primary := range []bool{false, true} {
		if err := setSelection(primary, "text/plain;charset=utf-8", []byte(text)); err != nil {
			return fmt.Errorf("putting text in the selection: %w", err)
		}
	}
	time.Sleep(60 * time.Millisecond)
	if err := h.pressWL("shift+Insert", modShift, keyInsert); err != nil {
		return err
	}
	time.Sleep(pasteSettle)
	return nil
}

const keyInsert = 0xff63

// maxSaved bounds what is kept of the human's selection to put back. A
// selection bigger than this is an image or a file, and it is cleared rather
// than held in memory: said here so nobody is surprised.
const maxSaved = 32 << 20

type savedSelection struct {
	mime string
	data []byte
	ok   bool
}

// saveSelection reads one selection in its most useful type: UTF-8 text if
// offered, else the first type offered.
func saveSelection(primary bool) savedSelection {
	list := []string{"--list-types"}
	if primary {
		list = append(list, "--primary")
	}
	out, err := output(nil, "wl-paste", list...)
	if err != nil {
		return savedSelection{} // empty, or no wl-paste
	}
	types := strings.Fields(string(out))
	if len(types) == 0 {
		return savedSelection{}
	}
	mime := types[0]
	for _, t := range types {
		if t == "text/plain;charset=utf-8" {
			mime = t
		}
	}
	args := []string{"--no-newline", "--type", mime}
	if primary {
		args = append(args, "--primary")
	}
	data, err := output(nil, "wl-paste", args...)
	if err != nil || len(data) > maxSaved {
		return savedSelection{}
	}
	return savedSelection{mime: mime, data: data, ok: true}
}

func (s savedSelection) restore(primary bool) {
	if s.ok {
		_ = setSelection(primary, s.mime, s.data)
		return
	}
	args := []string{"--clear"}
	if primary {
		args = append(args, "--primary")
	}
	_ = runTool(nil, "wl-copy", args...)
}

// setSelection hands data to wl-copy, which forks a server for it and exits.
func setSelection(primary bool, mime string, data []byte) error {
	args := []string{"--type", mime}
	if primary {
		args = append(args, "--primary")
	}
	return runTool(data, "wl-copy", args...)
}

// toolTimeout bounds one gsettings, wl-paste or wl-copy run; a compositor
// that stopped answering must not hang the script.
const toolTimeout = 5 * time.Second

// output runs a desktop tool with stdin and returns what it printed.
func output(stdin []byte, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	return cmd.Output()
}

// runTool runs a desktop tool without reading what it prints. wl-copy needs it:
// the server it forks keeps stdout open, so Output would wait for that server
// to lose the selection rather than for wl-copy to exit.
func runTool(stdin []byte, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	return cmd.Run()
}

// evdevLatin is where each Latin letter and digit sits on a US keyboard, as
// evdev numbers it (linux/input-event-codes.h). A shortcut is a key POSITION:
// ctrl+c in the Hebrew layout is the same key as in the Latin one, which is
// how a physical keyboard sends it and how applications match it.
var evdevLatin = map[rune]uint32{
	'q': 16, 'w': 17, 'e': 18, 'r': 19, 't': 20, 'y': 21, 'u': 22, 'i': 23, 'o': 24, 'p': 25,
	'a': 30, 's': 31, 'd': 32, 'f': 33, 'g': 34, 'h': 35, 'j': 36, 'k': 37, 'l': 38,
	'z': 44, 'x': 45, 'c': 46, 'v': 47, 'b': 48, 'n': 49, 'm': 50,
	'1': 2, '2': 3, '3': 4, '4': 5, '5': 6, '6': 7, '7': 8, '8': 9, '9': 10, '0': 11,
}

// pressCombo is Press on Wayland. A combination whose key the active layout
// does not carry goes by key position instead of being dropped.
func (h *Hand) pressCombo(spec string, mods uint16, ks uint32) error {
	if ks < 0x100 {
		ids, _ := sources()
		if !activeOnly(rune(ks), ids) {
			code, ok := evdevLatin[unicode.ToLower(rune(ks))]
			if !ok {
				return fmt.Errorf("pressing %q: the active keyboard layout has no %q key", spec, rune(ks))
			}
			return h.pressWith(spec, mods, func(down bool) error { return h.wl.keycode(code, down) })
		}
	}
	return h.pressWL(spec, mods, ks)
}

// pressWL sends one combination as keysyms, modifiers down in order and back up
// in reverse.
func (h *Hand) pressWL(spec string, mods uint16, ks uint32) error {
	return h.pressWith(spec, mods, func(down bool) error { return h.wl.keysym(ks, down) })
}

// pressWith holds the modifiers and taps the key tap sends.
func (h *Hand) pressWith(spec string, mods uint16, tap func(down bool) error) error {
	var down []uint32
	release := func() {
		for i := len(down) - 1; i >= 0; i-- {
			_ = h.wl.keysym(down[i], false)
			time.Sleep(15 * time.Millisecond)
		}
	}
	for _, m := range modKeysyms {
		if mods&m.mask == 0 {
			continue
		}
		if err := h.wl.keysym(m.keysym, true); err != nil {
			release()
			return fmt.Errorf("holding a modifier for %q: %w", spec, err)
		}
		down = append(down, m.keysym)
		time.Sleep(25 * time.Millisecond)
	}
	err := tap(true)
	if err == nil {
		time.Sleep(time.Duration(45+h.rnd.Intn(35)) * time.Millisecond)
		err = tap(false)
	}
	release()
	if err != nil {
		return fmt.Errorf("pressing %q: %w", spec, err)
	}
	return nil
}
