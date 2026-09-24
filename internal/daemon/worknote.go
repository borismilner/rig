package daemon

import (
	"context"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"

	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/worknote"
)

// serveWorkNote answers section 09's three verbs.
//
// ⛔ ALL THREE NEED A SEAT, INCLUDING THE TWO READS, and that is the opposite
// of the knowledge verbs beside them. A lesson is estate-wide and anybody may
// read one; `worknote.mine` is defined as "what THIS seat wrote", so a
// connection with no seat has no question to ask rather than a wide one. And
// `worknote.about` needs one because the seat is what the answer is attributed
// against when the caller goes on to write its own note about the same thing -
// the same argument B74 makes for the writes.
func (d *Daemon) serveWorkNote(ctx context.Context, c *conn, f *rigv1.Frame, st *record.Store, command string) {
	session, seat, epoch, ok := d.provenance(c)
	if !ok {
		d.refuseUnattributed(c, f, command)
		return
	}
	switch command {
	case "worknote.write":
		var req verbsv1.WorkNoteWriteRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "worknote.write: "+err.Error())
			return
		}
		n, err := worknote.Write(ctx, st, worknote.WriteRequest{
			Project: req.GetProject(), Body: req.GetBody(), Tags: req.GetTags(),
			Fields: req.GetFields(), PartOf: req.GetPartOf(),
			Session: session, Seat: seat, Epoch: epoch,
		})
		if err != nil {
			c.failErr(f.GetStreamId(), recordCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.WorkNoteWriteResponse{Note: workNoteToWire(n)})
	case "worknote.mine":
		var req verbsv1.WorkNoteMineRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "worknote.mine: "+err.Error())
			return
		}
		// ⛔ THE SEAT IS THE CONNECTION's AND THE REQUEST CARRIES NONE. A seat
		// argument here would let any agent read any other agent's working
		// notes, which is the forgery surface verbs.proto forbids by name for
		// every other provenance field.
		got, err := worknote.Mine(ctx, st, worknote.MineRequest{
			Seat: seat, Project: req.GetProject(), Limit: int(req.GetLimit()),
		})
		if err != nil {
			c.failErr(f.GetStreamId(), recordCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.WorkNoteMineResponse{
			Notes: workNotesToWire(got.Notes), Total: got.Total,
		})
	case "worknote.about":
		var req verbsv1.WorkNoteAboutRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "worknote.about: "+err.Error())
			return
		}
		got, err := worknote.About(ctx, st, worknote.AboutRequest{
			ID: req.GetId(), Limit: int(req.GetLimit()),
		})
		if err != nil {
			c.failErr(f.GetStreamId(), recordCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.WorkNoteAboutResponse{
			Notes: workNotesToWire(got.Notes), Total: got.Total,
		})
	}
}

func workNoteToWire(n worknote.Note) *verbsv1.WorkNote {
	return &verbsv1.WorkNote{
		Id: n.ID, Project: n.Project, Body: n.Body, Tags: n.Tags,
		Fields:   n.Fields,
		Attached: n.Attached, Missing: n.Missing,
		Prov: &verbsv1.Provenance{
			Session: n.Session, Seat: n.Seat, Epoch: n.Epoch,
			AtUnixNano: n.At.UnixNano(),
		},
	}
}

func workNotesToWire(in []worknote.Note) []*verbsv1.WorkNote {
	out := make([]*verbsv1.WorkNote, 0, len(in))
	for _, n := range in {
		out = append(out, workNoteToWire(n))
	}
	return out
}
