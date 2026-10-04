package daemon

import (
	"encoding/json"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// ⛔ rig.desk SAYS WHERE BORIS IS, AND ONLY rig SAYS IT: each change is
// stored and posted once, a repeat is not, and no seat may write or delete
// a rig.* key, so a program can trust what it reads there.
func TestTheDeskIsRigsToSayAndPostedOnChange(t *testing.T) {
	sock, _, d := upLeaseDaemonD(t)
	watch := dial(t, sock)
	cursor := waitOn(t, watch, 0, 1, "shared.rig.desk").GetLatest()

	at := time.Date(2026, 10, 4, 20, 0, 0, 0, time.UTC)
	d.setDesk(deskActive, at)
	d.setDesk(deskActive, at.Add(time.Minute))
	d.setDesk(deskLocked, at.Add(2*time.Minute))

	var posted int
	for deadline := time.Now().Add(5 * time.Second); posted < 2 && time.Now().Before(deadline); {
		got := waitOn(t, watch, cursor, 1000, "shared.rig.desk")
		for _, ev := range got.GetEvents() {
			posted++
			cursor = ev.GetSeq()
		}
	}
	if extra := waitOn(t, watch, cursor, 200, "shared.rig.desk").GetEvents(); posted != 2 || len(extra) != 0 {
		t.Fatalf("posted %d then %d more, want 2 then none", posted, len(extra))
	}

	a := seated(t, sock, "seat-a")
	got := sharedGet(t, a, deskKey)
	var desk deskState
	if err := json.Unmarshal([]byte(got.GetValue().GetValueJson()), &desk); err != nil ||
		desk.State != deskLocked || desk.Since != "2026-10-04T20:02:00Z" || got.GetValue().GetBy() != "rig" {
		t.Fatalf("rig.desk reads %+v (%v)", got.GetValue(), err)
	}

	v := got.GetValue().GetVersion()
	err := a.Call(recordCtx(t), "rig.shared.set", &verbsv1.SharedSetRequest{Key: deskKey, ValueJson: `{"state":"active"}`, ExpectedVersion: v}, &verbsv1.SharedSetResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a seat writing rig.desk")
	err = a.Call(recordCtx(t), "rig.shared.delete", &verbsv1.SharedDeleteRequest{Key: deskKey, ExpectedVersion: v}, &verbsv1.SharedDeleteResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "a seat deleting rig.desk")
}
