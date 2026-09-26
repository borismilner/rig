package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// The actions. The page, `rig storeworker <command>` at a terminal, and an
// agent through rig's invoke all end here, so the three are one program.

// seatPattern is what a reply-to seat may be before it is sent to rig.
var seatPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)

type assignArgs struct {
	Title   string `json:"title"`
	Prompt  string `json:"prompt"`
	ReplyTo string `json:"reply_to"`
}

// assign stores a run and queues it. The run and the day's counter are one
// store.transact, so the counter never counts a run that was not stored;
// the queue push carries the run id as its idempotency key, so a retried
// assign never queues a run twice.
func (a *app) assign(ctx context.Context, args assignArgs) (map[string]any, error) {
	args.Title = strings.TrimSpace(args.Title)
	if args.Title == "" {
		return nil, invalid("a run needs a title", `{"title":"summarise the inbox","prompt":"..."}`)
	}
	if args.ReplyTo != "" && !seatPattern.MatchString(args.ReplyTo) {
		return nil, invalid("reply_to is a seat name: letters, digits, '.', '_', ':' or '-'", `{"reply_to":"team-lead"}`)
	}
	now := time.Now()
	id := "r" + now.UTC().Format("0102-150405.000")
	id = strings.ReplaceAll(id, ".", "-")
	r := run{
		Title: args.Title, Prompt: args.Prompt, State: stateQueued,
		Cost:    costOf(args.Prompt),
		ReplyTo: args.ReplyTo, Created: now.Format(time.RFC3339),
	}
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	day := now.Format("2006-01-02")
	count, version := 0, uint64(0)
	var got verbsv1.StoreGetResponse
	if err := a.call(ctx, "store.get", "stats/"+day, &verbsv1.StoreGetRequest{
		Collection: "stats", Ids: []string{day},
	}, &got); err != nil {
		return nil, err
	}
	if docs := got.GetDocuments(); len(docs) == 1 {
		var s struct {
			Assigned int `json:"assigned"`
		}
		_ = json.Unmarshal([]byte(docs[0].GetDocument()), &s)
		count, version = s.Assigned, docs[0].GetVersion()
	}
	stats := fmt.Sprintf(`{"assigned":%d}`, count+1)
	if err := a.call(ctx, "store.transact", "run "+id+" and the day's count, all or none", &verbsv1.StoreTransactRequest{
		Ops: []*verbsv1.StoreOp{
			{Collection: "runs", Id: id, Document: string(body)},
			{Collection: "stats", Id: day, ExpectedVersion: version, Document: stats},
		},
	}, &verbsv1.StoreTransactResponse{}); err != nil {
		return nil, err
	}
	if err := a.call(ctx, "queue.push", "run "+id, &verbsv1.QueuePushRequest{
		Queue: a.queue, IdempotencyKey: id, Payload: []byte(id),
	}, &verbsv1.QueuePushResponse{}); err != nil {
		return nil, err
	}
	a.notify(ctx, registryv1.Severity_SEVERITY_INFO, "Queued: "+r.Title,
		fmt.Sprintf("Run %s will cost about $%.2f.", id, r.Cost))
	return map[string]any{"run": id, "state": r.State, "cost": r.Cost}, nil
}

// costOf is the fake price of a prompt: a cent per four characters, so a
// long prompt goes over the budget and parks.
func costOf(prompt string) float64 {
	return float64(len(prompt)) / 400
}

type runsArgs struct {
	State string `json:"state"`
	Limit int    `json:"limit"`
}

type runRow struct {
	ID      string `json:"id"`
	Version uint64 `json:"version"`
	run
}

// runs lists runs newest first, one state or all (store.query), and counts
// every state (store.query with count_only).
func (a *app) runs(ctx context.Context, args runsArgs) (map[string]any, error) {
	q := &verbsv1.StoreQueryRequest{
		Collection: "runs",
		Order:      []*verbsv1.StoreOrder{{Field: "created", Desc: true}},
		Limit:      uint32(min(max(args.Limit, 1), 200)), //nolint:gosec // clamped
	}
	if args.Limit == 0 {
		q.Limit = 50
	}
	note := "newest first"
	if args.State != "" {
		q.Where = []*verbsv1.StoreCondition{{Field: "state", Op: "eq", Value: fmt.Sprintf("%q", args.State)}}
		note = "state eq " + args.State
	}
	var resp verbsv1.StoreQueryResponse
	if err := a.call(ctx, "store.query", note, q, &resp); err != nil {
		return nil, err
	}
	rows := make([]runRow, 0, len(resp.GetDocuments()))
	for _, d := range resp.GetDocuments() {
		row := runRow{ID: d.GetId(), Version: d.GetVersion()}
		if err := json.Unmarshal([]byte(d.GetDocument()), &row.run); err != nil {
			continue
		}
		rows = append(rows, row)
	}
	counts := map[string]uint64{}
	for _, s := range states {
		var c verbsv1.StoreQueryResponse
		if err := a.call(ctx, "store.query", "count state eq "+s, &verbsv1.StoreQueryRequest{
			Collection: "runs", CountOnly: true,
			Where: []*verbsv1.StoreCondition{{Field: "state", Op: "eq", Value: fmt.Sprintf("%q", s)}},
		}, &c); err != nil {
			return nil, err
		}
		counts[s] = c.GetTotal()
	}
	return map[string]any{"runs": rows, "total": resp.GetTotal(), "counts": counts}, nil
}

type answerArgs struct {
	Run string `json:"run"`
	Yes bool   `json:"yes"`
}

// answer is Boris answering a parked run, from the page, the terminal or
// an agent: the worker waiting on it goes on or stops.
func (a *app) answer(_ context.Context, args answerArgs) (map[string]any, error) {
	a.mu.Lock()
	ch, ok := a.answers[args.Run]
	if ok {
		delete(a.answers, args.Run)
	}
	a.mu.Unlock()
	if !ok {
		return nil, &client.CallError{Method: "storeworker.answer", Status: &rigv1.Status{
			Code:         rigv1.Code_CODE_NOT_FOUND,
			Message:      fmt.Sprintf("run %q is not waiting for an answer", args.Run),
			Precondition: "the run is parked on a question",
			Fix:          "list the waiting runs",
			FixCommand:   `rig storeworker runs --args '{"state":"waiting"}'`,
		}}
	}
	ch <- args.Yes
	return map[string]any{"run": args.Run, "answered": args.Yes}, nil
}

// exportStore is store.export: every collection as text, committed to git.
func (a *app) exportStore(ctx context.Context) (map[string]any, error) {
	var resp verbsv1.StoreExportResponse
	if err := a.call(ctx, "store.export", "every collection", &verbsv1.StoreExportRequest{}, &resp); err != nil {
		return nil, err
	}
	note := "committed " + short7(resp.GetCommit())
	if resp.GetCommit() == "" {
		note = "unchanged since the last export"
	}
	a.mu.Lock()
	a.export = exportView{Commit: resp.GetCommit(), Dir: resp.GetDir(), Note: note}
	a.mu.Unlock()
	a.notify(ctx, registryv1.Severity_SEVERITY_SUCCESS, "Exported the store", note)
	return map[string]any{"dir": resp.GetDir(), "commit": resp.GetCommit(), "collections": len(resp.GetCollections())}, nil
}

// restore is store.import: the store as it was at the last export. rig
// keeps a snapshot of the store it replaced.
func (a *app) restore(ctx context.Context) (map[string]any, error) {
	var resp verbsv1.StoreImportResponse
	if err := a.call(ctx, "store.import", "every exported collection", &verbsv1.StoreImportRequest{}, &resp); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.export.Snapshot = resp.GetSnapshot()
	a.export.Note = fmt.Sprintf("restored %d collection(s)", len(resp.GetCollections()))
	a.mu.Unlock()
	a.notify(ctx, registryv1.Severity_SEVERITY_WARNING, "Restored the store from its export",
		"The store before it is kept at "+resp.GetSnapshot())
	return map[string]any{"snapshot": resp.GetSnapshot(), "collections": len(resp.GetCollections())}, nil
}

func short7(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// search asks rig's index over the free files (files.search); the worker
// indexed each transcript as it wrote it.
func (a *app) search(ctx context.Context, words string) ([]*verbsv1.FilesHit, error) {
	var resp verbsv1.FilesSearchResponse
	err := a.call(ctx, "files.search", words, &verbsv1.FilesSearchRequest{Query: words, Limit: 10}, &resp)
	return resp.GetHits(), err
}

// lessons asks the shared lessons (knowledge.search): what failed runs
// taught, readable by every program and agent.
func (a *app) lessons(ctx context.Context, words string) ([]*verbsv1.LessonHit, error) {
	var resp verbsv1.KnowledgeSearchResponse
	err := a.call(ctx, "knowledge.search", words, &verbsv1.KnowledgeSearchRequest{Query: words, Limit: 10}, &resp)
	return resp.GetHits(), err
}

func invalid(msg, example string) error {
	return &client.CallError{Method: "storeworker", Status: &rigv1.Status{
		Code: rigv1.Code_CODE_INVALID, Message: "storeworker: " + msg,
		Fix: "pass arguments like " + example,
	}}
}
