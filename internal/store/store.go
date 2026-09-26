// Package store is the database behind the program-facing store.* verbs
// (plan/48, the P9 build specification, slice 2).
//
// A program stores and fetches JSON documents through rig and never names the
// engine (R1-R3). This package is the one place that knows it is SQLite: one
// file per program under the estate's internal area, so a program's data is a
// file of its own to back up, export or drop, and no query can reach across
// programs because no connection spans two files (D2).
//
// ⛔ NO CALLER STRING REACHES SQL TEXT (D4, and plan/38's safe-code rule).
// Collection names and ids are checked against one pattern; a field is checked
// against a dotted-identifier pattern AND is then bound as a parameter to
// json_extract, never spliced. Every statement text in this file is a constant
// or is assembled only from constant fragments.
package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the engine, which no caller of this package sees
)

// SchemaVersion is the layout this build writes, stamped in PRAGMA user_version.
const SchemaVersion = 1

// MaxDocument is the largest body one put accepts. A daemon frame is capped at
// 1 MiB (B116), and a get of several documents must still fit one.
const MaxDocument = 256 << 10

// DefaultLimit and MaxLimit bound one query's answer for the same reason.
const (
	DefaultLimit = 500
	MaxLimit     = 5000
)

// MaxOps bounds one transaction, and MaxTerms each of a query's conditions,
// orderings and fields, so one call's statement stays far inside sqlite's
// limit on bound parameters and one caller cannot hold the write lock long.
const (
	MaxOps   = 500
	MaxTerms = 32
)

// steps are the forward migrations, index i taking the store from version i
// to i+1. Each runs inside the one transaction that also stamps the version.
var steps = []string{
	`CREATE TABLE docs (
		collection TEXT    NOT NULL,
		id         TEXT    NOT NULL,
		version    INTEGER NOT NULL,
		updated_ns INTEGER NOT NULL,
		body       TEXT    NOT NULL,
		PRIMARY KEY (collection, id)
	) WITHOUT ROWID`,
}

var (
	namePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	fieldPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}(\.[A-Za-z_][A-Za-z0-9_]{0,63}){0,7}$`)
)

// Store is one program's database.
type Store struct {
	db        *sql.DB
	path      string
	ephemeral bool
}

// Doc is one stored document.
type Doc struct {
	Collection string
	ID         string
	Version    uint64
	Updated    time.Time
	Body       json.RawMessage
}

// Open opens (creating if needed) the database of one program under dir,
// normally the estate's internal area joined with "programs".
//
// ephemeral marks the store of an unnamed estate, so every surface can say the
// data will not outlive it, as record.Store.Ephemeral does.
func Open(ctx context.Context, dir, program string, ephemeral bool) (*Store, error) {
	if err := CheckName("program", program); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, program+".db")
	// Created owner-only before sqlite sees it: sqlite would create it 0644
	// under the usual umask, and its -wal and -shm files copy the mode of the
	// database. An empty file is a valid empty database.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: creating %s: %w", path, err)
	}
	_ = f.Close()

	// The same three settings as the record store, for the same reasons
	// (internal/record/store.go): the write lock at BEGIN so compare-and-swap
	// reads and writes in one transaction, WAL so readers never wait on a
	// writer, and a busy timeout so a concurrent writer waits rather than
	// failing with an error a caller cannot tell from a conflict.
	db, err := sql.Open("sqlite", "file:"+path+
		"?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("store: opening %s: %w", path, err)
	}
	s := &Store{db: db, path: path, ephemeral: ephemeral}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Path is the database file, for backup and for rig's own reports. It is never
// handed to a program.
func (s *Store) Path() string { return s.path }

// Ephemeral says whether this store vanishes with its estate.
func (s *Store) Ephemeral() bool { return s.ephemeral }

// FutureSchemaError means the file was written by a newer rigd. It refuses to
// open and does not repair, for the reason record.FutureSchemaError gives: an
// older binary writing into a newer layout loses data and looks healthy.
type FutureSchemaError struct {
	Path  string
	Found uint32
	Known uint32
}

func (e *FutureSchemaError) Error() string {
	return fmt.Sprintf("store: %s is schema %d and this rigd understands %d: it was "+
		"written by a newer rigd and will not be opened", e.Path, e.Found, e.Known)
}

// migrate applies every missing step and stamps the version in ONE
// transaction, so a crash can never leave data moved and the number not.
func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var found uint32
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&found); err != nil {
		return fmt.Errorf("store: reading user_version: %w", err)
	}
	if found > SchemaVersion {
		return &FutureSchemaError{Path: s.path, Found: found, Known: SchemaVersion}
	}
	for v := found; v < SchemaVersion; v++ {
		if _, err := tx.ExecContext(ctx, steps[v]); err != nil {
			return fmt.Errorf("store: migrating %s to %d: %w", s.path, v+1, err)
		}
	}
	// PRAGMA takes no parameters; the value is this package's own constant.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return fmt.Errorf("store: stamping %s: %w", s.path, err)
	}
	return tx.Commit()
}

// CheckName accepts a program, collection or document id. It is the pattern
// that lets an export write <collection>.jsonl without a path ever escaping
// the program's directory.
func CheckName(what, name string) error {
	if !namePattern.MatchString(name) {
		return &InvalidError{
			What: what, Value: name,
			Rule: "lowercase letters, digits, '.', '_' or '-', starting with a letter or digit, at most 64",
		}
	}
	return nil
}

func checkField(field string) error {
	if !fieldPattern.MatchString(field) {
		return &InvalidError{
			What: "field", Value: field,
			Rule: "identifiers joined by '.', each a letter or '_' then letters, digits or '_'",
		}
	}
	return nil
}

// InvalidError is an argument refused before it reached the database.
type InvalidError struct {
	What, Value, Rule string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("store: %s %q is not allowed: %s", e.What, e.Value, e.Rule)
}

// ConflictError means the expected version was not the current one.
// Current is 0 when the document does not exist.
type ConflictError struct {
	Collection, ID string
	Expected       uint64
	Current        uint64
}

func (e *ConflictError) Error() string {
	switch {
	case e.Expected == 0:
		return fmt.Sprintf("store: %s/%s already exists at version %d: version 0 creates, "+
			"and this id is taken", e.Collection, e.ID, e.Current)
	case e.Current == 0:
		return fmt.Sprintf("store: %s/%s does not exist, and the call expected version %d",
			e.Collection, e.ID, e.Expected)
	default:
		return fmt.Sprintf("store: %s/%s is at version %d and the call expected %d: "+
			"somebody wrote it since you read it", e.Collection, e.ID, e.Current, e.Expected)
	}
}

// OpError says which operation of a transaction failed. Nothing was written.
type OpError struct {
	Index int
	Err   error
}

func (e *OpError) Error() string {
	return fmt.Sprintf("store: operation %d failed and nothing was written: %v", e.Index, e.Err)
}

func (e *OpError) Unwrap() error { return e.Err }

// Op is one step of a transaction: a put when Body is set, else a delete.
type Op struct {
	Collection string
	ID         string
	Expected   uint64
	Body       json.RawMessage
}

// Put writes one document. expected is the version the caller read, or 0 to
// create. It answers the new version.
func (s *Store) Put(ctx context.Context, collection, id string, expected uint64, body json.RawMessage) (uint64, error) {
	// A nil body is a delete to Transact, and a put must never become one:
	// an empty body is refused as not an object instead.
	if body == nil {
		body = json.RawMessage{}
	}
	vs, err := s.Transact(ctx, []Op{{Collection: collection, ID: id, Expected: expected, Body: body}})
	if err != nil {
		var oe *OpError
		if errors.As(err, &oe) {
			return 0, oe.Err
		}
		return 0, err
	}
	return vs[0], nil
}

// Delete removes one document at the version the caller read.
func (s *Store) Delete(ctx context.Context, collection, id string, expected uint64) error {
	_, err := s.Transact(ctx, []Op{{Collection: collection, ID: id, Expected: expected}})
	var oe *OpError
	if errors.As(err, &oe) {
		return oe.Err
	}
	return err
}

// Transact applies every operation or none. It answers each put's new version,
// and 0 for each delete.
func (s *Store) Transact(ctx context.Context, ops []Op) ([]uint64, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	if len(ops) > MaxOps {
		return nil, &InvalidError{
			What: "ops", Value: strconv.Itoa(len(ops)),
			Rule: fmt.Sprintf("at most %d in one transaction", MaxOps),
		}
	}
	prepared := make([]json.RawMessage, len(ops))
	for i, op := range ops {
		b, err := checkOp(op)
		if err != nil {
			return nil, &OpError{Index: i, Err: err}
		}
		prepared[i] = b
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UnixNano()
	out := make([]uint64, len(ops))
	for i, op := range ops {
		v, err := apply(ctx, tx, op, prepared[i], now)
		if err != nil {
			return nil, &OpError{Index: i, Err: err}
		}
		out[i] = v
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit: %w", err)
	}
	return out, nil
}

// checkOp validates an operation before any transaction begins, and answers a
// put's body compacted, so the stored bytes (and an export of them) do not
// depend on the caller's whitespace.
func checkOp(op Op) (json.RawMessage, error) {
	if err := CheckName("collection", op.Collection); err != nil {
		return nil, err
	}
	if err := CheckName("id", op.ID); err != nil {
		return nil, err
	}
	if op.Body == nil {
		if op.Expected == 0 {
			return nil, &InvalidError{
				What: "expected version", Value: "0",
				Rule: "a delete names the version it read",
			}
		}
		return nil, nil
	}
	if len(op.Body) > MaxDocument {
		return nil, &InvalidError{
			What: "document", Value: fmt.Sprintf("%d bytes", len(op.Body)),
			Rule: fmt.Sprintf("at most %d bytes", MaxDocument),
		}
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, op.Body); err != nil || buf.Len() == 0 || buf.Bytes()[0] != '{' {
		return nil, &InvalidError{
			What: "document", Value: abbreviate(op.Body),
			Rule: "a JSON object",
		}
	}
	return buf.Bytes(), nil
}

func apply(ctx context.Context, tx *sql.Tx, op Op, body json.RawMessage, now int64) (uint64, error) {
	var current uint64
	err := tx.QueryRowContext(ctx,
		`SELECT version FROM docs WHERE collection = ? AND id = ?`,
		op.Collection, op.ID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if current != op.Expected {
		return 0, &ConflictError{Collection: op.Collection, ID: op.ID, Expected: op.Expected, Current: current}
	}
	if body == nil {
		_, err := tx.ExecContext(ctx,
			`DELETE FROM docs WHERE collection = ? AND id = ? AND version = ?`,
			op.Collection, op.ID, op.Expected)
		return 0, err
	}
	next := current + 1
	_, err = tx.ExecContext(ctx,
		`INSERT INTO docs (collection, id, version, updated_ns, body) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (collection, id) DO UPDATE SET
		   version = excluded.version, updated_ns = excluded.updated_ns, body = excluded.body
		 WHERE docs.version = ?`,
		op.Collection, op.ID, next, now, string(body), current)
	return next, err
}

// Get answers the documents with these ids, in the order asked, skipping any
// that do not exist. Many ids in one call, because B108 measured one lookup
// per id as the largest avoidable cost.
func (s *Store) Get(ctx context.Context, collection string, ids []string) ([]Doc, error) {
	if err := CheckName("collection", collection); err != nil {
		return nil, err
	}
	if len(ids) > MaxLimit {
		return nil, &InvalidError{
			What: "ids", Value: strconv.Itoa(len(ids)),
			Rule: fmt.Sprintf("at most %d in one call", MaxLimit),
		}
	}
	var out []Doc
	for _, id := range ids {
		if err := CheckName("id", id); err != nil {
			return nil, err
		}
		d, err := scanDoc(s.db.QueryRowContext(ctx,
			`SELECT collection, id, version, updated_ns, body FROM docs WHERE collection = ? AND id = ?`,
			collection, id))
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

type rowScanner interface{ Scan(...any) error }

func scanDoc(r rowScanner) (Doc, error) {
	var d Doc
	var ns int64
	var body string
	if err := r.Scan(&d.Collection, &d.ID, &d.Version, &ns, &body); err != nil {
		return Doc{}, err
	}
	d.Updated = time.Unix(0, ns).UTC()
	d.Body = json.RawMessage(body)
	return d, nil
}

// Operator is a comparison in a query condition.
type Operator int

// The operators a condition may use. Zero is not one of them, so a condition
// built without an operator is refused rather than read as "equals".
const (
	OpUnspecified Operator = iota
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe
)

// sqlOp is the constant SQL for each operator: the only text a condition adds
// to a statement.
var sqlOp = map[Operator]string{OpEq: "=", OpNe: "!=", OpLt: "<", OpLe: "<=", OpGt: ">", OpGe: ">="}

// Cond compares one field of the document with a JSON scalar.
type Cond struct {
	Field string
	Op    Operator
	Value json.RawMessage
}

// Order sorts by one field.
type Order struct {
	Field string
	Desc  bool
}

// Query asks one collection. Conditions are joined by AND.
type Query struct {
	Collection string
	Where      []Cond
	Fields     []string // when set, each answered body carries only these
	Order      []Order  // then by id, always, so the answer is stable
	Limit      int      // 0 is DefaultLimit
	Offset     int      // skips this many matches, to read the page after More
	CountOnly  bool
}

// Result is a query's answer. Total counts every match; More says the limit
// cut the answer short.
type Result struct {
	Docs  []Doc
	Total int
	More  bool
}

// Query runs a query.
func (s *Store) Query(ctx context.Context, q Query) (Result, error) {
	if err := CheckName("collection", q.Collection); err != nil {
		return Result{}, err
	}
	for what, n := range map[string]int{"where": len(q.Where), "order": len(q.Order), "fields": len(q.Fields)} {
		if n > MaxTerms {
			return Result{}, &InvalidError{
				What: what, Value: fmt.Sprintf("%d terms", n),
				Rule: fmt.Sprintf("at most %d", MaxTerms),
			}
		}
	}
	if q.Limit < 0 || q.Limit > MaxLimit {
		return Result{}, &InvalidError{
			What: "limit", Value: strconv.Itoa(q.Limit),
			Rule: fmt.Sprintf("between 1 and %d, or 0 for %d", MaxLimit, DefaultLimit),
		}
	}
	if q.Offset < 0 {
		return Result{}, &InvalidError{What: "offset", Value: strconv.Itoa(q.Offset), Rule: "zero or more"}
	}
	where, args, err := whereClause(q)
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM docs`+where, args...).Scan(&res.Total); err != nil {
		return Result{}, err
	}
	if q.CountOnly {
		return res, nil
	}

	limit := q.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	order, orderArgs, err := orderClause(q.Order)
	if err != nil {
		return Result{}, err
	}
	for _, f := range q.Fields {
		if err := checkField(f); err != nil {
			return Result{}, err
		}
	}
	args = append(args, orderArgs...)
	args = append(args, limit, q.Offset)
	// where and order are joined from constant fragments only: every field
	// path and value a caller sent is a bound parameter in args.
	//nolint:gosec // G202: no caller string reaches the SQL text
	rows, err := s.db.QueryContext(ctx,
		`SELECT collection, id, version, updated_ns, body FROM docs`+where+order+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return Result{}, err
		}
		if len(q.Fields) > 0 {
			if d.Body, err = project(d.Body, q.Fields); err != nil {
				return Result{}, err
			}
		}
		res.Docs = append(res.Docs, d)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	res.More = res.Total > q.Offset+len(res.Docs)
	return res, nil
}

// whereClause builds the WHERE from constant fragments; every caller value,
// the field's JSON path included, is a bound argument.
func whereClause(q Query) (string, []any, error) {
	var b strings.Builder
	b.WriteString(` WHERE collection = ?`)
	args := []any{q.Collection}
	for _, c := range q.Where {
		if err := checkField(c.Field); err != nil {
			return "", nil, err
		}
		op, ok := sqlOp[c.Op]
		if !ok {
			return "", nil, &InvalidError{
				What: "operator", Value: strconv.Itoa(int(c.Op)),
				Rule: "one of eq, ne, lt, le, gt, ge",
			}
		}
		v, isNull, err := scalar(c.Value)
		if err != nil {
			return "", nil, err
		}
		if isNull {
			switch c.Op {
			case OpEq:
				b.WriteString(` AND json_extract(body, ?) IS NULL`)
			case OpNe:
				b.WriteString(` AND json_extract(body, ?) IS NOT NULL`)
			default:
				return "", nil, &InvalidError{
					What: "condition", Value: c.Field,
					Rule: "null compares only with eq or ne",
				}
			}
			args = append(args, "$."+c.Field)
			continue
		}
		b.WriteString(` AND json_extract(body, ?) ` + op + ` ?`)
		args = append(args, "$."+c.Field, v)
	}
	return b.String(), args, nil
}

func orderClause(order []Order) (string, []any, error) {
	var b strings.Builder
	var args []any
	b.WriteString(` ORDER BY `)
	for _, o := range order {
		if err := checkField(o.Field); err != nil {
			return "", nil, err
		}
		b.WriteString(`json_extract(body, ?)`)
		if o.Desc {
			b.WriteString(` DESC`)
		}
		b.WriteString(`, `)
		args = append(args, "$."+o.Field)
	}
	b.WriteString(`id`)
	return b.String(), args, nil
}

// scalar decodes a condition's value into what SQLite compares json_extract's
// answer with: a string, a number, a boolean as 1 or 0 (json_extract answers
// true and false that way), or null.
func scalar(raw json.RawMessage) (any, bool, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, false, &InvalidError{What: "value", Value: abbreviate(raw), Rule: "a JSON scalar"}
	}
	switch x := v.(type) {
	case nil:
		return nil, true, nil
	case string:
		return x, false, nil
	case bool:
		if x {
			return 1, false, nil
		}
		return 0, false, nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, false, nil
		}
		f, err := x.Float64()
		if err != nil {
			return nil, false, &InvalidError{What: "value", Value: x.String(), Rule: "a number SQLite can hold"}
		}
		return f, false, nil
	default:
		return nil, false, &InvalidError{
			What: "value", Value: abbreviate(raw),
			Rule: "a string, number, boolean or null; objects and arrays are not compared",
		}
	}
}

// project keeps only the named fields of a body, nested fields under their
// parents. A field the document does not carry is left out, not written null.
func project(body json.RawMessage, fields []string) (json.RawMessage, error) {
	var src map[string]any
	if err := json.Unmarshal(body, &src); err != nil {
		return nil, fmt.Errorf("store: a stored document is not an object: %w", err)
	}
	dst := map[string]any{}
	for _, f := range fields {
		parts := strings.Split(f, ".")
		v, ok := lookup(src, parts)
		if !ok {
			continue
		}
		put(dst, parts, v)
	}
	return json.Marshal(dst)
}

func lookup(m map[string]any, parts []string) (any, bool) {
	v, ok := m[parts[0]]
	if !ok || len(parts) == 1 {
		return v, ok
	}
	child, isMap := v.(map[string]any)
	if !isMap {
		return nil, false
	}
	return lookup(child, parts[1:])
}

func put(m map[string]any, parts []string, v any) {
	if len(parts) == 1 {
		m[parts[0]] = v
		return
	}
	child, ok := m[parts[0]].(map[string]any)
	if !ok {
		child = map[string]any{}
		m[parts[0]] = child
	}
	put(child, parts[1:], v)
}

// CollectionInfo describes one collection.
type CollectionInfo struct {
	Name  string
	Count int
	Bytes int64
}

// Collections lists every collection this program has written, by name.
func (s *Store) Collections(ctx context.Context) ([]CollectionInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT collection, count(*), sum(length(body)) FROM docs GROUP BY collection ORDER BY collection`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []CollectionInfo
	for rows.Next() {
		var c CollectionInfo
		if err := rows.Scan(&c.Name, &c.Count, &c.Bytes); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// abbreviate shortens a caller's value for an error message, so a refused
// 256 KiB body does not become a 256 KiB error.
func abbreviate(b []byte) string {
	const keep = 60
	if len(b) <= keep {
		return string(b)
	}
	return string(b[:keep]) + "..."
}
