package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// MaxAttachments is the largest number of records one Append may attach to.
//
// ⛔ IT IS A BOUND ON WORK DRIVEN BY A REMOTE CALLER, not a taste. The list
// arrives over a socket, every entry costs an existence lookup inside the
// write transaction, and an unbounded list is an unbounded transaction holding
// the store's write lock. The standing rules name unchecked bounds by name.
//
// The number is far above any real use - section 09's working note associates
// to "a work-item, a project, a task, anything it wants", and no caller has
// ever named more than two - and far below anything that could hold the lock
// long enough to matter.
const MaxAttachments = 64

// ErrTooManyAttachments refuses an Append whose attachment list is over the
// bound.
//
// ⛔ REFUSED RATHER THAN TRUNCATED. Truncating writes the record and silently
// drops the association, which is the exact shape this package keeps
// recording: a success that lost part of what it was handed, with nothing in
// the answer to say so.
var ErrTooManyAttachments = fmt.Errorf(
	"record: an append may attach to at most %d records", MaxAttachments)

// AppendRequest writes ONE NEW record and attaches it to records that already
// exist, in one transaction.
//
// WHY THIS IS NOT Put PLUS Link, WHICH IS THE OBVIOUS SHAPE AND IS WRONG FOR A
// CALLER THAT CAN DIE. Two calls have a gap between them, and a process killed
// in that gap leaves a record with no edge - reachable by nothing that asks
// about the thing it was written about. Step already refuses to have that gap
// for exactly this reason and says so; this is the same argument for callers
// that are not progress steps.
//
// ⛔ IT IS KIND-AGNOSTIC AND MUST STAY SO. plan/50 decision 5: the store does
// not know what its callers' kinds mean. Append names no kind, validates none
// beyond the two rules Put already enforces, and every vocabulary decision -
// what a kind is called, what its fields mean - belongs to the package above.
type AppendRequest struct {
	Kind    string
	Project string
	Body    string
	Fields  map[string]string

	// PartOf are the ids this record is attached to, with LinkPartOf.
	//
	// ⛔ AN ID HERE THAT DOES NOT EXIST DOES NOT FAIL THE WRITE. It is
	// reported in Appended.Missing and the record lands anyway. The rule this
	// serves is section 09's acceptance test, "so that nothing is ever lost":
	// a caller that mistyped one of three targets must not lose the prose it
	// was trying to attach. Link refuses a dangling edge and keeps refusing
	// it; what changes here is which of the two the caller loses.
	PartOf []string

	Session string
	Seat    string
	Epoch   uint64
}

// Appended is the record that was written and an exact account of the
// attachments.
//
// ⛔ TWO DISJOINT LISTS AND NOT A COUNT, for the reason RecordReplacement
// already gives about its three: "attached 2" over three requested targets
// leaves a caller unable to say which one did not take.
type Appended struct {
	Record Record

	// Attached are the ids an edge was written to, in the order given, with
	// duplicates collapsed.
	Attached []string

	// Missing are the ids that hold no record, so no edge was written. EMPTY
	// IS THE NORMAL ANSWER and a non-empty one is not an error: see PartOf.
	Missing []string
}

// Append writes a new record at version 1 and attaches it, in one transaction.
//
// APPEND-ONLY BY CONSTRUCTION, exactly as Step is: the id is minted here, so
// there is no id for a caller to supersede and no compare-and-swap to lose.
// A caller that wants to REPLACE what it wrote uses Put with the id this
// returns.
func (s *Store) Append(ctx context.Context, r AppendRequest) (Appended, error) {
	if r.Kind == "" || r.Project == "" {
		return Appended{}, errors.New("record: an append needs a kind and a project")
	}
	// THE TWO RULES Put ALREADY ENFORCES, AND NO THIRD ONE.
	//
	// A progress step is reached through Step and nowhere else, because
	// section 39 puts a work item's live state in the latest step; a second
	// door into that stream would make "the latest step is the live state"
	// false the first time anybody used it. A project or a case is identified
	// by a slug it supplies, and Append mints a UUIDv7, so it cannot write one.
	if r.Kind == KindProgress {
		return Appended{}, fmt.Errorf("record: a %s record is appended with Step, not with Append", KindProgress)
	}
	if slugIDKinds[r.Kind] {
		return Appended{}, fmt.Errorf("record: a %s is identified by its slug, so it is written with Put and an id, not appended", r.Kind)
	}
	if r.Session == "" || r.Seat == "" {
		return Appended{}, errors.New("record: an append needs its provenance: session and seat")
	}
	if len(r.PartOf) > MaxAttachments {
		return Appended{}, ErrTooManyAttachments
	}

	targets, err := attachTargets(r.PartOf)
	if err != nil {
		return Appended{}, err
	}

	id, err := uuidV7()
	if err != nil {
		return Appended{}, err
	}

	fields, err := json.Marshal(r.Fields)
	if err != nil {
		return Appended{}, fmt.Errorf("record: encoding fields: %w", err)
	}

	put := PutRequest{
		ID: id, Kind: r.Kind, Project: r.Project, Body: r.Body,
		Fields: r.Fields, Session: r.Session, Seat: r.Seat, Epoch: r.Epoch,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Appended{}, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// THE EXISTENCE CHECKS ARE INSIDE THE TRANSACTION, for Step's reason: a
	// target deleted between the check and the insert would leave the dangling
	// edge the check exists to prevent.
	attached := make([]string, 0, len(targets))
	missing := make([]string, 0)
	for _, dst := range targets {
		var exists int
		switch err := tx.QueryRowContext(ctx, `SELECT 1 FROM heads WHERE id = ?`, dst).Scan(&exists); {
		case errors.Is(err, sql.ErrNoRows):
			missing = append(missing, dst)
			continue
		case err != nil:
			return Appended{}, fmt.Errorf("record: looking up %s: %w", dst, err)
		}
		attached = append(attached, dst)
	}

	stamped := Provenance{Session: r.Session, Seat: r.Seat, Epoch: r.Epoch, CreatedAt: now().UTC()}
	if err := writeVersion(ctx, tx, put, 1, string(fields), stamped); err != nil {
		return Appended{}, err
	}
	for _, dst := range attached {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO links (src, type, dst) VALUES (?, ?, ?)
			 ON CONFLICT(src, type, dst) DO NOTHING`,
			id, LinkPartOf, dst,
		); err != nil {
			return Appended{}, fmt.Errorf("record: attaching %s to %s: %w", id, dst, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Appended{}, fmt.Errorf("record: committing %s: %w", id, err)
	}

	return Appended{
		Record: Record{
			ID: id, Version: 1, Kind: r.Kind, Project: r.Project,
			Body: r.Body, Fields: r.Fields, Prov: stamped,
		},
		Attached: attached,
		Missing:  missing,
	}, nil
}

// attachTargets cleans the caller's list: order preserved, duplicates
// collapsed, an empty entry refused.
//
// ⛔ AN EMPTY ENTRY IS REFUSED RATHER THAN SKIPPED. It is what a shell
// variable that expanded to nothing looks like, and skipping it writes the
// record with one fewer association than the caller asked for while reporting
// nothing missing - a silent narrowing, which is the failure this package
// refuses everywhere else.
//
// ⛔ DUPLICATES ARE COLLAPSED RATHER THAN REFUSED, because the edge is a set:
// Link is idempotent by contract and asserting one fact twice carries no new
// information. Collapsing here keeps Attached honest, so a caller that named a
// target twice is not told it took two edges.
func attachTargets(in []string) ([]string, error) {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for i, dst := range in {
		if dst == "" {
			return nil, fmt.Errorf("record: attachment %d is an empty id, which is what a variable that expanded to nothing looks like", i+1)
		}
		if seen[dst] {
			continue
		}
		seen[dst] = true
		out = append(out, dst)
	}
	return out, nil
}
