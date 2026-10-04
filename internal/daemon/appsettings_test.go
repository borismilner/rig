package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

const appSchema = `{"type":"object","additionalProperties":false,"properties":{
	"cap":{"type":"integer","minimum":1,"default":3,"x-rig-apply":"live","description":"contacts a day"}}}`

// ⛔ A PROGRAM'S SETTINGS ARE rig's TO SERVE: declared at hello, read and
// changed through config.get and config.set with program set, the change
// kept in apps/<id>.toml and posted as config.changed naming the program, so
// the program waits for it rather than polling.
func TestAProgramsSettingsAreReadChangedAndPosted(t *testing.T) {
	apps := t.TempDir()
	sock, _ := upDaemonWith(t, func(c *Config) { c.AppsDir = apps })

	prog := dial(t, sock)
	decl := testDeclaration("tally")
	decl.SettingsSchema = appSchema
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := prog.Hello(ctx, decl); err != nil {
		t.Fatal(err)
	}

	boris := dial(t, sock)
	cursor := waitOn(t, boris, 0, 1, "config.changed").GetLatest()
	var got registryv1.ConfigGetResponse
	if err := boris.Call(recordCtx(t), "rig.config.get", &registryv1.ConfigGetRequest{Program: "tally"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.GetValues()) != 1 || got.GetValues()[0].GetWinner().GetLayer() != "program-default" || got.GetValues()[0].GetWinner().GetValueJson() != "3" {
		t.Fatalf("before a change: %+v", got.GetValues())
	}

	err := boris.Call(recordCtx(t), "rig.config.set", &registryv1.ConfigSetRequest{Program: "tally", ValuesJson: map[string]string{"cap": "0"}}, &registryv1.ConfigSetResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "cap under its minimum")
	var set registryv1.ConfigSetResponse
	if err := boris.Call(recordCtx(t), "rig.config.set", &registryv1.ConfigSetRequest{Program: "tally", ValuesJson: map[string]string{"cap": "5"}}, &set); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(apps, "tally.toml")
	if set.GetOutcome()["cap"] != "applied" || set.GetSnapshotPath() != file {
		t.Fatalf("set answered %+v", &set)
	}
	if body, err := os.ReadFile(file); err != nil || !strings.Contains(string(body), "cap = 5") {
		t.Fatalf("%s holds %q (%v)", file, body, err)
	}

	var heard registryv1.ConfigChanged
	evs := waitOn(t, prog, cursor, 5000, "config.changed").GetEvents()
	if len(evs) != 1 || json.Unmarshal([]byte(evs[0].GetPayloadJson()), &heard) != nil ||
		heard.GetProgram() != "tally" || strings.Join(heard.GetKeys(), ",") != "cap" {
		t.Fatalf("config.changed: %v", evs)
	}

	err = boris.Call(recordCtx(t), "rig.config.get", &registryv1.ConfigGetRequest{Program: "nobody"}, &registryv1.ConfigGetResponse{})
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "a program that declared no settings")

	bad := testDeclaration("broken")
	bad.SettingsSchema = `{"type":"object","properties":{"x":{"type":"integer"}}}`
	_, err = dial(t, sock).Hello(ctx, bad)
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a schema leaf with no default")
}
