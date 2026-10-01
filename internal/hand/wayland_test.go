package hand

import "testing"

func TestSourceFor(t *testing.T) {
	ids := []string{"il", "us"}
	cases := []struct {
		r    rune
		want string
	}{
		{'a', "us"},
		{'Z', "us"},
		{'é', "us"},
		{'ש', "il"},
		{'ם', "il"},
		{'א', "il"},
		{'ת', "il"},
		{'1', ""},
		{' ', ""},
		{'.', ""},
		{'\n', ""},
	}
	for _, c := range cases {
		if got, ok := sourceFor(c.r, ids); got != c.want || !ok {
			t.Errorf("sourceFor(%q) = %q %v, want %q", c.r, got, ok, c.want)
		}
	}
	// A letter no configured layout carries is refused, never sent to be dropped.
	for _, r := range []rune{'ש', 'Ж'} {
		if _, ok := sourceFor(r, []string{"us"}); ok {
			t.Errorf("sourceFor(%q) with only us must be refused", r)
		}
	}
}

func TestKeysymWL(t *testing.T) {
	// The il layout carries the legacy keysyms, not the Unicode ones.
	if got := keysymWL('א'); got != 0x0ce0 {
		t.Errorf("aleph = %#x, want 0xce0", got)
	}
	if got := keysymWL('ת'); got != 0x0cfa {
		t.Errorf("taw = %#x, want 0xcfa", got)
	}
	if got := keysymWL('a'); got != 'a' {
		t.Errorf("a = %#x", got)
	}
	if got := keysymWL('\n'); got != KeyReturn {
		t.Errorf("newline = %#x, want Return", got)
	}
}

// Text the active layout cannot type is pasted, never switched to (Boris,
// 2026-10-01), and a neutral stretch joins a paste only between two.
func TestSplitRuns(t *testing.T) {
	cases := []struct {
		ids  []string
		text string
		want []run
	}{
		{
			[]string{"us", "il"},
			"tag 2026.7.3 שלום עולם ok",
			[]run{{"tag 2026.7.3 ", false}, {"שלום עולם", true}, {" ok", false}},
		},
		{
			[]string{"il", "us"},
			"שלום ok 1",
			[]run{{"שלום ", false}, {"ok", true}, {" 1", false}},
		},
		{[]string{"us"}, "a Ж b", []run{{"a ", false}, {"Ж", true}, {" b", false}}},
		{nil, "anything שלום", []run{{"anything שלום", false}}},
	}
	for _, c := range cases {
		got := splitRuns(c.text, c.ids)
		if len(got) != len(c.want) {
			t.Errorf("splitRuns(%q, %v) = %+v", c.text, c.ids, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitRuns(%q, %v) = %+v", c.text, c.ids, got)
				break
			}
		}
	}
}

func TestPlanTextNilLayoutPacesEverything(t *testing.T) {
	strokes, skipped := PlanText("aש1 .", nil, Typing{WPM: 300})
	if len(strokes) != 5 || len(skipped) != 0 {
		t.Fatalf("got %d strokes, %d skipped", len(strokes), len(skipped))
	}
}
