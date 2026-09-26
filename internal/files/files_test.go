package files

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

// isolated keeps every test off the developer's git configuration, so a
// test that passes cannot be passing because of ~/.gitconfig.
func isolated(t *testing.T) *Repo {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r, err := Open(ctx(t), filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t *testing.T, r *Repo, rel, body string) {
	t.Helper()
	p := filepath.Join(r.Dir(), rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func tracked(t *testing.T, r *Repo) string {
	t.Helper()
	out, err := r.git(ctx(t), nil, "ls-files", "--stage")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// R9: one commit carrying what changed, and nothing at all when nothing did.
func TestACommitOnlyWhenSomethingChanged(t *testing.T) {
	r := isolated(t)
	c := ctx(t)
	if res, err := r.Commit(c, time.Now()); err != nil || res.Commit != "" {
		t.Fatalf("an empty area was committed: %+v, %v", res, err)
	}
	write(t, r, "programs/graft/notes.md", "# notes\n")
	write(t, r, "lessons/a.md", "a lesson\n")
	first, err := r.Commit(c, time.Now())
	if err != nil || first.Commit == "" || len(first.Paths) != 2 {
		t.Fatalf("first pass: %+v, %v", first, err)
	}
	again, err := r.Commit(c, time.Now())
	if err != nil || again.Commit != "" {
		t.Fatalf("a pass with nothing changed committed: %+v, %v", again, err)
	}
	head, _ := r.git(c, nil, "rev-parse", "HEAD")
	if strings.TrimSpace(string(head)) != first.Commit {
		t.Fatal("HEAD moved on a pass with nothing to commit")
	}
	if err := os.Remove(filepath.Join(r.Dir(), "lessons/a.md")); err != nil {
		t.Fatal(err)
	}
	del, err := r.Commit(c, time.Now())
	if err != nil || del.Commit == "" || strings.Contains(tracked(t, r), "lessons/a.md") {
		t.Fatalf("a deletion was not committed: %+v, %v", del, err)
	}
}

// R32: a binary file is never committed, and neither is an oversized one.
func TestABinaryIsNeverCommitted(t *testing.T) {
	r := isolated(t)
	write(t, r, "resources/pdf/x.pdf", "%PDF\x00\x01binary")
	write(t, r, "resources/big.log", strings.Repeat("a", MaxCommitted+1))
	write(t, r, "docs/readme.md", "text\n")
	res, err := r.Commit(ctx(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := tracked(t, r)
	if strings.Contains(got, "x.pdf") || strings.Contains(got, "big.log") || !strings.Contains(got, "readme.md") {
		t.Fatalf("tracked after the pass:\n%s", got)
	}
	if strings.Join(res.Skipped, ",") != "resources/big.log,resources/pdf/x.pdf" {
		t.Fatalf("skipped %v", res.Skipped)
	}
}

// A path is a path: "*" must not stage the binary beside it, and "--force"
// must not become a flag.
func TestAHostileFileNameIsOnlyAName(t *testing.T) {
	r := isolated(t)
	write(t, r, "bin.dat", "\x00\x00")
	write(t, r, "*", "star\n")
	write(t, r, "--force", "dashes\n")
	write(t, r, ":(glob)**", "magic\n")
	if _, err := r.Commit(ctx(t), time.Now()); err != nil {
		t.Fatal(err)
	}
	got := tracked(t, r)
	if strings.Contains(got, "bin.dat") {
		t.Fatalf("a file named * staged the binary:\n%s", got)
	}
	for _, want := range []string{"\t*\n", "\t--force\n", "\t:(glob)**\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q was not committed as itself:\n%s", want, got)
		}
	}
}

// A symlink is committed as a link and its target is never read.
func TestASymlinkIsCommittedAsALink(t *testing.T) {
	r := isolated(t)
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("\x00binary outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(r.Dir(), "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx(t), time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := tracked(t, r); !strings.HasPrefix(got, "120000 ") {
		t.Fatalf("the link was not committed as a link:\n%s", got)
	}
}

// ⛔ A program writes the area freely, .git included, so nothing it writes
// there may run as rig: not a hook, not an fsmonitor.
func TestNothingAProgramWritesRunsAsRig(t *testing.T) {
	r := isolated(t)
	marker := filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\ntouch " + marker + "\n"
	for _, h := range []string{"pre-commit", "post-commit", "commit-msg"} {
		p := filepath.Join(r.Dir(), ".git", "hooks", h)
		if err := os.WriteFile(p, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mon := filepath.Join(t.TempDir(), "monitor")
	if err := os.WriteFile(mon, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := exec.Command("git", "-C", r.Dir(), "config", "core.fsmonitor", mon)
	if out, err := cfg.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	write(t, r, "a.md", "a\n")
	if res, err := r.Commit(ctx(t), time.Now()); err != nil || res.Commit == "" {
		t.Fatalf("commit: %+v, %v", res, err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a file in the area ran as rig")
	}
}

// Boris's own gitconfig does not reach rig's commits: a signing setup that
// would fail does not stop them.
func TestTheUsersGitConfigIsNotRead(t *testing.T) {
	r := isolated(t)
	home := os.Getenv("HOME")
	// A global excludes file hiding every .md would make rig silently stop
	// committing them, which is the setting that proves the file is unread.
	ignore := filepath.Join(home, "ignore")
	if err := os.WriteFile(ignore, []byte("*.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := "[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = /nonexistent/gpg\n[core]\n\texcludesFile = " + ignore + "\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	write(t, r, "a.md", "a\n")
	if res, err := r.Commit(ctx(t), time.Now()); err != nil || res.Commit == "" {
		t.Fatalf("the user's gitconfig broke rig's commit: %+v, %v", res, err)
	}
	author, _ := r.git(ctx(t), nil, "log", "-1", "--format=%an <%ae>")
	if strings.TrimSpace(string(author)) != "rig <rig@localhost>" {
		t.Fatalf("author %q", author)
	}
}

// MakeDir stays in the area even when a program planted a symlink out.
func TestMakeDirStaysInTheArea(t *testing.T) {
	r := isolated(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(r.Dir(), "programs")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.MakeDir("programs/graft"); err == nil {
		t.Fatal("a directory was made through a symlink out of the area")
	}
	if _, err := os.Stat(filepath.Join(outside, "graft")); !os.IsNotExist(err) {
		t.Fatalf("made outside the area: %v", err)
	}
	d, err := r.MakeDir("docs/sched")
	if err != nil || d != filepath.Join(r.Dir(), "docs", "sched") {
		t.Fatalf("%q, %v", d, err)
	}
}

// The committer commits within one interval, and once more on the way out.
func TestTheCommitterRunsOnItsIntervalAndAtTheEnd(t *testing.T) {
	r := isolated(t)
	run, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	commits := make(chan Result, 10)
	go func() {
		defer close(done)
		r.RunCommitter(run, 50*time.Millisecond, func(res Result, err error) {
			if err != nil {
				t.Errorf("committer: %v", err)
			}
			commits <- res
		})
	}()
	write(t, r, "a.md", "a\n")
	select {
	case res := <-commits:
		if res.Commit == "" {
			t.Fatal("reported a pass that committed nothing")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was committed within the interval")
	}
	stop()
	<-done
}

// A shutdown commits what the interval had not reached yet.
func TestTheCommitterCommitsOnTheWayOut(t *testing.T) {
	r := isolated(t)
	run, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.RunCommitter(run, time.Hour, func(Result, error) {})
	}()
	write(t, r, "b.md", "b\n")
	stop()
	<-done
	if !strings.Contains(tracked(t, r), "b.md") {
		t.Fatal("the last write was not committed on the way out")
	}
}

// Boris, 2026-09-26: never two commits closer than the interval - not on a
// tick, not across a restart, not on the way out.
func TestNoTwoCommitsCloserThanTheInterval(t *testing.T) {
	r := isolated(t)
	write(t, r, "a.md", "a\n")
	if res, err := r.Commit(ctx(t), time.Now()); err != nil || res.Commit == "" {
		t.Fatalf("setup commit: %+v, %v", res, err)
	}
	// A committer starting now is a restart just after that commit.
	run, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.RunCommitter(run, time.Hour, func(res Result, err error) {
			t.Errorf("committed inside the interval: %+v, %v", res, err)
		})
	}()
	write(t, r, "b.md", "b\n")
	time.Sleep(300 * time.Millisecond)
	stop()
	<-done
	if strings.Contains(tracked(t, r), "b.md") {
		t.Fatal("b.md was committed inside the interval")
	}
}

func TestACommitMessageNamesAtMostTwoHundredPaths(t *testing.T) {
	ps := make([]string, maxListed+5)
	for i := range ps {
		ps[i] = "p"
	}
	got := listed(ps)
	if strings.Count(got, "\n") != maxListed || !strings.HasSuffix(got, "and 5 more") {
		t.Fatalf("listed %d lines, ending %q", strings.Count(got, "\n"), got[len(got)-20:])
	}
}
