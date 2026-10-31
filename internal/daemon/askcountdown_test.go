package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/coord"
	"github.com/borismilner/rig/internal/record"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

func countdownRequest(key string) *registryv1.AskOpenRequest {
	return &registryv1.AskOpenRequest{Key: key, Kind: "work.handoff", Urgency: "normal", Options: []string{"Stop"}, TtlSeconds: 60, Title: "Handing off this work", FailureMode: askCountdown}
}

func countdownDaemon(t *testing.T) (string, *Daemon) {
	t.Helper()
	return upRecordDaemonWith(t, func(d *Daemon) {
		st, err := coord.Open("countdown")
		if err != nil {
			t.Fatal(err)
		}
		d.leases = st
		t.Cleanup(func() { _ = st.Close() })
	})
}

func openCountdown(t *testing.T, c *client.Client, key string) *registryv1.Ask {
	t.Helper()
	var got registryv1.AskOpenResponse
	if err := c.Call(recordCtx(t), "rig.ask.open", countdownRequest(key), &got); err != nil {
		t.Fatal(err)
	}
	return got.GetAsk()
}

func presentCountdown(t *testing.T, c *client.Client, id string) *registryv1.Ask {
	t.Helper()
	var got registryv1.AskPresentedResponse
	if err := c.Call(recordCtx(t), "rig.ask.presented", &registryv1.AskPresentedRequest{Id: id}, &got); err != nil {
		t.Fatal(err)
	}
	return got.GetAsk()
}

func readCountdown(t *testing.T, c *client.Client, id string) *registryv1.Ask {
	t.Helper()
	var got registryv1.AskGetResponse
	if err := c.Call(recordCtx(t), "rig.ask.get", &registryv1.AskGetRequest{Id: id}, &got); err != nil {
		t.Fatal(err)
	}
	return got.GetAsk()
}

// Inject clock discontinuities in persisted input, without depending on wall
// sleeps or changing the machine's clock. Production reads coord's BOOTTIME.
func changeCountdown(t *testing.T, d *Daemon, id string, fields map[string]string) {
	t.Helper()
	d.askMu.Lock()
	defer d.askMu.Unlock()
	ctx := recordCtx(t)
	r, err := d.records.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		r.Fields[k] = v
	}
	_, err = d.records.Put(ctx, record.PutRequest{ID: r.ID, IfVersion: r.Version, Kind: r.Kind, Project: r.Project, Body: r.Body, Fields: r.Fields, Session: "test", Seat: "test", Epoch: 1})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCountdownNeedsPresentationAndOnlyDurableExpiryProceeds(t *testing.T) {
	sock, d := countdownDaemon(t)
	c := seated(t, sock, "worker")
	a := openCountdown(t, c, "handoff")
	if a.GetState() != "awaiting_presentation" || a.GetExpiresUnixNano() != 0 {
		t.Fatalf("unarmed: %v", a)
	}
	var pending registryv1.AskListResponse
	if err := c.Call(recordCtx(t), "rig.ask.list", &registryv1.AskListRequest{OpenOnly: true}, &pending); err != nil {
		t.Fatal(err)
	}
	if len(pending.GetAsks()) != 1 || pending.GetAsks()[0].GetId() != a.GetId() {
		t.Fatal("unpresented request absent from open list")
	}
	again := openCountdown(t, c, "handoff")
	if again.GetId() != a.GetId() {
		t.Fatal("retry duplicated countdown")
	}
	armed := presentCountdown(t, c, a.GetId())
	if armed.GetState() != "open" || armed.GetExpiresUnixNano() == 0 {
		t.Fatalf("armed: %v", armed)
	}
	repeated := presentCountdown(t, c, a.GetId())
	if repeated.GetExpiresUnixNano() != armed.GetExpiresUnixNano() {
		t.Fatal("receipt extended deadline")
	}
	changeCountdown(t, d, a.GetId(), map[string]string{"ask_expires": "1"})
	if got := readCountdown(t, c, a.GetId()); got.GetState() != "open" {
		t.Fatalf("wall jump changed state: %v", got)
	}
	if _, err := d.sweepAsks(recordCtx(t)); err != nil {
		t.Fatal(err)
	}
	if got := readCountdown(t, c, a.GetId()); got.GetState() != "open" {
		t.Fatalf("wall clock expired BOOTTIME: %v", got)
	}
	changeCountdown(t, d, a.GetId(), map[string]string{"countdown_deadline": "1"})
	if got := readCountdown(t, c, a.GetId()); got.GetState() != "open" {
		t.Fatalf("read approved before durable expiry: %v", got)
	}
	if _, err := d.sweepAsks(recordCtx(t)); err != nil {
		t.Fatal(err)
	}
	if got := readCountdown(t, c, a.GetId()); got.GetState() != "proceed" || !got.GetAnswer().GetExpired() {
		t.Fatalf("expiry: %v", got)
	}
	err := c.Call(recordCtx(t), "rig.ask.answer", &registryv1.AskAnswerRequest{Id: a.GetId(), Option: "Stop"}, &registryv1.AskAnswerResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "late Stop")
}

func TestCountdownMissingPresentationRebootAndPolicyDeny(t *testing.T) {
	sock, d := countdownDaemon(t)
	c := seated(t, sock, "worker")
	for _, tc := range []struct {
		name      string
		presented bool
		fields    map[string]string
	}{
		{"unseen", false, map[string]string{"countdown_deadline": "1"}},
		{"reboot", true, map[string]string{"countdown_boot": "another-boot"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := openCountdown(t, c, tc.name)
			if tc.presented {
				presentCountdown(t, c, a.GetId())
			}
			changeCountdown(t, d, a.GetId(), tc.fields)
			if _, err := d.sweepAsks(recordCtx(t)); err != nil {
				t.Fatal(err)
			}
			if got := readCountdown(t, c, a.GetId()); got.GetState() != "denied" {
				t.Fatalf("unsafe expiry: %v", got)
			}
		})
	}
	if err := c.Call(recordCtx(t), "rig.toast.dnd", &registryv1.ToastDndRequest{Change: registryv1.DndChange_DND_CHANGE_ON}, &registryv1.ToastDndResponse{}); err != nil {
		t.Fatal(err)
	}
	if a := openCountdown(t, c, "dnd"); a.GetState() != "denied" {
		t.Fatalf("suppressed: %v", a)
	}
}

func TestCountdownStopDismissWithdrawAndReceiptAuthority(t *testing.T) {
	sock, _ := countdownDaemon(t)
	c := seated(t, sock, "worker")
	other := seated(t, sock, "other")
	unattended := program(t, sock, "unattended")
	for _, tc := range []struct {
		name              string
		dismiss, withdraw bool
		want              string
	}{
		{"stop", false, false, "stopped"}, {"dismiss", true, false, "stopped"}, {"withdraw", false, true, "withdrawn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := openCountdown(t, c, tc.name)
			err := unattended.Call(recordCtx(t), "rig.ask.presented", &registryv1.AskPresentedRequest{Id: a.GetId()}, &registryv1.AskPresentedResponse{})
			wantCode(t, err, rigv1.Code_CODE_DENIED, "program receipt")
			if tc.withdraw {
				err = other.Call(recordCtx(t), "rig.ask.withdraw", &registryv1.AskWithdrawRequest{Id: a.GetId()}, &registryv1.AskWithdrawResponse{})
				wantCode(t, err, rigv1.Code_CODE_DENIED, "other sender withdrawal")
				err = c.Call(recordCtx(t), "rig.ask.withdraw", &registryv1.AskWithdrawRequest{Id: a.GetId()}, &registryv1.AskWithdrawResponse{})
			} else {
				req := &registryv1.AskAnswerRequest{Id: a.GetId(), Dismissed: tc.dismiss}
				if !tc.dismiss {
					req.Option = "Stop"
				}
				err = c.Call(recordCtx(t), "rig.ask.answer", req, &registryv1.AskAnswerResponse{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := presentCountdown(t, c, a.GetId()); got.GetState() != tc.want {
				t.Fatalf("receipt revived request: %v", got)
			}
			var answer registryv1.ToastAnswerResponse
			if err := c.Call(recordCtx(t), "rig.toast.answer", &registryv1.ToastAnswerRequest{RecordId: a.GetId(), TimeoutMs: 1000}, &answer); err != nil {
				t.Fatal(err)
			}
			if !answer.GetAnswer().GetAnswered() {
				t.Fatal("terminal state did not wake waiter")
			}
		})
	}
}

func TestCountdownOneTerminalReplyUnderConcurrentStopAndSweep(t *testing.T) {
	sock, d := countdownDaemon(t)
	c := seated(t, sock, "worker")
	a := openCountdown(t, c, "race")
	presentCountdown(t, c, a.GetId())
	changeCountdown(t, d, a.GetId(), map[string]string{"countdown_deadline": "1"})
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = c.Call(recordCtx(t), "rig.ask.answer", &registryv1.AskAnswerRequest{Id: a.GetId(), Option: "Stop"}, &registryv1.AskAnswerResponse{})
	})
	wg.Go(func() {
		if _, err := d.sweepAsks(recordCtx(t)); err != nil {
			t.Error(err)
		}
	})
	wg.Wait()
	replies, err := d.records.Find(recordCtx(t), record.QueryFilter{Project: notificationProject, Kind: replyKind, Field: "notification", Value: a.GetId()})
	if err != nil || len(replies) != 1 {
		t.Fatalf("terminal replies=%d, err=%v", len(replies), err)
	}
}

func TestCountdownWaitAndRestartKeepOutcomeAndDeadline(t *testing.T) {
	r := newRestartable(t)
	c := seated(t, r.sock, "worker")
	a := openCountdown(t, c, "restart")
	armed := presentCountdown(t, c, a.GetId())
	r.down()
	r.start(t)
	c = seated(t, r.sock, "worker")
	again := presentCountdown(t, c, a.GetId())
	if again.GetExpiresUnixNano() != armed.GetExpiresUnixNano() {
		t.Fatal("restart extended grace")
	}
	req := countdownRequest("short")
	req.TtlSeconds = 1
	var opened registryv1.AskOpenResponse
	if err := c.Call(recordCtx(t), "rig.ask.open", req, &opened); err != nil {
		t.Fatal(err)
	}
	presentCountdown(t, c, opened.GetAsk().GetId())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var answer registryv1.ToastAnswerResponse
	if err := c.Call(ctx, "rig.toast.answer", &registryv1.ToastAnswerRequest{RecordId: opened.GetAsk().GetId(), TimeoutMs: 2500}, &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.GetAnswer().GetExpired() {
		t.Fatalf("blocking wait: %v", answer.GetAnswer())
	}
	r.down()
	r.start(t)
	c = seated(t, r.sock, "worker")
	if got := readCountdown(t, c, opened.GetAsk().GetId()); got.GetState() != "proceed" {
		t.Fatalf("restart forgot outcome: %v", got)
	}
}

func TestCountdownValidationNeverOffersAnApprovalButton(t *testing.T) {
	for _, tc := range []struct {
		options []string
		text    bool
	}{{[]string{"Proceed", "Stop"}, false}, {[]string{"Stop"}, true}, {nil, false}} {
		q := countdownRequest("invalid")
		q.Options = tc.options
		q.AcceptsText = tc.text
		if checkAskOpen(q) == "" {
			t.Fatal("invalid countdown accepted")
		}
	}
}

func TestCountdownWithdrawalEventSurvivesRestart(t *testing.T) {
	r := newRestartable(t)
	c := seated(t, r.sock, "worker")
	a := openCountdown(t, c, "withdraw-event")
	var mark registryv1.EventsWaitResponse
	if err := c.Call(recordCtx(t), "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{toastAnsweredKind}, TimeoutMs: 1}, &mark); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(recordCtx(t), "rig.ask.withdraw", &registryv1.AskWithdrawRequest{Id: a.GetId()}, &registryv1.AskWithdrawResponse{}); err != nil {
		t.Fatal(err)
	}
	r.down()
	r.start(t)
	c = seated(t, r.sock, "worker")
	var events registryv1.EventsWaitResponse
	if err := c.Call(recordCtx(t), "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{toastAnsweredKind}, After: mark.GetLatest(), Epoch: mark.GetEpoch(), TimeoutMs: 500}, &events); err != nil {
		t.Fatal(err)
	}
	if events.GetGap() || len(events.GetEvents()) != 1 {
		t.Fatalf("withdrawal wake after restart: %v", &events)
	}
	if got := readCountdown(t, c, a.GetId()); got.GetState() != "withdrawn" {
		t.Fatalf("withdrawal state: %v", got)
	}
}
