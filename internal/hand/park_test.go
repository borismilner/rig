package hand

import (
	"errors"
	"testing"
)

// gateLog is a Park and a Stepper that records what it was told and refuses
// the step numbered refuse.
type gateLog struct {
	seen   []string
	refuse int
}

func (g *gateLog) Blocked() bool { return false }
func (g *gateLog) Wait() error   { return nil }

func (g *gateLog) Before(i, _ int, st Step) error {
	g.seen = append(g.seen, string(st.Op))
	if i == g.refuse {
		return errors.New("he stopped the run")
	}
	return nil
}

// A Stepper is told before every step, and a refusal stops the script before
// that step runs, with the steps that did run reported.
func TestAStepperIsToldEveryStepAndItsRefusalStopsTheRun(t *testing.T) {
	steps, err := ParseScript("wait 1\nwpm 200\nwait 1")
	if err != nil {
		t.Fatal(err)
	}
	g := &gateLog{refuse: 1}
	h := &Hand{}
	h.SetPark(g)
	ran, err := h.Run(steps)
	if ran != 1 || err == nil || len(g.seen) != 2 || h.wpm == 200 {
		t.Fatalf("ran %d, err %v, told %v, wpm %d", ran, err, g.seen, h.wpm)
	}
}
