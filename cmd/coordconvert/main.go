// Command coordconvert moves one estate's coord.db from bbolt to SQLite, once
// (decision 0251, plan/48 decision 1, B103 slice 4).
//
// The file holds the epoch, a fencing token that must never go backwards, and
// mail a seat has not read, so it is carried across rather than rebuilt. It
// runs with rigd stopped: bbolt's own file lock refuses it otherwise. The new
// file is written beside the old one and the names are swapped only after it
// is complete, so the original survives as coord.db.bbolt.
//
// IT IS THE LAST IMPORT OF bbolt. When both estates have converted, this
// command and the go.mod line leave together.
package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"

	"github.com/borismilner/rig/internal/coord"
)

// boltSchema is the only bbolt layout any rigd wrote.
const boltSchema = 1

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: coordconvert <path to coord.db>  (rigd stopped)")
		os.Exit(2)
	}
	sum, err := convert(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "coordconvert:", err)
		os.Exit(1)
	}
	fmt.Println(sum)
}

// convert reads path as bbolt, writes path.sqlite-new, and swaps the names.
func convert(path string) (string, error) {
	backup, fresh := path+".bbolt", path+".sqlite-new"
	for _, p := range []string{backup, fresh} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s exists: an earlier run stopped partway, or the "+
				"estate is already converted. Nothing was changed", p)
		}
	}
	c, err := read(path)
	if err != nil {
		return "", err
	}
	if err := coord.WriteConverted(fresh, c); err != nil {
		_ = os.Remove(fresh)
		return "", err
	}
	for _, side := range []string{"-wal", "-shm"} {
		_ = os.Remove(fresh + side)
	}
	// Hard link first, so there is no moment with no coord.db and the
	// original is never moved by a rename that could be half done.
	if err := os.Link(path, backup); err != nil {
		return "", fmt.Errorf("keeping the original as %s: %w", backup, err)
	}
	if err := os.Rename(fresh, path); err != nil {
		return "", fmt.Errorf("putting the new store in place: %w (the original is "+
			"unchanged at %s)", err, path)
	}
	return fmt.Sprintf("converted %s: epoch %d, %d leases, %d messages (counter %d), "+
		"%d queues, %d tasks; the original is %s",
		path, c.Epoch, len(c.Leases), len(c.Messages), c.MessageSeq,
		len(c.Queues), len(c.Tasks), backup), nil
}

// read takes everything out of a bbolt coord.db, read-only.
func read(path string) (coord.Converted, error) {
	if _, err := os.Stat(path); err != nil {
		return coord.Converted{}, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: 2 * time.Second})
	if errors.Is(err, berrors.ErrTimeout) {
		return coord.Converted{}, fmt.Errorf("%s is locked: stop the rigd that holds it first", path)
	}
	if err != nil {
		return coord.Converted{}, fmt.Errorf("opening %s as bbolt: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	c := coord.Converted{
		Leases:  map[string][]byte{},
		Trimmed: map[string]uint64{},
		Queues:  map[string]uint64{},
	}
	err = db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("meta"))
		if meta == nil {
			return errors.New("no meta bucket: this is not a coord store")
		}
		if v := be(meta.Get([]byte("schema"))); v != boltSchema {
			return fmt.Errorf("schema %d, and only schema %d was ever written in bbolt", v, boltSchema)
		}
		c.Epoch = be(meta.Get([]byte("epoch")))
		c.BootID = string(meta.Get([]byte("boot_id")))

		if b := tx.Bucket([]byte("leases")); b != nil {
			if err := b.ForEach(func(k, v []byte) error {
				c.Leases[string(k)] = clone(v)
				return nil
			}); err != nil {
				return err
			}
		}
		if root := tx.Bucket([]byte("messages")); root != nil {
			c.MessageSeq = root.Sequence()
			if err := root.ForEachBucket(func(seat []byte) error {
				return root.Bucket(seat).ForEach(func(k, v []byte) error {
					c.Messages = append(c.Messages, coord.ConvertedMessage{
						Seat: string(seat), ID: be(k), JSON: clone(v),
					})
					return nil
				})
			}); err != nil {
				return err
			}
		}
		if b := tx.Bucket([]byte("messages_meta")); b != nil {
			if err := b.ForEach(func(k, v []byte) error {
				c.Trimmed[string(k)] = be(v)
				return nil
			}); err != nil {
				return err
			}
		}
		if root := tx.Bucket([]byte("queues")); root != nil {
			return root.ForEachBucket(func(name []byte) error {
				return readQueue(&c, string(name), root.Bucket(name))
			})
		}
		return nil
	})
	return c, err
}

// readQueue carries one queue's live and done tasks and its sequence. The
// keys bucket is not carried: every task holds its own key, and the new store
// indexes it.
func readQueue(c *coord.Converted, name string, q *bolt.Bucket) error {
	live, done := q.Bucket([]byte("live")), q.Bucket([]byte("done"))
	if live == nil || done == nil {
		return fmt.Errorf("the queue %q has no live or done bucket", name)
	}
	c.Queues[name] = live.Sequence()
	for _, b := range []struct {
		bucket *bolt.Bucket
		done   bool
	}{{live, false}, {done, true}} {
		if err := b.bucket.ForEach(func(k, v []byte) error {
			c.Tasks = append(c.Tasks, coord.ConvertedTask{
				Queue: name, Seq: be(k), Done: b.done, JSON: clone(v),
			})
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// be reads a big-endian counter of 4 or 8 bytes; anything else is zero, which
// every caller refuses or checks.
func be(b []byte) uint64 {
	var v uint64
	if len(b) != 4 && len(b) != 8 {
		return 0
	}
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v
}

// clone copies a value out of bbolt, whose slices die with the transaction.
func clone(b []byte) []byte { return append([]byte(nil), b...) }
