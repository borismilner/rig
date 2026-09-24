package daemon

import (
	"context"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// serveKnowledge answers section 40's three verbs. Search and get are reads;
// add writes a lesson under the caller's seat, the same attribution a record
// write takes, and refuses a connection that holds none.
func (d *Daemon) serveKnowledge(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store, command string) {
	switch command {
	case "knowledge.add":
		var req verbsv1.KnowledgeAddRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "knowledge.add: "+err.Error())
			return
		}
		session, seat, epoch, ok := d.provenance(c)
		if !ok {
			d.refuseUnattributed(c, f, command)
			return
		}
		l, err := st.AddLesson(ctx, record.LessonRequest{
			Title: req.GetTitle(), Summary: req.GetSummary(), Body: req.GetBody(), Tags: req.GetTags(),
			Session: session, Seat: seat, Epoch: epoch,
		})
		if err != nil {
			c.failErr(f.GetStreamId(), recordCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.KnowledgeAddResponse{Lesson: lessonToWire(l)})
	case "knowledge.search":
		var req verbsv1.KnowledgeSearchRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "knowledge.search: "+err.Error())
			return
		}
		hits, err := st.SearchLessons(ctx, req.GetQuery(), int(min(req.GetLimit(), record.MaxHits+1)))
		if err != nil {
			c.failErr(f.GetStreamId(), recordCode(err), err)
			return
		}
		resp := &verbsv1.KnowledgeSearchResponse{}
		for _, h := range hits {
			resp.Hits = append(resp.Hits, &verbsv1.LessonHit{
				Id: h.ID, Title: h.Title, Summary: h.Summary, Snippet: h.Snippet, Score: h.Score,
			})
		}
		c.reply(f.GetStreamId(), resp)
	case "knowledge.get":
		var req verbsv1.KnowledgeGetRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "knowledge.get: "+err.Error())
			return
		}
		l, err := st.GetLesson(ctx, req.GetId())
		if err != nil {
			c.failErr(f.GetStreamId(), recordCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.KnowledgeGetResponse{Lesson: lessonToWire(l)})
	}
}

func lessonToWire(l record.Lesson) *verbsv1.Lesson {
	return &verbsv1.Lesson{
		Id: l.ID, Title: l.Title, Summary: l.Summary, Body: l.Body, Tags: l.Tags,
		Prov: &verbsv1.Provenance{
			Session: l.Prov.Session, Seat: l.Prov.Seat, Epoch: l.Prov.Epoch,
			AtUnixNano: l.Prov.CreatedAt.UnixNano(),
		},
	}
}
