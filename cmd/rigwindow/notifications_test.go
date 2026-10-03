package main

import (
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

func rec(id string, at time.Time, body string, f map[string]string) *verbsv1.Record {
	return &verbsv1.Record{Id: id, Body: body, Fields: f, Prov: &verbsv1.Provenance{AtUnixNano: at.UnixNano()}}
}

func TestAssembleJoinsEvictsAndOrders(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	notes := []*verbsv1.Record{
		rec("01-old", now.Add(-8*24*time.Hour), "", map[string]string{"title": "old"}),
		rec("02-info", now.Add(-time.Hour), "b", map[string]string{"title": "info", "severity": "info", "sender": "shelf"}),
		rec("03-ask", now.Add(-2*time.Hour), "", map[string]string{"title": "deploy?", "replies": "yes | later"}),
		rec("04-text", now.Add(-30*time.Minute), "", map[string]string{"title": "why?", "replies": "", "reply_text": "yes"}),
		rec("05-dnd", now.Add(-10*time.Minute), "", map[string]string{"title": "quiet", "suppressed": "do not disturb"}),
	}
	replies := []*verbsv1.Record{
		rec("r1", now.Add(-90*time.Minute), "", map[string]string{"notification": "03-ask", "reply": "later", "by": "boris"}),
		rec("r2", now.Add(-80*time.Minute), "", map[string]string{"notification": "03-ask", "reply": "yes", "by": "late"}),
		rec("r3", now, "", map[string]string{"by": "orphan"}),
	}
	l := assemble(notes, replies, now.Add(-7*24*time.Hour))

	if l.Evicted != 1 {
		t.Errorf("evicted %d, want 1", l.Evicted)
	}
	var ids []string
	for _, n := range l.Notes {
		ids = append(ids, n.ID)
	}
	if got := strings.Join(ids, " "); got != "05-dnd 04-text 02-info 03-ask" {
		t.Errorf("order %q", got)
	}
	by := map[string]Note{}
	for _, n := range l.Notes {
		by[n.ID] = n
	}
	ask := by["03-ask"]
	if !ask.Asks || len(ask.Replies) != 2 || ask.Replies[1] != "later" {
		t.Errorf("ask: %+v", ask)
	}
	if ask.Answer == nil || ask.Answer.Reply != "later" || ask.Answer.By != "boris" {
		t.Errorf("the first answer must stand: %+v", ask.Answer)
	}
	if txt := by["04-text"]; !txt.Asks || !txt.ReplyText || len(txt.Replies) != 0 || txt.Answer != nil {
		t.Errorf("free text: %+v", txt)
	}
	if info := by["02-info"]; info.Asks || info.Sender != "shelf" || info.Body != "b" {
		t.Errorf("info: %+v", info)
	}
	if !by["05-dnd"].Suppressed {
		t.Error("a suppressed toast must say so")
	}
}

func TestAnswerRefusesBeforeDialling(t *testing.T) {
	if _, err := (RigService{}).Answer("", "yes", "", false); err == nil {
		t.Error("an answer to no notification must be refused")
	}
}
