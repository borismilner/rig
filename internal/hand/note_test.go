package hand

import (
	"strings"
	"testing"
)

// H7: a note says what the steps after it are for, in both forms, until the
// next note.
func TestANoteCarriesOnUntilTheNextInBothForms(t *testing.T) {
	text, err := ParseScript("click 1 1\nnote opening the settings\nclick 2 2\nkey Return\nnote saving\nkey ctrl+s")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := ParseSteps([]byte(`[{"op":"click","x":1,"y":1},
		{"op":"click","x":2,"y":2,"note":"opening the settings"},{"op":"key","keys":["Return"]},
		{"op":"key","keys":["ctrl+s"],"note":"saving"}]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", "opening the settings", "opening the settings", "saving"}
	for name, steps := range map[string][]Step{"text": text, "typed": typed} {
		if len(steps) != len(want) {
			t.Fatalf("%s: %d steps", name, len(steps))
		}
		for i, st := range steps {
			if st.Note != want[i] {
				t.Errorf("%s step %d: note %q, want %q", name, i+1, st.Note, want[i])
			}
		}
	}
	if _, err := ParseScript("note " + strings.Repeat("x", maxNote+1) + "\nclick 1 1"); err == nil {
		t.Error("an over-long note was taken")
	}
	if _, err := ParseSteps([]byte(`[{"op":"wait","ms":1,"note":"a\nb"}]`)); err == nil {
		t.Error("a two-line note was taken")
	}
}

// The strip may show a step, but never what a type step types.
func TestShownNeverCarriesTypedText(t *testing.T) {
	steps, err := ParseSteps([]byte(`[{"op":"type","text":"hunter2 secret"},{"op":"key","keys":["ctrl+l"]},{"op":"window","title":"Terminal"}]`))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{steps[0].Shown(), steps[1].Shown(), steps[2].Shown()}
	want := []string{"type 14 characters", "key ctrl+l", `window "Terminal"`}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d shown as %q, want %q", i+1, got[i], want[i])
		}
	}
	if strings.Contains(strings.Join(got, " "), "hunter2") {
		t.Fatal("typed text reached the strip")
	}
}
