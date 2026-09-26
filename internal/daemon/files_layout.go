package daemon

import (
	"context"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/files"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 48's layout of the free files (R28-R31, R33): where a kind of file
// goes, the layout in force, and relayout. internal/files has the rules.

// maxMovesListed bounds the moves a relayout answer names; it counts all.
const maxMovesListed = 200

func (d *Daemon) serveFilesLayout(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	repo := d.filesRepo(c, f, command)
	if repo == nil {
		return
	}
	ls := d.filesLayouts(c, f, command)
	if ls == nil {
		return
	}
	switch command {
	case "files.place":
		d.serveFilesPlace(c, f, repo, ls)
	case "files.layout":
		resp := &verbsv1.FilesLayoutResponse{File: ls.File()}
		cur := ls.InForce()
		for _, k := range cur.Kinds {
			resp.Kinds = append(resp.Kinds, &verbsv1.FilesKind{Name: k.Name, Place: k.Place})
		}
		if edited, err := ls.Edited(); err != nil {
			resp.Pending, resp.PendingError = true, err.Error()
		} else {
			resp.Pending = edited.Text() != cur.Text()
		}
		c.reply(f.GetStreamId(), resp)
	case "files.relayout":
		var req verbsv1.FilesRelayoutRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "files.relayout: "+err.Error())
			return
		}
		ix := d.filesIndex(c, f, command)
		if ix == nil {
			return
		}
		rep, err := ix.Relayout(ctx, ls, req.GetDryRun())
		if err != nil {
			c.failErr(f.GetStreamId(), filesCode(err), err)
			return
		}
		resp := &verbsv1.FilesRelayoutResponse{
			Changed:    rep.Changed,
			MovesTotal: uint32(min(len(rep.Moves), 1<<31)),
			Reindexed:  uint32(min(rep.Reindexed, 1<<31)), //nolint:gosec // clamped
			Applied:    rep.Applied,
		}
		for _, m := range rep.Moves[:min(len(rep.Moves), maxMovesListed)] {
			resp.Moves = append(resp.Moves, &verbsv1.FilesMove{From: m.From, To: m.To})
		}
		if rep.Applied {
			d.log.Info("the free files were relaid out", "moves", len(rep.Moves), "changed", rep.Changed)
		}
		c.reply(f.GetStreamId(), resp)
	}
}

// serveFilesPlace answers where a file goes. A place that takes {program}
// follows the store verbs' rule for whose (plan/48 D2).
func (d *Daemon) serveFilesPlace(c *conn, f *rigv1.Frame, repo *files.Repo, ls *files.Layouts) {
	var req verbsv1.FilesPlaceRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "files.place: "+err.Error())
		return
	}
	layout := ls.InForce()
	v := files.Vars{Subject: req.GetSubject(), Type: req.GetType()}
	k, known := layout.Kind(req.GetKind())
	if known && k.Uses(files.VarProgram) {
		program, refusal := storeProgram(c, req.GetProgram())
		if refusal != nil {
			refusal.Message = "rig.files.place: " + refusal.GetMessage()
			c.failStatus(f.GetStreamId(), refusal)
			return
		}
		v.Program = program
	}
	rel, err := layout.Place(req.GetKind(), v, req.GetName())
	if err != nil {
		c.failErr(f.GetStreamId(), filesCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &verbsv1.FilesPlaceResponse{
		Path: filepath.Join(repo.Dir(), rel), Relative: rel, Place: k.Place,
	})
}

func (d *Daemon) filesLayouts(c *conn, f *rigv1.Frame, command string) *files.Layouts {
	if ls := d.files.layouts.Load(); ls != nil {
		return ls
	}
	why := "the layout has not been read"
	if w := d.files.layoutWhy.Load(); w != nil {
		why = *w
	}
	c.failStatus(f.GetStreamId(), &rigv1.Status{
		Code:         rigv1.Code_CODE_UNAVAILABLE,
		Message:      "rig." + command + ": the free files' layout is unavailable: " + why,
		Precondition: "the applied layout in rig's internal area parsed",
		Actual:       why,
		Fix:          "read rigd's log for the reason, correct layout.applied, then restart rigd",
	})
	return nil
}
