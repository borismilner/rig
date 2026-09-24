package supervise

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Supervisor is PLAN.md section 18's machine running: the table in state.go
// applied to real children, on the interval, inside a restart budget.
//
// ⛔ NOTHING HERE DECIDES A DESTINATION. Every state change goes through
// Resolve, which is the section's table, so a trigger this file invents lands
// as a supervisor fault instead of as a plausible-looking state. That is the
// point of the table and it is why `move` is the only writer of p.state.
//
// THE CLOCK AND THE LAUNCHER ARE BOTH INJECTED, and not for neatness: a
// restart budget is a statement about time, and a test that has to sleep
// through a backoff is a test that either takes minutes or asserts nothing.
// Tick(now) is the whole loop; Run only calls it.
type Supervisor struct {
	start   Starter
	now     func() time.Time
	handles map[string]string
	grace   time.Duration

	mu       sync.Mutex
	programs map[string]*program
	order    []string

	// wake is pulsed when a child ends, so Run acts on a crash at once rather
	// than at the end of a health interval. Buffered and never blocking: the
	// sender is the goroutine waiting on the process, and a supervisor that
	// wedged it would stop hearing about every later exit.
	wake chan struct{}
}

// Options builds a Supervisor. Start and Now both default, so a caller that
// wants the real thing passes nothing.
type Options struct {
	// Start launches a child. Nil means the real one.
	Start Starter

	// Handles are the RIG_* variables every child gets: the socket to dial,
	// the wire version. RIG_PROGRAM_ID is added per program.
	Handles map[string]string

	// StopGrace is how long a child gets after SIGTERM. Zero means
	// DefaultStopGrace.
	StopGrace time.Duration

	// Now is the clock. Nil means time.Now.
	//
	// ⛔ IT IS INJECTED BECAUSE A RESTART BUDGET IS A STATEMENT ABOUT TIME. A
	// test that has to sleep through an exponential backoff either takes
	// minutes or asserts nothing, and the second is what usually happens. With
	// the clock in hand a whole budget - five restarts, doubling, exhaustion,
	// quarantine - runs in microseconds and asserts every step.
	Now func() time.Time
}

// New builds a supervisor with no programs declared.
func New(opts Options) *Supervisor {
	s := &Supervisor{
		start:    opts.Start,
		now:      opts.Now,
		handles:  opts.Handles,
		grace:    opts.StopGrace,
		programs: map[string]*program{},
		wake:     make(chan struct{}, 1),
	}
	if s.start == nil {
		s.start = Start
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.grace == 0 {
		s.grace = DefaultStopGrace
	}
	return s
}

// program is one declared program and everything section 18 needs to know
// about it: where it is in the table, why, and what happened before.
type program struct {
	spec  Spec
	state State
	since time.Time

	proc     Process
	lastExit *Exit

	// report is the last thing the program said about its own progress, and
	// advanced is when its marker last MOVED. The difference between the two
	// is the whole health definition: a report that repeats a marker is a
	// report, not progress.
	report   Report
	advanced time.Time

	// checked is when health was last judged, so Tick may be called as often
	// as a caller likes and health still happens on the interval.
	checked time.Time

	failures int
	panics   int

	// restarts are the moments rig restarted this program, pruned to the
	// budget's window. The budget is a count over a window, so the moments
	// have to be kept - a bare counter cannot forget.
	restarts []time.Time
	backoff  time.Duration
	readyAt  time.Time

	history []Event
}

// Report is what a supervised program says about its own progress.
//
// PLAN.md section 18: health is evidence of PROGRESS. The program advances
// Marker when it does something; it sets Waiting when it is blocked on
// something it can name; it sets Parked when it is stuck behind a question
// nobody has seen. Those are three different facts and collapsing any two of
// them is the measurement bug the section is about.
type Report struct {
	// Marker advances. Whether it counts steps, files or tokens is the
	// program's business; that it MOVED is rig's.
	Marker uint64

	// Waiting is what this program is blocked on, in words a human reads.
	// Empty means it is not blocked, which is the half that makes idle
	// unhealthy rather than restful.
	Waiting string

	// Parked is the question nobody has seen yet. Neither progress nor fault:
	// the only action that helps is showing it to a human.
	Parked string
}

// Observation is what one health judgement saw. It is not a state: it is the
// evidence a state change is made from, and it exists separately so the
// judging is testable without a transition.
type Observation uint8

const (
	// ObservedUnspecified is the zero and means no judgement was made.
	ObservedUnspecified Observation = iota

	// ObservedProgress - the marker advanced. Health, and the only thing that
	// is.
	ObservedProgress

	// ObservedWaiting - the marker is still and the program named what it is
	// waiting for. Not a failure: section 18 keeps waiting and stuck apart
	// precisely so this is not counted.
	ObservedWaiting

	// ObservedParked - parked on a question nobody has seen. Its own
	// observation, neither progress nor fault.
	ObservedParked

	// ObservedStalled - still, nothing declared, past the idle threshold.
	// ONE failure. "Idle-and-not-blocked is UNHEALTHY, with a threshold."
	ObservedStalled

	// ObservedDead - the process has exited. A health check that failed
	// DEFINITIVELY: a process that is gone cannot show progress and cannot
	// recover, so it does not get the three-strike ladder.
	ObservedDead
)

var observationNames = map[Observation]string{
	ObservedUnspecified: "UNSPECIFIED",
	ObservedProgress:    "progress",
	ObservedWaiting:     "waiting",
	ObservedParked:      "parked",
	ObservedStalled:     "stalled",
	ObservedDead:        "the process exited",
}

func (o Observation) String() string {
	if n, ok := observationNames[o]; ok {
		return n
	}
	return fmt.Sprintf("Observation(%d)", uint8(o))
}

// Event is one row of a program's history, which section 18 requires survive
// into quarantine: "a visible state with the full history and a manual
// restart, never a silent disappearance."
type Event struct {
	At      time.Time
	From    State
	To      State
	Trigger Trigger
	Actor   Actor

	// Note is what was actually seen, so the history says why and not only
	// what. A fault puts its whole sentence here.
	Note string
}

// historyCap bounds one program's history. A supervisor that remembers
// forever is a leak in the process that must not have one; 100 rows covers
// the restart cycle a human reads after a quarantine, which is the reason the
// history exists.
const historyCap = 100

// Status is one program as every surface sees it.
type Status struct {
	ID       string
	State    State
	PID      int
	Since    time.Time
	Failures int
	Restarts int

	// Marker, Waiting and Parked are the last report, carried to the surface
	// unchanged. Parked especially: rendering it as healthy or as a fault
	// loses the only action that helps.
	Marker  uint64
	Waiting string
	Parked  string

	// LastExit is how the last child ended, nil if none ever has.
	LastExit *Exit

	// NextAttempt is when the backoff elapses, zero unless RESTARTING.
	NextAttempt time.Time

	History []Event
}

// ErrNoSuchProgram is a verb naming a program that was never declared.
var ErrNoSuchProgram = errors.New("no such declared program")

// Declare replaces the declared set. Programs already running keep running:
// a declaration is a list of what MAY run, and stopping a healthy child
// because a file was re-read would make editing the file a destructive act.
func (s *Supervisor) Declare(specs []Spec) error {
	for _, spec := range specs {
		if err := spec.Validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, spec := range specs {
		if seen[spec.ID] {
			return fmt.Errorf("program %q is declared twice", spec.ID)
		}
		seen[spec.ID] = true
		if p, ok := s.programs[spec.ID]; ok {
			p.spec = spec
			continue
		}
		s.programs[spec.ID] = &program{spec: spec}
		s.order = append(s.order, spec.ID)
	}
	return nil
}

// Up launches the named programs, or every declared one when none is named.
//
// A program already past `-` is left alone and reported as it stands: `rig up`
// twice is the same estate as `rig up` once, which is what makes it safe to
// put in a login script.
func (s *Supervisor) Up(ids ...string) ([]Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(ids) == 0 {
		ids = append(ids, s.order...)
	}
	out := make([]Status, 0, len(ids))
	for _, id := range ids {
		p, ok := s.programs[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
		}
		if p.state == StateUnspecified {
			s.launch(p, now)
		}
		out = append(out, p.status())
	}
	return out, nil
}

// launch moves a program onto the table and starts its child. The caller holds
// the lock.
//
// A launch that FAILS is not a retry loop: section 18's STARTING ->
// QUARANTINED row is "any registration step fails", and a child that would not
// exec never had a registration to fail at. It lands in the same place, with
// the reason.
func (s *Supervisor) launch(p *program, now time.Time) {
	s.move(p, TriggerLaunch, now, "")
	if p.state != StateStarting {
		return
	}
	s.spawn(p, now)
}

// Registered is the program's own row: "handshake and declaration validated".
// The daemon calls it when a supervised program completes hello.
func (s *Supervisor) Registered(id string) error {
	return s.trigger(id, TriggerRegistered, "")
}

// Panicked is section 5j: the invoker recovered a panic. Twice inside the
// budget and the program is quarantined from wherever it is.
func (s *Supervisor) Panicked(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	p, ok := s.programs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
	}
	p.panics++
	if p.panics < 2 {
		return nil
	}
	s.move(p, TriggerPanickedTwice, now, reason)
	return nil
}

// Restart is a human asking for one.
//
// From QUARANTINED it is section 18's own row, and the ONLY actor allowed to
// take that edge. From anywhere else there is no row, and inventing one would
// be the inference the table exists to refuse - so it is exactly what a person
// means by it: stop the child, return the program to `-`, launch it again.
// Both halves are recorded with a human as the actor.
func (s *Supervisor) Restart(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	p, ok := s.programs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
	}
	if p.state == StateQuarantined {
		s.move(p, TriggerManualRestart, now, "")
		s.startAfterManual(p, now)
		return nil
	}
	s.halt(p, now, "a human asked for a restart")
	s.launch(p, now)
	return nil
}

// startAfterManual launches the child for a QUARANTINED -> STARTING that the
// table already took. The counters are cleared because a human acting IS the
// reset: section 18 makes quarantine terminal until somebody looks, so the
// look is what earns a fresh budget.
func (s *Supervisor) startAfterManual(p *program, now time.Time) {
	p.failures, p.panics, p.restarts, p.backoff = 0, 0, nil, 0
	p.readyAt = time.Time{}
	s.spawn(p, now)
}

// Stop ends a program and returns it to `-`, the table's own pre-STARTING row.
//
// ⛔ THERE IS NO `STOPPED` STATE AND THERE MUST NOT BE. Section 18: "The
// states, and this is the set. There are no others." A program rig is not
// running is a program with no state, which is exactly what the first row's
// dash means, and `rig up` puts it back on the table through TriggerLaunch.
// The history is KEPT: a stopped program still has to be able to say what
// happened to it.
func (s *Supervisor) Stop(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	p, ok := s.programs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
	}
	s.halt(p, now, "a human stopped it")
	return nil
}

// halt stops the child and clears the program off the table. The caller holds
// the lock.
func (s *Supervisor) halt(p *program, now time.Time, note string) {
	s.stopChild(p)
	p.record(Event{
		At: now, From: p.state, To: StateUnspecified,
		Trigger: TriggerManualRestart, Actor: ActorHuman, Note: note,
	})
	p.state, p.since = StateUnspecified, now
	p.failures, p.panics, p.backoff, p.restarts = 0, 0, 0, nil
	p.readyAt = time.Time{}
}

// stopChild asks the child to leave and forgets it. The exit still arrives on
// the channel and is dropped by Tick, which is why exits name a program rather
// than carrying a process.
func (s *Supervisor) stopChild(p *program) {
	if p.proc != nil {
		p.proc.Stop(s.grace)
		p.proc = nil
	}
}

// StopAll ends every running child. Shutdown, section 18: "Programs rig did
// not start are never killed" - so this only reaches what launch started.
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, id := range s.order {
		if p := s.programs[id]; p.state != StateUnspecified {
			s.halt(p, now, "rig is shutting down")
		}
	}
}

// Observe records what a program said about its own progress. It never moves
// a state by itself: the marker is evidence, and the judging happens on the
// interval in Tick.
func (s *Supervisor) Observe(id string, r Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	p, ok := s.programs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
	}
	if r.Marker > p.report.Marker {
		p.advanced = now
	}
	p.report = r
	return nil
}

// Health is every declared program as it stands.
func (s *Supervisor) Health(ids ...string) ([]Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(ids) == 0 {
		ids = append(ids, s.order...)
	}
	out := make([]Status, 0, len(ids))
	for _, id := range ids {
		p, ok := s.programs[id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
		}
		out = append(out, p.status())
	}
	return out, nil
}

// trigger takes one named edge for one program, for the callers that have
// nothing to decide.
func (s *Supervisor) trigger(id string, t Trigger, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	p, ok := s.programs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchProgram, id)
	}
	s.move(p, t, now, note)
	return nil
}

// move is THE ONLY WRITER OF p.state, and it writes what the table says.
//
// An illegal transition is section 18's supervisor fault: recorded with the
// whole sentence and quarantined, "because a supervisor that cannot account
// for its own state is the one component whose confusion is not survivable."
func (s *Supervisor) move(p *program, t Trigger, now time.Time, note string) {
	tr, err := Resolve(p.spec.ID, p.state, t)
	if err != nil {
		s.stopChild(p)
		p.record(Event{
			At: now, From: p.state, To: StateQuarantined,
			Trigger: t, Actor: ActorRig, Note: err.Error(),
		})
		p.state, p.since = StateQuarantined, now
		return
	}
	if tr.To == StateQuarantined {
		// ⛔ A QUARANTINED PROGRAM HAS NO CHILD RUNNING, and that is not
		// tidiness. Section 18 makes quarantine terminal until a human acts,
		// so a surface saying QUARANTINED over a process still doing work
		// would be the supervisor lying about the one state a person is meant
		// to act on. Every route into quarantine passes through here, which is
		// why the stop is here rather than at the four call sites.
		s.stopChild(p)
	}
	p.record(Event{
		At: now, From: tr.From, To: tr.To,
		Trigger: tr.Trigger, Actor: tr.Actor, Note: note,
	})
	p.state, p.since = tr.To, now
}

func (p *program) record(e Event) {
	p.history = append(p.history, e)
	if len(p.history) > historyCap {
		p.history = append(p.history[:0], p.history[len(p.history)-historyCap:]...)
	}
}

func (p *program) status() Status {
	st := Status{
		ID: p.spec.ID, State: p.state, Since: p.since,
		Failures: p.failures, Restarts: len(p.restarts),
		Marker: p.report.Marker, Waiting: p.report.Waiting, Parked: p.report.Parked,
		LastExit: p.lastExit, NextAttempt: p.readyAt,
		History: append([]Event(nil), p.history...),
	}
	if p.proc != nil {
		st.PID = p.proc.PID()
	}
	return st
}

// Tick is one pass of the supervisor: judge health on the interval, and let
// the restart budget act on anything backing off.
//
// A CHILD ENDING DOES NOT WAIT FOR A TICK. The goroutine watching the process
// applies the exit as it happens, because five seconds of a surface saying
// HEALTHY about a process that is gone is exactly the measurement bug section
// 18 is written against.
func (s *Supervisor) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for _, id := range s.order {
		p := s.programs[id]
		switch p.state {
		case StateStarting:
			s.checkRegistration(p, now)
		case StateHealthy, StateDegraded:
			s.checkHealth(p, now)
		case StateRestarting:
			s.checkBackoff(p, now)
		case StateUnspecified, StateQuarantined:
			// `-` is not supervised and QUARANTINED waits for a human. Named
			// rather than defaulted so a sixth state cannot land here silently.
		}
	}
}

// applyExit is a child ending, applied from the goroutine that saw it end.
func (s *Supervisor) applyExit(id string, exit Exit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	p, ok := s.programs[id]
	if !ok {
		return
	}
	if p.proc == nil {
		// Already stopped or already replaced: this is the exit of a child rig
		// asked to leave, and it is not evidence about the program's health.
		return
	}
	p.lastExit = &exit
	p.proc = nil
	s.observed(p, ObservedDead, now, "the child ended: "+exit.String())
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// checkRegistration quarantines a child that never finished its handshake.
//
// Section 18 gives STARTING exactly two exits and one of them is "any
// registration step fails ... Not a retry loop". A child that is alive and
// silent past the deadline has failed one, so the deliberately harsh row is
// the right one rather than a restart.
func (s *Supervisor) checkRegistration(p *program, now time.Time) {
	d := p.spec.Health.Register
	if d <= 0 {
		d = DefaultHealth().Register
	}
	if now.Sub(p.since) < d {
		return
	}
	s.move(p, TriggerRegistrationFailed, now,
		"no handshake within "+d.String())
}

// checkHealth judges one program, on its interval.
func (s *Supervisor) checkHealth(p *program, now time.Time) {
	interval := p.spec.Health.Interval
	if interval <= 0 {
		interval = DefaultHealth().Interval
	}
	if now.Sub(p.checked) < interval {
		return
	}
	p.checked = now
	obs := judge(p, now)
	s.observed(p, obs, now, obs.String())
}

// judge is section 18's health definition and nothing else, kept apart from
// the transition it causes so it can be tested as the reading it is.
func judge(p *program, now time.Time) Observation {
	idle := p.spec.Health.Idle
	if idle <= 0 {
		idle = DefaultHealth().Idle
	}
	switch {
	case now.Sub(p.advanced) < idle:
		// The marker moved inside the window. That is the only evidence of
		// progress there is.
		return ObservedProgress
	case p.report.Parked != "":
		// "Parked on a question nobody has seen is its own state - it is
		// neither progress nor a fault."
		return ObservedParked
	case p.report.Waiting != "":
		// "A waiting session declares WHAT it waits for", so waiting is
		// distinguishable from stuck and only one of them is a failure.
		return ObservedWaiting
	default:
		return ObservedStalled
	}
}

// observed turns one reading into whatever the table says it causes.
func (s *Supervisor) observed(p *program, obs Observation, now time.Time, note string) {
	switch obs {
	case ObservedProgress:
		p.failures = 0
		if p.state == StateDegraded {
			s.move(p, TriggerHealthRecovered, now, note)
		}
	case ObservedWaiting, ObservedParked:
		// Neither is a failure and neither is progress, so the counter is left
		// exactly where it was. This is the row that keeps a session blocked on
		// a person from being restarted out from under them.
	case ObservedStalled:
		p.failures++
		s.escalate(p, now, note)
	case ObservedDead:
		s.died(p, now, note)
	case ObservedUnspecified:
		// No judgement was made, so nothing follows from it.
	}
}

// escalate applies section 18's two counts: three failures is degraded, five
// is a restart, on ONE counter rather than two.
func (s *Supervisor) escalate(p *program, now time.Time, note string) {
	pol := p.spec.Health
	if pol.Degraded <= 0 {
		pol.Degraded = DefaultHealth().Degraded
	}
	if pol.Restart <= 0 {
		pol.Restart = DefaultHealth().Restart
	}
	switch {
	case p.state == StateHealthy && p.failures >= pol.Degraded:
		s.move(p, TriggerHealthFailures, now, note)
	case p.state == StateDegraded && p.failures >= pol.Restart:
		s.beginRestart(p, now, note)
	}
}

// died is a crash, and it is the case section 18's table has no row for.
//
// ⛔ A DEAD PROCESS IS A HEALTH CHECK THAT FAILED DEFINITIVELY. It cannot show
// progress and it cannot recover, so it does not get the three-strike ladder:
// one observation takes HEALTHY -> DEGRADED (the health check) and then
// DEGRADED -> RESTARTING (the restart budget), each recorded with its own
// actor. The table stays total and nothing is invented.
//
// From STARTING it is the section's own answer: a child that dies before it
// registers failed a registration step, which is a quarantine and deliberately
// not a retry loop.
func (s *Supervisor) died(p *program, now time.Time, note string) {
	if p.state == StateStarting {
		s.move(p, TriggerRegistrationFailed, now, note)
		return
	}
	pol := p.spec.Health
	if pol.Restart <= 0 {
		pol.Restart = DefaultHealth().Restart
	}
	p.failures = pol.Restart
	if p.state == StateHealthy {
		s.move(p, TriggerHealthFailures, now, note)
	}
	s.beginRestart(p, now, note)
}

// beginRestart takes DEGRADED -> RESTARTING and starts the backoff clock.
func (s *Supervisor) beginRestart(p *program, now time.Time, note string) {
	s.stopChild(p)
	s.move(p, TriggerBudgetRestart, now, note)
	if p.state != StateRestarting {
		// The move was refused and recorded as a fault; the clock must not run
		// for a program that is now quarantined.
		return
	}
	b := p.spec.Budget
	if b.Backoff <= 0 {
		b = DefaultBudget()
	}
	if p.backoff == 0 {
		p.backoff = b.Backoff
	} else {
		p.backoff *= 2
	}
	if b.MaxBackoff > 0 && p.backoff > b.MaxBackoff {
		p.backoff = b.MaxBackoff
	}
	p.readyAt = now.Add(p.backoff)
}

// checkBackoff is the restart budget acting: when the wait is over, either
// there is budget left and the program starts again, or there is not and it is
// quarantined with its history intact.
func (s *Supervisor) checkBackoff(p *program, now time.Time) {
	if now.Before(p.readyAt) {
		return
	}
	b := p.spec.Budget
	if b.Restarts <= 0 {
		b = DefaultBudget()
	}
	p.restarts = withinWindow(p.restarts, now, b.Window)
	if len(p.restarts) >= b.Restarts {
		s.move(p, TriggerBudgetExhausted, now, fmt.Sprintf(
			"%d restarts in %s is the whole budget", len(p.restarts), b.Window))
		return
	}
	p.restarts = append(p.restarts, now)
	s.move(p, TriggerBackoffElapsed, now, "backed off "+p.backoff.String())
	if p.state != StateStarting {
		return
	}
	p.readyAt = time.Time{}
	p.failures = 0
	s.spawn(p, now)
}

// spawn starts the child for a program the table has already moved to
// STARTING. launch is the version that takes TriggerLaunch as well.
func (s *Supervisor) spawn(p *program, now time.Time) {
	handles := map[string]string{}
	for k, v := range s.handles {
		handles[k] = v
	}
	handles["RIG_PROGRAM_ID"] = p.spec.ID
	proc, exited, err := s.start(p.spec, handles)
	if err != nil {
		s.move(p, TriggerRegistrationFailed, now, "the child would not start: "+err.Error())
		return
	}
	p.proc, p.checked, p.advanced, p.report = proc, now, now, Report{}
	id := p.spec.ID
	go func() {
		e, ok := <-exited
		if !ok {
			return
		}
		s.applyExit(id, e)
	}()
}

// withinWindow drops the restarts that are older than the budget's window. A
// zero window means the count never forgets, which is a legitimate declaration
// rather than a bug: "five restarts ever, then quarantine".
func withinWindow(at []time.Time, now time.Time, window time.Duration) []time.Time {
	if window <= 0 {
		return at
	}
	cut := now.Add(-window)
	out := at[:0]
	for _, t := range at {
		if t.After(cut) {
			out = append(out, t)
		}
	}
	return out
}

// Run drives Tick until the context ends, then stops every child rig started.
//
// The loop wakes on the tick AND on a child ending. A BACKOFF IS THEREFORE A
// MINIMUM AND NOT A DEADLINE: a program whose backoff elapses between ticks
// restarts at the next one. That is deliberate - one timer in rigd rather than
// a timer per backing-off program, which the footprint ruling decides - and it
// is why the backoff a surface shows is "not before", never "at".
func (s *Supervisor) Run(ctx context.Context) {
	tick := time.NewTicker(s.interval())
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			s.StopAll()
			return
		case <-s.wake:
			s.Tick()
		case <-tick.C:
			s.Tick()
		}
	}
}

// interval is the shortest health interval any declared program asked for, so
// a program wanting a one second check gets one without every other program
// being judged that often.
func (s *Supervisor) interval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	best := DefaultHealth().Interval
	for _, p := range s.programs {
		if d := p.spec.Health.Interval; d > 0 && d < best {
			best = d
		}
	}
	return best
}

// Declared lists the declared program ids in the order they were declared.
func (s *Supervisor) Declared() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.order...)
}

// SortStatus puts a status list in the order a surface should render it: the
// states that need a human first, then by id. Quarantine is a state nobody
// should have to scroll to.
func SortStatus(in []Status) {
	rank := map[State]int{
		StateQuarantined: 0, StateRestarting: 1, StateDegraded: 2,
		StateStarting: 3, StateHealthy: 4, StateUnspecified: 5,
	}
	sort.SliceStable(in, func(i, j int) bool {
		if rank[in[i].State] != rank[in[j].State] {
			return rank[in[i].State] < rank[in[j].State]
		}
		return in[i].ID < in[j].ID
	})
}
