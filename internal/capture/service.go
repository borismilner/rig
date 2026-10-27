package capture

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Hard ceilings no configuration can lift: they bound a caller's arithmetic,
// not the machine's capacity.
const (
	maxDim      = 16384
	maxAreaSide = 65536
	maxMatchLen = 200
	maxListed   = 8
)

// Service captures on behalf of callers, inside Dir.
type Service struct {
	be       Backend
	dir      string
	limits   func() Limits
	inflight atomic.Int64
	pruneMu  sync.Mutex
	now      func() time.Time
}

// New makes the service and its private directory. limits is read on every
// capture, so a live configuration change applies at once; nil means Defaults.
func New(be Backend, dir string, limits func() Limits) (*Service, error) {
	if be == nil {
		return nil, errors.New("capture: no backend")
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("capture: the directory %q is not absolute", dir)
	}
	if limits == nil {
		limits = Defaults
	}
	s := &Service{be: be, dir: dir, limits: limits, now: time.Now}
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	s.prune("")
	return s, nil
}

// Dir is where pictures are kept.
func (s *Service) Dir() string { return s.dir }

// ensureDir makes the directory private to this user: a symlink, a foreign
// owner or a loose mode would let another user read or replace a picture of
// this user's screen.
func (s *Service) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("capture: making %s: %w", s.dir, err)
	}
	st, err := os.Lstat(s.dir)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("capture: %s is not a directory (it is %s)", s.dir, st.Mode().Type())
	}
	if !ownedByUs(st) {
		return fmt.Errorf("capture: %s is not owned by this user", s.dir)
	}
	if st.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(s.dir, 0o700); err != nil {
			return fmt.Errorf("capture: %s is readable by others and cannot be tightened: %w", s.dir, err)
		}
	}
	return nil
}

// Info asks the backend who it is.
func (s *Service) Info(ctx context.Context) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, s.limits().filled().Timeout)
	defer cancel()
	in, err := s.be.Info(ctx)
	return in, mapCtx(ctx, err)
}

// Windows lists the windows whose title or application contains match
// (case-insensitive); an empty match lists them all.
func (s *Service) Windows(ctx context.Context, match string) ([]Window, error) {
	if len(match) > maxMatchLen {
		return nil, fmt.Errorf("%w: the match is at most %d bytes", ErrInvalid, maxMatchLen)
	}
	ctx, cancel := context.WithTimeout(ctx, s.limits().filled().Timeout)
	defer cancel()
	all, err := s.be.Windows(ctx)
	if err != nil {
		return nil, mapCtx(ctx, err)
	}
	if match == "" {
		return all, nil
	}
	m := strings.ToLower(match)
	var out []Window
	for _, w := range all {
		if strings.Contains(strings.ToLower(w.Title), m) || strings.Contains(strings.ToLower(w.App), m) {
			out = append(out, w)
		}
	}
	return out, nil
}

// Outputs lists the monitors.
func (s *Service) Outputs(ctx context.Context) ([]Output, error) {
	ctx, cancel := context.WithTimeout(ctx, s.limits().filled().Timeout)
	defer cancel()
	out, err := s.be.Outputs(ctx)
	return out, mapCtx(ctx, err)
}

// Check validates a request without touching the compositor, so a bad
// request is refused the same way whatever the backend is doing.
func Check(t Target, o Options) error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, a...)...)
	}
	switch o.Format {
	case "", "png", "jpeg":
	default:
		return bad("format is png or jpeg; got %q", o.Format)
	}
	if o.Quality < 0 || o.Quality > 100 {
		return bad("quality is 1 to 100; got %d", o.Quality)
	}
	if o.MaxDim < 0 || o.MaxDim > maxDim {
		return bad("max_dim is 1 to %d; got %d", maxDim, o.MaxDim)
	}
	if o.Inline < 0 {
		return bad("inline is a byte count; got %d", o.Inline)
	}
	c := o.Crop
	if c.X < 0 || c.Y < 0 || c.W < 0 || c.H < 0 || c.W > maxAreaSide || c.H > maxAreaSide ||
		(c.W == 0) != (c.H == 0) {
		return bad("crop needs a non-negative x, y and a width and height of 1 to %d", maxAreaSide)
	}
	switch t.Kind {
	case KindWindow:
		if t.ID == 0 && t.Title == "" && t.App == "" {
			return bad("a window is named by id, title or app")
		}
		if len(t.Title) > maxMatchLen || len(t.App) > maxMatchLen {
			return bad("a title or app match is at most %d bytes", maxMatchLen)
		}
		if o.Cursor {
			return bad("the pointer is not part of a window's picture; capture its output or an area")
		}
	case KindOutput:
		if t.Output < 0 || t.Output > 64 {
			return bad("output is a monitor index from 0; got %d", t.Output)
		}
	case KindScreen:
	case KindArea:
		a := t.Area
		if a.W <= 0 || a.H <= 0 || a.W > maxAreaSide || a.H > maxAreaSide ||
			a.X < -maxAreaSide || a.Y < -maxAreaSide || a.X > maxAreaSide || a.Y > maxAreaSide {
			return bad("an area needs a width and height of 1 to %d and a position within %d", maxAreaSide, maxAreaSide)
		}
	default:
		return bad("the target is a window, an output, the screen or an area")
	}
	return nil
}

// Capture takes one picture.
func (s *Service) Capture(ctx context.Context, t Target, o Options) (Shot, error) {
	if err := Check(t, o); err != nil {
		return Shot{}, err
	}
	lim := s.limits().filled()
	if n := s.inflight.Add(1); n > int64(lim.Concurrent) {
		s.inflight.Add(-1)
		return Shot{}, fmt.Errorf("%w: %d at once is the limit; retry shortly", ErrBusy, lim.Concurrent)
	}
	defer s.inflight.Add(-1)
	if err := s.ensureDir(); err != nil {
		return Shot{}, err
	}

	start := s.now()
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()

	shot := Shot{Target: t.describe()}
	tmp, final, err := s.names(o)
	if err != nil {
		return Shot{}, err
	}
	defer os.Remove(tmp)

	var raw Raw
	var monitors []Rect
	switch t.Kind {
	case KindWindow:
		var w Window
		if w, err = s.pickWindow(ctx, t); err != nil {
			return Shot{}, err
		}
		shot.Window = &w
		if raw, err = s.be.CaptureWindow(ctx, w.ID, tmp); err != nil {
			return Shot{}, mapCtx(ctx, err)
		}
	case KindOutput, KindScreen:
		var area Rect
		var outs []Output
		if outs, err = s.be.Outputs(ctx); err != nil {
			return Shot{}, mapCtx(ctx, err)
		}
		if area, shot.Output, err = outputArea(outs, t); err != nil {
			return Shot{}, err
		}
		if t.Kind == KindScreen {
			for _, o := range outs {
				monitors = append(monitors, Rect{X: o.X - area.X, Y: o.Y - area.Y, W: o.W, H: o.H})
			}
		}
		if raw, err = s.be.CaptureArea(ctx, area, o.Cursor, tmp); err != nil {
			return Shot{}, mapCtx(ctx, err)
		}
	case KindArea:
		if raw, err = s.be.CaptureArea(ctx, t.Area, o.Cursor, tmp); err != nil {
			return Shot{}, mapCtx(ctx, err)
		}
	}

	d, err := finish(tmp, raw, t.Kind, o, lim, monitors, final)
	if err != nil {
		return Shot{}, err
	}
	shot.Path, shot.Format = final, "png"
	if o.Format == "jpeg" {
		shot.Format = "jpeg"
	}
	shot.Bytes, shot.Width, shot.Height, shot.SHA256, shot.Shadowed = d.bytes, d.width, d.height, d.sum, d.shadowed
	shot.TakenAt, shot.Took = start, s.now().Sub(start)
	if want := min(o.Inline, lim.InlineBytes); want > 0 && d.bytes <= int64(want) {
		if b, err := os.ReadFile(final); err == nil {
			shot.Data = b
		}
	}
	s.prune(final)
	return shot, nil
}

// pickWindow resolves a target to one window.
func (s *Service) pickWindow(ctx context.Context, t Target) (Window, error) {
	all, err := s.be.Windows(ctx)
	if err != nil {
		return Window{}, mapCtx(ctx, err)
	}
	var hits []Window
	for _, w := range all {
		switch {
		case t.ID != 0:
			if w.ID == t.ID {
				hits = append(hits, w)
			}
		default:
			if matches(w.Title, t.Title) && matches(w.App, t.App) {
				hits = append(hits, w)
			}
		}
	}
	switch len(hits) {
	case 0:
		return Window{}, fmt.Errorf("%w: no window matches %s (capture.list shows what exists)", ErrNoTarget, t.describe())
	case 1:
		return hits[0], nil
	}
	if t.First {
		for _, w := range hits {
			if w.Focused {
				return w, nil
			}
		}
		return hits[len(hits)-1], nil // the compositor lists bottom to top
	}
	var names []string
	for i, w := range hits {
		if i == maxListed {
			names = append(names, fmt.Sprintf("and %d more", len(hits)-maxListed))
			break
		}
		names = append(names, fmt.Sprintf("%d %q", w.ID, w.Title))
	}
	return Window{}, fmt.Errorf("%w: %d windows match %s: %s; pass the id, narrow the match, or set first",
		ErrAmbiguous, len(hits), t.describe(), strings.Join(names, ", "))
}

func matches(have, want string) bool {
	return want == "" || strings.Contains(strings.ToLower(have), strings.ToLower(want))
}

// outputArea is the rectangle of one monitor, or of all of them.
func outputArea(outs []Output, t Target) (Rect, *Output, error) {
	if len(outs) == 0 {
		return Rect{}, nil, fmt.Errorf("%w: the compositor reports no monitors", ErrNoTarget)
	}
	if t.Kind == KindOutput {
		for i := range outs {
			if outs[i].Index == t.Output {
				o := outs[i]
				return Rect{X: o.X, Y: o.Y, W: o.W, H: o.H}, &o, nil
			}
		}
		return Rect{}, nil, fmt.Errorf("%w: there is no output %d (there are %d)", ErrNoTarget, t.Output, len(outs))
	}
	x0, y0, x1, y1 := outs[0].X, outs[0].Y, outs[0].X+outs[0].W, outs[0].Y+outs[0].H
	for _, o := range outs[1:] {
		x0, y0 = min(x0, o.X), min(y0, o.Y)
		x1, y1 = max(x1, o.X+o.W), max(y1, o.Y+o.H)
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, nil, nil
}

// names makes the two paths for one capture: the name the backend writes,
// and the name the finished picture is kept under.
func (s *Service) names(o Options) (tmp, final string, err error) {
	var b [6]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("capture: no randomness: %w", err)
	}
	id := hex.EncodeToString(b[:])
	ext := "png"
	if o.Format == "jpeg" {
		ext = "jpg"
	}
	stamp := s.now().UTC().Format("20060102T150405Z")
	return filepath.Join(s.dir, ".tmp-"+id+".png"),
		filepath.Join(s.dir, "shot-"+stamp+"-"+id+"."+ext), nil
}

// prune keeps the directory inside its limits. keep is never removed. Files
// that are not this service's (no shot- prefix) are never touched, and stray
// temporaries are removed once they are past any plausible capture.
func (s *Service) prune(keep string) {
	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()
	lim := s.limits().filled()
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		mod  time.Time
		size int64
	}
	var shots []file
	now := s.now()
	for _, e := range ents {
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(s.dir, e.Name())
		switch {
		case strings.HasPrefix(e.Name(), ".tmp-"):
			if now.Sub(info.ModTime()) > 2*lim.Timeout+time.Minute {
				os.Remove(p)
			}
		case strings.HasPrefix(e.Name(), "shot-"):
			shots = append(shots, file{p, info.ModTime(), info.Size()})
		}
	}
	sort.Slice(shots, func(i, j int) bool { return shots[i].mod.After(shots[j].mod) })
	var total int64
	for i, f := range shots {
		total += f.size
		if f.path == keep {
			continue
		}
		if i >= lim.KeepFiles || total > lim.KeepBytes || now.Sub(f.mod) > lim.KeepAge {
			os.Remove(f.path)
		}
	}
}

// mapCtx turns a context expiry into ErrTimeout and leaves other errors be.
func mapCtx(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(err, ErrTimeout) {
		return fmt.Errorf("%w (%v)", ErrTimeout, err)
	}
	return err
}
