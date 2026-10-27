package daemon

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/borismilner/rig/internal/capture"
	"github.com/borismilner/rig/internal/observe"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 58: seeing the screen. capture.list says what exists, capture.shot
// takes a picture of any of it. Boris, 2026-10-27: anybody may call, and
// every capture is logged. internal/capture does the work; this file is the
// wire, the refusals and the log line.

func (d *Daemon) serveCapture(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	if d.capture == nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig." + command + ": this daemon has no screen capture",
			Precondition: "rigd was started with its capture service",
			Actual:       "it was not; a daemon built for a test has none",
			Fix:          "ask the production daemon",
		})
		return
	}
	switch command {
	case "capture.list":
		d.serveCaptureList(ctx, c, f)
	case "capture.shot":
		d.serveCaptureShot(ctx, c, f)
	}
}

func (d *Daemon) serveCaptureList(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.CaptureListRequest
	if !unmarshalOr(c, f, "capture.list", &req) {
		return
	}
	start := time.Now()
	resp, err := d.captureList(ctx, req.GetMatch())
	d.auditCapture(c, "list", "windows", start, nil, err, map[string]string{"match": clipLog(req.GetMatch())})
	if err != nil {
		d.failCapture(c, f, "capture.list", err)
		return
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) captureList(ctx context.Context, match string) (*registryv1.CaptureListResponse, error) {
	info, err := d.capture.Info(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := d.capture.Windows(ctx, match)
	if err != nil {
		return nil, err
	}
	outs, err := d.capture.Outputs(ctx)
	if err != nil {
		return nil, err
	}
	resp := &registryv1.CaptureListResponse{Api: info.API, Shell: info.Shell}
	for i := range ws {
		resp.Windows = append(resp.Windows, windowWire(&ws[i]))
	}
	for i := range outs {
		resp.Outputs = append(resp.Outputs, outputWire(&outs[i]))
	}
	return resp, nil
}

func (d *Daemon) serveCaptureShot(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req registryv1.CaptureShotRequest
	if !unmarshalOr(c, f, "capture.shot", &req) {
		return
	}
	start := time.Now()
	target, opts, err := captureRequest(&req)
	var shot capture.Shot
	if err == nil {
		shot, err = d.capture.Capture(ctx, target, opts)
	}
	extra := map[string]string{"target": describeRequest(&req, target)}
	d.auditCapture(c, "shot", extra["target"], start, &shot, err, extra)
	if err != nil {
		d.failCapture(c, f, "capture.shot", err)
		return
	}
	c.reply(f.GetStreamId(), shotWire(&shot))
}

// captureRequest turns the wire request into the service's, inferring the
// target when it was not named.
func captureRequest(req *registryv1.CaptureShotRequest) (capture.Target, capture.Options, error) {
	var t capture.Target
	t.ID, t.Title, t.App, t.First = req.GetWindow(), req.GetTitle(), req.GetApp(), req.GetFirst()
	t.Output = int(req.GetOutput())
	if a := req.GetArea(); a != nil {
		t.Area = capture.Rect{X: int(a.GetX()), Y: int(a.GetY()), W: int(a.GetW()), H: int(a.GetH())}
	}
	switch req.GetTarget() {
	case "window":
		t.Kind = capture.KindWindow
	case "output":
		t.Kind = capture.KindOutput
	case "screen":
		t.Kind = capture.KindScreen
	case "area":
		t.Kind = capture.KindArea
	case "":
		switch {
		case req.GetWindow() != 0 || req.GetTitle() != "" || req.GetApp() != "":
			t.Kind = capture.KindWindow
		case req.GetArea() != nil:
			t.Kind = capture.KindArea
		default:
			return t, capture.Options{}, fmt.Errorf("%w: say what to capture: a window (window, title or app), an output, the screen or an area", capture.ErrInvalid)
		}
	default:
		return t, capture.Options{}, fmt.Errorf("%w: target is window, output, screen or area; got %q", capture.ErrInvalid, req.GetTarget())
	}
	o := capture.Options{
		Format:     req.GetFormat(),
		Quality:    int(req.GetQuality()),
		MaxDim:     int(req.GetMaxDim()),
		KeepShadow: req.GetKeepShadow(),
		Cursor:     req.GetCursor(),
		Inline:     int(req.GetInlineMax()),
	}
	if cr := req.GetCrop(); cr != nil {
		o.Crop = capture.Rect{X: int(cr.GetX()), Y: int(cr.GetY()), W: int(cr.GetW()), H: int(cr.GetH())}
	}
	// A uint32 beyond int32 would wrap negative on a 32-bit int; the service
	// would refuse it, but refuse it here in plain words.
	for name, v := range map[string]uint32{"max_dim": req.GetMaxDim(), "inline_max": req.GetInlineMax(), "output": req.GetOutput()} {
		if v > 1<<30 {
			return t, o, fmt.Errorf("%w: %s is too large", capture.ErrInvalid, name)
		}
	}
	return t, o, capture.Check(t, o)
}

func describeRequest(req *registryv1.CaptureShotRequest, t capture.Target) string {
	switch t.Kind {
	case capture.KindWindow:
		if t.ID != 0 {
			return "window " + strconv.FormatUint(t.ID, 10)
		}
		return "window title~" + strconv.Quote(clipLog(t.Title)) + " app~" + strconv.Quote(clipLog(t.App))
	case capture.KindOutput:
		return "output " + strconv.Itoa(t.Output)
	case capture.KindScreen:
		return "screen"
	case capture.KindArea:
		return fmt.Sprintf("area %d,%d %dx%d", t.Area.X, t.Area.Y, t.Area.W, t.Area.H)
	}
	return "unspecified " + strconv.Quote(clipLog(req.GetTarget()))
}

// clipLog bounds a caller-supplied string before it reaches the log.
func clipLog(s string) string {
	const limit = 80
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

// failCapture answers a refusal in section 9's shape.
func (d *Daemon) failCapture(c *conn, f *rigv1.Frame, command string, err error) {
	st := &rigv1.Status{Message: "rig." + command + ": " + err.Error()}
	switch {
	case errors.Is(err, capture.ErrInvalid), errors.Is(err, capture.ErrAmbiguous):
		st.Code = rigv1.Code_CODE_INVALID
	case errors.Is(err, capture.ErrNoTarget):
		st.Code = rigv1.Code_CODE_NOT_FOUND
		st.Fix = "rig capture list shows the windows and outputs that exist now"
	case errors.Is(err, capture.ErrTimeout):
		st.Code = rigv1.Code_CODE_DEADLINE
		st.Fix = "retry; if it repeats the compositor is stuck"
	case errors.Is(err, capture.ErrBusy):
		st.Code = rigv1.Code_CODE_UNAVAILABLE
		st.Fix = "retry in a moment"
	case errors.Is(err, capture.ErrUnavailable):
		st.Code = rigv1.Code_CODE_UNAVAILABLE
		st.Precondition = "the rig capture extension is installed and enabled in the GNOME Shell"
		st.Actual = err.Error()
		st.Fix = "run shellext/install.sh, then log out and in once (Wayland cannot reload a Shell extension)"
	case errors.Is(err, capture.ErrTooLarge), errors.Is(err, capture.ErrBadImage):
		st.Code = rigv1.Code_CODE_INTERNAL
	case errors.Is(err, context.Canceled):
		st.Code = rigv1.Code_CODE_UNAVAILABLE
	default:
		st.Code = rigv1.Code_CODE_INTERNAL
	}
	c.failStatus(f.GetStreamId(), st)
}

// auditCapture writes the usage-log line (plan 58, "every capture is
// logged"): who, what, when, with what result. It is an ordinary log record
// from rigd, because the audit kind has no reader on the wire, and a log line
// nobody can read is not a log. Callers see only their own client's records
// (decision 13), so the lines are for a holder of introspect. It never holds
// a picture or a window title, only ids and the caller's own words.
func (d *Daemon) auditCapture(c *conn, op, target string, start time.Time, shot *capture.Shot, err error, extra map[string]string) {
	if d.logs == nil {
		return
	}
	caller := c.name()
	if caller == "" {
		caller = callerName(c.principal())
	}
	attrs := map[string]string{
		"op": op, "caller": caller, "principal": c.principal().Kind.String(),
		"target": target, "ms": strconv.FormatInt(time.Since(start).Milliseconds(), 10),
	}
	if _, seat, _, ok := d.provenance(c); ok {
		attrs["seat"] = seat
	}
	for k, v := range extra {
		if _, taken := attrs[k]; !taken {
			attrs[k] = v
		}
	}
	level, msg := 0, "capture "+op
	switch {
	case err != nil:
		level, msg = 4, "capture "+op+" refused"
		attrs["result"], attrs["error"] = captureResult(err), clipLog(err.Error())
	case shot != nil && shot.Path != "":
		attrs["result"] = "ok"
		attrs["path"], attrs["bytes"] = shot.Path, strconv.FormatInt(shot.Bytes, 10)
		attrs["size"], attrs["sha256"] = fmt.Sprintf("%dx%d", shot.Width, shot.Height), shot.SHA256
		if shot.Window != nil {
			attrs["window"], attrs["app"] = strconv.FormatUint(shot.Window.ID, 10), clipLog(shot.Window.App)
		}
	default:
		attrs["result"] = "ok"
	}
	d.logs.Append(observe.Record{
		At: start.UnixNano(), Level: level, Client: "rigd",
		Message: msg, Attrs: attrs,
	})
}

func captureResult(err error) string {
	switch {
	case errors.Is(err, capture.ErrInvalid), errors.Is(err, capture.ErrAmbiguous):
		return "invalid"
	case errors.Is(err, capture.ErrNoTarget):
		return "not found"
	case errors.Is(err, capture.ErrBusy):
		return "busy"
	case errors.Is(err, capture.ErrTimeout):
		return "timeout"
	case errors.Is(err, capture.ErrUnavailable):
		return "unavailable"
	}
	return "failed"
}

func windowWire(w *capture.Window) *registryv1.CaptureWindow {
	return &registryv1.CaptureWindow{
		Id: w.ID, Title: w.Title, App: w.App, Pid: clampU32(w.PID), Workspace: int32(w.Workspace), //nolint:gosec // a workspace index
		Minimized: w.Minimized, Focused: w.Focused, Visible: w.Visible, Fullscreen: w.Fullscreen, Modal: w.Modal,
		X: int32(w.X), Y: int32(w.Y), W: clampU32(w.W), H: clampU32(w.H), //nolint:gosec // desktop coordinates
	}
}

func outputWire(o *capture.Output) *registryv1.CaptureOutput {
	return &registryv1.CaptureOutput{
		Index: clampU32(o.Index), Name: o.Name, X: int32(o.X), Y: int32(o.Y), //nolint:gosec // desktop coordinates
		W: clampU32(o.W), H: clampU32(o.H), Scale: o.Scale, Primary: o.Primary,
	}
}

func shotWire(s *capture.Shot) *registryv1.CaptureShotResponse {
	r := &registryv1.CaptureShotResponse{
		Path: s.Path, Format: s.Format, Bytes: uint64(s.Bytes), //nolint:gosec // a file size
		Width: clampU32(s.Width), Height: clampU32(s.Height), Sha256: s.SHA256, Target: s.Target,
		TakenAt: s.TakenAt.UTC().Format(time.RFC3339), TookMs: clampU32(int(s.Took.Milliseconds())),
		Data: s.Data, Shadow: s.Shadowed, Cursor: s.Cursor,
	}
	if s.Window != nil {
		r.Window = windowWire(s.Window)
	}
	if s.Output != nil {
		r.Output = outputWire(s.Output)
	}
	return r
}
