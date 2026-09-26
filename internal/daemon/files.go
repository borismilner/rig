package daemon

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/files"
	"github.com/borismilner/rig/internal/paths"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 48's free files (P9 slice 4). internal/files is the git side; this
// file opens the area when serving starts, runs the interval committer for as
// long as serving lasts, and answers files.root.

// DefaultFilesCommitEvery is the least time between two commits of the free
// files, and rigd's --files-commit-every moves it. Boris's figure, plan/48
// R34: never more often than once per 5 minutes, configurable.
const DefaultFilesCommitEvery = 5 * time.Minute

type freeFiles struct {
	dir   string // empty: this daemon was given no root
	every time.Duration

	repo atomic.Pointer[files.Repo]
	// why is the reason the area did not open, for files.root to say.
	why atomic.Pointer[string]

	// The index over the area (R25), in rig's internal area and never in
	// files/. It opens after the repository; indexWhy says why it did not.
	indexPath string
	index     atomic.Pointer[files.Index]
	indexWhy  atomic.Pointer[string]

	// The layout (R28-R31), two text files in rig's internal area.
	internal  string
	layouts   atomic.Pointer[files.Layouts]
	layoutWhy atomic.Pointer[string]
}

func newFreeFiles(root, estate string, every time.Duration) (*freeFiles, error) {
	switch {
	case every == 0:
		every = DefaultFilesCommitEvery
	case every < time.Second:
		// Below a second the committer would run git back to back, and a
		// negative interval panics the ticker.
		return nil, fmt.Errorf("the free files' commit interval must be at least a second, got %v", every)
	}
	ff := &freeFiles{every: every}
	if root == "" {
		return ff, nil
	}
	var areas paths.Areas
	var err error
	if estate == "" {
		areas, err = paths.ScratchAreas(root)
	} else {
		areas, err = paths.EstateAreas(root, estate)
	}
	if err != nil {
		return nil, err
	}
	ff.dir = areas.Files
	ff.indexPath = filepath.Join(areas.Internal, "files-index.db")
	ff.internal = areas.Internal
	return ff, nil
}

// startFiles opens the area and runs the committer until ctx ends. The
// returned wait blocks until the committer's last pass is done.
//
// ⛔ A FAILURE TO OPEN IS LOGGED, NOT FATAL: a machine without git still runs
// every other verb, and files.root answers why it cannot.
func (d *Daemon) startFiles(ctx context.Context) (wait func()) {
	ff := d.files
	if ff == nil || ff.dir == "" {
		return func() {}
	}
	repo, err := files.Open(ctx, ff.dir)
	if err != nil {
		why := err.Error()
		ff.why.Store(&why)
		d.log.Error("the free files are unavailable", "dir", ff.dir, "err", err)
		return func() {}
	}
	ff.repo.Store(repo)
	d.log.Info("free files", "dir", ff.dir, "commit_every", ff.every)
	if ls, err := files.OpenLayouts(ff.internal); err != nil {
		why := err.Error()
		ff.layoutWhy.Store(&why)
		d.log.Error("the free files' layout is unavailable", "dir", ff.internal, "err", err)
	} else {
		ff.layouts.Store(ls)
	}
	if ix, err := files.OpenIndex(ctx, ff.indexPath, repo); err != nil {
		why := err.Error()
		ff.indexWhy.Store(&why)
		d.log.Error("the free files' index is unavailable", "path", ff.indexPath, "err", err)
	} else {
		ff.index.Store(ix)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		repo.RunCommitter(ctx, ff.every, func(res files.Result, err error) {
			if err != nil {
				d.log.Error("committing the free files failed", "err", err)
				return
			}
			d.log.Info("free files committed", "commit", res.Commit,
				"paths", len(res.Paths), "skipped", len(res.Skipped))
		})
	}()
	return wg.Wait
}

func (d *Daemon) serveFilesRoot(c *conn, f *rigv1.Frame) {
	var req verbsv1.FilesRootRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "files.root: "+err.Error())
		return
	}
	repo := d.filesRepo(c, f, "files.root")
	if repo == nil {
		return
	}
	resp := &verbsv1.FilesRootResponse{CommitEveryS: uint32(min(d.files.every/time.Second, 1<<31))} //nolint:gosec // clamped
	if req.GetShared() {
		resp.Path = repo.Dir()
		c.reply(f.GetStreamId(), resp)
		return
	}
	program, refusal := storeProgram(c, req.GetProgram())
	if refusal != nil {
		refusal.Message = "rig.files.root: " + refusal.GetMessage()
		c.failStatus(f.GetStreamId(), refusal)
		return
	}
	resp.Program = program
	ls := d.filesLayouts(c, f, "files.root")
	if ls == nil {
		return
	}
	rel, err := ls.InForce().Dir(files.ProgramsKind, files.Vars{Program: program})
	if err != nil {
		c.failErr(f.GetStreamId(), filesCode(err), err)
		return
	}
	resp.Path = filepath.Join(repo.Dir(), rel)
	if c.scoped.Load() {
		// A program's own directory is made for it. A terminal or an agent is
		// told where it is and makes nothing, for the reason a store read
		// makes nothing: the name is not proved.
		if _, err := repo.MakeDir(rel); err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.files.root: "+err.Error())
			return
		}
	}
	c.reply(f.GetStreamId(), resp)
}

func (d *Daemon) filesRepo(c *conn, f *rigv1.Frame, command string) *files.Repo {
	ff := d.files
	if ff == nil || ff.dir == "" {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig." + command + ": this daemon was given no storage root, so it keeps no free files",
			Precondition: "rigd resolved a storage root at start",
			Actual:       "no root",
			Fix:          "start rigd with --root, or let it take its default",
		})
		return nil
	}
	if r := ff.repo.Load(); r != nil {
		return r
	}
	why := "the area has not opened"
	if w := ff.why.Load(); w != nil {
		why = *w
	}
	c.failStatus(f.GetStreamId(), &rigv1.Status{
		Code:         rigv1.Code_CODE_UNAVAILABLE,
		Message:      "rig." + command + ": the free files are unavailable: " + why,
		Precondition: "the free-files area opened as a git repository",
		Actual:       why,
		Fix:          "install git, then restart rigd",
	})
	return nil
}

// close closes the index. The daemon calls it once serving is over.
func (ff *freeFiles) close() error {
	if ff == nil {
		return nil
	}
	if ix := ff.index.Swap(nil); ix != nil {
		return ix.Close()
	}
	return nil
}

// serveFiles answers the files verbs.
func (d *Daemon) serveFiles(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	switch command {
	case "files.root":
		d.serveFilesRoot(c, f)
	case "files.place", "files.layout", "files.relayout":
		d.serveFilesLayout(ctx, c, f, command)
	default:
		d.serveFilesIndex(ctx, c, f, command)
	}
}

// serveFilesIndex answers files.index, files.search and files.unindexed.
func (d *Daemon) serveFilesIndex(ctx context.Context, c *conn, f *rigv1.Frame, command string) {
	ix := d.filesIndex(c, f, command)
	if ix == nil {
		return
	}
	id := f.GetStreamId()
	switch command {
	case "files.index":
		var req verbsv1.FilesIndexRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "files.index: "+err.Error())
			return
		}
		writer, ok := d.fileWriter(c)
		if !ok {
			d.refuseUnattributed(c, f, command)
			return
		}
		e, err := ix.Put(ctx, files.Request{
			Path: req.GetPath(), Title: req.GetTitle(), Summary: req.GetSummary(),
			Tags: req.GetTags(), Writer: writer,
		})
		if err != nil {
			c.failErr(id, filesCode(err), err)
			return
		}
		c.reply(id, &verbsv1.FilesIndexResponse{
			Path: e.Path, Removed: e.Removed, TextBytes: uint32(e.TextBytes), //nolint:gosec // at most MaxIndexedText
			Binary: e.Binary, Truncated: e.Truncated,
		})
	case "files.search":
		var req verbsv1.FilesSearchRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "files.search: "+err.Error())
			return
		}
		hits, err := ix.Search(ctx, req.GetQuery(), req.GetUnder(), int(min(req.GetLimit(), files.MaxHits+1)))
		if err != nil {
			c.failErr(id, filesCode(err), err)
			return
		}
		resp := &verbsv1.FilesSearchResponse{}
		for _, h := range hits {
			resp.Hits = append(resp.Hits, &verbsv1.FilesHit{
				Path: h.Path, Title: h.Title, Summary: h.Summary, Snippet: h.Snippet, Score: h.Score,
			})
		}
		c.reply(id, resp)
	case "files.unindexed":
		var req verbsv1.FilesUnindexedRequest
		if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "files.unindexed: "+err.Error())
			return
		}
		got, total, err := ix.Unindexed(ctx, req.GetUnder(), int(min(req.GetLimit(), files.MaxPending+1)))
		if err != nil {
			c.failErr(id, filesCode(err), err)
			return
		}
		resp := &verbsv1.FilesUnindexedResponse{Total: uint32(min(total, 1<<31))} //nolint:gosec // clamped
		for _, p := range got {
			resp.Files = append(resp.Files, &verbsv1.FilesPending{Path: p.Path, State: p.State})
		}
		c.reply(id, resp)
	}
}

// fileWriter is who an index entry says wrote it: a registered program by its
// name, otherwise the caller's seat, as a record write is attributed.
func (d *Daemon) fileWriter(c *conn) (string, bool) {
	if c.scoped.Load() {
		return "program:" + c.name(), true
	}
	if _, seat, _, ok := d.provenance(c); ok {
		return "seat:" + seat, true
	}
	return "", false
}

func filesCode(err error) rigv1.Code {
	var inv *files.InvalidError
	if errors.As(err, &inv) {
		return rigv1.Code_CODE_INVALID
	}
	return rigv1.Code_CODE_INTERNAL
}

func (d *Daemon) filesIndex(c *conn, f *rigv1.Frame, command string) *files.Index {
	if d.filesRepo(c, f, command) == nil {
		return nil
	}
	if ix := d.files.index.Load(); ix != nil {
		return ix
	}
	why := "the index has not opened"
	if w := d.files.indexWhy.Load(); w != nil {
		why = *w
	}
	c.failStatus(f.GetStreamId(), &rigv1.Status{
		Code:         rigv1.Code_CODE_UNAVAILABLE,
		Message:      "rig." + command + ": the free files' index is unavailable: " + why,
		Precondition: "the index database opened in rig's internal area",
		Actual:       why,
		Fix:          "read rigd's log for the reason, then restart rigd",
	})
	return nil
}
