package kernel

import (
	"fmt"
	"strings"
)

// Section 52's event kinds: the grammar, and who may publish which.
//
// A kind is dotted lower case (E2). rig's own kinds are bare and their first
// word is one of RigEventRoots; a program's start with its own id (E3), so
// whose a kind is can be read off the kind alone.

// RigEventRoots are the first words of rig's own kinds. A program whose id is
// one of them cannot declare events: its kinds would read as rig's.
var RigEventRoots = map[string]bool{
	"hand": true, "toast": true, "system": true, "timer": true, "config": true,
	// plan/53: rig posts lease and roster changes; a seat posts signals.
	"lease": true, "roster": true, "signal": true,
}

// MaxEventKind bounds a kind or a pattern, in bytes.
const MaxEventKind = 128

// BadEventWords says why the dotted words after a kind's owner are not
// lower case, or returns empty. Each word is a-z first, then a-z, 0-9, _ or -.
func BadEventWords(words string) string {
	if words == "" {
		return "has nothing after its owner: a kind names what happened"
	}
	for w := range strings.SplitSeq(words, ".") {
		if w == "" {
			return "has an empty word between dots"
		}
		for i, r := range w {
			lower := r >= 'a' && r <= 'z'
			if !lower && (i == 0 || (r < '0' || r > '9') && r != '_' && r != '-') {
				return fmt.Sprintf("has %q: a word is lower case, a-z first, then a-z, 0-9, _ or -", w)
			}
		}
	}
	return ""
}

// badEvent says why a program may not declare kind, or returns empty.
func badEvent(programID, kind string) string {
	if len(kind) > MaxEventKind {
		return fmt.Sprintf("is %d bytes, over %d", len(kind), MaxEventKind)
	}
	if RigEventRoots[programID] {
		return fmt.Sprintf("cannot be declared by a program named %q: that word starts rig's own kinds", programID)
	}
	words, ok := strings.CutPrefix(kind, programID+".")
	if !ok {
		return fmt.Sprintf("does not start with this program's id and a dot (%s.): a program publishes under its own name (section 52 E3)", programID)
	}
	return BadEventWords(words)
}

// DeclaresEvent reports whether p declared kind under events.
func (p Program) DeclaresEvent(kind string) bool {
	for _, k := range p.Events {
		if k == kind {
			return true
		}
	}
	return false
}
