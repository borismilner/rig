package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// rig store list shows durable and ephemeral in words, and its JSON carries
// ephemeral even when false, so an absent key is never read as durable.
func TestStoreListRendersBothKinds(t *testing.T) {
	resp := &verbsv1.StoreListResponse{Namespaces: []*verbsv1.StoreNamespace{
		{Name: "record", Path: "/s/record.db", Bytes: 10, SchemaVersion: 3},
		{Name: "programs/graft", Path: "/r/graft.db", Bytes: 5, SchemaVersion: 1, Ephemeral: true},
	}}
	var out bytes.Buffer
	printStoreList(&out, resp)
	text := out.String()
	for _, want := range []string{"record", "durable", "programs/graft", "ephemeral", "/r/graft.db"} {
		if !strings.Contains(text, want) {
			t.Errorf("the table lacks %q:\n%s", want, text)
		}
	}
	b, err := json.Marshal(storeJSON(resp))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"ephemeral":false`) || !strings.Contains(string(b), `"ephemeral":true`) {
		t.Errorf("the JSON does not carry both cases: %s", b)
	}
}
