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
