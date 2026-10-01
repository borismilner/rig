// Command righand is the hand: rig drives the desktop's pointer and keyboard
// (plan/05 section 5m, logbook hand.md). It is its own program rather than a
// service in rigd because the daemon links no display dependency, and it is
// reached only through an invocation from rigd: it binds no socket.
//
// On GNOME Wayland it drives through mutter's remote-desktop API; on X11
// through XTEST. Which one is internal/hand's business and never appears in
// the declaration.
//
//	rig righand script --args '{"script":"window Terminal\nclick centre\ntype ls\nkey Return"}'
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/hand"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

const id = "righand"

var version = "dev"

// maxScript bounds a script; AgentBox's longest showcase was under 4 KiB.
const maxScript = 64 << 10

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "righand:", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := client.Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	p := &program{log: log}
	c.Handle(p.handle)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = c.Hello(ctx, declaration())
	cancel()
	if err != nil {
		return err
	}
	fmt.Println("righand: registered")
	<-c.Done()
	return nil
}

type program struct {
	log *slog.Logger
	// one script drives the desktop at a time: two interleaved scripts would
	// each click where the other left the pointer.
	busy sync.Mutex
}

func (p *program) handle(method string, payload []byte) (proto.Message, error) {
	_, command, _ := strings.Cut(method, ".")
	if command == "ping" {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: id, Version: version}, nil
	}
	var req rigv1.CallRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	var (
		out any
		err error
	)
	switch command {
	case "script":
		out, err = p.script(method, req.GetArgs())
	case "windows":
		out, err = p.windows()
	case "where":
		out, err = p.where()
	default:
		return nil, refuse(method, rigv1.Code_CODE_NOT_FOUND, "righand: no command "+command, "")
	}
	if err != nil {
		var ce *client.CallError
		if errors.As(err, &ce) {
			return nil, ce
		}
		return nil, refuse(method, rigv1.Code_CODE_INTERNAL, "righand: "+err.Error(), "")
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &rigv1.CallResponse{Result: body}, nil
}

type scriptArgs struct {
	Script string  `json:"script"`
	Speed  float64 `json:"speed"`
	WPM    int     `json:"wpm"`
}

func (p *program) script(method string, raw []byte) (any, error) {
	var a scriptArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, refuse(method, rigv1.Code_CODE_INVALID, "righand: script: "+err.Error(), "")
	}
	switch {
	case strings.TrimSpace(a.Script) == "":
		return nil, refuse(method, rigv1.Code_CODE_INVALID, "righand: script is empty", "pass one step per line")
	case len(a.Script) > maxScript:
		return nil, refuse(method, rigv1.Code_CODE_INVALID,
			fmt.Sprintf("righand: script is %d bytes, over %d", len(a.Script), maxScript), "split it into several calls")
	case a.Speed < 0 || a.Speed > 10:
		return nil, refuse(method, rigv1.Code_CODE_INVALID, "righand: speed is 0 (the default, 1) to 10", "")
	case a.WPM < 0 || a.WPM > 2000:
		return nil, refuse(method, rigv1.Code_CODE_INVALID, "righand: wpm is 0 (the default, 300) to 2000", "")
	}
	// Parsed whole before the first event, so a typo on line nine cannot
	// leave the desktop half driven.
	steps, err := hand.ParseScript(a.Script)
	if err != nil {
		return nil, refuse(method, rigv1.Code_CODE_INVALID, "righand: "+err.Error(), "")
	}
	if !p.busy.TryLock() {
		return nil, refuse(method, rigv1.Code_CODE_CONFLICT,
			"righand: another script is driving the desktop", "retry when it has finished")
	}
	defer p.busy.Unlock()

	h, err := hand.Open(0)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	if a.Speed > 0 {
		h.SetSpeed(a.Speed)
	}
	if a.WPM > 0 {
		h.SetWPM(a.WPM)
	}
	// The shape of the script, never its text: what was typed is declared
	// sensitive, and a window title can be too.
	p.log.Info("script", "steps", len(steps), "ops", ops(steps))
	ran, err := h.Run(steps)
	if err != nil {
		p.log.Warn("script stopped", "ran", ran, "of", len(steps))
		return nil, refuse(method, rigv1.Code_CODE_INTERNAL,
			fmt.Sprintf("righand: %v (ran %d of %d steps)", err, ran, len(steps)), "")
	}
	return map[string]any{"ran": ran, "of": len(steps)}, nil
}

func ops(steps []hand.Step) string {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = string(s.Op)
	}
	return strings.Join(names, " ")
}

type window struct {
	Title string `json:"title"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	W     int    `json:"w"`
	H     int    `json:"h"`
}

func (p *program) windows() (any, error) {
	h, err := hand.Open(0)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	cands, err := h.Windows()
	if err != nil {
		return nil, err
	}
	out := make([]window, 0, len(cands))
	for _, c := range cands {
		out = append(out, window{Title: c.Name, X: c.Rect.X, Y: c.Rect.Y, W: c.Rect.W, H: c.Rect.H})
	}
	return map[string]any{"windows": out}, nil
}

func (p *program) where() (any, error) {
	h, err := hand.Open(0)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	at, err := h.Pointer()
	if err != nil {
		return nil, err
	}
	s := h.Screen()
	return map[string]any{
		"pointer": map[string]int{"x": at.X, "y": at.Y},
		"screen":  map[string]int{"x": s.X, "y": s.Y, "w": s.W, "h": s.H},
	}, nil
}

func refuse(method string, code rigv1.Code, msg, fix string) error {
	return &client.CallError{Method: method, Status: &rigv1.Status{Code: code, Message: msg, Fix: fix}}
}

const scriptHelp = "One step per line: `window TITLE` (coordinates below are inside it, and it is " +
	"re-checked before every click and keystroke; prefix = for an exact title), `screen`, " +
	"`move X Y`, `click [button|X Y [button]]`, `double`, `drag X1 Y1 X2 Y2`, `scroll N` " +
	"(negative is up), `type TEXT` (rest of the line), `key ctrl+alt+t` (also Escape, Return, " +
	"Tab, End, arrows), `wait MS`, `speed N`, `wpm N`. A coordinate is 400 from the near edge, " +
	"-46 from the far edge, 60%, centre, ~ for the pointer, ~+30 relative to it. The whole " +
	"script is checked before the first event."

func declaration() *rigv1.Declaration {
	cmd := func(c *rigv1.Command) *rigv1.Command {
		c.Interactive = rigv1.Tristate_TRISTATE_NO
		c.Streams = rigv1.Tristate_TRISTATE_NO
		c.NeedsDisplay = rigv1.Tristate_TRISTATE_YES
		c.Shape = rigv1.Shape_SHAPE_UNARY
		return c
	}
	return &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id: id, Name: "Hand", Version: version, Icon: "hand",
			Description: "Moves the pointer and presses keys on the desktop, as a person would.",
		},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "section 5m's first slice: script, windows and where; no display lease or strip yet",
		SemanticsGen: 1,
		Commands: []*rigv1.Command{
			cmd(&rigv1.Command{
				Id: "script", Title: "Drive the desktop", Summary: "Move, click, drag, scroll and type",
				Description: scriptHelp,
				Returns:     "How many steps ran, of how many.",
				Args: []byte(`{"type":"object","properties":{` +
					`"script":{"type":"string","description":"the steps, one per line"},` +
					`"speed":{"type":"number","description":"movement speed, 1 is a hand's pace"},` +
					`"wpm":{"type":"integer","description":"typing speed, default 300"}},` +
					`"required":["script"]}`),
				Examples:   []string{`rig righand script --args '{"script":"screen\nmove centre centre"}'`},
				Effects:    rigv1.Effects_EFFECTS_DRIVES_INPUT,
				Idempotent: rigv1.Tristate_TRISTATE_NO,
				Sensitive:  &rigv1.SensitiveFields{Pointers: []string{"/script"}},
				Duration:   rigv1.Duration_DURATION_SECONDS,
				Confirms:   rigv1.Tristate_TRISTATE_YES,
			}),
			cmd(&rigv1.Command{
				Id: "windows", Title: "Windows on screen", Summary: "Titles and boxes, for planning a script",
				Description: "Every titled window on the current workspace with its rectangle.",
				Returns:     "The windows.",
				Args:        []byte(`{"type":"object","properties":{}}`),
				Effects:     rigv1.Effects_EFFECTS_READ_ONLY,
				Idempotent:  rigv1.Tristate_TRISTATE_YES,
				// titles are document names, tickets and browser tabs (hand.md)
				Sensitive: &rigv1.SensitiveFields{Pointers: []string{"/windows"}},
				Duration:  rigv1.Duration_DURATION_INSTANT,
				Confirms:  rigv1.Tristate_TRISTATE_NO,
			}),
			cmd(&rigv1.Command{
				Id: "where", Title: "Where the pointer is", Summary: "The pointer and the screen box",
				Description: "On Wayland the pointer cannot be read; this is where the hand last put it, " +
					"or XWayland's last known position.",
				Returns:    "The pointer and the screen.",
				Args:       []byte(`{"type":"object","properties":{}}`),
				Effects:    rigv1.Effects_EFFECTS_READ_ONLY,
				Idempotent: rigv1.Tristate_TRISTATE_YES,
				Sensitive:  &rigv1.SensitiveFields{},
				Duration:   rigv1.Duration_DURATION_INSTANT,
				Confirms:   rigv1.Tristate_TRISTATE_NO,
			}),
		},
	}
}
