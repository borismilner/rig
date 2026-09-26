package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/instance"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// upStoreDaemon is an unnamed daemon told a storage root, serving a socket
// for programs and terminals; its MCP door is mailAgent(t, d, name).
func upStoreDaemon(t *testing.T) (sock, root string, d *Daemon) {
	t.Helper()
	return upRootedDaemon(t, 0)
}

// upRootedDaemon is upStoreDaemon with the free files' commit interval.
func upRootedDaemon(t *testing.T, every time.Duration) (sock, root string, d *Daemon) {
	t.Helper()
	dir, err := os.MkdirTemp("", "rigs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock = filepath.Join(dir, "s")
	root = filepath.Join(dir, "root")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	d, err = New(Config{Version: "test", Wire: "v1", Lock: lock, Root: root, FilesCommitEvery: every})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock, root, d
}

// plan/48 D2: a program reaches its own store and no other, whatever it
// names; the name comes off the connection.
func TestAProgramReachesOnlyItsOwnStore(t *testing.T) {
	sock, root, _ := upStoreDaemon(t)
	ctx := ctx5(t)
	graft := program(t, sock, "graft")
	shelf := program(t, sock, "shelf")

	var put verbsv1.StorePutResponse
	if err := graft.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
		Collection: "runs", Id: "r1", Document: `{"state":"queued"}`,
	}, &put); err != nil || put.GetVersion() != 1 {
		t.Fatalf("graft's own put: %v, %v", put.GetVersion(), err)
	}

	var q verbsv1.StoreQueryResponse
	if err := shelf.Call(ctx, "rig.store.query", &verbsv1.StoreQueryRequest{Collection: "runs"}, &q); err != nil {
		t.Fatal(err)
	}
	if q.GetTotal() != 0 {
		t.Fatalf("shelf's own store answered %d of graft's documents", q.GetTotal())
	}
	err := shelf.Call(ctx, "rig.store.query", &verbsv1.StoreQueryRequest{
		Program: "graft", Collection: "runs",
	}, &q)
	wantCode(t, err, rigv1.Code_CODE_DENIED, "shelf naming graft's store")
	err = shelf.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
		Program: "graft", Collection: "runs", Id: "r1", ExpectedVersion: 1, Document: `{"state":"stolen"}`,
	}, &put)
	wantCode(t, err, rigv1.Code_CODE_DENIED, "shelf writing into graft's store")

	var got verbsv1.StoreGetResponse
	if err := graft.Call(ctx, "rig.store.get", &verbsv1.StoreGetRequest{
		Program: "graft", Collection: "runs", Ids: []string{"r1"},
	}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.GetDocuments()) != 1 || got.GetDocuments()[0].GetDocument() != `{"state":"queued"}` {
		t.Fatalf("graft's document after shelf's attempts: %v", got.GetDocuments())
	}
	// The unnamed estate keeps it under its root's internal area.
	if _, err := os.Stat(filepath.Join(root, "internal", "programs", "graft.db")); err != nil {
		t.Fatalf("graft's database is not under the internal area: %v", err)
	}
}

// A terminal and an agent have no store of their own: they name the program,
// and see what the program wrote.
func TestATerminalAndAnAgentNameTheProgram(t *testing.T) {
	sock, _, d := upStoreDaemon(t)
	ctx := ctx5(t)
	graft := program(t, sock, "graft")
	if err := graft.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
		Collection: "runs", Id: "r1", Document: `{"state":"done","cost":3}`,
	}, &verbsv1.StorePutResponse{}); err != nil {
		t.Fatal(err)
	}

	term := dial(t, sock)
	var q verbsv1.StoreQueryResponse
	err := term.Call(ctx, "rig.store.query", &verbsv1.StoreQueryRequest{Collection: "runs"}, &q)
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a terminal naming no program")
	if err := term.Call(ctx, "rig.store.query", &verbsv1.StoreQueryRequest{
		Program: "graft", Collection: "runs",
		Where: []*verbsv1.StoreCondition{{Field: "cost", Op: "ge", Value: "3"}},
	}, &q); err != nil || q.GetTotal() != 1 {
		t.Fatalf("a terminal reading graft's store: %v, %v", q.GetTotal(), err)
	}

	agent := mailAgent(t, d, "store-agent")
	ans := callTool(ctx, t, agent, "store_query", map[string]any{
		"program": "graft", "collection": "runs",
		"where": []any{map[string]any{"field": "state", "op": "eq", "value": `"done"`}},
	})
	docs, _ := resultOf(t, ans)["documents"].([]any)
	if len(docs) != 1 || !strings.Contains(docs[0].(map[string]any)["document"].(string), `"cost":3`) {
		t.Fatalf("the agent's query answered %v", ans)
	}
	callTool(ctx, t, agent, "store_put", map[string]any{
		"program": "graft", "collection": "notes", "id": "n1", "document": `{"by":"agent"}`,
	})
	var got verbsv1.StoreGetResponse
	if err := graft.Call(ctx, "rig.store.get", &verbsv1.StoreGetRequest{
		Collection: "notes", Ids: []string{"n1"},
	}, &got); err != nil || len(got.GetDocuments()) != 1 {
		t.Fatalf("graft did not see the agent's write: %v, %v", got.GetDocuments(), err)
	}
	refused := refusedTool(ctx, t, agent, "store_collections", nil)
	if !strings.Contains(refused["error"], "name the program") {
		t.Fatalf("an agent naming no program was refused with %v", refused)
	}
}

// The engine's refusals keep their meaning on the wire.
func TestStoreRefusalsCarryTheirCodes(t *testing.T) {
	sock, _, _ := upStoreDaemon(t)
	ctx := ctx5(t)
	graft := program(t, sock, "graft")
	put := func(expected uint64, doc string) error {
		return graft.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
			Collection: "runs", Id: "r1", ExpectedVersion: expected, Document: doc,
		}, &verbsv1.StorePutResponse{})
	}
	if err := put(0, `{}`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, put(0, `{}`), rigv1.Code_CODE_CONFLICT, "a second create")
	wantCode(t, put(1, ``), rigv1.Code_CODE_INVALID, "an empty document")
	wantCode(t, graft.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
		Collection: "../x", Id: "r", Document: `{}`,
	}, &verbsv1.StorePutResponse{}), rigv1.Code_CODE_INVALID, "a collection that is a path")
	wantCode(t, graft.Call(ctx, "rig.store.transact", &verbsv1.StoreTransactRequest{Ops: []*verbsv1.StoreOp{
		{Collection: "runs", Id: "r1", ExpectedVersion: 1, Delete: true, Document: `{}`},
	}}, &verbsv1.StoreTransactResponse{}), rigv1.Code_CODE_INVALID, "a delete carrying a document")
	wantCode(t, graft.Call(ctx, "rig.store.query", &verbsv1.StoreQueryRequest{
		Collection: "runs", Where: []*verbsv1.StoreCondition{{Field: "a", Op: "like", Value: `"x"`}},
	}, &verbsv1.StoreQueryResponse{}), rigv1.Code_CODE_INVALID, "an unknown operator")

	// The empty put above did not become a delete.
	var got verbsv1.StoreGetResponse
	if err := graft.Call(ctx, "rig.store.get", &verbsv1.StoreGetRequest{
		Collection: "runs", Ids: []string{"r1"},
	}, &got); err != nil || len(got.GetDocuments()) != 1 {
		t.Fatalf("r1 after the refused writes: %v, %v", got.GetDocuments(), err)
	}
	var tx verbsv1.StoreTransactResponse
	if err := graft.Call(ctx, "rig.store.transact", &verbsv1.StoreTransactRequest{Ops: []*verbsv1.StoreOp{
		{Collection: "runs", Id: "r2", Document: `{"n":2}`},
		{Collection: "runs", Id: "r1", ExpectedVersion: 1, Delete: true},
	}}, &tx); err != nil || len(tx.GetVersions()) != 2 || tx.GetVersions()[0] != 1 {
		t.Fatalf("transact: %v, %v", tx.GetVersions(), err)
	}
	var cs verbsv1.StoreCollectionsResponse
	if err := graft.Call(ctx, "rig.store.collections", &verbsv1.StoreCollectionsRequest{}, &cs); err != nil {
		t.Fatal(err)
	}
	if cs.GetProgram() != "graft" || len(cs.GetCollections()) != 1 || cs.GetCollections()[0].GetDocuments() != 1 {
		t.Fatalf("collections: %v", &cs)
	}
}

// A page is cut to fit one frame, and says more, rather than failing to send.
func TestAQueryPageIsCutToFitAFrame(t *testing.T) {
	sock, _, _ := upStoreDaemon(t)
	ctx := ctx5(t)
	graft := program(t, sock, "graft")
	big := `{"pad":"` + strings.Repeat("x", 200<<10) + `"}`
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		if err := graft.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
			Collection: "blobs", Id: id, Document: big,
		}, &verbsv1.StorePutResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	seen := 0
	for pages := 0; ; pages++ {
		var q verbsv1.StoreQueryResponse
		if err := graft.Call(ctx, "rig.store.query", &verbsv1.StoreQueryRequest{
			Collection: "blobs", Offset: uint32(seen),
		}, &q); err != nil {
			t.Fatal(err)
		}
		if len(q.GetDocuments()) == 0 {
			t.Fatalf("an empty page after %d documents", seen)
		}
		seen += len(q.GetDocuments())
		if !q.GetMore() {
			if pages == 0 {
				t.Fatal("1.2 MiB of documents came back in one page")
			}
			break
		}
	}
	if seen != 6 {
		t.Fatalf("paged through %d documents, want 6", seen)
	}
	wantCode(t, graft.Call(ctx, "rig.store.get", &verbsv1.StoreGetRequest{
		Collection: "blobs", Ids: []string{"a", "b", "c", "d", "e", "f"},
	}, &verbsv1.StoreGetResponse{}), rigv1.Code_CODE_INVALID, "a get too large for one frame")
}

// A daemon given no root keeps no program stores, and says so.
func TestNoRootMeansNoStore(t *testing.T) {
	c := dial(t, up(t))
	wantCode(t, c.Call(ctx5(t), "rig.store.collections", &verbsv1.StoreCollectionsRequest{Program: "graft"},
		&verbsv1.StoreCollectionsResponse{}), rigv1.Code_CODE_UNAVAILABLE, "a store call with no root")
}

// A terminal's read of a program that never wrote is NOT_FOUND and leaves
// no file; a write creates the store.
func TestAReadNeverCreatesAStoreForATerminal(t *testing.T) {
	sock, root, _ := upStoreDaemon(t)
	ctx := ctx5(t)
	term := dial(t, sock)
	wantCode(t, term.Call(ctx, "rig.store.collections", &verbsv1.StoreCollectionsRequest{Program: "grfat"},
		&verbsv1.StoreCollectionsResponse{}), rigv1.Code_CODE_NOT_FOUND, "a read of a store never written")
	if _, err := os.Stat(filepath.Join(root, "internal", "programs", "grfat.db")); !os.IsNotExist(err) {
		t.Fatalf("the read left a database behind: %v", err)
	}
	if err := term.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
		Program: "graft", Collection: "runs", Id: "r1", Document: `{}`,
	}, &verbsv1.StorePutResponse{}); err != nil {
		t.Fatalf("a terminal's write did not create the store: %v", err)
	}
}

// Slice 5 on the wire: a program exports its own store, the export is
// committed, and an import after a delete gives the document back at its
// version, with a snapshot left behind that undoes it.
func TestAProgramExportsAndImportsItsOwnStore(t *testing.T) {
	sock, root, d := upStoreDaemon(t)
	ctx := ctx5(t)
	graft := program(t, sock, "graft")
	for _, id := range []string{"r2", "r1"} {
		if err := graft.Call(ctx, "rig.store.put", &verbsv1.StorePutRequest{
			Collection: "runs", Id: id, Document: `{"state":"done"}`,
		}, &verbsv1.StorePutResponse{}); err != nil {
			t.Fatal(err)
		}
	}

	var ex verbsv1.StoreExportResponse
	if err := graft.Call(ctx, "rig.store.export", &verbsv1.StoreExportRequest{}, &ex); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "exports", "graft", "runs.jsonl")
	body, err := os.ReadFile(file)
	if err != nil || ex.GetCommit() == "" || ex.GetDir() != filepath.Dir(file) ||
		len(ex.GetCollections()) != 1 || ex.GetCollections()[0].GetDocuments() != 2 {
		t.Fatalf("export answered %v, file %q, %v", &ex, body, err)
	}
	if !strings.HasPrefix(string(body), `{"id":"r1",`) {
		t.Fatalf("the export is not sorted by id:\n%s", body)
	}
	var again verbsv1.StoreExportResponse
	if err := graft.Call(ctx, "rig.store.export", &verbsv1.StoreExportRequest{}, &again); err != nil || again.GetCommit() != "" {
		t.Fatalf("an unchanged export committed again: %q, %v", again.GetCommit(), err)
	}

	shelf := program(t, sock, "shelf")
	err = shelf.Call(ctx, "rig.store.import", &verbsv1.StoreImportRequest{Program: "graft"}, &verbsv1.StoreImportResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "shelf importing into graft")
	err = graft.Call(ctx, "rig.store.export", &verbsv1.StoreExportRequest{Collections: []string{"nope"}}, &ex)
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "exporting a collection that does not exist")

	if err := graft.Call(ctx, "rig.store.delete", &verbsv1.StoreDeleteRequest{
		Collection: "runs", Id: "r1", ExpectedVersion: 1,
	}, &verbsv1.StoreDeleteResponse{}); err != nil {
		t.Fatal(err)
	}
	var im verbsv1.StoreImportResponse
	if err := graft.Call(ctx, "rig.store.import", &verbsv1.StoreImportRequest{}, &im); err != nil {
		t.Fatal(err)
	}
	if c := im.GetCollections(); len(c) != 1 || c[0].GetDocuments() != 2 || c[0].GetReplaced() != 1 {
		t.Fatalf("import answered %v", &im)
	}
	if !strings.HasPrefix(im.GetSnapshot(), filepath.Join(root, "internal", "snapshots", "store", "graft-")) {
		t.Fatalf("snapshot at %q", im.GetSnapshot())
	}
	if _, err := os.Stat(im.GetSnapshot()); err != nil {
		t.Fatal(err)
	}
	var got verbsv1.StoreGetResponse
	if err := graft.Call(ctx, "rig.store.get", &verbsv1.StoreGetRequest{
		Collection: "runs", Ids: []string{"r1"},
	}, &got); err != nil || len(got.GetDocuments()) != 1 || got.GetDocuments()[0].GetVersion() != 1 {
		t.Fatalf("r1 after the import: %v, %v", got.GetDocuments(), err)
	}

	// A broken export is refused whole, as invalid, and the agent door
	// carries both tools.
	if err := os.WriteFile(file, []byte("{nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = graft.Call(ctx, "rig.store.import", &verbsv1.StoreImportRequest{}, &im)
	wantCode(t, err, rigv1.Code_CODE_INVALID, "importing a broken line")
	agent := mailAgent(t, d, "export-agent")
	// The export overwrites the broken file with what was last committed,
	// so it is the same bytes and there is nothing new to commit.
	ans := callTool(ctx, t, agent, "store_export", map[string]any{"program": "graft"})
	if now, _ := os.ReadFile(file); string(now) != string(body) || resultOf(t, ans)["commit"] != "" {
		t.Fatalf("the agent's export answered %v, file %q", ans, now)
	}
	callTool(ctx, t, agent, "store_import", map[string]any{"program": "graft", "collections": []any{"runs"}})
}
