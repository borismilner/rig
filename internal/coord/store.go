// Package coord holds the coordination state that must survive a restart, and
// keys all of it to ONE ESTATE.
//
// It is PLAN.md section 37's isolation preconditions 2 and 4, re-armed by
// section 39 as the first capability that persists anything:
//
//   - PRECONDITION 2, state scoped per estate. Everything here lives under
//     paths.EstateStateDir, which is $XDG_STATE_HOME/rig/estates/<name>/. An
//     UNNAMED ESTATE GETS NO PERSISTENT STATE AT ALL and Open refuses it, which
//     is the answer rather than an omission - see paths.EstateStateDir.
//   - PRECONDITION 4, a restart is survivable and distinguishable from a blip.
//     The daemon publishes an EPOCH, every handle carries it, and it is bumped
//     ON EVERY START, UNCONDITIONALLY.
//
// WHY THE EPOCH IS UNCONDITIONAL, since a planned restart obviously could tell
// the store it was planned. A handle that trusts a restart because it was told
// about that restart in advance is a handle trusting a claim, and it breaks the
// first time the claim is wrong. The cost of treating a planned restart as a
// crash is one re-acquisition; the cost of the reverse is a fencing token that
// outlives the thing it fences, which is the failure leases exist to prevent.
// And rig's own development is the case that creates this constantly: rebuild,
// restart, rebuild is the inner loop of self-hosting, so the estate that walks
// this path most is the one running the work.
//
// THE STORAGE ENGINE IS SQLITE, THROUGH internal/store, as every other
// database in rig is (plan/48 decision 1, decision 0251). It replaced bbolt,
// which B25 had chosen before B28 ruled one engine. What is written here is
// the SEMANTICS section 16 specifies - the two-step expiry, the witness and the
// fencing - which no storage engine has an opinion about. A file still in
// bbolt's format is refused by name and converted once by cmd/coordconvert.
package coord

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/store"
)

// SchemaVersion is the on-disk layout this build understands.
//
// IT IS SEPARATE FROM THE WIRE VERSION ON PURPOSE (section 21, section 39): a
// daemon and a store move for different reasons, and one number for both makes
// every wire change look like a migration.
const SchemaVersion = 1

// DBName is the store's file inside the estate's state directory.
const DBName = "coord.db"

// txTimeout bounds one transaction, the deadline section 3 owes every call.
//
// Two rigd in one estate is already refused by the pidfile flock before any of
// this runs (section 5f), so a long wait here means something is wrong rather
// than busy, and blocking forever would make that look like a hang.
const txTimeout = 10 * time.Second

// FutureSchemaError means the store was written by a newer rigd.
//
// IT REFUSES TO START AND DOES NOT REPAIR (section 39). It is the one runner's
// error, so coord and the record store refuse in the same words.
type FutureSchemaError = store.FutureSchemaError

// BoltFileError means the estate's coord.db is still in bbolt's format.
//
// IT IS REFUSED, NOT CONVERTED IN PLACE (decision 0251): the file holds the
// epoch, which must never go backwards, and unread mail. The conversion is a
// one-off a person runs with rigd stopped, and it leaves the original beside
// the new file.
type BoltFileError struct{ Path string }

func (e *BoltFileError) Error() string {
	return fmt.Sprintf(
		"coord: %s is a bbolt file, and this rigd stores coord in SQLite: it "+
			"will not be opened\n"+
			"       stop rigd and run `coordconvert %s` once; it keeps the epoch "+
			"and the mail and leaves the original beside it as %s.bbolt",
		e.Path, e.Path, e.Path)
}

// UnnamedEstateError means persistent state was asked for without an estate name.
type UnnamedEstateError struct{ Err error }

func (e *UnnamedEstateError) Error() string {
	return "coord: an unnamed estate has no persistent state: " + e.Err.Error()
}
func (e *UnnamedEstateError) Unwrap() error { return e.Err }

// Store is one estate's coordination state.
type Store struct {
	db     *sql.DB
	estate string
	path   string
	epoch  uint64
	bootID string
	// rebooted records that the machine booted since this store was last
	// opened, which invalidates every stored deadline and every pid witness.
	rebooted bool
}

// Open opens (and creates) the store for a NAMED estate, bumping the epoch.
//
// THE EPOCH BUMP IS PART OF OPENING AND NOT A SEPARATE CALL, so there is no
// path that opens the store without taking a new epoch. A caller that could
// skip it would eventually skip it.
//
// The caller is expected to be holding the estate's single-instance lock
// already (section 5f, instance.Acquire): this is the estate's state and two
// daemons over it is exactly what that lock exists to prevent.
func Open(estate string) (*Store, error) {
	dir, err := paths.EstateStateDir(estate)
	if err != nil {
		return nil, &UnnamedEstateError{Err: err}
	}
	path := filepath.Join(dir, DBName)
	if err := refuseBolt(path); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), txTimeout)
	defer cancel()
	db, err := store.OpenDB(ctx, path, schema)
	if err != nil {
		return nil, err
	}

	bootID, err := readBootID()
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	s := &Store{db: db, estate: estate, path: path, bootID: bootID}
	if err := s.start(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// sqliteMagic is the first 16 bytes of every SQLite file.
var sqliteMagic = []byte("SQLite format 3\x00")

// refuseBolt refuses a file that is not empty and not SQLite, BEFORE the
// opener touches it: sqlite would answer "not a database" and name nothing.
func refuseBolt(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("coord: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len(sqliteMagic))
	n, err := io.ReadFull(f, head)
	if n == 0 && (errors.Is(err, io.EOF) || err == nil) {
		return nil // empty: a valid empty database, which the opener builds
	}
	if !bytes.Equal(head[:n], sqliteMagic) {
		return &BoltFileError{Path: path}
	}
	return nil
}

// schema is coord's registration with the one runner (plan/48 decision 3).
//
// LEASES, TASKS AND MESSAGES ARE STORED AS THE SAME JSON THE bbolt BUCKETS
// HELD, one row each, so the converter copies values rather than re-encoding
// them and a field added to a record needs no column. The keys a read seeks
// on - a lease's name, a message's seat and id, a task's queue, sequence and
// idempotency key - are columns.
var schema = store.Schema{
	What:    "coord store",
	Version: SchemaVersion,
	Create:  createSchema,
	Steps:   map[uint32]store.Step{},
}

func createSchema(ctx context.Context, tx *sql.Tx) error {
	const ddl = `
CREATE TABLE meta (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	epoch       INTEGER NOT NULL,
	boot_id     TEXT    NOT NULL,
	message_seq INTEGER NOT NULL
) STRICT;

CREATE TABLE leases (
	name TEXT PRIMARY KEY,
	rec  TEXT NOT NULL
) STRICT;

CREATE TABLE messages (
	id   INTEGER PRIMARY KEY,
	seat TEXT    NOT NULL,
	msg  TEXT    NOT NULL
) STRICT;
CREATE INDEX messages_by_seat ON messages (seat, id);

CREATE TABLE messages_trimmed (
	seat    TEXT    PRIMARY KEY,
	through INTEGER NOT NULL
) STRICT;

CREATE TABLE queues (
	name TEXT    PRIMARY KEY,
	seq  INTEGER NOT NULL
) STRICT;

CREATE TABLE tasks (
	queue TEXT    NOT NULL,
	seq   INTEGER NOT NULL,
	key   TEXT    NOT NULL,
	done  INTEGER NOT NULL,
	rec   TEXT    NOT NULL,
	PRIMARY KEY (queue, seq),
	UNIQUE (queue, key)
) STRICT;
`
	_, err := tx.ExecContext(ctx, ddl)
	return err
}

// update runs fn in one write transaction. _txlock=immediate takes the write
// lock at BEGIN, so every read-evaluate-write in this package is serialised
// exactly as bbolt's single writer serialised it.
func (s *Store) update(fn func(ctx context.Context, tx *sql.Tx) error) error {
	return s.run(false, fn)
}

// view runs fn in a read transaction, which WAL lets run beside a writer.
func (s *Store) view(fn func(ctx context.Context, tx *sql.Tx) error) error {
	return s.run(true, fn)
}

func (s *Store) run(readOnly bool, fn func(ctx context.Context, tx *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), txTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		return fmt.Errorf("coord: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// start bumps the epoch and records the boot, in ONE transaction.
//
// One transaction because a crash between them would leave a store that has
// been opened without having taken an epoch, and the next daemon could not
// tell. The schema check already ran in the opener, and a refused schema
// never reaches here, so a refused open bumps nothing.
func (s *Store) start() error {
	return s.update(func(ctx context.Context, tx *sql.Tx) error {
		var (
			epoch uint64
			prev  string
		)
		err := tx.QueryRowContext(ctx, `SELECT epoch, boot_id FROM meta WHERE id = 1`).Scan(&epoch, &prev)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// A new store.
		case err != nil:
			return fmt.Errorf("coord: reading the epoch: %w", err)
		default:
			s.rebooted = prev != s.bootID
		}

		// THE BUMP. Unconditional, on every start, with nothing consulted.
		epoch++
		if _, err := tx.ExecContext(ctx, `
INSERT INTO meta (id, epoch, boot_id, message_seq) VALUES (1, ?, ?, 0)
ON CONFLICT (id) DO UPDATE SET epoch = excluded.epoch, boot_id = excluded.boot_id`,
			epoch, s.bootID); err != nil {
			return fmt.Errorf("coord: bumping the epoch: %w", err)
		}
		s.epoch = epoch
		return nil
	})
}

// Epoch is the epoch this daemon published when it started. Every handle it
// issues carries it.
func (s *Store) Epoch() uint64 { return s.epoch }

// Estate is the estate this store belongs to.
func (s *Store) Estate() string { return s.estate }

// Path is the store file.
func (s *Store) Path() string { return s.path }

// BootID is the boot this daemon started under.
func (s *Store) BootID() string { return s.bootID }

// Rebooted reports whether the machine booted since this store was last opened.
//
// It is worth reporting rather than merely acting on: every deadline in the
// store is meaningless and every pid witness is dead, so a restart after a
// reboot frees leases wholesale, and an operator seeing that wants to know it
// was a reboot rather than a bug.
func (s *Store) Rebooted() bool { return s.rebooted }

// Close closes the store. The epoch is already durable.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return fmt.Errorf("coord: closing %s: %w", s.path, err)
	}
	return nil
}

// ErrClosed is returned by a call on a closed store.
var ErrClosed = errors.New("coord: the store is closed")
