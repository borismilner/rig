package main

import (
	"flag"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// backupAnswer is a complete BackupCreateResponse, so a renderer test reads as a
// change from a known-good answer rather than as a pile of literals.
func backupAnswer() *rigv1.BackupCreateResponse {
	return &rigv1.BackupCreateResponse{
		Path:            "/home/x/.local/state/rig/backups/a-20260924T101112Z.tar.gz",
		Sha256:          strings.Repeat("ab", 32),
		Bytes:           4096,
		SchemaVersion:   2,
		Records:         22,
		Heads:           17,
		CreatedUnixNano: time.Date(2026, 9, 24, 10, 11, 12, 0, time.UTC).UnixNano(),
	}
}

// ---- argv ------------------------------------------------------------------

// `rig backup` takes NO positional and the refusal is the whole of decision 6
// reaching the person who typed one.
//
// ⛔ IT MUST BE REFUSED RATHER THAN IGNORED, and the difference is the one
// this test exists for: a path typed here is somebody asking for the archive
// to land somewhere they chose, and a command that accepts the word and writes
// elsewhere has answered a different question.
//
// ⛔ THE ISOLATION BELOW IS NOT BELT AND BRACES AND IT WAS ADDED ON A
// MEASUREMENT. cmdBackup dials, and an in-process test that reaches connect()
// reaches whatever daemon this MACHINE is running - the refusal being checked
// here is the only thing standing in front of it. Running the mutation that
// removes that refusal, this test dialled the live production daemon and came
// back with "no such method rig.backup.create"; it was harmless only because
// the deployed binary predates the verb, and against a redeployed one it would
// have asked for a real archive of a real estate. A test whose safety depends
// on the thing it is testing is not isolated, and COORDINATION.md says a
// private XDG_RUNTIME_DIR is exactly what contains the socket.
func TestBackupRefusesAPathBecauseTheDaemonChoosesTheName(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	err := cmdBackup([]string{"/tmp/somewhere.tar.gz"})
	if err == nil {
		t.Fatal("`rig backup /tmp/somewhere.tar.gz` was accepted, so a caller " +
			"who named a path was given an archive somewhere else")
	}
	for _, want := range []string{
		"usage: rig backup",
		"there is no path to give",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q, so it does not tell the "+
				"caller why their path was not used: %v", want, err)
		}
	}
}

// EVERY NON-BOOLEAN FLAG ON THESE TWO VERBS MUST BE IN valuedFlags.
//
// The coupling and its failure mode are written out on
// TestEveryFlagThatTakesAValueIsDeclaredToThePartitioner in depth_test.go,
// which walks the verbs that return a bare *flag.FlagSet. These two do not -
// they return a struct, on briefFlagSet's precedent - so that walk cannot see
// them and this is their half of it.
//
// ⛔ --estate IS WHY THIS IS NOT A FORMALITY, AND THE FORM THAT MATTERS IS THE
// QUIET ONE. Measured with `"estate"` removed from valuedFlags, in a detached
// copy at 2047aeb: `--estate b <archive>` dies loudly with "flag needs an
// argument: -estate", but `--estate a --force <archive>` does not fail at all
// at the parser - partition leaves --force as the next token, flag.Parse sets
// --estate to the string "--force", and `a` joins the archive in the
// positionals. valuedFlags is the only thing that makes the second case
// behave, which is why the subtests below cover the argv path and not just
// the map.
func TestEveryBackupFlagThatTakesAValueIsDeclaredToThePartitioner(t *testing.T) {
	sets := map[string]*flag.FlagSet{
		"backup":  backupFlagSet().fs,
		"restore": restoreFlagSet().fs,
	}

	var checked int
	for verb, fs := range sets {
		fs.VisitAll(func(f *flag.Flag) {
			// flag's own marker, which is what the parser reads, rather than
			// the name or the default.
			bf, ok := f.Value.(interface{ IsBoolFlag() bool })
			if ok && bf.IsBoolFlag() {
				return
			}
			checked++
			if !valuedFlags[f.Name] {
				t.Errorf("rig %s --%s takes a value and valuedFlags does not "+
					"list it, so partition() hands its value to the positionals",
					verb, f.Name)
			}
		})
	}

	// The positive control: every assertion above is about an absence, and an
	// absence is also what a VisitAll that visited nothing produces.
	if checked == 0 {
		t.Fatal("no non-boolean flag was examined, so this test would pass " +
			"against a walk that read nothing")
	}
	// And the second control, which the two walks before this one had to
	// learn: a count alone passes whether the map holds one set or both,
	// because dropping a set drops its flags from the count too.
	if len(sets) != 2 {
		t.Fatalf("the walk covers %d flag sets, want one for backup and one "+
			"for restore", len(sets))
	}
}

// THE ARGV PATH, which is the one that actually broke for --depth and is the
// only place --estate's failure mode above is visible.
func TestTheEstateFlagSurvivesPartitioningInEveryWrittenForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"space separated", []string{"--estate", "a", "x.tar.gz"}},
		{"equals form", []string{"--estate=a", "x.tar.gz"}},
		{"before the archive", []string{"--estate", "a", "--force", "x.tar.gz"}},
		{"after the archive", []string{"x.tar.gz", "--estate", "a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rf := restoreFlagSet()
			flags, positional := partition(tc.argv)
			if err := rf.fs.Parse(flags); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if *rf.estate != "a" {
				t.Errorf("--estate parsed as %q, want a: partition split %v "+
					"into flags %v and positionals %v",
					*rf.estate, tc.argv, flags, positional)
			}
			if len(positional) != 1 || positional[0] != "x.tar.gz" {
				t.Errorf("the positionals are %v, want exactly [x.tar.gz]: "+
					"the estate name was read as an archive", positional)
			}
		})
	}
}

// ---- completion ------------------------------------------------------------

// THE COMPLETION IS WALKED OFF THE FLAG SETS, not listed beside them -
// complete.go calls that coupling "one string in three places that MUST
// agree", and a hand-kept list is the fourth.
//
// ⛔ AND THE ABSENT --timeout IS AN ASSERTION, NOT AN OVERSIGHT. `rig restore`
// reaches no daemon and waits for nothing (decision 7), so a deadline flag
// would be one that does nothing. A completion offering it would be the first
// place a person learned otherwise, and they would learn something false.
func TestTheCompletionOffersSectionFortySixsTwoVerbs(t *testing.T) {
	top := strings.Join(candidates(nil), " ")
	for _, want := range []string{"backup", "restore"} {
		if !strings.Contains(top, want) {
			t.Errorf("`rig <TAB>` does not offer %q, so the verb is "+
				"dispatched and undiscoverable: %q", want, top)
		}
	}

	restoreFlags := strings.Join(candidates([]string{"restore"}), " ")
	for _, want := range []string{"--estate", "--force", "--json"} {
		if !strings.Contains(restoreFlags, want) {
			t.Errorf("`rig restore <TAB>` does not offer %q: %q",
				want, restoreFlags)
		}
	}
	if strings.Contains(restoreFlags, "--timeout") {
		t.Errorf("`rig restore <TAB>` offers --timeout, and restore reaches "+
			"no daemon and waits for nothing: %q", restoreFlags)
	}
	// --args belongs to a program's DECLARED command, and it arrives if the
	// word falls through to flagsOf - which asks the registry about a program
	// of that name.
	if strings.Contains(restoreFlags, "--args") {
		t.Errorf("`rig restore <TAB>` offers --args, so it fell through to "+
			"the declared-command path: %q", restoreFlags)
	}

	backupOffers := strings.Join(candidates([]string{"backup"}), " ")
	for _, want := range []string{"--json", "--timeout"} {
		if !strings.Contains(backupOffers, want) {
			t.Errorf("`rig backup <TAB>` does not offer %q: %q",
				want, backupOffers)
		}
	}
}

// ---- the renderings, neither of which needs a daemon -----------------------

// THE ROW COUNT AND THE HEAD COUNT ARE DIFFERENT NUMBERS AND THE RENDERING
// MUST NOT LET THEM BE READ AS ONE.
//
// `rig record query` answers HEADS; the row count includes every superseded
// version. Generation 22 measured 2,568 rows against 1,057 heads in this store
// while a document said "2,538 records" and was counting neither. A reader
// handed two bare integers cannot tell which answers which question, so both
// carry the question they answer.
func TestTheBackupRenderingLabelsRecordsAndHeadsApart(t *testing.T) {
	got := backupText(backupAnswer())

	for _, want := range []string{
		"archive  /home/x/.local/state/rig/backups/a-20260924T101112Z.tar.gz",
		"bytes    4096",
		"records  22 (every version, including superseded ones)",
		"heads    17 (what rig record query answers)",
		"schema   2",
		"taken    2026-09-24T10:11:12Z",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the rendering has no %q line:\n%s", want, got)
		}
	}
	if !strings.Contains(got, strings.Repeat("ab", 32)) {
		t.Errorf("the sha256 is not printed, and it is the only thing a "+
			"reader can check the copied file against:\n%s", got)
	}
}

// A ZERO TIMESTAMP IS A CASE, NOT A TIME.
//
// Printed through time.Unix a zero renders as 1970, which reads as a real
// moment and sends a reader to look at a clock. It can only mean the field did
// not travel, which is a defect in the daemon that answered rather than a fact
// about the archive - the same argument estateEpochCell makes about the zero
// role, and DECISION 6's reason for owing two mutations on a string field.
func TestABackupWithNoTimestampSaysSoRatherThanPrinting1970(t *testing.T) {
	r := backupAnswer()
	r.CreatedUnixNano = 0

	got := backupTakenCell(r)
	if strings.Contains(got, "1970") {
		t.Errorf("a missing timestamp rendered as %q, which reads as a real "+
			"moment in 1970 rather than as a field that did not arrive", got)
	}
	if !strings.Contains(got, "defect") {
		t.Errorf("a missing timestamp rendered as %q, which does not tell the "+
			"reader this is the daemon's fault rather than the archive's", got)
	}

	// The positive control: the same cell on a real number must be the time,
	// or the assertions above pass against a cell that says "(not said)" to
	// everything.
	if got := backupTakenCell(backupAnswer()); got != "2026-09-24T10:11:12Z" {
		t.Errorf("a real timestamp rendered as %q, so this test cannot tell a "+
			"missing field from a present one", got)
	}
}

// EVERY KEY IS PRESENT ON EVERY ANSWER, with no omitempty anywhere.
//
// Section 10, and estateJSON writes the argument out in full: an absent key
// reads as "this was never considered", and all seven of these are considered
// on every call. A key that comes and goes is also how a consumer learns to
// treat absence as a value.
func TestTheBackupObjectCarriesEveryFieldOnEveryAnswer(t *testing.T) {
	// The empty response is the case that matters: with omitempty anywhere,
	// this object would be `{}`.
	obj := backupJSON(&rigv1.BackupCreateResponse{})

	for _, key := range []string{
		"path", "sha256", "bytes", "schema_version",
		"records", "heads", "created_unix_nano",
	} {
		if _, ok := obj[key]; !ok {
			t.Errorf("the --json object has no %q key on an empty answer, so "+
				"a consumer cannot tell a field that was zero from one that "+
				"was never considered: %v", key, obj)
		}
	}
	if len(obj) != 7 {
		t.Errorf("the object carries %d keys, want the wire's seven: %v",
			len(obj), obj)
	}

	// The spelling is the wire's, not protojson's lowerCamelCase. Section 10
	// makes the proto's own field names the contract.
	if _, ok := obj["schemaVersion"]; ok {
		t.Error("the object spells schema_version as schemaVersion, which is " +
			"protojson's spelling rather than the wire's")
	}
}
