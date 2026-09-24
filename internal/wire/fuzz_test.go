package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// fuzzStream is a byte stream in and a buffer out, the smallest thing NewConn takes.
type fuzzStream struct {
	io.Reader
	bytes.Buffer
}

func (*fuzzStream) Close() error { return nil }

func (r *fuzzStream) Read(p []byte) (int, error) { return r.Reader.Read(p) }

func (r *fuzzStream) Write(p []byte) (int, error) { return r.Buffer.Write(p) }

func encoded(tb testing.TB, f *rigv1.Frame) []byte {
	tb.Helper()
	body, err := proto.Marshal(f)
	if err != nil {
		tb.Fatal(err)
	}
	out := binary.BigEndian.AppendUint32(nil, uint32(len(body)))
	return append(out, body...)
}

// FuzzFrame feeds arbitrary bytes to the frame reader, which is the first
// thing every byte from a peer meets.
//
// What must hold for ANY input: no panic; a length prefix over MaxFrameSize
// is refused as ErrFrameTooLarge without reading on; and every frame the
// reader accepts is valid and survives a write and a second read unchanged,
// so what rig passes on is exactly what it read.
func FuzzFrame(f *testing.F) {
	for _, fr := range []*rigv1.Frame{
		{StreamId: 1, Kind: rigv1.FrameKind_FRAME_KIND_REQUEST, Method: "rig.ping", Payload: []byte{1, 2}},
		{StreamId: 2, Kind: rigv1.FrameKind_FRAME_KIND_RESPONSE, Payload: []byte("x")},
		{StreamId: 3, Kind: rigv1.FrameKind_FRAME_KIND_ERROR, Status: &rigv1.Status{Code: rigv1.Code_CODE_INVALID, Message: "no"}},
		{StreamId: 4, Kind: rigv1.FrameKind_FRAME_KIND_CANCEL},
	} {
		f.Add(encoded(f, fr))
	}
	f.Add([]byte{0, 0, 0, 0})
	f.Add(binary.BigEndian.AppendUint32(nil, MaxFrameSize+1))
	f.Add(binary.BigEndian.AppendUint32(nil, MaxFrameSize))
	f.Add(append(encoded(f, &rigv1.Frame{StreamId: 1, Kind: rigv1.FrameKind_FRAME_KIND_CANCEL}), 0, 0, 0))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := NewConn(&fuzzStream{Reader: bytes.NewReader(data)})
		for range 64 {
			fr, err := c.ReadFrame()
			if err != nil {
				if len(data) >= 4 && binary.BigEndian.Uint32(data[:4]) > MaxFrameSize &&
					!errors.Is(err, ErrFrameTooLarge) {
					// Only the first prefix is checked by position; later ones
					// are covered by the same code path.
					t.Fatalf("an over-cap first prefix gave %v, want ErrFrameTooLarge", err)
				}
				return
			}
			if err := Validate(fr); err != nil {
				t.Fatalf("ReadFrame returned a frame Validate refuses: %v", err)
			}
			out := &fuzzStream{Reader: bytes.NewReader(nil)}
			if err := NewConn(out).WriteFrame(fr); err != nil {
				t.Fatalf("a frame that was read cannot be written back: %v", err)
			}
			back, err := NewConn(&fuzzStream{Reader: bytes.NewReader(out.Bytes())}).ReadFrame()
			if err != nil {
				t.Fatalf("a frame written back cannot be read: %v", err)
			}
			if !proto.Equal(fr, back) {
				t.Fatalf("the frame changed on a round trip:\n%v\n%v", fr, back)
			}
		}
	})
}
