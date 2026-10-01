package hand

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A script is the form this feature is actually used in. One call that moves,
// clicks, types and waits beats six calls that each pay for a process, an X
// connection and a round trip, and it keeps the interesting property: the whole
// sequence is parsed and checked before the first event is sent, so a typo in
// step nine cannot leave the desktop half-driven with a button held down.
//
//	window agentbox          # coordinates below are inside that window
//	move 25% -46          # 25% across, 46px up from the bottom edge
//	click
//	type Ship it
//	key ctrl+Return
//	wait 400
//
// Blank lines and # comments are ignored. `type` and `window` take the rest of
// the line verbatim, so no quoting rules to remember.

// Op is one kind of step.
type Op string

const (
	OpWindow Op = "window"
	OpScreen Op = "screen"
	OpMove   Op = "move"
	OpClick  Op = "click"
	OpDouble Op = "double"
	OpDrag   Op = "drag"
	OpScroll Op = "scroll"
	OpType   Op = "type"
	OpKey    Op = "key"
	OpWait   Op = "wait"
	OpSpeed  Op = "speed"
	OpWPM    Op = "wpm"
)

// Step is one parsed line.
type Step struct {
	Op   Op
	Line int
	Raw  string

	Text   string   // window title, or the text to type
	Keys   []string // key combinations, in order
	X, Y   Coord    // move / click / drag from
	To     bool     // this step moves before it acts
	X2, Y2 Coord    // drag to
	Button byte     // 1 left, 2 middle, 3 right, 4-7 wheel
	N      int      // scroll notches (+down), wait milliseconds
	F      float64  // speed multiplier
	// Note is what the step is for, in words he reads on the strip (plan/05
	// H7). It carries on to the steps after it until another note.
	Note string
}

// maxNote bounds a note; the strip has one line for it.
const maxNote = 120

// checkNote refuses a note the strip could not show.
func checkNote(n string) error {
	if len(n) > maxNote {
		return fmt.Errorf("a note is at most %d bytes; the strip has one line for it", maxNote)
	}
	if strings.ContainsAny(n, "\n\r") {
		return errors.New("a note is one line")
	}
	return nil
}

// where names a step in an error. The text it would type is left out: what
// is typed is declared sensitive, and an error travels further than a log.
func (s Step) where() string {
	switch {
	case s.Line == 0:
		return s.Raw // the typed form's "step 3, click"
	case s.Op == OpType:
		return fmt.Sprintf("line %d (type)", s.Line)
	default:
		return fmt.Sprintf("line %d (%s)", s.Line, s.Raw)
	}
}

// Coord is one axis of a position, in the frame the script is currently in.
// The spellings exist because the useful positions are rarely absolute pixels:
// a card's buttons sit a fixed distance from its bottom edge whatever its height,
// and the middle of a window is the middle whatever its size.
//
//	400     400 pixels from the left/top edge
//	-46     46 pixels from the right/bottom edge
//	60%     60% of the way across/down
//	-25%    25% of the way in from the right/bottom edge
//	centre  the middle of that axis (the US spelling and middle too)
//	~       leave this axis where the pointer already is
//	~+30    30 pixels right/down of the pointer
type Coord struct {
	Kind  CoordKind
	Value float64
}

type CoordKind int

const (
	CoordEdge    CoordKind = iota // Value pixels from the near edge
	CoordFromEnd                  // Value pixels from the far edge
	CoordFrac                     // Value in 0..1 from the near edge
	CoordFracEnd                  // Value in 0..1 from the far edge
	CoordCenter
	CoordPointer // Value pixels from where the pointer is
)

// Resolve turns one axis into a root coordinate. origin and size describe the
// current frame on that axis; at is where the pointer is on it.
func (c Coord) Resolve(origin, size, at int) int {
	switch c.Kind {
	case CoordFromEnd:
		return origin + size - int(c.Value)
	case CoordFrac:
		return origin + int(c.Value*float64(size))
	case CoordFracEnd:
		return origin + size - int(c.Value*float64(size))
	case CoordCenter:
		return origin + size/2
	case CoordPointer:
		return at + int(c.Value)
	default:
		return origin + int(c.Value)
	}
}

// ParseCoord reads one coordinate token.
func ParseCoord(tok string) (Coord, error) {
	t := strings.TrimSpace(tok)
	switch {
	case t == "":
		return Coord{}, errors.New("empty coordinate")
	case strings.EqualFold(t, "centre"), strings.EqualFold(t, "center"), strings.EqualFold(t, "middle"): //nolint:misspell // both spellings are accepted
		return Coord{Kind: CoordCenter}, nil
	case t == "~":
		return Coord{Kind: CoordPointer}, nil
	case strings.HasPrefix(t, "~"):
		v, err := strconv.ParseFloat(strings.TrimPrefix(t, "~"), 64)
		if err != nil {
			return Coord{}, fmt.Errorf("%q is not a pointer-relative offset", tok)
		}
		return Coord{Kind: CoordPointer, Value: v}, nil
	}

	pct := strings.HasSuffix(t, "%")
	t = strings.TrimSuffix(t, "%")
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return Coord{}, fmt.Errorf("%q is not a coordinate", tok)
	}
	fromEnd := v < 0 || t == "-0"
	if v < 0 {
		v = -v
	}
	switch {
	case pct && fromEnd:
		return Coord{Kind: CoordFracEnd, Value: v / 100}, nil
	case pct:
		return Coord{Kind: CoordFrac, Value: v / 100}, nil
	case fromEnd:
		return Coord{Kind: CoordFromEnd, Value: v}, nil
	default:
		return Coord{Kind: CoordEdge, Value: v}, nil
	}
}

// buttons a script may name.
var buttonNames = map[string]byte{
	"left": 1, "l": 1, "middle": 2, "m": 2, "right": 3, "r": 3,
}

func parseButton(tok string) (byte, error) {
	if b, ok := buttonNames[strings.ToLower(tok)]; ok {
		return b, nil
	}
	n, err := strconv.Atoi(tok)
	if err != nil || n < 1 || n > 9 {
		return 0, fmt.Errorf("%q is not a mouse button (left, middle, right, or 1-9)", tok)
	}
	return byte(n), nil
}

// ParseScript reads a whole script. Every step is checked here so that Run can
// assume it is valid, and so a bad line is reported with its number before
// anything moves.
func ParseScript(src string) ([]Step, error) {
	var out []Step
	note := ""
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// "note <words>" is not a step: it says what the steps after it are
		// for, on the strip, until the next note.
		if op, rest, _ := strings.Cut(line, " "); strings.EqualFold(op, "note") {
			note = strings.TrimSpace(rest)
			if err := checkNote(note); err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			continue
		}
		st, err := ParseStep(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		st.Line, st.Raw, st.Note = i+1, line, note
		out = append(out, st)
	}
	if len(out) == 0 {
		return nil, errors.New("the script has no steps")
	}
	return out, nil
}

// ParseStep reads one line.
func ParseStep(line string) (Step, error) {
	op, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	fields := strings.Fields(rest)
	st := Step{Op: Op(strings.ToLower(op))}

	var err error
	switch st.Op {
	case OpWindow, OpScreen, OpType, OpKey:
		err = parseTextStep(&st, rest, fields)
	case OpMove, OpClick, OpDouble, OpDrag:
		err = parsePointerStep(&st, fields)
	case OpScroll, OpWait, OpSpeed, OpWPM:
		err = parseNumberStep(&st, fields)
	default:
		err = fmt.Errorf("unknown step %q (window, screen, move, click, double, drag, scroll, type, key, wait, speed, wpm)", op)
	}
	return st, err
}

// parseTextStep reads the steps whose argument is words: a title, text to
// type, or key combinations.
func parseTextStep(st *Step, rest string, fields []string) error {
	switch st.Op {
	case OpWindow:
		if rest == "" {
			return errors.New("window needs a title to look for")
		}
		st.Text = rest
	case OpScreen:
		if rest != "" {
			return errors.New("screen takes no arguments")
		}
	case OpType:
		if rest == "" {
			return errors.New("type needs something to type")
		}
		st.Text = rest
	default: // OpKey
		if len(fields) == 0 {
			return errors.New("key needs a combination, for example ctrl+alt+t or Escape")
		}
		st.Keys = fields
	}
	return nil
}

// parseCoords reads x y pairs into dst, in order.
func parseCoords(fields []string, dst ...*Coord) error {
	for i, d := range dst {
		c, err := ParseCoord(fields[i])
		if err != nil {
			return err
		}
		*d = c
	}
	return nil
}

// parsePointerStep reads the steps that move the pointer or press a button.
func parsePointerStep(st *Step, fields []string) error {
	switch st.Op {
	case OpMove:
		if len(fields) != 2 {
			return errors.New("move needs an x and a y, for example: move 25% -46")
		}
		st.To = true
		return parseCoords(fields, &st.X, &st.Y)
	case OpDrag:
		if len(fields) != 4 {
			return errors.New("drag needs x1 y1 x2 y2")
		}
		st.To, st.Button = true, 1
		return parseCoords(fields, &st.X, &st.Y, &st.X2, &st.Y2)
	}
	// OpClick, OpDouble
	st.Button = 1
	var err error
	switch len(fields) {
	case 0:
	case 1:
		st.Button, err = parseButton(fields[0])
	case 2, 3:
		st.To = true
		if err = parseCoords(fields, &st.X, &st.Y); err == nil && len(fields) == 3 {
			st.Button, err = parseButton(fields[2])
		}
	default:
		err = fmt.Errorf("%s takes an optional button, or x y, or x y button", st.Op)
	}
	return err
}

// parseNumberStep reads the steps that take one number.
func parseNumberStep(st *Step, fields []string) error {
	usage := map[Op]string{
		OpScroll: "scroll needs a number of notches (negative scrolls up)",
		OpWait:   "wait needs milliseconds",
		OpSpeed:  "speed needs a multiplier, for example 1.5",
		OpWPM:    "wpm needs a typing speed, for example 240",
	}
	if len(fields) != 1 {
		return errors.New(usage[st.Op])
	}
	if st.Op == OpSpeed {
		f, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || f <= 0 {
			return fmt.Errorf("%q is not a speed multiplier", fields[0])
		}
		st.F = f
		return nil
	}
	n, err := strconv.Atoi(fields[0])
	switch {
	case st.Op == OpScroll && (err != nil || n == 0):
		return fmt.Errorf("%q is not a number of notches", fields[0])
	case st.Op == OpWait && (err != nil || n < 0):
		return fmt.Errorf("%q is not a number of milliseconds", fields[0])
	case st.Op == OpWPM && (err != nil || n <= 0):
		return fmt.Errorf("%q is not a typing speed", fields[0])
	}
	st.N = n
	return nil
}

// Shown is the step as the strip may show it: the op and what it acts on,
// and never the text a type step types, which is declared sensitive. The
// window title and the keys are shown, since they are what he checks.
func (s Step) Shown() string {
	switch s.Op {
	case OpWindow:
		t := s.Text
		if r := []rune(t); len(r) > 40 {
			t = string(r[:40]) + "…"
		}
		return fmt.Sprintf("window %q", t)
	case OpType:
		return fmt.Sprintf("type %d characters", utf8.RuneCountInString(s.Text))
	case OpKey:
		return "key " + strings.Join(s.Keys, " ")
	case OpWait:
		return fmt.Sprintf("wait %dms", s.N)
	case OpScroll:
		return fmt.Sprintf("scroll %d", s.N)
	}
	return string(s.Op)
}
