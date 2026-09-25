package coord

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	bolt "go.etcd.io/bbolt"
)

// Section 16's directed messages, stored. A message is addressed to a SEAT and
// carries the GENERATION it was addressed to, which is the half of section 16
// paragraph 3 that no amount of care can replace:
//
//	"A message carries the generation it was addressed to, so a receiver can
//	tell it is not the session the sender believes it is talking to."
//
// ⛔ IT IS DURABLE, AND THAT IS NOT A DEFAULT COPIED FROM THE LEASES. Presence
// is deliberately connection state (internal/daemon/presence.go says so at
// length): an occupant lives exactly as long as its connection. Section 16
// paragraph 2 requires the opposite of a queued message - "a session's death
// does not destroy it and a successor claiming that seat receives it" - so the
// queue cannot live where the roster lives. It lives here, beside the leases,
// in the same per-estate store.
//
// WHAT THIS FILE DOES NOT DECIDE. Who may send, who a seat's current occupant
// is, and whether a pinned generation matches are all questions about the
// ROSTER, which is connection state one layer up. This file stores what it is
// told and reports what it holds; internal/daemon/message.go is where the
// refusals live, for the same reason internal/daemon/lease.go owns the holder
// and the witness rather than internal/coord/lease.go.

// MessageState is section 16 paragraph 2's five states, and WHO OWNS EACH
// TRANSITION is the whole content of the table.
//
// ⛔ NOTHING PROMOTES A MESSAGE ON THE RECIPIENT'S BEHALF. The daemon owns
// Queued and Delivered; the recipient owns Read, Acknowledged and ActedOn, and
// the last two are explicit calls rather than inferences from delivery. The
// failure being designed out is a sender treating "delivered" as agreement.
type MessageState string

const (
	// Queued: accepted and durable. A message nobody was parked for is
	// queued, and `delivered: 0` means nobody was parked - NOT that anything
	// was lost.
	Queued MessageState = "QUEUED"
	// Delivered: handed to a recipient's open subscription. The daemon sets
	// it when a parked await is woken by this message.
	Delivered MessageState = "DELIVERED"
	// Read: returned from an inbox or an await into the recipient's own
	// context. The recipient owns this one, and it is the first state the
	// recipient's own act produces.
	Read MessageState = "READ"
	// Acknowledged: the recipient states it understood. NEVER inferred.
	Acknowledged MessageState = "ACKNOWLEDGED"
	// ActedOn: the recipient states what it did, and it carries the outcome.
	// The only state a sender may plan against.
	ActedOn MessageState = "ACTED_ON"
)

// rank orders the five states so a promotion can never go backwards. A
// recipient that acknowledges a message twice, or acknowledges one it already
// acted on, must not lose the stronger fact.
var rank = map[MessageState]int{
	Queued: 0, Delivered: 1, Read: 2, Acknowledged: 3, ActedOn: 4,
}

// Message is one directed message as it is stored and as a reader sees it.
//
// THE IDENTITY OF THE ADDRESSEE IS FOUR FIELDS AND NOT ONE. (estate, seat,
// epoch, generation) is what proto Seat spends four paragraphs establishing,
// and a message that carried only the seat name would be addressed to a role
// rather than to a tenancy of it. Estate is implied by the store - one store
// is one estate - so three of the four are here.
type Message struct {
	// ID is the estate-wide sequence number, and it is the CURSOR UNIT. It is
	// monotonic across a daemon restart because it comes from the bucket's
	// own durable sequence rather than from a counter in memory.
	ID uint64 `json:"id"`

	// To is the recipient SEAT NAME - the addressable identity, independent
	// of who occupies it.
	To string `json:"to"`

	// ToGeneration is the generation the sender ADDRESSED, and zero means the
	// sender addressed the seat rather than a tenancy of it. Zero is a case
	// and not a missing value: "whoever holds this seat" is a legitimate and
	// common thing to mean, and it is distinct from "generation 1".
	ToGeneration uint64 `json:"to_generation"`

	// ToEpoch is which daemon run ToGeneration was counted in, and it is
	// carried for the reason proto Seat carries it beside the generation: the
	// counter restarts at 1 on every daemon start, so a generation alone
	// identifies nothing. Zero exactly when ToGeneration is zero.
	ToEpoch uint64 `json:"to_epoch"`

	// From, FromGeneration and FromEpoch are the sender's own identity, taken
	// from its roster row by the daemon and never read off the request.
	From           string `json:"from"`
	FromGeneration uint64 `json:"from_generation"`
	FromEpoch      uint64 `json:"from_epoch"`

	// Subject is one line; Body is the message. Both are bounded, and a
	// message over the bound is REFUSED rather than truncated (section 16:
	// "the bound is stated, and a message over it is refused").
	Subject string `json:"subject"`
	Body    string `json:"body"`

	State MessageState `json:"state"`

	// HeldForSuccessor records that the seat was HANDING_OFF when this was
	// accepted, so it was queued for the successor rather than delivered to
	// the closing session. Section 16 paragraph 3: "this is the whole reason
	// the state is published".
	HeldForSuccessor bool `json:"held_for_successor,omitempty"`

	// ReadGeneration and ReadEpoch are the tenancy that actually read it,
	// which is the fact the whole feature exists to expose. When they differ
	// from ToGeneration and ToEpoch, a session read mail addressed to a
	// DIFFERENT tenancy of its seat, and both ends are told so.
	ReadGeneration uint64 `json:"read_generation,omitempty"`
	ReadEpoch      uint64 `json:"read_epoch,omitempty"`

	// Outcome rides with ActedOn and is the reason that state is worth
	// having: a sender planning against it needs to know WHAT was done.
	Outcome string `json:"outcome,omitempty"`

	// SentUnixNano and MovedUnixNano are wall-clock, unlike every deadline in
	// this package, and the difference is deliberate: a deadline is a
	// decision rig makes and must be on CLOCK_BOOTTIME, while these two are
	// facts a HUMAN reads out of a log and must line up with everything else
	// on their screen. Nothing expires on them.
	SentUnixNano  int64 `json:"sent_unix_nano"`
	MovedUnixNano int64 `json:"moved_unix_nano"`
}

// MisaddressedTo reports whether this message was addressed to a DIFFERENT
// tenancy of the seat than the one named.
//
// ⛔ THIS IS THE FEATURE. Boris, 2026-09-11: "peers should be aware they may be
// contacted wrongly thinking they are the successor". A message pinned to
// generation 3 and read by generation 4 is a sender who believes it is talking
// to a session that has already gone, and the receiver is the only party in a
// position to notice.
//
// AN UNPINNED MESSAGE IS NEVER MISADDRESSED, because the sender addressed the
// SEAT and any occupant of it is the right recipient. Reporting those as
// misaddressed would make the flag fire on the common case and be ignored.
func (m Message) MisaddressedTo(generation, epoch uint64) bool {
	if m.ToGeneration == 0 {
		return false
	}
	return m.ToGeneration != generation || m.ToEpoch != epoch
}

// maxSubject and maxBody bound a message. A payload where a POINTER belongs is
// the named anti-pattern in section 16, and these are the bound it says must
// be stated: a subject is a line, a body is a paragraph and a hand-off brief,
// and anything larger is a record id or a file path.
//
// THE REFUSAL IS THE POINT RATHER THAN THE NUMBER. Truncating would deliver a
// message whose missing half is invisible to the recipient, which is worse
// than not delivering it - the recipient acts on what arrived.
const (
	MaxSubject = 256
	MaxBody    = 8192
)

// MaxPerSeat is how many messages one seat's queue retains.
//
// RETENTION IS WHY `gap` EXISTS AND THE TWO SHIP TOGETHER. Section 16: when a
// cursor is older than retention the batch CANNOT be complete, and a silently
// incomplete batch is how two agents come to believe they each own one chunk.
// So trimming records what it trimmed through, and any cursor below that line
// is answered with gap set rather than with a short batch that looks whole.
//
// It is a per-seat cap rather than an estate-wide one so a chatty pair cannot
// evict a quiet seat's unread mail.
const MaxPerSeat = 512

// OversizeError refuses a message over its bound, naming which field.
type OversizeError struct {
	What  string
	Bytes int
	Bound int
}

func (e *OversizeError) Error() string {
	return fmt.Sprintf(
		"the %s is %d bytes and the bound is %d: a message over the bound is "+
			"refused rather than truncated, because a recipient acts on what "+
			"arrived and cannot see the half that did not\n"+
			"       put a pointer where the payload is - a record id, a path - "+
			"and send that",
		e.What, e.Bytes, e.Bound)
}

// NoSuchMessageError means the id is not in this seat's queue.
//
// IT NAMES THE SEAT, because the common cause is a recipient acknowledging an
// id it read from somebody else's queue, and "message 7 not found" sends that
// caller looking for a deleted message instead of at its own address.
type NoSuchMessageError struct {
	ID   uint64
	Seat string
}

func (e *NoSuchMessageError) Error() string {
	return fmt.Sprintf("no message %d in the queue for seat %q", e.ID, e.Seat)
}

// Batch is what a cursored read returns.
type Batch struct {
	Messages []Message
	// Cursor is where to resume. It is the highest id CONSIDERED rather than
	// the highest id returned, so a reader whose batch was all for other
	// seats still advances.
	Cursor uint64
	// Gap is true when this batch CANNOT be complete because the cursor was
	// older than retention. Section 16: treat what you were tracking as
	// UNKNOWN, never as not having happened.
	Gap bool
}

// Send stores a message and returns it with its id and timestamps filled in.
//
// THE CALLER SUPPLIES THE IDENTITIES AND THIS FUNCTION TRUSTS THEM, which is
// the same division internal/coord/lease.go draws with the holder and the
// witness: deciding who the sender is requires the roster, and the roster is
// connection state that this package deliberately cannot see.
func (s *Store) Send(m Message, atUnixNano int64) (Message, error) {
	if s == nil || s.db == nil {
		return Message{}, ErrClosed
	}
	if m.To == "" || m.From == "" {
		return Message{}, fmt.Errorf(
			"coord: a message needs a recipient seat and a sender, got %q and %q", m.To, m.From)
	}
	if len(m.Subject) > MaxSubject {
		return Message{}, &OversizeError{What: "subject", Bytes: len(m.Subject), Bound: MaxSubject}
	}
	if len(m.Body) > MaxBody {
		return Message{}, &OversizeError{What: "body", Bytes: len(m.Body), Bound: MaxBody}
	}
	m.State = Queued
	m.SentUnixNano = atUnixNano
	m.MovedUnixNano = atUnixNano

	err := s.db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket(bucketMessages)
		id, err := root.NextSequence()
		if err != nil {
			return fmt.Errorf("coord: taking a message id: %w", err)
		}
		m.ID = id
		q, err := root.CreateBucketIfNotExists([]byte(m.To))
		if err != nil {
			return fmt.Errorf("coord: the queue for seat %q: %w", m.To, err)
		}
		if err := putMessage(q, m); err != nil {
			return err
		}
		return trim(tx, q, m.To)
	})
	if err != nil {
		return Message{}, err
	}
	return m, nil
}

// Inbox returns one seat's messages after a cursor, oldest first.
//
// EVERYTHING SINCE THE CURSOR ARRIVES IN ONE BATCH (section 16), so three
// messages that landed while a seat was editing are one wake-up rather than
// three missed ones. `limit` bounds the batch, and a bounded batch leaves the
// cursor where it stopped rather than pretending it saw the rest.
func (s *Store) Inbox(seat string, after uint64, limit int) (Batch, error) {
	if s == nil || s.db == nil {
		return Batch{}, ErrClosed
	}
	out := Batch{Cursor: after}
	err := s.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(bucketMessages)
		out.Gap = after < trimmedThrough(tx, seat)
		q := root.Bucket([]byte(seat))
		if q == nil {
			return nil
		}
		cur := q.Cursor()
		for k, v := cur.Seek(u64(after + 1)); k != nil; k, v = cur.Next() {
			if limit > 0 && len(out.Messages) >= limit {
				return nil
			}
			var m Message
			if err := json.Unmarshal(v, &m); err != nil {
				return fmt.Errorf("coord: decoding a message for seat %q: %w", seat, err)
			}
			out.Messages = append(out.Messages, m)
			out.Cursor = m.ID
		}
		return nil
	})
	if err != nil {
		return Batch{}, err
	}
	return out, nil
}

// MarkRead promotes a batch to READ and records WHICH TENANCY read it.
//
// THE READING TENANCY IS RECORDED RATHER THAN DERIVED, and that is what makes
// a misdelivery visible after the fact. A message addressed to generation 3
// and read by generation 4 keeps both numbers, so `message.list` shows a human
// exactly which sender was talking to a session that had already gone.
//
// IT NEVER MOVES A MESSAGE BACKWARDS. A recipient that reads its inbox again
// after acknowledging keeps the acknowledgement: promotion is by rank.
func (s *Store) MarkRead(seat string, ids []uint64, generation, epoch uint64, atUnixNano int64) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	if len(ids) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		q := tx.Bucket(bucketMessages).Bucket([]byte(seat))
		if q == nil {
			return nil
		}
		for _, id := range ids {
			m, err := getMessage(q, id)
			if err != nil {
				return err
			}
			if m == nil {
				// Trimmed between the read and the mark. Not an error: the
				// batch the caller holds is still what it was handed.
				continue
			}
			// The reading tenancy is recorded on the FIRST read and never
			// overwritten, so a successor reading a message its predecessor
			// already read does not erase who it actually reached.
			if m.ReadGeneration == 0 && m.ReadEpoch == 0 {
				m.ReadGeneration, m.ReadEpoch = generation, epoch
			}
			if promote(m, Read, atUnixNano) {
				if err := putMessage(q, *m); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// MarkDelivered records that a parked reader was woken by these messages.
//
// DELIVERED IS THE DAEMON'S AND READ IS THE RECIPIENT'S, which is why this is
// a separate call from MarkRead rather than a flag on it. `delivered: 0` means
// nobody was parked, not that anything was lost, and that distinction is only
// available if the two transitions are recorded by the two different owners.
func (s *Store) MarkDelivered(seat string, ids []uint64, atUnixNano int64) error {
	if s == nil || s.db == nil {
		return ErrClosed
	}
	if len(ids) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		q := tx.Bucket(bucketMessages).Bucket([]byte(seat))
		if q == nil {
			return nil
		}
		for _, id := range ids {
			m, err := getMessage(q, id)
			if err != nil {
				return err
			}
			if m == nil {
				continue
			}
			if promote(m, Delivered, atUnixNano) {
				if err := putMessage(q, *m); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Ack promotes one message to ACKNOWLEDGED or ACTED_ON, with an outcome.
//
// IT IS THE RECIPIENT'S OWN STATEMENT AND IT IS EXPLICIT. Section 16 is
// unambiguous that neither state may be inferred from delivery, which is why
// this is a verb a recipient calls rather than something a read does for it.
func (s *Store) Ack(seat string, id uint64, state MessageState, outcome string, generation, epoch uint64, atUnixNano int64) (Message, error) {
	if s == nil || s.db == nil {
		return Message{}, ErrClosed
	}
	if state != Acknowledged && state != ActedOn {
		return Message{}, fmt.Errorf(
			"coord: a recipient may promote a message to %s or %s and to nothing "+
				"else, got %q: the daemon owns queued and delivered, and read is "+
				"what a read does", Acknowledged, ActedOn, state)
	}
	if len(outcome) > MaxSubject {
		return Message{}, &OversizeError{What: "outcome", Bytes: len(outcome), Bound: MaxSubject}
	}
	var out Message
	err := s.db.Update(func(tx *bolt.Tx) error {
		q := tx.Bucket(bucketMessages).Bucket([]byte(seat))
		if q == nil {
			return &NoSuchMessageError{ID: id, Seat: seat}
		}
		m, err := getMessage(q, id)
		if err != nil {
			return err
		}
		if m == nil {
			return &NoSuchMessageError{ID: id, Seat: seat}
		}
		if m.ReadGeneration == 0 && m.ReadEpoch == 0 {
			m.ReadGeneration, m.ReadEpoch = generation, epoch
		}
		if outcome != "" {
			m.Outcome = outcome
		}
		promote(m, state, atUnixNano)
		if err := putMessage(q, *m); err != nil {
			return err
		}
		out = *m
		return nil
	})
	if err != nil {
		return Message{}, err
	}
	return out, nil
}

// Messages is every message the estate still holds, oldest first.
//
// A READ-ONLY VIEW THAT PROMOTES NOTHING, which is what makes it safe to put
// on a human's prompt. It is `lease.list`'s shape and it exists for the same
// two readers: a person supervising the estate, and a SENDER, who has no other
// way to learn that what it sent was acted on. Section 16 makes acted-on "the
// only state a sender may plan against", and a state a sender cannot observe
// is not one it can plan against.
func (s *Store) Messages() ([]Message, error) {
	if s == nil || s.db == nil {
		return nil, ErrClosed
	}
	var out []Message
	err := s.db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket(bucketMessages)
		return root.ForEachBucket(func(seat []byte) error {
			q := root.Bucket(seat)
			return q.ForEach(func(_, v []byte) error {
				var m Message
				if err := json.Unmarshal(v, &m); err != nil {
					return fmt.Errorf("coord: decoding a message for seat %q: %w", seat, err)
				}
				out = append(out, m)
				return nil
			})
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Highest is the last id this estate has issued, and it is what a reader that
// wants only NEW mail starts its cursor at.
//
// IT IS THE BUCKET'S SEQUENCE AND NOT THE LAST KEY, so it does not fall back
// when the newest message is trimmed. A cursor that moved backwards would
// re-deliver mail a seat had already read.
func (s *Store) Highest() (uint64, error) {
	if s == nil || s.db == nil {
		return 0, ErrClosed
	}
	var out uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		out = tx.Bucket(bucketMessages).Sequence()
		return nil
	})
	return out, err
}

// promote moves a message forward through the five states and never back.
// It reports whether anything changed, so a caller can skip a write.
func promote(m *Message, to MessageState, atUnixNano int64) bool {
	if rank[to] <= rank[m.State] {
		return false
	}
	m.State = to
	m.MovedUnixNano = atUnixNano
	return true
}

// trim enforces MaxPerSeat and RECORDS WHAT IT TRIMMED THROUGH.
//
// ⛔ THE RECORD IS THE HALF THAT MATTERS. Dropping the oldest message is
// ordinary; dropping it without leaving a line that says so is how a reader
// gets a short batch it believes is complete. The line is what `gap` is
// computed from, and the two must never be separated.
// ⛔ THE COUNT IS WALKED AND NOT TAKEN FROM Stats().KeyN, WHICH IS WRONG HERE.
// Stats walks the pages, and a key Put earlier in this same writable
// transaction is still in the node cache rather than on a page - so it counts
// one short, and the queue settles one over the cap forever. Measured: 513
// messages against a cap of 512.
func trim(tx *bolt.Tx, q *bolt.Bucket, seat string) error {
	held := 0
	if err := q.ForEach(func(_, _ []byte) error { held++; return nil }); err != nil {
		return fmt.Errorf("coord: counting the queue for seat %q: %w", seat, err)
	}
	over := held - MaxPerSeat
	if over <= 0 {
		return nil
	}
	cur := q.Cursor()
	var through uint64
	for k, _ := cur.First(); k != nil && over > 0; k, _ = cur.Next() {
		through = binary.BigEndian.Uint64(k)
		if err := cur.Delete(); err != nil {
			return fmt.Errorf("coord: trimming the queue for seat %q: %w", seat, err)
		}
		over--
	}
	return tx.Bucket(bucketMsgMeta).Put([]byte(seat), u64(through))
}

// trimmedThrough is the highest id dropped from this seat's queue by
// retention. Zero means nothing has ever been dropped.
func trimmedThrough(tx *bolt.Tx, seat string) uint64 {
	raw := tx.Bucket(bucketMsgMeta).Get([]byte(seat))
	if raw == nil {
		return 0
	}
	return binary.BigEndian.Uint64(raw)
}

func putMessage(q *bolt.Bucket, m Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("coord: encoding message %d: %w", m.ID, err)
	}
	return q.Put(u64(m.ID), raw)
}

func getMessage(q *bolt.Bucket, id uint64) (*Message, error) {
	raw := q.Get(u64(id))
	if raw == nil {
		return nil, nil
	}
	var m Message
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("coord: decoding message %d: %w", id, err)
	}
	return &m, nil
}
