package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(ctx(t), t.TempDir(), "graft", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// D3: a writer holding a stale version is refused and told the current one.
func TestAStaleVersionIsRefused(t *testing.T) {
	s := open(t)
	c := ctx(t)
	v1, err := s.Put(c, "runs", "r1", 0, raw(`{"state":"queued"}`))
	if err != nil || v1 != 1 {
		t.Fatalf("create: %d, %v", v1, err)
	}
	if _, err := s.Put(c, "runs", "r1", v1, raw(`{"state":"running"}`)); err != nil {
		t.Fatal(err)
	}
	_, err = s.Put(c, "runs", "r1", v1, raw(`{"state":"lost"}`))
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Current != 2 || ce.Expected != 1 {
		t.Fatalf("a stale put was not refused as a conflict: %v", err)
	}
	if _, err := s.Put(c, "runs", "r1", 0, raw(`{}`)); !errors.As(err, &ce) {
		t.Fatalf("creating over an existing id was not refused: %v", err)
	}
	got, _ := s.Get(c, "runs", []string{"r1"})
	if len(got) != 1 || string(got[0].Body) != `{"state":"running"}` || got[0].Version != 2 {
		t.Fatalf("the refused writes changed the document: %+v", got)
	}
}

// Two writers racing on one version: exactly one wins.
func TestTwoRacingWritersOneWins(t *testing.T) {
	s := open(t)
	c := ctx(t)
	if _, err := s.Put(c, "runs", "r1", 0, raw(`{"n":0}`)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Put(c, "runs", "r1", 1, raw(`{"n":1}`))
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d of %d writers won the same version: %v", wins, len(errs), errs)
	}
}

// D4: a name that is a path never reaches the disk or SQL.
func TestAHostileNameIsRefusedBeforeTheDatabase(t *testing.T) {
	s := open(t)
	c := ctx(t)
	for _, bad := range []string{"../x", "a/b", "", "Runs", "x;drop", strings.Repeat("a", 65)} {
		if _, err := s.Put(c, bad, "id", 0, raw(`{}`)); err == nil {
			t.Errorf("collection %q was accepted", bad)
		}
		if _, err := s.Put(c, "runs", bad, 0, raw(`{}`)); err == nil {
			t.Errorf("id %q was accepted", bad)
		}
	}
	if _, err := Open(c, t.TempDir(), "../escape", false); err == nil {
		t.Error("a program name that is a path was accepted")
	}
}

// D4: a field carrying a quote cannot change what the query means.
func TestAFieldWithAQuoteCannotAlterTheQuery(t *testing.T) {
	s := open(t)
	c := ctx(t)
	_, _ = s.Put(c, "runs", "a", 0, raw(`{"owner":"x"}`))
	for _, f := range []string{`owner') OR 1=1 --`, `owner"`, `$.owner`, `owner[0]`} {
		_, err := s.Query(c, Query{
			Collection: "runs",
			Where:      []Cond{{Field: f, Op: OpEq, Value: raw(`"nobody"`)}},
		})
		var ie *InvalidError
		if !errors.As(err, &ie) {
			t.Errorf("field %q was not refused: %v", f, err)
		}
	}
	// And a value with a quote is only a value.
	res, err := s.Query(c, Query{
		Collection: "runs",
		Where:      []Cond{{Field: "owner", Op: OpEq, Value: raw(`"x' OR '1'='1"`)}},
	})
	if err != nil || res.Total != 0 {
		t.Fatalf("a quoted value matched something: %+v, %v", res, err)
	}
}

// transact: one failing step leaves nothing written.
func TestATransactionIsAllOrNothing(t *testing.T) {
	s := open(t)
	c := ctx(t)
	_, _ = s.Put(c, "runs", "taken", 0, raw(`{}`))
	_, err := s.Transact(c, []Op{
		{Collection: "runs", ID: "new1", Body: raw(`{"a":1}`)},
		{Collection: "runs", ID: "taken", Body: raw(`{"a":2}`)}, // conflicts: exists at 1
	})
	var oe *OpError
	if !errors.As(err, &oe) || oe.Index != 1 {
		t.Fatalf("the failing step was not named: %v", err)
	}
	if got, _ := s.Get(c, "runs", []string{"new1"}); len(got) != 0 {
		t.Fatal("a step before the failure was written")
	}
	vs, err := s.Transact(c, []Op{
		{Collection: "runs", ID: "new1", Body: raw(`{"a":1}`)},
		{Collection: "runs", ID: "taken", Expected: 1},
	})
	if err != nil || vs[0] != 1 || vs[1] != 0 {
		t.Fatalf("a good transaction: %v, %v", vs, err)
	}
	if got, _ := s.Get(c, "runs", []string{"taken"}); len(got) != 0 {
		t.Fatal("the delete in the transaction did not happen")
	}
}

func TestDeleteNamesTheVersionItRead(t *testing.T) {
	s := open(t)
	c := ctx(t)
	_, _ = s.Put(c, "runs", "r", 0, raw(`{}`))
	if err := s.Delete(c, "runs", "r", 0); err == nil {
		t.Fatal("a delete with no version was accepted")
	}
	var ce *ConflictError
	if err := s.Delete(c, "runs", "r", 7); !errors.As(err, &ce) {
		t.Fatalf("a delete at the wrong version: %v", err)
	}
	if err := s.Delete(c, "runs", "r", 1); err != nil {
		t.Fatal(err)
	}
}

func TestQueryFiltersOrdersProjectsAndCounts(t *testing.T) {
	s := open(t)
	c := ctx(t)
	for id, body := range map[string]string{
		"a": `{"state":"done","cost":3.5,"meta":{"owner":"boris","tag":"x"},"ok":true}`,
		"b": `{"state":"done","cost":1,"meta":{"owner":"seat"},"ok":false}`,
		"c": `{"state":"queued","cost":9}`,
		"d": `{"state":"done","cost":2,"ok":true}`,
	} {
		if _, err := s.Put(c, "runs", id, 0, raw(body)); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = s.Put(c, "other", "a", 0, raw(`{"state":"done"}`))

	res, err := s.Query(c, Query{
		Collection: "runs",
		Where:      []Cond{{Field: "state", Op: OpEq, Value: raw(`"done"`)}},
		Order:      []Order{{Field: "cost", Desc: true}}, Limit: 2,
		Fields: []string{"cost", "meta.owner"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || !res.More || len(res.Docs) != 2 {
		t.Fatalf("total %d more %v docs %d", res.Total, res.More, len(res.Docs))
	}
	if res.Docs[0].ID != "a" || string(res.Docs[0].Body) != `{"cost":3.5,"meta":{"owner":"boris"}}` {
		t.Fatalf("first doc %s %s", res.Docs[0].ID, res.Docs[0].Body)
	}
	if res.Docs[1].ID != "d" || string(res.Docs[1].Body) != `{"cost":2}` {
		t.Fatalf("second doc %s %s", res.Docs[1].ID, res.Docs[1].Body)
	}

	n, _ := s.Query(c, Query{
		Collection: "runs", CountOnly: true,
		Where: []Cond{{Field: "ok", Op: OpEq, Value: raw(`true`)}, {Field: "cost", Op: OpGt, Value: raw(`2`)}},
	})
	if n.Total != 1 || n.Docs != nil {
		t.Fatalf("count of ok and cost>2 = %+v, want 1 and no docs", n)
	}
	nulls, _ := s.Query(c, Query{
		Collection: "runs",
		Where:      []Cond{{Field: "meta", Op: OpEq, Value: raw(`null`)}},
	})
	if nulls.Total != 2 {
		t.Fatalf("documents with no meta = %d, want 2", nulls.Total)
	}
	if _, err := s.Query(c, Query{Collection: "runs", Where: []Cond{{Field: "state", Value: raw(`"x"`)}}}); err == nil {
		t.Fatal("a condition with no operator was read as something")
	}
}

func TestGetAnswersManyInTheOrderAsked(t *testing.T) {
	s := open(t)
	c := ctx(t)
	for _, id := range []string{"a", "b", "c"} {
		_, _ = s.Put(c, "runs", id, 0, raw(`{"id":"`+id+`"}`))
	}
	got, err := s.Get(c, "runs", []string{"c", "missing", "a"})
	if err != nil || len(got) != 2 || got[0].ID != "c" || got[1].ID != "a" {
		t.Fatalf("get: %+v, %v", got, err)
	}
}

func TestADocumentMustBeAnObjectAndIsStoredCompacted(t *testing.T) {
	s := open(t)
	c := ctx(t)
	for _, bad := range []string{`[1]`, `"x"`, `{`, ``, `3`} {
		if _, err := s.Put(c, "runs", "r", 0, raw(bad)); err == nil {
			t.Errorf("body %q was accepted", bad)
		}
	}
	_, _ = s.Put(c, "runs", "r", 0, raw("{ \"a\" :  1 }"))
	got, _ := s.Get(c, "runs", []string{"r"})
	if string(got[0].Body) != `{"a":1}` {
		t.Fatalf("stored %s", got[0].Body)
	}
	big := `{"x":"` + strings.Repeat("a", MaxDocument) + `"}`
	if _, err := s.Put(c, "runs", "big", 0, raw(big)); err == nil {
		t.Fatal("a document over the ceiling was accepted")
	}
}

func TestCollectionsAreListedWithCounts(t *testing.T) {
	s := open(t)
	c := ctx(t)
	_, _ = s.Put(c, "runs", "a", 0, raw(`{}`))
	_, _ = s.Put(c, "runs", "b", 0, raw(`{}`))
	_, _ = s.Put(c, "assignments", "a", 0, raw(`{"x":1}`))
	cs, err := s.Collections(c)
	if err != nil || len(cs) != 2 || cs[0].Name != "assignments" || cs[1].Count != 2 {
		t.Fatalf("collections: %+v, %v", cs, err)
	}
}

// One file per program, owner-only, and the data survives a reopen.
func TestEachProgramIsItsOwnFileAndSurvivesAReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "internal", "store")
	c := ctx(t)
	a, _ := Open(c, dir, "graft", false)
	b, _ := Open(c, dir, "shelf", false)
	_, _ = a.Put(c, "runs", "r", 0, raw(`{"who":"graft"}`))
	if got, _ := b.Get(c, "runs", []string{"r"}); len(got) != 0 {
		t.Fatal("one program read another's document")
	}
	_ = a.Close()
	_ = b.Close()
	fi, err := os.Stat(dir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v, %v", fi.Mode().Perm(), err)
	}
	for _, name := range []string{"graft.db", "graft.db-wal"} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %v, want owner-only", name, fi.Mode().Perm())
		}
	}
	a2, _ := Open(c, dir, "graft", false)
	defer func() { _ = a2.Close() }()
	if got, _ := a2.Get(c, "runs", []string{"r"}); len(got) != 1 {
		t.Fatal("the document did not survive a reopen")
	}
	if a2.Path() != filepath.Join(dir, "graft.db") {
		t.Fatalf("path %s", a2.Path())
	}
}

// A file written by a newer rigd is refused, never opened and never repaired.
func TestAFutureSchemaIsRefused(t *testing.T) {
	dir := t.TempDir()
	c := ctx(t)
	s, _ := Open(c, dir, "graft", false)
	if _, err := s.db.ExecContext(c, "PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err := Open(c, dir, "graft", false)
	var fe *FutureSchemaError
	if !errors.As(err, &fe) || fe.Found != 99 {
		t.Fatalf("a future schema was opened: %v", err)
	}
}

// Every list a caller hands in has a ceiling, checked before any SQL.
func TestEveryCallerListIsBounded(t *testing.T) {
	s := open(t)
	c := ctx(t)
	ops := make([]Op, MaxOps+1)
	for i := range ops {
		ops[i] = Op{Collection: "runs", ID: fmt.Sprintf("r%d", i), Body: raw(`{}`)}
	}
	var ie *InvalidError
	if _, err := s.Transact(c, ops); !errors.As(err, &ie) {
		t.Fatalf("%d ops were accepted: %v", len(ops), err)
	}
	conds := make([]Cond, MaxTerms+1)
	for i := range conds {
		conds[i] = Cond{Field: "a", Op: OpEq, Value: raw(`1`)}
	}
	fields := make([]string, MaxTerms+1)
	for i := range fields {
		fields[i] = "a"
	}
	for name, q := range map[string]Query{
		"where":          {Collection: "runs", Where: conds},
		"fields":         {Collection: "runs", Fields: fields},
		"limit":          {Collection: "runs", Limit: MaxLimit + 1},
		"negative limit": {Collection: "runs", Limit: -1},
	} {
		if _, err := s.Query(c, q); !errors.As(err, &ie) {
			t.Errorf("an over-long %s was accepted: %v", name, err)
		}
	}
	if _, err := s.Transact(c, ops[:MaxOps]); err != nil {
		t.Fatalf("exactly %d ops were refused: %v", MaxOps, err)
	}
}
