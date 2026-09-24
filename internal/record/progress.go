package record

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// stepStates are the three a step may carry.
//
// Section 39: "the live state (started/blocked/done) is the latest
// progress.step, not a second field to keep in sync."
//
// ⛔ RULED: THE `StepState` ENUM IN wire.proto IS THE VOCABULARY, and every
// other copy is derived from it or pinned to it by a test. A program outside
// rig learns the set from the enum it already compiles against, never by
// being refused.
//
// There were four copies and nothing merged them: this map,
// `stepStateSpellings` in cmd/rig/progress.go, `stepStateNames` in
// internal/daemon/record.go, and the enum. The exported `IsStepState`
// predicate left with the seeder at plan/50 move 6. Now the CLI's spellings
// walk the enum's descriptor, and
// TestTheStepVocabularyIsTheWireEnumAndNothingElse in internal/daemon pins
// both this set and the daemon's map to the enum. The store keeps its own copy
// because it does not import the wire, and StepStates exports it so that test
// can hold it to the enum.
var stepStateOrder = []string{"started", "blocked", "done"}

var stepStates = func() map[string]bool {
	m := make(map[string]bool, len(stepStateOrder))
	for _, s := range stepStateOrder {
		m[s] = true
	}
	return m
}()

// StepStates is the set a step may carry, in the wire enum's order. It is a
// copy, so a caller cannot widen what the store accepts.
func StepStates() []string {
	return append([]string(nil), stepStateOrder...)
}

// StepRequest appends one step to a work item's stream.
type StepRequest struct {
	// Item is the record id of the work item this step is about.
	Item string

	// State is started, blocked or done.
	State string

	// Note is what happened, in the seat's own words. Optional: a step with a
	// state and no note is still the signal that something moved.
	Note string

	Project string
	Session string
	Seat    string
	Epoch   uint64
}

// Step appends one step to a work item's stream and returns the step written.
//
// APPEND-ONLY BY CONSTRUCTION: every step is its own record at version 1, and
// nothing supersedes it. The step and its edge to the item are written in ONE
// transaction, because a step that exists with no edge is invisible to every
// derivation that matters - it would be a progress record nothing can find,
// which is worse than a refused write.
func (s *Store) Step(ctx context.Context, r StepRequest) (Record, error) {
	if r.Item == "" {
		return Record{}, errors.New("record: a step needs the item it is about")
	}
	if !stepStates[r.State] {
		return Record{}, fmt.Errorf("record: %q is not a step state; it is started, blocked or done", r.State)
	}
	if r.Project == "" {
		return Record{}, errors.New("record: a step needs a project")
	}
	if r.Session == "" || r.Seat == "" {
		return Record{}, errors.New("record: a step needs its provenance: session and seat")
	}

	id, err := uuidV7()
	if err != nil {
		return Record{}, err
	}

	put := PutRequest{
		ID: id, Kind: KindProgress, Project: r.Project, Body: r.Note,
		Fields:  map[string]string{"state": r.State, "item": r.Item},
		Session: r.Session, Seat: r.Seat, Epoch: r.Epoch,
	}
	fields, err := json.Marshal(put.Fields)
	if err != nil {
		return Record{}, fmt.Errorf("record: encoding step fields: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, fmt.Errorf("record: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// THE ITEM MUST EXIST, AND THE CHECK IS INSIDE THE TRANSACTION.
	//
	// Outside it, an item deleted between the check and the write leaves the
	// orphan this function exists to prevent - the same reasoning the
	// compare-and-swap in Put is built on, and one concurrency model rather
	// than two.
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM heads WHERE id = ?`, r.Item).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, &NotFoundError{ID: r.Item}
		}
		return Record{}, fmt.Errorf("record: looking up item %s: %w", r.Item, err)
	}

	stamped := Provenance{Session: r.Session, Seat: r.Seat, Epoch: r.Epoch, CreatedAt: now().UTC()}
	if err := writeVersion(ctx, tx, put, 1, string(fields), stamped); err != nil {
		return Record{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO links (src, type, dst) VALUES (?, ?, ?)`,
		id, LinkPartOf, r.Item,
	); err != nil {
		return Record{}, fmt.Errorf("record: attaching step %s to %s: %w", id, r.Item, err)
	}

	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("record: committing step on %s: %w", r.Item, err)
	}
	return Record{
		ID: id, Version: 1, Kind: KindProgress, Project: r.Project,
		Body: r.Note, Fields: put.Fields, Prov: stamped,
	}, nil
}

// Stream returns every step on one item, OLDEST FIRST.
//
// Ordered by the daemon's clock and then by id. The id is the tie-breaker and
// it is a real one rather than a formality: tests freeze the clock, so every
// step in a test shares a created_at, and UUIDv7 is monotonic within a process.
// That ordering property is the whole reason section 39 chose v7 over v4, and
// this is the first place it is load-bearing rather than decorative.
//
// ⛔ IT ORDERS BY id ALONE, AND created_at IS NOT A TIE-BREAK BESIDE IT.
// This read `ORDER BY r.created_at, r.id`, which was correct - id caught every
// tie - and contradicted lateststeps.go in print, where the same question is
// argued the other way: the daemon clock has millisecond resolution and tests
// freeze it outright, so created_at is not usable as an ordering and MAX(id) is
// both the order and the tie-break. Two files asserting opposite things about
// one column is the drift section 39 exists to catch, and a later reader
// believes whichever they open first. One argument now governs both.
func (s *Store) Stream(ctx context.Context, item string) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.version, r.kind, r.project, r.body, r.fields,
			r.session, r.seat, r.epoch, r.created_at
		 FROM links l JOIN records r ON r.id = l.src
		 WHERE l.dst = ? AND l.type = ? AND r.kind = ?
		 ORDER BY r.id`, item, LinkPartOf, KindProgress)
	if err != nil {
		return nil, fmt.Errorf("record: reading the stream of %s: %w", item, err)
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
