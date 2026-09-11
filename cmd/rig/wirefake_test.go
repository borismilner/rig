package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	wirepkg "github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// A FAKE DAEMON, SO THE REQUEST THIS PACKAGE SENDS CAN BE ASSERTED.
//
// This package's documented coverage hole is the fifteen functions that need a
// live rigd, and every defect found in `cmd/rig` this week was found by RUNNING
// the binary rather than by a test: the --depth flag shipped broken past fifteen
// unit tests because none of them went through argv, and the depth a request
// actually carries had nothing watching it at all - a mutation that dropped the
// field from the wire survived a full suite.
//
// The gap is narrow enough to close in one file. cmd/rig already links
// internal/wire transitively (it is how the client frames anything), so a test
// can listen on a unix socket, read the frame the client sent, and assert what
// is IN it. That is a different claim from "the renderer produced the right
// words", and it is the claim nothing else here can make.
//
// The import is ALIASED because this package already has a package-level `wire`
// holding the wire contract's version string. Two unrelated things wanting one
// short name is the whole reason the collision exists, and aliasing here is
// cheaper than renaming a variable that appears in shipped --json output.
//
// It is NOT a substitute for the live demonstration. A fake answers what it is
// told to answer, so it can prove what rig SENDS and never what rigd DOES.

// fakeDaemon is a unix socket that answers one method with one message and
// records every request it was sent.
type fakeDaemon struct {
	socket string

	mu       sync.Mutex
	requests []*rigv1.Frame

	// dropFirst is how many connections to accept, record from, and then hang
	// up on WITHOUT replying. It is how a rig restart is staged: from the
	// client's side an unanswered frame on a closed socket is exactly what a
	// daemon going away mid-call looks like.
	dropFirst int

	// replyError, when set, is answered instead of reply: an ERROR frame, as
	// rig sends when it refuses. It exists because a mutation proved nothing
	// was watching what the client does with a refusal.
	replyError *rigv1.Status

	reply proto.Message
}

// startFakeDaemon listens on a socket in a temp dir and serves until the test
// ends. The directory is short because a unix socket path is capped around 108
// bytes and a Go test's own temp path can approach it.
func startFakeDaemon(t *testing.T, reply proto.Message) *fakeDaemon {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigfake")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	d := &fakeDaemon{socket: filepath.Join(dir, "s"), reply: reply}
	ln, err := net.Listen("unix", d.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go d.serve(nc)
		}
	}()
	return d
}

func (d *fakeDaemon) serve(nc net.Conn) {
	c := wirepkg.NewConn(nc)
	defer c.Close()
	for {
		f, err := c.ReadFrame()
		if err != nil {
			return
		}
		d.mu.Lock()
		d.requests = append(d.requests, f)
		drop := d.dropFirst > 0
		if drop {
			d.dropFirst--
		}
		d.mu.Unlock()
		if drop {
			return // hang up with the frame recorded and unanswered
		}

		d.mu.Lock()
		refuse := d.replyError
		d.mu.Unlock()
		if refuse != nil {
			if err := c.WriteFrame(&rigv1.Frame{
				StreamId: f.GetStreamId(),
				Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
				Status:   refuse,
			}); err != nil {
				return
			}
			continue
		}

		body, err := proto.Marshal(d.reply)
		if err != nil {
			return
		}
		if err := c.WriteFrame(&rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_RESPONSE,
			Payload:  body,
		}); err != nil {
			return
		}
	}
}

// sent returns the requests this daemon was sent, decoded into out.
func (d *fakeDaemon) sent(t *testing.T) []*rigv1.ProgramsRequest {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []*rigv1.ProgramsRequest
	for _, f := range d.requests {
		req := &rigv1.ProgramsRequest{}
		if err := proto.Unmarshal(f.GetPayload(), req); err != nil {
			t.Fatalf("the request payload did not decode: %v", err)
		}
		out = append(out, req)
	}
	return out
}

// THE DEPTH A CALLER ASKS FOR IS THE DEPTH THAT GOES ON THE WIRE.
//
// programAt takes a depth so `describe` can say "in full" rather than inherit
// it: an omitted depth arrives at the daemon as the zero and is restored to
// DEPTH_FULL by a compatibility rule written to keep the OLD output unchanged.
// Resting a new surface's definition on that rule means a change to it silently
// re-points describe.
//
// THIS TEST EXISTS BECAUSE A MUTATION PROVED IT WAS NEEDED. Dropping `Depth: d`
// from the request survived the entire suite - every renderer test still passed,
// because a renderer cannot see what was asked for.
func TestTheDepthIsAskedForRatherThanInherited(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{
		Programs: []*rigv1.Program{{
			Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.0.0"},
		}},
	})
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := programAt(ctx, c, "fakeapp", rigv1.Depth_DEPTH_FULL); err != nil {
		t.Fatalf("programAt: %v", err)
	}

	sent := d.sent(t)
	if len(sent) != 1 {
		t.Fatalf("the daemon was sent %d requests, want 1", len(sent))
	}
	if got := sent[0].GetDepth(); got != rigv1.Depth_DEPTH_FULL {
		t.Errorf("the request carried depth %s, want DEPTH_FULL. An omitted "+
			"depth is restored to full by a COMPATIBILITY rule, so this would "+
			"still look right today and would stop being right the moment that "+
			"rule moved", got)
	}

	// The control, and it is what makes the assertion above mean anything: a
	// different depth must actually travel differently. Without it the test
	// passes against a wire that ignores the field entirely.
	if _, err := programAt(ctx, c, "fakeapp", rigv1.Depth_DEPTH_PROGRAMS); err != nil {
		t.Fatalf("programAt at programs depth: %v", err)
	}
	sent = d.sent(t)
	if len(sent) != 2 {
		t.Fatalf("the daemon was sent %d requests, want 2", len(sent))
	}
	if got := sent[1].GetDepth(); got != rigv1.Depth_DEPTH_PROGRAMS {
		t.Errorf("the second request carried depth %s, want DEPTH_PROGRAMS: "+
			"the field is not travelling at all", got)
	}
}

// A program that is not connected is refused with what IS connected, and the
// refusal is the structured one rather than prose - section 5k, because "no
// such program" and "that program is not running right now" send a reader in
// different directions.
func TestAProgramThatIsNotConnectedIsRefusedWithTheOnesThatAre(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{
		Programs: []*rigv1.Program{
			{Identity: &rigv1.Identity{Id: "ledger"}},
			{Identity: &rigv1.Identity{Id: "docket"}},
		},
	})
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = programAt(ctx, c, "fakeapp", rigv1.Depth_DEPTH_FULL)
	if err == nil {
		t.Fatal("a program the daemon never listed was accepted")
	}
	text := errorText(err)
	for _, want := range []string{"fakeapp", "docket", "ledger"} {
		if !strings.Contains(text, want) {
			t.Errorf("the refusal does not name %q, so a reader cannot tell a "+
				"typo from a program that is not running:\n%s", want, text)
		}
	}
}

// frames returns the raw frames this daemon was sent, before any payload is
// decoded. sent() above throws the envelope away, and the request id lives on
// the envelope rather than in the body.
func (d *fakeDaemon) frames(t *testing.T) []*rigv1.Frame {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*rigv1.Frame(nil), d.requests...)
}

// EVERY CALL PUTS A CLIENT-GENERATED REQUEST ID ON THE WIRE.
//
// Section 5d duty 2. Before this, `request_id` was wire field 4, plumbed
// through the daemon in both directions, and set by nothing: a grep for it
// across client/ and cmd/rig returned no occurrences at all, so the field
// travelled empty from every rig surface that exists.
//
// STABILITY ACROSS A RETRY IS COVERED, and it is covered elsewhere:
// TestARetriedCallKeepsItsRequestID, below. This comment used to say the
// opposite - that the property could not be tested because duties 3 and 4 did
// not exist - and it was true when it was written. It is deleted rather than
// left standing, because a gap that has been closed and still reads as open
// sends the next reader to close it twice.
func TestEveryCallCarriesARequestID(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{
		Programs: []*rigv1.Program{{
			Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.0.0"},
		}},
	})
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := programAt(ctx, c, "fakeapp", rigv1.Depth_DEPTH_FULL); err != nil {
		t.Fatalf("programAt: %v", err)
	}

	sent := d.frames(t)
	if len(sent) != 1 {
		t.Fatalf("the daemon was sent %d frames, want 1", len(sent))
	}
	got := sent[0].GetRequestId()
	if got == "" {
		t.Fatal("the request carried an EMPTY request id. That is what every " +
			"rig surface sent before section 5d duty 2 was built, and it is " +
			"the state this test exists to stop returning to")
	}

	// The shape is pinned so that a change to it is visible in a diff rather
	// than discovered by whatever eventually consumes the id.
	const wantLen = len("req-") + 16 // 8 random bytes, hex
	if !strings.HasPrefix(got, "req-") || len(got) != wantLen {
		t.Errorf("the request id is %q, which is not the documented shape "+
			"(req- and 16 hex characters)", got)
	}
}

// THE CONTROL, AND IT IS WHAT MAKES THE TEST ABOVE MEAN ANYTHING.
//
// A hardcoded constant would satisfy every assertion in TestEveryCallCarries
// ARequestID: it is non-empty and it has the right shape. An id that never
// varies is worse than no id, because the daemon's window would eventually
// treat two unrelated calls as the same one and answer the second with the
// first one's reply.
//
// So this asserts the id is a property of the INVOCATION. Two calls, two ids.
func TestTwoCallsDoNotShareARequestID(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{
		Programs: []*rigv1.Program{{
			Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.0.0"},
		}},
	})
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 2 {
		if _, err := programAt(ctx, c, "fakeapp", rigv1.Depth_DEPTH_FULL); err != nil {
			t.Fatalf("programAt: %v", err)
		}
	}

	sent := d.frames(t)
	if len(sent) != 2 {
		t.Fatalf("the daemon was sent %d frames, want 2", len(sent))
	}
	if a, b := sent[0].GetRequestId(), sent[1].GetRequestId(); a == b {
		t.Errorf("two separate calls both carried request id %q. The id is "+
			"minted per invocation, so a constant here means the window that "+
			"consumes it would deduplicate calls that are not duplicates", a)
	}
}

// THE REQUEST ID SURVIVES A RETRY, WHICH IS THE HALF 58b6c1e COULD NOT PROVE.
//
// Section 5d duty 2 asks for an id "stable across a retry", and until duty 3's
// reconnect existed there was no retry for it to be stable across: a mutation
// that minted the id twice per call survived the whole suite, because nothing
// ever re-sent. That mutation is now caught here, and the comment admitting
// the gap has been deleted rather than inherited.
//
// The staging is a rig restart in miniature. The daemon accepts the first
// connection, records the frame and hangs up without answering. The client
// sees its stream close with no reply, re-dials, and sends again. Two frames
// arrive; they must carry ONE id.
//
// It matters because section 4's window keys on that id: an id that changed
// per attempt would make the re-send look like a second call, which is the
// inversion internal/daemon/meta.go warns about in as many words.
func TestARetriedCallKeepsItsRequestID(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{
		Programs: []*rigv1.Program{{
			Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.0.0"},
		}},
	})
	d.dropFirst = 1

	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := programAt(ctx, c, "fakeapp", rigv1.Depth_DEPTH_FULL); err != nil {
		t.Fatalf("the call did not survive one dropped connection: %v", err)
	}

	sent := d.frames(t)
	if len(sent) != 2 {
		t.Fatalf("the daemon saw %d frames, want 2 (one dropped, one answered). "+
			"If this is 1, nothing retried and the test proved nothing", len(sent))
	}
	if a, b := sent[0].GetRequestId(), sent[1].GetRequestId(); a != b {
		t.Errorf("the retry carried request id %q where the first attempt "+
			"carried %q. A per-attempt id makes a re-send look like a new "+
			"call, which is what the id exists to prevent", b, a)
	}
	if sent[0].GetRequestId() == "" {
		t.Error("both attempts carried an empty request id, so they match for " +
			"the wrong reason")
	}
}

// RIG BEING ABSENT IS ONE TYPED ERROR, NOT A PROSE STRING.
//
// Section 5g asks for "one typed unavailable error". This asserts a caller can
// actually branch on it: errors.As to *CallError, then Code. Before duty 4 an
// absent rig surfaced as a dial error wrapped in fmt.Errorf, which a caller
// could only match by reading its text.
//
// The deadline is short on purpose. The retry loop backs off to the CALLER's
// deadline, so this also pins that an exhausted deadline ends the loop instead
// of running past it.
func TestAnAbsentRigIsOneTypedError(t *testing.T) {
	dir, err := os.MkdirTemp("", "rignone")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// A daemon that is up for the dial and gone for every retry.
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{})
	d.dropFirst = 1000
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = os.Remove(d.socket) // rig is now genuinely absent

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// The STUB is what section 5g constrains, so the stub is what is asked.
	// Going through programAt would test cmd/rig's refusal wrapper instead:
	// the CLI deliberately converts a CallError into its own rendered refusal,
	// which is right for a terminal and is a different claim from this one.
	err = c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{
		Depth: rigv1.Depth_DEPTH_FULL,
	}, &rigv1.ProgramsResponse{})
	if err == nil {
		t.Fatal("calling an absent rig returned no error at all")
	}
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("the error is %T (%v), which a caller can only match by "+
			"reading its text. Section 5g asks for one TYPED error", err, err)
	}
	if ce.Code() != rigv1.Code_CODE_UNAVAILABLE {
		t.Errorf("an absent rig reported %s, want CODE_UNAVAILABLE, which is "+
			"the wire's own word for rig not being there", ce.Code())
	}
}

// A REFUSAL IS AN ANSWER, SO IT IS NEVER RETRIED.
//
// THIS TEST EXISTS BECAUSE A MUTATION PROVED IT WAS NEEDED. Flipping the retry
// decision on the ERROR-frame branch from false to true survived every test
// here: the call still returned the refusal, so nothing looked wrong. What it
// would actually do is re-send a refused call once per backoff step until the
// caller's deadline, turning one refusal into five or six and applying a
// mutating call repeatedly against a daemon whose dedup window does not exist
// yet.
//
// The distinction the retry loop rests on: rig ANSWERING is a result, however
// unwelcome, and only a connection that was not there or died mid-call earns
// another attempt.
func TestARefusalIsNotRetried(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.ProgramsResponse{})
	d.replyError = &rigv1.Status{
		Code:    rigv1.Code_CODE_DENIED,
		Message: "refused by house rules",
	}

	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Long enough that a retry loop would get several attempts in. If this
	// were the call's own deadline doing the work, the test would pass for
	// the wrong reason.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{}, &rigv1.ProgramsResponse{})
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("a refused call returned %T (%v), want *client.CallError", err, err)
	}
	if ce.Code() != rigv1.Code_CODE_DENIED {
		t.Errorf("the refusal arrived as %s, want CODE_DENIED. A retry loop "+
			"that swallowed it would report unavailable instead", ce.Code())
	}
	if n := len(d.frames(t)); n != 1 {
		t.Errorf("the daemon was sent %d frames for one refused call, want 1. "+
			"Re-sending a refusal applies a mutating call more than once", n)
	}
}
