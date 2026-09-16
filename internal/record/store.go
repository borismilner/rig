// Package record holds the continuity record: PLAN.md section 39.
//
// It is the project's own memory - what is decided, what is claimed, what must
// be read before a write, and progress as it happens - held as RECORDS, KINDS
// and LINKS rather than as prose in a document nobody reads whole.
//
// WHAT THIS PACKAGE IS NOT. It is not the wire. Section 39's eleven verbs land
// in proto/ as one deliberate change; nothing here knows a protobuf exists. It
// is not the projection either: section 39's spine makes rig the writer and a
// git repository the reader of last resort, and the export is its own slice.
//
// THE STORAGE ENGINE IS NOT HAND-BUILT, per section 38b. BACKLOG.md B28 ran the
// library search over five axes and resolved it by measurement:
// modernc.org/sqlite ALONE, pure Go, carrying the store, query by field, links
// and traversal, and full text in one dependency. bleve was refused at
// +13,344,871 bytes over a corpus of 6,222,787 - twice the size of everything
// it would ever index - and its axis was already served, because FTS5 and
// recursive CTEs are the same binary size as plain SQLite. Both are in the
// amalgamation.
//
// WHY A SECOND ENGINE BESIDE bbolt, WHICH IS A REAL COST AND IS PAID
// DELIBERATELY. internal/coord runs on bbolt because B25 measured it the best
// answer for leases and compare-and-swap, and that search did not cover this
// one (section 39, tension 13). The record wants query by field, ranked full
// text and graph traversal, and bbolt has none of the three natively. Building
// them over a key-value store is the half section 38b objects to.
//
// THE SCHEMA VERSION AND THE REFUSAL BELOW ARE SECTION 39's, and they are the
// same semantics internal/coord already implements over bbolt. The mechanism
// differs - user_version rather than a meta bucket - and the rules do not.
package record

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// The pure-Go SQLite driver, registered for its side effect. Ruled by B28:
	// no cgo, so the daemon cross-compiles and ships as one static binary.
	_ "modernc.org/sqlite"

	"github.com/boris-milner/rig/internal/paths"
)

// SchemaVersion is the on-disk layout this build understands.
//
// IT IS SEPARATE FROM THE WIRE VERSION ON PURPOSE (section 21, section 39): a
// daemon and a store move for different reasons, and one number for both makes
// every wire change look like a migration. internal/coord carries its own for
// the same reason, and the two are independent.
const SchemaVersion = 1

// DBName is the store's file inside the estate's state directory.
const DBName = "record.db"

// FutureSchemaError means the store was written by a newer rigd.
//
// IT REFUSES TO START AND DOES NOT REPAIR (section 39). A downgrade that
// silently opens a newer store is how a rollback destroys the data it exists to
// protect: the old binary cannot see the fields it does not know about, writes
// records without them, and the damage is only visible after the rollback is
// rolled back.
//
// THIS IS THE ONE REQUIREMENT IN THE SET WHOSE FAILURE IS SILENT. A migration
// that does not run fails loudly at the first query. A store opened a version
// too old loses data and looks healthy.
type FutureSchemaError struct {
	Path  string
	Found uint32
	Known uint32
}

func (e *FutureSchemaError) Error() string {
	return fmt.Sprintf(
		"the record store at %s is schema %d and this rigd understands %d: it "+
			"was written by a newer rigd and will not be opened\n"+
			"       rig does not guess at a newer layout and does not repair "+
			"one. Run the newer rigd, or rebuild the store from its export",
		e.Path, e.Found, e.Known)
}

// UnnamedEstateError means persistent state was asked for without an estate name.
//
// Section 37 precondition 2, re-armed by section 39 as the first capability
// that persists anything: two estates sharing one uid must not see one
// another's records.
type UnnamedEstateError struct{ Err error }

func (e *UnnamedEstateError) Error() string {
	return "record: an unnamed estate has no persistent state: " + e.Err.Error()
}
func (e *UnnamedEstateError) Unwrap() error { return e.Err }

// Store is one estate's continuity record.
type Store struct {
	db     *sql.DB
	estate string
	path   string
}

// Open opens (and creates) the record store for a NAMED estate.
//
// The caller is expected to be holding the estate's single-instance lock
// already (section 5f, instance.Acquire): this is the estate's state and two
// daemons over it is what that lock exists to prevent.
func Open(estate string) (*Store, error) {
	dir, err := paths.EstateStateDir(estate)
	if err != nil {
		return nil, &UnnamedEstateError{Err: err}
	}
	// 0700: this is the user's own state and nothing here is a socket, so no
	// client boundary rides on the mode (section 14).
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("record: creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, DBName)

	// THREE SETTINGS, AND EACH ONE IS A REQUIREMENT RATHER THAN A TUNING.
	//
	//   _txlock=immediate takes the write lock at BEGIN rather than at the
	//   first write, so a transaction cannot start, read, and then fail to
	//   upgrade. That upgrade failure is SQLITE_BUSY arriving in the middle of
	//   work that has already decided what to write, which is the shape
	//   compare-and-swap must never be in: the read that the swap is checked
	//   against and the write must be the same transaction or two callers can
	//   both pass their check.
	//
	//   journal_mode(WAL) is section 39's "a reader is never blocked behind a
	//   writer". Every arriving session calls project.brief, and a seat waiting
	//   on another seat's write is the coordination pain the record exists to
	//   remove.
	//
	//   busy_timeout(5000) makes a concurrent writer WAIT rather than fail.
	//   Without it, two seats putting at once produce SQLITE_BUSY, which is a
	//   different error from the version conflict a caller is meant to handle,
	//   and a caller cannot tell "somebody edited this" from "try again".
	db, err := sql.Open("sqlite", "file:"+path+
		"?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("record: opening %s: %w", path, err)
	}

	s := &Store{db: db, estate: estate, path: path}
	if err := s.start(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// start reads the schema version, migrates if it is behind, and stamps it, in
// ONE transaction.
//
// One transaction because a crash between a migration and its version bump
// leaves a store that LIES ABOUT ITSELF - the data moved and the number did
// not - and the next start would then migrate data that is already migrated.
// Section 39 requires the data change and the version bump to be atomic for
// exactly this reason.
func (s *Store) start() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var found uint32
	if err := tx.QueryRow("PRAGMA user_version").Scan(&found); err != nil {
		return fmt.Errorf("record: reading user_version: %w", err)
	}

	switch {
	case found == 0:
		// A new store. Build it and stamp it.
		//
		// ABSENT IS NOT THE SAME AS UNKNOWN, and conflating the two is how a
		// rollback initialises over live data. Both mean "I cannot read this";
		// only one of them is safe to answer by building a new schema.
		if err := createSchema(tx); err != nil {
			return err
		}
	case found > SchemaVersion:
		// THE REFUSAL. It does not open the store read-only, does not migrate
		// down, does not repair, and does not rename the file aside. Every one
		// of those is a guess about a layout this build has never seen.
		//
		// It runs BEFORE any migration and before anything is served, because a
		// check that happens after the first write has not prevented anything.
		return &FutureSchemaError{Path: s.path, Found: found, Known: SchemaVersion}
	case found < SchemaVersion:
		// Where forward migrations run. There are none yet because this IS
		// schema 1; the branch is named so the first one has an obvious home
		// and cannot be bolted onto the check above.
		if err := migrate(tx, found); err != nil {
			return err
		}
	}

	// PRAGMA does not take a bound parameter, and SchemaVersion is a constant
	// in this package rather than anything a caller supplies.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("record: stamping user_version: %w", err)
	}
	return tx.Commit()
}

// createSchema builds a new store at the current version.
func createSchema(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE records (
	id         TEXT    NOT NULL,
	version    INTEGER NOT NULL,
	kind       TEXT    NOT NULL,
	project    TEXT    NOT NULL,
	body       TEXT    NOT NULL,
	fields     TEXT    NOT NULL,
	session    TEXT    NOT NULL,
	seat       TEXT    NOT NULL,
	epoch      INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (id, version)
) STRICT;

CREATE TABLE heads (
	id      TEXT    PRIMARY KEY,
	version INTEGER NOT NULL
) STRICT;

-- LINKS ARE INDEXED IN BOTH DIRECTIONS, and the reverse one is the point.
-- record.refs is "what points AT this record", which section 39 calls the
-- direction files cannot go, and it has to be a LOOKUP rather than a scan at
-- any fan-in: rig's own citation graph has one node at in-degree 758.
--
-- THE REVERSE INDEX IS ALSO WHAT MAKES THE CYCLE GUARD POSSIBLE. Section 39,
-- "a blocks cycle is DETECTED AND REPORTED, NEVER RESOLVED": it is found rather
-- than run into, it appears in the brief as a blocked condition naming its
-- items, every item outside it keeps its place, and rig does not pick an edge to
-- break. Choosing which blocks edge is the wrong one is a judgement about the
-- work, which is domain logic, which is non-goal 1.
--
-- SO A CYCLE IS NOT REFUSED AT LINK TIME. The schema has to let a traversal
-- find one cheaply rather than hope none exists, which is what the reverse
-- index is for.
CREATE TABLE links (
	src  TEXT NOT NULL,
	type TEXT NOT NULL,
	dst  TEXT NOT NULL,
	PRIMARY KEY (src, type, dst)
) STRICT;

CREATE INDEX records_by_kind ON records (project, kind);
CREATE INDEX links_by_dst ON links (dst, type);
`
	if _, err := tx.Exec(ddl); err != nil {
		return fmt.Errorf("record: creating schema: %w", err)
	}
	return nil
}

// migrations maps the schema a store is AT to the step that moves it forward
// one version. Empty because this IS schema 1 and nothing has ever shipped
// before it.
//
// KEYED BY THE VERSION IT MIGRATES FROM, so a step cannot be run against a
// store it was not written for, and so "which of these has already run" is
// answered by the stamped version rather than by a second ledger that can
// disagree with it.
var migrations = map[uint32]func(*sql.Tx) error{}

// migrate runs the forward migrations from found up to SchemaVersion.
//
// FORWARD ONLY, and no down-migration is written or run (section 39). Each step
// must be IDEMPOTENT, because the way this actually fails is a crash partway
// rather than a clean failure: the process dies between the step and the stamp,
// and the next start runs the same step again against data it already moved.
//
// The caller runs this inside the same transaction as the stamp, so a crash
// leaves the store at its old version with none of the step applied, rather
// than at a version that lies about what is in it.
func migrate(tx *sql.Tx, found uint32) error {
	for v := found; v < SchemaVersion; v++ {
		step, ok := migrations[v]
		if !ok {
			return fmt.Errorf("record: no migration from schema %d, which this "+
				"build should not be able to produce", v)
		}
		if err := step(tx); err != nil {
			return fmt.Errorf("record: migrating schema %d to %d: %w", v, v+1, err)
		}
	}
	return nil
}

// Close releases the store.
func (s *Store) Close() error { return s.db.Close() }

// Path is where the store lives, for a caller that has to name it.
func (s *Store) Path() string { return s.path }
