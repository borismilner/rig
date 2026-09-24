package worknote

import (
	"context"
	"strings"
	"testing"

	"github.com/borismilner/rig/internal/record"
)

var tctx = context.Background()

func store(t *testing.T) *record.Store {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, err := record.Open("development")
	if err != nil {
		t.Fatalf("opening the record store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func write(t *testing.T, s *record.Store, seat, body string, tags []string, partOf ...string) Note {
	t.Helper()
	n, err := Write(tctx, s, WriteRequest{
		Project: "rig", Body: body, Tags: tags, PartOf: partOf,
		Session: "unix:" + seat, Seat: seat, Epoch: 7,
	})
	if err != nil {
		t.Fatalf("Write(%q): %v", body, err)
	}
	return n
}

// item writes something for a note to attach to, through the plain record
// door, which is what a real caller attaches to.
func item(t *testing.T, s *record.Store, id, title string) string {
	t.Helper()
	w, err := s.Put(tctx, record.PutRequest{
		ID: id, Kind: "work-item", Project: "rig", Body: title,
		Fields:  map[string]string{"title": title},
		Session: "unix:lead", Seat: "lead", Epoch: 1,
	})
	if err != nil {
		t.Fatalf("writing %s: %v", id, err)
	}
	return w.ID
}

// ⛔ THE KIND AND THE SURFACE WORD CANNOT DRIFT, AND THIS TEST IS WHY BOTH ARE
// SPELLED OUT HERE AS LITERALS. plan/09 A5 rules the stored kind is
// `working-note`; the verbs, tools and CLI say `worknote` because `note` is
// the word that left rig for docket and a `note.*` family would read as the
// planner returning. A kind is matched EXACTLY by record.query, so renaming
// one side would leave every note ever written unfindable from the other, and
// nothing but this assertion connects them.
func TestTheStoredKindAndTheSurfaceWordCannotDrift(t *testing.T) {
	if Kind != "working-note" {
		t.Fatalf("Kind = %q, want working-note (plan/09 A5)", Kind)
	}
	if strings.ReplaceAll(Kind, "-", "") != "workingnote" || !strings.HasPrefix(Kind, "work") {
		t.Fatalf("Kind = %q no longer matches the surface word worknote", Kind)
	}
	if Kind == "note" || Kind == "scratchpad" {
		t.Fatalf("Kind = %q, which plan/09 A5 refuses by name", Kind)
	}
}

// ⛔ A1's ACCEPTANCE TEST, WHICH IS THE FEATURE: the notes come back to the
// SEAT after the process that wrote them is gone. There is no process here, so
// what stands in for the death is that the read is keyed on the seat alone and
// the SESSION deliberately differs - which is exactly the state a resumed
// agent is in.
func TestASeatGetsItsNotesBackUnderADifferentSession(t *testing.T) {
	s := store(t)
	write(t, s, "p2", "what I thought at 14:00", []string{"design"})
	write(t, s, "p2", "and then at 14:30", []string{"design", "review"})
	write(t, s, "p1", "another seat's note", nil)

	// THE SECOND OCCUPANCY: same seat, a session that never wrote anything.
	got, err := Mine(tctx, s, MineRequest{Seat: "p2"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Notes) != 2 || got.More() {
		t.Fatalf("p2 got %d of %d, want 2 of 2", len(got.Notes), got.Total)
	}
	if got.Notes[0].Body != "and then at 14:30" {
		t.Errorf("order = %q first, want newest first", got.Notes[0].Body)
	}
	for _, n := range got.Notes {
		if n.Seat != "p2" {
			t.Errorf("a %s note is in p2's answer", n.Seat)
		}
		if n.Written == "" || !strings.HasSuffix(n.Written, "Z") {
			t.Errorf("note %s has no written stamp: %q", n.ID, n.Written)
		}
	}
	// AND THE TAGS SURVIVE THE ROUND TRIP AS A LIST, not as tag: fields.
	if strings.Join(got.Notes[0].Tags, ",") != "design,review" {
		t.Errorf("tags = %v, want design and review", got.Notes[0].Tags)
	}
	if len(got.Notes[0].Fields) != 0 {
		t.Errorf("the tag encoding leaked into Fields: %v", got.Notes[0].Fields)
	}
}

// ⛔ B65's ARGUMENT: a tag that cannot be selected on "is worse than no tag,
// because it looks like an index". This asserts the encoding is queryable
// through the record.query that ALREADY SHIPS, with no new verb, which is what
// D5 bought.
func TestATagIsSelectableThroughTheQueryThatAlreadyShips(t *testing.T) {
	s := store(t)
	write(t, s, "p2", "tagged for review", []string{"review", "api"})
	write(t, s, "p2", "not tagged for review", []string{"api"})
	write(t, s, "p2", "tagged nothing", nil)

	hits, err := s.Find(tctx, record.QueryFilter{
		Project: "rig", Kind: Kind, Field: TagPrefix + "review", Value: TagValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Body != "tagged for review" {
		t.Fatalf("the tag predicate found %d rows, want the one tagged review", len(hits))
	}
	// AND A TAG THAT IS A PREFIX OF ANOTHER DOES NOT MATCH IT, which a joined
	// tags field with a LIKE would have got wrong.
	write(t, s, "p2", "tagged reviewed", []string{"reviewed"})
	again, err := s.Find(tctx, record.QueryFilter{
		Project: "rig", Kind: Kind, Field: TagPrefix + "review", Value: TagValue,
	})
	if err != nil || len(again) != 1 {
		t.Fatalf("%d rows matched tag review, want 1: reviewed must not match it", len(again))
	}
}

// ⛔ A2: "a work-item, a project, a task, anything it wants" - and read
// backwards, every seat's notes on one thing, which is the handover.
func TestNotesAttachToAnythingAndComeBackFromIt(t *testing.T) {
	s := store(t)
	b1 := item(t, s, "B1", "the work item")
	b2 := item(t, s, "B2", "another work item")
	write(t, s, "p1", "p1 looked at B1", []string{"design"}, b1)
	write(t, s, "p2", "p2 looked at B1 too", nil, b1)
	write(t, s, "p2", "attached to both", nil, b1, b2)
	write(t, s, "p2", "attached to nothing", nil)

	got, err := About(tctx, s, AboutRequest{ID: b1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 {
		t.Fatalf("notes about B1: %d, want 3", got.Total)
	}
	// ⛔ ABOUT IS NOT FILTERED BY SEAT: it is what the ESTATE knows about the
	// item, which is the question a second agent picking it up has.
	seats := map[string]bool{}
	for _, n := range got.Notes {
		seats[n.Seat] = true
	}
	if !seats["p1"] || !seats["p2"] {
		t.Errorf("About answered only %v, so a handover cannot read the other seat's notes", seats)
	}

	// A MISSING TARGET KEEPS THE PROSE AND NAMES THE BAD ID, which is what
	// "nothing is ever lost" means when a caller mistypes one of two.
	n, err := Write(tctx, s, WriteRequest{
		Project: "rig", Body: "one good target, one typo", PartOf: []string{b2, "B99"},
		Session: "unix:p2", Seat: "p2", Epoch: 7,
	})
	if err != nil {
		t.Fatalf("a mistyped attachment lost the note: %v", err)
	}
	if len(n.Attached) != 1 || n.Attached[0] != b2 || len(n.Missing) != 1 || n.Missing[0] != "B99" {
		t.Errorf("attached %v missing %v, want B2 attached and B99 missing", n.Attached, n.Missing)
	}
	if n.ID == "" || n.Body != "one good target, one typo" {
		t.Errorf("the note itself did not survive: %+v", n)
	}
}

// ⛔ A6's DONE-WHEN IS "WITHOUT READING EVERYTHING", so the read defaults to a
// bound and says what it did not hand back.
func TestAReadIsBoundedAndSaysWhatItDidNotHandBack(t *testing.T) {
	s := store(t)
	for i := range DefaultLimit + 5 {
		write(t, s, "p2", "note "+string(rune('a'+i%26))+strings.Repeat("!", i), nil)
	}

	byDefault, err := Mine(tctx, s, MineRequest{Seat: "p2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byDefault.Notes) != DefaultLimit || byDefault.Total != uint64(DefaultLimit+5) || !byDefault.More() {
		t.Fatalf("default read = %d of %d More()=%v, want %d of %d and true",
			len(byDefault.Notes), byDefault.Total, byDefault.More(), DefaultLimit, DefaultLimit+5)
	}
	// AN OVER-LARGE LIMIT IS CLAMPED RATHER THAN REFUSED: Total tells the
	// caller what it did not get, so nothing is silent.
	clamped, err := Mine(tctx, s, MineRequest{Seat: "p2", Limit: MaxLimit * 10})
	if err != nil {
		t.Fatalf("an over-large limit was refused rather than clamped: %v", err)
	}
	if clamped.Total != uint64(DefaultLimit+5) || clamped.More() {
		t.Errorf("clamped read = %d of %d, want the whole set", len(clamped.Notes), clamped.Total)
	}
	// AND THE SEAT IS STILL REFUSED WHEN ABSENT, never read as every seat.
	if _, err := Mine(tctx, s, MineRequest{}); err == nil {
		t.Error("an empty seat was read as every seat")
	}
	if _, err := About(tctx, s, AboutRequest{}); err == nil {
		t.Error("an empty id was accepted by About")
	}
}

// ⛔ A3 IS "ANY TAGS IT WANTS", SO THE VALIDATION IS SHAPE AND NEVER
// VOCABULARY. Nothing here decides which tags are allowed; what is refused is
// a tag that could not be selected back out.
func TestATagIsValidatedForShapeAndNeverForVocabulary(t *testing.T) {
	s := store(t)
	// ANY WORD AT ALL, including ones no registry would have.
	odd := []string{"עברית", "B77", "plan/09", "x-2026-09-24", "🙂"}
	n, err := Write(tctx, s, WriteRequest{
		Project: "rig", Body: "odd tags", Tags: odd,
		Session: "unix:p2", Seat: "p2", Epoch: 7,
	})
	if err != nil {
		t.Fatalf("a tag was refused on vocabulary rather than shape: %v", err)
	}
	if len(n.Tags) != len(odd) {
		t.Errorf("tags = %v, want all %d", n.Tags, len(odd))
	}

	ok := WriteRequest{Project: "rig", Body: "b", Session: "unix:p2", Seat: "p2"}
	for name, mut := range map[string]func(*WriteRequest){
		"no body":          func(r *WriteRequest) { r.Body = "   " },
		"no project":       func(r *WriteRequest) { r.Project = "" },
		"no seat":          func(r *WriteRequest) { r.Seat = "" },
		"a huge body":      func(r *WriteRequest) { r.Body = strings.Repeat("x", MaxBody+1) },
		"an empty tag":     func(r *WriteRequest) { r.Tags = []string{"ok", ""} },
		"a tag with space": func(r *WriteRequest) { r.Tags = []string{"two words"} },
		"a huge tag":       func(r *WriteRequest) { r.Tags = []string{strings.Repeat("t", MaxTagLen+1)} },
		"too many tags":    func(r *WriteRequest) { r.Tags = make([]string, MaxTags+1) },
		"a tag as a field": func(r *WriteRequest) { r.Fields = map[string]string{TagPrefix + "sneaky": "1"} },
	} {
		r := ok
		mut(&r)
		if _, err := Write(tctx, s, r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// A DUPLICATE TAG COLLAPSES rather than being refused: a tag set is a set,
	// and asserting one twice carries no new information.
	dup, err := Write(tctx, s, WriteRequest{
		Project: "rig", Body: "duplicated", Tags: []string{"api", "api", "review"},
		Session: "unix:p2", Seat: "p2",
	})
	if err != nil || len(dup.Tags) != 2 {
		t.Errorf("duplicate tags gave %v %v, want two", dup.Tags, err)
	}
}

// FIELDS RIDE BESIDE THE PROSE and come back separated from the tags, so a
// caller never has to know the encoding to read back what it wrote.
func TestFieldsAndTagsComeBackSeparated(t *testing.T) {
	s := store(t)
	n, err := Write(tctx, s, WriteRequest{
		Project: "rig", Body: "the prose",
		Fields:  map[string]string{"title": "a short title", "status": "open"},
		Tags:    []string{"design"},
		Session: "unix:p2", Seat: "p2", Epoch: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	back, err := Mine(tctx, s, MineRequest{Seat: "p2"})
	if err != nil || len(back.Notes) != 1 {
		t.Fatalf("reading it back: %+v %v", back, err)
	}
	got := back.Notes[0]
	if got.ID != n.ID || got.Fields["title"] != "a short title" || got.Fields["status"] != "open" {
		t.Errorf("fields = %v, want title and status", got.Fields)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "design" {
		t.Errorf("tags = %v, want design", got.Tags)
	}
	if _, leaked := got.Fields[TagPrefix+"design"]; leaked {
		t.Error("the tag encoding leaked into Fields on the way back")
	}
	// A READ CARRIES NO ATTACHMENT ACCOUNT: Attached and Missing are what one
	// WRITE did, and only a write can have one.
	if len(got.Attached) != 0 || len(got.Missing) != 0 {
		t.Errorf("a read claimed an attachment account: %v %v", got.Attached, got.Missing)
	}
}
