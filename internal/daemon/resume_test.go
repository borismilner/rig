package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// §20: synctest cannot model suspend, so the clocks are injected. Each wait
// returns once, and each read answers the next reading.
func TestASuspendIsPublishedAndAClockChangeIsNot(t *testing.T) {
	sock, d := upDaemon(t, nil)
	readings := []clocks{
		{boot: 10 * time.Second, mono: 10 * time.Second},
		// the wall clock was set: both clocks moved alike
		{boot: 20 * time.Second, mono: 20 * time.Second},
		// eight hours asleep: BOOTTIME ran on, MONOTONIC did not
		{boot: 8*time.Hour + 30*time.Second, mono: 30 * time.Second},
	}
	signals := make(chan struct{}, 2)
	signals <- struct{}{}
	signals <- struct{}{}
	wait := func() error {
		select {
		case <-signals:
			return nil
		default:
			return errors.New("closed")
		}
	}
	read := func() (clocks, error) {
		c := readings[0]
		readings = readings[1:]
		return c, nil
	}
	d.watchResume(context.Background(), wait, read)

	var got registryv1.EventsWaitResponse
	if err := dial(t, sock).Call(ctx5(t), "rig.events.wait", &registryv1.EventsWaitRequest{
		Kinds: []string{"system.*"}, TimeoutMs: 100,
	}, &got); err != nil {
		t.Fatal(err)
	}
	evs := got.GetEvents()
	if len(evs) != 1 || evs[0].GetKind() != "system.resumed" ||
		!strings.Contains(evs[0].GetPayloadJson(), `"sleptMs":"28800000"`) {
		t.Fatalf("published %v", evs)
	}
}

// The kernel's signal is there on this machine, and closing it ends the wait.
func TestTheResumeSignalOpensAndCloses(t *testing.T) {
	wait, closer, err := clockCancels()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- wait() }()
	time.Sleep(20 * time.Millisecond)
	if err := closer(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a closed signal answered as a resume")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closing did not end the wait")
	}
}
