// Package fts turns a caller's words into an SQLite FTS5 match expression.
// Two indexes use it: section 40's lessons and plan/48's index over the free
// files (R25). It is the only place caller text meets the FTS5 query language.
package fts

import (
	"errors"
	"strings"
	"unicode"
)

// MaxTerms is how many words a query keeps; the rest are dropped.
const MaxTerms = 16

// ErrNoWords means the query held no letter or digit to search for.
var ErrNoWords = errors.New("a search needs at least one word")

// Query makes an expression that can never be read as FTS5 syntax: every word
// becomes a quoted string with its quotes doubled, and the words are OR-ed so
// bm25 ranks by how many match.
func Query(q string) (string, error) {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return "", ErrNoWords
	}
	if len(words) > MaxTerms {
		words = words[:MaxTerms]
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " OR "), nil
}
