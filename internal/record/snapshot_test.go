package record

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// putN writes n records and answers their ids, so a caller can supersede
// some of them and pull the row count away from the head count.
func putN(t *testing.T, s *Store, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := range n {
		rec, err := s.Put(context.Background(), PutRequest{
			Kind:    "note",
			Project: "rig",
			Body:    "a record written so the snapshot has something to carry",
			Session: "s-snapshot", Seat: "backup", Epoch: 1,
		})
		if err != nil {
			t.Fatalf("writing record %d of %d: %v", i+1, n, err)
		}
		ids = append(ids, rec.ID)
	}
	return ids
}

// supersede writes a second version of each id, which adds a ROW and moves no
// HEAD.
//
// ⛔ IT IS WHAT MAKES THE TWO COUNTS DIFFERENT NUMBERS IN THE TEST BELOW, and
// without it neither count can be shown to be read from its own table. This
// store is 2,568 rows against 1,057 heads in production and a document said
// "2,538 records" while counting neither, which is the confusion the manifest
// carries both numbers to end.
func supersede(t *testing.T, s *Store, ids []string) {
	t.Helper()
	for _, id := range ids {
		if _, err := s.Put(context.Background(), PutRequest{
			ID: id, IfVersion: 1,
			Kind:    "note",
			Project: "rig",
			Body:    "the second version of this record",
			Session: "s-snapshot", Seat: "backup", Epoch: 1,
		}); err != nil {
			t.Fatalf("superseding %s: %v", id, err)
		}
	}
}

// openSnapshot opens a snapshot file the way a reader of a restored estate
// would, and registers the close.
func openSnapshot(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("opening the snapshot at %s: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func scalar[T any](t *testing.T, db *sql.DB, q string) T {
	t.Helper()
	var v T
	if err := db.QueryRow(q).Scan(&v); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v
}

// TEST 1 OF PLAN.md SECTION 46. A snapshot of a store holding N records opens
// with N records, carries the same user_version, and answers `ok` to
// PRAGMA integrity_check.
//
// ⛔ THE THREE CLAUSES ARE NOT ONE CLAUSE WRITTEN THREE WAYS, and the file
// below proves each separately, because COORDINATION.md's rule is to count the
// distinct ASSERTIONS a mutation turns red rather than the mutations. A
// snapshot that copied the main file and dropped the WAL would have the right
// user_version and the wrong count; a snapshot truncated after the fact has
// the right count in its header and fails integrity.
//
// THE RED CONTROL IS TRUNCATION BY ONE PAGE, and it is run below as its own
// test rather than described here, so the evidence and the claim live in the
// same file.
func TestASnapshotCarriesEveryRecordTheStoreHeld(t *testing.T) {
	const (
		n          = 17 // records
		superseded = 5  // of them written a second time
		rows       = n + superseded
	)
	s := openStore(t, estate(t, "development"))
	ids := putN(t, s, n)
	supersede(t, s, ids[:superseded])

	dst := filepath.Join(t.TempDir(), "snapshot.db")
	snap, err := s.Snapshot(context.Background(), dst)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if snap.Path != dst {
		t.Errorf("the snapshot reports path %q, want %q", snap.Path, dst)
	}

	// ⛔ THE TWO COUNTS ARE DIFFERENT NUMBERS HERE ON PURPOSE. With every
	// record written once they are equal, and a Snapshot that read `records`
	// into both fields would pass. Five of the seventeen are superseded, so
	// rows and heads differ by five and each assertion can only be satisfied by
	// reading its own table.
	if snap.Records != rows {
		t.Errorf("the snapshot reports %d records, want %d (%d records, %d of "+
			"them written twice)", snap.Records, rows, n, superseded)
	}
	if snap.Heads != n {
		t.Errorf("the snapshot reports %d heads, want %d: a superseded record "+
			"adds a ROW and moves no HEAD, so a heads count that answers %d is "+
			"counting the rows table", snap.Heads, n, rows)
	}
	if snap.SchemaVersion != SchemaVersion {
		t.Errorf("the snapshot reports schema %d, want %d", snap.SchemaVersion, SchemaVersion)
	}

	// ⛔ NO SIDECAR, WHICH IS HALF OF WHY DECISION 2 IS `VACUUM INTO` AND NOT A
	// FILE COPY. A snapshot arriving with a -wal beside it would make the
	// archive a set of files that have to stay in step, and decision 8 already
	// records what a stale -wal does when it is replayed into a fresh main
	// file. Asserted BEFORE the read below, which would create one.
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dst + suffix); err == nil {
			t.Errorf("the snapshot left %s beside it, so the archive would have "+
				"to carry more than one file per store", filepath.Base(dst+suffix))
		}
	}

	// And now the same facts read out of the FILE by an independent
	// connection, because everything above is the writer describing its own
	// work. A Snapshot struct filled from the live store rather than from the
	// copy would pass every assertion above and none below.
	db := openSnapshot(t, dst)
	if got := scalar[uint64](t, db, "SELECT COUNT(*) FROM records"); got != rows {
		t.Errorf("the snapshot FILE holds %d records, want %d: the struct above "+
			"was describing something other than these bytes", got, rows)
	}
	if got := scalar[uint64](t, db, "SELECT COUNT(*) FROM heads"); got != n {
		t.Errorf("the snapshot FILE holds %d heads, want %d", got, n)
	}
	if got := scalar[uint32](t, db, "PRAGMA user_version"); got != SchemaVersion {
		t.Errorf("the snapshot FILE is schema %d, want %d", got, SchemaVersion)
	}
	if got := scalar[string](t, db, "PRAGMA integrity_check"); got != "ok" {
		t.Errorf("PRAGMA integrity_check on the snapshot answered %q, want \"ok\"", got)
	}
}

// THE RED CONTROL FOR TEST 1, RUN RATHER THAN DESCRIBED.
//
// ⛔ A PASSING integrity_check IS AN ASSERTION ABOUT AN ABSENCE, and this
// repository has paid repeatedly for absences nothing could distinguish from a
// check that did not run. So the damage is inflicted here and the same PRAGMA
// is asked again: if it answers `ok` to a snapshot missing its last page, then
// its `ok` in the test above asserted nothing at all.
//
// ONE PAGE, NOT ONE BYTE. SQLite reads whole pages, and a file whose length is
// not a multiple of the page size is a different failure (a short read) from a
// file with a page missing (a broken b-tree). The page size is read out of the
// file rather than assumed, because it is whatever the source store's was.
func TestTruncatingASnapshotByOnePageFailsItsIntegrityCheck(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	_ = putN(t, s, 200)

	dst := filepath.Join(t.TempDir(), "snapshot.db")
	if _, err := s.Snapshot(context.Background(), dst); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	pageSize := scalar[int64](t, openSnapshot(t, dst), "PRAGMA page_size")
	if pageSize <= 0 {
		t.Fatalf("the snapshot reports page_size %d, so this control cannot "+
			"remove a page and proves nothing", pageSize)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= pageSize {
		t.Fatalf("the snapshot is %d bytes at page size %d, which is too small "+
			"to lose a page and still be a database", info.Size(), pageSize)
	}
	if err := os.Truncate(dst, info.Size()-pageSize); err != nil {
		t.Fatal(err)
	}

	// A fresh connection: the one above has the old page count cached.
	db, err := sql.Open("sqlite", "file:"+dst)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var answer string
	err = db.QueryRow("PRAGMA integrity_check").Scan(&answer)
	if err == nil && answer == "ok" {
		t.Fatalf("a snapshot with its last %d-byte page removed still answers "+
			"integrity_check \"ok\", so the pass in "+
			"TestASnapshotCarriesEveryRecordTheStoreHeld asserts nothing", pageSize)
	}
	t.Logf("truncated by one %d-byte page: integrity_check answered %q, err %v",
		pageSize, answer, err)
}

// TEST 2 OF PLAN.md SECTION 46. A snapshot taken while another connection
// holds an OPEN WRITE TRANSACTION returns without error, and holds either N or
// N+1 records - never a torn state.
//
// ⛔ THE TEST'S JOB IS THAT NO SQLITE_BUSY ESCAPES. Section 46 owes this row no
// red control, because consistency is `VACUUM INTO`'s own guarantee and not
// something this package implements. What IS this package's is that the store
// is opened in a mode where a concurrent writer does not turn a backup into a
// failed call: WAL plus busy_timeout(5000), both set in open().
//
// THE WRITER IS A SECOND *Store ON THE SAME FILE, not a second transaction on
// the same handle. Two transactions on one *sql.DB is a pool question rather
// than a locking one, and the case worth proving is the real one - rigd holding
// the store while something else writes to it.
func TestASnapshotTakenUnderAnOpenWriteTransactionDoesNotFail(t *testing.T) {
	const n = 9
	name := estate(t, "development")
	s := openStore(t, name)
	_ = putN(t, s, n)

	writer, err := Open(name)
	if err != nil {
		t.Fatalf("opening a second connection to the same store: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// The write transaction is opened and HELD across the snapshot. _txlock is
	// immediate on this DSN, so the write lock is taken at BEGIN rather than at
	// the first statement: by the time the snapshot runs, the writer really
	// does hold it.
	tx, err := writer.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("beginning the held write transaction: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO records (id, version, kind, project, body, fields,
			session, seat, epoch, created_at)
		 VALUES ('held-open', 1, 'note', 'rig', 'uncommitted', '{}', 's', 'backup', 1, 0)`,
	); err != nil {
		_ = tx.Rollback()
		t.Fatalf("writing inside the held transaction: %v", err)
	}

	var (
		wg      sync.WaitGroup
		snap    Snapshot
		snapErr error
	)
	dst := filepath.Join(t.TempDir(), "snapshot.db")
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		snap, snapErr = s.Snapshot(ctx, dst)
	}()

	// The snapshot runs while the transaction is open. It is given a moment to
	// reach the lock before the transaction is released, so the overlap is real
	// rather than a race this test happens to win.
	time.Sleep(200 * time.Millisecond)
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rolling the held transaction back: %v", err)
	}
	wg.Wait()

	if snapErr != nil {
		t.Fatalf("a snapshot taken under an open write transaction failed: %v\n"+
			"       that is the SQLITE_BUSY this row exists to refuse: a backup "+
			"that fails whenever somebody is writing is a backup nobody can "+
			"schedule", snapErr)
	}
	if snap.Records != n && snap.Records != n+1 {
		t.Errorf("the snapshot holds %d records; only %d or %d are consistent "+
			"states, and anything else is a torn read", snap.Records, n, n+1)
	}

	db := openSnapshot(t, dst)
	if got := scalar[string](t, db, "PRAGMA integrity_check"); got != "ok" {
		t.Errorf("integrity_check on a snapshot taken under a live writer "+
			"answered %q, want \"ok\"", got)
	}
	if got := scalar[uint64](t, db, "SELECT COUNT(*) FROM records"); got != snap.Records {
		t.Errorf("the snapshot FILE holds %d records and the struct said %d",
			got, snap.Records)
	}
}

// An empty destination is refused by name rather than defaulted, and a
// relative one with it: PLAN.md section 38's fourth standing rule is that no
// caller path is joined unvalidated, and these are the two shapes this
// function can check knowing nothing about the caller.
func TestASnapshotRefusesADestinationItCannotCheck(t *testing.T) {
	s := openStore(t, estate(t, "development"))

	if _, err := s.Snapshot(context.Background(), ""); !errors.Is(err, ErrSnapshotPathEmpty) {
		t.Errorf("Snapshot(\"\") answered %v, want ErrSnapshotPathEmpty", err)
	}
	if _, err := s.Snapshot(context.Background(), "snapshot.db"); err == nil {
		t.Error("Snapshot took a relative path, so where the file landed " +
			"depends on the daemon's working directory, which no caller of this can see")
	}
}
