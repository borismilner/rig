package main

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/paths"
)

// The bbolt layout every rigd before slice 4 wrote, planted by hand so the
// test needs no old binary: meta, leases, messages with one sub-bucket per
// seat and the counter as the root's sequence, messages_meta, and a queue's
// live, done and keys buckets.
func plantBolt(t *testing.T, path string) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	u := func(n int, v uint64) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, v)
		return b[8-n:]
	}
	err = db.Update(func(tx *bolt.Tx) error {
		meta, _ := tx.CreateBucket([]byte("meta"))
		_ = meta.Put([]byte("schema"), u(4, 1))
		_ = meta.Put([]byte("epoch"), u(8, 41))
		_ = meta.Put([]byte("boot_id"), []byte("old-boot"))

		leases, _ := tx.CreateBucket([]byte("leases"))
		_ = leases.Put([]byte("work/a"), []byte(`{"name":"work/a","holder":"lead",`+
			`"token":5,"epoch":41,"witness":{"kind":"unwitnessed"},`+
			`"boot_id":"old-boot","deadline":1,"released":true}`))

		msgs, _ := tx.CreateBucket([]byte("messages"))
		_ = msgs.SetSequence(9)
		lead, _ := msgs.CreateBucket([]byte("lead"))
		for _, id := range []uint64{7, 8} {
			_ = lead.Put(u(8, id), []byte(`{"id":`+string(rune('0'+id))+
				`,"to":"lead","from":"p9","subject":"s","body":"b","state":"QUEUED"}`))
		}
		mm, _ := tx.CreateBucket([]byte("messages_meta"))
		_ = mm.Put([]byte("lead"), u(8, 6))

		qs, _ := tx.CreateBucket([]byte("queues"))
		q, _ := qs.CreateBucket([]byte("jobs"))
		live, _ := q.CreateBucket([]byte("live"))
		done, _ := q.CreateBucket([]byte("done"))
		keys, _ := q.CreateBucket([]byte("keys"))
		_ = live.SetSequence(3)
		_ = live.Put(u(8, 3), []byte(`{"seq":3,"key":"k3","payload":"aGk=","attempts":0}`))
		_ = done.Put(u(8, 1), []byte(`{"seq":1,"key":"k1","payload":"aGk=","attempts":1,"done_by":"w"}`))
		_ = keys.Put([]byte("k1"), u(8, 1))
		_ = keys.Put([]byte("k3"), u(8, 3))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func estatePath(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := paths.EstateStateDir("convtest")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, coord.DBName)
}

// Decision 0251: the epoch never goes backwards, the mail and its counter
// carry over, and the original stays beside the new file.
func TestAConvertedStoreKeepsTheEpochAndTheMail(t *testing.T) {
	path := estatePath(t)
	plantBolt(t, path)

	// Before the conversion the new rigd refuses the file by name.
	if _, err := coord.Open("convtest"); !errors.As(err, new(*coord.BoltFileError)) {
		t.Fatalf("a bbolt coord.db was not refused by name: %v", err)
	}

	sum, err := convert(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sum, "epoch 41") {
		t.Errorf("summary: %s", sum)
	}
	if _, err := os.Stat(path + ".bbolt"); err != nil {
		t.Fatalf("the original was not kept: %v", err)
	}

	s, err := coord.Open("convtest")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if s.Epoch() != 42 {
		t.Errorf("epoch %d after converting 41 and opening once, want 42", s.Epoch())
	}

	if hi, _ := s.Highest(); hi != 9 {
		t.Errorf("the message counter is %d, want the bucket's sequence 9", hi)
	}
	b, err := s.Inbox("lead", 0, 0)
	if err != nil || len(b.Messages) != 2 || !b.Gap {
		t.Errorf("inbox from 0: %+v %v; want two messages and a gap below 6", b, err)
	}
	if b, _ := s.Inbox("lead", 6, 0); b.Gap {
		t.Error("a cursor at the trim line reported a gap")
	}
	m, err := s.Send(coord.Message{To: "lead", From: "p9"}, 1)
	if err != nil || m.ID != 10 {
		t.Errorf("the first new message is %d (%v), want 10", m.ID, err)
	}

	ls, err := s.Leases()
	if err != nil || len(ls) != 1 || ls[0].Holder != "lead" || ls[0].Token != 5 {
		t.Errorf("leases: %+v %v", ls, err)
	}

	tasks, nDone, err := s.Tasks("jobs")
	if err != nil || len(tasks) != 1 || tasks[0].ID != "3" || nDone != 1 {
		t.Errorf("tasks: %+v done %d %v", tasks, nDone, err)
	}
	if _, dup, err := s.Push("jobs", "k1", []byte("hi")); err != nil || !dup {
		t.Errorf("a done task's key pushed again is not a duplicate: %v %v", dup, err)
	}
	if task, _, err := s.Push("jobs", "k4", []byte("x")); err != nil || task.ID != "4" {
		t.Errorf("the next task is %q (%v), want 4: the sequence must carry", task.ID, err)
	}
}

// A second run, or a run after a partial one, changes nothing.
func TestConvertingTwiceIsRefused(t *testing.T) {
	path := estatePath(t)
	plantBolt(t, path)
	if _, err := convert(path); err != nil {
		t.Fatal(err)
	}
	if _, err := convert(path); err == nil || !strings.Contains(err.Error(), "already converted") {
		t.Fatalf("a second conversion: %v", err)
	}
}

// With rigd running the file is locked, and the converter says so.
func TestALockedStoreIsRefused(t *testing.T) {
	path := estatePath(t)
	plantBolt(t, path)
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := convert(path); err == nil || !strings.Contains(err.Error(), "stop the rigd") {
		t.Fatalf("converting a held store: %v", err)
	}
	if _, err := os.Stat(path + ".sqlite-new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused conversion left a file behind")
	}
}
