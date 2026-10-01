package daemon

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borismilner/rig/internal/config"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// plan/47 acceptance 3 and 4 over the wire: a set applies live in the same
// process, a set with one bad value applies nothing and says why, every
// resolution is on disk, and a change is published on the bus.
func TestASettingChangesLiveAndIsRefusedWhole(t *testing.T) {
	lv := new(slog.LevelVar)
	snap := filepath.Join(t.TempDir(), "config", "resolved.toml")
	sock, _ := upDaemonWith(t, func(c *Config) {
		c.Settings = config.Load(config.MustRigSchema(), config.Sources{Environ: []string{"RIG_LOG_LEVEL=debug"}})
		c.LogLevel, c.SnapshotPath = lv, snap
	})
	ctx := ctx5(t)
	term := dial(t, sock)
	if lv.Level() != slog.LevelDebug {
		t.Fatalf("the level at start is %s, not the env layer's debug", lv.Level())
	}
	if b, err := os.ReadFile(snap); err != nil || !strings.Contains(string(b), "# log.level from env\nlog.level = \"debug\"") {
		t.Fatalf("no snapshot at start: %s %v", b, err)
	}

	var got registryv1.ConfigGetResponse
	if err := term.Call(ctx, "rig.config.get", &registryv1.ConfigGetRequest{Prefix: "log.level"}, &got); err != nil {
		t.Fatal(err)
	}
	v := got.GetValues()
	if len(v) != 1 || v[0].GetWinner().GetLayer() != "env" || v[0].GetWinner().GetValueJson() != `"debug"` ||
		len(v[0].GetLosers()) != 1 || v[0].GetLosers()[0].GetLayer() != "default" || got.GetSnapshotPath() != snap {
		t.Fatalf("origin of log.level: %v", &got)
	}
	err := term.Call(ctx, "rig.config.get", &registryv1.ConfigGetRequest{Prefix: "log.levle"}, &registryv1.ConfigGetResponse{})
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "a key nothing declares")

	var first registryv1.EventsWaitResponse
	_ = term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{"config.*"}, TimeoutMs: 1}, &first)

	err = term.Call(ctx, "rig.config.set", &registryv1.ConfigSetRequest{
		ValuesJson: map[string]string{"log.level": `"loud"`, "display.name": `"x"`},
	}, &registryv1.ConfigSetResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a set with one bad value")
	if err == nil || !strings.Contains(err.Error(), "log.level") {
		t.Fatalf("the refusal does not name the key: %v", err)
	}
	if lv.Level() != slog.LevelDebug {
		t.Fatal("a refused set moved the level")
	}

	var set registryv1.ConfigSetResponse
	if err := term.Call(ctx, "rig.config.set", &registryv1.ConfigSetRequest{
		ValuesJson: map[string]string{"log.level": `"warn"`},
	}, &set); err != nil {
		t.Fatal(err)
	}
	if set.GetOutcome()["log.level"] != "applied" || lv.Level() != slog.LevelWarn {
		t.Fatalf("set answered %v and the level is %s", &set, lv.Level())
	}
	if b, _ := os.ReadFile(snap); !strings.Contains(string(b), "# log.level from runtime\nlog.level = \"warn\"") {
		t.Fatalf("the snapshot was not rewritten:\n%s", b)
	}
	var changed registryv1.EventsWaitResponse
	if err := term.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{
		After: first.GetLatest(), Epoch: first.GetEpoch(), Kinds: []string{"config.*"}, TimeoutMs: 2000,
	}, &changed); err != nil {
		t.Fatal(err)
	}
	if evs := changed.GetEvents(); len(evs) != 1 || evs[0].GetKind() != "config.changed" ||
		evs[0].GetPayloadJson() != `{"keys":["log.level"]}` {
		t.Fatalf("published %v", &changed)
	}
}
