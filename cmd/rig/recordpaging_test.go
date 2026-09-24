package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	wirepkg "github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// B116's CLI half: `record.query` answers a page at a time, and `rig record
// query` prints what it always printed.
//
// ⛔ THIS NEEDS A FAKE THAT ANSWERS DIFFERENTLY EACH TIME, which is why it does
// not use wirefake_test.go's fakeDaemon: that one replies with a single fixed
// message, and a paging loop against a fixed reply either never terminates or
// terminates for the wrong reason. The fake here keys its answer on the
// request's own cursor, so the loop is driven by what it sends.

// pagingFake serves rig.record.query out of a script of pages.
type pagingFake struct {
	socket string

	// pages is the script: each entry is the ids of one page, and the cursor
	// handed out is the index of the next one.
	pages [][]string

	// index maps a cursor back to the page it names. It is built by
	// startPagingFake, so the fake never parses its own cursor.
	index map[string]int

	// stuck makes every answer carry the SAME cursor, which is the malformed
	// daemon a caller must not loop on forever.
	stuck bool

	mu    sync.Mutex
	after []string // every `after` the client sent, in order
}

func startPagingFake(t *testing.T, f *pagingFake) {
	t.Helper()
	// Short, because sun_path is capped near 108 bytes and a Go test's TMPDIR
	// can approach it on its own.
	dir, err := os.MkdirTemp("", "rigpage")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	f.socket = filepath.Join(dir, "s")

	f.index = map[string]int{"": 0}
	for i := range f.pages {
		f.index[fmt.Sprintf("page-%d", i)] = i
	}

	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(nc)
		}
	}()
}

func (f *pagingFake) serve(nc net.Conn) {
	c := wirepkg.NewConn(nc)
	defer c.Close()
	for {
		frame, err := c.ReadFrame()
		if err != nil {
			return
		}
		var req rigv1.RecordQueryRequest
		if err := proto.Unmarshal(frame.GetPayload(), &req); err != nil {
			return
		}

		f.mu.Lock()
		f.after = append(f.after, req.GetAfter())
		f.mu.Unlock()

		// The cursor this fake issues is "page-<n>", opaque to the client
		// exactly as the daemon's is. An `after` it did not issue answers
		// nothing rather than guessing a position.
		page, ok := f.index[req.GetAfter()]
		if !ok {
			page = len(f.pages)
		}
		resp := &rigv1.RecordQueryResponse{}
		if page < len(f.pages) {
			for _, id := range f.pages[page] {
				resp.Records = append(resp.Records, &rigv1.Record{
					Id: id, Version: 1, Kind: "note", Project: "rig",
					Body: "body of " + id,
					Prov: &rigv1.Provenance{Session: "s", Seat: "fake"},
				})
			}
		}
		switch {
		case f.stuck:
			resp.Next = "page-1"
		case page+1 < len(f.pages):
			resp.Next = fmt.Sprintf("page-%d", page+1)
		}

		body, err := proto.Marshal(resp)
		if err != nil {
			return
		}
		if err := c.WriteFrame(&rigv1.Frame{
			StreamId: frame.GetStreamId(),
			Kind:     rigv1.FrameKind_FRAME_KIND_RESPONSE,
			Payload:  body,
		}); err != nil {
			return
		}
	}
}

func (f *pagingFake) cursorsSent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.after...)
}

func pagingClient(t *testing.T, f *pagingFake) (*client.Client, context.Context) {
	t.Helper()
	c, err := client.Dial(f.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c, ctx
}

// TestTheQueryVerbPrintsEveryRecordOnceAcrossThreePages is plan/50's "the CLI
// against a three-page daemon prints every record once".
func TestTheQueryVerbPrintsEveryRecordOnceAcrossThreePages(t *testing.T) {
	f := &pagingFake{pages: [][]string{
		{"A", "B", "C"},
		{"D", "E"},
		{"F"},
	}}
	startPagingFake(t, f)
	c, ctx := pagingClient(t, f)

	recs, err := wireRecord{c}.Query(ctx, QueryArgs{Project: "rig"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	want := []string{"A", "B", "C", "D", "E", "F"}
	if len(recs) != len(want) {
		t.Fatalf("three pages of %v produced %d records, want %d",
			f.pages, len(recs), len(want))
	}
	for i, r := range recs {
		if r.ID != want[i] {
			t.Fatalf("at position %d the caller saw %s, want %s", i, r.ID, want[i])
		}
	}

	// ⛔ THE CURSOR HAS TO TRAVEL, and a renderer cannot see whether it did.
	// The first request carries none and each later one carries what the
	// previous answer handed back; without this, a client that sent no cursor
	// at all would still pass the assertion above against a fake that ignores
	// it.
	sent := f.cursorsSent()
	wantSent := []string{"", "page-1", "page-2"}
	if len(sent) != len(wantSent) {
		t.Fatalf("the fake was sent %d requests (%q), want %d", len(sent), sent, len(wantSent))
	}
	for i := range wantSent {
		if sent[i] != wantSent[i] {
			t.Errorf("request %d carried after=%q, want %q", i, sent[i], wantSent[i])
		}
	}

	// And the listing a reader sees carries all six. The renderer is the one
	// that was always there and is unchanged by B116, so the assertion that
	// matters is the one above - each id exactly once, in order - and this is
	// the end-to-end control on it.
	out := queryText(QueryArgs{Project: "rig"}, recs, 100)
	for _, id := range want {
		if !strings.Contains(out, id) {
			t.Errorf("the listing does not mention %s at all:\n%s", id, out)
		}
	}
}

// TestAQueryStopsWhenTheCursorDoesNotMove: a daemon that answers the same
// cursor forever must produce an error, not a hang. A client that hangs on a
// malformed answer is worse than one that says what it got.
func TestAQueryStopsWhenTheCursorDoesNotMove(t *testing.T) {
	f := &pagingFake{pages: [][]string{{"A"}, {"B"}}, stuck: true}
	startPagingFake(t, f)
	c, ctx := pagingClient(t, f)

	done := make(chan error, 1)
	go func() {
		_, err := wireRecord{c}.Query(ctx, QueryArgs{Project: "rig"})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a daemon that never advances its cursor was accepted")
		}
		if !strings.Contains(err.Error(), "same page cursor twice") {
			t.Errorf("the refusal does not say what went wrong: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the query did not return: the caller is looping on a cursor " +
			"that does not move")
	}
}

// TestAQueryAgainstADaemonWithoutPagingIsOneCall: `next` unset is the last
// page, so a rig built before B116 answers exactly once and the loop costs
// nothing.
func TestAQueryAgainstADaemonWithoutPagingIsOneCall(t *testing.T) {
	f := &pagingFake{pages: [][]string{{"A", "B"}}}
	startPagingFake(t, f)
	c, ctx := pagingClient(t, f)

	recs, err := wireRecord{c}.Query(ctx, QueryArgs{Project: "rig"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("a one-page answer produced %d records, want 2", len(recs))
	}
	if sent := f.cursorsSent(); len(sent) != 1 || sent[0] != "" {
		t.Errorf("the client made %d calls (%q), want one with no cursor",
			len(sent), sent)
	}
}
