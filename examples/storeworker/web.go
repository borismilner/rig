package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// The pane's server: the page, pane.js, and a small JSON API the page polls.
// It listens on loopback only, which rig requires of a pane_url.

// web holds what the page does as Boris rather than as the program: a second
// connection, not registered, which is how a terminal reaches rig. Holding
// the gpu lease and calling storeworker's own commands through rig both go
// over it, so rig sees a different caller from the worker.
type web struct {
	a    *app
	addr string
	you  *client.Client

	mu      sync.Mutex
	youGPU  *verbsv1.LeaseHandle
	gpuStop context.CancelFunc
}

// handler is the mux. Every /api call passes guard first.
func (w *web) handler(rigFS http.FileSystem) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /pane", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("content-type", "text/html; charset=utf-8")
		rw.Header().Set("cache-control", "no-store")
		_, _ = io.WriteString(rw, page)
	})
	mux.Handle("GET /rig/", http.StripPrefix("/rig/", http.FileServer(rigFS)))
	mux.HandleFunc("GET /api/tab/{name}", w.guard(w.tab))
	mux.HandleFunc("POST /api/do/{action}", w.guard(w.do))
	return mux
}

// guard refuses a request that did not come from this page.
//
// ⛔ A LOOPBACK SERVER IS STILL REACHABLE FROM ANY WEB PAGE Boris OPENS. The
// Host must be the address served, so a rebound DNS name is refused, and
// every call must carry a header a cross-site form cannot set without a
// preflight this server never answers.
func (w *web) guard(next func(*http.Request) (any, error)) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Host != w.addr || r.Header.Get("X-Storeworker") != "1" {
			http.Error(rw, "not from the storeworker page", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(rw, r.Body, 64<<10)
		out, err := next(r)
		rw.Header().Set("content-type", "application/json")
		rw.Header().Set("cache-control", "no-store")
		if err != nil {
			rw.WriteHeader(http.StatusOK) // the page shows refusals, they are part of the demo
			_ = json.NewEncoder(rw).Encode(map[string]any{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(rw).Encode(out)
	}
}

func reqCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 15*time.Second)
}

// tab answers one tab's data. Only the tab on screen is polled.
func (w *web) tab(r *http.Request) (any, error) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	ctx = quietly(ctx)
	a := w.a
	a.mu.Lock()
	base := map[string]any{
		"worker": a.worker, "health": a.health, "export": a.export,
		"toasts": append([]toastRow(nil), a.toasts...), "calls": append([]callRow(nil), a.calls...),
		"program": a.id, "seat": a.seat, "queue": a.queue, "budget": a.budget,
	}
	a.mu.Unlock()
	q := r.URL.Query()
	put := func(k string, v any, err error) {
		if err != nil {
			base[k+"_error"] = err.Error()
			return
		}
		base[k] = v
	}
	switch r.PathValue("name") {
	case "program":
		cmds := make([]map[string]any, 0, len(commands))
		for _, c := range commands {
			cmds = append(cmds, map[string]any{
				"id": c.id, "summary": c.summary, "example": c.example,
				"effects":  strings.ToLower(strings.TrimPrefix(c.effects.String(), "EFFECTS_")),
				"confirms": c.confirms == rigv1.Tristate_TRISTATE_YES,
			})
		}
		base["commands"] = cmds
	case "store":
		runs, err := a.runs(ctx, runsArgs{State: q.Get("state")})
		put("runs", runs, err)
		var cs verbsv1.StoreCollectionsResponse
		err = a.call(ctx, "store.collections", "", &verbsv1.StoreCollectionsRequest{}, &cs)
		put("collections", cs.GetCollections(), err)
	case "queue":
		var ql verbsv1.QueueListResponse
		err := a.call(ctx, "queue.list", a.queue, &verbsv1.QueueListRequest{Queue: a.queue}, &ql)
		put("tasks", tasksView(ql.GetTasks()), err)
		base["done"] = ql.GetDone()
	case "leases":
		var ll verbsv1.LeaseListResponse
		err := a.call(ctx, "lease.list", "", &verbsv1.LeaseListRequest{}, &ll)
		put("leases", leasesView(ll.GetLeases()), err)
		w.mu.Lock()
		base["you_hold"] = w.youGPU != nil
		w.mu.Unlock()
	case "asking":
		runs, err := a.runs(ctx, runsArgs{State: stateWaiting})
		put("waiting", runs, err)
	case "toasts":
		var dnd registryv1.ToastDndResponse
		err := a.call(ctx, "toast.dnd", "query", &registryv1.ToastDndRequest{Change: registryv1.DndChange_DND_CHANGE_QUERY}, &dnd)
		put("dnd", map[string]any{"on": dnd.GetOn(), "suppressed": dnd.GetSuppressed()}, err)
	case "files":
		words := q.Get("q")
		if words == "" {
			words = "storeworker"
		}
		hits, err := a.search(ctx, words)
		put("hits", hits, err)
		var un verbsv1.FilesUnindexedResponse
		err = a.call(ctx, "files.unindexed", "programs/"+a.id, &verbsv1.FilesUnindexedRequest{Under: "programs/" + a.id, Limit: 20}, &un)
		put("unindexed", un.GetFiles(), err)
		var lay verbsv1.FilesLayoutResponse
		err = a.call(ctx, "files.layout", "", &verbsv1.FilesLayoutRequest{}, &lay)
		put("layout", lay.GetKinds(), err)
	case "lessons":
		words := q.Get("q")
		if words == "" {
			words = "storeworker"
		}
		hits, err := a.lessons(ctx, words)
		put("hits", hits, err)
	case "mail":
		var peers verbsv1.PeersResponse
		err := a.call(ctx, "peers", "", &verbsv1.PeersRequest{}, &peers)
		put("peers", seatsView(peers.GetCrew()), err)
		var inbox verbsv1.MessageInboxResponse
		err = a.call(ctx, "message.inbox", "seat "+a.seat, &verbsv1.MessageInboxRequest{Limit: 20}, &inbox)
		put("inbox", mailView(inbox.GetMessages()), err)
	case "export":
		a.mu.Lock()
		dir := a.export.Dir
		a.mu.Unlock()
		if dir != "" {
			base["preview"] = head(filepath.Join(dir, "runs.jsonl"), 6)
		}
	case "health":
		var peers verbsv1.PeersResponse
		err := a.call(ctx, "peers", "", &verbsv1.PeersRequest{}, &peers)
		put("peers", seatsView(peers.GetCrew()), err)
	case "words": // the page carries them; the worker's state is all it needs
	default:
		return nil, errors.New("no such tab")
	}
	return base, nil
}

// do runs one of the page's actions.
func (w *web) do(r *http.Request) (any, error) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	var in struct {
		Title, Prompt, ReplyTo, Run, Severity, To, Subject, Body, Summary, Command, Args string
		Yes                                                                              bool
		Seconds                                                                          int
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("the request is not JSON: %w", err)
	}
	a := w.a
	switch r.PathValue("action") {
	case "assign":
		return a.assign(ctx, assignArgs{Title: in.Title, Prompt: in.Prompt, ReplyTo: in.ReplyTo})
	case "assign-three":
		var ids []any
		for i := 1; i <= 3; i++ {
			out, err := a.assign(ctx, assignArgs{Title: fmt.Sprintf("quick run %d of 3", i), Prompt: "short"})
			if err != nil {
				return nil, err
			}
			ids = append(ids, out["run"])
			time.Sleep(5 * time.Millisecond) // ids are by the millisecond
		}
		return map[string]any{"runs": ids}, nil
	case "assign-expensive":
		return a.assign(ctx, assignArgs{Title: "an expensive run", Prompt: strings.Repeat("a long prompt ", 60)})
	case "assign-failing":
		return a.assign(ctx, assignArgs{Title: "a run that fails", Prompt: "please fail"})
	case "answer":
		return a.answer(ctx, answerArgs{Run: in.Run, Yes: in.Yes})
	case "stale-write":
		return a.staleWrite(ctx, in.Run)
	case "gpu-hold":
		return w.holdGPU(in.Seconds)
	case "gpu-release":
		return w.releaseGPU(ctx)
	case "toast":
		sev, ok := registryv1.Severity_value["SEVERITY_"+strings.ToUpper(in.Severity)]
		if !ok || sev == 0 {
			return nil, fmt.Errorf("no severity %q", in.Severity)
		}
		a.notify(ctx, registryv1.Severity(sev), "storeworker says "+in.Severity, "Sent from the Toasts tab.")
		return map[string]any{"sent": in.Severity}, nil
	case "mail":
		var resp verbsv1.MessageSendResponse
		err := a.call(ctx, "message.send", "to "+in.To, &verbsv1.MessageSendRequest{
			To: in.To, Subject: in.Subject, Body: in.Body,
		}, &resp)
		return map[string]any{"id": resp.GetMessage().GetId(), "delivered": resp.GetDelivered(),
			"state": strings.ToLower(strings.TrimPrefix(resp.GetToState().String(), "SEAT_STATE_"))}, err
	case "lesson":
		var resp verbsv1.KnowledgeAddResponse
		err := a.call(ctx, "knowledge.add", in.Title, &verbsv1.KnowledgeAddRequest{
			Title: in.Title, Summary: in.Summary, Body: in.Body, Tags: []string{"storeworker"},
		}, &resp)
		return map[string]any{"added": in.Title}, err
	case "export":
		return a.exportStore(ctx)
	case "restore":
		// The page asked in place: the button takes a second press.
		return a.restore(ctx)
	case "invoke":
		return w.invoke(ctx, in.Command, in.Args)
	}
	return nil, errors.New("no such action")
}

// staleWrite shows compare-and-swap: it reads a run, writes it at the version
// read, then writes again at that SAME version, which rig refuses.
func (a *app) staleWrite(ctx context.Context, id string) (any, error) {
	r, v, err := a.getRun(ctx, id)
	if err != nil {
		return nil, err
	}
	first, err := a.putRun(ctx, id, v, r, "at the version read")
	if err != nil {
		return nil, err
	}
	_, stale := a.putRun(ctx, id, v, r, "again at the old version")
	return map[string]any{
		"read": v, "first_write": first,
		"second_write": fmt.Sprintf("refused: %v", stale),
	}, nil
}

// holdGPU takes the gpu lease as Boris for some seconds, so the worker has
// to wait for it.
func (w *web) holdGPU(seconds int) (any, error) {
	seconds = min(max(seconds, 5), 120)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.youGPU != nil {
		return nil, errors.New("you already hold it")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var resp verbsv1.LeaseAcquireResponse
	if err := w.you.Call(ctx, "rig.lease.acquire", &verbsv1.LeaseAcquireRequest{
		Name: gpuLease, TtlMs: uint32(seconds * 1000), //nolint:gosec // clamped above
	}, &resp); err != nil {
		w.a.logCall("lease.acquire", gpuLease, err)
		return nil, err
	}
	w.a.logCall("lease.acquire", fmt.Sprintf("%s for %ds, as you", gpuLease, seconds), nil)
	w.youGPU = resp.GetHandle()
	// The lease simply runs out at its ttl; forgetting it then keeps the
	// page honest without a renew loop.
	gctx, gcancel := context.WithCancel(context.Background())
	w.gpuStop = gcancel
	go func() {
		sleep(gctx, time.Duration(seconds)*time.Second)
		w.mu.Lock()
		w.youGPU = nil
		w.mu.Unlock()
	}()
	return map[string]any{"holder": resp.GetHandle().GetHolder(), "token": resp.GetHandle().GetToken(), "seconds": seconds}, nil
}

func (w *web) releaseGPU(ctx context.Context) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := w.youGPU
	if h == nil {
		return nil, errors.New("you do not hold it")
	}
	err := w.you.Call(ctx, "rig.lease.release", &verbsv1.LeaseReleaseRequest{
		Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch(),
	}, &verbsv1.LeaseReleaseResponse{})
	w.a.logCall("lease.release", gpuLease+", as you", err)
	w.youGPU = nil
	if w.gpuStop != nil {
		w.gpuStop()
	}
	return map[string]any{"released": err == nil}, err
}

// invoke calls one of storeworker's own commands THROUGH rig, as a terminal
// or an agent would: out over the second connection, routed by rig to this
// program's handler, and back.
func (w *web) invoke(ctx context.Context, cmd, args string) (any, error) {
	known := false
	for _, c := range commands {
		known = known || c.id == cmd
	}
	if !known {
		return nil, fmt.Errorf("storeworker has no command %q", cmd)
	}
	if strings.TrimSpace(args) == "" {
		args = "{}"
	}
	var resp rigv1.CallResponse
	err := w.you.Call(ctx, w.a.id+"."+cmd, &rigv1.CallRequest{Args: []byte(args)}, &resp)
	w.a.logCall(w.a.id+"."+cmd, "your call, routed by rig", err)
	if err != nil {
		return nil, err
	}
	return map[string]any{"result": json.RawMessage(resp.GetResult())}, nil
}

// logCall records a call made outside call: as Boris rather than as the
// program, or one call logged only on some answers.
func (a *app) logCall(verb, note string, err error) {
	row := callRow{At: time.Now().Format("15:04:05"), Verb: verb, Note: note}
	if err != nil {
		row.Failed, row.Note = true, note+": "+short(err.Error())
	}
	a.logRow(row)
}

func tasksView(ts []*verbsv1.Task) []map[string]any {
	out := make([]map[string]any, 0, len(ts))
	for _, t := range ts {
		out = append(out, map[string]any{
			"id": t.GetId(), "key": t.GetIdempotencyKey(), "attempts": t.GetAttempts(),
			"state":  strings.ToLower(strings.TrimPrefix(t.GetState().String(), "TASK_STATE_")),
			"holder": t.GetClaim().GetHolder(),
		})
	}
	return out
}

func leasesView(ls []*verbsv1.Lease) []map[string]any {
	out := make([]map[string]any, 0, len(ls))
	for _, l := range ls {
		out = append(out, map[string]any{
			"name": l.GetName(), "holder": l.GetHolder(), "token": l.GetToken(),
			"remaining_ms": l.GetRemainingMs(),
			"state":        strings.ToLower(strings.TrimPrefix(l.GetState().String(), "LEASE_STATE_")),
		})
	}
	return out
}

func seatsView(ss []*verbsv1.Seat) []map[string]any {
	out := make([]map[string]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, map[string]any{
			"seat": s.GetSeat(), "purpose": s.GetPurpose(), "activity": s.GetActivity(),
			"state": strings.ToLower(strings.TrimPrefix(s.GetState().String(), "SEAT_STATE_")),
		})
	}
	return out
}

func mailView(ms []*verbsv1.Message) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			"id": m.GetId(), "from": m.GetFrom(), "subject": m.GetSubject(), "body": m.GetBody(),
			"sent": time.Unix(0, m.GetSentUnixNano()).Format("15:04:05"),
		})
	}
	return out
}

// head is the first n lines of a file this program's export wrote.
func head(path string, n int) []string {
	f, err := os.Open(path) //nolint:gosec // rig's answer for this program's own export
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() && len(out) < n {
		out = append(out, short(sc.Text()))
	}
	return out
}
