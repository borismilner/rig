package main

import (
	"strings"
	"testing"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

func TestSuperviseRefusesBeforeDialling(t *testing.T) {
	// Both refusals must come back without a daemon: the page must not be
	// able to name a verb Supervise was not written for.
	for _, tc := range []struct{ action, program, want string }{
		{"start", "", "no program named"},
		{"delete", "shelf", `unknown action "delete"`},
		{"", "shelf", "unknown action"},
	} {
		_, err := RigService{}.Supervise(tc.action, tc.program)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Supervise(%q, %q) = %v, want %q", tc.action, tc.program, err, tc.want)
		}
	}
}

func TestCardWords(t *testing.T) {
	if got := effectsName(rigv1.Effects_EFFECTS_WRITES_FILES); got != "writes-files" {
		t.Errorf("effectsName = %q", got)
	}
	if got := effectsName(rigv1.Effects_EFFECTS_UNSPECIFIED); got != "" {
		t.Errorf("an unsaid effect must stay unsaid, got %q", got)
	}
	if got := loadName(rigv1.Load_LOAD_ON_CALL); got != "on call" {
		t.Errorf("loadName = %q", got)
	}
	if got := loadName(rigv1.Load_LOAD_UNSPECIFIED); got != "" {
		t.Errorf("an unsaid load must stay unsaid, got %q", got)
	}
	cs := commands([]*rigv1.Command{{Id: "search", Summary: "find", Effects: rigv1.Effects_EFFECTS_READ_ONLY}})
	if len(cs) != 1 || cs[0].ID != "search" || cs[0].Summary != "find" || cs[0].Effects != "read-only" {
		t.Errorf("commands = %+v", cs)
	}
}
