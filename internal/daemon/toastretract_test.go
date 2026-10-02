package daemon

import (
	"slices"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// plan/53 slice 7: toast.retract.

// ⛔ A SENDER TAKES BACK ITS OWN TOAST, AND ONLY ITS OWN, AND THE RECORD
// KEEPS BOTH: the toast is retracted with the reason, not deleted, and the
// tray is handed the withdrawal so the bubble leaves.
func TestASenderTakesBackItsToastAndTheRecordSaysSo(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	sender := seated(t, sock, "backend-1")
	other := seated(t, sock, "backend-2")
	tray := dial(t, sock)

	var sent registryv1.NotifyResponse
	if err := sender.Call(ctx, "rig.notify", &registryv1.NotifyRequest{
		Severity: registryv1.Severity_SEVERITY_WARNING, Title: "build failed",
	}, &sent); err != nil {
		t.Fatal(err)
	}
	id := sent.GetToast().GetRecordId()

	err := other.Call(ctx, "rig.toast.retract", &registryv1.ToastRetractRequest{RecordId: id}, &registryv1.ToastRetractResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "another seat took back the toast")
	var none registryv1.ToastRetractResponse
	if err := other.Call(ctx, "rig.toast.retract", &registryv1.ToastRetractRequest{}, &none); err != nil || len(none.GetRecordIds()) != 0 {
		t.Fatalf("another seat's sweep took back %v: %v", none.GetRecordIds(), err)
	}

	var got registryv1.ToastRetractResponse
	if err := sender.Call(ctx, "rig.toast.retract", &registryv1.ToastRetractRequest{RecordId: id, Reason: "fixed in 3f2a"}, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.GetRecordIds(), []string{id}) {
		t.Fatalf("took back %v, want %s", got.GetRecordIds(), id)
	}

	var rec verbsv1.RecordGetResponse
	if err := sender.Call(ctx, "rig.record.get", &verbsv1.RecordGetRequest{Id: id}, &rec); err != nil {
		t.Fatalf("the toast left the record: %v", err)
	}
	if r := rec.GetRecord().GetRetraction(); !strings.Contains(r.GetReason(), "fixed in 3f2a") {
		t.Fatalf("the record does not say it was taken back: %+v", rec.GetRecord())
	}

	var ring registryv1.ToastWaitResponse
	if err := tray.Call(ctx, "rig.toast.wait", &registryv1.ToastWaitRequest{After: sent.GetToast().GetSeq()}, &ring); err != nil {
		t.Fatal(err)
	}
	if ts := ring.GetToasts(); len(ts) != 1 || !ts[0].GetRetracted() || ts[0].GetRecordId() != id {
		t.Fatalf("the tray was handed %+v, want the withdrawal of %s", ts, id)
	}

	var again registryv1.ToastRetractResponse
	if err := sender.Call(ctx, "rig.toast.retract", &registryv1.ToastRetractRequest{RecordId: id}, &again); err != nil {
		t.Fatal(err)
	}
	if len(again.GetRecordIds()) != 0 || again.GetNote() == "" {
		t.Fatalf("a second retraction was taken as a new one: %+v", &again)
	}
}

// ⛔ A WAITER ON THE REPLY LEARNS THE TOAST WAS TAKEN BACK, and a late reply
// is refused. AgentBox's dismissal left the waiter waiting. A toast already
// answered was dealt with, so it is not taken back.
func TestATakenBackAskTellsItsWaiterAndRefusesALateReply(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	sender := seated(t, sock, "backend-1")
	person := seated(t, sock, "boris")

	ask := func(title string) string {
		var sent registryv1.NotifyResponse
		if err := sender.Call(ctx, "rig.notify", &registryv1.NotifyRequest{
			Severity: registryv1.Severity_SEVERITY_INFO, Title: title, Replies: []string{"Yes", "No"},
		}, &sent); err != nil {
			t.Fatal(err)
		}
		return sent.GetToast().GetRecordId()
	}
	answeredID, openID := ask("deploy now?"), ask("run the migration?")
	if err := person.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{RecordId: answeredID, Reply: "Yes"}, &registryv1.ToastReplyResponse{}); err != nil {
		t.Fatal(err)
	}

	woke := make(chan *registryv1.ToastAnswer, 1)
	go func() {
		var resp registryv1.ToastAnswerResponse
		if err := sender.Call(ctx, "rig.toast.answer", &registryv1.ToastAnswerRequest{RecordId: openID, TimeoutMs: 10_000}, &resp); err != nil {
			t.Errorf("rig.toast.answer: %v", err)
		}
		woke <- resp.GetAnswer()
	}()
	time.Sleep(50 * time.Millisecond)

	err := sender.Call(ctx, "rig.toast.retract", &registryv1.ToastRetractRequest{RecordId: answeredID}, &registryv1.ToastRetractResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "an answered toast was taken back")
	var got registryv1.ToastRetractResponse
	if err := sender.Call(ctx, "rig.toast.retract", &registryv1.ToastRetractRequest{}, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.GetRecordIds(), []string{openID}) {
		t.Fatalf("the sweep took back %v, want only the unanswered %s", got.GetRecordIds(), openID)
	}
	select {
	case a := <-woke:
		if !a.GetAnswered() || !a.GetWithdrawn() || a.GetBy() != "backend-1" {
			t.Fatalf("the waiter learned %+v", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter on the reply was not told")
	}
	err = person.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{RecordId: openID, Reply: "No"}, &registryv1.ToastReplyResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a reply to a toast taken back")
}

// With no id, only what may still be on screen is taken back: a toast past
// its dwell has closed by itself, and somebody else's is never touched.
func TestASweepTakesBackOnlyWhatIsStillOnScreen(t *testing.T) {
	var r toastRing
	now := time.Now()
	old := now.Add(-time.Hour).UnixNano()
	for _, toast := range []*registryv1.Toast{
		{RecordId: "fresh", Sender: "a", Severity: registryv1.Severity_SEVERITY_INFO, AtUnixNano: now.UnixNano()},
		{RecordId: "closed", Sender: "a", Severity: registryv1.Severity_SEVERITY_INFO, AtUnixNano: old},
		{RecordId: "urgent", Sender: "a", Severity: registryv1.Severity_SEVERITY_URGENT, AtUnixNano: old},
		{RecordId: "theirs", Sender: "b", Severity: registryv1.Severity_SEVERITY_INFO, AtUnixNano: now.UnixNano()},
	} {
		r.add(toast)
	}
	if got := r.open("a", now); !slices.Equal(got, []string{"fresh", "urgent"}) {
		t.Fatalf("open = %v, want fresh and urgent", got)
	}
	r.withdraw("fresh", "a")
	if got := r.open("a", now); !slices.Equal(got, []string{"urgent"}) {
		t.Fatalf("after the withdrawal open = %v", got)
	}
}
