package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func seed(t *testing.T, s *Store) {
	t.Helper()
	c := ctx(t)
	for _, p := range []struct{ coll, id, body string }{
		{"runs", "r2", `{"state":"queued","note":"a <b> & c"}`},
		{"runs", "r1", `{"state": "done", "n": 3}`},
		{"tags", "t1", `{"name":"x"}`},
	} {
		if _, err := s.Put(c, p.coll, p.id, 0, raw(p.body)); err != nil {
			t.Fatal(err)
		}
	}
	// A second write, so a version above 1 must survive the round trip.
	if _, err := s.Put(c, "runs", "r1", 1, raw(`{"state":"done","n":4}`)); err != nil {
		t.Fatal(err)
	}
}

func all(t *testing.T, s *Store, coll string) []Doc {
	t.Helper()
	r, err := s.Query(ctx(t), Query{Collection: coll})
	if err != nil {
		t.Fatal(err)
	}
	return r.Docs
}

// Slice 5's bar: export, delete rows, import, identical documents and versions.
func TestExportThenImportRoundTrips(t *testing.T) {
	s := open(t)
	c := ctx(t)
	seed(t, s)
	before := map[string][]Doc{"runs": all(t, s, "runs"), "tags": all(t, s, "tags")}

	dir := t.TempDir()
	res, err := s.Export(c, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Collections) != 2 || res.Collections[0].Documents != 2 || res.Collections[1].Documents != 1 {
		t.Fatalf("export answered %+v", res)
	}

	if err := s.Delete(c, "runs", "r1", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(c, "runs", "r3", 0, raw(`{"late":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(c, "tags", "t1", 1); err != nil {
		t.Fatal(err)
	}

	snap := filepath.Join(t.TempDir(), "snap.db")
	ir, err := s.Import(c, dir, nil, snap)
	if err != nil {
		t.Fatal(err)
	}
	want := []Imported{{"runs", 2, 2}, {"tags", 1, 0}}
	if !reflect.DeepEqual(ir.Collections, want) {
		t.Fatalf("import answered %+v, want %+v", ir.Collections, want)
	}
	for coll, docs := range before {
		if got := all(t, s, coll); !reflect.DeepEqual(got, docs) {
			t.Fatalf("%s after the round trip:\n got %+v\nwant %+v", coll, got, docs)
		}
	}
	// The version survived, so a caller that read it before still writes.
	if _, err := s.Put(c, "runs", "r1", 2, raw(`{"after":1}`)); err != nil {
		t.Fatalf("the restored version did not take a write: %v", err)
	}
	if fi, err := os.Stat(snap); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot: %v, %v", fi, err)
	}
}

// R35: sorted by id, and two exports of an unchanged store are the same bytes.
func TestExportIsSortedAndByteStable(t *testing.T) {
	s := open(t)
	c := ctx(t)
	seed(t, s)
	d1, d2 := t.TempDir(), t.TempDir()
	if _, err := s.Export(c, d1, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Export(c, d2, nil); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(d1, "runs.jsonl"))
	b, _ := os.ReadFile(filepath.Join(d2, "runs.jsonl"))
	if string(a) != string(b) {
		t.Fatalf("two exports differ:\n%s\n%s", a, b)
	}
	lines := strings.Split(strings.TrimSuffix(string(a), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], `{"id":"r1","version":2,`) ||
		!strings.HasPrefix(lines[1], `{"id":"r2",`) {
		t.Fatalf("not one line per document sorted by id:\n%s", a)
	}
	if !strings.Contains(lines[1], `a <b> & c`) {
		t.Fatalf("HTML characters were escaped: %s", lines[1])
	}
}

// A whole export drops the file of a collection that is gone; a named one
// touches only what it names.
func TestAWholeExportRemovesStaleFiles(t *testing.T) {
	s := open(t)
	c := ctx(t)
	seed(t, s)
	dir := t.TempDir()
	if _, err := s.Export(c, dir, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(c, "tags", "t1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Export(c, dir, []string{"runs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tags.jsonl")); err != nil {
		t.Fatalf("a named export removed another collection's file: %v", err)
	}
	res, err := s.Export(c, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Removed, []string{"tags"}) {
		t.Fatalf("removed %v", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "tags.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tags.jsonl survived a whole export: %v", err)
	}
}

func TestExportRefusesAnUnknownCollection(t *testing.T) {
	s := open(t)
	seed(t, s)
	_, err := s.Export(ctx(t), t.TempDir(), []string{"nope"})
	var nf *NotFoundError
	if !errors.As(err, &nf) || !reflect.DeepEqual(nf.Have, []string{"runs", "tags"}) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Export(ctx(t), t.TempDir(), []string{"../x"}); err == nil {
		t.Fatal("a collection name that climbs out was accepted")
	}
}

// D8 and the order: a bad line anywhere refuses the import before the store
// or the snapshot is touched.
func TestABadLineRefusesTheWholeImport(t *testing.T) {
	for name, content := range map[string]string{
		"not json":      "{nope\n",
		"unknown field": `{"id":"a","version":1,"updated_ns":0,"doc":{},"x":1}` + "\n",
		"version zero":  `{"id":"a","version":0,"updated_ns":0,"doc":{}}` + "\n",
		"bad id":        `{"id":"A/B","version":1,"updated_ns":0,"doc":{}}` + "\n",
		"not an object": `{"id":"a","version":1,"updated_ns":0,"doc":[1]}` + "\n",
		"duplicate id": `{"id":"a","version":1,"updated_ns":0,"doc":{}}` + "\n" +
			`{"id":"a","version":2,"updated_ns":0,"doc":{}}` + "\n",
		"two objects": `{"id":"a","version":1,"updated_ns":0,"doc":{}} {}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := open(t)
			c := ctx(t)
			seed(t, s)
			dir := t.TempDir()
			if _, err := s.Export(c, dir, nil); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "tags.jsonl"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := s.Delete(c, "runs", "r2", 1); err != nil {
				t.Fatal(err)
			}
			snap := filepath.Join(t.TempDir(), "snap.db")
			_, err := s.Import(c, dir, nil, snap)
			var le *LineError
			var inv *InvalidError
			if !errors.As(err, &le) || !errors.As(err, &inv) {
				t.Fatalf("got %v", err)
			}
			if got := all(t, s, "runs"); len(got) != 1 {
				t.Fatalf("runs was replaced although tags was refused: %+v", got)
			}
			if _, err := os.Stat(snap); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("a refused import wrote a snapshot")
			}
		})
	}
}

// An import names what it replaces; the rest of the store is left alone.
func TestANamedImportLeavesTheRestAlone(t *testing.T) {
	s := open(t)
	c := ctx(t)
	seed(t, s)
	dir := t.TempDir()
	if _, err := s.Export(c, dir, []string{"tags"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(c, "runs", "r9", 0, raw(`{}`)); err != nil {
		t.Fatal(err)
	}
	res, err := s.Import(c, dir, []string{"tags"}, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Untouched, []string{"runs"}) || len(all(t, s, "runs")) != 3 {
		t.Fatalf("untouched %v, runs %d", res.Untouched, len(all(t, s, "runs")))
	}
	var nf *NotFoundError
	if _, err := s.Import(c, dir, []string{"runs"}, filepath.Join(t.TempDir(), "s.db")); !errors.As(err, &nf) {
		t.Fatalf("importing a collection with no file: %v", err)
	}
	if _, err := s.Import(c, t.TempDir(), nil, filepath.Join(t.TempDir(), "s.db")); !errors.As(err, &nf) {
		t.Fatalf("importing an empty directory: %v", err)
	}
}

// The snapshot never lands on top of an existing file.
func TestImportRefusesAnExistingSnapshotPath(t *testing.T) {
	s := open(t)
	c := ctx(t)
	seed(t, s)
	dir := t.TempDir()
	if _, err := s.Export(c, dir, nil); err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(t.TempDir(), "taken.db")
	if err := os.WriteFile(snap, []byte("his"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Import(c, dir, nil, snap); err == nil {
		t.Fatal("an import wrote its snapshot over an existing file")
	}
	if b, _ := os.ReadFile(snap); string(b) != "his" {
		t.Fatalf("the existing file was changed: %q", b)
	}
}
