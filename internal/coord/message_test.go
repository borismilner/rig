package coord

import (
	"errors"
	"strings"
	"testing"
)

// newMessageStore is the fixture every test here shares: a fake boot, a fake
// clock and a store in a temporary estate. The clock is faked even though
// messages carry WALL time, because Open reads the boot id and the leases in
// the same store read CLOCK_BOOTTIME.
func newMessageStore(t *testing.T) *Store {
	t.Helper()
	fakeBoot(t)
	fakeClock(t)
	return openStore(t, estate(t, "msgtest"))
}

func send(t *testing.T, s *Store, m Message, at int64) Message {
	t.Helper()
	out, err := s.Send(m, at)
	if err != nil {
		t.Fatalf("sending to %q: %v", m.To, err)
	}
	return out
}

func TestAMessageIsQueuedAgainstTheSeatAndCarriesItsAddress(t *testing.T) {
	s := newMessageStore(t)

	got := send(t, s, Message{
		To: "backend-1", ToGeneration: 3, ToEpoch: 7,
		From: "lead", FromGeneration: 1, FromEpoch: 7,
		Subject: "take the refusal path", Body: "plan/16 paragraph 3",
	}, 1000)

	if got.ID != 1 {
		t.Fatalf("first message id = %d, want 1", got.ID)
	}
	if got.State != Queued {
		t.Fatalf("state = %q, want %q: nothing was parked, so it is queued", got.State, Queued)
	}
	if got.ToGeneration != 3 || got.ToEpoch != 7 {
		t.Fatalf("addressed to generation %d epoch %d, want 3 and 7",
			got.ToGeneration, got.ToEpoch)
	}
	if got.SentUnixNano != 1000 || got.MovedUnixNano != 1000 {
		t.Fatalf("timestamps = %d and %d, want both 1000", got.SentUnixNano, got.MovedUnixNano)
	}
}

// A seat reads its own queue and nobody else's. The addressing model is the
// feature: mail belongs to the seat it was directed at.
func TestInboxReturnsOnlyTheSeatsOwnMail(t *testing.T) {
	s := newMessageStore(t)
	send(t, s, Message{To: "backend-1", From: "lead", Body: "one"}, 1)
	send(t, s, Message{To: "backend-2", From: "lead", Body: "two"}, 2)
	send(t, s, Message{To: "backend-1", From: "lead", Body: "three"}, 3)

	b, err := s.Inbox("backend-1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(b.Messages))
	}
	if b.Messages[0].Body != "one" || b.Messages[1].Body != "three" {
		t.Fatalf("got %q and %q, want one and three",
			b.Messages[0].Body, b.Messages[1].Body)
	}
	if b.Cursor != 3 {
		t.Fatalf("cursor = %d, want 3", b.Cursor)
	}
	if b.Gap {
		t.Fatal("gap is true on a queue nothing has been trimmed from")
	}
}

// EVERYTHING SINCE THE CURSOR ARRIVES IN ONE BATCH. Three messages that landed
// while a seat was busy are one wake-up, not three missed ones.
func TestACursorResumesWhereItStopped(t *testing.T) {
	s := newMessageStore(t)
	first := send(t, s, Message{To: "seat", From: "lead", Body: "one"}, 1)
	send(t, s, Message{To: "seat", From: "lead", Body: "two"}, 2)
	send(t, s, Message{To: "seat", From: "lead", Body: "three"}, 3)

	b, err := s.Inbox("seat", first.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Messages) != 2 {
		t.Fatalf("after the cursor got %d messages, want 2", len(b.Messages))
	}
	if b.Messages[0].Body != "two" {
		t.Fatalf("resumed at %q, want two", b.Messages[0].Body)
	}
}

func TestABoundedBatchLeavesTheCursorWhereItStopped(t *testing.T) {
	s := newMessageStore(t)
	for i := range 5 {
		send(t, s, Message{To: "seat", From: "lead", Body: "x"}, int64(i))
	}
	b, err := s.Inbox("seat", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Messages) != 2 || b.Cursor != 2 {
		t.Fatalf("got %d messages and cursor %d, want 2 and 2", len(b.Messages), b.Cursor)
	}
}

// ⛔ THE DEFECT THE WHOLE FEATURE EXISTS FOR. A message pinned to one tenancy
// and read by another is MISADDRESSED, and an unpinned one never is.
func TestAPinnedMessageIsMisaddressedToADifferentTenancy(t *testing.T) {
	pinned := Message{ToGeneration: 3, ToEpoch: 7}
	if !pinned.MisaddressedTo(4, 7) {
		t.Fatal("generation 3 read by generation 4 is not reported as misaddressed")
	}
	if !pinned.MisaddressedTo(3, 8) {
		t.Fatal("generation 3 of epoch 7 read under epoch 8 is not reported as " +
			"misaddressed: the counter restarts at 1 on every daemon start, so " +
			"the epoch is half the identity")
	}
	if pinned.MisaddressedTo(3, 7) {
		t.Fatal("the tenancy it was addressed to is reported as misaddressed")
	}
	if (Message{}).MisaddressedTo(9, 9) {
		t.Fatal("an unpinned message is reported as misaddressed: the sender " +
			"addressed the SEAT, so any occupant is the right recipient")
	}
}

func TestReadingRecordsTheTenancyThatActuallyReadIt(t *testing.T) {
	s := newMessageStore(t)
	m := send(t, s, Message{To: "seat", ToGeneration: 1, ToEpoch: 2, From: "lead"}, 1)

	if err := s.MarkRead("seat", []uint64{m.ID}, 2, 2, 50); err != nil {
		t.Fatal(err)
	}
	b, err := s.Inbox("seat", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := b.Messages[0]
	if got.State != Read {
		t.Fatalf("state = %q, want %q", got.State, Read)
	}
	if got.ReadGeneration != 2 || got.ReadEpoch != 2 {
		t.Fatalf("read by generation %d epoch %d, want 2 and 2",
			got.ReadGeneration, got.ReadEpoch)
	}
	if !got.MisaddressedTo(2, 2) {
		t.Fatal("a message addressed to generation 1 and read by generation 2 " +
			"is not flagged: this is the defect Boris raised himself")
	}
}

// A second read by a SUCCESSOR must not erase which tenancy the message
// actually reached first.
func TestTheFirstReaderIsRecordedAndNotOverwritten(t *testing.T) {
	s := newMessageStore(t)
	m := send(t, s, Message{To: "seat", From: "lead"}, 1)

	if err := s.MarkRead("seat", []uint64{m.ID}, 1, 5, 10); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead("seat", []uint64{m.ID}, 2, 5, 20); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Inbox("seat", 0, 0)
	if b.Messages[0].ReadGeneration != 1 {
		t.Fatalf("read generation = %d, want 1: the first reader is the fact",
			b.Messages[0].ReadGeneration)
	}
}

// NOTHING PROMOTES A MESSAGE ON THE RECIPIENT'S BEHALF, and nothing moves one
// backwards either.
func TestPromotionNeverGoesBackwards(t *testing.T) {
	s := newMessageStore(t)
	m := send(t, s, Message{To: "seat", From: "lead"}, 1)

	if _, err := s.Ack("seat", m.ID, ActedOn, "rebased and pushed", 1, 1, 30); err != nil {
		t.Fatal(err)
	}
	// A recipient re-reading its inbox must keep the stronger fact.
	if err := s.MarkRead("seat", []uint64{m.ID}, 1, 1, 40); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDelivered("seat", []uint64{m.ID}, 50); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Inbox("seat", 0, 0)
	if b.Messages[0].State != ActedOn {
		t.Fatalf("state = %q, want %q", b.Messages[0].State, ActedOn)
	}
	if b.Messages[0].Outcome != "rebased and pushed" {
		t.Fatalf("outcome = %q, want the one the recipient stated", b.Messages[0].Outcome)
	}
}

func TestOnlyTheRecipientsOwnTwoStatesMayBeAcked(t *testing.T) {
	s := newMessageStore(t)
	m := send(t, s, Message{To: "seat", From: "lead"}, 1)

	for _, state := range []MessageState{Queued, Delivered, Read} {
		if _, err := s.Ack("seat", m.ID, state, "", 1, 1, 2); err == nil {
			t.Fatalf("promoting to %q through Ack was accepted: the daemon owns "+
				"queued and delivered, and read is what a read does", state)
		}
	}
}

func TestAckingAnIDFromSomebodyElsesQueueIsRefusedByName(t *testing.T) {
	s := newMessageStore(t)
	m := send(t, s, Message{To: "backend-1", From: "lead"}, 1)

	_, err := s.Ack("backend-2", m.ID, Acknowledged, "", 1, 1, 2)
	var missing *NoSuchMessageError
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want a NoSuchMessageError", err)
	}
	if missing.Seat != "backend-2" {
		t.Fatalf("the refusal names seat %q, want backend-2: naming the message "+
			"alone sends the caller looking for a deleted one", missing.Seat)
	}
}

// A MESSAGE OVER THE BOUND IS REFUSED RATHER THAN TRUNCATED. A recipient acts
// on what arrived and cannot see the half that did not.
func TestAnOversizeMessageIsRefusedAndNotTruncated(t *testing.T) {
	s := newMessageStore(t)

	_, err := s.Send(Message{To: "seat", From: "lead", Body: strings.Repeat("x", MaxBody+1)}, 1)
	var over *OversizeError
	if !errors.As(err, &over) {
		t.Fatalf("err = %v, want an OversizeError", err)
	}
	if over.What != "body" {
		t.Fatalf("the refusal names %q, want body", over.What)
	}
	if b, _ := s.Inbox("seat", 0, 0); len(b.Messages) != 0 {
		t.Fatal("the refused message was stored anyway")
	}

	_, err = s.Send(Message{To: "seat", From: "lead", Subject: strings.Repeat("x", MaxSubject+1)}, 1)
	if !errors.As(err, &over) || over.What != "subject" {
		t.Fatalf("an oversize subject gave %v, want an OversizeError naming the subject", err)
	}
}

// ⛔ RETENTION AND `gap` SHIP TOGETHER. A cursor older than retention gets a
// batch that CANNOT be complete, and it is told so rather than handed a short
// batch that looks whole.
func TestACursorOlderThanRetentionReportsAGap(t *testing.T) {
	s := newMessageStore(t)
	for i := range MaxPerSeat + 5 {
		send(t, s, Message{To: "seat", From: "lead", Body: "x"}, int64(i))
	}

	b, err := s.Inbox("seat", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Gap {
		t.Fatal("a cursor of 0 against a trimmed queue did not report a gap")
	}
	if len(b.Messages) != MaxPerSeat {
		t.Fatalf("queue holds %d messages, want the cap of %d", len(b.Messages), MaxPerSeat)
	}

	// A cursor inside what is still held is NOT a gap.
	fresh, err := s.Inbox("seat", uint64(MaxPerSeat+4), 0)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Gap {
		t.Fatal("a current cursor reported a gap")
	}
}

// Retention is per seat, so a chatty pair cannot evict a quiet seat's mail.
func TestOneSeatsRetentionDoesNotTouchAnother(t *testing.T) {
	s := newMessageStore(t)
	send(t, s, Message{To: "quiet", From: "lead", Body: "keep me"}, 1)
	for i := range MaxPerSeat + 5 {
		send(t, s, Message{To: "chatty", From: "lead", Body: "x"}, int64(i))
	}

	b, err := s.Inbox("quiet", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Messages) != 1 || b.Messages[0].Body != "keep me" {
		t.Fatalf("the quiet seat lost its mail to another seat's traffic: %+v", b.Messages)
	}
	if b.Gap {
		t.Fatal("the quiet seat was told it had a gap because another seat was trimmed")
	}
}

// Section 16 paragraph 2: a session's death does not destroy a queued message
// and a successor claiming that seat receives it. Closing and reopening the
// store is the strongest form of that claim this layer can make.
func TestAQueuedMessageSurvivesTheStoreClosing(t *testing.T) {
	fakeBoot(t)
	fakeClock(t)
	name := estate(t, "survive")

	first, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	sent := send(t, first, Message{
		To: "seat", ToGeneration: 1, ToEpoch: first.Epoch(),
		From: "lead", Body: "brief the successor",
	}, 1)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })

	b, err := second.Inbox("seat", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Messages) != 1 || b.Messages[0].Body != "brief the successor" {
		t.Fatalf("the message did not survive the restart: %+v", b.Messages)
	}
	if !b.Messages[0].MisaddressedTo(1, second.Epoch()) {
		t.Fatal("a generation pinned under the previous epoch was not flagged " +
			"against this one: the counter restarts at 1 on every start")
	}

	// THE CURSOR MUST NOT RESTART. An id reissued after a restart would make
	// a reader's stored cursor skip mail it has never seen.
	next := send(t, second, Message{To: "seat", From: "lead", Body: "after"}, 2)
	if next.ID <= sent.ID {
		t.Fatalf("id after the restart = %d, want more than %d", next.ID, sent.ID)
	}
}

func TestMessagesIsEveryQueueInOrderAndPromotesNothing(t *testing.T) {
	s := newMessageStore(t)
	send(t, s, Message{To: "b", From: "lead", Body: "one"}, 1)
	send(t, s, Message{To: "a", From: "lead", Body: "two"}, 2)
	send(t, s, Message{To: "b", From: "lead", Body: "three"}, 3)

	all, err := s.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d messages, want 3", len(all))
	}
	for i, want := range []string{"one", "two", "three"} {
		if all[i].Body != want {
			t.Fatalf("row %d is %q, want %q: the view is estate-wide and in id order",
				i, all[i].Body, want)
		}
		if all[i].State != Queued {
			t.Fatalf("row %d is %q: the read-only view promoted a message",
				i, all[i].State)
		}
	}
}

func TestHighestIsWhereAReaderWantingOnlyNewMailStarts(t *testing.T) {
	s := newMessageStore(t)
	if h, err := s.Highest(); err != nil || h != 0 {
		t.Fatalf("highest on an empty estate = %d, %v, want 0", h, err)
	}
	send(t, s, Message{To: "seat", From: "lead"}, 1)
	send(t, s, Message{To: "other", From: "lead"}, 2)
	h, err := s.Highest()
	if err != nil {
		t.Fatal(err)
	}
	if h != 2 {
		t.Fatalf("highest = %d, want 2: it is estate-wide, not per seat", h)
	}
}

func TestAMessageNeedsBothEnds(t *testing.T) {
	s := newMessageStore(t)
	if _, err := s.Send(Message{From: "lead"}, 1); err == nil {
		t.Fatal("a message with no recipient seat was accepted")
	}
	if _, err := s.Send(Message{To: "seat"}, 1); err == nil {
		t.Fatal("a message with no sender was accepted")
	}
}

func TestAClosedStoreRefusesEveryMessageCall(t *testing.T) {
	fakeBoot(t)
	fakeClock(t)
	s, err := Open(estate(t, "closed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(Message{To: "a", From: "b"}, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("Send on a closed store = %v, want ErrClosed", err)
	}
	if _, err := s.Inbox("a", 0, 0); !errors.Is(err, ErrClosed) {
		t.Fatalf("Inbox on a closed store = %v, want ErrClosed", err)
	}
	if err := s.MarkRead("a", []uint64{1}, 1, 1, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("MarkRead on a closed store = %v, want ErrClosed", err)
	}
	if err := s.MarkDelivered("a", []uint64{1}, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("MarkDelivered on a closed store = %v, want ErrClosed", err)
	}
	if _, err := s.Ack("a", 1, Acknowledged, "", 1, 1, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("Ack on a closed store = %v, want ErrClosed", err)
	}
	if _, err := s.Messages(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Messages on a closed store = %v, want ErrClosed", err)
	}
	if _, err := s.Highest(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Highest on a closed store = %v, want ErrClosed", err)
	}
}
