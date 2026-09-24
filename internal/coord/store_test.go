package coord

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/borismilner/rig/internal/paths"
)

// SECTION 37 PRECONDITION 4, RULING V15: "the epoch is bumped on every start,
// UNCONDITIONALLY".
func TestTheEpochIsBumpedOnEveryStart(t *testing.T) {
	name := estate(t, "development")
	fakeBoot(t)
	fakeClock(t)

	var seen []uint64
	for range 4 {
		s, err := Open(name)
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, s.Epoch())
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for i, got := range seen {
		if want := uint64(i + 1); got != want {
			t.Fatalf("epochs were %v, want 1,2,3,4: start %d published %d", seen, i+1, got)
		}
	}
}

// THE CLAUSE THAT IS EASY TO BUILD WRONG, AND THE WRONG VERSION IS THE HELPFUL
// ONE: a planned restart must be indistinguishable from a crash.
//
// A store closed cleanly and a store abandoned without a Close must publish the
// same next epoch. If a clean shutdown ever recorded "I meant this", something
// downstream would eventually trust it, and a handle that trusts a restart it
// was told about breaks the first time it was told wrong.
func TestAPlannedRestartIsIndistinguishableFromACrash(t *testing.T) {
	fakeBoot(t)
	fakeClock(t)

	// A clean stop.
	clean := estate(t, "development")
	a, err := Open(clean)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := Open(clean)
	if err != nil {
		t.Fatal(err)
	}
	afterClean := b.Epoch()
	_ = b.Close()

	// A crash: the process is gone and Close was never called. Dropping the
	// handle without closing is the nearest thing a single process can do,
	// and the real kill -9 is exercised by the live demonstration.
	crashed := estate(t, "development")
	c, err := Open(crashed)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.db.Close(); err != nil { // the file handle goes, the store does not tidy up
		t.Fatal(err)
	}
	d, err := Open(crashed)
	if err != nil {
		t.Fatal(err)
	}
	afterCrash := d.Epoch()
	_ = d.Close()

	if afterClean != afterCrash {
		t.Fatalf("a clean stop published epoch %d on restart and a crash published %d: "+
			"nothing may distinguish a planned restart from a crash",
			afterClean, afterCrash)
	}
}

// SECTION 37 PRECONDITION 2: "an unnamed estate gets no persistent state at
// all, and that is the answer rather than an omission".
func TestAnUnnamedEstateCannotOpenAStore(t *testing.T) {
	estate(t, "")
	s, err := Open("")
	if err == nil {
		_ = s.Close()
		t.Fatal("an unnamed estate was given a store")
	}
	var unnamed *UnnamedEstateError
	if !errors.As(err, &unnamed) {
		t.Fatalf("the refusal should be an *UnnamedEstateError so a caller can "+
			"tell it from a disk error; got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "no persistent state") {
		t.Fatalf("the refusal should say an unnamed estate has no persistent "+
			"state, so a reader learns it is the answer; got: %v", err)
	}
}

// PRECONDITION 2, THE FAILURE IT ACTUALLY PREVENTS: the development estate
// writing into production's store raises nothing at all, in either direction.
func TestTwoEstatesDoNotShareAStore(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	fakeBoot(t)
	fakeClock(t)
	proc := fakeProc(t)
	spawn(t, proc, 4242, 99)
	w, err := WitnessProcess(4242)
	if err != nil {
		t.Fatal(err)
	}

	prod := openStore(t, "production")
	dev := openStore(t, "development")

	if prod.Path() == dev.Path() {
		t.Fatalf("both estates opened the same store file %s", prod.Path())
	}
	if _, err := prod.Acquire("deploy", "prod-seat", w, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := dev.Inspect("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Free {
		t.Fatalf("development sees production's lease as %s held by %q: the two "+
			"estates share state, which is silent in both directions",
			got.State, got.Holder)
	}
}

// THE SHARED ROOT KEEPS EXACTLY ONE TENANT (section 37, precondition 2). The
// store is strictly below it; the cross-estate name claim is what the root is
// for, and anything else there is visible to both estates by construction.
func TestTheStoreSitsUnderTheEstateSubtreeAndNotAtTheRoot(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	fakeBoot(t)
	fakeClock(t)

	s := openStore(t, "production")
	root, err := paths.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := paths.EstateStateDir("production")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(s.Path()) != sub {
		t.Fatalf("the store is at %s, want it inside %s", s.Path(), sub)
	}
	// Nothing but the estates directory may appear at the root.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "estates" {
			t.Errorf("the shared state root gained %q: everything there is visible "+
				"to BOTH estates, and its one tenant is the cross-estate name claim",
				e.Name())
		}
	}
}

// SECTION 39: "an UNKNOWN (newer) schema version REFUSES TO START. It does not
// guess and it does not repair."
func TestANewerSchemaRefusesToOpen(t *testing.T) {
	name := estate(t, "development")
	fakeBoot(t)
	fakeClock(t)

	s := openStore(t, name)
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// A newer rigd wrote this store.
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	future := make([]byte, 4)
	binary.BigEndian.PutUint32(future, SchemaVersion+1)
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put(keySchema, future)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := Open(name)
	if err == nil {
		_ = got.Close()
		t.Fatal("a store from a newer rigd was opened")
	}
	var future2 *FutureSchemaError
	if !errors.As(err, &future2) {
		t.Fatalf("want *FutureSchemaError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "will not be opened") {
		t.Fatalf("the refusal should say it will not open rather than that it "+
			"failed; got: %v", err)
	}

	// AND IT MUST NOT HAVE BUMPED THE EPOCH ON THE WAY OUT. A refused open is
	// not a start, and leaving a bump behind would make every later epoch
	// wrong by however many times a downgrade was attempted.
	db, err = bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var epoch uint64
	if err := db.View(func(tx *bolt.Tx) error {
		epoch = binary.BigEndian.Uint64(tx.Bucket(bucketMeta).Get(keyEpoch))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if epoch != 1 {
		t.Fatalf("the refused open left epoch %d; a refused open is not a start", epoch)
	}
}

// A reboot is a different and larger event than a restart, and the store knows
// which happened.
func TestAStoreNoticesAReboot(t *testing.T) {
	name := estate(t, "development")
	reboot := fakeBoot(t)
	fakeClock(t)

	s := openStore(t, name)
	if s.Rebooted() {
		t.Fatal("the first open of a new store reported a reboot")
	}
	_ = s.Close()

	again, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if again.Rebooted() {
		t.Fatal("a plain restart reported a reboot")
	}
	_ = again.Close()

	reboot(bootTwo)
	after, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = after.Close() }()
	if !after.Rebooted() {
		t.Fatal("the boot id changed and the store did not notice: every stored " +
			"deadline is on CLOCK_BOOTTIME and is meaningless after a reboot")
	}
}
