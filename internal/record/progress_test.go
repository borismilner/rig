package record

import (
	"errors"
	"strings"
	"testing"
)

// SECTION 39: `progress.step` APPENDS ONE STEP TO A WORK ITEM'S STREAM.
//
// A stream is append-only by construction: every step is its OWN record at
// version 1 and no step is ever superseded. That is what makes "the live state
// is the latest progress.step" true without a second field to keep in sync -
// section 39 is explicit that completion lives in the stream and NOT in
// `status`, and a stream you can rewrite cannot carry that claim.
func TestEachStepIsItsOwnRecordAndTheStreamReadsOldestFirst(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	item := workItem(t, s, "wi-b41", "the CLI roster verb")

	for _, st := range []struct{ state, note string }{
		{"started", "picked it up"},
		{"blocked", "waiting on the wire"},
		{"done", "landed"},
	} {
		if _, err := s.Step(tctx, StepRequest{
			Item: item, State: st.state, Note: st.note, Project: "rig",
			Session: "record", Seat: "backend-record", Epoch: 6,
		}); err != nil {
			t.Fatalf("appending %s: %v", st.state, err)
		}
	}

	stream, err := s.stream(tctx, item)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream) != 3 {
		t.Fatalf("the stream has %d steps, want 3", len(stream))
	}
	for i, want := range []string{"started", "blocked", "done"} {
		if got := stream[i].Fields["state"]; got != want {
			t.Fatalf("step %d is %q, want %q - the stream is not oldest-first", i, got, want)
		}
		if stream[i].Version != 1 {
			t.Fatalf("step %d is at version %d; a step is never superseded", i, stream[i].Version)
		}
	}
	// EVERY STEP IS A DISTINCT RECORD, not three versions of one.
	if stream[0].ID == stream[1].ID || stream[1].ID == stream[2].ID {
		t.Fatal("two steps share an id; the stream is versions of one record, not a stream")
	}
}

// ⛔ THE INVARIANT HAS TO BE ENFORCED, NOT DOCUMENTED. If a caller can reach a
// progress record through the general put, "the latest step is the live state"
// stops being true the first time somebody rewrites history.
func TestAProgressRecordCannotBeWrittenOrRewrittenThroughPut(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	item := workItem(t, s, "wi-b28", "the store question")
	step, err := s.Step(tctx, StepRequest{
		Item: item, State: "started", Note: "picked it up", Project: "rig",
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Creating one directly.
	direct := req("", "a step that did not go through Step")
	direct.Kind = "progress"
	if _, err := s.Put(tctx, direct); err == nil {
		t.Fatal("put created a progress record directly; the stream invariant is unenforced")
	}

	// Superseding the one that exists.
	rewrite := req(step.ID, "it was never blocked, honest")
	rewrite.Kind = "progress"
	rewrite.IfVersion = 1
	if _, err := s.Put(tctx, rewrite); err == nil {
		t.Fatal("put superseded a progress step; a stream that can be rewritten is not a stream")
	}
}

// A step names the item it belongs to and a state the brief can read. Both are
// refused by name rather than stored empty, because a step with no item is
// invisible to every derivation that matters and a bad state silently drops the
// item out of next-up.
func TestAStepIsRefusedByNameWithoutAnItemOrAKnownState(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	item := workItem(t, s, "wi-b21", "an item to hang steps on")

	base := StepRequest{Item: item, State: "started", Project: "rig", Session: "record", Seat: "backend-record", Epoch: 6}

	noItem := base
	noItem.Item = ""
	if _, err := s.Step(tctx, noItem); err == nil || !strings.Contains(err.Error(), "item") {
		t.Fatalf("a step with no item should be refused by name, got: %v", err)
	}

	badState := base
	badState.State = "nearly-done"
	if _, err := s.Step(tctx, badState); err == nil || !strings.Contains(err.Error(), "nearly-done") {
		t.Fatalf("an unknown state should be refused and QUOTED, got: %v", err)
	}

	// ⛔ ASSERT WHICH REFUSAL, NOT THAT ONE HAPPENED. An err != nil here passed
	// against a mutation that had removed the existence check entirely: the
	// lookup still errored, just down a different branch. The second time this
	// seat has shipped that shape, so it is worth stating plainly - where more
	// than one error is reachable, "it failed" is not evidence about which
	// guard is doing the work.
	missing := base
	missing.Item = "wi-does-not-exist"
	_, err := s.Step(tctx, missing)
	if err == nil {
		t.Fatal("a step against an item that does not exist was accepted; it would be invisible to every derivation")
	}
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("a step on a missing item should be refused as NotFound, got %T: %v", err, err)
	}
	if notFound.ID != "wi-does-not-exist" {
		t.Fatalf("the refusal names %q, want the item that was missing", notFound.ID)
	}
}

// workItem writes a work-item record and returns its generated id.
func workItem(t *testing.T, s *Store, id, title string) string {
	t.Helper()
	r := PutRequest{
		ID: id, Kind: "work-item", Project: "rig", Body: title,
		Fields:  map[string]string{"title": title, "status": "active"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	}
	w, err := s.Put(tctx, r)
	if err != nil {
		t.Fatalf("writing work item %s: %v", id, err)
	}
	return w.ID
}
