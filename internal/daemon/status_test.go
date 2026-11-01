package daemon

import (
	"strings"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

func TestStatusUpdatesAreBoundedAndOwned(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	owner := seated(t, sock, "backend-1")
	other := seated(t, sock, "backend-2")

	var put registryv1.StatusPutResponse
	if err := owner.Call(ctx, "rig.status.put", &registryv1.StatusPutRequest{
		Text: "build started\nstep 1 of 2", TtlMs: 1000,
	}, &put); err != nil {
		t.Fatal(err)
	}
	first := put.GetStatus()
	if first.GetId() == "" || first.GetOwner() != "backend-1" || first.GetText() != "build started\nstep 1 of 2" ||
		first.GetExpiresUnixNano()-first.GetUpdatedUnixNano() != int64(time.Second) {
		t.Fatalf("status put: %v", first)
	}

	if err := owner.Call(ctx, "rig.status.put", &registryv1.StatusPutRequest{Id: first.GetId(), Text: "build finished"}, &put); err != nil {
		t.Fatal(err)
	}
	if put.GetStatus().GetId() != first.GetId() || put.GetStatus().GetText() != "build finished" {
		t.Fatalf("status replace made a duplicate: %v", put.GetStatus())
	}
	err := other.Call(ctx, "rig.status.put", &registryv1.StatusPutRequest{Id: first.GetId(), Text: "spoof"}, &put)
	wantCode(t, err, rigv1.Code_CODE_DENIED, "another owner replaced a status")
	var list registryv1.StatusListResponse
	if err := other.Call(ctx, "rig.status.list", &registryv1.StatusListRequest{}, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.GetStatuses()) != 1 || list.GetStatuses()[0].GetId() != first.GetId() {
		t.Fatalf("status list contains duplicates or misses the update: %v", list.GetStatuses())
	}
	if err := owner.Call(ctx, "rig.status.remove", &registryv1.StatusRemoveRequest{Id: first.GetId()}, &registryv1.StatusRemoveResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Call(ctx, "rig.status.list", &registryv1.StatusListRequest{}, &list); err != nil || len(list.GetStatuses()) != 0 {
		t.Fatalf("removed status remains active: %v, %v", list.GetStatuses(), err)
	}
	if err := owner.Call(ctx, "rig.status.put", &registryv1.StatusPutRequest{Text: "remove from the desktop"}, &put); err != nil {
		t.Fatal(err)
	}
	terminal := dial(t, sock)
	if err := terminal.Call(ctx, "rig.status.remove", &registryv1.StatusRemoveRequest{Id: put.GetStatus().GetId()}, &registryv1.StatusRemoveResponse{}); err != nil {
		t.Fatalf("unannounced desktop user could not dismiss an update: %v", err)
	}
}

func TestStatusTextAndExpiryBounds(t *testing.T) {
	for _, s := range []string{"", "  ", strings.Repeat("x", statusMaxText+1), "a\nb\nc\nd\ne", "bad\x00text"} {
		if validStatusText(s) == "" {
			t.Errorf("accepted invalid status text %q", s)
		}
	}
	for _, s := range []string{"working", "line one\nline two", strings.Repeat("x", statusMaxText)} {
		if why := validStatusText(s); why != "" {
			t.Errorf("refused valid status text (%d bytes): %s", len(s), why)
		}
	}
	for _, ms := range []uint32{999, uint32((24*time.Hour)/time.Millisecond) + 1} {
		if _, err := statusTTL(ms); err == nil {
			t.Errorf("accepted expiry %dms", ms)
		}
	}
	if ttl, err := statusTTL(0); err != nil || ttl != statusDefaultTTL {
		t.Fatalf("default expiry: %v, %v", ttl, err)
	}

	now := time.Now()
	d := &Daemon{statuses: map[string]*statusEntry{
		"expired": {status: &registryv1.TransientStatus{ExpiresUnixNano: now.Add(-time.Second).UnixNano()}},
		"active":  {status: &registryv1.TransientStatus{ExpiresUnixNano: now.Add(time.Minute).UnixNano()}},
	}}
	if wait := d.expireStatusesLocked(now); wait <= 0 || len(d.statuses) != 1 || d.statuses["active"] == nil {
		t.Fatalf("expiry sweep kept stale content or lost its next deadline: wait=%s statuses=%v", wait, d.statuses)
	}
}
