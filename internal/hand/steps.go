package hand

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// The typed form of a script: an array of step objects, which is the contract
// (plan/05 section 5m). The one-step-per-line text that ParseScript reads is
// sugar over it, so both produce the same []Step and Run cannot tell them
// apart.
//
//	[{"op":"window","title":"Terminal"},
//	 {"op":"click","x":"centre","y":"centre"},
//	 {"op":"type","text":"ls\n"},
//	 {"op":"key","keys":["ctrl+l"]}]
//
// A coordinate is a number (pixels, negative from the far edge) or a string in
// ParseCoord's notation ("60%", "centre", "~+30").

// typedStep is one step object as it arrives. Pointers tell an absent field
// from a zero one, which is what lets a step refuse a field its op does not
// take.
type typedStep struct {
	Op     string     `json:"op"`
	Title  *string    `json:"title"`
	Text   *string    `json:"text"`
	Keys   []string   `json:"keys"`
	X      *typedAxis `json:"x"`
	Y      *typedAxis `json:"y"`
	X2     *typedAxis `json:"x2"`
	Y2     *typedAxis `json:"y2"`
	Button *string    `json:"button"`
	N      *int       `json:"n"`
	MS     *int       `json:"ms"`
	By     *float64   `json:"by"`
	WPM    *int       `json:"wpm"`
	// Note is taken by every op (plan/05 H7), and carries on to the steps
	// after it until another note.
	Note *string `json:"note"`
}

// typedAxis is one coordinate, kept as the token ParseCoord reads.
type typedAxis string

func (a *typedAxis) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*a = typedAxis(s)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return errors.New("a coordinate is a number or a string such as \"60%\" or \"centre\"")
	}
	*a = typedAxis(strconv.FormatFloat(f, 'f', -1, 64))
	return nil
}

// stepFields is what each op takes; anything else on the object is refused,
// so a misspelt field fails before the first event rather than being ignored.
var stepFields = map[Op][]string{
	OpWindow: {"title"},
	OpScreen: {},
	OpMove:   {"x", "y"},
	OpClick:  {"x", "y", "button"},
	OpDouble: {"x", "y", "button"},
	OpDrag:   {"x", "y", "x2", "y2"},
	OpScroll: {"n"},
	OpType:   {"text"},
	OpKey:    {"keys"},
	OpWait:   {"ms"},
	OpSpeed:  {"by"},
	OpWPM:    {"wpm"},
}

// maxSteps bounds a typed script, as maxScript bounds the text form's bytes.
const maxSteps = 4096

// ParseSteps reads the typed form. Every step is checked here, as ParseScript
// checks every line, so a bad step is reported with its index before anything
// moves.
func ParseSteps(raw []byte) ([]Step, error) {
	var objs []json.RawMessage
	if err := json.Unmarshal(raw, &objs); err != nil {
		return nil, fmt.Errorf("steps is an array of step objects: %w", err)
	}
	switch {
	case len(objs) == 0:
		return nil, errors.New("the script has no steps")
	case len(objs) > maxSteps:
		return nil, fmt.Errorf("%d steps, over %d", len(objs), maxSteps)
	}
	out := make([]Step, 0, len(objs))
	note := ""
	for i, obj := range objs {
		st, n, err := parseTypedStep(obj)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", i+1, err)
		}
		if n != nil {
			note = strings.TrimSpace(*n)
		}
		st.Raw, st.Note = fmt.Sprintf("step %d, %s", i+1, st.Op), note
		out = append(out, st)
	}
	return out, nil
}

func parseTypedStep(obj json.RawMessage) (Step, *string, error) {
	var t typedStep
	dec := json.NewDecoder(bytes.NewReader(obj))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return Step{}, nil, err
	}
	if t.Note != nil {
		if err := checkNote(strings.TrimSpace(*t.Note)); err != nil {
			return Step{}, nil, err
		}
	}
	st, err := t.step()
	return st, t.Note, err
}

// step is the object as a Step, checked as its op requires.
func (t *typedStep) step() (Step, error) {
	op := Op(strings.ToLower(t.Op))
	allowed, ok := stepFields[op]
	if !ok {
		return Step{}, fmt.Errorf("unknown op %q (window, screen, move, click, double, drag, scroll, type, key, wait, speed, wpm)", t.Op)
	}
	if extra := t.present(allowed); extra != "" {
		return Step{}, fmt.Errorf("%s takes no %q", op, extra)
	}
	switch op {
	case OpWindow:
		if t.Title == nil || strings.TrimSpace(*t.Title) == "" {
			return Step{}, errors.New("window needs a title to look for")
		}
		return Step{Op: op, Text: strings.TrimSpace(*t.Title)}, nil
	case OpType:
		// Taken as given: leading spaces and newlines are what the caller
		// asked to type, which the text form cannot carry.
		if t.Text == nil || *t.Text == "" {
			return Step{}, errors.New("type needs something to type")
		}
		return Step{Op: op, Text: *t.Text}, nil
	case OpKey:
		for _, k := range t.Keys {
			if strings.TrimSpace(k) == "" || strings.ContainsAny(k, " \t\n") {
				return Step{}, fmt.Errorf("%q is not one key combination", k)
			}
		}
	}
	// Every other op's arguments are tokens with no free text in them, so
	// they go through the text form's own checks rather than a second copy.
	return ParseStep(t.line(op))
}

// present names the first field set on t that op does not take.
func (t *typedStep) present(allowed []string) string {
	set := map[string]bool{
		"title": t.Title != nil, "text": t.Text != nil, "keys": t.Keys != nil,
		"x": t.X != nil, "y": t.Y != nil, "x2": t.X2 != nil, "y2": t.Y2 != nil,
		"button": t.Button != nil, "n": t.N != nil, "ms": t.MS != nil,
		"by": t.By != nil, "wpm": t.WPM != nil,
	}
	for _, a := range allowed {
		delete(set, a)
	}
	for _, name := range []string{"title", "text", "keys", "x", "y", "x2", "y2", "button", "n", "ms", "by", "wpm"} {
		if set[name] {
			return name
		}
	}
	return ""
}

// line is the step in the text form, for the ops whose arguments are tokens.
func (t *typedStep) line(op Op) string {
	f := []string{string(op)}
	axis := func(a *typedAxis) {
		if a != nil {
			f = append(f, string(*a))
		}
	}
	axis(t.X)
	axis(t.Y)
	axis(t.X2)
	axis(t.Y2)
	if t.Button != nil {
		f = append(f, *t.Button)
	}
	f = append(f, t.Keys...)
	for _, n := range []*int{t.N, t.MS, t.WPM} {
		if n != nil {
			f = append(f, strconv.Itoa(*n))
		}
	}
	if t.By != nil {
		f = append(f, strconv.FormatFloat(*t.By, 'f', -1, 64))
	}
	return strings.Join(f, " ")
}
