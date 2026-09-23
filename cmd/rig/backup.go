package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// cmdBackupOrRestore is section 46's single seam into run's dispatch switch.
//
// ⛔ IT EXISTS BECAUSE `run` HAS ONE ARM TO SPEND, NOT BECAUSE THE TWO VERBS
// SHARE ANYTHING. They share nothing: `backup` dials the daemon and `restore`
// deliberately never does (decision 7). Two case arms took run's cyclomatic
// complexity to 26 against gocyclo's ceiling of 25, in a function this
// capability does not own - so the branch is here, in the capability's own
// file, which is also where "entries yes, helpers no" says it belongs.
//
// The verb is passed rather than re-derived: run has already stripped it, and
// a second reading of argv is a second place for the two to disagree.
func cmdBackupOrRestore(verb string, args []string) error {
	if verb == "restore" {
		return cmdRestore(args)
	}
	return cmdBackup(args)
}

// cmdBackup asks the daemon to archive the estate this shell reached
// (PLAN.md section 46).
//
// ⛔ THE CALLER NAMES NOTHING AND THAT IS THE DESIGN, NOT AN OMISSION.
// Decision 6: an empty request has nothing for a client and a daemon to
// disagree about, which is how section 38's fourth standing rule - no caller
// path joined unvalidated - is met here rather than by a validator. The daemon
// chooses the directory and the name; this prints where it landed.
//
// ⛔ AND THE VERB EXISTS BECAUSE `cp record.db` WAS NEVER A BACKUP. The store
// runs in WAL mode, so a committed write lives in record.db-wal until a
// checkpoint folds it into record.db. Measured on the production estate,
// 2026-09-23: record.db 6.7 MB modified four days earlier, record.db-wal 4.2 MB
// modified that morning. Every write of those four days was in the file the
// hand copies did not take. A backup of a WAL database is taken through SQLite,
// which is a thing only the daemon links.
func cmdBackup(args []string) (err error) {
	bf := backupFlagSet()
	flags, positional := partition(args)
	if err := bf.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *bf.asJSON) }()
	if len(positional) != 0 {
		// Refused rather than ignored: a path typed here is a caller expecting
		// to choose where the archive goes, and silently writing somewhere else
		// would be worse than saying no.
		return badArgumentf("usage: rig backup [--json] [--timeout=30s]\n" +
			"       rig chooses the archive's name and location and prints " +
			"where it landed; there is no path to give")
	}

	c, err := connect()
	if err != nil {
		// Not a form of the answer: nothing was archived, and reporting the
		// directory rig WOULD have written to is the guess this refusal exists
		// to remove.
		return noDaemon(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *bf.timeout)
	defer cancel()

	resp := &rigv1.BackupCreateResponse{}
	if err := call(ctx, c, "rig.backup.create", &rigv1.BackupCreateRequest{}, resp); err != nil {
		return err
	}

	if *bf.asJSON {
		return json.NewEncoder(os.Stdout).Encode(backupJSON(resp))
	}
	fmt.Print(backupText(resp))
	return nil
}

// backupFlags is `backup`'s flag set, carried on a struct rather than returned
// as a bare *flag.FlagSet.
//
// THAT IS THE `cli` SEAT'S PRECEDENT AND IT IS LOAD-BEARING.
// cmd/rig/depth_test.go walks every top-level function in this package that
// returns a *flag.FlagSet and fails the build for one it does not itself list
// in a hand-kept map - a map this seat does not own. briefFlagSet and
// recordFlagSet already answer that by returning their own type, and so does
// this. The coupling to valuedFlags that the walk exists to enforce is asserted
// in backup_test.go instead, over this seat's own sets.
type backupFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
}

func backupFlagSet() *backupFlags {
	b := &backupFlags{fs: flag.NewFlagSet("backup", flag.ContinueOnError)}
	b.asJSON = b.fs.Bool("json", false, "emit JSON")
	b.timeout = b.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	return b
}

// backupJSON is the object --json emits.
//
// Section 10: the proto's own field names are the contract, spelled as the wire
// spells them rather than in the lowerCamelCase protojson would produce. Every
// key is present on every answer, with no omitempty: an absent key reads as
// "this was never considered", and all seven were.
func backupJSON(r *rigv1.BackupCreateResponse) map[string]any {
	return map[string]any{
		"path":              r.GetPath(),
		"sha256":            r.GetSha256(),
		"bytes":             r.GetBytes(),
		"schema_version":    r.GetSchemaVersion(),
		"records":           r.GetRecords(),
		"heads":             r.GetHeads(),
		"created_unix_nano": r.GetCreatedUnixNano(),
	}
}

// backupText is the human rendering, a function of its own so every line is
// testable without a live daemon.
//
// ⛔ THE HEAD COUNT AND THE ROW COUNT ARE BOTH PRINTED AND ARE LABELLED APART.
// `rig record query` answers HEADS and the row count includes every superseded
// version, so a reader given one number cannot tell which question it answers.
// Generation 22 measured 2,568 rows against 1,057 heads in this store while a
// document said "2,538 records" and was counting neither.
func backupText(r *rigv1.BackupCreateResponse) string {
	var b strings.Builder
	row := func(label, value string) {
		fmt.Fprintf(&b, "%-*s%s\n", backupColumn, label, value)
	}
	row("archive", r.GetPath())
	row("bytes", strconv.FormatUint(r.GetBytes(), 10))
	row("sha256", r.GetSha256())
	row("schema", strconv.FormatUint(r.GetSchemaVersion(), 10))
	row("records", strconv.FormatUint(r.GetRecords(), 10)+
		" (every version, including superseded ones)")
	row("heads", strconv.FormatUint(r.GetHeads(), 10)+
		" (what rig record query answers)")
	row("taken", backupTakenCell(r))
	return b.String()
}

// backupColumn is the column a value starts in. Seven is the longest label
// ("archive", "records") plus two spaces of gutter.
const backupColumn = 7 + 2

// backupTakenCell renders the timestamp, and a zero is a CASE rather than a
// time.
//
// Printed through time.Unix a zero renders as 1970, which reads as a real
// moment and sends a reader looking at the clock. It can only mean the field
// did not travel, which is a defect in the daemon that answered rather than a
// fact about the archive - the same argument estateEpochCell spends a paragraph
// on, for the same class of mistake.
func backupTakenCell(r *rigv1.BackupCreateResponse) string {
	n := r.GetCreatedUnixNano()
	if n == 0 {
		return "(not said) - the daemon did not report when the snapshot was " +
			"taken, so this is a defect rather than a fact about the archive"
	}
	return time.Unix(0, n).UTC().Format(time.RFC3339)
}
