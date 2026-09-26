package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/files"
	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/store"
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
	repo := d.filesRepo(c, f)
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
	if c.scoped.Load() {
		// A program's own directory is made for it.
		dir, err := repo.ProgramDir(program)
		if err != nil {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, "rig.files.root: "+err.Error())
			return
		}
		resp.Path = dir
	} else {
		// A terminal or an agent is told where it is and makes nothing, for
		// the reason a store read makes nothing: the name is not proved.
		dir, ok := programPath(repo.Dir(), program)
		if !ok {
			c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID,
				"rig.files.root: program "+program+" is not a name rig puts on disk")
			return
		}
		resp.Path = dir
	}
	c.reply(f.GetStreamId(), resp)
}

// programPath is where a program's directory is, without making it.
func programPath(root, program string) (string, bool) {
	if store.CheckName("program", program) != nil {
		return "", false
	}
	return filepath.Join(root, files.ProgramsDir, program), true
}

func (d *Daemon) filesRepo(c *conn, f *rigv1.Frame) *files.Repo {
	ff := d.files
	if ff == nil || ff.dir == "" {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig.files.root: this daemon was given no storage root, so it keeps no free files",
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
		Message:      "rig.files.root: the free files are unavailable: " + why,
		Precondition: "the free-files area opened as a git repository",
		Actual:       why,
		Fix:          "install git, then restart rigd",
	})
	return nil
}
