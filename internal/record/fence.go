package record

import (
	"regexp"
	"strings"
)

// mdFence matches the opening or closing line of a fenced code block: three or
// more backticks or tildes, optionally indented, with whatever info string the
// writer put after it.
var mdFence = regexp.MustCompile("^\\s{0,3}(`{3,}|~{3,})(.*)$")

// fenceScan tracks which lines of a markdown document lie inside a fenced code
// block, so that a `#` line inside one is not read as a heading.
//
// ⛔ ONE DERIVATION, FOUR CALLERS. The rule lived in plan.go and was copied
// into section.go, and `mdHeading`'s other two callers had neither. Two
// derivations of one rule is this package's named failure, so the backport is
// an extraction: plan.go's behaviour is the definition and the other three are
// brought to it. DECISIONS.md, 2026-09-18, carries the measurement.
//
// The closing rule is CommonMark's: only a fence of the same character, at
// least as long as the opener, with nothing but whitespace after it, closes the
// block. That is what lets a ```` ```go ```` block contain a ``` line of output
// without ending early.
type fenceScan struct {
	// fence is the opening marker of the block being read, and "" outside one.
	fence string
}

// read takes the next line of the document and answers what the document means
// by it. marker is true for the line that OPENED or CLOSED a block; code is
// true for any line that is part of one, the marker lines included.
//
// ⛔ THE ANSWER IS THREE-WAY BECAUSE THE CALLERS DIFFER ON THE MARKER LINE, not
// because the rule does: planHeadings drops it, MarkdownSection keeps it, and a
// caller that only wants to know whether a heading is real reads `code`. A
// shorter or different fence inside an open block is neither - it is content.
func (f *fenceScan) read(raw string) (marker, code bool) {
	m := mdFence.FindStringSubmatch(raw)
	if m == nil {
		return false, f.fence != ""
	}
	switch {
	case f.fence == "":
		f.fence = m[1]
		return true, true
	case m[1][0] == f.fence[0] && len(m[1]) >= len(f.fence) && strings.TrimSpace(m[2]) == "":
		f.fence = ""
		return true, true
	default:
		return false, true
	}
}

// unclosed answers the opening marker of a block the document never closed, and
// "" when every block was closed.
//
// ⛔ AN UNCLOSED FENCE IS AN ERROR AND NOT A SHRUG, in every parser that reads
// this project's OWN documents. Everything after it was read as code and no
// heading past it was seen, so the parse is missing an unknown number of
// entries - and `missing` would name them all without saying why. A document
// that cannot be read is not a document that states nothing.
func (f *fenceScan) unclosed() string { return f.fence }
