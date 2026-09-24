package daemon

import (
	"errors"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/coord"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 16's claimable queues, served. internal/coord/queue.go is the whole
// mechanism; like lease.go, this file only decides WHO claims (the caller's
// seat) and WHAT witnesses them (the caller's pid from SO_PEERCRED), and both
// come off the connection.

// serveQueue dispatches the four queue verbs.
func (d *Daemon) serveQueue(c *conn, f *rigv1.Frame, command string) {
	if d.leases == nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code: rigv1.Code_CODE_UNAVAILABLE,
			Message: "rig." + command + ": this estate has no queue store: an " +
				"unnamed estate keeps no persistent state, and a queue that lost " +
				"its tasks on a restart would lose work",
			Precondition: "rigd was started with --estate",
			Actual:       "this daemon serves an unnamed estate",
			Fix:          "start rigd with --estate <name>",
		})
		return
	}
	switch command {
	case "queue.push":
		d.serveQueuePush(c, f)
	case "queue.claim":
		d.serveQueueClaim(c, f)
	case "queue.complete":
		d.serveQueueComplete(c, f)
	case "queue.list":
		d.serveQueueList(c, f)
	}
}

func (d *Daemon) serveQueuePush(c *conn, f *rigv1.Frame) {
	var req verbsv1.QueuePushRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "queue.push: "+err.Error())
		return
	}
	task, dup, err := d.leases.Push(req.GetQueue(), req.GetIdempotencyKey(), req.GetPayload())
	if err != nil {
		c.failErr(f.GetStreamId(), queueCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.QueuePushResponse{Task: d.taskToWire(task), Duplicate: dup})
}

func (d *Daemon) serveQueueClaim(c *conn, f *rigv1.Frame) {
	var req verbsv1.QueueClaimRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "queue.claim: "+err.Error())
		return
	}
	_, seat, _, ok := d.provenance(c)
	if !ok {
		refuseUnseatedLease(c, f, "queue.claim")
		return
	}
	// ALWAYS WITNESSED. An unwitnessed claim would never requeue on its own,
	// and a queue whose tasks wait on a recorded break is not at-least-once.
	w, err := coord.WitnessProcess(c.principal().PID)
	if err != nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_INVALID,
			Message:      "queue.claim: rig could not witness this caller's process: " + err.Error(),
			Precondition: "the socket reports a live pid for the caller",
			Actual:       "no pid rig can poll",
			Fix:          "claim from a process rig can see; a claim requeues when its worker dies, and that needs a witness",
		})
		return
	}
	task, h, err := d.leases.Claim(req.GetQueue(), seat, w, ttlOf(req.GetTtlMs()))
	if err != nil {
		c.failErr(f.GetStreamId(), queueCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.QueueClaimResponse{Task: d.taskToWire(task), Handle: handleToWire(h)})
}

func (d *Daemon) serveQueueComplete(c *conn, f *rigv1.Frame) {
	var req verbsv1.QueueCompleteRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "queue.complete: "+err.Error())
		return
	}
	task, err := d.leases.Complete(coord.Handle{
		Name: req.GetName(), Token: req.GetToken(), Epoch: req.GetEpoch(),
	})
	if err != nil {
		c.failErr(f.GetStreamId(), queueCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.QueueCompleteResponse{Task: d.taskToWire(task)})
}

func (d *Daemon) serveQueueList(c *conn, f *rigv1.Frame) {
	var req verbsv1.QueueListRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "queue.list: "+err.Error())
		return
	}
	if req.GetQueue() == "" {
		names, err := d.leases.Queues()
		if err != nil {
			c.failErr(f.GetStreamId(), queueCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.QueueListResponse{Queues: names})
		return
	}
	tasks, done, err := d.leases.Tasks(req.GetQueue())
	if err != nil {
		c.failErr(f.GetStreamId(), queueCode(err), err)
		return
	}
	resp := &verbsv1.QueueListResponse{Done: uint32(min(done, int(^uint32(0))))} //nolint:gosec // clamped above
	for _, t := range tasks {
		resp.Tasks = append(resp.Tasks, d.taskToWire(t))
	}
	c.reply(f.GetStreamId(), resp)
}

// queueCode maps coord's queue refusals onto the wire's codes, and falls back
// to the lease mapping for the fencing a claim shares with a lease.
func queueCode(err error) rigv1.Code {
	var empty *coord.EmptyError
	var conflict *coord.KeyConflictError
	var notClaim *coord.NotClaimError
	switch {
	case errors.As(err, &empty), errors.As(err, &notClaim):
		return rigv1.Code_CODE_NOT_FOUND
	case errors.As(err, &conflict):
		return rigv1.Code_CODE_CONFLICT
	default:
		return leaseCode(err)
	}
}

var taskStateWire = map[coord.TaskState]verbsv1.TaskState{
	coord.TaskReady:    verbsv1.TaskState_TASK_STATE_READY,
	coord.TaskClaimed:  verbsv1.TaskState_TASK_STATE_CLAIMED,
	coord.TaskOrphaned: verbsv1.TaskState_TASK_STATE_ORPHANED,
	coord.TaskDone:     verbsv1.TaskState_TASK_STATE_DONE,
}

func (d *Daemon) taskToWire(t coord.Task) *verbsv1.Task {
	out := &verbsv1.Task{
		Queue: t.Queue, Id: t.ID, IdempotencyKey: t.Key, Payload: t.Payload,
		State: taskStateWire[t.State], Attempts: t.Attempts, DoneBy: t.DoneBy,
	}
	if t.Claim.Name != "" {
		if at, err := coord.Now(); err == nil {
			out.Claim = leaseToWire(t.Claim, at)
		}
	}
	return out
}
