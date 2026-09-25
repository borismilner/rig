package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 16's directed messages over the real wire. internal/coord's tests
// prove the storage - the five states, the cursor, retention and the gap -
// and these prove the three things only the roster can decide: who the sender
// is, whether the address exists, and whether the pin still matches.

// TestAMessageReachesASeatAndCarriesItsSender is the ordinary path: one seat
// writes to another by NAME, the recipient reads its own mail, and the sender
// learns from rig.message.list that it was acted on.
func TestAMessageReachesASeatAndCarriesItsSender(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)
	lead := seated(t, sock, "team-lead")
	worker := seated(t, sock, "backend-1")

	var sent verbsv1.MessageSendResponse
	if err := lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
		To: "backend-1", Subject: "the proto is regenerated", Body: "pull before you build",
	}, &sent); err != nil {
		t.Fatalf("rig.message.send: %v", err)
	}
	// THE SENDER IS THE CONNECTION'S AND IS NEVER IN THE REQUEST.
	if got := sent.GetMessage().GetFrom(); got != "team-lead" {
		t.Fatalf("the message says it came from %q, want team-lead", got)
	}
	if sent.GetMessage().GetState() != verbsv1.MessageState_MESSAGE_STATE_QUEUED {
		t.Fatalf("a message nobody was parked for is %s, want QUEUED", sent.GetMessage().GetState())
	}
	if sent.GetDelivered() != 0 {
		t.Fatalf("delivered %d with nobody parked; zero is not a loss and must say so", sent.GetDelivered())
	}

	var box verbsv1.MessageInboxResponse
	if err := worker.Call(ctx, "rig.message.inbox", &verbsv1.MessageInboxRequest{}, &box); err != nil {
		t.Fatalf("rig.message.inbox: %v", err)
	}
	if len(box.GetMessages()) != 1 || box.GetMessages()[0].GetSubject() != "the proto is regenerated" {
		t.Fatalf("the inbox answered %+v", box.GetMessages())
	}
	got := box.GetMessages()[0]
	if got.GetState() != verbsv1.MessageState_MESSAGE_STATE_READ {
		t.Fatalf("a message that came back from an inbox is %s, want READ", got.GetState())
	}
	if len(box.GetMisaddressed()) != 0 {
		t.Fatalf("an UNPINNED message was flagged as misaddressed: %v", box.GetMisaddressed())
	}
	if box.GetCursor() != got.GetId() || box.GetGap() {
		t.Fatalf("cursor %d gap %v for message %d", box.GetCursor(), box.GetGap(), got.GetId())
	}

	// The cursor is honoured: the same seat asking again learns nothing new.
	var second verbsv1.MessageInboxResponse
	if err := worker.Call(ctx, "rig.message.inbox", &verbsv1.MessageInboxRequest{
		After: box.GetCursor(),
	}, &second); err != nil || len(second.GetMessages()) != 0 {
		t.Fatalf("reading past the cursor gave %d messages, %v", len(second.GetMessages()), err)
	}

	// ACTED_ON IS THE RECIPIENT'S OWN STATEMENT AND CARRIES WHAT IT DID.
	var acked verbsv1.MessageAckResponse
	if err := worker.Call(ctx, "rig.message.ack", &verbsv1.MessageAckRequest{
		Id: got.GetId(), State: verbsv1.MessageState_MESSAGE_STATE_ACTED_ON, Outcome: "pulled and rebuilt",
	}, &acked); err != nil {
		t.Fatalf("rig.message.ack: %v", err)
	}
	if acked.GetMessage().GetState() != verbsv1.MessageState_MESSAGE_STATE_ACTED_ON ||
		acked.GetMessage().GetOutcome() != "pulled and rebuilt" {
		t.Fatalf("acked %+v", acked.GetMessage())
	}

	// The SENDER's surface. It promotes nothing and it is how a sender learns
	// that acted-on - the only state it may plan against - has happened.
	var listed verbsv1.MessageListResponse
	if err := lead.Call(ctx, "rig.message.list", &verbsv1.MessageListRequest{Seat: "backend-1"}, &listed); err != nil {
		t.Fatalf("rig.message.list: %v", err)
	}
	if len(listed.GetMessages()) != 1 ||
		listed.GetMessages()[0].GetState() != verbsv1.MessageState_MESSAGE_STATE_ACTED_ON {
		t.Fatalf("the estate view answered %+v", listed.GetMessages())
	}
	if listed.GetMessages()[0].GetMisaddressed() {
		t.Fatalf("an unpinned message read by the seat's own occupant was called misaddressed")
	}
}

// ⛔ THIS IS THE DEFECT THE FEATURE EXISTS FOR. Boris, 2026-09-11: "peers
// should be aware they may be contacted wrongly thinking they are the
// successor". A pin that no longer matches is REFUSED at send, naming the
// generation actually in the seat, and a pin that goes stale between the send
// and the read is FLAGGED to the reader instead of being delivered quietly.
func TestAMessagePinnedToADeadGenerationIsRefusedAndAStaleOneIsFlagged(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)
	lead := seated(t, sock, "team-lead")

	first := seated(t, sock, "backend-1")
	gen1, epoch := seatOf(ctx, t, first, "backend-1")

	// Pinned to the tenancy that is actually there: accepted.
	var pinned verbsv1.MessageSendResponse
	if err := lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
		To: "backend-1", ToGeneration: gen1, ToEpoch: epoch,
		Subject: "for you specifically", Body: "finish the migration you started",
	}, &pinned); err != nil {
		t.Fatalf("a message pinned to the live tenancy: %v", err)
	}

	// A pin is two numbers or it is neither: the counter restarts with the
	// daemon, so one without the other silently matches a previous run.
	err := lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
		To: "backend-1", ToGeneration: gen1,
	}, &verbsv1.MessageSendResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a generation with no epoch")

	// The tenancy ends and a successor takes the seat.
	_ = first.Close()
	var second *client.Client
	var gen2 uint64
	for range 200 {
		second = dial(t, sock)
		e := second.Call(ctx, "rig.announce", &verbsv1.AnnounceRequest{
			Seat: "backend-1", Purpose: "the successor", Activity: "resuming",
		}, &verbsv1.AnnounceResponse{})
		if e == nil {
			break
		}
		// The predecessor's connection is closed from this end; the daemon
		// frees the seat when its own handler returns, which is a moment later.
		_ = second.Close()
		second = nil
		time.Sleep(5 * time.Millisecond)
	}
	if second == nil {
		t.Fatal("the successor never got the seat")
	}
	gen2, _ = seatOf(ctx, t, second, "backend-1")
	if gen2 <= gen1 {
		t.Fatalf("the successor is generation %d and the predecessor was %d", gen2, gen1)
	}

	// REFUSED, and the refusal names the generation that is really there.
	err = lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
		To: "backend-1", ToGeneration: gen1, ToEpoch: epoch, Subject: "stale",
	}, &verbsv1.MessageSendResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a message pinned to a tenancy that has ended")

	// And the one that was ALREADY queued when the seat turned over is not
	// lost and is not delivered quietly: the successor gets it WITH the flag.
	var box verbsv1.MessageInboxResponse
	if err := second.Call(ctx, "rig.message.inbox", &verbsv1.MessageInboxRequest{}, &box); err != nil {
		t.Fatalf("the successor's inbox: %v", err)
	}
	if len(box.GetMisaddressed()) != 1 || box.GetMisaddressed()[0] != pinned.GetMessage().GetId() {
		t.Fatalf("the successor was not warned: misaddressed %v for message %d",
			box.GetMisaddressed(), pinned.GetMessage().GetId())
	}

	// The SENDER is told too, after the fact, from the two pairs the message
	// carries: this is how a human finds mail that reached the wrong session.
	var listed verbsv1.MessageListResponse
	if err := lead.Call(ctx, "rig.message.list", &verbsv1.MessageListRequest{}, &listed); err != nil {
		t.Fatalf("rig.message.list: %v", err)
	}
	var flagged int
	for _, m := range listed.GetMessages() {
		if m.GetMisaddressed() {
			flagged++
		}
	}
	if flagged != 1 {
		t.Fatalf("%d messages flagged to the sender, want 1: %+v", flagged, listed.GetMessages())
	}
}

// A seat nobody holds and a name nobody has ever used are DIFFERENT refusals,
// because they send a sender to different places: wait, versus look at what
// you typed.
func TestAnEmptySeatAndAnUnknownNameAreRefusedDifferently(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)
	lead := seated(t, sock, "team-lead")

	gone := seated(t, sock, "backend-1")
	_ = gone.Close()

	var empty, unknown string
	for range 200 {
		err := lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
			To: "backend-1", Subject: "anyone there",
		}, &verbsv1.MessageSendResponse{})
		var ce *client.CallError
		if asCallError(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND {
			empty = ce.Status.GetMessage()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if empty == "" {
		t.Fatal("a send to a seat whose occupant has gone was never refused")
	}

	err := lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
		To: "backedn-1", Subject: "typo",
	}, &verbsv1.MessageSendResponse{})
	var ce *client.CallError
	if !asCallError(err, &ce) || ce.Code() != rigv1.Code_CODE_NOT_FOUND {
		t.Fatalf("a send to a name nobody has used gave %v", err)
	}
	unknown = ce.Status.GetMessage()
	if empty == unknown {
		t.Fatalf("an empty seat and an unknown name were refused with the same sentence: %q", empty)
	}
}

// An await parks until there is mail, and the message it wakes on is DELIVERED
// by the daemon before the reader promotes it to READ - which is what makes
// `delivered` a number a sender can act on rather than a guess.
func TestAnAwaitParksAndIsWokenByASend(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)
	lead := seated(t, sock, "team-lead")
	worker := seated(t, sock, "backend-1")

	// A POINTER TO THE RESPONSE, because a proto message carries a mutex and
	// go vet refuses a struct that copies one.
	type answer struct {
		resp *verbsv1.MessageAwaitResponse
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		a := answer{resp: &verbsv1.MessageAwaitResponse{}}
		a.err = worker.Call(ctx, "rig.message.await", &verbsv1.MessageAwaitRequest{WaitMs: 4000}, a.resp)
		done <- a
	}()

	// Send until a reader is actually parked, so the test does not depend on
	// the goroutine above having reached the daemon first.
	var sent verbsv1.MessageSendResponse
	for range 400 {
		if err := lead.Call(ctx, "rig.message.send", &verbsv1.MessageSendRequest{
			To: "backend-1", Subject: "wake up", Body: "the build is red",
		}, &sent); err != nil {
			t.Fatalf("rig.message.send: %v", err)
		}
		if sent.GetDelivered() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if sent.GetDelivered() == 0 {
		t.Fatal("no send ever found a parked reader")
	}
	if sent.GetMessage().GetState() != verbsv1.MessageState_MESSAGE_STATE_DELIVERED {
		t.Fatalf("a message handed to a parked reader is %s, want DELIVERED", sent.GetMessage().GetState())
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("rig.message.await: %v", got.err)
	}
	if got.resp.GetTimedOut() || len(got.resp.GetMessages()) == 0 {
		t.Fatalf("the await answered %+v", got.resp)
	}
}

// An await that finds nothing says so rather than answering an empty batch a
// caller cannot tell from a read.
func TestAnAwaitWithNoMailSaysItTimedOut(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	worker := seated(t, sock, "backend-1")
	var resp verbsv1.MessageAwaitResponse
	if err := worker.Call(recordCtx(t), "rig.message.await",
		&verbsv1.MessageAwaitRequest{WaitMs: 30}, &resp); err != nil {
		t.Fatalf("rig.message.await: %v", err)
	}
	if !resp.GetTimedOut() || len(resp.GetMessages()) != 0 {
		t.Fatalf("an await over an empty queue answered %+v", &resp)
	}
}

// An inbox belongs to a seat, and an unseated connection has none to open.
// The seat is never named in the request, so there is nothing to claim.
func TestAnUnseatedConnectionHasNoInbox(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	c := dial(t, sock)
	err := c.Call(recordCtx(t), "rig.message.inbox", &verbsv1.MessageInboxRequest{},
		&verbsv1.MessageInboxResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "an inbox read by a connection in no seat")
}

// seatOf reads one seat's generation and epoch off the roster, which is where
// a sender gets a pin from in real use.
func seatOf(ctx context.Context, t *testing.T, c *client.Client, seat string) (generation, epoch uint64) {
	t.Helper()
	var resp verbsv1.PeersResponse
	if err := c.Call(ctx, "rig.peers", &verbsv1.PeersRequest{}, &resp); err != nil {
		t.Fatalf("rig.peers: %v", err)
	}
	for _, s := range resp.GetCrew() {
		if s.GetSeat() == seat {
			return s.GetGeneration(), s.GetEpoch()
		}
	}
	t.Fatalf("seat %q is not on the roster", seat)
	return 0, 0
}

// asCallError is errors.As for the client's refusal, named so the two places
// that need the SENTENCE rather than only the code read as one thing.
func asCallError(err error, into **client.CallError) bool {
	return errors.As(err, into)
}
