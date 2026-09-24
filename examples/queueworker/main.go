// Command queueworker drains one of rig's claimable queues (PLAN.md section
// 16), and is the worker docs/programs.md points at.
//
// It takes a seat, then claims the oldest ready task, heartbeats the claim
// while it works, and completes it. Delivery is at-least-once: a worker that
// dies after the work and before Complete leaves a task that runs again, so a
// real worker makes the work safe to repeat under the task's idempotency key.
// This one says when a delivery may be a replay.
//
//	rig queue push builds commit-abc --payload '{"ref":"abc"}'
//	go run ./examples/queueworker -queue builds
//
// The queue needs a named estate: an unnamed one keeps no queues.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

func main() {
	queue := flag.String("queue", "builds", "the queue to drain")
	seat := flag.String("seat", "queueworker", "the seat this worker takes")
	ttl := flag.Duration("ttl", 30*time.Second, "how long a claim holds between heartbeats")
	flag.Parse()

	c, err := client.Connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "queueworker:", err)
		os.Exit(1)
	}
	defer c.Close()
	n, err := drain(context.Background(), c, *seat, *queue, *ttl, work)
	if err != nil {
		fmt.Fprintln(os.Stderr, "queueworker:", err)
		os.Exit(1)
	}
	fmt.Printf("queueworker: %d done, %q is empty\n", n, *queue)
}

// work is the job. Here it prints; a real one builds, deploys or indexes.
func work(t *verbsv1.Task) error {
	fmt.Printf("task %s key %s: %s\n", t.GetId(), t.GetIdempotencyKey(), t.GetPayload())
	return nil
}

// drain takes a seat and works the queue until it is empty, answering how
// many tasks it completed. A claim is witnessed by this process, so if it
// dies the claim is requeued once rig observes it gone.
func drain(ctx context.Context, c *client.Client, seat, queue string, ttl time.Duration,
	do func(*verbsv1.Task) error,
) (int, error) {
	if err := c.Call(ctx, "rig.announce", &verbsv1.AnnounceRequest{
		Seat: seat, Purpose: "draining the " + queue + " queue", Activity: "working",
	}, &verbsv1.AnnounceResponse{}); err != nil {
		return 0, err
	}
	done := 0
	for {
		var claim verbsv1.QueueClaimResponse
		err := c.Call(ctx, "rig.queue.claim", &verbsv1.QueueClaimRequest{
			Queue: queue, TtlMs: uint32(ttl.Milliseconds()), //nolint:gosec // a flag, in range
		}, &claim)
		var ce *client.CallError
		if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND {
			return done, nil // nothing ready
		}
		if err != nil {
			return done, err
		}
		task, h := claim.GetTask(), claim.GetHandle()
		if task.GetAttempts() > 1 {
			fmt.Printf("task %s is on attempt %d: it may already have run once\n",
				task.GetId(), task.GetAttempts())
		}

		if err := withHeartbeat(ctx, c, h, ttl, func() error { return do(task) }); err != nil {
			// Give the claim back unfinished so another worker can take it
			// now rather than after the deadline.
			_ = c.Call(ctx, "rig.lease.release", &verbsv1.LeaseReleaseRequest{
				Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
			}, &verbsv1.LeaseReleaseResponse{})
			return done, err
		}
		if err := c.Call(ctx, "rig.queue.complete", &verbsv1.QueueCompleteRequest{
			Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
		}, &verbsv1.QueueCompleteResponse{}); err != nil {
			// A CONFLICT here means the claim moved on while this worker
			// stalled: the task went to another worker, and this run was the
			// duplicate the idempotency key is for.
			return done, err
		}
		done++
	}
}

// withHeartbeat runs fn while renewing the claim every third of its ttl. The
// heartbeat is rig.lease.renew: a claim is a lease.
func withHeartbeat(ctx context.Context, c *client.Client, h *verbsv1.LeaseHandle,
	ttl time.Duration, fn func() error,
) error {
	stop := make(chan struct{})
	beat := make(chan struct{})
	go func() {
		defer close(beat)
		tick := time.NewTicker(ttl / 3)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				_ = c.Call(ctx, "rig.lease.renew", &verbsv1.LeaseRenewRequest{
					Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
					TtlMs: uint32(ttl.Milliseconds()), //nolint:gosec // a flag, in range
				}, &verbsv1.LeaseRenewResponse{})
			}
		}
	}()
	err := fn()
	close(stop)
	<-beat
	return err
}
