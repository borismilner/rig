package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// The worker: it takes runs off rig's queue one at a time, the way graft's
// runner takes assignments. Everything it does is a rig verb the page lists.

const (
	claimTTL = 30 * time.Second
	gpuLease = "storeworker:gpu"
)

// work runs until ctx ends.
func (a *app) work(ctx context.Context) {
	_ = a.call(ctx, "announce", "seat "+a.seat, &verbsv1.AnnounceRequest{
		Seat: a.seat, Purpose: "running storeworker's queued runs", Activity: "starting",
	}, &verbsv1.AnnounceResponse{})
	for ctx.Err() == nil {
		var claim verbsv1.QueueClaimResponse
		// Quiet, and logged below only when it got a run: an idle worker
		// asks every three seconds, and nothing ready is not a failure.
		err := a.call(quietly(ctx), "queue.claim", a.queue, &verbsv1.QueueClaimRequest{
			Queue: a.queue, TtlMs: uint32(claimTTL.Milliseconds()),
		}, &claim)
		if code(err) != rigv1.Code_CODE_NOT_FOUND {
			a.logCall("queue.claim", a.queue, err)
		}
		switch {
		case code(err) == rigv1.Code_CODE_NOT_FOUND:
			a.setWorker(ctx, func(w *workerView) { *w = workerView{Activity: "idle: no run queued", Done: w.Done} })
			a.report(ctx, "no run queued")
			sleep(ctx, 3*time.Second)
			continue
		case err != nil:
			a.setWorker(ctx, func(w *workerView) { w.Activity = "cannot claim: " + short(err.Error()) })
			sleep(ctx, 5*time.Second)
			continue
		}
		h := claim.GetHandle()
		id := string(claim.GetTask().GetPayload())
		a.setWorker(ctx, func(w *workerView) {
			w.Run, w.Claim = id, fmt.Sprintf("%s token %d", h.GetName(), h.GetToken())
		})
		stop := a.heartbeat(ctx, h)
		a.process(ctx, id)
		stop()
		_ = a.call(ctx, "queue.complete", "run "+id, &verbsv1.QueueCompleteRequest{
			Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
		}, &verbsv1.QueueCompleteResponse{})
		a.setWorker(ctx, func(w *workerView) { w.Run, w.Claim = "", "" })
	}
}

// heartbeat renews a lease every third of its ttl until stopped: a claim is
// a lease, and one not renewed goes back to the queue for another worker.
func (a *app) heartbeat(ctx context.Context, h *verbsv1.LeaseHandle) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(claimTTL / 3)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				_ = a.call(ctx, "lease.renew", "keep "+h.GetName(), &verbsv1.LeaseRenewRequest{
					Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
					TtlMs: uint32(claimTTL.Milliseconds()),
				}, &verbsv1.LeaseRenewResponse{})
			}
		}
	}()
	return func() { close(done); <-finished }
}

// process is one run from claim to its end state.
func (a *app) process(ctx context.Context, id string) {
	r, _, err := a.getRun(ctx, id)
	if err != nil {
		return
	}
	switch r.State {
	case stateQueued:
	case stateRunning, stateWaiting:
		// Redelivered: the worker that claimed it stopped mid-run, and the
		// claim ran out. It starts over, and asks again if it has to.
	default:
		return // already ended, or restored to a state that has
	}
	r, err = a.update(ctx, id, "running", func(r *run) { r.State = stateRunning })
	if err != nil {
		return
	}
	a.setWorker(ctx, func(w *workerView) { w.Activity = "running " + id + ": " + r.Title })

	if r.Cost > a.budget && !a.approved(ctx, id, r) {
		return
	}

	// One run at a time holds the gpu, across every worker on the estate
	// and anyone else who takes it: the worker waits until it is free.
	var lease verbsv1.LeaseAcquireResponse
	for {
		err := a.call(ctx, "lease.acquire", gpuLease, &verbsv1.LeaseAcquireRequest{
			Name: gpuLease, TtlMs: uint32(claimTTL.Milliseconds()),
		}, &lease)
		if code(err) != rigv1.Code_CODE_CONFLICT || ctx.Err() != nil {
			break
		}
		a.setWorker(ctx, func(w *workerView) { w.Activity = "run " + id + ": waiting for the gpu lease, someone else holds it" })
		sleep(ctx, 2*time.Second)
	}
	if lease.GetHandle() != nil {
		lh := lease.GetHandle()
		a.setWorker(ctx, func(w *workerView) { w.Lease = fmt.Sprintf("%s token %d", lh.GetName(), lh.GetToken()) })
		stop := a.heartbeat(ctx, lh)
		defer func() {
			stop()
			_ = a.call(ctx, "lease.release", gpuLease, &verbsv1.LeaseReleaseRequest{
				Name: lh.GetName(), Token: lh.GetToken(), Epoch: lh.GetEpoch(),
			}, &verbsv1.LeaseReleaseResponse{})
			a.setWorker(ctx, func(w *workerView) { w.Lease = "" })
		}()
	}

	var transcript strings.Builder
	fmt.Fprintf(&transcript, "# %s\n\nRun %s, started %s.\n\n## Prompt\n\n%s\n\n## Steps\n\n",
		r.Title, id, time.Now().Format(time.RFC3339), r.Prompt)
	failed := strings.Contains(strings.ToLower(r.Prompt), "fail")
	for i, s := range []string{"read the assignment", "do the work", "check the result"} {
		a.setWorker(ctx, func(w *workerView) { w.Activity = fmt.Sprintf("run %s step %d of 3: %s", id, i+1, s) })
		sleep(ctx, a.step)
		fmt.Fprintf(&transcript, "%d. %s: ok\n", i+1, s)
	}
	if failed {
		transcript.WriteString("\n## Result\n\nFailed: the prompt asked to fail.\n")
	} else {
		transcript.WriteString("\n## Result\n\nDone.\n")
	}
	rel := a.writeTranscript(ctx, id, r, transcript.String())

	end := stateDone
	if failed {
		end = stateFailed
	}
	if _, err := a.update(ctx, id, end, func(r *run) {
		r.State, r.Transcript, r.Finished = end, rel, time.Now().Format(time.RFC3339)
	}); err != nil {
		// Most often a restore that ran while this run did: the store went
		// back to an export from before it existed.
		a.notify(ctx, registryv1.Severity_SEVERITY_WARNING, "Lost: "+r.Title,
			"Run "+id+" finished, but it is no longer in the store: "+short(err.Error()))
		return
	}
	r.Transcript = rel
	a.finish(ctx, id, r, end)
}

// approved parks the run on a question for Boris and waits for his answer:
// the run is marked waiting in the store, the question goes out as a toast
// and as the health report's parked question, and the page, the terminal
// and an agent can all answer it.
func (a *app) approved(ctx context.Context, id string, r run) bool {
	q := fmt.Sprintf("Run %s (%s) will cost $%.2f, over the $%.2f budget. Go ahead?", id, r.Title, r.Cost, a.budget)
	ch := make(chan bool, 1)
	a.mu.Lock()
	a.answers[id] = ch
	a.mu.Unlock()
	_, _ = a.update(ctx, id, "waiting for an answer", func(r *run) { r.State, r.Question = stateWaiting, q })
	a.setWorker(ctx, func(w *workerView) { w.Activity, w.Parked = "waiting for Boris on run "+id, q })
	a.report(ctx, "Boris's answer on run "+id)
	a.notify(ctx, registryv1.Severity_SEVERITY_URGENT, "storeworker needs you", q+
		` Answer in the storeworker pane, or: rig storeworker answer --args '{"run":"`+id+`","yes":true}'`)

	var yes bool
	select {
	case yes = <-ch:
	case <-ctx.Done():
		return false
	}
	a.setWorker(ctx, func(w *workerView) { w.Parked = "" })
	a.report(ctx, "")
	if !yes {
		_, _ = a.update(ctx, id, "denied", func(r *run) {
			r.State, r.Question, r.Finished = stateDenied, "", time.Now().Format(time.RFC3339)
		})
		a.notify(ctx, registryv1.Severity_SEVERITY_INFO, "Not run: "+r.Title, "You said no, so run "+id+" stopped.")
		return false
	}
	_, _ = a.update(ctx, id, "approved", func(r *run) { r.State, r.Question = stateRunning, "" })
	a.setWorker(ctx, func(w *workerView) { w.Activity = "running " + id + ": " + r.Title })
	return true
}

// writeTranscript asks rig where the file goes (files.place), writes it
// there directly, and indexes it (files.index) so a search finds it. It
// answers the path relative to the free-files root, or "" if rig keeps no
// free files here.
func (a *app) writeTranscript(ctx context.Context, id string, r run, text string) string {
	var place verbsv1.FilesPlaceResponse
	if err := a.call(ctx, "files.place", "programs/"+id+".md", &verbsv1.FilesPlaceRequest{
		Kind: "programs", Name: id + ".md",
	}, &place); err != nil {
		return ""
	}
	if err := os.MkdirAll(filepath.Dir(place.GetPath()), 0o700); err != nil {
		return ""
	}
	if err := os.WriteFile(place.GetPath(), []byte(text), 0o600); err != nil {
		return ""
	}
	_ = a.call(ctx, "files.index", place.GetRelative(), &verbsv1.FilesIndexRequest{
		Path: place.GetRelative(), Title: r.Title,
		Summary: "storeworker transcript of run " + id, Tags: []string{"storeworker", "transcript"},
	}, &verbsv1.FilesIndexResponse{})
	return place.GetRelative()
}

// finish tells everyone who should know: a toast for Boris, a lesson when
// it failed, and mail to the seat that asked.
func (a *app) finish(ctx context.Context, id string, r run, end string) {
	if end == stateFailed {
		a.notify(ctx, registryv1.Severity_SEVERITY_ERROR, "Failed: "+r.Title, "Run "+id+" failed; a lesson was recorded.")
		_ = a.call(ctx, "knowledge.add", "lesson from "+id, &verbsv1.KnowledgeAddRequest{
			Title:   "storeworker: a prompt that asks to fail, fails",
			Summary: "Run " + id + " (" + r.Title + ") failed because its prompt contained the word fail.",
			Body:    "The fake runner fails any run whose prompt contains \"fail\". Remove the word to run it.",
			Tags:    []string{"storeworker", "failure"},
		}, &verbsv1.KnowledgeAddResponse{})
	} else {
		a.notify(ctx, registryv1.Severity_SEVERITY_SUCCESS, "Done: "+r.Title, "Run "+id+" finished.")
	}
	if r.ReplyTo != "" {
		_ = a.call(ctx, "message.send", "to "+r.ReplyTo, &verbsv1.MessageSendRequest{
			To: r.ReplyTo, Subject: "storeworker run " + id + " " + end,
			Body: fmt.Sprintf("%s ended %s. Transcript: files/%s", r.Title, end, r.Transcript),
		}, &verbsv1.MessageSendResponse{})
	}
	a.setWorker(ctx, func(w *workerView) { w.Done++ })
	a.report(ctx, "")
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
