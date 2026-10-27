package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// clock is a desk's time, moved by the test.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// grantedDesk is a desk whose seat "a" holds a live grant of the given
// answer, with the clock the test moves and the events the desk audited.
func grantedDesk(t *testing.T, a registryv1.HandAction) (*handDesk, *clock, *[]string) {
	t.Helper()
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	h := newHandDesk(clk.now)
	var mu sync.Mutex
	var events []string
	h.audit = func(op string, attrs map[string]string) {
		mu.Lock()
		events = append(events, op+" "+attrs["seat"]+" "+attrs["how"])
		mu.Unlock()
	}
	g, err := h.grantAsk("session-a", "a", "three scripts", "Editor", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.answer(a); err != nil {
		t.Fatal(err)
	}
	if st, err := h.grantAnswered(context.Background(), g); err != nil || st.GetGrantPhase() != registryv1.HandGrantPhase_HAND_GRANT_PHASE_LIVE {
		t.Fatalf("grant not live: %v %v", st, err)
	}
	return h, clk, &events
}

func grantPhase(h *handDesk) registryv1.HandGrantPhase {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshot().GetGrantPhase()
}

// G3: inside the window the seat's runs drive at once, one prompt in all, and
// another seat still gets its own countdown.
func TestGrantedSeatRunsWithoutACountdown(t *testing.T) {
	h, _, _ := grantedDesk(t, registryv1.HandAction_HAND_ACTION_GRANT_30S)
	for i := range 3 {
		run, err := h.ask("righand-conn", "righand", "a", "script", 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if p := phase(h); p != registryv1.HandPhase_HAND_PHASE_DRIVING || !run.underGrant {
			t.Fatalf("run %d: phase %s, underGrant %v; want driving under the grant", i, p, run.underGrant)
		}
		if st, err := h.step(context.Background(), "righand-conn", "click 1", "", nil); err != nil || st.GetGrantSeat() != "a" {
			t.Fatalf("step: %v %v", st, err)
		}
		h.release("righand-conn")
		if grantPhase(h) != registryv1.HandGrantPhase_HAND_GRANT_PHASE_LIVE {
			t.Fatal("the grant must outlive a script")
		}
	}
	if _, err := h.ask("other-conn", "righand", "b", "script", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if p := phase(h); p != registryv1.HandPhase_HAND_PHASE_ASKING {
		t.Fatalf("another seat's run is %s; want its own countdown", p)
	}
}

// G4, G5: at its deadline the grant ends, a running script finishes its step
// and is stopped at the next, and the next script asks again.
func TestGrantExpiryEndsTheRunAtItsNextStep(t *testing.T) {
	h, clk, events := grantedDesk(t, registryv1.HandAction_HAND_ACTION_GRANT_30S)
	if _, err := h.ask("c", "righand", "a", "script", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := h.step(context.Background(), "c", "step 1", "", nil); err != nil {
		t.Fatal(err)
	}
	clk.add(31 * time.Second)
	h.mu.Lock()
	h.expire()
	left := h.run != nil
	h.mu.Unlock()
	if !left {
		t.Fatal("the run was cut at expiry; it must finish its step")
	}
	_, err := h.step(context.Background(), "c", "step 2", "", nil)
	refusedWith(t, err, rigv1.Code_CODE_DENIED, "the grant ran out")
	if grantPhase(h) != registryv1.HandGrantPhase_HAND_GRANT_PHASE_NONE {
		t.Fatal("an expired grant is still shown")
	}
	if _, err := h.ask("c2", "righand", "a", "fourth", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if p := phase(h); p != registryv1.HandPhase_HAND_PHASE_ASKING {
		t.Fatalf("the fourth script is %s after expiry; want a countdown", p)
	}
	for _, want := range []string{"grant asked a ", "grant live a ", "run started a ", "grant ended a ran out", "run ended a the grant ran out"} {
		found := false
		for _, e := range *events {
			found = found || e == want
		}
		if !found {
			t.Errorf("audit lacks %q: %q", want, *events)
		}
	}
}

// G4: stop and pause take the window back at once, with or without a run.
func TestStopAndPauseRevokeTheGrant(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  registryv1.HandAction
		run  bool
	}{
		{"stop idle", registryv1.HandAction_HAND_ACTION_STOP, false},
		{"pause idle", registryv1.HandAction_HAND_ACTION_PAUSE, false},
		{"stop running", registryv1.HandAction_HAND_ACTION_STOP, true},
		{"pause running", registryv1.HandAction_HAND_ACTION_PAUSE, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := grantedDesk(t, registryv1.HandAction_HAND_ACTION_GRANT_5M)
			if tc.run {
				if _, err := h.ask("c", "righand", "a", "script", 20*time.Second); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := h.answer(tc.act); err != nil {
				t.Fatal(err)
			}
			if grantPhase(h) != registryv1.HandGrantPhase_HAND_GRANT_PHASE_NONE {
				t.Fatal("the grant survived")
			}
			if _, err := h.ask("c2", "righand", "a", "next", 20*time.Second); tc.run && err == nil && tc.act == registryv1.HandAction_HAND_ACTION_PAUSE {
				t.Fatal("a paused run still holds the desktop")
			}
		})
	}
}

// G2: no verb the seat can reach makes a grant live, longer or renewed.
func TestASeatCannotGrantExtendOrRenewItsOwn(t *testing.T) {
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	h := newHandDesk(clk.now)
	g, err := h.grantAsk(nil, "a", "r", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := h.grantAnswered(ctx, g); err == nil {
		t.Fatal("an unanswered ask came back granted")
	}
	if grantPhase(h) != registryv1.HandGrantPhase_HAND_GRANT_PHASE_NONE {
		t.Fatal("a caller that left must withdraw its ask")
	}
	// Live: asking again is refused, so there is no renewal.
	h, _, _ = grantedDesk(t, registryv1.HandAction_HAND_ACTION_GRANT_30S)
	_, err = h.grantAsk(nil, "a", "again", "", time.Minute)
	refusedWith(t, err, rigv1.Code_CODE_CONFLICT, "never renewed")
	// And another seat cannot give it back.
	_, err = h.ungrant("b")
	refusedWith(t, err, rigv1.Code_CODE_DENIED, "only that seat")
}

// G2 on the wire: a terminal asks and blocks, a program is refused every
// answering word (the new ones included) and a seatless program cannot ask,
// and only the person's terminal makes it live.
func TestOnlyHeAnswersAGrant(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	prog := program(t, sock, "handprog")
	asker, person := dial(t, sock), dial(t, sock)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := prog.Call(ctx, "rig.hand.grant", &registryv1.HandGrantRequest{Reason: "r", DurationMs: 30000}, &registryv1.HandGrantResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a program with no seat asking")

	granted := make(chan error, 1)
	var live registryv1.HandGrantResponse
	go func() {
		granted <- asker.Call(ctx, "rig.hand.grant", &registryv1.HandGrantRequest{Reason: "demo", DurationMs: 30000, Scope: "Editor"}, &live)
	}()
	var w registryv1.HandWaitResponse
	for w.GetState().GetGrantPhase() != registryv1.HandGrantPhase_HAND_GRANT_PHASE_ASKING {
		if err := person.Call(ctx, "rig.hand.wait", &registryv1.HandWaitRequest{After: w.GetState().GetSeq(), TimeoutMs: 2000}, &w); err != nil {
			t.Fatal(err)
		}
	}
	if w.GetState().GetGrantReason() != "demo" || w.GetState().GetGrantScope() != "Editor" {
		t.Fatalf("state %v", w.GetState())
	}
	for _, act := range []registryv1.HandAction{registryv1.HandAction_HAND_ACTION_GRANT_30S, registryv1.HandAction_HAND_ACTION_GRANT_5M} {
		err = prog.Call(ctx, "rig.hand.answer", &registryv1.HandAnswerRequest{Action: act}, &registryv1.HandAnswerResponse{})
		wantCode(t, err, rigv1.Code_CODE_DENIED, "a program answering "+act.String())
	}
	select {
	case err := <-granted:
		t.Fatalf("the ask was answered without him: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := person.Call(ctx, "rig.hand.answer", &registryv1.HandAnswerRequest{Action: registryv1.HandAction_HAND_ACTION_GRANT_60S}, &registryv1.HandAnswerResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := <-granted; err != nil {
		t.Fatalf("grant: %v", err)
	}
	if st := live.GetState(); st.GetGrantPhase() != registryv1.HandGrantPhase_HAND_GRANT_PHASE_LIVE || st.GetGrantWindowMs() != 60000 {
		t.Fatalf("live state %v; want his 60 seconds, not the 30 asked", st)
	}
}

// G2: silence declines.
func TestAnUnansweredGrantLapsesAsDeclined(t *testing.T) {
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	h := newHandDesk(clk.now)
	g, err := h.grantAsk(nil, "a", "r", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	clk.add(handGrantAsk + time.Second)
	_, err = h.grantAnswered(context.Background(), g)
	refusedWith(t, err, rigv1.Code_CODE_DENIED, "not answered")
}

// G4: the session's end ends the grant and cuts its run; another session's end
// does not.
func TestAGrantEndsWithItsSession(t *testing.T) {
	h, _, _ := grantedDesk(t, registryv1.HandAction_HAND_ACTION_GRANT_60S)
	if _, err := h.ask("c", "righand", "a", "script", 20*time.Second); err != nil {
		t.Fatal(err)
	}
	h.dropGrant("session-b")
	if grantPhase(h) != registryv1.HandGrantPhase_HAND_GRANT_PHASE_LIVE {
		t.Fatal("another session's end took the grant")
	}
	h.dropGrant("session-a")
	if grantPhase(h) != registryv1.HandGrantPhase_HAND_GRANT_PHASE_NONE {
		t.Fatal("the grant outlived its session")
	}
	_, err := h.step(context.Background(), "c", "next", "", nil)
	refusedWith(t, err, rigv1.Code_CODE_DENIED, "the session left")
}
