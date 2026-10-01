package daemon

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/48 decision 6 over the real wire: a query naming fields answers the
// four fixed ones and the named ones only. Red control: return body unasked.
// The unprojected answer staying byte-identical is
// TestAQueryWithNoLimitAndNoCursorIsWhatItWas.
func TestAProjectedQueryCarriesOnlyTheNamedFields(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "projection")
	ctx := pagingCtx(t)

	var put verbsv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &verbsv1.RecordPutRequest{
		Id: "A", Kind: "work", Project: "rig", Body: strings.Repeat("b", 4000),
		Fields: map[string]string{"title": "first", "status": "open", "owner": "lead"},
	}, &put); err != nil {
		t.Fatal(err)
	}

	query := func(fields ...string) *verbsv1.RecordQueryResponse {
		t.Helper()
		var resp verbsv1.RecordQueryResponse
		if err := c.Call(ctx, "rig.record.query", &verbsv1.RecordQueryRequest{
			Project: "rig", Kind: "work", Fields: fields,
		}, &resp); err != nil {
			t.Fatalf("query %v: %v", fields, err)
		}
		if len(resp.GetRecords()) != 1 {
			t.Fatalf("query %v: %d records", fields, len(resp.GetRecords()))
		}
		return &resp
	}

	full := query()
	thin := query("title", "status")
	r := thin.GetRecords()[0]
	if r.GetId() != "A" || r.GetVersion() != 1 || r.GetKind() != "work" || r.GetProject() != "rig" {
		t.Errorf("a fixed field is missing: %v", r)
	}
	if got := slices.Sorted(maps.Keys(r.GetFields())); !slices.Equal(got, []string{"status", "title"}) {
		t.Errorf("fields %v, want status and title", got)
	}
	if r.GetBody() != "" || r.GetProv() != nil {
		t.Error("body or prov travelled unasked")
	}
	if a, b := proto.Size(full), proto.Size(thin); b*10 > a {
		t.Errorf("projected %d bytes against %d: not an order of magnitude", b, a)
	}

	if got := query("body").GetRecords()[0]; got.GetBody() == "" || len(got.GetFields()) != 0 {
		t.Error("naming body did not bring body, or brought fields")
	}

	var resp verbsv1.RecordQueryResponse
	if err := c.Call(ctx, "rig.record.query", &verbsv1.RecordQueryRequest{
		Project: "rig", Fields: []string{"title", ""},
	}, &resp); err == nil {
		t.Error("an empty field name was accepted")
	}
}
