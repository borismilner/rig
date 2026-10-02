package daemon

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/observe"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// plan/49 decision 2 and 8, and §44's acceptance clause 3: THE LEAK TEST.

const (
	leakArg    = "arg-s3cr3t-7f1c"
	leakResult = "result-s3cr3t-9e2d"
)

// vault registers id with one command, login, declaring sensitive, and
// answers it with the token it was given plus one of its own; padded makes
// the answer larger than the inline cap, so it goes to the blob area.
func vault(t *testing.T, sock, id string, sensitive []string) {
	t.Helper()
	c := dial(t, sock)
	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.CallRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		pad := ""
		if bytes.Contains(req.GetArgs(), []byte(`"big":true`)) {
			pad = strings.Repeat("x", 2*observe.DefaultPayloadCap)
		}
		return &rigv1.CallResponse{Result: []byte(`{"ok":true,"pad":"` + pad + `","token":"` + leakResult + `"}`)}, nil
	})
	decl := testDeclaration(id)
	login := proto.CloneOf(decl.GetCommands()[0])
	login.Id, login.Title = "login", "Log in"
	login.Effects = rigv1.Effects_EFFECTS_NETWORK
	login.Sensitive = &rigv1.SensitiveFields{Pointers: sensitive}
	login.Args = []byte(`{"type":"object"}`)
	decl.Commands = append(decl.Commands, login)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, decl); err != nil {
		t.Fatal(err)
	}
}

// leaks calls login twice, small and large, closes the store so everything
// is on disk, and answers the files under logs/ that hold either secret.
func leaks(t *testing.T, sensitive []string) (found []string, calls []observe.Record, store *observe.Store) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "logs")
	store = observe.New(observe.Options{})
	if err := store.Attach(dir); err != nil {
		t.Fatal(err)
	}
	sock, _ := upDaemonWith(t, func(c *Config) { c.Logs = store })
	vault(t, sock, "vault", sensitive)
	caller := dial(t, sock)
	for _, args := range []string{
		`{"user":"boris","token":"` + leakArg + `"}`,
		`{"user":"boris","big":true,"token":"` + leakArg + `"}`,
	} {
		if err := caller.Call(ctx5(t), "vault.login", &rigv1.CallRequest{Args: []byte(args)}, &rigv1.CallResponse{}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := store.Query(observe.Query{Calls: true, MinLevel: -8})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(leakArg)) || bytes.Contains(b, []byte(leakResult)) {
			found = append(found, filepath.Base(filepath.Dir(path))+"/"+e.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found, res.Records, store
}

// ⛔ A DECLARED-SENSITIVE VALUE IS IN NO FILE UNDER logs/, inline or in the
// blob area, while the call itself is recorded with everything else it said.
func TestADeclaredSensitiveValueNeverReachesTheLogs(t *testing.T) {
	found, calls, store := leaks(t, []string{"/token"})
	if len(found) != 0 {
		t.Fatalf("the declared-sensitive token was written to %v", found)
	}
	if len(calls) != 2 {
		t.Fatalf("recorded %d calls, want 2: %+v", len(calls), calls)
	}
	small, big := calls[0].Attrs, calls[1].Attrs
	if calls[0].Message != "vault.login" || !strings.Contains(small["args"], `"user":"boris"`) ||
		!strings.Contains(small["args"], `"token":"[redacted]"`) || !strings.Contains(small["result"], `"token":"[redacted]"`) {
		t.Fatalf("the small call recorded %v", small)
	}
	if big["result.blob"] == "" || big["result"] != "" || big["result.bytes"] == "" {
		t.Fatalf("the large answer was not put in the blob area: %v", big)
	}
	blob, err := store.ReadBlob(big["result.blob"])
	if err != nil || !bytes.Contains(blob, []byte(`"token":"[redacted]"`)) || !bytes.HasPrefix(blob, []byte(`{"ok":true`)) {
		t.Fatalf("the blob read back %.80q, %v", blob, err)
	}
	if _, err := store.ReadBlob("../../etc/passwd@0+10"); err == nil {
		t.Fatal("a reference outside the blob area was read")
	}
}

// THE RED CONTROL: the same calls declaring nothing sensitive, and the
// token IS found, so the grep above can see what it looks for, in both the
// segment and the blob area.
func TestTheLeakTestFindsAnUndeclaredToken(t *testing.T) {
	found, _, _ := leaks(t, []string{})
	var seg, blob bool
	for _, f := range found {
		seg = seg || strings.HasPrefix(f, "segments/")
		blob = blob || strings.HasPrefix(f, "blobs/")
	}
	if !seg || !blob {
		t.Fatalf("an undeclared token was found only in %v; the leak test cannot see a segment and a blob", found)
	}
}

// Decision 9: the secrets service is recorded by key name only, though it
// declares nothing sensitive.
func TestTheSecretsServiceIsRecordedByKeyNameOnly(t *testing.T) {
	store := observe.New(observe.Options{})
	if err := store.Attach(filepath.Join(t.TempDir(), "logs")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sock, _ := upDaemonWith(t, func(c *Config) { c.Logs = store })
	vault(t, sock, secretsProgram, []string{})
	caller := dial(t, sock)
	args := `{"key":"github","token":"` + leakArg + `"}`
	if err := caller.Call(ctx5(t), "secrets.login", &rigv1.CallRequest{Args: []byte(args)}, &rigv1.CallResponse{}); err != nil {
		t.Fatal(err)
	}
	res, err := store.Query(observe.Query{Calls: true})
	if err != nil || len(res.Records) != 1 {
		t.Fatalf("recorded %+v", res.Records)
	}
	a := res.Records[0].Attrs
	if a["args"] != `{"key":"github"}` || a["result"] != "" || a["result.note"] == "" {
		t.Fatalf("a secrets call recorded %v", a)
	}
}

// A call record is not a log record: `rig logs` never shows one.
func TestRigLogsDoesNotShowCallRecords(t *testing.T) {
	_, calls, _ := leaks(t, []string{"/token"})
	if len(calls) == 0 {
		t.Fatal("no call was recorded")
	}
	store := observe.New(observe.Options{})
	store.AppendCall(observe.Call{At: time.Now(), Method: "vault.login", Caller: "t"})
	if got, err := store.Query(observe.Query{MinLevel: -8}); err != nil || len(got.Records) != 0 {
		t.Fatalf("a log query answered call records %+v, %v", got.Records, err)
	}
}
