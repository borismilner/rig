package record

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
)

// Snapshot is a consistent copy of the store on disk, and every number in it
// is read BACK OUT OF THE COPY rather than off the live store or off a
// constant in this process.
//
// ⛔ THE WHOLE REASON THIS TYPE EXISTS IS THAT A FILESYSTEM COPY OF THIS STORE
// IS NOT A BACKUP. The store runs in WAL mode (see open, above), so a
// committed write lives in record.db-wal until a checkpoint folds it into
// record.db, and a copy of the main file alone carries none of the frames
// still in the WAL. Measured 2026-09-23 on the production estate: record.db
// 6.7 MB modified four days earlier, record.db-wal 4.2 MB modified that
// morning. Every write of those four days was in the file the hand copies did
// not take. PLAN.md section 46 carries the table.
//
// THE NUMBERS ARE READ FROM THE SNAPSHOT AND THAT IS A REQUIREMENT, NOT
// TIDINESS. A manifest whose schema version came from SchemaVersion would
// describe the binary that wrote the archive rather than the bytes inside it,
// and the two disagree exactly when it matters: a store carried forward by a
// migration that half-ran is the case store.go calls the silent failure. A
// number read out of the artefact can be wrong about the world; a number read
// out of the writer cannot even be wrong about the artefact.
type Snapshot struct {
	// Path is the file that was written.
	Path string

	// SchemaVersion is the snapshot's own PRAGMA user_version.
	SchemaVersion uint32

	// Records is the row count in `records`, which counts EVERY VERSION
	// including every superseded one.
	Records uint64

	// Heads is the row count in `heads`, which is one row per record.
	//
	// ⛔ IT IS THE NUMBER A RESTORE IS CHECKED AGAINST, because record.query
	// answers heads. Generation 22 measured 2,568 rows against 1,057 heads in
	// this store while a document said "2,538 records" and was counting
	// neither. The two are carried separately so nobody has to guess which one
	// a count means.
	Heads uint64
}

// ErrSnapshotPathEmpty refuses a snapshot with nowhere to go.
//
// It is refused rather than defaulted because every default is a path the
// caller did not name, and PLAN.md section 38's fourth standing rule is that
// no caller path is joined unvalidated. An empty destination is the one case
// where there is nothing to validate and nothing to guess.
var ErrSnapshotPathEmpty = errors.New(
	"record: a snapshot needs a destination file and none was given; rig does " +
		"not choose one, because a path nobody named is a path nobody checked")

// Snapshot writes a consistent copy of this store to dst and describes it.
//
// ⛔ IT IS `VACUUM INTO` AND THE ALTERNATIVES WERE RULED OUT ON MECHANISM
// (PLAN.md section 46, decision 2). The statement is transactional, so the
// output is a consistent image of the database under WAL; it does not block a
// writer; the result carries no -wal or -shm sidecar, so the copy is ONE file
// rather than three that have to be kept in step; and it is compacted on the
// way out. The named fallback is modernc's own online backup API, which needs
// a raw driver connection to reach, and that is why it is the second choice.
//
// ⛔ THE DESTINATION IS A BOUND PARAMETER AND NEVER SPLICED INTO THE SQL.
// SQLite's INTO clause takes an expression, so `?` works, and that removes the
// whole class of question about what a path containing a quote does. Nothing
// in this package builds SQL by concatenation around a caller's value, and
// this is the one statement where a caller's value reaches SQL at all.
//
// THE FILE MUST NOT EXIST. SQLite refuses to vacuum into an existing file,
// which is a property worth keeping rather than working around: the caller
// names a fresh path in a directory it owns, so a snapshot can never silently
// land on top of something.
func (s *Store) Snapshot(ctx context.Context, dst string) (Snapshot, error) {
	if dst == "" {
		return Snapshot{}, ErrSnapshotPathEmpty
	}
	if !filepath.IsAbs(dst) {
		return Snapshot{}, fmt.Errorf(
			"record: a snapshot destination must be an absolute path and %q is "+
				"not; the daemon's working directory is not a thing a caller of "+
				"this can see", dst)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dst); err != nil {
		return Snapshot{}, fmt.Errorf("record: snapshotting %s into %s: %w",
			s.path, dst, err)
	}
	return describeSnapshot(ctx, dst)
}

// describeSnapshot reads the three numbers back out of the file just written.
//
// It opens a SECOND connection to the snapshot rather than asking the live
// store, and that is the point of the function: a count taken from the live
// store races the next writer, so it would describe neither the snapshot nor
// the store as it is now. Read from the copy, the numbers are true of the
// bytes the archive will carry, whatever happens next.
//
// query_only is set so this connection cannot write to the artefact it is
// describing, including the recovery a plain open would perform.
func describeSnapshot(ctx context.Context, path string) (Snapshot, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=query_only(1)")
	if err != nil {
		return Snapshot{}, fmt.Errorf("record: reading back the snapshot at %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	snap := Snapshot{Path: path}
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&snap.SchemaVersion); err != nil {
		return Snapshot{}, fmt.Errorf("record: reading the snapshot's user_version at %s: %w", path, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM records").Scan(&snap.Records); err != nil {
		return Snapshot{}, fmt.Errorf("record: counting the snapshot's records at %s: %w", path, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM heads").Scan(&snap.Heads); err != nil {
		return Snapshot{}, fmt.Errorf("record: counting the snapshot's heads at %s: %w", path, err)
	}
	return snap, nil
}
