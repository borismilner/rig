package hand

import (
	"reflect"
	"strings"
	"testing"
)

// The typed form is the contract and the text form is sugar over it, so the
// same script in both must produce the same steps.
func TestTheTypedFormIsTheTextFormsSteps(t *testing.T) {
	text := "window Terminal\nscreen\nmove 25% -46\nclick\n" +
		"click right\nclick 10 20 middle\n" +
		"double centre ~+30\ndrag 1 2 3 4\nscroll -3\ntype hello there\nkey ctrl+l Return\n" +
		"wait 250\nspeed 1.5\nwpm 240"
	typed := `[{"op":"window","title":"Terminal"},{"op":"screen"},{"op":"move","x":"25%","y":-46},` +
		`{"op":"click"},{"op":"click","button":"right"},{"op":"click","x":10,"y":20,"button":"middle"},` +
		`{"op":"double","x":"centre","y":"~+30"},{"op":"drag","x":1,"y":2,"x2":3,"y2":4},` +
		`{"op":"scroll","n":-3},{"op":"type","text":"hello there"},{"op":"key","keys":["ctrl+l","Return"]},` +
		`{"op":"wait","ms":250},{"op":"speed","by":1.5},{"op":"wpm","wpm":240}]`
	want, err := ParseScript(text)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSteps([]byte(typed))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%d steps, want %d", len(got), len(want))
	}
	for i := range want {
		want[i].Line, want[i].Raw = 0, ""
		got[i].Raw = ""
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("step %d: typed %+v, text %+v", i+1, got[i], want[i])
		}
	}
}

// What the text form cannot carry, the typed form types as given.
func TestTypedTextIsTypedAsGiven(t *testing.T) {
	got, err := ParseSteps([]byte(`[{"op":"type","text":"  two lines\nשלום"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Text != "  two lines\nשלום" {
		t.Errorf("text %q", got[0].Text)
	}
}

func TestABadTypedStepIsRefusedWithItsIndex(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"not an array":   {`{"op":"screen"}`, "array of step objects"},
		"empty":          {`[]`, "no steps"},
		"unknown op":     {`[{"op":"screen"},{"op":"jump"}]`, `step 2: unknown op "jump"`},
		"misspelt field": {`[{"op":"type","txt":"x"}]`, `unknown field "txt"`},
		"wrong field":    {`[{"op":"move","x":1,"y":2,"text":"x"}]`, `move takes no "text"`},
		"missing y":      {`[{"op":"move","x":1}]`, "move needs an x and a y"},
		"bad coordinate": {`[{"op":"move","x":"far","y":2}]`, `"far" is not a coordinate`},
		"object coord":   {`[{"op":"move","x":{},"y":2}]`, "a coordinate is a number or a string"},
		"empty text":     {`[{"op":"type","text":""}]`, "type needs something to type"},
		"blank title":    {`[{"op":"window","title":"  "}]`, "window needs a title"},
		"spaced key":     {`[{"op":"key","keys":["ctrl+l Return"]}]`, "not one key combination"},
		"no keys":        {`[{"op":"key"}]`, "key needs a combination"},
		"negative wait":  {`[{"op":"wait","ms":-1}]`, "not a number of milliseconds"},
		"zero scroll":    {`[{"op":"scroll","n":0}]`, "not a number of notches"},
		"bad button":     {`[{"op":"click","button":"thumb"}]`, "not a mouse button"},
	} {
		_, err := ParseSteps([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want it to say %q", name, err, tc.want)
		}
	}
}

func TestTooManyTypedStepsAreRefused(t *testing.T) {
	in := "[" + strings.Repeat(`{"op":"screen"},`, maxSteps) + `{"op":"screen"}]`
	if _, err := ParseSteps([]byte(in)); err == nil || !strings.Contains(err.Error(), "over") {
		t.Errorf("%d steps: %v", maxSteps+1, err)
	}
}

// An error names the step, never the text it would have typed.
func TestAStepErrorLeavesTheTypedTextOut(t *testing.T) {
	text, _ := ParseScript("type my secret")
	typed, _ := ParseSteps([]byte(`[{"op":"type","text":"my secret"}]`))
	for _, st := range append(text, typed...) {
		if strings.Contains(st.where(), "secret") {
			t.Errorf("where() = %q", st.where())
		}
	}
}
