// Package wire is the hand-framed protobuf transport.
//
// A frame on the socket is a 4-byte big-endian length prefix followed by one
// encoded rig.v1.Frame. That is the whole framing, and it is deliberate: gRPC's
// server on a unix socket measured +9.80 MiB resident for HTTP/2 machinery a
// single local socket does not need, which is 58% of the footprint budget
// (PLAN.md sections 4, 5f, 17). Protobuf stays; gRPC goes.
//
// The package holds no domain type. It moves frames and refuses malformed ones;
// what a method means is not its business (PLAN.md section 5h).
package wire

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// MaxFrameSize is the largest frame this build will read or write.
//
// PLAN.md does not state a ceiling. Section 4 measures up to 64 KB payloads
// ("64KB frames at 19us is 3.4 GB/s effective, fine for a table of 100k rows"),
// so 1 MiB leaves an order of magnitude of headroom while keeping a hostile or
// corrupt prefix from turning into an allocation. A frame over the limit is
// refused, never truncated: a silently shortened frame is a decode error at
// best and a wrong value at worst.
const MaxFrameSize = 1 << 20

// ErrFrameTooLarge is returned for a length prefix or an outbound frame over
// MaxFrameSize. It is not wrapped, so a caller can test for it directly.
var ErrFrameTooLarge = errors.New("wire: frame exceeds MaxFrameSize")

// Conn reads and writes frames over one duplex stream.
//
// Writes are serialised: streams are multiplexed onto one connection, so
// several goroutines write frames concurrently and a torn prefix would
// desynchronise the peer permanently. Reads are not serialised - one reader
// goroutine per connection is the intended shape.
type Conn struct {
	r    *bufio.Reader
	w    io.Writer
	c    io.Closer
	wmu  sync.Mutex
	wbuf []byte
}

// NewConn wraps a duplex stream. rw is usually a *net.UnixConn.
func NewConn(rw io.ReadWriteCloser) *Conn {
	return &Conn{
		r:    bufio.NewReaderSize(rw, 64<<10),
		w:    rw,
		c:    rw,
		wbuf: make([]byte, 4, 4+4096),
	}
}

// Close closes the underlying stream.
func (c *Conn) Close() error { return c.c.Close() }

// WriteFrame validates, encodes and writes one frame.
//
// A frame that fails validation is not written: refusing on the way out turns a
// local bug into a local error instead of a peer's decode failure, which is the
// harder one to attribute.
func (c *Conn) WriteFrame(f *rigv1.Frame) error {
	if err := Validate(f); err != nil {
		return fmt.Errorf("wire: refusing to write invalid frame: %w", err)
	}
	body, err := proto.Marshal(f)
	if err != nil {
		return fmt.Errorf("wire: marshal: %w", err)
	}
	if len(body) > MaxFrameSize {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(body))
	}

	c.wmu.Lock()
	defer c.wmu.Unlock()
	buf := append(c.wbuf[:0], 0, 0, 0, 0)
	binary.BigEndian.PutUint32(buf, uint32(len(body)))
	buf = append(buf, body...)
	if _, err := c.w.Write(buf); err != nil {
		return fmt.Errorf("wire: write: %w", err)
	}
	if cap(buf) <= 4+(64<<10) {
		c.wbuf = buf // keep the grown buffer, but do not hold a huge one
	}
	return nil
}

// ReadFrame reads one frame. It returns io.EOF only when the stream ended
// cleanly on a frame boundary; a prefix followed by a short body is
// io.ErrUnexpectedEOF, because those two mean different things to a supervisor.
func (c *Conn) ReadFrame() (*rigv1.Frame, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(c.r, prefix[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	n := binary.BigEndian.Uint32(prefix[:])
	if n == 0 {
		return nil, errors.New("wire: zero-length frame")
	}
	if n > MaxFrameSize {
		// Do not allocate and do not try to skip: the stream is no longer
		// trustworthy, so the connection is the thing that gets dropped.
		return nil, fmt.Errorf("%w: prefix claims %d bytes", ErrFrameTooLarge, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, io.ErrUnexpectedEOF
	}
	f := &rigv1.Frame{}
	if err := proto.Unmarshal(body, f); err != nil {
		return nil, fmt.Errorf("wire: unmarshal: %w", err)
	}
	if err := Validate(f); err != nil {
		return nil, err
	}
	return f, nil
}

// Validate refuses a frame this build cannot act on.
//
// Enum zero is banned and an unknown enum value is a hard refusal at the
// boundary, never a zero-value fall-through (PLAN.md section 21). Without this,
// a newer peer's FrameKind decodes as UNSPECIFIED and a caller reads it as
// whatever the first branch happens to be - the failure that makes a wire
// "compatible" in a round-trip test while meaning something else in production.
func Validate(f *rigv1.Frame) error {
	if f == nil {
		return errors.New("wire: nil frame")
	}
	if f.GetStreamId() == 0 {
		return errors.New("wire: stream_id 0 is reserved")
	}
	switch f.GetKind() {
	case rigv1.FrameKind_FRAME_KIND_REQUEST:
		if f.GetMethod() == "" {
			return errors.New("wire: REQUEST with no method")
		}
	case rigv1.FrameKind_FRAME_KIND_RESPONSE,
		rigv1.FrameKind_FRAME_KIND_STREAM_DATA,
		rigv1.FrameKind_FRAME_KIND_STREAM_END,
		rigv1.FrameKind_FRAME_KIND_CANCEL:
		// nothing further is required of these
	case rigv1.FrameKind_FRAME_KIND_ERROR:
		code := f.GetStatus().GetCode()
		if code == rigv1.Code_CODE_UNSPECIFIED {
			return errors.New("wire: ERROR with unspecified code")
		}
		if _, known := rigv1.Code_name[int32(code)]; !known {
			return fmt.Errorf("wire: ERROR with unknown code %d", code)
		}
	case rigv1.FrameKind_FRAME_KIND_UNSPECIFIED:
		return errors.New("wire: frame kind unspecified")
	default:
		return fmt.Errorf("wire: unknown frame kind %d", f.GetKind())
	}
	return nil
}
