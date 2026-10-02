package observe

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Redacted is what a sensitive value reads as in a record.
const Redacted = `"[redacted]"`

// Redactor blanks a command's declared sensitive values (plan/49 decision 8,
// §15): JSON pointers compiled once, at registration, into a matcher over the
// payload's encoding. A scan finds each sensitive value's byte span without
// decoding the payload into Go values, and descends only into a container on
// the way to a pointer; everything else is skipped whole.
//
// One list serves both arguments and results, since §5e declares the
// pointers into both.
type Redactor struct {
	exact    map[string]bool
	prefixes map[string]bool // every proper ancestor of a declared pointer
	all      bool            // "" was declared: the whole payload
}

// Compile builds the redactor for one command's pointers. An empty list
// compiles to a redactor that copies nothing it does not have to.
func Compile(pointers []string) (*Redactor, error) {
	r := &Redactor{exact: map[string]bool{}, prefixes: map[string]bool{}}
	for _, p := range pointers {
		if p == "" {
			r.all = true
			continue
		}
		if !strings.HasPrefix(p, "/") {
			return nil, errors.New("observe: " + strconv.Quote(p) + " is not a JSON pointer")
		}
		r.exact[p] = true
		for i := len(p) - 1; i > 0; i-- {
			if p[i] == '/' {
				r.prefixes[p[:i]] = true
			}
		}
		r.prefixes[""] = true
	}
	return r, nil
}

// Empty says the redactor blanks nothing.
func (r *Redactor) Empty() bool { return r == nil || (!r.all && len(r.exact) == 0) }

// ErrNoPointerSpace is a payload that is not one JSON value. It has no
// pointer space for a declaration to name, so §15's option 2 applies: the
// caller records its size and never its contents.
var ErrNoPointerSpace = errors.New("observe: the payload is not JSON, so it is recorded by size only")

// Apply answers payload with every sensitive value replaced by Redacted.
// A payload that is not one JSON value is ErrNoPointerSpace. With nothing
// to blank the payload is checked and returned as it is, not copied.
func (r *Redactor) Apply(payload []byte) ([]byte, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return payload, nil
	}
	if r != nil && r.all {
		if !jsontext.Value(payload).IsValid() {
			return nil, ErrNoPointerSpace
		}
		return []byte(Redacted), nil
	}
	if r.Empty() {
		if !jsontext.Value(payload).IsValid() {
			return nil, ErrNoPointerSpace
		}
		return payload, nil
	}
	spans, err := r.spans(payload)
	if err != nil {
		return nil, ErrNoPointerSpace
	}
	if len(spans) == 0 {
		return payload, nil
	}
	out := make([]byte, 0, len(payload))
	at := 0
	for _, s := range spans {
		out = append(append(out, payload[at:s[0]]...), Redacted...)
		at = s[1]
	}
	return append(out, payload[at:]...), nil
}

// decoders are reused, since a decoder's buffers are most of a scan's cost.
var decoders = sync.Pool{New: func() any {
	rd := new(bytes.Reader)
	return &pooled{rd: rd, d: jsontext.NewDecoder(rd)}
}}

type pooled struct {
	rd *bytes.Reader
	d  *jsontext.Decoder
}

// scan is one Apply's working state: the pointer of the value about to be
// read, built in place so a lookup allocates nothing.
type scan struct {
	path  []byte // the innermost open container's pointer
	marks []int  // len(path) at each container opened
	key   []byte // the next value's pointer
}

// spans finds the byte spans of the sensitive values, in order.
func (r *Redactor) spans(payload []byte) ([][2]int, error) {
	p, _ := decoders.Get().(*pooled) // the pool holds only these
	p.rd.Reset(payload)
	p.d.Reset(p.rd)
	d := p.d
	defer func() {
		p.rd.Reset(nil)
		p.d.Reset(p.rd)
		decoders.Put(p)
	}()
	var sc scan
	var spans [][2]int
	for started := false; ; started = true {
		k := d.PeekKind()
		if k == 0 || started && d.StackDepth() == 0 {
			break
		}
		if k == ']' || k == '}' {
			if _, err := d.ReadToken(); err != nil {
				return nil, err
			}
			n := len(sc.marks) - 1
			sc.path, sc.marks = sc.path[:sc.marks[n]], sc.marks[:n]
			continue
		}
		kind, n := d.StackIndex(d.StackDepth())
		sc.key = append(sc.key[:0], sc.path...)
		switch kind {
		case '{':
			name, err := d.ReadValue() // a member name comes before its value
			if err != nil {
				return nil, err
			}
			sc.key = appendToken(append(sc.key, '/'), name)
			k = d.PeekKind()
		case '[':
			sc.key = strconv.AppendInt(append(sc.key, '/'), n, 10)
		}
		switch {
		case r.exact[string(sc.key)]:
			v, err := d.ReadValue()
			if err != nil {
				return nil, err
			}
			end := int(d.InputOffset())
			spans = append(spans, [2]int{end - len(v), end})
		case (k == '{' || k == '[') && r.prefixes[string(sc.key)]:
			if _, err := d.ReadToken(); err != nil {
				return nil, err
			}
			sc.marks = append(sc.marks, len(sc.path))
			sc.path = append(sc.path, sc.key[len(sc.path):]...)
		default:
			if err := d.SkipValue(); err != nil {
				return nil, err
			}
		}
	}
	// One value, and nothing after it.
	if _, err := d.ReadToken(); !errors.Is(err, io.EOF) {
		return nil, ErrNoPointerSpace
	}
	return spans, nil
}

// appendToken appends a member name, quoted as it arrived, as one RFC 6901
// reference token: unescaped, then "~" as "~0" and "/" as "~1".
func appendToken(dst []byte, quoted jsontext.Value) []byte {
	raw := quoted[1 : len(quoted)-1]
	if bytes.IndexByte(raw, '\\') >= 0 {
		var err error
		if raw, err = jsontext.AppendUnquote(nil, quoted); err != nil {
			return append(dst, quoted...) // cannot match a declared pointer
		}
	}
	if !bytes.ContainsAny(raw, "~/") {
		return append(dst, raw...)
	}
	for _, c := range raw {
		switch c {
		case '~':
			dst = append(dst, '~', '0')
		case '/':
			dst = append(dst, '~', '1')
		default:
			dst = append(dst, c)
		}
	}
	return dst
}

// Pointers is a redactor's declared pointers, sorted, for the coverage log.
func (r *Redactor) Pointers() []string {
	if r == nil {
		return nil
	}
	out := slices.Sorted(func(yield func(string) bool) {
		for p := range r.exact {
			if !yield(p) {
				return
			}
		}
	})
	if r.all {
		out = append([]string{""}, out...)
	}
	return out
}
