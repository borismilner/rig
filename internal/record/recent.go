package record

import (
	"context"
	"errors"
	"fmt"
)

// The two reads an agent needs of its own work, and neither existed.
//
// ⛔ THE SEAT WAS A COLUMN AND NEVER A PREDICATE, WHICH IS THE DEFECT plan/09
// CALLS THE MOST DIRECTLY FATAL ONE IN ITS SET. `records.seat` and
// `records.session` are written from the daemon's provenance on every version
// and appear in no WHERE clause anywhere in this package, so "show me what I
// wrote" - the first question an agent asks when it comes back - could only be
// answered by reading every record in the store and filtering in Go. A
// capability whose retrieval costs a full read is a write-only pit with a
// query in front of it.
//
// ⛔ AND THE SEAT IS THE KEY RATHER THAN THE SESSION, WHICH IS THE WHOLE POINT
// OF COMING BACK. A session dies with its process; the SEAT is section 16's
// addressable identity and outlives every occupancy of it. Keying the handback
// on the session would answer "what did THIS process write", which after a
// kill is always nothing. Every row still carries its session, so grouping one
// occupancy's work is a fold over the answer rather than a second query.
//
// ⛔ THE PREDICATE IS THE COLUMN AND NOT A FIELD, AND THAT IS A FORGERY
// ARGUMENT RATHER THAN A PERFORMANCE ONE. Copying the seat into the record's
// `fields` map would have let the existing field predicate select on it with
// no new code - and `fields` is caller-supplied, so any `record.put` could
// then write a record attributed to another seat. verbs.proto states the rule
// this would break: session, seat and epoch are ABSENT from every request,
// "not optional, not ignored". Provenance is the daemon's, so the query that
// trusts it must read the daemon's column.

// Recent is a bounded answer with an unbounded count beside it.
//
// ⛔ Total IS NOT len(Records) AND THAT IS THE FIELD'S ONLY REASON TO EXIST.
// The done-when for section 09's A6 is that an agent gets its notes back
// "without reading everything", which means the answer is cut - and an answer
// that is cut without saying so is indistinguishable from a complete one.
// Section 5k's rule, applied to a limit: 10 of 37 is an answer, 10 is a claim
// about the store that happens to be false.
type Recent struct {
	// Records are the newest first, at most Limit of them.
	Records []Record

	// Total is how many match the filter with no limit at all.
	Total uint64
}

// More reports whether the answer was cut.
func (r Recent) More() bool { return r.Total > uint64(len(r.Records)) }

// ErrNoSeat refuses a seat-keyed read with no seat.
//
// ⛔ REFUSED RATHER THAN READ AS "EVERY SEAT", unlike QueryFilter's empty
// fields. The empty-means-every rule there is safe because Put refuses to
// write an empty project or kind, so the wildcard collides with no stored
// value. It is not safe here: this read exists to answer "MINE", and a filter
// that silently widened to every seat on the machine would hand one agent
// another agent's working notes while looking like a successful narrow read.
var ErrNoSeat = errors.New(
	"record: a seat-keyed read needs the seat, and an empty one is not a wildcard here")

// SeatFilter selects the records one seat is the current author of.
//
// ⛔ "CURRENT AUTHOR" IS THE HEAD VERSION'S SEAT, STATED BECAUSE THE OTHER
// READING IS ALSO DEFENSIBLE. A record superseded by a second seat answers to
// that second seat here and not to the one that created it. For an
// append-only caller - a working note is never superseded - the two readings
// are the same set, and for a superseded record "who wrote what is there now"
// is the question a resuming agent is actually asking.
type SeatFilter struct {
	// Seat is required. See ErrNoSeat.
	Seat string

	// Kind and Project narrow further. Empty means every value, exactly as in
	// QueryFilter: no record can hold an empty one.
	Kind    string
	Project string

	// Limit bounds the rows returned and never the count. It is required and
	// positive, for ErrPageLimit's reason.
	Limit int
}

// FindBySeat answers the most recent records one seat wrote, newest first.
func (s *Store) FindBySeat(ctx context.Context, f SeatFilter) (Recent, error) {
	if f.Seat == "" {
		return Recent{}, ErrNoSeat
	}
	if f.Limit <= 0 {
		return Recent{}, ErrPageLimit
	}

	var rowsQ, countQ string
	args := []any{f.Seat}
	switch {
	case f.Kind == "" && f.Project == "":
		rowsQ, countQ = seatRows, seatCount
	case f.Project == "":
		rowsQ, countQ = seatKindRows, seatKindCount
		args = append(args, f.Kind)
	case f.Kind == "":
		rowsQ, countQ = seatProjectRows, seatProjectCount
		args = append(args, f.Project)
	default:
		rowsQ, countQ = seatKindProjectRows, seatKindProjectCount
		args = append(args, f.Kind, f.Project)
	}
	return s.recent(ctx, rowsQ, countQ, args, f.Limit,
		"records written by seat "+f.Seat)
}

// ErrNoAttachment refuses an attachment read with no record to read around.
var ErrNoAttachment = errors.New(
	"record: an attachment read needs the id the records are attached to")

// AttachedFilter selects the records attached to one record by one edge type.
//
// THIS IS THE OTHER HALF OF A2 - section 09's "a note associates to anything"
// read backwards. Refs already answers "what points at this record", and it
// answers with a REFERENCE: an id, a kind, a title and a depth. A caller that
// wants the prose then pays one record.get per row, which is the N+1 section 9
// calls a context cost. This answers with the records themselves.
type AttachedFilter struct {
	// To is the record they are attached to. Required.
	To string

	// Type is the edge type, from the closed set. Required: an untyped
	// traversal would mix a note attached to an item with the item's own
	// progress steps, which are attached by the same edge.
	Type string

	// Kind narrows to one kind of attached record. Empty means every kind.
	Kind string

	// Limit bounds the rows and never the count.
	Limit int
}

// FindAttachedTo answers the most recent records attached to one record,
// newest first.
func (s *Store) FindAttachedTo(ctx context.Context, f AttachedFilter) (Recent, error) {
	if f.To == "" {
		return Recent{}, ErrNoAttachment
	}
	if !linkTypes[f.Type] {
		return Recent{}, fmt.Errorf("record: %q is not a link type; section 39 names %s", f.Type, knownLinkTypes())
	}
	if f.Limit <= 0 {
		return Recent{}, ErrPageLimit
	}

	rowsQ, countQ := attachedRows, attachedCount
	args := []any{f.To, f.Type}
	if f.Kind != "" {
		rowsQ, countQ = attachedKindRows, attachedKindCount
		args = append(args, f.Kind)
	}
	return s.recent(ctx, rowsQ, countQ, args, f.Limit,
		f.Type+" records attached to "+f.To)
}

// recent runs one shape's two queries: the bounded rows and the unbounded
// count.
//
// ⛔ THE COUNT RUNS EVEN WHEN THE PAGE IS SHORT, AND SPENDING THAT QUERY IS
// THE POINT. Inferring "there are no more" from a short page is the inference
// Total exists to replace: it is right for the common case and wrong exactly
// when a row was retracted between the two reads, which is the case a reader
// cannot detect.
func (s *Store) recent(
	ctx context.Context, rowsQ, countQ string, args []any, limit int, what string,
) (Recent, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return Recent{}, fmt.Errorf("record: counting %s: %w", what, err)
	}
	count, err := fromColumn("count", total)
	if err != nil {
		return Recent{}, err
	}

	rows, err := s.db.QueryContext(ctx, rowsQ, append(append([]any{}, args...), limit)...)
	if err != nil {
		return Recent{}, fmt.Errorf("record: reading %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Record, 0, limit)
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return Recent{}, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return Recent{}, err
	}
	return Recent{Records: out, Total: count}, nil
}

// The shapes, as constants, for the reason store.go's are: nothing a caller
// supplies is ever concatenated into SQL, and a new predicate is a new
// constant rather than a `q +=` the next person extends with a value.
//
// ⛔ NEWEST FIRST, BY THE STAMP AND THEN BY THE ID. The id alone would very
// nearly do it - UUIDv7 is time-ordered - but "very nearly" is the wrong
// property for the ordering a resuming agent reads its own work in, and two
// records minted in the same millisecond by two processes order by the
// generator's sequence rather than by when they were written. created_at is
// the daemon's stamp and is what the answer claims to be ordered by; the id
// breaks the tie so the order is total and a page boundary cannot repeat or
// skip a row.
const (
	recentOrder = ` ORDER BY r.created_at DESC, r.id DESC LIMIT ?`

	// countSelect mirrors findSelect exactly, including the retraction
	// predicate, so a count can never disagree with the rows it is beside.
	countSelect = `SELECT COUNT(*)
		 FROM records r JOIN heads h ON h.id = r.id AND h.version = r.version
		 WHERE NOT EXISTS (SELECT 1 FROM retractions x WHERE x.id = r.id)`

	// ⛔ THE SEAT PREDICATE IS NOT INDEXED AND THE COST IS STATED RATHER THAN
	// HIDDEN, exactly as store.go states it for the field predicate. The
	// schema indexes (project, kind) and (dst, type) and nothing on `seat`, so
	// a seat read is a scan of the head rows. At rig's own store - 1,214
	// records after the whole plan was seeded, measured by the paging seat on
	// 2026-09-24 - that is nothing. If it ever becomes hot the answer is an
	// index on (seat, created_at) behind a schema bump, which is a migration
	// and is deliberately not spent here.
	bySeat    = ` AND r.seat = ?`
	byKind    = ` AND r.kind = ?`
	byProject = ` AND r.project = ?`

	seatRows  = findSelect + bySeat + recentOrder
	seatCount = countSelect + bySeat

	seatKindRows  = findSelect + bySeat + byKind + recentOrder
	seatKindCount = countSelect + bySeat + byKind

	seatProjectRows  = findSelect + bySeat + byProject + recentOrder
	seatProjectCount = countSelect + bySeat + byProject

	seatKindProjectRows  = findSelect + bySeat + byKind + byProject + recentOrder
	seatKindProjectCount = countSelect + bySeat + byKind + byProject

	// THE ATTACHMENT READ IS AN EXISTS SUBQUERY AND NOT A JOIN, for the reason
	// store.go gives about json_each: a join against `links` multiplies the
	// row by every matching edge and then needs a DISTINCT to put it back. One
	// row in, at most one row out, whatever the graph holds. It uses
	// links_by_dst (dst, type), which is the index the schema added for
	// record.refs.
	attachedTo = ` AND EXISTS (SELECT 1 FROM links l
			WHERE l.src = r.id AND l.dst = ? AND l.type = ?)`

	attachedRows  = findSelect + attachedTo + recentOrder
	attachedCount = countSelect + attachedTo

	attachedKindRows  = findSelect + attachedTo + byKind + recentOrder
	attachedKindCount = countSelect + attachedTo + byKind
)
