package daemon

import (
	"testing"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// ⛔ A RETRACTED RECORD LEAVES EVERY LIST THE WIRE ANSWERS, AND record.get
// STILL EXPLAINS IT. plan/39's retract row: "it stops appearing in a brief or
// a query, and record.get still explains what it was and that it was
// retracted".
//
// This is the three-list assertion internal/record/control_test.go lost at
// plan/50 move 8, put back where the lists now come from. It used to read
// Store.Brief and check next-up, open and closed. The brief is docket's now
// and docket assembles those three lists from rig.record.query over this
// socket, so the three lists rig answers are the three query shapes a
// derivation reads: a project's work items by kind, the same narrowed by a
// field, and the paged form docket's stub loops on - and record.refs, which
// docket reads once per head. A retracted record that survived any one of
// them would reach a brief again from outside rig, where no rig test could
// see it.
func TestARetractedRecordLeavesEveryListOnTheWireAndStillExplainsItself(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "backend-record")
	ctx := recordCtx(t)

	for _, r := range []struct{ id, kind string }{
		{"rig", "project"},
		{"B90", "work-item"},
		{"B91", "work-item"},
		{"B92", "work-item"},
	} {
		if err := c.Call(ctx, "rig.record.put", &verbsv1.RecordPutRequest{
			Id: r.id, Kind: r.kind, Project: "rig", Body: r.id,
			Fields: map[string]string{"title": r.id, "status": "active"},
		}, &verbsv1.RecordPutResponse{}); err != nil {
			t.Fatalf("rig.record.put(%s): %v", r.id, err)
		}
	}

	const gone = "B91"
	if err := c.Call(ctx, "rig.record.retract", &verbsv1.RecordRetractRequest{
		Id: gone, Reason: "written by mistake",
	}, &verbsv1.RecordRetractResponse{}); err != nil {
		t.Fatalf("rig.record.retract: %v", err)
	}

	query := func(req *verbsv1.RecordQueryRequest) *verbsv1.RecordQueryResponse {
		t.Helper()
		var resp verbsv1.RecordQueryResponse
		if err := c.Call(ctx, "rig.record.query", req, &resp); err != nil {
			t.Fatalf("rig.record.query(%v): %v", req, err)
		}
		return &resp
	}
	ids := func(rs []*verbsv1.Record) map[string]bool {
		out := map[string]bool{}
		for _, r := range rs {
			out[r.GetId()] = true
		}
		return out
	}
	want := map[string]bool{"B90": true, "B92": true}
	check := func(list string, got map[string]bool) {
		t.Helper()
		if got[gone] {
			t.Errorf("%s: %s is retracted and the list still carries it as work", list, gone)
		}
		for id := range want {
			if !got[id] {
				t.Errorf("%s: live record %s is missing, so the absence of %s "+
					"proves nothing", list, id, gone)
			}
		}
	}

	// 1. The project's work items, which is what open and next-up are cut from.
	check("by kind", ids(query(&verbsv1.RecordQueryRequest{
		Project: "rig", Kind: "work-item",
	}).GetRecords()))

	// 2. The same narrowed by a field, which is how a derivation files by
	// status. Closed was never a place a retracted record could hide: a
	// retraction is not a closing word, it is an absence.
	check("by field", ids(query(&verbsv1.RecordQueryRequest{
		Project: "rig", Kind: "work-item", Field: "status", Value: "active",
	}).GetRecords()))

	// 3. The paged form, one record per page, following `next` to the end.
	// A page boundary is where a filter applied per page rather than per
	// answer would let a withdrawn record through.
	paged := map[string]bool{}
	after := ""
	for range 10 {
		resp := query(&verbsv1.RecordQueryRequest{
			Project: "rig", Kind: "work-item", Limit: 1, After: after,
		})
		for id := range ids(resp.GetRecords()) {
			paged[id] = true
		}
		if after = resp.GetNext(); after == "" {
			break
		}
	}
	if after != "" {
		t.Fatal("the paged query never ended, so the paged list was never read whole")
	}
	check("paged", paged)

	// 4. And record.refs, which docket reads once per head: a retracted record
	// pointing at a live one is not a ref to it.
	for _, src := range []string{"B90", gone} {
		if err := c.Call(ctx, "rig.record.link", &verbsv1.RecordLinkRequest{
			Src: src, Type: "cites", Dst: "B92",
		}, &verbsv1.RecordLinkResponse{}); err != nil {
			t.Fatalf("rig.record.link(%s): %v", src, err)
		}
	}
	var refs verbsv1.RecordRefsResponse
	if err := c.Call(ctx, "rig.record.refs", &verbsv1.RecordRefsRequest{Id: "B92"}, &refs); err != nil {
		t.Fatalf("rig.record.refs: %v", err)
	}
	if len(refs.GetRefs()) != 1 || refs.GetRefs()[0].GetSrc() != "B90" || refs.GetTruncated() {
		t.Errorf("refs: %v truncated=%v, want only B90 and complete - %s is "+
			"retracted and still reaches a brief this way", refs.GetRefs(), refs.GetTruncated(), gone)
	}

	// ⛔ AND record.get MUST STILL ANSWER, WITH THE FACT AND ITS REASON. A
	// NotFound here would make retract indistinguishable from delete.
	var got verbsv1.RecordGetResponse
	if err := c.Call(ctx, "rig.record.get", &verbsv1.RecordGetRequest{Id: gone}, &got); err != nil {
		t.Fatalf("rig.record.get on a retracted record: %v - it must still "+
			"explain what the record was and that it was retracted", err)
	}
	ret := got.GetRecord().GetRetraction()
	if ret == nil {
		t.Fatal("record.get answered about a retracted record and said nothing " +
			"about the retraction, so a reader cannot tell it from live work")
	}
	if ret.GetReason() != "written by mistake" {
		t.Errorf("the retraction reason is %q, want the one given", ret.GetReason())
	}
	if ret.GetProv().GetSeat() != "backend-record" {
		t.Errorf("the retraction carries seat %q, want the retracting seat", ret.GetProv().GetSeat())
	}
}
