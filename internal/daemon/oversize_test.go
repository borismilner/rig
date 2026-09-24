package daemon

import (
	"strings"
	"testing"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// ⛔ AN ANSWER TOO LARGE FOR ONE FRAME IS A REFUSAL THE CALLER HEARS, NOT A
// TIMEOUT. Two versions of a 700 KB record fit one frame each and not one
// frame together, so record.history's answer is over wire.MaxFrameSize. The
// daemon used to log the failed write and send nothing, and the caller waited
// out its deadline for a call that had finished.
func TestAnAnswerOverTheFrameCapIsRefusedToTheCaller(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	// Two DIFFERENT bodies: an identical put is a no-op and writes no version.
	for v := range uint64(2) {
		body := strings.Repeat(string(rune('a'+v)), 700<<10)
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: "big", Kind: "note", Project: "rig", Body: body, IfVersion: v,
		}, &rigv1.RecordPutResponse{}); err != nil {
			t.Fatalf("rig.record.put version %d: %v", v+1, err)
		}
	}

	err := c.Call(ctx, "rig.record.history", &rigv1.RecordHistoryRequest{Id: "big"},
		&rigv1.RecordHistoryResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a history over the frame cap")
	if !strings.Contains(err.Error(), "too large for one frame") {
		t.Errorf("the refusal does not say why: %v", err)
	}

	// The connection survives: the oversize frame was never written.
	var got rigv1.RecordGetResponse
	if err := c.Call(ctx, "rig.record.get", &rigv1.RecordGetRequest{Id: "big"}, &got); err != nil {
		t.Fatalf("the next call on the same connection failed: %v", err)
	}
}
