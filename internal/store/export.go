package store

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Export and import (plan/48 slice 5, R19, R21-R23, R35, D8).
//
// An export is one file per collection, <collection>.jsonl, holding one line
// per document sorted by id (R35), so a one-row change is a one-line diff. A
// line carries the version and the write time as well as the document, which
// is what lets an import put back exactly what was exported: a caller holding
// version 7 before the export still holds version 7 after the import.
//
// This file writes and reads the files; committing them to git is the
// daemon's, which owns the exports repository.

// ExportSuffix ends every collection file of an export.
const ExportSuffix = ".jsonl"

// maxLine bounds one line of an export read back. A stored document is at
// most MaxDocument compacted; the rest is room for the envelope and for a
// file edited by hand with some whitespace in it.
const maxLine = 2*MaxDocument + 4096

// line is one document in an export. The field order is the order written.
type line struct {
	ID        string          `json:"id"`
	Version   uint64          `json:"version"`
	UpdatedNs int64           `json:"updated_ns"`
	Doc       json.RawMessage `json:"doc"`
}

// NotFoundError is a collection that was named and is not there: not in the
// store on an export, no file on an import. Have lists what is.
type NotFoundError struct {
	What, Name string
	Have       []string
}

func (e *NotFoundError) Error() string {
	have := "none"
	if len(e.Have) > 0 {
		have = strings.Join(e.Have, ", ")
	}
	return fmt.Sprintf("store: %s %q does not exist; there is: %s", e.What, e.Name, have)
}

// LineError is a line of an export that cannot be imported. Nothing was
// written: an import is checked whole before the store is touched.
type LineError struct {
	File string
	Line int
	Err  error
}

func (e *LineError) Error() string {
	return fmt.Sprintf("store: %s line %d cannot be imported, and nothing was: %v", e.File, e.Line, e.Err)
}

func (e *LineError) Unwrap() error { return e.Err }

// Exported is one collection written by an export.
type Exported struct {
	Collection string
	Documents  int
	Bytes      int64
}

// ExportResult is what an export wrote, and the files of collections that no
// longer exist which a whole export removed.
type ExportResult struct {
	Collections []Exported
	Removed     []string
}

// Export writes the named collections, or every collection when none is
// named, into dir, one file each. All of them are read from ONE read
// transaction, so an export is a single moment of the store even while
// programs keep writing.
//
// ⛔ A WHOLE EXPORT REMOVES THE FILES OF COLLECTIONS THE STORE NO LONGER HAS,
// so the directory is the store and not the store plus its history; git keeps
// the history. A named export touches only the files it names.
func (s *Store) Export(ctx context.Context, dir string, collections []string) (ExportResult, error) {
	for _, c := range collections {
		if err := CheckName("collection", c); err != nil {
			return ExportResult{}, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ExportResult{}, fmt.Errorf("store: creating %s: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ExportResult{}, fmt.Errorf("store: opening %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	// A connection of its own, so BEGIN is a plain deferred one: the pool's
	// transactions take the write lock at BEGIN, and an export must not stop
	// every writer while it reads. Under WAL a read transaction sees one
	// snapshot until it ends.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return ExportResult{}, err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return ExportResult{}, fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK") }()

	have, err := collectionNames(ctx, conn)
	if err != nil {
		return ExportResult{}, err
	}
	whole := len(collections) == 0
	if whole {
		collections = have
	}
	for _, c := range collections {
		if !slices.Contains(have, c) {
			return ExportResult{}, &NotFoundError{What: "collection", Name: c, Have: have}
		}
	}

	var res ExportResult
	for _, c := range dedupe(collections) {
		e, err := exportOne(ctx, conn, root, c)
		if err != nil {
			return ExportResult{}, err
		}
		res.Collections = append(res.Collections, e)
	}
	if whole {
		if res.Removed, err = removeStale(root, collections); err != nil {
			return ExportResult{}, err
		}
	}
	return res, nil
}

func collectionNames(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT DISTINCT collection FROM docs ORDER BY collection`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func dedupe(names []string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return slices.Compact(out)
}

// exportOne writes one collection to a temporary file and renames it over the
// old one, so a failure half way leaves the previous export whole.
func exportOne(ctx context.Context, conn *sql.Conn, root *os.Root, collection string) (Exported, error) {
	name := collection + ExportSuffix
	tmp := "." + name + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Exported{}, fmt.Errorf("store: writing %s: %w", tmp, err)
	}
	defer func() { _ = root.Remove(tmp) }() // a no-op once renamed

	n, err := writeCollection(ctx, conn, f, collection)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Exported{}, fmt.Errorf("store: exporting %s: %w", collection, err)
	}
	if err := root.Rename(tmp, name); err != nil {
		return Exported{}, fmt.Errorf("store: placing %s: %w", name, err)
	}
	fi, err := root.Stat(name)
	if err != nil {
		return Exported{}, err
	}
	return Exported{Collection: collection, Documents: n, Bytes: fi.Size()}, nil
}

func writeCollection(ctx context.Context, conn *sql.Conn, w io.Writer, collection string) (int, error) {
	rows, err := conn.QueryContext(ctx,
		`SELECT id, version, updated_ns, body FROM docs WHERE collection = ? ORDER BY id`, collection)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	// A document holding "<" is exported as "<": the default escaping would
	// write <, and the import would then not give back the same bytes.
	enc.SetEscapeHTML(false)
	n := 0
	for rows.Next() {
		var l line
		var body string
		if err := rows.Scan(&l.ID, &l.Version, &l.UpdatedNs, &body); err != nil {
			return 0, err
		}
		l.Doc = json.RawMessage(body)
		if err := enc.Encode(l); err != nil { // Encode ends the line
			return 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return n, bw.Flush()
}

// removeStale removes every <collection>.jsonl in the directory that keep
// does not name. Only names an export could have written are touched.
func removeStale(root *os.Root, keep []string) ([]string, error) {
	names, err := exportFiles(root)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, c := range names {
		if slices.Contains(keep, c) {
			continue
		}
		if err := root.Remove(c + ExportSuffix); err != nil {
			return nil, fmt.Errorf("store: removing the export of %s: %w", c, err)
		}
		removed = append(removed, c)
	}
	return removed, nil
}

// exportFiles lists the collections a directory holds an export of, sorted.
func exportFiles(root *os.Root) ([]string, error) {
	d, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = d.Close() }()
	entries, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		c, ok := strings.CutSuffix(e.Name(), ExportSuffix)
		if ok && e.Type().IsRegular() && namePattern.MatchString(c) {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Imported is one collection an import replaced: how many documents it holds
// now, and how many it held before.
type Imported struct {
	Collection string
	Documents  int
	Replaced   int
}

// ImportResult is what an import did. Untouched are the store's collections
// the import did not name, which it leaves exactly as they were.
type ImportResult struct {
	Collections []Imported
	Untouched   []string
}

// Import replaces the named collections, or every collection dir holds an
// export of when none is named, with what the export says (D8).
//
// ⛔ THE ORDER IS THE SAFETY. Every line of every file is read and checked
// first, and a bad one refuses the whole import with nothing written. Then
// snapshot, a path that must not exist yet, gets a copy of the store as it
// was. Only then are the collections replaced, all of them in one
// transaction, so an import is either whole or not at all.
func (s *Store) Import(ctx context.Context, dir string, collections []string, snapshot string) (ImportResult, error) {
	for _, c := range collections {
		if err := CheckName("collection", c); err != nil {
			return ImportResult{}, err
		}
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return ImportResult{}, &NotFoundError{What: "export directory", Name: dir}
	}
	if err != nil {
		return ImportResult{}, fmt.Errorf("store: opening %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()

	have, err := exportFiles(root)
	if err != nil {
		return ImportResult{}, err
	}
	if len(collections) == 0 {
		if len(have) == 0 {
			return ImportResult{}, &NotFoundError{What: "export directory", Name: dir}
		}
		collections = have
	}
	collections = dedupe(collections)
	sets := make(map[string][]Doc, len(collections))
	for _, c := range collections {
		if !slices.Contains(have, c) {
			return ImportResult{}, &NotFoundError{What: "exported collection", Name: c, Have: have}
		}
		docs, err := readExport(root, c)
		if err != nil {
			return ImportResult{}, err
		}
		sets[c] = docs
	}

	if err := s.Snapshot(ctx, snapshot); err != nil {
		return ImportResult{}, err
	}
	res, err := s.replace(ctx, collections, sets)
	if err != nil {
		return ImportResult{}, err
	}
	all, err := s.Collections(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	for _, ci := range all {
		if !slices.Contains(collections, ci.Name) {
			res.Untouched = append(res.Untouched, ci.Name)
		}
	}
	return res, nil
}

// readExport reads and checks one collection's file whole.
func readExport(root *os.Root, collection string) ([]Doc, error) {
	name := collection + ExportSuffix
	f, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("store: reading %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	seen := map[string]bool{}
	var docs []Doc
	n := 0
	for sc.Scan() {
		n++
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		d, err := parseLine(sc.Bytes(), collection)
		if err != nil {
			return nil, &LineError{File: name, Line: n, Err: err}
		}
		if seen[d.ID] {
			return nil, &LineError{File: name, Line: n, Err: &InvalidError{
				What: "id", Value: d.ID, Rule: "each id once in a collection",
			}}
		}
		seen[d.ID] = true
		docs = append(docs, d)
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, &LineError{File: name, Line: n + 1, Err: &InvalidError{
				What: "line", Value: "longer than the limit",
				Rule: fmt.Sprintf("at most %d bytes", maxLine),
			}}
		}
		return nil, fmt.Errorf("store: reading %s: %w", name, err)
	}
	return docs, nil
}

func parseLine(b []byte, collection string) (Doc, error) {
	var l line
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return Doc{}, &InvalidError{
			What: "line", Value: abbreviate(b),
			Rule: `a JSON object {"id","version","updated_ns","doc"}`,
		}
	}
	if dec.More() {
		return Doc{}, &InvalidError{What: "line", Value: abbreviate(b), Rule: "one JSON object per line"}
	}
	if err := CheckName("id", l.ID); err != nil {
		return Doc{}, err
	}
	if l.Version == 0 {
		return Doc{}, &InvalidError{What: "version", Value: "0", Rule: "1 or more: a stored document has been written at least once"}
	}
	body, err := checkOp(Op{Collection: collection, ID: l.ID, Expected: l.Version, Body: l.Doc})
	if err != nil {
		return Doc{}, err
	}
	return Doc{
		Collection: collection, ID: l.ID, Version: l.Version,
		Updated: time.Unix(0, l.UpdatedNs).UTC(), Body: body,
	}, nil
}

// replace empties each collection and writes its documents back with their
// own versions and times, all in one transaction.
func (s *Store) replace(ctx context.Context, collections []string, sets map[string][]Doc) (ImportResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var res ImportResult
	for _, c := range collections {
		r, err := tx.ExecContext(ctx, `DELETE FROM docs WHERE collection = ?`, c)
		if err != nil {
			return ImportResult{}, err
		}
		gone, err := r.RowsAffected()
		if err != nil {
			return ImportResult{}, err
		}
		for _, d := range sets[c] {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO docs (collection, id, version, updated_ns, body) VALUES (?, ?, ?, ?, ?)`,
				c, d.ID, d.Version, d.Updated.UnixNano(), string(d.Body)); err != nil {
				return ImportResult{}, err
			}
		}
		res.Collections = append(res.Collections, Imported{
			Collection: c, Documents: len(sets[c]), Replaced: int(gone),
		})
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("store: commit: %w", err)
	}
	return res, nil
}

// Snapshot writes a consistent copy of the store to dst, which must be an
// absolute path that does not exist yet. It is VACUUM INTO, for the reasons
// record.Store.Snapshot gives, and the path is a bound parameter.
func (s *Store) Snapshot(ctx context.Context, dst string) error {
	if !filepath.IsAbs(dst) {
		return fmt.Errorf("store: a snapshot needs an absolute path, got %q", dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("store: creating %s: %w", filepath.Dir(dst), err)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return fmt.Errorf("store: snapshotting %s into %s: %w", s.path, dst, err)
	}
	// VACUUM INTO creates the file with the process umask; the copy holds
	// the same data as the store, so it is owner-only like the store.
	return os.Chmod(dst, 0o600)
}
