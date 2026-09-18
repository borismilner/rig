package record

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// FriendlyTag turns a heading's TEXT into a tag a person can read in a filter.
//
// The bar and the reasoning are `plan/39`, "the bar is useful and friendly";
// the values it replaced are in `DECISIONS.md`, 2026-09-18. What matters here:
// the KEY keeps its id and its full slug, this is the READING surface, and the
// two are separate on purpose.
//
// The rule, in order:
//
//  1. drop decoration, then a leading id token   `B46 - THE MVP...` -> `THE MVP...`
//  2. cut at the first clause end                `..., AND IT EXISTED IN...`
//  3. drop a leading article
//  4. slug it, by this package's one slug rule
//  5. cap on a word boundary
//
// ⛔ STEP 2 IS WHY THIS TAKES TEXT AND NOT A SLUG. A clause ends at punctuation
// the slug has thrown away, so the only cut available on a slug is at the word
// `and` - which eats `search-and-filter` to `search`.
func FriendlyTag(heading string) string {
	// ⛔ THE DECORATION COMES OFF HERE RATHER THAN IN THE CALLER. A leading
	// `⛔` makes every step below miss, because the id pattern is anchored:
	// `⛔ B46 - X` keeps its id and cuts to `b46`. `decisionTitle` is
	// idempotent, so a caller passing already-cleaned text pays nothing.
	t := strings.TrimSpace(decisionTitle(heading))
	if t == "" {
		return ""
	}
	t = strings.TrimSpace(leadingID.ReplaceAllString(t, ""))
	// An id-only first clause is kept whole: `M7, ordered by what this team
	// uses today` would cut to `M7`, which is the value the bar rejects.
	if head := firstClause(t); head != "" && !idOnly.MatchString(head) {
		t = head
	}
	t = strings.TrimSpace(leadingArticle.ReplaceAllString(t, ""))
	return capOnWord(decisionSlug(t), friendlyTagMax)
}

// friendlyTagMax is a READING bound, measured against rig's own backlog
// headings rather than chosen: 25 is the longest a reader needs whole, 44 the
// longest this rule produces uncapped. `plan/39` carries the measurement.
const friendlyTagMax = 32

var (
	// leadingID is a backlog id at the START of a heading, with whatever
	// separator the document put after it. Anchored, because an id mentioned
	// mid-heading is part of what the heading SAYS.
	leadingID = regexp.MustCompile(`^B\d+[a-z]?\s*[-:–—]?\s*`)

	// idOnly is a clause that is nothing but a short label - `M7`, `B46`, `D1`.
	idOnly = regexp.MustCompile(`^[A-Za-z]\d+[a-z]?$`)

	// leadingArticle drops the three words that carry no meaning at the front
	// of a tag. Not a stop-word list: these three are the only ones that
	// appear first in a heading in this project's documents.
	leadingArticle = regexp.MustCompile(`(?i)^(the|a|an)\s+`)

	// clauseEnd is where a heading stops naming its subject and starts
	// explaining it.
	clauseEnd = regexp.MustCompile(`\s*(,|;|:|\s-\s|\s–\s|\s—\s)`)
)

// firstClause is the heading up to its first clause boundary, or "" where it
// has none.
func firstClause(s string) string {
	if m := clauseEnd.FindStringIndex(s); m != nil {
		return strings.TrimSpace(s[:m[0]])
	}
	return ""
}

// capOnWord truncates a slug at or before limit, never mid-word: a reader cannot
// tell `mvp-acceptance-te` from a heading that says exactly that. Counts RUNES,
// so a multi-byte heading is bounded by what is read rather than by bytes.
func capOnWord(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	cut := string([]rune(s)[:limit])
	if i := strings.LastIndex(cut, "-"); i > 0 {
		return cut[:i]
	}
	return cut
}
