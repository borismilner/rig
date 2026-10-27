package capture

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// bus starts a private session bus and returns a dial function for it.
func bus(t *testing.T) func() (*dbus.Conn, error) {
	t.Helper()
	path, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed")
	}
	cmd := exec.Command(path, "--session", "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr = strings.TrimSpace(addr)
	return func() (*dbus.Conn, error) {
		c, err := dbus.Connect(addr)
		return c, err
	}
}

// ext is a stand-in for the Shell extension.
type ext struct {
	api     uint32
	windows string
	outputs string
	raw     string
	err     *dbus.Error
	calls   atomic.Int64
	delay   time.Duration
}

func (e *ext) Version() (uint32, string, *dbus.Error) { return e.api, "50.1", nil }
func (e *ext) ListWindows() (string, *dbus.Error)     { e.calls.Add(1); return e.windows, nil }
func (e *ext) ListOutputs() (string, *dbus.Error)     { return e.outputs, nil }
func (e *ext) CaptureWindow(_ uint64, path string) (string, *dbus.Error) {
	time.Sleep(e.delay)
	if e.err != nil {
		return "", e.err
	}
	if err := writePNG(path, solid(30, 20, color.NRGBA{B: 255, A: 255})); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return e.raw, nil
}

func (e *ext) CaptureArea(_, _, w, h int32, _ bool, path string) (string, *dbus.Error) {
	if e.err != nil {
		return "", e.err
	}
	if err := writePNG(path, solid(int(w), int(h), color.NRGBA{B: 255, A: 255})); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return e.raw, nil
}

func serve(t *testing.T, dial func() (*dbus.Conn, error), e *ext) {
	t.Helper()
	c, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Export(e, busPath, busIface); err != nil {
		t.Fatal(err)
	}
	if rep, err := c.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || rep != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("name: %v %v", rep, err)
	}
}

const oneWindow = `[{"id":7,"title":"term","app":"gjs","pid":1,"workspace":1,"minimized":false,"focused":false,"visible":true,"x":1,"y":2,"w":30,"h":20}]`

func goodExt() *ext {
	return &ext{
		api: APIVersion, windows: oneWindow,
		outputs: `[{"index":0,"name":"DP-1","x":0,"y":0,"w":64,"h":48,"scale":1,"primary":true}]`,
		raw:     `{"width":30,"height":20,"scale":1,"frame":{"X":0,"Y":0,"W":30,"H":20}}`,
	}
}

func TestGNOMEEndToEnd(t *testing.T) {
	dial := bus(t)
	serve(t, dial, goodExt())
	g := NewGNOME(dial)
	defer g.Close()
	s, err := New(g, t.TempDir()+"/c", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	in, err := s.Info(ctx)
	if err != nil || in.API != APIVersion || in.Shell != "50.1" {
		t.Fatalf("info: %v %+v", err, in)
	}
	ws, err := s.Windows(ctx, "")
	if err != nil || len(ws) != 1 || ws[0].Minimized || !ws[0].Visible || ws[0].Workspace != 1 || ws[0].ID != 7 {
		t.Fatalf("windows: %v %+v", err, ws)
	}
	shot, err := s.Capture(ctx, Target{Kind: KindWindow, ID: 7}, Options{})
	if err != nil || shot.Width != 30 || shot.Height != 20 {
		t.Fatalf("window: %v %+v", err, shot)
	}
	shot, err = s.Capture(ctx, Target{Kind: KindScreen}, Options{})
	if err != nil || shot.Width != 64 || shot.Height != 48 {
		t.Fatalf("screen: %v %+v", err, shot)
	}
}

func TestGNOMENoExtension(t *testing.T) {
	dial := bus(t)
	g := NewGNOME(dial)
	defer g.Close()
	_, err := g.Info(context.Background())
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "extension") {
		t.Fatalf("want ErrUnavailable naming the extension, got %v", err)
	}
	if _, err := g.Windows(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("windows: %v", err)
	}
}

func TestGNOMENoBus(t *testing.T) {
	g := NewGNOME(func() (*dbus.Conn, error) { return nil, errors.New("no socket") })
	if _, err := g.Info(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestGNOMERefusesAnotherAPI(t *testing.T) {
	dial := bus(t)
	e := goodExt()
	e.api = 2
	serve(t, dial, e)
	g := NewGNOME(dial)
	defer g.Close()
	_, err := g.Windows(context.Background())
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "API 2") {
		t.Fatalf("got %v", err)
	}
	if e.calls.Load() != 0 {
		t.Fatal("ListWindows was called on an extension of another API")
	}
}

func TestGNOMEMapsExtensionErrors(t *testing.T) {
	cases := map[string]error{
		busIface + ".Error.NoSuchWindow": ErrNoTarget,
		busIface + ".Error.BadArgument":  ErrInvalid,
		busIface + ".Error.TooLarge":     ErrTooLarge,
		busIface + ".Error.Failed":       ErrBadImage,
		"org.example.Odd":                ErrUnavailable,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			dial := bus(t)
			e := goodExt()
			e.err = dbus.NewError(name, []any{"because"})
			serve(t, dial, e)
			g := NewGNOME(dial)
			defer g.Close()
			_, err := g.CaptureWindow(context.Background(), 7, t.TempDir()+"/x.png")
			if !errors.Is(err, want) || !strings.Contains(err.Error(), "because") && !errors.Is(want, ErrNoTarget) {
				t.Fatalf("want %v, got %v", want, err)
			}
		})
	}
}

func TestGNOMETimeout(t *testing.T) {
	dial := bus(t)
	e := goodExt()
	e.delay = 2 * time.Second
	serve(t, dial, e)
	g := NewGNOME(dial)
	defer g.Close()
	s, err := New(g, t.TempDir()+"/c", func() Limits { return Limits{Timeout: 100 * time.Millisecond} })
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = s.Capture(context.Background(), Target{Kind: KindWindow, ID: 7}, Options{})
	if !errors.Is(err, ErrTimeout) || time.Since(start) > time.Second {
		t.Fatalf("got %v after %v", err, time.Since(start))
	}
}

func TestGNOMEBadReplies(t *testing.T) {
	for name, tweak := range map[string]func(*ext){
		"windows not json": func(e *ext) { e.windows = "<<<" },
		"raw not json":     func(e *ext) { e.raw = "nope" },
		"windows too big":  func(e *ext) { e.windows = "[" + strings.Repeat(" ", maxReply+1) + "]" },
	} {
		t.Run(name, func(t *testing.T) {
			dial := bus(t)
			e := goodExt()
			tweak(e)
			serve(t, dial, e)
			g := NewGNOME(dial)
			defer g.Close()
			s, _ := New(g, t.TempDir()+"/c", nil)
			_, err := s.Windows(context.Background(), "")
			if name == "raw not json" {
				_, err = s.Capture(context.Background(), Target{Kind: KindWindow, ID: 7}, Options{})
			}
			if err == nil {
				t.Fatal("a bad reply was accepted")
			}
		})
	}
}

func TestGNOMERefusesUnsafePaths(t *testing.T) {
	g := NewGNOME(func() (*dbus.Conn, error) { return nil, errors.New("must not be dialled") })
	for _, p := range []string{"relative.png", "/a/../b.png", "/a\x00b.png", ""} {
		if _, err := g.CaptureWindow(context.Background(), 1, p); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: got %v", p, err)
		}
	}
}

func TestGNOMERedialsAfterTheBusDrops(t *testing.T) {
	dial := bus(t)
	serve(t, dial, goodExt())
	g := NewGNOME(dial)
	defer g.Close()
	if _, err := g.Info(context.Background()); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	_ = g.conn.Close() // the connection dies under us
	g.mu.Unlock()
	if _, err := g.Windows(context.Background()); err != nil {
		t.Fatalf("a dead connection should be redialled: %v", err)
	}
}

func TestMinimizedWindowIsParsed(t *testing.T) {
	var ws []jsonWindow
	if err := json.Unmarshal([]byte(`[{"id":7,"minimized":true,"visible":false}]`), &ws); err != nil ||
		len(ws) != 1 || !ws[0].Minimized {
		t.Fatalf("minimized flag lost: %v %+v", err, ws)
	}
}
