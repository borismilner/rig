// Package clienttest runs a real rig daemon inside a Go test, so a program's
// author can test their program against rig with nothing installed.
//
// It is the daemon rigd runs, not a fake of it: the same registration
// checks, house rules, routing and refusals, on a private socket in a
// temporary directory. What a test sees is what the program will see in
// production, which a hand-written fake cannot promise.
//
//	func TestGreet(t *testing.T) {
//		clienttest.Start(t)           // rigd, private to this test
//		c, _ := client.Connect()      // finds it through XDG_RUNTIME_DIR
//		... c.Hello, then call your command from a second client ...
//	}
//
// Start sets XDG_RUNTIME_DIR and XDG_STATE_HOME for the test, so a test using
// it cannot run in parallel with another that depends on those variables
// (testing.T.Setenv refuses a parallel test for the same reason). The estate
// is unnamed, so nothing is written outside the temporary directory and
// nothing persists after the test.
package clienttest

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/borismilner/rig/internal/daemon"
	"github.com/borismilner/rig/internal/instance"
)

// Start runs rigd for the rest of the test and returns its socket path.
// client.Connect finds the same socket, because Start points
// XDG_RUNTIME_DIR at it. Everything stops and is removed at test cleanup.
func Start(tb testing.TB) string {
	tb.Helper()
	// Short on purpose: a unix socket path is capped near 108 bytes and a
	// test's own temp directory can already be most of that.
	dir, err := os.MkdirTemp("", "rigct")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	tb.Setenv("XDG_RUNTIME_DIR", dir)
	tb.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	run := filepath.Join(dir, "rig")
	if err := os.MkdirAll(run, 0o700); err != nil {
		tb.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(run, "rigd.pid"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = lock.Close() })

	d, err := daemon.New(daemon.Config{Version: "clienttest", Wire: "v1", Lock: lock})
	if err != nil {
		tb.Fatal(err)
	}
	sock := filepath.Join(run, "rigd.sock")
	ctx, cancel := context.WithCancel(context.Background())
	l, err := (&net.ListenConfig{}).Listen(ctx, "unix", sock)
	if err != nil {
		cancel()
		tb.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.Serve(ctx, l) }()
	tb.Cleanup(func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			tb.Errorf("clienttest: rigd stopped with %v", err)
		}
	})
	return sock
}
