// Package guidelines is what a program or an agent must be to work with rig,
// each rule dated (plan/55 requirement 29, decision 0266).
//
// The rules live in guidelines.json beside this file and are embedded in
// rigd, which serves them as rig.guidelines. Not in the window: a window
// built beside another rig would show its own build's rules (B89's skew).
//
// A rule's date is when it entered rig's code where it is built, read from
// git, or the day it was ruled where it is not. A program built before a
// rule's date may need adjusting; Built says when a program was built.
package guidelines

import (
	"bytes"
	"debug/buildinfo"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Rule is one dated guideline.
type Rule struct {
	ID    string `json:"id"`
	Date  string `json:"date"`
	Who   string `json:"who"`
	Built bool   `json:"built"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Cite  string `json:"cite"`
}

//go:embed guidelines.json
var raw []byte

// Rules answers every rule, newest first, ties by id descending.
func Rules() ([]Rule, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var rs []Rule
	if err := dec.Decode(&rs); err != nil {
		return nil, fmt.Errorf("guidelines: %w", err)
	}
	if err := check(rs); err != nil {
		return nil, err
	}
	slices.SortStableFunc(rs, func(a, b Rule) int {
		if c := strings.Compare(b.Date, a.Date); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	return rs, nil
}

// Revision is the newest rule's date, the revision a program is current
// against; "" for no rules.
func Revision(rs []Rule) string {
	var r string
	for _, x := range rs {
		r = max(r, x.Date)
	}
	return r
}

func check(rs []Rule) error {
	seen := map[string]bool{}
	for i, r := range rs {
		switch {
		case r.ID == "":
			return fmt.Errorf("guidelines: rule %d has no id", i)
		case seen[r.ID]:
			return fmt.Errorf("guidelines: %s is declared twice", r.ID)
		case r.Who != "programs" && r.Who != "agents":
			return fmt.Errorf("guidelines: %s: who is %q, not programs or agents", r.ID, r.Who)
		case r.Title == "", r.Body == "", r.Cite == "":
			return fmt.Errorf("guidelines: %s needs a title, a body and a cite", r.ID)
		}
		if _, err := time.Parse(time.DateOnly, r.Date); err != nil {
			return fmt.Errorf("guidelines: %s: date %q is not YYYY-MM-DD", r.ID, r.Date)
		}
		seen[r.ID] = true
	}
	return nil
}

// Build is when a program's binary was built from source.
type Build struct {
	// CommitTime is vcs.time from the Go build info, RFC 3339; "" when unknown.
	CommitTime string
	// Modified is vcs.modified: built from a tree with uncommitted changes.
	Modified bool
	// UnknownBecause says why CommitTime is empty; "" when it is not.
	UnknownBecause string
}

// Built reads the commit time from the Go build info of the binary at path.
// It reads the file and never runs it. A binary with no build info (another
// language) or built outside git answers unknown with the reason, never a
// guessed date (decision 0266).
func Built(path string) Build {
	if path == "" {
		return Build{UnknownBecause: "rig did not start it, so it knows no binary"}
	}
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return Build{UnknownBecause: "its binary carries no Go build info"}
	}
	var b Build
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.time":
			b.CommitTime = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	if b.CommitTime == "" {
		b.UnknownBecause = "it was built outside a git checkout, or with -buildvcs=false"
	}
	return b
}
