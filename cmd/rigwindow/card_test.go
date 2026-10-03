package main

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
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

func TestTryRefusesBeforeDialling(t *testing.T) {
	for _, tc := range []struct{ owner, id, args, want string }{
		{"", "health", "{}", "no capability named"},
		{"rig", "", "{}", "no capability named"},
		{"rig", "health", "not json", "not a JSON object"},
		{"rig", "health", "[1]", "not a JSON object"},
	} {
		_, err := RigService{}.Try(tc.owner, tc.id, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Try(%q, %q, %q) = %v, want %q", tc.owner, tc.id, tc.args, err, tc.want)
		}
	}
}

func TestVerbCapability(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		ann  *mcp.ToolAnnotations
		want string
	}{
		{&mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &no}, "read-only"},
		{&mcp.ToolAnnotations{DestructiveHint: &yes}, "destructive"},
		{&mcp.ToolAnnotations{DestructiveHint: &no}, "changes state"},
		// Nothing declared stays unsaid, and the panel confirms it.
		{nil, ""},
	} {
		c := verbCapability(&mcp.Tool{Name: "x", Description: "Does a. Then b.", Annotations: tc.ann})
		if c.Effects != tc.want {
			t.Errorf("effects = %q, want %q", c.Effects, tc.want)
		}
		if c.Owner != "rig" || c.Summary != "Does a." {
			t.Errorf("owner %q summary %q", c.Owner, c.Summary)
		}
	}
}

func TestGuidelinesFrom(t *testing.T) {
	g := guidelinesFrom(&verbsv1.GuidelinesResponse{
		Revision: "2026-10-03",
		Rules:    []*verbsv1.Guideline{{Id: "G4", Date: "2026-10-03", Who: "programs", Built: true}},
		Programs: []*verbsv1.ProgramBuild{
			{Program: "shelf", CommitTime: "2026-10-01T22:30:00Z"},
			{Program: "bad", CommitTime: "yesterday"},
			{Program: "script", UnknownBecause: "its binary carries no Go build info"},
		},
	})
	if len(g.Rules) != 1 || g.Rules[0].ID != "G4" {
		t.Fatalf("rules: %+v", g.Rules)
	}
	want := time.Date(2026, 10, 1, 22, 30, 0, 0, time.UTC).Local().Format(time.DateOnly)
	if g.Programs[0].Day != want {
		t.Errorf("shelf day %q, want %q", g.Programs[0].Day, want)
	}
	if g.Programs[1].Day != "" || g.Programs[1].CommitTime != "" || g.Programs[1].UnknownBecause == "" {
		t.Errorf("a malformed time must read unknown: %+v", g.Programs[1])
	}
	if g.Programs[2].Day != "" || g.Programs[2].UnknownBecause == "" {
		t.Errorf("script: %+v", g.Programs[2])
	}
}
