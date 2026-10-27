package hand

import (
	"fmt"
	"strings"
)

// A grant's scope (plan/05 section 5m, G6) limits a script to one window by
// title. CheckScope is the early half, run before the first event so a script
// that leaves the scope never starts; SetScope is the late half, which holds
// the window a `window` step actually found to the same title, because the
// title a script asked for and the window that matched are rarely the same
// word. The lock every click and keystroke is checked against (target.go) does
// the rest, so a step cannot leave the window it was scoped to.

// inScope is true when title names scope, ignoring case and a leading "=",
// which is Choose's exact-match mark.
func inScope(title, scope string) bool {
	title = strings.TrimPrefix(strings.TrimSpace(title), "=")
	return strings.Contains(strings.ToLower(title), strings.ToLower(strings.TrimSpace(scope)))
}

// CheckScope refuses a script with a step outside scope: a `window` step for
// another title, a `screen` step (which lets go of the window), or an act
// before any window step has chosen where it lands. An empty scope allows any.
func CheckScope(steps []Step, scope string) error {
	if strings.TrimSpace(scope) == "" {
		return nil
	}
	in := false
	for _, st := range steps {
		switch st.Op {
		case OpWindow:
			if !inScope(st.Text, scope) {
				return fmt.Errorf("%s is outside the grant's scope %q", st.where(), scope)
			}
			in = true
		case OpScreen:
			return fmt.Errorf("%s leaves the window the grant is scoped to (%q)", st.where(), scope)
		case OpWait, OpSpeed, OpWPM:
		default:
			if !in {
				return fmt.Errorf("%s acts before any window step names the grant's scope %q", st.where(), scope)
			}
		}
	}
	return nil
}

// SetScope holds every window a step finds to scope; empty lifts it.
func (h *Hand) SetScope(scope string) { h.scope = strings.TrimSpace(scope) }
