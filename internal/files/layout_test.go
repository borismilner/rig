package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustLayout(t *testing.T, text string) Layout {
	t.Helper()
	l, err := ParseLayout(text)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func isInvalid(err error) bool {
	var inv *InvalidError
	return errors.As(err, &inv)
}

// R29, R31: a writer asks where a file goes and gets one answer.
func TestTheDefaultLayoutPlacesEachKind(t *testing.T) {
	l := mustLayout(t, DefaultLayout)
	for _, c := range []struct {
		kind string
		v    Vars
		name string
		want string
	}{
		{"docs", Vars{Subject: "scheduler"}, "cron.md", "docs/scheduler/cron.md"},
		{"resources", Vars{Type: "pdf", Subject: "scheduler"}, "rfc.pdf", "resources/pdf/scheduler/rfc.pdf"},
		{"lessons", Vars{}, "retries.md", "lessons/retries.md"},
		{"programs", Vars{Program: "graft"}, "", "programs/graft"},
		// A value the place does not use is not an error.
		{"lessons", Vars{Subject: "unused"}, "a.md", "lessons/a.md"},
	} {
		got, err := l.Place(c.kind, c.v, c.name)
		if err != nil || got != c.want {
			t.Errorf("%s %+v %q: %q, %v; want %q", c.kind, c.v, c.name, got, err, c.want)
		}
	}
}

// R33: a kind the layout does not name is refused with the kinds that exist.
func TestAnUnknownKindIsRefusedWithTheList(t *testing.T) {
	l := mustLayout(t, DefaultLayout)
	_, err := l.Place("images", Vars{}, "a.png")
	if !isInvalid(err) || !strings.Contains(err.Error(), "docs, resources, lessons, programs") {
		t.Fatalf("%v", err)
	}
}

func TestAPlaceTakesNamesNotPaths(t *testing.T) {
	l := mustLayout(t, DefaultLayout)
	for _, c := range []struct {
		kind string
		v    Vars
		name string
	}{
		{"docs", Vars{}, "a.md"},                         // subject missing
		{"docs", Vars{Subject: "../x"}, "a.md"},          // climbs
		{"docs", Vars{Subject: "a/b"}, "a.md"},           // two directories
		{"docs", Vars{Subject: "Sched"}, "a.md"},         // not a name
		{"docs", Vars{Subject: "s"}, "../../etc/passwd"}, // file name climbs
		{"docs", Vars{Subject: "s"}, ".."},               // file name is a parent
		{"docs", Vars{Subject: "s"}, ".git"},             // git's own
		{"programs", Vars{Program: "a\x00b"}, ""},        // NUL
		{"resources", Vars{Subject: "s"}, "x.pdf"},       // type missing
	} {
		if got, err := l.Place(c.kind, c.v, c.name); !isInvalid(err) {
			t.Errorf("%s %+v %q answered %q, %v", c.kind, c.v, c.name, got, err)
		}
	}
}

func TestALayoutIsRefusedWholeOnABadLine(t *testing.T) {
	const programs = "programs programs/{program}\n"
	for _, text := range []string{
		"docs {subject}/docs\n" + programs,              // top level is a placeholder
		"docs docs/{topic}\n" + programs,                // unknown placeholder
		"docs docs/{subject}/{subject}\n" + programs,    // placeholder twice
		"docs docs/\nnotes docs/{subject}\n" + programs, // shared top level
		"docs docs/\ndocs other/\n" + programs,          // kind twice
		"docs ../docs\n" + programs,                     // climbs
		"docs .git/x\n" + programs,                      // git's own
		"Docs docs\n" + programs,                        // kind not a name
		"docs docs/ extra\n" + programs,                 // three words
		"docs docs/\n",                                  // no programs kind
		"programs programs/\n",                          // programs without {program}
		"programs programs/{program}/{subject}\n",       // programs with more
	} {
		if _, err := ParseLayout(text); !isInvalid(err) {
			t.Errorf("accepted:\n%s", text)
		}
	}
	l, err := ParseLayout("# a comment\n\n  docs   docs/{subject}   # trailing\n" + programs)
	if err != nil || len(l.Kinds) != 2 {
		t.Fatalf("%+v, %v", l, err)
	}
}

func openLayouts(t *testing.T) (*Layouts, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "internal")
	ls, err := OpenLayouts(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ls, dir
}

func edit(t *testing.T, ls *Layouts, text string) {
	t.Helper()
	if err := os.WriteFile(ls.File(), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// R30: the layout is a file Boris edits, and an edit changes nothing until
// it is applied.
func TestTheLayoutIsAFileAndAnEditWaitsForRelayout(t *testing.T) {
	ls, dir := openLayouts(t)
	for _, f := range []string{"layout.txt", "layout.applied"} {
		fi, err := os.Stat(filepath.Join(dir, f))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", f, fi, err)
		}
	}
	edit(t, ls, strings.Replace(DefaultLayout, "docs       docs/{subject}/", "docs documentation/{subject}", 1))
	if k, _ := ls.InForce().Kind("docs"); k.Place != "docs/{subject}" {
		t.Fatalf("an edit took effect before relayout: %s", k.Place)
	}
	// A deleted edit file comes back from the applied copy.
	if err := os.Remove(ls.File()); err != nil {
		t.Fatal(err)
	}
	ls2, err := OpenLayouts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if e, err := ls2.Edited(); err != nil || e.Text() != DefaultLayout {
		t.Fatalf("%v", err)
	}
}

func relaidOut(t *testing.T) (*Repo, *Index, *Layouts) {
	t.Helper()
	r, ix := indexed(t)
	ls, _ := openLayouts(t)
	return r, ix, ls
}

func exists(t *testing.T, r *Repo, rel string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(r.Dir(), rel))
	return err == nil
}

const movedDocs = "docs documentation/{subject}\nresources resources/{type}/{subject}\n" +
	"lessons lessons/\nprograms programs/{program}\n"

// R30 done-bar: an edited layout moves the files, their index entries follow,
// and a search still finds them at the new place.
func TestRelayoutMovesFilesAndTheirEntries(t *testing.T) {
	r, ix, ls := relaidOut(t)
	write(t, r, "docs/sched/cron.md", "five fields of cron\n")
	write(t, r, "docs/sched/deep/notes.md", "nested notes\n")
	write(t, r, "lessons/a.md", "a lesson\n")
	put(t, ix, "docs/sched/cron.md", "Cron")
	put(t, ix, "lessons/a.md", "A")
	edit(t, ls, movedDocs)

	dry, err := ix.Relayout(ctx(t), ls, true)
	if err != nil || dry.Applied || len(dry.Moves) != 2 || !exists(t, r, "docs/sched/cron.md") {
		t.Fatalf("dry run: %+v, %v", dry, err)
	}
	rep, err := ix.Relayout(ctx(t), ls, false)
	if err != nil || !rep.Applied || rep.Reindexed != 1 || len(rep.Moves) != 2 {
		t.Fatalf("relayout: %+v, %v", rep, err)
	}
	for _, p := range []string{"documentation/sched/cron.md", "documentation/sched/deep/notes.md", "lessons/a.md"} {
		if !exists(t, r, p) {
			t.Errorf("%s is missing", p)
		}
	}
	if exists(t, r, "docs") {
		t.Error("the emptied docs/ was left behind")
	}
	hits, _ := ix.Search(ctx(t), "cron", "", 0)
	if len(hits) != 1 || hits[0].Path != "documentation/sched/cron.md" {
		t.Fatalf("search after relayout: %+v", hits)
	}
	// The move kept the file's mtime, so its entry is not stale.
	got, _, _ := ix.Unindexed(ctx(t), "documentation", 0)
	if len(got) != 1 || got[0].Path != "documentation/sched/deep/notes.md" || got[0].State != StateNew {
		t.Fatalf("unindexed after relayout: %+v", got)
	}
	if p, _ := ls.InForce().Place("docs", Vars{Subject: "sched"}, "x.md"); p != "documentation/sched/x.md" {
		t.Fatalf("place after relayout: %s", p)
	}
}

// ⛔ A relayout that would strand or overwrite a file moves nothing.
func TestARelayoutThatWouldStrandAFileMovesNothing(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, r *Repo)
		next  string
		why   string
	}{
		{
			"a kind removed with files in it", nil,
			"resources resources/{type}/{subject}\nlessons lessons/\nprograms programs/{program}\n", "is gone from the edited layout",
		},
		{
			"a file the old place does not lay out",
			func(t *testing.T, r *Repo) { write(t, r, "docs/loose.md", "loose\n") }, movedDocs, "docs/loose.md",
		},
		{
			"a new place needing a value the files lack", nil,
			"docs documentation/{type}/{subject}\nresources resources/{type}/{subject}\n" +
				"lessons lessons/\nprograms programs/{program}\n", "needs {type}",
		},
		{
			"a move landing on an existing file",
			func(t *testing.T, r *Repo) { write(t, r, "documentation/sched/cron.md", "already here\n") }, movedDocs, "which exists",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, ix, ls := relaidOut(t)
			write(t, r, "docs/sched/cron.md", "cron\n")
			if c.setup != nil {
				c.setup(t, r)
			}
			edit(t, ls, c.next)
			_, err := ix.Relayout(ctx(t), ls, false)
			if !isInvalid(err) || !strings.Contains(err.Error(), c.why) {
				t.Fatalf("not refused for %q: %v", c.why, err)
			}
			if !exists(t, r, "docs/sched/cron.md") {
				t.Fatal("a refused relayout moved a file")
			}
			if k, _ := ls.InForce().Kind("docs"); k.Place != "docs/{subject}" {
				t.Fatal("a refused relayout changed the layout in force")
			}
		})
	}
}

// A kind renamed with its place unchanged moves nothing and is allowed.
func TestAKindRenamedInPlaceMovesNothing(t *testing.T) {
	r, ix, ls := relaidOut(t)
	write(t, r, "docs/sched/cron.md", "cron\n")
	edit(t, ls, strings.Replace(DefaultLayout, "docs       docs/{subject}/", "notes docs/{subject}", 1))
	rep, err := ix.Relayout(ctx(t), ls, false)
	if err != nil || len(rep.Moves) != 0 || !rep.Applied {
		t.Fatalf("%+v, %v", rep, err)
	}
	if p, err := ls.InForce().Place("notes", Vars{Subject: "sched"}, ""); err != nil || p != "docs/sched" {
		t.Fatalf("%q, %v", p, err)
	}
}

// ⛔ A move that fails part-way puts back every move already made.
func TestAFailedRelayoutPutsBackWhatItMoved(t *testing.T) {
	r, ix, ls := relaidOut(t)
	write(t, r, "docs/sched/cron.md", "cron\n")
	write(t, r, "resources/pdf/sched/rfc.pdf", "%PDF\n")
	// res/ exists and cannot be written, so the resources moves fail after
	// the docs moves were made.
	if err := os.Mkdir(filepath.Join(r.Dir(), "res"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(r.Dir(), "res"), 0o700) })
	edit(t, ls, "docs documentation/{subject}\nresources res/{type}/{subject}\n"+
		"lessons lessons/\nprograms programs/{program}\n")
	if _, err := ix.Relayout(ctx(t), ls, false); err == nil {
		t.Fatal("the relayout did not fail")
	}
	if !exists(t, r, "docs/sched/cron.md") || exists(t, r, "documentation/sched/cron.md") {
		t.Fatal("the docs move was not put back")
	}
	if k, _ := ls.InForce().Kind("docs"); k.Place != "docs/{subject}" {
		t.Fatal("a failed relayout changed the layout in force")
	}
}
