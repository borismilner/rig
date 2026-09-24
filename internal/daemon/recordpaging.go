package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// B116's daemon half: the page budget and the opaque cursor that carries a
// `record.query` answer across more than one frame.
//
// ⛔ WHY IT IS THE DAEMON'S AND NOT THE STORE'S. A page ends on a BYTE count,
// and bytes are wire policy: internal/record has no business knowing what a
// frame is, and a store that knew would have to be told again by every other
// transport. The store answers a count; this file decides what that count
// should be. The specification is plan/50, "B116, the answer is paged".

// RecordPageBudget is the largest encoded `record.query` answer this daemon
// will build.
//
// ⛔ A QUARTER OF wire.MaxFrameSize, AND THE HEADROOM IS THE POINT. The budget
// is counted over the records; the frame that carries them also holds the
// cursor, the stream id and the frame's own tags, and a budget set AT the
// frame size would make every one of those an overflow. Four times the
// headroom costs a round trip per 256 KiB and buys a bound that cannot be
// argued into failing.
const RecordPageBudget = 256 << 10

// recordPageBatch is how many rows one read from the store fetches while a
// page is being filled.
//
// ⛔ IT IS NOT THE PAGE SIZE. The page ends on bytes, so the daemon reads in
// batches until the budget is met: a big batch would hold records in memory
// that the page cannot carry, and a batch of one would be a query per record.
// 64 small records is a few KB read ahead at worst.
const recordPageBatch = 64

// recordPageMaxLimit caps the caller's own `limit` before it becomes an int.
//
// The budget is what actually bounds a page, so a caller asking for more than
// this loses nothing: it gets whatever the budget allows, exactly as a caller
// asking for none does.
const recordPageMaxLimit uint32 = 1 << 20

// maxRecordCursor bounds the cursor a caller may send back.
//
// A cursor this daemon issued is around a hundred bytes. The bound is checked
// BEFORE the base64 decode allocates, because the length of the input is the
// one thing a caller controls for free.
const maxRecordCursor = 4096

// errNotACursor refuses an `after` this daemon did not issue.
//
// ⛔ REFUSED, NEVER READ AS A POSITION. A cursor that decodes to something
// arbitrary positions somewhere arbitrary, and a query that silently starts in
// the middle answers a subset that looks exactly like a complete answer. This
// is the one place where a read can be wrong in the reassuring direction, so
// it fails loudly instead.
var errNotACursor = errors.New(
	"after is not a cursor this daemon issued. It is opaque: pass back the " +
		"`next` from the previous answer, or omit it to start at the first page")

// recordCursor is the wire form of record.Cursor: JSON in base64, one short
// key per part.
//
// ⛔ OPAQUE MEANS NOBODY DECODES IT, NOT THAT NOBODY CAN. The encoding is
// deliberately boring - encoding/json and encoding/base64, no new dependency
// and nothing hand-rolled - because its job is to be unambiguous and to refuse
// what it did not write, not to be a secret. It carries no capability and no
// identity: a caller who decodes one learns the position it already sent.
type recordCursor struct {
	P string `json:"p"`
	K string `json:"k"`
	I string `json:"i"`
}

// encodeRecordCursor renders a cursor for the `next` field.
func encodeRecordCursor(c record.Cursor) (string, error) {
	raw, err := json.Marshal(recordCursor{P: c.Project, K: c.Kind, I: c.ID})
	if err != nil {
		// Three strings cannot fail to marshal. It is reported rather than
		// swallowed because the swallowed form is an empty `next`, which the
		// caller reads as "last page" and stops - a truncated answer that
		// nothing announces.
		return "", fmt.Errorf("encoding the page cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeRecordCursor reads an `after` back. The empty string is the first
// page, which is the only value a caller is ever expected to invent.
func decodeRecordCursor(s string) (record.Cursor, error) {
	if s == "" {
		return record.Cursor{}, nil
	}
	if len(s) > maxRecordCursor {
		return record.Cursor{}, errNotACursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return record.Cursor{}, errNotACursor
	}
	dec := json.NewDecoder(bytes.NewReader(raw))

	// ⛔ UNKNOWN FIELDS AND TRAILING BYTES ARE BOTH REFUSED. Without either
	// check, `{"p":"a","k":"b","i":"c"}` followed by anything at all decodes
	// happily, so a caller could append to a cursor and still be positioned -
	// which is a client decoding the opaque value, arriving through the back
	// door.
	dec.DisallowUnknownFields()
	var c recordCursor
	if err := dec.Decode(&c); err != nil {
		return record.Cursor{}, errNotACursor
	}
	if dec.More() {
		return record.Cursor{}, errNotACursor
	}

	// Every part of a cursor this daemon issued is non-empty, because no
	// stored record has an empty project, kind or id. An empty part is
	// therefore not a cursor, and the zero cursor is spelled as an omitted
	// `after` rather than as an encoded nothing.
	if c.P == "" || c.K == "" || c.I == "" {
		return record.Cursor{}, errNotACursor
	}
	return record.Cursor{Project: c.P, Kind: c.K, ID: c.I}, nil
}

// recordPage fills one page of a query answer under the byte budget.
//
// It returns the records and the cursor to resume from, which is empty on the
// last page. limit is the caller's own cap on the COUNT and 0 means the budget
// alone decides.
//
// ⛔ A PAGE ALWAYS CARRIES AT LEAST ONE RECORD. A record that alone exceeds the
// budget would otherwise produce an empty page with a cursor that never
// advances, which is an infinite loop in every client at once. A body can only
// have entered the store through a frame, so it is already below one; if that
// ever stops being true the frame write refuses loudly, which is the right
// failure and not this one.
func recordPage(
	ctx context.Context, st *record.Store, f record.QueryFilter, after record.Cursor, limit int,
) ([]*rigv1.Record, record.Cursor, error) {
	var (
		out  []*rigv1.Record
		cur  = after
		size int
	)
	for {
		if limit > 0 && len(out) >= limit {
			return out, cur, nil
		}
		want := recordPageBatch
		if limit > 0 && limit-len(out) < want {
			want = limit - len(out)
		}
		batch, err := st.FindPage(ctx, f, cur, want)
		if err != nil {
			return nil, record.Cursor{}, err
		}
		for _, r := range batch {
			w := recordToWire(r)

			// The cost of this record IN the answer: its own encoded size
			// plus the tag and length that carry it as a repeated field.
			// proto.Size of the whole response per record would be quadratic
			// over a page, and at 256 KiB of small records that is thousands
			// of re-encodings.
			n := protowire.SizeTag(recordsFieldNumber) +
				protowire.SizeBytes(proto.Size(w))
			if len(out) > 0 && size+n > RecordPageBudget {
				return out, cur, nil
			}
			out = append(out, w)
			size += n
			cur = record.CursorAt(r)
		}

		// A short batch means the store has nothing after the cursor, so this
		// is the last page and the caller must be told to stop.
		if len(batch) < want {
			return out, record.Cursor{}, nil
		}
	}
}

// recordsFieldNumber is RecordQueryResponse.records, whose tag every record on
// a page pays for. It is named rather than spelled 1 at the call site because
// the number is part of the wire and a bare 1 reads as an index.
const recordsFieldNumber = 1
