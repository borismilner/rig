package coord

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The shared table (plan/53 slice 3): small keyed state every seat on the
// estate can see, written under compare-and-swap, which AgentBox's `shared`
// gave agents and rig did not. "Chunk 7 is mine" is the fact it carries, and
// neither a lease nor a signal can.
//
// What it changes from AgentBox's:
//
//   - VERSIONS COME FROM ONE COUNTER AND ARE NEVER REUSED. AgentBox restarts
//     a deleted key at version 1, so a writer holding version 1 of the old
//     claim lands on the new one. Here a key re-created after a delete gets a
//     version nobody has seen.
//   - AN OWNER IS A SEAT AND A WITNESS, the same pair a lease records, so a
//     recycled pid or a reboot never reads as the owner still running.
//   - A DELETE NAMES THE VERSION IT READ. An unconditional delete is how one
//     seat erases the takeover another just made.

const (
	// MaxSharedKeys is how many keys the table holds at once. A new key past
	// it is refused rather than an old one evicted: a claim that vanishes
	// hands one item to two seats.
	MaxSharedKeys = 1000
	// MaxSharedValue bounds one value, in bytes of JSON.
	MaxSharedValue = 16 << 10
	// MaxSharedList bounds one family read.
	MaxSharedList = 200
)

// ErrSharedFull refuses a new key when the table is at MaxSharedKeys.
var ErrSharedFull = fmt.Errorf("coord: the shared table holds %d keys, its cap; delete the keys whose work is finished", MaxSharedKeys)

// Shared is one key as stored.
type Shared struct {
	Key     string          `json:"-"`
	Version uint64          `json:"-"`
	Value   json.RawMessage `json:"value"`
	// Owner is the seat that claimed it, empty for state nobody owns.
	Owner   string  `json:"owner,omitempty"`
	Witness Witness `json:"witness"`
	// By is the seat that wrote this version, owned or not.
	By string `json:"by"`
	// Updated is when this version was written, Unix nanoseconds.
	Updated int64 `json:"updated"`
}

const sharedSchema = `
CREATE TABLE shared (
	key     TEXT    PRIMARY KEY,
	version INTEGER NOT NULL,
	rec     TEXT    NOT NULL
) STRICT;

CREATE TABLE shared_seq (
	id  INTEGER PRIMARY KEY CHECK (id = 1),
	seq INTEGER NOT NULL
) STRICT;
INSERT INTO shared_seq (id, seq) VALUES (1, 0);
`

func addShared(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, sharedSchema)
	return err
}

// SharedSet writes key if it is at expected: 0 means it must not exist, any
// other number is the version the caller read. applied false is a lost race,
// answered with the key as it stands (Version 0 when it does not exist).
func (s *Store) SharedSet(key string, value json.RawMessage, expected uint64, owner string, w Witness, by string) (Shared, bool, error) {
	if s.isClosed() {
		return Shared{}, false, ErrClosed
	}
	var out Shared
	applied := false
	err := s.update(func(ctx context.Context, tx *sql.Tx) error {
		cur, found, err := sharedRow(ctx, tx, key)
		if err != nil {
			return err
		}
		if found != (expected != 0) || found && cur.Version != expected {
			out = cur
			return nil
		}
		if !found {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM shared`).Scan(&n); err != nil {
				return fmt.Errorf("coord: counting shared keys: %w", err)
			}
			if n >= MaxSharedKeys {
				return ErrSharedFull
			}
		}
		var seq uint64
		if err := tx.QueryRowContext(ctx,
			`UPDATE shared_seq SET seq = seq + 1 WHERE id = 1 RETURNING seq`).Scan(&seq); err != nil {
			return fmt.Errorf("coord: numbering %q: %w", key, err)
		}
		next := Shared{Key: key, Version: seq, Value: value, Owner: owner, Witness: w, By: by, Updated: time.Now().UnixNano()}
		if owner == "" {
			next.Witness = Witness{}
		}
		rec, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("coord: encoding %q: %w", key, err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO shared (key, version, rec) VALUES (?, ?, ?)
ON CONFLICT (key) DO UPDATE SET version = excluded.version, rec = excluded.rec`,
			key, seq, string(rec)); err != nil {
			return fmt.Errorf("coord: writing %q: %w", key, err)
		}
		out, applied = next, true
		return nil
	})
	return out, applied, err
}

// SharedDelete removes key if it is still at expected, which must be the
// version the caller read. It answers the key as it was: what went, or what
// refused.
func (s *Store) SharedDelete(key string, expected uint64) (Shared, bool, error) {
	if s.isClosed() {
		return Shared{}, false, ErrClosed
	}
	if expected == 0 {
		return Shared{}, false, errors.New("coord: a delete names the version it read")
	}
	var out Shared
	applied := false
	err := s.update(func(ctx context.Context, tx *sql.Tx) error {
		cur, found, err := sharedRow(ctx, tx, key)
		if err != nil {
			return err
		}
		out = cur
		if !found || cur.Version != expected {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM shared WHERE key = ?`, key); err != nil {
			return fmt.Errorf("coord: deleting %q: %w", key, err)
		}
		applied = true
		return nil
	})
	return out, applied, err
}

// SharedGet reads one key. Not found answers the key at version 0.
func (s *Store) SharedGet(key string) (Shared, bool, error) {
	if s.isClosed() {
		return Shared{}, false, ErrClosed
	}
	var out Shared
	var found bool
	err := s.view(func(ctx context.Context, tx *sql.Tx) error {
		var err error
		out, found, err = sharedRow(ctx, tx, key)
		return err
	})
	return out, found, err
}

// SharedList reads the keys starting with prefix, in key order, at most
// MaxSharedList, and whether there were more. An empty prefix is every key.
func (s *Store) SharedList(prefix string) ([]Shared, bool, error) {
	return s.sharedRows(`SELECT key, version, rec FROM shared WHERE substr(key, 1, ?) = ? ORDER BY key LIMIT ?`,
		MaxSharedList, len(prefix), prefix)
}

// SharedOwnedBy reads every key owner claimed.
func (s *Store) SharedOwnedBy(owner string) ([]Shared, error) {
	out, _, err := s.sharedRows(`SELECT key, version, rec FROM shared WHERE json_extract(rec, '$.owner') = ? ORDER BY key LIMIT ?`,
		MaxSharedKeys, owner)
	return out, err
}

// sharedRows runs query, whose last parameter is the row limit, and answers
// at most limit rows and whether there were more.
func (s *Store) sharedRows(query string, limit int, args ...any) ([]Shared, bool, error) {
	if s.isClosed() {
		return nil, false, ErrClosed
	}
	var out []Shared
	more := false
	err := s.view(func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, append(args, limit+1)...)
		if err != nil {
			return fmt.Errorf("coord: reading shared keys: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			v, err := scanShared(rows)
			if err != nil {
				return err
			}
			if len(out) == limit {
				more = true
				break
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, more, err
}

func sharedRow(ctx context.Context, tx *sql.Tx, key string) (Shared, bool, error) {
	v, err := scanShared(tx.QueryRowContext(ctx, `SELECT key, version, rec FROM shared WHERE key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return Shared{Key: key}, false, nil
	}
	if err != nil {
		return Shared{}, false, err
	}
	return v, true, nil
}

func scanShared(r interface{ Scan(...any) error }) (Shared, error) {
	var (
		v   Shared
		rec string
	)
	if err := r.Scan(&v.Key, &v.Version, &rec); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Shared{}, err
		}
		return Shared{}, fmt.Errorf("coord: reading a shared key: %w", err)
	}
	if err := json.Unmarshal([]byte(rec), &v); err != nil {
		return Shared{}, fmt.Errorf("coord: decoding %q: %w", v.Key, err)
	}
	return v, nil
}
