package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/borismilner/rig/internal/record"
)

// ⛔ THE AGENT DOOR'S READ PATH, AGAINST A REAL STORE RATHER THAN A DOUBLE.
//
// Everything in internal/meta that exercises `record_query` goes through
// `holdsRecords`, whose Query DISCARDS EVERY ARGUMENT and returns a canned
// value. No test there could have gone red on the defect below, whatever the
// adapter did - a check that cannot fail reads as a pass, and this project has
// now recorded that eight times.
//
// So these tests hold the adapter itself and give it records to find.
//
// ⛔ DEFECT A LIVED HERE AND ITS SURFACE IS GONE. `projectExists` and the test
// that pinned it went out with `project_brief` at PLAN.md section 50 move 8;
// the defect it recorded was an empty kind read as a value rather than as
// "every", and `(*Store).Find` is where that rule now lives alone.

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
