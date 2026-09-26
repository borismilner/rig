package daemon

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/files"
	"github.com/borismilner/rig/internal/kernel"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// serveKnowledge answers section 40's three verbs from the shared lessons
// folder (plan/48 R24, R25): a lesson is a file there, and the search is the
// free files' index over it. Search and get are reads; add writes a lesson
// under the caller's seat, the attribution a record write takes, and refuses
// a connection that holds none.
func (d *Daemon) serveKnowledge(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	id := f.GetStreamId()
	// A write names who wrote it before anything else is asked of it.
	session, seat, epoch, seated := d.provenance(c)
	if command == "knowledge.add" && !seated {
		d.refuseUnattributed(c, f, command)
		return
	}
	ix, dir, err := d.lessons()
	if err != nil {
		c.failErr(id, lessonCode(err), err)
		return
	}
	switch command {
	case "knowledge.add":
		var req verbsv1.KnowledgeAddRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "knowledge.add: "+err.Error())
			return
		}
		l, err := ix.AddLesson(ctx, dir, files.Lesson{
			Title: req.GetTitle(), Summary: req.GetSummary(), Body: req.GetBody(), Tags: req.GetTags(),
			Session: session, Seat: seat, Epoch: epoch,
		})
		if err != nil {
			c.failErr(id, lessonCode(err), err)
			return
		}
		c.reply(id, &verbsv1.KnowledgeAddResponse{Lesson: lessonToWire(l)})
	case "knowledge.search":
		var req verbsv1.KnowledgeSearchRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "knowledge.search: "+err.Error())
			return
		}
		hits, err := ix.SearchLessons(ctx, dir, req.GetQuery(), int(min(req.GetLimit(), files.MaxHits+1)))
		if err != nil {
			c.failErr(id, lessonCode(err), err)
			return
		}
		resp := &verbsv1.KnowledgeSearchResponse{}
		for _, h := range hits {
			resp.Hits = append(resp.Hits, &verbsv1.LessonHit{
				Id: h.ID, Title: h.Title, Summary: h.Summary, Snippet: h.Snippet, Score: h.Score,
			})
		}
		c.reply(id, resp)
	case "knowledge.get":
		var req verbsv1.KnowledgeGetRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "knowledge.get: "+err.Error())
			return
		}
		l, err := ix.GetLesson(ctx, dir, req.GetId())
		if err != nil {
			c.failErr(id, lessonCode(err), err)
			return
		}
		c.reply(id, &verbsv1.KnowledgeGetResponse{Lesson: lessonToWire(l)})
	}
}

// lessons is the index and the lessons folder under the layout in force, or
// a refusal saying which is missing.
func (d *Daemon) lessons() (*files.Index, string, error) {
	unavailable := func(actual string) error {
		return &kernel.RefusalError{
			Err:          errors.New("the lessons are unavailable: " + actual),
			Precondition: "the free files, their index and their layout opened (plan/48)",
			Actual:       actual,
			Fix:          "start rigd with a storage root and git on PATH, then read rigd's log",
		}
	}
	ff := d.files
	if ff == nil || ff.dir == "" {
		return nil, "", unavailable("this daemon was given no storage root")
	}
	ix, ls := ff.index.Load(), ff.layouts.Load()
	if ix == nil || ls == nil {
		return nil, "", unavailable("the free files' index or layout did not open")
	}
	dir, err := ls.InForce().Dir(lessonsKind, files.Vars{})
	if err != nil {
		return nil, "", err
	}
	return ix, dir, nil
}

// lessonsKind is the layout's kind the lessons live in (R24, R31).
const lessonsKind = "lessons"

func lessonCode(err error) rigv1.Code {
	var nf *files.NotFoundError
	var ref *kernel.RefusalError
	switch {
	case errors.As(err, &nf):
		return rigv1.Code_CODE_NOT_FOUND
	case errors.As(err, &ref):
		return rigv1.Code_CODE_UNAVAILABLE
	}
	return filesCode(err)
}

func lessonToWire(l files.Lesson) *verbsv1.Lesson {
	return &verbsv1.Lesson{
		Id: l.ID, Title: l.Title, Summary: l.Summary, Body: l.Body, Tags: l.Tags,
		Prov: &verbsv1.Provenance{
			Session: l.Session, Seat: l.Seat, Epoch: l.Epoch, AtUnixNano: l.Created.UnixNano(),
		},
	}
}
