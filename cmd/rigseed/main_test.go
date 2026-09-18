// What this seeder WRITES is the thing under test, because what it writes is
// what Boris reads. The put is an exec of a real binary against a real daemon,
// so the testable unit is the argument list - putArgs builds it in one place
// for exactly that reason.
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// fieldValue reads one `--field name=value` out of a built argument list. It
// returns "" for a field that is not there, which is a distinguishable answer
// because every field this seeder writes has a non-empty value.
func fieldValue(args []string, name string) string {
	want := name + "="
	for i, a := range args {
		if a == fieldFlag && i+1 < len(args) && strings.HasPrefix(args[i+1], want) {
			return strings.TrimPrefix(args[i+1], want)
		}
	}
	return ""
}

// ⛔ A ROW THE DOCUMENT CLOSED MUST NOT BE SEEDED `active`.
//
// B19 was RETRACTED as falsified and the store published it as active work
// with the falsified sentence as its title. B55 and B56 were CLOSED BY A
// RULING and the brief listed them as live work. Both inversions came from one
// line writing `status=active` for every row without looking at it.
func TestTheStoredStatusFollowsTheDocumentsDisposition(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}

	for _, c := range []struct {
		name string
		it   record.BacklogItem
		want string
	}{
		{
			name: "an ordinary open row",
			it:   record.BacklogItem{ID: "B1", Title: "open work"},
			want: "active",
		},
		{
			name: "a struck title, which is the document's own closure mark",
			it:   record.BacklogItem{ID: "B19", Title: "a falsified claim", Done: true, Struck: true},
			want: "closed",
		},
		{
			name: "a terminal disposition leading the item cell",
			it:   record.BacklogItem{ID: "B21", Title: "CLOSED 2026-09-16 late", Done: true},
			want: "closed",
		},
		{
			name: "closed by a ruling rather than by work",
			it:   record.BacklogItem{ID: "B55", Title: "ruled, not asked", ClaimsDone: true, RuledClosed: true},
			want: "closed-by-ruling",
		},
		{
			// The document decides, not the parser: a row claiming a terminal
			// state without being struck and without a tick is OPEN, and the
			// seeder already reports it for a human to look at.
			name: "claims a terminal state unstruck, which the document leaves open",
			it:   record.BacklogItem{ID: "B15", Title: "claims done", ClaimsDone: true},
			want: "active",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := fieldValue(putArgs(o, c.it, 3), "status")
			if got != c.want {
				t.Fatalf("%s seeds status=%q, want %q", c.it.ID, got, c.want)
			}
		})
	}
}

// ⛔ THE SEEDER FILES NO PROGRESS STEP FOR A ROW IT DID NOT WORK, AND THE
// REASON IS MEASURED RATHER THAN PREFERRED.
//
// It filed `done` against every closed row. `done` asserts the work was
// COMPLETED, and three of backlog.go's four terminal words say it was not -
// B19 was RETRACTED as falsified. The honest word cannot be sent: wire.proto's
// `StepState` is STARTED, BLOCKED, DONE and cmd/rig refuses anything else
// before the call leaves. A live seeding run against a throwaway estate
// stopped on B55 with exactly that refusal.
func TestTheSeederFilesNoStepForARowItDidNotWork(t *testing.T) {
	for _, it := range []record.BacklogItem{
		{ID: "B1"},
		{ID: "B19", Done: true, Struck: true},
		{ID: "B21", Done: true},
		{ID: "B55", ClaimsDone: true, RuledClosed: true},
		{ID: "B15", ClaimsDone: true},
	} {
		if got := stepStateFor(it); got != "" {
			t.Errorf("%s files a step in state %q; an imported row has no live state", it.ID, got)
		}
	}
}

// ⛔ AND IF IT EVER FILES ONE AGAIN, IT MUST BE A WORD THE STORE ACCEPTS.
//
// The vocabulary has four copies - `stepStates` in internal/record,
// `stepStateSpellings` in cmd/rig, `stepStateWire` in internal/daemon and the
// enum in wire.proto - and nothing let a caller check against any of them.
// This is the check that was missing when a live run reached row B55 before
// anything said the word could not be sent.
func TestAnyStateTheSeederFilesIsOneTheStoreAccepts(t *testing.T) {
	// POSITIVE CONTROL: the predicate must be able to say no, or this test
	// passes over an empty set and over a broken predicate alike.
	if record.IsStepState("closed-by-ruling") {
		t.Fatal("record.IsStepState accepts a word the wire cannot carry, " +
			"so it cannot refuse anything and this test measures nothing")
	}
	if !record.IsStepState("done") {
		t.Fatal("record.IsStepState refuses done, so it is not the store's set")
	}

	for _, it := range []record.BacklogItem{
		{ID: "B1"},
		{ID: "B19", Done: true, Struck: true},
		{ID: "B55", ClaimsDone: true, RuledClosed: true},
	} {
		if state := stepStateFor(it); state != "" && !record.IsStepState(state) {
			t.Errorf("%s files state %q and the store refuses it", it.ID, state)
		}
	}
}

// ⛔ HOW THE DOCUMENT CLOSED A ROW MUST SURVIVE THE COARSENING.
//
// It used to live in the progress step's note, and the step is gone. Without
// this field `closed` and `closed-by-ruling` are all that is left of three
// different closure conventions.
func TestTheClosureReasonIsStoredOnTheRecord(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	for _, c := range []struct {
		name string
		it   record.BacklogItem
		want string
	}{
		{
			"a struck title",
			record.BacklogItem{ID: "B19", Done: true, Struck: true},
			"closed in BACKLOG.md by a struck title",
		},
		{
			"a terminal lead in the item cell",
			record.BacklogItem{ID: "B21", Done: true},
			"closed in BACKLOG.md by a terminal disposition in its item cell",
		},
		{
			"a ruling",
			record.BacklogItem{ID: "B55", ClaimsDone: true, RuledClosed: true},
			"closed in BACKLOG.md by a ruling rather than by work",
		},
		{"an open row, which has no closure to explain", record.BacklogItem{ID: "B1"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := fieldValue(putArgs(o, c.it, 1), "closure_note"); got != c.want {
				t.Fatalf("%s stored closure_note=%q, want %q", c.it.ID, got, c.want)
			}
		})
	}
}

// closedRowsSeededActive counts, over one document, the rows the parser calls
// closed that this seeder would still write as live work. It returns the
// number of closed rows it looked at, so an empty answer is distinguishable
// from a clean one.
func closedRowsSeededActive(t *testing.T, path string) (closed int) {
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	items, err := record.ParseBacklog(f)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	o := options{project: "rig", backlog: path}
	for _, it := range items {
		if !it.Done && !it.RuledClosed {
			continue
		}
		closed++
		if got := fieldValue(putArgs(o, it, 1), "status"); got == record.StatusActive {
			t.Errorf("%s is closed in %s and seeds status=active: %.60q", it.ID, path, it.Title)
		}
	}
	return closed
}

// shapeFixture is the parser's own document of SHAPES, borrowed rather than
// copied. A second fixture holding the same shapes is the two-readers-of-one-
// document drift this seeder's header names as the project's most expensive
// recorded failure, one directory over.
const shapeFixture = "../../internal/record/testdata/backlog-shapes.md"

// ⛔ THE PROPERTY, OVER A WHOLE DOCUMENT, NOT THREE PINNED IDS.
//
// A pin on B19, B55 and B56 goes stale the day a fourth row is closed. The
// loop is what keeps the answer true after somebody edits the document.
func TestNoClosedRowInTheShapeFixtureIsSeededAsLiveWork(t *testing.T) {
	closed := closedRowsSeededActive(t, shapeFixture)
	// POSITIVE CONTROL. An empty loop passes silently. The fixture holds six
	// rows the parser calls Done - a struck one, three terminal leads, a
	// REJECTED, a RETRACTED and the row missing its pipe - and no ruled row,
	// because it has no ticks.
	if closed != 6 {
		t.Fatalf("the shape fixture yielded %d closed rows and held 6 when this was "+
			"written; the fixture or the parser moved and this test is no longer "+
			"looking at what it thinks", closed)
	}
}

// The same property over the REAL document, which is where B19, B55 and B56
// actually live.
//
// ⛔ IT SKIPS RATHER THAN FAILS WHEN THE DOCUMENT IS NOT THERE, AND THAT IS
// NOT A HOLE. `BACKLOG.md` is a GITIGNORED SYMLINK into the logbook, which is
// a different repository - a fresh clone and a CI runner have a dangling link
// and no document to read. The test above is the one that always runs; this
// one is the rig-on-rig check for a machine that has the logbook.
func TestNoClosedRowInTheLiveBacklogIsSeededAsLiveWork(t *testing.T) {
	const live = "../../BACKLOG.md"
	if _, err := os.Stat(live); err != nil {
		t.Skipf("%s does not resolve on this machine (%v), so there is no live "+
			"document to measure; the shape-fixture test carries the property", live, err)
	}
	closed := closedRowsSeededActive(t, live)
	if closed < 3 {
		t.Fatalf("the live document yielded %d closed rows and the attack measured 11 "+
			"(9 struck or terminal-led, 2 ruled); this test is not looking at what it thinks", closed)
	}
}

// ⛔ "SUPERSEDED 68" OVER 68 NO-OPS IS THE SAME CLASS OF FALSE STATEMENT THIS
// SEEDER WAS JUST REPAIRED FOR.
//
// The store no longer mints a version for a put whose content already matches
// the head, so the version the store answers with - not the fact that a call
// was made - is what says whether anything changed.
func TestTheReportSeparatesASupersessionFromANoOp(t *testing.T) {
	for _, c := range []struct {
		name                           string
		exists, dryRun                 bool
		before, after                  uint64
		created, superseded, unchanged int
	}{
		{name: "a create", exists: false, before: 0, after: 1, created: 1},
		{name: "a real supersession", exists: true, before: 3, after: 4, superseded: 1},
		{name: "a put the store declined to version", exists: true, before: 4, after: 4, unchanged: 1},
		{
			// A dry run never asks the store, so it cannot know. It says the
			// coarser thing rather than guessing the finer one.
			name: "a dry run, which cannot know", exists: true, dryRun: true,
			before: 4, after: 0, superseded: 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := &result{}
			r.count(c.exists, c.dryRun, c.before, c.after)
			if r.created != c.created {
				t.Errorf("created=%d, want %d", r.created, c.created)
			}
			if r.superseded != c.superseded {
				t.Errorf("superseded=%d, want %d", r.superseded, c.superseded)
			}
			if r.unchanged != c.unchanged {
				t.Errorf("unchanged=%d, want %d", r.unchanged, c.unchanged)
			}
		})
	}
}

// ⛔ EVERY TAG THIS SEEDER WRITES MUST COME BACK OUT OF THE STORE'S OWN READER,
// AND FOR THE LIFE OF THIS FIELD NONE OF THEM DID.
//
// `tagsFor` joined with a comma, `headingIntent` and `rankedIntent` each wrote a
// bare word, and `record.DecodeTags` parses JSON - so all three decoded to
// nothing. Measured on Boris's live production store 2026-09-18: 54 record
// versions carry a non-empty tags field and ZERO of them parse.
//
// ⛔ THE ASSERTION IS A ROUND TRIP AND NOT A SPELLING CHECK, WHICH IS WHY IT
// CATCHES THE NEXT GRAIN TOO. A test comparing the field against
// `["a","b"]` passes for a writer that hand-rolls the JSON and drifts from
// `EncodeTags` later; running the reader is the only form that cannot.
//
// ⛔ AND IT IS ONE SUBTEST PER GRAIN BECAUSE THREE GRAINS WRITE THIS FIELD.
// One table row covering "the seeder" would go red on the first grain and never
// reach the other two - the count-the-assertions rule this repo already carries.
func TestEveryTagTheSeederWritesIsReadableByTheStore(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}

	for _, c := range []struct {
		name   string
		fields map[string]string
		want   []string
	}{
		{
			name: "the row grain, irregular and placed",
			fields: rowIntent(o, record.BacklogItem{
				ID: "B21", Title: "a shifted row", Section: "open",
				Malformed: true,
			}).fields,
			want: []string{"section:open", "malformed"},
		},
		{
			name: "the heading grain",
			fields: headingIntent(o, record.Unimported{
				ID: "B46", Title: "the acceptance test", Section: "rig-s-development-plan",
			}, "").fields,
			want: []string{"heading-borne", "section:rig-s-development-plan"},
		},
		{
			name: "the ranked grain, whose section is also an id qualifier",
			fields: rankedIntent(o, record.Unimported{
				Label: "6a", Section: "the-critical-path-to-the-gate", Line: 110,
			}, "rig/the-critical-path-to-the-gate/6a").fields,
			want: []string{"rank-only", "section:the-critical-path-to-the-gate"},
		},
		// ⛔ THE UNPLACED CASES, AND THEY ARE HERE BECAUSE A MUTATION SURVIVED
		// WITHOUT THEM. Making `sectioned` emit the tag unconditionally left
		// every assertion above GREEN, so nothing held the line that a record
		// the document placed nowhere gets NO section tag. A tag reading
		// `section:` is worse than no tag: it says the document stated a place
		// and then names none.
		{
			name: "a heading the document placed nowhere",
			fields: headingIntent(o, record.Unimported{
				ID: "B46", Title: "the acceptance test",
			}, "").fields,
			want: []string{"heading-borne"},
		},
		{
			name: "a ranked row the document placed nowhere",
			fields: rankedIntent(o, record.Unimported{
				Label: "6a", Line: 110,
			}, "rig//6a").fields,
			want: []string{"rank-only"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := c.fields[fieldTags]
			if raw == "" {
				t.Fatalf("this grain wrote no %s field at all", fieldTags)
			}
			got := record.DecodeTags(raw)
			if len(got) == 0 {
				t.Fatalf("the store reads NO tags off %q, which is the defect this test exists for", raw)
			}
			if len(got) != len(c.want) {
				t.Fatalf("decoded %v off %q, want %v", got, raw, c.want)
			}
			for i, w := range c.want {
				if got[i] != w {
					t.Errorf("tag %d is %q, want %q (decoded %v)", i, got[i], w, got)
				}
			}
		})
	}
}

// ⛔ A ROW IS TAGGED WITH WHERE THE DOCUMENT PUT IT, WHICH IS THE THIRD OF THE
// THREE SOURCES §11 NAMES AND THE ONLY ONE NOTHING CARRIED.
//
// Boris, 2026-09-17: "I'm sure they can be grouped or at least tagged so that
// the user can see what relates to what." §11 answers it from what the
// documents ALREADY assert - the section a row sits under, the `part-of`
// parent, the owner column - and forbids a vocabulary a seat invents. `owner`
// and `part-of` were carried; the section was not.
//
// ⛔ THE EMPTY CASE IS HERE BECAUSE A TAG READING `section:` WOULD BE WORSE
// THAN NO TAG - it says the document stated a place and names none.
func TestARowIsTaggedWithTheSectionItSitsUnder(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}

	placed := record.DecodeTags(rowIntent(o, record.BacklogItem{
		ID: "B90", Title: "the management panel", Section: "open",
	}).fields[fieldTags])
	if len(placed) != 1 || placed[0] != "section:open" {
		t.Errorf("a placed row carries %v, want exactly [section:open]", placed)
	}

	loose := rowIntent(o, record.BacklogItem{ID: "B1", Title: "a row in no section"}).fields
	if raw, ok := loose[fieldTags]; ok {
		t.Errorf("a row the document placed nowhere carries tags %q; it must carry none", raw)
	}
}
