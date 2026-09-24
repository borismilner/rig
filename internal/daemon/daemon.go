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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/mcpserver"
	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/wire"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
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

	// Estate is the name this estate claimed, empty when it claimed none
	// (PLAN.md section 37).
	//
	// It is the NAME only. The role is derived from it at reply time and is
	// never stored, so there is no second place for the two to disagree from.
	Estate string

	// Epoch is the number the estate's state published when it was opened,
	// bumped unconditionally on every start (section 37, precondition 4).
	//
	// It is passed in rather than read here because the daemon does not open
	// the store: rigd opens it under the name claim, before anything binds,
	// and a daemon that reached for it a second time would be a second opener
	// of a file that must have exactly one.
	//
	// Zero is correct for an unnamed estate, which opens no store and
	// therefore has no epoch. A real epoch is always at least 1 because the
	// store bumps before it publishes.
	Epoch uint64

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

	// Leases is the estate's lease store, which rigd opened under the name
	// claim for the epoch above. Passed in for the same reason the epoch is:
	// the file must have exactly one opener. Nil for an unnamed estate, and
	// the lease verbs then refuse naming the cause.
	Leases *coord.Store
}

// Daemon serves one socket.
type Daemon struct {
	// toasts wakes a renderer when rig.notify is called (toast.go).
	toasts toastRing

	version string
	wire    string
	estate  string
	epoch   uint64
	log     *slog.Logger
	lock    *instance.Lock

	// kernel owns the registry. The daemon cannot hold the registry itself:
	// noregistryhandle fails the build on the type leaving internal/kernel,
	// which is section 14's rule that an unscoped read is unrepresentable
	// rather than discouraged.
	kernel *kernel.Kernel

	// ask is how a confirm reaches a person. Nil means nobody is present.
	ask Asker

	// mcps is every live MCP server, one per connected agent, so a promoted
	// tool list tracks the estate rather than the instant an agent connected.
	mmu  sync.Mutex
	mcps map[*mcpserver.Server]struct{}

	// programs is connections, not declarations - what the kernel knows is
	// what a program said, and what this map knows is where to send a call.
	mu       sync.RWMutex
	programs map[string]*conn

	// presence is the estate's roster (section 16, section 37 PART B row 1).
	// It is connection state and holds nothing durable, which is why it could
	// be built before the write-ahead log exists.
	presence *presence

	// records is section 39's continuity record, and it is the first DURABLE
	// thing this daemon holds.
	//
	// NIL IS A REAL AND CORRECT STATE, not a hole. record.Open takes the
	// estate NAME and refuses without one, the same way section 37 gives an
	// unnamed estate no state directory - so every test daemon, and every
	// unnamed estate, runs with this nil and the record verbs refuse in terms
	// that name the cause. recordStore in record.go is the one reader.
	records *record.Store

	// leases is section 16's lease store. Nil for an unnamed estate.
	leases *coord.Store

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
	c.reply(streamID, &verbsv1.DownResponse{
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

	// THE RECORD STORE IS OPENED HERE AND NOT BY rigd, WHICH IS THE OPPOSITE
	// OF WHAT Config's Epoch COMMENT DOES - and the difference is the reason.
	// rigd opens the ESTATE STATE store "under the name claim, before anything
	// binds", because the epoch has to exist before a daemon can answer with
	// it. The record store has no such ordering: it needs the estate NAME,
	// which is already on Config, and nothing answers from it until a call
	// arrives. Opening it here keeps the lifetime with the thing that serves
	// it rather than splitting one store across two files.
	//
	// AN UNNAMED ESTATE NOW GETS AN EPHEMERAL STORE RATHER THAN NONE, AND THAT
	// CHANGED 2026-09-17.
	//
	// ⛔ IT USED TO CARRY A NIL STORE AND REFUSE EVERY RECORD VERB, which was
	// a reasoned position - section 37 gives an unnamed estate no persistent
	// state - and had a cost nobody had counted. Section 09's A0 survey measured
	// it: a third estate NAME is refused, an unnamed estate had no store, and a
	// second daemon on a named estate is B72, so every door was shut and every
	// write an agent made landed in one of the two estates on the human's
	// screen. Four lead generations wrote nothing for that survey, and the
	// reason was that the instrument did not exist.
	//
	// ⛔ SECTION 37's RULE IS INTACT: the scratch store lives in the RUNTIME
	// directory, which the operating system clears, so nothing persists across
	// runs and no third estate arrives by the back door. What it buys is a
	// sandbox an agent can be pointed at with one environment variable.
	//
	// Any failure is real and stops the daemon, because a store that half-opened
	// is worse than none - and that was true of the named path before this and
	// is true of both now.
	var records *record.Store
	if cfg.Estate != "" {
		st, err := record.Open(cfg.Estate)
		if err != nil {
			var unnamed *record.UnnamedEstateError
			if !errors.As(err, &unnamed) {
				return nil, fmt.Errorf("daemon: opening the record store: %w", err)
			}
		} else {
			records = st
		}
	} else {
		st, err := record.OpenScratch()
		if err != nil {
			return nil, fmt.Errorf("daemon: opening the scratch record store: %w", err)
		}
		records = st
	}

	return &Daemon{
		version:  cfg.Version,
		wire:     cfg.Wire,
		estate:   cfg.Estate,
		epoch:    cfg.Epoch,
		log:      log,
		lock:     cfg.Lock,
		kernel:   k,
		ask:      cfg.Ask,
		live:     make(map[net.Conn]struct{}),
		programs: make(map[string]*conn),
		presence: newPresence(cfg.Estate, cfg.Epoch),
		records:  records,
		leases:   cfg.Leases,
	}, nil
}

// Close releases what New opened: the record store. Call it after Serve and
// ServeMCP have both returned, because either can still be answering from
// the store until then. The lease store is not closed here: rigd opened it
// and rigd closes it. Safe to call on a daemon with no store.
func (d *Daemon) Close() error {
	if d.records == nil {
		return nil
	}
	return d.records.Close()
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

	// occ is this connection's roster identity, allocated once at accept and
	// released by the defer below. The MCP door allocates its own; presence
	// keys on the token rather than on this struct precisely so it can.
	occ *occupancy

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
		occ:     &occupancy{},
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
		// The seat empties with the connection. This is the whole expiry
		// mechanism for presence: no TTL, no reaper, no orphan state.
		d.presence.leave(c.occ)
		d.resyncMCP(ctx)
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

// serveSession answers rig.session.
//
// IT IS ITS OWN METHOD BECAUSE serveSelf CROSSED gocyclo's CEILING WHEN THIS
// CASE WAS INLINE, and that ceiling is doing its job rather than obstructing:
// a dispatch switch that grows a branch per method is exactly the function
// that becomes unreadable one method at a time. Extracting the body is the
// cheap half of the fix and it is the whole fix here.
func (d *Daemon) serveSession(c *conn, f *rigv1.Frame) {
	// SECTION 5f's SESSION TOKEN, AND THE WALK OF SECTION 14's SIX CALLER
	// ROWS IS PART OF THIS CHANGE RATHER THAN A REVIEW COMMENT.
	//
	//   1-3. An agent, a terminal, a script. Connected, unregistered, and
	//        none of them receives a HelloResponse. THIS METHOD IS FOR
	//        THESE THREE ROWS - it is the only message they have on which
	//        to receive a token, which is why a field on HelloResponse
	//        could not have been the only carrier.
	//   4.   A registered program. Already given its token by hello, and
	//        may still call this to resume one. Calling it does not
	//        change `scoped`, so registration remains the only way to
	//        become a scoped caller.
	//   5.   An HTTP client. It has no unix peer and cannot reach this
	//        socket at all today. THE HAZARD IS A BRIDGE, and it is named
	//        here so slice 6 meets it as a constraint rather than
	//        rediscovering it: a token minted for a unix caller must
	//        never be honoured for a bearer principal, or the deliberately
	//        narrower HTTP scopes are laundered onto this socket.
	//   6.   A scheduled fire, a house rule, an internal timer. IT HAS NO
	//        CONNECTION, so it has no token - `Token` is empty for rig's
	//        own principal by construction, not by exemption. An empty
	//        resume from it therefore mints nothing and it can present
	//        nothing. That is why the empty check below is explicit: a
	//        caller with no token must be refused rather than handed a
	//        session it could never have owned.
	var req verbsv1.SessionRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
			"rig.session: "+err.Error())
		return
	}

	if req.GetResume() != "" {
		// EVERY RESUME ANSWERS SESSION_DEAD AT M6, AND THAT IS CORRECT
		// RATHER THAN UNIMPLEMENTED. A session holds nothing until M7:
		// leases, claims and subscriptions are M7's content, and section
		// 5f puts the dedup window "persisted in the WAL", which is M7
		// too. So there is genuinely nothing this daemon could restore,
		// and SESSION_DEAD is the true answer to "did you keep my state".
		//
		// THE ALTERNATIVE WAS BUILT AND DELETED, AND THE REASON IS THE
		// WHOLE ARGUMENT. A table of minted tokens would let this reply
		// `resumed: true` - which would be rig claiming it restored a
		// session when it restored nothing, because there was nothing to
		// restore. A false claim the caller then plans against is worse
		// than a refusal it can act on, and this estate has spent the day
		// finding present-tense comments describing machinery that does
		// not exist. A `resumed: true` here would have become the next
		// one.
		//
		// A resume that cannot be honoured is also never a quiet fresh
		// mint: a caller that asked to resume and got a NEW session would
		// believe it kept state it had lost, which is what section 5f's
		// "Silence is not an answer either way" is about.
		c.fail(f.GetStreamId(), rigv1.Code_CODE_SESSION_DEAD,
			"rig.session: this daemon holds no resumable session state "+
				"yet, so nothing can be resumed. Coordination state - "+
				"leases, claims, subscriptions - arrives with the peers "+
				"service, and its durability with the write-ahead log. "+
				"Your token is still valid to carry; there is simply "+
				"nothing behind it to restore")
		return
	}

	me := c.principal()
	if me.Token == "" {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED,
			"rig.session: this caller has no connection, so it has no "+
				"session to name (section 14, row 6)")
		return
	}

	// READING THE TOKEN THIS CONNECTION ALREADY HAS, WHICH IS WHY THIS IS
	// DECLARED read-only AND idempotent. It is minted at accept for every
	// connection, so asking twice is the same answer and never a second
	// session.
	c.reply(f.GetStreamId(), &verbsv1.SessionResponse{
		Session: me.Token,
		Resumed: false,
	})
}

// authorizeSelf runs one of rig's own methods past the rules table and answers
// the refusal itself, so serveSelf's switch reads as the methods and nothing
// else. It reports whether the call may proceed.
func (d *Daemon) authorizeSelf(ctx context.Context, c *conn, f *rigv1.Frame, command string) bool {
	dec, allowed, err := d.authorize(ctx, callerOf(c, f), kernel.SelfID, command)
	if err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INTERNAL, err.Error())
		return false
	}
	if !allowed {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_DENIED, dec.Reason)
		return false
	}
	return true
}

// servedMethods is every rig.* method this daemon serves, sorted: rig's own
// declaration plus rig.hello, which is prior to it (self.go says why hello is
// not declared). HelloResponse.methods carries it.
func servedMethods() []string {
	cmds := selfDeclaration().Commands
	out := make([]string, 0, len(cmds)+1)
	out = append(out, kernel.SelfID+".hello")
	for _, c := range cmds {
		out = append(out, kernel.SelfID+"."+c.ID)
	}
	sort.Strings(out)
	return out
}

// serveHello is the program handshake: it registers the connection as one
// program and mints that principal. Split out of serveSelf only for length.
func (d *Daemon) serveHello(ctx context.Context, c *conn, f *rigv1.Frame) {
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
		// ROLLBACK, NOT DEREGISTER, AND THE DIFFERENCE IS A TOMBSTONE.
		// Nothing departed here: the registration was undone in the same
		// breath it was made, because another connection already holds the
		// name. Deregister would leave a departure record saying this program
		// was here and left - for a program that never answered a call - and
		// the next caller asking about that name would be told a program died
		// when a duplicate was refused. The connection-closed path above is
		// the real departure and keeps Deregister.
		d.kernel.Rollback(who.SessionID)
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
		Methods:       servedMethods(),

		// Section 14 row 4 - a registered program - is the one caller
		// kind with a handshake, so it is the one that never has to ask
		// for its token. The other three connected rows have no
		// HelloResponse to receive one on, which is what rig.session is
		// for. Delivering it here is not a second way to become a caller
		// kind: `Scoped` above is still the whole authorisation state,
		// and this restores coordination state only.
		Session: who.Token,
	})

	// The program is registered, so any agent already connected gains its
	// promoted tools without reconnecting. After the reply, not before: a
	// registration must not wait on a projection of itself.
	d.resyncMCP(ctx)
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
	if command != "hello" && !d.authorizeSelf(ctx, c, f, command) {
		return
	}

	switch command {
	case "hello":
		d.serveHello(ctx, c, f)

	case "programs":
		// The read side of the registry, through the calling principal's own
		// view. There is no unscoped read to offer: See takes a principal and
		// the filter is inside it.
		//
		// A malformed payload is NOT refused here, deliberately: this method
		// took an empty request before it took a depth, so a caller that
		// sends nothing at all is an old caller rather than a broken one, and
		// depthIn turns its zero into the estate it used to get.
		var req registryv1.ProgramsRequest
		_ = proto.Unmarshal(f.GetPayload(), &req)

		// The depth decides how much is said about each program and never
		// which programs are named: the scope filter is inside See and runs
		// first either way.
		estate, err := d.kernel.See(c.principal()).Estate(depthIn(req.GetDepth()))
		if err != nil {
			c.failErr(f.GetStreamId(), rigv1.Code_CODE_INVALID, err)
			return
		}
		var resp registryv1.ProgramsResponse
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

	case "estate":
		// UNSCOPED, and it is the only rig method whose answer does not depend
		// on who is asking (section 14, walked against all six caller rows).
		// Every field here is a fact about this daemon; none is data belonging
		// to another principal, so there is nothing to filter. A later field
		// could quietly destroy that, which is why adding one means walking
		// the table again.
		c.reply(f.GetStreamId(), &registryv1.EstateResponse{
			Name:          d.estate,
			Role:          estateRole(d.estate),
			DaemonVersion: d.version,
			Wire:          d.wire,
			SemanticsGen:  selfDeclaration().SemanticsGen,

			// Added here and on meta.Estate in the SAME change, which is the
			// rule stated at EstateIdentity in meta.go: two surfaces
			// answering "which rig is this" differently is the failure that
			// answer exists to prevent, and the epoch is the field where the
			// divergence would be worst - an agent on one surface knowing it
			// was restarted under while an agent on the other does not, same
			// daemon, same instant.
			Epoch: d.epoch,
		})

	case "announce":
		d.serveAnnounce(c, f)

	case "activity":
		d.serveActivity(c, f)

	case "peers":
		d.servePeers(c, f)

	case "describe":
		d.serveDescribe(ctx, c, f)

	case "session":
		d.serveSession(c, f)

	// SECTION 39's RECORD VERBS, all eight through one arm.
	//
	// They are grouped rather than listed because this switch is the function
	// gocyclo already caught once: serveSession was extracted for crossing the
	// ceiling with ONE inline case, and eight more listed here would bury the
	// dispatch it lives in. The inner switch is record.go's, beside the
	// handlers it chooses between.
	//
	// `record.refs` JOINED THEM 2026-09-16 late. Its reverse lookup is built
	// (internal/record/refs.go), and its wire messages were grown to carry the
	// three fields the store computes and they did not - record.go says why.
	// ⛔ `record.retract`, `record.delete` AND `record.replace` JOINED THEM
	// 2026-09-17. B77, ruled by Boris: "everybody can delete/retract records
	// and replace records". Three capabilities, not three names for one.
	case "record.put", "record.get", "record.query", "record.history",
		"record.link", "record.unlink", "record.refs",
		"record.retract", "record.delete", "record.replace",
		"progress.step", "project.brief",
		"knowledge.add", "knowledge.search", "knowledge.get":
		d.serveRecord(ctx, c, f, command)

	// SECTION 16's LEASES, all five through one arm. lease.go has why the
	// holder and the witness come off the connection.
	// SECTION 12's TOASTS. toast.go has why the daemon writes the record.
	case "notify":
		d.serveNotify(ctx, c, f)
	case "toast.wait":
		d.serveToastWait(ctx, c, f)

	case "lease.list", "lease.acquire", "lease.renew", "lease.release", "lease.break",
		"lease.check":
		d.serveLease(c, f, command)

	// SECTION 16's QUEUES. A claim is a lease, so the store is the lease
	// store; queue.go has why the claimer comes off the connection.
	case "queue.push", "queue.claim", "queue.complete", "queue.list":
		d.serveQueue(c, f, command)

	case "backup.create":
		// Its own arm rather than a member of the group above: the record
		// verbs all take a store and read or write rows in it, and this one
		// writes a FILE. Section 46's decision 11 refusal also runs after
		// recordStore rather than inside it, which the grouped arm cannot
		// express.
		d.serveBackupCreate(ctx, c, f)

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

// error renders the refusal for a caller that has no frame to receive one.
func (f *callFailure) error() error {
	if f.err != nil {
		return f.err
	}
	return errors.New(f.status.GetMessage())
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
	from.send(relay, "could not relay reply", "program", program)
}

func (c *conn) reply(stream uint32, msg proto.Message) {
	body, err := proto.Marshal(msg)
	if err != nil {
		c.fail(stream, rigv1.Code_CODE_INTERNAL, err.Error())
		return
	}
	c.send(&rigv1.Frame{
		StreamId: stream,
		Kind:     rigv1.FrameKind_FRAME_KIND_RESPONSE,
		Payload:  body,
	}, "could not write response")
}

// send writes an answer frame, and an answer too large for the wire is
// REFUSED TO THE CALLER rather than only logged.
//
// ⛔ A LOGGED FAILURE HERE IS A SILENT ONE AT THE OTHER END. WriteFrame
// refuses a frame over wire.MaxFrameSize before writing a byte, so the
// connection is intact - but the caller was never sent anything and waits
// out its whole deadline, then reports a timeout about a call that finished.
// The error frame names the size and the cap, so the caller learns the
// answer exists and has to be asked for in smaller pieces.
func (c *conn) send(f *rigv1.Frame, warn string, attrs ...any) {
	err := c.w.WriteFrame(f)
	if err == nil {
		return
	}
	c.log.Warn(warn, append(attrs, "err", err)...)
	if errors.Is(err, wire.ErrFrameTooLarge) {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, fmt.Sprintf(
			"the answer is too large for one frame (%v, cap %d bytes); ask for "+
				"less per call - a page, a limit, or fewer ids", err, wire.MaxFrameSize))
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
