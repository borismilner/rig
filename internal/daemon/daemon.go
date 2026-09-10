// Package daemon is rigd: one socket, one instance, and the routing between a
// client and a program.
//
// What it is NOT, at M0: a registry. A program says hello and is routed to by
// name. M1 replaces that with real registration - a declaration validated
// against its schema, capabilities, and the computed projection (PLAN.md M1).
// The line is kept deliberately visible so the registry lands as one thing
// rather than accreting here.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/instance"
	"github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// CallTimeout bounds a routed call.
//
// PLAN.md section 18: "every call has a deadline. A program that misses them is
// degraded, then restarted. A hung program can never block a rig goroutine."
// M0 has no supervisor to degrade anything, so the deadline is all there is -
// which makes it the part that must exist from the first commit.
const CallTimeout = 10 * time.Second

// Config is what a daemon needs to exist.
type Config struct {
	Version string
	Wire    string
	Log     *slog.Logger

	// Lock is the single-instance claim, and it is REQUIRED.
	//
	// Section 5f says rigd takes the flock "before it binds". Stating an
	// ordering rule in prose puts it one careless refactor from being false,
	// so the type carries it instead: a Daemon cannot be constructed without
	// the proof that this process is the only rigd, and the only way to hold
	// that proof is to have taken the lock already. Two daemons over one state
	// tree give two serialisation points, two WALs and two lock namespaces,
	// and every property section 16 proves is false for as long as it lasts.
	Lock *instance.Lock
}

// Daemon serves one socket.
type Daemon struct {
	version string
	wire    string
	log     *slog.Logger
	lock    *instance.Lock

	mu       sync.RWMutex
	programs map[string]*conn
}

// New builds a daemon, and refuses without a held single-instance lock.
func New(cfg Config) (*Daemon, error) {
	if cfg.Lock == nil {
		return nil, errors.New("daemon: no single-instance lock; " +
			"section 5f requires the flock before the bind, and this is where that is enforced")
	}
	if cfg.Wire == "" {
		return nil, errors.New("daemon: no wire version")
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Daemon{
		version:  cfg.Version,
		wire:     cfg.Wire,
		log:      log,
		lock:     cfg.Lock,
		programs: make(map[string]*conn),
	}, nil
}

// conn is one connection and everything decided about it.
type conn struct {
	w   *wire.Conn
	log *slog.Logger

	// scoped is section 14's predicate, and the only authorisation state a
	// connection carries. It is set by the program handshake and never unset:
	// a connection that registered as a program is scoped for its whole life,
	// so a program cannot shed the scope by asking again.
	scoped  atomic.Bool
	program atomic.Value // string

	// Daemon-initiated streams are even; client-initiated are odd. The two
	// sides therefore never collide without negotiating anything.
	nextStream atomic.Uint32

	pmu     sync.Mutex
	pending map[uint32]chan *rigv1.Frame
}

func (c *conn) name() string {
	s, _ := c.program.Load().(string)
	return s
}

// Serve accepts until the listener closes or ctx ends.
func (d *Daemon) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		nc, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // asked to stop; not a failure
			}
			return fmt.Errorf("daemon: accept: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.handle(ctx, nc)
		}()
	}
}

func (d *Daemon) handle(ctx context.Context, nc net.Conn) {
	c := &conn{
		w:       wire.NewConn(nc),
		log:     d.log,
		pending: make(map[uint32]chan *rigv1.Frame),
	}
	c.nextStream.Store(0) // +2 each time, so daemon streams stay even
	defer func() {
		_ = c.w.Close()
		if n := c.name(); n != "" {
			d.mu.Lock()
			if d.programs[n] == c {
				delete(d.programs, n)
			}
			d.mu.Unlock()
			d.log.Info("program gone", "program", n)
		}
	}()

	for {
		f, err := c.w.ReadFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				d.log.Warn("connection dropped", "program", c.name(), "err", err)
			}
			return
		}
		// A terminal frame on a daemon-opened stream is a reply to something
		// this daemon asked. Deliver it and do not treat it as a request.
		if c.deliver(f) {
			continue
		}
		go d.dispatch(ctx, c, f)
	}
}

// deliver hands a reply to whoever is waiting for that stream. It reports
// whether the frame was a reply at all.
func (c *conn) deliver(f *rigv1.Frame) bool {
	c.pmu.Lock()
	ch, ok := c.pending[f.GetStreamId()]
	if ok {
		delete(c.pending, f.GetStreamId())
	}
	c.pmu.Unlock()
	if !ok {
		return false
	}
	ch <- f
	return true
}

func (d *Daemon) dispatch(ctx context.Context, c *conn, f *rigv1.Frame) {
	if f.GetKind() != rigv1.FrameKind_FRAME_KIND_REQUEST {
		// M0 speaks request/response only. Refusing rather than ignoring is
		// what makes an unimplemented kind visible instead of a hang.
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			fmt.Sprintf("M0 serves REQUEST only, got %s", f.GetKind()))
		return
	}

	program, command, ok := splitMethod(f.GetMethod())
	if !ok {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			fmt.Sprintf("method %q is not <program>.<command>", f.GetMethod()))
		return
	}

	if program == "rig" {
		d.serveSelf(c, f, command)
		return
	}
	d.route(ctx, c, f, program)
}

// serveSelf answers the methods rig implements itself.
func (d *Daemon) serveSelf(c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "hello":
		var req rigv1.HelloRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "hello: "+err.Error())
			return
		}
		if req.GetProgram() == "" {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "hello: empty program id")
			return
		}
		if req.GetProgram() == "rig" {
			// Otherwise a program shadows the daemon's own namespace and
			// rig.ping stops reaching rig.
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, `hello: "rig" is reserved`)
			return
		}

		d.mu.Lock()
		if existing, taken := d.programs[req.GetProgram()]; taken && existing != c {
			d.mu.Unlock()
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
				fmt.Sprintf("program %q is already connected", req.GetProgram()))
			return
		}
		d.programs[req.GetProgram()] = c
		d.mu.Unlock()

		c.program.Store(req.GetProgram())
		c.scoped.Store(true)
		d.log.Info("program said hello", "program", req.GetProgram(), "version", req.GetVersion())

		c.reply(f.GetStreamId(), &rigv1.HelloResponse{
			Wire:          d.wire,
			DaemonVersion: d.version,
			Scoped:        true,
		})

	case "ping":
		var req rigv1.PingRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "ping: "+err.Error())
			return
		}
		c.reply(f.GetStreamId(), &rigv1.PingResponse{
			Nonce:   req.GetNonce(),
			Program: "rig",
			Version: d.version,
		})

	default:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "no such method rig."+command)
	}
}

// route forwards a call to a program and relays its answer back.
func (d *Daemon) route(ctx context.Context, from *conn, f *rigv1.Frame, program string) {
	d.mu.RLock()
	to := d.programs[program]
	d.mu.RUnlock()
	if to == nil {
		from.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND,
			fmt.Sprintf("no program %q is connected", program))
		return
	}

	// A new even stream on the program's connection, with a slot waiting for
	// the reply before the request goes out - otherwise a fast program can
	// answer into a map that has no receiver yet.
	sid := to.nextStream.Add(2)
	ch := make(chan *rigv1.Frame, 1)
	to.pmu.Lock()
	to.pending[sid] = ch
	to.pmu.Unlock()

	cleanup := func() {
		to.pmu.Lock()
		delete(to.pending, sid)
		to.pmu.Unlock()
	}

	out := &rigv1.Frame{
		StreamId:  sid,
		Kind:      rigv1.FrameKind_FRAME_KIND_REQUEST,
		Method:    f.GetMethod(),
		RequestId: f.GetRequestId(),
		Payload:   f.GetPayload(),
	}
	if err := to.w.WriteFrame(out); err != nil {
		cleanup()
		from.fail(f.GetStreamId(), rigv1.Code_CODE_UNAVAILABLE,
			fmt.Sprintf("program %q: %v", program, err))
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	select {
	case reply := <-ch:
		relay := &rigv1.Frame{
			StreamId: f.GetStreamId(),
			Kind:     reply.GetKind(),
			Payload:  reply.GetPayload(),
			Status:   reply.GetStatus(),
		}
		if err := from.w.WriteFrame(relay); err != nil {
			d.log.Warn("could not relay reply", "program", program, "err", err)
		}
	case <-callCtx.Done():
		cleanup()
		// The deadline is rig's, not the program's: a hung program must never
		// hold a rig goroutine or the caller (section 18).
		from.fail(f.GetStreamId(), rigv1.Code_CODE_DEADLINE,
			fmt.Sprintf("program %q did not answer within %s", program, CallTimeout))
	}
}

func (c *conn) reply(stream uint32, msg proto.Message) {
	body, err := proto.Marshal(msg)
	if err != nil {
		c.fail(stream, rigv1.Code_CODE_INTERNAL, err.Error())
		return
	}
	if err := c.w.WriteFrame(&rigv1.Frame{
		StreamId: stream,
		Kind:     rigv1.FrameKind_FRAME_KIND_RESPONSE,
		Payload:  body,
	}); err != nil {
		c.log.Warn("could not write response", "err", err)
	}
}

func (c *conn) fail(stream uint32, code rigv1.Code, msg string) {
	if err := c.w.WriteFrame(&rigv1.Frame{
		StreamId: stream,
		Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
		Status:   &rigv1.Status{Code: code, Message: msg},
	}); err != nil {
		c.log.Warn("could not write error", "err", err)
	}
}

// splitMethod splits "<program>.<command>" on the FIRST dot, so a command may
// contain dots and a program id may not.
func splitMethod(m string) (program, command string, ok bool) {
	i := strings.IndexByte(m, '.')
	if i <= 0 || i == len(m)-1 {
		return "", "", false
	}
	return m[:i], m[i+1:], true
}
