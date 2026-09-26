package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lesson(title string) Lesson {
	return Lesson{
		Title: title, Summary: "about " + title, Tags: []string{"sched"},
		Body: "the scheduler retries a failed job with exponential backoff, " +
			strings.Repeat("and a great deal more detail. ", 100),
		Seat: "backend-1", Session: "s1", Epoch: 3,
	}
}

// R24, R25: a lesson is a file in the shared folder, found by a snippet and
// read whole by its id.
func TestALessonIsAFileFoundByASnippet(t *testing.T) {
	r, ix := indexed(t)
	l, err := ix.AddLesson(ctx(t), "lessons", lesson("Retry policy"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(r.Dir(), "lessons", l.ID+".md"))
	if err != nil || !strings.Contains(string(raw), "title: Retry policy\n") {
		t.Fatalf("the lesson is not a file with its header: %v\n%s", err, raw)
	}
	hits, err := ix.SearchLessons(ctx(t), "lessons", "exponential backoff", 0)
	if err != nil || len(hits) != 1 || hits[0].ID != l.ID || hits[0].Title != "Retry policy" {
		t.Fatalf("%+v, %v", hits, err)
	}
	if len(hits[0].Snippet) > 200 {
		t.Fatalf("a %d-byte snippet is a body", len(hits[0].Snippet))
	}
	got, err := ix.GetLesson(ctx(t), "lessons", l.ID)
	if err != nil || got.Body != l.Body || got.Seat != "backend-1" || got.Epoch != 3 ||
		!got.Created.Equal(l.Created) || len(got.Tags) != 1 {
		t.Fatalf("get: %+v, %v", got, err)
	}
	var nf *NotFoundError
	if _, err := ix.GetLesson(ctx(t), "lessons", "no-such-lesson"); !errors.As(err, &nf) {
		t.Fatalf("a missing lesson: %v", err)
	}
	if _, err := ix.GetLesson(ctx(t), "lessons", "../files-index"); !isInvalid(err) {
		t.Fatalf("an id that is a path: %v", err)
	}
}

// The folder is the source: a lesson edited or written by hand is searched
// as it now reads, and a deleted one is not found.
func TestTheFolderIsTheSourceOfTheLessons(t *testing.T) {
	r, ix := indexed(t)
	l, err := ix.AddLesson(ctx(t), "lessons", lesson("Retry policy"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(r.Dir(), "lessons", l.ID+".md")
	raw, _ := os.ReadFile(p)
	edited := strings.Replace(string(raw), "exponential backoff", "jittered delay", 1)
	if err := os.WriteFile(p, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(p, later, later)
	if hits, _ := ix.SearchLessons(ctx(t), "lessons", "jittered", 0); len(hits) != 1 {
		t.Fatalf("an edited lesson is not searched as it reads: %+v", hits)
	}
	// Written by hand, with a header, never through rig.
	write(t, r, "lessons/by-hand.md", "---\ntitle: Cron syntax\nsummary: five fields\nseat: boris\n---\n\nminute hour day month weekday\n")
	hits, _ := ix.SearchLessons(ctx(t), "lessons", "weekday", 0)
	if len(hits) != 1 || hits[0].ID != "by-hand" || hits[0].Title != "Cron syntax" {
		t.Fatalf("a hand-written lesson: %+v", hits)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if hits, _ := ix.SearchLessons(ctx(t), "lessons", "jittered", 0); len(hits) != 0 {
		t.Fatalf("a deleted lesson is still found: %+v", hits)
	}
}

// A file in the folder with no header is found by its writer's entry.
func TestALessonWithNoHeaderIsNamedByItsEntry(t *testing.T) {
	r, ix := indexed(t)
	write(t, r, "lessons/plain.md", "plain prose about retries\n")
	if _, err := ix.Put(ctx(t), Request{Path: "lessons/plain.md", Title: "Plain", Summary: "no header", Writer: "seat:x"}); err != nil {
		t.Fatal(err)
	}
	hits, _ := ix.SearchLessons(ctx(t), "lessons", "prose", 0)
	if len(hits) != 1 || hits[0].ID != "plain" {
		t.Fatalf("%+v", hits)
	}
	got, err := ix.GetLesson(ctx(t), "lessons", "plain")
	if err != nil || got.Title != "Plain" || got.Seat != "x" || got.Body != "plain prose about retries\n" {
		t.Fatalf("%+v, %v", got, err)
	}
}

// The move out of the record store keeps every id and never overwrites.
func TestImportKeepsTheIdAndRunsTwice(t *testing.T) {
	_, ix := indexed(t)
	l := lesson("Old lesson")
	l.ID, l.Created = "0192a000-0000-7000-8000-000000000001", time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	if wrote, err := ix.ImportLesson(ctx(t), "lessons", l); err != nil || !wrote {
		t.Fatalf("%v %v", wrote, err)
	}
	l.Title = "Changed"
	if wrote, err := ix.ImportLesson(ctx(t), "lessons", l); err != nil || wrote {
		t.Fatalf("a second import overwrote: %v %v", wrote, err)
	}
	got, err := ix.GetLesson(ctx(t), "lessons", l.ID)
	if err != nil || got.Title != "Old lesson" || !got.Created.Equal(l.Created) {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestALessonIsRefusedOnItsFace(t *testing.T) {
	_, ix := indexed(t)
	for _, mut := range []func(*Lesson){
		func(l *Lesson) { l.Title = "" },
		func(l *Lesson) { l.Title = "two\nlines" },
		func(l *Lesson) { l.Summary = "two\nlines" },
		func(l *Lesson) { l.Body = strings.Repeat("x", MaxLessonBody+1) },
		func(l *Lesson) { l.Seat = "" },
		func(l *Lesson) { l.Seat = "a\ntitle: forged" },
		func(l *Lesson) { l.Tags = []string{"two words"} },
	} {
		l := lesson("t")
		mut(&l)
		if _, err := ix.AddLesson(ctx(t), "lessons", l); !isInvalid(err) {
			t.Errorf("%+v: %v", l, err)
		}
	}
}
