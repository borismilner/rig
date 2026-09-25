package meta

import (
	"context"
	"errors"

	"github.com/borismilner/rig/internal/kernel"
)

// WorkNote is one section 09 working note as a tool answers it. Tags come back
// as a list; the `tag:` encoding in the store is not the caller's business.
type WorkNote struct {
	ID      string            `json:"id"`
	Project string            `json:"project"`
	Body    string            `json:"body"`
	Tags    []string          `json:"tags"`
	Fields  map[string]string `json:"fields"`
	Seat    string            `json:"seat"`
	Epoch   uint64            `json:"epoch"`
	Written string            `json:"written"`

	// Attached and Missing are a WRITE's account: the ids an edge went to, and
	// the ids that held no record. Empty on a read.
	Attached []string `json:"attached"`
	Missing  []string `json:"missing"`
}

// WorkNoteList is worknote_mine's and worknote_about's answer. Total is the
// count with no limit, so a cut answer says it was cut; a struct so an empty
// list still renders as `"notes": []`.
type WorkNoteList struct {
	Notes []WorkNote `json:"notes"`
	Total uint64     `json:"total"`
}

// WorkNotes is section 09's working notes behind the MCP door. A Records
// implementation that also implements it serves the three tools; one that does
// not answers them as unavailable, as Lessons does.
//
// No method takes the seat: it is the connection's, so one agent cannot read
// or sign another's notes.
type WorkNotes interface {
	WriteNote(ctx context.Context, project, body string, tags []string, fields map[string]string, partOf []string) (WorkNote, error)
	MyNotes(ctx context.Context, project string, limit int) (WorkNoteList, error)
	NotesAbout(ctx context.Context, id string, limit int) (WorkNoteList, error)
}

// worknote answers worknote_write, worknote_mine and worknote_about.
func (s *Server) worknote(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	return s.recordTool(who, r.Tool, func(rc Records, out *Answer) error {
		wn, ok := rc.(WorkNotes)
		if !ok {
			return errors.New("meta: this estate's store does not serve working notes")
		}
		switch r.Tool {
		case WorkNoteWriteTool:
			n, err := wn.WriteNote(ctx, r.Project, r.Body, r.Tags, r.Fields, r.PartOf)
			if err != nil {
				return err
			}
			out.Record = &RecordAnswer{Note: &n}
		case WorkNoteMineTool:
			l, err := wn.MyNotes(ctx, r.Project, r.Limit)
			if err != nil {
				return err
			}
			out.Record = &RecordAnswer{Notes: &l}
		default:
			l, err := wn.NotesAbout(ctx, r.RecordID, r.Limit)
			if err != nil {
				return err
			}
			out.Record = &RecordAnswer{Notes: &l}
		}
		return nil
	})
}
