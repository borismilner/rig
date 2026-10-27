package hand

import (
	"strings"
	"testing"
)

// G6: a grant scoped to a window refuses, before the first event, any step
// that is outside it.
func TestCheckScope(t *testing.T) {
	for _, tc := range []struct {
		name, scope, script, want string
	}{
		{"any scope allows anything", "", "screen\nclick 5 5", ""},
		{"inside", "Editor", "window my Editor\nclick 50% 50%\ntype hello", ""},
		{"case and exact mark", "editor", "window =My EDITOR\nkey ctrl+s", ""},
		{"another window", "Editor", "window Terminal\nclick 1 1", "outside the grant's scope"},
		{"a second window outside", "Editor", "window Editor\nclick 1 1\nwindow Terminal", "outside the grant's scope"},
		{"screen lets go of the window", "Editor", "window Editor\nscreen\nclick 9 9", "leaves the window"},
		{"act before a window", "Editor", "click 9 9\nwindow Editor", "before any window step"},
		{"type before a window", "Editor", "type secret\nwindow Editor", "before any window step"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps, err := ParseScript(tc.script)
			if err != nil {
				t.Fatal(err)
			}
			err = CheckScope(steps, tc.scope)
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("got %v, want %q", err, tc.want)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatalf("the refusal quotes typed text: %v", err)
			}
		})
	}
}
