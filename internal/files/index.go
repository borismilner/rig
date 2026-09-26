package files

// The index over the free files, plan/48 R25 and R26: a writer indexes what it
// wrote with a title, a one-line summary and tags, rig indexes the file's text
// beside them, and a search answers a snippet and never a body.
//
// THE INDEX IS NOT IN THE AREA. It is an SQLite file in rig's internal area,
// so nothing a program writes into files/ can edit it, and git never carries
// it. FTS5 for the same reasons as section 40's lessons: it is compiled into
// the SQLite rig already links.
//
// ⛔ EVERY PATH IS READ THROUGH os.Root, so a caller's path and any symlink on
// the way resolve inside the area or are refused. A path is checked lexically
// first (no "..", not absolute, not .git) so the refusal says why.

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "modernc.org/sqlite" // the engine

	"github.com/borismilner/rig/internal/fts"
)

// Bounds on an entry. A summary is the line a search shows, so it is one line.
const (
	MaxTitle       = 200
	MaxSummary     = 400
	MaxTags        = 16
	MaxTag         = 64
	MaxPath        = 1024
	MaxIndexedText = 1 << 20
	DefaultHits    = 5
	MaxHits        = 20
	DefaultPending = 100
	MaxPending     = 1000
	indexSchema    = 1
)

const indexDDL = `
CREATE TABLE IF NOT EXISTS entries (
	id INTEGER PRIMARY KEY,
	path TEXT NOT NULL UNIQUE,
	title TEXT NOT NULL,
	summary TEXT NOT NULL,
	tags TEXT NOT NULL,
	size INTEGER NOT NULL,
	mtime_ns INTEGER NOT NULL,
	writer TEXT NOT NULL,
	indexed_ns INTEGER NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS entry_text USING fts5(title, summary, tags, body);`

// Index is the index over one area.
type Index struct {
	db   *sql.DB
	root *os.Root
	// mu lets a relayout move files and rewrite entries with no index call
	// between; every other call shares it.
	mu sync.RWMutex
}

// OpenIndex opens, or creates owner-only, the index database at dbPath over
// the area repo keeps.
func OpenIndex(ctx context.Context, dbPath string, repo *Repo) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("files: creating %s: %w", filepath.Dir(dbPath), err)
	}
	f, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("files: creating %s: %w", dbPath, err)
	}
	_ = f.Close()
	root, err := os.OpenRoot(repo.Dir())
	if err != nil {
		return nil, fmt.Errorf("files: opening %s: %w", repo.Dir(), err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+
		"?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("files: opening %s: %w", dbPath, err)
	}
	ix := &Index{db: db, root: root}
	if err := ix.migrate(ctx); err != nil {
		_ = ix.Close()
		return nil, err
	}
	return ix, nil
}

func (ix *Index) migrate(ctx context.Context) error {
	var v int
	if err := ix.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("files: reading the index schema: %w", err)
	}
	switch {
	case v == indexSchema:
		return nil
	case v > indexSchema:
		return fmt.Errorf("files: the index is schema %d and this rig knows %d; a newer rig wrote it", v, indexSchema)
	}
	if _, err := ix.db.ExecContext(ctx, indexDDL); err != nil {
		return fmt.Errorf("files: creating the index: %w", err)
	}
	if _, err := ix.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", indexSchema)); err != nil {
		return fmt.Errorf("files: stamping the index schema: %w", err)
	}
	return nil
}

// Close closes the index.
func (ix *Index) Close() error {
	return errors.Join(ix.db.Close(), ix.root.Close())
}

// InvalidError is a request the index refuses on its face.
type InvalidError struct{ Why string }

func (e *InvalidError) Error() string { return "files: " + e.Why }

func invalid(format string, a ...any) error { return &InvalidError{Why: fmt.Sprintf(format, a...)} }

// CleanPath is a caller's path as the index keys it: relative to the area,
// slash-separated, with no "..", and not inside .git. "" is refused.
func CleanPath(p string) (string, error) {
	switch {
	case p == "":
		return "", invalid("a path is needed: where the file is under the free-files root")
	case len(p) > MaxPath:
		return "", invalid("a path is at most %d bytes, got %d", MaxPath, len(p))
	case strings.ContainsRune(p, 0):
		return "", invalid("a path holds no NUL byte")
	case filepath.IsAbs(p):
		return "", invalid("path %q is absolute; give it relative to the free-files root", p)
	}
	for _, el := range strings.Split(filepath.ToSlash(p), "/") {
		if el == ".." {
			return "", invalid("path %q climbs with \"..\"; a path stays under the free-files root", p)
		}
	}
	c := path.Clean(filepath.ToSlash(p))
	if c == "." {
		return "", invalid("path %q is the root itself, not a file", p)
	}
	if c == ".git" || strings.HasPrefix(c, ".git/") {
		return "", invalid("path %q is inside .git, which is rig's and not a file to index", p)
	}
	return c, nil
}

// cleanUnder is an optional prefix: "" means the whole area.
func cleanUnder(under string) (string, error) {
	if under == "" {
		return "", nil
	}
	return CleanPath(under)
}

// Request indexes one file. Writer is who indexes it, set by the daemon from
// the connection, never read off a request.
type Request struct {
	Path, Title, Summary string
	Tags                 []string
	Writer               string
}

// Entry is what indexing did.
type Entry struct {
	Path string
	// Removed means the file is gone, so its entry was dropped.
	Removed bool
	// TextBytes is how much of the file's text is searchable; 0 for a binary.
	TextBytes int
	Binary    bool
	// Truncated means only the first MaxIndexedText bytes are searchable.
	Truncated bool
}

func checkRequest(r Request) error {
	switch {
	case strings.TrimSpace(r.Title) == "":
		return invalid("an entry needs a title")
	case strings.TrimSpace(r.Summary) == "":
		return invalid("an entry needs a one-line summary; it is what a search shows")
	case strings.ContainsAny(r.Summary, "\n\r"):
		return invalid("a summary is one line")
	case len(r.Title) > MaxTitle:
		return invalid("a title is at most %d bytes, got %d", MaxTitle, len(r.Title))
	case len(r.Summary) > MaxSummary:
		return invalid("a summary is at most %d bytes, got %d", MaxSummary, len(r.Summary))
	case len(r.Tags) > MaxTags:
		return invalid("an entry has at most %d tags, got %d", MaxTags, len(r.Tags))
	case r.Writer == "":
		return invalid("an entry needs its writer")
	}
	for _, t := range r.Tags {
		if t == "" || len(t) > MaxTag || strings.IndexFunc(t, unicode.IsSpace) >= 0 {
			return invalid("tag %q is not a tag: one word, 1 to %d bytes", t, MaxTag)
		}
	}
	return nil
}

// Put indexes the file at r.Path, replacing its earlier entry. A path whose
// file is gone has its entry dropped, so the index follows the area.
func (ix *Index) Put(ctx context.Context, r Request) (Entry, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	rel, err := CleanPath(r.Path)
	if err != nil {
		return Entry{}, err
	}
	fi, err := ix.root.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		if _, err := ix.drop(ctx, rel); err != nil {
			return Entry{}, err
		}
		return Entry{Path: rel, Removed: true}, nil
	}
	if err != nil {
		return Entry{}, invalid("path %q cannot be read: %v", rel, trimRoot(err))
	}
	if err := checkRequest(r); err != nil {
		return Entry{}, err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return Entry{}, invalid("path %q is a symlink; index the file it points at", rel)
	case !fi.Mode().IsRegular():
		return Entry{}, invalid("path %q is not a regular file", rel)
	}
	body, size, mtime, ent, err := ix.read(rel)
	if err != nil {
		return Entry{}, err
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return Entry{}, fmt.Errorf("files: indexing %s: %w", rel, err)
	}
	defer func() { _ = tx.Rollback() }()
	tags := strings.Join(r.Tags, " ")
	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO entries (path, title, summary, tags, size, mtime_ns, writer, indexed_ns)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path) DO UPDATE SET title = excluded.title, summary = excluded.summary,
		   tags = excluded.tags, size = excluded.size, mtime_ns = excluded.mtime_ns,
		   writer = excluded.writer, indexed_ns = excluded.indexed_ns
		 RETURNING id`,
		rel, r.Title, r.Summary, tags, size, mtime, r.Writer, time.Now().UnixNano()).Scan(&id); err != nil {
		return Entry{}, fmt.Errorf("files: indexing %s: %w", rel, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entry_text WHERE rowid = ?`, id); err != nil {
		return Entry{}, fmt.Errorf("files: indexing %s: %w", rel, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entry_text (rowid, title, summary, tags, body) VALUES (?, ?, ?, ?, ?)`,
		id, r.Title, r.Summary, tags, body); err != nil {
		return Entry{}, fmt.Errorf("files: indexing %s: %w", rel, err)
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, fmt.Errorf("files: indexing %s: %w", rel, err)
	}
	ent.Path = rel
	return ent, nil
}

// read is a file's searchable text and the size and mtime the entry keeps,
// all from one open file so they describe the same bytes.
func (ix *Index) read(rel string) (body string, size, mtime int64, ent Entry, err error) {
	f, err := ix.root.Open(rel)
	if err != nil {
		return "", 0, 0, Entry{}, invalid("path %q cannot be read: %v", rel, trimRoot(err))
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return "", 0, 0, Entry{}, fmt.Errorf("files: reading %s: %w", rel, err)
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxIndexedText+1))
	if err != nil {
		return "", 0, 0, Entry{}, fmt.Errorf("files: reading %s: %w", rel, err)
	}
	size, mtime = fi.Size(), fi.ModTime().UnixNano()
	if bytes.IndexByte(b[:min(len(b), sniff)], 0) >= 0 {
		// A binary is found by its title, summary and tags alone.
		return "", size, mtime, Entry{Binary: true}, nil
	}
	if len(b) > MaxIndexedText {
		b, ent.Truncated = b[:MaxIndexedText], true
	}
	// The wire carries snippets as proto strings, which must be UTF-8.
	body = strings.ToValidUTF8(string(b), "�")
	ent.TextBytes = len(body)
	return body, size, mtime, ent, nil
}

// trimRoot keeps an os.Root error's reason and drops its absolute path, which
// a caller outside the machine has no use for.
func trimRoot(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

func (ix *Index) drop(ctx context.Context, rel string) (bool, error) {
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("files: dropping %s: %w", rel, err)
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	err = tx.QueryRowContext(ctx, `DELETE FROM entries WHERE path = ? RETURNING id`, rel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("files: dropping %s: %w", rel, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entry_text WHERE rowid = ?`, id); err != nil {
		return false, fmt.Errorf("files: dropping %s: %w", rel, err)
	}
	return true, tx.Commit()
}

// Hit is one search answer: enough to decide whether to open the file, and
// never its body.
type Hit struct {
	Path, Title, Summary, Snippet string
	Score                         float64
}

// Search answers the best-matching entries, best first. under narrows it to a
// directory; limit 0 means DefaultHits, above MaxHits is refused.
func (ix *Index) Search(ctx context.Context, query, under string, limit int) ([]Hit, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if limit == 0 {
		limit = DefaultHits
	}
	if limit < 0 || limit > MaxHits {
		return nil, invalid("a search answers 1 to %d hits, not %d", MaxHits, limit)
	}
	u, err := cleanUnder(under)
	if err != nil {
		return nil, err
	}
	expr, err := fts.Query(query)
	if err != nil {
		return nil, invalid("%v", err)
	}
	// A title outweighs a summary and tags, which outweigh the body.
	rows, err := ix.db.QueryContext(ctx,
		`SELECT e.path, e.title, e.summary, snippet(entry_text, -1, '[', ']', '...', 12),
		        bm25(entry_text, 4.0, 2.0, 2.0, 1.0) AS score
		 FROM entry_text JOIN entries e ON e.id = entry_text.rowid
		 WHERE entry_text MATCH ?
		   AND (? = '' OR e.path = ? OR substr(e.path, 1, length(?) + 1) = ? || '/')
		 ORDER BY score LIMIT ?`, expr, u, u, u, u, limit)
	if err != nil {
		return nil, fmt.Errorf("files: searching the index: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.Path, &h.Title, &h.Summary, &h.Snippet, &h.Score); err != nil {
			return nil, err
		}
		h.Score = -h.Score // bm25 is lower-is-better; the wire says higher-is-better
		// A hit is read one line per field, so the file's line breaks go.
		h.Snippet = strings.Join(strings.Fields(h.Snippet), " ")
		out = append(out, h)
	}
	return out, rows.Err()
}

// Pending states: a file with no entry, a file changed since its entry, and
// an entry whose file is gone.
const (
	StateNew   = "new"
	StateStale = "stale"
	StateGone  = "gone"
)

// Pending is one file the index does not describe as it is.
type Pending struct{ Path, State string }

type stamp struct{ size, mtime int64 }

// Unindexed lists what needs indexing under under, sorted by path, at most
// limit of them (0 means DefaultPending), and how many there are in all.
// Symlinks are not listed: the index refuses them.
func (ix *Index) Unindexed(ctx context.Context, under string, limit int) ([]Pending, int, error) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if limit == 0 {
		limit = DefaultPending
	}
	if limit < 0 || limit > MaxPending {
		return nil, 0, invalid("a listing answers 1 to %d files, not %d", MaxPending, limit)
	}
	u, err := cleanUnder(under)
	if err != nil {
		return nil, 0, err
	}
	known, err := ix.stamps(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	var all []Pending
	start := "."
	if u != "" {
		start = u
	}
	err = fs.WalkDir(ix.root.FS(), start, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err == nil:
		case p == start && errors.Is(err, fs.ErrNotExist):
			return fs.SkipAll
		case p == start:
			return err
		default:
			// A directory a program made unreadable is passed over, not
			// allowed to fail the listing for everyone else.
			return nil
		}
		if d.IsDir() {
			if p == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // vanished between the listing and the stat
		}
		s, ok := known[p]
		delete(known, p)
		switch {
		case !ok:
			all = append(all, Pending{Path: p, State: StateNew})
		case s.size != fi.Size() || s.mtime != fi.ModTime().UnixNano():
			all = append(all, Pending{Path: p, State: StateStale})
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, 0, fmt.Errorf("files: listing the area: %w", trimRoot(err))
	}
	for p := range known {
		all = append(all, Pending{Path: p, State: StateGone})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })
	return all[:min(len(all), limit)], len(all), nil
}

func (ix *Index) stamps(ctx context.Context, u string) (map[string]stamp, error) {
	rows, err := ix.db.QueryContext(ctx,
		`SELECT path, size, mtime_ns FROM entries
		 WHERE ? = '' OR path = ? OR substr(path, 1, length(?) + 1) = ? || '/'`, u, u, u, u)
	if err != nil {
		return nil, fmt.Errorf("files: reading the index: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]stamp{}
	for rows.Next() {
		var p string
		var s stamp
		if err := rows.Scan(&p, &s.size, &s.mtime); err != nil {
			return nil, err
		}
		out[p] = s
	}
	return out, rows.Err()
}
