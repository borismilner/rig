package main

import (
	"strings"
	"testing"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// ⛔ A DECISION'S ID HAS A SLASH AND ITS FIRST SEGMENT IS NOT A SECTION. Every
// one of the 1,485 decisions in his store is keyed
// `2026-09-10-fourth-session-.../the-four-holes`, so a section derived by
// splitting on "/" would group each decision under a heading of its own and
// the Spec view would fill with dates.
func TestOnlyASectionNumberIsReadAsASection(t *testing.T) {
	for _, c := range []struct{ id, want string }{
		{"39/the-continuity-record/a-heading", "39"},
		{"9/built-for-agents", "9"},
		{"11/the-window/the-rail", "11"},
		{"2026-09-10-fourth-session-m1-slice-3/the-four-holes", ""},
		{"B83", ""},
		{"", ""},
		{"1000/too-long-to-be-a-section", ""},
		{"39a/not-a-number", ""},
	} {
		if got := sectionOf(c.id); got != c.want {
			t.Errorf("sectionOf(%q) = %q; want %q", c.id, got, c.want)
		}
	}
}

// ⛔ TWO ORDERS, AND COLLAPSING THEM IS THE DEFECT. The specification reads in
// section order and a dated record reads newest first; one rule for both
// scatters section 39 through section 11, or puts a ruling from last September
// above one made an hour ago.
func TestTheSpecReadsInSectionOrderAndDatedRecordsReadNewestFirst(t *testing.T) {
	rows := []RecordRow{
		{ID: "2026-09-10-early/a"},
		{ID: "11/the-window", Section: "11"},
		{ID: "2026-09-18-late/b"},
		{ID: "9/built-for-agents", Section: "9"},
		{ID: "39/the-record", Section: "39"},
	}
	sortRows(rows)
	var got []string
	for _, r := range rows {
		got = append(got, r.ID)
	}
	want := "9/built-for-agents 11/the-window 39/the-record 2026-09-18-late/b 2026-09-10-early/a"
	if strings.Join(got, " ") != want {
		t.Errorf("order =\n  %s\nwant\n  %s", strings.Join(got, " "), want)
	}
}

// A retracted record is SHOWN and flagged, never dropped: a list that silently
// omits what the store is still holding is a list that lies.
func TestARetractedRecordIsCarriedAndFlagged(t *testing.T) {
	rows := rowsFrom([]*rigv1.Record{
		{Id: "39/a", Fields: map[string]string{"title": "kept"}},
		{Id: "39/b", Fields: map[string]string{"title": "pulled"},
			Retraction: &rigv1.Retraction{Reason: "superseded"}},
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %d; a retracted record was dropped rather than flagged", len(rows))
	}
	if rows[0].Retracted || !rows[1].Retracted {
		t.Errorf("retracted flags = %v, %v; want false, true", rows[0].Retracted, rows[1].Retracted)
	}
}

// ⛔ THE KIND IS REQUIRED, AND THE REFUSAL MUST NAME THE ALTERNATIVES. An
// unfiltered query over this store is 2,538 records and 2.6 MB; section 10
// says every error answers with what would fix it.
func TestARecordListRefusesWithoutAKindAndSaysWhatToAsk(t *testing.T) {
	_, err := RigService{}.Records("rig", "")
	if err == nil {
		t.Fatal("an unfiltered record list was accepted")
	}
	for _, want := range []string{"kind", "requirement", "decision"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %s", want, err)
		}
	}
	if _, err := (RigService{}).Records("", "requirement"); err == nil {
		t.Error("a record list without a project was accepted")
	}
}
