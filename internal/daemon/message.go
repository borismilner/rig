package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/kernel"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 16's directed messages, served. internal/coord/message.go is the
// whole storage mechanism - the five states, the cursor, retention and the gap
// - and this file decides the three things only the ROSTER can answer, exactly
// as lease.go decides the holder and the witness:
//
//   - WHO IS SENDING. The sender's seat, generation and epoch come off the
//     connection's own roster row and are never read off the request. A sender
//     that could name itself could impersonate the peer a recipient trusts.
//   - WHETHER THE ADDRESS EXISTS. A seat nobody holds, a name nobody has ever
//     used and a seat in the middle of a handover are three different answers,
//     and a sender given one sentence for all three learns nothing.
//   - WHETHER THE PIN STILL MATCHES. ⛔ THIS IS THE FEATURE. Boris,
//     2026-09-11: "peers should be aware they may be contacted wrongly
//     thinking they are the successor". A message pinned to a tenancy that has
//     since ended is refused at send, and one whose seat turns over between
//     the send and the read is FLAGGED at the read - to the reader, and to the
//     sender through rig.message.list. Never silently delivered.
//
// ⛔ THE VERBS ARE VALUES FIRST AND FRAMES SECOND, and that is why this file is
// arranged the way it is. Two doors serve these verbs - the wire, and the MCP
// tools an agent sees - and the refusals ARE the feature. A second copy of "is
// this pin still current" behind the agent door would be a second place for
// the guard to weaken, and nothing would fail. So sendMail, readMail, ackMail
// and listMail return values and refusals; the frame handlers below and
// mcp_message.go both call them, and neither decides anything.

const (
	// maxMessageWait bounds a parked rig.message.await, for the reason
	// maxToastWait bounds a parked toast.wait: a call that can park forever is
	// a connection rig cannot account for.
	maxMessageWait = 60 * time.Second

	// defaultMessageWait is what a caller that named no wait gets. A zero here
	// would turn await into inbox silently, which is the one thing the two
	// verbs exist to keep apart.
	defaultMessageWait = 30 * time.Second
)

// mailbell wakes rig.message.await calls parked on a seat, and counts how many
// are parked so a sender can be told whether anybody was listening.
//
// ⛔ PRESENCE CANNOT DO THIS AND MUST NOT LEARN TO. The roster is deliberately
// ignorant of everything durable (its own comment says so at length), and this
// queue is durable. So the wake-up is a separate, purely in-memory thing whose
// loss costs nothing: a daemon restarted with its parked readers gone leaves
// every message where it was, and the successor reads from its cursor.
//
// Its zero value is ready to use, as toastRing's is.
type mailbell struct {
	mu     sync.Mutex
	wake   map[string]chan struct{}
	parked map[string]int
}

// waiter is a channel that closes when the next message for this seat lands.
//
// TAKEN BEFORE THE QUEUE IS READ, which is what closes the race: a message
// arriving between the read and the park closes this channel, so the park
// returns at once instead of sleeping through the thing it waited for.
func (b *mailbell) waiter(seat string) <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.wake == nil {
		b.wake = make(map[string]chan struct{})
	}
	ch, ok := b.wake[seat]
	if !ok {
		ch = make(chan struct{})
		b.wake[seat] = ch
	}
	return ch
}

// park and unpark count the readers waiting on a seat right now.
func (b *mailbell) park(seat string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.parked == nil {
		b.parked = make(map[string]int)
	}
	b.parked[seat]++
}

func (b *mailbell) unpark(seat string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.parked[seat] <= 1 {
		delete(b.parked, seat)
		return
	}
	b.parked[seat]--
}

// ring wakes everybody parked on a seat and reports how many that was.
//
// The channel is closed and DROPPED rather than reused: closing one twice
// panics, and a closed channel left in the map never parks anybody again.
func (b *mailbell) ring(seat string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.wake[seat]; ok {
		close(ch)
		delete(b.wake, seat)
	}
	return b.parked[seat]
}

// mailRefusalError is a refusal that carries its own wire code.
//
// It exists because the message core is shared by two doors: the wire needs a
// rigv1.Code and the MCP door needs a kernel.RefusalError, and re-deriving the
// code at each door from the sentence would be two places to get it wrong.
type mailRefusalError struct {
	*kernel.RefusalError
	code rigv1.Code
}

// Unwrap exposes the RefusalError itself. Without it the embedded one's Unwrap
// is promoted and skips straight to the inner sentence, so kernel.AsRefusal
// finds nothing and the MCP door loses precondition, actual and fix.
func (r *mailRefusalError) Unwrap() error { return r.RefusalError }

func refuseMail(code rigv1.Code, msg, precondition, actual, fix string) error {
	return &mailRefusalError{
		RefusalError: &kernel.RefusalError{
			Err: errors.New(msg), Precondition: precondition, Actual: actual, Fix: fix,
		},
		code: code,
	}
}

// mailSend is one directed message as a caller asks for it, at either door.
type mailSend struct {
	To           string
	ToGeneration uint64
	ToEpoch      uint64
	Subject      string
	Body         string
}

// mailSent is what a send answers with.
type mailSent struct {
	Message coord.Message
	ToState verbsv1.SeatState
	// Delivered is how many parked readers this was handed to. ZERO IS NOT A
	// LOSS: the queue is durable and the seat reads it on its next inbox.
	Delivered int
}

// mailBatch is what a read answers with, at either door.
type mailBatch struct {
	Messages []coord.Message
	Cursor   uint64
	Gap      bool
	// Misaddressed names the ids IN THIS BATCH addressed to a different
	// tenancy of the reader's seat than the reader's own.
	Misaddressed []uint64
	TimedOut     bool
}

// mailStore is the estate's store, or the refusal that names why there is none.
func (d *Daemon) mailStore(command string) (*coord.Store, error) {
	if d.leases != nil {
		return d.leases, nil
	}
	return nil, refuseMail(rigv1.Code_CODE_UNAVAILABLE,
		"rig."+command+": this estate has no message store: an unnamed estate "+
			"keeps no persistent state, and a queued message that did not survive "+
			"its recipient's death could not reach a successor, which is the whole "+
			"of what a directed message is for",
		"rigd was started with --estate",
		"this daemon serves an unnamed estate",
		"start rigd with --estate <name>")
}

// sendMail queues a message for a seat, or refuses and says why.
func (d *Daemon) sendMail(from string, fromGen, fromEpoch uint64, in mailSend) (mailSent, error) {
	st, err := d.mailStore("message.send")
	if err != nil {
		return mailSent{}, err
	}
	if from == "" {
		return mailSent{}, refuseUnseatedMail("message.send",
			"the message could not say who sent it, and a recipient that cannot "+
				"tell who is asking cannot act on it")
	}
	if len(in.To) > maxLeaseText {
		return mailSent{}, refuseMail(rigv1.Code_CODE_INVALID, fmt.Sprintf(
			"rig.message.send: the recipient seat is %d bytes, over the %d-byte bound",
			len(in.To), maxLeaseText),
			fmt.Sprintf("a seat name is at most %d bytes", maxLeaseText),
			fmt.Sprintf("this one is %d", len(in.To)),
			"send to the seat name `rig peers` shows, which is an identifier "+
				"rather than a sentence")
	}
	state, err := d.addressee(in)
	if err != nil {
		return mailSent{}, err
	}
	// ⛔ WALL CLOCK, AND THIS IS THE ONE THING IN THE PACKAGE THAT WANTS IT.
	// Every deadline rig makes is on CLOCK_BOOTTIME (coord.Now) so a suspended
	// laptop cannot expire a lease. Nothing here expires: these two stamps are
	// read by a HUMAN beside everything else on their screen, so they have to
	// be on the clock their log lines are on.
	at := time.Now().UnixNano()

	// HANDING_OFF IS QUEUED AND HELD, NOT DELIVERED. Section 16 paragraph 3:
	// the state is published precisely so a sender is not answered by a
	// session that is closing. The message waits for the successor, and the
	// sender is told in the same answer rather than left to infer it.
	held := state == verbsv1.SeatState_SEAT_STATE_HANDING_OFF
	stored, err := st.Send(coord.Message{
		To: in.To, ToGeneration: in.ToGeneration, ToEpoch: in.ToEpoch,
		From: from, FromGeneration: fromGen, FromEpoch: fromEpoch,
		Subject: in.Subject, Body: in.Body, HeldForSuccessor: held,
	}, at)
	if err != nil {
		return mailSent{}, err
	}
	out := mailSent{Message: stored, ToState: state}
	if held {
		return out, nil
	}
	out.Delivered = d.mail.ring(in.To)
	if out.Delivered > 0 {
		// The daemon's own transition, recorded by the daemon, at the moment
		// it hands the message to an open subscription. A later read
		// supersedes it with READ, which is the recipient's to set.
		if err := st.MarkDelivered(in.To, []uint64{stored.ID}, at); err != nil {
			return mailSent{}, err
		}
		out.Message.State = coord.Delivered
	}
	return out, nil
}

// addressee resolves the recipient seat and checks the pin, or refuses.
//
// The refusals are deliberately different sentences, because they send a
// sender to different places: a vacant seat means wait or pick somebody else,
// an unknown name means look at what you typed, and a stale pin means the
// session you meant has gone.
func (d *Daemon) addressee(in mailSend) (verbsv1.SeatState, error) {
	if in.To == "" {
		return 0, refuseMail(rigv1.Code_CODE_INVALID,
			"rig.message.send: a message needs a recipient seat",
			"the request names the seat to deliver to",
			"no recipient seat was given",
			"name a seat - `rig peers` lists who is here")
	}
	// A PIN IS TWO NUMBERS OR IT IS NEITHER. The generation counter restarts
	// at 1 on every daemon start, so a generation without an epoch matches a
	// tenancy from a previous run and the guard silently does nothing.
	gen, epoch := in.ToGeneration, in.ToEpoch
	if (gen == 0) != (epoch == 0) {
		return 0, refuseMail(rigv1.Code_CODE_INVALID, fmt.Sprintf(
			"rig.message.send: a pin is a generation AND an epoch, got generation "+
				"%d and epoch %d", gen, epoch),
			"a pinned message carries both numbers, or neither",
			"exactly one of the two was given",
			"the generation counter restarts at 1 every time rigd starts, so one "+
				"number without the other would match a session from a previous "+
				"run. Send both, from `rig peers`, or neither, which addresses "+
				"whoever holds the seat")
	}
	occ, live := d.presence.seatNamed(in.To)
	if !live {
		return 0, d.refuseNoSuchAddressee(in.To)
	}
	if gen != 0 && (gen != occ.generation || epoch != occ.epoch) {
		return 0, refuseMail(rigv1.Code_CODE_CONFLICT, fmt.Sprintf(
			"rig.message.send: seat %q is generation %d of epoch %d, and this "+
				"message is addressed to generation %d of epoch %d. The session "+
				"you meant has ended and somebody else holds the seat now",
			in.To, occ.generation, occ.epoch, gen, epoch),
			"the pin names the tenancy actually in the seat",
			fmt.Sprintf("the seat turned over: it is on generation %d of epoch %d",
				occ.generation, occ.epoch),
			"re-read the seat with `rig peers` and send to the tenancy it names, "+
				"or send with no pin at all, which addresses whoever holds the seat "+
				"and is the right thing when the work belongs to the seat rather "+
				"than to one session")
	}
	return occ.state, nil
}

// refuseNoSuchAddressee separates a seat that is EMPTY from a name that is
// UNKNOWN.
//
// ⛔ A NEW SEND TO A SEAT NOBODY HOLDS IS REFUSED, AND MAIL ALREADY QUEUED
// STILL SURVIVES. Section 16 paragraph 2 promises a queued message outlives
// its recipient and reaches the successor; paragraph 3 refuses a send to a
// vacant seat. They only look contradictory: the promise is about mail already
// in the queue, and the refusal is about posting NEW mail into a void, where a
// sender would wait on an answer nothing is going to produce.
func (d *Daemon) refuseNoSuchAddressee(to string) error {
	live := d.presence.liveSeats()
	here := "nobody is in any seat on this estate"
	if len(live) > 0 {
		here = "here now: " + strings.Join(live, ", ")
	}
	if d.presence.knownSeat(to) {
		return refuseMail(rigv1.Code_CODE_NOT_FOUND, fmt.Sprintf(
			"rig.message.send: seat %q exists on this estate and nobody is in it, "+
				"so there is nothing to deliver to", to),
			"somebody holds the seat being written to",
			"the seat is empty - its last occupant has gone",
			"wait for a successor to announce into it and send then, or send to "+
				"somebody who is here ("+here+"). Mail ALREADY in that seat's queue "+
				"is not lost: a successor reads it on arrival")
	}
	return refuseMail(rigv1.Code_CODE_NOT_FOUND, fmt.Sprintf(
		"rig.message.send: no seat called %q has ever been taken on this estate, "+
			"so this is a name rather than an empty seat", to),
		"the recipient seat is one somebody has announced into",
		"that name is unknown to this daemon's roster",
		"check the spelling against `rig peers` - "+here)
}

// readMail answers rig.message.inbox and rig.message.await, which are one
// function because they differ only in whether they park. A zero wait is the
// inbox; anything else parks until there is mail or the wait passes.
//
// ⛔ THE SEAT IS THE CALLER'S AND IS NEVER NAMED BY THE CALLER. An inbox that
// took a seat name would let any connection read any seat's mail, and the
// whole identity model here rests on a seat being something you hold rather
// than something you claim.
func (d *Daemon) readMail(ctx context.Context, occ occupant, after uint64, limit int, wait time.Duration) (mailBatch, error) {
	command := verbMessageInbox
	if wait > 0 {
		command = "message.await"
	}
	st, err := d.mailStore(command)
	if err != nil {
		return mailBatch{}, err
	}
	if occ.seat == "" {
		return mailBatch{}, refuseUnseatedMail(command,
			"an inbox belongs to a seat and this connection is not in one. The "+
				"seat is read off the connection and is never named in the request, "+
				"so there is no mailbox to open")
	}
	if wait == 0 {
		batch, err := st.Inbox(occ.seat, after, limit)
		if err != nil {
			return mailBatch{}, err
		}
		return d.promoteRead(st, occ, batch)
	}
	d.mail.park(occ.seat)
	defer d.mail.unpark(occ.seat)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		wake := d.mail.waiter(occ.seat)
		batch, err := st.Inbox(occ.seat, after, limit)
		if err != nil {
			return mailBatch{}, err
		}
		if len(batch.Messages) > 0 {
			return d.promoteRead(st, occ, batch)
		}
		select {
		case <-wake:
		case <-timer.C:
			return mailBatch{Cursor: batch.Cursor, Gap: batch.Gap, TimedOut: true}, nil
		case <-ctx.Done():
			return mailBatch{}, ctx.Err()
		}
	}
}

// promoteRead moves a batch to READ and flags what was addressed to a
// different tenancy of this seat.
//
// ⛔ THE FLAG IS COMPUTED AGAINST THE READER, NOT AGAINST THE SEAT. The
// question is not "is this message old", it is "am I the session the sender
// believes it is talking to", and only the reader's own generation and epoch
// answer it.
func (d *Daemon) promoteRead(st *coord.Store, occ occupant, batch coord.Batch) (mailBatch, error) {
	out := mailBatch{Cursor: batch.Cursor, Gap: batch.Gap}
	ids := make([]uint64, 0, len(batch.Messages))
	for _, m := range batch.Messages {
		ids = append(ids, m.ID)
		if m.MisaddressedTo(occ.generation, occ.epoch) {
			out.Misaddressed = append(out.Misaddressed, m.ID)
		}
	}
	at := time.Now().UnixNano()
	if err := st.MarkRead(occ.seat, ids, occ.generation, occ.epoch, at); err != nil {
		return mailBatch{}, err
	}
	for _, m := range batch.Messages {
		// The store records the reading tenancy on the FIRST read only, so the
		// same rule is applied to the copy being answered with rather than the
		// row being read back: an answer must say what its own write did.
		if m.ReadGeneration == 0 && m.ReadEpoch == 0 {
			m.ReadGeneration, m.ReadEpoch = occ.generation, occ.epoch
		}
		if rankOf(m.State) < rankOf(coord.Read) {
			m.State, m.MovedUnixNano = coord.Read, at
		}
		out.Messages = append(out.Messages, m)
	}
	return out, nil
}

// ackMail is the recipient's own statement about one of its own messages.
func (d *Daemon) ackMail(occ occupant, id uint64, state coord.MessageState, outcome string) (coord.Message, error) {
	st, err := d.mailStore("message.ack")
	if err != nil {
		return coord.Message{}, err
	}
	if occ.seat == "" {
		return coord.Message{}, refuseUnseatedMail("message.ack",
			"an acknowledgement is a statement by the seat the message was sent "+
				"to, and this connection is not in a seat")
	}
	if state != coord.Acknowledged && state != coord.ActedOn {
		return coord.Message{}, refuseMail(rigv1.Code_CODE_INVALID, fmt.Sprintf(
			"rig.message.ack: a recipient may promote a message to %s or %s and to "+
				"nothing else, got %q", coord.Acknowledged, coord.ActedOn, state),
			"the new state is acknowledged or acted-on",
			"a state only the daemon or a read may set",
			"QUEUED and DELIVERED are the daemon's, and READ is what reading your "+
				"inbox already did. Say ACKNOWLEDGED when you understood it, or "+
				"ACTED_ON with an outcome when you did something about it")
	}
	return st.Ack(occ.seat, id, state, outcome, occ.generation, occ.epoch, time.Now().UnixNano())
}

// listMail is the read-only estate view, and it PROMOTES NOTHING.
//
// It needs no seat, and that is deliberate: a SENDER has no other way to learn
// that what it sent was acted on, and a sender is by definition not sitting in
// the recipient's seat. Section 16 makes acted-on the only state a sender may
// plan against, and a state a sender cannot observe is not one it can plan
// against.
func (d *Daemon) listMail(seat string) ([]coord.Message, error) {
	st, err := d.mailStore("message.list")
	if err != nil {
		return nil, err
	}
	all, err := st.Messages()
	if err != nil {
		return nil, err
	}
	if seat == "" {
		return all, nil
	}
	out := make([]coord.Message, 0, len(all))
	for _, m := range all {
		if m.To == seat {
			out = append(out, m)
		}
	}
	return out, nil
}

// ---- the wire door --------------------------------------------------------

// serveMessage dispatches the five message verbs through one arm of the
// daemon's switch, which gocyclo holds under its ceiling.
func (d *Daemon) serveMessage(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "message.send":
		d.serveMessageSend(c, f)
	case verbMessageInbox, "message.await":
		d.serveMessageRead(ctx, c, f, command)
	case "message.ack":
		d.serveMessageAck(c, f)
	case "message.list":
		d.serveMessageList(c, f)
	}
}

func (d *Daemon) serveMessageSend(c *conn, f *rigv1.Frame) {
	var req verbsv1.MessageSendRequest
	if !readFrame(c, f, "message.send", &req) {
		return
	}
	from, gen, epoch := d.sender(c)
	sent, err := d.sendMail(from, gen, epoch, mailSend{
		To: req.GetTo(), ToGeneration: req.GetToGeneration(), ToEpoch: req.GetToEpoch(),
		Subject: req.GetSubject(), Body: req.GetBody(),
	})
	if err != nil {
		c.failErr(f.GetStreamId(), messageCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.MessageSendResponse{
		Message: messageToWire(sent.Message),
		ToState: sent.ToState,
		//nolint:gosec // bounded by the number of live connections
		Delivered: uint32(sent.Delivered),
	})
}

func (d *Daemon) serveMessageRead(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	after, limit, wait, ok := readRequest(c, f, command)
	if !ok {
		return
	}
	occ, _ := d.presence.occupantOf(c.occ)
	batch, err := d.readMail(ctx, occ, after, int(limit), wait)
	if err != nil {
		// A cancelled context is this connection going away, not a refusal:
		// there is nobody left to answer.
		if errors.Is(err, context.Canceled) {
			return
		}
		c.failErr(f.GetStreamId(), messageCode(err), err)
		return
	}
	out := make([]*verbsv1.Message, 0, len(batch.Messages))
	for _, m := range batch.Messages {
		out = append(out, messageToWire(m))
	}
	if command == verbMessageInbox {
		c.reply(f.GetStreamId(), &verbsv1.MessageInboxResponse{
			Messages: out, Cursor: batch.Cursor, Gap: batch.Gap, Misaddressed: batch.Misaddressed,
		})
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.MessageAwaitResponse{
		Messages: out, Cursor: batch.Cursor, Gap: batch.Gap,
		Misaddressed: batch.Misaddressed, TimedOut: batch.TimedOut,
	})
}

func (d *Daemon) serveMessageAck(c *conn, f *rigv1.Frame) {
	var req verbsv1.MessageAckRequest
	if !readFrame(c, f, "message.ack", &req) {
		return
	}
	occ, _ := d.presence.occupantOf(c.occ)
	m, err := d.ackMail(occ, req.GetId(), ackState(req.GetState()), req.GetOutcome())
	if err != nil {
		c.failErr(f.GetStreamId(), messageCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.MessageAckResponse{Message: messageToWire(m)})
}

func (d *Daemon) serveMessageList(c *conn, f *rigv1.Frame) {
	var req verbsv1.MessageListRequest
	if !readFrame(c, f, "message.list", &req) {
		return
	}
	all, err := d.listMail(req.GetSeat())
	if err != nil {
		c.failErr(f.GetStreamId(), messageCode(err), err)
		return
	}
	resp := &verbsv1.MessageListResponse{}
	for _, m := range all {
		resp.Messages = append(resp.Messages, messageToWire(m))
	}
	c.reply(f.GetStreamId(), resp)
}

// readRequest unpacks whichever of the two read requests was sent, and turns
// await's milliseconds into the wait that tells the two apart.
func readRequest(c *conn, f *rigv1.Frame, command string) (after uint64, limit uint32, wait time.Duration, ok bool) {
	if command == verbMessageInbox {
		var req verbsv1.MessageInboxRequest
		if !readFrame(c, f, command, &req) {
			return 0, 0, 0, false
		}
		return req.GetAfter(), req.GetLimit(), 0, true
	}
	var req verbsv1.MessageAwaitRequest
	if !readFrame(c, f, command, &req) {
		return 0, 0, 0, false
	}
	return req.GetAfter(), req.GetLimit(), awaitWait(req.GetWaitMs()), true
}

// awaitWait turns a caller's milliseconds into a park, the same at both doors:
// zero is the default rather than no wait, and nothing parks past the bound.
func awaitWait(ms uint32) time.Duration {
	if ms == 0 {
		return defaultMessageWait
	}
	return min(time.Duration(ms)*time.Millisecond, maxMessageWait)
}

// readFrame unmarshals a request or fails the call, so five handlers do not
// each carry the same four lines.
func readFrame(c *conn, f *rigv1.Frame, command string, into proto.Message) bool {
	if err := proto.Unmarshal(f.GetPayload(), into); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, command+": "+err.Error())
		return false
	}
	return true
}

// sender is WHO IS SENDING, and all three fields are the connection's.
//
// An announced seat carries a real tenancy. A terminal does not: it is named
// by the daemon so `rig message send` works at a prompt, and its generation
// and epoch stay zero, which is not a tenancy and says so rather than
// borrowing the daemon's numbers and reading as one.
func (d *Daemon) sender(c *conn) (seat string, generation, epoch uint64) {
	if occ, found := d.presence.occupantOf(c.occ); found && occ.seat != "" {
		return occ.seat, occ.generation, occ.epoch
	}
	_, seat, _, _ = d.provenance(c)
	return seat, 0, 0
}

// refuseUnseatedMail is the message verbs' version of refuseUnseatedLease: the
// same shape, with the reason this particular verb needs a seat.
func refuseUnseatedMail(command, because string) error {
	return refuseMail(rigv1.Code_CODE_DENIED,
		"rig."+command+": this connection holds no seat, so "+because,
		"the caller announced into a named seat",
		"this connection has not announced, or announced without a seat",
		"call rig.announce on this connection first, with a seat. The seat is "+
			"the daemon's to name and is never read off the request")
}

// messageCode maps a message refusal onto the wire's codes.
func messageCode(err error) rigv1.Code {
	var refusal *mailRefusalError
	var oversize *coord.OversizeError
	var missing *coord.NoSuchMessageError
	switch {
	case errors.As(err, &refusal):
		return refusal.code
	case errors.As(err, &oversize):
		return rigv1.Code_CODE_INVALID
	case errors.As(err, &missing):
		return rigv1.Code_CODE_NOT_FOUND
	default:
		return leaseCode(err)
	}
}

var messageStateWire = map[coord.MessageState]verbsv1.MessageState{
	coord.Queued:       verbsv1.MessageState_MESSAGE_STATE_QUEUED,
	coord.Delivered:    verbsv1.MessageState_MESSAGE_STATE_DELIVERED,
	coord.Read:         verbsv1.MessageState_MESSAGE_STATE_READ,
	coord.Acknowledged: verbsv1.MessageState_MESSAGE_STATE_ACKNOWLEDGED,
	coord.ActedOn:      verbsv1.MessageState_MESSAGE_STATE_ACTED_ON,
}

// ackState maps the wire enum onto the store's states. Anything a recipient
// may not set comes back empty, and ackMail refuses it by name.
func ackState(s verbsv1.MessageState) coord.MessageState {
	switch s {
	case verbsv1.MessageState_MESSAGE_STATE_ACKNOWLEDGED:
		return coord.Acknowledged
	case verbsv1.MessageState_MESSAGE_STATE_ACTED_ON:
		return coord.ActedOn
	default:
		return ""
	}
}

// rankOf orders the five states for the one place outside coord that has to
// know a promotion never runs backwards: rendering the answer a read produces.
func rankOf(s coord.MessageState) int {
	switch s {
	case coord.Delivered:
		return 1
	case coord.Read:
		return 2
	case coord.Acknowledged:
		return 3
	case coord.ActedOn:
		return 4
	default:
		return 0
	}
}

// messageToWire renders one message.
//
// `misaddressed` here is the AFTER-THE-FACT record, computed from the two
// pairs the message already carries, so `rig message list` shows a human and a
// sender exactly which mail reached a tenancy it was not meant for. The live
// warning a reader gets is a different field on the read responses, because it
// is about the reader rather than about the row.
func messageToWire(m coord.Message) *verbsv1.Message {
	return &verbsv1.Message{
		Id: m.ID, To: m.To, ToGeneration: m.ToGeneration, ToEpoch: m.ToEpoch,
		From: m.From, FromGeneration: m.FromGeneration, FromEpoch: m.FromEpoch,
		Subject: m.Subject, Body: m.Body,
		State:            messageStateWire[m.State],
		HeldForSuccessor: m.HeldForSuccessor,
		ReadGeneration:   m.ReadGeneration, ReadEpoch: m.ReadEpoch,
		Outcome:      m.Outcome,
		SentUnixNano: m.SentUnixNano, MovedUnixNano: m.MovedUnixNano,
		Misaddressed: m.ReadGeneration != 0 && m.MisaddressedTo(m.ReadGeneration, m.ReadEpoch),
	}
}
