package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func indexed(t *testing.T) (*Repo, *Index) {
	t.Helper()
	r := isolated(t)
	ix, err := OpenIndex(ctx(t), filepath.Join(t.TempDir(), "internal", "files-index.db"), r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	return r, ix
}

func put(t *testing.T, ix *Index, rel, title string) Entry {
	t.Helper()
	e, err := ix.Put(ctx(t), Request{Path: rel, Title: title, Summary: "about " + title, Writer: "seat-a"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// R25: a search finds a file by the words in it and answers a snippet, never
// the body.
func TestASearchFindsAFileAndNeverAnswersItsBody(t *testing.T) {
	r, ix := indexed(t)
	body := strings.Repeat("filler words that pad the file out\n", 200) +
		"the scheduler retries a failed job with exponential backoff " +
		strings.Repeat("more padding\n", 200)
	write(t, r, "lessons/retries.md", body)
	write(t, r, "docs/other.md", "nothing to see\n")
	put(t, ix, "lessons/retries.md", "Retry policy")
	put(t, ix, "docs/other.md", "Other")

	hits, err := ix.Search(ctx(t), "exponential backoff", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "lessons/retries.md" || hits[0].Title != "Retry policy" {
		t.Fatalf("hits = %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "[exponential]") {
		t.Fatalf("the snippet does not mark the match: %q", hits[0].Snippet)
	}
	if strings.ContainsAny(hits[0].Snippet, "\n\r\t") {
		t.Fatalf("a snippet spans lines: %q", hits[0].Snippet)
	}
	if len(hits[0].Snippet) > 200 {
		t.Fatalf("a snippet of %d bytes is a body, not a snippet", len(hits[0].Snippet))
	}
	// under narrows by directory, and a sibling that shares a prefix is out.
	write(t, r, "lessons2/x.md", "exponential backoff too\n")
	put(t, ix, "lessons2/x.md", "Sibling")
	if hits, _ := ix.Search(ctx(t), "exponential", "lessons", 0); len(hits) != 1 || hits[0].Path != "lessons/retries.md" {
		t.Fatalf("under=lessons answered %+v", hits)
	}
}

// Words reach FTS5 as quoted terms, so query syntax is a word like any other.
func TestASearchReadsNoQuerySyntax(t *testing.T) {
	r, ix := indexed(t)
	write(t, r, "a.md", "alpha\n")
	put(t, ix, "a.md", "A")
	for _, q := range []string{`alpha OR "`, `NEAR(alpha`, `title:alpha`, `alpha*`, `"; DROP TABLE entries; --`} {
		if _, err := ix.Search(ctx(t), q, "", 0); err != nil {
			t.Fatalf("%q: %v", q, err)
		}
	}
	if hits, _ := ix.Search(ctx(t), "alpha", "", 0); len(hits) != 1 {
		t.Fatal("the index did not survive the queries above")
	}
}

// R26: rig lists what the writers still owe: new, changed and gone.
func TestUnindexedListsNewStaleAndGone(t *testing.T) {
	r, ix := indexed(t)
	write(t, r, "a.md", "one\n")
	write(t, r, "b.md", "two\n")
	write(t, r, "c.md", "three\n")
	put(t, ix, "b.md", "B")
	put(t, ix, "c.md", "C")
	if _, err := ix.Put(ctx(t), Request{Path: "b.md", Title: "B", Summary: "b", Writer: "w"}); err != nil {
		t.Fatal(err)
	}
	// Changed after indexing: the mtime moves even when the size does not.
	write(t, r, "b.md", "TWO\n")
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(r.Dir(), "b.md"), future, future); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(r.Dir(), "c.md")); err != nil {
		t.Fatal(err)
	}
	got, total, err := ix.Unindexed(ctx(t), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []Pending{{"a.md", StateNew}, {"b.md", StateStale}, {"c.md", StateGone}}
	if total != 3 || len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %+v (total %d), want %+v", got, total, want)
	}
	// Indexing a gone path drops its entry, and then nothing is owed for it.
	if e := put(t, ix, "c.md", "C"); !e.Removed {
		t.Fatalf("a gone file was not dropped: %+v", e)
	}
	put(t, ix, "a.md", "A")
	put(t, ix, "b.md", "B")
	if got, total, _ := ix.Unindexed(ctx(t), "", 0); total != 0 {
		t.Fatalf("still owed: %+v", got)
	}
	// .git is rig's own and never listed.
	if _, err := os.Stat(filepath.Join(r.Dir(), ".git")); err != nil {
		t.Fatal(err)
	}
}

// ⛔ A caller's path stays inside the area, whether it climbs, is absolute,
// names .git, or goes through a symlink that points out.
func TestAPathOutsideTheAreaIsRefused(t *testing.T) {
	r, ix := indexed(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("the secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(r.Dir(), "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(r.Dir(), "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"../" + filepath.Base(outside) + "/secret.txt",
		"docs/../../x",
		filepath.Join(outside, "secret.txt"),
		".git/config",
		"out/secret.txt",
		"link.txt",
		"",
		".",
	} {
		_, err := ix.Put(ctx(t), Request{Path: p, Title: "t", Summary: "s", Writer: "w"})
		var inv *InvalidError
		if !errors.As(err, &inv) {
			t.Errorf("%q was not refused as invalid: %v", p, err)
			continue
		}
		// The caller's own words are echoed; a path it did not give is not.
		if !filepath.IsAbs(p) && strings.Contains(err.Error(), outside) {
			t.Errorf("%q: the refusal names a path outside the area: %v", p, err)
		}
	}
	if hits, _ := ix.Search(ctx(t), "secret", "", 0); len(hits) != 0 {
		t.Fatalf("a file outside the area reached the index: %+v", hits)
	}
	// And the listing does not walk out through the directory link either.
	got, _, err := ix.Unindexed(ctx(t), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if strings.Contains(p.Path, "secret") {
			t.Fatalf("the listing walked out of the area: %+v", got)
		}
	}
}

// A binary is indexed by its words alone; a huge text by its head.
func TestABinaryIsFoundByItsTitleAndAHugeTextByItsHead(t *testing.T) {
	r, ix := indexed(t)
	write(t, r, "resources/img/logo.png", "\x89PNG\x00\x00zebra")
	e := put(t, ix, "resources/img/logo.png", "Company logo")
	if !e.Binary || e.TextBytes != 0 {
		t.Fatalf("binary entry = %+v", e)
	}
	if hits, _ := ix.Search(ctx(t), "zebra", "", 0); len(hits) != 0 {
		t.Fatal("a binary's bytes were searched")
	}
	if hits, _ := ix.Search(ctx(t), "logo", "", 0); len(hits) != 1 {
		t.Fatal("a binary is not found by its title")
	}
	write(t, r, "big.log", strings.Repeat("x", MaxIndexedText)+" tailword\n")
	if e := put(t, ix, "big.log", "Big"); !e.Truncated || e.TextBytes != MaxIndexedText {
		t.Fatalf("huge entry = %+v", e)
	}
	// Invalid UTF-8 is repaired, since a snippet is a proto string.
	write(t, r, "latin1.txt", "caf\xe9 society\n")
	put(t, ix, "latin1.txt", "Latin")
	hits, _ := ix.Search(ctx(t), "society", "", 0)
	if len(hits) != 1 || !strings.Contains(hits[0].Snippet, "�") {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestAnEntryIsRefusedOnItsFace(t *testing.T) {
	r, ix := indexed(t)
	write(t, r, "a.md", "a\n")
	for _, req := range []Request{
		{Path: "a.md", Summary: "s", Writer: "w"},
		{Path: "a.md", Title: "t", Writer: "w"},
		{Path: "a.md", Title: "t", Summary: "two\nlines", Writer: "w"},
		{Path: "a.md", Title: "t", Summary: "s", Tags: []string{"two words"}, Writer: "w"},
		{Path: "a.md", Title: "t", Summary: "s"},
		{Path: "a.md", Title: strings.Repeat("t", MaxTitle+1), Summary: "s", Writer: "w"},
	} {
		var inv *InvalidError
		if _, err := ix.Put(ctx(t), req); !errors.As(err, &inv) {
			t.Errorf("%+v: %v", req, err)
		}
	}
	if _, err := ix.Search(ctx(t), "a", "", MaxHits+1); err == nil {
		t.Error("a search above the ceiling was answered")
	}
	if _, _, err := ix.Unindexed(ctx(t), "", MaxPending+1); err == nil {
		t.Error("a listing above the ceiling was answered")
	}
}

func TestTheIndexIsOwnerOnlyAndReopens(t *testing.T) {
	r := isolated(t)
	db := filepath.Join(t.TempDir(), "internal", "files-index.db")
	ix, err := OpenIndex(ctx(t), db, r)
	if err != nil {
		t.Fatal(err)
	}
	write(t, r, "a.md", "persisted words\n")
	put(t, ix, "a.md", "A")
	_ = ix.Close()
	if fi, _ := os.Stat(db); fi.Mode().Perm() != 0o600 {
		t.Fatalf("index mode %v", fi.Mode().Perm())
	}
	ix, err = OpenIndex(ctx(t), db, r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	if hits, _ := ix.Search(ctx(t), "persisted", "", 0); len(hits) != 1 {
		t.Fatal("the index did not survive a reopen")
	}
}

// A directory a program made unreadable does not break the listing.
func TestAnUnreadableDirectoryDoesNotBreakTheListing(t *testing.T) {
	r, ix := indexed(t)
	write(t, r, "a.md", "a\n")
	write(t, r, "locked/b.md", "b\n")
	locked := filepath.Join(r.Dir(), "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	got, total, err := ix.Unindexed(ctx(t), "", 0)
	if err != nil || total != 1 || got[0].Path != "a.md" {
		t.Fatalf("got %+v (total %d), %v", got, total, err)
	}
}
