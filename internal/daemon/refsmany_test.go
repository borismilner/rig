package daemon

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// refsFixture writes n requirements, each cited by two work items, and
// returns the requirement ids.
func refsFixture(ctx context.Context, tb testing.TB, c *client.Client, n int) []string {
	tb.Helper()
	put := func(id, kind string) {
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: id, Kind: kind, Project: "rig", Fields: map[string]string{"title": id},
		}, &rigv1.RecordPutResponse{}); err != nil {
			tb.Fatalf("rig.record.put(%s): %v", id, err)
		}
	}
	link := func(src, dst string) {
		if err := c.Call(ctx, "rig.record.link", &rigv1.RecordLinkRequest{
			Src: src, Type: "cites", Dst: dst,
		}, &rigv1.RecordLinkResponse{}); err != nil {
			tb.Fatalf("rig.record.link(%s, %s): %v", src, dst, err)
		}
	}
	put("rig", "project")
	var ids []string
	for i := range n {
		r := "R" + strconv.Itoa(i)
		put(r, "requirement")
		for _, w := range []string{"W" + strconv.Itoa(i) + "a", "W" + strconv.Itoa(i) + "b"} {
			put(w, "work-item")
			link(w, r)
		}
		ids = append(ids, r)
	}
	return ids
}

// ⛔ EACH SUBJECT IN A BATCH IS ANSWERED EXACTLY AS IF ASKED ALONE. The
// batch is a transport saving and must not be a second semantics: the test
// compares every result to the one-id answer for the same id, message for
// message, in the order asked.
func TestRecordRefsForManyIDsIsTheOneIDAnswerForEach(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)
	ids := refsFixture(ctx, t, c, 3)
	ids = []string{ids[2], ids[0], ids[1]} // order asked is order answered

	var many rigv1.RecordRefsResponse
	if err := c.Call(ctx, "rig.record.refs", &rigv1.RecordRefsRequest{
		Ids: ids, Depth: 2,
	}, &many); err != nil {
		t.Fatalf("rig.record.refs with ids: %v", err)
	}
	if len(many.GetResults()) != len(ids) {
		t.Fatalf("asked about %d ids and got %d results", len(ids), len(many.GetResults()))
	}
	for i, id := range ids {
		var one rigv1.RecordRefsResponse
		if err := c.Call(ctx, "rig.record.refs", &rigv1.RecordRefsRequest{
			Id: id, Depth: 2,
		}, &one); err != nil {
			t.Fatalf("rig.record.refs(%s): %v", id, err)
		}
		if !proto.Equal(many.GetResults()[i], &one) {
			t.Errorf("result %d is not the one-id answer for %s.\nbatch %v\nalone %v",
				i, id, many.GetResults()[i], &one)
		}
		if len(one.GetRefs()) != 2 {
			t.Errorf("%s has %d refs, want the 2 the fixture wrote", id, len(one.GetRefs()))
		}
	}
}

// Every way to ask wrongly is refused by name, and a batch never answers
// with a hole in it.
func TestRecordRefsForManyIDsRefusesRatherThanAnswersPartly(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)
	ids := refsFixture(ctx, t, c, 1)

	tooMany := make([]string, maxRefsIDs+1)
	for i := range tooMany {
		tooMany[i] = ids[0]
	}
	for _, tc := range []struct {
		name string
		req  *rigv1.RecordRefsRequest
		code rigv1.Code
	}{
		{"id and ids", &rigv1.RecordRefsRequest{Id: ids[0], Ids: ids}, rigv1.Code_CODE_INVALID},
		{"an unknown id", &rigv1.RecordRefsRequest{Ids: []string{ids[0], "nope"}}, rigv1.Code_CODE_NOT_FOUND},
		{"above the cap", &rigv1.RecordRefsRequest{Ids: tooMany}, rigv1.Code_CODE_INVALID},
		{"depth above the cap", &rigv1.RecordRefsRequest{Ids: ids, Depth: 9}, rigv1.Code_CODE_INVALID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := c.Call(ctx, "rig.record.refs", tc.req, &rigv1.RecordRefsResponse{})
			wantCode(t, err, tc.code, tc.name)
		})
	}
}

// BenchmarkRecordRefs is plan/48's case measured over the socket: what
// points at each of 220 records, as 220 one-id calls and as one batch.
func BenchmarkRecordRefs(b *testing.B) {
	sock := upRecordDaemon(b)
	c, err := client.Dial(sock)
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := c.Call(ctx, "rig.announce", &rigv1.AnnounceRequest{
		Seat: "bench", Purpose: "measuring refs", Activity: "benchmarking",
	}, &rigv1.AnnounceResponse{}); err != nil {
		b.Fatal(err)
	}
	ids := refsFixture(ctx, b, c, 220)

	b.Run("one-id-x220", func(b *testing.B) {
		for b.Loop() {
			for _, id := range ids {
				if err := c.Call(ctx, "rig.record.refs", &rigv1.RecordRefsRequest{Id: id},
					&rigv1.RecordRefsResponse{}); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("ids-x220", func(b *testing.B) {
		for b.Loop() {
			if err := c.Call(ctx, "rig.record.refs", &rigv1.RecordRefsRequest{Ids: ids},
				&rigv1.RecordRefsResponse{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// ⛔ A BATCH WHOSE ANSWER IS OVER THE PAGE BUDGET IS REFUSED BY NAME, even
// though it would fit one frame. Without this test the budget check could be
// removed and every batch under 1 MiB would still pass, because the frame cap
// only catches what the budget was meant to stop earlier.
func TestRecordRefsForManyIDsRefusesAnAnswerOverTheBudget(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	put := func(id, kind, title string) {
		t.Helper()
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: id, Kind: kind, Project: "rig", Fields: map[string]string{"title": title},
		}, &rigv1.RecordPutResponse{}); err != nil {
			t.Fatalf("rig.record.put(%s): %v", id, err)
		}
	}
	put("rig", "project", "rig")
	put("R", "requirement", "R")
	for _, w := range []string{"Wa", "Wb"} {
		put(w, "work-item", strings.Repeat(w, 1<<10))
		if err := c.Call(ctx, "rig.record.link", &rigv1.RecordLinkRequest{
			Src: w, Type: "cites", Dst: "R",
		}, &rigv1.RecordLinkResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	// About 4 KiB per result, 100 results: over the 256 KiB budget and well
	// under the 1 MiB frame, so only the budget can refuse it.
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = "R"
	}
	err := c.Call(ctx, "rig.record.refs", &rigv1.RecordRefsRequest{Ids: ids}, &rigv1.RecordRefsResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a batch over the budget")
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("the refusal does not name the budget: %v", err)
	}
}
