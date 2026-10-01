package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRigd answers initialize and tools/call on a unix socket, and stays
// silent on any call whose id is in silent, so a test can hold one in flight.
type fakeRigd struct {
	l    net.Listener
	got  chan string
	conn chan net.Conn
}

func startFakeRigd(t *testing.T, sock string, silent map[string]bool) *fakeRigd {
	t.Helper()
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRigd{l: l, got: make(chan string, 32), conn: make(chan net.Conn, 1)}
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		f.conn <- c
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				_ = c.Close() // rigd ends the session when the host does
				return
			}
			f.got <- string(line)
			var m rpcFrame
			_ = json.Unmarshal(line, &m)
			if m.Method == "" || len(m.ID) == 0 || silent[string(m.ID)] {
				continue
			}
			_, _ = c.Write([]byte(`{"jsonrpc":"2.0","id":` + string(m.ID) +
				`,"result":{"answered":"` + m.Method + `"}}` + "\n"))
		}
	}()
	return f
}

// kill drops the connection and the listener, as a rigd restart does.
func (f *fakeRigd) kill(t *testing.T) {
	t.Helper()
	select {
	case c := <-f.conn:
		_ = f.l.Close()
		_ = c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("the bridge never connected to this rigd")
	}
}

func (f *fakeRigd) next(t *testing.T) string {
	t.Helper()
	select {
	case s := <-f.got:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("rigd received nothing")
		return ""
	}
}

type bridgeRig struct {
	sock   string
	hostW  io.WriteCloser
	hostR  *bufio.Reader
	result chan error
}

func startBridge(t *testing.T) *bridgeRig {
	t.Helper()
	// A short directory: a unix socket path has a 107-byte ceiling.
	dir, err := os.MkdirTemp("", "rmb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return &bridgeRig{sock: filepath.Join(dir, "s"), result: make(chan error, 1)}
}

func (br *bridgeRig) run(t *testing.T, wait time.Duration) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	br.hostW, br.hostR = inW, bufio.NewReader(outR)
	dial := func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", br.sock)
	}
	c, err := dial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		br.result <- newBridge(inR, outW, dial, wait).run(c)
		_ = outW.Close()
	}()
}

func (br *bridgeRig) send(t *testing.T, line string) {
	t.Helper()
	if _, err := br.hostW.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
}

func (br *bridgeRig) read(t *testing.T) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		s, _ := br.hostR.ReadString('\n')
		got <- s
	}()
	select {
	case s := <-got:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("the host received nothing")
		return ""
	}
}

const testInit = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"host","version":"1"},"capabilities":{}}}`

func TestABridgeCarriesTheSessionOverARestart(t *testing.T) {
	br := startBridge(t)
	a := startFakeRigd(t, br.sock, map[string]bool{"3": true})
	br.run(t, 10*time.Second)

	br.send(t, testInit)
	a.next(t)
	if got := br.read(t); !strings.Contains(got, `"id":1`) {
		t.Fatalf("host got %q, want the initialize answer", got)
	}
	br.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	a.next(t)
	br.send(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"slow"}}`)
	a.next(t)

	// rigd restarts with call 3 in flight.
	a.kill(t)
	if got := br.read(t); !strings.Contains(got, `"id":3`) || !strings.Contains(got, `"error"`) {
		t.Fatalf("host got %q, want an error for the call in flight", got)
	}
	b := startFakeRigd(t, br.sock, nil)

	replayed := b.next(t)
	if !strings.Contains(replayed, `"id":"rig-mcp-resume-1"`) ||
		!strings.Contains(replayed, `"method":"initialize"`) ||
		!strings.Contains(replayed, `"clientInfo":{"name":"host"`) {
		t.Fatalf("the new rigd got %q, want the host's initialize under the bridge's own id", replayed)
	}
	if got := b.next(t); !strings.Contains(got, "notifications/initialized") {
		t.Fatalf("the new rigd got %q, want notifications/initialized", got)
	}
	// The replayed answer is swallowed: the host's next line is the
	// tools-changed notice, not a second initialize result.
	if got := br.read(t); !strings.Contains(got, "notifications/tools/list_changed") {
		t.Fatalf("host got %q, want tools/list_changed", got)
	}

	br.send(t, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list"}}`)
	b.next(t)
	if got := br.read(t); !strings.Contains(got, `"id":4`) || !strings.Contains(got, "result") {
		t.Fatalf("host got %q, want the new rigd's answer to call 4", got)
	}

	_ = br.hostW.Close()
	select {
	case err := <-br.result:
		if err != nil {
			t.Fatalf("the host closed its stdin, so the bridge must exit 0; got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge did not exit after the host closed its stdin")
	}
}

func TestABridgeExitsNonZeroWhenRigdStaysDown(t *testing.T) {
	br := startBridge(t)
	a := startFakeRigd(t, br.sock, nil)
	br.run(t, 300*time.Millisecond)
	br.send(t, testInit)
	a.next(t)
	br.read(t)

	a.kill(t)
	select {
	case err := <-br.result:
		if err == nil || !strings.Contains(err.Error(), "did not come back") {
			t.Fatalf("got %v, want a non-zero exit saying rigd did not come back", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge waited past its bound")
	}
}

func TestABridgeWithNoHandshakeRedialsWithoutReplaying(t *testing.T) {
	br := startBridge(t)
	a := startFakeRigd(t, br.sock, nil)
	br.run(t, 10*time.Second)
	a.kill(t)
	b := startFakeRigd(t, br.sock, nil)

	br.send(t, `{"jsonrpc":"2.0","id":7,"method":"tools/list"}`)
	if got := b.next(t); !strings.Contains(got, `"id":7`) {
		t.Fatalf("the new rigd got %q first, want the host's own request and no replay", got)
	}
}
