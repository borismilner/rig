package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/store"
	"github.com/fsnotify/fsnotify"
)

func TestReadConfigNormalizesDefaultsAndOrder(t *testing.T) {
	got, err := ReadConfig(strings.NewReader(`{
		"watches": [
			{"id":"zeta","paths":["/tmp/z","/tmp/z"]},
			{"id":"alpha","paths":["/tmp/a"]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxWatches != DefaultMaxWatches || got.MaxEntries != DefaultMaxEntries {
		t.Fatalf("limits = %+v", got)
	}
	if got.Groups[0].ID != "alpha" || got.Groups[1].ID != "zeta" {
		t.Fatalf("groups = %+v", got.Groups)
	}
	if got.Groups[1].DebounceMS != DefaultDebounce.Milliseconds() {
		t.Fatalf("debounce_ms = %d", got.Groups[1].DebounceMS)
	}
	if len(got.Groups[1].Paths) != 1 {
		t.Fatalf("duplicate path survived: %+v", got.Groups[1].Paths)
	}
}

type fakeBackend struct {
	mu      sync.Mutex
	paths   map[string]bool
	events  chan fsnotify.Event
	errors  chan error
	closed  bool
	failAdd bool
}

func fakeSource() *fakeBackend {
	return &fakeBackend{paths: map[string]bool{}, events: make(chan fsnotify.Event, 32), errors: make(chan error, 4)}
}

func (b *fakeBackend) Add(path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failAdd {
		return errors.New("registration failed")
	}
	b.paths[path] = true
	return nil
}

func (b *fakeBackend) Remove(path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.paths, path)
	return nil
}

func (b *fakeBackend) WatchList() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []string{}
	for p := range b.paths {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}
func (b *fakeBackend) Events() <-chan fsnotify.Event { return b.events }
func (b *fakeBackend) Errors() <-chan error          { return b.errors }
func (b *fakeBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.paths = map[string]bool{}
	return nil
}

type engineTest struct {
	t           *testing.T
	w           *Watcher
	state       *State
	now         atomic.Int64
	root        string
	mu          sync.Mutex
	events      []Event
	gaps        []Gap
	failPublish bool
}

func newEngineTest(t *testing.T) *engineTest {
	t.Helper()
	h := &engineTest{t: t, root: filepath.Join(t.TempDir(), "files")}
	if err := os.Mkdir(h.root, 0o700); err != nil {
		t.Fatal(err)
	}
	h.now.Store(time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC).UnixNano())
	var err error
	h.state, err = openState(context.Background(), filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	h.open()
	t.Cleanup(func() {
		if h.w != nil {
			_ = h.w.Close()
		}
		_ = h.state.Close()
	})
	return h
}

func (h *engineTest) open() {
	h.t.Helper()
	var err error
	h.w, err = New(context.Background(), h.state, Options{
		NewBackend: func() (Backend, error) { return fakeSource(), nil },
		Now:        func() time.Time { return time.Unix(0, h.now.Load()) },
		Publish: func(_ context.Context, e Event) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.events = append(h.events, e)
			if h.failPublish {
				return errors.New("bus failed")
			}
			return nil
		},
		ReportGap: func(_ context.Context, g Gap) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.gaps = append(h.gaps, g)
			return nil
		},
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

func (h *engineTest) configure(groups ...Group) {
	h.t.Helper()
	if len(groups) == 0 {
		groups = []Group{{ID: "notes", Paths: []string{h.root}, DebounceMS: 1000}}
	}
	if err := h.w.Configure(context.Background(), Config{Groups: groups}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *engineTest) write(name, body string) string {
	h.t.Helper()
	path := filepath.Join(h.root, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		h.t.Fatal(err)
	}
	return path
}

func (h *engineTest) consume(path string, op fsnotify.Op) {
	h.t.Helper()
	if err := h.w.consume(context.Background(), h.w.backend, fsnotify.Event{Name: path, Op: op}, true); err != nil {
		h.t.Fatal(err)
	}
}

func (h *engineTest) fire() {
	h.t.Helper()
	if err := h.w.fireDue(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}
func (h *engineTest) advance(d time.Duration) { h.now.Add(int64(d)) }
func (h *engineTest) restart() {
	h.t.Helper()
	_ = h.w.Close()
	path := h.state.Path()
	_ = h.state.Close()
	var err error
	h.state, err = openState(context.Background(), path)
	if err != nil {
		h.t.Fatal(err)
	}
	h.open()
}

func (h *engineTest) fired() []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Event
	for _, e := range h.events {
		if strings.HasSuffix(e.Kind, ".fired") {
			out = append(out, e)
		}
	}
	return out
}

func TestFreshConfigureAndDebounceAfterLastEvent(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	if len(h.events) != 0 {
		t.Fatalf("initial baseline published %+v", h.events)
	}
	if u := h.w.Usage(); u.Watches != 2 || u.Entries != 2 {
		t.Fatalf("usage %+v", u)
	}
	h.consume(path, fsnotify.Write)
	h.advance(800 * time.Millisecond)
	h.consume(path, fsnotify.Chmod)
	h.advance(200 * time.Millisecond)
	h.fire()
	if len(h.fired()) != 0 {
		t.Fatal("fired before quiet period")
	}
	h.advance(800 * time.Millisecond)
	h.fire()
	h.fire()
	got := h.fired()
	if len(got) != 1 || got[0].Count != 2 || got[0].ID == "" {
		t.Fatalf("fired %+v", got)
	}
	_, states, err := h.state.load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if states["notes"].Pending != nil {
		t.Fatal("acknowledged batch remains pending")
	}
}

func TestRestartPreservesPendingDeadlineAndOverdueFire(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.consume(path, fsnotify.Write)
	p := *h.w.groups["notes"].Pending
	h.advance(600 * time.Millisecond)
	h.restart()
	if got := h.w.groups["notes"].Pending; !reflect.DeepEqual(got, &p) {
		t.Fatalf("restart changed pending: %+v -> %+v", p, got)
	}
	h.advance(2 * time.Second)
	h.restart()
	h.fire()
	got := h.fired()
	if len(got) != 1 || got[0].ID != p.ID {
		t.Fatalf("overdue fired %+v", got)
	}
	if len(h.gaps) != 2 || h.gaps[0].Reason != "restart" {
		t.Fatalf("coverage %+v", h.gaps)
	}
}

func TestFailedPublisherRetainsFireAcrossRestart(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.consume(path, fsnotify.Write)
	h.advance(time.Second)
	h.failPublish = true
	if err := h.w.fireDue(context.Background()); err == nil {
		t.Fatal("publication failure was swallowed")
	}
	h.failPublish = false
	h.restart()
	h.fire()
	got := h.fired()
	if len(got) != 2 || got[0].ID != got[1].ID {
		t.Fatalf("retry changed delivery identity: %+v", got)
	}
}

func TestFailedChangedPublicationStillLeavesDurableFire(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.failPublish = true
	if err := h.w.consume(context.Background(), h.w.backend, fsnotify.Event{Name: path, Op: fsnotify.Write}, true); err == nil {
		t.Fatal("expected publish failure")
	}
	h.failPublish = false
	h.advance(time.Second)
	h.restart()
	h.fire()
	if len(h.fired()) != 1 {
		t.Fatal("failed changed publication lost pending fire")
	}
}

func TestRejectedReplacementKeepsConfigBackendAndState(t *testing.T) {
	h := newEngineTest(t)
	h.configure()
	before := h.w.Config()
	b := h.w.backend
	h.w.options.NewBackend = func() (Backend, error) { f := fakeSource(); f.failAdd = true; return f, nil }
	if err := h.w.Declare(context.Background(), Group{ID: "more", Paths: []string{h.root + "/missing"}}); err == nil {
		t.Fatal("failed backend was accepted")
	}
	if h.w.backend != b || !reflect.DeepEqual(h.w.Config(), before) {
		t.Fatal("failed replacement changed active config")
	}
	saved, _, err := h.state.load(context.Background())
	if err != nil || !reflect.DeepEqual(saved, before) {
		t.Fatalf("saved config %+v: %v", saved, err)
	}
}

func TestOverflowDeclaresGapReconcilesAndInvalidatesUnchangedGroup(t *testing.T) {
	h := newEngineTest(t)
	h.configure()
	dir := filepath.Join(h.root, "new")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	h.write("new/child", "arrived before subscription")
	if err := h.w.backendError(context.Background(), h.w.backend, fsnotify.ErrEventOverflow, true); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(h.w.backend.WatchList(), dir) {
		t.Fatal("new directory is unwatched")
	}
	h.advance(time.Second)
	h.fire()
	if got := h.fired(); len(got) != 1 || !got[0].Gap {
		t.Fatalf("overflow fire %+v", got)
	}
	if err := h.w.backendError(context.Background(), h.w.backend, fsnotify.ErrEventOverflow, true); err != nil {
		t.Fatal(err)
	}
	h.advance(time.Second)
	h.fire()
	if len(h.fired()) != 2 {
		t.Fatal("unchanged overflow was silent")
	}
}

func TestRunReturnsForClosedChannelsAndClosesBackend(t *testing.T) {
	for _, channel := range []string{"events", "errors"} {
		t.Run(channel, func(t *testing.T) {
			h := newEngineTest(t)
			h.configure()
			b := h.w.backend.(*fakeBackend)
			if channel == "events" {
				close(b.events)
			} else {
				close(b.errors)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := h.w.Run(ctx)
			if err == nil || !strings.Contains(err.Error(), "channel closed") {
				t.Fatalf("Run = %v", err)
			}
			if !b.closed {
				t.Fatal("backend leaked")
			}
		})
	}
}

func TestRootRecreationAndPopulatedDirectoryArrival(t *testing.T) {
	h := newEngineTest(t)
	h.configure()
	if err := os.Remove(h.root); err != nil {
		t.Fatal(err)
	}
	h.consume(h.root, fsnotify.Remove)
	if err := os.MkdirAll(filepath.Join(h.root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := h.write("nested/note", "ready before parent event")
	h.consume(h.root, fsnotify.Create)
	if _, ok := h.w.groups["notes"].Entries[path]; !ok {
		t.Fatal("populated subtree missed")
	}
	if !slices.Contains(h.w.backend.WatchList(), filepath.Dir(path)) {
		t.Fatal("new subtree unwatched")
	}
}

func TestMatchingOverlapFilesGlobsAndSelfExclusion(t *testing.T) {
	h := newEngineTest(t)
	note := h.write("a.md", "a")
	other := h.write("b.txt", "b")
	h.configure(Group{ID: "all", Paths: []string{h.root}}, Group{ID: "markdown", Paths: []string{h.root + "/*.md"}}, Group{ID: "one", Paths: []string{note}})
	h.consume(other, fsnotify.Write)
	h.consume(note, fsnotify.Write)
	counts := map[string]int{}
	for _, e := range h.events {
		counts[e.WatchID]++
	}
	if !reflect.DeepEqual(counts, map[string]int{"all": 2, "markdown": 1, "one": 1}) {
		t.Fatalf("matching %+v", counts)
	}
	before := len(h.events)
	h.consume(h.state.Path()+"-wal", fsnotify.Write)
	if len(h.events) != before {
		t.Fatal("own persistence generated a change")
	}
}

func TestCallbackRunsAfterDurableChangeAndIsSerialized(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.w.options.Publish = func(ctx context.Context, _ Event) error {
		if h.w.mu.TryLock() {
			h.w.mu.Unlock()
			return errors.New("callback not serialised")
		}
		_, states, err := h.state.load(ctx)
		if err != nil {
			return err
		}
		if states["notes"].Pending == nil {
			return errors.New("publication preceded persistence")
		}
		return nil
	}
	h.consume(path, fsnotify.Write)
}

func TestFailedSnapshotTransactionDoesNotAdvanceBaseline(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	before := h.w.groups["notes"].Entries[path]
	_, err := h.state.db.Exec(`CREATE TRIGGER fail_snapshot BEFORE UPDATE ON snapshots BEGIN SELECT RAISE(ABORT,'test disk failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	h.write("note", "different size")
	err = h.w.consume(context.Background(), h.w.backend, fsnotify.Event{Name: path, Op: fsnotify.Write}, true)
	if err == nil {
		t.Fatal("failed transaction was accepted")
	}
	if got := h.w.groups["notes"]; got.Pending != nil || got.Entries[path] != before {
		t.Fatal("failed transaction mutated memory")
	}
	if _, err = h.state.db.Exec(`DROP TRIGGER fail_snapshot`); err != nil {
		t.Fatal(err)
	}
	h.restart()
	h.advance(time.Second)
	h.fire()
	if len(h.fired()) != 1 {
		t.Fatal("restart lost uncommitted filesystem change")
	}
}

func TestReplacementCountsBothLiveBackendsAgainstCeiling(t *testing.T) {
	h := newEngineTest(t)
	h.configure()
	c := h.w.Config()
	c.MaxWatches = 2
	c.Groups[0].Description = "changes configuration"
	err := h.w.Configure(context.Background(), c)
	var limit *LimitError
	if !errors.As(err, &limit) {
		t.Fatalf("replacement allocated over ceiling: %v", err)
	}
}

func TestReadConfigRejectsNullAndOversizeTrailingData(t *testing.T) {
	for _, data := range []string{"null", `{}` + strings.Repeat(" ", 1<<20) + `{}`} {
		if _, err := ReadConfig(strings.NewReader(data)); err == nil {
			t.Fatalf("accepted invalid config (%d bytes)", len(data))
		}
	}
}

func TestFiredRetryKeepsOldBatchSeparateFromLaterChanges(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.consume(path, fsnotify.Write)
	h.advance(time.Second)
	h.failPublish = true
	if err := h.w.fireDue(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	first := h.fired()[0]
	h.failPublish = false
	h.write("note", "another version")
	h.restart()
	h.advance(time.Second)
	h.fire()
	got := h.fired()
	if len(got) != 3 || got[1] != first || got[2].ID == first.ID {
		t.Fatalf("retry merged a newer change into an already published ID: %+v", got)
	}
}

func TestNativeEventsAtomicReplaceAndDebouncedFire(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := t.TempDir()
	path := filepath.Join(root, "note")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := openState(ctx, filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = state.Close() }()
	accepted := make(chan Event, 100)
	w, err := New(ctx, state, Options{Publish: func(_ context.Context, e Event) error { accepted <- e; return nil }, ReportGap: func(context.Context, Gap) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Configure(ctx, Config{Groups: []Group{{ID: "file", Paths: []string{path}, DebounceMS: 40}}}); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- w.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	for i := range 2 {
		tmp := filepath.Join(root, "replacement")
		if err = os.WriteFile(tmp, []byte(fmt.Sprintf("version %d", i)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
		for {
			select {
			case e := <-accepted:
				if e.Kind == "rig.watch.file.fired" {
					goto fired
				}
			case <-ctx.Done():
				t.Fatal("native replacement did not fire")
			}
		}
	fired:
	}
}

func TestSuccessfulPublishFailedAckRetriesIdenticalDelivery(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.consume(path, fsnotify.Write)
	h.advance(time.Second)
	base := h.w.options.Publish
	h.w.options.Publish = func(ctx context.Context, e Event) error {
		if err := base(ctx, e); err != nil {
			return err
		}
		if strings.HasSuffix(e.Kind, ".fired") {
			_, err := h.state.db.Exec(`CREATE TRIGGER fail_ack BEFORE UPDATE ON watches BEGIN SELECT RAISE(ABORT,'test ack failure'); END`)
			return err
		}
		return nil
	}
	if err := h.w.fireDue(context.Background()); err == nil {
		t.Fatal("expected failed acknowledgement")
	}
	first := h.fired()[0]
	if _, err := h.state.db.Exec(`DROP TRIGGER fail_ack`); err != nil {
		t.Fatal(err)
	}
	h.write("note", "new offline change")
	h.restart()
	h.advance(time.Second)
	h.fire()
	got := h.fired()
	if len(got) != 3 || got[1] != first || got[2].ID == first.ID {
		t.Fatalf("delivery after failed ack: %+v", got)
	}
}

func TestOrdinaryEventsCannotExceedSnapshotLimit(t *testing.T) {
	h := newEngineTest(t)
	if err := h.w.Configure(context.Background(), Config{Groups: []Group{{ID: "notes", Paths: []string{h.root}}}, MaxEntries: 1}); err != nil {
		t.Fatal(err)
	}
	path := h.write("note", "appears before create is consumed")
	err := h.w.consume(context.Background(), h.w.backend, fsnotify.Event{Name: path, Op: fsnotify.Write}, true)
	if err == nil || !strings.Contains(err.Error(), "max_entries") {
		t.Fatalf("unbounded ordinary event: %v", err)
	}
	if h.w.Usage().Entries != 1 {
		t.Fatal("refused event changed baseline")
	}
}

func TestDeliverySchemaRefusesFoundationReader(t *testing.T) {
	h := newEngineTest(t)
	db, err := store.OpenDB(context.Background(), h.state.Path(), store.Schema{What: "old watch state", Version: 1})
	if db != nil {
		_ = db.Close()
	}
	var future *store.FutureSchemaError
	if !errors.As(err, &future) {
		t.Fatalf("old reader could erase delivery state: %v", err)
	}
}

func TestDeclareRemoveAndReturnedConfigIsolation(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	copied := h.w.Config()
	copied.Groups[0].Paths[0] = "/changed-by-caller"
	if h.w.Config().Groups[0].Paths[0] != h.root {
		t.Fatal("Config exposed mutable paths")
	}
	if err := h.w.Declare(context.Background(), Group{ID: "one", Paths: []string{path}, DebounceMS: 2000}); err != nil {
		t.Fatal(err)
	}
	h.consume(path, fsnotify.Write)
	if len(h.w.groups) != 2 {
		t.Fatal("Declare replaced unrelated group")
	}
	c := h.w.Config()
	c.Groups = nil
	if err := h.w.Configure(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	h.restart()
	h.advance(time.Hour)
	h.fire()
	if h.w.Usage().Watches != 0 || len(h.fired()) != 0 {
		t.Fatal("explicit removal retained pending work")
	}
}

func TestSymlinkBoundaryAndMissingRoot(t *testing.T) {
	h := newEngineTest(t)
	outside := t.TempDir()
	link := filepath.Join(h.root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	h.configure()
	if slices.Contains(h.w.backend.WatchList(), link) || slices.Contains(h.w.backend.WatchList(), outside) {
		t.Fatal("followed directory symlink")
	}
	if err := h.w.Declare(context.Background(), Group{ID: "escape", Paths: []string{link + "/child"}}); err == nil {
		t.Fatal("accepted symlink ancestor")
	}
	missing := filepath.Join(h.root, "missing", "nested")
	if err := h.w.Declare(context.Background(), Group{ID: "future", Paths: []string{missing}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(missing, 0o700); err != nil {
		t.Fatal(err)
	}
	h.consume(filepath.Dir(missing), fsnotify.Create)
	if !slices.Contains(h.w.backend.WatchList(), missing) {
		t.Fatal("missing root did not become watched")
	}
}

func TestFailedRescanRetainsBaselineAndSubscriptions(t *testing.T) {
	h := newEngineTest(t)
	h.configure()
	before := h.w.Usage()
	if err := os.Mkdir(filepath.Join(h.root, "new"), 0o700); err != nil {
		t.Fatal(err)
	}
	h.w.backend.(*fakeBackend).failAdd = true
	if err := h.w.Rescan(context.Background()); err == nil {
		t.Fatal("expected failed registration")
	}
	if got := h.w.Usage(); got != before {
		t.Fatalf("failed rescan changed state: %+v -> %+v", before, got)
	}
}

func TestRunCancellationPreservesPendingAndWakeFires(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.consume(path, fsnotify.Write)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.w.Run(ctx) }()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h.restart()
	h.advance(time.Second)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	fired := make(chan struct{}, 1)
	base := h.w.options.Publish
	h.w.options.Publish = func(ctx context.Context, e Event) error {
		err := base(ctx, e)
		if strings.HasSuffix(e.Kind, ".fired") {
			fired <- struct{}{}
		}
		return err
	}
	go func() { done <- h.w.Run(ctx) }()
	h.w.Wake()
	select {
	case <-fired:
	case <-ctx.Done():
		t.Fatal("wake did not fire overdue pending")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestStateRoundTripKeepsPendingAndSnapshotTogether(t *testing.T) {
	ctx := context.Background()
	state, err := openState(ctx, filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })

	config, err := normalise(Config{Groups: []Group{{ID: "inbox", Paths: []string{"/tmp/inbox"}}}})
	if err != nil {
		t.Fatal(err)
	}
	group := &groupState{
		Group:    config.Groups[0],
		Sequence: 7,
		Pending:  &pending{FirstNS: 10, LastNS: 20, DueNS: 30, Count: 2, Gap: true},
		Entries:  map[string]stamp{"/tmp/inbox/a": {Size: 4, Modified: 5, Mode: 0o644}},
	}
	entry := group.Entries["/tmp/inbox/a"]
	if err := state.save(ctx, &config, []update{{
		state:  group,
		deltas: []delta{{path: "/tmp/inbox/a", value: &entry}},
	}}, nil); err != nil {
		t.Fatal(err)
	}

	loadedConfig, states, err := state.load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := states["inbox"]
	if len(loadedConfig.Groups) != 1 || got == nil || got.Sequence != 7 || got.Pending == nil {
		t.Fatalf("round trip = config=%+v state=%+v", loadedConfig, got)
	}
	if got.Pending.DueNS != 30 || !got.Pending.Gap {
		t.Fatalf("pending = %+v", got.Pending)
	}
	if entry := got.Entries["/tmp/inbox/a"]; entry.Size != 4 || entry.Modified != 5 {
		t.Fatalf("snapshot entry = %+v", entry)
	}
}

func TestClockMovingBackwardsDoesNotMakePendingUnreadable(t *testing.T) {
	h := newEngineTest(t)
	path := h.write("note", "a")
	h.configure()
	h.consume(path, fsnotify.Write)
	h.advance(-time.Hour)
	h.consume(path, fsnotify.Write)
	h.restart()
	h.advance(time.Second)
	h.fire()
	if got := h.fired(); len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("clock adjustment lost pending batch: %+v", got)
	}
}
