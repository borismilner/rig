package record

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func req(id string, body string) PutRequest {
	return PutRequest{
		ID: id, Kind: "requirement", Project: "rig", Body: body,
		Session: "rig-backend-record", Seat: "backend-record", Epoch: 6,
	}
}

// SECTION 39, SLICE 1's ACCEPTANCE DEMONSTRATION, in its own words: "a
// requirement is written, superseded twice, and ITS FIRST WORDING IS READ BACK
// WITH THE SESSION THAT WROTE IT."
//
// This is the whole case for append-only in one test. A document answers "what
// does this say"; only a record answers "what did this say BEFORE, and who
// changed it".
func TestARequirementSupersededTwiceStillReadsBackItsFirstWording(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	first := req("req-tray", "the tray icon is rig's, not AgentBox's")
	first.Session = "session-one"
	v1, err := s.Put(first)
	if err != nil {
		t.Fatalf("writing the requirement: %v", err)
	}
	if v1.Version != 1 {
		t.Fatalf("a new record is version %d, want 1", v1.Version)
	}

	second := req("req-tray", "the tray icon is rig's, and AgentBox embeds")
	second.Session = "session-two"
	second.IfVersion = 1
	if _, err := s.Put(second); err != nil {
		t.Fatalf("superseding once: %v", err)
	}

	third := req("req-tray", "one tray icon where six were")
	third.Session = "session-three"
	third.IfVersion = 2
	if _, err := s.Put(third); err != nil {
		t.Fatalf("superseding twice: %v", err)
	}

	// THE HEAD IS THE LATEST WORDING.
	head, err := s.Get("req-tray")
	if err != nil {
		t.Fatal(err)
	}
	if head.Version != 3 || head.Body != "one tray icon where six were" {
		t.Fatalf("head is v%d %q, want v3 and the third wording", head.Version, head.Body)
	}

	// AND THE FIRST WORDING IS STILL THERE, WITH THE SESSION THAT WROTE IT.
	original, err := s.GetVersion("req-tray", 1)
	if err != nil {
		t.Fatalf("reading back the first version: %v", err)
	}
	if original.Body != "the tray icon is rig's, not AgentBox's" {
		t.Fatalf("the first wording read back as %q", original.Body)
	}
	if original.Prov.Session != "session-one" {
		t.Fatalf("the first version's provenance names session %q, want session-one: "+
			"a version whose provenance follows the head is not provenance, it is a "+
			"copy of the present", original.Prov.Session)
	}

	// HISTORY CARRIES ALL THREE, OLDEST FIRST, EACH WITH ITS OWN WRITER.
	hist, err := s.History("req-tray")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("history has %d versions, want 3", len(hist))
	}
	for i, want := range []string{"session-one", "session-two", "session-three"} {
		if hist[i].Version != uint64(i+1) {
			t.Errorf("history[%d] is version %d, want %d", i, hist[i].Version, i+1)
		}
		if hist[i].Prov.Session != want {
			t.Errorf("history[%d] names session %q, want %q", i, hist[i].Prov.Session, want)
		}
	}
}

// SECTION 39, FAILURE SEMANTICS: "two sessions write the same record -
// COMPARE-AND-SWAP ON THE VERSION. A put naming a stale version fails and
// RETURNS THE CURRENT ONE so the caller can merge rather than guess."
//
// EIGHT CONCURRENT PUTS ALL NAMING VERSION 1. Exactly one may win.
//
// WHY CONCURRENT RATHER THAN SEQUENTIAL, and it is the only formulation that
// catches the real defect: a sequential stale put is caught by any check at
// all, including one outside the transaction. Two callers that both READ
// version 1 and then both write are only stopped when the read the swap is
// checked against and the write are the SAME transaction. A sequential test
// passes against an implementation that is broken under exactly the condition
// this exists to protect.
func TestOnlyOneOfEightConcurrentPutsOnTheSameVersionWins(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	if _, err := s.Put(req("req-race", "the first wording")); err != nil {
		t.Fatal(err)
	}

	const racers = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		wins      int
		conflicts []*ConflictError
		other     []error
		start     = make(chan struct{})
	)

	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := req("req-race", "wording from racer")
			r.IfVersion = 1
			<-start
			_, err := s.Put(r)

			mu.Lock()
			defer mu.Unlock()
			var conflict *ConflictError
			switch {
			case err == nil:
				wins++
			case errors.As(err, &conflict):
				conflicts = append(conflicts, conflict)
			default:
				other = append(other, err)
			}
			_ = i
		}()
	}
	close(start)
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("%d puts failed with something other than a conflict, first: %v", len(other), other[0])
	}
	if wins != 1 {
		t.Fatalf("%d of %d concurrent puts on version 1 succeeded, want exactly 1: "+
			"every extra winner is one seat's write silently overwriting another's, "+
			"which is the failure compare-and-swap exists to prevent", wins, racers)
	}
	if len(conflicts) != racers-1 {
		t.Fatalf("%d conflicts, want %d", len(conflicts), racers-1)
	}

	// THE CONFLICT HAS TO CARRY THE CURRENT VERSION, or the caller cannot merge
	// and is pushed into a read-then-retry loop that can livelock.
	for _, c := range conflicts {
		if c.Current != 2 {
			t.Errorf("a conflict reported Current=%d, want 2: without the current "+
				"version the caller has to guess", c.Current)
		}
		if c.Named != 1 {
			t.Errorf("a conflict reported Named=%d, want 1", c.Named)
		}
	}

	// AND THE STORE HOLDS EXACTLY TWO VERSIONS, not eight.
	hist, err := s.History("req-race")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("the store holds %d versions after the race, want 2", len(hist))
	}
}

// SECTION 39: a create against an id that already exists is a CONFLICT rather
// than an overwrite. IfVersion 0 means create.
func TestACreateAgainstAnExistingIDIsRefused(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	if _, err := s.Put(req("req-once", "the first wording")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Put(req("req-once", "a second create, not a supersede"))
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("a create against a taken id returned %v, want *ConflictError: "+
			"otherwise a caller that forgot IfVersion silently overwrites", err)
	}
	if conflict.Current != 1 {
		t.Fatalf("conflict reported Current=%d, want 1", conflict.Current)
	}
}

// SECTION 39: "provenance timestamps are the DAEMON'S, never the client's."
func TestTheStoreStampsTheTimeAndTheCallerCannot(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	fixed := time.Date(2026, 9, 16, 19, 30, 0, 0, time.UTC)
	old := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = old })

	got, err := s.Put(req("req-clock", "written at a known instant"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Prov.CreatedAt.Equal(fixed) {
		t.Fatalf("Put stamped %v, want the daemon clock's %v", got.Prov.CreatedAt, fixed)
	}

	read, err := s.Get("req-clock")
	if err != nil {
		t.Fatal(err)
	}
	if !read.Prov.CreatedAt.Equal(fixed) {
		t.Fatalf("the stamp read back as %v, want %v", read.Prov.CreatedAt, fixed)
	}
}

// SECTION 39: "query by field - THIS IS INDEXED. A requirement cannot hide in
// 5,218 lines because it is not in 5,218 lines."
func TestQueryReturnsHeadsOfOneKindInOneProject(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	for _, id := range []string{"req-a", "req-b"} {
		if _, err := s.Put(req(id, "first")); err != nil {
			t.Fatal(err)
		}
	}
	d := req("dec-a", "a decision, not a requirement")
	d.Kind = "decision"
	if _, err := s.Put(d); err != nil {
		t.Fatal(err)
	}
	o := req("req-other", "another project's requirement")
	o.Project = "agentbox"
	if _, err := s.Put(o); err != nil {
		t.Fatal(err)
	}

	// Supersede one, so the query has a chance to return a stale version.
	bump := req("req-a", "second wording")
	bump.IfVersion = 1
	if _, err := s.Put(bump); err != nil {
		t.Fatal(err)
	}

	got, err := s.Query("rig", "requirement")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		// THREE WAYS TO FAIL THIS AND THE MESSAGE HAS TO NAME ALL THREE. An
		// earlier wording blamed a kind or project leak only, and a mutation
		// that made Query return every version rather than heads tripped it
		// with a message pointing at the wrong cause.
		t.Fatalf("query returned %d records, want 2 (req-a, req-b): either a "+
			"decision leaked in, or another project's requirement did, or a "+
			"SUPERSEDED version came back beside its head", len(got))
	}
	if got[0].ID != "req-a" || got[0].Version != 2 || got[0].Body != "second wording" {
		t.Fatalf("query returned %s v%d %q, want req-a v2 and the second wording: "+
			"a query that returns a superseded version answers about the past",
			got[0].ID, got[0].Version, got[0].Body)
	}
}
