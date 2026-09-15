package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// PRECONDITION 3, ONE CASE PER ROW OF THE RULING (section 37, 2026-09-14).
//
// Every row that is not "stamped and equal" must produce a line, and the line
// must carry what makes it actionable. A policy function that returned "" for
// everything would pass a test that only checked the equal case, so the equal
// case is one row among eight rather than the test.
func TestSkewLineCoversEveryRowOfTheRuling(t *testing.T) {
	prod := func(v string) *rigv1.EstateResponse {
		return &rigv1.EstateResponse{Name: "production", DaemonVersion: v}
	}
	notFound := &client.CallError{
		Method: "rig.estate",
		Status: &rigv1.Status{Code: rigv1.Code_CODE_NOT_FOUND, Message: "no such method rig.estate"},
	}
	denied := &client.CallError{
		Method: "rig.estate",
		Status: &rigv1.Status{Code: rigv1.Code_CODE_DENIED, Message: "a house rule said no"},
	}

	for _, tc := range []struct {
		name  string
		build string
		resp  *rigv1.EstateResponse
		err   error
		want  []string // every one must appear; nil means the line must be empty
	}{
		{"stamped and equal is silent", "v0.4.0", prod("v0.4.0"), nil, nil},
		{
			"stamped and different names both builds and the estate",
			"v0.4.0-3-gabc1234-dirty", prod("v0.4.0"), nil,
			[]string{"build skew:", "v0.4.0-3-gabc1234-dirty", "is v0.4.0", `estate "production"`, "XDG_RUNTIME_DIR"},
		},
		{
			"an unnamed estate says so rather than printing an empty name",
			"v2", &rigv1.EstateResponse{DaemonVersion: "v1"}, nil,
			[]string{"build skew:", "no name claimed"},
		},
		{
			"an unstamped rig is not a match", unstamped, prod("v0.4.0"), nil,
			[]string{"not checked", "unstamped build", "v0.4.0"},
		},
		{
			"two unstamped builds are not a match either", unstamped, prod(unstamped), nil,
			[]string{"not checked", "unstamped"},
		},
		{
			"an empty daemon_version is nothing said, not a build", "v0.4.0", prod(""), nil,
			[]string{"not checked", "reporting no build"},
		},
		{
			"a daemon without rig.estate is the older half", "v0.4.0", nil, notFound,
			[]string{"build skew:", "no rig.estate", "older", "v0.4.0"},
		},
		{
			"a daemon without rig.estate, as call() wraps it", "v0.4.0", nil,
			&refusal{CallError: notFound},
			[]string{"build skew:", "no rig.estate", "older"},
		},
		{
			"a refused check says refused", "v0.4.0", nil, &refusal{CallError: denied},
			[]string{"not checked", "refused", "a house rule said no"},
		},
		{
			"a refused check says refused, unwrapped", "v0.4.0", nil, denied,
			[]string{"not checked", "refused", "a house rule said no"},
		},
		{
			"an unanswered check says so", "v0.4.0", nil, context.DeadlineExceeded,
			[]string{"not checked", "did not answer"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := skewLine(tc.build, tc.resp, tc.err)
			if tc.want == nil {
				if got != "" {
					t.Fatalf("want no line, got %q", got)
				}
				return
			}
			if !strings.HasPrefix(got, "rig: warning: ") {
				t.Errorf("%q is not a warning line", got)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("%q does not contain %q", got, w)
				}
			}
		})
	}
}

// THE CHECK ASKS THE DAEMON, AND WHAT THE DAEMON SAYS REACHES THE LINE.
//
// skewLine being right proves nothing about checkSkew calling it with what came
// back. This goes through a real socket: the fake records the method it was
// sent and answers with a build, and the control run with a matching build
// proves the line depends on the answer rather than being printed regardless.
func TestTheSkewCheckAsksTheDaemonWhoItIs(t *testing.T) {
	d := startFakeDaemon(t, &rigv1.EstateResponse{Name: "production", DaemonVersion: "v0.4.0"})
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var differ bytes.Buffer
	checkSkew(&differ, c, "v0.5.0-dirty")
	if !strings.Contains(differ.String(), "v0.4.0") || !strings.Contains(differ.String(), "v0.5.0-dirty") {
		t.Errorf("a daemon at v0.4.0 and a rig at v0.5.0-dirty printed %q", differ.String())
	}

	var same bytes.Buffer
	checkSkew(&same, c, "v0.4.0")
	if same.Len() != 0 {
		t.Errorf("a matching build printed %q; the line must depend on the answer", same.String())
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) != 2 {
		t.Fatalf("the daemon was sent %d requests for two checks", len(d.requests))
	}
	for _, f := range d.requests {
		if f.GetMethod() != "rig.estate" {
			t.Errorf("the check sent %q, want rig.estate", f.GetMethod())
		}
	}
}

// A REAL VERB WARNS, NOT JUST THE HELPER.
//
// A MUTATION PROVED THIS WAS NEEDED: deleting the checkSkew call from connect()
// survived every other test here, because they call checkSkew directly and the
// source walk only proves verbs call connect(). This runs `rig estate` against a
// fake daemon at the socket path this package actually dials, with a stamped
// build that differs, and reads what reached the warning writer.
func TestARealVerbWarnsOnSkew(t *testing.T) {
	runtime, err := os.MkdirTemp("", "rigskew")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	sock, err := paths.Socket()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	d := serveFakeDaemonAt(t, sock, &rigv1.EstateResponse{Name: "production", DaemonVersion: "v0.4.0"})

	var out bytes.Buffer
	oldOut, oldVersion := skewOut, version
	skewOut, version = &out, "v0.5.0"
	t.Cleanup(func() { skewOut, version = oldOut, oldVersion })

	if err := cmdEstate([]string{"--json"}); err != nil {
		t.Fatalf("rig estate against the fake: %v", err)
	}
	if !strings.Contains(out.String(), "build skew:") || !strings.Contains(out.String(), "v0.4.0") {
		t.Errorf("rig estate at v0.5.0 against a daemon at v0.4.0 warned %q", out.String())
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) != 2 {
		t.Errorf("rig estate sent %d requests; want the check plus the verb", len(d.requests))
	}
}

// serveFakeDaemonAt is startFakeDaemon at a path the test chooses, which is
// what lets a verb that dials paths.Socket() reach it.
func serveFakeDaemonAt(t *testing.T, sock string, reply proto.Message) *fakeDaemon {
	t.Helper()
	d := &fakeDaemon{socket: sock, reply: reply}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go d.serve(nc)
		}
	}()
	return d
}

// EVERY VERB THAT REACHES A DAEMON GOES THROUGH connect.
//
// The ruling says the CLI warns on EVERY verb, and a list of call sites is the
// thing that goes stale: a verb added next month with its own client.Connect
// would skip the check and nothing would say so. So this walks the package
// source. Exactly two files may dial directly - skew.go, which IS the check,
// and complete.go, exempt because a line on stderr during tab completion lands
// in the user's prompt.
func TestEveryDaemonVerbChecksSkew(t *testing.T) {
	allowed := map[string]bool{"skew.go": true, "complete.go": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var viaConnect int
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "client.Connect()") && !allowed[name] {
			t.Errorf("%s dials rigd with client.Connect() and so skips the build "+
				"skew check; use connect()", name)
		}
		viaConnect += strings.Count(string(src), ":= connect()")
	}
	// The control: a walk that found no files, or a rename of connect, would
	// otherwise pass with nothing checked. Seven verbs dialled rigd on the day
	// this was written.
	if viaConnect < 7 {
		t.Fatalf("found %d calls to connect(); seven verbs reached rigd when "+
			"this test was written, so the walk did not see the package", viaConnect)
	}
}

// "dev" IS THE UNSTAMPED WORD ON BOTH SIDES.
//
// The policy treats one spelling as "nobody stamped this", and it has to be the
// compile-time default in both binaries. If rigd's default changed to anything
// else, an unstamped daemon would compare as a real build and a dev pair would
// warn as skew instead of saying the builds cannot be compared.
func TestTheUnstampedWordIsBothBinariesDefault(t *testing.T) {
	for _, path := range []string{"main.go", filepath.Join("..", "rigd", "main.go")} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), `version = "`+unstamped+`"`) {
			t.Errorf("%s does not default version to %q, which skew.go treats as "+
				"the unstamped build", path, unstamped)
		}
	}
}
