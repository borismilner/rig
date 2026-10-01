package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// handDesk4 is a desk with short bounds, and a run asked for through it.
func handDesk4(t *testing.T, window time.Duration) (*handDesk, chan error) {
	t.Helper()
	h := newHandDesk(time.Now)
	h.holdMax, h.pauseMax = 150*time.Millisecond, 150*time.Millisecond
	run, err := h.ask("prog", "righand", "3 steps", window)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := h.granted(context.Background(), run)
		done <- err
	}()
	return h, done
}

func phase(h *handDesk) registryv1.HandPhase {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.st.GetPhase()
}

func within(t *testing.T, done <-chan error, d time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatal("still blocked")
		return nil
	}
}

func refusedWith(t *testing.T, err error, code rigv1.Code, words string) {
	t.Helper()
	var he *handError
	if !errors.As(err, &he) || he.code != code || !strings.Contains(he.msg, words) {
		t.Fatalf("got %v, want %s with %q", err, code, words)
	}
}

// H1: silence is consent.
func TestSilenceAtTheCountdownStartsTheRun(t *testing.T) {
	h, done := handDesk4(t, 80*time.Millisecond)
	start := time.Now()
	if err := within(t, done, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 60*time.Millisecond {
		t.Fatal("drove before the countdown ran out")
	}
	if phase(h) != registryv1.HandPhase_HAND_PHASE_DRIVING {
		t.Fatalf("phase %s", phase(h))
	}
}

// H2: he declines, and the program is refused.
func TestADeclinedCountdownRefusesTheProgram(t *testing.T) {
	h, done := handDesk4(t, 5*time.Second)
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_DECLINE); err != nil {
		t.Fatal(err)
	}
	refusedWith(t, within(t, done, time.Second), rigv1.Code_CODE_DENIED, handDeclined)
	if _, err := h.step(context.Background(), "prog", "x"); err == nil {
		t.Fatal("a declined run took a step")
	}
}

// H2: a held countdown never starts by itself, and letting it run carries
// on from where it was held.
func TestAHeldCountdownWaitsAndRunsOnFromWhereItWas(t *testing.T) {
	h, done := handDesk4(t, 300*time.Millisecond)
	h.holdMax = 5 * time.Second
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_HOLD); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if phase(h) != registryv1.HandPhase_HAND_PHASE_HELD {
		t.Fatalf("held countdown went to %s", phase(h))
	}
	st, err := h.answer(registryv1.HandAction_HAND_ACTION_RESUME)
	if err != nil || st.GetPhase() != registryv1.HandPhase_HAND_PHASE_ASKING || st.GetLeftMs() < 200 {
		t.Fatalf("resume: %v %v", st, err)
	}
	start := time.Now()
	if err := within(t, done, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Fatal("the rest of the countdown was skipped")
	}
}

func TestAHoldPastItsBoundIsADecline(t *testing.T) {
	h, done := handDesk4(t, 5*time.Second)
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_HOLD); err != nil {
		t.Fatal(err)
	}
	refusedWith(t, within(t, done, 2*time.Second), rigv1.Code_CODE_DEADLINE, handHeldLong)
}

func TestAllowStartsAtOnce(t *testing.T) {
	h, done := handDesk4(t, 5*time.Second)
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_ALLOW); err != nil {
		t.Fatal(err)
	}
	if err := within(t, done, time.Second); err != nil {
		t.Fatal(err)
	}
}

// H4: taking the desktop back blocks the next step until he resumes, and
// stop refuses it.
func TestTakeBackBlocksTheStepAndStopRefusesIt(t *testing.T) {
	h, done := handDesk4(t, 5*time.Second)
	h.pauseMax = 5 * time.Second
	_, _ = h.answer(registryv1.HandAction_HAND_ACTION_ALLOW)
	_ = within(t, done, time.Second)
	ctx := context.Background()
	if st, err := h.step(ctx, "prog", "step 1 of 3: click"); err != nil || st.GetActivity() != "step 1 of 3: click" {
		t.Fatalf("step: %v %v", st, err)
	}
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_PAUSE); err != nil {
		t.Fatal(err)
	}
	stepped := make(chan error, 1)
	go func() { _, err := h.step(ctx, "prog", "step 2 of 3: type"); stepped <- err }()
	select {
	case err := <-stepped:
		t.Fatalf("a step ran while he had the desktop: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_RESUME); err != nil {
		t.Fatal(err)
	}
	if err := within(t, stepped, time.Second); err != nil {
		t.Fatal(err)
	}
	_, _ = h.answer(registryv1.HandAction_HAND_ACTION_PAUSE)
	go func() { _, err := h.step(ctx, "prog", "step 3 of 3: key"); stepped <- err }()
	time.Sleep(50 * time.Millisecond)
	if _, err := h.answer(registryv1.HandAction_HAND_ACTION_STOP); err != nil {
		t.Fatal(err)
	}
	refusedWith(t, within(t, stepped, time.Second), rigv1.Code_CODE_DENIED, handStopped)
	if st := h.release("prog"); st.GetEnded() != handStopped {
		t.Fatalf("release after stop rewrote the ending: %v", st)
	}
}

func TestAPausePastItsBoundStopsTheRun(t *testing.T) {
	h, done := handDesk4(t, 5*time.Second)
	_, _ = h.answer(registryv1.HandAction_HAND_ACTION_ALLOW)
	_ = within(t, done, time.Second)
	_, _ = h.answer(registryv1.HandAction_HAND_ACTION_PAUSE)
	_, err := h.step(context.Background(), "prog", "x")
	refusedWith(t, err, rigv1.Code_CODE_DEADLINE, handPausedLong)
}

func TestOneRunAtATimeAndItEndsWithItsConnection(t *testing.T) {
	h, done := handDesk4(t, 5*time.Second)
	if _, err := h.ask("other", "x", "y", time.Second); err == nil {
		t.Fatal("a second run was let in")
	}
	if _, err := h.step(context.Background(), "other", "x"); err == nil {
		t.Fatal("a stranger stepped")
	}
	h.drop("prog")
	refusedWith(t, within(t, done, time.Second), rigv1.Code_CODE_UNAVAILABLE, handLeft)
	if _, err := h.ask("other", "x", "y", time.Second); err != nil {
		t.Fatalf("the desktop stayed held after its program left: %v", err)
	}
}

func TestAnAnswerThePhaseDoesNotTakeIsRefused(t *testing.T) {
	h := newHandDesk(time.Now)
	for _, a := range []registryv1.HandAction{
		registryv1.HandAction_HAND_ACTION_ALLOW, registryv1.HandAction_HAND_ACTION_PAUSE,
		registryv1.HandAction_HAND_ACTION_STOP, registryv1.HandAction_HAND_ACTION_UNSPECIFIED,
	} {
		_, err := h.answer(a)
		refusedWith(t, err, rigv1.Code_CODE_CONFLICT, "does not apply")
	}
}

func TestTheStripWakesOnAChangeAndOnADeadline(t *testing.T) {
	h := newHandDesk(time.Now)
	ctx := context.Background()
	st, _ := h.after(ctx, 0, 0)
	got := make(chan uint64, 1)
	go func() { s, _ := h.after(ctx, st.GetSeq(), 5*time.Second); got <- s.GetSeq() }()
	time.Sleep(30 * time.Millisecond)
	_, _ = h.ask("p", "x", "y", 100*time.Millisecond)
	if s := <-got; s <= st.GetSeq() {
		t.Fatal("did not wake on the ask")
	}
	// The countdown runs out with nobody else calling: the waiter itself
	// must see it, or the strip shows a countdown below zero.
	s, _ := h.after(ctx, st.GetSeq()+1, 2*time.Second)
	if s.GetPhase() != registryv1.HandPhase_HAND_PHASE_DRIVING {
		t.Fatalf("after the deadline the strip saw %s", s.GetPhase())
	}
}

func TestClipCutsOnARune(t *testing.T) {
	s := strings.Repeat("א", 150) // two bytes each
	got := clip(s)
	if !strings.HasSuffix(got, "…") || len(got) > maxHandText+len("…") || strings.ContainsRune(got, '�') {
		t.Fatalf("clip: %q", got)
	}
}

// H5 on the wire: a program never answers; a terminal does. A countdown
// below ten seconds is refused, and the holder comes off the connection.
func TestOnlyATerminalAnswersAndTheHolderIsTheConnection(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	prog := program(t, sock, "handprog")
	term := dial(t, sock)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := prog.Call(ctx, "rig.hand.request", &registryv1.HandRequestRequest{CountdownMs: 500}, &registryv1.HandRequestResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a 500ms countdown")

	granted := make(chan error, 1)
	go func() {
		granted <- prog.Call(ctx, "rig.hand.request", &registryv1.HandRequestRequest{Reason: "demo"}, &registryv1.HandRequestResponse{})
	}()
	var w registryv1.HandWaitResponse
	for w.GetState().GetPhase() != registryv1.HandPhase_HAND_PHASE_ASKING {
		if err := term.Call(ctx, "rig.hand.wait", &registryv1.HandWaitRequest{After: w.GetState().GetSeq(), TimeoutMs: 2000}, &w); err != nil {
			t.Fatal(err)
		}
	}
	if w.GetState().GetHolder() != "handprog" || w.GetState().GetReason() != "demo" {
		t.Fatalf("state %v", w.GetState())
	}
	err = prog.Call(ctx, "rig.hand.answer", &registryv1.HandAnswerRequest{Action: registryv1.HandAction_HAND_ACTION_ALLOW}, &registryv1.HandAnswerResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a program answering")
	if err := term.Call(ctx, "rig.hand.answer", &registryv1.HandAnswerRequest{Action: registryv1.HandAction_HAND_ACTION_ALLOW}, &registryv1.HandAnswerResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := <-granted; err != nil {
		t.Fatalf("granted: %v", err)
	}
	if err := prog.Call(ctx, "rig.hand.step", &registryv1.HandStepRequest{Activity: "step 1 of 1: wait"}, &registryv1.HandStepResponse{}); err != nil {
		t.Fatal(err)
	}
	// The program's connection closing ends its run.
	_ = prog.Close()
	for w.GetState().GetPhase() != registryv1.HandPhase_HAND_PHASE_IDLE {
		if err := term.Call(ctx, "rig.hand.wait", &registryv1.HandWaitRequest{After: w.GetState().GetSeq(), TimeoutMs: 2000}, &w); err != nil {
			t.Fatal(err)
		}
	}
	if w.GetState().GetEnded() != handLeft {
		t.Fatalf("ended %q", w.GetState().GetEnded())
	}
}
