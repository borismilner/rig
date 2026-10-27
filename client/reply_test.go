package client

import (
	"net"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/wire"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// A reply nobody waits for is dropped, not answered: answering it made the
// daemon answer in turn, and the two traded error frames at thousands a
// second for as long as the connection lived.
func TestUnmatchedReplyIsNotAnswered(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c := &Client{closed: make(chan struct{})}
	cn := &conn{w: wire.NewConn(a)}
	peer := wire.NewConn(b)

	for _, k := range []rigv1.FrameKind{rigv1.FrameKind_FRAME_KIND_ERROR, rigv1.FrameKind_FRAME_KIND_RESPONSE} {
		done := make(chan struct{})
		go func() { c.answer(cn, &rigv1.Frame{StreamId: 7, Kind: k}); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("answering an unmatched %s blocked: it wrote a frame back", k)
		}
	}
	_ = peer
}
