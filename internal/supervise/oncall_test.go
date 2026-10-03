package supervise

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Section 54: a program started when it is called.

func onCallSpec(id string) Spec {
	s := testSpec(id)
	s.OnCall = true
	return s
}

func resting(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, onCallSpec("beacon"))
	t.Cleanup(h.sup.StopAll)
	if err := h.sup.Rest("beacon"); err != nil {
		t.Fatal(err)
	}
	h.mustState("beacon", StateAtRest)
	return h
}

func (h *harness) call() (<-chan error, func()) {
	h.t.Helper()
	ready, done, err := h.sup.Call("beacon")
	if err != nil {
		h.t.Fatalf("call beacon: %v", err)
	}
	return ready, done
}

func answered(t *testing.T, ready <-chan error) error {
	t.Helper()
	select {
	case err := <-ready:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("the call was never answered")
		return nil
	}
}

// At rest nothing runs; ten first calls at once launch ONE process, and all
// ten are released by its registration.
func TestTenCallsAtRestShareOneStart(t *testing.T) {
	h := resting(t)
	if h.startCount("beacon") != 0 {
		t.Fatal("a program at rest was started")
	}
	var wg sync.WaitGroup
	readies := make(chan (<-chan error), 10)
	for range 10 {
		wg.Go(func() {
			ready, done := h.call()
			defer done()
			readies <- ready
		})
	}
	wg.Wait()
	close(readies)
	if n := h.startCount("beacon"); n != 1 {
		t.Fatalf("ten calls launched %d processes", n)
	}
	h.mustState("beacon", StateStarting)
	if err := h.sup.Registered("beacon"); err != nil {
		t.Fatal(err)
	}
	for ready := range readies {
		if err := answered(t, ready); err != nil {
			t.Fatalf("a waiting call was told %v", err)
		}
	}
}

// Its normal end: exit 0 with no call in flight is AT_REST and costs no
// budget, and the next call starts it again.
func TestACleanIdleExitIsRestNotACrash(t *testing.T) {
	h := resting(t)
	_, done := h.call()
	if err := h.sup.Registered("beacon"); err != nil {
		t.Fatal(err)
	}
	done()
	h.crash("beacon", Exit{Code: 0})
	h.mustState("beacon", StateAtRest)
	if st := h.status("beacon"); st.Restarts != 0 {
		t.Fatalf("an idle exit was counted as %d failures", st.Restarts)
	}
	ready, done := h.call()
	defer done()
	if h.startCount("beacon") != 2 {
		t.Fatal("the call after an idle exit did not start it again")
	}
	_ = h.sup.Registered("beacon")
	if err := answered(t, ready); err != nil {
		t.Fatal(err)
	}
}

// RED CONTROL: the same end with exit 1 is a crash, and exit 0 with a call in
// flight is one too - so the test above measures the exit, not the state.
func TestACrashOrAnExitDuringACallCountsAgainstTheBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exit     Exit
		inFlight bool
	}{
		{"exit 1", Exit{Code: 1}, false},
		{"exit 0 during a call", Exit{Code: 0}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := resting(t)
			_, done := h.call()
			_ = h.sup.Registered("beacon")
			if !tc.inFlight {
				done()
			}
			h.crash("beacon", tc.exit)
			st := h.status("beacon")
			if st.State != StateAtRest || st.Restarts != 1 {
				t.Fatalf("%s left it %s with %d failures, want AT_REST with 1", tc.name, st.State, st.Restarts)
			}
			if h.startCount("beacon") != 1 {
				t.Fatal("rig relaunched a crashed on-call program nobody was calling")
			}
			done()
		})
	}
}

// A start that fails tells the waiting call why, with what the child said;
// the budget counts it, and past the budget a call is refused, not started.
func TestAFailedStartAnswersTheCallAndTheBudgetQuarantines(t *testing.T) {
	h := resting(t)
	ready, done := h.call()
	h.crash("beacon", Exit{Code: 2})
	done()
	var se *StartError
	if err := answered(t, ready); !errors.As(err, &se) || !strings.Contains(se.Reason, "exit 2") {
		t.Fatalf("the waiting call was told %v", err)
	}
	h.mustState("beacon", StateAtRest)

	// The second failure is the whole budget of two: a registration that
	// never comes.
	ready, done = h.call()
	h.clk.advance(11 * time.Second)
	h.sup.Tick()
	done()
	if err := answered(t, ready); !errors.As(err, &se) || !strings.Contains(se.Reason, "did not register") {
		t.Fatalf("a start that never registered was told %v", err)
	}
	h.mustState("beacon", StateQuarantined)
	if _, _, err := h.sup.Call("beacon"); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("a quarantined on-call program was called: %v", err)
	}
	if n := h.startCount("beacon"); n != 2 {
		t.Fatalf("%d starts, want the two that failed", n)
	}
}

// A child that would not exec is the same failed start.
func TestAChildThatWillNotExecFailsTheCall(t *testing.T) {
	h := resting(t)
	h.mu.Lock()
	h.refuse["beacon"] = errors.New("no such file")
	h.mu.Unlock()
	ready, done := h.call()
	defer done()
	if err := answered(t, ready); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("told %v", err)
	}
	h.mustState("beacon", StateAtRest)
}

// The declare run: started to read its declaration, then put to rest - unless
// a call came for it meanwhile, which keeps it.
func TestRestStopsADeclareRunUnlessACallCame(t *testing.T) {
	h := newHarness(t, onCallSpec("beacon"))
	defer h.sup.StopAll()
	h.up("beacon")
	if err := h.sup.Rest("beacon"); err != nil {
		t.Fatal(err)
	}
	h.mustState("beacon", StateAtRest)
	if !h.proc("beacon").wasStopped() {
		t.Fatal("the declare run's child was left running")
	}

	h.up("beacon")
	_, done := h.call()
	defer done()
	_ = h.sup.Rest("beacon")
	h.mustState("beacon", StateHealthy)
}

// A stop is rest, not off the table: the program still starts when called.
func TestStoppingAnOnCallProgramLeavesItAtRest(t *testing.T) {
	h := resting(t)
	_, done := h.call()
	_ = h.sup.Registered("beacon")
	done()
	if err := h.sup.Stop("beacon"); err != nil {
		t.Fatal(err)
	}
	h.mustState("beacon", StateAtRest)
}

func TestAutostartAndOnCallAreRefusedTogether(t *testing.T) {
	s := onCallSpec("beacon")
	s.Autostart = true
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "contradict") {
		t.Fatalf("validated %v", err)
	}
}

// Calling a program not declared on call is refused, so the daemon cannot
// start an ordinary program by accident.
func TestCallRefusesAProgramNotOnCall(t *testing.T) {
	h := newHarness(t)
	if _, _, err := h.sup.Call("app"); !errors.Is(err, ErrNotOnCall) {
		t.Fatalf("got %v", err)
	}
}

func TestProgramsJSONReadsOnCallAndRefusesItWithAutostart(t *testing.T) {
	specs, err := Load(write(t, `{"programs": [{"id": "beacon", "path": "/usr/bin/beacon", "on_call": true}]}`))
	if err != nil || len(specs) != 1 || !specs[0].OnCall {
		t.Fatalf("read %+v, %v", specs, err)
	}
	if _, err := Load(write(t, `{"programs": [{"id": "beacon", "path": "/usr/bin/beacon", "on_call": true, "autostart": true}]}`)); err == nil {
		t.Fatal("on_call with autostart was accepted")
	}
}
