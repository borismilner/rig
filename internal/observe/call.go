package observe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KindCall marks a call record (plan/49 decision 2). `rig logs` never shows
// one: its reader is §15's `rig history`.
const KindCall = "call"

// DefaultPayloadCap is decision 7's inline cap, logs.call.payload_cap.
const DefaultPayloadCap = 4096

// blobBytes is where a blob file rotates.
const blobBytes = 16 << 20

// Call is one dispatched call, its payloads already redacted by the caller.
// A payload with no pointer space arrives with Opaque set and nil bytes:
// §15's option 2, its size recorded and its contents never.
type Call struct {
	At       time.Time
	Took     time.Duration
	Caller   string
	Method   string
	Code     string // "" for success, else the status code's name
	Args     []byte
	ArgsSize int
	ArgsNote string // why Args is not the payload, when it is not
	Result   []byte
	ResSize  int
	ResNote  string
}

// AppendCall records a call. A payload over the cap goes to the blob area
// by reference; its full size is always recorded.
func (s *Store) AppendCall(c Call) {
	attrs := map[string]string{
		"caller":       c.Caller,
		"took_us":      strconv.FormatInt(c.Took.Microseconds(), 10),
		"args.bytes":   strconv.Itoa(c.ArgsSize),
		"result.bytes": strconv.Itoa(c.ResSize),
	}
	if c.Code != "" {
		attrs["code"] = c.Code
	}
	s.payload(attrs, "args", c.Args, c.ArgsNote)
	s.payload(attrs, "result", c.Result, c.ResNote)
	level := 0
	if c.Code != "" {
		level = 4
	}
	s.Append(Record{At: c.At.UnixNano(), Level: level, Client: c.Caller, Message: c.Method, Kind: KindCall, Attrs: attrs})
}

func (s *Store) payload(attrs map[string]string, key string, b []byte, note string) {
	switch {
	case note != "":
		attrs[key+".note"] = note
	case len(b) == 0:
	case len(b) <= s.payloadCap():
		attrs[key] = string(b)
	default:
		ref, err := s.blobs.put(s.Dir(), b)
		if err != nil {
			attrs[key+".note"] = "over the inline cap, and the blob area refused it: " + err.Error()
			return
		}
		attrs[key+".blob"] = ref
	}
}

func (s *Store) payloadCap() int {
	if s.opt.PayloadCap > 0 {
		return s.opt.PayloadCap
	}
	return DefaultPayloadCap
}

// blobArea is decision 7's blob area: payloads over the inline cap, written
// at once under their own lock rather than held in the ring, which a few
// large answers would otherwise fill. A record holds "file@offset+length".
type blobArea struct {
	mu   sync.Mutex
	f    *os.File
	name string
	size int64
}

func (b *blobArea) put(dir string, payload []byte) (string, error) {
	if dir == "" {
		return "", errors.New("the store has no directory yet")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.f == nil {
		blobs := filepath.Join(dir, "blobs")
		if err := os.MkdirAll(blobs, 0o700); err != nil {
			return "", err
		}
		// Named by creation time, so the names sort as the records do and
		// retention can tell which segments a file may still serve.
		b.name = fmt.Sprintf("%020d.blob", time.Now().UnixNano())
		f, err := os.OpenFile(filepath.Join(blobs, b.name), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return "", err
		}
		b.f, b.size = f, 0
	}
	off := b.size
	n, err := b.f.Write(payload)
	b.size += int64(n)
	if err == nil && b.size >= blobBytes {
		err = b.close()
	}
	if err != nil {
		return "", err
	}
	return b.name + "@" + strconv.FormatInt(off, 10) + "+" + strconv.Itoa(len(payload)), nil
}

func (b *blobArea) close() error {
	if b.f == nil {
		return nil
	}
	err := b.f.Close()
	b.f = nil
	return err
}

// ReadBlob answers the payload a record's "*.blob" attribute names.
func (s *Store) ReadBlob(ref string) ([]byte, error) {
	name, span, ok := strings.Cut(ref, "@")
	offs, lens, ok2 := strings.Cut(span, "+")
	off, err1 := strconv.ParseInt(offs, 10, 64)
	n, err2 := strconv.Atoi(lens)
	if !ok || !ok2 || err1 != nil || err2 != nil || off < 0 || n < 0 || n > maxLine ||
		filepath.Base(name) != name || !strings.HasSuffix(name, ".blob") {
		return nil, fmt.Errorf("observe: %q is not a blob reference", ref)
	}
	f, err := os.Open(filepath.Join(s.Dir(), "blobs", name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	out := make([]byte, n)
	if _, err := f.ReadAt(out, off); err != nil {
		return nil, err
	}
	return out, nil
}

// unclipped are the attributes AppendCall has already bounded by the
// payload cap, which may be larger than maxAttrValue.
var unclipped = []string{"args", "result"}

func clipsAttr(r *Record, k string) bool {
	return r.Kind != KindCall || !slices.Contains(unclipped, k)
}
