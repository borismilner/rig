package daemon

import (
	"reflect"
	"testing"

	"github.com/boris-milner/rig/internal/record"
)

// ⛔ EVERY FIELD THE STORE COMPUTES MUST CROSS THE WIRE, AND THIS TEST EXISTS
// BECAUSE FIVE OF THEM DID NOT.
//
// The store's ItemState has carried description_short, priority, status, owner
// and tags since section 39 row 2. itemToWire copied five fields and silently
// left those behind, so the window rendered a truncated title with nothing
// behind it and clicking a row could only show the same line again. Nothing
// failed; the fields were simply absent, which is why it survived to a
// screenshot from Boris.
//
// ⛔ IT IS WRITTEN AS A FIELD-COUNT CHECK ON PURPOSE. Asserting five named
// values would pass again the next time somebody adds a sixth field to
// record.ItemState and forgets this function - which is the exact failure being
// fixed. This fails when the struct grows, and that is the point.
func TestEveryItemFieldTheStoreComputesReachesTheWire(t *testing.T) {
	// 13, and every one has been decided about rather than counted:
	//   ID Title State Since Note                   crossed before this change
	//   DescriptionShort Priority Status Owner Tags crossed as of 2026-09-17
	//   TargetDate Semver                           crossed as of 2026-09-17
	//   HasNote                                     DELIBERATELY not on the
	//     wire: it is exactly `note != ""`, and a derived bool beside the field
	//     it is derived from is a thing that can go stale on its own.
	const fieldsThisTestKnowsAbout = 13
	got := reflect.TypeOf(record.ItemState{}).NumField()
	if got != fieldsThisTestKnowsAbout {
		t.Fatalf("record.ItemState has %d fields, this test was written for %d.\n"+
			"A field was added or removed. Decide whether it belongs on the wire, "+
			"teach itemToWire about it, then update this count - do NOT just bump "+
			"the number, because that is how the last five went missing.",
			got, fieldsThisTestKnowsAbout)
	}
}

func TestTheCardFieldsCrossTheWireIntact(t *testing.T) {
	in := record.ItemState{
		ID:               "B62",
		Title:            "a work item stores what a human reviewer needs",
		DescriptionShort: "the row's own prose, cut at a word boundary",
		Priority:         "high",
		Status:           "active",
		Owner:            "team-lead",
		Tags:             []string{"window", "legibility"},
		TargetDate:       "2026-09-30",
		Semver:           "v0.1.0",
		Note:             "the latest step's words",
	}

	w := itemToWire(in)

	if w.GetDescriptionShort() != in.DescriptionShort {
		t.Errorf("description_short dropped: %q", w.GetDescriptionShort())
	}
	if w.GetPriority() != in.Priority {
		t.Errorf("priority dropped: %q", w.GetPriority())
	}
	// ⛔ status is the RECORD's, and it is not State. They have been confused
	// before, so the test names both.
	if w.GetStatus() != in.Status {
		t.Errorf("status dropped: %q", w.GetStatus())
	}
	if w.GetOwner() != in.Owner {
		t.Errorf("owner dropped: %q", w.GetOwner())
	}
	if !reflect.DeepEqual(w.GetTags(), in.Tags) {
		t.Errorf("tags dropped or reshaped: %v, want %v", w.GetTags(), in.Tags)
	}
	if w.GetTargetDate() != in.TargetDate {
		t.Errorf("target_date dropped: %q", w.GetTargetDate())
	}
	if w.GetSemver() != in.Semver {
		t.Errorf("semver dropped: %q", w.GetSemver())
	}
}

// ⛔ AN ITEM WITH NONE OF THESE MUST NOT INVENT ANY. A record written before
// these fields existed has them empty, and the brief answers for the store as
// it is rather than filling gaps.
func TestAnItemWithNoCardFieldsCrossesEmptyRatherThanInvented(t *testing.T) {
	w := itemToWire(record.ItemState{ID: "B1", Title: "bare"})
	if w.GetDescriptionShort() != "" || w.GetOwner() != "" || w.GetPriority() != "" {
		t.Errorf("a bare item arrived with content it never had: %+v", w)
	}
	if len(w.GetTags()) != 0 {
		t.Errorf("a bare item arrived with tags: %v", w.GetTags())
	}
}
