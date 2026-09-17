package record

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// MarkdownSection returns the prose under the first heading whose text is
// exactly `heading`, stopping at the next heading of the same or higher level.
//
// ⛔ IT EXISTS SO A DESCRIPTION IS COPIED FROM A DOCUMENT RATHER THAN COMPOSED
// BY WHOEVER IS SEEDING. Boris, 2026-09-17, on the fields a project must carry:
// "the AI agent populating it should have no problem setting proper fields for
// that ... human-friendly fields are obligatory." And the constraint that
// follows from it, recorded in plan/11: "the fields must be ones a writer can
// fill from what the source DOCUMENT already says. A field that can only be
// filled by composing new prose is a field that will arrive empty or invented,
// and both outcomes fail this."
//
// ⛔ NEITHER PARSER HERE IS NEW. It reuses `mdHeading`, which backlog.go and
// decisions.go already share, and `planFence`, which carries CommonMark's
// closing rule so a fenced block containing a `#` line cannot be read as a
// heading. plan.go records that the fence rule was owed to mdHeading's callers
// and not backported from there; this function takes both and so is correct
// without changing either, which is the same reasoning that left them alone.
//
// A MISSING SECTION IS NOT AN ERROR. It answers empty, and the caller decides
// whether an absent description is a defect or simply a project nobody has
// described. Returning an error would make "this document has no such heading"
// indistinguishable from "this document could not be read", and only the second
// is a failure.
func MarkdownSection(r io.Reader, heading string) (string, error) {
	want := strings.TrimSpace(heading)
	if want == "" {
		return "", errors.New("record: MarkdownSection needs a heading to look for")
	}

	var (
		sc      = bufio.NewScanner(r)
		out     []string
		fence   string
		level   int
		started bool
	)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for sc.Scan() {
		raw := sc.Text()

		// Fences first: a heading inside one is not a heading.
		if m := planFence.FindStringSubmatch(raw); m != nil {
			switch {
			case fence == "":
				fence = m[1]
			// CommonMark: only a fence of the same character, at least as
			// long as the opener, with nothing after it, closes the block.
			case m[1][0] == fence[0] && len(m[1]) >= len(fence) &&
				strings.TrimSpace(m[2]) == "":
				fence = ""
			}
			if started {
				out = append(out, raw)
			}
			continue
		}
		if fence != "" {
			if started {
				out = append(out, raw)
			}
			continue
		}

		if m := mdHeading.FindStringSubmatch(raw); m != nil {
			got := len(m[1])
			if !started {
				if strings.TrimSpace(m[2]) == want {
					started, level = true, got
				}
				continue
			}
			// Same level or shallower ends the section. A DEEPER heading is
			// kept, because a subsection belongs to the section above it.
			if got <= level {
				break
			}
		}
		if started {
			out = append(out, raw)
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("record: reading the document: %w", err)
	}
	if !started {
		return "", nil
	}
	return proseOnly(out), nil
}

// proseOnly drops what a description must not carry and joins what is left.
//
// ⛔ HTML LINES GO. README.md interleaves `<picture>`, `<source>` and `<img>`
// with its prose, and a description field carrying a markup tag is one no
// surface can render and no person wants to read. The rule is deliberately
// crude - a line whose first non-space character is `<` is not prose - because
// a real HTML parser here would be a second document model to keep correct.
func proseOnly(lines []string) string {
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		// ⛔ JOINED WITH A SPACE, NOT A NEWLINE, AND A TEST AGAINST THE REAL
		// README IS WHAT FORCED IT. A markdown paragraph's single newlines are
		// SOFT breaks - the source file's wrapping, not the writer's. README.md
		// wraps its one-line idea across two source lines, so joining on "\n"
		// put a newline inside `description_short`, a field section 39 defines
		// as "one line, for lists and briefs". Every surface downstream would
		// then have to strip it, and the first one that forgot would render a
		// broken row.
		paras = append(paras, unbold(strings.Join(cur, " ")))
		cur = nil
	}
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			flush()
			continue
		}
		if strings.HasPrefix(t, "<") {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return strings.Join(paras, "\n\n")
}

// unbold strips the emphasis markers around a paragraph that is entirely bold.
//
// README.md states its one-line idea as `**...**`, and the markers are a
// document's way of making a line stand out on a rendered page. Carried into a
// stored field they become four literal asterisks in a terminal heading. Only a
// WHOLLY bold paragraph is unwrapped: emphasis inside a sentence is the
// writer's and is left exactly as written.
func unbold(p string) string {
	t := strings.TrimSpace(p)
	if len(t) >= 4 && strings.HasPrefix(t, "**") && strings.HasSuffix(t, "**") &&
		!strings.Contains(t[2:len(t)-2], "**") {
		return strings.TrimSpace(t[2 : len(t)-2])
	}
	return p
}
