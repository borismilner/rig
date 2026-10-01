package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// This file is the one opener and the one migration runner every database in
// rig goes through: the record store, coord, the files index and each
// program's store (plan/48 decisions 1 and 3, B103). Before it, each wrote its
// own, and the runner is the part whose failure is silent.

// Step moves a database forward one version, inside the runner's transaction.
type Step func(ctx context.Context, tx *sql.Tx) error

// Schema is what a namespace registers with the runner: the version this build
// writes, how to build a new file at that version, and the forward steps.
type Schema struct {
	// What names the database in errors: "record store", "coord store".
	What string
	// Version is the layout this build writes, stamped in PRAGMA user_version.
	Version uint32
	// Create builds an empty file straight at Version. Absent is not unknown:
	// version 0 is the only value that may be answered by building.
	Create Step
	// Steps maps a version to the step that takes a file from it to the next.
	Steps map[uint32]Step
}

// FutureSchemaError means the file was written by a newer rigd.
//
// IT REFUSES TO OPEN AND DOES NOT REPAIR (section 39). An older binary writing
// into a newer layout cannot see the fields it does not know, writes without
// them, and the loss is only visible after the rollback is rolled back. Of all
// the storage requirements this is the one whose failure is silent.
type FutureSchemaError struct {
	What  string
	Path  string
	Found uint32
	Known uint32
}

func (e *FutureSchemaError) Error() string {
	what := e.What
	if what == "" {
		what = "store"
	}
	return fmt.Sprintf(
		"the %s at %s is schema %d and this rigd understands %d: it "+
			"was written by a newer rigd and will not be opened\n"+
			"       rig does not guess at a newer layout and does not repair "+
			"one. Run the newer rigd, or rebuild the store from its export",
		what, e.Path, e.Found, e.Known)
}

// OpenDB opens (creating owner-only if needed) the SQLite file at path and
// brings it to sc.Version before returning.
//
// THREE SETTINGS, AND EACH ONE IS A REQUIREMENT RATHER THAN A TUNING:
//
//   - _txlock=immediate takes the write lock at BEGIN, so compare-and-swap reads
//     and writes in one transaction and cannot fail to upgrade halfway.
//   - journal_mode(WAL) is section 39's "a reader is never blocked behind a
//     writer".
//   - busy_timeout(5000) makes a concurrent writer wait rather than fail with an
//     error a caller cannot tell from a version conflict.
func OpenDB(ctx context.Context, path string, sc Schema) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("%s: creating %s: %w", sc.What, filepath.Dir(path), err)
	}
	// Created owner-only before sqlite sees it: sqlite would create it 0644
	// under the usual umask, and its -wal and -shm files copy the database's
	// mode. An empty file is a valid empty database; an existing file keeps
	// its mode.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%s: creating %s: %w", sc.What, path, err)
	}
	_ = f.Close()

	db, err := sql.Open("sqlite", "file:"+path+
		"?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("%s: opening %s: %w", sc.What, path, err)
	}
	if err := Migrate(ctx, db, path, sc); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// OpenReadOnly opens a SQLite file that this connection can never write,
// including the recovery a plain open would perform: for reading back an
// artefact such as a snapshot.
func OpenReadOnly(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=query_only(1)")
	if err != nil {
		return nil, fmt.Errorf("store: opening %s read-only: %w", path, err)
	}
	return db, nil
}

// Migrate reads the file's version, refuses a future one, and builds or steps
// it forward and stamps it, in ONE transaction: a crash between a step and its
// stamp would leave a file that lies about itself, and the next start would
// migrate data already migrated.
func Migrate(ctx context.Context, db *sql.DB, path string, sc Schema) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin: %w", sc.What, err)
	}
	defer func() { _ = tx.Rollback() }()

	var found uint32
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&found); err != nil {
		return fmt.Errorf("%s: reading user_version: %w", sc.What, err)
	}
	switch {
	case found == sc.Version:
		return nil
	case found > sc.Version:
		// Before any step and before anything is served: a check after the
		// first write has prevented nothing.
		return &FutureSchemaError{What: sc.What, Path: path, Found: found, Known: sc.Version}
	case found == 0:
		if err := sc.Create(ctx, tx); err != nil {
			return fmt.Errorf("%s: creating %s: %w", sc.What, path, err)
		}
	default:
		for v := found; v < sc.Version; v++ {
			step, ok := sc.Steps[v]
			if !ok {
				return fmt.Errorf("%s: no migration from schema %d, which this "+
					"build should not be able to produce", sc.What, v)
			}
			if err := step(ctx, tx); err != nil {
				return fmt.Errorf("%s: migrating schema %d to %d: %w", sc.What, v, v+1, err)
			}
		}
	}
	// PRAGMA takes no bound parameter; the value is the namespace's constant,
	// never a caller's.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", sc.Version)); err != nil {
		return fmt.Errorf("%s: stamping %s: %w", sc.What, path, err)
	}
	return tx.Commit()
}

// Version reads a file's stamped schema version without migrating it, for the
// store listing.
func Version(ctx context.Context, db *sql.DB) (uint32, error) {
	var v uint32
	err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}
