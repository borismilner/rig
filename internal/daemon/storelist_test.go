package daemon

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

func storeList(t *testing.T, sock string) map[string]*verbsv1.StoreNamespace {
	t.Helper()
	var resp verbsv1.StoreListResponse
	if err := dial(t, sock).Call(ctx5(t), "rig.store.list", &verbsv1.StoreListRequest{}, &resp); err != nil {
		t.Fatalf("rig.store.list: %v", err)
	}
	out := map[string]*verbsv1.StoreNamespace{}
	for _, ns := range resp.GetNamespaces() {
		out[ns.GetName()] = ns
	}
	return out
}

// plan/48 decision 7 on a named estate: record and coord, their files and the
// versions the files carry. Red control: hardcode the list.
func TestStoreListNamesRecordAndCoordOnANamedEstate(t *testing.T) {
	sock, _ := upRecordDaemonWith(t, func(d *Daemon) {
		st, err := coord.Open("recordwire")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		d.leases = st
	})
	got := storeList(t, sock)
	rec, co := got["record"], got["coord"]
	if rec == nil || co == nil {
		t.Fatalf("listed %v, want record and coord", got)
	}
	if !strings.HasSuffix(rec.GetPath(), "/recordwire/record.db") || rec.GetSchemaVersion() == 0 ||
		rec.GetBytes() == 0 || rec.GetEphemeral() {
		t.Errorf("record: %v", rec)
	}
	if !strings.HasSuffix(co.GetPath(), "/recordwire/coord.db") || co.GetSchemaVersion() != coord.SchemaVersion {
		t.Errorf("coord: %v", co)
	}
	// ⛔ protojson drops a false: the false case must still read as false.
	b, _ := protojson.Marshal(rec)
	if strings.Contains(string(b), "ephemeral") {
		t.Errorf("a durable store's JSON carries ephemeral: %s", b)
	}
}

// An unnamed estate's program stores are ephemeral and say so; the true case
// travels. A registered program is refused the estate's list.
func TestStoreListMarksEphemeralAndRefusesAProgram(t *testing.T) {
	sock, _, _ := upStoreDaemon(t)
	graft := program(t, sock, "graft")
	var put verbsv1.StorePutResponse
	if err := graft.Call(ctx5(t), "rig.store.put", &verbsv1.StorePutRequest{
		Collection: "runs", Id: "a", Document: `{"n":1}`,
	}, &put); err != nil {
		t.Fatal(err)
	}
	ns := storeList(t, sock)["programs/graft"]
	if ns == nil || ns.GetSchemaVersion() != 1 || !ns.GetEphemeral() {
		t.Fatalf("programs/graft: %v", ns)
	}
	b, _ := protojson.Marshal(ns)
	if !strings.Contains(string(b), `"ephemeral":true`) {
		t.Errorf("the true case did not travel: %s", b)
	}
	var resp verbsv1.StoreListResponse
	if err := graft.Call(ctx5(t), "rig.store.list", &verbsv1.StoreListRequest{}, &resp); err == nil {
		t.Error("a registered program read the estate's store list")
	}
}
