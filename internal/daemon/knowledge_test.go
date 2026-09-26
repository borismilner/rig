package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/record"
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

// plan/48 R24's one-time move: the lessons the record store holds become
// files in the shared lessons folder at start, keeping their ids, and a
// second pass copies nothing twice.
func TestTheRecordsLessonsMoveToTheFolderOnce(t *testing.T) {
	var old record.Lesson
	sock, d := upRecordDaemonWith(t, func(d *Daemon) {
		var err error
		old, err = d.records.AddLesson(context.Background(), record.LessonRequest{
			Title: "Checkpoint before a copy", Summary: "a WAL store copied cold loses writes",
			Body: "Run a checkpoint first.", Tags: []string{"sqlite"}, Session: "s", Seat: "backend-1", Epoch: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	ctx := recordCtx(t)
	c := seated(t, sock, "reader")
	var got verbsv1.KnowledgeGetResponse
	if err := c.Call(ctx, "rig.knowledge.get", &verbsv1.KnowledgeGetRequest{Id: old.ID}, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetLesson().GetTitle() != old.Title || got.GetLesson().GetProv().GetSeat() != "backend-1" ||
		got.GetLesson().GetProv().GetAtUnixNano() != old.Prov.CreatedAt.UnixNano() {
		t.Fatalf("moved lesson: %v", got.GetLesson())
	}
	ix, dir, err := d.lessons()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d.files.dir, dir, old.ID+".md")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("the lesson is not a file: %v", err)
	}
	// Edited in the folder, then moved again with the marker gone: the edit
	// stands, because a file that exists is never overwritten.
	if err := os.WriteFile(p, []byte("---\ntitle: Edited\nsummary: s\nseat: boris\n---\n\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(d.files.internal, lessonsMovedMarker)
	if err := os.Remove(marker); err != nil {
		t.Fatalf("the move left no marker: %v", err)
	}
	d.moveLessons(ctx)
	if l, _ := ix.GetLesson(ctx, dir, old.ID); l.Title != "Edited" {
		t.Fatalf("a second move overwrote the folder: %q", l.Title)
	}
	// Deleted from the folder, then a restart: it stays deleted.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	d.moveLessons(ctx)
	if _, err := os.Stat(p); err == nil {
		t.Fatal("a lesson deleted from the folder came back from the record store")
	}
	// And a lesson written now is a file in the folder too.
	var added verbsv1.KnowledgeAddResponse
	if err := c.Call(ctx, "rig.knowledge.add", &verbsv1.KnowledgeAddRequest{
		Title: "New", Summary: "written after the move", Body: "b",
	}, &added); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d.files.dir, dir, added.GetLesson().GetId()+".md")); err != nil {
		t.Fatalf("a new lesson is not a file: %v", err)
	}
}
