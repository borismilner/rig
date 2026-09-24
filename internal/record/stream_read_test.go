package record

import (
	"context"
	"fmt"
)

// stream returns every step on one item, OLDEST FIRST.
//
// A TEST INSTRUMENT. It was the exported Store.Stream and nothing outside the
// tests ever called it: section 39 serves progress.step and no stream verb,
// and a caller reads a stream back through record.refs on the item, whose
// part-of answer is ordered by id. The ordering argument below is why that
// works, and it is kept with the read the tests use to prove it.
//
// Ordered by the daemon's clock and then by id. The id is the tie-breaker and
// it is a real one rather than a formality: tests freeze the clock, so every
// step in a test shares a created_at, and UUIDv7 is monotonic within a process.
// That ordering property is the whole reason section 39 chose v7 over v4, and
// this is the first place it is load-bearing rather than decorative.
//
// ⛔ IT ORDERS BY id ALONE, AND created_at IS NOT A TIE-BREAK BESIDE IT.
// This read `ORDER BY r.created_at, r.id`, which was correct - id caught every
// tie - and contradicted the store's latest-step read in print, where the same
// question was argued the other way: the daemon clock has millisecond
// resolution and tests freeze it outright, so created_at is not usable as an
// ordering and the id is both the order and the tie-break. That read left rig
// with the brief it served, and the argument stays here.
func (s *Store) stream(ctx context.Context, item string) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
			r.session, r.seat, r.epoch, r.created_at
		 FROM links l JOIN records r ON r.id = l.src
		 WHERE l.dst = ? AND l.type = ? AND r.kind = ?
		 ORDER BY r.id`, item, LinkPartOf, KindProgress)
	if err != nil {
		return nil, fmt.Errorf("record: reading the stream of %s: %w", item, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
