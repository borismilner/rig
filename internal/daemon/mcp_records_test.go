package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// ⛔ THE AGENT DOOR'S READ PATH, AGAINST A REAL STORE RATHER THAN A DOUBLE.
//
// Everything in internal/meta that exercises `record_query` and
// `project_brief` goes through `holdsRecords`, whose Query and Brief DISCARD
// EVERY ARGUMENT and return a canned value. No test there could have gone red
// on either defect below, whatever the adapter did - a check that cannot fail
// reads as a pass, and this project has now recorded that eight times.
//
// So these tests hold the adapter itself and give it records to find.

// recordStore opens a private store for one test.
//
// XDG_STATE_HOME is redirected per test, so nothing here can reach a real
// estate. A private runtime directory would NOT have been enough: the store
// resolves through the STATE directory, and a seat that isolated only the
// socket measured an empty store and concluded it was isolated.
func recordStore(t *testing.T) *record.Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigmcp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	st, err := record.Open("mcprecords")
	if err != nil {
		t.Fatalf("opening the record store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedRecords writes the shapes the two defects below are about: two projects,
// three kinds, and a typed field to select on.
func seedRecords(t *testing.T, st *record.Store) {
	t.Helper()
	ctx := context.Background()
	for _, r := range []record.PutRequest{
		{ID: "rig", Kind: "project", Project: "rig", Body: "rig itself"},
		{
			ID: "B76", Kind: "work-item", Project: "rig", Body: "name a missing container",
			Fields: map[string]string{"owner": "read-path"},
		},
		{ID: "d-1", Kind: "decision", Project: "rig", Body: "a decision"},
		{ID: "other-1", Kind: "work-item", Project: "other", Body: "somewhere else"},
	} {
		r.Session, r.Seat, r.Epoch = "test", "test", 1
		if _, err := st.Put(ctx, r); err != nil {
			t.Fatalf("seeding %s: %v", r.ID, err)
		}
	}
}

// ⛔ DEFECT A: `project_brief` COULD NOT FIND ANY PROJECT. NOT ONE. EVER.
//
// `projectExists` asked `(*Store).Query(ctx, project, "")`, and that method's
// SQL is `WHERE r.project = ? AND r.kind = ?` - LITERAL equality on both. An
// empty kind is not a wildcard there, it is a kind no record can have, because
// Put refuses to write one. So the count was always zero, `known` was always
// false, and `Brief` returned `{"Found":false}` for every slug ever asked.
//
// ⛔ THE FUNCTION'S OWN DOC COMMENT NAMES THIS AS THE FAILURE IT EXISTS TO
// PREVENT - "keying existence on one privileged kind would report the live
// project as absent". It arrived anyway, and the privileged kind was "".
//
// ⛔ WHY IT MATTERED MORE THAN ITS SIZE. project_brief is section 39's resume
// mechanism and section 9's A6: the single tool behind "use rig to work on
// rig". It was 100% dead on the agent surface and nothing looked wrong,
// because "this project does not exist" is a well-formed ANSWER.
//
// THE CONTROL IS THE SECOND ROW. A fix that returned true unconditionally
// passes the first row alone, and would be worse than the defect - it would
// report every typo as a real project, which is B76 inverted.
func TestTheAgentDoorCanFindAProjectThatExists(t *testing.T) {
	st := recordStore(t)
	seedRecords(t, st)
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		project string
		want    bool
	}{
		{"a project with a container record and work under it", "rig", true},
		// ⛔ A PROJECT WITH NO `project` RECORD STILL EXISTS. Any record
		// counts: rig's own store was this shape for most of its history, and
		// keying on the container would reintroduce the defect through the fix.
		{"work filed under a slug that has no container record", "other", true},
		{"a slug nothing was ever filed under", "zzz-no-such-project", false},
		{"the empty slug is not a wildcard here", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := projectExists(ctx, st, tc.project)
			if err != nil {
				t.Fatalf("projectExists(%q): %v", tc.project, err)
			}
			if got != tc.want {
				t.Errorf("projectExists(%q) = %v, want %v.\n"+
					"project_brief short-circuits on this, so a false here "+
					"means the agent surface answers {\"Found\":false} for a "+
					"project that is right there in the store",
					tc.project, got, tc.want)
			}
		})
	}
}

// ⛔ DEFECT B: `record_query` ANSWERED ONLY WHEN BOTH project AND kind WERE
// GIVEN, AND ANSWERED NOTHING OTHERWISE.
//
// Same root cause and a different surface. Measured against the live door:
// `project=rig` returned 0 rows where the CLI returned 3; `kind=requirement`
// returned 0 where the CLI returned 2; neither returned 0 where the CLI
// returned the whole census. Only both-supplied agreed.
//
// ⛔ AND THE TOOL DESCRIPTION TOLD THE AGENT TO READ THE BUG AS CORRECT: "an
// empty result means nothing matched - it is an answer, not a failure." An
// agent has no way from that sentence to the defect.
//
// ⛔ THE REMEDY IS `(*Store).Find`, WHICH ALREADY SWITCHES OVER ALL EIGHT
// SHAPES - empty project and empty kind included, with B65's field predicate
// in SQL - and which the CLI has used all along. That is why the two surfaces
// disagreed: one of them was already right.
func TestTheAgentDoorQueriesOnEitherFilterOrNeither(t *testing.T) {
	st := recordStore(t)
	seedRecords(t, st)
	ctx := context.Background()
	m := &mcpCaller{Daemon: &Daemon{records: st}}

	// ⛔ THE ORDER IS ASSERTED AND NOT SORTED AWAY. `Find` orders by
	// `r.project, r.kind, r.id`, the CLI renders that order, and an agent
	// reading a listing beside a terminal's has to see the same rows in the
	// same places. Comparing as sets would let the two surfaces drift into
	// two orderings with nothing saying so.
	for _, tc := range []struct {
		name    string
		project string
		kind    string
		fields  map[string]string
		want    []string
	}{
		{
			name: "project alone", project: "rig",
			want: []string{"d-1", "rig", "B76"},
		},
		{
			name: "kind alone", kind: "work-item",
			want: []string{"other-1", "B76"},
		},
		{
			name: "neither - the census",
			want: []string{"other-1", "d-1", "rig", "B76"},
		},
		{
			name:    "both, which is the shape that always worked",
			project: "rig", kind: "work-item",
			want: []string{"B76"},
		},
		// ⛔ B65's FIELD PREDICATE, REACHABLE WITH NEITHER OTHER FILTER. This
		// row was unanswerable before: the field filter ran over whatever the
		// project-and-kind query returned, and that was always nothing.
		{
			name: "a field alone", fields: map[string]string{"owner": "read-path"},
			want: []string{"B76"},
		},
		{
			name:   "a field that matches nothing is still an answer",
			fields: map[string]string{"owner": "nobody"},
			want:   nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := m.Query(ctx, tc.project, tc.kind, tc.fields)
			if err != nil {
				t.Fatalf("Query(%q, %q, %v): %v",
					tc.project, tc.kind, tc.fields, err)
			}
			got := make([]string, 0, len(rows))
			for _, r := range rows {
				got = append(got, r.ID)
			}
			if !sameIDs(got, tc.want) {
				t.Errorf("Query(project=%q, kind=%q, fields=%v) = %v, want %v.\n"+
					"The CLI answers this question through (*Store).Find and "+
					"gets it right; a disagreement here is the agent surface "+
					"and the terminal giving two answers to one question",
					tc.project, tc.kind, tc.fields, got, tc.want)
			}
		})
	}
}

func sameIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
