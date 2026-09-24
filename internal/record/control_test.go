package record

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

// rec writes one record and returns its id.
func rec(t *testing.T, s *Store, id, kind, title string) string {
	t.Helper()
	w, err := s.Put(tctx, PutRequest{
		ID: id, Kind: kind, Project: "rig", Body: title,
		Fields:  map[string]string{"title": title, "status": "active"},
		Session: "control", Seat: "backend-record", Epoch: 7,
	})
	if err != nil {
		t.Fatalf("writing %s: %v", id, err)
	}
	return w.ID
}

// edgeSet is a comparable form of an edge list.
func edgeSet(es []Edge) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Src+" -"+e.Type+"-> "+e.Dst)
	}
	sort.Strings(out)
	return out
}

// ---- retract: the id and the history SURVIVE ------------------------------

// ⛔ THE WHOLE OF retract IS THAT IT DESTROYS NOTHING, AND BORIS'S OWN TABLE
// SETS THE TWO DISTINGUISHING QUESTIONS: does the id survive (yes), and does
// the HISTORY survive (yes)? A retract that wrote a new version would fail the
// second while passing every test that only looked at the head.
func TestARetractedRecordKeepsItsIDAndEveryVersionOfItsHistory(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	id := rec(t, s, "B90", "work-item", "the first wording")
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: "rig", Body: "the second wording",
		Fields:  map[string]string{"title": "the second wording", "status": "active"},
		Session: "control", Seat: "backend-record", Epoch: 7, IfVersion: 1,
	}); err != nil {
		t.Fatalf("superseding: %v", err)
	}

	before, err := s.History(tctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("the fixture has %d versions, want 2", len(before))
	}

	if _, err := s.Retract(tctx, RetractRequest{
		ID: id, Reason: "filed twice", Session: "control",
		Seat: "backend-record", Epoch: 7,
	}); err != nil {
		t.Fatalf("retracting: %v", err)
	}

	after, err := s.History(tctx, id)
	if err != nil {
		t.Fatalf("the history of a retracted record is unreadable: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("retract changed the history.\nbefore %+v\nafter  %+v", before, after)
	}
}

// ⛔ IT LEAVES THE QUERIES AND IT DOES NOT LEAVE record.get,
// which is the half of the contract a delete cannot satisfy: "record.get still
// explains what it was and that it was retracted".
func TestARetractedRecordLeavesEveryListAndStillExplainsItselfToGet(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	rec(t, s, "rig", "project", "the project")
	live := rec(t, s, "B90", "work-item", "still open")
	gone := rec(t, s, "B91", "work-item", "written by mistake")

	if _, err := s.Retract(tctx, RetractRequest{
		ID: gone, Reason: "written by mistake", Session: "control",
		Seat: "backend-record", Epoch: 7,
	}); err != nil {
		t.Fatal(err)
	}

	found, err := s.Find(tctx, QueryFilter{Kind: "work-item"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != live {
		t.Fatalf("Find returned %d records, want only %s - a retracted record "+
			"is still in the answer", len(found), live)
	}

	q, err := s.Query(tctx, "rig", "work-item")
	if err != nil {
		t.Fatal(err)
	}
	if len(q) != 1 || q[0].ID != live {
		t.Fatalf("Query returned %+v, want only %s", q, live)
	}

	// ⛔ THE BRIEF'S HALF OF THIS TEST LEFT WITH THE BRIEF. plan/50 move 8:
	// the derivation moved to the docket program, so "a retracted record is
	// not in next-up, not in open, and NOT filed under closed either" is now
	// docket's assertion to make against these same two lists. What rig can
	// still prove is what rig still answers, which is the three verbs above
	// and below: Find and Query drop it, Get keeps explaining it. The
	// three-list form of this assertion is back over the wire, in
	// internal/daemon/control_test.go, against the query shapes docket reads.
	// ⛔ AND record.get MUST STILL ANSWER, WITH THE FACT AND ITS REASON. A
	// NotFound here would make retract indistinguishable from delete to every
	// reader, which is the collapse Boris's table exists to prevent.
	got, err := s.Get(tctx, gone)
	if err != nil {
		t.Fatalf("get on a retracted record: %v - it must still explain "+
			"what the record was and that it was retracted", err)
	}
	if got.Retraction == nil {
		t.Fatal("get answered about a retracted record and said nothing about " +
			"the retraction, so a reader cannot tell it from live work")
	}
	if got.Retraction.Reason != "written by mistake" {
		t.Errorf("the retraction reason is %q, want the one given", got.Retraction.Reason)
	}
	if got.Retraction.Prov.Seat != "backend-record" {
		t.Errorf("the retraction carries seat %q - a retraction with no "+
			"provenance cannot be argued with", got.Retraction.Prov.Seat)
	}
	if got.Body != "written by mistake" {
		t.Errorf("the record's own content did not survive its retraction: %q", got.Body)
	}
}

// Retracting what is not there is a refusal and not a silent success.
func TestRetractingAnUnknownRecordIsRefused(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	_, err := s.Retract(tctx, RetractRequest{ID: "nope", Session: "c", Seat: "s", Epoch: 7})
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("retracting an unknown id gave %v, want a NotFoundError", err)
	}
}

// ⛔ A SECOND RETRACT IS NOT AN ERROR AND IT DOES NOT OVERWRITE THE FIRST. The
// caller is asserting a state, which is Link's set semantics - but the
// PROVENANCE of a withdrawal is evidence, and the second caller did not
// withdraw it. The answer says which happened.
func TestRetractingTwiceKeepsTheFirstWithdrawalAndSaysSo(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	id := rec(t, s, "B90", "work-item", "x")

	first, err := s.Retract(tctx, RetractRequest{
		ID: id, Reason: "the real reason", Session: "c", Seat: "first", Epoch: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Already {
		t.Fatal("the first retraction reported itself as already done")
	}

	second, err := s.Retract(tctx, RetractRequest{
		ID: id, Reason: "a later guess", Session: "c", Seat: "second", Epoch: 8,
	})
	if err != nil {
		t.Fatalf("retracting twice refused: %v", err)
	}
	if !second.Already {
		t.Error("the second retraction did not report that it was already retracted")
	}
	if second.Reason != "the real reason" || second.Prov.Seat != "first" {
		t.Errorf("the second retract overwrote the first withdrawal's evidence: %+v", second)
	}
}

// ---- delete: the id and the history DO NOT survive ------------------------

// ⛔ DELETE IS NOT RETRACT SPELLED DIFFERENTLY. Boris's two distinguishing
// questions, answered the other way: the id does not survive and neither does
// the history.
func TestDeleteRemovesTheRecordAndEveryVersionOfIt(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	id := rec(t, s, "B90", "work-item", "v1")
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: "rig", Body: "v2",
		Fields:  map[string]string{"title": "v2"},
		Session: "c", Seat: "s", Epoch: 7, IfVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}

	d, err := s.Delete(tctx, DeleteRequest{ID: id})
	if err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if d.Versions != 2 {
		t.Errorf("delete reported %d versions removed, want 2 - a count that "+
			"is not the truth is worse than no count", d.Versions)
	}

	var nf *NotFoundError
	if _, err := s.Get(tctx, id); !errors.As(err, &nf) {
		t.Errorf("get still answers about a deleted record: %v", err)
	}
	if _, err := s.History(tctx, id); !errors.As(err, &nf) {
		t.Errorf("the history of a deleted record survived: %v", err)
	}
}

// ⛔ THE EDGES GO, AND THE VERB SAYS WHICH. Ruled by Boris 2026-09-17 against
// the lead's recommendation, cost stated and accepted - so the obligation moved
// from the verb to the REPORT. A destructive verb that answers "deleted" and
// nothing else is B75's shape arriving through a verb whose job is to lose data.
func TestDeleteDropsEveryEdgeTouchingTheRecordAndNamesEachOne(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	parent := rec(t, s, "B46", "work-item", "the parent")
	doomed := rec(t, s, "B90", "work-item", "the one going")
	child := rec(t, s, "B91", "work-item", "a child of the doomed")
	other := rec(t, s, "B92", "work-item", "points at the doomed")

	for _, e := range []Edge{
		{Src: doomed, Type: LinkPartOf, Dst: parent}, // outbound
		{Src: child, Type: LinkPartOf, Dst: doomed},  // inbound
		{Src: other, Type: LinkCites, Dst: doomed},   // inbound, second type
		{Src: child, Type: LinkPartOf, Dst: parent},  // untouched
	} {
		if err := s.Link(tctx, e.Src, e.Type, e.Dst); err != nil {
			t.Fatal(err)
		}
	}

	d, err := s.Delete(tctx, DeleteRequest{ID: doomed})
	if err != nil {
		t.Fatal(err)
	}

	// SORTED, because edgeSet sorts: the report's ORDER is a property of the
	// query and is asserted separately below, not smuggled into this list.
	want := []string{
		doomed + " -part-of-> " + parent,
		child + " -part-of-> " + doomed,
		other + " -cites-> " + doomed,
	}
	sort.Strings(want)
	if got := edgeSet(d.Edges); !reflect.DeepEqual(got, want) {
		t.Fatalf("delete reported these edges dropped:\n%v\nwant:\n%v", got, want)
	}

	// ⛔ AND THE REPORT'S OWN ORDER IS DETERMINISTIC, which the sort above
	// deliberately hides. A destructive verb whose account comes back in a
	// different order per call cannot be diffed between a dry run and the act,
	// which is the one comparison this verb's output exists to support.
	if !sort.SliceIsSorted(d.Edges, func(i, j int) bool {
		a, b := d.Edges[i], d.Edges[j]
		if a.Src != b.Src {
			return a.Src < b.Src
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Dst < b.Dst
	}) {
		t.Errorf("the dropped-edge report is not in a stable order: %v", d.Edges)
	}

	// ⛔ BOTH DIRECTIONS GO. An implementation that dropped only the inbound
	// edges would leave the store asserting that a record which no longer
	// exists is part of something.
	from, err := s.linksFrom(tctx, parent, LinkPartOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(from) != 0 {
		t.Errorf("the parent still has outbound part-of edges: %v", from)
	}
	survivors, err := s.Refs(tctx, RefsRequest{ID: parent, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range survivors.Refs {
		if in.ID == doomed {
			t.Error("an edge from the deleted record survived the delete")
		}
	}
	if len(survivors.Refs) != 1 || survivors.Refs[0].ID != child {
		t.Errorf("the untouched edge did not survive: %+v", survivors.Refs)
	}
}

// ⛔ THE DRY RUN SHOWS EXACTLY WHAT THE REAL ONE WOULD TAKE AND WRITES NOTHING.
// Boris ruled that a dry run must be able to show the dropped edges first, and
// a preview that is computed by a different path from the act is a preview of
// something else.
func TestADeleteDryRunReportsTheSameLossAndChangesNothing(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	parent := rec(t, s, "B46", "work-item", "the parent")
	doomed := rec(t, s, "B90", "work-item", "the one going")
	if err := s.Link(tctx, doomed, LinkPartOf, parent); err != nil {
		t.Fatal(err)
	}

	dry, err := s.Delete(tctx, DeleteRequest{ID: doomed, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun {
		t.Error("a dry run did not say it was one, so its output reads as a receipt")
	}
	if _, err := s.Get(tctx, doomed); err != nil {
		t.Fatalf("the dry run removed the record: %v", err)
	}

	done, err := s.Delete(tctx, DeleteRequest{ID: doomed})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(edgeSet(dry.Edges), edgeSet(done.Edges)) ||
		dry.Versions != done.Versions {
		t.Fatalf("the dry run promised %v/%d and the delete took %v/%d",
			edgeSet(dry.Edges), dry.Versions, edgeSet(done.Edges), done.Versions)
	}
}

func TestDeletingAnUnknownRecordIsRefused(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	_, err := s.Delete(tctx, DeleteRequest{ID: "nope"})
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("deleting an unknown id gave %v, want a NotFoundError", err)
	}
}

// ---- replace: a DIFFERENT record takes the place -------------------------

// ⛔ REPLACE IS THE DUPLICATE CASE AND SUPERSEDE CANNOT EXPRESS IT, because
// supersede keeps the id. The survivor must absorb the loser's INBOUND edges -
// that is the whole capability, and it is why shipping delete without replace
// leaves the duplicate case with no answer but edge loss.
func TestReplaceMovesTheInboundEdgesOntoTheSurvivor(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	parent := rec(t, s, "B46", "work-item", "the parent")
	loser := rec(t, s, "B90", "work-item", "filed twice, the copy")
	survivor := rec(t, s, "B91", "work-item", "filed twice, the one we keep")
	citer := rec(t, s, "B92", "work-item", "cites the copy")

	for _, e := range []Edge{
		{Src: citer, Type: LinkCites, Dst: loser},
		{Src: loser, Type: LinkPartOf, Dst: parent},
	} {
		if err := s.Link(tctx, e.Src, e.Type, e.Dst); err != nil {
			t.Fatal(err)
		}
	}

	r, err := s.Replace(tctx, ReplaceRequest{
		Old: loser, New: survivor, Session: "c", Seat: "backend-record", Epoch: 7,
	})
	if err != nil {
		t.Fatalf("replacing: %v", err)
	}

	want := []string{citer + " -cites-> " + loser}
	if got := edgeSet(r.Moved); !reflect.DeepEqual(got, want) {
		t.Fatalf("replace moved %v, want %v - only the INBOUND edges move", got, want)
	}

	in, err := s.Refs(tctx, RefsRequest{ID: survivor, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	var sawCiter bool
	for _, e := range in.Refs {
		if e.ID == citer {
			sawCiter = true
		}
	}
	if !sawCiter {
		t.Errorf("the survivor did not absorb the citation: %+v", in.Refs)
	}

	// ⛔ AND THE LOSER IS RETRACTED RATHER THAN DELETED, so `record.get` can
	// still tell a reader where the fact went. Deleting it would answer the
	// duplicate case by losing the evidence that there ever was one.
	got, err := s.Get(tctx, loser)
	if err != nil {
		t.Fatalf("the replaced record is unreadable: %v", err)
	}
	if got.Retraction == nil {
		t.Fatal("the replaced record is not retracted, so it still reads as live")
	}
	if got.Retraction.ReplacedBy != survivor {
		t.Errorf("the retraction does not name the survivor: %+v", got.Retraction)
	}
}

// ⛔ AN EDGE THE SURVIVOR ALREADY HAS IS A MERGE AND NOT A CONFLICT, and an
// edge FROM the survivor would become a self-edge. Both are reported rather
// than silently dropped: a replace that says "moved 1" over three edges is the
// same reassuring lie the delete report exists to refuse.
func TestReplaceMergesADuplicateEdgeAndRefusesToBuildASelfEdge(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	loser := rec(t, s, "B90", "work-item", "the copy")
	survivor := rec(t, s, "B91", "work-item", "the one we keep")
	citer := rec(t, s, "B92", "work-item", "cites both")

	for _, e := range []Edge{
		{Src: citer, Type: LinkCites, Dst: loser},
		{Src: citer, Type: LinkCites, Dst: survivor}, // the survivor already has it
		{Src: survivor, Type: LinkCites, Dst: loser}, // would become a self-edge
	} {
		if err := s.Link(tctx, e.Src, e.Type, e.Dst); err != nil {
			t.Fatal(err)
		}
	}

	r, err := s.Replace(tctx, ReplaceRequest{
		Old: loser, New: survivor, Session: "c", Seat: "s", Epoch: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := edgeSet(r.Merged); !reflect.DeepEqual(got,
		[]string{citer + " -cites-> " + loser}) {
		t.Errorf("the duplicate edge was not reported as merged: %v", got)
	}
	if got := edgeSet(r.Dropped); !reflect.DeepEqual(got,
		[]string{survivor + " -cites-> " + loser}) {
		t.Errorf("the would-be self-edge was not reported as dropped: %v", got)
	}

	in, err := s.Refs(tctx, RefsRequest{ID: survivor, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range in.Refs {
		if e.ID == survivor {
			t.Fatal("replace built a self-edge on the survivor")
		}
	}
}

func TestReplacingARecordWithItselfIsRefused(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	id := rec(t, s, "B90", "work-item", "x")
	if _, err := s.Replace(tctx, ReplaceRequest{
		Old: id, New: id, Session: "c", Seat: "s", Epoch: 7,
	}); err == nil {
		t.Fatal("replacing a record with itself was allowed, which would " +
			"retract the survivor and call it a replacement")
	}
}

func TestReplacingEitherSideUnknownIsRefused(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	id := rec(t, s, "B90", "work-item", "x")
	var nf *NotFoundError

	if _, err := s.Replace(tctx, ReplaceRequest{
		Old: "nope", New: id, Session: "c", Seat: "s", Epoch: 7,
	}); !errors.As(err, &nf) {
		t.Errorf("replacing an unknown old gave %v, want NotFoundError", err)
	}
	if _, err := s.Replace(tctx, ReplaceRequest{
		Old: id, New: "nope", Session: "c", Seat: "s", Epoch: 7,
	}); !errors.As(err, &nf) {
		t.Errorf("replacing INTO an unknown survivor gave %v, want NotFoundError - "+
			"it would move every inbound edge onto nothing", err)
	}
}

// ⛔ EVERYBODY IS HIS WORD. Nothing in this package may grow a permission
// check: §42's default is "absolutely without restrictions", and a seat that
// ships one of these behind an owner test has built the wrong product.
func TestAnyoneCanRetractDeleteAndReplaceWhoeverWroteTheRecord(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	mine := rec(t, s, "B90", "work-item", "written by one seat")
	theirs := rec(t, s, "B91", "work-item", "written by the same seat")
	third := rec(t, s, "B92", "work-item", "also theirs")

	if _, err := s.Retract(tctx, RetractRequest{
		ID: mine, Session: "someone-else", Seat: "a-different-seat", Epoch: 99,
	}); err != nil {
		t.Errorf("a different seat could not retract: %v", err)
	}
	if _, err := s.Replace(tctx, ReplaceRequest{
		Old: theirs, New: third, Session: "someone-else",
		Seat: "a-different-seat", Epoch: 99,
	}); err != nil {
		t.Errorf("a different seat could not replace: %v", err)
	}
	if _, err := s.Delete(tctx, DeleteRequest{ID: third}); err != nil {
		t.Errorf("a different seat could not delete: %v", err)
	}
}
