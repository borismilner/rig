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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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

	// ephemeral is true for an UNNAMED estate's scratch store, which lives in
	// the runtime directory and does not survive it.
	//
	// ⛔ IT IS ON THE STORE RATHER THAN INFERRED FROM THE ESTATE NAME BEING
	// EMPTY, so no caller has to re-derive it and no caller can get it wrong. A
	// surface that rendered a scratch store as the durable record would tell an
	// agent its working notes were saved when they will vanish with the runtime
	// directory - a write that reports success and loses the data, which is
	// B75's shape arriving one layer down.
	ephemeral bool
}

// Ephemeral says whether this store vanishes with the runtime directory.
//
// EVERY SURFACE THAT SHOWS RECORDS OWES THIS ANSWER. "Durable" is the default a
// reader assumes, so the exception has to be carried rather than left to be
// noticed.
func (s *Store) Ephemeral() bool { return s.ephemeral }

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
	return open(dir, estate, false)
}

// OpenScratch opens the EPHEMERAL record store an unnamed estate keeps in its
// runtime directory.
//
// ⛔ IT IS THE SANDBOX AN AGENT HAD NO WAY TO GET, and the absence of it is
// why section 09's A0 survey had zero written after four generations: a third
// estate name is refused, a second daemon on a named estate is B72, and an
// unnamed estate had no store at all - so a seat told to measure the record
// verbs had to choose between not measuring them and writing into the estate on
// the human's screen.
//
// ⛔ IT IS NOT PERSISTENT AND MUST NEVER BECOME SO. EstateStateDir's rule -
// persistent state needs a NAME, because an ephemeral estate that persisted
// would be a third estate arriving by the back door - is untouched: this keys on
// the runtime directory, which the operating system clears, and it sets
// Ephemeral so every surface can say which store a caller is holding.
func OpenScratch() (*Store, error) {
	dir, err := paths.EstateScratchDir()
	if err != nil {
		return nil, err
	}
	// ⛔ IT IS DISCARDED AT EVERY START, AND THAT IS THE SEMANTICS RATHER THAN A
	// CLEANUP. Found by the daemon suite within minutes of this landing: the
	// store keys on the runtime directory, the directory outlives the daemon,
	// and so a second unnamed run inherited the first one's records - a put
	// answered CODE_CONFLICT for an id that test had never written.
	//
	// ⛔ THE BUG WAS THE MEANING, NOT THE COLLISION. EstateStateDir's rule says
	// an ephemeral estate that PERSISTED ACROSS RUNS "would be a third estate
	// arriving by the back door", and a scratch store surviving a daemon
	// restart is exactly that: durable state, keyed on a directory instead of
	// on a name, with nothing claiming the name and nothing able to refuse a
	// duplicate. Discarding it is what makes "ephemeral" true rather than
	// advertised.
	//
	// ⛔ AND IT IS WHY ISOLATION IS THE CALLER'S TO ASK FOR. Two agents that
	// both want a private store give themselves different XDG_RUNTIME_DIRs,
	// which is already the machine-wide singleton for the socket, the pidfile
	// and the flock - so one runtime directory is one daemon by construction
	// and one scratch store belongs to it alone. Sharing the DEFAULT runtime
	// directory means sharing this store; that is the same rule every other
	// per-runtime thing here follows.
	for _, f := range []string{DBName, DBName + "-wal", DBName + "-shm"} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("record: clearing the scratch store: %w", err)
		}
	}
	return open(dir, "", true)
}

func open(dir, estate string, ephemeral bool) (*Store, error) {
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

	s := &Store{db: db, estate: estate, path: path, ephemeral: ephemeral}
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
//
// THE ONE ROOT CONTEXT IN THIS PACKAGE IS HERE, AND IT IS EXCUSED RATHER THAN
// GIVEN A DEADLINE. Open runs once at daemon start, before anything binds, so
// there is no caller deadline to honour: a context threaded in from New would
// only let a slow disk abort startup, which failing to start already does.
// Ruled by the team-lead 2026-09-16 with the blast radius as the argument - a
// thirteenth signature moves daemon.go:239 and New's twelve call sites for no
// change in behaviour. The exemption is one line with a mandatory reason and
// nocontextfree reports it as a violation the moment it stops being needed,
// which is the difference between this and turning the rule off in a config.
func (s *Store) start() error {
	//rig:allow nocontextfree: Open runs once at daemon start and has no caller deadline to honour
	ctx := context.Background()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var found uint32
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&found); err != nil {
		return fmt.Errorf("record: reading user_version: %w", err)
	}

	switch {
	case found == 0:
		// A new store. Build it and stamp it.
		//
		// ABSENT IS NOT THE SAME AS UNKNOWN, and conflating the two is how a
		// rollback initialises over live data. Both mean "I cannot read this";
		// only one of them is safe to answer by building a new schema.
		if err := createSchema(ctx, tx); err != nil {
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
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("record: stamping user_version: %w", err)
	}
	return tx.Commit()
}

// createSchema builds a new store at the current version.
func createSchema(ctx context.Context, tx *sql.Tx) error {
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
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
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

// QueryFilter narrows a Find. EVERY FIELD IS OPTIONAL, and this is the point
// of the type.
//
// ⛔ AN EMPTY FIELD MEANS *EVERY VALUE*, NEVER "THE EMPTY VALUE". A record
// cannot have an empty kind or an empty project - Put refuses both by name -
// so there is no value for an empty filter to collide with.
//
// WHY IT IS A STRUCT AND NOT TWO STRINGS. Two adjacent parameters of the same
// type that a caller can swap with no compiler complaint is the exact shape
// RefsArgs was changed away from. Here it is worse than usual, because a
// swapped project and kind under the old "both required" rule returned zero
// rows, which reads as "the store holds none of those" rather than as a typo.
type QueryFilter struct {
	// Project is the project or case to look in. Empty means every project.
	Project string

	// Kind is what the records are. Empty means every kind.
	//
	// ⛔ THE OPTIONALITY OF THIS FIELD IS WHAT MAKES A CENSUS POSSIBLE AT ALL.
	// `kind` is not a closed set - nothing constrains it beyond non-empty - so
	// while a query REQUIRED one, a record written under an unguessed kind was
	// invisible to every question anybody could ask, and no answer about what
	// the store holds could be complete. That cost a real attack specialist 13
	// round trips and a hedge on a finding that should have been flat.
	Kind string

	// Field and Value are section 39's THIRD filter, and it was missing.
	//
	// ⛔ THIS IS B65. Section 39 defines record.query as "by kind, field and
	// project" and justifies its longest passage - the exhaustive metadata
	// table - with "a typed field an agent queries with record.query". Until
	// this existed, every one of those fields was write-only: they could be
	// stored and they could not be selected on, so `owner`, `status` and
	// `priority` were decoration. The 2026-09-17 attack graded clause B a
	// FAILURE partly on this.
	//
	// Field empty means NO field predicate. Field set means: this record has
	// this key, and its value is exactly Value.
	//
	// ⛔ AND THE EMPTY-MEANS-EVERY RULE ABOVE DOES NOT EXTEND TO Value, WHICH
	// IS THE ONE ASYMMETRY IN THIS TYPE AND IS DELIBERATE. Project and Kind
	// can treat empty as "every value" because Put refuses to write an empty
	// one, so there is no stored value for the wildcard to collide with. A
	// FIELD value has no such guarantee - a record may legitimately carry
	// `closure_note: ""` - so an empty Value here means the empty string, and
	// matching it is a real question somebody may ask. Collapsing the two
	// rules would make `Field: "owner", Value: ""` silently return every
	// record that has an owner, which is a different question wearing the same
	// bytes.
	Field string

	// Value is the exact value Field must have. See Field: empty means the
	// empty string, not "any value".
	Value string
}

// ErrValueWithoutField refuses a filter that names a value and no field.
//
// ⛔ IT IS REFUSED RATHER THAN IGNORED BECAUSE IGNORING IT WIDENS THE ANSWER.
// A caller whose field name expanded to nothing but whose value did not has
// asked a question it cannot have meant, and the silent reading - drop the
// predicate - returns MORE records than intended while looking like a
// successful narrow query. That is the failure this package has recorded
// repeatedly: a result that is wrong in the reassuring direction.
var ErrValueWithoutField = errors.New(
	"record: a query filter names a value with no field to match it against, " +
		"which cannot be what was meant - dropping the predicate would widen " +
		"the answer silently. Name the field, or clear the value")

// Find returns the head of every record matching the filter, with the filter's
// empty fields meaning "every value".
//
// THIS IS SECTION 39's `record.query`, whose specification is "by kind, field
// and project" and which never said all three were mandatory. The CLI and the
// wire had turned an optional filter set into a required conjunction.
//
// ⛔ THE FOUR QUERIES ARE CONSTANTS AND THE FILTER CHOOSES ONE. Two shapes
// were rejected:
//
//   - A disjunction on each filter (matching the empty string OR the column)
//     reads the same and costs differently. SQLite will not use
//     records_by_kind (project, kind) through an OR, so the SCOPED case -
//     which is every call rig makes today - silently degrades to a scan of the
//     table the index was added for.
//   - Assembling the WHERE clause from a slice at run time is the same four
//     queries with a `q +=` in the middle, and gosec is right to flag that
//     even when every operand is a literal: the next person to add a filter
//     reaches for the variable, not for a new constant.
//
// It does NOT replace Query in records.go, which is the both-required special
// case and is owned by another seat this session. Folding that one into
// `return s.Find(ctx, QueryFilter{Project: project, Kind: kind})` is owed, and
// is a one-line change the moment that file is free.
func (s *Store) Find(ctx context.Context, f QueryFilter) ([]Record, error) {
	if f.Field == "" && f.Value != "" {
		return nil, ErrValueWithoutField
	}

	var q string
	var args []any
	switch {
	case f.Project == "" && f.Kind == "" && f.Field == "":
		q = findEverything
	case f.Kind == "" && f.Field == "":
		q, args = findByProject, []any{f.Project}
	case f.Project == "" && f.Field == "":
		q, args = findByKind, []any{f.Kind}
	case f.Field == "":
		q, args = findByProjectAndKind, []any{f.Project, f.Kind}

	// From here the field predicate is present, and the four shapes above
	// repeat with it appended. Eight constants rather than four, written out,
	// because the alternative is the run-time assembly this function's own
	// doc comment refuses.
	case f.Project == "" && f.Kind == "":
		q, args = findByField, []any{f.Field, f.Value}
	case f.Kind == "":
		q, args = findByProjectAndField, []any{f.Project, f.Field, f.Value}
	case f.Project == "":
		q, args = findByKindAndField, []any{f.Kind, f.Field, f.Value}
	default:
		q, args = findByProjectKindAndField,
			[]any{f.Project, f.Kind, f.Field, f.Value}
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("record: querying %s: %w", f.describe(), err)
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

// The four shapes Find can take, as constants so nothing is ever concatenated
// around a caller's value.
//
// ORDERED BY PROJECT FIRST, which is invisible to a scoped call - one project
// means the leading key is constant and the order is by id, exactly as Query
// always returned it - and is the only readable order for an unscoped one.
const (
	findSelect = `SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
			r.session, r.seat, r.epoch, r.created_at
		 FROM records r JOIN heads h ON h.id = r.id AND h.version = r.version`
	findOrder = ` ORDER BY r.project, r.kind, r.id`

	findEverything       = findSelect + findOrder
	findByProject        = findSelect + ` WHERE r.project = ?` + findOrder
	findByKind           = findSelect + ` WHERE r.kind = ?` + findOrder
	findByProjectAndKind = findSelect + ` WHERE r.project = ? AND r.kind = ?` + findOrder

	// ⛔ THE FIELD PREDICATE IS `json_each` AND NOT `json_extract`, AND THE
	// REASON IS THAT A PATH IS A LITTLE LANGUAGE. json_extract wants `$.owner`,
	// so a bound field name has to be concatenated into a path - `'$.' || ?` -
	// and at that point a field named `a.b` navigates two levels and a field
	// with a quote in it is a parse error or worse. Quoting it (`'$."' || ? ||
	// '"'`) moves the problem rather than solving it. json_each binds the KEY
	// as an ordinary parameter compared with `=`, so there is no path, no
	// escaping rule, and no field name that means something other than itself.
	//
	// ⛔ AND IT IS AN `EXISTS` SUBQUERY RATHER THAN A JOIN. A join against
	// json_each multiplies the row by every key in the document and then needs
	// a DISTINCT to put it back, which is a correctness bug waiting for the
	// first caller who adds another predicate. EXISTS is a semi-join: one row
	// in, at most one row out, whatever the document holds.
	//
	// THE COST, NAMED RATHER THAN HIDDEN: this predicate cannot use an index -
	// there is none on `fields` and SQLite cannot build one over json_each - so
	// a field query is a scan of whatever project and kind have already
	// narrowed to. At rig's own 85 records that is nothing. IT IS ORDERED LAST
	// in every constant below so the indexed predicates cut first. If a field
	// query ever becomes hot, the answer is a generated column plus an index on
	// it, not a different query shape here.
	findWhereField = ` EXISTS (SELECT 1 FROM json_each(r.fields) je
			WHERE je.key = ? AND je.value = ?)`

	findByField           = findSelect + ` WHERE` + findWhereField + findOrder
	findByProjectAndField = findSelect + ` WHERE r.project = ? AND` +
		findWhereField + findOrder
	findByKindAndField = findSelect + ` WHERE r.kind = ? AND` +
		findWhereField + findOrder
	findByProjectKindAndField = findSelect +
		` WHERE r.project = ? AND r.kind = ? AND` + findWhereField + findOrder
)

// describe names what a filter asked for, in the words a caller would use.
//
// IT IS FOR ERRORS AND FOR THE CLI's EMPTY ANSWER, and both need the same
// sentence: "no records at all" and "no requirement records in rig" are
// different facts, and a reader who typed nothing needs to be told that
// nothing is what was asked.
func (f QueryFilter) describe() string {
	var what string
	switch {
	case f.Project == "" && f.Kind == "":
		what = "every record in every project"
	case f.Project == "":
		what = f.Kind + " records in every project"
	case f.Kind == "":
		what = "every record in " + f.Project
	default:
		what = f.Kind + " records in " + f.Project
	}

	// ⛔ THE FIELD PREDICATE IS NAMED TOO, AND IT IS QUOTED. "no requirement
	// records in rig with status=closed" and "no requirement records in rig"
	// are different facts, and a reader who narrowed by a field and got
	// nothing needs to see the narrowing in the answer - otherwise the field
	// predicate is the one part of the question that can silently not have
	// happened. The value is quoted because an EMPTY one is a real query, and
	// `with status=` reads like a truncation while `with status=""` does not.
	if f.Field != "" {
		what += " with " + f.Field + "=" + strconv.Quote(f.Value)
	}
	return what
}
