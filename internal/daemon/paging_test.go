package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/wire"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// B116 over the real wire: `record.query --kind requirement` over the whole
// 50-section plan answered 1,123,924 bytes, past wire.MaxFrameSize, so the
// frame was refused on the way out and the query died where no caller could
// see why.
//
// ⛔ THE STRONGEST ASSERTION IN THIS FILE IS THAT THE CALL SUCCEEDS AT ALL.
// A frame over MaxFrameSize is refused by the writer, so an answer bigger than
// a frame cannot arrive by accident: the fixture below holds 1.5 MiB of
// bodies, and before this change the query it makes did not come back.

// pagingCtx is longer than recordCtx because the fixture writes 1.5 MiB in 64
// separate transactions before anything is asserted.
func pagingCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// bigFixture writes n records of bodyLen bytes each and returns their ids in
// the store's order, which for one project and one kind is the id order.
func bigFixture(ctx context.Context, t *testing.T, c *client.Client, n, bodyLen int) []string {
	t.Helper()
	body := strings.Repeat("x", bodyLen)
	ids := make([]string, 0, n)
	for i := range n {
		id := fmt.Sprintf("P%03d", i)
		var put rigv1.RecordPutResponse
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: id, Kind: "requirement", Project: "rig", Body: body,
		}, &put); err != nil {
			t.Fatalf("writing %s: %v", id, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// TestAnAnswerLargerThanAFrameArrivesInPages is plan/50's "pages are exact":
// N pages, every frame under MaxFrameSize, union complete, no duplicate, no
// gap.
func TestAnAnswerLargerThanAFrameArrivesInPages(t *testing.T) {
	const (
		records = 64
		bodyLen = 24 << 10 // 1.5 MiB of bodies, so a single frame cannot hold it
	)
	sock := upRecordDaemon(t)
	c := seated(t, sock, "paging")
	ctx := pagingCtx(t)

	want := bigFixture(ctx, t, c, records, bodyLen)
	if records*bodyLen <= wire.MaxFrameSize {
		t.Fatalf("the fixture is %d bytes of bodies, which fits in a frame: "+
			"it cannot demonstrate paging", records*bodyLen)
	}

	var (
		got   []string
		next  string
		pages int
	)
	for {
		var resp rigv1.RecordQueryResponse
		if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
			Project: "rig", Kind: "requirement", After: next,
		}, &resp); err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		pages++
		if pages > records+1 {
			t.Fatalf("the walk did not end after %d pages: the cursor is "+
				"not advancing", pages)
		}

		size := proto.Size(&resp)
		if size > wire.MaxFrameSize {
			t.Fatalf("page %d encodes to %d bytes, over MaxFrameSize (%d)",
				pages, size, wire.MaxFrameSize)
		}

		// ⛔ THE BUDGET MAY BE EXCEEDED BY AT MOST ONE RECORD, and that is the
		// contract rather than slack in the test: a page always carries at
		// least one record, so a record larger than the budget rides alone.
		if over := size - RecordPageBudget; over > bodyLen+1024 {
			t.Errorf("page %d encodes to %d bytes, %d over the budget of %d - "+
				"more than the one record a page is allowed to exceed it by",
				pages, size, over, RecordPageBudget)
		}
		if len(resp.GetRecords()) == 0 {
			t.Fatalf("page %d carried no records at all, which is a cursor "+
				"that cannot advance", pages)
		}
		for _, r := range resp.GetRecords() {
			got = append(got, r.GetId())
			if r.GetBody() == "" {
				t.Fatalf("%s came back with no body: a page is not a "+
					"projection", r.GetId())
			}
		}
		next = resp.GetNext()
		if next == "" {
			break
		}
	}

	t.Logf("%d bytes of bodies arrived in %d pages under a %d byte budget",
		records*bodyLen, pages, RecordPageBudget)
	if pages < 2 {
		t.Fatalf("%d bytes of bodies came back in %d page(s), so nothing here "+
			"exercised a page boundary", records*bodyLen, pages)
	}
	if len(got) != len(want) {
		t.Fatalf("the walk saw %d records over %d pages, want %d: a page was "+
			"dropped or repeated", len(got), pages, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at position %d the walk saw %s, want %s", i, got[i], want[i])
		}
	}
	seen := make(map[string]int, len(got))
	for _, id := range got {
		seen[id]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("%s came back %d times", id, n)
		}
	}
}

// TestAQueryWithNoLimitAndNoCursorIsWhatItWas is plan/50's "additive" row.
//
// ⛔ THE FIXTURE CARRIES NO FIELDS, DELIBERATELY. Record.fields is a protobuf
// map, and a map has no defined encoding order, so a BYTE comparison over a
// record with fields would flake rather than assert. Without one, the answer's
// encoding is stable and the comparison can be exact - which is what "byte
// identical to before" has to mean to be checkable at all.
func TestAQueryWithNoLimitAndNoCursorIsWhatItWas(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "paging")
	ctx := pagingCtx(t)

	old := &rigv1.RecordQueryResponse{}
	for _, id := range []string{"A", "B", "C"} {
		var put rigv1.RecordPutResponse
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: id, Kind: "note", Project: "rig", Body: "body of " + id,
		}, &put); err != nil {
			t.Fatalf("writing %s: %v", id, err)
		}
		// What the old handler would have built: every matching record, in
		// the store's order, and nothing else set.
		old.Records = append(old.Records, put.GetRecord())
	}

	var resp rigv1.RecordQueryResponse
	if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
		Project: "rig", Kind: "note",
	}, &resp); err != nil {
		t.Fatalf("rig.record.query: %v", err)
	}
	if resp.GetNext() != "" {
		t.Errorf("an answer that fits in one page carried a cursor %q, so "+
			"every client would ask for a second page", resp.GetNext())
	}

	want, err := proto.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	got, err := proto.Marshal(&resp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the answer is %d bytes and the pre-B116 shape is %d: a "+
			"request carrying neither limit nor after must be byte-identical "+
			"to what it was", len(got), len(want))
	}
}

// TestAGarbageCursorIsRefused is DECISION 6's second mutation on `after`: a
// WRONG non-empty value.
//
// ⛔ REFUSED AND NOT IGNORED. A cursor read as "start at the beginning" would
// answer a page that looks complete; one read as an arbitrary position would
// answer a subset that looks complete. Both are wrong in the reassuring
// direction, which is why the refusal is the assertion.
func TestAGarbageCursorIsRefused(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "paging")
	ctx := pagingCtx(t)

	valid, err := encodeRecordCursor(record.Cursor{
		Project: "rig", Kind: "note", ID: "A",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, after := range []string{
		"garbage",
		"!!!not base64!!!",
		base64.RawURLEncoding.EncodeToString([]byte("{}")),
		base64.RawURLEncoding.EncodeToString([]byte(`{"p":"rig","k":"note"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"p":"rig","k":"note","i":""}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"p":"rig","k":"note","i":"A","x":1}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"p":"rig","k":"note","i":"A"} trailing`)),
		valid + strings.Repeat("A", maxRecordCursor),
	} {
		var resp rigv1.RecordQueryResponse
		err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
			Project: "rig", After: after,
		}, &resp)
		if err == nil {
			t.Errorf("after=%.40q was ACCEPTED and answered %d records",
				after, len(resp.GetRecords()))
			continue
		}
		var refusal *client.CallError
		if !errors.As(err, &refusal) {
			t.Errorf("after=%.40q was refused unstructured: %T %v", after, err, err)
			continue
		}
		if refusal.Code() != rigv1.Code_CODE_INVALID {
			t.Errorf("after=%.40q was refused %v, want CODE_INVALID: a caller "+
				"cannot tell a bad cursor from rig being unavailable",
				after, refusal.Code())
		}
	}
}

// TestAnEmptyCursorIsTheFirstPage is DECISION 6's FIRST mutation on `after`.
//
// protojson omits the empty string, so an empty `after` and an absent one are
// the same bytes and this can only assert that the empty value is SERVED as
// the first page rather than refused with the garbage above. The control is
// the previous test, where a wrong value does not answer.
func TestAnEmptyCursorIsTheFirstPage(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "paging")
	ctx := pagingCtx(t)

	for _, id := range []string{"A", "B"} {
		var put rigv1.RecordPutResponse
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: id, Kind: "note", Project: "rig", Body: "b",
		}, &put); err != nil {
			t.Fatalf("writing %s: %v", id, err)
		}
	}

	var absent, empty rigv1.RecordQueryResponse
	if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
		Project: "rig",
	}, &absent); err != nil {
		t.Fatalf("with no cursor: %v", err)
	}
	if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
		Project: "rig", After: "",
	}, &empty); err != nil {
		t.Fatalf("with an empty cursor: %v", err)
	}
	if !proto.Equal(&absent, &empty) {
		t.Error("an empty `after` answered something other than the first page")
	}
	if len(absent.GetRecords()) != 2 {
		t.Errorf("the first page carried %d records, want 2",
			len(absent.GetRecords()))
	}
}

// TestALimitCapsThePageCount: `limit` bounds the COUNT, the budget still
// bounds the bytes, and the walk still reconstructs the whole answer.
func TestALimitCapsThePageCount(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "paging")
	ctx := pagingCtx(t)

	want := bigFixture(ctx, t, c, 10, 64)

	var (
		got  []string
		next string
	)
	for i := 0; ; i++ {
		if i > len(want)+1 {
			t.Fatalf("the walk did not end after %d pages of 3", i)
		}
		var resp rigv1.RecordQueryResponse
		if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
			Project: "rig", Kind: "requirement", Limit: 3, After: next,
		}, &resp); err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		if n := len(resp.GetRecords()); n > 3 {
			t.Fatalf("page %d carried %d records against a limit of 3", i, n)
		}
		for _, r := range resp.GetRecords() {
			got = append(got, r.GetId())
		}
		next = resp.GetNext()
		if next == "" {
			break
		}
	}
	if len(got) != len(want) {
		t.Fatalf("walking in pages of 3 saw %d records, want %d", len(got), len(want))
	}

	// A limit larger than the answer is one page and no cursor: the cap is a
	// maximum, not a page size.
	var whole rigv1.RecordQueryResponse
	if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
		Project: "rig", Kind: "requirement", Limit: 1000,
	}, &whole); err != nil {
		t.Fatalf("with a limit over the answer: %v", err)
	}
	if len(whole.GetRecords()) != len(want) || whole.GetNext() != "" {
		t.Errorf("a limit of 1000 over %d records answered %d with next=%q",
			len(want), len(whole.GetRecords()), whole.GetNext())
	}
}

// TestACursorRoundTripsAndRefusesWhatItDidNotWrite covers the codec directly,
// because the wire tests above can only reach it through a query and cannot
// show that what `next` carries is what `after` reads back.
func TestACursorRoundTripsAndRefusesWhatItDidNotWrite(t *testing.T) {
	for _, c := range []record.Cursor{
		{Project: "rig", Kind: "requirement", ID: "B116"},
		{Project: "a project with spaces", Kind: "k/i:n d", ID: "0192-7f\"x"},
	} {
		s, err := encodeRecordCursor(c)
		if err != nil {
			t.Fatalf("encoding %+v: %v", c, err)
		}
		back, err := decodeRecordCursor(s)
		if err != nil {
			t.Fatalf("decoding %q: %v", s, err)
		}
		if back != c {
			t.Errorf("the cursor round-tripped to %+v, want %+v", back, c)
		}
	}

	// The empty string is the first page and is the ONLY value a caller is
	// expected to invent.
	if back, err := decodeRecordCursor(""); err != nil || back != (record.Cursor{}) {
		t.Errorf("an empty cursor decoded to %+v, %v - want the zero cursor "+
			"and no error", back, err)
	}
}
