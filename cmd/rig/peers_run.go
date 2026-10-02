package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// cmdPeersRun is `rig peers run --lease=NAME -- CMD ...` (plan/53 slice 6,
// PLAN.md section 16): CMD runs holding NAME, and stops when the hold is
// lost. AgentBox's `sync lock NAME -- CMD` released the lock when CMD ended;
// nothing stopped CMD when the lock went. Here a renew that fails stops it,
// and a holder that stalls is stopped by rigd, which kills the group this
// fenced before the lease can pass on.

// exitLeaseLost is what a run answers when it lost its lease, sysexits'
// EX_TEMPFAIL: try again later.
const exitLeaseLost = 75

// runGrace is the most a run stopped for a lost lease has after SIGTERM.
const runGrace = 2 * time.Second

// grace is the SIGTERM-to-SIGKILL wait for a TTL: a sixth of it, at most
// runGrace. A run stops at the deadline less two of these, so even the
// SIGKILL lands before the hold ends.
func grace(ttl time.Duration) time.Duration { return min(runGrace, ttl/6) }

const peersRunUsage = "usage: rig peers run --lease=NAME [--ttl=30s] [--wait=0s] -- CMD [ARG...]"

func cmdPeersRun(args []string) error {
	split := slices.Index(args, "--")
	if split < 0 || split == len(args)-1 {
		return badArgumentf("%s", peersRunUsage)
	}
	fs := flag.NewFlagSet("peers run", flag.ContinueOnError)
	name := fs.String("lease", "", "the lease to hold while CMD runs")
	ttl := fs.Duration("ttl", 30*time.Second, "the lease's TTL; it is renewed every third of it")
	wait := fs.Duration("wait", 0, "how long to queue for a held lease, at most 25m")
	if err := fs.Parse(args[:split]); err != nil {
		return err
	}
	argv := args[split+1:]
	switch {
	case *name == "" || fs.NArg() != 0:
		return badArgumentf("%s", peersRunUsage)
	case *ttl < 3*time.Second || *ttl > time.Hour:
		return badArgumentf("--ttl must be between 3s and 1h, got %s", *ttl)
	case *wait < 0 || *wait > 25*time.Minute:
		return badArgumentf("--wait must be between 0 and 25m, got %s", *wait)
	}

	r := &leasedRun{name: *name, ttl: *ttl}
	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	r.c = c
	defer func() { r.c.Close() }()
	actx, acancel := context.WithTimeout(context.Background(), *wait+10*time.Second)
	err = r.acquire(actx, *wait)
	acancel()
	if err != nil {
		return err
	}

	// No context: CMD ends when it ends or when this stops it, never on a
	// deadline of its own.
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec,noctx // CMD is the caller's own argv, run as the caller
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Its own group, so a stop reaches everything it started; and killed if
	// this process dies, so it never outlives the hold's witness.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		r.release()
		return err
	}
	r.pid = cmd.Process.Pid
	fctx, fcancel := callCtx()
	err = r.fence(fctx)
	fcancel()
	if err != nil {
		// Never run unfenced: the fence is the point of this verb.
		r.stop()
		_ = cmd.Wait()
		r.release()
		return fmt.Errorf("rig peers run: the lease %s could not be fenced, so the command was stopped: %w", r.name, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)

	tick := time.NewTicker(r.ttl / 3)
	defer tick.Stop()
	deadline := time.Now().Add(r.ttl)
	for {
		select {
		case err := <-done:
			r.release()
			return exitOf(err)
		case s := <-sigs:
			// Passed on, so ^C stops the command the way it would unwrapped.
			if sig, ok := s.(syscall.Signal); ok {
				_ = syscall.Kill(-r.pid, sig)
			}
		case <-tick.C:
			// One attempt, bounded by the stop point, so a daemon that is gone
			// or wedged cannot hold the loop past it.
			stopAt := deadline.Add(-2 * grace(r.ttl))
			ctx, cancel := context.WithDeadline(context.Background(), stopAt)
			err := r.renew(ctx)
			cancel()
			switch {
			case err == nil:
				deadline = time.Now().Add(r.ttl)
				continue
			case time.Now().Add(r.ttl / 3).Before(stopAt):
				continue // a missed renew with a tick to spare: try at the next
			}
			r.stop()
			select {
			case <-done:
			case <-time.After(grace(r.ttl)):
				_ = syscall.Kill(-r.pid, syscall.SIGKILL)
				<-done
			}
			return &exitCodeError{exitLeaseLost, fmt.Errorf(
				"rig peers run: the lease %s could not be kept (%w), so the command was stopped", r.name, err)}
		}
	}
}

// leasedRun is one run's hold: its handle, and the group it fenced.
type leasedRun struct {
	c    *client.Client
	name string
	ttl  time.Duration
	h    *verbsv1.LeaseHandle
	pid  int
}

func callCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (r *leasedRun) acquire(ctx context.Context, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, wait+10*time.Second)
	defer cancel()
	resp := &verbsv1.LeaseAcquireResponse{}
	if err := call(ctx, r.c, "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{
		Name: r.name, TtlMs: ms(r.ttl), WaitMs: ms(wait),
	}, resp); err != nil {
		if rf, ok := errors.AsType[*refusal](err); ok && rf.Status.GetCode() == rigv1.Code_CODE_CONFLICT {
			return &exitCodeError{exitLeaseLost, err} // held: try again later
		}
		return err
	}
	if resp.GetTimedOut() {
		inc := resp.GetIncumbent()
		return &exitCodeError{exitLeaseLost, fmt.Errorf("rig peers run: %s is held by %s (%s) and stayed held for %s",
			r.name, inc.GetHolder(), inc.GetHolderPurpose(), wait)}
	}
	r.h = resp.GetHandle()
	return nil
}

func (r *leasedRun) fence(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return call(ctx, r.c, "rig.lease.fence", &verbsv1.LeaseFenceRequest{
		Name: r.name, Token: r.h.GetToken(), Epoch: r.h.GetEpoch(), Pid: uint32(r.pid), //nolint:gosec // a pid the kernel gave this process
	}, &verbsv1.LeaseFenceResponse{})
}

// renew keeps the hold. A daemon that restarted fenced the handle by its
// epoch, so the run takes its own lease again, the reconnect path, and
// fences the new hold.
func (r *leasedRun) renew(ctx context.Context) error {
	resp := &verbsv1.LeaseRenewResponse{}
	err := call(ctx, r.c, "rig.lease.renew", &verbsv1.LeaseRenewRequest{
		Name: r.name, Token: r.h.GetToken(), Epoch: r.h.GetEpoch(), TtlMs: ms(r.ttl),
	}, resp)
	if err == nil {
		r.h = resp.GetHandle()
		return nil
	}
	c, cerr := connect()
	if cerr != nil {
		return errors.Join(err, cerr)
	}
	r.c.Close()
	r.c = c
	if aerr := r.acquire(ctx, 0); aerr != nil {
		return errors.Join(err, aerr)
	}
	return r.fence(ctx)
}

// ms is a duration the flags bounded to an hour, in milliseconds.
func ms(d time.Duration) uint32 {
	return uint32(min(max(d.Milliseconds(), 0), math.MaxUint32))
}

func (r *leasedRun) stop() { _ = syscall.Kill(-r.pid, syscall.SIGTERM) }

func (r *leasedRun) release() {
	if r.h == nil {
		return
	}
	ctx, cancel := callCtx()
	defer cancel()
	_ = call(ctx, r.c, "rig.lease.release", &verbsv1.LeaseReleaseRequest{
		Name: r.name, Token: r.h.GetToken(), Epoch: r.h.GetEpoch(),
	}, &verbsv1.LeaseReleaseResponse{})
}

// exitOf passes CMD's own exit status on, 128 plus the signal for a
// command a signal ended, as a shell reports it.
func exitOf(err error) error {
	if err == nil {
		return nil
	}
	ee, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		return err
	}
	code := ee.ExitCode()
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() { //nolint:misspell // the stdlib's spelling
		code = 128 + int(ws.Signal())
	}
	return &exitCodeError{code, fmt.Errorf("rig peers run: the command exited with status %d", code)}
}
