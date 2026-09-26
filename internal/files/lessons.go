package files

// Section 40's lessons as files, plan/48 R24 and R25: a lesson is one file in
// the shared lessons folder, the folder is the source and git keeps it, and
// knowledge.search is the index over that folder, answering a snippet and
// never a body.
//
// A LESSON FILE CARRIES ITS OWN HEADER, so the file alone rebuilds the index
// and a lesson edited by hand is searched as it now reads:
//
//	---
//	id: 0192...
//	title: Retry policy
//	summary: how failed jobs are retried
//	tags: retries scheduler
//	seat: backend-1
//	session: ...
//	epoch: 3
//	created: 2026-09-26T10:00:00Z
//	---
//
//	the body
//
// The id is the file name without ".md", so a lesson's id survives the move
// out of the record store. A file in the folder with no header is still a
// lesson: its writer indexes it with files.index (R26), and its title and
// summary come from that entry.

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// Bounds on a lesson, the record store's own so nothing moved over is refused.
const (
	MaxLessonBody = 64 << 10
	lessonExt     = ".md"
)

// Lesson is one lesson, whole.
type Lesson struct {
	ID, Title, Summary, Body string
	Tags                     []string
	Seat, Session            string
	Epoch                    uint64
	Created                  time.Time
}

// LessonHit is one search answer.
type LessonHit struct {
	ID, Title, Summary, Snippet string
	Score                       float64
}

// NotFoundError is a lesson id with no file.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return "files: no lesson " + e.ID }

func checkLesson(l Lesson) error {
	oneLine := func(s string) bool { return !strings.ContainsAny(s, "\n\r") }
	switch {
	case strings.TrimSpace(l.Title) == "":
		return invalid("a lesson needs a title")
	case strings.TrimSpace(l.Summary) == "":
		return invalid("a lesson needs a one-line summary; it is what a search shows")
	case !oneLine(l.Title) || !oneLine(l.Summary):
		return invalid("a lesson's title and summary are one line each; the detail goes in the body")
	case len(l.Title) > MaxTitle:
		return invalid("a lesson title is at most %d bytes, got %d", MaxTitle, len(l.Title))
	case len(l.Summary) > MaxSummary:
		return invalid("a lesson summary is at most %d bytes, got %d", MaxSummary, len(l.Summary))
	case len(l.Body) > MaxLessonBody:
		return invalid("a lesson body is at most %d bytes, got %d", MaxLessonBody, len(l.Body))
	case len(l.Tags) > MaxTags:
		return invalid("a lesson has at most %d tags, got %d", MaxTags, len(l.Tags))
	case l.Seat == "" || l.Session == "":
		return invalid("a lesson needs its provenance: session and seat")
	case !oneLine(l.Seat) || !oneLine(l.Session):
		return invalid("a lesson's seat and session are one line each")
	case !namePattern.MatchString(l.ID):
		return invalid("lesson id %q is not a name", l.ID)
	}
	for _, t := range l.Tags {
		if t == "" || len(t) > MaxTag || strings.IndexFunc(t, unicode.IsSpace) >= 0 {
			return invalid("tag %q is not a tag: one word, 1 to %d bytes", t, MaxTag)
		}
	}
	return nil
}

func encodeLesson(l Lesson) []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	for _, kv := range [][2]string{
		{"id", l.ID},
		{"title", l.Title},
		{"summary", l.Summary},
		{"tags", strings.Join(l.Tags, " ")},
		{"seat", l.Seat},
		{"session", l.Session},
		{"epoch", strconv.FormatUint(l.Epoch, 10)},
		{"created", l.Created.UTC().Format(time.RFC3339Nano)},
	} {
		b.WriteString(kv[0] + ": " + kv[1] + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(l.Body)
	return b.Bytes()
}

// decodeLesson reads a lesson file's header and body; ok is false for a file
// with no header.
func decodeLesson(id string, raw []byte) (Lesson, bool) {
	rest, found := bytes.CutPrefix(raw, []byte("---\n"))
	if !found {
		return Lesson{}, false
	}
	head, body, found := bytes.Cut(rest, []byte("\n---\n"))
	if !found {
		return Lesson{}, false
	}
	l := Lesson{ID: id, Body: strings.TrimPrefix(string(body), "\n")}
	sc := bufio.NewScanner(bytes.NewReader(head))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ": ")
		if !ok {
			k, v = strings.TrimSuffix(sc.Text(), ":"), ""
		}
		switch k {
		case "title":
			l.Title = v
		case "summary":
			l.Summary = v
		case "tags":
			l.Tags = strings.Fields(v)
		case "seat":
			l.Seat = v
		case "session":
			l.Session = v
		case "epoch":
			l.Epoch, _ = strconv.ParseUint(v, 10, 64)
		case "created":
			l.Created, _ = time.Parse(time.RFC3339Nano, v)
		}
	}
	return l, l.Title != ""
}

func lessonPath(dir, id string) (string, error) {
	if !namePattern.MatchString(id) {
		return "", invalid("lesson id %q is not a name", id)
	}
	return CleanPath(path.Join(dir, id+lessonExt))
}

// AddLesson writes a new lesson into dir, the layout's lessons folder, and
// indexes it. The id is minted here.
func (ix *Index) AddLesson(ctx context.Context, dir string, l Lesson) (Lesson, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Lesson{}, fmt.Errorf("files: minting a lesson id: %w", err)
	}
	l.ID, l.Created = id.String(), time.Now().UTC()
	if _, err := ix.importLesson(ctx, dir, l); err != nil {
		return Lesson{}, err
	}
	return l, nil
}

// ImportLesson writes a lesson that already has its id and provenance, the
// one-time move out of the record store. A lesson whose file exists is left
// alone, so the move can run again.
func (ix *Index) ImportLesson(ctx context.Context, dir string, l Lesson) (bool, error) {
	return ix.importLesson(ctx, dir, l)
}

func (ix *Index) importLesson(ctx context.Context, dir string, l Lesson) (bool, error) {
	if err := checkLesson(l); err != nil {
		return false, err
	}
	rel, err := lessonPath(dir, l.ID)
	if err != nil {
		return false, err
	}
	if err := ix.root.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("files: making %s: %w", dir, trimRoot(err))
	}
	f, err := ix.root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("files: writing %s: %w", rel, trimRoot(err))
	}
	_, werr := f.Write(encodeLesson(l))
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = ix.root.Remove(rel)
		return false, fmt.Errorf("files: writing %s: %w", rel, err)
	}
	if _, err := ix.Put(ctx, Request{
		Path: rel, Title: l.Title, Summary: l.Summary, Tags: l.Tags, Writer: "seat:" + l.Seat,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// GetLesson reads one lesson whole, as its file reads now.
func (ix *Index) GetLesson(ctx context.Context, dir, id string) (Lesson, error) {
	rel, err := lessonPath(dir, id)
	if err != nil {
		return Lesson{}, err
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	f, err := ix.root.Open(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return Lesson{}, &NotFoundError{ID: id}
	}
	if err != nil {
		return Lesson{}, invalid("lesson %s cannot be read: %v", id, trimRoot(err))
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, MaxLessonBody+4<<10))
	if err != nil {
		return Lesson{}, fmt.Errorf("files: reading %s: %w", rel, err)
	}
	raw = bytes.ToValidUTF8(raw, []byte("�"))
	if l, ok := decodeLesson(id, raw); ok {
		return l, nil
	}
	// No header: the writer's index entry names it.
	l := Lesson{ID: id, Title: id, Body: string(raw)}
	var tags, writer string
	err = ix.db.QueryRowContext(ctx, `SELECT title, summary, tags, writer FROM entries WHERE path = ?`, rel).
		Scan(&l.Title, &l.Summary, &tags, &writer)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Lesson{}, fmt.Errorf("files: reading the index: %w", err)
	}
	l.Tags, l.Seat = strings.Fields(tags), strings.TrimPrefix(writer, "seat:")
	return l, nil
}

// SearchLessons answers the best-matching lessons in dir, best first, after
// bringing the index up to date with the folder.
func (ix *Index) SearchLessons(ctx context.Context, dir, query string, limit int) ([]LessonHit, error) {
	if _, err := ix.RefreshLessons(ctx, dir); err != nil {
		return nil, err
	}
	hits, err := ix.Search(ctx, query, dir, limit)
	if err != nil {
		return nil, err
	}
	out := make([]LessonHit, 0, len(hits))
	for _, h := range hits {
		id, ok := lessonID(dir, h.Path)
		if !ok {
			continue // a file in a subfolder or not markdown is not a lesson
		}
		out = append(out, LessonHit{ID: id, Title: h.Title, Summary: h.Summary, Snippet: h.Snippet, Score: h.Score})
	}
	return out, nil
}

func lessonID(dir, rel string) (string, bool) {
	name, ok := strings.CutPrefix(rel, dir+"/")
	if !ok || strings.Contains(name, "/") {
		return "", false
	}
	id, ok := strings.CutSuffix(name, lessonExt)
	return id, ok && namePattern.MatchString(id)
}

// RefreshLessons indexes every lesson file in dir that is new or changed and
// carries a header, and drops the entries of lessons whose file is gone. A
// file with no header is its writer's to index (R26). It answers how many
// entries it wrote or dropped.
func (ix *Index) RefreshLessons(ctx context.Context, dir string) (int, error) {
	pending, _, err := ix.Unindexed(ctx, dir, MaxPending)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range pending {
		id, ok := lessonID(dir, p.Path)
		if !ok {
			continue
		}
		if p.State == StateGone {
			if _, err := ix.Put(ctx, Request{Path: p.Path}); err != nil {
				return n, err
			}
			n++
			continue
		}
		raw, err := ix.readHead(p.Path)
		if err != nil {
			continue // unreadable now; the listing will name it again
		}
		l, ok := decodeLesson(id, raw)
		if !ok {
			continue
		}
		seat := l.Seat
		if seat == "" {
			seat = "unknown"
		}
		if _, err := ix.Put(ctx, Request{
			Path: p.Path, Title: l.Title, Summary: l.Summary, Tags: l.Tags, Writer: "seat:" + seat,
		}); err != nil {
			continue // a header edited out of bounds is left for its writer
		}
		n++
	}
	return n, nil
}

// readHead reads enough of a file for its header.
func (ix *Index) readHead(rel string) ([]byte, error) {
	f, err := ix.root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, MaxLessonBody+4<<10))
}
