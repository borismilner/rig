package capture

import (
	"context"
	"image/png"
	"os"
	"strings"
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
	if _, err := svc.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 90000, Y: 0, W: 10, H: 10}}, Options{}); err == nil {
		t.Error("an area outside the desktop was accepted")
	}
}
