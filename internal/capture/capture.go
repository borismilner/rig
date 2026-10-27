// Package capture is plan section 58: rig can see the screen. A Backend
// reaches the compositor (today a GNOME Shell extension over D-Bus, gnome.go)
// and this package turns its raw PNG into what a caller asked for - a window,
// an output or an area, cropped, scaled, in PNG or JPEG - inside a bounded,
// private directory.
//
// Everything a caller can influence is checked here, before it reaches the
// compositor or the disk: sizes, pixel counts, formats, counts in flight.
// The compositor side is trusted no further than its answer is checked: the
// file it wrote is measured and decoded under the same limits.
package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The refusals a caller can act on. The daemon maps each to a wire status.
var (
	// ErrUnavailable: no compositor side answered.
	ErrUnavailable = errors.New("capture: no compositor backend")
	// ErrNoTarget: the window, output or area is not there.
	ErrNoTarget = errors.New("capture: no such target")
	// ErrAmbiguous: a match named more than one window.
	ErrAmbiguous = errors.New("capture: the match names several windows")
	// ErrInvalid: the request is malformed.
	ErrInvalid = errors.New("capture: invalid request")
	// ErrBusy: too many captures in flight.
	ErrBusy = errors.New("capture: too many captures in flight")
	// ErrTooLarge: the picture exceeds a limit.
	ErrTooLarge = errors.New("capture: the picture is over a limit")
	// ErrBadImage: the backend produced something that is not a picture.
	ErrBadImage = errors.New("capture: the backend returned an unreadable picture")
	// ErrTimeout: the compositor did not answer in time.
	ErrTimeout = errors.New("capture: the compositor did not answer in time")
)

// Rect is a rectangle in pixels.
type Rect struct{ X, Y, W, H int }

// Empty reports a rectangle with no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Window is one top-level window as the compositor lists it.
type Window struct {
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

// Output is one monitor.
type Output struct {
	Index   int     `json:"index"`
	Name    string  `json:"name"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	W       int     `json:"w"`
	H       int     `json:"h"`
	Scale   float64 `json:"scale"`
	Primary bool    `json:"primary"`
}

// Raw is what the backend reports about the PNG it wrote.
type Raw struct {
	Width, Height int
	// Frame is the window's own rectangle inside the picture, which also
	// holds its shadow. Empty for outputs and areas.
	Frame Rect
	Scale float64
}

// Info identifies the backend.
type Info struct {
	API   uint32
	Shell string
}

// Backend is the compositor side. An output is captured as an area of its
// geometry, so the backend has only two ways to take a picture. Each writes a PNG to path (a name the
// Service has just chosen in its own private directory) or returns an error
// wrapping one of this package's sentinels.
type Backend interface {
	Info(ctx context.Context) (Info, error)
	Windows(ctx context.Context) ([]Window, error)
	Outputs(ctx context.Context) ([]Output, error)
	CaptureWindow(ctx context.Context, id uint64, path string) (Raw, error)
	CaptureArea(ctx context.Context, r Rect, cursor bool, path string) (Raw, error)
}

// Kind says what is captured.
type Kind int

const (
	KindWindow Kind = iota + 1 // one window, by id or match
	KindOutput                 // one monitor
	KindScreen                 // every monitor, as one picture
	KindArea                   // a rectangle of the desktop
)

func (k Kind) String() string {
	switch k {
	case KindWindow:
		return "window"
	case KindOutput:
		return "output"
	case KindScreen:
		return "screen"
	case KindArea:
		return "area"
	}
	return "unknown"
}

// Target says what to capture. For a window give an ID, or Title and/or App
// (case-insensitive substrings). A match naming several windows is refused
// with the candidates, unless First is set, which takes the focused one among
// them and otherwise the topmost listed.
type Target struct {
	Kind   Kind
	ID     uint64
	Title  string
	App    string
	First  bool
	Output int
	Area   Rect
}

// Options says what to do with the picture.
type Options struct {
	// Format is "png" (default) or "jpeg".
	Format string
	// Quality is the JPEG quality, 1 to 100; default 85.
	Quality int
	// MaxDim shrinks the picture so neither side exceeds it; 0 keeps it.
	MaxDim int
	// Crop keeps this rectangle of the picture (after any frame crop).
	Crop Rect
	// KeepShadow keeps the window's shadow margin. Windows are cropped to
	// their frame unless this is set.
	KeepShadow bool
	// Cursor draws the pointer (outputs, screen, area only).
	Cursor bool
	// Inline returns the bytes as well, when at most this many; 0 returns
	// none. Capped by the Service's limit.
	Inline int
}

// Shot is the result of one capture.
type Shot struct {
	Path     string
	Format   string
	Bytes    int64
	Width    int
	Height   int
	SHA256   string
	Target   string
	Window   *Window
	Output   *Output
	TakenAt  time.Time
	Took     time.Duration
	Data     []byte
	Shadowed bool
}

// Limits bound everything. The zero value is replaced by Defaults.
type Limits struct {
	Timeout     time.Duration // per capture
	Concurrent  int           // captures in flight
	MaxBytes    int64         // one file as the backend wrote it
	MaxPixels   int64         // width * height of one picture
	KeepFiles   int           // files kept in the directory
	KeepBytes   int64         // bytes kept in the directory
	KeepAge     time.Duration // age after which a file is pruned
	InlineBytes int           // most bytes ever returned inline
}

// Defaults are the limits rigd starts with.
func Defaults() Limits {
	return Limits{
		Timeout:     15 * time.Second,
		Concurrent:  4,
		MaxBytes:    96 << 20,
		MaxPixels:   34_000_000, // 7680x4320 and a little over
		KeepFiles:   200,
		KeepBytes:   512 << 20,
		KeepAge:     24 * time.Hour,
		InlineBytes: 1 << 20,
	}
}

func (l Limits) filled() Limits {
	d := Defaults()
	if l.Timeout <= 0 {
		l.Timeout = d.Timeout
	}
	if l.Concurrent <= 0 {
		l.Concurrent = d.Concurrent
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxPixels <= 0 {
		l.MaxPixels = d.MaxPixels
	}
	if l.KeepFiles <= 0 {
		l.KeepFiles = d.KeepFiles
	}
	if l.KeepBytes <= 0 {
		l.KeepBytes = d.KeepBytes
	}
	if l.KeepAge <= 0 {
		l.KeepAge = d.KeepAge
	}
	if l.InlineBytes <= 0 {
		l.InlineBytes = d.InlineBytes
	}
	return l
}

// describe names a target for the log and the answer, never including more
// than the caller supplied.
func (t Target) describe() string {
	switch t.Kind {
	case KindWindow:
		switch {
		case t.ID != 0:
			return fmt.Sprintf("window %d", t.ID)
		default:
			var parts []string
			if t.Title != "" {
				parts = append(parts, "title~"+quote(t.Title))
			}
			if t.App != "" {
				parts = append(parts, "app~"+quote(t.App))
			}
			return "window " + strings.Join(parts, " ")
		}
	case KindOutput:
		return fmt.Sprintf("output %d", t.Output)
	case KindScreen:
		return "screen"
	case KindArea:
		return fmt.Sprintf("area %d,%d %dx%d", t.Area.X, t.Area.Y, t.Area.W, t.Area.H)
	}
	return "unknown"
}

func quote(s string) string {
	if len(s) > 60 {
		s = s[:60]
	}
	return fmt.Sprintf("%q", s)
}
