package daemon

import (
	"context"
	"errors"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/meta"
	"github.com/borismilner/rig/internal/worknote"
)

// The working-note tools are the MCP door onto internal/worknote, and they
// need a seat for the reason serveWorkNote gives: mine is "what THIS seat
// wrote", so an unseated caller has no question to ask.
var _ meta.WorkNotes = (*mcpCaller)(nil)

func (m *mcpCaller) WriteNote(ctx context.Context, project, body string, tags []string, fields map[string]string, partOf []string) (meta.WorkNote, error) {
	st, err := m.store()
	if err != nil {
		return meta.WorkNote{}, err
	}
	session, seat, epoch, err := m.writer()
	if err != nil {
		return meta.WorkNote{}, err
	}
	n, err := worknote.Write(ctx, st, worknote.WriteRequest{
		Project: project, Body: body, Tags: tags, Fields: fields, PartOf: partOf,
		Session: session, Seat: seat, Epoch: epoch,
	})
	if err != nil {
		return meta.WorkNote{}, err
	}
	return noteOf(n), nil
}

func (m *mcpCaller) MyNotes(ctx context.Context, project string, limit int) (meta.WorkNoteList, error) {
	st, err := m.store()
	if err != nil {
		return meta.WorkNoteList{}, err
	}
	seat, err := m.noteReader()
	if err != nil {
		return meta.WorkNoteList{}, err
	}
	got, err := worknote.Mine(ctx, st, worknote.MineRequest{Seat: seat, Project: project, Limit: limit})
	if err != nil {
		return meta.WorkNoteList{}, err
	}
	return noteListOf(got), nil
}

func (m *mcpCaller) NotesAbout(ctx context.Context, id string, limit int) (meta.WorkNoteList, error) {
	st, err := m.store()
	if err != nil {
		return meta.WorkNoteList{}, err
	}
	if _, err := m.noteReader(); err != nil {
		return meta.WorkNoteList{}, err
	}
	got, err := worknote.About(ctx, st, worknote.AboutRequest{ID: id, Limit: limit})
	if err != nil {
		return meta.WorkNoteList{}, err
	}
	return noteListOf(got), nil
}

// noteReader is writer's seat check for the two reads, worded as a read: an
// agent told "a record write needs a seat" after asking for its notes learns
// the wrong model of the verb.
func (m *mcpCaller) noteReader() (string, error) {
	occ, found := m.presence.occupantOf(m.occ)
	if !found || occ.seat == "" {
		return "", &kernel.RefusalError{
			Err:          errors.New("reading working notes needs a seat"),
			Precondition: "worknote_mine is what YOUR seat wrote, and the seat comes from your roster row rather than from the request",
			Actual:       "this connection holds no seat",
			Fix:          "call announce with your seat name first, and wait for its answer",
		}
	}
	return occ.seat, nil
}

func noteOf(n worknote.Note) meta.WorkNote {
	out := meta.WorkNote{
		ID: n.ID, Project: n.Project, Body: n.Body, Tags: n.Tags, Fields: n.Fields,
		Seat: n.Seat, Epoch: n.Epoch, Written: n.At.UTC().Format(worknote.Stamp),
		Attached: n.Attached, Missing: n.Missing,
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if out.Fields == nil {
		out.Fields = map[string]string{}
	}
	if out.Attached == nil {
		out.Attached = []string{}
	}
	if out.Missing == nil {
		out.Missing = []string{}
	}
	return out
}

func noteListOf(in worknote.Notes) meta.WorkNoteList {
	out := meta.WorkNoteList{Notes: make([]meta.WorkNote, 0, len(in.Notes)), Total: in.Total}
	for _, n := range in.Notes {
		out.Notes = append(out.Notes, noteOf(n))
	}
	return out
}
