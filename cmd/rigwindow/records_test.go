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

// ⛔ THE SECTION HEADING CAME BACK EMPTY ON EVERY RECORD IN HIS STORE, because
// this read `fields["section-title"]` and no seeder writes that key. The Spec
// view would have shipped 42 groups headed "1", "2", "3".
//
// ⛔ AND THE HEAD IS THE LOWEST doc-line RATHER THAN THE LOWEST LEVEL. Section
// 11 holds TEN level-2 records, so picking by level picks a heading out of the
// middle of the document. This fixture is that shape.
func TestASectionIsTitledByTheTopOfItsDocumentNotByItsFirstLevelTwo(t *testing.T) {
	rows := rowsFrom([]*rigv1.Record{
		{Id: "11/the-rail", Fields: map[string]string{
			"title": "The rail", "section": "11", "level": "2", "doc-line": "240"}},
		{Id: "11/the-window", Fields: map[string]string{
			"title": "11. The window", "section": "11", "level": "2", "doc-line": "1"}},
		{Id: "11/the-window/never-stale", Fields: map[string]string{
			"title": "The window may never be stale", "section": "11",
			"level": "3", "doc-line": "88"}},
	})
	for _, r := range rows {
		if r.SectionTitle != "11. The window" {
			t.Errorf("%s is headed %q; want the section's own title", r.ID, r.SectionTitle)
		}
	}
	if rows[0].Level != 2 {
		t.Errorf("level = %d; the document's heading depth was not carried", rows[0].Level)
	}
}

// The stored field is what groups a record; the id is only the fallback. Both
// agree on all 387 of his requirement heads, which is exactly why the
// difference has to be asserted rather than observed.
func TestTheStoredSectionWinsOverTheOneTheIdImplies(t *testing.T) {
	rows := rowsFrom([]*rigv1.Record{
		{Id: "B83", Fields: map[string]string{"title": "a ranked row", "section": "39"}},
		{Id: "9/built-for-agents", Fields: map[string]string{"title": "no field here"}},
	})
	by := map[string]string{}
	for _, r := range rows {
		by[r.ID] = r.Section
	}
	if by["B83"] != "39" {
		t.Errorf("B83 grouped under %q; the stored section field was ignored", by["B83"])
	}
	if by["9/built-for-agents"] != "9" {
		t.Errorf("the id fallback did not fire: %q", by["9/built-for-agents"])
	}
}

// ⛔ 91 OF 533 DECISIONS CARRY NO DATE, and their ids begin with a letter. The
// old id-descending sort therefore floated every one of them above every dated
// ruling: the newest decision in the store was 91 rows down the page.
func TestAnUndatedDecisionSinksRatherThanFloats(t *testing.T) {
	rows := rowsFrom([]*rigv1.Record{
		{Id: "a-constant-dressed-as-a-live-field", Fields: map[string]string{"title": "undated"}},
		{Id: "2026-09-10-fifth-session/x", Fields: map[string]string{"title": "old"}},
		{Id: "2026-09-18-today/y", Fields: map[string]string{"title": "newest"}},
	})
	var got []string
	for _, r := range rows {
		got = append(got, r.ID)
	}
	want := "2026-09-18-today/y 2026-09-10-fifth-session/x a-constant-dressed-as-a-live-field"
	if strings.Join(got, " ") != want {
		t.Errorf("order =\n  %s\nwant\n  %s", strings.Join(got, " "), want)
	}
	if rows[0].Date != "2026-09-18" || rows[2].Date != "" {
		t.Errorf("dates = %q, %q; want the id's day and nothing", rows[0].Date, rows[2].Date)
	}
}

// The document's own statement beats the key that happens to start with a date.
func TestAStatedDateBeatsTheIdsPrefix(t *testing.T) {
	for _, c := range []struct{ id, stated, want string }{
		{"2026-09-10-x/y", "2026-09-11", "2026-09-11"},
		{"2026-09-10-x/y", "", "2026-09-10"},
		{"B83", "", ""},
		{"2026-09-1x-bad/y", "", ""},
		{"short", "", ""},
	} {
		if got := dateOf(c.id, c.stated); got != c.want {
			t.Errorf("dateOf(%q, %q) = %q; want %q", c.id, c.stated, got, c.want)
		}
	}
}
