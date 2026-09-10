package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// pipe gives two Conns talking to each other over in-memory pipes.
func pipe(t *testing.T) (a, b *Conn) {
	t.Helper()
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	a = NewConn(rwc{ar, aw})
	b = NewConn(rwc{br, bw})
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	return a, b
}

type rwc struct {
	io.Reader
	io.WriteCloser
}

func (r rwc) Close() error { return r.WriteCloser.Close() }

func TestRoundTrip(t *testing.T) {
	a, b := pipe(t)
	want := &rigv1.Frame{
		StreamId:  1,
		Kind:      rigv1.FrameKind_FRAME_KIND_REQUEST,
		Method:    "rig.ping",
		RequestId: "req-1",
		Payload:   []byte("hello"),
	}
	go func() {
		if err := a.WriteFrame(want); err != nil {
			t.Errorf("write: %v", err)
		}
	}()
	got, err := b.ReadFrame()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !proto.Equal(want, got) {
		t.Fatalf("round trip changed the frame:\n want %v\n got  %v", want, got)
	}
}

// A frame kind this build does not know must be refused, not defaulted. This is
// the test behind PLAN.md section 21's ban on meaningful enum zero: without the
// refusal, a newer peer's kind decodes as UNSPECIFIED and the receiver acts on
// whatever its first branch is.
func TestUnknownEnumIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind rigv1.FrameKind
	}{
		{"zero", rigv1.FrameKind_FRAME_KIND_UNSPECIFIED},
		{"from the future", rigv1.FrameKind(99)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Encode past Validate, the way a peer on a newer build would.
			body, err := proto.Marshal(&rigv1.Frame{StreamId: 1, Kind: tc.kind})
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			var p [4]byte
			binary.BigEndian.PutUint32(p[:], uint32(len(body)))
			buf.Write(p[:])
			buf.Write(body)

			c := NewConn(rwc{&buf, nopWriteCloser{io.Discard}})
			if _, err := c.ReadFrame(); err == nil {
				t.Fatalf("kind %v was accepted; it must be refused", tc.kind)
			}
		})
	}
}

func TestErrorFrameNeedsAKnownCode(t *testing.T) {
	c := NewConn(rwc{strings.NewReader(""), nopWriteCloser{io.Discard}})
	err := c.WriteFrame(&rigv1.Frame{
		StreamId: 1,
		Kind:     rigv1.FrameKind_FRAME_KIND_ERROR,
		Status:   &rigv1.Status{Code: rigv1.Code_CODE_UNSPECIFIED},
	})
	if err == nil {
		t.Fatal("an ERROR frame with an unspecified code was written")
	}
}

func TestStreamIDZeroIsReserved(t *testing.T) {
	c := NewConn(rwc{strings.NewReader(""), nopWriteCloser{io.Discard}})
	err := c.WriteFrame(&rigv1.Frame{
		StreamId: 0,
		Kind:     rigv1.FrameKind_FRAME_KIND_REQUEST,
		Method:   "rig.ping",
	})
	if err == nil {
		t.Fatal("stream_id 0 was accepted")
	}
}

func TestRequestNeedsAMethod(t *testing.T) {
	c := NewConn(rwc{strings.NewReader(""), nopWriteCloser{io.Discard}})
	if err := c.WriteFrame(&rigv1.Frame{
		StreamId: 1,
		Kind:     rigv1.FrameKind_FRAME_KIND_REQUEST,
	}); err == nil {
		t.Fatal("a REQUEST with no method was accepted")
	}
}

// An oversized length prefix must be refused without allocating what it asks
// for. The frame is never truncated to fit: a shortened frame is a wrong value
// dressed as a valid one.
func TestOversizePrefixIsRefusedNotAllocated(t *testing.T) {
	var buf bytes.Buffer
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], 1<<30)
	buf.Write(p[:])

	c := NewConn(rwc{&buf, nopWriteCloser{io.Discard}})
	_, err := c.ReadFrame()
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("want ErrFrameTooLarge, got %v", err)
	}
}

func TestOutboundOversizeIsRefused(t *testing.T) {
	c := NewConn(rwc{strings.NewReader(""), nopWriteCloser{io.Discard}})
	err := c.WriteFrame(&rigv1.Frame{
		StreamId: 1,
		Kind:     rigv1.FrameKind_FRAME_KIND_STREAM_DATA,
		Payload:  make([]byte, MaxFrameSize+1),
	})
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("want ErrFrameTooLarge, got %v", err)
	}
}

// A prefix followed by a short body is ErrUnexpectedEOF, not EOF. A supervisor
// treats a clean close and a severed connection differently (PLAN.md 18).
func TestTruncatedBodyIsUnexpectedEOF(t *testing.T) {
	var buf bytes.Buffer
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], 64)
	buf.Write(p[:])
	buf.Write(make([]byte, 10))

	c := NewConn(rwc{&buf, nopWriteCloser{io.Discard}})
	if _, err := c.ReadFrame(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want ErrUnexpectedEOF, got %v", err)
	}
}

func TestCleanCloseIsEOF(t *testing.T) {
	c := NewConn(rwc{strings.NewReader(""), nopWriteCloser{io.Discard}})
	if _, err := c.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
}

// Streams are multiplexed onto one connection, so many goroutines write at
// once. A torn length prefix desynchronises the peer permanently, so every
// frame must arrive whole and decodable.
func TestConcurrentWritesAreNotTorn(t *testing.T) {
	const writers, each = 8, 50
	a, b := pipe(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range writers * each {
			f, err := b.ReadFrame()
			if err != nil {
				t.Errorf("read %d: %v", i, err)
				return
			}
			if int(f.GetStreamId()) != len(f.GetPayload()) {
				t.Errorf("frame is torn: stream %d carries %d bytes",
					f.GetStreamId(), len(f.GetPayload()))
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for w := 1; w <= writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range each {
				// stream_id encodes the payload length, so a torn or
				// interleaved write is detectable by the reader.
				id := uint32(w*100 + i)
				if err := a.WriteFrame(&rigv1.Frame{
					StreamId: id,
					Kind:     rigv1.FrameKind_FRAME_KIND_STREAM_DATA,
					Payload:  make([]byte, id),
				}); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	<-done
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
