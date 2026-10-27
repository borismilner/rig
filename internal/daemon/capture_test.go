package daemon

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/capture"
	"github.com/borismilner/rig/internal/observe"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// screen is a compositor with three windows and one monitor.
type screen struct{ err error }

func (s screen) Info(context.Context) (capture.Info, error) {
	return capture.Info{API: 1, Shell: "50.1"}, s.err
}

func (s screen) Windows(context.Context) ([]capture.Window, error) {
	return []capture.Window{
		{ID: 11, Title: "secret-plans.odt", App: "writer", W: 80, H: 60, Minimized: true, Workspace: 2},
		{ID: 12, Title: "build log", App: "terminal", W: 80, H: 60},
		{ID: 13, Title: "build log", App: "terminal", W: 40, H: 30, Focused: true},
	}, s.err
}

func (s screen) Outputs(context.Context) ([]capture.Output, error) {
	return []capture.Output{{Index: 0, Name: "DP-1", W: 160, H: 90, Scale: 1, Primary: true}}, s.err
}

func paint(path string, w, h int) error {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o600)
}

func (s screen) CaptureWindow(_ context.Context, id uint64, path string) (capture.Raw, error) {
	if s.err != nil {
		return capture.Raw{}, s.err
	}
	return capture.Raw{Width: 80, Height: 60, Scale: 1}, paint(path, 80, 60)
}

func (s screen) CaptureArea(_ context.Context, r capture.Rect, _ bool, path string) (capture.Raw, error) {
	if s.err != nil {
		return capture.Raw{}, s.err
	}
	return capture.Raw{Width: r.W, Height: r.H, Scale: 1}, paint(path, r.W, r.H)
}

func withScreen(t *testing.T, be capture.Backend) (string, *Daemon, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cap")
	svc, err := capture.New(be, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	sock, d := upRecordDaemonWith(t, func(d *Daemon) {
		d.capture = svc
		d.logs = observe.New(observe.Options{})
	})
	return sock, d, dir
}

func captureLog(t *testing.T, d *Daemon) []observe.Record {
	t.Helper()
	res, err := d.logs.Query(observe.Query{Grep: regexp.MustCompile("capture")})
	if err != nil {
		t.Fatal(err)
	}
	return res.Records
}

func TestAnybodyMayListAndCaptureAndEveryCallIsLogged(t *testing.T) {
	sock, d, dir := withScreen(t, screen{})
	ctx := recordCtx(t)
	seat := seated(t, sock, "builder")
	prog := program(t, sock, "unattended")

	var list registryv1.CaptureListResponse
	if err := prog.Call(ctx, "rig.capture.list", &registryv1.CaptureListRequest{}, &list); err != nil {
		t.Fatalf("a plain program listing: %v", err)
	}
	if len(list.GetWindows()) != 3 || len(list.GetOutputs()) != 1 || list.GetApi() != 1 || list.GetShell() != "50.1" {
		t.Fatalf("list: %v", &list)
	}
	if w := list.GetWindows()[0]; !w.GetMinimized() || w.GetWorkspace() != 2 || w.GetId() != 11 {
		t.Fatalf("hidden windows must be listed: %v", w)
	}
	var filtered registryv1.CaptureListResponse
	if err := seat.Call(ctx, "rig.capture.list", &registryv1.CaptureListRequest{Match: "TERMINAL"}, &filtered); err != nil || len(filtered.GetWindows()) != 2 {
		t.Fatalf("filtered: %v %v", err, &filtered)
	}

	var shot registryv1.CaptureShotResponse
	if err := seat.Call(ctx, "rig.capture.shot", &registryv1.CaptureShotRequest{Window: 11, InlineMax: 1 << 20}, &shot); err != nil {
		t.Fatalf("a seat capturing a minimized window: %v", err)
	}
	if shot.GetWidth() != 80 || shot.GetHeight() != 60 || shot.GetWindow().GetId() != 11 || len(shot.GetData()) == 0 ||
		uint64(len(shot.GetData())) != shot.GetBytes() || shot.GetSha256() == "" || shot.GetTakenAt() == "" {
		t.Fatalf("shot: %v", &shot)
	}
	if filepath.Dir(shot.GetPath()) != dir {
		t.Fatalf("picture kept at %s, want under %s", shot.GetPath(), dir)
	}
	if st, err := os.Stat(shot.GetPath()); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("picture file: %v %v", st, err)
	}
	if err := prog.Call(ctx, "rig.capture.shot", &registryv1.CaptureShotRequest{Target: "screen"}, &shot); err != nil || shot.GetWidth() != 160 {
		t.Fatalf("a plain program capturing the screen: %v %v", err, &shot)
	}

	recs := captureLog(t, d)
	if len(recs) < 4 {
		t.Fatalf("every call should be logged, got %d records: %+v", len(recs), recs)
	}
	var sawSeat, sawProg bool
	for _, r := range recs {
		if r.Client != "rigd" || r.Attrs["op"] == "" || r.Attrs["result"] == "" || r.Attrs["target"] == "" || r.Attrs["caller"] == "" {
			t.Fatalf("incomplete line: %+v", r)
		}
		sawSeat = sawSeat || r.Attrs["seat"] == "builder"
		sawProg = sawProg || r.Attrs["caller"] == "unattended"
		for _, v := range r.Attrs {
			if strings.Contains(v, "secret-plans") {
				t.Fatalf("a window title reached the log: %+v", r)
			}
		}
	}
	if !sawSeat || !sawProg {
		t.Fatalf("callers not recorded (seat %v, program %v): %+v", sawSeat, sawProg, recs)
	}
}

func TestCaptureRefusalsAreLoggedAndSpecific(t *testing.T) {
	sock, d, dir := withScreen(t, screen{})
	ctx := recordCtx(t)
	c := seated(t, sock, "prober")
	shot := func(r *registryv1.CaptureShotRequest) error {
		return c.Call(ctx, "rig.capture.shot", r, &registryv1.CaptureShotResponse{})
	}

	err := shot(&registryv1.CaptureShotRequest{App: "terminal"})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "an ambiguous match")
	if !strings.Contains(err.Error(), "12") || !strings.Contains(err.Error(), "13") || !strings.Contains(err.Error(), "first") {
		t.Fatalf("the refusal should name the candidates and the way out: %v", err)
	}
	if err := shot(&registryv1.CaptureShotRequest{App: "terminal", First: true}); err != nil {
		t.Fatalf("first: %v", err)
	}
	wantCode(t, shot(&registryv1.CaptureShotRequest{Window: 999}), rigv1.Code_CODE_NOT_FOUND, "no such window")
	wantCode(t, shot(&registryv1.CaptureShotRequest{}), rigv1.Code_CODE_INVALID, "no target")
	wantCode(t, shot(&registryv1.CaptureShotRequest{Target: "desktop"}), rigv1.Code_CODE_INVALID, "unknown target")
	wantCode(t, shot(&registryv1.CaptureShotRequest{Window: 11, Format: "bmp"}), rigv1.Code_CODE_INVALID, "bad format")
	wantCode(t, shot(&registryv1.CaptureShotRequest{Window: 11, Cursor: true}), rigv1.Code_CODE_INVALID, "cursor on a window")
	wantCode(t, shot(&registryv1.CaptureShotRequest{Target: "output", Output: 7}), rigv1.Code_CODE_NOT_FOUND, "no such output")
	wantCode(t, shot(&registryv1.CaptureShotRequest{Window: 11, MaxDim: 1 << 31}), rigv1.Code_CODE_INVALID, "an absurd size")
	wantCode(t, shot(&registryv1.CaptureShotRequest{Target: "area", Area: &registryv1.CaptureRect{}}), rigv1.Code_CODE_INVALID, "an empty area")

	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("only the one good capture should have left a picture, got %d files", len(entries))
	}
	var refused int
	for _, r := range captureLog(t, d) {
		if r.Attrs["result"] != "ok" {
			refused++
			if r.Level == 0 {
				t.Fatalf("a refusal should be logged above info: %+v", r)
			}
		}
	}
	if refused != 9 {
		t.Fatalf("want 9 refusals logged, got %d", refused)
	}
}

func TestCaptureWithoutTheExtensionSaysHowToFixIt(t *testing.T) {
	sock, d, _ := withScreen(t, screen{err: capture.ErrUnavailable})
	ctx := recordCtx(t)
	c := seated(t, sock, "prober")
	err := c.Call(ctx, "rig.capture.shot", &registryv1.CaptureShotRequest{Window: 11}, &registryv1.CaptureShotResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "no extension")
	var ce *client.CallError
	if !errors.As(err, &ce) || !strings.Contains(ce.Status.GetFix(), "install") || ce.Status.GetPrecondition() == "" {
		t.Fatalf("the refusal must carry the fix and the precondition: %v", err)
	}
	recs := captureLog(t, d)
	if len(recs) == 0 || recs[len(recs)-1].Attrs["result"] != "unavailable" {
		t.Fatalf("the failure must be logged: %+v", recs)
	}
}

func TestABlindDaemonRefusesAndSaysSo(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "prober")
	for _, v := range []string{"rig.capture.list", "rig.capture.shot"} {
		var err error
		if v == "rig.capture.list" {
			err = c.Call(recordCtx(t), v, &registryv1.CaptureListRequest{}, &registryv1.CaptureListResponse{})
		} else {
			err = c.Call(recordCtx(t), v, &registryv1.CaptureShotRequest{Window: 1}, &registryv1.CaptureShotResponse{})
		}
		wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, v+" on a daemon without capture")
		if !strings.Contains(err.Error(), "no screen capture") {
			t.Fatalf("%s: %v", v, err)
		}
	}
}

func TestCaptureVerbsAreOnTheAgentDoor(t *testing.T) {
	for _, id := range []string{"capture.list", "capture.shot"} {
		if _, ok := bridged[id]; !ok {
			t.Errorf("%s is declared but not an MCP tool", id)
		}
		var declared bool
		for _, c := range selfDeclaration().Commands {
			declared = declared || c.ID == id
		}
		if !declared {
			t.Errorf("%s is not declared", id)
		}
	}
	if verbTool("capture.shot") != "capture_shot" {
		t.Fatal("the tool name")
	}
}
