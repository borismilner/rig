package coord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

// Section 16's claimable queues: "Claim a task under a lease, heartbeat it, and
// it is requeued if you die. Duplicates are possible: a task carries a
// mandatory idempotency_key and the consumer contract is replay-safety."
//
// A CLAIM IS A LEASE, NOT A SECOND MECHANISM. Claiming a task acquires a lease
// named for it, so the heartbeat is Renew, the two-step expiry is the lease's
// own, and a stale worker is fenced by the lease's token. Section 16: "One
// mechanism, three findings". A task whose claim lease is:
//
//   - held is CLAIMED;
//   - past its deadline with the witness alive or unknown is ORPHANED, and is
//     not offered again, because the worker may only be slow;
//   - free, because the witness was observed dead, released or broken, is
//     READY again, and its attempts count says it was offered before.
//
// AT-LEAST-ONCE, AND IT SAYS SO. A worker that finished and died before
// Complete leaves a task that will run again. The idempotency key rides with
// the task so the consumer can make a replay harmless; rig cannot.
//
// Storage: queues/<queue>/live holds tasks not yet done, keyed by an 8-byte
// sequence so a claim takes the oldest first; queues/<queue>/done holds the
// finished ones; queues/<queue>/keys maps an idempotency key to its task id,
// done or not, so a key pushed again after its task finished is still a
// duplicate. Nothing is trimmed, on section 16's "values are NEVER trimmed".

// TaskState is where a task is.
type TaskState string

const (
	// TaskReady can be claimed.
	TaskReady TaskState = "READY"
	// TaskClaimed is held by a worker inside its lease deadline.
	TaskClaimed TaskState = "CLAIMED"
	// TaskOrphaned is claimed past its deadline by a worker not observed
	// dead. It is not offered again: the worker may only be slow.
	TaskOrphaned TaskState = "ORPHANED"
	// TaskDone was completed by the worker holding its current claim.
	TaskDone TaskState = "DONE"
)

// Bounds. A queue name and an idempotency key are identifiers; a payload is a
// description of work, not the work's data.
const (
	MaxQueueName   = 128
	MaxTaskKey     = 256
	MaxTaskPayload = 64 << 10
)

// claimPrefix begins the name of every lease that is a task's claim.
const claimPrefix = "queue/"

// IsClaimLease reports whether a lease name belongs to a queue's claim. The
// daemon refuses a caller acquiring such a name directly, which would block a
// task nobody claimed.
func IsClaimLease(name string) bool { return strings.HasPrefix(name, claimPrefix) }

// Task is one unit of work in a queue.
type Task struct {
	Queue string
	// ID is the task's sequence number in its queue, as text.
	ID      string
	Key     string
	Payload []byte
	State   TaskState
	// Attempts is how many times the task has been claimed.
	Attempts uint32
	// Claim is the task's lease, evaluated now. Empty for a task never
	// claimed.
	Claim Status
	// DoneBy is the holder that completed it.
	DoneBy string
}

// taskRecord is the stored form.
type taskRecord struct {
	Seq      uint64 `json:"seq"`
	Key      string `json:"key"`
	Payload  []byte `json:"payload"`
	Attempts uint32 `json:"attempts"`
	DoneBy   string `json:"done_by,omitempty"`
}

// EmptyError means a claim found nothing ready.
type EmptyError struct {
	Queue string
	// Claimed and Orphaned count what is there and not offered.
	Claimed, Orphaned int
}

func (e *EmptyError) Error() string {
	return fmt.Sprintf("the queue %q has nothing ready: %d claimed, %d orphaned",
		e.Queue, e.Claimed, e.Orphaned)
}

// KeyConflictError means an idempotency key was pushed again with a different
// payload. The same key twice is a duplicate; the same key over different work
// is a bug in the producer, and taking either silently would lose the other.
type KeyConflictError struct {
	Queue, Key, TaskID string
}

func (e *KeyConflictError) Error() string {
	return fmt.Sprintf("the queue %q already has task %s under the idempotency key %q "+
		"with a different payload", e.Queue, e.TaskID, e.Key)
}

// NotClaimError means a handle given to Complete is not a queue claim.
type NotClaimError struct{ Name string }

func (e *NotClaimError) Error() string {
	return fmt.Sprintf("the lease %q is not a queue claim", e.Name)
}

func checkQueueName(queue string) error {
	if queue == "" || len(queue) > MaxQueueName || !utf8.ValidString(queue) ||
		strings.ContainsFunc(queue, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return fmt.Errorf("coord: a queue name is 1 to %d bytes of printable UTF-8, got %q",
			MaxQueueName, queue)
	}
	return nil
}

func claimName(queue string, seq uint64) string {
	return claimPrefix + queue + "/" + strconv.FormatUint(seq, 10)
}

// parseClaim splits a claim lease name into its queue and sequence. The queue
// may itself contain '/', so the sequence is what follows the last one.
func parseClaim(name string) (string, uint64, bool) {
	rest, ok := strings.CutPrefix(name, claimPrefix)
	if !ok {
		return "", 0, false
	}
	i := strings.LastIndexByte(rest, '/')
	if i <= 0 {
		return "", 0, false
	}
	seq, err := strconv.ParseUint(rest[i+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return rest[:i], seq, true
}

// queueBuckets returns the queue's three buckets, creating them when create is
// set. A queue nobody has pushed to has none, and reading one answers empty.
func queueBuckets(tx *bolt.Tx, queue string, create bool) (live, done, keys *bolt.Bucket, err error) {
	root := tx.Bucket(bucketQueues)
	q := root.Bucket([]byte(queue))
	if q == nil {
		if !create {
			return nil, nil, nil, nil
		}
		if q, err = root.CreateBucket([]byte(queue)); err != nil {
			return nil, nil, nil, fmt.Errorf("coord: creating the queue %q: %w", queue, err)
		}
	}
	names := [][]byte{[]byte("live"), []byte("done"), []byte("keys")}
	out := make([]*bolt.Bucket, len(names))
	for i, n := range names {
		if out[i] = q.Bucket(n); out[i] == nil {
			if out[i], err = q.CreateBucket(n); err != nil {
				return nil, nil, nil, fmt.Errorf("coord: creating the queue %q: %w", queue, err)
			}
		}
	}
	return out[0], out[1], out[2], nil
}

func decodeTask(queue string, raw []byte) (taskRecord, error) {
	var t taskRecord
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, fmt.Errorf("coord: decoding a task in %q: %w", queue, err)
	}
	return t, nil
}

func putTask(b *bolt.Bucket, queue string, t taskRecord) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("coord: encoding a task in %q: %w", queue, err)
	}
	return b.Put(u64(t.Seq), raw)
}

// Push adds a task, or answers the existing one when the idempotency key was
// pushed before. duplicate reports which.
func (s *Store) Push(queue, key string, payload []byte) (task Task, duplicate bool, err error) {
	if s == nil || s.db == nil {
		return Task{}, false, ErrClosed
	}
	if err := checkQueueName(queue); err != nil {
		return Task{}, false, err
	}
	if key == "" || len(key) > MaxTaskKey {
		return Task{}, false, fmt.Errorf("coord: every task carries an idempotency key "+
			"of 1 to %d bytes, and it is mandatory: delivery is at-least-once, so the "+
			"consumer needs it to make a replay harmless; got %d bytes", MaxTaskKey, len(key))
	}
	if len(payload) > MaxTaskPayload {
		return Task{}, false, fmt.Errorf("coord: a task payload is at most %d bytes, got %d",
			MaxTaskPayload, len(payload))
	}
	at, err := now()
	if err != nil {
		return Task{}, false, err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		live, done, keys, err := queueBuckets(tx, queue, true)
		if err != nil {
			return err
		}
		if seqRaw := keys.Get([]byte(key)); seqRaw != nil {
			raw := live.Get(seqRaw)
			if raw == nil {
				raw = done.Get(seqRaw)
			}
			if raw == nil {
				return fmt.Errorf("coord: the key %q in %q names a task that is not stored", key, queue)
			}
			t, err := decodeTask(queue, raw)
			if err != nil {
				return err
			}
			if !bytes.Equal(t.Payload, payload) {
				return &KeyConflictError{Queue: queue, Key: key, TaskID: strconv.FormatUint(t.Seq, 10)}
			}
			task, duplicate = s.taskOf(tx, at, queue, t), true
			return nil
		}
		seq, err := live.NextSequence()
		if err != nil {
			return err
		}
		t := taskRecord{Seq: seq, Key: key, Payload: payload}
		if err := putTask(live, queue, t); err != nil {
			return err
		}
		if err := keys.Put([]byte(key), u64(seq)); err != nil {
			return err
		}
		task = s.taskOf(tx, at, queue, t)
		return nil
	})
	return task, duplicate, err
}

// taskOf evaluates a stored task now, reading its claim lease.
func (s *Store) taskOf(tx *bolt.Tx, at Instant, queue string, t taskRecord) Task {
	out := Task{
		Queue: queue, ID: strconv.FormatUint(t.Seq, 10), Key: t.Key,
		Payload: t.Payload, Attempts: t.Attempts, DoneBy: t.DoneBy, State: TaskReady,
	}
	if t.DoneBy != "" {
		out.State = TaskDone
		return out
	}
	r, err := getRecord(tx, claimName(queue, t.Seq))
	if err != nil || r == nil {
		return out
	}
	state, live := r.evaluate(at, s.bootID)
	out.Claim = r.status(state, live)
	switch state {
	case Held:
		out.State = TaskClaimed
	case Orphaned:
		out.State = TaskOrphaned
	}
	return out
}

// Claim takes the oldest ready task under a lease for holder, witnessed by w.
//
// A TASK THIS HOLDER CLAIMED BEFORE A DAEMON RESTART IS ITS TO TAKE BACK, the
// reconnect path Acquire documents: every handle from the old epoch is fenced,
// so re-claiming it is how the worker gets its work back with a new token. A
// holder's claim from THIS epoch is never handed to it again, because two
// goroutines behind one seat would otherwise fence each other.
func (s *Store) Claim(queue, holder string, w Witness, ttl time.Duration) (Task, Handle, error) {
	if s == nil || s.db == nil {
		return Task{}, Handle{}, ErrClosed
	}
	if err := checkQueueName(queue); err != nil {
		return Task{}, Handle{}, err
	}
	if holder == "" {
		return Task{}, Handle{}, fmt.Errorf("coord: a claim on %q needs a holder", queue)
	}
	if ttl <= 0 {
		return Task{}, Handle{}, fmt.Errorf("coord: a claim on %q was asked for with ttl %s; "+
			"every claim is a lease and there is no infinite hold", queue, ttl)
	}
	at, err := now()
	if err != nil {
		return Task{}, Handle{}, err
	}
	var (
		task Task
		h    Handle
	)
	err = s.db.Update(func(tx *bolt.Tx) error {
		live, _, _, err := queueBuckets(tx, queue, false)
		if err != nil {
			return err
		}
		empty := &EmptyError{Queue: queue}
		if live == nil {
			return empty
		}
		c := live.Cursor()
		for k, raw := c.First(); k != nil; k, raw = c.Next() {
			t, err := decodeTask(queue, raw)
			if err != nil {
				return err
			}
			name := claimName(queue, t.Seq)
			r, err := getRecord(tx, name)
			if err != nil {
				return err
			}
			if r != nil && !r.Released {
				state, _ := r.evaluate(at, s.bootID)
				mine := r.Holder == holder && r.Epoch != s.epoch
				if state == Held && !mine {
					empty.Claimed++
					continue
				}
				if state == Orphaned && !mine {
					empty.Orphaned++
					continue
				}
			}
			if h, err = s.acquireTx(tx, at, name, holder, w, ttl); err != nil {
				return err
			}
			t.Attempts++
			if err := putTask(live, queue, t); err != nil {
				return err
			}
			task = s.taskOf(tx, at, queue, t)
			return nil
		}
		return empty
	})
	if err != nil {
		return Task{}, Handle{}, err
	}
	return task, h, nil
}

// Complete finishes the task a claim handle names, and releases the claim.
//
// IT IS FENCED EXACTLY AS Renew IS: the handle's epoch must be this daemon's
// and its token the claim's current one. A worker that stalled, lost its claim
// to a requeue and woke up is refused here, and its work is the duplicate the
// idempotency key exists for.
func (s *Store) Complete(h Handle) (Task, error) {
	if s == nil || s.db == nil {
		return Task{}, ErrClosed
	}
	queue, seq, ok := parseClaim(h.Name)
	if !ok {
		return Task{}, &NotClaimError{Name: h.Name}
	}
	if h.Epoch != s.epoch {
		return Task{}, &FencedError{Name: h.Name, What: "epoch", Want: s.epoch, Got: h.Epoch}
	}
	at, err := now()
	if err != nil {
		return Task{}, err
	}
	var task Task
	err = s.db.Update(func(tx *bolt.Tx) error {
		r, err := getRecord(tx, h.Name)
		if err != nil {
			return err
		}
		if r == nil || r.Released {
			return &NotHeldError{Name: h.Name}
		}
		if r.Token != h.Token {
			return &FencedError{Name: h.Name, What: "token", Want: r.Token, Got: h.Token}
		}
		live, done, _, err := queueBuckets(tx, queue, false)
		if err != nil {
			return err
		}
		if live == nil {
			return &NotClaimError{Name: h.Name}
		}
		raw := live.Get(u64(seq))
		if raw == nil {
			return &NotClaimError{Name: h.Name}
		}
		t, err := decodeTask(queue, raw)
		if err != nil {
			return err
		}
		t.DoneBy = r.Holder
		if err := putTask(done, queue, t); err != nil {
			return err
		}
		if err := live.Delete(u64(seq)); err != nil {
			return err
		}
		r.Released = true
		r.Witness = NoWitness()
		if err := putRecord(tx, r); err != nil {
			return err
		}
		task = s.taskOf(tx, at, queue, t)
		return nil
	})
	return task, err
}

// Tasks reports a queue's tasks not yet done, oldest first, evaluated now, and
// how many are done.
func (s *Store) Tasks(queue string) ([]Task, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, ErrClosed
	}
	if err := checkQueueName(queue); err != nil {
		return nil, 0, err
	}
	at, err := now()
	if err != nil {
		return nil, 0, err
	}
	var (
		out   []Task
		nDone int
	)
	err = s.db.View(func(tx *bolt.Tx) error {
		live, done, _, err := queueBuckets(tx, queue, false)
		if err != nil || live == nil {
			return err
		}
		nDone = done.Stats().KeyN
		return live.ForEach(func(_, raw []byte) error {
			t, err := decodeTask(queue, raw)
			if err != nil {
				return err
			}
			out = append(out, s.taskOf(tx, at, queue, t))
			return nil
		})
	})
	return out, nDone, err
}

// Queues names every queue that has been pushed to.
func (s *Store) Queues() ([]string, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketQueues).ForEach(func(k, _ []byte) error {
			out = append(out, string(k))
			return nil
		})
	})
	return out, err
}
