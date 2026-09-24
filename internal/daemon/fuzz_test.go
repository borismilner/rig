package daemon

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/wire"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// The two request paths this branch added that take caller-shaped strings:
// record.refs with a list of ids, and the lease verbs with a name and a
// reason. Each target drives a real daemon over its socket, one daemon per
// fuzz worker, so what is exercised is the whole path a caller's bytes take.
//
// What must hold for ANY input: the daemon answers, with a response or with a
// refusal carrying a defined code - never a hang, never a dropped connection -
// and the bounds hold: more than maxRefsIDs ids, or a lease text over
// maxLeaseText bytes, is refused as INVALID.

// answered fails unless err is nil or a structured refusal with a known code.
func answered(t *testing.T, what string, err error) *client.CallError {
	t.Helper()
	if err == nil {
		return nil
	}
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("%s was not answered: %v", what, err)
	}
	if _, known := rigv1.Code_name[int32(ce.Code())]; !known || ce.Code() == rigv1.Code_CODE_UNSPECIFIED {
		t.Fatalf("%s was refused with code %d, which is not a defined code", what, ce.Code())
	}
	return ce
}

func fuzzCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func FuzzRecordRefsIDs(f *testing.F) {
	f.Add("R0\x00R1", uint32(0), false)
	f.Add("", uint32(2), true)
	f.Add("nope", uint32(9), false)
	f.Add(strings.Repeat("R0\x00", 300), uint32(1), false)
	f.Add("\xff\xfe\x00R2", uint32(5), true)

	sock := upRecordDaemon(f)
	c, err := client.Dial(sock)
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	if err := c.Call(ctx, "rig.announce", &verbsv1.AnnounceRequest{
		Seat: "fuzz", Purpose: "fuzzing record.refs", Activity: "fuzzing",
	}, &verbsv1.AnnounceResponse{}); err != nil {
		f.Fatal(err)
	}
	refsFixture(ctx, f, c, 3)
	cancel()

	f.Fuzz(func(t *testing.T, joined string, depth uint32, cross bool) {
		// The Go client refuses to marshal invalid UTF-8, so such bytes never
		// reach rigd through it; FuzzRawPayload sends them raw instead.
		ids := strings.Split(strings.ToValidUTF8(joined, "?"), "\x00")
		var resp verbsv1.RecordRefsResponse
		ce := answered(t, "rig.record.refs", c.Call(fuzzCtx(t), "rig.record.refs",
			&verbsv1.RecordRefsRequest{Ids: ids, Depth: depth, CrossProject: cross}, &resp))
		if len(ids) > maxRefsIDs && ce == nil {
			t.Fatalf("%d ids were answered; the cap is %d", len(ids), maxRefsIDs)
		}
		if ce == nil && len(resp.GetResults()) != len(ids) {
			t.Fatalf("asked about %d ids and got %d results", len(ids), len(resp.GetResults()))
		}
	})
}

func FuzzLeaseText(f *testing.F) {
	f.Add("deploy", "the holder's VM was torn down", uint32(1000), false)
	f.Add("", "", uint32(0), true)
	f.Add(strings.Repeat("n", maxLeaseText+1), "r", uint32(1), false)
	f.Add("x", strings.Repeat("r", maxLeaseText+1), uint32(1), true)
	f.Add("\x00/../\xff", "\n\t", uint32(1<<31), false)

	sock, _ := upLeaseDaemon(f)
	c, err := client.Dial(sock)
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	if err := c.Call(ctx, "rig.announce", &verbsv1.AnnounceRequest{
		Seat: "fuzz", Purpose: "fuzzing the lease verbs", Activity: "fuzzing",
	}, &verbsv1.AnnounceResponse{}); err != nil {
		f.Fatal(err)
	}
	cancel()

	f.Fuzz(func(t *testing.T, name, reason string, ttl uint32, unwitnessed bool) {
		name, reason = strings.ToValidUTF8(name, "?"), strings.ToValidUTF8(reason, "?")
		ctx := fuzzCtx(t)
		var got verbsv1.LeaseAcquireResponse
		ce := answered(t, "rig.lease.acquire", c.Call(ctx, "rig.lease.acquire",
			&verbsv1.LeaseAcquireRequest{Name: name, TtlMs: ttl, Unwitnessed: unwitnessed}, &got))
		if len(name) > maxLeaseText && (ce == nil || ce.Code() != rigv1.Code_CODE_INVALID) {
			t.Fatalf("a %d-byte lease name was not refused as INVALID: %v", len(name), ce)
		}
		if ce == nil {
			h := got.GetHandle()
			answered(t, "rig.lease.release", c.Call(ctx, "rig.lease.release",
				&verbsv1.LeaseReleaseRequest{Name: h.GetName(), Token: h.GetToken(), Epoch: h.GetEpoch()},
				&verbsv1.LeaseReleaseResponse{}))
		}
		bce := answered(t, "rig.lease.break", c.Call(ctx, "rig.lease.break",
			&verbsv1.LeaseBreakRequest{Name: name, Reason: reason}, &verbsv1.LeaseBreakResponse{}))
		if (len(name) > maxLeaseText || len(reason) > maxLeaseText) &&
			(bce == nil || bce.Code() != rigv1.Code_CODE_INVALID) {
			t.Fatalf("a break with a %d-byte name and a %d-byte reason was not refused as INVALID: %v",
				len(name), len(reason), bce)
		}
		answered(t, "rig.lease.list", c.Call(ctx, "rig.lease.list",
			&verbsv1.LeaseListRequest{}, &verbsv1.LeaseListResponse{}))
	})
}

// FuzzRawPayload sends arbitrary payload bytes to each verb this branch added,
// through the wire rather than the client, so a payload the client would never
// build - invalid UTF-8 in a string field, a truncated varint, a wrong wire
// type - still reaches rigd's own decoding. Every one must be answered with a
// RESPONSE or an ERROR frame carrying a defined code, on the same stream.
func FuzzRawPayload(f *testing.F) {
	methods := []string{
		"record.refs", "lease.acquire", "lease.renew",
		"lease.release", "lease.break", "lease.list", "describe",
	}
	f.Add(uint8(0), []byte{0x22, 0x02, 0xff, 0xfe})
	f.Add(uint8(1), []byte{0x0a, 0x03, 'a', 'b', 'c', 0x10, 0x88, 0x27})
	f.Add(uint8(4), []byte{0x0a, 0x01, 'x', 0x12, 0x80})
	f.Add(uint8(6), []byte{0x0a, 0x04, 'n', 'o', 'p', 'e'})
	f.Add(uint8(2), []byte{})

	sock, _ := upLeaseDaemon(f)
	nc, err := net.Dial("unix", sock)
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = nc.Close() })
	wc := wire.NewConn(nc)
	stream := uint32(0)

	f.Fuzz(func(t *testing.T, which uint8, payload []byte) {
		stream++
		method := "rig." + methods[int(which)%len(methods)]
		if err := wc.WriteFrame(&rigv1.Frame{
			StreamId: stream, Kind: rigv1.FrameKind_FRAME_KIND_REQUEST,
			Method: method, Payload: payload,
		}); err != nil {
			t.Fatalf("writing %s: %v", method, err)
		}
		_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
		got, err := wc.ReadFrame()
		if err != nil {
			t.Fatalf("%s with payload %x was not answered: %v", method, payload, err)
		}
		if got.GetStreamId() != stream {
			t.Fatalf("%s answered on stream %d, want %d", method, got.GetStreamId(), stream)
		}
		switch got.GetKind() {
		case rigv1.FrameKind_FRAME_KIND_RESPONSE:
		case rigv1.FrameKind_FRAME_KIND_ERROR:
			if got.GetStatus().GetCode() == rigv1.Code_CODE_UNSPECIFIED {
				t.Fatalf("%s refused with no code", method)
			}
		default:
			t.Fatalf("%s answered with a %s frame", method, got.GetKind())
		}
	})
}
