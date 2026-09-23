package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/boris-milner/rig/internal/backup"
	"github.com/boris-milner/rig/internal/instance"

	// TEST-ONLY, AND THE IMPORT IS THE WHOLE POINT OF THE FIRST TEST BELOW.
	//
	// schemaCeiling in restore.go is a number this package carries because
	// importing internal/record would link modernc.org/sqlite into `rig`
	// (section 46, decision 10 and finding F2). A carried number rots unless
	// something couples it, and a test-only import is the only coupling that
	// costs the binary nothing. layering_test.go does exactly this with
	// internal/meta, and main_test.go with internal/daemon's CallTimeout, so
	// this is the house pattern rather than a new idea - and
	// TestTheClientLinksNoSQLiteDriver below is what proves the import stayed
	// test-only.
	//
	// ALIASED because record_test.go already has a `record` helper in this
	// package. A collision between a package name and a test fixture is a
	// compile error, and the alias is the smaller of the two changes: the
	// fixture is another seat's row.
	recordstore "github.com/boris-milner/rig/internal/record"
)

// manifest is a complete Manifest, so a renderer test reads as a change from a
// known-good archive rather than as a pile of literals.
func manifest() backup.Manifest {
	return backup.Manifest{
		CreatedUnixNano: time.Date(2026, 9, 24, 10, 11, 12, 0, time.UTC).UnixNano(),
		Estate:          "a",
		Format:          backup.Format,
		Heads:           17,
		Records:         22,
		RigVersion:      "v0.1.0",
		SchemaVersion:   2,
		Wire:            "v1",
	}
}

func restoreResult() backup.RestoreResult {
	return backup.RestoreResult{
		Manifest: manifest(),
		Dir:      "/home/x/.local/state/rig/estates/b",
	}
}

// ---- the two layering pins -------------------------------------------------

// ⛔ BUMPING recordstore.SchemaVersion WITHOUT BUMPING schemaCeiling MAKES `rig
// restore` REFUSE EVERY FRESH ARCHIVE, and this is what turns that into a
// build failure rather than a support question.
//
// Decision 10 requires restore to refuse a manifest newer than "the restoring
// binary's recordstore.SchemaVersion". `rig restore` never connects to a daemon
// (decision 7) and cmd/rig may not link internal/record, so the number is
// carried here. This test is the only thing standing between the two copies.
func TestTheRestoresSchemaCeilingIsTheStoresOwnSchemaVersion(t *testing.T) {
	if schemaCeiling != recordstore.SchemaVersion {
		t.Fatalf("cmd/rig carries schemaCeiling %d and internal/record is at "+
			"SchemaVersion %d.\n"+
			"A ceiling BELOW the store's makes `rig restore` refuse every "+
			"archive a current rigd writes, with a message about the archive "+
			"being too new for a build that wrote it.\n"+
			"A ceiling ABOVE it accepts an archive this binary cannot migrate.\n"+
			"Move schemaCeiling in cmd/rig/restore.go to %d.",
			schemaCeiling, recordstore.SchemaVersion, recordstore.SchemaVersion)
	}
}

// ⛔ `rig` MUST LINK NO SQLITE DRIVER, AND layering_test.go DOES NOT CHECK
// THIS. Finding F5, measured 2026-09-23.
//
// Section 46's test-9 row credits TestTheClientLinksNeitherTheDaemonNorItsValidator
// with going red when internal/backup imports internal/record. It does not:
// that test forbids internal/meta, internal/daemon, jsonschema and
// golang.org/x/text, and names neither the store nor any driver. Measured in a
// detached copy with the import added, `go list -deps ./cmd/rig | grep -c
// modernc` printed 8 and the layering test STAYED GREEN.
//
// So the assertion lives here, where this seat's own files are what would
// break it. internal/backup carries the other half -
// TestThisPackageLinksNothingButTheStandardLibrary - and this one is the
// binary's side of the same fact: internal/backup could stay clean while
// cmd/rig reached for the store directly.
func TestTheClientLinksNoSQLiteDriver(t *testing.T) {
	// `.` rather than ./cmd/rig: a test runs in its own package directory, so
	// the relative path cannot rot if this package is ever moved.
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps . could not run, so this test proved NOTHING "+
			"and must not be read as a pass: %v\n%s", err, out)
	}
	deps := strings.Split(strings.TrimSpace(string(out)), "\n")

	// THE POSITIVE CONTROL. Every assertion below is an absence, and an
	// absence is what a truncated or misdirected `go list` also produces. The
	// control is this seat's own package rather than an arbitrary one: it
	// proves the walk saw the dependency edge the assertion is about.
	const control = "github.com/boris-milner/rig/internal/backup"
	var sawControl bool
	for _, d := range deps {
		if d == control {
			sawControl = true
		}
	}
	if !sawControl {
		t.Fatalf("the control package %q is not in `go list -deps .` output, "+
			"so the command answered about something other than this binary "+
			"and every absence below is meaningless. %d lines came back",
			control, len(deps))
	}

	var linked []string
	for _, d := range deps {
		if strings.HasPrefix(d, "modernc.org/") ||
			d == "github.com/boris-milner/rig/internal/record" {
			linked = append(linked, d)
		}
	}
	if len(linked) != 0 {
		t.Errorf("cmd/rig links %d package(s) it must not: %v\n"+
			"Section 22 puts the SQLite driver in rigd alone, and section 46 "+
			"decision 1 is built on it: the daemon takes the snapshot BECAUSE "+
			"only the daemon links the store. The usual cause is a non-test "+
			"import of internal/record added to reach SchemaVersion - carry "+
			"the number as schemaCeiling and pin it in a test instead.",
			len(linked), linked)
	}
}

// ---- argv, every refusal reached before the disk is touched ----------------

// ⛔ EVERY CASE HERE IS REFUSED BEFORE THE ESTATE'S CLAIM IS TAKEN AND BEFORE
// ANY PATH IS RESOLVED, which is what makes them safe to run in process. A
// case that got as far as instance.AcquireName would resolve THIS MACHINE's
// state directory, and the estate it named might be a real one.
func TestRestoreRefusesBadArgvBeforeItReachesAnyEstate(t *testing.T) {
	// ⛔ BOTH VARIABLES, AND THE SECOND IS THE ONE THAT MATTERS. A private
	// XDG_RUNTIME_DIR isolates the SOCKET and not the store; the estate's
	// state directory resolves through XDG_STATE_HOME. Measured on this team
	// twice, and COORDINATION.md carries both instances. The cases below are
	// all refused before either path is resolved - this is what makes that a
	// property rather than a claim about the order of the checks.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	for _, tc := range []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "no archive at all",
			argv: []string{"--estate", "b"},
			want: []string{"one archive, and none was given"},
		},
		{
			name: "two archives",
			argv: []string{"--estate", "b", "one.tar.gz", "two.tar.gz"},
			want: []string{"one archive, and 2 were given"},
		},
		{
			name: "no estate",
			argv: []string{"x.tar.gz"},
			want: []string{"--estate is required"},
		},
		{
			// paths.ValidEstateName's job, reached here so a traversal never
			// gets as far as a path join. Section 38's fourth standing rule.
			name: "an estate name that is a path",
			argv: []string{"--estate", "../../etc", "x.tar.gz"},
			want: []string{"rig restore:"},
		},
		{
			name: "an empty estate name",
			argv: []string{"--estate=", "x.tar.gz"},
			want: []string{"--estate is required"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := cmdRestore(tc.argv)
			if err == nil {
				t.Fatalf("`rig restore %s` was accepted",
					strings.Join(tc.argv, " "))
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refused for the wrong reason.\n want: %q\n  got: %v",
						want, err)
				}
			}
		})
	}
}

// ---- the refusals a person meets with a real archive in hand ---------------

// EVERY REFUSAL THIS VERB PRODUCES ITSELF CARRIES A PRECONDITION, AN ACTUAL
// AND A FIX, and none of them falls through to the generic arm by accident.
//
// ⛔ THE GENERIC ARM IS THE ONE TO WATCH. It is correct for an unreadable
// archive and WRONG for any of the four typed errors, because it says "the
// archive is not one this build can read whole" - which is a true sentence
// about a corrupt file and a misleading one about a daemon holding the estate.
// A type dropped from restoreRefusal's chain does not fail to compile and does
// not fail any other test in this package: it silently starts answering the
// wrong question.
func TestEveryTypedRestoreFailureGetsItsOwnRefusalRatherThanTheGenericOne(t *testing.T) {
	for _, tc := range []struct {
		name         string
		err          error
		wantCode     string
		wantInActual string
		wantFix      string
	}{
		{
			name:         "the estate is held by a running daemon",
			err:          &instance.NameHeldError{Name: "b", Path: "/run/x.pid", Incumbent: 4242},
			wantCode:     codeBadArgument,
			wantInActual: "claimed by process 4242",
			wantFix:      "rig down",
		},
		{
			name:         "the estate already holds state",
			err:          &backup.DirExistsError{Dir: "/state/estates/b"},
			wantCode:     codeBadArgument,
			wantInActual: "/state/estates/b already holds state",
			wantFix:      "rig restore --force",
		},
		{
			name:         "the archive is newer than this build",
			err:          &backup.SchemaTooNewError{Found: 3, Ceiling: 2},
			wantCode:     codeBadArgument,
			wantInActual: "archive is schema 3 and this build reads up to 2",
			wantFix:      "rig version",
		},
		{
			name:         "a member does not match its digest",
			err:          &backup.IntegrityError{Member: "record.db"},
			wantCode:     codeBadResult,
			wantInActual: "member record.db does not",
			wantFix:      "sha256sum <archive>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj, _, structured := shape(restoreRefusal(tc.err))
			if !structured {
				t.Fatal("the refusal is not a structured rigError, so --json " +
					"cannot carry it and section 10's promise is broken here")
			}
			if obj.Code != tc.wantCode {
				t.Errorf("code %q, want %q", obj.Code, tc.wantCode)
			}
			if !strings.Contains(obj.Actual, tc.wantInActual) {
				t.Errorf("actual is %q, want it to say %q",
					obj.Actual, tc.wantInActual)
			}
			if obj.FixCommand != tc.wantFix {
				t.Errorf("fix_command is %q, want %q", obj.FixCommand, tc.wantFix)
			}
			if obj.Precondition == "" {
				t.Error("no precondition, so the refusal says what happened " +
					"and not what was required")
			}
			// ⛔ THE ASSERTION THAT CATCHES A DROPPED errors.As ARM. All four
			// of these fall through to the generic refusal without it, and
			// the generic one is a plausible-looking answer.
			if strings.Contains(obj.Actual, "it is not") {
				t.Errorf("this fell through to the generic refusal, which "+
					"answers about an unreadable archive: %+v", obj)
			}
		})
	}

	// The control on the fall-through itself: an untyped error must still
	// reach the generic arm, or the assertion above is only proving that
	// nothing ever reaches it.
	obj, _, _ := shape(restoreRefusal(os.ErrNotExist))
	if !strings.Contains(obj.Actual, "it is not") {
		t.Errorf("an untyped failure did not reach the generic refusal, so "+
			"the four above prove nothing about the chain: %+v", obj)
	}
}

// ---- the renderings, neither of which needs a daemon -----------------------

// ⛔ THE LAST BLOCK OF THE HUMAN RENDERING IS THE POINT OF THE WHOLE VERB.
//
// Section 44: the restore is the test, not the backup. A restore that reported
// success and left the reader to invent a way of verifying it would be the
// reassuring answer rather than the useful one, so it ends with the two
// commands to run and the number to expect.
func TestTheRestoreRenderingEndsWithTheCheckToRun(t *testing.T) {
	got := restoreText("b", restoreResult())

	for _, want := range []string{
		"estate    b",
		"at        /home/x/.local/state/rig/estates/b",
		"heads     17",
		"records   22",
		"rigd --estate b",
		"expect 17",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the rendering has no %q in it:\n%s", want, got)
		}
	}
	// Nothing was replaced, so nothing about undoing a replacement may be
	// printed: an undo instruction naming a directory that does not exist is
	// worse than none.
	if strings.Contains(got, "replaced") || strings.Contains(got, "mv ") {
		t.Errorf("a restore that replaced nothing printed an undo "+
			"instruction:\n%s", got)
	}
}

// ⛔ THE PREVIOUS STATE IS RENAMED, NOT DELETED, AND THE PATH IS PRINTED.
// Decision 8. A person who restored the wrong archive needs that path to get
// back, and the moment it is only in a log it is effectively gone.
func TestAForcedRestorePrintsHowToUndoItself(t *testing.T) {
	res := restoreResult()
	res.Replaced = "/home/x/.local/state/rig/estates/b.replaced-20260924T101112Z"

	got := restoreText("b", res)
	if !strings.Contains(got, res.Replaced) {
		t.Errorf("the rendering does not print where the old state went:\n%s", got)
	}
	if !strings.Contains(got, "mv "+res.Replaced+" "+res.Dir) {
		t.Errorf("the rendering does not give the command that undoes the "+
			"restore:\n%s", got)
	}
	if !strings.Contains(got, "RENAMED, not deleted") {
		t.Errorf("the rendering does not say the old state still exists, "+
			"which is the fact that makes --force safe to have typed:\n%s", got)
	}
}

// A MANIFEST FIELD THE ARCHIVE DID NOT CARRY IS A CASE, NOT AN EMPTY STRING.
//
// The same argument as backupTakenCell's zero: `from estate , taken , by rig `
// reads as a rendering bug and tells the reader nothing about which half is
// missing.
func TestAManifestFieldTheArchiveDidNotCarryReadsAsNotSaid(t *testing.T) {
	got := restoreFromCell(backup.Manifest{})
	if n := strings.Count(got, notSaid); n != 3 {
		t.Errorf("an empty manifest rendered %d of its three fields as %q: %q",
			n, notSaid, got)
	}
	if strings.Contains(got, "1970") {
		t.Errorf("a missing timestamp rendered as a 1970 date: %q", got)
	}

	// The positive control: a full manifest must render none of them that way,
	// or the count above passes against a cell that says it to everything.
	if got := restoreFromCell(manifest()); strings.Contains(got, notSaid) {
		t.Errorf("a complete manifest still reported a missing field: %q", got)
	}
}

// EVERY KEY IS PRESENT ON EVERY ANSWER, with no omitempty anywhere - section
// 10, and the argument is written out in full on estateJSON.
func TestTheRestoreObjectCarriesEveryFieldOnEveryAnswer(t *testing.T) {
	obj := restoreJSON("", backup.RestoreResult{})

	for _, key := range []string{
		"estate", "dir", "replaced", "heads", "records",
		"schema_version", "created_unix_nano", "rig_version", "from_estate",
	} {
		if _, ok := obj[key]; !ok {
			t.Errorf("the --json object has no %q key on an empty result: %v",
				key, obj)
		}
	}
	if len(obj) != 9 {
		t.Errorf("the object carries %d keys, want nine: %v", len(obj), obj)
	}

	// And it must actually encode: a map[string]any holding an unencodable
	// value is a runtime failure on the one path a caller parses.
	b, err := json.Marshal(restoreJSON("b", restoreResult()))
	if err != nil {
		t.Fatalf("the --json object does not encode: %v", err)
	}
	for _, want := range []string{`"estate":"b"`, `"heads":17`, `"from_estate":"a"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the encoded object has no %s in it: %s", want, b)
		}
	}
}
