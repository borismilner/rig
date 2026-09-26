// Package files is the free-files area of plan/48 (R12, R17, R20, R24, R32): a
// directory programs and agents write directly, which rig keeps as a git
// repository and commits on an interval.
//
// git is the system binary, exec'd with a fixed argv and never through a shell
// (plan/48 D5). rig commits TEXT ONLY: a binary file is never staged (R32),
// and that is decided here by content, not by a .gitignore a program could
// write.
package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxCommitted is the largest text file rig commits. A text file over it is
// left out like a binary: a multi-hundred-megabyte log is not history anyone
// reads, and it would make every clone carry it forever.
const MaxCommitted = 10 << 20

// sniff is how much of a file decides text or binary: git's own rule, a NUL
// byte in the first 8,000 bytes.
const sniff = 8000

// ErrNoGit means the git binary could not be run.
var ErrNoGit = errors.New("files: the git binary is not on PATH, so rig cannot keep the free files in git")

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Repo is the free-files area.
type Repo struct {
	dir string
	// mu makes one commit at a time: the interval committer and a shutdown
	// commit must not stage over each other.
	mu sync.Mutex
}

// Open makes dir if it is missing, owner-only, and a git repository if it is
// not one yet.
func Open(ctx context.Context, dir string) (*Repo, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("files: the area must be an absolute path, got %q", dir)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrNoGit
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("files: creating %s: %w", dir, err)
	}
	r := &Repo{dir: dir}
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		if _, err := r.git(ctx, nil, "init", "--quiet", "--initial-branch=main"); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("files: reading %s: %w", dir, err)
	}
	return r, nil
}

// Dir is the area's root, the shared folder every program may use (R24).
func (r *Repo) Dir() string { return r.dir }

// MakeDir makes a directory under the area, owner-only. rel comes from the
// layout; os.Root still refuses it if a symlink a program planted would take
// it outside the area.
func (r *Repo) MakeDir(rel string) (string, error) {
	root, err := os.OpenRoot(r.dir)
	if err != nil {
		return "", fmt.Errorf("files: opening %s: %w", r.dir, err)
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(rel, 0o700); err != nil {
		return "", fmt.Errorf("files: making %s: %w", rel, trimRoot(err))
	}
	return filepath.Join(r.dir, rel), nil
}

// Result is what one commit pass did.
type Result struct {
	// Commit is the new commit's id, empty when nothing changed.
	Commit string
	// Paths are the paths the commit carries, additions and deletions both.
	Paths []string
	// Skipped are changed paths left out: binary, too large, or unreadable.
	Skipped []string
}

// Commit stages every changed text file and commits them as one commit, and
// does nothing at all when nothing changed (R9).
func (r *Repo) Commit(ctx context.Context, now time.Time) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	changed, err := r.changed(ctx)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, c := range changed {
		if c.deleted || r.committable(c.path) {
			res.Paths = append(res.Paths, c.path)
		} else {
			res.Skipped = append(res.Skipped, c.path)
		}
	}
	if len(res.Paths) == 0 {
		return res, nil
	}
	// Paths go on standard input, NUL-separated, never in argv: a file named
	// "--force" is a path here and not a flag, and ten thousand files do not
	// overflow the argument list.
	var in bytes.Buffer
	for _, p := range res.Paths {
		in.WriteString(p)
		in.WriteByte(0)
	}
	if _, err := r.git(ctx, in.Bytes(), "add", "--all", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return Result{}, err
	}
	msg := fmt.Sprintf("rig: %d file(s) at %s\n\n%s\n", len(res.Paths),
		now.UTC().Format(time.RFC3339), listed(res.Paths))
	if _, err := r.git(ctx, []byte(msg), "commit", "--quiet", "--no-verify", "--file=-"); err != nil {
		return Result{}, err
	}
	sha, err := r.git(ctx, nil, "rev-parse", "HEAD")
	if err != nil {
		return Result{}, err
	}
	res.Commit = strings.TrimSpace(string(sha))
	return res, nil
}

// maxListed bounds the paths a commit message names; the commit itself
// carries every one.
const maxListed = 200

func listed(paths []string) string {
	if len(paths) <= maxListed {
		return strings.Join(paths, "\n")
	}
	return strings.Join(paths[:maxListed], "\n") + fmt.Sprintf("\n... and %d more", len(paths)-maxListed)
}

type change struct {
	path    string
	deleted bool
}

// changed reads git's view of the working tree: every path that differs from
// HEAD, untracked files included.
func (r *Repo) changed(ctx context.Context) ([]change, error) {
	out, err := r.git(ctx, nil, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return nil, err
	}
	var cs []change
	for _, rec := range bytes.Split(out, []byte{0}) {
		// "XY path": two status letters, a space, the path verbatim.
		if len(rec) < 4 {
			continue
		}
		xy, p := string(rec[:2]), string(rec[3:])
		cs = append(cs, change{path: p, deleted: strings.Contains(xy, "D")})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].path < cs[j].path })
	return cs, nil
}

// committable says a path is a text file rig may commit: a regular file or a
// symlink, not over MaxCommitted, with no NUL in its first 8,000 bytes.
//
// A symlink is committed as the link and never followed: its target may be
// anywhere on the disk, and reading it would let a link decide what rig reads.
func (r *Repo) committable(rel string) bool {
	p := filepath.Join(r.dir, rel)
	fi, err := os.Lstat(p)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxCommitted {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, sniff)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false
	}
	return bytes.IndexByte(head[:n], 0) < 0
}

// git runs one git command in the area.
//
// ⛔ EVERY SETTING BELOW STOPS A FILE IN THE AREA FROM RUNNING CODE AS RIG,
// and programs write that area freely. core.hooksPath=/dev/null disables every
// hook, including one a program drops into .git/hooks; core.fsmonitor=false
// stops `git status` executing a monitor; the user's global and the system
// config are not read, so rig's commits do not depend on Boris's gitconfig
// (signing, templates, aliases). The identity is rig's own.
func (r *Repo) git(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	full := append([]string{
		"-C", r.dir,
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false",
		"-c", "commit.gpgSign=false",
		"-c", "user.name=rig",
		"-c", "user.email=rig@localhost",
	}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		// A path is a path: a file named "*" or ":(glob)x" must not match
		// other files, which could stage a binary this package refused.
		"GIT_LITERAL_PATHSPECS=1",
	)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNoGit
		}
		return nil, fmt.Errorf("files: git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// RunCommitter commits changed text files until ctx ends, and ⛔ NEVER TWO
// COMMITS CLOSER TOGETHER THAN every (Boris, 2026-09-26: "Git should not
// commit more often than once per 5 minutes where this time interval is
// configurable"). That holds across a restart, measured from HEAD's own
// commit time, and for the pass a shutdown runs: what that pass may not
// commit yet stays on disk and goes in the next start's first commit.
//
// report is told of every commit and every failure; a pass that only
// skipped files is not reported, since a binary left out stays left out.
func (r *Repo) RunCommitter(ctx context.Context, every time.Duration, report func(Result, error)) {
	last := r.headTime(ctx)
	pass := func(ctx context.Context) {
		now := time.Now()
		if !last.IsZero() && now.Sub(last) < every {
			return
		}
		res, err := r.Commit(ctx, now)
		if err != nil || res.Commit != "" {
			report(res, err)
		}
		if res.Commit != "" {
			last = now
		}
	}
	// A timer to the exact moment the next commit is allowed, not a ticker:
	// a ticker's tick may land a hair before the gap and lose a whole round.
	next := func() time.Duration {
		if last.IsZero() {
			return every / 10
		}
		return max(time.Until(last.Add(every)), every/10)
	}
	pass(ctx)
	t := time.NewTimer(next())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// The run's own context is over, so the last pass gets a short
			// one of its own rather than none.
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			pass(final)
			cancel()
			return
		case <-t.C:
			pass(ctx)
			t.Reset(next())
		}
	}
}

// headTime is when HEAD was committed, a second late rather than early
// because git keeps whole seconds; zero when there is no commit yet.
func (r *Repo) headTime(ctx context.Context) time.Time {
	out, err := r.git(ctx, nil, "log", "-1", "--format=%ct")
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec+1, 0)
}
