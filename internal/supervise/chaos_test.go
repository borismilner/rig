package supervise

import (
	"context"
	"errors"
	"flag"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

// `make test-chaos` passes -iterations; plain `go test` runs a short pass so
// the invariants are exercised on every ci run and not only when asked.
var (
	iterations = flag.Int("iterations", 40, "TestChaos: how many random actions")
	chaosSeed  = flag.Uint64("chaos-seed", 0, "TestChaos: replay one run; 0 picks a seed and logs it")
)

// TestChaos drives the real supervisor over real processes with random
// actions - kill -9, SIGSTOP (a hang), stop, restart, up, a registration -
// while its loop runs, and checks after every action what must always hold:
// every state is one of section 18's, every history row chains onto the one
// before, and no history outgrows its cap. At the end StopAll must leave no
// child that was ever launched alive.
//
// A failure logs the seed; -chaos-seed replays the same run.
func TestChaos(t *testing.T) {
	seed := *chaosSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	t.Logf("seed %d (replay with -args -chaos-seed=%d)", seed, seed)
	rng := rand.New(rand.NewPCG(seed, seed))

	dir := t.TempDir()
	script := filepath.Join(dir, "child.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 600\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var launched []int
	sup := New(Options{
		StopGrace: 100 * time.Millisecond,
		Start: func(spec Spec, handles map[string]string) (Process, <-chan Exit, error) {
			p, e, err := Start(spec, handles)
			if err == nil {
				mu.Lock()
				launched = append(launched, p.PID())
				mu.Unlock()
			}
			return p, e, err
		},
	})
	ids := []string{"a", "b", "c"}
	var specs []Spec
	for _, id := range ids {
		specs = append(specs, Spec{
			ID: id, Path: script,
			Health: HealthPolicy{
				Interval: 20 * time.Millisecond, Timeout: 10 * time.Millisecond,
				Idle: time.Hour, Register: 300 * time.Millisecond, Degraded: 3, Restart: 5,
			},
			Budget: Budget{Restarts: 3, Window: time.Minute, Backoff: 10 * time.Millisecond, MaxBackoff: 40 * time.Millisecond},
		})
	}
	if err := sup.Declare(specs); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	running := make(chan struct{})
	go func() { defer close(running); sup.Run(ctx) }()

	signalChild := func(id string, sig syscall.Signal) {
		st, err := sup.Health(id)
		if err == nil && st[0].PID > 0 {
			_ = syscall.Kill(st[0].PID, sig)
		}
	}
	actions := []struct {
		name string
		do   func(id string)
	}{
		{"up", func(id string) { _, _ = sup.Up(id) }},
		{"stop", func(id string) { _ = sup.Stop(id) }},
		{"restart", func(id string) { _ = sup.Restart(id) }},
		{"register", func(id string) { _ = sup.Registered(id) }},
		{"kill -9", func(id string) { signalChild(id, syscall.SIGKILL) }},
		{"hang", func(id string) { signalChild(id, syscall.SIGSTOP) }},
	}

	legal := map[State]bool{StateUnspecified: true}
	for _, s := range States() {
		legal[s] = true
	}
	for i := range *iterations {
		id := ids[rng.IntN(len(ids))]
		a := actions[rng.IntN(len(actions))]
		a.do(id)
		time.Sleep(time.Duration(rng.IntN(30)) * time.Millisecond)

		all, err := sup.Health()
		if err != nil {
			t.Fatalf("step %d (%s %s): health: %v", i, a.name, id, err)
		}
		for _, st := range all {
			if !legal[st.State] {
				t.Fatalf("step %d (%s %s): %s is in state %d, which section 18 does not have", i, a.name, id, st.ID, st.State)
			}
			if len(st.History) > historyCap {
				t.Fatalf("step %d: %s has %d history rows, over the cap of %d", i, st.ID, len(st.History), historyCap)
			}
			for j := 1; j < len(st.History); j++ {
				if prev, cur := st.History[j-1], st.History[j]; prev.To != cur.From {
					t.Fatalf("step %d (%s %s): %s's history breaks at row %d: %s -> %s, then %s -> %s",
						i, a.name, id, st.ID, j, prev.From, prev.To, cur.From, cur.To)
				}
			}
			if last := len(st.History); last > 0 && st.History[last-1].To != st.State {
				t.Fatalf("step %d (%s %s): %s is %s but its history ends in %s",
					i, a.name, id, st.ID, st.State, st.History[last-1].To)
			}
		}
	}

	cancel()
	<-running // Run's way out is StopAll, which waits for its children
	mu.Lock()
	defer mu.Unlock()
	for _, pid := range launched {
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			stat, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
			t.Errorf("pid %d outlived StopAll (kill 0: %v): %s", pid, err, stat)
		}
	}
	t.Logf("%d actions, %d children launched, none left", *iterations, len(launched))
}
