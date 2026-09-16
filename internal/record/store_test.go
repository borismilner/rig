package record

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

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
