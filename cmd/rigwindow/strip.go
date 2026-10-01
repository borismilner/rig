package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// The HANDS OFF strip, the screen's half (plan/05 section 5m, H1-H5). rigd
// holds the one run and its clock; this only draws it and carries his
// answer back. It follows the toasts' footprint ruling: the tray holds one
// long poll on rig.hand.wait, and a run that is not idle starts
// `rigwindow --strip`, which exits a moment after the run is over. When the
// strip cannot start, the countdown goes to the desktop's notifications,
// because silence is consent and a countdown he cannot see is not one.

//go:embed strip.html
var stripPage []byte

const (
	stripWidth  = 720 // the window; the bar inside is 680 plus room for its shadow
	stripHeight = 112
	stripMargin = 6
	// How long the strip stays after the run ends, so he reads how it ended.
	stripLinger = 2500 * time.Millisecond
	handPoll    = 55 * time.Second
)

// --- the tray's half ---------------------------------------------------------

// stripWatcher starts the strip when a run begins. running is true while a
// strip process is alive.
type stripWatcher struct {
	spawn    func() (<-chan error, error)
	fallback func(*registryv1.HandState) error
	warn     func(string)

	mu      sync.Mutex
	running bool
}

// seen is called with each state the tray's wait returns.
func (w *stripWatcher) seen(st *registryv1.HandState) {
	if st.GetPhase() == registryv1.HandPhase_HAND_PHASE_IDLE {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	exited, err := w.spawn()
	if err != nil {
		w.warn("the HANDS OFF strip would not start, using the desktop's notifications: " + err.Error())
		if err := w.fallback(st); err != nil {
			w.warn("the desktop's notification service refused too: " + err.Error())
		}
		return
	}
	w.running = true
	started := time.Now()
	go func() {
		err := <-exited
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		if err != nil && time.Since(started) < 5*time.Second {
			w.warn("the HANDS OFF strip died at start, using the desktop's notifications: " + err.Error())
			_ = w.fallback(st)
		}
	}()
}

// watchHand is the tray's long poll on the desktop's run.
func watchHand(w *stripWatcher) {
	var after uint64
	for {
		st, err := handWait(after, handPoll)
		if err != nil {
			after = 0 // a restarted rigd counts from 1 again
			time.Sleep(trayRefresh)
			continue
		}
		after = st.GetSeq()
		w.seen(st)
	}
}

// handWait is one rig.hand.wait: the state once its seq passes after, or as
// it is when wait runs out.
func handWait(after uint64, wait time.Duration) (*registryv1.HandState, error) {
	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), wait+readDeadline)
	defer cancel()
	var resp registryv1.HandWaitResponse
	err = c.Call(ctx, "rig.hand.wait", &registryv1.HandWaitRequest{
		After: after, TimeoutMs: uint32(wait.Milliseconds()), //nolint:gosec // under a minute
	}, &resp)
	return resp.GetState(), err
}

// spawnStrip runs this binary again as the strip.
func spawnStrip() (<-chan error, error) {
	exe, err := selfExecutable()
	if err != nil {
		return nil, err
	}
	//rig:allow nocontextfree: the strip ends itself once the run is over, so its end is the exit channel
	cmd := exec.CommandContext(context.Background(), exe, "--strip")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

// notifyHand is the fallback: the countdown as an urgent desktop
// notification, with the terminal words that answer it.
func notifyHand(st *registryv1.HandState) error {
	return notifyDesktop(&registryv1.Toast{
		Severity: registryv1.Severity_SEVERITY_URGENT,
		Title:    st.GetHolder() + " asks for the desktop",
		Body:     st.GetReason() + "\nrig hand hold, or rig hand decline, in a terminal",
	})
}

// --- the strip's half --------------------------------------------------------

// stripJSON is the state as the page draws it. Every duration is worked out
// here from rigd's clock, so the page never compares two clocks.
type stripJSON struct {
	Phase    string `json:"phase"`
	Holder   string `json:"holder"`
	Reason   string `json:"reason"`
	Activity string `json:"activity"`
	Ended    string `json:"ended"`
	// ASKING: until it drives. HELD: until the hold declines. PAUSED: until
	// the pause stops the run.
	LeftMs   int64 `json:"left_ms"`
	WindowMs int64 `json:"window_ms"`
	// HELD: what was left of the countdown when he held it.
	HeldMs int64 `json:"held_ms"`
	// DRIVING and PAUSED: since the run started driving.
	SinceMs int64 `json:"since_ms"`
	Quit    bool  `json:"quit"`
}

// stripFeed is the strip's state between rigd and the page.
type stripFeed struct {
	mu     sync.Mutex
	st     *registryv1.HandState
	idleAt time.Time // when the run was last seen go idle
	answer func(registryv1.HandAction) error
}

func (f *stripFeed) set(st *registryv1.HandState, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idle := st.GetPhase() == registryv1.HandPhase_HAND_PHASE_IDLE
	switch {
	case !idle:
		f.idleAt = time.Time{}
	case f.idleAt.IsZero():
		f.idleAt = now
	}
	f.st = st
}

// done is true once the run has been idle for stripLinger.
func (f *stripFeed) done(now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.idleAt.IsZero() && now.Sub(f.idleAt) > stripLinger
}

func (f *stripFeed) view(now time.Time) stripJSON {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.st
	out := stripJSON{
		Phase:  strings.ToLower(strings.TrimPrefix(st.GetPhase().String(), "HAND_PHASE_")),
		Holder: st.GetHolder(), Reason: st.GetReason(), Activity: st.GetActivity(), Ended: st.GetEnded(),
		WindowMs: int64(st.GetWindowMs()), HeldMs: int64(st.GetLeftMs()),
		Quit: !f.idleAt.IsZero() && now.Sub(f.idleAt) > stripLinger,
	}
	if dl := st.GetDeadlineUnixNano(); dl != 0 {
		out.LeftMs = max(time.Duration(dl-now.UnixNano()), 0).Milliseconds()
	}
	if d := st.GetDrivingUnixNano(); d != 0 {
		out.SinceMs = max(now.Sub(time.Unix(0, d)), 0).Milliseconds()
	}
	return out
}

// stripActions are the page's buttons, by the word each posts.
var stripActions = map[string]registryv1.HandAction{
	"allow": registryv1.HandAction_HAND_ACTION_ALLOW, "decline": registryv1.HandAction_HAND_ACTION_DECLINE,
	"hold": registryv1.HandAction_HAND_ACTION_HOLD, "pause": registryv1.HandAction_HAND_ACTION_PAUSE,
	"resume": registryv1.HandAction_HAND_ACTION_RESUME, "stop": registryv1.HandAction_HAND_ACTION_STOP,
}

// ServeHTTP is the page's only door to Go.
func (f *stripFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/strip.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(stripPage)
	case "/strip/poll":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.view(time.Now()))
	case "/strip/answer":
		action, ok := stripActions[r.URL.Query().Get("action")]
		if r.Method != http.MethodPost || !ok || f.answer == nil {
			http.Error(w, "answer is POST with a known action", http.StatusBadRequest)
			return
		}
		if err := f.answer(action); err != nil {
			http.Error(w, err.Error(), http.StatusConflict) // the page shows it on the bar
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// answerHand files his answer. The strip's connection is neither a program
// nor an agent, which is what rigd lets answer (H5).
func answerHand(a registryv1.HandAction) error {
	c, err := client.Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()
	return c.Call(ctx, "rig.hand.answer", &registryv1.HandAnswerRequest{Action: a}, &registryv1.HandAnswerResponse{})
}

// runStrip is the strip process: one frameless, always-on-top bar, never
// under the hand (H6), gone once the run is over.
func runStrip() error {
	//rig:allow nocontextfree: the strip lives until the run is over, so its end is app.Quit rather than a deadline
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed := &stripFeed{st: &registryv1.HandState{}, answer: answerHand}
	// XWayland, so the window can refuse focus and be placed: strip_gtk.go.
	if os.Getenv("GDK_BACKEND") == "" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}
	app := application.New(application.Options{
		Name:     "rig hands off",
		Assets:   application.AssetOptions{Handler: feed},
		LogLevel: slog.LevelWarn,
	})
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Rig HANDS OFF",
		Width:          stripWidth,
		Height:         stripHeight,
		Hidden:         true,
		Frameless:      true,
		AlwaysOnTop:    true,
		DisableResize:  true,
		BackgroundType: application.BackgroundTypeTransparent,
		Linux:          application.LinuxWindow{WindowIsTranslucent: true},
		URL:            "/strip.html",
	})
	go func() {
		var after uint64
		for ctx.Err() == nil {
			st, err := handWait(after, handPoll)
			if err != nil {
				// rigd gone: nothing can drive, so the strip has nothing
				// to guard. It says so by ending.
				feed.set(&registryv1.HandState{Phase: registryv1.HandPhase_HAND_PHASE_IDLE, Ended: "rig stopped"}, time.Now())
				time.Sleep(time.Second)
				after = 0
				continue
			}
			after = st.GetSeq()
			feed.set(st, time.Now())
		}
	}()
	go func() {
		spot := &stripSpot{}
		defer spot.close()
		tick := time.NewTicker(40 * time.Millisecond)
		defer tick.Stop()
		for range tick.C {
			if feed.done(time.Now()) {
				app.Quit()
				return
			}
			if !spot.placed && !spot.place(app, win) {
				continue
			}
			p, has := feed.aim()
			spot.dodge(win, p, has)
		}
	}()
	return app.Run()
}

// aim is the point a step waits on, if any.
func (f *stripFeed) aim() (stripPt, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return stripPt{int(f.st.GetAimX()), int(f.st.GetAimY())}, f.st.GetHasAim()
}

type stripPt struct{ x, y int }

// stripSpot is where the strip is and where it may go: the bottom centre of
// the work area by his panel, or the top centre when the hand needs the
// bottom (H6, his choice on 2026-10-01). It reports every move to rigd on
// one held connection, which rigd forgets when the strip exits.
type stripSpot struct {
	placed      bool
	x, top, bot int
	atTop       bool
	c           *client.Client
}

func (s *stripSpot) y() int {
	if s.atTop {
		return s.top
	}
	return s.bot
}

// covers is true when p is under the strip; rigd pads it, so this need not.
func (s *stripSpot) covers(p stripPt) bool {
	return p.x >= s.x && p.x < s.x+stripWidth && p.y >= s.y() && p.y < s.y()+stripHeight
}

// place puts the bar at the bottom centre of the primary work area.
func (s *stripSpot) place(app *application.App, win *application.WebviewWindow) bool {
	scr := app.Screen.GetPrimary()
	if scr == nil {
		return false
	}
	wa := scr.WorkArea
	if x11, ok := application.InvokeSyncWithResult(func() workArea {
		r, ok := x11WorkArea()
		return workArea{r, ok}
	}).get(); ok && x11.Width > 0 && x11.Height > 0 {
		wa = x11
	}
	s.x = wa.X + (wa.Width-stripWidth)/2
	s.top, s.bot = wa.Y+stripMargin, wa.Y+wa.Height-stripHeight-stripMargin
	themeOnce.Do(func() { application.InvokeSync(clearThemeBackground) })
	if !application.InvokeSyncWithResult(func() bool { return stripNoFocus(win, s.x, s.y()) }) {
		fmt.Fprintln(os.Stderr, "rigwindow: the HANDS OFF strip is not an X11 window, so it cannot refuse the keyboard")
	}
	win.Show()
	s.placed = true
	s.report()
	return true
}

// dodge moves the strip to the other edge when the point a step waits on is
// under it, and tells rigd where it went, which is what lets the step go on.
func (s *stripSpot) dodge(win *application.WebviewWindow, p stripPt, has bool) {
	if !has || !s.covers(p) {
		return
	}
	s.atTop = !s.atTop
	if s.covers(p) {
		return // a strip on either edge covers it: rigd refuses the step
	}
	application.InvokeSync(func() { stripMove(win, s.x, s.y()) })
	s.report()
}

// report tells rigd the strip's rect, dialling again if the held connection
// was lost.
func (s *stripSpot) report() {
	for range 2 {
		if s.c == nil {
			c, err := client.Connect()
			if err != nil {
				return
			}
			s.c = c
		}
		ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
		err := s.c.Call(ctx, "rig.hand.strip", &registryv1.HandStripRequest{
			X: int32(s.x), Y: int32(s.y()), Width: stripWidth, Height: stripHeight, //nolint:gosec // screen coordinates
		}, &registryv1.HandStripResponse{})
		cancel()
		if err == nil {
			return
		}
		fmt.Fprintln(os.Stderr, "rigwindow: rigd did not take the strip's place: "+err.Error())
		s.close()
	}
}

func (s *stripSpot) close() {
	if s.c != nil {
		_ = s.c.Close()
		s.c = nil
	}
}

func (w *stripWatcher) isRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
