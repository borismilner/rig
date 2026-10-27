package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// The GNOME Shell backend (plan section 58). A Shell extension, shipped in
// shellext/rigcapture@rig, owns org.rig.Capture on the user's session bus and
// paints window actors and the stage to PNG from inside the compositor. That
// is the only place on GNOME Wayland that can see a window which is covered,
// minimized or on another workspace without asking the person each time.
//
// The contract is version 1: Version, ListWindows, ListOutputs,
// CaptureWindow, CaptureArea. A newer extension with a different API number
// is refused rather than half understood.

const (
	busName  = "org.rig.Capture"
	busPath  = "/org/rig/Capture"
	busIface = "org.rig.Capture"
	// APIVersion is the extension contract this file speaks.
	APIVersion = 1
	// maxReply bounds a JSON reply from the extension.
	maxReply = 4 << 20
)

// GNOME talks to the extension. The zero value is not usable; use NewGNOME.
type GNOME struct {
	dial func() (*dbus.Conn, error)
	mu   sync.Mutex
	conn *dbus.Conn
}

// NewGNOME connects lazily to the session bus. dial may be nil.
func NewGNOME(dial func() (*dbus.Conn, error)) *GNOME {
	if dial == nil {
		dial = func() (*dbus.Conn, error) { return dbus.ConnectSessionBus() }
	}
	return &GNOME{dial: dial}
}

// Close drops the bus connection.
func (g *GNOME) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn == nil {
		return nil
	}
	err := g.conn.Close()
	g.conn = nil
	return err
}

func (g *GNOME) bus() (*dbus.Conn, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conn != nil && g.conn.Connected() {
		return g.conn, nil
	}
	if g.conn != nil {
		_ = g.conn.Close()
		g.conn = nil
	}
	c, err := g.dial()
	if err != nil {
		return nil, unavailable("the session bus cannot be reached: " + err.Error())
	}
	g.conn = c
	return c, nil
}

func unavailable(why string) error {
	return fmt.Errorf("%w: %s", ErrUnavailable, why)
}

// call runs one method and returns its single string result. It redials once
// if the bus connection died between calls.
func (g *GNOME) call(ctx context.Context, method string, args ...any) (string, error) {
	var lastErr error
	for range 2 {
		c, err := g.bus()
		if err != nil {
			return "", err
		}
		call := c.Object(busName, busPath).CallWithContext(ctx, busIface+"."+method, 0, args...)
		if call.Err == nil {
			var out string
			if err := call.Store(&out); err != nil {
				return "", fmt.Errorf("%w: %s answered with the wrong shape: %w", ErrBadImage, method, err)
			}
			if len(out) > maxReply {
				return "", fmt.Errorf("%w: %s answered with %d bytes", ErrTooLarge, method, len(out))
			}
			return out, nil
		}
		lastErr = call.Err
		if errors.Is(call.Err, dbus.ErrClosed) || !c.Connected() {
			g.mu.Lock()
			if g.conn == c {
				_ = g.conn.Close()
				g.conn = nil
			}
			g.mu.Unlock()
			continue
		}
		break
	}
	return "", mapBusError(ctx, lastErr)
}

// mapBusError turns a D-Bus failure into one of this package's refusals.
func mapBusError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w (%w)", ErrTimeout, ctx.Err())
	}
	var de dbus.Error
	if !errors.As(err, &de) {
		return unavailable(err.Error())
	}
	msg := firstString(de.Body)
	switch de.Name {
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner":
		return unavailable("the rig capture extension is not running in the GNOME Shell")
	case busIface + ".Error.NoSuchWindow":
		return fmt.Errorf("%w: %s", ErrNoTarget, orElse(msg, "the window is gone"))
	case busIface + ".Error.BadArgument":
		return fmt.Errorf("%w: %s", ErrInvalid, msg)
	case busIface + ".Error.TooLarge":
		return fmt.Errorf("%w: %s", ErrTooLarge, msg)
	case busIface + ".Error.Failed":
		return fmt.Errorf("%w: the compositor could not paint it: %s", ErrBadImage, msg)
	}
	return unavailable(de.Name + ": " + msg)
}

func firstString(body []any) string {
	for _, b := range body {
		if s, ok := b.(string); ok {
			return s
		}
	}
	return ""
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Info reports the extension's API number and the Shell version.
func (g *GNOME) Info(ctx context.Context) (Info, error) {
	c, err := g.bus()
	if err != nil {
		return Info{}, err
	}
	call := c.Object(busName, busPath).CallWithContext(ctx, busIface+".Version", 0)
	if call.Err != nil {
		return Info{}, mapBusError(ctx, call.Err)
	}
	var in Info
	if err := call.Store(&in.API, &in.Shell); err != nil {
		return Info{}, fmt.Errorf("%w: Version answered with the wrong shape: %w", ErrBadImage, err)
	}
	if in.API != APIVersion {
		return in, unavailable(fmt.Sprintf("the extension speaks API %d and this rig speaks %d; reinstall the extension", in.API, APIVersion))
	}
	return in, nil
}

type jsonWindow struct {
	ID         uint64 `json:"id"`
	Title      string `json:"title"`
	App        string `json:"app"`
	PID        int    `json:"pid"`
	Workspace  int    `json:"workspace"`
	Minimized  bool   `json:"minimized"`
	Focused    bool   `json:"focused"`
	Visible    bool   `json:"visible"`
	Fullscreen bool   `json:"fullscreen"`
	Modal      bool   `json:"modal"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	W          int    `json:"w"`
	H          int    `json:"h"`
}

// Windows lists the top-level windows, bottom of the stack first.
func (g *GNOME) Windows(ctx context.Context) ([]Window, error) {
	if _, err := g.Info(ctx); err != nil {
		return nil, err
	}
	s, err := g.call(ctx, "ListWindows")
	if err != nil {
		return nil, err
	}
	var in []jsonWindow
	if err := json.Unmarshal([]byte(s), &in); err != nil {
		return nil, fmt.Errorf("%w: ListWindows is not JSON: %w", ErrBadImage, err)
	}
	out := make([]Window, 0, len(in))
	for _, w := range in {
		out = append(out, Window(w))
	}
	return out, nil
}

// Outputs lists the monitors.
func (g *GNOME) Outputs(ctx context.Context) ([]Output, error) {
	s, err := g.call(ctx, "ListOutputs")
	if err != nil {
		return nil, err
	}
	var out []Output
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("%w: ListOutputs is not JSON: %w", ErrBadImage, err)
	}
	return out, nil
}

type jsonRaw struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
	Cursor string  `json:"cursor"`
	Frame  *struct {
		X, Y, W, H int
	} `json:"frame"`
}

func parseRaw(s string) (Raw, error) {
	var j jsonRaw
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		return Raw{}, fmt.Errorf("%w: the capture reply is not JSON: %w", ErrBadImage, err)
	}
	r := Raw{Width: j.Width, Height: j.Height, Scale: j.Scale, Cursor: j.Cursor}
	if j.Frame != nil {
		r.Frame = Rect{X: j.Frame.X, Y: j.Frame.Y, W: j.Frame.W, H: j.Frame.H}
	}
	return r, nil
}

// CaptureWindow paints one window, wherever it is.
func (g *GNOME) CaptureWindow(ctx context.Context, id uint64, path string) (Raw, error) {
	if err := checkPath(path); err != nil {
		return Raw{}, err
	}
	s, err := g.call(ctx, "CaptureWindow", id, path)
	if err != nil {
		return Raw{}, err
	}
	return parseRaw(s)
}

// CaptureArea captures a rectangle of the desktop as it is displayed.
func (g *GNOME) CaptureArea(ctx context.Context, r Rect, cursor bool, path string) (Raw, error) {
	if err := checkPath(path); err != nil {
		return Raw{}, err
	}
	s, err := g.call(ctx, "CaptureArea", int32(r.X), int32(r.Y), int32(r.W), int32(r.H), cursor, path) //nolint:gosec // Check bounds an area to maxAreaSide, and outputs come from the compositor
	if err != nil {
		return Raw{}, err
	}
	return parseRaw(s)
}

// checkPath refuses a path the extension should never be asked to write.
func checkPath(p string) error {
	if !strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) || strings.Contains(p, "..") {
		return fmt.Errorf("%w: capture path %q", ErrInvalid, p)
	}
	return nil
}
