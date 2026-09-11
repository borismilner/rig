package main

import (
	"context"
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
		d.mu.Unlock()

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
// WHAT THIS TEST DOES NOT PROVE, said plainly rather than left to be assumed:
// that the id is STABLE ACROSS A RETRY. It cannot, because there is no retry
// to be stable across - section 5d's duties 3 and 4 are not built yet, and a
// grep for retry or backoff in client/ finds only the comment saying where
// they belong. The id is minted above everything in Call that could re-send,
// which makes stability true by construction; the test that can actually fail
// arrives with the reconnect loop, and this comment is here so nobody reads
// the two tests below as having covered it.
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
