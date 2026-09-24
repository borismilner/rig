package daemon

import (
	"slices"
	"testing"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 16's queues over the real wire. internal/coord's tests prove the
// mechanism - requeue on witness death, fencing, a restart. These prove the
// verbs are reachable, that the claimer is the connection's seat and process,
// that the heartbeat is rig.lease.renew, and that refusals arrive as codes.
func TestATaskIsPushedClaimedAndCompletedOverTheWire(t *testing.T) {
	sock, _ := upLeaseDaemon(t)
	ctx := recordCtx(t)
	producer := seated(t, sock, "producer")
	worker := seated(t, sock, "worker")

	var pushed verbsv1.QueuePushResponse
	if err := producer.Call(ctx, "rig.queue.push", &verbsv1.QueuePushRequest{
		Queue: "builds", IdempotencyKey: "commit-abc", Payload: []byte(`{"ref":"abc"}`),
	}, &pushed); err != nil {
		t.Fatalf("rig.queue.push: %v", err)
	}
	if pushed.GetDuplicate() || pushed.GetTask().GetState() != verbsv1.TaskState_TASK_STATE_READY {
		t.Fatalf("a first push answered %+v", &pushed)
	}
	var again verbsv1.QueuePushResponse
	if err := producer.Call(ctx, "rig.queue.push", &verbsv1.QueuePushRequest{
		Queue: "builds", IdempotencyKey: "commit-abc", Payload: []byte(`{"ref":"abc"}`),
	}, &again); err != nil || !again.GetDuplicate() || again.GetTask().GetId() != pushed.GetTask().GetId() {
		t.Fatalf("the same key pushed twice: %+v %v", &again, err)
	}
	err := producer.Call(ctx, "rig.queue.push", &verbsv1.QueuePushRequest{
		Queue: "builds", IdempotencyKey: "commit-abc", Payload: []byte(`{"ref":"other"}`),
	}, &verbsv1.QueuePushResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "one key over a different payload")
	err = producer.Call(ctx, "rig.queue.push", &verbsv1.QueuePushRequest{
		Queue: "builds", Payload: []byte("x"),
	}, &verbsv1.QueuePushResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a push with no idempotency key")

	var claimed verbsv1.QueueClaimResponse
	if err := worker.Call(ctx, "rig.queue.claim", &verbsv1.QueueClaimRequest{
		Queue: "builds", TtlMs: 60_000,
	}, &claimed); err != nil {
		t.Fatalf("rig.queue.claim: %v", err)
	}
	h := claimed.GetHandle()
	if claimed.GetTask().GetIdempotencyKey() != "commit-abc" || h.GetHolder() != "worker" ||
		claimed.GetTask().GetState() != verbsv1.TaskState_TASK_STATE_CLAIMED {
		t.Fatalf("claimed %+v", &claimed)
	}

	err = producer.Call(ctx, "rig.queue.claim", &verbsv1.QueueClaimRequest{
		Queue: "builds", TtlMs: 60_000,
	}, &verbsv1.QueueClaimResponse{})
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "claiming from a queue whose only task is claimed")

	err = producer.Call(ctx, "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{
		Name: h.GetName(), TtlMs: 60_000,
	}, &verbsv1.LeaseAcquireResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "acquiring a claim's lease directly")

	// The heartbeat is rig.lease.renew, not a verb of its own.
	if err := worker.Call(ctx, "rig.lease.renew", &verbsv1.LeaseRenewRequest{
		Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(), TtlMs: 60_000,
	}, &verbsv1.LeaseRenewResponse{}); err != nil {
		t.Fatalf("the heartbeat: %v", err)
	}

	err = worker.Call(ctx, "rig.queue.complete", &verbsv1.QueueCompleteRequest{
		Name: h.GetName(), Token: h.GetToken() + 1, Epoch: h.GetEpoch(),
	}, &verbsv1.QueueCompleteResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "completing with a stale token")

	var done verbsv1.QueueCompleteResponse
	if err := worker.Call(ctx, "rig.queue.complete", &verbsv1.QueueCompleteRequest{
		Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
	}, &done); err != nil {
		t.Fatalf("rig.queue.complete: %v", err)
	}
	if done.GetTask().GetState() != verbsv1.TaskState_TASK_STATE_DONE || done.GetTask().GetDoneBy() != "worker" {
		t.Fatalf("completed %+v", &done)
	}

	var list verbsv1.QueueListResponse
	if err := producer.Call(ctx, "rig.queue.list", &verbsv1.QueueListRequest{Queue: "builds"}, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.GetTasks()) != 0 || list.GetDone() != 1 {
		t.Fatalf("the list after completion: %+v", &list)
	}
	var names verbsv1.QueueListResponse
	if err := producer.Call(ctx, "rig.queue.list", &verbsv1.QueueListRequest{}, &names); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names.GetQueues(), []string{"builds"}) {
		t.Fatalf("the queue names: %v", names.GetQueues())
	}
}
