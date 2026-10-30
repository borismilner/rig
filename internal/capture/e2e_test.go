package capture

import (
	"context"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// TestRealShell runs the Service against the real extension in a headless
// Shell. tools/shellcapture-e2e/run.sh sets it up and sets RIG_SHELL_E2E; it
// is skipped everywhere else.
func TestRealShell(t *testing.T) {
	if os.Getenv("RIG_SHELL_E2E") == "" {
		t.Skip("RIG_SHELL_E2E is not set")
	}
	be := NewGNOME(func() (*dbus.Conn, error) { return dbus.ConnectSessionBus() })
	defer be.Close()
	svc, err := New(be, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	info, err := svc.Info(ctx)
	if err != nil || info.API != 1 {
		t.Fatalf("info = %+v, %v", info, err)
	}
	outs, err := svc.Outputs(ctx)
	if err != nil || len(outs) == 0 {
		t.Fatalf("outputs = %+v, %v", outs, err)
	}
	t.Logf("outputs: %+v", outs)

	centre := func(path string) [3]uint8 {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		r, g, bl, _ := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
		return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
	}
	want := map[string][3]uint8{"red": {255, 0, 0}, "green": {0, 255, 0}, "blue": {0, 0, 255}}
	for name, rgb := range want {
		shot, err := svc.Capture(ctx, Target{Kind: KindWindow, Title: "rigspike-" + name}, Options{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%s: %dx%d shadowed=%v %s", name, shot.Width, shot.Height, shot.Shadowed, shot.Window.Title)
		if shot.Width != 400 || shot.Height != 300 || shot.Shadowed {
			t.Errorf("%s: want the 400x300 frame, got %dx%d shadowed=%v", name, shot.Width, shot.Height, shot.Shadowed)
		}
		if got := centre(shot.Path); got != rgb {
			t.Errorf("%s: centre pixel %v, want %v", name, got, rgb)
		}
	}

	if _, err := svc.Capture(ctx, Target{Kind: KindWindow, Title: "rigspike-"}, Options{}); err == nil ||
		!strings.Contains(err.Error(), "several") {
		t.Errorf("an ambiguous match gave %v", err)
	}
	if _, err := svc.Capture(ctx, Target{Kind: KindWindow, ID: 999999}, Options{}); err == nil {
		t.Error("a missing window id was accepted")
	}
	scr, err := svc.Capture(ctx, Target{Kind: KindScreen}, Options{})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	t.Logf("screen: %dx%d", scr.Width, scr.Height)
	if scr.Width != outs[0].W || scr.Height != outs[0].H {
		t.Errorf("screen is %dx%d, the output is %dx%d", scr.Width, scr.Height, outs[0].W, outs[0].H)
	}
	area, err := svc.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 10, Y: 10, W: 100, H: 50}}, Options{})
	if err != nil || area.Width != 100 || area.Height != 50 {
		t.Errorf("area = %+v, %v", area, err)
	}
	// The pointer: hidden until a pointer exists, then drawn.
	hid, err := svc.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 0, Y: 0, W: 300, H: 300}}, Options{Cursor: true})
	if err != nil || hid.Cursor != "hidden" {
		t.Errorf("before any pointer: %+v, %v", hid, err)
	}
	if conn, err := dbus.ConnectSessionBus(); err != nil {
		t.Errorf("session bus: %v", err)
	} else {
		defer conn.Close()
		rd := conn.Object("org.gnome.Mutter.RemoteDesktop", "/org/gnome/Mutter/RemoteDesktop")
		var sp dbus.ObjectPath
		if err := rd.Call("org.gnome.Mutter.RemoteDesktop.CreateSession", 0).Store(&sp); err != nil {
			t.Errorf("remote desktop session: %v", err)
		} else {
			sess := conn.Object("org.gnome.Mutter.RemoteDesktop", sp)
			if err := sess.Call("org.gnome.Mutter.RemoteDesktop.Session.Start", 0).Err; err != nil {
				t.Errorf("start: %v", err)
			}
			sess.Call("org.gnome.Mutter.RemoteDesktop.Session.NotifyPointerMotionRelative", 0, 100.0, 100.0)
			time.Sleep(time.Second)
			drawn, err := svc.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 0, Y: 0, W: 300, H: 300}}, Options{Cursor: true})
			if err != nil || drawn.Cursor != "drawn" {
				t.Errorf("with a pointer: %+v, %v", drawn, err)
			}
			plain, err := svc.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 0, Y: 0, W: 300, H: 300}}, Options{})
			if err != nil || plain.Cursor != "" || plain.SHA256 == drawn.SHA256 {
				t.Errorf("without --cursor the picture must differ and say nothing: %+v, %v", plain, err)
			}
		}
	}
	if _, err := svc.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 90000, Y: 0, W: 10, H: 10}}, Options{}); err == nil {
		t.Error("an area outside the desktop was accepted")
	}

	mainPID, err := strconv.Atoi(os.Getenv("RIG_SHELL_E2E_PID"))
	if err != nil || mainPID <= 0 {
		t.Fatalf("main window pid is unavailable: %v", err)
	}
	otherPID, err := strconv.Atoi(os.Getenv("RIG_SHELL_E2E_OTHER_PID"))
	if err != nil || otherPID <= 0 || otherPID == mainPID {
		t.Fatalf("secondary window pid is invalid: %v", err)
	}
	windows, err := svc.Windows(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	mainWindow, otherWindow := findPIDWindow(windows, mainPID), findPIDWindow(windows, otherPID)
	if mainWindow == nil || otherWindow == nil {
		t.Fatalf("test windows not found: main=%+v secondary=%+v", mainWindow, otherWindow)
	}
	blueWindow := findPIDWindowTitle(windows, mainPID, "rigspike-blue")
	if blueWindow == nil {
		t.Fatal("blue test window is missing")
	}
	mainPoint := outlineTopPixel(blueWindow)
	otherPoint := outlineTopPixel(otherWindow)
	if err := svc.SetDebugBrowserPID(ctx, mainPID); err != nil {
		t.Fatalf("register main PID: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	mainPixel := capturePixel(t, svc, ctx, mainPoint)
	if !isOutlineRed(mainPixel) {
		t.Fatalf("main process outline pixel is %v, want red", mainPixel)
	}
	mainProcess, err := os.FindProcess(mainPID)
	if err != nil {
		t.Fatal(err)
	}
	if err := mainProcess.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("request a window resize: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		windows, err = svc.Windows(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		resized := findPIDWindowTitle(windows, mainPID, "rigspike-blue")
		if resized != nil && (resized.W > blueWindow.W || resized.H > blueWindow.H) {
			blueWindow = resized
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if blueWindow.W <= 400 || blueWindow.H <= 300 {
		t.Fatalf("test window did not resize: %+v", blueWindow)
	}
	mainPoint = outlineTopPixel(blueWindow)
	time.Sleep(200 * time.Millisecond)
	mainPixel = capturePixel(t, svc, ctx, mainPoint)
	if !isOutlineRed(mainPixel) {
		t.Fatalf("resized main process outline pixel is %v, want red", mainPixel)
	}
	if err := svc.SetDebugBrowserPID(ctx, otherPID); err != nil {
		t.Fatalf("register secondary PID: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	otherPixel := capturePixel(t, svc, ctx, otherPoint)
	if !isOutlineRed(otherPixel) {
		t.Fatalf("secondary process outline pixel is %v, want red", otherPixel)
	}
	mainPixel = capturePixel(t, svc, ctx, mainPoint)
	if isOutlineRed(mainPixel) {
		t.Fatalf("main process retained an outline after registering the other PID: %v", mainPixel)
	}
	markerBus, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer markerBus.Close()
	if err := markerBus.Object(busName, busPath).CallWithContext(ctx,
		busIface+".SetDebugBrowserPID", 0, int32(otherPID), "0").Err; err != nil {
		t.Fatalf("register stale process token: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	otherPixel = capturePixel(t, svc, ctx, otherPoint)
	if isOutlineRed(otherPixel) {
		t.Fatalf("Shell retained an outline for a stale process token: %v", otherPixel)
	}
	if err := svc.SetDebugBrowserPID(ctx, mainPID); err != nil {
		t.Fatalf("restore main PID: %v", err)
	}
	proc, err := os.FindProcess(mainPID)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("close outline target process: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		windows, err = svc.Windows(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if findPIDWindow(windows, mainPID) == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if findPIDWindow(windows, mainPID) != nil {
		t.Fatal("the closed target still has a Shell window")
	}
	time.Sleep(200 * time.Millisecond)
	mainPixel = capturePixel(t, svc, ctx, mainPoint)
	if isOutlineRed(mainPixel) {
		t.Fatalf("outline remained after its window closed: %v", mainPixel)
	}
}

func findPIDWindow(windows []Window, pid int) *Window {
	for i := range windows {
		if windows[i].PID == pid {
			return &windows[i]
		}
	}
	return nil
}

func findPIDWindowTitle(windows []Window, pid int, title string) *Window {
	for i := range windows {
		if windows[i].PID == pid && windows[i].Title == title {
			return &windows[i]
		}
	}
	return nil
}

func outlineTopPixel(w *Window) Rect {
	return Rect{X: w.X + w.W/2, Y: w.Y + 1, W: 1, H: 1}
}

func capturePixel(t *testing.T, svc *Service, ctx context.Context, area Rect) color.NRGBA {
	t.Helper()
	shot, err := svc.Capture(ctx, Target{Kind: KindArea, Area: area}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(shot.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(img.Bounds().Min.X, img.Bounds().Min.Y).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func isOutlineRed(c color.NRGBA) bool {
	return c.R > 240 && c.G < 30 && c.B < 30 && c.A > 240
}
