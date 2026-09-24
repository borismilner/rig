package main

import (
	"context"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/client/clienttest"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// A producer pushes three tasks, one of them twice under the same key, and the
// worker drains exactly the three, in order.
func TestTheWorkerDrainsTheQueueOnce(t *testing.T) {
	clienttest.StartEstate(t, "queueworker")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	producer, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	for _, key := range []string{"a", "b", "a", "c"} {
		if err := producer.Call(ctx, "rig.queue.push", &verbsv1.QueuePushRequest{
			Queue: "builds", IdempotencyKey: key, Payload: []byte("build " + key),
		}, &verbsv1.QueuePushResponse{}); err != nil {
			t.Fatal(err)
		}
	}

	worker, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	var seen []string
	n, err := drain(ctx, worker, "worker", "builds", time.Minute, func(task *verbsv1.Task) error {
		seen = append(seen, task.GetIdempotencyKey())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(seen) != 3 || seen[0] != "a" || seen[1] != "b" || seen[2] != "c" {
		t.Fatalf("drained %d: %v, want a, b, c once each", n, seen)
	}
}
