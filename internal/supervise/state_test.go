package supervise

import (
	"errors"
	"strings"
	"testing"
)

// The table is the specification, so the test asserts the specification
// rather than the code: every row of PLAN.md section 18 is written out here
// independently, and a table that drifts from the section fails here first.
func TestEveryRowOfSectionEighteensTable(t *testing.T) {
	rows := []struct {
		from    State
		trigger Trigger
		to      State
		actor   Actor
	}{
		{StateUnspecified, TriggerLaunch, StateStarting, ActorRig},
		{StateStarting, TriggerRegistered, StateHealthy, ActorProgram},
		{StateStarting, TriggerRegistrationFailed, StateQuarantined, ActorRig},
		{StateHealthy, TriggerHealthFailures, StateDegraded, ActorHealthCheck},
		{StateDegraded, TriggerHealthRecovered, StateHealthy, ActorHealthCheck},
		{StateDegraded, TriggerBudgetRestart, StateRestarting, ActorRestartBudget},
		{StateRestarting, TriggerBackoffElapsed, StateStarting, ActorRestartBudget},
		{StateRestarting, TriggerBudgetExhausted, StateQuarantined, ActorRestartBudget},
		{StateQuarantined, TriggerManualRestart, StateStarting, ActorHuman},
	}
	for _, r := range rows {
		got, err := Resolve("p", r.from, r.trigger)
		if err != nil {
			t.Fatalf("%s on %q: %v", r.from, r.trigger, err)
		}
		if got.To != r.to || got.Actor != r.actor {
			t.Errorf("%s on %q went to %s caused by %s, section 18 says %s caused by %s",
				r.from, r.trigger, got.To, got.Actor, r.to, r.actor)
		}
	}
	if len(Table()) != len(rows)+1 {
		t.Errorf("the table has %d rows and section 18 has %d, the panic row included",
			len(Table()), len(rows)+1)
	}
}

// Section 18: "any -> QUARANTINED, a hosted program panics twice in budget,
// caused by the invoker". Every state, including QUARANTINED itself, where
// section 5j says it stays.
func TestAPanicTwiceQuarantinesFromAnyState(t *testing.T) {
	for _, s := range append(States(), StateUnspecified) {
		got, err := Resolve("p", s, TriggerPanickedTwice)
		if err != nil {
			t.Fatalf("panic twice from %s: %v", s, err)
		}
		if got.To != StateQuarantined || got.Actor != ActorInvoker {
			t.Errorf("panic twice from %s went to %s caused by %s, want QUARANTINED caused by the invoker",
				s, got.To, got.Actor)
		}
		if got.From != s {
			t.Errorf("panic twice from %s reported From=%s", s, got.From)
		}
	}
}

// "Everything not in the table above is illegal." The test walks the whole
// product of states and triggers and checks that exactly the rows above are
// accepted - which is the only way to assert totality rather than assert the
// rows one more time.
func TestEverythingNotInTheTableIsAFault(t *testing.T) {
	legal := map[edge]bool{}
	for _, tr := range Table() {
		if tr.Trigger == TriggerPanickedTwice {
			continue
		}
		legal[edge{tr.From, tr.Trigger}] = true
	}
	allStates := append(States(), StateUnspecified)
	allTriggers := []Trigger{
		TriggerUnspecified, TriggerLaunch, TriggerRegistered,
		TriggerRegistrationFailed, TriggerHealthFailures, TriggerHealthRecovered,
		TriggerBudgetRestart, TriggerBackoffElapsed, TriggerBudgetExhausted,
		TriggerManualRestart,
	}
	for _, s := range allStates {
		for _, tg := range allTriggers {
			_, err := Resolve("p", s, tg)
			if legal[edge{s, tg}] {
				if err != nil {
					t.Errorf("%s on %q is in the table and was refused: %v", s, tg, err)
				}
				continue
			}
			if err == nil {
				t.Errorf("%s on %q is not in section 18's table and was allowed", s, tg)
			}
		}
	}
}

// A fault has to be readable by the person who finds the program
// quarantined, so it names the program, the state, the trigger and what would
// have been legal. A bare "invalid transition" is the thing section 18 calls
// unsurvivable confusion.
func TestAFaultSaysWhatWouldHaveBeenLegal(t *testing.T) {
	_, err := Resolve("shelf", StateHealthy, TriggerManualRestart)
	if err == nil {
		t.Fatal("a manual restart of a healthy program is not in the table and was allowed")
	}
	var f *FaultError
	if !errors.As(err, &f) {
		t.Fatalf("want a *FaultError, got %T", err)
	}
	msg := err.Error()
	for _, want := range []string{"shelf", "HEALTHY", "manual restart", "three health failures"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the fault does not mention %q: %s", want, msg)
		}
	}
}

// Section 21's rule one layer in: a zero that means something is banned, so
// every zero here reads as UNSPECIFIED rather than as the first real value.
func TestTheZeroOfEveryEnumIsUnspecified(t *testing.T) {
	if State(0) != StateUnspecified || State(0).String() != "UNSPECIFIED" {
		t.Error("State's zero is not UNSPECIFIED")
	}
	if Actor(0) != ActorUnspecified || Actor(0).String() != "UNSPECIFIED" {
		t.Error("Actor's zero is not UNSPECIFIED")
	}
	if Trigger(0) != TriggerUnspecified || Trigger(0).String() != "UNSPECIFIED" {
		t.Error("Trigger's zero is not UNSPECIFIED")
	}
	// And a value from the future prints as itself rather than as a
	// neighbour, so a log line from a newer peer is not misread.
	if got := State(99).String(); got != "State(99)" {
		t.Errorf("an unknown state printed as %q", got)
	}
}

// Only a human leaves QUARANTINED. The section puts it in bold and it is the
// row a future convenience would break first, so it gets its own test.
func TestOnlyAHumanLeavesQuarantine(t *testing.T) {
	for _, tr := range Table() {
		if tr.From == StateQuarantined && tr.To != StateQuarantined && tr.Actor != ActorHuman {
			t.Errorf("%q leaves QUARANTINED caused by %s, and section 18 says a human and only a human",
				tr.Trigger, tr.Actor)
		}
	}
}
