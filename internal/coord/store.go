// Package coord holds the coordination state that must survive a restart, and
// keys all of it to ONE ESTATE.
//
// It is PLAN.md section 37's isolation preconditions 2 and 4, re-armed by
// section 39 as the first capability that persists anything:
//
//   - PRECONDITION 2, state scoped per estate. Everything here lives under
//     paths.EstateStateDir, which is $XDG_STATE_HOME/rig/estates/<name>/. An
//     UNNAMED ESTATE GETS NO PERSISTENT STATE AT ALL and Open refuses it, which
//     is the answer rather than an omission - see paths.EstateStateDir.
//   - PRECONDITION 4, a restart is survivable and distinguishable from a blip.
//     The daemon publishes an EPOCH, every handle carries it, and it is bumped
//     ON EVERY START, UNCONDITIONALLY.
//
// WHY THE EPOCH IS UNCONDITIONAL, since a planned restart obviously could tell
// the store it was planned. A handle that trusts a restart because it was told
// about that restart in advance is a handle trusting a claim, and it breaks the
// first time the claim is wrong. The cost of treating a planned restart as a
// crash is one re-acquisition; the cost of the reverse is a fencing token that
// outlives the thing it fences, which is the failure leases exist to prevent.
// And rig's own development is the case that creates this constantly: rebuild,
// restart, rebuild is the inner loop of self-hosting, so the estate that walks
// this path most is the one running the work.
//
// THE STORAGE ENGINE IS NOT HAND-BUILT, per section 38b. BACKLOG.md B25 ran the
// library search and resolved it by measurement: go.etcd.io/bbolt carries
// leases, compare-and-swap, the write-ahead log and cursored subscriptions in
// one dependency at +355 KB, MIT. pebble was refused at +19.8 MB. What is
// written here is the SEMANTICS section 16 specifies - the two-step expiry, the
// witness and the fencing - which no key-value store has an opinion about.
package coord

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/borismilner/rig/internal/paths"
)

// SchemaVersion is the on-disk layout this build understands.
//
// IT IS SEPARATE FROM THE WIRE VERSION ON PURPOSE (section 21, section 39): a
// daemon and a store move for different reasons, and one number for both makes
// every wire change look like a migration.
const SchemaVersion = 1

// DBName is the store's file inside the estate's state directory.
const DBName = "coord.db"

// openTimeout bounds the wait for bbolt's own file lock.
//
// Two rigd in one estate is already refused by the pidfile flock before any of
// this runs (section 5f), so a wait here means something is wrong rather than
// busy - a leftover process, or a second binary reaching into an estate it does
// not own. Blocking forever would make that look like a hang.
const openTimeout = 3 * time.Second

var (
	bucketMeta   = []byte("meta")
	bucketLeases = []byte("leases")

	// bucketMessages holds ONE SUB-BUCKET PER RECIPIENT SEAT, keyed by the
	// estate-wide message id. The nesting is what makes "this seat's mail
	// after this cursor" a seek rather than a scan of everybody's, and it is
	// what lets retention be per seat so a chatty pair cannot evict a quiet
	// seat's unread mail.
	//
	// The root bucket's own SEQUENCE is the estate-wide message counter. It
	// is durable, so a cursor survives a restart - which a counter in memory
	// would not, and section 16 requires a queued message to outlive the
	// session it was addressed to.
	bucketMessages = []byte("messages")

	// bucketMsgMeta records, per seat, the highest id retention has dropped.
	// It is separate from the queues because it must not be confused with a
	// message: every key inside a queue is an 8-byte id and nothing else.
	bucketMsgMeta = []byte("messages_meta")

	keySchema = []byte("schema")
	keyEpoch  = []byte("epoch")
	keyBootID = []byte("boot_id")
)

// FutureSchemaError means the store was written by a newer rigd.
//
// IT REFUSES TO START AND DOES NOT REPAIR (section 39). A downgrade that
// silently opens a newer store is how a rollback destroys the data it exists to
// protect: the old binary cannot see the fields it does not know about, writes
// records without them, and the damage is only visible after the rollback is
// rolled back.
type FutureSchemaError struct {
	Path  string
	Found uint32
	Known uint32
}

func (e *FutureSchemaError) Error() string {
	return fmt.Sprintf(
		"the store at %s is schema %d and this rigd understands %d: it was "+
			"written by a newer rigd and will not be opened\n"+
			"       rig does not guess at a newer layout and does not repair one. "+
			"Run the newer rigd, or rebuild the store from its export",
		e.Path, e.Found, e.Known)
}

// UnnamedEstateError means persistent state was asked for without an estate name.
type UnnamedEstateError struct{ Err error }

func (e *UnnamedEstateError) Error() string {
	return "coord: an unnamed estate has no persistent state: " + e.Err.Error()
}
func (e *UnnamedEstateError) Unwrap() error { return e.Err }

// Store is one estate's coordination state.
type Store struct {
	db     *bolt.DB
	estate string
	path   string
	epoch  uint64
	bootID string
	// rebooted records that the machine booted since this store was last
	// opened, which invalidates every stored deadline and every pid witness.
	rebooted bool
}

// Open opens (and creates) the store for a NAMED estate, bumping the epoch.
//
// THE EPOCH BUMP IS PART OF OPENING AND NOT A SEPARATE CALL, so there is no
// path that opens the store without taking a new epoch. A caller that could
// skip it would eventually skip it.
//
// The caller is expected to be holding the estate's single-instance lock
// already (section 5f, instance.Acquire): this is the estate's state and two
// daemons over it is exactly what that lock exists to prevent. bbolt takes its
// own file lock underneath, which is a second line rather than the first.
func Open(estate string) (*Store, error) {
	dir, err := paths.EstateStateDir(estate)
	if err != nil {
		return nil, &UnnamedEstateError{Err: err}
	}
	// 0700: this is the user's own state and nothing here is a socket, so no
	// client boundary rides on the mode (section 14).
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("coord: creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, DBName)

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: openTimeout})
	if err != nil {
		return nil, fmt.Errorf("coord: opening %s: %w", path, err)
	}

	bootID, err := readBootID()
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	s := &Store{db: db, estate: estate, path: path, bootID: bootID}
	if err := s.start(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// start writes the schema, bumps the epoch and records the boot, in ONE
// transaction.
//
// One transaction because a crash between the schema check and the epoch bump
// would leave a store that has been opened without having taken an epoch, and
// the next daemon could not tell. bbolt's transaction is the guarantee that
// there is no such state to be in.
func (s *Store) start() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(bucketMeta)
		if err != nil {
			return fmt.Errorf("coord: meta bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketLeases); err != nil {
			return fmt.Errorf("coord: leases bucket: %w", err)
		}

		// THE MESSAGE BUCKETS ARE CREATED HERE AND THE SCHEMA VERSION DOES
		// NOT MOVE FOR THEM, which is a decision rather than an omission.
		// SchemaVersion exists to stop a rigd opening a store whose LAYOUT it
		// cannot read, and a store that gained a bucket is not that: an older
		// rigd opening this store finds every lease and every meta key
		// exactly where it left them and simply never looks in here. Bumping
		// would make every older binary refuse a store it can read perfectly,
		// which is the damage FutureSchemaError exists to prevent rather than
		// to cause. A CHANGE TO THE SHAPE OF WHAT IS STORED INSIDE THESE
		// BUCKETS IS THE OPPOSITE CASE AND MUST BUMP IT.
		if _, err := tx.CreateBucketIfNotExists(bucketMessages); err != nil {
			return fmt.Errorf("coord: messages bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketMsgMeta); err != nil {
			return fmt.Errorf("coord: messages meta bucket: %w", err)
		}

		if raw := meta.Get(keySchema); raw == nil {
			// A new store. Stamp it.
			if err := meta.Put(keySchema, u32(SchemaVersion)); err != nil {
				return err
			}
		} else {
			found := binary.BigEndian.Uint32(raw)
			if found > SchemaVersion {
				return &FutureSchemaError{Path: s.path, Found: found, Known: SchemaVersion}
			}
			// found < SchemaVersion is where forward migrations run. There are
			// none yet because this IS schema 1; the branch is named so the
			// first one has an obvious home and cannot be bolted onto the
			// refusal above.
			if found < SchemaVersion {
				if err := migrate(tx, found); err != nil {
					return err
				}
				if err := meta.Put(keySchema, u32(SchemaVersion)); err != nil {
					return err
				}
			}
		}

		// THE BUMP. Unconditional, on every start, with nothing consulted.
		epoch := uint64(0)
		if raw := meta.Get(keyEpoch); raw != nil {
			epoch = binary.BigEndian.Uint64(raw)
		}
		epoch++
		if err := meta.Put(keyEpoch, u64(epoch)); err != nil {
			return err
		}
		s.epoch = epoch

		if prev := meta.Get(keyBootID); prev != nil && string(prev) != s.bootID {
			s.rebooted = true
		}
		return meta.Put(keyBootID, []byte(s.bootID))
	})
}

// migrations maps a schema version to the step that moves a store from it to
// the next one. IT IS DELIBERATELY EMPTY: this IS schema 1, so the only value
// that could reach it is 0 and no build stamps 0.
//
// It is a table rather than a chain of ifs because the ladder below then needs
// no editing at all to gain a step, which is the property that keeps a
// migration a local change instead of a rewrite of the thing that runs it.
var migrations = map[uint32]func(*bolt.Tx) error{}

// migrate runs forward-only migrations from an older schema, one step at a
// time, until the store is at SchemaVersion.
//
// Forward only, run at START, idempotent - section 39, and section 18 already
// made the restart the natural moment because rig does not hot-upgrade itself.
// Idempotence matters because the way this actually fails is a crash partway.
//
// IT RETURNS NIL WHEN THERE IS NOTHING TO DO, and that is not a formality.
// The first shape of this function was a stub that returned an error
// unconditionally, which made the caller's `err != nil` dead-true and, worse,
// left whoever writes the first real migration inheriting a function that
// cannot report success. staticcheck caught it as SA4023, reported by
// backend-presence 2026-09-16.
func migrate(tx *bolt.Tx, from uint32) error {
	for v := from; v < SchemaVersion; v++ {
		step, ok := migrations[v]
		if !ok {
			return fmt.Errorf("coord: no migration from schema %d, which this "+
				"build should not be able to produce", v)
		}
		if err := step(tx); err != nil {
			return fmt.Errorf("coord: migrating schema %d to %d: %w", v, v+1, err)
		}
	}
	return nil
}

// Epoch is the epoch this daemon published when it started. Every handle it
// issues carries it.
func (s *Store) Epoch() uint64 { return s.epoch }

// Estate is the estate this store belongs to.
func (s *Store) Estate() string { return s.estate }

// Path is the store file.
func (s *Store) Path() string { return s.path }

// BootID is the boot this daemon started under.
func (s *Store) BootID() string { return s.bootID }

// Rebooted reports whether the machine booted since this store was last opened.
//
// It is worth reporting rather than merely acting on: every deadline in the
// store is meaningless and every pid witness is dead, so a restart after a
// reboot frees leases wholesale, and an operator seeing that wants to know it
// was a reboot rather than a bug.
func (s *Store) Rebooted() bool { return s.rebooted }

// Close closes the store. The epoch is already durable.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return fmt.Errorf("coord: closing %s: %w", s.path, err)
	}
	return nil
}

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func u64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// ErrClosed is returned by a call on a closed store.
var ErrClosed = errors.New("coord: the store is closed")
