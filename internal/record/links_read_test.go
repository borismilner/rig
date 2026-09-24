package record

import (
	"context"
	"fmt"
)

// linksFrom reads the destinations of one type of edge leaving a record.
//
// A TEST INSTRUMENT, AND ONLY THAT. It was the exported Store.LinksFrom, whose
// one production caller was the brief's blocking walk; the brief left at
// plan/50 move 8 and nothing outside the tests read it after. The tests still
// need to see what Link, Unlink, Delete and Refs left behind, so the read
// lives here rather than in the store's surface.
func (s *Store) linksFrom(ctx context.Context, src, typ string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
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
