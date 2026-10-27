package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// `rig capture list [MATCH]` and `rig capture shot [target] [options]` -
// section 58's eyes. A window is named by --window ID (from `list`) or by
// --title and --app substrings; or take --output N, --screen or --area
// X,Y,WxH. Windows are captured whether or not they are in front, visible
// or on this workspace. The answer is the path of a file; --json gives the
// whole record.

func cmdCapture(args []string) error {
	if len(args) == 0 {
		return badArgumentf("usage: rig capture <list|shot> ...")
	}
	switch args[0] {
	case "list":
		return cmdCaptureList(args[1:])
	case "shot":
		return cmdCaptureShot(args[1:])
	}
	return badArgumentf("rig capture has list and shot; %q is neither", args[0])
}

func cmdCaptureList(args []string) (err error) {
	fs := flag.NewFlagSet("capture list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) > 1 {
		return badArgumentf("usage: rig capture list [MATCH] [--json]")
	}
	match := ""
	if len(positional) == 1 {
		match = positional[0]
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	var resp registryv1.CaptureListResponse
	if err := call(ctx, c, "rig.capture.list", &registryv1.CaptureListRequest{Match: match}, &resp); err != nil {
		return err
	}
	if *asJSON {
		return printProtoJSON(&resp)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tWS\tSTATE\tAPP\tSIZE\tTITLE")
	for _, w := range resp.GetWindows() {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%dx%d\t%s\n", w.GetId(), w.GetWorkspace(), windowState(w),
			w.GetApp(), w.GetW(), w.GetH(), w.GetTitle())
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, o := range resp.GetOutputs() {
		primary := ""
		if o.GetPrimary() {
			primary = " (primary)"
		}
		fmt.Printf("output %d  %s  %dx%d at %d,%d  scale %g%s\n", o.GetIndex(), o.GetName(), o.GetW(), o.GetH(), o.GetX(), o.GetY(), o.GetScale(), primary)
	}
	return nil
}

// windowState says in a word why a window might not be on screen.
func windowState(w *registryv1.CaptureWindow) string {
	switch {
	case w.GetMinimized():
		return "minimized"
	case w.GetFocused():
		return "focused"
	case !w.GetVisible():
		return "hidden"
	case w.GetFullscreen():
		return "fullscreen"
	}
	return "shown"
}

func cmdCaptureShot(args []string) (err error) {
	fs := flag.NewFlagSet("capture shot", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	timeout := fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	window := fs.Uint64("window", 0, "a window's id from `rig capture list`")
	title := fs.String("title", "", "capture the window whose title contains this")
	app := fs.String("app", "", "capture the window whose application contains this")
	first := fs.Bool("first", false, "if several match, take the focused one, else the topmost")
	output := fs.Int("output", -1, "capture this monitor")
	screen := fs.Bool("screen", false, "capture every monitor as one picture")
	area := fs.String("area", "", "capture a rectangle of the desktop: X,Y,WxH")
	crop := fs.String("crop", "", "keep a rectangle of the picture: X,Y,WxH")
	format := fs.String("format", "", "png (default) or jpeg")
	quality := fs.Int("quality", 0, "JPEG quality, 1 to 100")
	maxDim := fs.Int("max-dim", 0, "shrink so the longest side is at most this")
	shadow := fs.Bool("keep-shadow", false, "keep a window's shadow margin")
	cursor := fs.Bool("cursor", false, "draw the pointer (not for a window)")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) != 0 {
		return badArgumentf("usage: rig capture shot (--window ID | --title T [--app A] | --output N | --screen | --area X,Y,WxH) [options]")
	}
	req := &registryv1.CaptureShotRequest{
		Window: *window, Title: *title, App: *app, First: *first,
		Format: *format, Quality: uint32(max(*quality, 0)), MaxDim: uint32(max(*maxDim, 0)), //nolint:gosec // clamped at zero
		KeepShadow: *shadow, Cursor: *cursor,
	}
	targets := 0
	if *window != 0 || *title != "" || *app != "" {
		req.Target, targets = "window", targets+1
	}
	if *output >= 0 {
		req.Target, req.Output, targets = "output", uint32(*output), targets+1 //nolint:gosec // non-negative
	}
	if *screen {
		req.Target, targets = "screen", targets+1
	}
	if *area != "" {
		r, err := parseRect(*area)
		if err != nil {
			return badArgumentf("--area: %v", err)
		}
		req.Target, req.Area, targets = "area", r, targets+1
	}
	if *crop != "" {
		r, err := parseRect(*crop)
		if err != nil {
			return badArgumentf("--crop: %v", err)
		}
		req.Crop = r
	}
	if targets != 1 {
		return badArgumentf("name exactly one target: --window/--title/--app, --output, --screen or --area (got %d)", targets)
	}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	var resp registryv1.CaptureShotResponse
	if err := call(ctx, c, "rig.capture.shot", req, &resp); err != nil {
		return err
	}
	if *asJSON {
		return printProtoJSON(&resp)
	}
	if *cursor && resp.GetCursor() == "hidden" {
		fmt.Fprintln(os.Stderr, "rig: the pointer is hidden, so the picture has none; move the mouse and retry")
	}
	fmt.Println(resp.GetPath())
	fmt.Fprintf(os.Stderr, "%s %dx%d, %d bytes, %s, %d ms\n", resp.GetFormat(), resp.GetWidth(), resp.GetHeight(),
		resp.GetBytes(), resp.GetTarget(), resp.GetTookMs())
	return nil
}

func printProtoJSON(m proto.Message) error {
	b, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(b))
	return err
}

// parseRect reads X,Y,WxH.
func parseRect(s string) (*registryv1.CaptureRect, error) {
	bad := fmt.Errorf("%q is not X,Y,WxH (for example 100,50,640x480)", s)
	xy, wh, ok := strings.Cut(s, "x")
	if !ok {
		return nil, bad
	}
	parts := strings.Split(xy, ",")
	if len(parts) != 3 {
		return nil, bad
	}
	var n [4]int64
	for i, p := range []string{parts[0], parts[1], parts[2], wh} {
		v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 32)
		if err != nil {
			return nil, bad
		}
		n[i] = v
	}
	if n[2] <= 0 || n[3] <= 0 {
		return nil, fmt.Errorf("%q has no area", s)
	}
	return &registryv1.CaptureRect{X: int32(n[0]), Y: int32(n[1]), W: uint32(n[2]), H: uint32(n[3])}, nil //nolint:gosec // parsed as 32-bit, width and height positive
}
