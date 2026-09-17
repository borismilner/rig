package record

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// tctx is the context every store call in these tests takes.
//
// ONE ROOT FOR THE WHOLE PACKAGE'S TESTS, AND IT IS ALLOWED TO BE UNDEADLINED.
// Section 3's no-call-without-a-deadline rule is enforced by the nocontextfree
// analyzer, and Product() is false for a test file (internal/analysis.go:382),
// so a test may hold a root context without wrapping it. One package-level
// context rather than one per test because nothing in this package exercises
// cancellation: the deadline these calls exist to honour is the daemon's, and
// the daemon is not in this package. A test that ever needs a cancelled store
// call should derive its own and say why.
var tctx = context.Background()

// estate points the state directory at a temp dir and names the estate.
//
// A NAMED estate, because section 37 precondition 2 gives an unnamed one no
// persistent state at all and Open refuses it. That refusal has its own test
// below rather than being the accidental subject of every other one.
func estate(t *testing.T, name string) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return name
}

// openStore opens and registers the close.
func openStore(t *testing.T, name string) *Store {
	t.Helper()
	s, err := Open(name)
	if err != nil {
		t.Fatalf("opening the record store for %q: %v", name, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// stampVersion writes user_version DIRECTLY, without going through Open.
//
// It is how a store written by a NEWER rigd is simulated without having a newer
// rigd to write one. The value is a constant expression in the test rather than
// anything a caller supplies, which is why the format string is safe here and
// is not a pattern to copy into the package.
func stampVersion(t *testing.T, path string, v uint32) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("reopening %s to stamp it: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("PRAGMA user_version = " + itoa(v)); err != nil {
		t.Fatalf("stamping user_version=%d: %v", v, err)
	}
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

// SECTION 39: "an UNKNOWN (newer) schema version REFUSES TO START. It does not
// guess and it does not repair. A downgrade that silently opens a newer store
// is how data is destroyed by a rollback, which is the one failure a rollback
// exists to avoid."
//
// THIS IS THE ONE REQUIREMENT IN THE SET WHOSE FAILURE IS SILENT, which is why
// it is the first thing this package demonstrates. A migration that does not
// run fails loudly at the first query; a newer store opened by an older binary
// loses fields it cannot see and looks healthy while doing it.
func TestAnUnknownNewerSchemaRefusesToStart(t *testing.T) {
	name := estate(t, "development")

	s, err := Open(name)
	if err != nil {
		t.Fatalf("creating the store: %v", err)
	}
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// A store written by a rigd one schema ahead of this build.
	stampVersion(t, path, SchemaVersion+1)

	reopened, err := Open(name)
	if err == nil {
		_ = reopened.Close()
		t.Fatalf("Open accepted a schema %d store while this build understands %d: "+
			"a newer store opened by an older binary loses every field the older "+
			"binary cannot see, and reports nothing", SchemaVersion+1, SchemaVersion)
	}

	var future *FutureSchemaError
	if !errors.As(err, &future) {
		t.Fatalf("Open refused a newer store with %T (%v), want *FutureSchemaError: "+
			"the refusal has to be distinguishable from an ordinary open failure, "+
			"because a caller has to tell 'run the newer rigd' from 'the disk is full'", err, err)
	}
	if future.Found != SchemaVersion+1 || future.Known != SchemaVersion {
		t.Fatalf("refusal reported found=%d known=%d, want found=%d known=%d",
			future.Found, future.Known, SchemaVersion+1, SchemaVersion)
	}

	// THE REFUSAL HAS TO NAME BOTH NUMBERS AND THE PATH. A refusal that says
	// only "incompatible" leaves the operator with nothing to act on, and this
	// is the message they meet during a rollback that is already going badly.
	msg := future.Error()
	for _, want := range []string{path, "will not be opened", "does not guess"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal message does not contain %q:\n%s", want, msg)
		}
	}
}

// SECTION 39: a fresh store is stamped and opens cleanly, and the ABSENT case
// is distinguishable from the NEWER one.
//
// Conflating the two is how a rollback initialises over live data: "no version"
// and "a version I do not know" both mean "I cannot read this", and only one of
// them is safe to answer by building a new schema.
func TestAFreshStoreIsStampedAndReopens(t *testing.T) {
	name := estate(t, "development")

	s := openStore(t, name)
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(name)
	if err != nil {
		t.Fatalf("reopening a store this build wrote: %v", err)
	}
	defer func() { _ = again.Close() }()

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var got uint32
	if err := db.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != SchemaVersion {
		t.Fatalf("a fresh store is stamped %d, want %d", got, SchemaVersion)
	}
}

// SECTION 37 PRECONDITION 2, re-armed by section 39 as the first capability
// that persists anything: an UNNAMED estate gets no persistent state at all.
func TestAnUnnamedEstateIsRefused(t *testing.T) {
	estate(t, "")

	s, err := Open("")
	if err == nil {
		_ = s.Close()
		t.Fatal("Open created a record store for an unnamed estate: two estates " +
			"sharing one uid would then share one record")
	}
	var unnamed *UnnamedEstateError
	if !errors.As(err, &unnamed) {
		t.Fatalf("refused an unnamed estate with %T (%v), want *UnnamedEstateError", err, err)
	}
}

// findFixture writes nine records across three projects and four kinds, so a
// filter can be wrong in every direction and be caught.
//
// ⛔ ONE KIND EXISTS IN ONE PROJECT ONLY (`census-only` in `standards`). That
// is the record a census under the old both-required rule could never see, and
// it is here so "every kind" is tested against a kind nobody would guess.
func findFixture(t *testing.T, s *Store) {
	t.Helper()
	rows := []struct{ id, kind, project string }{
		{"rig-req-1", "requirement", "rig"},
		{"rig-req-2", "requirement", "rig"},
		{"rig-dec-1", "decision", "rig"},
		{"rig-wi-1", "work-item", "rig"},
		{"std-req-1", "requirement", "standards"},
		{"std-cen-1", "census-only", "standards"},
		{"other-req-1", "requirement", "other"},
		{"other-dec-1", "decision", "other"},
		{"other-dec-2", "decision", "other"},
	}
	for _, r := range rows {
		if _, err := s.Put(tctx, PutRequest{
			ID: r.id, Kind: r.kind, Project: r.project, Body: "",
			Fields:  map[string]string{"title": r.id},
			Session: "record", Seat: "backend-record", Epoch: 6,
		}); err != nil {
			t.Fatalf("writing %s: %v", r.id, err)
		}
	}
}

func foundIDs(rs []Record) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// ⛔ AN EMPTY FILTER FIELD MEANS EVERY VALUE, AND A WRONG ONE MEANS NOTHING.
//
// The two go in one test on purpose, because they are the two mutations of the
// same line and only the pair pins it. An implementation that always applies
// the predicate fails the empty case; one that applies it only sometimes, or
// that treats a non-match as "no filter", passes the empty case and hands a
// caller the whole store for a typo.
//
// The gap this closes: `kind` is not a closed set, so while a query REQUIRED
// one, a record under an unguessed kind was invisible to every question
// anybody could write and no census could be complete.
func TestAnEmptyFilterFieldMeansEveryValueAndAWrongOneMeansNothing(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	findFixture(t, s)

	all, err := s.Find(tctx, QueryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 9 {
		t.Fatalf("an empty filter returned %d records %v, want all 9: an empty "+
			"field means every value, and a census that cannot ask for "+
			"everything is a census that can never be complete",
			len(all), foundIDs(all))
	}

	everyKindInRig, err := s.Find(tctx, QueryFilter{Project: "rig"})
	if err != nil {
		t.Fatal(err)
	}
	if len(everyKindInRig) != 4 {
		t.Errorf("project rig with no kind returned %d records %v, want 4",
			len(everyKindInRig), foundIDs(everyKindInRig))
	}

	everyProjectOneKind, err := s.Find(tctx, QueryFilter{Kind: "requirement"})
	if err != nil {
		t.Fatal(err)
	}
	if len(everyProjectOneKind) != 4 {
		t.Errorf("kind requirement with no project returned %d records %v, "+
			"want 4 across rig, standards and other. An absent project is what "+
			"lets a caller ask which projects exist at all",
			len(everyProjectOneKind), foundIDs(everyProjectOneKind))
	}

	both, err := s.Find(tctx, QueryFilter{Project: "rig", Kind: "requirement"})
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 2 {
		t.Errorf("project rig and kind requirement returned %d records %v, want 2",
			len(both), foundIDs(both))
	}

	// ⛔ THE SECOND MUTATION. An empty value and an unserved field are the same
	// bytes on the wire, so "empty means all" is only safe if a NON-EMPTY
	// value that matches nothing returns nothing rather than everything.
	wrongProject, err := s.Find(tctx, QueryFilter{Project: "no-such-project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(wrongProject) != 0 {
		t.Errorf("a project that does not exist returned %d records %v, want 0. "+
			"A mistyped filter must answer nothing, never everything",
			len(wrongProject), foundIDs(wrongProject))
	}

	wrongKind, err := s.Find(tctx, QueryFilter{Kind: "no-such-kind"})
	if err != nil {
		t.Fatal(err)
	}
	if len(wrongKind) != 0 {
		t.Errorf("a kind that does not exist returned %d records %v, want 0",
			len(wrongKind), foundIDs(wrongKind))
	}

	wrongPair, err := s.Find(tctx, QueryFilter{Project: "rig", Kind: "census-only"})
	if err != nil {
		t.Fatal(err)
	}
	if len(wrongPair) != 0 {
		t.Errorf("a kind that exists in another project returned %d records %v "+
			"for project rig, want 0: the predicates are AND-ed, not OR-ed",
			len(wrongPair), foundIDs(wrongPair))
	}
}

// ⛔ THE CENSUS THE STORE COULD NOT ANSWER, ANSWERED.
//
// This is the whole point of the optional kind, expressed as the question that
// cost an attack specialist 13 round trips: what kinds does this store hold?
// `census-only` is in the fixture precisely because no reader of plan/39 would
// guess it, so a census built by enumerating known kinds misses it and reports
// a complete-looking answer.
func TestEveryKindIsReachableWithoutGuessingItsName(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	findFixture(t, s)

	all, err := s.Find(tctx, QueryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, r := range all {
		kinds[r.Kind] = true
	}
	for _, want := range []string{"requirement", "decision", "work-item", "census-only"} {
		if !kinds[want] {
			t.Errorf("kind %q is in the store and an unfiltered query did not "+
				"reach it; a census that cannot enumerate kinds cannot be "+
				"complete, which is the defect this closes", want)
		}
	}

	projects := map[string]bool{}
	for _, r := range all {
		projects[r.Project] = true
	}
	if len(projects) != 3 {
		t.Errorf("an unfiltered query saw %d projects %v, want 3. Enumerating "+
			"projects is what the window's project tab has to do, and there is "+
			"no other verb that answers it", len(projects), projects)
	}
}

// ⛔ A SCOPED FIND IS ORDERED BY ID, EXACTLY AS Query WAS.
//
// The ORDER BY grew two leading keys so an unscoped answer is readable. Inside
// one project and one kind both are constant, so the order a caller already
// depends on must not have moved.
func TestAScopedFindIsStillOrderedByID(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	findFixture(t, s)

	got, err := s.Find(tctx, QueryFilter{Project: "other", Kind: "decision"})
	if err != nil {
		t.Fatal(err)
	}
	ids := foundIDs(got)
	if len(ids) != 2 || ids[0] != "other-dec-1" || ids[1] != "other-dec-2" {
		t.Fatalf("a scoped find returned %v, want [other-dec-1 other-dec-2] in "+
			"that order", ids)
	}
}
