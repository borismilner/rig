package daemon

import (
	"context"
	"strconv"
	"testing"
	"time"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

func askRequest(key string) *registryv1.AskOpenRequest {
	return &registryv1.AskOpenRequest{
		Key: key, Kind: "otp.release", Urgency: "normal", Refs: []string{"phone/inbox/42"},
		Options: []string{"Allow", "Deny"}, TtlSeconds: 60, Title: "Release a code?",
	}
}

func TestAskOpenIsIdempotentAndOneAnswerWins(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	sender := seated(t, sock, "phone")
	person := seated(t, sock, "boris")
	programCaller := program(t, sock, "unattended")
	req := askRequest("request-42")
	var first, again registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", req, &first); err != nil {
		t.Fatal(err)
	}
	if err := sender.Call(ctx, "rig.ask.open", req, &again); err != nil {
		t.Fatal(err)
	}
	if first.GetAsk().GetId() == "" || again.GetAsk().GetId() != first.GetAsk().GetId() {
		t.Fatalf("retry made a second question: %v, %v", first.GetAsk(), again.GetAsk())
	}
	err := programCaller.Call(ctx, "rig.ask.answer", &registryv1.AskAnswerRequest{
		Id: first.GetAsk().GetId(), Option: "Allow",
	}, &registryv1.AskAnswerResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "unattended program answering")
	changed := askRequest("request-42")
	changed.Title = "Different question"
	err = sender.Call(ctx, "rig.ask.open", changed, &registryv1.AskOpenResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "one key, different contents")

	var answered registryv1.AskAnswerResponse
	if err := person.Call(ctx, "rig.ask.answer", &registryv1.AskAnswerRequest{
		Id: first.GetAsk().GetId(), Option: "Deny",
	}, &answered); err != nil {
		t.Fatal(err)
	}
	if a := answered.GetAsk(); a.GetState() != "answered" || a.GetAnswer().GetReply() != "Deny" ||
		a.GetAnswer().GetBy() != "boris" {
		t.Fatalf("answer was %v", a)
	}
	err = person.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{
		RecordId: first.GetAsk().GetId(), Reply: "Allow",
	}, &registryv1.ToastReplyResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "second answer")

	var listed registryv1.AskListResponse
	if err := sender.Call(ctx, "rig.ask.list", &registryv1.AskListRequest{Sender: "phone"}, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.GetAsks()) != 1 || listed.GetAsks()[0].GetId() != first.GetAsk().GetId() {
		t.Fatalf("list returned %v", listed.GetAsks())
	}
}

func TestGatingAskBypassesDNDAndDisambiguatingAskIsHeld(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	sender := seated(t, sock, "worker")
	if err := sender.Call(ctx, "rig.toast.dnd", &registryv1.ToastDndRequest{
		Change: registryv1.DndChange_DND_CHANGE_ON,
	}, &registryv1.ToastDndResponse{}); err != nil {
		t.Fatal(err)
	}
	var gating registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", askRequest("gate"), &gating); err != nil {
		t.Fatal(err)
	}
	if gating.GetAsk().GetDeliveryNote() != "shown on desktop" {
		t.Fatalf("gating delivery: %s", gating.GetAsk().GetDeliveryNote())
	}
	other := askRequest("clarify")
	other.FailureMode = "disambiguating"
	var held registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", other, &held); err != nil {
		t.Fatal(err)
	}
	if held.GetAsk().GetDeliveryNote() != "held by do not disturb" {
		t.Fatalf("disambiguating delivery: %s", held.GetAsk().GetDeliveryNote())
	}
	var ring registryv1.ToastWaitResponse
	if err := sender.Call(ctx, "rig.toast.wait", &registryv1.ToastWaitRequest{}, &ring); err != nil {
		t.Fatal(err)
	}
	if len(ring.GetToasts()) != 1 || ring.GetToasts()[0].GetRecordId() != gating.GetAsk().GetId() {
		t.Fatalf("DND showed %v", ring.GetToasts())
	}
}

func TestAskExpiryAndWithdrawalSurviveRestart(t *testing.T) {
	r := newRestartable(t)
	ctx := recordCtx(t)
	sender := seated(t, r.sock, "phone")
	soon := askRequest("soon")
	soon.TtlSeconds = 1
	var opened registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", soon, &opened); err != nil {
		t.Fatal(err)
	}
	other := askRequest("withdrawn")
	var withdrawn registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", other, &withdrawn); err != nil {
		t.Fatal(err)
	}
	var took registryv1.AskWithdrawResponse
	if err := sender.Call(ctx, "rig.ask.withdraw", &registryv1.AskWithdrawRequest{
		Id: withdrawn.GetAsk().GetId(),
	}, &took); err != nil {
		t.Fatal(err)
	}
	if took.GetAsk().GetState() != "withdrawn" {
		t.Fatalf("withdrawal: %v", took.GetAsk())
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var state registryv1.AskGetResponse
		if err := sender.Call(ctx, "rig.ask.get", &registryv1.AskGetRequest{Id: opened.GetAsk().GetId()}, &state); err != nil {
			t.Fatal(err)
		}
		if state.GetAsk().GetAnswer().GetExpired() {
			break // the expiry transition was filed before restart
		}
		if time.Now().After(deadline) {
			t.Fatalf("expiry was not filed: %v", state.GetAsk())
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.down()
	r.start(t)
	sender = seated(t, r.sock, "phone")
	person := seated(t, r.sock, "boris")
	var expired registryv1.AskGetResponse
	if err := sender.Call(ctx, "rig.ask.get", &registryv1.AskGetRequest{Id: opened.GetAsk().GetId()}, &expired); err != nil {
		t.Fatal(err)
	}
	if expired.GetAsk().GetState() != "denied" {
		t.Fatalf("gating expiry: %v", expired.GetAsk())
	}
	err := person.Call(ctx, "rig.ask.answer", &registryv1.AskAnswerRequest{
		Id: opened.GetAsk().GetId(), Option: "Allow",
	}, &registryv1.AskAnswerResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "late answer")
	err = person.Call(ctx, "rig.toast.reply", &registryv1.ToastReplyRequest{
		RecordId: withdrawn.GetAsk().GetId(), Reply: "Allow",
	}, &registryv1.ToastReplyResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "withdrawn answer")
	var retry registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", other, &retry); err != nil {
		t.Fatal(err)
	}
	if retry.GetAsk().GetId() != withdrawn.GetAsk().GetId() || retry.GetAsk().GetState() != "withdrawn" {
		t.Fatalf("withdrawn retry: %v", retry.GetAsk())
	}
}

func TestAskPolicyNeverRateAndChannelChoice(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	sender := seated(t, sock, "worker")
	var set registryv1.ConfigSetResponse
	if err := sender.Call(ctx, "rig.config.set", &registryv1.ConfigSetRequest{
		ValuesJson: map[string]string{"ask.policies": `[
			{"kind":"otp.release","mode":"never"},
			{"kind":"status.confirm","mode":"ordinary","max_per_hour":1},
			{"kind":"phone.only","mode":"always","urgent_channels":["phone"]}
		]`},
	}, &set); err != nil {
		t.Fatal(err)
	}
	var denied registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", askRequest("never"), &denied); err != nil {
		t.Fatal(err)
	}
	if a := denied.GetAsk(); a.GetState() != "denied" || a.GetChannel() != "none" {
		t.Fatalf("never rule: %v", a)
	}
	one := askRequest("rate-1")
	one.Kind = "status.confirm"
	one.FailureMode = "disambiguating"
	var first registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", one, &first); err != nil {
		t.Fatal(err)
	}
	two := askRequest("rate-2")
	two.Kind = "status.confirm"
	two.FailureMode = "disambiguating"
	var second registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", two, &second); err != nil {
		t.Fatal(err)
	}
	if first.GetAsk().GetDeliveryNote() != "shown on desktop" ||
		second.GetAsk().GetDeliveryNote() != "held by hourly rate limit" {
		t.Fatalf("rate delivery: %s, %s", first.GetAsk().GetDeliveryNote(), second.GetAsk().GetDeliveryNote())
	}
	phone := askRequest("phone-only")
	phone.Kind, phone.Urgency = "phone.only", "urgent"
	var unavailable registryv1.AskOpenResponse
	if err := sender.Call(ctx, "rig.ask.open", phone, &unavailable); err != nil {
		t.Fatal(err)
	}
	if a := unavailable.GetAsk(); a.GetChannel() != "none" || a.GetDeliveryNote() != "requested channels unavailable on this estate" {
		t.Fatalf("channel choice: %v", a)
	}
}

func TestAnOlderAskCanBeAnsweredAfterTheToastRingDropsIt(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sender := seated(t, sock, "worker")
	person := seated(t, sock, "boris")
	var oldest string
	for i := range maxToastAsks + 1 {
		var opened registryv1.AskOpenResponse
		if err := sender.Call(ctx, "rig.ask.open", askRequest("many-"+strconv.Itoa(i)), &opened); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldest = opened.GetAsk().GetId()
		}
	}
	var answer registryv1.AskAnswerResponse
	if err := person.Call(ctx, "rig.ask.answer", &registryv1.AskAnswerRequest{
		Id: oldest, Option: "Deny",
	}, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.GetAsk().GetState() != "answered" {
		t.Fatalf("old question was %v", answer.GetAsk())
	}
}

func TestQuietHoursCrossMidnight(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jerusalem")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		hour, minute int
		quiet        bool
	}{
		{22, 0, true}, {23, 59, true}, {6, 59, true}, {7, 0, false}, {12, 0, false},
	} {
		now := time.Date(2026, 10, 13, tc.hour, tc.minute, 0, 0, loc)
		if got := inQuietHours(now, "22:00", "07:00"); got != tc.quiet {
			t.Fatalf("%s quiet=%v, want %v", now.Format("15:04"), got, tc.quiet)
		}
	}
}
