// ⛔ EVERY CASE IN THIS FILE IS A HEADING ACTUALLY IN ONE OF THIS PROJECT'S
// DOCUMENTS. Invented headings would prove the function does what it says and
// nothing about whether a person can read its output, which is the whole bar.
// `plan/39`.
package record

import "testing"

func TestAFriendlyTagIsReadableWhereTheRawSlugIsNot(t *testing.T) {
	for _, c := range []struct {
		heading string
		want    string
		why     string
	}{
		{
			heading: "⛔ B46 - THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT ALL",
			want:    "mvp-acceptance-test",
			why:     "his own example. The raw slug is 61 characters of id, shouting and editorial",
		},
		{
			heading: "Open",
			want:    "open",
			why:     "already friendly, and the rule must not damage it",
		},
		{
			heading: "The critical path to the gate",
			want:    "critical-path-to-the-gate",
			why:     "no clause boundary, so the whole heading survives minus its article",
		},
		{
			heading: "DATED ITEMS - the only place in this project where a date is allowed",
			want:    "dated-items",
			why:     "the dash is the boundary; everything after it explains rather than names",
		},
		{
			heading: "Rejected, kept so the argument is not had twice",
			want:    "rejected",
			why:     "the comma is the boundary",
		},
		{
			heading: "Held for Boris, not backlog items",
			want:    "held-for-boris",
			why:     "the subject is before the comma",
		},
		{
			heading: "The admission bar, which is the charter's and does not relax",
			want:    "admission-bar",
			why:     "article dropped AND clause cut, which is both steps on one heading",
		},
		{
			heading: "M7, ordered by what this team ACTUALLY uses today",
			want:    "m7-ordered-by-what-this-team",
			why:     "⛔ the first clause is `M7`, which is the very value he called useless, so the heading keeps its second clause and the cap bounds it",
		},
		{
			heading: "How the first dogfooding cycle actually went, recorded because the loop is new",
			want:    "how-the-first-dogfooding-cycle",
			why:     "44 characters after the cut, so the cap truncates it - on a word boundary",
		},
		{
			heading: "⛔ B46d - THE CYCLE IS CONSTRUCTED, AND THE RELAY SAID OTHERWISE",
			want:    "cycle-is-constructed",
			why:     "a sub-lettered id leads too, and `the` goes with it",
		},
		{
			heading: "",
			want:    "",
			why:     "no heading is no tag. A tag reading `section:` says the document stated a place and names none",
		},
	} {
		t.Run(c.want, func(t *testing.T) {
			if got := FriendlyTag(c.heading); got != c.want {
				t.Errorf("FriendlyTag(%q)\n = %q\nwant %q\n  (%s)", c.heading, got, c.want, c.why)
			}
		})
	}
}

// ⛔ THE CAP NEVER CUTS A WORD IN HALF: a reader cannot tell
// `mvp-acceptance-te` from a heading that says that, so a mid-word cut
// invents a value rather than shortening one.
func TestTheCapFallsOnAWordBoundary(t *testing.T) {
	long := "Search and filter over every field a record carries and then some"
	got := FriendlyTag(long)
	if len(got) > friendlyTagMax {
		t.Fatalf("FriendlyTag returned %d characters, cap is %d: %q", len(got), friendlyTagMax, got)
	}
	if got == "" {
		t.Fatal("a long heading must still produce a tag")
	}
	// The value must be a prefix of the uncapped slug at a hyphen boundary.
	full := decisionSlug("Search and filter over every field a record carries and then some")
	if got != capOnWord(full, friendlyTagMax) {
		t.Errorf("got %q, want the word-bounded cap of %q", got, full)
	}
	for _, r := range got {
		if r == ' ' {
			t.Errorf("a tag must not contain a space: %q", got)
		}
	}
}

// ⛔ AN ID INSIDE A HEADING IS PART OF WHAT THE HEADING SAYS. Only a LEADING
// id is a label, which is why the pattern is anchored.
func TestOnlyALeadingIdIsDroppedFromATag(t *testing.T) {
	if got, want := FriendlyTag("Why B52 was never a defect"), "why-b52-was-never-a-defect"; got != want {
		t.Errorf("FriendlyTag = %q, want %q - the id is the subject here", got, want)
	}
}
