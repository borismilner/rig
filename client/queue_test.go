package client

// The bounded outbound queue, section 5g's third mechanism.
//
// This is an INTERNAL test on purpose. Asserting the queue is genuinely full
// before testing what a full one does needs to read its depth, and an exported
// accessor for that would grow the surface section 3 budgets - a decision in
// the plan, bought to make one test honest. From inside the package the depth
// is just a channel length.
//
// Without that check the test passes for the wrong reason whenever the
// holders have not taken their slots yet, which is the defect class this
// repository keeps finding: a check that cannot tell "nothing is wrong" from
// "I was not looking yet".

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// deafSocket listens, accepts, and never answers anything.
func deafSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigdeaf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			_ = nc.Close() // hang up at once: a call gets no reply
		}
	}()
	return path
}

func TestAFullOutboundQueueRefusesInsteadOfWaiting(t *testing.T) {
	c, err := Dial(deafSocket(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	hold, cancelHold := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelHold()

	var holders sync.WaitGroup
	for range outbound {
		holders.Add(1)
		go func() {
			defer holders.Done()
			_ = c.Call(hold, "rig.programs", &rigv1.ProgramsRequest{},
				&rigv1.ProgramsResponse{})
		}()
	}

	// Every holder takes its slot on its FIRST failure, one hang-up away.
	deadline := time.Now().Add(10 * time.Second)
	for len(c.queue) < outbound && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if n := len(c.queue); n < outbound {
		t.Fatalf("only %d of %d slots were taken, so the queue was never full "+
			"and nothing below this line means anything", n, outbound)
	}

	// A 20 second deadline against a sub-5s assertion. The gap is what
	// separates "refused because the queue is full" from "waited and gave up".
	over, cancelOver := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelOver()
	start := time.Now()
	err = c.Call(over, "rig.programs", &rigv1.ProgramsRequest{}, &rigv1.ProgramsResponse{})
	took := time.Since(start)

	var ce *CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_UNAVAILABLE {
		t.Fatalf("a call past the bound returned %v, want a CallError carrying "+
			"CODE_UNAVAILABLE", err)
	}
	if took > 5*time.Second {
		t.Errorf("the refused call took %v against a 20s deadline, so it was "+
			"waiting for a slot. That is the unbounded wait the bound exists "+
			"to prevent, moved somewhere less visible", took)
	}

	cancelHold()
	holders.Wait()
}
