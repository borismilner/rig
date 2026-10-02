package coord

import (
	"context"
	"database/sql"
	"fmt"
)

// A seat's signals, kept past a restart (plan/53 slice 1b). The daemon's bus
// numbers them; this file keeps them, per kind, for MaxSignalsPerKind and
// SignalMaxAge, and records what retention took so a reader is told.
//
// Retention is AgentBox's (1000 per topic, 7 days), and so is the rule that
// a gap is answered from what a trim RECORDED taking per kind, never deduced
// from what survived: one quiet kind's old row would otherwise hide that a
// busy kind was trimmed above the reader's cursor.

const (
	// MaxSignalsPerKind is how many of one kind are kept.
	MaxSignalsPerKind = 1000
	// SignalMaxAge is how long one is kept, in nanoseconds: seven days.
	SignalMaxAge = 7 * 24 * 60 * 60 * 1e9
	// MaxSignalBatch bounds one read.
	MaxSignalBatch = 1000
)

// Signal is one stored signal. Seq is the bus's, unique across restarts.
type Signal struct {
	Seq     uint64
	Kind    string
	At      int64
	Source  string
	To      string // the addressee as the bus names it, or empty for everyone
	Payload string
}

const signalSchema = `
CREATE TABLE signals (
	seq     INTEGER PRIMARY KEY,
	kind    TEXT    NOT NULL,
	at      INTEGER NOT NULL,
	source  TEXT    NOT NULL,
	to_seat TEXT    NOT NULL,
	payload TEXT    NOT NULL
) STRICT;
CREATE INDEX signals_by_kind ON signals (kind, seq);

CREATE TABLE signals_trimmed (
	kind    TEXT    PRIMARY KEY,
	through INTEGER NOT NULL
) STRICT;
`

func addSignals(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, signalSchema)
	return err
}

// PutSignal stores sig and trims its kind, by count and by age.
func (s *Store) PutSignal(sig Signal) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	return s.update(func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO signals (seq, kind, at, source, to_seat, payload) VALUES (?, ?, ?, ?, ?, ?)`,
			sig.Seq, sig.Kind, sig.At, sig.Source, sig.To, sig.Payload); err != nil {
			return fmt.Errorf("coord: storing signal %d: %w", sig.Seq, err)
		}
		return trimSignals(ctx, tx, sig.Kind, sig.At-SignalMaxAge)
	})
}

// trimSignals drops kind's rows past the count or older than cutoff, and
// records the highest seq it dropped.
func trimSignals(ctx context.Context, tx *sql.Tx, kind string, cutoff int64) error {
	var through sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
SELECT max(seq) FROM signals WHERE kind = ? AND (at < ? OR seq <= (
	SELECT seq FROM signals WHERE kind = ? ORDER BY seq DESC LIMIT 1 OFFSET ?))`,
		kind, cutoff, kind, MaxSignalsPerKind).Scan(&through); err != nil {
		return fmt.Errorf("coord: trimming signals of %q: %w", kind, err)
	}
	if !through.Valid {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM signals WHERE kind = ? AND seq <= ?`, kind, through.Int64); err != nil {
		return fmt.Errorf("coord: trimming signals of %q: %w", kind, err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO signals_trimmed (kind, through) VALUES (?, ?)
ON CONFLICT (kind) DO UPDATE SET through = max(through, excluded.through)`, kind, through.Int64); err != nil {
		return fmt.Errorf("coord: recording the trim of %q: %w", kind, err)
	}
	return nil
}

// TrimSignalsBefore ages out every kind at once, for the kinds nobody posts
// to any more. rigd calls it at start.
func (s *Store) TrimSignalsBefore(cutoff int64) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	return s.update(func(ctx context.Context, tx *sql.Tx) error {
		kinds, err := stringColumn(ctx, tx, `SELECT DISTINCT kind FROM signals WHERE at < ?`, cutoff)
		if err != nil {
			return err
		}
		for _, k := range kinds {
			if err := trimSignals(ctx, tx, k, cutoff); err != nil {
				return err
			}
		}
		return nil
	})
}

// SignalsAfter answers the stored signals after seq that keep accepts, oldest
// first, at most limit of them, and whether there were more.
func (s *Store) SignalsAfter(after uint64, limit int, keep func(*Signal) bool) ([]Signal, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, ErrClosed
	}
	var out []Signal
	more := false
	err := s.view(func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT seq, kind, at, source, to_seat, payload FROM signals WHERE seq > ? ORDER BY seq`, after)
		if err != nil {
			return fmt.Errorf("coord: reading signals: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var sig Signal
			if err := rows.Scan(&sig.Seq, &sig.Kind, &sig.At, &sig.Source, &sig.To, &sig.Payload); err != nil {
				return fmt.Errorf("coord: reading signals: %w", err)
			}
			if !keep(&sig) {
				continue
			}
			if len(out) == limit {
				more = true
				break
			}
			out = append(out, sig)
		}
		return rows.Err()
	})
	return out, more, err
}

// SignalsTrimmed answers, per kind, the highest seq retention has dropped.
func (s *Store) SignalsTrimmed() (map[string]uint64, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}
	out := map[string]uint64{}
	err := s.view(func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT kind, through FROM signals_trimmed`)
		if err != nil {
			return fmt.Errorf("coord: reading the signal trims: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var k string
			var through uint64
			if err := rows.Scan(&k, &through); err != nil {
				return err
			}
			out[k] = through
		}
		return rows.Err()
	})
	return out, err
}

func stringColumn(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("coord: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
