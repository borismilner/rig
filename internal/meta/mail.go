package meta

import (
	"context"

	"github.com/borismilner/rig/internal/kernel"
)

// Mailbox is section 16's directed messages, reachable from the agent surface.
//
// OPTIONAL AND RESOLVED PER CALL, Roster's shape. An unnamed estate keeps no
// message store, and a surface with no daemon under it has no roster to
// address, so both answer UNAVAILABLE rather than refusing.
//
// NO METHOD TAKES A PRINCIPAL OR A SEAT FOR THE CALLER, for Roster's reason:
// the implementation is per-connection, so the sender and the inbox's owner are
// the connection's row. A seat argument would let any caller read any seat's
// mail. The design is in plan/16 and the p1 seat notes.
type Mailbox interface {
	SendMail(ctx context.Context, in MailRequest) (MailSent, error)

	// ReadMail is message_inbox with wait zero and message_await otherwise.
	ReadMail(ctx context.Context, after uint64, limit int, waitMs uint32, await bool) (MailBatch, error)

	AckMail(ctx context.Context, id uint64, state, outcome string) (Mail, error)

	// ListMail promotes nothing and needs no seat: it is how a SENDER learns
	// what became of what it sent.
	ListMail(ctx context.Context, seat string) ([]Mail, error)
}

// MailRequest is the message tools' arguments.
//
// ⛔ ITS OWN STRUCT RATHER THAN MORE FIELDS ON Request. `To`, `State` and
// `Body` already mean an edge's far end, a work item's state and a record's
// body there, and one field meaning two things is how a reader learns the
// wrong model of a verb.
type MailRequest struct {
	To string `json:"to,omitempty"`

	// ToGeneration and ToEpoch pin one tenancy of the seat. Both or neither:
	// the generation restarts at 1 on every daemon start.
	ToGeneration uint64 `json:"toGeneration,omitempty"`
	ToEpoch      uint64 `json:"toEpoch,omitempty"`

	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`

	After  uint64 `json:"after,omitempty"`
	WaitMs uint32 `json:"waitMs,omitempty"`

	// ID, State and Outcome are message_ack's.
	ID      uint64 `json:"id,omitempty"`
	State   string `json:"state,omitempty"`
	Outcome string `json:"outcome,omitempty"`

	// Seat filters message_list. It never names the caller.
	Seat string `json:"seat,omitempty"`
}

// Mail is one directed message as the agent surface reads it.
//
// State is the store's own word (queued, delivered, read, acknowledged,
// acted-on). Misaddressed is the after-the-fact record: this message was read
// by a tenancy other than the one it was pinned to.
type Mail struct {
	ID               uint64 `json:"id"`
	To               string `json:"to"`
	ToGeneration     uint64 `json:"toGeneration"`
	ToEpoch          uint64 `json:"toEpoch"`
	From             string `json:"from"`
	FromGeneration   uint64 `json:"fromGeneration"`
	FromEpoch        uint64 `json:"fromEpoch"`
	Subject          string `json:"subject"`
	Body             string `json:"body"`
	State            string `json:"state"`
	HeldForSuccessor bool   `json:"heldForSuccessor"`
	ReadGeneration   uint64 `json:"readGeneration"`
	ReadEpoch        uint64 `json:"readEpoch"`
	Outcome          string `json:"outcome"`
	SentUnixNano     int64  `json:"sentUnixNano"`
	MovedUnixNano    int64  `json:"movedUnixNano"`
	Misaddressed     bool   `json:"misaddressed"`
}

// MailSent is what message_send answers.
type MailSent struct {
	Message Mail `json:"message"`

	// ToState is the recipient seat's state at the send: active, or
	// handing_off, in which case the message is HELD for the successor.
	ToState string `json:"toState"`

	// Delivered is how many parked readers took it. Zero is not a loss.
	Delivered int `json:"delivered"`
}

// MailBatch is what message_inbox and message_await answer.
type MailBatch struct {
	Messages []Mail `json:"messages"`
	Cursor   uint64 `json:"cursor"`
	Gap      bool   `json:"gap"`
	TimedOut bool   `json:"timedOut"`

	// Misaddressed names the ids IN THIS BATCH that were pinned to a different
	// tenancy of your seat than yours. ⛔ THIS IS THE FEATURE: the sender
	// believed it was talking to someone else, so read those with that in mind.
	Misaddressed []uint64 `json:"misaddressed"`
}

// MailAnswer is the five tools' payload, one pointer on Answer for Record's
// reason: one type, so no tool can quietly stop carrying Partial.
type MailAnswer struct {
	Sent   *MailSent  `json:"sent,omitempty"`
	Batch  *MailBatch `json:"batch,omitempty"`
	Acked  *Mail      `json:"acked,omitempty"`
	Listed *MailList  `json:"listed,omitempty"`
}

// MailList is message_list's answer. A struct so an empty list still renders
// as `"messages": []`, which is an answer, rather than vanishing.
type MailList struct {
	Messages []Mail `json:"messages"`
}

// unavailableMail is what the five tools report with no mailbox under them.
const unavailableMail = "this estate's directed messages"

func (s *Server) mailbox() (Mailbox, bool) {
	mb, ok := s.invoker.(Mailbox)
	return mb, ok
}

// mail answers the five message tools.
func (s *Server) mail(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	out, err := s.rosterAnswer(who, r.Tool)
	if err != nil {
		return Answer{}, err
	}
	mb, ok := s.mailbox()
	if !ok {
		out.Unavailable = append(out.Unavailable, unavailableMail)
		return out, nil
	}
	in := r.Mail
	ans := &MailAnswer{}
	switch r.Tool {
	case MessageSendTool:
		sent, err := mb.SendMail(ctx, in)
		if err != nil {
			return Answer{}, err
		}
		ans.Sent = &sent
	case MessageInboxTool, MessageAwaitTool:
		b, err := mb.ReadMail(ctx, in.After, r.Limit, in.WaitMs, r.Tool == MessageAwaitTool)
		if err != nil {
			return Answer{}, err
		}
		if b.Messages == nil {
			b.Messages = []Mail{}
		}
		if b.Misaddressed == nil {
			b.Misaddressed = []uint64{}
		}
		ans.Batch = &b
	case MessageAckTool:
		m, err := mb.AckMail(ctx, in.ID, in.State, in.Outcome)
		if err != nil {
			return Answer{}, err
		}
		ans.Acked = &m
	default:
		all, err := mb.ListMail(ctx, in.Seat)
		if err != nil {
			return Answer{}, err
		}
		if all == nil {
			all = []Mail{}
		}
		ans.Listed = &MailList{Messages: all}
	}
	out.Mail = ans
	return out, nil
}
