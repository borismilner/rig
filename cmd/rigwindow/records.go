package main

import (
	"context"
	"fmt"
	"sort"
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

	// Section is the grouping a reader actually thinks in - "39", "11" - and it
	// is FREE: a requirement's id leads with its section number, by the id
	// scheme Boris ruled on 2026-09-18. Empty when the id has no such prefix,
	// which is every decision.
	Section      string `json:"section"`
	SectionTitle string `json:"sectionTitle"`

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
		out = append(out, RecordRow{
			ID:           r.GetId(),
			Version:      r.GetVersion(),
			Kind:         r.GetKind(),
			Title:        f["title"],
			Section:      sectionOf(r.GetId()),
			SectionTitle: f["section-title"],
			Body:         r.GetBody(),
			Source:       f["source"],
			Line:         f["doc-line"],
			Tags:         f["tags"],
			Retracted:    r.GetRetraction() != nil,
		})
	}
	sortRows(out)
	return out
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
		default:
			return a.ID > b.ID // dated: newest first
		}
	})
}
