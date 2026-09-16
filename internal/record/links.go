package record

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The eight link types section 39 names, and there are only eight.
const (
	LinkRulesOn        = "rules-on"
	LinkCites          = "cites"
	LinkSupersedes     = "supersedes"
	LinkImplements     = "implements"
	LinkCheckedAgainst = "checked-against"
	LinkProducedBy     = "produced-by"
	LinkBlocks         = "blocks"
	// LinkPartOf is declared in progress.go, where Step uses it.
)

// linkTypes is the closed set.
//
// ⛔ CLOSED, AND THE REASON IS THAT THE OPEN VERSION FAILS SILENTLY. Every
// derivation queries edges BY TYPE, so an edge written as `block` is not a
// wrong answer - it is an edge nothing will ever look for, sitting in the
// table satisfying its own insert. A typo becomes a fact that is true and
// invisible, which is the worst shape a defect can take in a store whose whole
// argument is that things stop hiding in prose.
var linkTypes = map[string]bool{
	LinkRulesOn: true, LinkCites: true, LinkSupersedes: true,
	LinkImplements: true, LinkCheckedAgainst: true, LinkProducedBy: true,
	LinkBlocks: true, LinkPartOf: true,
}

func knownLinkTypes() string {
	out := make([]string, 0, len(linkTypes))
	for t := range linkTypes {
		out = append(out, t)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// Link asserts a typed, directed edge between two records.
//
// IDEMPOTENT BY CONTRACT. The caller is asserting that the edge exists, and
// after a successful call it does. A second assertion of the same fact carries
// no new information and is not an error - that is set semantics, and it is
// what lets a caller re-run a declaration without first reading the graph.
func (s *Store) Link(src, typ, dst string) error {
	if err := s.checkEdge(src, typ, dst); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`INSERT INTO links (src, type, dst) VALUES (?, ?, ?)
		 ON CONFLICT(src, type, dst) DO NOTHING`, src, typ, dst); err != nil {
		return fmt.Errorf("record: linking %s -%s-> %s: %w", src, typ, dst, err)
	}
	return nil
}

// Unlink removes an edge. Idempotent for the same reason Link is: afterwards
// the edge is gone, which is what the caller asked for.
func (s *Store) Unlink(src, typ, dst string) error {
	if err := s.checkEdge(src, typ, dst); err != nil {
		return err
	}

	// ⛔ THE STREAM INVARIANT IS ENFORCED HERE TOO, OR IT IS NOT ENFORCED.
	//
	// Put refuses to write or rewrite a progress record, so the only remaining
	// door into "a step that is not in its item's stream" is detaching its
	// edge. A step with no part-of edge is a record no derivation can reach -
	// it has not been deleted, it has been hidden, which is worse.
	if typ == LinkPartOf {
		var kind string
		switch err := s.db.QueryRow(`SELECT kind FROM records WHERE id = ? AND version = 1`, src).Scan(&kind); {
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("record: checking what %s is: %w", src, err)
		case kind == KindProgress:
			return fmt.Errorf("record: %s is a %s step and cannot be detached from its item; a step outside its stream is reachable by nothing", src, KindProgress)
		}
	}

	if _, err := s.db.Exec(
		`DELETE FROM links WHERE src = ? AND type = ? AND dst = ?`, src, typ, dst); err != nil {
		return fmt.Errorf("record: unlinking %s -%s-> %s: %w", src, typ, dst, err)
	}
	return nil
}

// LinksFrom returns the destinations of one type of edge leaving a record.
func (s *Store) LinksFrom(src, typ string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT dst FROM links WHERE src = ? AND type = ? ORDER BY dst`, src, typ)
	if err != nil {
		return nil, fmt.Errorf("record: reading %s edges from %s: %w", typ, src, err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var dst string
		if err := rows.Scan(&dst); err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	return out, rows.Err()
}

// checkEdge is every refusal both verbs share.
func (s *Store) checkEdge(src, typ, dst string) error {
	if src == "" {
		return errors.New("record: a link needs a source")
	}
	if dst == "" {
		return errors.New("record: a link needs a destination")
	}
	// THE EMPTY TYPE IS COVERED HERE AND HAS NO CHECK OF ITS OWN. A separate
	// `typ == ""` guard was written first and a mutation proved it dead: the
	// closed set already refuses "", with a better message, and two guards for
	// one condition means the message a caller gets depends on which runs
	// first.
	if !linkTypes[typ] {
		return fmt.Errorf("record: %q is not a link type; section 39 names %s", typ, knownLinkTypes())
	}
	if src == dst {
		// A one-node cycle is a typo every time. Section 39 requires a cycle be
		// reported and never resolved, but that is about cycles a real
		// dependency graph grows between distinct items. Refusing this one at
		// the edge costs nothing, where letting it through costs a caller a
		// brief that reports a cycle it cannot act on.
		return fmt.Errorf("record: a record cannot be linked to itself (%s -%s-> itself)", src, typ)
	}
	for _, end := range []string{src, dst} {
		var exists int
		switch err := s.db.QueryRow(`SELECT 1 FROM heads WHERE id = ?`, end).Scan(&exists); {
		case errors.Is(err, sql.ErrNoRows):
			return &NotFoundError{ID: end}
		case err != nil:
			return fmt.Errorf("record: looking up %s: %w", end, err)
		}
	}
	return nil
}
