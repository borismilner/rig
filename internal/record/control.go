// Full control over the records: retract, delete and replace.
//
// ⛔ THREE CAPABILITIES AND NOT THREE NAMES FOR ONE. Boris, 2026-09-17, and
// the misspelling in his own sentence is HIS and is kept: correcting a
// quotation is falsifying evidence, and this project has already found two
// invented ones.
//
// implements, quoted verbatim from the transcript. plan/39 carries the same
// sentence. A corrected quotation is no longer a quotation.
//
//	"I want full controll over the records, so everybody can
//	 delete/retract records and replace records"
//
// A seat that ships retract and reports the sentence satisfied has shipped a
// third of it. His two distinguishing questions, and the answers are what keep
// the three verbs apart:
//
//	retract   the id survives, and so does the history
//	delete    neither survives
//	replace   the SURVIVOR's history survives; the loser is withdrawn
//	          and points at it
//
// ⛔ AND "EVERYBODY" IS HIS WORD. Nothing here checks who wrote the record,
// who is asking, or whether a seat owns anything. PLAN.md section 42's default
// is "absolutely without restrictions" - a policy table he configures, not a
// boundary against a hostile agent - so a permission check added here would be
// building the wrong product. There is a test asserting its absence.
//
//nolint:misspell // `controll` is Boris's own spelling in the ruling this file
package record

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Edge is one typed, directed link, as a value a report can carry.
//
// It exists because the destructive verbs owe an ACCOUNT of what they took,
// and a count alone cannot be checked against anything.
type Edge struct {
	Src  string
	Type string
	Dst  string
}

// String is the form the CLI prints and the tests compare.
func (e Edge) String() string { return e.Src + " -" + e.Type + "-> " + e.Dst }

// Retraction is a record withdrawn: what was withdrawn, why, and by whom.
//
// ⛔ IT IS A ROW OF ITS OWN AND NOT A FIELD ON THE RECORD, AND THAT IS THE
// CONTRACT RATHER THAN A STORAGE PREFERENCE. Writing the mark into the record
// would mean a new VERSION, and Boris's table says the history survives a
// retraction - so the one mechanism that expresses "withdrawn" would be the
// one that falsifies the thing it is asserting about.
//
// ⛔ IT IS ALSO NOT A `status` FIELD. `status` is the closed section's
// vocabulary (B68), and a retraction is not a closing word: closed work
// happened and finished, a retracted record should never have been there. The
// brief must not file one under the other, and there is a test saying so.
type Retraction struct {
	ID     string
	Reason string

	// ReplacedBy is the survivor when this retraction came from a replace,
	// and empty when it came from a plain retract. It is what lets
	// `record.get` on the loser tell a reader WHERE THE FACT WENT rather than
	// only that it left.
	ReplacedBy string

	Prov Provenance

	// Already is true when this call found the record already retracted and
	// returned the EARLIER withdrawal untouched.
	//
	// ⛔ THE FIELD EXISTS BECAUSE THE PROVENANCE OF A WITHDRAWAL IS EVIDENCE.
	// A second retract is not an error - the caller is asserting a state, which
	// is Link's set semantics - but the second caller did not withdraw it, and
	// silently restamping the row with their name and their reason would
	// rewrite who decided what. The first withdrawal stands; this says so.
	Already bool
}

// RetractRequest withdraws a record without destroying anything.
type RetractRequest struct {
	ID     string
	Reason string

	Session string
	Seat    string
	Epoch   uint64
}

// DeleteRequest removes a record, its versions and every edge touching it.
type DeleteRequest struct {
	ID string

	// DryRun computes the whole answer and writes nothing.
	//
	// ⛔ IT IS THE SAME CODE PATH, DELIBERATELY. Boris ruled that a dry run
	// must be able to show the dropped edges first; a preview computed by a
	// different query from the act is a preview of something else, and the
	// day they disagree the preview is the one that gets believed.
	DryRun bool
}

// Deletion is what a delete took, and it is the whole point of the verb's
// output.
//
// ⛔ THE ACCOUNT IS OWED BECAUSE THE REFUSAL WAS RULED OUT. Boris chose "delete
// and drop the edges" over the lead's recommendation of "refuse and name the
// citers", with the cost stated: deleting a parent leaves its children with no
// record that they ever had one. ⛔ **SO THE OBLIGATION MOVED FROM THE VERB TO
// THE REPORT.** A destructive verb that answers `deleted` and nothing else is
// this project's B75 - success reported over data loss - arriving through a
// verb whose job is to lose data.
type Deletion struct {
	ID       string
	Versions uint64

	// Edges is every edge dropped, in BOTH directions, each one named.
	//
	// ⛔ BOTH DIRECTIONS, AND THE OUTBOUND HALF IS THE ONE AN IMPLEMENTATION
	// FORGETS. Dropping only the inbound edges leaves the store asserting that
	// a record which no longer exists is part of something.
	Edges []Edge

	DryRun bool
}

// ReplaceRequest puts a DIFFERENT record in the place of an existing one.
//
// ⛔ SUPERSEDE CANNOT EXPRESS THIS, BECAUSE SUPERSEDE KEEPS THE ID. This is the
// duplicate case - two ids holding one fact - and it is why `replace` is not
// optional beside `delete`: it is the case where the edges genuinely must
// survive, onto the survivor.
type ReplaceRequest struct {
	// Old is the loser: the id whose inbound edges move and which is withdrawn.
	Old string

	// New is the survivor: a record that already exists and absorbs them.
	New string

	Reason string

	Session string
	Seat    string
	Epoch   uint64
}

// Replacement accounts for every inbound edge of the loser, in one of three
// buckets, and the three are disjoint and exhaustive.
//
// ⛔ THREE BUCKETS AND NOT A COUNT, for the Deletion reason. A replace that
// answers "moved 1" over three edges is the same reassuring lie: two of them
// went somewhere and the caller cannot tell where.
type Replacement struct {
	Old string
	New string

	// Moved is an edge that now points at the survivor instead.
	Moved []Edge

	// Merged is an edge the survivor ALREADY had. The fact is not lost - it is
	// already true of the survivor - so this is a merge rather than a conflict,
	// and reporting it separately is what stops it reading as a move.
	Merged []Edge

	// Dropped is an edge FROM the survivor to the loser, which would have
	// become a self-edge. ⛔ REPORTED RATHER THAN SILENTLY REMOVED: it is the
	// one bucket where a fact genuinely goes, and in the duplicate case it is
	// the survivor's own note about its copy.
	Dropped []Edge

	Retraction Retraction
}

// Retract withdraws a record. The id and every version survive.
func (s *Store) Retract(ctx context.Context, r RetractRequest) (Retraction, error) {
	return s.retract(ctx, r, "")
}

// retract is the shared body; replace calls it with the survivor's id.
func (s *Store) retract(ctx context.Context, r RetractRequest, replacedBy string) (Retraction, error) {
	if r.ID == "" {
		return Retraction{}, errors.New("record: a retraction needs an id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Retraction{}, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	out, err := retractTx(ctx, tx, r, replacedBy)
	if err != nil {
		return Retraction{}, err
	}
	if err := tx.Commit(); err != nil {
		return Retraction{}, fmt.Errorf("record: committing the retraction of %s: %w", r.ID, err)
	}
	return out, nil
}

func retractTx(ctx context.Context, tx *sql.Tx, r RetractRequest, replacedBy string) (Retraction, error) {
	if err := mustExistTx(ctx, tx, r.ID); err != nil {
		return Retraction{}, err
	}

	// THE EARLIER WITHDRAWAL WINS, AND IS RETURNED RATHER THAN OVERWRITTEN.
	if found, ok, err := retractionTx(ctx, tx, r.ID); err != nil {
		return Retraction{}, err
	} else if ok {
		found.Already = true
		return found, nil
	}

	// ⛔ THROUGH toColumn LIKE EVERY OTHER uint64 THE STORE WRITES. SQLite's
	// INTEGER is signed, so a bare conversion turns an epoch above the int64
	// ceiling into a negative one and the row reads back as a different
	// estate's. The package already has one place that refuses instead.
	epoch, err := toColumn("epoch", r.Epoch)
	if err != nil {
		return Retraction{}, err
	}
	now := now().UTC()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO retractions (id, reason, replaced_by, session, seat, epoch, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Reason, replacedBy, r.Session, r.Seat, epoch, now.UnixNano(),
	); err != nil {
		return Retraction{}, fmt.Errorf("record: retracting %s: %w", r.ID, err)
	}
	return Retraction{
		ID: r.ID, Reason: r.Reason, ReplacedBy: replacedBy,
		Prov: Provenance{
			Session: r.Session, Seat: r.Seat, Epoch: r.Epoch, CreatedAt: now,
		},
	}, nil
}

// retractionTx reads one retraction, if there is one.
func retractionTx(ctx context.Context, tx *sql.Tx, id string) (Retraction, bool, error) {
	var (
		out   Retraction
		epoch int64
		nanos int64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT id, reason, replaced_by, session, seat, epoch, created_at
		 FROM retractions WHERE id = ?`, id).
		Scan(&out.ID, &out.Reason, &out.ReplacedBy, &out.Prov.Session,
			&out.Prov.Seat, &epoch, &nanos)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Retraction{}, false, nil
	case err != nil:
		return Retraction{}, false, fmt.Errorf("record: reading the retraction of %s: %w", id, err)
	}
	if out.Prov.Epoch, err = fromColumn("epoch", epoch); err != nil {
		return Retraction{}, false, err
	}
	out.Prov.CreatedAt = time.Unix(0, nanos).UTC()
	return out, true, nil
}

// retractionOf reads one retraction outside a transaction.
func (s *Store) retractionOf(ctx context.Context, id string) (*Retraction, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	out, ok, err := retractionTx(ctx, tx, id)
	if err != nil || !ok {
		return nil, err
	}
	return &out, nil
}

// Delete removes a record, every version of it, and every edge touching it.
func (s *Store) Delete(ctx context.Context, r DeleteRequest) (Deletion, error) {
	if r.ID == "" {
		return Deletion{}, errors.New("record: a delete needs an id")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Deletion{}, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := mustExistTx(ctx, tx, r.ID); err != nil {
		return Deletion{}, err
	}

	// ⛔ THE ACCOUNT IS TAKEN BEFORE ANYTHING IS REMOVED, and it is the same
	// read in both modes. That is what makes the dry run a preview of THIS act
	// rather than of a second implementation that agrees today.
	edges, err := edgesTouchingTx(ctx, tx, r.ID)
	if err != nil {
		return Deletion{}, err
	}
	versions, err := versionCountTx(ctx, tx, r.ID)
	if err != nil {
		return Deletion{}, err
	}
	out := Deletion{ID: r.ID, Versions: versions, Edges: edges, DryRun: r.DryRun}
	if r.DryRun {
		return out, nil
	}

	for _, q := range []string{
		`DELETE FROM links WHERE src = ? OR dst = ?`,
		`DELETE FROM records WHERE id = ?`,
		`DELETE FROM heads WHERE id = ?`,
		`DELETE FROM retractions WHERE id = ?`,
	} {
		args := []any{r.ID}
		if q == `DELETE FROM links WHERE src = ? OR dst = ?` {
			args = append(args, r.ID)
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return Deletion{}, fmt.Errorf("record: deleting %s: %w", r.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Deletion{}, fmt.Errorf("record: committing the delete of %s: %w", r.ID, err)
	}
	return out, nil
}

// Replace puts New in Old's place: New absorbs Old's inbound edges and Old is
// withdrawn pointing at New.
//
// ⛔ THE LOSER IS RETRACTED AND NOT DELETED, AND THIS IS THE LEAD'S CALL RATHER
// THAN BORIS'S. His sentence says the survivor absorbs the edges; it does not
// say what becomes of the loser. Retracting it keeps `record.get <loser>`
// answering "this was replaced by <survivor>", which is the question a reader
// who holds the old id actually has. ⛔ **Deleting it would answer the
// duplicate case by destroying the evidence that there ever was a duplicate**,
// and `delete` already exists for a caller who wants exactly that.
func (s *Store) Replace(ctx context.Context, r ReplaceRequest) (Replacement, error) {
	switch {
	case r.Old == "" || r.New == "":
		return Replacement{}, errors.New("record: a replace needs both ids")
	case r.Old == r.New:
		// ⛔ IT WOULD RETRACT THE SURVIVOR AND CALL IT A REPLACEMENT.
		return Replacement{}, fmt.Errorf(
			"record: %s cannot replace itself - a replace withdraws the record "+
				"it replaces, so this would withdraw the survivor", r.Old)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Replacement{}, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// BOTH SIDES FIRST. Moving every inbound edge onto an id with no record is
	// a silent way to lose all of them.
	if err := mustExistTx(ctx, tx, r.Old); err != nil {
		return Replacement{}, err
	}
	if err := mustExistTx(ctx, tx, r.New); err != nil {
		return Replacement{}, err
	}

	in, err := inboundEdgesTx(ctx, tx, r.Old)
	if err != nil {
		return Replacement{}, err
	}

	out := Replacement{Old: r.Old, New: r.New}
	for _, e := range in {
		if e.Src == r.New {
			// The survivor's own edge to its copy. Moving it builds a
			// self-edge, so it is dropped and REPORTED as dropped.
			out.Dropped = append(out.Dropped, e)
		} else {
			had, err := edgeExistsTx(ctx, tx, e.Src, e.Type, r.New)
			if err != nil {
				return Replacement{}, err
			}
			if had {
				out.Merged = append(out.Merged, e)
			} else {
				out.Moved = append(out.Moved, e)
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO links (src, type, dst) VALUES (?, ?, ?)
					 ON CONFLICT(src, type, dst) DO NOTHING`,
					e.Src, e.Type, r.New); err != nil {
					return Replacement{}, fmt.Errorf(
						"record: moving %s onto %s: %w", e, r.New, err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM links WHERE src = ? AND type = ? AND dst = ?`,
			e.Src, e.Type, e.Dst); err != nil {
			return Replacement{}, fmt.Errorf("record: dropping %s: %w", e, err)
		}
	}

	reason := r.Reason
	if reason == "" {
		reason = "replaced by " + r.New
	}
	ret, err := retractTx(ctx, tx, RetractRequest{
		ID: r.Old, Reason: reason, Session: r.Session, Seat: r.Seat, Epoch: r.Epoch,
	}, r.New)
	if err != nil {
		return Replacement{}, err
	}
	out.Retraction = ret

	if err := tx.Commit(); err != nil {
		return Replacement{}, fmt.Errorf("record: committing the replace of %s: %w", r.Old, err)
	}
	return out, nil
}

// ---- the small reads these three share ------------------------------------

// mustExistTx refuses an id the store has never held.
//
// ⛔ IT ASKS `records` AND NOT `heads`, so a record that is retracted still
// EXISTS for these verbs. A retracted record must remain deletable and
// replaceable, or a mistaken retraction is unrecoverable.
func mustExistTx(ctx context.Context, tx *sql.Tx, id string) error {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM records WHERE id = ?`, id).Scan(&n); err != nil {
		return fmt.Errorf("record: looking for %s: %w", id, err)
	}
	if n == 0 {
		return &NotFoundError{ID: id}
	}
	return nil
}

func versionCountTx(ctx context.Context, tx *sql.Tx, id string) (uint64, error) {
	var n int64
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM records WHERE id = ?`, id).Scan(&n); err != nil {
		return 0, fmt.Errorf("record: counting the versions of %s: %w", id, err)
	}
	return fromColumn("version count", n)
}

// edgesTouchingTx is every edge with this record at EITHER end, ordered so the
// report is repeatable.
func edgesTouchingTx(ctx context.Context, tx *sql.Tx, id string) ([]Edge, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT src, type, dst FROM links WHERE src = ? OR dst = ?
		 ORDER BY src, type, dst`, id, id)
	if err != nil {
		return nil, fmt.Errorf("record: reading the edges of %s: %w", id, err)
	}
	return scanEdges(rows, id)
}

// inboundEdgesTx is every edge POINTING AT this record.
func inboundEdgesTx(ctx context.Context, tx *sql.Tx, id string) ([]Edge, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT src, type, dst FROM links WHERE dst = ? ORDER BY src, type`, id)
	if err != nil {
		return nil, fmt.Errorf("record: reading what points at %s: %w", id, err)
	}
	return scanEdges(rows, id)
}

func scanEdges(rows *sql.Rows, id string) ([]Edge, error) {
	defer func() { _ = rows.Close() }()
	var out []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst); err != nil {
			return nil, fmt.Errorf("record: scanning an edge of %s: %w", id, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func edgeExistsTx(ctx context.Context, tx *sql.Tx, src, typ, dst string) (bool, error) {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM links WHERE src = ? AND type = ? AND dst = ?`,
		src, typ, dst).Scan(&n); err != nil {
		return false, fmt.Errorf("record: looking for %s -%s-> %s: %w", src, typ, dst, err)
	}
	return n > 0, nil
}
