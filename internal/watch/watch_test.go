package watch

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
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

func TestStateRoundTripKeepsPendingAndSnapshotTogether(t *testing.T) {
	ctx := context.Background()
	state, err := openState(ctx, filepath.Join(t.TempDir(), "watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })

	config, err := normalize(Config{Groups: []Group{{ID: "inbox", Paths: []string{"/tmp/inbox"}}}})
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
