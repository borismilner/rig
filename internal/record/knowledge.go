package record

// Section 40's knowledge-sharing section: a lesson is written ONCE by whoever
// learned it and found by whoever needs it, across projects and sessions,
// and it is NEVER READ WHOLE - an agent searches, reads a line or two per
// hit, and fetches the one lesson that fits.
//
// THE INDEX IS SQLITE FTS5, chosen against section 40's named candidates.
// FTS5 is compiled into the SQLite rig already links, so it costs no
// dependency and no byte of binary (the footprint ruling); bleve would bring
// a second index engine and its dependency tree beside a store that already
// ranks. Semantic search is a separate decision section 40 forbids making by
// default, and it is not made here. Ranking is FTS5's bm25; the hit carries a
// snippet, never the body.
//
// ESTATE-SCOPED, NOT PROJECT-SCOPED, and NOT THE CONTINUITY RECORD: lessons
// are their own table in the estate's store, not records of a project, so a
// lesson learned in one project is found from any other.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// lessonsDDL is the one table. Every column but the searchable four is
// UNINDEXED: stored with the lesson, not tokenised.
const lessonsDDL = `
CREATE VIRTUAL TABLE IF NOT EXISTS lessons USING fts5(
	id UNINDEXED, title, summary, body, tags,
	session UNINDEXED, seat UNINDEXED, epoch UNINDEXED, created_at UNINDEXED
);`

// Bounds on what a lesson may carry. A summary is the line a search shows,
// so it is kept to one; a body is where the detail goes.
const (
	MaxLessonTitle   = 200
	MaxLessonSummary = 400
	MaxLessonBody    = 64 << 10
	MaxLessonTags    = 16
	MaxLessonTag     = 64
	DefaultHits      = 5
	MaxHits          = 20
	maxQueryTerms    = 16
)

// Lesson is one lesson, whole.
type Lesson struct {
	ID, Title, Summary, Body string
	Tags                     []string
	Prov                     Provenance
}

// LessonHit is what a search returns per lesson: enough to decide whether to
// fetch it, and nothing more.
type LessonHit struct {
	ID, Title, Summary string
	// Snippet is the matched passage with the terms marked, about a dozen
	// words.
	Snippet string
	// Score is bm25's, higher is a better match.
	Score float64
}

// LessonRequest writes one lesson. Provenance is the caller's, supplied by
// the daemon, never read off a request.
type LessonRequest struct {
	Title, Summary, Body string
	Tags                 []string
	Session, Seat        string
	Epoch                uint64
}

// AddLesson writes a lesson and returns it with its new id.
func (s *Store) AddLesson(ctx context.Context, r LessonRequest) (Lesson, error) {
	if err := checkLesson(r); err != nil {
		return Lesson{}, err
	}
	id, err := uuidV7()
	if err != nil {
		return Lesson{}, err
	}
	ecol, err := toColumn("epoch", r.Epoch)
	if err != nil {
		return Lesson{}, err
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO lessons (id, title, summary, body, tags, session, seat, epoch, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, r.Title, r.Summary, r.Body, strings.Join(r.Tags, " "),
		r.Session, r.Seat, ecol, now.UnixNano()); err != nil {
		return Lesson{}, fmt.Errorf("record: writing a lesson: %w", err)
	}
	return Lesson{
		ID: id, Title: r.Title, Summary: r.Summary, Body: r.Body, Tags: r.Tags,
		Prov: Provenance{Session: r.Session, Seat: r.Seat, Epoch: r.Epoch, CreatedAt: now},
	}, nil
}

func checkLesson(r LessonRequest) error {
	switch {
	case strings.TrimSpace(r.Title) == "":
		return errors.New("record: a lesson needs a title")
	case strings.TrimSpace(r.Summary) == "":
		return errors.New("record: a lesson needs a one-line summary; it is what a search shows")
	case strings.ContainsAny(r.Summary, "\n\r"):
		return errors.New("record: a lesson's summary is one line; the detail goes in the body")
	case len(r.Title) > MaxLessonTitle:
		return fmt.Errorf("record: a lesson title is at most %d bytes, got %d", MaxLessonTitle, len(r.Title))
	case len(r.Summary) > MaxLessonSummary:
		return fmt.Errorf("record: a lesson summary is at most %d bytes, got %d", MaxLessonSummary, len(r.Summary))
	case len(r.Body) > MaxLessonBody:
		return fmt.Errorf("record: a lesson body is at most %d bytes, got %d", MaxLessonBody, len(r.Body))
	case len(r.Tags) > MaxLessonTags:
		return fmt.Errorf("record: a lesson has at most %d tags, got %d", MaxLessonTags, len(r.Tags))
	case r.Session == "" || r.Seat == "":
		return errors.New("record: a lesson needs its provenance: session and seat")
	}
	for _, t := range r.Tags {
		if t == "" || len(t) > MaxLessonTag || strings.IndexFunc(t, unicode.IsSpace) >= 0 {
			return fmt.Errorf("record: tag %q is not a tag: one word, 1 to %d bytes", t, MaxLessonTag)
		}
	}
	return nil
}

// ftsQuery turns a caller's words into an FTS5 expression that can never be
// read as FTS5 syntax: every word becomes a quoted string with its quotes
// doubled, and the words are OR-ed so bm25 ranks by how many match. The SQL
// around it is parameterised; this is the only place caller text meets the
// query language, and it only ever produces quoted terms.
func ftsQuery(q string) (string, error) {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return "", errors.New("record: a lesson search needs at least one word")
	}
	if len(words) > maxQueryTerms {
		words = words[:maxQueryTerms]
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " OR "), nil
}

// SearchLessons returns the best-matching lessons, best first. limit 0 means
// DefaultHits; above MaxHits is refused rather than clamped.
func (s *Store) SearchLessons(ctx context.Context, query string, limit int) ([]LessonHit, error) {
	if limit == 0 {
		limit = DefaultHits
	}
	if limit < 0 || limit > MaxHits {
		return nil, fmt.Errorf("record: a lesson search returns 1 to %d hits, not %d", MaxHits, limit)
	}
	expr, err := ftsQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, summary, snippet(lessons, 3, '[', ']', '...', 12), bm25(lessons)
		 FROM lessons WHERE lessons MATCH ? ORDER BY rank LIMIT ?`, expr, limit)
	if err != nil {
		return nil, fmt.Errorf("record: searching lessons: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LessonHit
	for rows.Next() {
		var h LessonHit
		if err := rows.Scan(&h.ID, &h.Title, &h.Summary, &h.Snippet, &h.Score); err != nil {
			return nil, err
		}
		h.Score = -h.Score // bm25 is lower-is-better; the wire says higher-is-better
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetLesson returns one lesson whole.
func (s *Store) GetLesson(ctx context.Context, id string) (Lesson, error) {
	var (
		l          Lesson
		tags       string
		epoch, now int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, summary, body, tags, session, seat, epoch, created_at
		 FROM lessons WHERE id = ?`, id).
		Scan(&l.ID, &l.Title, &l.Summary, &l.Body, &tags, &l.Prov.Session, &l.Prov.Seat, &epoch, &now)
	if errors.Is(err, sql.ErrNoRows) {
		return Lesson{}, &NotFoundError{ID: id}
	}
	if err != nil {
		return Lesson{}, fmt.Errorf("record: reading lesson %s: %w", id, err)
	}
	if l.Prov.Epoch, err = fromColumn("epoch", epoch); err != nil {
		return Lesson{}, err
	}
	l.Prov.CreatedAt = time.Unix(0, now).UTC()
	l.Tags = strings.Fields(tags)
	return l, nil
}
