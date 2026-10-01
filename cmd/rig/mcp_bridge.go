package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// The bridge behind `rig mcp`: it carries an agent host's MCP session over a
// rigd restart instead of ending it. PLAN.md section 18 ("rig mcp across a
// daemon restart") and decision 0254 carry the reasoning; decision 0126 is why
// a redial does not need to tell the seat anything itself.

// redialFor bounds how long the bridge waits for rigd to come back. A deploy
// restarts rigd in a second or two; past this the daemon is down, not
// restarting, and the bridge exits non-zero as decision 0123 requires.
const redialFor = 30 * time.Second

// dialFunc opens one connection to rigd's MCP socket.
type dialFunc func(ctx context.Context) (net.Conn, error)

// rpcFrame is the part of a JSON-RPC message the bridge reads: enough to tell
// a request from a response and to pair them.
type rpcFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
}

type bridge struct {
	in        <-chan []byte // the host's lines; closed when its stdin ends
	inErr     error         // why it ended; read only after in is closed
	out       io.Writer
	dial      dialFunc
	redialFor time.Duration

	mu      sync.Mutex // guards out and everything below
	pending map[string]json.RawMessage
	carry   []byte // a host line no connection accepted; sent first on the next
	init    []byte // the host's initialize request, replayed on a redial
	inited  []byte // the host's notifications/initialized, replayed after it
	resumes int
}

func newBridge(in io.Reader, out io.Writer, dial dialFunc, wait time.Duration) *bridge {
	lines := make(chan []byte)
	b := &bridge{
		in: lines, out: out, dial: dial, redialFor: wait,
		pending: map[string]json.RawMessage{},
	}
	go func() {
		r := bufio.NewReader(in)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				lines <- line
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					b.inErr = fmt.Errorf("rig mcp: reading from the agent host: %w", err)
				}
				close(lines)
				return
			}
		}
	}()
	return b
}

// run serves the session over c and every connection after it. It returns nil
// only when the host closed its stdin, as decision 0123 requires.
func (b *bridge) run(c net.Conn) error {
	r := bufio.NewReader(c)
	var unsent []byte
	for {
		lost, err := b.serve(c, r, unsent)
		if !lost {
			return err
		}
		unsent = b.carry
		b.carry = nil
		b.failPending()
		var cause error
		c, r, cause = b.redial()
		if c == nil {
			return lostDaemon(cause)
		}
	}
}

// serve pumps one connection. lost is true when the daemon side ended while
// the host was still there, which is the case worth a redial.
func (b *bridge) serve(c net.Conn, r *bufio.Reader, unsent []byte) (lost bool, err error) {
	fromDaemon := make(chan error, 1)
	go func() { fromDaemon <- b.copyFromDaemon(r) }()

	if unsent != nil && !b.send(c, unsent) {
		_ = c.Close()
		<-fromDaemon
		return true, nil
	}

	for {
		select {
		case line, ok := <-b.in:
			if !ok {
				// The host said stop. Half-close so the daemon finishes
				// answering, and drain what it still has to say.
				if u, isUnix := c.(*net.UnixConn); isUnix {
					_ = u.CloseWrite()
				}
				<-fromDaemon
				_ = c.Close()
				return false, b.inErr
			}
			b.noteHost(line)
			if !b.send(c, line) {
				_ = c.Close()
				<-fromDaemon
				return true, nil
			}
		case derr := <-fromDaemon:
			_ = c.Close()
			var host *hostGoneError
			if errors.As(derr, &host) {
				return false, derr
			}
			return true, nil
		}
	}
}

// send writes one host line to the daemon. A line the write refused never
// reached rigd, so it is carried to the next connection rather than failed:
// resending it is not a replay, because nothing ran it.
func (b *bridge) send(c net.Conn, line []byte) bool {
	if _, err := c.Write(line); err != nil {
		var f rpcFrame
		b.mu.Lock()
		_ = json.Unmarshal(line, &f)
		if len(f.ID) > 0 {
			delete(b.pending, string(f.ID))
		}
		// The handshake must reach the next rigd exactly once: an unsent
		// initialize goes as the host's own (so the host gets its answer) and
		// is not replayed; an unsent initialized is left to the replay.
		switch f.Method {
		case "initialize":
			b.init, b.inited = nil, nil
			b.carry = line
		case "notifications/initialized":
		default:
			b.carry = line
		}
		b.mu.Unlock()
		return false
	}
	return true
}

// hostGoneError is a failed write to the host: the session is over, so the
// bridge must not redial on its behalf.
type hostGoneError struct{ err error }

func (e *hostGoneError) Error() string {
	return "rig mcp: writing to the agent host: " + e.err.Error()
}
func (e *hostGoneError) Unwrap() error { return e.err }

// copyFromDaemon forwards whole lines only. A line cut off by the daemon dying
// is dropped: half a message would corrupt the host's stream.
func (b *bridge) copyFromDaemon(r *bufio.Reader) error {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return err
		}
		b.noteDaemon(line)
		if werr := b.writeHost(line); werr != nil {
			return &hostGoneError{err: werr}
		}
	}
}

func (b *bridge) writeHost(line []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.out.Write(line)
	return err
}

// noteHost records what a redial needs: the handshake, and which requests
// are waiting for an answer.
func (b *bridge) noteHost(line []byte) {
	var f rpcFrame
	if json.Unmarshal(line, &f) != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch f.Method {
	case "initialize":
		b.init = append([]byte(nil), line...)
	case "notifications/initialized":
		b.inited = append([]byte(nil), line...)
	}
	if f.Method != "" && len(f.ID) > 0 {
		b.pending[string(f.ID)] = f.ID
	}
}

func (b *bridge) noteDaemon(line []byte) {
	var f rpcFrame
	if json.Unmarshal(line, &f) != nil || f.Method != "" || len(f.ID) == 0 {
		return
	}
	b.mu.Lock()
	delete(b.pending, string(f.ID))
	b.mu.Unlock()
}

// failPending answers every request the lost daemon never answered. It is an
// error and never a replay: PLAN.md section 18 says an interrupted call is
// never silently retried, because only the caller knows if it is idempotent.
func (b *bridge) failPending() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, id := range b.pending {
		msg, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]any{
				"code": -32603,
				"message": "rig mcp: rigd went away while this call was in flight, " +
					"so it was never answered and may or may not have taken effect. " +
					"The bridge is reconnecting; check the state before retrying",
			},
		})
		_, _ = b.out.Write(append(msg, '\n'))
		delete(b.pending, key)
	}
}

// redial waits for rigd to come back and replays the handshake on the new
// connection. A nil conn means it did not come back within redialFor.
func (b *bridge) redial() (net.Conn, *bufio.Reader, error) {
	start := time.Now()
	deadline := start.Add(b.redialFor)
	backoff := 50 * time.Millisecond
	cause := errors.New("no attempt was made")
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		c, err := b.dial(ctx)
		cancel()
		if err == nil {
			r := bufio.NewReader(c)
			if err = b.replay(c, r); err == nil {
				fmt.Fprintf(os.Stderr, "rig mcp: rigd came back after %s; session carried over\n",
					time.Since(start).Round(time.Millisecond))
				return c, r, nil
			}
			_ = c.Close()
		}
		cause = err
		time.Sleep(backoff)
		backoff = min(backoff*2, 2*time.Second)
	}
	return nil, nil, fmt.Errorf("rigd did not come back within %s: %w", b.redialFor, cause)
}

// replay repeats the host's handshake on a fresh connection, under an id of
// the bridge's own, and swallows the answer: the host already has one.
func (b *bridge) replay(c net.Conn, r *bufio.Reader) error {
	b.mu.Lock()
	initReq, inited := b.init, b.inited
	b.resumes++
	id := fmt.Sprintf("%q", fmt.Sprintf("rig-mcp-resume-%d", b.resumes))
	b.mu.Unlock()
	if initReq == nil {
		return nil // the host never initialized; there is nothing to repeat
	}

	var msg map[string]json.RawMessage
	if err := json.Unmarshal(initReq, &msg); err != nil {
		return fmt.Errorf("the saved initialize does not parse: %w", err)
	}
	msg["id"] = json.RawMessage(id)
	req, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := c.Write(append(req, '\n')); err != nil {
		return err
	}

	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = c.SetReadDeadline(time.Time{}) }()
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return fmt.Errorf("waiting for the replayed initialize: %w", err)
		}
		var f struct {
			rpcFrame
			Error json.RawMessage `json:"error,omitempty"`
		}
		if json.Unmarshal(line, &f) == nil && string(f.ID) == id && f.Method == "" {
			if len(f.Error) > 0 {
				return fmt.Errorf("the new rigd refused the replayed initialize: %s", f.Error)
			}
			break
		}
		if err := b.writeHost(line); err != nil {
			return &hostGoneError{err: err}
		}
	}
	if inited != nil {
		if _, err := c.Write(inited); err != nil {
			return err
		}
	}
	// The new build may carry different tools; let the host ask again.
	return b.writeHost([]byte(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}` + "\n"))
}
