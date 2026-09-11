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
	"os"
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

	// stop ends Serve, and it is how rig.down reaches the shutdown that until
	// now only a signal could start.
	//
	// It is set by Serve rather than by New because the context to cancel is
	// Serve's argument, and a Daemon that was never served has nothing to
	// stop. Nil until then, and rig.down says so rather than panicking: a
	// daemon under test that is dispatched to directly is a real case
	// (TestSliceFourDemo builds one), and "no listener is running" is a
	// truthful answer to "stop the listener".
	stopMu sync.Mutex
	stop   context.CancelFunc
}

// replyThenStop answers rig.down and only then ends Serve. The ordering is the
// whole function, and it is a function so that the ordering is structural.
//
// Cancelling runs closeLive(), which closes every live connection INCLUDING
// the caller's. Stop before replying and the caller races the teardown for its
// own answer: it usually wins, and when it loses it sees a dropped connection,
// which is indistinguishable from the daemon having crashed mid-call. rig would
// still have stopped, so the defect would present as cosmetic while actually
// lying about whether rig agreed to stop or died.
//
// A `defer` rather than two adjacent statements BECAUSE NO TEST CATCHES THIS.
// It is a race, not a deterministic failure: a mutation swapping the two
// statements passed the whole suite, because the reply almost always wins.
// A test that catches a race one run in ten is worse than no test, so the
// ordering is enforced by construction instead - reversing it now means
// deleting a defer whose comment says not to, rather than moving a line.
func (d *Daemon) replyThenStop(c *conn, streamID uint32, stop context.CancelFunc) {
	defer stop()
	c.reply(streamID, &rigv1.DownResponse{
		// A Linux pid is bounded by /proc/sys/kernel/pid_max, whose own
		// ceiling is 2^22 (PID_MAX_LIMIT), so it cannot overflow int32 - and
		// the proto field is int32 for that same reason rather than by
		// accident. Kept as a directive with the bound written down rather
		// than a runtime check, because a check here could only report an
		// impossible state at the moment rig is shutting down.
		//nolint:gosec // pid_max <= 2^22, so int -> int32 cannot overflow
		Pid:     int32(os.Getpid()),
		Version: d.version,
	})
}

// setStop records how to end Serve. Separate from Serve only so the locking is
// in one place rather than inline in a function that already has a goroutine
// and a WaitGroup in it.
func (d *Daemon) setStop(cancel context.CancelFunc) {
	d.stopMu.Lock()
	defer d.stopMu.Unlock()
	d.stop = cancel
}

// stopper returns the cancel func, or nil if Serve is not running.
func (d *Daemon) stopper() context.CancelFunc {
	d.stopMu.Lock()
	defer d.stopMu.Unlock()
	return d.stop
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
	k := kernel.New()
	if err := declareSelf(k); err != nil {
		return nil, fmt.Errorf("daemon: rig could not declare its own commands: %w", err)
	}
	return &Daemon{
		version:  cfg.Version,
		wire:     cfg.Wire,
		log:      log,
		lock:     cfg.Lock,
		kernel:   k,
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
	// Derived so rig.down can end Serve the same way SIGTERM does, rather
	// than through a second shutdown path that would have to be kept in step
	// with this one. Cancelling the derived context runs exactly the sequence
	// below, which is the sequence a signal already runs.
	//
	// The deadlines that matter are per call (section 18), on contexts derived
	// from this one. A deadline HERE would be an expiry date on serving at all.
	//rig:allow nocontextfree: this context bounds the daemon's whole serving life, so being unbounded is the requirement rather than an oversight
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.setStop(cancel)

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

	if program == kernel.SelfID {
		d.serveSelf(ctx, c, f, command)
		return
	}
	d.route(ctx, c, f, program, command)
}

// serveSelf answers the methods rig implements itself.
func (d *Daemon) serveSelf(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	// rig's own methods meet the same floor as everything else, because
	// section 13a's placement argument - "no surface can forget it" - is
	// about surfaces, and this is one. It used to be the single surface that
	// never reached the rules table.
	//
	// hello is the exception and it is not an exemption: it is the handshake
	// that MINTS the principal, so there is no (caller, effects) pair to
	// match before it. `caller` is what hello establishes. Authorising it
	// would mean asking who is calling before there is an answer.
	if command != "hello" {
		dec, allowed, err := d.authorize(ctx, callerOf(c, f), kernel.SelfID, command)
		if err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err.Error())
			return
		}
		if !allowed {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, dec.Reason)
			return
		}
	}

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
		if req.GetProgram() == kernel.SelfID {
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
		//
		// A malformed payload is NOT refused here, deliberately: this method
		// took an empty request before it took a depth, so a caller that
		// sends nothing at all is an old caller rather than a broken one, and
		// depthIn turns its zero into the estate it used to get.
		var req rigv1.ProgramsRequest
		_ = proto.Unmarshal(f.GetPayload(), &req)

		// The depth decides how much is said about each program and never
		// which programs are named: the scope filter is inside See and runs
		// first either way.
		estate, err := d.kernel.See(c.principal()).Estate(depthIn(req.GetDepth()))
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INVALID, err)
			return
		}
		var resp rigv1.ProgramsResponse
		for _, p := range estate {
			resp.Programs = append(resp.Programs, programToWire(p))
		}
		c.reply(f.GetStreamId(), &resp)

	case "ping":
		var req rigv1.PingRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "ping: "+err.Error())
			return
		}

		// A probe naming another program is rig probing THAT program, and it
		// still reaches the program as <program>.ping - what moved is the
		// method the caller sends, not the one the program answers. The frame
		// is rebuilt rather than forwarded, because route copies the method
		// verbatim and the program must not be handed rig's own method name.
		//
		// It goes through route, so it passes the same authorization floor as
		// any other call rather than round-tripping behind it.
		if target := req.GetProgram(); target != "" && target != kernel.SelfID {
			d.route(ctx, c, &rigv1.Frame{
				StreamId:  f.GetStreamId(),
				Kind:      f.GetKind(),
				Method:    target + "." + ProbeCommand,
				RequestId: f.GetRequestId(),
				Payload:   f.GetPayload(),
			}, target, ProbeCommand)
			return
		}

		c.reply(f.GetStreamId(), &rigv1.PingResponse{
			Nonce:   req.GetNonce(),
			Program: kernel.SelfID,
			Version: d.version,
		})

	case "down":
		// Authorised above like everything else, and it is declared
		// destructive in self.go, so a house rule denying destructive calls
		// refuses this one before it reaches here.
		stop := d.stopper()
		if stop == nil {
			// Serve is not running, so there is no listener to stop. Only
			// reachable for a Daemon dispatched to directly, which tests do.
			c.fail(f.GetStreamId(), rigv1.Code_CODE_UNAVAILABLE,
				"rig.down: this daemon is not serving a listener, so there is nothing to stop")
			return
		}

		d.log.Info("stopping on rig.down", "pid", os.Getpid())
		d.replyThenStop(c, f.GetStreamId(), stop)

	default:
		c.fail(f.GetStreamId(), rigv1.Code_CODE_NOT_FOUND, "no such method rig."+command)
	}
}

// callFailure is a refusal the boundary produced, in the one form both
// surfaces need.
//
// The wire path turns it into an ERROR frame; the in-process path turns it
// into an error return. It holds the whole Status rather than a code and a
// sentence so section 9's four fields - the failed precondition, the state
// actually found, the fix, and the exact command that fixes it - survive to
// either one. A refusal that reaches only one surface is section 35's defect:
// acceptance is not a contract, retrievability is.
type callFailure struct {
	status *rigv1.Status

	// err is the original refusal where there was one, so the in-process
	// surface returns what the kernel actually raised rather than a sentence
	// rebuilt out of a Status.
	err error
}

func failure(code rigv1.Code, msg string) *callFailure {
	return &callFailure{status: &rigv1.Status{Code: code, Message: msg}}
}

// failureErr is failure for a refusal that may know more than its sentence,
// and it is failErr's logic with the connection taken out.
func failureErr(code rigv1.Code, err error) *callFailure {
	st := &rigv1.Status{Code: code, Message: err.Error()}
	if f, ok := kernel.AsRefusal(err); ok {
		st.Precondition = f.Precondition
		st.Actual = f.Actual
		st.Fix = f.Fix
		st.FixCommand = f.FixCommand
	}
	return &callFailure{status: st, err: err}
}

// callerOf reads the floor's inputs off a connection and the frame it sent.
func callerOf(c *conn, f *rigv1.Frame) caller {
	return caller{
		who:       c.principal(),
		method:    f.GetMethod(),
		args:      f.GetPayload(),
		requestID: f.GetRequestId(),
	}
}

// call is THE authorisation path, and every surface that invokes a declared
// command goes down this one.
//
// It is synchronous and returns the program's reply, so a surface holding no
// connection - the MCP server here at M2, the HTTP routes at slice 6 - meets
// the same floor as the wire instead of needing one of its own. Section 13a:
// "a new surface SPLITS the existing path and reuses its floor; it never grows
// a second one. Two floors is how they diverge, and the one that diverges is
// whichever was added last."
//
// It was extracted from route rather than written beside it, deliberately.
// Writing it beside route would have produced the second floor that rule
// forbids, and it would have looked correct: the same four checks in the same
// order, drifting the first time one of them changed.
func (d *Daemon) call(
	ctx context.Context, from caller, program, command string,
) (*rigv1.Frame, *callFailure) {
	// Section 14, and this is the check routing did not have: a principal
	// reaches only what it may see. Without it a program could invoke a
	// program it could not list, because route looked the target up in the
	// connected-programs map rather than in the caller's own view.
	//
	// The refusal is MISSING, not refused, and the message is byte-identical
	// to the not-connected one below on purpose. Section 14: "a program it may
	// not see is reported missing rather than refused, because 'you may not
	// see shelf' tells the caller that shelf exists." A distinguishable
	// refusal here would be an existence oracle for every program on the box.
	//
	// A terminal is unaffected. Introspect starts true and only hello turns it
	// off, so the CLI and the window still reach everything; what this
	// restricts is one PROGRAM calling another, which today means a program
	// reaches itself and nothing else.
	if _, visible := d.kernel.See(from.who).Program(program); !visible {
		return nil, failure(rigv1.Code_CODE_NOT_FOUND,
			fmt.Sprintf("no program %q is connected", program))
	}

	d.mu.RLock()
	to := d.programs[program]
	d.mu.RUnlock()
	if to == nil {
		return nil, failure(rigv1.Code_CODE_NOT_FOUND,
			fmt.Sprintf("no program %q is connected", program))
	}

	// The authorization floor, and it is here rather than on any surface so
	// that no surface can forget it (section 13a).
	//
	// The visibility half is decided above, before the routing map is even
	// read, so section 14's "a principal sees the programs it may reach" is
	// now true of routing. The invoker still matches on what the TARGET
	// declared rather than on the caller's view, and closing gap 1 did not
	// make that redundant - see Kernel.pair for what it decides for a caller
	// that can reach its target.
	dec, allowed, err := d.authorize(ctx, from, program, command)
	if err != nil {
		return nil, failure(rigv1.Code_CODE_INTERNAL, err.Error())
	}
	if !allowed {
		return nil, failure(rigv1.Code_CODE_DENIED, dec.Reason)
	}

	// And only then, what was sent. A refused caller learns nothing about the
	// schema it would have had to satisfy.
	if err := d.validateArgs(from, program, command); err != nil {
		return nil, failureErr(rigv1.Code_CODE_INVALID, err)
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
		Method:    from.method,
		RequestId: from.requestID,
		Payload:   from.args,
	}
	if err := to.w.WriteFrame(out); err != nil {
		cleanup()
		return nil, failure(rigv1.Code_CODE_UNAVAILABLE,
			fmt.Sprintf("program %q: %v", program, err))
	}

	callCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	select {
	case reply := <-ch:
		return reply, nil
	case <-callCtx.Done():
		cleanup()
		// The deadline is rig's, not the program's: a hung program must never
		// hold a rig goroutine or the caller (section 18).
		return nil, failure(rigv1.Code_CODE_DEADLINE,
			fmt.Sprintf("program %q did not answer within %s", program, CallTimeout))
	}
}

// route forwards a call to a program and relays its answer back.
//
// Everything it used to decide now lives in call, so this is a renderer: it
// turns one refusal into an ERROR frame and one reply into a RESPONSE. The
// wire cannot drift from the in-process path because there is nothing left
// here to drift.
func (d *Daemon) route(ctx context.Context, from *conn, f *rigv1.Frame, program, command string) {
	reply, bad := d.call(ctx, callerOf(from, f), program, command)
	if bad != nil {
		from.failStatus(f.GetStreamId(), bad.status)
		return
	}
	relay := &rigv1.Frame{
		StreamId: f.GetStreamId(),
		Kind:     reply.GetKind(),
		Payload:  reply.GetPayload(),
		Status:   reply.GetStatus(),
	}
	if err := from.w.WriteFrame(relay); err != nil {
		d.log.Warn("could not relay reply", "program", program, "err", err)
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
	c.failStatus(stream, &rigv1.Status{Code: code, Message: msg})
}

// failErr is fail for a refusal that may know more than its sentence.
//
// A kernel.RefusalError carries what section 9 asks a failed call for - the
// precondition, the state actually found, and the fix including its exact
// command - and this is the one place those reach the wire. An error that is
// not a RefusalError produces exactly the frame fail would have: the structure is an
// addition, never a condition of reporting a failure.
func (c *conn) failErr(stream uint32, code rigv1.Code, err error) {
	st := &rigv1.Status{Code: code, Message: err.Error()}
	if f, ok := kernel.AsRefusal(err); ok {
		st.Precondition = f.Precondition
		st.Actual = f.Actual
		st.Fix = f.Fix
		st.FixCommand = f.FixCommand
	}
	c.failStatus(stream, st)
}

func (c *conn) failStatus(stream uint32, st *rigv1.Status) {
	if err := c.w.WriteFrame(&rigv1.Frame{
		StreamId: stream,
		Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
		Status:   st,
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
