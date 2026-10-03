package daemon

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"

	"github.com/borismilner/rig/internal/record"
)

// plan/55's board: panel.put, panel.list, panel.act.

// ⛔ A CARD IS CHANGED ONLY BY WHOEVER PUT IT, A CHANGE CARRIES ONLY WHAT IT
// SAYS, AND EVERY VERSION IS KEPT: a close is the last version, never a
// delete, and a closed card refuses another change.
func TestACardIsItsOwnersAndEveryVersionIsKept(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	owner := seated(t, sock, "backend-1")
	other := seated(t, sock, "backend-2")

	var put verbsv1.PanelPutResponse
	if err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{
		Title: proto.String("graft run"), Status: proto.String("starting"), Severity: proto.String("info"),
		Progress: proto.Float64(0.25), Facts: &verbsv1.PanelFacts{Facts: []*verbsv1.PanelFact{{Label: "jobs", Value: "3"}}},
		Actions: &verbsv1.PanelActions{Labels: []string{"Stop"}},
	}, &put); err != nil {
		t.Fatal(err)
	}
	card := put.GetCard()
	if card.GetId() == "" || card.GetFrom() != "backend-1" || card.GetProgress() != 0.25 || !card.GetHasProgress() ||
		len(card.GetFacts()) != 1 || card.GetActions()[0] != "Stop" {
		t.Fatalf("put %v", card)
	}

	err := other.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Card: card.GetId(), Status: proto.String("mine now")}, &verbsv1.PanelPutResponse{})
	wantCode(t, err, rigv1.Code_CODE_DENIED, "another seat changed the card")

	if err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{
		Card: card.GetId(), Status: proto.String("halfway"), ClearProgress: true, Busy: proto.Bool(true),
	}, &put); err != nil {
		t.Fatal(err)
	}
	if c := put.GetCard(); c.GetTitle() != "graft run" || c.GetStatus() != "halfway" || c.GetHasProgress() || !c.GetBusy() ||
		len(c.GetFacts()) != 1 || c.GetVersion() != card.GetVersion()+1 {
		t.Fatalf("a change kept the wrong fields: %v", c)
	}

	if err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Card: card.GetId(), Close: true}, &put); err != nil {
		t.Fatal(err)
	}
	err = owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Card: card.GetId(), Status: proto.String("again")}, &verbsv1.PanelPutResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a closed card was changed")

	var list verbsv1.PanelListResponse
	if err := other.Call(ctx, "rig.panel.list", &verbsv1.PanelListRequest{}, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.GetCards()) != 1 || !list.GetCards()[0].GetClosed() {
		t.Fatalf("the board after a close: %v", list.GetCards())
	}
}

// The caps are refused before anything is written, and a new card needs a
// title. Each case names the one field that is wrong.
func TestTheCardCapsAreRefused(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	owner := seated(t, sock, "backend-1")
	facts := func(n int) *verbsv1.PanelFacts {
		fs := &verbsv1.PanelFacts{}
		for range n {
			fs.Facts = append(fs.Facts, &verbsv1.PanelFact{Label: "l", Value: "v"})
		}
		return fs
	}
	for _, r := range []struct {
		req *verbsv1.PanelPutRequest
		why string
	}{
		{&verbsv1.PanelPutRequest{}, "no title"},
		{&verbsv1.PanelPutRequest{Title: proto.String("")}, "an empty title"},
		{&verbsv1.PanelPutRequest{Title: proto.String(strings.Repeat("x", 201))}, "a title over 200"},
		{&verbsv1.PanelPutRequest{Title: proto.String("t"), Body: proto.String(strings.Repeat("x", 4097))}, "a body over 4096"},
		{&verbsv1.PanelPutRequest{Title: proto.String("t"), Severity: proto.String("urgent")}, "an unknown severity"},
		{&verbsv1.PanelPutRequest{Title: proto.String("t"), Progress: proto.Float64(1.5)}, "progress over 1"},
		{&verbsv1.PanelPutRequest{Title: proto.String("t"), Facts: facts(7)}, "7 facts"},
		{&verbsv1.PanelPutRequest{Title: proto.String("t"), Actions: &verbsv1.PanelActions{Labels: []string{"a", "b", "c", "d"}}}, "4 actions"},
		{&verbsv1.PanelPutRequest{Title: proto.String("t"), Actions: &verbsv1.PanelActions{Labels: []string{"a", "a"}}}, "a repeated action"},
	} {
		err := owner.Call(ctx, "rig.panel.put", r.req, &verbsv1.PanelPutResponse{})
		wantCode(t, err, rigv1.Code_CODE_INVALID, r.why)
	}
	err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Card: "rec-none", Status: proto.String("s")}, &verbsv1.PanelPutResponse{})
	wantCode(t, err, rigv1.Code_CODE_NOT_FOUND, "a change to no card")
}

// Past 40 open cards a sender's oldest is closed, so a runaway sender
// cannot grow the board without end; another sender's are not touched.
func TestASendersOldestCardIsClosedPastTheCap(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	owner := seated(t, sock, "backend-1")
	other := seated(t, sock, "backend-2")
	var first, theirs verbsv1.PanelPutResponse
	if err := other.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Title: proto.String("theirs")}, &theirs); err != nil {
		t.Fatal(err)
	}
	for i := range maxOpenPanelCards + 1 {
		var put verbsv1.PanelPutResponse
		if err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Title: proto.String("card")}, &put); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first.Card = put.GetCard()
		}
	}
	var list verbsv1.PanelListResponse
	if err := owner.Call(ctx, "rig.panel.list", &verbsv1.PanelListRequest{}, &list); err != nil {
		t.Fatal(err)
	}
	open := map[string]int{}
	for _, c := range list.GetCards() {
		if !c.GetClosed() {
			open[c.GetFrom()]++
		}
		if c.GetId() == first.GetCard().GetId() && !c.GetClosed() {
			t.Fatal("the oldest card is still open past the cap")
		}
	}
	if open["backend-1"] != maxOpenPanelCards || open["backend-2"] != 1 {
		t.Fatalf("open cards per sender: %v", open)
	}
	var mine verbsv1.PanelListResponse
	if err := owner.Call(ctx, "rig.panel.list", &verbsv1.PanelListRequest{From: "backend-2"}, &mine); err != nil {
		t.Fatal(err)
	}
	if len(mine.GetCards()) != 1 || mine.GetCards()[0].GetId() != theirs.GetCard().GetId() {
		t.Fatalf("one sender's board: %v", mine.GetCards())
	}
}

// ⛔ A PRESS GOES TO THE CARD'S OWNER, AND ONLY FOR AN ACTION THE OPEN CARD
// OFFERS: it is filed as card-act and posted as panel.acted addressed to
// the owner's seat, so another seat waiting on the bus does not see it.
func TestAPressReachesTheCardsOwner(t *testing.T) {
	sock := upRecordDaemon(t)
	ctx := recordCtx(t)
	owner := seated(t, sock, "backend-1")
	other := seated(t, sock, "backend-2")
	person := seated(t, sock, "boris")

	var put verbsv1.PanelPutResponse
	if err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{
		Title: proto.String("deploy"), Actions: &verbsv1.PanelActions{Labels: []string{"Stop", "Retry"}},
	}, &put); err != nil {
		t.Fatal(err)
	}
	id := put.GetCard().GetId()

	err := person.Call(ctx, "rig.panel.act", &verbsv1.PanelActRequest{Card: id, Action: "Delete"}, &verbsv1.PanelActResponse{})
	wantCode(t, err, rigv1.Code_CODE_INVALID, "an action the card does not offer")
	var act verbsv1.PanelActResponse
	if err := person.Call(ctx, "rig.panel.act", &verbsv1.PanelActRequest{Card: id, Action: "Retry"}, &act); err != nil {
		t.Fatal(err)
	}
	if act.GetBy() != "boris" || act.GetTo() != "backend-1" || act.GetAction() != "Retry" {
		t.Fatalf("the press %v", &act)
	}

	var got registryv1.EventsWaitResponse
	if err := owner.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{"panel.acted"}, TimeoutMs: 1000}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.GetEvents()) != 1 || !strings.Contains(got.GetEvents()[0].GetPayloadJson(), `"Retry"`) {
		t.Fatalf("the owner waited and got %v", &got)
	}
	var theirs registryv1.EventsWaitResponse
	if err := other.Call(ctx, "rig.events.wait", &registryv1.EventsWaitRequest{Kinds: []string{"panel.acted"}, TimeoutMs: 200}, &theirs); err != nil {
		t.Fatal(err)
	}
	if len(theirs.GetEvents()) != 0 {
		t.Fatalf("another seat saw the press: %v", &theirs)
	}

	var filed verbsv1.RecordQueryResponse
	if err := person.Call(ctx, "rig.record.query", &verbsv1.RecordQueryRequest{Project: panelProject, Kind: panelActKind}, &filed); err != nil {
		t.Fatal(err)
	}
	if rs := filed.GetRecords(); len(rs) != 1 || rs[0].GetFields()["by"] != "boris" || rs[0].GetFields()["card"] != id {
		t.Fatalf("the press as filed: %v", rs)
	}

	if err := owner.Call(ctx, "rig.panel.put", &verbsv1.PanelPutRequest{Card: id, Close: true}, &put); err != nil {
		t.Fatal(err)
	}
	err = person.Call(ctx, "rig.panel.act", &verbsv1.PanelActRequest{Card: id, Action: "Retry"}, &verbsv1.PanelActResponse{})
	wantCode(t, err, rigv1.Code_CODE_CONFLICT, "a press on a closed card")
}

// The board shows a closed card for a day, then forgets it; newest first.
func TestTheBoardShowsAClosedCardForADay(t *testing.T) {
	now := time.Now()
	rec := func(id string, updated time.Time, closed string) record.Record {
		return record.Record{ID: id, Fields: map[string]string{
			"updated": strconv.FormatInt(updated.UnixNano(), 10), "closed": closed, "title": id,
		}}
	}
	got := boardOf([]record.Record{
		rec("old-open", now.Add(-48*time.Hour), "no"),
		rec("old-closed", now.Add(-25*time.Hour), "yes"),
		rec("new-closed", now.Add(-time.Hour), "yes"),
		rec("newest", now, "no"),
	}, now)
	var ids []string
	for _, c := range got {
		ids = append(ids, c.GetId())
	}
	if strings.Join(ids, ",") != "newest,new-closed,old-open" {
		t.Fatalf("the board is %v", ids)
	}
}
