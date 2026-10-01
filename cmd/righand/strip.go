package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/hand"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// The hand's half of the HANDS OFF strip (plan/05 section 5m, H1-H5). A
// script asks rigd for the desktop and blocks through his countdown, passes
// rigd's gate before every step, and stops between characters of a type once
// he took the desktop back. rigd owns the state; this only follows it.

// gateWait bounds one hand.step: rigd ends a pause after ten minutes, so a
// step that waits longer means rigd is gone.
const gateWait = 11 * time.Minute

// deskGate is the hand.Park the script runs under.
type deskGate struct {
	c *client.Client
	// held is true while the run is not driving. It is read between every
	// two characters of a type, so it is a load and never a call.
	held atomic.Bool
}

var (
	_ hand.Park    = (*deskGate)(nil)
	_ hand.Stepper = (*deskGate)(nil)
)

func (g *deskGate) Blocked() bool { return g.held.Load() }

// Wait blocks in rigd until he resumes, and fails once he stopped the run.
func (g *deskGate) Wait() error {
	if err := g.gate(""); err != nil {
		return err
	}
	g.held.Store(false)
	return nil
}

// Before names the step on the strip: its number and op, never its text.
func (g *deskGate) Before(i, n int, st hand.Step) error {
	return g.gate(fmt.Sprintf("step %d of %d: %s", i+1, n, st.Op))
}

func (g *deskGate) gate(activity string) error {
	ctx, cancel := context.WithTimeout(context.Background(), gateWait)
	defer cancel()
	return g.c.Call(ctx, "rig.hand.step", &registryv1.HandStepRequest{Activity: activity}, &registryv1.HandStepResponse{})
}

// follow keeps held current until ctx ends, so a type stops at the next
// character once he takes the desktop back rather than at the end of the
// text.
func (g *deskGate) follow(ctx context.Context) {
	var seq uint64
	for ctx.Err() == nil {
		call, cancel := context.WithTimeout(ctx, 40*time.Second)
		var resp registryv1.HandWaitResponse
		err := g.c.Call(call, "rig.hand.wait", &registryv1.HandWaitRequest{After: seq, TimeoutMs: 30_000}, &resp)
		cancel()
		if err != nil {
			// rigd gone, or ctx over: the gate before the next step is
			// what refuses, so a lost watch only costs the mid-type stop.
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
			continue
		}
		seq = resp.GetState().GetSeq()
		g.held.Store(resp.GetState().GetPhase() != registryv1.HandPhase_HAND_PHASE_DRIVING)
	}
}

// askDesktop asks rigd for the desktop, blocking through his countdown. It
// answers the gate to run under and the release to defer.
func (p *program) askDesktop(why string, steps []hand.Step) (*deskGate, func(), error) {
	if strings.TrimSpace(why) == "" {
		why = fmt.Sprintf("%d steps: %s", len(steps), ops(steps))
	}
	ctx, cancel := context.WithTimeout(context.Background(), gateWait)
	defer cancel()
	if err := p.c.Call(ctx, "rig.hand.request", &registryv1.HandRequestRequest{Reason: why}, &registryv1.HandRequestResponse{}); err != nil {
		return nil, nil, err
	}
	g := &deskGate{c: p.c}
	//rig:allow nocontextfree: the watch lives exactly as long as the run, and release is what ends it
	watch, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); g.follow(watch) }()
	release := func() {
		stop()
		<-done
		rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rcancel()
		_ = p.c.Call(rctx, "rig.hand.release", &registryv1.HandReleaseRequest{}, &registryv1.HandReleaseResponse{})
	}
	return g, release, nil
}

// codeOf passes rigd's refusal through to the caller, so a declined run reads
// as denied and a held-too-long one as a deadline.
func codeOf(err error) rigv1.Code {
	var ce *client.CallError
	if errors.As(err, &ce) {
		return ce.Code()
	}
	return rigv1.Code_CODE_UNAVAILABLE
}
