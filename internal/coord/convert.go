package coord

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/borismilner/rig/internal/store"
)

// Converted is everything the one-off converter carries out of a bbolt
// coord.db (decision 0251). The values are the JSON the buckets held, which is
// the JSON this package stores, so nothing is re-encoded on the way across.
type Converted struct {
	// Epoch is carried as it is; the next Open bumps it, so it never goes
	// backwards.
	Epoch  uint64
	BootID string
	// MessageSeq is the messages bucket's own sequence, the estate-wide
	// message counter. It is carried and not recomputed from the highest id
	// held, because a trimmed newest message would make that fall back.
	MessageSeq uint64
	Leases     map[string][]byte
	Messages   []ConvertedMessage
	Trimmed    map[string]uint64
	// Queues maps a queue to its sequence, the live bucket's.
	Queues map[string]uint64
	Tasks  []ConvertedTask
}

// ConvertedMessage is one message and the seat queue it was in.
type ConvertedMessage struct {
	Seat string
	ID   uint64
	JSON []byte
}

// ConvertedTask is one task, and whether it was in the done bucket.
type ConvertedTask struct {
	Queue string
	Seq   uint64
	Done  bool
	JSON  []byte
}

// WriteConverted builds a NEW coord store at path from c, in one transaction.
//
// IT REFUSES AN EXISTING FILE: the converter writes beside the original and
// swaps names only after this returns, so nothing here may overwrite. Every
// value is decoded before it is written, so a record the new store could not
// read is found now rather than on the first lease call after the switch.
func WriteConverted(path string, c Converted) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("coord: %s exists; the converter writes only a new file", path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), txTimeout)
	defer cancel()
	db, err := store.OpenDB(ctx, path, schema)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("coord: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeConverted(ctx, tx, c); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("coord: committing %s: %w", path, err)
	}
	// Folded back into the main file, so the result is one file to rename.
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("coord: checkpointing %s: %w", path, err)
	}
	return nil
}

func writeConverted(ctx context.Context, tx *sql.Tx, c Converted) error {
	if c.Epoch == 0 || c.BootID == "" {
		return fmt.Errorf("coord: the source has epoch %d and boot id %q; a store "+
			"that was ever opened has both", c.Epoch, c.BootID)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta (id, epoch, boot_id, message_seq) VALUES (1, ?, ?, ?)`,
		c.Epoch, c.BootID, c.MessageSeq); err != nil {
		return fmt.Errorf("coord: writing the epoch: %w", err)
	}
	for name, raw := range c.Leases {
		var r record
		if err := json.Unmarshal(raw, &r); err != nil || r.Name != name {
			return fmt.Errorf("coord: the lease %q does not decode as one: %w", name, err)
		}
		if err := putRecord(ctx, tx, &r); err != nil {
			return err
		}
	}
	for _, cm := range c.Messages {
		var m Message
		if err := json.Unmarshal(cm.JSON, &m); err != nil || m.ID != cm.ID || m.To != cm.Seat {
			return fmt.Errorf("coord: message %d for seat %q does not decode as one: %w",
				cm.ID, cm.Seat, err)
		}
		if cm.ID > c.MessageSeq {
			return fmt.Errorf("coord: message %d is above the counter %d", cm.ID, c.MessageSeq)
		}
		if err := putMessage(ctx, tx, m); err != nil {
			return err
		}
	}
	for seat, through := range c.Trimmed {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO messages_trimmed (seat, through) VALUES (?, ?)`,
			seat, through); err != nil {
			return fmt.Errorf("coord: writing the trim line for seat %q: %w", seat, err)
		}
	}
	for queue, seq := range c.Queues {
		if err := checkQueueName(queue); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO queues (name, seq) VALUES (?, ?)`, queue, seq); err != nil {
			return fmt.Errorf("coord: writing the queue %q: %w", queue, err)
		}
	}
	for _, ct := range c.Tasks {
		seq, ok := c.Queues[ct.Queue]
		if !ok {
			return fmt.Errorf("coord: task %d names the queue %q, which has no row", ct.Seq, ct.Queue)
		}
		t, err := decodeTask(ct.Queue, ct.JSON)
		if err != nil {
			return err
		}
		if t.Seq != ct.Seq || t.Seq > seq {
			return fmt.Errorf("coord: task %d in %q is stored under %d, or above the "+
				"queue's sequence %d", t.Seq, ct.Queue, ct.Seq, seq)
		}
		if err := putTask(ctx, tx, ct.Queue, t, ct.Done); err != nil {
			return err
		}
	}
	return nil
}
