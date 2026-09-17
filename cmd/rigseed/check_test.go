// The detector, tested where it can be wrong.
//
// ⛔ EVERY ASSERTION HERE IS ABOUT A SET. The instrument exists because a count
// did not move while two rows did, and a test suite that checked totals would
// reproduce the defect it is guarding.
package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// ids pulls the ids out of a plan, so an assertion reads as a set.
func ids(want []intent) []string {
	out := make([]string, 0, len(want))
	for _, in := range want {
		out = append(out, in.id)
	}
	return out
}

func has(set []string, want string) bool {
	for _, s := range set {
		if s == want {
			return true
		}
	}
	return false
}

func intentFor(t *testing.T, p plan, id string) intent {
	t.Helper()
	for _, in := range p.want {
		if in.id == id {
			return in
		}
	}
	t.Fatalf("no record planned for %s; the plan holds %v", id, ids(p.want))
	return intent{}
}

// ⛔ THE DEFECT B66 NAMES, AS A TEST: TWO SETS OF THE SAME SIZE THAT DISAGREE.
//
// The document states {B1 B2} and the store holds {B1 B3}. Both are two, so a
// census reports no change and is wrong in both directions at once. That is not
// a hypothetical: a count that did not move while two rows did is why this
// instrument was commissioned, and an arithmetically impossible census reached
// Boris once already.
func TestADivergenceOfEqualSizeIsSeenBecauseTheAnswerIsASet(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{
		{ID: "B1", Title: "in both"},
		{ID: "B2", Title: "in the document only"},
	}})
	store := map[string]map[string]string{
		"B1": fieldsOf(intentFor(t, p, "B1")),
		"B3": {"title": "in the store only"},
	}

	d := diff(p, store)
	if got := strings.Join(d.missing, " "); got != "B2" {
		t.Errorf("missing = {%s}, want {B2}", got)
	}
	if got := strings.Join(d.extra, " "); got != "B3" {
		t.Errorf("extra = {%s}, want {B3}", got)
	}
	if len(p.want) != len(store) {
		t.Fatalf("this test is not looking at what it thinks: the two sides must be "+
			"the SAME SIZE for it to mean anything, and they are %d and %d",
			len(p.want), len(store))
	}
}

// fieldsOf copies what the seeder would write, so a fixture store can hold a
// record that is genuinely up to date rather than one that merely exists.
func fieldsOf(in intent) map[string]string {
	out := make(map[string]string, len(in.fields))
	for k, v := range in.fields {
		out[k] = v
	}
	return out
}

// ⛔ A RECORD THAT IS PRESENT AND WRONG IS NOT A CLEAN ANSWER.
//
// B65 is STRUCK in the document and reads `status: active` in production, and a
// detector that compared only membership would call that set-equal. A lead then
// quoted the document's closed count as the store's, which is the same drift
// reaching a figure put in front of a reader.
func TestARecordWhoseStoredFieldsContradictTheDocumentIsStale(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{
		{ID: "B65", Title: "struck in the document", Done: true, Struck: true},
	}})

	stored := fieldsOf(intentFor(t, p, "B65"))
	stored[fieldStatus] = record.StatusActive

	d := diff(p, map[string]map[string]string{"B65": stored})
	if len(d.missing) != 0 || len(d.extra) != 0 {
		t.Fatalf("membership must agree for this test to mean anything: missing=%v extra=%v",
			d.missing, d.extra)
	}
	if got := strings.Join(d.stale, " "); got != "B65(status)" {
		t.Errorf("stale = {%s}, want {B65(status)} - the field name is what makes it actionable", got)
	}
}

// ⛔ THE HEADING GRAIN BECOMES A RECORD, WHICH IS THE WHOLE OF B66's APERTURE.
//
// B46 is the MVP's own acceptance test and it was in no record at all, because
// the import's unit was a table row.
func TestAnIdStatedInAHeadingBecomesARecord(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B46a", Title: "a child row", PartOf: "B46"}},
		Unimported: []record.Unimported{{
			Kind: record.UnimportedHeading, ID: "B46",
			Label: "⛔ B46 - THE MVP ACCEPTANCE TEST",
			Title: "THE MVP ACCEPTANCE TEST", Line: 137,
		}},
	})

	if !has(ids(p.want), "B46") {
		t.Fatalf("B46 is stated in a heading and was not planned; the plan holds %v", ids(p.want))
	}
	in := intentFor(t, p, "B46")
	if in.grain != grainHeading {
		t.Errorf("B46 was planned at grain %q, want %q", in.grain, grainHeading)
	}
	// ⛔ THE DERIVED TITLE, NOT THE VERBATIM LABEL. Seeded from `Label` this
	// read `⛔ B46 - THE MVP ACCEPTANCE TEST` - decoration and its own id -
	// where every row title goes through `titleOf`.
	if in.fields["title"] != "THE MVP ACCEPTANCE TEST" {
		t.Errorf("B46 title = %q, want the parser's derived title", in.fields["title"])
	}
	// ⛔ THIS ASSERTION IS THE REVERSE OF WHAT IT WAS, AND THE REVERSAL IS
	// THE FINDING. It required NO status, on the reasoning that a heading has
	// no state cell so the document says nothing. **The document does say it,
	// at a grain that is not a cell:** its own closure convention is the
	// strikethrough - B7's state cell reads "done, struck not deleted" - and
	// `BacklogItem.Done` reads that same mark on a row. Overturned by the lead
	// 2026-09-17, in the session that wrote it.
	//
	// ⛔ THE OLD ANSWER COST THE ROW ITS POINT: a record with no `status` is
	// absent from the brief's open list, so B46 was imported into invisibility
	// and B66's complaint stood while its row read as closed.
	if got := in.fields[fieldStatus]; got != record.StatusActive {
		t.Errorf("B46 status = %q, want %q - the heading is not struck, and "+
			"unstruck is what this document means by open", got, record.StatusActive)
	}
	if in.fields["tags"] != tagHeadingBorne {
		t.Errorf("B46 tags = %q, want %q - without it, a status that is absent "+
			"cannot be told from one something lost", in.fields["tags"], tagHeadingBorne)
	}
}

// ⛔ A PARENT EDGE COMES FROM THE ID, NEVER FROM WHERE THE HEADING SITS.
//
// The parser measured heading enclosure producing twenty-four edges the
// document never states, because the table under `## B46` became the general
// open-items table. Under is a fact about the FILE; part-of is a claim about
// the WORK.
func TestAHeadingsParentComesFromTheIdAndNotFromWhereItSits(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B1", Title: "anything"}},
		Unimported: []record.Unimported{
			// B46 itself, or B46d's edge would be an orphan rather than a
			// parent - which is what the document actually holds and what the
			// orphan pass caught when this fixture left it out.
			{Kind: record.UnimportedHeading, ID: "B46", Label: "the parent heading", Line: 137},
			{Kind: record.UnimportedHeading, ID: "B46d", Label: "a real sub-heading", Line: 183, Under: "B46"},
			{Kind: record.UnimportedHeading, ID: "B70", Label: "unrelated work filed under B46", Line: 400, Under: "B46"},
		},
	})

	if got := intentFor(t, p, "B46d").partOf; got != "B46" {
		t.Errorf("B46d part-of = %q, want B46 - the id agrees with the nesting", got)
	}
	if got := intentFor(t, p, "B70").partOf; got != "" {
		t.Errorf("B70 part-of = %q, want none: its id does not agree with the heading "+
			"it sits under, and inventing the edge is the falsified derivation", got)
	}
	if got := strings.Join(p.unnested, " "); got != "B70 sits under B46" {
		t.Errorf("unnested = {%s}, want the disagreement REPORTED rather than "+
			"silently dropped", got)
	}
}

// ⛔ AN UnimportedKind THIS FILE HAS NEVER HEARD OF MUST LAND SOMEWHERE LOUD.
//
// B66 is a class being invisible by construction. A split written as a list of
// kinds to skip would let the next kind added in internal/record fall through
// into "imported" - the same defect one layer up. internal/record is already
// growing one, so this is not hypothetical.
func TestAnUnknownUnimportedKindIsReportedAndNeverImported(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	const future = record.UnimportedKind("a-kind-invented-after-this-file-was-written")
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B1", Title: "anything"}},
		Unimported: []record.Unimported{
			{Kind: future, ID: "B99", Label: "something new", Line: 7},
		},
	})

	if has(ids(p.want), "B99") {
		t.Fatalf("a kind this file does not know was IMPORTED; the plan holds %v", ids(p.want))
	}
	if len(p.unimported) != 1 || p.unimported[0].Kind != future {
		t.Fatalf("the unknown kind was not reported; unimported = %v", p.unimported)
	}

	var b bytes.Buffer
	reportUnimported(&b, "BACKLOG.md", p.unimported)
	for _, want := range []string{string(future), "B99", "BACKLOG.md:7"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the report does not name %q:\n%s", want, b.String())
		}
	}
}

// ⛔ TWO PUTS AGAINST ONE ID IN ONE RUN SUPERSEDE EACH OTHER, AND WHICH CONTENT
// WINS WOULD BE DECIDED BY DOCUMENT ORDER. A silent wrong answer, so it is
// refused and said out loud.
func TestAnIdStatedAtBothGrainsIsWrittenOnceAndReported(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B46", Title: "the row's title"}},
		Unimported: []record.Unimported{
			{Kind: record.UnimportedHeading, ID: "B46", Label: "the heading's title", Line: 137},
		},
	})

	if n := len(p.want); n != 1 {
		t.Fatalf("B46 was planned %d times, want once: %v", n, ids(p.want))
	}
	if got := intentFor(t, p, "B46").title; got != "the row's title" {
		t.Errorf("B46 title = %q; the row is the grain with a state cell and must win", got)
	}
	if got := strings.Join(p.collided, " "); got != "B46" {
		t.Errorf("collided = {%s}, want {B46} - a collision resolved in silence is "+
			"the same defect as a row dropped in silence", got)
	}
}

// ⛔ ONLY A DIRECT EDGE IS A CHILD. `rig record refs` walks four hops by
// default, so a grandchild comes back too and counting it would invent a
// part-of the document never stated.
func TestOnlyADirectEdgeCountsAsAChild(t *testing.T) {
	const reply = `{"id":"B46","depth":4,"truncated":false,"cycles":[],"in":[
		{"src":"B46a","type":"part-of","distance":1},
		{"src":"B46a-1","type":"part-of","distance":2},
		{"src":"B12","type":"cites","distance":1}]}`

	have, err := parsePartOfChildren([]byte(reply), "B46")
	if err != nil {
		t.Fatalf("parsing a well-formed reply: %v", err)
	}
	if !have["B46a"] {
		t.Error("the direct part-of child was not seen")
	}
	if have["B46a-1"] {
		t.Error("a grandchild two hops away was counted as a child")
	}
	if have["B12"] {
		t.Error("a `cites` edge was counted as a part-of")
	}
}

// ⛔ A TRUNCATED WALK IS NOT AN ANSWER. rig prints the key on every reply so a
// partial answer cannot be read as a complete one, and a detector that took a
// cut-short walk for "no such edge" would report a divergence the store has not
// got. This is the pass-versus-no-run class, and refusing is the only honest
// arm of it.
func TestATruncatedRefsWalkIsRefusedRatherThanAnswered(t *testing.T) {
	const reply = `{"id":"B46","depth":4,"truncated":true,"cycles":[],"in":[]}`
	if _, err := parsePartOfChildren([]byte(reply), "B46"); err == nil {
		t.Fatal("a truncated walk was answered as though it were complete")
	}
}

// The edge diff itself, with the store lookup injected so it runs without a
// daemon.
func TestTheEdgeDiffIsASetOnBothSides(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{
		{ID: "B46a", Title: "stated and held", PartOf: "B46"},
		{ID: "B46b", Title: "stated and absent", PartOf: "B46"},
		{ID: "B46", Title: "the parent"},
	}})

	d := divergence{}
	err := d.compareEdges(p, func(parent string) (map[string]bool, error) {
		if parent != "B46" {
			t.Errorf("asked about %q; only a PARENT should be asked about", parent)
		}
		return map[string]bool{"B46a": true, "B46z": true}, nil
	})
	if err != nil {
		t.Fatalf("comparing edges: %v", err)
	}
	if got := strings.Join(d.missingEdges, " "); got != "B46b -part-of-> B46" {
		t.Errorf("missingEdges = {%s}", got)
	}
	if got := strings.Join(d.extraEdges, " "); got != "B46z -part-of-> B46" {
		t.Errorf("extraEdges = {%s}", got)
	}
}

// ⛔ AN EMPTY SET IS PRINTED, NOT SKIPPED. A set that disappears when it is
// empty cannot be told from a set the check never computed, which is this
// repository's most-recorded defect class.
func TestAnEmptySetIsPrintedAsEmpty(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{{ID: "B1", Title: "x"}}})
	d := diff(p, map[string]map[string]string{"B1": fieldsOf(intentFor(t, p, "B1"))})

	var b bytes.Buffer
	d.report(&b, o, p, "production")
	out := b.String()

	for _, label := range []string{"MISSING", "EXTRA", "STALE", "MISSING EDGES", "EXTRA EDGES", "UNIMPORTED", "NEVER DEFINES"} {
		if !strings.Contains(out, label) {
			t.Errorf("the report never names %s, so a reader cannot tell it was computed:\n%s", label, out)
		}
	}
	if n := strings.Count(out, "{}"); n < 8 {
		t.Errorf("only %d sets printed as empty; every one of them must say {} rather "+
			"than vanish:\n%s", n, out)
	}
	if len(d.nonEmpty()) != 0 {
		t.Errorf("nonEmpty = %v over a store that matches the document exactly", d.nonEmpty())
	}
}

// The exit line names WHICH sets diverged, because "something was wrong" sends
// the reader back to the output to find out what.
func TestTheRefusalNamesWhichSetsDiverged(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{
		Items: []record.BacklogItem{{ID: "B1", Title: "x"}},
		Unimported: []record.Unimported{
			{Kind: record.UnimportedRowWithoutID, Label: "6a", Line: 131},
		},
	})
	d := diff(p, map[string]map[string]string{})

	got := strings.Join(d.nonEmpty(), " ")
	if got != "missing unimported" {
		t.Errorf("nonEmpty = %q, want %q", got, "missing unimported")
	}
}

// ⛔ THE LIVE DOCUMENT, WHICH IS WHERE B46 ACTUALLY LIVES.
//
// It SKIPS rather than fails when the document is not there: BACKLOG.md is a
// gitignored symlink into the logbook, so a fresh clone and a CI runner have a
// dangling link. The tests above carry the properties; this one is the
// rig-on-rig measurement.
func TestRigsOwnBacklogPlansBothGrains(t *testing.T) {
	const live = "../../BACKLOG.md"
	if _, err := os.Stat(live); err != nil {
		t.Skipf("%s does not resolve on this machine (%v)", live, err)
	}
	doc, err := readBacklog(live)
	if err != nil {
		t.Fatalf("reading %s: %v", live, err)
	}
	p := planFor(options{project: "rig", backlog: live}, doc)

	// POSITIVE CONTROL. An empty document would pass every assertion below in
	// silence, which is how this project's checks have failed nine times.
	if len(doc.Items) < 50 {
		t.Fatalf("the parse yielded %d rows and the document held 75 when this was "+
			"written; this test is not looking at what it thinks", len(doc.Items))
	}
	if len(doc.Unimported) == 0 {
		t.Fatal("the document reported nothing unimported, which is the blindness " +
			"B66 names rather than a clean answer")
	}

	if !has(ids(p.want), "B46") {
		t.Error("B46 - the MVP's own acceptance test - is still in no planned record")
	}
	// ⛔ NOT ONE HEADING MAY REMAIN UNIMPORTED. The aperture is the point.
	for _, u := range p.unimported {
		if u.Kind == record.UnimportedHeading {
			t.Errorf("%s is stated in a heading at line %d and was left unimported",
				u.ID, u.Line)
		}
	}
	// ⛔ AND THE OTHER KINDS MUST STILL BE REPORTED, NOT QUIETLY ADOPTED. The
	// eleven ordered rows and the five adopter-table ids are a RULING nobody
	// here may take.
	if len(p.unimported) == 0 {
		t.Error("everything was imported, which would mean the ranked rows and the " +
			"adopter table were adopted without a ruling")
	}
}

// ⛔ AN EDGE TOWARDS AN ID THE DOCUMENT NEVER DEFINES IS A FINDING, NOT A CRASH.
//
// internal/record's checkEdge refuses a link with a missing end, so asserting
// one aborts a seeding run half-written. Before B46 became a record its six
// children named exactly such a parent, so this is a shape the document has
// actually held.
func TestAPartOfTowardsAnUndefinedIdIsReportedAndNeverAttempted(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{Items: []record.BacklogItem{
		{ID: "B99a", Title: "a child of a parent nobody wrote", PartOf: "B99"},
	}})

	if got := intentFor(t, p, "B99a").partOf; got != "" {
		t.Errorf("B99a still carries part-of %q; the link would abort the run", got)
	}
	if got := strings.Join(p.orphaned, " "); got != "B99a -part-of-> B99" {
		t.Errorf("orphaned = {%s}, want {B99a -part-of-> B99}", got)
	}

	d := diff(p, map[string]map[string]string{"B99a": fieldsOf(intentFor(t, p, "B99a"))})
	if !has(d.nonEmpty(), "part-of-towards-an-undefined-id") {
		t.Errorf("nonEmpty = %v; an edge the document states and nothing can carry "+
			"must not exit clean", d.nonEmpty())
	}
	var b bytes.Buffer
	d.report(&b, o, p, "production")
	if !strings.Contains(b.String(), "B99a -part-of-> B99") {
		t.Errorf("the report never names the orphaned edge:\n%s", b.String())
	}
}

// AND A STRUCK HEADING IS CLOSED, BY THE SAME CONVENTION READ THE OTHER WAY.
//
// ⛔ NOTHING IN rig's OWN BACKLOG EXERCISES THIS. No heading there is
// struck, so the live document only ever produces `active` and this fixture is
// the whole of the evidence for the other branch. A predicate seen from one
// side is half-tested, and saying so is cheaper than discovering it.
func TestAStruckHeadingIsSeededClosed(t *testing.T) {
	o := options{project: "rig", backlog: "BACKLOG.md"}
	p := planFor(o, record.BacklogParse{
		Unimported: []record.Unimported{{
			Kind: record.UnimportedHeading, ID: "B46",
			Label: "⛔ ~~B46 - THE MVP ACCEPTANCE TEST~~",
			Title: "THE MVP ACCEPTANCE TEST", Struck: true, Line: 137,
		}},
	})

	in := intentFor(t, p, "B46")
	if got := in.fields[fieldStatus]; got != record.StatusClosed {
		t.Errorf("a struck heading was seeded status=%q, want %q", got, record.StatusClosed)
	}
	// The control: the title must survive the strike rather than being eaten
	// with it, which is what a naive trim of the decoration set does.
	if in.fields["title"] != "THE MVP ACCEPTANCE TEST" {
		t.Errorf("the strike took the title with it: %q", in.fields["title"])
	}
}
