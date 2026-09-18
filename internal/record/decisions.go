package record

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

// EntryKind separates the two things DECISIONS.md holds under the same
// markdown shape.
//
// ⛔ IT IS REPORTED RATHER THAN USED TO DROP ANYTHING. Section 39's migration
// ruling is import everything and flag what is irregular, and the alternative
// here was to decide that "What rig is" is not a decision and then silently
// not carry it - which is the B46 failure exactly: a thing the document
// addresses that no record holds and no instrument can name.
type EntryKind string

const (
	// EntryDecision is a ruling: a dated heading, a `Decided:`/`Measured:`
	// heading, or any sub-heading beneath either.
	EntryDecision EntryKind = "decision"

	// EntrySection is one of the document's standing sections - the preamble
	// that says what rig is, how it is named, and what is still open. It is
	// carried so that a sub-heading's parent always exists.
	EntrySection EntryKind = "section"
)

const (
	// UnimportedDuplicateKey is a heading whose derived key is already taken.
	//
	// ⛔ IT EXISTS BECAUSE THE ALTERNATIVE IS SILENT. Three sub-headings in
	// rig's own DECISIONS.md read `### The ruling`, so a key that is the
	// child's own text collides and one document fact quietly rides on
	// another's record forever. Qualifying by the parent fixes those three;
	// this reports the case the qualification does not reach, instead of
	// minting `-2` and calling it an id.
	UnimportedDuplicateKey UnimportedKind = "duplicate-key"

	// UnimportedHeadingTooDeep is a heading below the two levels this document
	// writes. Carrying it would invent a grain nobody uses; dropping it
	// without saying so is how a grain change goes unnoticed.
	UnimportedHeadingTooDeep UnimportedKind = "heading-too-deep"

	// UnimportedHeadingNoTitle is a heading whose text is nothing once the
	// document's decoration is removed. There is no title to key on.
	UnimportedHeadingNoTitle UnimportedKind = "heading-no-title"
)

// DecisionEntry is one heading of a decisions document with the prose beneath
// it.
type DecisionEntry struct {
	// Key is the stable document key, and set equality against the store is
	// computed on it. A top-level heading's key is its own slug; a
	// sub-heading's is `parent/child`, because the child's text alone is not
	// unique in rig's own document.
	Key string

	// Title is the heading text with the document's decoration removed.
	Title string

	// Date is the ISO date the heading leads with, where it leads with one.
	//
	// ⛔ NEVER INHERITED FROM THE HEADING ABOVE. 46 of 163 top-level headings
	// state no date, and giving them the previous one's would put a
	// fabricated date on a ruling - a fact that reads exactly like a measured
	// one once it is in a record.
	Date string

	// Kind is decision or section. See EntryKind.
	Kind EntryKind

	// Level is the markdown heading level, 2 or 3.
	Level int

	// PartOf is the Key of the enclosing top-level heading, empty at the top
	// level. The parent is always itself an entry, which is the thing B66
	// found missing on the backlog side: six children there name a parent
	// that is in no record.
	PartOf string

	// Line is the 1-based line of the heading.
	Line int

	// Body is the prose from under the heading to the line before the next
	// heading of any level, trimmed.
	Body string
}

// DecisionParse is one pass over a decisions document.
type DecisionParse struct {
	// Decisions is in document order, which is the order the document's own
	// title says they were made in.
	Decisions []DecisionEntry

	// Unimported is everything the document addresses that no entry carries.
	Unimported []Unimported
}

var (
	decisionDate    = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\b`)
	decisionSlugBad = regexp.MustCompile(`[^a-z0-9]+`)
	decisionMarkup  = regexp.MustCompile("[`*~_]+")
)

// decisionTitle strips the document's decoration and markdown emphasis from a
// heading, leaving what a reader would call its title.
func decisionTitle(text string) string {
	s := decisionMarkup.ReplaceAllString(text, "")
	s = strings.Trim(s, decoration)
	return strings.Join(strings.Fields(s), " ")
}

// decisionSlug derives the key component for one heading.
//
// ⛔ IT IS DERIVED FROM THE TITLE AND IS THEREFORE NOT STABLE UNDER AN EDIT OF
// THE TITLE. That is stated rather than solved: the document carries no ids,
// so every available key is derived from something a person can change, and a
// key that silently follows a rename would hide the rename. A retitled heading
// reads as one key gone and one key arrived, which is what the detector should
// say.
func decisionSlug(title string) string {
	s := decisionSlugBad.ReplaceAllString(strings.ToLower(title), "-")
	s = strings.Trim(s, "-")
	const maxSlug = 80
	if len(s) > maxSlug {
		s = strings.Trim(s[:maxSlug], "-")
	}
	return s
}

// ParseDecisions reads a decisions document and returns one entry per heading.
func ParseDecisions(r io.Reader) ([]DecisionEntry, error) {
	p, err := ParseDecisionsDocument(r)
	return p.Decisions, err
}

// ParseDecisionsDocument reads a decisions document and returns its entries
// and everything it could not carry.
//
// ⛔ THE PASS IS IN TWO PARTS BECAUSE THE SECTION RULE IS POSITIONAL AND
// CANNOT BE DECIDED ON THE LINE. A top-level heading is a section iff it sits
// before the FIRST dated top-level heading. The rejected alternative was a
// vocabulary - `Decided:`, `Measured:`, `Recorded honestly:` - which fails
// silently the first time somebody writes a fourth verb, and the failure is a
// ruling that vanishes.
func ParseDecisionsDocument(r io.Reader) (DecisionParse, error) {
	type head struct {
		level int
		text  string
		line  int
		body  []string
	}

	var heads []head
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Text()
		if m := mdHeading.FindStringSubmatch(raw); m != nil {
			heads = append(heads, head{level: len(m[1]), text: strings.TrimSpace(m[2]), line: line})
			continue
		}
		if n := len(heads); n > 0 {
			heads[n-1].body = append(heads[n-1].body, raw)
		}
	}
	if err := sc.Err(); err != nil {
		return DecisionParse{}, err
	}

	firstDated := -1
	for i, h := range heads {
		if h.level == 2 && decisionDate.MatchString(decisionTitle(h.text)) {
			firstDated = i
			break
		}
	}

	out := DecisionParse{}
	taken := map[string]bool{}
	parent := ""
	// ⛔ Unimported.Section WAS EMPTY ON ALL THREE OF THIS FILE'S KINDS, AND A
	// FIELD POPULATED ON FOUR KINDS AND EMPTY ON THREE IS INDISTINGUISHABLE
	// FROM ONE THAT WAS LOST. The backlog grain added it and could not reach
	// here; the stack is the backlog grain's own type rather than a second pop
	// loop, so the two files cannot drift on what "enclosing" means.
	//
	// ⛔ EVERY HEADING PUSHES A FRAME, INCLUDING THE ONES THIS LOOP DECLINES.
	// A level-4 heading is not an entry and it still encloses whatever follows
	// it, and the H1 the loop skips is what a top-level heading sits under.
	var sections sectionStack
	for i, h := range heads {
		sections.popTo(h.level)
		section := sections.section()
		sectionTitle := sections.sectionTitle()
		sections = append(sections, headingFrame{
			level: h.level, section: sectionSlug(h.text),
			sectionTitle: decisionTitle(h.text),
		})

		// ⛔ THE H1 IS THE DOCUMENT'S OWN TITLE AND IS NOT AN ENTRY. Levels
		// below 3 are not used by this document; carrying them would invent a
		// grain nobody writes.
		if h.level < 2 || h.level > 3 {
			if h.level > 3 {
				out.Unimported = append(out.Unimported, Unimported{
					Kind: UnimportedHeadingTooDeep, Label: h.text, Line: h.line,
					Under: parent, Section: section, SectionTitle: sectionTitle,
				})
			}
			continue
		}

		title := decisionTitle(h.text)
		if title == "" {
			out.Unimported = append(out.Unimported, Unimported{
				Kind: UnimportedHeadingNoTitle, Label: h.text, Line: h.line,
				Under: parent, Section: section, SectionTitle: sectionTitle,
			})
			continue
		}

		e := DecisionEntry{
			Title: title,
			Kind:  EntryDecision,
			Level: h.level,
			Line:  h.line,
			Body:  strings.TrimSpace(strings.Join(h.body, "\n")),
		}
		if m := decisionDate.FindStringSubmatch(title); m != nil {
			e.Date = m[1]
		}

		slug := decisionSlug(title)
		switch h.level {
		case 2:
			e.Key = slug
			if firstDated >= 0 && i < firstDated {
				e.Kind = EntrySection
			}
			parent = e.Key
		case 3:
			e.PartOf = parent
			e.Key = slug
			if parent != "" {
				e.Key = parent + "/" + slug
			}
		}

		if taken[e.Key] {
			out.Unimported = append(out.Unimported, Unimported{
				Kind: UnimportedDuplicateKey, ID: e.Key, Label: title, Line: h.line,
				Under: parent, Section: section, SectionTitle: sectionTitle,
			})
			continue
		}
		taken[e.Key] = true
		out.Decisions = append(out.Decisions, e)
	}
	return out, nil
}
