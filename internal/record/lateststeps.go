package record

import (
	"database/sql"
	"fmt"
)

// THE LATEST STEP PER ITEM IS A PER-GROUP MAXIMUM, AND WHICH SQL YOU WRITE FOR
// IT IS A CORRECTNESS SURFACE RATHER THAN A TUNING EXERCISE.
//
// Ruled after a measurement that nearly shipped backwards: the spread between
// reasonable formulations of this one question is 117x at 44,000 rows and
// 8,200x at 4.4 million. A seat that writes whichever form it thought of first
// has a 50 ms target and may be orders of magnitude outside it, with a query
// that returns the right rows. So all four are written here, all four are
// measured, and the one in use says why.
//
// ⛔ THEY ORDER BY id, NOT BY created_at, AND THAT IS NOT A SHORTCUT.
// Ordering by created_at alone is WRONG: the daemon clock has millisecond
// resolution and tests freeze it outright, so a MAX(created_at) returns EVERY
// step in the stream on a tie and the derivation silently reports several
// "latest" steps for one item. A UUIDv7 id encodes the timestamp and is
// monotonic within the process that minted it, so MAX(id) is both the
// tie-break and the ordering. This is the second place section 39's choice of
// v7 over v4 is load-bearing, and the first where getting it wrong returns
// wrong rows rather than ugly ones.

// latestStepsSQL are the four formulations, keyed by name.
//
// Each takes the project twice where it needs it and returns the full record
// columns plus the item the step belongs to.
var latestStepsSQL = map[string]string{
	// 1. GROUP BY with a join back to the winning row.
	"group-by-max": `
		SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
		       r.session, r.seat, r.epoch, r.created_at, l.dst
		FROM records r
		JOIN links l ON l.src = r.id AND l.type = 'part-of'
		JOIN (SELECT l2.dst AS item, MAX(r2.id) AS newest
		      FROM records r2
		      JOIN links l2 ON l2.src = r2.id AND l2.type = 'part-of'
		      WHERE r2.kind = 'progress' AND r2.project = ?
		      GROUP BY l2.dst) m
		  ON m.item = l.dst AND m.newest = r.id
		WHERE r.kind = 'progress'`,

	// 2. Correlated subquery, one MAX per group.
	"correlated": `
		SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
		       r.session, r.seat, r.epoch, r.created_at, l.dst
		FROM records r
		JOIN links l ON l.src = r.id AND l.type = 'part-of'
		WHERE r.kind = 'progress' AND r.project = ?
		  AND r.id = (SELECT MAX(r2.id)
		              FROM records r2
		              JOIN links l2 ON l2.src = r2.id AND l2.type = 'part-of'
		              WHERE l2.dst = l.dst AND r2.kind = 'progress')`,

	// 3. Window function over the whole scan.
	"window": `
		SELECT id, version, kind, project, body, fields,
		       session, seat, epoch, created_at, item
		FROM (SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
		             r.session, r.seat, r.epoch, r.created_at, l.dst AS item,
		             ROW_NUMBER() OVER (PARTITION BY l.dst ORDER BY r.id DESC) AS rn
		      FROM records r
		      JOIN links l ON l.src = r.id AND l.type = 'part-of'
		      WHERE r.kind = 'progress' AND r.project = ?)
		WHERE rn = 1`,

	// 4. Anti-join: keep the step no later step exists for.
	"not-exists": `
		SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
		       r.session, r.seat, r.epoch, r.created_at, l.dst
		FROM records r
		JOIN links l ON l.src = r.id AND l.type = 'part-of'
		WHERE r.kind = 'progress' AND r.project = ?
		  AND NOT EXISTS (SELECT 1
		                  FROM records r2
		                  JOIN links l2 ON l2.src = r2.id AND l2.type = 'part-of'
		                  WHERE l2.dst = l.dst AND r2.kind = 'progress'
		                    AND r2.id > r.id)`,
}

// latestStepsForm is the formulation in use, CHOSEN ON MEASUREMENT.
//
// ⛔ THE FIRST CHOICE HERE WAS "correlated", ON AN INHERITED NUMBER, AND IT WAS
// THE WORST OF THE FOUR BY 582x. Measured 2026-09-16 at section 39's stress
// shape, 44 items x 1,000 steps = 44,000 progress records, median of 5:
//
//	group-by-max      54.4 ms     1.0x   <- taken
//	window           140.8 ms     2.6x
//	not-exists        14.03 s   257.8x
//	correlated        31.68 s   581.8x
//
// The two slow forms are correlated per row: for each of 44,000 steps they
// re-derive that item's group, so the work is quadratic in stream length while
// the two fast ones are one pass. THE INDEX IS NOT THE PROBLEM AND ADDING ONE
// WOULD NOT HAVE HELPED - records_by_kind (project, kind) already exists.
//
// THE INHERITED 613us FOR "correlated" WAS REAL AND MEASURED SOMETHING ELSE:
// a different corpus with no links join, where the inner query was a cheap
// point lookup rather than a group re-derivation. A number carried across a
// schema change is a number about the old schema, which is exactly why the
// ruling says run all four rather than reuse the last answer.
const latestStepsForm = "group-by-max"

// latestSteps returns the newest step on every item in a project, by item id.
func (s *Store) latestSteps(project string) (map[string]Record, error) {
	return s.latestStepsUsing(latestStepsForm, project)
}

// latestStepsUsing runs one named formulation. Exists so the benchmark can
// measure all four against the SAME corpus through the SAME scan path - a
// benchmark that measures its own helper rather than the database is the
// instrument failure this question already paid for once.
func (s *Store) latestStepsUsing(form, project string) (map[string]Record, error) {
	q, ok := latestStepsSQL[form]
	if !ok {
		return nil, fmt.Errorf("record: %q is not a latest-step formulation", form)
	}
	rows, err := s.db.Query(q, project)
	if err != nil {
		return nil, fmt.Errorf("record: latest steps for %s via %s: %w", project, form, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]Record{}
	for rows.Next() {
		rec, item, err := scanStepRow(rows)
		if err != nil {
			return nil, err
		}
		out[item] = rec
	}
	return out, rows.Err()
}

func scanStepRow(sc scanner) (Record, string, error) {
	var (
		rec                  Record
		version, epoch, nano int64
		fields, item         string
	)
	if err := sc.Scan(&rec.ID, &version, &rec.Kind, &rec.Project, &rec.Body,
		&fields, &rec.Prov.Session, &rec.Prov.Seat, &epoch, &nano, &item); err != nil {
		return Record{}, "", err
	}
	var err error
	if rec.Version, err = fromColumn("version", version); err != nil {
		return Record{}, "", err
	}
	if rec.Prov.Epoch, err = fromColumn("epoch", epoch); err != nil {
		return Record{}, "", err
	}
	rec.Prov.CreatedAt = unixNano(nano)
	if err := decodeFields(fields, &rec); err != nil {
		return Record{}, "", err
	}
	return rec, item, nil
}

// coarseCitations counts citations that resolve only to a whole section.
//
// RULED BY BORIS 2026-09-16: the migration imports all 4,206 citations and
// flags the coarse ones rather than dropping them, and he accepted the named
// risk - "a flagged count that nobody ever burns down" - ON THE CONDITION THAT
// project.brief SURFACES IT. A brief that omits this has not implemented the
// ruling.
//
// The grain lives on the DESTINATION record, because a link carries no fields
// of its own. A `cites` edge pointing at a record whose grain is "section" is
// a failed row: section 37 is 942 lines, and a link to it is the same coarse
// pointer wearing a new format. Reads ZERO until the migration runs, which is
// honest rather than empty - the count is a real zero, not a missing feature.
func (s *Store) coarseCitations(project string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM links l
		JOIN records src ON src.id = l.src
		JOIN heads hs ON hs.id = src.id AND hs.version = src.version
		JOIN records dst ON dst.id = l.dst
		JOIN heads hd ON hd.id = dst.id AND hd.version = dst.version
		WHERE l.type = ? AND src.project = ?
		  AND dst.fields ->> 'grain' = 'section'`, LinkCites, project).Scan(&n)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("record: counting coarse citations in %s: %w", project, err)
	}
	return n, nil
}
