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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/paths"
	"github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// Handler answers a request rigd routed to this program.
type Handler func(method string, payload []byte) (proto.Message, error)

// conn is ONE physical connection, and it is separate from Client because a
// Client now outlives its connections.
//
// Section 5d duty 3 says the stub reconnects when rig restarts. That is only
// expressible if the things that die with a socket are separable from the
// things that do not: the framer, the streams waiting on it and the reason it
// ended die here, while the socket path, the stream counter and the handler
// live on Client and survive.
type conn struct {
	w *wire.Conn

	pmu     sync.Mutex
	pending map[uint32]chan *rigv1.Frame

	// gone closes when this connection's read loop ends. It is per-connection
	// on purpose: Client.Done is a different question and a coarser one.
	gone    chan struct{}
	readErr atomic.Value // error
}

func (cn *conn) err() error {
	if e, ok := cn.readErr.Load().(error); ok {
		return e
	}
	return nil
}

// Client is a connection to rigd that survives rig restarting.
type Client struct {
	// socket is remembered so a reconnect can re-dial it. Dial used to throw
	// the path away, which made duty 3 unimplementable without an API change.
	socket string

	// Client-opened streams are odd; the daemon's are even (section 5f's
	// multiplexing), so neither side needs to negotiate a range.
	next atomic.Uint32

	handler Handler

	mu  sync.Mutex
	cur *conn

	closeOnce sync.Once
	closed    chan struct{}
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
	c := &Client{socket: socket, closed: make(chan struct{})}
	c.next.Store(1) // +2, so client streams stay odd
	c.cur = c.adopt(nc)
	return c, nil
}

// adopt wraps a live socket in a conn and starts its read loop.
func (c *Client) adopt(nc net.Conn) *conn {
	cn := &conn{
		w:       wire.NewConn(nc),
		pending: make(map[uint32]chan *rigv1.Frame),
		gone:    make(chan struct{}),
	}
	go c.read(cn)
	return cn
}

// connection returns a live connection, re-dialling if the last one ended.
//
// This is duty 3. It is deliberately lazy rather than a background loop: a
// program that is not calling anything does not need a socket, and a
// reconnection nobody asked for is a wakeup on an idle machine.
func (c *Client) connection() (*conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur != nil {
		select {
		case <-c.cur.gone:
		default:
			return c.cur, nil
		}
	}
	//nolint:noctx // Same reason as Dial above: a unix socket connect has no
	// network to bound. The call's deadline is what bounds the retry loop.
	nc, err := net.Dial("unix", c.socket)
	if err != nil {
		return nil, fmt.Errorf("client: dial %s: %w", c.socket, err)
	}
	c.cur = c.adopt(nc)
	return c.cur, nil
}

// Handle sets the handler for requests rigd routes to this program. Set it
// before Hello, or a request can arrive with nothing to answer it.
func (c *Client) Handle(h Handler) { c.handler = h }

// Done closes when this CLIENT is finished, which now means Close was called.
//
// ITS MEANING CHANGED WITH DUTY 3, AND THE CHANGE IS THE FEATURE. It used to
// close when the connection ended, because a dropped socket was the end of the
// client. Under section 5g it is not: the client tolerates rig's absence and
// re-dials, so a program that exited on this signal would have been exiting on
// a rig restart - the exact thing section 5g exists to stop.
//
// A caller that wants to know rig is unreachable reads it off a CALL, as the
// typed unavailable error section 5g asks for. That is the one place it can be
// answered with a deadline attached.
func (c *Client) Done() <-chan struct{} { return c.closed }

// Err returns why the last connection ended, or nil. It is informational: a
// connection ending is no longer terminal, so this reports rather than decides.
func (c *Client) Err() error {
	c.mu.Lock()
	cn := c.cur
	c.mu.Unlock()
	if cn == nil {
		return nil
	}
	return cn.err()
}

func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		c.mu.Lock()
		cn := c.cur
		c.mu.Unlock()
		if cn != nil {
			err = cn.w.Close()
		}
	})
	return err
}

func (c *Client) read(cn *conn) {
	defer close(cn.gone)
	for {
		f, err := cn.w.ReadFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				cn.readErr.Store(err)
			}
			// Wake everything waiting, or a caller blocks until its deadline
			// on a connection that is already gone.
			cn.pmu.Lock()
			for id, ch := range cn.pending {
				close(ch)
				delete(cn.pending, id)
			}
			cn.pmu.Unlock()
			return
		}

		cn.pmu.Lock()
		ch, waiting := cn.pending[f.GetStreamId()]
		if waiting {
			delete(cn.pending, f.GetStreamId())
		}
		cn.pmu.Unlock()
		if waiting {
			ch <- f
			continue
		}
		// Not a reply: rigd is asking this program for something.
		go c.answer(cn, f)
	}
}

func (c *Client) answer(cn *conn, f *rigv1.Frame) {
	if c.handler == nil || f.GetKind() != rigv1.FrameKind_FRAME_KIND_REQUEST {
		_ = cn.w.WriteFrame(&rigv1.Frame{
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
		_ = cn.w.WriteFrame(&rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
			Status:   &rigv1.Status{Code: rigv1.Code_CODE_INTERNAL, Message: err.Error()},
		})
		return
	}
	body, err := proto.Marshal(msg)
	if err != nil {
		_ = cn.w.WriteFrame(&rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
			Status:   &rigv1.Status{Code: rigv1.Code_CODE_INTERNAL, Message: err.Error()},
		})
		return
	}
	_ = cn.w.WriteFrame(&rigv1.Frame{
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

	// DUTY 4's RETRY LOOP, AND IT SITS BELOW THE MINT ON PURPOSE.
	//
	// Every attempt below re-sends the SAME rid. That is the whole reason the
	// mint is above this line, and it is now a property a test can fail rather
	// than a comment asking to be trusted.
	var last error
	for attempt := 0; ; attempt++ {
		retry, err := c.attempt(ctx, method, rid, body, out)
		if !retry {
			return err
		}
		last = err

		// Backoff to the DEADLINE, which is the caller's, not a fixed count.
		// Section 5g says "backoff to a deadline"; a retry budget in attempts
		// would give a caller with ten seconds the same patience as one with
		// one, and neither of them what they asked for.
		wait := time.NewTimer(backoff(attempt))
		select {
		case <-wait.C:
		case <-ctx.Done():
			wait.Stop()
			return unavailable(method, last)
		case <-c.closed:
			wait.Stop()
			return unavailable(method, last)
		}
	}
}

// attempt makes ONE try. It returns whether the failure is worth another.
//
// The split exists so the retry decision is in one readable place: rig
// ANSWERING, with a refusal or a bad payload, is a result and never retried -
// re-sending would turn one refusal into several. Only a connection that was
// not there, or died mid-call, earns another attempt.
func (c *Client) attempt(
	ctx context.Context, method, rid string, body []byte, out proto.Message,
) (retry bool, err error) {
	select {
	case <-c.closed:
		return false, unavailable(method, errors.New("the client is closed"))
	case <-ctx.Done():
		return false, unavailable(method, ctx.Err())
	default:
	}

	cn, err := c.connection()
	if err != nil {
		return true, err
	}

	sid := c.next.Add(2)
	ch := make(chan *rigv1.Frame, 1)
	cn.pmu.Lock()
	cn.pending[sid] = ch
	cn.pmu.Unlock()
	defer func() {
		cn.pmu.Lock()
		delete(cn.pending, sid)
		cn.pmu.Unlock()
	}()

	if err := cn.w.WriteFrame(&rigv1.Frame{
		StreamId:  sid,
		Kind:      rigv1.FrameKind_FRAME_KIND_REQUEST,
		Method:    method,
		RequestId: rid,
		Payload:   body,
	}); err != nil {
		return true, fmt.Errorf("client: %s: %w", method, err)
	}

	select {
	case f, open := <-ch:
		if !open {
			// The read loop closed this stream, so the connection ended
			// before an answer arrived. Worth another attempt: the id is
			// stable, so a daemon with the window of section 4 can recognise
			// the re-send rather than applying it twice.
			if e := cn.err(); e != nil {
				return true, fmt.Errorf("client: %s: connection ended: %w", method, e)
			}
			return true, fmt.Errorf("client: %s: connection closed before a reply", method)
		}
		if f.GetKind() == rigv1.FrameKind_FRAME_KIND_ERROR {
			return false, &CallError{Method: method, Status: f.GetStatus()}
		}
		if err := proto.Unmarshal(f.GetPayload(), out); err != nil {
			return false, fmt.Errorf("client: %s: unmarshal: %w", method, err)
		}
		return false, nil
	case <-ctx.Done():
		return false, fmt.Errorf("client: %s: %w", method, ctx.Err())
	}
}

// backoff is the delay before attempt n+1: 10ms doubling to a 1s ceiling.
//
// The ceiling matters more than the curve. Without one, a long deadline turns
// the last gap into most of the wait, so a rig that came back early is not
// noticed until well after it did.
func backoff(attempt int) time.Duration {
	const base, ceiling = 10 * time.Millisecond, time.Second
	d := base << min(attempt, 10)
	return min(d, ceiling)
}

// unavailable is section 5g's ONE typed error for "rig is not there".
//
// It is a *CallError carrying CODE_UNAVAILABLE rather than a new exported
// type, and that is deliberate on two counts. Section 3 budgets this package's
// exported symbols, so a new one costs a decision in the plan; and CODE_
// UNAVAILABLE is already the wire's word for "rig is not there, or is shutting
// down", so inventing a second vocabulary for the same fact is the collapse
// this repository keeps finding. A caller tests it the way it tests any
// refusal: errors.As for *CallError, then Code.
func unavailable(method string, cause error) error {
	return &CallError{
		Method: method,
		Status: &rigv1.Status{
			Code:    rigv1.Code_CODE_UNAVAILABLE,
			Message: "rig is not reachable: " + cause.Error(),
			Fix:     "start rigd, then run this again",
		},
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
// IT IS math/rand/v2 RATHER THAN crypto/rand, AND THE REASON IS MEASURED.
//
// crypto/rand costs this package 159,744 bytes and drags in SEVENTEEN
// crypto/internal/fips140 packages - SHA-256, SHA-3, SHA-512, HMAC, AES,
// AES-GCM and the FIPS self-check - which every program linking the stub then
// carries forever. Measured on cmd/fakeapp with make's own flags: 5,849,351
// bytes with math/rand/v2, which is byte-identical to the binary before this
// id existed, against 6,009,095 with crypto/rand. Section 5d wants this
// package "as close to frozen as it can be" because "every symbol in it is a
// rebuild nobody can avoid later"; 156KB of cryptography for an eight-byte
// dedup key is the opposite of that.
//
// A REQUEST ID IS A DEDUP KEY, NOT AN AUTHENTICATOR, and that is the whole
// argument. Nothing in section 4 or section 5d asks for it to be unguessable;
// they ask for it to be client-generated and stable across a retry. The daemon
// does use crypto/rand, for session TOKENS, and that is correct - a token
// authenticates and this does not. math/rand/v2 is seeded from the operating
// system per process, so two processes do not agree on a sequence.
//
// IF THE WINDOW THAT EVENTUALLY CONSUMES THIS TURNS OUT TO NEED UNGUESSABLE
// IDS, the change is one import and the price is the 159,744 bytes above,
// now known in advance rather than discovered by a red ratchet.
func requestID() string {
	b := make([]byte, 8)
	for i := range b {
		//nolint:gosec // G404 is right that this is not crypto/rand, and that
		// is the decision rather than an oversight. A request id is a dedup
		// key, not an authenticator: nothing in section 4 or section 5d asks
		// for it to be unguessable, and the one value in rig that must be
		// unguessable - the session token - is minted with crypto/rand in the
		// daemon and stays that way. The measured price of silencing this
		// linter honestly is 159,744 bytes and seventeen fips140 packages in
		// every program that links the stub.
		b[i] = byte(rand.UintN(256))
	}
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
