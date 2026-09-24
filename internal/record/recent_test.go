package record

import (
	"errors"
	"testing"
)

// appendAs writes one record under a named seat, which is the predicate these
// tests are about.
func appendAs(t *testing.T, s *Store, seat, kind, body string, partOf ...string) Appended {
	t.Helper()
	got, err := s.Append(tctx, AppendRequest{
		Kind: kind, Project: "rig", Body: body, PartOf: partOf,
		Session: "unix:" + seat, Seat: seat, Epoch: 7,
	})
	if err != nil {
		t.Fatalf("Append as %s: %v", seat, err)
	}
	return got
}

func bodies(r Recent) []string {
	out := make([]string, 0, len(r.Records))
	for _, rec := range r.Records {
		out = append(out, rec.Body)
	}
	return out
}

// ⛔ THE SEAT IS THE KEY, AND ANOTHER SEAT'S NOTES ARE NEVER IN THE ANSWER.
// This read exists to answer "MINE", so a filter that widened would hand one
// agent another agent's working notes while looking like a successful narrow
// read.
func TestASeatReadAnswersOnlyThatSeatsRecords(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	appendAs(t, s, "p1", "working-note", "p1 first")
	appendAs(t, s, "p2", "working-note", "p2 first")
	appendAs(t, s, "p1", "working-note", "p1 second")
	appendAs(t, s, "p1", "decision", "p1 decided something")

	got, err := s.FindBySeat(tctx, SeatFilter{Seat: "p1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 || len(got.Records) != 3 {
		t.Fatalf("seat p1 got %d of %d, want 3 of 3: %v", len(got.Records), got.Total, bodies(got))
	}
	for _, rec := range got.Records {
		if rec.Prov.Seat != "p1" {
			t.Errorf("a %s record is in p1's answer: %q", rec.Prov.Seat, rec.Body)
		}
	}
	// NEWEST FIRST, which is the order a resuming agent reads its own work in.
	if got.Records[0].Body != "p1 decided something" || got.Records[2].Body != "p1 first" {
		t.Errorf("order = %v, want newest first", bodies(got))
	}
	// AND THE KIND NARROWS WITHIN THE SEAT, which is how a working-note read
	// avoids the seat's other records.
	notes, err := s.FindBySeat(tctx, SeatFilter{Seat: "p1", Kind: "working-note", Limit: 10})
	if err != nil || notes.Total != 2 {
		t.Fatalf("p1's working notes: %d of %d, want 2: %v", len(notes.Records), notes.Total, err)
	}
	// AND THE PROJECT NARROWS INDEPENDENTLY OF THE KIND, so all four query
	// shapes are exercised rather than only the two a note happens to use.
	inRig, err := s.FindBySeat(tctx, SeatFilter{Seat: "p1", Project: "rig", Limit: 10})
	if err != nil || inRig.Total != 3 {
		t.Fatalf("p1 in rig: %d, want 3: %v", inRig.Total, err)
	}
	both, err := s.FindBySeat(tctx, SeatFilter{Seat: "p1", Kind: "decision", Project: "rig", Limit: 10})
	if err != nil || both.Total != 1 {
		t.Fatalf("p1's decisions in rig: %d, want 1: %v", both.Total, err)
	}
	elsewhere, err := s.FindBySeat(tctx, SeatFilter{Seat: "p1", Project: "docket", Limit: 10})
	if err != nil || elsewhere.Total != 0 || len(elsewhere.Records) != 0 {
		t.Fatalf("p1 in a project it never wrote to: %+v %v", elsewhere, err)
	}
}

// ⛔ Total IS NOT len(Records), AND THAT IS THE FIELD'S ONLY REASON TO EXIST.
// An answer that is cut without saying so is indistinguishable from a
// complete one, and section 09's A6 wants the notes back "without reading
// everything" - which means the answer is always cut.
func TestACutAnswerSaysItWasCut(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	for i := range 7 {
		appendAs(t, s, "p2", "working-note", "note "+itoa(uint32(i)))
	}

	cut, err := s.FindBySeat(tctx, SeatFilter{Seat: "p2", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(cut.Records) != 3 || cut.Total != 7 || !cut.More() {
		t.Fatalf("cut = %d of %d, More()=%v; want 3 of 7 and true", len(cut.Records), cut.Total, cut.More())
	}
	// THE CUT TAKES THE NEWEST, not an arbitrary three.
	if cut.Records[0].Body != "note 6" || cut.Records[2].Body != "note 4" {
		t.Errorf("the cut answer is %v, want the three newest", bodies(cut))
	}

	whole, err := s.FindBySeat(tctx, SeatFilter{Seat: "p2", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if whole.More() || whole.Total != 7 || len(whole.Records) != 7 {
		t.Errorf("an uncut answer claims to be cut: %d of %d", len(whole.Records), whole.Total)
	}
}

// ⛔ A RETRACTED ROW LEAVES BOTH THE ROWS AND THE COUNT, because countSelect
// mirrors findSelect including the retraction predicate. A count that
// disagreed with the rows beside it would report "3 of 4" over three live
// records, which is a claim about the store that is false.
func TestARetractedRecordLeavesBothTheRowsAndTheCount(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	keep := appendAs(t, s, "p3", "working-note", "kept")
	gone := appendAs(t, s, "p3", "working-note", "withdrawn")

	if _, err := s.Retract(tctx, RetractRequest{
		ID: gone.Record.ID, Reason: "wrong on the facts",
		Session: "unix:p3", Seat: "p3", Epoch: 7,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.FindBySeat(tctx, SeatFilter{Seat: "p3", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Records) != 1 || got.Records[0].ID != keep.Record.ID {
		t.Fatalf("a withdrawn record is still in the answer: %d of %d, %v", len(got.Records), got.Total, bodies(got))
	}
	if got.More() {
		t.Error("the count kept the withdrawn row while the rows dropped it")
	}
}

// ⛔ AN EMPTY SEAT IS A REFUSAL AND NOT A WILDCARD, unlike QueryFilter's empty
// fields: this read answers "MINE", so widening it silently is the failure.
func TestASeatReadRefusesTheArgumentsItCannotAnswer(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	if _, err := s.FindBySeat(tctx, SeatFilter{Limit: 10}); !errors.Is(err, ErrNoSeat) {
		t.Errorf("an empty seat gave %v, want ErrNoSeat", err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := s.FindBySeat(tctx, SeatFilter{Seat: "p1", Limit: limit}); !errors.Is(err, ErrPageLimit) {
			t.Errorf("limit %d gave %v, want ErrPageLimit", limit, err)
		}
	}
	// AN UNKNOWN SEAT IS AN ANSWER, not an error: it wrote nothing.
	got, err := s.FindBySeat(tctx, SeatFilter{Seat: "nobody-here", Limit: 10})
	if err != nil || got.Total != 0 || got.More() {
		t.Errorf("an unknown seat gave %+v %v, want an empty answer", got, err)
	}
}

// ⛔ THE OTHER HALF OF A2, read backwards: "what is attached to this" answered
// with the RECORDS rather than with refs, so a caller does not pay one
// record.get per row to read the prose it asked for.
func TestAnAttachmentReadAnswersTheRecordsAndNotJustTheirIds(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	item := rec(t, s, "B10", "work-item", "the item")
	other := rec(t, s, "B11", "work-item", "another item")
	appendAs(t, s, "p1", "working-note", "note on the item", item)
	appendAs(t, s, "p2", "working-note", "another note on the item", item)
	appendAs(t, s, "p1", "artefact", "an artefact of the item", item)
	appendAs(t, s, "p1", "working-note", "note on the other one", other)

	got, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: LinkPartOf, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 || len(got.Records) != 3 {
		t.Fatalf("attached to %s: %d of %d, want 3 of 3: %v", item, len(got.Records), got.Total, bodies(got))
	}
	if got.Records[0].Body != "an artefact of the item" {
		t.Errorf("order = %v, want newest first", bodies(got))
	}
	// THE PROSE IS THERE, which is what "records rather than refs" means.
	if got.Records[1].Body == "" || got.Records[1].Prov.Seat == "" {
		t.Errorf("an attached record carries no body or no provenance: %+v", got.Records[1])
	}
	// AND THE KIND NARROWS, so a note read does not collect the artefacts.
	notes, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: LinkPartOf, Kind: "working-note", Limit: 10})
	if err != nil || notes.Total != 2 {
		t.Fatalf("working notes on %s: %d, want 2: %v", item, notes.Total, err)
	}
	// ONE ROW IN, AT MOST ONE ROW OUT: a second edge of another type between
	// the same pair must not double the record in the answer.
	if err := s.Link(tctx, notes.Records[0].ID, LinkCites, item); err != nil {
		t.Fatal(err)
	}
	again, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: LinkPartOf, Limit: 10})
	if err != nil || again.Total != 3 || len(again.Records) != 3 {
		t.Fatalf("a second edge multiplied the row: %d of %d", len(again.Records), again.Total)
	}
}

// AN UNTYPED TRAVERSAL IS REFUSED, because it would mix a note attached to an
// item with the item's own progress steps, which use the same edge.
func TestAnAttachmentReadRefusesTheArgumentsItCannotAnswer(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	item := rec(t, s, "B12", "work-item", "the item")
	if _, err := s.FindAttachedTo(tctx, AttachedFilter{Type: LinkPartOf, Limit: 10}); !errors.Is(err, ErrNoAttachment) {
		t.Errorf("no target gave %v, want ErrNoAttachment", err)
	}
	for _, typ := range []string{"", "part_of", "related"} {
		if _, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: typ, Limit: 10}); err == nil {
			t.Errorf("link type %q was accepted", typ)
		}
	}
	if _, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: LinkPartOf}); !errors.Is(err, ErrPageLimit) {
		t.Errorf("a zero limit was accepted")
	}
	// A RECORD NOBODY ATTACHED ANYTHING TO IS AN ANSWER, not an error.
	got, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: LinkPartOf, Limit: 10})
	if err != nil || got.Total != 0 {
		t.Errorf("an unattached record gave %+v %v, want an empty answer", got, err)
	}
}
