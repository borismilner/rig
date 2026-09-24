package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// `rig queue` - section 16's claimable queues at the prompt.
//
//	rig queue push <queue> <idempotency-key> [--payload P]
//	rig queue list [<queue>]
//
// CLAIMING IS NOT HERE, and that is the mechanism rather than a gap. A claim
// is witnessed by the claiming process, so a claim made by this command would
// be witnessed by a process that exits the moment it prints, and the task
// would requeue as soon as its deadline passed. A worker claims over its own
// long-lived connection: examples/queueworker is one.

type queueFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration
	payload *string
}

func queueFlagSet() *queueFlags {
	q := &queueFlags{fs: flag.NewFlagSet("queue", flag.ContinueOnError)}
	q.asJSON = q.fs.Bool("json", false, "emit JSON")
	q.timeout = q.fs.Duration("timeout", defaultCallTimeout, "how long to wait")
	q.payload = q.fs.String("payload", "", "push: what the task is, for the worker")
	return q
}

func cmdQueue(args []string) (err error) {
	q := queueFlagSet()
	flags, positional := partition(args)
	if err := q.fs.Parse(flags); err != nil {
		return err
	}
	defer func() { err = inMode(err, *q.asJSON) }()
	usage := "usage: rig queue push <queue> <idempotency-key> [--payload P] | list [<queue>]"
	if len(positional) == 0 {
		return badArgumentf("%s", usage)
	}
	sub, rest := positional[0], positional[1:]
	switch {
	case sub == "push" && len(rest) == 2:
	case sub == "list" && len(rest) <= 1:
	default:
		return badArgumentf("%s", usage)
	}

	c, err := connect()
	if err != nil {
		return noDaemon(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *q.timeout)
	defer cancel()
	out := json.NewEncoder(os.Stdout)

	if sub == "push" {
		var resp verbsv1.QueuePushResponse
		if err := call(ctx, c, "rig.queue.push", &verbsv1.QueuePushRequest{
			Queue: rest[0], IdempotencyKey: rest[1], Payload: []byte(*q.payload),
		}, &resp); err != nil {
			return err
		}
		if *q.asJSON {
			m := taskJSON(resp.GetTask())
			m["duplicate"] = resp.GetDuplicate()
			return out.Encode(m)
		}
		if resp.GetDuplicate() {
			fmt.Printf("already queued under that key: ")
		}
		fmt.Println(taskLine(resp.GetTask()))
		return nil
	}

	req := &verbsv1.QueueListRequest{}
	if len(rest) == 1 {
		req.Queue = rest[0]
	}
	var resp verbsv1.QueueListResponse
	if err := call(ctx, c, "rig.queue.list", req, &resp); err != nil {
		return err
	}
	if req.GetQueue() == "" {
		names := resp.GetQueues()
		if names == nil {
			names = []string{}
		}
		if *q.asJSON {
			return out.Encode(map[string]any{"queues": names})
		}
		if len(names) == 0 {
			fmt.Println("no queues. `rig queue push <queue> <key>` makes one.")
		}
		for _, n := range names {
			fmt.Println(n)
		}
		return nil
	}
	if *q.asJSON {
		tasks := make([]map[string]any, 0, len(resp.GetTasks()))
		for _, t := range resp.GetTasks() {
			tasks = append(tasks, taskJSON(t))
		}
		return out.Encode(map[string]any{"tasks": tasks, "done": resp.GetDone()})
	}
	for _, t := range resp.GetTasks() {
		fmt.Println(taskLine(t))
	}
	fmt.Printf("%d unfinished, %d done\n", len(resp.GetTasks()), resp.GetDone())
	return nil
}

// taskState is the enum's word without its prefix, lowercased.
func taskState(t *verbsv1.Task) string {
	return strings.ToLower(strings.TrimPrefix(t.GetState().String(), "TASK_STATE_"))
}

// taskLine is one task for a person: its id, state, key, attempts and who
// holds or held it.
func taskLine(t *verbsv1.Task) string {
	line := fmt.Sprintf("%s/%s  %-8s  key %s  attempts %d", t.GetQueue(), t.GetId(),
		taskState(t), t.GetIdempotencyKey(), t.GetAttempts())
	switch {
	case t.GetDoneBy() != "":
		line += "  done by " + t.GetDoneBy()
	case t.GetClaim() != nil && t.GetState() != verbsv1.TaskState_TASK_STATE_READY:
		line += "  by " + t.GetClaim().GetHolder()
	}
	return line
}

func taskJSON(t *verbsv1.Task) map[string]any {
	m := map[string]any{
		"queue": t.GetQueue(), "id": t.GetId(), "idempotency_key": t.GetIdempotencyKey(),
		"payload": string(t.GetPayload()), "state": taskState(t), "attempts": t.GetAttempts(),
	}
	if t.GetDoneBy() != "" {
		m["done_by"] = t.GetDoneBy()
	}
	if cl := t.GetClaim(); cl != nil {
		m["claim"] = map[string]any{
			"lease": cl.GetName(), "holder": cl.GetHolder(), "token": cl.GetToken(),
			"remaining_ms": cl.GetRemainingMs(), "owner_gone": cl.GetOwnerGone(),
		}
	}
	return m
}
