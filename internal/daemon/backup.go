package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/boris-milner/rig/internal/backup"
	"github.com/boris-milner/rig/internal/paths"
	"github.com/boris-milner/rig/internal/record"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
	"google.golang.org/protobuf/proto"
)

// The backup verb (PLAN.md section 46).
//
// ⛔ THE DAEMON TAKES THE SNAPSHOT AND THE CLIENT NEVER OPENS THE DATABASE.
// Section 22 puts modernc.org/sqlite in rigd alone and cmd/rig/layering_test.go
// is the gate that keeps it there, so a `rig backup` that read the store itself
// would be a second opener of a file that must have exactly one - and would
// double the client's binary on the way. The daemon already holds the store
// open, which makes this the cheap side as well as the correct one.
//
// THE BULK PLANE SECTION 44 NAMED FOR S2 IS NOT SPENT HERE. The CLI and the
// daemon share one filesystem, so the archive is written to disk and the wire
// carries only its manifest: a path, a digest and four numbers. A 7 MB stream
// over the control plane would be the thing the bulk plane exists to avoid,
// and it is not needed to get the bytes to the person who asked.

// serveBackupCreate archives the estate this daemon is running.
//
// ⛔ THE ORDER OF THE TWO REFUSALS IS SPECIFIED, not incidental (section 46
// decision 11 as corrected). `recordStore` runs FIRST so the store-did-not-open
// refusal stays in one place and reads the same from every verb; the unnamed
// estate is refused AFTERWARDS, here, because `recordStore` no longer does it.
// Since the scratch store landed on 2026-09-17 an unnamed estate opens
// record.OpenScratch(), so d.records is non-nil and recordStore returns it
// happily. A backup of a store that is discarded at the next start would be an
// archive of nothing, which is why this is a refusal rather than an empty file.
func (d *Daemon) serveBackupCreate(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req rigv1.BackupCreateRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "backup.create: "+err.Error())
		return
	}
	st, ok := d.recordStore(c, f, "backup.create")
	if !ok {
		return
	}
	if d.estate == "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_UNAVAILABLE,
			"rig.backup.create: this estate is unnamed, so it has no persistent "+
				"state to back up. Its record store is a scratch store, discarded at "+
				"every start by design (PLAN.md section 37), and an archive of it "+
				"would describe a store that will not exist tomorrow. Name the estate "+
				"to back it up.")
		return
	}

	resp, err := d.createBackup(ctx, st, time.Now())
	if err != nil {
		c.failErr(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err)
		return
	}
	c.reply(f.GetStreamId(), resp)
}

// createBackup is the whole of the verb with no connection in it.
//
// Split out so a test can run the real thing - a real store, a real snapshot, a
// real archive on a real disk - without a socket, and so the clock is an
// argument rather than something a test has to work around.
func (d *Daemon) createBackup(ctx context.Context, st *record.Store, at time.Time) (*rigv1.BackupCreateResponse, error) {
	dir, err := paths.BackupDir()
	if err != nil {
		return nil, fmt.Errorf("backup.create: %w", err)
	}
	// 0700: an archive of the record store carries everything every seat has
	// written, and the directory it sits in is the user's alone.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("backup.create: making %s: %w", dir, err)
	}

	name, err := backup.ArchiveName(d.estate, at)
	if err != nil {
		return nil, fmt.Errorf("backup.create: %w", err)
	}
	archive := filepath.Join(dir, name)

	// The snapshot is taken into a private directory beside the archive rather
	// than into the estate's own, so nothing a daemon opens ever sees a
	// half-written database file with a name it might recognise.
	work, err := os.MkdirTemp(dir, ".snapshot-")
	if err != nil {
		return nil, fmt.Errorf("backup.create: making a working directory under "+
			"%s: %w", dir, err)
	}
	member := filepath.Join(work, backup.RecordMember)
	// ⛔ TWO NAMED REMOVES AND NOT os.RemoveAll. Both paths were built by this
	// function moments ago and nothing else has ever seen them, so there is
	// nothing here for a recursive delete to reach that a named one cannot -
	// and section 46 decision 8 keeps that verb out of this area entirely. A
	// leftover .snapshot-* directory after a crash is named for what it is.
	defer func() {
		_ = os.Remove(member)
		_ = os.Remove(work)
	}()

	snap, err := st.Snapshot(ctx, member)
	if err != nil {
		return nil, fmt.Errorf("backup.create: %w", err)
	}

	// EVERY NUMBER COMES OUT OF THE SNAPSHOT. Snapshot reads user_version and
	// both counts back from the file it just wrote through a second connection,
	// so the manifest describes the bytes the archive carries rather than the
	// binary that wrote it. record.SchemaVersion is deliberately not read here.
	written, err := backup.Write(archive, backup.Manifest{
		CreatedUnixNano: at.UnixNano(),
		Estate:          d.estate,
		Format:          backup.Format,
		Heads:           snap.Heads,
		Records:         snap.Records,
		RigVersion:      d.version,
		SchemaVersion:   uint64(snap.SchemaVersion),
		Wire:            d.wire,
	}, []backup.Source{{Name: backup.RecordMember, Path: member}})
	if err != nil {
		return nil, fmt.Errorf("backup.create: %w", err)
	}

	// ⛔ A NEGATIVE LENGTH IS NOT POSSIBLE AND IS STILL CHECKED. The wire field
	// is unsigned and the counter is signed, so the conversion is the one place
	// a length could change sign on its way to a caller. Nothing here can
	// produce it; the check costs one comparison and removes the question.
	if written.Bytes < 0 {
		return nil, fmt.Errorf("backup.create: the archive at %s measured %d "+
			"bytes, which is not a length", written.Path, written.Bytes)
	}

	return &rigv1.BackupCreateResponse{
		Path:            written.Path,
		Sha256:          written.SHA256,
		Bytes:           uint64(written.Bytes),
		SchemaVersion:   uint64(snap.SchemaVersion),
		Records:         snap.Records,
		Heads:           snap.Heads,
		CreatedUnixNano: at.UnixNano(),
	}, nil
}
