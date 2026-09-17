package record

import (
	"errors"
	"strings"
	"testing"
)

// noop builds the put this file is about: one work item, one field map.
//
// Every test here uses ONE id, because each opens its own store and the
// question is what a second put at the same id does.
func noop(body string, fields map[string]string) PutRequest {
	return PutRequest{
		ID: "B1", Kind: "work-item", Project: "rig", Body: body, Fields: fields,
		Session: "session-one", Seat: "backend-record", Epoch: 6,
	}
}

// ⛔ 195 OF THIS STORE'S 263 VERSIONS ARE COPIES OF THEIR PREDECESSOR.
//
// Measured on the live production store: 68 records, and not one of them has
// ever had a content change between two versions. Every supersession rig has
// performed is a no-op, so `history` answers "what did this say before" with
// the same bytes four times, and section 39's slice 1 demonstration - a
// requirement superseded twice whose first wording is read back - passes
// vacuously because the first wording and the last are identical.
func TestAPutIdenticalToTheHeadMintsNoNewVersion(t *testing.T) {
	s := openStore(t, estate(t, "development"))

	first, err := s.Put(tctx, noop("the title", map[string]string{"status": "active"}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if first.Version != 1 {
		t.Fatalf("a create is version %d, want 1", first.Version)
	}

	again, err := s.Put(tctx, func() PutRequest {
		r := noop("the title", map[string]string{"status": "active"})
		r.IfVersion = 1
		return r
	}())
	if err != nil {
		t.Fatalf("re-putting identical content: %v", err)
	}
	if again.Version != 1 {
		t.Fatalf("an identical put returned version %d, want 1: it minted a version "+
			"that says nothing changed", again.Version)
	}

	hist, err := s.History(tctx, "B1")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("history is %d versions after two identical puts, want 1", len(hist))
	}
}

// ⛔ A NEW SESSION IS NOT A CHANGE, AND THAT IS A DECISION RATHER THAN AN
// OVERSIGHT.
//
// The seeder re-runs under a new session id every time. If provenance counted,
// the fix would do nothing at all: four re-runs produced four identical
// versions of B1 under four different session ids, which is the measurement
// that opened this. The version chain answers "what did this record SAY";
// provenance answers "who wrote THIS version". A run that read the document
// and found it unchanged is a fact about the RUN.
func TestANewSessionOverIdenticalContentIsNotAChange(t *testing.T) {
	s := openStore(t, estate(t, "development"))

	if _, err := s.Put(tctx, noop("the title", map[string]string{"a": "1"})); err != nil {
		t.Fatalf("creating: %v", err)
	}

	second := noop("the title", map[string]string{"a": "1"})
	second.IfVersion = 1
	second.Session = "session-two"
	second.Seat = "a-different-seat"
	second.Epoch = 99
	got, err := s.Put(tctx, second)
	if err != nil {
		t.Fatalf("re-putting under a new session: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("a new session over identical content minted version %d, want 1", got.Version)
	}

	// AND THE RETURNED RECORD CARRIES THE PROVENANCE THAT IS STORED, NOT THE
	// ONE THE CALLER OFFERED. Returning the caller's would report a write that
	// did not happen.
	if got.Prov.Session != "session-one" || got.Prov.Seat != "backend-record" || got.Prov.Epoch != 6 {
		t.Fatalf("the no-op returned provenance %+v, want the stored session-one/backend-record/6",
			got.Prov)
	}
	head, err := s.Get(tctx, "B1")
	if err != nil {
		t.Fatal(err)
	}
	if head.Prov.Session != "session-one" {
		t.Fatalf("the head's provenance is now %q; a no-op must not rewrite it", head.Prov.Session)
	}
}

// ⛔ THE PERMISSIVE DIRECTION SILENTLY DROPS REAL EDITS, WHICH IS WORSE THAN
// THE DEFECT BEING FIXED. Every observable part of a record gets its own case.
func TestEveryRealChangeStillMintsAVersion(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(r *PutRequest)
	}{
		{"the body", func(r *PutRequest) { r.Body = "a different title" }},
		{"a field's value", func(r *PutRequest) { r.Fields = map[string]string{"a": "2"} }},
		{"a field added", func(r *PutRequest) { r.Fields = map[string]string{"a": "1", "b": "1"} }},
		{"a field removed", func(r *PutRequest) { r.Fields = map[string]string{} }},
		// KIND AND PROJECT ARE DELIBERATELY NOT HERE. They were, and they
		// passed, because a supersede could change either - which is the
		// relocation defect, not a real edit. They are now REFUSALS, and
		// TestASupersedeCannotRelocateARecordOutOfItsProject below is where
		// they went.
	} {
		t.Run(c.name, func(t *testing.T) {
			s := openStore(t, estate(t, "development"))
			if _, err := s.Put(tctx, noop("the title", map[string]string{"a": "1"})); err != nil {
				t.Fatalf("creating: %v", err)
			}
			changed := noop("the title", map[string]string{"a": "1"})
			changed.IfVersion = 1
			c.change(&changed)

			got, err := s.Put(tctx, changed)
			if err != nil {
				t.Fatalf("superseding: %v", err)
			}
			if got.Version != 2 {
				t.Fatalf("changing %s returned version %d, want 2: a real edit was dropped",
					c.name, got.Version)
			}
			hist, err := s.History(tctx, "B1")
			if err != nil {
				t.Fatal(err)
			}
			if len(hist) != 2 {
				t.Fatalf("changing %s left %d versions in history, want 2", c.name, len(hist))
			}
		})
	}
}

// ⛔ THE COMPARE-AND-SWAP RUNS FIRST AND THE NO-OP DOES NOT EXCUSE A STALE PUT.
//
// A caller naming version 1 of a record that is at version 2 has not seen
// version 2, whatever its content happens to be. Answering "fine, nothing
// changed" would tell them their read was current when it was not.
func TestAStalePutStillConflictsEvenWhenTheContentMatches(t *testing.T) {
	s := openStore(t, estate(t, "development"))

	if _, err := s.Put(tctx, noop("first", map[string]string{"a": "1"})); err != nil {
		t.Fatalf("creating: %v", err)
	}
	second := noop("second", map[string]string{"a": "1"})
	second.IfVersion = 1
	if _, err := s.Put(tctx, second); err != nil {
		t.Fatalf("superseding: %v", err)
	}

	stale := noop("second", map[string]string{"a": "1"})
	stale.IfVersion = 1
	_, err := s.Put(tctx, stale)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("a stale put whose content matches the head returned %v, want a ConflictError", err)
	}
	if conflict.Current != 2 {
		t.Fatalf("the conflict names current version %d, want 2", conflict.Current)
	}
}

// A nil map and an empty one are the same record to every reader, so neither
// is a change into the other. This is the ONE place the comparison is
// deliberately looser than byte equality, and it is looser in a direction
// nothing can observe: `len(Fields)` is 0 either way.
func TestNilAndEmptyFieldsAreTheSameRecord(t *testing.T) {
	s := openStore(t, estate(t, "development"))

	if _, err := s.Put(tctx, noop("the title", nil)); err != nil {
		t.Fatalf("creating: %v", err)
	}
	empty := noop("the title", map[string]string{})
	empty.IfVersion = 1
	got, err := s.Put(tctx, empty)
	if err != nil {
		t.Fatalf("re-putting with an empty map: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("nil -> empty minted version %d, want 1", got.Version)
	}
}

// ⛔ AN ID IS GLOBAL IN THIS SCHEMA, SO A SUPERSEDE COULD ANNEX A RECORD OUT
// OF ITS OWN PROJECT AND EVERY INSTRUMENT WOULD REPORT A CLEAN ANSWER.
//
// `heads` is keyed on `id` alone and `records` on `(id, version)`, so Put's
// head lookup has no project predicate and cannot have one. The store holds
// `B1`..`B63` as ids - which plan/39:800 forbids and :796 rules should be
// UUIDv7 - so the collision is reachable by an importer, not just by malice.
//
// The failure is silent in both directions: the record's own project queries
// EMPTY, which reads as "no such work", and the annexing project gains a
// record nobody put there.
func TestASupersedeCannotRelocateARecordOutOfItsProject(t *testing.T) {
	for _, c := range []struct {
		name    string
		change  func(r *PutRequest)
		wantErr string
	}{
		{"into another project", func(r *PutRequest) { r.Project = "logbook" }, "project"},
		{"into another kind", func(r *PutRequest) { r.Kind = "requirement" }, "kind"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := openStore(t, estate(t, "development"))
			if _, err := s.Put(tctx, noop("the grain", map[string]string{"a": "1"})); err != nil {
				t.Fatalf("creating: %v", err)
			}

			annex := noop("the grain", map[string]string{"a": "1"})
			annex.IfVersion = 1
			c.change(&annex)

			_, err := s.Put(tctx, annex)
			if err == nil {
				// Errorf rather than Fatalf ON PURPOSE: the two checks below
				// are the CONSEQUENCE, and an unfixed run must print them.
				t.Errorf("a put moving B1 %s was accepted; a new version of an id "+
					"must not change what the record IS", c.name)
			} else if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("the refusal does not name %q, so the caller cannot tell "+
					"what it did wrong: %v", c.wantErr, err)
			}

			// THE OBSERVATION THAT MAKES IT FATAL: the record's own project
			// queries empty after the relocation, which reads as "no such work".
			mine, err := s.Query(tctx, "rig", "work-item")
			if err != nil {
				t.Fatal(err)
			}
			if len(mine) != 1 {
				t.Fatalf("Query(rig, work-item) returned %d records, want 1: B1 left "+
					"its own project and its project's query says nothing is there", len(mine))
			}

			head, err := s.Get(tctx, "B1")
			if err != nil {
				t.Fatal(err)
			}
			if head.Project != "rig" || head.Kind != "work-item" || head.Version != 1 {
				t.Fatalf("B1's head is %s/%s v%d, want rig/work-item v1",
					head.Project, head.Kind, head.Version)
			}
		})
	}
}
