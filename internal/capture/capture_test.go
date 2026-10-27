package capture

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fake is a compositor that paints solid pictures.
type fake struct {
	mu      sync.Mutex
	windows []Window
	outputs []Output
	// shadow is the margin around a window's frame, in pixels.
	shadow int
	// write replaces the picture writer when set.
	write func(path string) error
	// block holds each capture until released.
	block   chan struct{}
	entered chan struct{}
	calls   int
}

func solid(w, h int, c color.Color) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func writePNG(path string, img image.Image) error {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o600)
}

func (f *fake) Info(context.Context) (Info, error) { return Info{API: APIVersion, Shell: "50.1"}, nil }
func (f *fake) Windows(context.Context) ([]Window, error) {
	return append([]Window(nil), f.windows...), nil
}
func (f *fake) Outputs(context.Context) ([]Output, error) {
	return append([]Output(nil), f.outputs...), nil
}

func (f *fake) wait(ctx context.Context) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (f *fake) CaptureWindow(ctx context.Context, id uint64, path string) (Raw, error) {
	if err := f.wait(ctx); err != nil {
		return Raw{}, err
	}
	if f.write != nil {
		return Raw{}, f.write(path)
	}
	for _, w := range f.windows {
		if w.ID == id {
			s := f.shadow
			img := solid(w.W+2*s, w.H+2*s, color.NRGBA{R: 255, A: 255})
			// the shadow is transparent
			for y := 0; y < img.Bounds().Dy(); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					if x < s || y < s || x >= s+w.W || y >= s+w.H {
						img.SetNRGBA(x, y, color.NRGBA{})
					}
				}
			}
			return Raw{Width: img.Bounds().Dx(), Height: img.Bounds().Dy(),
				Frame: Rect{X: s, Y: s, W: w.W, H: w.H}, Scale: 1}, writePNG(path, img)
		}
	}
	return Raw{}, ErrNoTarget
}

func (f *fake) CaptureArea(ctx context.Context, r Rect, _ bool, path string) (Raw, error) {
	if err := f.wait(ctx); err != nil {
		return Raw{}, err
	}
	if f.write != nil {
		return Raw{}, f.write(path)
	}
	return Raw{Width: r.W, Height: r.H, Scale: 1}, writePNG(path, solid(r.W, r.H, color.NRGBA{G: 255, A: 255}))
}

func newFake() *fake {
	return &fake{
		shadow: 14,
		windows: []Window{
			{ID: 1, Title: "notes - editor", App: "gedit", W: 400, H: 300},
			{ID: 2, Title: "build log", App: "terminal", W: 800, H: 600, Focused: true},
			{ID: 3, Title: "build log (old)", App: "terminal", W: 200, H: 100},
		},
		outputs: []Output{
			{Index: 0, Name: "DP-1", X: 0, Y: 0, W: 1920, H: 1080, Scale: 1, Primary: true},
			{Index: 1, Name: "HDMI-1", X: 1920, Y: 0, W: 1280, H: 1024, Scale: 1},
		},
	}
}

func newSvc(t *testing.T, f *fake, lim Limits) *Service {
	t.Helper()
	s, err := New(f, filepath.Join(t.TempDir(), "cap"), func() Limits { return lim })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func dims(t *testing.T, path string) (int, int) {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	cfg, _, err := image.DecodeConfig(fh)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestWindowByIDIsCroppedToItsFrame(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	shot, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if shot.Width != 400 || shot.Height != 300 || shot.Shadowed {
		t.Fatalf("want a 400x300 frame, got %dx%d shadowed=%v", shot.Width, shot.Height, shot.Shadowed)
	}
	if w, h := dims(t, shot.Path); w != 400 || h != 300 {
		t.Fatalf("file is %dx%d", w, h)
	}
	if shot.Window == nil || shot.Window.ID != 1 || shot.SHA256 == "" || shot.Bytes <= 0 {
		t.Fatalf("incomplete shot: %+v", shot)
	}
	if st, _ := os.Stat(shot.Path); st.Mode().Perm() != 0o600 {
		t.Fatalf("picture mode %v, want 0600", st.Mode().Perm())
	}
}

func TestKeepShadowReturnsTheWholePaint(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	shot, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{KeepShadow: true})
	if err != nil {
		t.Fatal(err)
	}
	if shot.Width != 428 || shot.Height != 328 || !shot.Shadowed {
		t.Fatalf("got %dx%d shadowed=%v", shot.Width, shot.Height, shot.Shadowed)
	}
}

func TestMatchByTitleAndApp(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	shot, err := s.Capture(context.Background(), Target{Kind: KindWindow, Title: "EDITOR"}, Options{})
	if err != nil || shot.Window.ID != 1 {
		t.Fatalf("title match: %v %+v", err, shot.Window)
	}
	shot, err = s.Capture(context.Background(), Target{Kind: KindWindow, App: "term", Title: "old"}, Options{})
	if err != nil || shot.Window.ID != 3 {
		t.Fatalf("title and app: %v %+v", err, shot.Window)
	}
}

func TestAmbiguousMatchListsCandidatesAndFirstPicksFocused(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	_, err := s.Capture(context.Background(), Target{Kind: KindWindow, App: "terminal"}, Options{})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("want ErrAmbiguous, got %v", err)
	}
	if !strings.Contains(err.Error(), `2 "build log"`) || !strings.Contains(err.Error(), "3 ") {
		t.Fatalf("candidates missing: %v", err)
	}
	shot, err := s.Capture(context.Background(), Target{Kind: KindWindow, App: "terminal", First: true}, Options{})
	if err != nil || shot.Window.ID != 2 {
		t.Fatalf("first should take the focused one: %v %+v", err, shot.Window)
	}
}

func TestNoSuchWindow(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	for _, tg := range []Target{
		{Kind: KindWindow, ID: 99},
		{Kind: KindWindow, Title: "nothing like this"},
	} {
		if _, err := s.Capture(context.Background(), tg, Options{}); !errors.Is(err, ErrNoTarget) {
			t.Fatalf("%+v: want ErrNoTarget, got %v", tg, err)
		}
	}
}

func TestOutputScreenAndArea(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	ctx := context.Background()
	shot, err := s.Capture(ctx, Target{Kind: KindOutput, Output: 1}, Options{})
	if err != nil || shot.Width != 1280 || shot.Height != 1024 || shot.Output == nil || shot.Output.Name != "HDMI-1" {
		t.Fatalf("output 1: %v %+v", err, shot)
	}
	shot, err = s.Capture(ctx, Target{Kind: KindScreen}, Options{})
	if err != nil || shot.Width != 3200 || shot.Height != 1080 {
		t.Fatalf("screen is the union of the outputs: %v %dx%d", err, shot.Width, shot.Height)
	}
	shot, err = s.Capture(ctx, Target{Kind: KindArea, Area: Rect{X: 10, Y: 20, W: 300, H: 200}}, Options{})
	if err != nil || shot.Width != 300 || shot.Height != 200 {
		t.Fatalf("area: %v %dx%d", err, shot.Width, shot.Height)
	}
	if _, err = s.Capture(ctx, Target{Kind: KindOutput, Output: 5}, Options{}); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("output 5: want ErrNoTarget, got %v", err)
	}
}

func TestScreenWithNoMonitors(t *testing.T) {
	f := newFake()
	f.outputs = nil
	s := newSvc(t, f, Limits{})
	if _, err := s.Capture(context.Background(), Target{Kind: KindScreen}, Options{}); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("want ErrNoTarget, got %v", err)
	}
}

func TestMaxDimShrinksButNeverEnlarges(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	ctx := context.Background()
	shot, err := s.Capture(ctx, Target{Kind: KindOutput, Output: 0}, Options{MaxDim: 480})
	if err != nil || shot.Width != 480 || shot.Height != 270 {
		t.Fatalf("1920x1080 to 480: %v %dx%d", err, shot.Width, shot.Height)
	}
	shot, err = s.Capture(ctx, Target{Kind: KindWindow, ID: 3}, Options{MaxDim: 4000})
	if err != nil || shot.Width != 200 || shot.Height != 100 {
		t.Fatalf("a small picture stays: %v %dx%d", err, shot.Width, shot.Height)
	}
	shot, err = s.Capture(ctx, Target{Kind: KindArea, Area: Rect{W: 1000, H: 3}}, Options{MaxDim: 10})
	if err != nil || shot.Width != 10 || shot.Height != 1 {
		t.Fatalf("a thin picture keeps one row: %v %dx%d", err, shot.Width, shot.Height)
	}
}

func TestScalingKeepsColour(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	shot, err := s.Capture(context.Background(), Target{Kind: KindOutput, Output: 0}, Options{MaxDim: 64})
	if err != nil {
		t.Fatal(err)
	}
	fh, _ := os.Open(shot.Path)
	defer fh.Close()
	img, err := png.Decode(fh)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := img.At(10, 10).RGBA()
	if r != 0 || g>>8 != 255 || b != 0 || a>>8 != 255 {
		t.Fatalf("green should survive the average, got %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestJPEG(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	shot, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{Format: "jpeg", Quality: 60})
	if err != nil {
		t.Fatal(err)
	}
	if shot.Format != "jpeg" || !strings.HasSuffix(shot.Path, ".jpg") {
		t.Fatalf("got %s %s", shot.Format, shot.Path)
	}
	fh, _ := os.Open(shot.Path)
	defer fh.Close()
	if _, err := jpeg.Decode(fh); err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
}

func TestCrop(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	ctx := context.Background()
	shot, err := s.Capture(ctx, Target{Kind: KindOutput, Output: 0}, Options{Crop: Rect{X: 100, Y: 50, W: 200, H: 100}})
	if err != nil || shot.Width != 200 || shot.Height != 100 {
		t.Fatalf("crop: %v %dx%d", err, shot.Width, shot.Height)
	}
	shot, err = s.Capture(ctx, Target{Kind: KindOutput, Output: 0}, Options{Crop: Rect{X: 1800, Y: 1000, W: 500, H: 500}})
	if err != nil || shot.Width != 120 || shot.Height != 80 {
		t.Fatalf("a crop is clipped to the picture: %v %dx%d", err, shot.Width, shot.Height)
	}
	if _, err = s.Capture(ctx, Target{Kind: KindOutput, Output: 0}, Options{Crop: Rect{X: 5000, Y: 0, W: 10, H: 10}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a crop outside: want ErrInvalid, got %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(s.Dir(), ".tmp-*"))
	if len(files) != 0 {
		t.Fatalf("temporaries left behind: %v", files)
	}
}

func TestInline(t *testing.T) {
	s := newSvc(t, newFake(), Limits{InlineBytes: 1 << 20})
	ctx := context.Background()
	shot, err := s.Capture(ctx, Target{Kind: KindWindow, ID: 3}, Options{Inline: 1 << 20})
	if err != nil || int64(len(shot.Data)) != shot.Bytes {
		t.Fatalf("inline: %v %d of %d", err, len(shot.Data), shot.Bytes)
	}
	shot, _ = s.Capture(ctx, Target{Kind: KindWindow, ID: 3}, Options{Inline: 10})
	if shot.Data != nil {
		t.Fatal("a picture over the caller's inline cap must not be returned inline")
	}
	s = newSvc(t, newFake(), Limits{InlineBytes: 10})
	shot, _ = s.Capture(ctx, Target{Kind: KindWindow, ID: 3}, Options{Inline: 1 << 30})
	if shot.Data != nil {
		t.Fatal("the service cap beats the caller's")
	}
}

func TestCheckRefusesMalformedRequests(t *testing.T) {
	win := Target{Kind: KindWindow, ID: 1}
	cases := map[string]struct {
		t Target
		o Options
	}{
		"no kind":          {Target{}, Options{}},
		"empty window":     {Target{Kind: KindWindow}, Options{}},
		"cursor on window": {win, Options{Cursor: true}},
		"bad format":       {win, Options{Format: "bmp"}},
		"quality":          {win, Options{Quality: 101}},
		"negative quality": {win, Options{Quality: -1}},
		"max_dim":          {win, Options{MaxDim: 100000}},
		"negative max_dim": {win, Options{MaxDim: -3}},
		"negative inline":  {win, Options{Inline: -1}},
		"half a crop":      {win, Options{Crop: Rect{W: 10}}},
		"negative crop":    {win, Options{Crop: Rect{X: -1, W: 1, H: 1}}},
		"huge title":       {Target{Kind: KindWindow, Title: strings.Repeat("x", 500)}, Options{}},
		"output index":     {Target{Kind: KindOutput, Output: -1}, Options{}},
		"empty area":       {Target{Kind: KindArea}, Options{}},
		"huge area":        {Target{Kind: KindArea, Area: Rect{W: 1 << 30, H: 1}}, Options{}},
	}
	for name, c := range cases {
		if err := Check(c.t, c.o); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	if err := Check(win, Options{Format: "jpeg", Quality: 70, MaxDim: 800}); err != nil {
		t.Errorf("a good request was refused: %v", err)
	}
}

func TestBusyWhenTooManyInFlight(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{})
	f.entered = make(chan struct{}, 4)
	s := newSvc(t, f, Limits{Concurrent: 2})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{})
		}()
	}
	<-f.entered
	<-f.entered
	if _, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("want ErrBusy, got %v", err)
	}
	close(f.block)
	wg.Wait()
	if _, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{}); err != nil {
		t.Fatalf("after the others finished: %v", err)
	}
}

func TestTimeoutIsReportedAndLeavesNothingBehind(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{}) // never released
	s := newSvc(t, f, Limits{Timeout: 50 * time.Millisecond})
	start := time.Now()
	_, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
	if files, _ := os.ReadDir(s.Dir()); len(files) != 0 {
		t.Fatalf("left behind: %v", files)
	}
}

func TestCallerCancellation(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{})
	f.entered = make(chan struct{}, 1)
	s := newSvc(t, f, Limits{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-f.entered; cancel() }()
	if _, err := s.Capture(ctx, Target{Kind: KindWindow, ID: 1}, Options{}); err == nil {
		t.Fatal("a cancelled capture must fail")
	}
	if files, _ := os.ReadDir(s.Dir()); len(files) != 0 {
		t.Fatalf("left behind: %v", files)
	}
}

// bigHeader is a PNG whose header claims w by h and whose body is absent: the
// shape of a decompression bomb. The CRC is right, so only a limit stops it.
func bigHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6
	chunk := append([]byte("IHDR"), ihdr...)
	_ = binary.Write(&b, binary.BigEndian, uint32(len(ihdr)))
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func TestHostileFilesFromTheBackend(t *testing.T) {
	cases := map[string]struct {
		write func(path string) error
		want  error
	}{
		"empty file": {func(p string) error { return os.WriteFile(p, nil, 0o600) }, ErrBadImage},
		"not a png":  {func(p string) error { return os.WriteFile(p, []byte("hello, I am not a picture"), 0o600) }, ErrBadImage},
		"truncated":  {func(p string) error { return os.WriteFile(p, bigHeader(10, 10), 0o600) }, ErrBadImage},
		"pixel bomb": {func(p string) error { return os.WriteFile(p, bigHeader(60000, 60000), 0o600) }, ErrTooLarge},
		"zero size":  {func(p string) error { return os.WriteFile(p, bigHeader(0, 5), 0o600) }, ErrBadImage},
		"too many bytes": {func(p string) error {
			return os.WriteFile(p, append(bigHeader(1, 1), make([]byte, 5000)...), 0o600)
		}, ErrTooLarge},
		"never written": {func(string) error { return nil }, ErrBadImage},
		"a symlink": {func(p string) error {
			target := filepath.Join(filepath.Dir(p), "elsewhere.png")
			if err := writePNG(target, solid(4, 4, color.White)); err != nil {
				return err
			}
			return os.Symlink(target, p)
		}, ErrBadImage},
		"a directory": {func(p string) error { return os.Mkdir(p, 0o700) }, ErrBadImage},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			f.write = c.write
			s := newSvc(t, f, Limits{MaxBytes: 4096})
			_, err := s.Capture(context.Background(), Target{Kind: KindArea, Area: Rect{W: 4, H: 4}}, Options{})
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
			for _, e := range readDir(t, s.Dir()) {
				if strings.HasPrefix(e, "shot-") {
					t.Fatalf("a rejected file was kept: %s", e)
				}
			}
		})
	}
}

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestBackendErrorsPassThrough(t *testing.T) {
	f := newFake()
	f.write = func(string) error { return ErrUnavailable }
	s := newSvc(t, f, Limits{})
	if _, err := s.Capture(context.Background(), Target{Kind: KindArea, Area: Rect{W: 4, H: 4}}, Options{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestPrune(t *testing.T) {
	f := newFake()
	s := newSvc(t, f, Limits{KeepFiles: 3})
	foreign := filepath.Join(s.Dir(), "notes.txt")
	if err := os.WriteFile(foreign, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(s.Dir(), ".tmp-old.png")
	_ = os.WriteFile(stale, []byte("x"), 0o600)
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(stale, old, old)
	var last Shot
	for i := 0; i < 8; i++ {
		var err error
		last, err = s.Capture(context.Background(), Target{Kind: KindWindow, ID: 3}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(time.Duration(i-8) * time.Second)
		_ = os.Chtimes(last.Path, at, at)
	}
	var shots int
	for _, n := range readDir(t, s.Dir()) {
		if strings.HasPrefix(n, "shot-") {
			shots++
		}
	}
	if shots > 4 { // KeepFiles, plus the newest which is never pruned
		t.Fatalf("%d pictures kept against a limit of 3", shots)
	}
	if _, err := os.Stat(last.Path); err != nil {
		t.Fatal("the picture just taken was pruned")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("pruning touched a file that is not a picture")
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("a stale temporary was kept")
	}
}

func TestPruneByAgeAndBytes(t *testing.T) {
	s := newSvc(t, newFake(), Limits{KeepAge: time.Hour})
	a, _ := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 3}, Options{})
	old := time.Now().Add(-3 * time.Hour)
	_ = os.Chtimes(a.Path, old, old)
	b, _ := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 3}, Options{})
	if _, err := os.Stat(a.Path); err == nil {
		t.Fatal("an old picture was kept")
	}
	if _, err := os.Stat(b.Path); err != nil {
		t.Fatal("the new picture is gone")
	}
	s = newSvc(t, newFake(), Limits{KeepBytes: 1})
	c, _ := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 3}, Options{})
	if _, err := os.Stat(c.Path); err != nil {
		t.Fatal("the newest picture must survive even over the byte limit")
	}
}

func TestDirectoryIsPrivate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cap")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(newFake(), dir, nil); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v, want 0700", st.Mode().Perm())
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(newFake(), link, nil); err == nil {
		t.Fatal("a symlinked directory must be refused")
	}
	if _, err := New(newFake(), "relative/dir", nil); err == nil {
		t.Fatal("a relative directory must be refused")
	}
	if _, err := New(nil, dir, nil); err == nil {
		t.Fatal("no backend must be refused")
	}
}

func TestWindowsListingFilter(t *testing.T) {
	s := newSvc(t, newFake(), Limits{})
	all, err := s.Windows(context.Background(), "")
	if err != nil || len(all) != 3 {
		t.Fatalf("%v %d", err, len(all))
	}
	some, _ := s.Windows(context.Background(), "BUILD")
	if len(some) != 2 {
		t.Fatalf("want 2, got %d", len(some))
	}
	if _, err := s.Windows(context.Background(), strings.Repeat("x", 1000)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestLiveLimitsApplyToTheNextCapture(t *testing.T) {
	lim := Limits{MaxPixels: 100}
	s, err := New(newFake(), filepath.Join(t.TempDir(), "c"), func() Limits { return lim })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	lim = Limits{}
	if _, err := s.Capture(context.Background(), Target{Kind: KindWindow, ID: 1}, Options{}); err != nil {
		t.Fatalf("after the limit was lifted: %v", err)
	}
}

func FuzzCheck(f *testing.F) {
	f.Add(1, uint64(0), "t", "a", 3, 0, 0, 100, 100, "png", 80, 800)
	f.Fuzz(func(t *testing.T, kind int, id uint64, title, app string, out, ax, ay, aw, ah int, format string, q, dim int) {
		_ = Check(Target{Kind: Kind(kind), ID: id, Title: title, App: app, Output: out, Area: Rect{ax, ay, aw, ah}},
			Options{Format: format, Quality: q, MaxDim: dim})
	})
}
