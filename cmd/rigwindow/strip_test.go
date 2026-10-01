package main

import (
	"errors"
	"testing"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// The countdown and the elapsed time are worked out from rigd's clock, and
// the strip quits only once the run has been idle for the linger.
func TestTheStripViewUsesRigdsClockAndLingers(t *testing.T) {
	now := time.Unix(1000, 0)
	f := &stripFeed{}
	f.set(&registryv1.HandState{
		Phase: registryv1.HandPhase_HAND_PHASE_ASKING, Holder: "righand", WindowMs: 20000,
		DeadlineUnixNano: now.Add(12 * time.Second).UnixNano(),
	}, now)
	if v := f.view(now); v.Phase != "asking" || v.LeftMs != 12000 || v.Quit {
		t.Fatalf("asking: %+v", v)
	}
	f.set(&registryv1.HandState{Phase: registryv1.HandPhase_HAND_PHASE_IDLE, Ended: "finished"}, now)
	if f.done(now.Add(stripLinger / 2)) {
		t.Fatal("quit before he could read how it ended")
	}
	if !f.done(now.Add(stripLinger + time.Millisecond)) {
		t.Fatal("never quit")
	}
}

// One strip per run, none for an idle state, and a strip that cannot start
// sends the countdown to the desktop's notifications.
func TestTheTrayStartsOneStripAndFallsBack(t *testing.T) {
	spawned, fell := 0, 0
	exited := make(chan error)
	w := &stripWatcher{
		spawn:    func() (<-chan error, error) { spawned++; return exited, nil },
		fallback: func(*registryv1.HandState) error { fell++; return nil },
		warn:     func(string) {},
	}
	asking := &registryv1.HandState{Phase: registryv1.HandPhase_HAND_PHASE_ASKING}
	w.seen(&registryv1.HandState{Phase: registryv1.HandPhase_HAND_PHASE_IDLE})
	w.seen(asking)
	w.seen(asking)
	if spawned != 1 {
		t.Fatalf("spawned %d", spawned)
	}
	w.spawn = func() (<-chan error, error) { return nil, errors.New("no display") }
	close(exited)
	for w.isRunning() {
		time.Sleep(time.Millisecond)
	}
	w.seen(asking)
	if fell != 1 {
		t.Fatalf("fell back %d times", fell)
	}
}

// H6: the strip is under a point only inside its own window.
func TestTheStripCoversOnlyItsWindow(t *testing.T) {
	s := &stripSpot{x: 1680, top: 6, bot: 914}
	if !s.covers(stripPt{2040, 970}) || s.covers(stripPt{2040, 60}) || s.covers(stripPt{1679, 970}) {
		t.Fatal("bottom")
	}
	s.atTop = true
	if !s.covers(stripPt{2040, 60}) || s.covers(stripPt{2040, 970}) {
		t.Fatal("top")
	}
}
