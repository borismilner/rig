package files

// Relayout, plan/48 R30: Boris edits the layout and rig moves the files
// already written to match it, rewriting their index entries so nothing a
// search points at breaks.
//
// ⛔ ALL OR NOTHING. Every move is planned before any is made, and the plan is
// refused whole when a file would be stranded (a kind removed with files
// still in it, a file the old place does not lay out, a new place needing a
// value the old one never gave) or would land on an existing file. A failure
// part-way puts back what was moved.
//
// ⛔ NO COMMIT OF ITS OWN. The moves land in the committer's next pass, since
// Boris's R34 says no two commits closer than the interval. git reads a
// deleted and an added file with the same text as a rename, so history
// follows a text file across the move.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// Move is one file relayout moves.
type Move struct{ From, To string }

// RelayoutReport is what relayout did, or would do on a dry run.
type RelayoutReport struct {
	// Changed are the kinds whose place changed, added or removed.
	Changed []string
	Moves   []Move
	// Reindexed is how many index entries now carry the new path.
	Reindexed int
	// Applied means the edited layout is now the layout in force.
	Applied bool
}

const maxStrays = 20

// Relayout applies the edited layout: plans every move, and unless dryRun
// makes them, rewrites the index and puts the edited layout in force.
func (ix *Index) Relayout(ctx context.Context, ls *Layouts, dryRun bool) (RelayoutReport, error) {
	next, err := ls.Edited()
	if err != nil {
		return RelayoutReport{}, err
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	cur := ls.InForce()
	rep, err := ix.plan(cur, next)
	if err != nil || dryRun {
		return rep, err
	}
	var done []Move
	undo := func() {
		for i := len(done) - 1; i >= 0; i-- {
			_ = ix.root.Rename(done[i].To, done[i].From)
		}
	}
	for _, m := range rep.Moves {
		if err := ctx.Err(); err != nil {
			undo()
			return RelayoutReport{}, err
		}
		if err := ix.root.MkdirAll(path.Dir(m.To), 0o700); err != nil {
			undo()
			return RelayoutReport{}, fmt.Errorf("files: making %s: %w", path.Dir(m.To), trimRoot(err))
		}
		if err := ix.root.Rename(m.From, m.To); err != nil {
			undo()
			return RelayoutReport{}, fmt.Errorf("files: moving %s to %s: %w", m.From, m.To, trimRoot(err))
		}
		done = append(done, m)
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		undo()
		return RelayoutReport{}, fmt.Errorf("files: rewriting the index: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range rep.Moves {
		res, err := tx.ExecContext(ctx, `UPDATE entries SET path = ? WHERE path = ?`, m.To, m.From)
		if err != nil {
			undo()
			return RelayoutReport{}, fmt.Errorf("files: rewriting the index: %w", err)
		}
		n, _ := res.RowsAffected()
		rep.Reindexed += int(n)
	}
	// The layout goes in force before the index commits, so a failure to
	// write it leaves the index and the files as they were.
	if err := ls.apply(next); err != nil {
		undo()
		return RelayoutReport{}, err
	}
	if err := tx.Commit(); err != nil {
		_ = ls.apply(cur)
		undo()
		return RelayoutReport{}, fmt.Errorf("files: rewriting the index: %w", err)
	}
	rep.Applied = true
	for _, m := range rep.Moves {
		ix.pruneEmpty(path.Dir(m.From))
	}
	return rep, nil
}

// plan works out every move from cur to next and refuses the relayout whole
// if any file would be stranded or overwritten.
func (ix *Index) plan(cur, next Layout) (RelayoutReport, error) {
	var rep RelayoutReport
	var strays []string
	for _, k := range cur.Kinds {
		nk, kept := next.Kind(k.Name)
		if kept && nk.Place == k.Place {
			continue
		}
		if !kept && samePlace(next, k.Place) {
			// Renamed, not moved: another kind now lays out these files.
			rep.Changed = append(rep.Changed, k.Name)
			continue
		}
		rep.Changed = append(rep.Changed, k.Name)
		files, err := ix.filesUnder(k.segs[0])
		if err != nil {
			return RelayoutReport{}, err
		}
		if len(files) == 0 {
			continue
		}
		if !kept {
			return RelayoutReport{}, invalid("kind %s is gone from the edited layout but %d file(s) are under %s/; "+
				"move or delete them, or keep the kind", k.Name, len(files), k.segs[0])
		}
		for _, v := range nk.vars() {
			if !slices.Contains(k.vars(), v) {
				return RelayoutReport{}, invalid("kind %s's new place %s needs {%s}, which its files under %s "+
					"do not carry; give it a directory name instead", k.Name, nk.Place, v, k.Place)
			}
		}
		for _, f := range files {
			vals, rest, ok := k.match(f)
			if !ok {
				strays = append(strays, f)
				continue
			}
			rep.Moves = append(rep.Moves, Move{From: f, To: nk.render(vals) + "/" + rest})
		}
	}
	for _, k := range next.Kinds {
		if _, had := cur.Kind(k.Name); !had {
			rep.Changed = append(rep.Changed, k.Name)
		}
	}
	if len(strays) > 0 {
		shown := strays[:min(len(strays), maxStrays)]
		return RelayoutReport{}, invalid("%d file(s) are not where the current layout puts them, so relayout "+
			"cannot tell where they go: %s; move them into place first", len(strays), strings.Join(shown, ", "))
	}
	for _, m := range rep.Moves {
		if _, err := ix.root.Lstat(m.To); !errors.Is(err, fs.ErrNotExist) {
			return RelayoutReport{}, invalid("moving %s would land on %s, which exists; nothing was moved", m.From, m.To)
		}
	}
	return rep, nil
}

// filesUnder lists every file and symlink under a top-level directory.
// ⛔ Unlike the listing, an unreadable directory refuses: relayout cannot
// promise to move what it cannot see.
func (ix *Index) filesUnder(top string) ([]string, error) {
	var out []string
	err := fs.WalkDir(ix.root.FS(), top, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil && p == top && errors.Is(err, fs.ErrNotExist):
			return fs.SkipAll
		case err != nil:
			return invalid("%s cannot be read, so relayout cannot promise to move what is in it: %v", p, trimRoot(err))
		case d.IsDir():
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

// pruneEmpty removes directories a move left empty, up to the top level.
// Remove refuses a directory that is not empty, which is what stops it.
func (ix *Index) pruneEmpty(dir string) {
	for dir != "." && dir != "/" && dir != "" {
		if err := ix.root.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return
		}
		dir = path.Dir(dir)
	}
}

func samePlace(l Layout, place string) bool {
	for _, k := range l.Kinds {
		if k.Place == place {
			return true
		}
	}
	return false
}
