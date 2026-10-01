package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func exec(ddl string) Step { return execStep(ddl) }

func version(t *testing.T, path string) uint32 {
	t.Helper()
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	v, err := Version(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// A new file is built at the current version; an older one is stepped
// forward through every step; reopening at the same version runs nothing.
func TestRunnerBuildsAndStepsForward(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	v1 := Schema{What: "test store", Version: 1, Create: exec(`CREATE TABLE a (k TEXT)`)}
	db, err := OpenDB(ctx, path, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	ran := 0
	counted := func(ddl string) Step {
		return func(ctx context.Context, tx *sql.Tx) error { ran++; return exec(ddl)(ctx, tx) }
	}
	v3 := Schema{
		What: "test store", Version: 3, Create: exec(`CREATE TABLE never (k TEXT)`),
		Steps: map[uint32]Step{1: counted(`CREATE TABLE b (k TEXT)`), 2: counted(`CREATE TABLE c (k TEXT)`)},
	}
	db, err = OpenDB(ctx, path, v3)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('a','b','c')`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("tables after stepping: %d %v", n, err)
	}
	_ = db.Close()
	if ran != 2 || version(t, path) != 3 {
		t.Fatalf("ran %d steps, version %d", ran, version(t, path))
	}
	db, err = OpenDB(ctx, path, v3)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if ran != 2 {
		t.Fatalf("a reopen at the same version ran a step")
	}
}

// Red control: stamping before the steps, or outside their transaction,
// leaves the version moved while the data did not.
func TestAFailingStepLeavesTheVersionUnchanged(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	db, err := OpenDB(ctx, path, Schema{What: "t", Version: 1, Create: exec(`CREATE TABLE a (k TEXT)`)})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	bad := Schema{What: "t", Version: 3, Steps: map[uint32]Step{
		1: exec(`CREATE TABLE b (k TEXT)`),
		2: func(context.Context, *sql.Tx) error { return errors.New("boom") },
	}}
	if _, err := OpenDB(ctx, path, bad); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("a failing step opened: %v", err)
	}
	if v := version(t, path); v != 1 {
		t.Fatalf("version is %d after a failed migration, want 1", v)
	}
	db, _ = OpenReadOnly(path)
	defer func() { _ = db.Close() }()
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'b'`).Scan(&n)
	if n != 0 {
		t.Fatal("step 1 survived the failure of step 2")
	}
}

// Red control: accepting a newer file. Refused, named, and not touched.
func TestRunnerRefusesAFutureVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	db, err := OpenDB(ctx, path, Schema{What: "t", Version: 5, Create: exec(`CREATE TABLE a (k TEXT)`)})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	_, err = OpenDB(ctx, path, Schema{What: "coord store", Version: 2})
	var fe *FutureSchemaError
	if !errors.As(err, &fe) || fe.Found != 5 || fe.Known != 2 {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "the coord store at") {
		t.Errorf("the refusal does not name the store: %v", err)
	}
	if version(t, path) != 5 {
		t.Error("the refused file was restamped")
	}
}

// An intermediate version with no registered step is an error, never a
// silent rebuild.
func TestAMissingStepIsRefused(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	db, _ := OpenDB(ctx, path, Schema{What: "t", Version: 1, Create: exec(`CREATE TABLE a (k TEXT)`)})
	_ = db.Close()
	if _, err := OpenDB(ctx, path, Schema{What: "t", Version: 2}); err == nil {
		t.Fatal("a store with no step from 1 opened at 2")
	}
}
