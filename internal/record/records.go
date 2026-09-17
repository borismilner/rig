package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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

	// Retraction is set when this record has been WITHDRAWN, and nil otherwise.
	// B77, ruled by Boris 2026-09-17.
	//
	// ⛔ ONLY Get AND GetVersion FILL IT, AND THAT IS THE CONTRACT RATHER THAN
	// AN OVERSIGHT. His sentence is that a retracted record "stops appearing in
	// a brief or a query" and that `record.get` still explains what it was and
	// that it was retracted - so the lists do not carry retracted records at
	// all, and the one verb that still answers about them is the one that says
	// so. A reader holding a record out of Find never needs to check this
	// field, because a retracted one could not have come from there.
	//
	// ⛔ A POINTER AND NOT A BOOL PLUS FIELDS. A retraction carries a reason,
	// a survivor and its own provenance, and a `Retracted bool` would have made
	// "withdrawn, nobody said why" and "not withdrawn" the same zero value on
	// every field that matters.
	Retraction *Retraction
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

// KindDecision, KindRequirement and KindArtefact are the three of section 39's
// ten kinds that nothing could RENDER until B64.
//
// ⛔ NAMING THEM IS NOT THE FIX AND MUST NOT BE REPORTED AS ONE. `kind` has
// never been a closed set and `Put` has never refused a value, so three more
// constants change nothing a caller can observe: `record put --kind decision`
// worked before this line existed, and `record query --kind decision` answered.
// What was missing is that NOTHING TOLD A READER THEY WERE THERE - the brief's
// sections were a closed eleven and none of them mentioned a decision, so the
// records were reachable only by a caller who already knew to ask for them.
// The twelfth section is the fix; these are the spelling it shares with it.
//
// THEY ARE HERE FOR THE REASON KindNote GIVES ABOVE: two derivations asking the
// same question with a literal each is one typo away from a silent miss, and a
// typo in a kind is invisible by construction because an unknown kind is a
// legal kind.
const (
	KindDecision    = "decision"
	KindRequirement = "requirement"
	KindArtefact    = "artefact"
)

// KindWorkItem and KindStandard complete section 39's ten.
//
// ⛔ THE NAMES LAND HERE AND THE LITERALS ARE NOT CHASED IN THE SAME CHANGE.
// `work-item` is a bare string in brief.go, in cmd/rigseed and across the
// tests; replacing them is a wide mechanical diff, and putting one beside a
// behaviour change makes the behaviour change unreviewable. The names exist so
// the next writer has something to reach for.
//
// KindStandard has no consumer at all yet - the standards register is slice 7
// and nothing is built. It is named with the others so that the ten kinds are
// countable in one place, which is the property whose absence let three of them
// go unrendered for four generations.
const (
	KindWorkItem = "work-item"
	KindStandard = "standard"
)

// KindProject and KindCase are section 39's two CONTAINERS.
//
// A case is "for what is ongoing and never ships" - Boris, 2026-09-15: "we have
// projects but we have also things that are ongoing". The brief derives both,
// and which one it is decides which sections mean anything: row 11 is a case's
// notes, and a case has no semver because it does not ship.
const (
	KindProject = "project"
	KindCase    = "case"
)

// The four stages section 39 names for a feature, and there are only four.
const (
	StagePlanned    = "planned"
	StageBuilding   = "building"
	StageShipped    = "shipped"
	StageDeprecated = "deprecated"
)

// The values a work item's `status` field may carry.
//
// ⛔ THE SET HAS A TERMINAL HALF BECAUSE WITHOUT ONE THE STORE INVERTS ITS OWN
// DOCUMENT. `status` was idea-or-active, which has no way to say "this is
// over", so rigseed wrote `active` over every row it imported - including B19,
// RETRACTED as falsified, which the store then published as live work with the
// falsified sentence as its title, and B55 and B56, closed by a ruling, which
// stood in the brief's open list. Measured on the live production store,
// 2026-09-17.
//
// ⛔ AND THE DISPOSITION IS HERE RATHER THAN IN THE PROGRESS STREAM, WHICH IS
// WHERE SECTION 39 WOULD PUT IT, FOR A MEASURED REASON. A step's state travels
// as the `StepState` enum in wire.proto, which has exactly STARTED, BLOCKED
// and DONE; cmd/rig refuses anything else before it sends. A live seeding run
// against a throwaway estate stopped on B55 saying so. Until that enum gains
// members, `done` is the only terminal word the stream can carry, and `done`
// asserts the work was COMPLETED - which is the opposite of what happened to a
// retracted item. Nothing is served by a mechanism that can only lie.
//
// ⛔ THEY ARE NOT ENFORCED IN Put YET, AND THAT IS DELIBERATE RATHER THAN
// FORGOTTEN. A container's status is `open` today (a case's, in the brief's
// own tests), so a closed set enforced here would refuse records the store
// already holds. Naming the words in one place is the half that costs nothing;
// the guard needs a decision about containers first.
const (
	// StatusIdea means the item has not been picked up. Section 39: no
	// progress stream is expected yet.
	StatusIdea = "idea"

	// StatusActive means the item is in hand. It is the ONLY value the brief
	// treats as open work.
	StatusActive = "active"

	// StatusClosed means the work is over WITHOUT the claim that it was
	// finished. It is what a row closed in a document gets when the document
	// did not say which of DONE, CLOSED, REJECTED or RETRACTED it was.
	StatusClosed = "closed"

	// StatusClosedByRuling is backlog.go's third closure convention, kept
	// apart for the reason its own comment gives: "Collapsing it into Done
	// would assert that a ruling completed the work."
	StatusClosedByRuling = "closed-by-ruling"
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
var slugIDKinds = map[string]bool{KindProject: true, KindCase: true}

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

	// ⛔ A PUT WHOSE CONTENT IS ALREADY THE HEAD IS A NO-OP, NOT A NEW VERSION.
	//
	// MEASURED ON THE LIVE PRODUCTION STORE, 2026-09-17: 68 records, 263
	// stored versions, and 195 of them (74%) byte-identical to the version
	// before. Not one record in the store has ever had a content change
	// between two of its versions. Every supersession rig has performed was a
	// no-op, because the seeder probes the head and writes head+1 without ever
	// comparing what it is about to write.
	//
	// That is not untidiness. This file's own argument for append-only is that
	// "what did this say before" becomes a query; over a chain of copies the
	// query answers and the answer is empty. Section 39's slice 1 acceptance -
	// a requirement superseded twice whose FIRST WORDING is read back - passes
	// vacuously when the first wording and the last are the same bytes. And
	// section 39's lossless projection is one file per record so that a diff
	// is readable, which spends the readable diff on 195 empty commits.
	//
	// ⛔ WHAT "IDENTICAL" MEANS, AND THE PERMISSIVE DIRECTION IS THE DANGEROUS
	// ONE. Every part of a Record a reader can observe is compared - kind,
	// project, body, and the whole field map, key set included. Missing one
	// would silently DROP a real edit, which is far worse than the duplicate
	// versions this removes.
	//
	// ⛔ PROVENANCE IS DELIBERATELY NOT PART OF IT. A new session, a new seat,
	// a new epoch and a later clock are not a change to the record: the version
	// chain answers "what did this SAY", provenance answers "who wrote THIS
	// version". Counting them would make this comparison do nothing at all -
	// the four re-runs that produced four identical versions of B1 each ran
	// under a different session id, which is the whole measurement. If the
	// audit trail of "the seeder ran and confirmed no change" is wanted, it is
	// a RUN record, not a version of every row the run touched.
	//
	// IT IS BELOW THE COMPARE-AND-SWAP AND NOT ABOVE IT. A caller naming a
	// stale version has not seen what is there, whatever its content happens
	// to be, and answering "nothing changed" would tell them their read was
	// current when it was not.
	if head > 0 {
		cur, err := versionInTx(ctx, tx, r.ID, head)
		if err != nil {
			return Record{}, err
		}
		if err := checkIdentity(cur, r); err != nil {
			return Record{}, err
		}
		if sameContent(cur, r) {
			return cur, nil
		}
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

// checkIdentity refuses a supersede that changes what the record IS.
//
// ⛔ AN ID IS GLOBAL IN THIS SCHEMA AND NOTHING ELSE GUARDS IT. `heads` is
// keyed on `id` alone and `records` on `(id, version)`, so Put's head lookup
// has no project predicate and cannot be given one without a schema change.
// A put that names an existing id with a different project therefore ANNEXES
// that record: the head moves, and the record's own project queries EMPTY,
// which every read surface reports as "no such work" rather than as an error.
//
// ⛔ IT IS REACHABLE BY AN IMPORTER, NOT ONLY BY MALICE. The store holds
// `B1`..`B63` as ids, which plan/39:800 forbids and :796 rules should be
// UUIDv7 - so two documents in two projects numbering their rows from one is
// all it takes. Demonstrated, not argued: `B1` moved from rig/work-item to
// logbook/requirement in a single put, `Query(rig, work-item)` then returned
// 0, and `History(B1)` interleaved both projects.
//
// ⛔ AND IT COMPOSES WITH THE TRAVERSAL PRUNE. A global id plus a per-hop
// cross-project prune means `refs` says nothing points at the annexed record
// either. Every instrument gives a clean, empty, wrong answer, and none of
// them goes red.
//
// The message names both what was attempted and why it is not a version,
// because the caller that reaches it is a seeder or an importer and the
// person reading it is deciding whether their ID SCHEME is wrong.
func checkIdentity(cur Record, r PutRequest) error {
	const why = "a new version of an id keeps its project and its kind; " +
		"changing either makes it a different record, not a version of this one. " +
		"If two projects number their rows from one, the id scheme is what to " +
		"change: section 39 rules a work item's id is a UUIDv7"
	if cur.Project != r.Project {
		return fmt.Errorf("record: %s is in project %q at version %d and this put says "+
			"project %q: %s", r.ID, cur.Project, cur.Version, r.Project, why)
	}
	if cur.Kind != r.Kind {
		return fmt.Errorf("record: %s is a %q at version %d and this put says kind %q: %s",
			r.ID, cur.Kind, cur.Version, r.Kind, why)
	}
	return nil
}

// versionInTx reads one version of a record INSIDE the caller's transaction.
//
// GetVersion asks the same question of s.db, and that is the wrong connection
// here: the whole point of the no-op check is that it sees what the
// compare-and-swap just read, under the same write lock.
func versionInTx(ctx context.Context, tx *sql.Tx, id string, version uint64) (Record, error) {
	col, err := toColumn("version", version)
	if err != nil {
		return Record{}, err
	}
	rec, err := scanRecord(tx.QueryRowContext(ctx,
		`SELECT id, version, kind, project, body, fields,
			session, seat, epoch, created_at
		 FROM records WHERE id = ? AND version = ?`, id, col))
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, &NotFoundError{ID: id, Version: version}
	}
	return rec, err
}

// sameContent reports whether a put would write exactly what is already there.
//
// A nil field map and an empty one are the same record - `len(Fields)` is 0
// either way and no reader can tell them apart - so maps.Equal's treatment of
// them as equal is the answer this wants rather than an accident of it. That
// is the ONE place this is looser than byte equality, and it is loose in a
// direction nothing can observe.
//
// KIND AND PROJECT STAY IN THE COMPARISON THOUGH checkIdentity ALREADY
// REFUSES A PUT THAT CHANGES EITHER. This predicate is the definition of
// "identical" on its own terms, and the day that refusal is relaxed - a
// deliberate rename, a migration - the no-op must not be the thing that
// silently swallows the change.
func sameContent(cur Record, r PutRequest) bool {
	return cur.Kind == r.Kind &&
		cur.Project == r.Project &&
		cur.Body == r.Body &&
		maps.Equal(cur.Fields, r.Fields)
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
	if err != nil {
		return Record{}, err
	}
	// ⛔ THE ONE VERB THAT STILL ANSWERS ABOUT A WITHDRAWN RECORD HAS TO SAY
	// THAT IT IS ONE. B77's contract: the lists drop it, and `record.get` still
	// explains what it was and that it was retracted. A Get that answered
	// identically for a live and a withdrawn record would make retract
	// indistinguishable from nothing at all to every reader.
	if rec.Retraction, err = s.retractionOf(ctx, id); err != nil {
		return Record{}, err
	}
	return rec, nil
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
	if err != nil {
		return Record{}, err
	}
	// THE RETRACTION IS OF THE RECORD AND NOT OF A VERSION, so every version
	// carries it. Reading version 1 of a withdrawn record must not look like
	// reading a live one.
	if rec.Retraction, err = s.retractionOf(ctx, id); err != nil {
		return Record{}, err
	}
	return rec, nil
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
		   AND NOT EXISTS (SELECT 1 FROM retractions x WHERE x.id = r.id)
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
