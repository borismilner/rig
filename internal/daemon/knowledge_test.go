package daemon

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 40 over the socket: a lesson written once under a seat, found by
// search from another connection without its body, and fetched whole. The
// consult cost is section 40's acceptance NUMBER, so the test measures it:
// the bytes one search answer costs against the lesson it saves reading.
func TestALessonIsWrittenOnceAndFoundCheaply(t *testing.T) {
	sock := upRecordDaemon(t)
	w := seated(t, sock, "backend-record")
	ctx := recordCtx(t)

	body := strings.Repeat("Measured on the production estate, step by step. ", 80) +
		"A cp of record.db is not a backup: committed writes live in record.db-wal until a checkpoint."
	var added verbsv1.KnowledgeAddResponse
	if err := w.Call(ctx, "rig.knowledge.add", &verbsv1.KnowledgeAddRequest{
		Title: "Backing up a WAL-mode SQLite store", Summary: "cp loses committed writes; use VACUUM INTO",
		Body: body, Tags: []string{"sqlite", "backup"},
	}, &added); err != nil {
		t.Fatalf("rig.knowledge.add: %v", err)
	}
	if added.GetLesson().GetProv().GetSeat() != "backend-record" {
		t.Errorf("the lesson is attributed to %q", added.GetLesson().GetProv().GetSeat())
	}

	r := seated(t, sock, "another-seat")
	var found verbsv1.KnowledgeSearchResponse
	if err := r.Call(ctx, "rig.knowledge.search", &verbsv1.KnowledgeSearchRequest{
		Query: "how to back up the sqlite store",
	}, &found); err != nil {
		t.Fatalf("rig.knowledge.search: %v", err)
	}
	if len(found.GetHits()) != 1 || found.GetHits()[0].GetId() != added.GetLesson().GetId() {
		t.Fatalf("search answered %+v", found.GetHits())
	}
	consult, whole := proto.Size(&found), len(body)
	t.Logf("section 40's number: one consult is %d bytes on the wire against a %d-byte lesson", consult, whole)
	if consult*10 > whole {
		t.Errorf("a consult costs %d bytes against a %d-byte lesson: it must be a small fraction", consult, whole)
	}

	var got verbsv1.KnowledgeGetResponse
	if err := r.Call(ctx, "rig.knowledge.get", &verbsv1.KnowledgeGetRequest{Id: added.GetLesson().GetId()}, &got); err != nil {
		t.Fatalf("rig.knowledge.get: %v", err)
	}
	if got.GetLesson().GetBody() != body {
		t.Error("knowledge.get did not return the lesson whole")
	}

	p := program(t, sock, "shelf")
	err := p.Call(ctx, "rig.knowledge.add", &verbsv1.KnowledgeAddRequest{Title: "t", Summary: "s"},
		&verbsv1.KnowledgeAddResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a lesson from a connection with no seat")
	err = r.Call(ctx, "rig.knowledge.search", &verbsv1.KnowledgeSearchRequest{Query: "x", Limit: 21},
		&verbsv1.KnowledgeSearchResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a search over the hit limit")
}
