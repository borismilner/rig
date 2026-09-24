package daemon

import (
	"context"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// A notification is filed in the record by the daemon, with the sender as
// provenance, and wakes a waiter already parked on rig.toast.wait.
func TestANotificationIsFiledAndWakesTheWaiter(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	sender := seated(t, sock, "backend-1")
	tray := dial(t, sock)

	woke := make(chan *registryv1.ToastWaitResponse, 1)
	go func() {
		var resp registryv1.ToastWaitResponse
		if err := tray.Call(ctx, "rig.toast.wait", &registryv1.ToastWaitRequest{TimeoutMs: 10_000}, &resp); err != nil {
			t.Errorf("rig.toast.wait: %v", err)
		}
		woke <- &resp
	}()
	time.Sleep(50 * time.Millisecond) // let the waiter park

	var sent registryv1.NotifyResponse
	if err := sender.Call(ctx, "rig.notify", &registryv1.NotifyRequest{
		Severity: registryv1.Severity_SEVERITY_WARNING, Title: "disk 91% full", Body: "/var is filling",
	}, &sent); err != nil {
		t.Fatalf("rig.notify: %v", err)
	}
	if sent.GetToast().GetSender() != "backend-1" || sent.GetToast().GetSeq() == 0 {
		t.Fatalf("sent %+v", sent.GetToast())
	}

	select {
	case got := <-woke:
		if len(got.GetToasts()) != 1 || got.GetToasts()[0].GetTitle() != "disk 91% full" {
			t.Fatalf("the waiter woke with %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter was not woken by the notification")
	}

	var rec verbsv1.RecordGetResponse
	if err := sender.Call(ctx, "rig.record.get", &verbsv1.RecordGetRequest{Id: sent.GetToast().GetRecordId()}, &rec); err != nil {
		t.Fatalf("the notification is not in the record: %v", err)
	}
	r := rec.GetRecord()
	if r.GetKind() != "notification" || r.GetFields()["severity"] != "warning" || r.GetProv().GetSeat() != "backend-1" {
		t.Fatalf("the record is %+v", r)
	}
}

func TestANotificationWithoutASeverityIsRefusedAndAWaitTimesOut(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	c := seated(t, sock, "backend-1")
	err := c.Call(ctx, "rig.notify", &registryv1.NotifyRequest{Title: "x"}, &registryv1.NotifyResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "a notification without a severity")

	start := time.Now()
	var resp registryv1.ToastWaitResponse
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := c.Call(wctx, "rig.toast.wait", &registryv1.ToastWaitRequest{TimeoutMs: 100}, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.GetToasts()) != 0 || time.Since(start) < 90*time.Millisecond {
		t.Fatalf("an idle wait answered %+v after %s", &resp, time.Since(start))
	}
}
