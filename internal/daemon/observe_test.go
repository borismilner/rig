package daemon

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/observe"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// plan/49 acceptance 2 over the wire: rigd's own records read back from a
// segment, through the principal filter, and followed with a cursor.
func TestLogsQueryReadsTheStoreThroughThePrincipalFilter(t *testing.T) {
	store := observe.New(observe.Options{})
	if err := store.Attach(filepath.Join(t.TempDir(), "logs")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	log := slog.New(observe.NewHandler(store, "rigd", slog.LevelDebug, nil))
	log.Info("rigd up", "pid", 7)
	log.Debug("a debug line")
	store.Append(observe.Record{At: time.Now().UnixNano(), Client: "graft", Message: "graft's own"})

	sock, _ := upDaemonWith(t, func(c *Config) { c.Logs = store })
	ctx := ctx5(t)
	term := dial(t, sock)

	var all registryv1.LogsQueryResponse
	if err := term.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{}, &all); err != nil {
		t.Fatal(err)
	}
	if got := len(all.GetRecords()); got != 2 || all.GetRecords()[0].GetMessage() != "rigd up" ||
		all.GetRecords()[0].GetAttrs()["pid"] != "7" || all.GetLatest() != 3 {
		t.Fatalf("a terminal read %v", &all)
	}

	var debug registryv1.LogsQueryResponse
	if err := term.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{MinLevel: -4, Clients: []string{"rigd"}}, &debug); err != nil {
		t.Fatal(err)
	}
	if len(debug.GetRecords()) != 2 || debug.GetRecords()[1].GetMessage() != "a debug line" {
		t.Fatalf("rigd at debug: %v", &debug)
	}

	// A program reads its own records and nobody else's (decision 13).
	graft := eventProgram(t, sock, "graft")
	var own registryv1.LogsQueryResponse
	if err := graft.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{}, &own); err != nil {
		t.Fatal(err)
	}
	if len(own.GetRecords()) != 1 || own.GetRecords()[0].GetClient() != "graft" {
		t.Fatalf("a program read %v", &own)
	}

	err := term.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{Grep: "("}, &registryv1.LogsQueryResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a grep that does not compile")

	// Follow: the call parks until the next record after the cursor.
	got := make(chan *registryv1.LogsQueryResponse, 1)
	go func() {
		var next registryv1.LogsQueryResponse
		_ = term.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{After: all.GetLatest(), TimeoutMs: 3000}, &next)
		got <- &next
	}()
	time.Sleep(50 * time.Millisecond)
	log.Warn("after the cursor")
	select {
	case next := <-got:
		if len(next.GetRecords()) != 1 || next.GetRecords()[0].GetMessage() != "after the cursor" {
			t.Fatalf("the follow woke with %v", next)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("the follow never woke")
	}
}

func TestLogsQueryWithoutAStoreSaysSo(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	err := dial(t, sock).Call(ctx5(t), "rig.logs.query", &registryv1.LogsQueryRequest{}, &registryv1.LogsQueryResponse{})
	wantCode(t, err, rigv1.Code_CODE_UNAVAILABLE, "a daemon with no store")
}

func logStore(t *testing.T) (*observe.Store, string) {
	t.Helper()
	store := observe.New(observe.Options{RateRecords: 10, RateBurst: 20})
	if err := store.Attach(filepath.Join(t.TempDir(), "logs")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sock, _ := upDaemonWith(t, func(c *Config) { c.Logs = store })
	return store, sock
}

// Decision 4: a program's records are filed under its own name whatever
// the batch says, past its ceiling they are counted as sampled, and its own
// dropped records become a band.
func TestIngestFilesUnderTheCallersNameAndCountsWhatItSampled(t *testing.T) {
	store, sock := logStore(t)
	ctx := ctx5(t)
	graft := eventProgram(t, sock, "graft")
	batch := &registryv1.LogsIngestRequest{DroppedBefore: 3}
	for range 25 {
		batch.Records = append(batch.Records, &registryv1.LogRecord{Client: "rigd", Seq: 99, Level: 4, Message: "job failed", Attrs: map[string]string{"job": "7"}})
	}
	var resp registryv1.LogsIngestResponse
	if err := graft.Call(ctx, "rig.logs.ingest", batch, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetAccepted() != 20 || resp.GetSampled() != 5 {
		t.Fatalf("accepted %d, sampled %d; want 20 and 5", resp.GetAccepted(), resp.GetSampled())
	}
	res, err := store.Query(observe.Query{Clients: []string{"graft"}})
	if err != nil || len(res.Records) != 20 || res.Records[0].Attrs["job"] != "7" || res.Records[0].Level != 4 {
		t.Fatalf("graft's records read %+v, %v", res.Records, err)
	}
	if forged, _ := store.Query(observe.Query{Clients: []string{"rigd"}, MinLevel: 4}); len(forged.Records) != 0 {
		t.Fatalf("a batch naming rigd was filed as rigd: %+v", forged.Records)
	}
	causes := map[string]uint64{}
	for _, g := range res.Gaps {
		causes[g.Cause] += g.Total
	}
	if causes[observe.CauseClientDropped] != 3 || causes[observe.CauseSampled] != 5 {
		t.Fatalf("the bands read %+v", res.Gaps)
	}

	big := &registryv1.LogsIngestRequest{Records: make([]*registryv1.LogRecord, maxIngestBatch+1)}
	for i := range big.GetRecords() {
		big.Records[i] = &registryv1.LogRecord{}
	}
	wantCode(t, graft.Call(ctx, "rig.logs.ingest", big, &resp), rigv1.Code_CODE_INVALID, "a batch over the bound")
}

// A seated agent's records go under its seat; and a seat cannot pose as rigd.
func TestIngestFromASeatIsFiledUnderTheSeat(t *testing.T) {
	store, sock := logStore(t)
	ctx := ctx5(t)
	agent := seated(t, sock, "backend-1")
	if err := agent.Call(ctx, "rig.logs.ingest", &registryv1.LogsIngestRequest{
		Records: []*registryv1.LogRecord{{Message: "built"}},
	}, &registryv1.LogsIngestResponse{}); err != nil {
		t.Fatal(err)
	}
	res, _ := store.Query(observe.Query{Clients: []string{"backend-1"}})
	if len(res.Records) != 1 || res.Records[0].At == 0 {
		t.Fatalf("the seat's record read %+v", res.Records)
	}
	rigd := seated(t, sock, "rigd")
	err := rigd.Call(ctx, "rig.logs.ingest", &registryv1.LogsIngestRequest{
		Records: []*registryv1.LogRecord{{Message: "forged"}},
	}, &registryv1.LogsIngestResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a seat named rigd ingesting")
}

// Decision 13: opening another client's records writes one audit entry per
// reader per view per minute; reading your own writes none.
func TestReadingAnotherClientsLogsIsAuditedOncePerMinute(t *testing.T) {
	store, sock := logStore(t)
	ctx := ctx5(t)
	graft := eventProgram(t, sock, "graft")
	if err := graft.Call(ctx, "rig.logs.ingest", &registryv1.LogsIngestRequest{
		Records: []*registryv1.LogRecord{{Message: "graft's"}},
	}, &registryv1.LogsIngestResponse{}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := graft.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{}, &registryv1.LogsQueryResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	audits := func() []observe.Record {
		res, _ := store.Query(observe.Query{Kind: observe.KindAudit})
		return res.Records
	}
	if a := audits(); len(a) != 0 {
		t.Fatalf("a program reading its own logs was audited: %+v", a)
	}
	term := dial(t, sock)
	for range 3 {
		if err := term.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{Clients: []string{"graft"}}, &registryv1.LogsQueryResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	a := audits()
	if len(a) != 1 || a[0].Attrs["clients"] != "graft" || a[0].Attrs["reader"] == "" {
		t.Fatalf("three reads of graft's logs audited as %+v", a)
	}
	var shown registryv1.LogsQueryResponse
	if err := term.Call(ctx, "rig.logs.query", &registryv1.LogsQueryRequest{Clients: []string{"rigd"}, MinLevel: -8}, &shown); err != nil {
		t.Fatal(err)
	}
	for _, r := range shown.GetRecords() {
		if r.GetMessage() == "read another client's logs" {
			t.Fatal("rig logs showed an audit entry")
		}
	}
}

// Decision 11's coverage over the wire: the segments with their size, and
// the gaps the caller may read, so a program is shown only its own holes.
func TestCoverageShowsTheSegmentsAndOnlyTheCallersGaps(t *testing.T) {
	store, sock := logStore(t)
	ctx := ctx5(t)
	now := time.Now().UnixNano()
	store.Append(observe.Record{At: now, Client: "graft", Message: "one"})
	store.NoteGap(observe.Gap{From: now, To: now + 1, Client: "graft", Cause: observe.CauseClientDropped, Total: 2})
	store.NoteGap(observe.Gap{From: now, To: now + 1, Client: "shelf", Cause: observe.CauseClientDropped, Total: 4})

	var all registryv1.LogsCoverageResponse
	if err := dial(t, sock).Call(ctx, "rig.logs.coverage", &registryv1.LogsCoverageRequest{}, &all); err != nil {
		t.Fatal(err)
	}
	segs := all.GetSegments()
	if len(segs) == 0 || !segs[len(segs)-1].GetOpen() || all.GetBytes() == 0 {
		t.Fatalf("a terminal's coverage read %v", &all)
	}
	if len(all.GetGaps()) != 2 {
		t.Fatalf("a terminal was shown %d gaps, want both", len(all.GetGaps()))
	}

	var own registryv1.LogsCoverageResponse
	if err := eventProgram(t, sock, "graft").Call(ctx, "rig.logs.coverage", &registryv1.LogsCoverageRequest{}, &own); err != nil {
		t.Fatal(err)
	}
	if len(own.GetGaps()) != 1 || own.GetGaps()[0].GetClient() != "graft" {
		t.Fatalf("graft was shown %v", own.GetGaps())
	}
}

// A pin holds every client's records, so only a caller that may read them
// all sets one; a segment that is not there is NOT_FOUND, not a silent no.
func TestPinNeedsIntrospectionAndAnExistingSegment(t *testing.T) {
	store, sock := logStore(t)
	ctx := ctx5(t)
	store.Append(observe.Record{At: time.Now().UnixNano(), Client: "rigd", Message: "one"})
	segs, err := store.Segments()
	if err != nil || len(segs) == 0 {
		t.Fatalf("%v %v", segs, err)
	}
	id := segs[0].ID

	err = eventProgram(t, sock, "graft").Call(ctx, "rig.logs.pin", &registryv1.LogsPinRequest{Segment: id}, &registryv1.LogsPinResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a program pinning")

	term := dial(t, sock)
	for _, step := range []struct {
		unpin, changed bool
	}{{false, true}, {false, false}, {true, true}, {true, false}} {
		var resp registryv1.LogsPinResponse
		if err := term.Call(ctx, "rig.logs.pin", &registryv1.LogsPinRequest{Segment: id, Unpin: step.unpin}, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.GetChanged() != step.changed {
			t.Fatalf("unpin=%v answered changed=%v, want %v", step.unpin, resp.GetChanged(), step.changed)
		}
	}
	err = term.Call(ctx, "rig.logs.pin", &registryv1.LogsPinRequest{Segment: id + 999_999}, &registryv1.LogsPinResponse{})
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "a segment that is not there")
}
