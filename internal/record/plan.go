// The specification's own parser: `plan/NN-*.md` into one entry per heading.
//
// ⛔ THE GRAIN IS BORIS'S AND NOT THIS FILE'S. `plan/39`, "THE GRAIN IS THE
// HEADING. RULED BY BORIS 2026-09-16 LATE": one record per `##`, `###` and
// `####`, put to him as three options with their costs. The same passage
// refuses the whole-section grain by name - "§37 is 942 lines; a link to it
// points at a DOCUMENT rather than a requirement, which is attack finding 5's
// failed row" - and refuses the bold-lead grain on size. Nothing here may widen
// or narrow it; the code that would is one character and the ruling is not.
//
// ⛔ AND IT IS NOT A SECOND SPELLING OF THE ID RULE. `decisionTitle` and
// `decisionSlug` are what the decisions import keys on, and they are called
// here rather than copied, so a heading that reads the same way in both
// documents cannot key two ways. Two derivations of one rule is this project's
// most expensive recorded failure, and an id is the worst place to have it.
package record

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// planLevels is the ruled grain, as a pair of bounds rather than as two
// literals sprinkled through the loop.
//
// ⛔ THE UPPER BOUND IS A RULING AND NOT A TUNING, and it has been widened
// once, on purpose, by the person whose ruling it is.
//
// Boris ruled the grain on 2026-09-16 as `##`, `###` and `####`, and the bound
// was 4. On 2026-09-17 he was shown the thirteen `#####` headings that fell
// outside it - THREE OF THEM HIS OWN RULINGS, including section 39's "fill
// everything in first", which is the ruling the whole plan import came from -
// and he widened it to 5. A ruling nobody could reach from rig was the argument.
//
// The cost he was shown and accepted: 329 requirement records become 342, about
// four percent more graph. What it bought: the reported-unimported set for this
// reason goes to zero, so `rigseed --check` stops carrying a question nobody had
// answered.
//
// ⛔ IT STAYS A RULING. Widening it again is his call and not a seat's, and
// anything below the bound must keep arriving as UnimportedHeadingTooDeep rather
// than being dropped - section 39's migration rule is import everything and flag
// what is irregular.
const (
	planMinLevel = 2
	planMaxLevel = 5
)

// planFileName is the shape `tools/plansplit.py` gives every section file. The
// leading number IS the section number, which is why it is read from the NAME
// rather than from the heading: plansplit enforces that a file's name matches
// its heading, so the name is the checked half of that pair.
var planFileName = regexp.MustCompile(`^(\d+)-.+\.md$`)

// PlanEntry is one heading of one section file, at the ruled grain.
type PlanEntry struct {
	// Key is the record id: the section number, then every enclosing heading's
	// short name, then this heading's own.
	//
	// ⛔ THE SECTION NUMBER LEADS BECAUSE THE NAMES ALONE ARE NOT UNIQUE ACROSS
	// 42 FILES. "Where it sits" and "What it is, in one line" are headings
	// several sections use, and a key that dropped the number would silently
	// make one section's heading supersede another's - two puts against one id,
	// with document order deciding which survives.
	Key string

	// Title is the heading text with the document's decoration and markdown
	// emphasis removed, by the same function the decisions import uses.
	Title string

	// Section is the number in the FILE NAME, not one read out of the heading.
	Section int

	// Level is the markdown heading level: 2, 3, 4 or 5.
	Level int

	// PartOf is the Key of the nearest enclosing heading that is itself an
	// entry, empty where there is none.
	//
	// ⛔ A LEVEL-2 HEADING STATES NO PARENT, AND THE FILE IS NOT ONE. A section
	// file's headings are siblings in markdown, including the first; making the
	// first one the parent of the rest would be this parser inventing a
	// hierarchy the document does not write. The section number travels as a
	// FIELD, which is the machine-checkable way to say "these belong together"
	// without asserting an edge nobody stated.
	PartOf string

	// File is the path the entry was read from, `plan/NN-slug.md`.
	File string

	// Line is the 1-based line of the heading.
	Line int

	// Body is the prose from under the heading to the line before the next
	// heading of ANY level, trimmed.
	Body string

	// SectionTitle is the text of the section's own heading, which
	// `tools/plansplit.py` writes as the first line of every section file and
	// checks the file name against.
	//
	// ⛔ IT IS THE TEXT AND NOT THE FILE NAME'S SLUG, because `FriendlyTag`
	// cuts at a clause end and a slug has thrown that punctuation away. The
	// same reason is written on `FriendlyTag` itself.
	//
	// ⛔ AND IT IS READ FROM THE FILE RATHER THAN RE-DERIVED FROM `Section`.
	// A number names a file; only the file states its title, and a second
	// table mapping one to the other is the two-derivations failure this
	// package's header names.
	SectionTitle string
}

// PlanParse is one pass over the whole of `plan/`.
type PlanParse struct {
	// Entries is in file order, then document order.
	Entries []PlanEntry

	// Unimported is every heading the document states that no entry carries.
	Unimported []Unimported
}

// ParsePlanDir reads every section file in one directory.
//
// ⛔ IT TAKES AN fs.FS SO THE TEST DOES NOT NEED A DIRECTORY ON DISK, and so a
// caller cannot hand it a path that escapes into somewhere else. The seeder
// opens the one directory it was pointed at and passes it whole; nothing here
// joins a caller's string onto a path.
//
// ⛔ A FILE WHOSE NAME IS NOT `NN-*.md` IS AN ERROR AND NOT A SKIP. plansplit
// generates every name in here, so one that does not match means either the
// caller pointed at the wrong directory or the generator's contract broke - and
// both of those are worth stopping for. A skip would import 41 of 42 sections
// and report a clean run.
func ParsePlanDir(fsys fs.FS) (PlanParse, error) {
	names, err := fs.Glob(fsys, "*")
	if err != nil {
		return PlanParse{}, fmt.Errorf("record: reading the plan directory: %w", err)
	}
	sort.Strings(names)

	var out PlanParse
	taken := map[string]PlanEntry{}
	for _, name := range names {
		m := planFileName.FindStringSubmatch(name)
		if m == nil {
			return PlanParse{}, fmt.Errorf("record: %q is in the plan directory and is not a "+
				"section file; plansplit names every one of them NN-slug.md, so this is either "+
				"the wrong directory or a generator contract that has broken", name)
		}
		section, err := strconv.Atoi(m[1])
		if err != nil {
			return PlanParse{}, fmt.Errorf("record: %q: reading its section number: %w", name, err)
		}

		f, err := fsys.Open(name)
		if err != nil {
			return PlanParse{}, fmt.Errorf("record: opening %q: %w", name, err)
		}
		one, err := parsePlanSection(f, name, section, taken)
		_ = f.Close()
		if err != nil {
			return PlanParse{}, fmt.Errorf("record: %q: %w", name, err)
		}
		out.Entries = append(out.Entries, one.Entries...)
		out.Unimported = append(out.Unimported, one.Unimported...)
	}
	return out, nil
}

// ParsePlanSection reads one section file. It is exported so one file can be
// exercised on its own, which is how every property below is tested.
func ParsePlanSection(r io.Reader, file string, section int) (PlanParse, error) {
	return parsePlanSection(r, file, section, map[string]PlanEntry{})
}

// parsePlanSection carries the `taken` set across files, because a key that
// collides between two sections is the one failure a per-file parse cannot see.
func parsePlanSection(r io.Reader, file string, section int, taken map[string]PlanEntry) (PlanParse, error) {
	heads, err := planHeadings(r)
	if err != nil {
		return PlanParse{}, err
	}

	var out PlanParse

	// chain holds the slug of the enclosing heading at each level, and keys
	// holds its Key, so a child can name its parent without re-deriving it.
	var chain, keys [planMaxLevel + 1]string

	// The section's own heading is the FIRST one in the file - plansplit writes
	// it there and refuses a file whose name does not match it. Reading it off
	// the heading list rather than the name keeps one derivation of the pair.
	var sectionTitle string
	if len(heads) > 0 {
		sectionTitle = decisionTitle(heads[0].text)
	}

	for _, h := range heads {
		title := decisionTitle(h.text)

		// ⛔ EVERY HEADING CLEARS THE LEVELS BELOW IT, INCLUDING THE ONES THIS
		// LOOP DECLINES. A `#####` is not an entry and it still ends whatever
		// `####` was open, and an H1 ends everything. Clearing only on an
		// accepted heading would let a `####` under one `###` be keyed under
		// the previous one.
		for l := h.level + 1; l <= planMaxLevel; l++ {
			chain[l], keys[l] = "", ""
		}

		if h.level < planMinLevel || h.level > planMaxLevel {
			if h.level > planMaxLevel {
				out.Unimported = append(out.Unimported, Unimported{
					Kind: UnimportedHeadingTooDeep, Label: h.text, Line: h.line,
					Under: keys[h.level-1], Section: strconv.Itoa(section),
				})
			}
			continue
		}

		if title == "" {
			out.Unimported = append(out.Unimported, Unimported{
				Kind: UnimportedHeadingNoTitle, Label: h.text, Line: h.line,
				Under: keys[h.level-1], Section: strconv.Itoa(section),
			})
			continue
		}

		slug := planSlug(title)
		e := PlanEntry{
			Title:   title,
			Section: section,
			Level:   h.level,
			File:    file,
			Line:    h.line,
			Body:    strings.TrimSpace(strings.Join(h.body, "\n")),
			Key:     planKey(section, chain[:], h.level, slug),
			PartOf:  planParent(keys[:], h.level),

			SectionTitle: sectionTitle,
		}

		// ⛔ A COLLISION IS REPORTED AND THE FIRST READ WINS, WHICH IS THE
		// DECISIONS IMPORT'S RULE AND NOT A NEW ONE. Two puts against one id in
		// one run supersede each other, so which content survived would be
		// decided by the order the files were read in - a silent wrong answer.
		if first, ok := taken[e.Key]; ok {
			out.Unimported = append(out.Unimported, Unimported{
				Kind: UnimportedDuplicateKey, ID: e.Key, Label: h.text, Line: h.line,
				Under: first.File, Section: strconv.Itoa(section),
			})
			continue
		}
		taken[e.Key] = e

		chain[h.level], keys[h.level] = slug, e.Key
		out.Entries = append(out.Entries, e)
	}
	return out, nil
}

// planSlug is one heading's component of the key.
//
// ⛔ IT IS `FriendlyTag` AND NOT `decisionSlug`, RULED BY BORIS 2026-09-18 ON
// MEASUREMENT. The full slug of every enclosing heading gave keys with a p50 of
// 93 characters and a MAXIMUM OF 287, and 272 of the 379 repeated their own
// section number because `plansplit` writes each file's first heading as
// `## NN. Title`. The friendly rule takes the same set to p50 44 and max 95
// with ZERO collisions. Shown both, he ruled: *"if it's free today and it's
// better then you should do it"*, and it is free exactly once - the live store
// holds no requirement record, so nothing has to be retracted.
//
// ⛔ AND IT IS ONE RULE RATHER THAN A SECOND SPELLING. `FriendlyTag` was
// written for the reading surface and its own comment used to say the key and
// the tag were separate on purpose; that separation was about the tag not
// carrying an ID, and it does not argue for an unreadable key. Deriving both
// from one function is what stops a heading naming itself two ways.
//
// ⛔ THE FALLBACK IS OWED BECAUSE THE FRIENDLY RULE CAN EMPTY A TITLE. A
// heading that is nothing but an id - `B46` - has its id stripped as decoration
// and comes back empty, and an empty component would key two headings the same
// way. `decisionSlug` keeps it whole, so the fallback is the OLD rule rather
// than a new one.
func planSlug(title string) string {
	if s := FriendlyTag(title); s != "" {
		return s
	}
	return decisionSlug(title)
}

// planKey is the record id one heading gets.
//
// ⛔ IT SKIPS AN EMPTY ANCESTOR RATHER THAN WRITING AN EMPTY PATH COMPONENT. A
// `###` with no `##` above it is irregular markdown and section 39 says import
// it anyway; a key of `39//something` would be imported, would look like a
// typo, and would be a different id from the same heading once the `##` above
// it was written.
func planKey(section int, chain []string, level int, slug string) string {
	parts := []string{strconv.Itoa(section)}
	for l := planMinLevel; l < level; l++ {
		if chain[l] != "" {
			parts = append(parts, chain[l])
		}
	}
	return strings.Join(append(parts, slug), "/")
}

// planParent is the Key of the nearest enclosing heading that is an entry.
func planParent(keys []string, level int) string {
	for l := level - 1; l >= planMinLevel; l-- {
		if keys[l] != "" {
			return keys[l]
		}
	}
	return ""
}

// planHead is one heading and the prose under it.
type planHead struct {
	level int
	text  string
	line  int
	body  []string
}

// planHeadings splits one section file into its headings, IGNORING ANYTHING
// INSIDE A FENCED CODE BLOCK.
//
// ⛔ THE FENCE RULE IS A CORRECTNESS FIX AND IT SHIPPED AT ZERO HITS, WHICH IS
// THE ONLY TIME IT IS FREE. It is `fenceScan`'s now and all four of
// `mdHeading`'s callers have it; this parser's behaviour is what defines it.
func planHeadings(r io.Reader) ([]planHead, error) {
	var (
		heads  []planHead
		fences fenceScan
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Text()

		// The fence that opened or closed a block is dropped; everything
		// else inside one is this heading's body, the shorter or different
		// fences included.
		if marker, code := fences.read(raw); marker {
			continue
		} else if code {
			heads = appendBody(heads, raw)
			continue
		}

		if m := mdHeading.FindStringSubmatch(raw); m != nil {
			heads = append(heads, planHead{level: len(m[1]), text: strings.TrimSpace(m[2]), line: line})
			continue
		}
		heads = appendBody(heads, raw)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// An unclosed fence is an error and not a shrug: fenceScan.unclosed says why.
	if fence := fences.unclosed(); fence != "" {
		return nil, fmt.Errorf("a fenced block opened with %q is never closed, so every "+
			"heading after it was read as code; this parse declines to answer rather "+
			"than report the rest of the file as absent", fence)
	}
	return heads, nil
}

// appendBody puts one line under the heading it follows, and drops it where
// there is no heading yet - the lines above a file's first heading are the
// document's own preamble and belong to no entry.
func appendBody(heads []planHead, raw string) []planHead {
	if n := len(heads); n > 0 {
		heads[n-1].body = append(heads[n-1].body, raw)
	}
	return heads
}
