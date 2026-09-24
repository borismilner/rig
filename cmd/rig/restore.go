package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borismilner/rig/internal/backup"
	"github.com/borismilner/rig/internal/estate"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/paths"
)

// schemaCeiling is the record schema this build can restore, and it is a
// CONSTANT HERE rather than a reference to record.SchemaVersion.
//
// ⛔ THE REASON IS THE LAYERING AND NOT PREFERENCE. internal/record links
// modernc.org/sqlite, and TestTheClientLinksNeitherTheDaemonNorItsValidator
// exists to keep this binary clear of the daemon's dependencies; importing the
// store to read one integer would drag the whole driver into `rig`. So the
// number is carried here and cmd/rig/restore_test.go imports internal/record
// TEST-ONLY to assert the two are equal. That is the shape this package already
// uses for the daemon's call deadline and for internal/meta in layering_test.go:
// a test-only import costs the binary nothing and is the only way two numbers
// in two packages can be stopped from drifting.
//
// ⛔ BUMPING THE STORE'S SCHEMA WITHOUT BUMPING THIS ONE MAKES `rig restore`
// REFUSE EVERY FRESH ARCHIVE, and the test above is what turns that into a
// build failure instead of a support question.
const schemaCeiling = 2

// cmdRestore puts an archive back onto an estate (PLAN.md section 46).
//
// ⛔ IT IS OFFLINE AND IT NEVER CONNECTS TO A DAEMON (decision 7). A restore
// replaces the files a running daemon holds open, so it cannot be a verb on the
// wire: there is no moment at which a live rigd could serve the call and still
// be serving the same store afterwards. It takes the estate's OWN claim -
// instance.AcquireName on paths.EstateLock - which is the same claim rigd takes,
// so a daemon holding that estate on ANY runtime directory produces the same
// refusal, and two restores cannot interleave either.
//
// The CLI can do the whole job because a restore is file placement and
// verification: internal/backup is standard library only, and the daemon
// migrates an older schema on its next open, which Store.start already does.
func cmdRestore(args []string) (err error) {
	rf := restoreFlagSet()
	flags, positional := partition(args)
	if err := rf.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *rf.asJSON) }()

	if len(positional) != 1 {
		return badArgumentf("usage: rig restore --estate <name> [--force] "+
			"[--json] <archive>\n"+
			"       one archive, and %s given. The archive is the .tar.gz "+
			"rig backup printed the path of", countedArchives(len(positional)))
	}
	if *rf.estate == "" {
		return badArgumentf("rig restore: --estate is required.\n" +
			"       A restore writes over an estate's whole state directory, " +
			"so there is no estate it could sensibly default to - not the one " +
			"this shell reached, because a restore does not reach one at all")
	}
	if err := paths.ValidEstateName(*rf.estate); err != nil {
		return badArgumentf("rig restore: %v", err)
	}

	// ⛔ LEXICALLY VALID IS NOT THE SAME AS OPENABLE, AND THIS IS THE CHECK
	// THAT WAS MISSING (B114). paths.ValidEstateName above asks whether the
	// name is a safe path component; internal/estate asks whether any daemon
	// will ever open it. Without the second, `rig restore --estate b` exited 0
	// and told the reader to run `rigd --estate b`, which exits 1 - measured
	// with two real binaries 2026-09-23. Both checks are cheap and both come
	// before the claim, so a refused name reaches neither the lock nor the
	// disk.
	if err := estate.CheckName(*rf.estate); err != nil {
		return badArgumentf("rig restore: %v.\n"+
			"       Restoring into any other name writes a whole estate "+
			"directory no rigd will ever open", err)
	}

	// ⛔ THE CLAIM COMES BEFORE ANY LOOK AT THE DISK. rigd takes this same
	// claim, so holding it is how a running daemon is detected - and detecting
	// it afterwards would mean having already read, and possibly staged, state
	// a live daemon is writing to.
	lockPath, err := paths.EstateLock(*rf.estate)
	if err != nil {
		return badArgumentf("rig restore: %v", err)
	}
	lock, err := instance.AcquireName(lockPath, *rf.estate)
	if err != nil {
		return restoreRefusal(err)
	}
	defer func() { _ = lock.Close() }()

	dir, err := paths.EstateStateDir(*rf.estate)
	if err != nil {
		return badArgumentf("rig restore: %v", err)
	}

	res, err := backup.Restore(backup.RestoreRequest{
		Archive:       positional[0],
		Dir:           dir,
		Force:         *rf.force,
		SchemaCeiling: schemaCeiling,
		At:            time.Now(),
	})
	if err != nil {
		return restoreRefusal(err)
	}

	if *rf.asJSON {
		return json.NewEncoder(os.Stdout).Encode(restoreJSON(*rf.estate, res))
	}
	fmt.Print(restoreText(*rf.estate, res))
	return nil
}

// countedArchives is the phrase for how many positionals arrived, written out
// because "0 given" reads as a count of something the reader did type.
func countedArchives(n int) string {
	if n == 0 {
		return "none was"
	}
	return strconv.Itoa(n) + " were"
}

// restoreFlags is `restore`'s flag set. See backupFlags for why it is a struct.
type restoreFlags struct {
	fs     *flag.FlagSet
	estate *string
	force  *bool
	asJSON *bool
}

func restoreFlagSet() *restoreFlags {
	r := &restoreFlags{fs: flag.NewFlagSet("restore", flag.ContinueOnError)}
	r.estate = r.fs.String("estate", "", "which estate to restore into")
	r.force = r.fs.Bool("force", false,
		"move existing state aside to <name>.replaced-<stamp> first")
	r.asJSON = r.fs.Bool("json", false, "emit JSON")
	// NO --timeout, and the absence is a fact rather than an omission: this
	// verb reaches no daemon and waits for nothing, so a deadline would be a
	// flag that does nothing.
	return r
}

// restoreRefusal renders the refusals this verb produces itself, each with the
// precondition, the actual state and the fix section 9 asks for.
//
// ⛔ EVERY ONE OF THEM IS A REFUSAL A PERSON MEETS WITH A REAL ARCHIVE IN HAND
// AND SOMETHING AT STAKE, which is why each carries a fix command rather than
// only a sentence. A restore refused with prose alone is a person about to try
// something more drastic.
func restoreRefusal(err error) error {
	var held *instance.NameHeldError
	if errors.As(err, &held) {
		return local(jsonStatus{
			Code:         codeBadArgument,
			Message:      err.Error(),
			Precondition: "no daemon is running the estate being restored",
			Actual: "estate " + held.Name + " is claimed by process " +
				strconv.Itoa(held.Incumbent),
			Fix: "stop that daemon, then run this again. A restore replaces " +
				"the files a running daemon holds open, so it is offline by " +
				"construction",
			FixCommand: "rig down",
		})
	}

	var exists *backup.DirExistsError
	if errors.As(err, &exists) {
		return local(jsonStatus{
			Code:         codeBadArgument,
			Message:      err.Error(),
			Precondition: "the estate has no state, or --force was given",
			Actual:       exists.Dir + " already holds state",
			Fix: "pass --force. The directory is RENAMED whole to " +
				"<name>.replaced-<stamp> and nothing is deleted, so this is " +
				"undone by renaming it back",
			FixCommand: "rig restore --force",
		})
	}

	var newer *backup.SchemaTooNewError
	if errors.As(err, &newer) {
		return local(jsonStatus{
			Code:         codeBadArgument,
			Message:      err.Error(),
			Precondition: "the archive's schema is one this build can read",
			Actual: "the archive is schema " + strconv.FormatUint(newer.Found, 10) +
				" and this build reads up to " + strconv.FormatUint(newer.Ceiling, 10),
			Fix: "restore it with the rig that wrote it. An OLDER archive is " +
				"fine and is migrated when the daemon next opens the store; " +
				"only a newer one is refused",
			FixCommand: "rig version",
		})
	}

	var integrity *backup.IntegrityError
	if errors.As(err, &integrity) {
		return local(jsonStatus{
			Code:         codeBadResult,
			Message:      err.Error(),
			Precondition: "every member matches the digest its manifest declares",
			Actual:       "member " + integrity.Member + " does not",
			Fix: "check the copy. Nothing was restored and the estate was not " +
				"touched; the bytes that were extracted are left beside it, " +
				"named .restoring-<stamp>, rather than deleted",
			FixCommand: "sha256sum <archive>",
		})
	}

	return local(jsonStatus{
		Code:         codeBadArgument,
		Message:      err.Error(),
		Precondition: "the archive is one this build can read whole",
		Actual:       "it is not",
		Fix: "nothing was restored and the estate was not touched. A rig " +
			"archive carries manifest.json and record.db and nothing else",
		FixCommand: "tar tzf <archive>",
	})
}

// estateKey is the `estate` key in this verb's --json object.
//
// A CONSTANT ONLY BECAUSE goconst COUNTS, and the count is a fact about the
// whole package rather than about this file: `estate` reaches six occurrences
// across cmd/rig the moment section 46 lands, and six is the threshold
// (.golangci.yml:81). complete.go's verbVersion comment already draws the line
// this respects - an output key is a contract with whoever reads --json and a
// verb is an input the dispatcher matches on, so the two are NOT given one
// name. This is the output key alone.
const estateKey = "estate"

// epochKey is the JSON key an epoch travels under, in every object this client
// emits that carries one - the estate, a seat, a lease and a record's
// provenance. One spelling for one fact.
const epochKey = "epoch"

// restoreJSON is the object --json emits.
//
// The parameter is `name` rather than `estate` because internal/estate is
// imported above and a parameter shadowing a package is how a later edit ends
// up calling a method on a string (gocritic importShadow, .golangci.yml).
func restoreJSON(name string, r backup.RestoreResult) map[string]any {
	return map[string]any{
		estateKey:           name,
		"dir":               r.Dir,
		"replaced":          r.Replaced,
		"heads":             r.Manifest.Heads,
		"records":           r.Manifest.Records,
		"schema_version":    r.Manifest.SchemaVersion,
		"created_unix_nano": r.Manifest.CreatedUnixNano,
		"rig_version":       r.Manifest.RigVersion,
		"from_estate":       r.Manifest.Estate,
	}
}

// restoreText is the human rendering, and it ENDS WITH THE CHECK TO RUN.
//
// ⛔ THAT LAST BLOCK IS THE POINT OF THE WHOLE VERB. Section 44: the restore is
// the test, not the backup - so a restore that reported success and left the
// reader to invent a way of verifying it would be the reassuring answer rather
// than the useful one. The head count is the number to compare because
// `rig record query` answers heads, and it is printed here so the reader does
// not have to go and find it. The check is the human query's LAST LINE, which
// counts heads; the --json answer is one line, so a `grep -c` over it printed
// 1 for every non-empty store. Measured 2026-09-24 in the lead's hand run.
func restoreText(name string, r backup.RestoreResult) string {
	var b strings.Builder
	row := func(label, value string) {
		fmt.Fprintf(&b, "%-*s%s\n", restoreColumn, label, value)
	}
	row("estate", name)
	row("at", r.Dir)
	row("from", restoreFromCell(r.Manifest))
	row("heads", strconv.FormatUint(r.Manifest.Heads, 10))
	row("records", strconv.FormatUint(r.Manifest.Records, 10))
	row("schema", strconv.FormatUint(r.Manifest.SchemaVersion, 10))
	if r.Replaced != "" {
		row("replaced", r.Replaced)
		fmt.Fprintf(&b, "\nThe previous state was RENAMED, not deleted. "+
			"To undo this restore:\n  mv %s %s\n", r.Replaced, r.Dir)
	}
	fmt.Fprintf(&b, "\nNow check it:\n  rigd --estate %s\n"+
		"  rig record query | tail -1   expect \"%d records\" - the query counts heads\n",
		name, r.Manifest.Heads)
	return b.String()
}

// restoreColumn is the column a value starts in. Eight is the longest label
// ("replaced") plus two spaces of gutter.
const restoreColumn = 8 + 2

// notSaid is what a field the manifest did not carry renders as, and it is a
// constant HERE rather than three literals.
//
// The spelling is peers.go's and record.go's, deliberately: a reader who has
// met it once on `rig peers` should not have to work out whether a second
// spelling means a second thing. It is not hoisted into a shared file because
// those two are other seats' rows - this constant is scoped to the verb that
// needs it, and the three uses below are what goconst counts.
const notSaid = "(not said)"

// restoreFromCell names the daemon that wrote the archive, so a reader can tell
// a restore of their own estate from a restore of somebody else's.
func restoreFromCell(m backup.Manifest) string {
	when := notSaid
	if m.CreatedUnixNano != 0 {
		when = time.Unix(0, m.CreatedUnixNano).UTC().Format(time.RFC3339)
	}
	from := m.Estate
	if from == "" {
		from = notSaid
	}
	version := m.RigVersion
	if version == "" {
		version = notSaid
	}
	return "estate " + from + ", taken " + when + ", by rig " + version
}
