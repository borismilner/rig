package guidelines

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesLoadNewestFirst(t *testing.T) {
	rs, err := Rules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) == 0 {
		t.Fatal("no rules")
	}
	for i := 1; i < len(rs); i++ {
		if rs[i].Date > rs[i-1].Date {
			t.Errorf("%s (%s) is after %s (%s)", rs[i].ID, rs[i].Date, rs[i-1].ID, rs[i-1].Date)
		}
	}
	if got := Revision(rs); got != rs[0].Date {
		t.Errorf("revision %q, newest rule %q", got, rs[0].Date)
	}
}

func TestCheckRefuses(t *testing.T) {
	ok := Rule{ID: "G1", Date: "2026-09-10", Who: "programs", Title: "t", Body: "b", Cite: "c"}
	cases := map[string]func(r *Rule){
		"no id":     func(r *Rule) { r.ID = "" },
		"who":       func(r *Rule) { r.Who = "humans" },
		"date":      func(r *Rule) { r.Date = "10/09/2026" },
		"no cite":   func(r *Rule) { r.Cite = "" },
		"no title":  func(r *Rule) { r.Title = "" },
		"duplicate": nil,
	}
	for name, mod := range cases {
		rs := []Rule{ok}
		if mod == nil {
			rs = append(rs, ok)
		} else {
			r := ok
			mod(&r)
			rs = []Rule{r}
		}
		if check(rs) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := check([]Rule{ok}); err != nil {
		t.Errorf("a good rule was refused: %v", err)
	}
}

func TestBuiltSaysWhyItDoesNotKnow(t *testing.T) {
	if b := Built(""); b.CommitTime != "" || b.UnknownBecause == "" {
		t.Errorf("no path: %+v", b)
	}
	script := filepath.Join(t.TempDir(), "prog.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b := Built(script); b.CommitTime != "" || !strings.Contains(b.UnknownBecause, "no Go build info") {
		t.Errorf("a script: %+v", b)
	}
}
