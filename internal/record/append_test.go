package record

import (
	"errors"
	"strings"
	"testing"
)

// appendOne is the happy shape, so a test that is about one refusal does not
// restate the other six fields.
func appendOne(t *testing.T, s *Store, body string, partOf ...string) Appended {
	t.Helper()
	got, err := s.Append(tctx, AppendRequest{
		Kind: "working-note", Project: "rig", Body: body,
		PartOf:  partOf,
		Session: "unix:1", Seat: "backend-record", Epoch: 7,
	})
	if err != nil {
		t.Fatalf("Append(%q): %v", body, err)
	}
	return got
}

// ⛔ SECTION 09 A1's ACCEPTANCE TEST IS "SO THAT NOTHING IS EVER LOST", AND
// THIS IS THE CASE THAT DECIDES WHETHER IT HOLDS. A caller that mistyped one
// of three targets must keep the prose it was trying to attach; the edge is
// what it loses, and it is told exactly which one.
func TestAnAppendKeepsTheProseWhenAnAttachmentIsMissing(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	item := rec(t, s, "B1", "work-item", "the item")

	got := appendOne(t, s, "what I thought at 14:00", item, "no-such-record")

	if len(got.Attached) != 1 || got.Attached[0] != item {
		t.Errorf("Attached = %v, want just %s", got.Attached, item)
	}
	if len(got.Missing) != 1 || got.Missing[0] != "no-such-record" {
		t.Errorf("Missing = %v, want the one bad id", got.Missing)
	}
	// THE PROSE SURVIVED, which is the half the requirement is about.
	back, err := s.Get(tctx, got.Record.ID)
	if err != nil || back.Body != "what I thought at 14:00" {
		t.Fatalf("the record did not survive its missing attachment: %+v %v", back, err)
	}
	if back.Version != 1 || back.Prov.Seat != "backend-record" {
		t.Errorf("version %d seat %q, want 1 and the appending seat", back.Version, back.Prov.Seat)
	}
	// AND THE EDGE THAT DID TAKE IS REAL, not just reported.
	in, err := s.FindAttachedTo(tctx, AttachedFilter{To: item, Type: LinkPartOf, Limit: 10})
	if err != nil || in.Total != 1 || in.Records[0].ID != got.Record.ID {
		t.Fatalf("the reported edge is not in the store: %+v %v", in, err)
	}
}

// ⛔ ONE TRANSACTION, WHICH IS THE WHOLE REASON Append EXISTS RATHER THAN Put
// PLUS Link. The gap between two calls is where a killed process leaves a
// record no edge reaches. It is asserted from the caller's side, which is the
// only side a caller has: every id the answer calls Attached is reachable
// backwards, and a failed append leaves NO record behind at all.
func TestAnAppendWritesTheRecordAndItsEdgesTogetherOrNeither(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	item := rec(t, s, "B2", "work-item", "the item")
	other := rec(t, s, "B3", "work-item", "another item")

	got := appendOne(t, s, "attached to both", item, other)
	for _, dst := range got.Attached {
		in, err := s.FindAttachedTo(tctx, AttachedFilter{To: dst, Type: LinkPartOf, Limit: 10})
		if err != nil || in.Total != 1 {
			t.Fatalf("%s has %d attached records, want the note: %v", dst, in.Total, err)
		}
	}

	// A REFUSED APPEND LEAVES NOTHING. The refusal below happens before the
	// transaction opens, so the assertion is that the store's population did
	// not move - the observable form of "neither".
	before, err := s.Find(tctx, QueryFilter{Project: "rig"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(tctx, AppendRequest{
		Kind: "working-note", Project: "rig", Body: "never written",
		PartOf:  []string{item, ""},
		Session: "unix:1", Seat: "backend-record",
	}); err == nil {
		t.Fatal("an empty attachment id was accepted")
	}
	after, err := s.Find(tctx, QueryFilter{Project: "rig"})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("a refused append wrote %d records", len(after)-len(before))
	}
}

// ⛔ DUPLICATES COLLAPSE AND ORDER IS THE CALLER'S, so Attached is an honest
// account rather than an echo: a caller that named one target twice is not
// told it took two edges.
func TestAnAppendReportsTheAttachmentsItActuallyTook(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	a := rec(t, s, "B4", "work-item", "a")
	b := rec(t, s, "B5", "work-item", "b")

	got := appendOne(t, s, "twice named", b, a, b)
	if len(got.Attached) != 2 || got.Attached[0] != b || got.Attached[1] != a {
		t.Errorf("Attached = %v, want [%s %s] in the caller's order with the duplicate gone", got.Attached, b, a)
	}
	if len(got.Missing) != 0 {
		t.Errorf("Missing = %v, want empty", got.Missing)
	}
}

// ⛔ THE BOUND IS REFUSED RATHER THAN TRUNCATED. Truncating writes the record
// and silently drops the association, which is the success-that-lost-something
// this package refuses everywhere.
func TestAnAppendRefusesMoreAttachmentsThanItWillHold(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	many := make([]string, MaxAttachments+1)
	for i := range many {
		many[i] = "id-" + itoa(uint32(i))
	}
	_, err := s.Append(tctx, AppendRequest{
		Kind: "working-note", Project: "rig", Body: "too many",
		PartOf:  many,
		Session: "unix:1", Seat: "backend-record",
	})
	if !errors.Is(err, ErrTooManyAttachments) {
		t.Fatalf("err = %v, want ErrTooManyAttachments", err)
	}
	// AND EXACTLY THE BOUND IS ACCEPTED, so the refusal is off by nothing.
	got, err := s.Append(tctx, AppendRequest{
		Kind: "working-note", Project: "rig", Body: "at the bound",
		PartOf:  many[:MaxAttachments],
		Session: "unix:1", Seat: "backend-record",
	})
	if err != nil {
		t.Fatalf("exactly %d attachments was refused: %v", MaxAttachments, err)
	}
	if len(got.Missing) != MaxAttachments {
		t.Errorf("%d of %d bad ids were reported missing", len(got.Missing), MaxAttachments)
	}
}

// ⛔ THE TWO RULES Put ENFORCES AND NO THIRD ONE, so the store stays
// kind-agnostic (plan/50 decision 5) while the two structural exceptions
// survive: a progress step has one door, and a slug-identified kind cannot
// take a minted id.
func TestAnAppendRefusesOnlyTheShapesPutAlreadyRefuses(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	ok := AppendRequest{Kind: "working-note", Project: "rig", Body: "b", Session: "unix:1", Seat: "backend-record"}
	for name, mut := range map[string]func(*AppendRequest){
		"no kind":     func(r *AppendRequest) { r.Kind = "" },
		"no project":  func(r *AppendRequest) { r.Project = "" },
		"no session":  func(r *AppendRequest) { r.Session = "" },
		"no seat":     func(r *AppendRequest) { r.Seat = "" },
		"a progress":  func(r *AppendRequest) { r.Kind = KindProgress },
		"a project":   func(r *AppendRequest) { r.Kind = KindProject },
		"a case":      func(r *AppendRequest) { r.Kind = KindCase },
		"an empty id": func(r *AppendRequest) { r.PartOf = []string{""} },
	} {
		r := ok
		mut(&r)
		if _, err := s.Append(tctx, r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// AND AN UNKNOWN KIND IS FINE, which is what kind-agnostic means.
	r := ok
	r.Kind = "a-kind-nobody-declared"
	if _, err := s.Append(tctx, r); err != nil {
		t.Errorf("an undeclared kind was refused, so the store is not kind-agnostic: %v", err)
	}
}

// AN APPEND MINTS ITS OWN ID, which is SWEEP-4's requirement in plan/09: human
// ids collide with generated note ids by construction, so a note must never
// take one.
func TestAnAppendMintsItsOwnIdAndCannotBeHandedOne(t *testing.T) {
	s := openStore(t, estate(t, "development"))
	first := appendOne(t, s, "one")
	second := appendOne(t, s, "two")
	if first.Record.ID == second.Record.ID {
		t.Fatal("two appends minted the same id")
	}
	for _, got := range []Appended{first, second} {
		if len(got.Record.ID) != 36 || strings.Count(got.Record.ID, "-") != 4 {
			t.Errorf("id %q is not a UUID, so it could collide with a human id", got.Record.ID)
		}
	}
	// NEWEST LAST BY MINTING ORDER: UUIDv7 is time-ordered, and the read side
	// relies on the stamp rather than on this. Asserted so a generator swap
	// that broke it would be caught here rather than in a paging test.
	if first.Record.ID >= second.Record.ID {
		t.Errorf("%s was minted before %s and does not sort before it", first.Record.ID, second.Record.ID)
	}
}
