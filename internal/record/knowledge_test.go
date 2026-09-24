package record

import (
	"errors"
	"strings"
	"testing"
)

func lesson(t *testing.T, s *Store, title, summary, body string, tags ...string) Lesson {
	t.Helper()
	l, err := s.AddLesson(tctx, LessonRequest{
		Title: title, Summary: summary, Body: body, Tags: tags,
		Session: "unix:1", Seat: "backend-record", Epoch: 7,
	})
	if err != nil {
		t.Fatalf("AddLesson(%q): %v", title, err)
	}
	return l
}

// ⛔ NEVER READ WHOLE (section 40): a search answers a title, a one-line
// summary and a snippet per hit, best first, and never a body; the body is
// one GetLesson away for the hit that fits.
func TestALessonIsFoundBySearchAndReadOnlyWhenFetched(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	body := strings.Repeat("filler words about nothing in particular. ", 200) +
		"A copy of record.db taken with cp is NOT a backup, because committed writes live in the WAL."
	wal := lesson(t, s, "SQLite WAL and backups", "cp of a WAL-mode database loses committed writes", body, "sqlite", "backup")
	lesson(t, s, "Go unix sockets", "sun_path is 108 bytes and fails as EINVAL", "Keep socket paths short.", "go")

	hits, err := s.SearchLessons(tctx, "how do I backup sqlite with cp", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].ID != wal.ID {
		t.Fatalf("the WAL lesson is not the best hit: %+v", hits)
	}
	for _, h := range hits {
		if len(h.Snippet) > 400 || strings.Contains(h.Snippet, "filler words about nothing in particular. filler") {
			t.Errorf("a hit carried the body, not a snippet: %d bytes", len(h.Snippet))
		}
	}
	if !strings.Contains(hits[0].Snippet, "[") {
		t.Errorf("the snippet marks no matched term: %q", hits[0].Snippet)
	}

	got, err := s.GetLesson(tctx, wal.ID)
	if err != nil || got.Body != body || got.Prov.Seat != "backend-record" || len(got.Tags) != 2 {
		t.Fatalf("GetLesson: %+v %v", got, err)
	}
	if _, err := s.GetLesson(tctx, "nope"); !errors.As(err, new(*NotFoundError)) {
		t.Errorf("an unknown lesson gave %v, want NotFound", err)
	}
}

// ⛔ A CALLER'S WORDS ARE NEVER READ AS FTS5 SYNTAX. Operators, quotes,
// column filters and prefix stars are searched for as words or dropped,
// never interpreted, and never an error from the query language.
func TestALessonSearchNeverInterpretsTheCallersSyntax(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	lesson(t, s, "near misses", "one line", "body")
	for _, q := range []string{`title:x`, `"unbalanced`, `a AND`, `NEAR(a b)`, `pre*`, `-x`, `'); DROP TABLE lessons; --`} {
		if _, err := s.SearchLessons(tctx, q, 0); err != nil {
			t.Errorf("query %q: %v", q, err)
		}
	}
	if _, err := s.SearchLessons(tctx, "near", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SearchLessons(tctx, "   ", 0); err == nil {
		t.Error("an empty query was accepted")
	}
	if _, err := s.SearchLessons(tctx, "x", MaxHits+1); err == nil {
		t.Error("a limit over MaxHits was accepted rather than refused")
	}
}

func TestALessonIsRefusedWhenItBreaksItsShape(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	ok := LessonRequest{Title: "t", Summary: "s", Body: "b", Session: "x", Seat: "y"}
	for name, mut := range map[string]func(*LessonRequest){
		"no title":          func(r *LessonRequest) { r.Title = " " },
		"no summary":        func(r *LessonRequest) { r.Summary = "" },
		"two-line summary":  func(r *LessonRequest) { r.Summary = "a\nb" },
		"long title":        func(r *LessonRequest) { r.Title = strings.Repeat("t", MaxLessonTitle+1) },
		"long body":         func(r *LessonRequest) { r.Body = strings.Repeat("b", MaxLessonBody+1) },
		"a tag with spaces": func(r *LessonRequest) { r.Tags = []string{"two words"} },
		"no provenance":     func(r *LessonRequest) { r.Seat = "" },
	} {
		r := ok
		mut(&r)
		if _, err := s.AddLesson(tctx, r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
