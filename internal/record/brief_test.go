package record

import (
	"reflect"
	"testing"
)

func step(t *testing.T, s *Store, item, state string) {
	t.Helper()
	if _, err := s.Step(StepRequest{
		Item: item, State: state, Project: "rig",
		Session: "record", Seat: "backend-record", Epoch: 6,
	}); err != nil {
		t.Fatalf("stepping %s to %s: %v", item, state, err)
	}
}

// SECTION 39's SLICE 2 ACCEPTANCE TEST: a work item is driven start to finish
// and the report is read off the brief with NO SEAT HAVING WRITTEN A SENTENCE
// OF PROSE. Every string asserted below is either an id or a field the caller
// typed as data - nothing here is narrative.
func TestAWorkItemDrivenStartToFinishLeavesTheBriefAndTheStreamHoldsIt(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	item := workItem(t, s, "b41", "the CLI roster verb")
	step(t, s, item, "started")
	step(t, s, item, "blocked")
	step(t, s, item, "started")

	b, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.NextUp) != 1 || b.NextUp[0].ID != item {
		t.Fatalf("next up is %+v, want just %s", b.NextUp, item)
	}
	if b.NextUp[0].State != "started" {
		t.Fatalf("the brief reports state %q, want the LATEST step", b.NextUp[0].State)
	}

	// Drive it to done, and it leaves the brief entirely.
	step(t, s, item, "done")
	b, err = s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.NextUp) != 0 || len(b.Open) != 0 {
		t.Fatalf("a done item is still in the brief: next=%+v open=%+v", b.NextUp, b.Open)
	}

	// ⛔ AND THE HISTORY SURVIVES IT. The item leaving the brief is a
	// derivation changing its answer, not a record being erased.
	stream, err := s.Stream(item)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream) != 4 {
		t.Fatalf("the stream has %d steps, want all 4 - the brief is derived, not destructive", len(stream))
	}
}

// ⛔ DEFECT D5: `status == active` ALONE SHIPS A LIST THAT NEVER EMPTIES.
// Section 39 puts completion in the stream, so an item whose status is still
// active but whose latest step is done must fall out of next-up. This is the
// test that separates the two readings.
func TestAnItemWhoseLatestStepIsDoneLeavesNextUpEvenWhileStatusSaysActive(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	done := workItem(t, s, "b28", "the store question")
	live := workItem(t, s, "b45", "the depscheck rows")
	step(t, s, done, "done")
	step(t, s, live, "started")

	b, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	ids := briefIDs(b)
	if !reflect.DeepEqual(ids, []string{live}) {
		t.Fatalf("the brief carries %v, want only %s - status alone kept the finished item", ids, live)
	}
}

// Execution order is a topological sort over `blocks`: a blocker comes before
// what it blocks.
func TestNextUpIsATopologicalSortOverBlocks(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	a := workItem(t, s, "b-first", "must happen first")
	b := workItem(t, s, "b-second", "waits on the first")
	c := workItem(t, s, "b-third", "waits on the second")
	for _, id := range []string{a, b, c} {
		step(t, s, id, "started")
	}
	// a blocks b blocks c.
	if err := s.Link(a, LinkBlocks, b); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(b, LinkBlocks, c); err != nil {
		t.Fatal(err)
	}

	br, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	if got := briefIDs(br); !reflect.DeepEqual(got, []string{a, b, c}) {
		t.Fatalf("execution order is %v, want %v - the sort is not honouring blocks", got, []string{a, b, c})
	}
	if len(br.Cycles) != 0 {
		t.Fatalf("an acyclic graph reported cycles: %v", br.Cycles)
	}

	// WHAT IS BLOCKED, AND ON WHOM.
	if len(br.Blocked) != 2 {
		t.Fatalf("blocked list is %+v, want two entries", br.Blocked)
	}
	for _, bl := range br.Blocked {
		if len(bl.BlockedBy) != 1 {
			t.Fatalf("%s is blocked by %v, want exactly one", bl.Item, bl.BlockedBy)
		}
	}
}

// ⛔ THE CYCLE REPORT, WATCHED FIRING. Section 39: detected, reported, ordered
// around, NEVER resolved. rig does not pick an edge to break - that is a
// judgement about the work, which is domain logic and non-goal 1.
func TestABlocksCycleIsReportedByNameAndTheRestIsStillOrdered(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	free := workItem(t, s, "b-free", "not in the cycle")
	x := workItem(t, s, "b-x", "x")
	y := workItem(t, s, "b-y", "y")
	z := workItem(t, s, "b-z", "z")
	for _, id := range []string{free, x, y, z} {
		step(t, s, id, "started")
	}
	// x -> y -> z -> x
	for _, e := range [][2]string{{x, y}, {y, z}, {z, x}} {
		if err := s.Link(e[0], LinkBlocks, e[1]); err != nil {
			t.Fatal(err)
		}
	}

	br, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(br.Cycles) != 1 {
		t.Fatalf("cycles reported: %v, want exactly one", br.Cycles)
	}
	want := []string{x, y, z}
	if !reflect.DeepEqual(br.Cycles[0], want) {
		t.Fatalf("the cycle names %v, want %v", br.Cycles[0], want)
	}

	// ⛔ AND IT STILL ANSWERS. Refusing to answer is the other failure section
	// 39 names, beside swallowing it silently.
	ids := briefIDs(br)
	if len(ids) != 4 {
		t.Fatalf("the brief dropped items because of the cycle: %v", ids)
	}
	if ids[0] != free {
		t.Fatalf("the orderable item is not first: %v", ids)
	}
	// NOTHING WAS RESOLVED: all three edges survive.
	for _, e := range [][2]string{{x, y}, {y, z}, {z, x}} {
		out, err := s.LinksFrom(e[0], LinkBlocks)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || out[0] != e[1] {
			t.Fatalf("rig broke an edge to resolve the cycle: %s -> %v", e[0], out)
		}
	}
}

// An item merely DOWNSTREAM of a cycle is stuck, but it is not the problem and
// naming it in the cycle report sends a reader to the wrong edge.
func TestAnItemDownstreamOfACycleIsNotNamedAsPartOfIt(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	x := workItem(t, s, "b-x", "x")
	y := workItem(t, s, "b-y", "y")
	down := workItem(t, s, "b-down", "downstream of the cycle")
	for _, id := range []string{x, y, down} {
		step(t, s, id, "started")
	}
	for _, e := range [][2]string{{x, y}, {y, x}, {y, down}} {
		if err := s.Link(e[0], LinkBlocks, e[1]); err != nil {
			t.Fatal(err)
		}
	}

	br, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(br.Cycles) != 1 || !reflect.DeepEqual(br.Cycles[0], []string{x, y}) {
		t.Fatalf("cycle reported as %v, want just [%s %s]", br.Cycles, x, y)
	}
}

// next_up_n is the project's, default 5, and the two lists are DISJOINT -
// section 39's defect D2: an item in both got two incompatible rendering rules.
func TestNextUpNSplitsTheListsAndTheyAreDisjoint(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	if _, err := s.Put(PutRequest{
		ID: "rig", Kind: "project", Project: "rig", Body: "rig itself",
		Fields:  map[string]string{"title": "rig", "status": "active", "next_up_n": "2"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"i1", "i2", "i3", "i4"} {
		id := workItem(t, s, n, n)
		step(t, s, id, "started")
	}

	br, err := s.Brief("rig")
	if err != nil {
		t.Fatal(err)
	}
	if len(br.NextUp) != 2 {
		t.Fatalf("next up has %d, want the project's next_up_n of 2", len(br.NextUp))
	}
	if len(br.Open) != 2 {
		t.Fatalf("open has %d, want the remaining 2", len(br.Open))
	}
	seen := map[string]bool{}
	for _, it := range append(append([]ItemState{}, br.NextUp...), br.Open...) {
		if seen[it.ID] {
			t.Fatalf("%s is in BOTH lists; they are specified disjoint", it.ID)
		}
		seen[it.ID] = true
	}
}

func briefIDs(b Brief) []string {
	out := make([]string, 0, len(b.NextUp)+len(b.Open))
	for _, it := range b.NextUp {
		out = append(out, it.ID)
	}
	for _, it := range b.Open {
		out = append(out, it.ID)
	}
	return out
}
