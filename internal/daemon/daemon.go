// Package daemon is rigd: one socket, one instance, and the routing between a
// client and a program.
//
// The registry is NOT here. It is internal/kernel, and this package holds the
// other half of the pair: connections, and the translation between the wire
// and the kernel's vocabulary. The split is the one section 14 forces - a
// caller cannot hold a registry handle, so the daemon holds a kernel and asks
// it for a view of what the calling principal may see.
//
// What is still not here, and is M1's next slice: the invoker. A call is
// routed by name and no house rule is consulted yet (section 13a).
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
	"github.com/boris-milner/rig/internal/kernel"
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

	// Ask routes a gating question to whoever is present (section 13a).
	//
	// Nil at M1, because no surface exists to answer on yet, and nil is not a
	// hole: a gating question nobody can answer is denied. It is on Config
	// rather than reached for globally so a test can put a real answerer
	// behind it without a surface existing.
	Ask Asker

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

	// kernel owns the registry. The daemon cannot hold the registry itself:
	// noregistryhandle fails the build on the type leaving internal/kernel,
	// which is section 14's rule that an unscoped read is unrepresentable
	// rather than discouraged.
	kernel *kernel.Kernel

	// ask is how a confirm reaches a person. Nil means nobody is present.
	ask Asker

	// programs is connections, not declarations - what the kernel knows is
	// what a program said, and what this map knows is where to send a call.
	mu       sync.RWMutex
	programs map[string]*conn

	// live is every accepted connection, so shutdown can close them.
	//
	// It exists because a HEALTHY program used to block shutdown: Serve waits
	// for its handlers, a handler sits in ReadFrame until its connection
	// closes, and nothing closed the connections. SIGTERM therefore did
	// nothing while any program was connected, and section 5l's unit has no
	// ExecStop - so `systemctl --user stop rigd` would have hung until
	// systemd's SIGKILL timeout, with nothing saying why.
	cmu     sync.Mutex
	live    map[net.Conn]struct{}
	closing bool
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
		kernel:   kernel.New(),
		ask:      cfg.Ask,
		live:     make(map[net.Conn]struct{}),
		programs: make(map[string]*conn),
	}, nil
}

// track records a live connection, or refuses it because rig is stopping.
func (d *Daemon) track(nc net.Conn) bool {
	d.cmu.Lock()
	defer d.cmu.Unlock()
	if d.closing {
		return false
	}
	d.live[nc] = struct{}{}
	return true
}

func (d *Daemon) untrack(nc net.Conn) {
	d.cmu.Lock()
	defer d.cmu.Unlock()
	delete(d.live, nc)
}

// closeLive drops every connection so shutdown can complete.
//
// It does NOT drain in-flight calls. Section 18 gives every call a deadline
// and the client stub is built to survive rig going away (section 5g), so a
// call in flight fails as a dropped connection - which is a case that already
// has to work. A graceful drain, with the lifecycle notice that tells a
// program rig is stopping, is section 5g's `rig.stopping` event at M6, and
// pretending to do it here would mean waiting up to CallTimeout on every
// shutdown for a case that is already handled.
func (d *Daemon) closeLive() {
	d.cmu.Lock()
	conns := make([]net.Conn, 0, len(d.live))
	for nc := range d.live {
		conns = append(conns, nc)
	}
	d.closing = true
	d.cmu.Unlock()

	for _, nc := range conns {
		_ = nc.Close()
	}
	if len(conns) > 0 {
		d.log.Info("closed live connections on shutdown", "count", len(conns))
	}
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

	// who this connection is, decided at accept and changed only by the
	// program handshake. Read on every registry access, so it is a value in
	// an atomic rather than a struct behind a mutex.
	who atomic.Value // kernel.Principal

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

// principal is who this connection is. Never absent: handle sets it before
// the first frame is read.
func (c *conn) principal() kernel.Principal {
	p, _ := c.who.Load().(kernel.Principal)
	return p
}

// Serve accepts until the listener closes or ctx ends.
func (d *Daemon) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		// The listener first, so nothing new arrives, then every live
		// connection, so the handlers waiting on ReadFrame return and the
		// wait below can finish.
		_ = l.Close()
		d.closeLive()
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
		if !d.track(nc) {
			// Accepted in the same instant as the shutdown. Closing it here
			// beats handing it to a handler that is about to be torn down.
			_ = nc.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer d.untrack(nc)
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
	c.who.Store(newPrincipal(nc))
	defer func() {
		_ = c.w.Close()
		// The registry forgets by session, so a restarted program can take
		// its own id back. The connection map forgets by name, and only if
		// this connection is still the one holding it.
		d.kernel.Deregister(c.principal().SessionID)
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
	d.route(ctx, c, f, program, command)
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

		// One handshake per connection.
		//
		// Section 14 makes registration a property of the connection, and a
		// second hello was worse than merely odd: with a different id it
		// registered a second program on the same session, and only the last
		// id was removed from the routing map on close - leaving a name that
		// pointed at a dead connection and answered every call with a
		// timeout.
		if c.scoped.Load() {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
				fmt.Sprintf("this connection is already registered as %q: "+
					"registration is the handshake and happens once, so a "+
					"changed declaration is a reconnect", c.name()))
			return
		}

		// The declaration is refused before anything is recorded, and the
		// refusal names every missing property at once - a program author
		// fixing a generated declaration one error per run is a program
		// author who stops generating it.
		decl, err := declarationFromWire(&req)
		if err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, err.Error())
			return
		}

		who, err := d.kernel.Register(asProgram(c.principal(), decl.Identity.ID), decl)
		if err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, err.Error())
			return
		}

		d.mu.Lock()
		if existing, taken := d.programs[req.GetProgram()]; taken && existing != c {
			d.mu.Unlock()
			d.kernel.Deregister(who.SessionID)
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
				fmt.Sprintf("program %q is already connected", req.GetProgram()))
			return
		}
		d.programs[req.GetProgram()] = c
		d.mu.Unlock()

		c.who.Store(who)
		c.program.Store(req.GetProgram())
		c.scoped.Store(true)
		d.log.Info("program registered",
			"program", decl.Identity.ID, "version", decl.Identity.Version,
			"coverage", decl.Coverage.String(), "commands", len(decl.Commands),
			"semantics_gen", decl.SemanticsGen, "session", who.SessionID)

		c.reply(f.GetStreamId(), &rigv1.HelloResponse{
			Wire:          d.wire,
			DaemonVersion: d.version,
			Scoped:        true,
		})

	case "programs":
		// The read side of the registry, through the calling principal's own
		// view. There is no unscoped read to offer: See takes a principal and
		// the filter is inside it.
		var resp rigv1.ProgramsResponse
		for _, p := range d.kernel.See(c.principal()).Programs() {
			resp.Programs = append(resp.Programs, programToWire(p))
		}
		c.reply(f.GetStreamId(), &resp)

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
func (d *Daemon) route(ctx context.Context, from *conn, f *rigv1.Frame, program, command string) {
	d.mu.RLock()
	to := d.programs[program]
	d.mu.RUnlock()
	if to == nil {
		from.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND,
			fmt.Sprintf("no program %q is connected", program))
		return
	}

	// The authorization floor, and it is here rather than on any surface so
	// that no surface can forget it (section 13a).
	//
	// Note what the line above does NOT do: it looks the program up in the
	// routing map, which is every connected program, not in the caller's
	// scoped view. So a caller reaches a program it could not list, and
	// section 14's "a principal sees the programs it may reach" is not true
	// of routing yet. That gap is older than the invoker and closing it
	// changes what one program may ask another for, so the invoker does not
	// paper over it: it matches on what the target declared and leaves the
	// visibility question where it belongs.
	dec, allowed, err := d.authorize(ctx, from, f, program, command)
	if err != nil {
		from.fail(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err.Error())
		return
	}
	if !allowed {
		from.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, dec.Reason)
		return
	}

	// And only then, what was sent. A refused caller learns nothing about the
	// schema it would have had to satisfy.
	if !d.validateArgs(from, f, program, command) {
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
