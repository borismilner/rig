// Package supervise is PLAN.md section 18: the supervisor as a machine.
//
// The section is explicit that prose was the defect - "five state names that
// appear only in section 5's candidate section, no state set, no transition
// table, no owner, no statement of what is illegal" - so this package is the
// table, and every rule it states is a value here rather than a sentence in a
// comment.
//
// ⛔ IT LINKS NOTHING BUT THE STANDARD LIBRARY, and that is a requirement
// rather than a preference. cmd/rigwindow is the tray, which exists to hold
// a few megabytes instead of the window's 202 MB (cmd/rigwindow/supervisor.go
// says why), and it supervises its own child through this package. A proto
// import here would put the wire into the tray. The daemon translates these
// types to the wire the same way it translates the kernel's.
package supervise

import (
	"fmt"
	"sort"
	"strings"
)

// State is one of the five in PLAN.md section 18, and there are no others.
// The section says so in as many words: "The states, and this is the set."
//
// Zero is UNSPECIFIED and is not a state a program can be in. Section 21 bans
// a meaningful enum zero, and this is the same rule one layer in: a program
// whose state was never set must not read as STARTING by accident.
type State uint8

const (
	// StateUnspecified is the zero, and it is never legal.
	StateUnspecified State = iota

	// StateStarting - rig has launched the child and it has not completed
	// the handshake.
	StateStarting

	// StateHealthy - registered, and showing evidence of progress.
	StateHealthy

	// StateDegraded - reachable, and failing its health definition.
	StateDegraded

	// StateRestarting - inside the restart budget, backing off.
	StateRestarting

	// StateQuarantined - out of budget, or failed registration. Visible,
	// with history, and it stays until a human acts.
	StateQuarantined
)

var stateNames = map[State]string{
	StateUnspecified: "UNSPECIFIED",
	StateStarting:    "STARTING",
	StateHealthy:     "HEALTHY",
	StateDegraded:    "DEGRADED",
	StateRestarting:  "RESTARTING",
	StateQuarantined: "QUARANTINED",
}

func (s State) String() string {
	if n, ok := stateNames[s]; ok {
		return n
	}
	return fmt.Sprintf("State(%d)", uint8(s))
}

// Actor is who caused a transition.
//
// PLAN.md section 18: "Four actors can move a program and they are named
// here" - the health check, the restart budget, the invoker's panic recovery,
// and a human. Launch and registration are rig and the program themselves,
// which the transition table names as causes too, so the set here is six
// rather than four: the section's four are the ones that move a program that
// is already running, and collapsing the other two into them would lose which
// half of a restart cycle a row came from.
//
// A DRAINING notice is deliberately NOT here. Section 18: "it is not a fifth
// actor, and reading it as one is the mistake this row prevents" - it moves
// leases, not program states.
type Actor uint8

const (
	// ActorUnspecified is the zero, and it is never legal.
	ActorUnspecified Actor = iota

	// ActorRig is rig itself: a launch, or a registration it refused.
	ActorRig

	// ActorProgram is the supervised program: it completed its handshake.
	ActorProgram

	// ActorHealthCheck is section 18's first named actor.
	ActorHealthCheck

	// ActorRestartBudget is section 18's second.
	ActorRestartBudget

	// ActorInvoker is section 18's third: panic recovery (section 5j).
	ActorInvoker

	// ActorHuman is section 18's fourth, and the ONLY one that may leave
	// QUARANTINED.
	ActorHuman
)

var actorNames = map[Actor]string{
	ActorUnspecified:   "UNSPECIFIED",
	ActorRig:           "rig",
	ActorProgram:       "the program",
	ActorHealthCheck:   "the health check",
	ActorRestartBudget: "the restart budget",
	ActorInvoker:       "the invoker",
	ActorHuman:         "a human",
}

func (a Actor) String() string {
	if n, ok := actorNames[a]; ok {
		return n
	}
	return fmt.Sprintf("Actor(%d)", uint8(a))
}

// Trigger is what happened. It is part of the table's identity rather than a
// label: section 18 gives a trigger per row, and two rows into the same state
// differ by nothing else.
type Trigger uint8

const (
	// TriggerUnspecified is the zero, and it is never legal.
	TriggerUnspecified Trigger = iota

	// TriggerLaunch - rig launched the child, or a program connected unbidden.
	TriggerLaunch

	// TriggerRegistered - handshake and declaration validated.
	TriggerRegistered

	// TriggerRegistrationFailed - any registration step failed.
	TriggerRegistrationFailed

	// TriggerHealthFailures - three health failures, or a missed deadline.
	TriggerHealthFailures

	// TriggerHealthRecovered - health recovers.
	TriggerHealthRecovered

	// TriggerBudgetRestart - five failures: the budget takes the program.
	TriggerBudgetRestart

	// TriggerBackoffElapsed - the backoff ran out and budget remains.
	TriggerBackoffElapsed

	// TriggerBudgetExhausted - the budget ran out.
	TriggerBudgetExhausted

	// TriggerPanickedTwice - a hosted program panicked twice in budget
	// (section 5j).
	TriggerPanickedTwice

	// TriggerManualRestart - a human asked for it, and only a human can.
	TriggerManualRestart
)

var triggerNames = map[Trigger]string{
	TriggerUnspecified:        "UNSPECIFIED",
	TriggerLaunch:             "launch",
	TriggerRegistered:         "handshake and declaration validated",
	TriggerRegistrationFailed: "a registration step failed",
	TriggerHealthFailures:     "three health failures, or a missed deadline",
	TriggerHealthRecovered:    "health recovered",
	TriggerBudgetRestart:      "five failures",
	TriggerBackoffElapsed:     "backoff elapsed, budget remains",
	TriggerBudgetExhausted:    "budget exhausted",
	TriggerPanickedTwice:      "panicked twice inside the budget",
	TriggerManualRestart:      "a manual restart",
}

func (t Trigger) String() string {
	if n, ok := triggerNames[t]; ok {
		return n
	}
	return fmt.Sprintf("Trigger(%d)", uint8(t))
}

// edge is one row of section 18's transition table.
type edge struct {
	from    State
	trigger Trigger
}

// Transition is what an edge resolves to.
type Transition struct {
	From    State
	To      State
	Trigger Trigger
	Actor   Actor
}

// table IS PLAN.md section 18's transition table, row for row, and it is the
// whole of what is legal.
//
// ⛔ A ROW IS KEYED BY (from, trigger) AND NOT BY (from, to). The section's
// own reason for having a table at all is that "*inferable* transitions are
// exactly the ones nobody checks are total" - so the caller says what
// HAPPENED and the table says where that lands. A caller naming the
// destination would be inferring, which is the thing this replaces.
//
// anyState is the one row that is not keyed by a state: "any -> QUARANTINED,
// a hosted program panics twice in budget". It is looked up after a miss.
var table = map[edge]Transition{
	{StateUnspecified, TriggerLaunch}: {
		From: StateUnspecified, To: StateStarting,
		Trigger: TriggerLaunch, Actor: ActorRig,
	},
	{StateStarting, TriggerRegistered}: {
		From: StateStarting, To: StateHealthy,
		Trigger: TriggerRegistered, Actor: ActorProgram,
	},
	{StateStarting, TriggerRegistrationFailed}: {
		From: StateStarting, To: StateQuarantined,
		Trigger: TriggerRegistrationFailed, Actor: ActorRig,
	},
	{StateHealthy, TriggerHealthFailures}: {
		From: StateHealthy, To: StateDegraded,
		Trigger: TriggerHealthFailures, Actor: ActorHealthCheck,
	},
	{StateDegraded, TriggerHealthRecovered}: {
		From: StateDegraded, To: StateHealthy,
		Trigger: TriggerHealthRecovered, Actor: ActorHealthCheck,
	},
	{StateDegraded, TriggerBudgetRestart}: {
		From: StateDegraded, To: StateRestarting,
		Trigger: TriggerBudgetRestart, Actor: ActorRestartBudget,
	},
	{StateRestarting, TriggerBackoffElapsed}: {
		From: StateRestarting, To: StateStarting,
		Trigger: TriggerBackoffElapsed, Actor: ActorRestartBudget,
	},
	{StateRestarting, TriggerBudgetExhausted}: {
		From: StateRestarting, To: StateQuarantined,
		Trigger: TriggerBudgetExhausted, Actor: ActorRestartBudget,
	},
	{StateQuarantined, TriggerManualRestart}: {
		From: StateQuarantined, To: StateStarting,
		Trigger: TriggerManualRestart, Actor: ActorHuman,
	},
}

// panicTwice is section 18's "any -> QUARANTINED" row. It is held apart
// rather than written out five times because writing it out five times is how
// a sixth state would one day get four of them.
var panicTwice = Transition{
	To: StateQuarantined, Trigger: TriggerPanickedTwice, Actor: ActorInvoker,
}

// FaultError is an attempted illegal transition.
//
// PLAN.md section 18: "An attempted illegal transition is not ignored and not
// best-guessed: it is recorded as a supervisor fault and the program is
// quarantined, because a supervisor that cannot account for its own state is
// the one component whose confusion is not survivable."
//
// So it is an error type rather than a bool: the reason has to reach the
// history a human reads after the quarantine, and a bool cannot carry it.
type FaultError struct {
	Program string
	From    State
	Trigger Trigger
}

func (f *FaultError) Error() string {
	return fmt.Sprintf(
		"supervisor fault: program %q is %s and nothing in section 18's table "+
			"moves it on %q. Legal from %s: %s",
		f.Program, f.From, f.Trigger, f.From, strings.Join(legalFrom(f.From), ", "))
}

// legalFrom lists the triggers the table accepts in a state, so the fault
// says what WOULD have been legal rather than only what was not.
func legalFrom(s State) []string {
	var out []string
	for e := range table {
		if e.from == s {
			out = append(out, e.trigger.String())
		}
	}
	out = append(out, panicTwice.Trigger.String())
	sort.Strings(out)
	return out
}

// Resolve answers section 18's table: from this state, on this trigger, where
// does the program go and who moved it.
//
// A trigger the table does not carry from this state returns a *Fault, which
// the caller quarantines on. Nothing here guesses.
func Resolve(program string, from State, trigger Trigger) (Transition, error) {
	if trigger == TriggerPanickedTwice {
		// The one row keyed by "any". It is legal from every state
		// INCLUDING quarantined, where it is a no-op that still records:
		// section 5j says twice in budget and it stays quarantined.
		t := panicTwice
		t.From = from
		return t, nil
	}
	if t, ok := table[edge{from, trigger}]; ok {
		return t, nil
	}
	return Transition{}, &FaultError{Program: program, From: from, Trigger: trigger}
}

// States lists the five in the order section 18's table gives them, for any
// surface that renders the set.
func States() []State {
	return []State{
		StateStarting, StateHealthy, StateDegraded,
		StateRestarting, StateQuarantined,
	}
}

// Table returns section 18's rows, sorted, for the test that pins them and
// for any surface that wants to show what is legal.
func Table() []Transition {
	out := make([]Transition, 0, len(table)+1)
	for _, t := range table {
		out = append(out, t)
	}
	out = append(out, panicTwice)
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].Trigger < out[j].Trigger
	})
	return out
}
