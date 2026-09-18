package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// ⛔ 89% OF WHAT rig HOLDS HAD NO SCREEN, AND THIS IS THE CALL THAT CHANGES IT.
// Measured 2026-09-18 on his production store: 2,538 records and 2.63 MB of
// prose, of which the window's two calls - Projects and Brief - could reach at
// most 571. The 481 requirements and 1,485 decisions, 2.33 MB, had no route to
// the screen at all. Boris the same day: "In the GUI I don't see the
// utilization of the many goodies we have stored in `rig`."
// logbook/projects/rig/gui-phase-design-2026-09-18.md, slice 1.
//
// ⛔ IT ADDS NO VERB AND NO SCHEMA. `record.query` already answers project and
// kind, and `Projects` above already calls it. The gap was never in the daemon.

// RecordRow is one record as a list needs it: enough to render a row and open
// it, and not the whole store's prose twice.
type RecordRow struct {
	ID      string `json:"id"`
	Version uint64 `json:"version"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`

	// Section is the grouping a reader actually thinks in - "39", "11".
	//
	// ⛔ THE STORE CARRIES IT AS A FIELD AND THE ID IS ONLY THE FALLBACK.
	// Measured against his production store 2026-09-18: `section` is present on
	// all 387 requirement heads and agrees with the id's first segment on every
	// one of them. Reading the field is not a preference - deriving a grouping
	// from a key means the day an id scheme changes, the grouping changes
	// silently and nothing fails.
	Section string `json:"section"`

	// SectionTitle is "11. The window" - what a reader wants above the group,
	// rather than the bare number.
	//
	// ⛔ IT IS COMPUTED, BECAUSE THERE IS NO SUCH FIELD. This read
	// `fields["section-title"]`, which no seeder has ever written, so it was
	// empty on every record in the store and the Spec view would have shipped
	// with 42 headings reading "1", "2", "3". The store's own answer is the
	// title of the record at the TOP of the section's document, which rowsFrom
	// finds by doc-line.
	SectionTitle string `json:"sectionTitle"`

	// Level is the heading depth the document states: 2 is a section head, 3
	// and below are nested under it. It is what lets the Spec render as the
	// tree the plan actually is rather than as 387 flat rows.
	Level int `json:"level"`

	// Date is the day a dated record belongs to, "" when it has none.
	//
	// ⛔ 91 OF 533 DECISIONS HAVE NO DATE ANYWHERE - not in `doc-date`, which
	// only 140 carry, and not in the id. Measured 2026-09-18. A view that
	// groups by day must therefore have an undated group; inventing one from
	// the seeding run's clock would date a ruling by when it was imported.
	Date string `json:"date"`

	Body string `json:"body"`

	// Source and Line are where the document says it, so a reader can go and
	// look. They are the fields the seeder writes, unaltered.
	Source string `json:"source"`
	Line   string `json:"line"`
	Tags   string `json:"tags"`

	// Retracted is stated rather than filtered: a record the store is still
	// holding but no longer standing behind is a thing a reader must be able
	// to see, and silently dropping it is how a list lies.
	Retracted bool `json:"retracted"`
}

// Records answers every record of one kind in one project, ordered for reading.
//
// ⛔ THE KIND IS REQUIRED AND THE REFUSAL SAYS WHAT THE ALTERNATIVES ARE. An
// unfiltered query over this store returns 2,538 records and 2.6 MB through a
// single frame; a list that big is not a screen, it is a download. The caller
// asks for what it is about to render.
func (RigService) Records(project, kind string) ([]RecordRow, error) {
	if project == "" {
		return nil, fmt.Errorf("a record list needs a project to be about")
	}
	if kind == "" {
		return nil, fmt.Errorf("a record list needs a kind: requirement, decision, " +
			"work-item, note or progress. An unfiltered read of this store is " +
			"megabytes and is not a screen")
	}

	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.RecordQueryResponse{}
	req := &rigv1.RecordQueryRequest{Project: project, Kind: kind}
	if err := c.Call(ctx, "rig.record.query", req, resp); err != nil {
		return nil, fmt.Errorf("the store could not be asked for the %s records of %q: %w",
			kind, project, err)
	}
	return rowsFrom(resp.GetRecords()), nil
}

// rowsFrom maps the wire's records to rows and puts them in reading order.
func rowsFrom(recs []*rigv1.Record) []RecordRow {
	out := make([]RecordRow, 0, len(recs))
	for _, r := range recs {
		f := r.GetFields()
		sec := f["section"]
		if sec == "" {
			sec = sectionOf(r.GetId())
		}
		lvl, _ := strconv.Atoi(f["level"])
		out = append(out, RecordRow{
			ID:        r.GetId(),
			Version:   r.GetVersion(),
			Kind:      r.GetKind(),
			Title:     f["title"],
			Section:   sec,
			Level:     lvl,
			Date:      dateOf(r.GetId(), f["doc-date"]),
			Body:      r.GetBody(),
			Source:    f["source"],
			Line:      f["doc-line"],
			Tags:      f["tags"],
			Retracted: r.GetRetraction() != nil,
		})
	}
	nameSections(out)
	sortRows(out)
	return out
}

// nameSections gives every row in a section the title of that section's head.
//
// ⛔ THE HEAD IS THE LOWEST doc-line, NOT THE LOWEST LEVEL, and the difference
// is measured rather than stylistic: four of his 42 sections hold more than one
// level-2 record - section 11 holds TEN - so "the level-2 one" picks an
// arbitrary heading out of the middle of the document. Every section is one
// source file whose first heading is the section's own, which is what doc-line
// 1 finds. Verified over all 42 on 2026-09-18.
func nameSections(rows []RecordRow) {
	type head struct {
		line  int
		title string
	}
	heads := map[string]head{}
	for _, r := range rows {
		if r.Section == "" || r.Title == "" {
			continue
		}
		// A row with no readable doc-line must never win the comparison, so it
		// is ranked last rather than treated as line zero.
		line, err := strconv.Atoi(r.Line)
		if err != nil {
			continue
		}
		if h, ok := heads[r.Section]; !ok || line < h.line {
			heads[r.Section] = head{line: line, title: r.Title}
		}
	}
	for i := range rows {
		if h, ok := heads[rows[i].Section]; ok {
			rows[i].SectionTitle = h.title
		}
	}
}

// dateOf is the day a record belongs to: what the document stated, else the
// date its id leads with, else nothing.
//
// ⛔ THE TWO NEVER DISAGREE AND THAT WAS CHECKED, not assumed - over the 140
// decisions carrying both, `doc-date` and the id prefix matched 140 times. The
// stated field still wins, because a document that says a date is a stronger
// claim than a key that happens to start with one.
func dateOf(id, stated string) string {
	if stated != "" {
		return stated
	}
	if len(id) < 10 {
		return ""
	}
	d := id[:10]
	for i, c := range d {
		if i == 4 || i == 7 {
			if c != '-' {
				return ""
			}
			continue
		}
		if c < '0' || c > '9' {
			return ""
		}
	}
	return d
}

// sectionOf is the id's leading segment when that segment is a section number.
//
// ⛔ IT IS NOT A SPLIT ON "/" AND THAT IS THE POINT. A decision's id is
// `2026-09-10-fourth-session-.../the-four-holes`, which HAS a slash and whose
// first segment is not a section. Answering "2026-09-10-fourth-session" as a
// section would group every decision under itself.
func sectionOf(id string) string {
	head, _, ok := strings.Cut(id, "/")
	if !ok {
		return ""
	}
	if head == "" || len(head) > 3 {
		return ""
	}
	for _, c := range head {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return head
}

// sortRows puts a list in the order its reader expects, which is not one order.
//
// ⛔ A DATED ID READS NEWEST FIRST AND A SECTIONED ONE READS IN SECTION ORDER,
// and the two must not be one rule. Sorting the specification by "newest" would
// scatter section 39 through section 11; sorting decisions by id ascending puts
// 2026-09-10 above the ruling he made an hour ago.
func sortRows(rows []RecordRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.Section != "" && b.Section != "":
			if len(a.Section) != len(b.Section) {
				return len(a.Section) < len(b.Section) // "9" before "11"
			}
			if a.Section != b.Section {
				return a.Section < b.Section
			}
			return a.ID < b.ID
		case a.Section != "" || b.Section != "":
			return a.Section != "" // sectioned records first, and grouped
		case a.Date != b.Date:
			// ⛔ NEWEST FIRST BY DATE, AND AN UNDATED ROW GOES LAST RATHER THAN
			// FIRST. 91 of his 533 decisions carry no date at all; sorting ids
			// descending as strings put every one of them - they begin with a
			// letter - above every dated ruling, so the newest decision in the
			// store was 91 rows down the page.
			if a.Date == "" || b.Date == "" {
				return b.Date == ""
			}
			return a.Date > b.Date
		default:
			return a.ID > b.ID
		}
	})
}
