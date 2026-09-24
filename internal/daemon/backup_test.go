package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/backup"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// The backup verb over the real wire (PLAN.md section 46).
//
// ⛔ THIS FILE IS WHERE internal/backup AND internal/record MEET, AND IT IS THE
// ONLY PLACE THEY MAY. internal/backup is standard library only so that cmd/rig
// can import it without linking modernc.org/sqlite, which means the archive
// package cannot check a single thing about the store it carries. This package
// imports both, so the two constants that must never drift are pinned here and
// the "restore it and query it" half of the acceptance test runs here.

const backupEstate = "backupwire"

// upBackupDaemon stands a NAMED estate up on its own state directory.
//
// ⛔ XDG_STATE_HOME IS REDIRECTED AND THAT IS NOT BOILERPLATE. paths.BackupDir
// resolves through it, so a test that skipped this would write an archive of
// its fixture store into the developer's own ~/.local/state/rig/backups. It is
// also what gives the estate its store at all: record.Open resolves
// paths.EstateStateDir(name) under the same root.
func upBackupDaemon(t *testing.T) (string, *Daemon) {
	t.Helper()
	// Short, deliberately: sun_path is 108 bytes and t.TempDir under a long
	// TMPDIR silently exceeds it, failing as EINVAL rather than as a path
	// length. The same trap is recorded in upDaemonLogged.
	dir, err := os.MkdirTemp("", "rigb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{
		Version: "0.0.0-test", Wire: "v1", Lock: lock, Estate: backupEstate, Epoch: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.records == nil {
		t.Fatalf("estate %q opened no record store, so nothing below tests "+
			"anything", backupEstate)
	}
	t.Cleanup(func() { _ = d.records.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock, d
}

// putNote writes one note and answers its id. A non-zero ifVersion supersedes
// the record at that version, which is how this file makes `records` and
// `heads` different numbers.
func putNote(t *testing.T, c *client.Client, id string, ifVersion, n uint64) string {
	t.Helper()
	var resp verbsv1.RecordPutResponse
	if err := c.Call(recordCtx(t), "rig.record.put", &verbsv1.RecordPutRequest{
		Id: id, IfVersion: ifVersion, Kind: "note", Project: "rig",
		Body:   "a note the archive has to carry",
		Fields: map[string]string{"n": strconv.FormatUint(n, 10)},
	}, &resp); err != nil {
		t.Fatalf("rig.record.put(%q): %v", id, err)
	}
	return resp.GetRecord().GetId()
}

// TestTheArchivesMemberNameIsTheStoresOwnFileName is the pin section 46's
// member table needs and cannot state in either package.
//
// ⛔ internal/backup MAY NOT IMPORT internal/record, so backup.RecordMember is
// a separate constant from record.DBName and nothing in either package can
// notice them diverging. A rename in one would produce an archive whose member
// a restore places under a name the daemon does not open, and every test in
// both packages would stay green. This is the only place both names are
// visible at once, which is the same shape as the client reading the daemon's
// deadline in a test so two numbers cannot drift.
func TestTheArchivesMemberNameIsTheStoresOwnFileName(t *testing.T) {
	if backup.RecordMember != record.DBName {
		t.Errorf("the archive carries the store as %q and the store opens %q.\n"+
			"       A restore would place the file under a name the daemon never "+
			"opens, and the estate would come up empty with nothing reporting an "+
			"error.", backup.RecordMember, record.DBName)
	}
}

// TestBackupCreateAnswersWithTheArchiveItActuallyWrote is acceptance test 10.
//
// ⛔ EVERY FIELD IS CHECKED AGAINST THE FILE ON DISK RATHER THAN AGAINST
// ANOTHER FIELD OF THE SAME REPLY. Section 46 binds `path` and `sha256` and
// owes TWO mutations each - empty and wrong-non-empty - because protojson drops
// the empty string, so a field that never travelled is indistinguishable from
// one deliberately left empty. A test that compared the reply to itself would
// survive both. Comparing to the bytes on disk survives neither.
func TestBackupCreateAnswersWithTheArchiveItActuallyWrote(t *testing.T) {
	sock, _ := upBackupDaemon(t)
	c := seated(t, sock, "backup-test")

	const kinds = 9
	for i := range kinds {
		putNote(t, c, "", 0, uint64(i))
	}

	var resp verbsv1.BackupCreateResponse
	if err := c.Call(recordCtx(t), "rig.backup.create",
		&verbsv1.BackupCreateRequest{}, &resp); err != nil {
		t.Fatalf("rig.backup.create: %v", err)
	}

	// `path` travelled, and it names a file that is really there.
	if resp.GetPath() == "" {
		t.Fatal("the reply carries no path, so the caller has no way to find " +
			"the archive that was just written")
	}
	info, err := os.Stat(resp.GetPath())
	if err != nil {
		t.Fatalf("the reply named %q and there is no such file: %v",
			resp.GetPath(), err)
	}

	// And it is the path decision 6 specifies, not merely A path.
	dir, err := paths.BackupDir()
	if err != nil {
		t.Fatalf("paths.BackupDir: %v", err)
	}
	if got := filepath.Dir(resp.GetPath()); got != dir {
		t.Errorf("the archive landed in %q and section 46 decision 6 puts it in "+
			"%q", got, dir)
	}
	name := filepath.Base(resp.GetPath())
	if !strings.HasPrefix(name, backupEstate+"-") || !strings.HasSuffix(name, ".tar.gz") {
		t.Errorf("the archive is named %q, want <estate>-<UTC stamp>.tar.gz", name)
	}

	// `sha256` travelled, and it is the digest of those bytes.
	raw, err := os.ReadFile(resp.GetPath())
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	digest := sha256.Sum256(raw)
	if want := hex.EncodeToString(digest[:]); resp.GetSha256() != want {
		t.Errorf("the reply says sha256 %q and the file hashes to %q.\n"+
			"       This is the number a person runs sha256sum against after "+
			"copying the archive off the machine.", resp.GetSha256(), want)
	}
	if resp.GetBytes() != uint64(info.Size()) {
		t.Errorf("the reply says %d bytes and the file is %d",
			resp.GetBytes(), info.Size())
	}

	// The four numbers, against the manifest inside the archive.
	m, err := backup.Read(resp.GetPath())
	if err != nil {
		t.Fatalf("reading back the archive the daemon just wrote: %v", err)
	}
	for _, c := range []struct {
		field     string
		got, want uint64
	}{
		{"heads", resp.GetHeads(), m.Heads},
		{"records", resp.GetRecords(), m.Records},
		{"schema_version", resp.GetSchemaVersion(), m.SchemaVersion},
	} {
		if c.got != c.want {
			t.Errorf("the reply says %s %d and the manifest says %d",
				c.field, c.got, c.want)
		}
	}
	if resp.GetCreatedUnixNano() != m.CreatedUnixNano {
		t.Errorf("the reply says created_unix_nano %d and the manifest says %d",
			resp.GetCreatedUnixNano(), m.CreatedUnixNano)
	}
	if m.Heads != kinds {
		t.Errorf("the manifest reports %d heads and %d records were written",
			m.Heads, kinds)
	}
	if m.RigVersion != "0.0.0-test" || m.Wire != "v1" || m.Estate != backupEstate {
		t.Errorf("the manifest names the writer as version %q wire %q estate %q, "+
			"and this daemon is 0.0.0-test / v1 / %s",
			m.RigVersion, m.Wire, m.Estate, backupEstate)
	}

	// Nothing was left beside the archive: the snapshot's working directory is
	// the daemon's own and does not survive the call.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		var left []string
		for _, e := range entries {
			left = append(left, e.Name())
		}
		t.Errorf("%s holds %v, want just the archive", dir, left)
	}
}

// TestARestoredArchiveAnswersTheSameQueryTheOriginalDid is acceptance test 3's
// other half, and it is the clause section 44 calls the real test: THE RESTORE
// IS THE TEST, NOT THE BACKUP.
//
// The archive is written by a real daemon from a real store, restored by
// internal/backup into an estate directory nothing has ever run in, and then
// opened by internal/record and QUERIED. Anything short of that - checking the
// digests, checking the manifest - proves a file was copied, not that an estate
// came back.
func TestARestoredArchiveAnswersTheSameQueryTheOriginalDid(t *testing.T) {
	sock, _ := upBackupDaemon(t)
	c := seated(t, sock, "backup-test")

	const written = 12
	ids := make([]string, 0, written)
	for i := range written {
		ids = append(ids, putNote(t, c, "", 0, uint64(i)))
	}
	// Superseding makes `records` and `heads` DIFFERENT numbers, so a restore
	// that carried one table and answered with the other could not pass.
	for _, id := range ids[:5] {
		putNote(t, c, id, 1, 99)
	}

	var resp verbsv1.BackupCreateResponse
	if err := c.Call(recordCtx(t), "rig.backup.create",
		&verbsv1.BackupCreateRequest{}, &resp); err != nil {
		t.Fatalf("rig.backup.create: %v", err)
	}
	if resp.GetRecords() <= resp.GetHeads() {
		t.Fatalf("the snapshot reports %d records and %d heads; five of twelve "+
			"were superseded, so this test is no longer telling the two tables "+
			"apart", resp.GetRecords(), resp.GetHeads())
	}

	// A DIFFERENT state root, so the restored estate is somewhere no daemon has
	// ever run and nothing can be inherited from the one that wrote it.
	fresh := t.TempDir()
	t.Setenv("XDG_STATE_HOME", fresh)
	dir, err := paths.EstateStateDir("restored")
	if err != nil {
		t.Fatalf("paths.EstateStateDir: %v", err)
	}
	res, err := backup.Restore(backup.RestoreRequest{
		Archive:       resp.GetPath(),
		Dir:           dir,
		SchemaCeiling: uint64(record.SchemaVersion),
		At:            time.Now(),
	})
	if err != nil {
		t.Fatalf("restoring into a fresh estate: %v", err)
	}

	st, err := record.Open("restored")
	if err != nil {
		t.Fatalf("opening the restored estate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	got, err := st.Find(context.Background(), record.QueryFilter{})
	if err != nil {
		t.Fatalf("querying the restored estate: %v", err)
	}
	if uint64(len(got)) != res.Manifest.Heads {
		t.Errorf("the restored estate answers %d heads and the manifest says "+
			"%d.\n"+
			"       record.query answers HEADS, so this is the number section "+
			"46's acceptance clause compares.", len(got), res.Manifest.Heads)
	}
	if uint64(len(got)) != resp.GetHeads() {
		t.Errorf("the restored estate answers %d heads and the daemon reported "+
			"%d when it took the backup", len(got), resp.GetHeads())
	}
}

// TestBackupCreateRefusesAnUnnamedEstate is section 46 decision 11 as
// corrected.
//
// ⛔ THE CORRECTION MATTERS AND THIS TEST IS WHY IT WAS FOUND. `recordStore` no
// longer refuses an unnamed estate: since the scratch store landed on
// 2026-09-17 an unnamed estate opens record.OpenScratch(), so d.records is
// non-nil and recordStore hands it over. The refusal is d.estate == "", checked
// after recordStore so the store-did-not-open message stays in one place.
func TestBackupCreateRefusesAnUnnamedEstate(t *testing.T) {
	sock, d := upDaemon(t, nil)
	if d.records == nil {
		t.Skip("this build gives an unnamed estate no scratch store, so " +
			"recordStore refuses first and decision 11 is not what is under test")
	}

	c := dial(t, sock)
	err := c.Call(recordCtx(t), "rig.backup.create",
		&verbsv1.BackupCreateRequest{}, &verbsv1.BackupCreateResponse{})
	if err == nil {
		t.Fatal("an unnamed estate was backed up. Its store is discarded at " +
			"every start, so the archive would describe a store that will not " +
			"exist tomorrow")
	}
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("the refusal is %v, and it must be a wire refusal", err)
	}
	if ce.Code() != rigv1.Code_CODE_UNAVAILABLE {
		t.Errorf("the refusal carries code %v, want CODE_UNAVAILABLE", ce.Code())
	}
	if !strings.Contains(err.Error(), "unnamed") {
		t.Errorf("the refusal does not say the estate is unnamed, so the caller "+
			"is left to guess: %v", err)
	}
}

// TestTwoBackupsInOneSecondRefuseRatherThanOverwrite is the one thing this verb
// could do that would be worse than failing.
//
// The archive name carries a second-resolution UTC stamp, so two calls inside
// one second choose the same name. Write opens the file exclusively, so the
// second one is REFUSED - it does not land on the first. The clock is an
// argument to createBackup for exactly this reason: the collision is produced
// rather than raced for.
func TestTwoBackupsInOneSecondRefuseRatherThanOverwrite(t *testing.T) {
	_, d := upBackupDaemon(t)
	at := time.Date(2026, 9, 23, 14, 5, 6, 0, time.UTC)

	first, err := d.createBackup(context.Background(), d.records, at)
	if err != nil {
		t.Fatalf("the first backup: %v", err)
	}
	before, err := os.ReadFile(first.GetPath())
	if err != nil {
		t.Fatalf("reading the first archive: %v", err)
	}

	if _, err := d.createBackup(context.Background(), d.records, at); err == nil {
		t.Fatal("a second backup at the same instant was accepted, so it landed " +
			"on the first")
	}
	after, err := os.ReadFile(first.GetPath())
	if err != nil {
		t.Fatalf("reading the first archive back: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the first archive changed: it was %d bytes and is now %d",
			len(before), len(after))
	}
}
