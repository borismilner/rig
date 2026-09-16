package record

import (
	"testing"
)

// cardProject is the one project these tests derive against.
//
// A parameter would read as generality the tests do not have - every call would
// pass the same string - and golangci-lint says so through unparam. A test
// needing a second project writes it inline, the way the cross-boundary case
// below writes its note's project directly.
const cardProject = "card"

// item writes a work-item with whatever card fields the test cares about.
func item(t *testing.T, s *Store, id string, fields map[string]string) {
	t.Helper()
	if fields == nil {
		fields = map[string]string{}
	}
	if _, ok := fields["status"]; !ok {
		fields["status"] = "active"
	}
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "work-item", Project: cardProject, Body: "b",
		Fields: fields, Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing %s: %v", id, err)
	}
}

// only returns the single ItemState a one-item brief carries, from whichever
// list it landed in, so a test about the CARD does not also assert the ordering.
func only(t *testing.T, s *Store) ItemState {
	t.Helper()
	b, err := s.Brief(tctx, cardProject)
	if err != nil {
		t.Fatalf("brief of %s: %v", cardProject, err)
	}
	got := append(append([]ItemState{}, b.NextUp...), b.Open...)
	if len(got) != 1 {
		t.Fatalf("expected exactly one item in the brief, got %d", len(got))
	}
	return got[0]
}

// ⛔ EVERY FIELD SECTION 39 ROW 2 NAMES, ASSERTED SEPARATELY.
//
// One assertion per field rather than a struct comparison, because a struct
// comparison fails once and names the whole value: a card missing `semver` and a
// card missing all nine produce the same failure, and the first is the one that
// will actually happen. Each field here is given a DISTINCT value, so a
// derivation reading the wrong key cannot pass by coincidence - which is what a
// table of empty strings would allow.
func TestTheCompactCardCarriesEveryFieldRowTwoNames(t *testing.T) {
	s := openStore(t, estate(t, cardProject))
	item(t, s, "wi-1", map[string]string{
		"title":             "the title",
		"description_short": "one line for a list",
		"description_long":  "the paragraph the card must NOT carry",
		"priority":          "high",
		"status":            "active",
		"owner":             "backend-record",
		"tags":              EncodeTags([]string{"wire", "ops"}),
		"target_date":       "2026-10-01",
		"semver":            "0.3.1",
	})

	got := only(t, s)
	for _, tc := range []struct{ field, got, want string }{
		{"Title", got.Title, "the title"},
		{"DescriptionShort", got.DescriptionShort, "one line for a list"},
		{"Priority", got.Priority, "high"},
		{"Status", got.Status, "active"},
		{"Owner", got.Owner, "backend-record"},
		{"TargetDate", got.TargetDate, "2026-10-01"},
		{"Semver", got.Semver, "0.3.1"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
	if len(got.Tags) != 2 || got.Tags[0] != "wire" || got.Tags[1] != "ops" {
		t.Errorf("Tags = %v, want [wire ops] in that order", got.Tags)
	}

	// ⛔ THE CARD IS NEVER description_long, AND NOTHING ELSE ASSERTS IT.
	// Row 2 says so in as many words. A struct with no field for it cannot
	// carry it, so this is a guard against a later seat adding one for
	// convenience rather than a test of today's code - which is exactly the
	// kind this package keeps discovering it needed.
	for _, v := range []string{got.Title, got.DescriptionShort, got.Note, got.Semver} {
		if v == "the paragraph the card must NOT carry" {
			t.Errorf("description_long reached the compact card")
		}
	}
}

// ⛔ status AND State ARE DIFFERENT FIELDS AND THIS IS THE TEST THAT SAYS SO.
//
// Section 39 keeps them apart on purpose: status is whether the item has been
// picked up, State is the latest progress step. A derivation that read one into
// the other would pass every other test in this package, because every other
// test writes status "active" and a step state of "started" and never compares
// them. Here they are deliberately different words.
func TestStatusIsTheRecordsAndStateIsTheLatestStep(t *testing.T) {
	s := openStore(t, estate(t, cardProject))
	item(t, s, "wi-1", map[string]string{"title": "t", "status": "active"})
	if _, err := s.Step(tctx, StepRequest{
		Item: "wi-1", State: "blocked", Project: cardProject, Note: "waiting on the wire",
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("stepping: %v", err)
	}

	got := only(t, s)
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q - it is the RECORD's field", got.Status, "active")
	}
	if got.State != "blocked" {
		t.Errorf("State = %q, want %q - it is the latest STEP's", got.State, "blocked")
	}
	if got.Status == got.State {
		t.Errorf("Status and State are both %q: one field is being read into both", got.Status)
	}
	if got.Note != "waiting on the wire" {
		t.Errorf("Note = %q, want the latest step's words", got.Note)
	}
}

// ⛔ HasNote IS ABOUT A note RECORD, NOT ABOUT THE STEP'S NOTE FIELD.
//
// The distinction is the whole reason the flag exists, and it is the one a
// reader is most likely to collapse: `ItemState.Note` is right there and it is
// non-empty for every item with a progress stream. An implementation that set
// HasNote from it would report every stepped item as carrying a comment, which
// is the reassuring-lie direction - a reader told there is a question waiting
// goes and finds a status line.
func TestHasNoteIsAnAttachedRecordAndNotTheStepsOwnWords(t *testing.T) {
	s := openStore(t, estate(t, cardProject))
	item(t, s, "wi-stepped", map[string]string{"title": "has a busy stream"})
	item(t, s, "wi-noted", map[string]string{"title": "has a comment"})

	// A step on the first item only. Its Note field will be non-empty.
	if _, err := s.Step(tctx, StepRequest{
		Item: "wi-stepped", State: "started", Project: cardProject,
		Note: "plenty of words here", Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("stepping: %v", err)
	}
	// A note record on the second, attached the way section 39 row 3 says.
	if _, err := s.Put(tctx, PutRequest{
		ID: "n-1", Kind: KindNote, Project: cardProject, Body: "is this still needed?",
		Session: "s", Seat: "boris",
	}); err != nil {
		t.Fatalf("writing the note: %v", err)
	}
	if err := s.Link(tctx, "n-1", LinkPartOf, "wi-noted"); err != nil {
		t.Fatalf("attaching the note: %v", err)
	}

	b, err := s.Brief(tctx, cardProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	byID := map[string]ItemState{}
	for _, i := range append(append([]ItemState{}, b.NextUp...), b.Open...) {
		byID[i.ID] = i
	}
	if len(byID) != 2 {
		t.Fatalf("expected both items in the brief, got %d", len(byID))
	}

	stepped, noted := byID["wi-stepped"], byID["wi-noted"]
	if stepped.Note == "" {
		t.Fatal("the premise of this test is broken: the stepped item has no step note")
	}
	if stepped.HasNote {
		t.Errorf("wi-stepped has a step note %q and NO note record, and HasNote is true: "+
			"the flag is being read off ItemState.Note", stepped.Note)
	}
	if !noted.HasNote {
		t.Errorf("wi-noted has a note record attached part-of it and HasNote is false")
	}
	if noted.Note != "" {
		t.Fatal("the premise of this test is broken: the noted item should have no steps")
	}
}

// ⛔ A NOTE FROM ANOTHER PROJECT STILL COUNTS, AND THAT IS THE SCOPING DECISION.
//
// Section 39 rules that links MAY cross a project boundary. Scoping the flag on
// the NOTE's project rather than the item's would drop exactly the notes written
// while working on something else - the ones most likely to carry something this
// project has not noticed - and the item would read as having no comment.
func TestANoteFromAnotherProjectStillFlagsTheItem(t *testing.T) {
	s := openStore(t, estate(t, cardProject))
	item(t, s, "wi-1", map[string]string{"title": "ours"})
	if _, err := s.Put(tctx, PutRequest{
		ID: "n-1", Kind: KindNote, Project: "somewhere-else", Body: "seen from over here",
		Session: "s", Seat: "boris",
	}); err != nil {
		t.Fatalf("writing the note: %v", err)
	}
	if err := s.Link(tctx, "n-1", LinkPartOf, "wi-1"); err != nil {
		t.Fatalf("attaching across the boundary: %v", err)
	}
	if got := only(t, s); !got.HasNote {
		t.Error("a note in another project attached to this item did not set HasNote: " +
			"the query is scoped on the note rather than on the item")
	}
}

// Tags round-trip, and a value that is not a JSON array is NO TAGS rather than
// a refusal. A brief reads whatever is in the store, and one hand-written field
// on one record out of forty-five must not take the whole answer down.
func TestTagsRoundTripAndAMalformedValueIsNoTagsNotAnError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		want  []string
	}{
		{"a written list", EncodeTags([]string{"a", "b"}), []string{"a", "b"}},
		{"one tag", EncodeTags([]string{"solo"}), []string{"solo"}},
		{"an empty list encodes to nothing", EncodeTags(nil), nil},
		{"absent", "", nil},
		{"hand-written commas", "a,b", nil},
		{"truncated JSON", `["a",`, nil},
		{"a JSON object", `{"a":1}`, nil},
		{"a JSON array of the wrong type", `[1,2]`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openStore(t, estate(t, cardProject))
			item(t, s, "wi-1", map[string]string{"title": "t", "tags": tc.field})
			got := only(t, s).Tags
			if len(got) != len(tc.want) {
				t.Fatalf("Tags = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Tags = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
