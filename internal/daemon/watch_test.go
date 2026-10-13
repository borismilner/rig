package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/backup"
	"github.com/borismilner/rig/internal/config"
	"github.com/borismilner/rig/internal/instance"
	"github.com/borismilner/rig/internal/paths"
	"github.com/borismilner/rig/internal/record"
	"github.com/borismilner/rig/internal/watch"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

func TestWatchSurvivesDaemonRestartAndBackupRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "rigwatch")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("RIG_ROOT", "")
	root := filepath.Join(dir, "notes")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	groups, _ := json.Marshal([]watch.Group{{ID: "notes", Paths: []string{root}, DebounceMS: 1200}})
	settings := config.Load(config.MustRigSchema(), config.Sources{Flags: map[string]string{"watch.groups": string(groups)}})
	lock, err := instance.Acquire(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()

	start := func(epoch uint64) (*Daemon, *client.Client, func()) {
		t.Helper()
		d, err := New(Config{Wire: "v1", Estate: "watchwire", Epoch: epoch, Lock: lock, Settings: settings})
		if err != nil {
			t.Fatal(err)
		}
		sock := filepath.Join(dir, "sock")
		l, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- d.Serve(runCtx, l) }()
		c, err := client.Dial(sock)
		if err != nil {
			t.Fatal(err)
		}
		ended := false
		closeRun := func() {
			if ended {
				return
			}
			ended = true
			_ = c.Close()
			stop()
			if err := <-done; err != nil {
				t.Error(err)
			}
			if err := d.Close(); err != nil {
				t.Error(err)
			}
			_ = os.Remove(sock)
		}
		t.Cleanup(closeRun)
		return d, c, closeRun
	}
	wait := func(c *client.Client, kind string, after uint64, ms uint32) *registryv1.EventsWaitResponse {
		t.Helper()
		var got registryv1.EventsWaitResponse
		if err := c.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{kind}, After: after, TimeoutMs: ms}, &got); err != nil {
			t.Fatal(err)
		}
		return &got
	}
	d, c, stop := start(1)
	if backup.WatchMember != watch.DBName {
		t.Fatal("backup and watcher disagree on the restored filename")
	}
	ns := storeList(t, filepath.Join(dir, "sock"))["watch"]
	if ns == nil || ns.GetSchemaVersion() != uint64(watch.SchemaVersion) || ns.GetEphemeral() {
		t.Fatalf("watch inventory: %v", ns)
	}
	if err := os.WriteFile(filepath.Join(root, "note"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := wait(c, "rig.watch.notes.changed", 0, 3000)
	if len(got.GetEvents()) == 0 {
		t.Fatal("real file write produced no changed event")
	}
	var changed watch.Event
	if err := json.Unmarshal([]byte(got.GetEvents()[0].GetPayloadJson()), &changed); err != nil {
		t.Fatal(err)
	}
	// Snapshot while the debounce is pending, including any uncheckpointed WAL.
	archive, err := d.createBackup(ctx, d.records, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stop()
	_, c, stop = start(2)
	fired := wait(c, "rig.watch.notes.fired", 0, 4000)
	if len(fired.GetEvents()) != 1 {
		t.Fatalf("restart fires: %v", fired)
	}
	var event watch.Event
	if err := json.Unmarshal([]byte(fired.GetEvents()[0].GetPayloadJson()), &event); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(changed.ID, event.ID+":") || event.FirstNS != changed.FirstNS {
		t.Fatalf("restart replaced pending batch: before %+v, after %+v", changed, event)
	}
	if again := wait(c, "rig.watch.notes.fired", fired.GetLatest(), 150); len(again.GetEvents()) != 0 {
		t.Fatalf("duplicate fire after acknowledgement: %v", again)
	}
	stop()

	// Restore into a second isolated state tree, then open it with the daemon.
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "restored"))
	stateDir, err := paths.EstateStateDir("watchwire")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Restore(backup.RestoreRequest{Archive: archive.GetPath(), Dir: stateDir, SchemaCeiling: record.SchemaVersion, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, c, stop = start(3)
	defer stop()
	fired = wait(c, "rig.watch.notes.fired", 0, 3000)
	if len(fired.GetEvents()) != 1 {
		t.Fatalf("restored pending watch did not fire: %v", fired)
	}
	if err := json.Unmarshal([]byte(fired.GetEvents()[0].GetPayloadJson()), &event); err != nil || !strings.HasPrefix(changed.ID, event.ID+":") {
		t.Fatalf("restored fire = %+v, %v", event, err)
	}
}

func TestWatchResolvedSettingsRejectInvalidGroups(t *testing.T) {
	for _, raw := range []string{
		`[{"id":"a","paths":["relative"]}]`,
		`[{"id":"a","paths":["/tmp"],"ignore":["**"]}]`,
		`[{"id":"a","paths":["/tmp"]},{"id":"a","paths":["/tmp"]}]`,
	} {
		d := &Daemon{settings: config.Load(config.MustRigSchema(), config.Sources{Flags: map[string]string{"watch.groups": raw}})}
		if _, err := d.resolvedWatch(); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestWatchFailureReturnsAnErrorFromServe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "rigwf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("RIG_ROOT", "")
	lock, err := instance.Acquire(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	d, err := New(Config{Wire: "v1", Lock: lock, Estate: "watchfail"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	// No group is configured, so New opened no watch; give it one whose
	// publish always fails.
	if d.watchState, err = watch.OpenState(ctx, "watchfail"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "notes")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	want := errors.New("publication unavailable")
	cfg := watch.Config{Groups: []watch.Group{{ID: "notes", Paths: []string{root}}}}
	d.watcher, err = watch.New(ctx, d.watchState, watch.Options{
		InitialConfig: &cfg,
		Publish:       func(context.Context, watch.Event) error { return want },
		ReportGap:     func(context.Context, watch.Gap) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", filepath.Join(dir, "sock"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Serve(ctx, l) }()
	if err := os.WriteFile(filepath.Join(root, "note"), []byte("change"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, want) {
		t.Fatalf("supervisor sees %v, want the watch failure", err)
	}
}
