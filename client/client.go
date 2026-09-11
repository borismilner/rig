// Package client is the stub a program links to talk to rig.
//
// It is deliberately outside internal/. Go forbids any other module from
// importing an internal package, so while this lived at internal/client no
// program outside this repository could reach rig at all - fakeapp worked
// only because it ships inside rig's own module. PLAN.md section 3 gives this
// package a symbol budget, section 5d says what it may and may not do, and
// section 25 defers graft until it exists; surface.go is the enumeration
// section 3 asks for.
//
// One implementation, used by cmd/rig and by a program's own side, because
// PLAN.md section 5d's whole complaint about the previous design was a second
// implementation of things that should exist once.
package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/paths"
	"github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// Handler answers a request rigd routed to this program.
type Handler func(method string, payload []byte) (proto.Message, error)

// Client is one connection to rigd.
type Client struct {
	w *wire.Conn

	// Client-opened streams are odd; the daemon's are even (section 5f's
	// multiplexing), so neither side needs to negotiate a range.
	next atomic.Uint32

	pmu     sync.Mutex
	pending map[uint32]chan *rigv1.Frame

	handler Handler

	closeOnce sync.Once
	readErr   atomic.Value // error
	done      chan struct{}
}

// Connect dials rig at its well-known socket.
//
// This is the entry point a program uses. Dial exists for a caller that knows
// the path already - a test, or rig's own CLI with a socket override - but a
// program must not have to resolve the path itself: the resolution rules live
// in section 5f, and a program reimplementing them is a second implementation
// of the thing this package exists to have exactly one of.
func Connect() (*Client, error) {
	socket, err := paths.Socket()
	if err != nil {
		return nil, err
	}
	return Dial(socket)
}

// Dial connects to the socket and starts the read loop.
func Dial(socket string) (*Client, error) {
	//nolint:noctx // A unix socket connect touches the filesystem and the
	// listen backlog, never a network. There is nothing for a dial deadline
	// to bound; the deadline that matters is on the call, and nocontextfree
	// is the analyzer that enforces it (section 3).
	nc, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("client: dial %s: %w", socket, err)
	}
	c := &Client{
		w:       wire.NewConn(nc),
		pending: make(map[uint32]chan *rigv1.Frame),
		done:    make(chan struct{}),
	}
	c.next.Store(1) // +2, so client streams stay odd
	go c.read()
	return c, nil
}

// Handle sets the handler for requests rigd routes to this program. Set it
// before Hello, or a request can arrive with nothing to answer it.
func (c *Client) Handle(h Handler) { c.handler = h }

// Done closes when the connection ends. Err says why.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err returns the read loop's error, or nil if it ended cleanly.
func (c *Client) Err() error {
	if e, ok := c.readErr.Load().(error); ok {
		return e
	}
	return nil
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() { err = c.w.Close() })
	return err
}

func (c *Client) read() {
	defer close(c.done)
	for {
		f, err := c.w.ReadFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				c.readErr.Store(err)
			}
			// Wake everything waiting, or a caller blocks until its deadline
			// on a connection that is already gone.
			c.pmu.Lock()
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.pmu.Unlock()
			return
		}

		c.pmu.Lock()
		ch, waiting := c.pending[f.GetStreamId()]
		if waiting {
			delete(c.pending, f.GetStreamId())
		}
		c.pmu.Unlock()
		if waiting {
			ch <- f
			continue
		}
		// Not a reply: rigd is asking this program for something.
		go c.answer(f)
	}
}

func (c *Client) answer(f *rigv1.Frame) {
	if c.handler == nil || f.GetKind() != rigv1.FrameKind_FRAME_KIND_REQUEST {
		_ = c.w.WriteFrame(&rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
			Status: &rigv1.Status{
				Code:    rigv1.Code_CODE_NOT_FOUND,
				Message: "this program serves no requests",
			},
		})
		return
	}
	msg, err := c.handler(f.GetMethod(), f.GetPayload())
	if err != nil {
		_ = c.w.WriteFrame(&rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
			Status:   &rigv1.Status{Code: rigv1.Code_CODE_INTERNAL, Message: err.Error()},
		})
		return
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		_ = c.w.WriteFrame(&rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
			Status:   &rigv1.Status{Code: rigv1.Code_CODE_INTERNAL, Message: err.Error()},
		})
		return
	}
	_ = c.w.WriteFrame(&rigv1.Frame{
		StreamId: f.GetStreamId(),
		Kind:     rigv1.FrameKind_FRAME_KIND_RESPONSE,
		Payload:  body,
	})
}

// Call makes one request and decodes the response into out.
func (c *Client) Call(ctx context.Context, method string, in, out proto.Message) error {
	body, err := proto.Marshal(in)
	if err != nil {
		return fmt.Errorf("client: marshal %s: %w", method, err)
	}

	// Section 5d duty 2, and WHERE it is minted is the whole point.
	//
	// One Call is one logical invocation. The id is minted here, once, above
	// everything that could ever re-send: when duty 3's reconnect and duty 4's
	// backoff land in this package, their loop goes below this line and the id
	// it carries is already fixed. That is what makes "stable across a retry"
	// true by construction rather than by a rule somebody has to remember.
	//
	// Minting inside the write instead would produce an id that is unique per
	// ATTEMPT, which is the exact inversion internal/daemon/meta.go warns
	// about: it would make every retry look like a new call and defeat the
	// window meant to consume it.
	rid := requestID()

	sid := c.next.Add(2)
	ch := make(chan *rigv1.Frame, 1)
	c.pmu.Lock()
	c.pending[sid] = ch
	c.pmu.Unlock()
	defer func() {
		c.pmu.Lock()
		delete(c.pending, sid)
		c.pmu.Unlock()
	}()

	if err := c.w.WriteFrame(&rigv1.Frame{
		StreamId:  sid,
		Kind:      rigv1.FrameKind_FRAME_KIND_REQUEST,
		Method:    method,
		RequestId: rid,
		Payload:   body,
	}); err != nil {
		return fmt.Errorf("client: %s: %w", method, err)
	}

	select {
	case f, open := <-ch:
		if !open {
			if e := c.Err(); e != nil {
				return fmt.Errorf("client: %s: connection ended: %w", method, e)
			}
			return fmt.Errorf("client: %s: connection closed before a reply", method)
		}
		if f.GetKind() == rigv1.FrameKind_FRAME_KIND_ERROR {
			return &CallError{Method: method, Status: f.GetStatus()}
		}
		if err := proto.Unmarshal(f.GetPayload(), out); err != nil {
			return fmt.Errorf("client: %s: unmarshal: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("client: %s: %w", method, ctx.Err())
	}
}

// Hello completes the program handshake by declaring what this program is.
// After it, this connection is scoped for its whole life (section 14).
//
// The declaration is required, and it is the handshake rather than a later
// call because section 14's predicate is about the connection: a connection
// that completed registration is a program, and there is no other way to
// become one. Section 5e says what a declaration must contain, and rig
// refuses one that is missing a mandatory property rather than reading an
// absent field as the safe answer.
//
// program and version are sent alongside the declaration and taken from it,
// so a caller has one place to say each.
func (c *Client) Hello(ctx context.Context, decl *rigv1.Declaration) (*rigv1.HelloResponse, error) {
	out := &rigv1.HelloResponse{}
	err := c.Call(ctx, "rig.hello", &rigv1.HelloRequest{
		Program:     decl.GetIdentity().GetId(),
		Version:     decl.GetIdentity().GetVersion(),
		Declaration: decl,
	}, out)
	return out, err
}

// requestID mints the client-generated id section 5d duty 2 asks for.
//
// EVERY call is stamped, not only the mutating ones, and that is a deliberate
// reading of a tension inside section 5d itself: duty 2 says "stamp every
// mutating call", while the paragraph below it forbids this package from
// knowing "what commands exist". Deciding that a call mutates needs declared
// effects, which is exactly the schema knowledge that paragraph bars. Stamping
// unconditionally needs none, and it cannot get the answer wrong: a stamped
// read is ignored by a window keyed on the method, where an unstamped write is
// the failure the id exists to prevent. Ruled 2026-09-11.
//
// It is unexported, so section 3's surface does not grow.
//
// The shape matches randomID in internal/daemon, on purpose, so ids across the
// estate read alike. It is not shared code: this package must not import the
// daemon, and cmd/rig's layering test exists to keep it that way. A daemon
// minting its own identities and a client minting its own call ids are two
// parties doing two things that happen to want the same spelling.
//
// crypto/rand.Read never returns an error and always fills its argument
// (Go 1.24 onwards), so there is nothing here to handle or to paper over with
// a counter that could collide.
func requestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}

// CallError is a wire-level refusal, carrying rig's own status.
type CallError struct {
	Method string
	Status *rigv1.Status
}

func (e *CallError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Method,
		e.Status.GetCode(), e.Status.GetMessage())
}

// Code lets a caller branch on the refusal without string matching.
func (e *CallError) Code() rigv1.Code { return e.Status.GetCode() }
