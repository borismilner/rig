package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// now is the daemon's clock, replaced in tests.
//
// SECTION 39: "provenance timestamps are the daemon's, never the client's."
// A client's clock is a claim, and a record stamped from one cannot be ordered
// against a record stamped from another. Section 16's resume-grace epoch already
// handles a suspended laptop and this inherits it rather than inventing a
// second answer.
var now = time.Now

// Provenance is who wrote a version, when, and under which daemon.
//
// NONE OF IT IS OPTIONAL. Section 39 makes the quotation rule - a quotation
// attributed to Boris that cannot be traced is a paraphrase until proved
// otherwise - a PROVENANCE CHECK rather than an investigation, and a nullable
// field turns the check back into one.
//
// THE EPOCH IS WHY A STAMP FROM BEFORE A RESTART IS DISTINGUISHABLE FROM ONE
// AFTER IT (section 37 precondition 4, re-armed by section 39).
type Provenance struct {
	Session   string
	Seat      string
	Epoch     uint64
	CreatedAt time.Time
}

// Record is one version of one record.
//
// APPEND-ONLY: a change writes a NEW version and the previous one is retained,
// so "what did this say before" is a query rather than a git archaeology
// expedition. Nothing here is ever updated in place except the head pointer.
type Record struct {
	ID      string
	Version uint64
	Kind    string
	Project string
	Body    string
	Fields  map[string]string
	Prov    Provenance
}

// PutRequest creates a record or supersedes one.
type PutRequest struct {
	// ID is the record to write. Slice 1 requires the caller to supply it;
	// section 39 specifies UUIDv7 via google/uuid, which is already in the
	// module graph as an indirect dependency and needs one line of go.mod to
	// become direct. That line is outside the grant this seat was given.
	ID string

	// IfVersion is the version the caller believes is current. ZERO MEANS
	// CREATE, and a create against an id that already exists is a conflict
	// rather than an overwrite.
	IfVersion uint64

	Kind    string
	Project string
	Body    string
	Fields  map[string]string

	Session string
	Seat    string
	Epoch   uint64
}

// ConflictError means the version named by a put is not the current one.
//
// IT CARRIES THE CURRENT VERSION, which is the whole point. Section 39: "a put
// naming a stale version fails and RETURNS THE CURRENT ONE so the caller can
// merge rather than guess." An error that says only "conflict" forces the
// caller into a read-then-retry loop that can livelock.
type ConflictError struct {
	ID      string
	Named   uint64
	Current uint64
}

func (e *ConflictError) Error() string {
	if e.Named == 0 {
		return fmt.Sprintf("record %s already exists at version %d: a put with "+
			"IfVersion 0 creates, and this id is taken", e.ID, e.Current)
	}
	return fmt.Sprintf("record %s is at version %d and the put named %d: "+
		"somebody wrote it since you read it", e.ID, e.Current, e.Named)
}

// NotFoundError means no such record, or no such version of one.
type NotFoundError struct {
	ID      string
	Version uint64
}

func (e *NotFoundError) Error() string {
	if e.Version == 0 {
		return "no record " + e.ID
	}
	return fmt.Sprintf("no version %d of record %s", e.Version, e.ID)
}

// KindNote is Boris's comment and question mechanism, section 39 row 3: a note
// is a RECORD attached to a project or a work-item, never a field on either.
//
// It is named here rather than written as a literal because the brief's
// compact-card flag and the notes-in-full section are two derivations asking
// the same question, and a typo in one of them is an edge nothing looks for -
// the same silent-failure argument that closed the link-type set.
const KindNote = "note"

// KindFeature is section 39 row 10's kind: a feature with a `stage`, which
// Boris asked for by name.
const KindFeature = "feature"

// The four stages section 39 names for a feature, and there are only four.
const (
	StagePlanned    = "planned"
	StageBuilding   = "building"
	StageShipped    = "shipped"
	StageDeprecated = "deprecated"
)

// slugIDKinds are the kinds whose id is a caller-supplied SLUG rather than a
// generated UUIDv7.
//
// SECTION 39 MAKES THIS ITS ONE ID-SCHEME EXCEPTION, and it is not an oversight
// to be tidied away later. A project's id is its slug because that slug is
// already a path on disk - tension 14's name in ~/.rig/scope/name - and every
// other record's "project it belongs to" field is that same slug. A case takes
// the same exception for the same reason. Generating a UUID for either would
// mean the path and the id disagree, and the path is the thing a human reads.
var slugIDKinds = map[string]bool{"project": true, "case": true}

// Put creates a record or supersedes one, and returns the version it wrote.
func (s *Store) Put(ctx context.Context, r PutRequest) (Record, error) {
	if r.Kind == "" || r.Project == "" {
		return Record{}, errors.New("record: a put needs a kind and a project")
	}
	// A PROGRESS STEP IS REACHED THROUGH Step AND NOWHERE ELSE.
	//
	// Section 39 puts a work item's live state in the LATEST step rather than in
	// a status field, so the stream is the state. A stream a caller can write
	// into directly, or supersede, is not a stream - it is a mutable list with
	// an append convention, and "the latest step is the live state" stops being
	// true the first time anybody rewrites one. The invariant is enforced here
	// because documenting it protects nothing.
	if r.Kind == KindProgress {
		return Record{}, fmt.Errorf("record: a %s record is appended with Step, not written with Put", KindProgress)
	}
	if r.ID == "" {
		id, err := s.generateID(r)
		if err != nil {
			return Record{}, err
		}
		r.ID = id
	}
	if r.Session == "" || r.Seat == "" {
		return Record{}, errors.New("record: a put needs its provenance: session and seat")
	}

	fields, err := json.Marshal(r.Fields)
	if err != nil {
		return Record{}, fmt.Errorf("record: encoding fields: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var head uint64
	switch err := tx.QueryRowContext(ctx, "SELECT version FROM heads WHERE id = ?", r.ID).Scan(&head); {
	case errors.Is(err, sql.ErrNoRows):
		head = 0
	case err != nil:
		return Record{}, fmt.Errorf("record: reading head of %s: %w", r.ID, err)
	}

	// COMPARE-AND-SWAP, AND IT IS INSIDE THE TRANSACTION THAT WRITES.
	//
	// The read above and the insert below are one transaction, opened with
	// _txlock=immediate so the write lock is held from BEGIN. That is what makes
	// this a swap rather than a hope: two callers cannot both read version 1 and
	// both proceed, because the second one's BEGIN waits for the first one's
	// COMMIT and then reads the version the first one wrote.
	//
	// A CHECK OUTSIDE THE TRANSACTION PASSES EVERY SEQUENTIAL TEST AND FAILS THE
	// ONLY CASE IT EXISTS FOR, which is why the demonstration for this is eight
	// concurrent putters rather than one stale one.
	//
	// ONE CONCURRENCY MODEL IN THE DAEMON, NOT TWO (section 39): this is the
	// same compare-and-swap the blackboard uses, and a second model is how
	// section 16's lease and claim policies ended up opposite.
	if r.IfVersion != head {
		return Record{}, &ConflictError{ID: r.ID, Named: r.IfVersion, Current: head}
	}

	next := head + 1
	stamped := Provenance{
		Session:   r.Session,
		Seat:      r.Seat,
		Epoch:     r.Epoch,
		CreatedAt: now().UTC(),
	}

	if err := writeVersion(ctx, tx, r, next, string(fields), stamped); err != nil {
		return Record{}, err
	}

	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("record: committing %s version %d: %w", r.ID, next, err)
	}

	return Record{
		ID: r.ID, Version: next, Kind: r.Kind, Project: r.Project,
		Body: r.Body, Fields: r.Fields, Prov: stamped,
	}, nil
}

// Get returns a record at its head.
func (s *Store) Get(ctx context.Context, id string) (Record, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
			r.session, r.seat, r.epoch, r.created_at
		 FROM records r JOIN heads h ON h.id = r.id AND h.version = r.version
		 WHERE r.id = ?`, id)
	rec, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, &NotFoundError{ID: id}
	}
	return rec, err
}

// GetVersion returns one named version of a record.
//
// THIS IS WHAT MAKES APPEND-ONLY WORTH THE STORAGE: section 39's slice 1
// demonstration is a requirement superseded twice whose FIRST WORDING is read
// back with the session that wrote it.
func (s *Store) GetVersion(ctx context.Context, id string, version uint64) (Record, error) {
	col, err := toColumn("version", version)
	if err != nil {
		return Record{}, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, version, kind, project, body, fields,
			session, seat, epoch, created_at
		 FROM records WHERE id = ? AND version = ?`, id, col)
	rec, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, &NotFoundError{ID: id, Version: version}
	}
	return rec, err
}

// Query returns the head of every record of a kind in a project.
//
// THIS IS SECTION 39's "indexed". A requirement cannot hide in 5,218 lines
// because it is not in 5,218 lines: it is a record with a kind.
func (s *Store) Query(ctx context.Context, project, kind string) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
			r.session, r.seat, r.epoch, r.created_at
		 FROM records r JOIN heads h ON h.id = r.id AND h.version = r.version
		 WHERE r.project = ? AND r.kind = ?
		 ORDER BY r.id`, project, kind)
	if err != nil {
		return nil, fmt.Errorf("record: querying %s/%s: %w", project, kind, err)
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

// History returns every version of one record, oldest first, with provenance.
func (s *Store) History(ctx context.Context, id string) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, version, kind, project, body, fields,
			session, seat, epoch, created_at
		 FROM records WHERE id = ? ORDER BY version`, id)
	if err != nil {
		return nil, fmt.Errorf("record: history of %s: %w", id, err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, &NotFoundError{ID: id}
	}
	return out, nil
}

// scanner is what QueryRow and Rows have in common.
type scanner interface{ Scan(dest ...any) error }

func scanRecord(sc scanner) (Record, error) {
	var (
		rec     Record
		version int64
		epoch   int64
		nanos   int64
		fields  string
	)
	if err := sc.Scan(&rec.ID, &version, &rec.Kind, &rec.Project, &rec.Body,
		&fields, &rec.Prov.Session, &rec.Prov.Seat, &epoch, &nanos); err != nil {
		return Record{}, err
	}
	var err error
	if rec.Version, err = fromColumn("version", version); err != nil {
		return Record{}, err
	}
	if rec.Prov.Epoch, err = fromColumn("epoch", epoch); err != nil {
		return Record{}, err
	}
	rec.Prov.CreatedAt = time.Unix(0, nanos).UTC()
	if err := json.Unmarshal([]byte(fields), &rec.Fields); err != nil {
		return Record{}, fmt.Errorf("record: decoding fields of %s v%d: %w",
			rec.ID, rec.Version, err)
	}
	return rec, nil
}

// generateID mints an id for a put that did not carry one.
//
// SECTION 39, THE ID-SCHEME TABLE: UUIDv7, VIA google/uuid. THE VERSION IS THE
// REQUIREMENT AND NOT A DETAIL. v4 is equally unique and would satisfy every
// test that only asks whether two records collide; section 39 chose v7 for
// TIME-ORDERING, so that records sort chronologically by id alone and
// ls records/<kind>/ comes out in the order they were written. A generator that
// is merely unique has met none of the stated requirement.
//
// google/uuid's getV7Time holds a mutex and is documented to return a strictly
// greater (milli << 12 + seq) than any previous call, so ids minted in sequence
// order even inside one millisecond.
func (s *Store) generateID(r PutRequest) (string, error) {
	if slugIDKinds[r.Kind] {
		return "", fmt.Errorf("record: a %s is identified by its slug, so a put creating one needs an id", r.Kind)
	}
	if r.IfVersion != 0 {
		return "", errors.New("record: a put superseding a version needs the id it supersedes")
	}
	return uuidV7()
}

// uuidV7 mints one time-ordered id.
func uuidV7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("record: generating an id: %w", err)
	}
	return id.String(), nil
}

// writeVersion inserts one version and moves the head pointer to it.
//
// IT IS THE SINGLE WRITE PATH AND THAT IS THE POINT. Put reaches it after its
// compare-and-swap; Step reaches it with the version it already knows is 1. A
// second copy of this SQL is how the two would drift - a column added for one
// caller and forgotten for the other is a defect that shows up as a record
// whose provenance is half-written.
func writeVersion(ctx context.Context, tx *sql.Tx, r PutRequest, version uint64, fields string, p Provenance) error {
	vcol, err := toColumn("version", version)
	if err != nil {
		return err
	}
	ecol, err := toColumn("epoch", p.Epoch)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO records (id, version, kind, project, body, fields,
			session, seat, epoch, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, vcol, r.Kind, r.Project, r.Body, fields,
		p.Session, p.Seat, ecol, p.CreatedAt.UnixNano(),
	); err != nil {
		return fmt.Errorf("record: writing %s version %d: %w", r.ID, version, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO heads (id, version) VALUES (?, ?)
		 ON CONFLICT(id) DO UPDATE SET version = excluded.version`,
		r.ID, vcol,
	); err != nil {
		return fmt.Errorf("record: moving head of %s: %w", r.ID, err)
	}
	return nil
}

// unixNano is the store's one place for turning a stored timestamp back into a
// time, so the two scan paths cannot disagree about the unit.
func unixNano(n int64) time.Time { return time.Unix(0, n).UTC() }

// decodeFields unmarshals a record's field blob.
func decodeFields(blob string, rec *Record) error {
	if err := json.Unmarshal([]byte(blob), &rec.Fields); err != nil {
		return fmt.Errorf("record: decoding fields of %s v%d: %w", rec.ID, rec.Version, err)
	}
	return nil
}

// ⛔ THE COLUMN IS SIGNED AND THE FIELD IS NOT, SO EVERY CROSSING IS CHECKED.
//
// SQLite's INTEGER is int64 and section 39's Version and Epoch are uint64, so
// each conversion between them can wrap, silently, in both directions. Neither
// end is reachable by rig's own callers - a version counts up from 1 - but
// "unreachable" was a claim about the callers of the day this was written, and
// SINCE rig 05a3ceb THE DAEMON SERVES THESE VERBS OVER A SOCKET: GetVersion
// takes its version from a remote caller now.
//
// The wrapped write is the worst available shape - it SUCCEEDS, and the record
// reads back later under a version nobody asked for - so both directions refuse
// by name, which is what the rest of this package does with input it cannot
// honour.

// toColumn narrows a uint64 field to the signed column that stores it.
func toColumn(field string, v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("record: %s %d is too large for the store, whose integer column is signed", field, v)
	}
	return int64(v), nil
}

// fromColumn widens a signed column back to the uint64 field it feeds. A
// negative value is not a caller's mistake - it is a corrupt database, and
// saying so is more useful than a very large number.
func fromColumn(field string, v int64) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("record: the store returned %s %d; a negative %s means the database is corrupt", field, v, field)
	}
	return uint64(v), nil
}
