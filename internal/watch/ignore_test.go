package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/store"
	"github.com/fsnotify/fsnotify"
)

func TestIgnoreConfigValidationAndCopies(t *testing.T) {
	for _, pattern := range []string{"", "[", "a/b", "**", `a\b`, "a\x00b"} {
		if _, err := normalise(Config{Groups: []Group{{ID: "notes", Paths: []string{"/notes"}, Ignore: []string{pattern}}}}); err == nil {
			t.Fatalf("accepted ignore %q", pattern)
		}
	}
	h := newEngineTest(t)
	g := Group{ID: "notes", Paths: []string{h.root}, Ignore: []string{"*.tmp", "generated", "*.tmp"}}
	h.configure(g)
	g.Ignore[0] = "*"
	got := h.w.Config()
	if !reflect.DeepEqual(got.Groups[0].Ignore, []string{"*.tmp", "generated"}) {
		t.Fatalf("ignore = %v", got.Groups[0].Ignore)
	}
	got.Groups[0].Ignore[0] = "*"
	h.restart()
	if !reflect.DeepEqual(h.w.Config().Groups[0].Ignore, []string{"*.tmp", "generated"}) {
		t.Fatal("config alias or restart lost ignores")
	}
}

func TestIgnoresAcrossOverlappingGroupsAndGlobs(t *testing.T) {
	h := newEngineTest(t)
	for _, dir := range []string{"generated", ".git", "node_modules", "build", ".cache"} {
		if err := os.Mkdir(filepath.Join(h.root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		h.write(filepath.Join(dir, "entry"), "a")
	}
	path := filepath.Join(h.root, "generated", "entry")
	h.configure(
		Group{ID: "filtered", Paths: []string{h.root}, Ignore: []string{"generated"}},
		Group{ID: "included", Paths: []string{filepath.Join(h.root, "*")}, Ignore: []string{}},
	)
	for _, op := range []fsnotify.Op{fsnotify.Write, fsnotify.Create, fsnotify.Rename, fsnotify.Remove} {
		h.consume(filepath.Join(h.root, ".git", "entry"), op)
	}
	if len(h.events) != 0 {
		t.Fatalf("ignored events: %+v", h.events)
	}
	h.consume(path, fsnotify.Write)
	if len(h.events) != 1 || h.events[0].WatchID != "included" {
		t.Fatalf("overlap events: %+v", h.events)
	}
	for _, dir := range []string{".git", "node_modules", "build", ".cache"} {
		for _, subscription := range h.w.backend.WatchList() {
			if beneath(subscription, filepath.Join(h.root, dir)) {
				t.Fatalf("ignored subscription %s", subscription)
			}
		}
	}
	// Removing the group that needs generated must prune its subscription.
	h.configure(Group{ID: "filtered", Paths: []string{h.root}, Ignore: []string{"generated"}})
	if got := h.w.Usage(); got.Entries != 1 || got.Watches != 2 {
		t.Fatalf("pruned usage = %+v", got)
	}
}

func TestExplicitIgnoredRootAndMissingRoot(t *testing.T) {
	h := newEngineTest(t)
	h.configure(Group{ID: "git", Paths: []string{filepath.Join(h.root, ".git", "objects")}})
	if got := h.w.Usage(); got.Entries != 0 || got.Watches != 0 {
		t.Fatalf("explicit ignored root = %+v", got)
	}
	root := filepath.Join(h.root, "later", "notes")
	h.configure(Group{ID: "notes", Paths: []string{root}, Ignore: []string{"*.tmp"}})
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	h.consume(filepath.Dir(root), fsnotify.Create)
	path := h.write(filepath.Join("later", "notes", "file"), "a")
	h.consume(path, fsnotify.Create)
	if _, ok := h.w.groups["notes"].Entries[path]; !ok {
		t.Fatal("missing root sentinel did not discover file")
	}
}

func TestRealGitRepoIgnoresStayUnderMaxEntries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	git := func(input string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		cmd.Stdin = strings.NewReader(input)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("", "init", "--quiet")
	for i := range 64 {
		git(fmt.Sprintf("object %d", i), "hash-object", "-w", "--stdin")
	}
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := openState(ctx, filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = state.Close() }()
	events := make(chan Event, 32)
	w, err := New(ctx, state, Options{
		Publish:   func(_ context.Context, e Event) error { events <- e; return nil },
		ReportGap: func(context.Context, Gap) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	if err = w.Configure(ctx, Config{MaxEntries: 4, MaxWatches: 8, Groups: []Group{{ID: "repo", Paths: []string{root}, Ignore: []string{}, DebounceMS: 50}}}); err != nil {
		t.Fatal(err)
	}
	if got := w.Usage(); got.Entries != 2 || got.Watches != 2 {
		t.Fatalf("git repo usage = %+v", got)
	}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	git("one more ignored object", "hash-object", "-w", "--stdin")
	select {
	case e := <-events:
		t.Fatalf("git object triggered event: %+v", e)
	case <-time.After(150 * time.Millisecond):
	}
	if err := os.WriteFile(file, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case e := <-events:
			if e.Kind == "rig.watch.repo.fired" {
				t.Logf("real git repo: %d entries, %d subscriptions, fired count %d", w.Usage().Entries, w.Usage().Watches, e.Count)
				return
			}
		case <-ctx.Done():
			t.Fatal("real filesystem change did not fire")
		}
	}
}

func TestIgnoreSchemaUpgradesAndRefusesOlderReaders(t *testing.T) {
	for _, version := range []uint32{1, 2} {
		t.Run(strconv.FormatUint(uint64(version), 10), func(t *testing.T) {
			h := newEngineTest(t)
			path := h.write("note", "a")
			h.configure()
			h.consume(path, fsnotify.Write)
			due := h.w.groups["notes"].Pending.DueNS
			if _, err := h.state.db.ExecContext(context.Background(), fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
				t.Fatal(err)
			}
			h.restart()
			if h.w.groups["notes"].Pending.DueNS != due {
				t.Fatal("schema upgrade changed pending deadline")
			}
			db, err := store.OpenDB(context.Background(), h.state.Path(), store.Schema{What: "old watch state", Version: version})
			if db != nil {
				_ = db.Close()
			}
			var future *store.FutureSchemaError
			if !errors.As(err, &future) {
				t.Fatalf("old reader could erase ignores: %v", err)
			}
		})
	}
}

func TestInitialConfigDropsUnavailableOldRootBeforeScanning(t *testing.T) {
	h := newEngineTest(t)
	root := filepath.Join(h.root, "nested")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	h.configure(Group{ID: "notes", Paths: []string{root}})
	if err := h.w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(h.root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), h.root); err != nil {
		t.Fatal(err)
	}
	empty := Config{}
	w, err := New(context.Background(), h.state, Options{
		InitialConfig: &empty,
		Publish:       func(context.Context, Event) error { return nil },
		ReportGap:     func(context.Context, Gap) error { return nil },
	})
	if err != nil {
		t.Fatalf("removed watch still scanned its obsolete symlink root: %v", err)
	}
	defer func() { _ = w.Close() }()
	if len(w.Config().Groups) != 0 || w.Usage().Watches != 0 {
		t.Fatal("removed watch retained its subscriptions")
	}
}
