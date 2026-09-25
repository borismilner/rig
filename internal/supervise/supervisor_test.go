package supervise

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// The supervisor is tested against a FAKE CLOCK AND A FAKE LAUNCHER, and the
// two are the whole reason these tests assert anything. A restart budget is a
// statement about time: with a real clock the only honest test of "five
// restarts, doubling from one second, then quarantine" takes 31 seconds and
// every shortcut around that ends up asserting the shortcut. Here the same
// walk runs in microseconds and every step is checked.

type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func newClock() *fakeClock {
	return &fakeClock{at: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// fakeProc is a child that never existed. Stop closes its exit channel, which
// is what a real child's Wait does when it leaves, so the goroutine watching
// it returns and the leak check stays green.
type fakeProc struct {
	pid  int
	exit chan Exit
	once sync.Once

	mu      sync.Mutex
	stopped bool
}

func (f *fakeProc) PID() int { return f.pid }

func (f *fakeProc) Stop(time.Duration) {
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
	// As a real child does: asked to leave, it EXITS, by a signal, and that
	// exit arrives on the watching goroutine after rig has moved on. A fake
	// that only closed the channel hid the restart bug TestChaos found.
	f.once.Do(func() {
		f.exit <- Exit{Signal: "SIGTERM"}
		close(f.exit)
	})
}

func (f *fakeProc) wasStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}

type harness struct {
	t   *testing.T
	sup *Supervisor
	clk *fakeClock

	mu      sync.Mutex
	procs   map[string]*fakeProc
	starts  map[string]int
	refuse  map[string]error
	handles []map[string]string
}

func newHarness(t *testing.T, specs ...Spec) *harness {
	t.Helper()
	h := &harness{
		t: t, clk: newClock(),
		procs: map[string]*fakeProc{}, starts: map[string]int{}, refuse: map[string]error{},
	}
	h.sup = New(Options{
		Now:     h.clk.now,
		Handles: map[string]string{"RIG_SOCKET": "/run/rig.sock"},
		Start: func(spec Spec, handles map[string]string) (Process, <-chan Exit, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			if err := h.refuse[spec.ID]; err != nil {
				return nil, nil, err
			}
			h.starts[spec.ID]++
			p := &fakeProc{pid: 1000 + h.starts[spec.ID], exit: make(chan Exit, 1)}
			h.procs[spec.ID] = p
			h.handles = append(h.handles, handles)
			return p, p.exit, nil
		},
	})
	if len(specs) == 0 {
		specs = []Spec{testSpec("app")}
	}
	if err := h.sup.Declare(specs); err != nil {
		t.Fatalf("declare: %v", err)
	}
	return h
}

// testSpec is a program with section 18's shape and small numbers, so a whole
// budget fits in a handful of ticks.
func testSpec(id string) Spec {
	return Spec{
		ID: id, Path: "/bin/true",
		Health: HealthPolicy{
			Interval: time.Second, Timeout: time.Second, Idle: 2 * time.Second,
			Register: 10 * time.Second, Degraded: 3, Restart: 5,
		},
		Budget: Budget{
			Restarts: 2, Window: time.Hour,
			Backoff: time.Second, MaxBackoff: 4 * time.Second,
		},
	}
}

func (h *harness) proc(id string) *fakeProc {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.procs[id]
}

//nolint:unparam // the harness takes an id because it supervises a set; one test declares two programs
func (h *harness) startCount(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.starts[id]
}

func (h *harness) status(id string) Status {
	h.t.Helper()
	got, err := h.sup.Health(id)
	if err != nil {
		h.t.Fatalf("health %q: %v", id, err)
	}
	return got[0]
}

func (h *harness) state(id string) State { return h.status(id).State }

// crash ends the child the way a crash does: an exit status arrives on the
// channel the supervisor is watching. It waits for the supervisor to have
// APPLIED it, because the observation happens on the watching goroutine and a
// test that asserted before it ran would be asserting the scheduler.
//
//nolint:unparam // the harness takes an id because it supervises a set; one test declares two programs
func (h *harness) crash(id string, exit Exit) {
	h.t.Helper()
	p := h.proc(id)
	if p == nil {
		h.t.Fatalf("crash %q: never started", id)
	}
	before := h.status(id).LastExit
	p.exit <- exit
	h.waitFor("the exit to be applied", func() bool {
		return h.status(id).LastExit != before
	})
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Microsecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

// up launches and registers, which is the ordinary path to HEALTHY.
func (h *harness) up(id string) {
	h.t.Helper()
	if _, err := h.sup.Up(id); err != nil {
		h.t.Fatalf("up %q: %v", id, err)
	}
	if err := h.sup.Registered(id); err != nil {
		h.t.Fatalf("registered %q: %v", id, err)
	}
	if got := h.state(id); got != StateHealthy {
		h.t.Fatalf("after registering: state %s, want HEALTHY", got)
	}
}

func (h *harness) mustState(id string, want State) {
	h.t.Helper()
	if got := h.state(id); got != want {
		h.t.Fatalf("program %q: state %s, want %s", id, got, want)
	}
}

// hasEdge reports whether the history records exactly this transition, by the
// right actor. The actor is asserted because section 18 names four of them on
// purpose: a restart caused by the health check instead of the restart budget
// would be a history that lies about who decided.
func hasEdge(st Status, from, to State, actor Actor) bool {
	for _, e := range st.History {
		if e.From == from && e.To == to && e.Actor == actor {
			return true
		}
	}
	return false
}

func TestUpLaunchesAndRegistrationMakesItHealthy(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()

	got, err := h.sup.Up()
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if len(got) != 1 || got[0].State != StateStarting {
		t.Fatalf("up returned %+v, want one STARTING program", got)
	}
	if got[0].PID == 0 {
		t.Fatal("up returned a program with no pid, so nothing was launched")
	}
	if err := h.sup.Registered("app"); err != nil {
		t.Fatalf("registered: %v", err)
	}
	h.mustState("app", StateHealthy)

	// The launch row is the table's own "- -> STARTING, caused by rig".
	if !hasEdge(h.status("app"), StateUnspecified, StateStarting, ActorRig) {
		t.Error("no launch row in the history")
	}
	// And the registration row is caused by THE PROGRAM, not by rig.
	if !hasEdge(h.status("app"), StateStarting, StateHealthy, ActorProgram) {
		t.Error("registration was not recorded as the program's own act")
	}
}

func TestUpIsIdempotentAndCarriesTheHandles(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()

	h.up("app")
	if _, err := h.sup.Up("app"); err != nil {
		t.Fatalf("second up: %v", err)
	}
	if n := h.startCount("app"); n != 1 {
		t.Fatalf("up twice started %d children, want 1: `rig up` must be safe to repeat", n)
	}
	h.mustState("app", StateHealthy)

	h.mu.Lock()
	handles := h.handles[0]
	h.mu.Unlock()
	if handles["RIG_PROGRAM_ID"] != "app" {
		t.Errorf("child got RIG_PROGRAM_ID %q, want its own id", handles["RIG_PROGRAM_ID"])
	}
	if handles["RIG_SOCKET"] != "/run/rig.sock" {
		t.Errorf("child got RIG_SOCKET %q, want the socket rig is serving", handles["RIG_SOCKET"])
	}
}

// A CRASH IS THE CASE SECTION 18'S TABLE HAS NO ROW FOR, and this is the test
// that pins how it rides the rows there are: one observation, two transitions,
// two different actors.
func TestACrashTakesBothEdgesInOneObservation(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	h.crash("app", Exit{Code: 2})
	h.mustState("app", StateRestarting)

	st := h.status("app")
	if !hasEdge(st, StateHealthy, StateDegraded, ActorHealthCheck) {
		t.Error("a dead process was not recorded as a health check failing")
	}
	if !hasEdge(st, StateDegraded, StateRestarting, ActorRestartBudget) {
		t.Error("the restart was not recorded as the restart budget acting")
	}
	if st.LastExit == nil || st.LastExit.Code != 2 {
		t.Errorf("last exit %+v, want the child's code 2", st.LastExit)
	}
	if st.NextAttempt.IsZero() {
		t.Error("RESTARTING with no next attempt: the countdown is what the surface renders")
	}
}

// A child that dies BEFORE registering gets section 18's own harsh row.
func TestDeathBeforeRegistrationQuarantinesRatherThanRetrying(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()

	if _, err := h.sup.Up("app"); err != nil {
		t.Fatalf("up: %v", err)
	}
	h.crash("app", Exit{Code: 1})
	h.mustState("app", StateQuarantined)

	if n := h.startCount("app"); n != 1 {
		t.Fatalf("started %d times: a failed registration must not be a retry loop", n)
	}
	if !hasEdge(h.status("app"), StateStarting, StateQuarantined, ActorRig) {
		t.Error("the quarantine was not recorded as rig refusing the registration")
	}
}

func TestAChildThatNeverHandshakesIsQuarantinedAtTheDeadline(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()

	if _, err := h.sup.Up("app"); err != nil {
		t.Fatalf("up: %v", err)
	}
	h.clk.advance(9 * time.Second)
	h.sup.Tick()
	h.mustState("app", StateStarting)

	h.clk.advance(2 * time.Second)
	h.sup.Tick()
	h.mustState("app", StateQuarantined)
	if !h.proc("app").wasStopped() {
		t.Error("the silent child was left running after its quarantine")
	}
}

func TestALaunchThatWillNotExecIsQuarantined(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.mu.Lock()
	h.refuse["app"] = notExecError{}
	h.mu.Unlock()

	if _, err := h.sup.Up("app"); err != nil {
		t.Fatalf("up: %v", err)
	}
	h.mustState("app", StateQuarantined)
}

type notExecError struct{}

func (notExecError) Error() string { return "exec format error" }

// Section 18's two counts, on ONE counter: three failures is degraded, five is
// a restart.
func TestThreeStallsDegradeAndFiveRestart(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	// Past the idle threshold with nothing declared: that is the failure.
	h.clk.advance(3 * time.Second)
	for i := 1; i <= 2; i++ {
		h.sup.Tick()
		h.clk.advance(time.Second)
	}
	if got := h.status("app").Failures; got != 2 {
		t.Fatalf("after two stalls: %d failures, want 2", got)
	}
	h.mustState("app", StateHealthy)

	h.sup.Tick()
	h.mustState("app", StateDegraded)

	for i := 4; i <= 5; i++ {
		h.clk.advance(time.Second)
		h.sup.Tick()
	}
	h.mustState("app", StateRestarting)
	if !hasEdge(h.status("app"), StateHealthy, StateDegraded, ActorHealthCheck) {
		t.Error("the degrade was not recorded as the health check")
	}
}

// WAITING IS NOT STUCK, and that distinction is the reason a program declares
// what it waits for at all.
func TestWaitingIsNotAFailureAndParkedSurfaces(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	if err := h.sup.Observe("app", Report{Marker: 1, Waiting: "the build"}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	h.clk.advance(10 * time.Second)
	for range 6 {
		h.sup.Tick()
		h.clk.advance(time.Second)
	}
	h.mustState("app", StateHealthy)
	if got := h.status("app").Failures; got != 0 {
		t.Fatalf("a waiting program collected %d failures, want 0", got)
	}
	if got := h.status("app").Waiting; got != "the build" {
		t.Errorf("waiting reads %q, want what the program declared", got)
	}

	if err := h.sup.Observe("app", Report{Marker: 1, Parked: "which branch?"}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	h.clk.advance(2 * time.Second)
	h.sup.Tick()
	h.mustState("app", StateHealthy)
	if got := h.status("app").Parked; got != "which branch?" {
		t.Errorf("parked reads %q: the question is the only thing that helps here", got)
	}
}

func TestProgressRecoversADegradedProgram(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	h.clk.advance(3 * time.Second)
	for range 3 {
		h.sup.Tick()
		h.clk.advance(time.Second)
	}
	h.mustState("app", StateDegraded)

	if err := h.sup.Observe("app", Report{Marker: 7}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	h.sup.Tick()
	h.mustState("app", StateHealthy)
	if got := h.status("app").Failures; got != 0 {
		t.Errorf("recovery left %d failures, want the counter reset", got)
	}
	if !hasEdge(h.status("app"), StateDegraded, StateHealthy, ActorHealthCheck) {
		t.Error("the recovery was not recorded as the health check")
	}
}

// THE WHOLE BUDGET, WALKED: doubling backoff, the cap, two restarts, then
// quarantine with the history intact.
func TestTheRestartBudgetDoublesThenQuarantinesWithItsHistory(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	waits := []time.Duration{time.Second, 2 * time.Second}
	for i, want := range waits {
		h.crash("app", Exit{Code: 1})
		h.mustState("app", StateRestarting)

		st := h.status("app")
		if got := st.NextAttempt.Sub(h.clk.now()); got != want {
			t.Fatalf("restart %d: backoff %s, want %s", i+1, got, want)
		}
		// Before the backoff elapses, nothing happens.
		h.sup.Tick()
		h.mustState("app", StateRestarting)

		h.clk.advance(want)
		h.sup.Tick()
		h.mustState("app", StateStarting)
		if err := h.sup.Registered("app"); err != nil {
			t.Fatalf("registered: %v", err)
		}
	}
	if n := h.startCount("app"); n != 3 {
		t.Fatalf("started %d times, want the launch plus two restarts", n)
	}

	// The third crash exhausts a budget of two.
	h.crash("app", Exit{Code: 1})
	h.clk.advance(4 * time.Second)
	h.sup.Tick()
	h.mustState("app", StateQuarantined)
	if n := h.startCount("app"); n != 3 {
		t.Fatalf("started %d times: an exhausted budget must not start anything", n)
	}

	st := h.status("app")
	if !hasEdge(st, StateRestarting, StateQuarantined, ActorRestartBudget) {
		t.Error("the quarantine was not recorded as the budget running out")
	}
	if len(st.History) < 8 {
		t.Errorf("%d history rows: section 18 requires the full history survive into quarantine", len(st.History))
	}
	var sawExhausted bool
	for _, e := range st.History {
		if e.Trigger == TriggerBudgetExhausted && strings.Contains(e.Note, "whole budget") {
			sawExhausted = true
		}
	}
	if !sawExhausted {
		t.Error("the quarantine row does not say the budget is what ran out")
	}
	if !st.NextAttempt.IsZero() {
		t.Errorf("a quarantined program shows a next attempt at %v; rig will not try again", st.NextAttempt)
	}
}

// THE BACKOFF IS CAPPED, which is what keeps a program that fails all night
// from waiting hours between attempts inside a budget it will never exhaust.
func TestTheBackoffIsCappedAtMaxBackoff(t *testing.T) {
	spec := testSpec("app")
	spec.Budget = Budget{Restarts: 10, Window: time.Hour, Backoff: time.Second, MaxBackoff: 2 * time.Second}
	h := newHarness(t, spec)
	defer h.sup.StopAll()
	h.up("app")

	for _, want := range []time.Duration{time.Second, 2 * time.Second, 2 * time.Second} {
		h.crash("app", Exit{Code: 1})
		if got := h.status("app").NextAttempt.Sub(h.clk.now()); got != want {
			t.Fatalf("backoff %s, want %s (capped)", got, want)
		}
		h.clk.advance(want)
		h.sup.Tick()
		if err := h.sup.Registered("app"); err != nil {
			t.Fatalf("registered: %v", err)
		}
	}
}

// ONLY A HUMAN LEAVES QUARANTINE, and the history has to say so.
func TestOnlyAHumanRestartsAQuarantinedProgram(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()

	if _, err := h.sup.Up("app"); err != nil {
		t.Fatalf("up: %v", err)
	}
	h.crash("app", Exit{Code: 1})
	h.mustState("app", StateQuarantined)

	// Every non-human trigger is refused from here, and a refusal is a fault
	// that leaves it quarantined rather than moving it.
	if err := h.sup.Registered("app"); err != nil {
		t.Fatalf("registered: %v", err)
	}
	h.mustState("app", StateQuarantined)

	if err := h.sup.Restart("app"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	h.mustState("app", StateStarting)
	if !hasEdge(h.status("app"), StateQuarantined, StateStarting, ActorHuman) {
		t.Error("leaving quarantine was not recorded as a human's act")
	}
	if got := h.status("app").Restarts; got != 0 {
		t.Errorf("a human's restart left %d restarts on the budget, want a fresh one", got)
	}
}

func TestRestartingARunningProgramStopsItAndLaunchesAgain(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")
	first := h.proc("app")

	if err := h.sup.Restart("app"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	h.mustState("app", StateStarting)
	if !first.wasStopped() {
		t.Error("the old child was left running")
	}
	if n := h.startCount("app"); n != 2 {
		t.Fatalf("started %d times, want the original plus the restart", n)
	}

	// The old child's exit lands after the new child exists. It is not
	// evidence about the new one, which must stay tracked and STARTING.
	second := h.proc("app")
	for range 20 {
		time.Sleep(5 * time.Millisecond)
		if st := h.status("app"); st.State != StateStarting || st.PID != second.PID() {
			t.Fatalf("the old child's exit was charged to the new one: %s, pid %d (new child is %d)",
				st.State, st.PID, second.PID())
		}
	}
}

// `rig stop` RETURNS A PROGRAM TO THE TABLE'S DASH. There is no STOPPED state
// and this is the test that keeps one from being added.
func TestStopReturnsToTheDashAndUpStartsItAgain(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")
	first := h.proc("app")

	if err := h.sup.Stop("app"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	h.mustState("app", StateUnspecified)
	if !first.wasStopped() {
		t.Error("stop did not stop the child")
	}
	if len(h.status("app").History) == 0 {
		t.Error("stop threw the history away; a stopped program still has to say what happened")
	}

	if _, err := h.sup.Up("app"); err != nil {
		t.Fatalf("up after stop: %v", err)
	}
	h.mustState("app", StateStarting)
	if n := h.startCount("app"); n != 2 {
		t.Fatalf("started %d times, want two", n)
	}
}

func TestPanickingTwiceQuarantinesFromAnywhere(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	if err := h.sup.Panicked("app", "nil map write"); err != nil {
		t.Fatalf("panicked: %v", err)
	}
	h.mustState("app", StateHealthy)

	if err := h.sup.Panicked("app", "nil map write"); err != nil {
		t.Fatalf("panicked: %v", err)
	}
	h.mustState("app", StateQuarantined)
	if !hasEdge(h.status("app"), StateHealthy, StateQuarantined, ActorInvoker) {
		t.Error("the quarantine was not recorded as the invoker's panic recovery")
	}
}

// AN ILLEGAL TRANSITION IS A SUPERVISOR FAULT, recorded and quarantined.
func TestAnIllegalTransitionIsAFaultAndQuarantines(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	// Nothing in section 18's table registers a program that is already
	// HEALTHY, so this is the fault path.
	if err := h.sup.Registered("app"); err != nil {
		t.Fatalf("registered: %v", err)
	}
	h.mustState("app", StateQuarantined)

	st := h.status("app")
	last := st.History[len(st.History)-1]
	if !strings.Contains(last.Note, "supervisor fault") {
		t.Errorf("the history says %q, want the fault and its reason", last.Note)
	}
	if !h.proc("app").wasStopped() {
		t.Error("a program rig cannot account for was left running")
	}
}

func TestStopAllEndsEveryRunningChild(t *testing.T) {
	h := newHarness(t, testSpec("one"), testSpec("two"))
	h.up("one")
	h.up("two")

	h.sup.StopAll()
	for _, id := range []string{"one", "two"} {
		h.mustState(id, StateUnspecified)
		if !h.proc(id).wasStopped() {
			t.Errorf("%s was left running", id)
		}
	}
}

func TestEveryVerbRefusesAProgramNobodyDeclared(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()

	if _, err := h.sup.Up("ghost"); err == nil {
		t.Error("up accepted an undeclared program")
	}
	if err := h.sup.Stop("ghost"); err == nil {
		t.Error("stop accepted an undeclared program")
	}
	if err := h.sup.Restart("ghost"); err == nil {
		t.Error("restart accepted an undeclared program")
	}
	if err := h.sup.Observe("ghost", Report{}); err == nil {
		t.Error("observe accepted an undeclared program")
	}
	if err := h.sup.Panicked("ghost", "x"); err == nil {
		t.Error("panicked accepted an undeclared program")
	}
	if _, err := h.sup.Health("ghost"); err == nil {
		t.Error("health accepted an undeclared program")
	}
}

func TestDeclareRefusesADuplicateAndAnInvalidSpec(t *testing.T) {
	s := New(Options{Start: func(Spec, map[string]string) (Process, <-chan Exit, error) {
		return nil, nil, nil
	}})
	if err := s.Declare([]Spec{testSpec("a"), testSpec("a")}); err == nil {
		t.Error("two programs with one id were accepted")
	}
	bad := testSpec("b")
	bad.Path = "relative/path"
	if err := s.Declare([]Spec{bad}); err == nil {
		t.Error("a relative executable path was accepted")
	}
}

// A SECOND DECLARATION DOES NOT STOP WHAT IS RUNNING. Editing the programs
// file must not be a destructive act.
func TestRedeclaringLeavesRunningProgramsAlone(t *testing.T) {
	h := newHarness(t)
	defer h.sup.StopAll()
	h.up("app")

	spec := testSpec("app")
	spec.Args = []string{"--new"}
	if err := h.sup.Declare([]Spec{spec, testSpec("second")}); err != nil {
		t.Fatalf("declare: %v", err)
	}
	h.mustState("app", StateHealthy)
	h.mustState("second", StateUnspecified)
	if got := h.sup.Declared(); len(got) != 2 {
		t.Fatalf("declared %v, want both programs", got)
	}
}

func TestSortStatusPutsWhatNeedsAHumanFirst(t *testing.T) {
	in := []Status{
		{ID: "d", State: StateHealthy},
		{ID: "a", State: StateUnspecified},
		{ID: "c", State: StateQuarantined},
		{ID: "b", State: StateDegraded},
	}
	SortStatus(in)
	want := []string{"c", "b", "d", "a"}
	for i, id := range want {
		if in[i].ID != id {
			t.Fatalf("order %v, want quarantine first and unsupervised last", in)
		}
	}
}

func TestWithinWindowForgetsOldRestartsAndAZeroWindowNever(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	at := []time.Time{now.Add(-2 * time.Hour), now.Add(-time.Minute)}
	if got := withinWindow(append([]time.Time(nil), at...), now, time.Hour); len(got) != 1 {
		t.Errorf("kept %d restarts, want only the one inside the window", len(got))
	}
	if got := withinWindow(append([]time.Time(nil), at...), now, 0); len(got) != 2 {
		t.Errorf("a zero window forgot %d: it means the count never forgets", 2-len(got))
	}
}
