package record

import (
	"fmt"
	"strings"
	"testing"
)

const caseID = "c-health"

// caseRecord writes the container itself. attention_n is left off unless a test
// is about the override, so the default is what most of these exercise.
func caseRecord(t *testing.T, s *Store, attentionN string) {
	t.Helper()
	f := map[string]string{"title": "the health case", "status": "open"}
	if attentionN != "" {
		f["attention_n"] = attentionN
	}
	if _, err := s.Put(tctx, PutRequest{
		ID: caseID, Kind: KindCase, Project: caseID, Body: "b",
		Fields: f, Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("writing the case: %v", err)
	}
}

func caseNote(t *testing.T, s *Store, id, priority string) {
	t.Helper()
	if _, err := s.Put(tctx, PutRequest{
		ID: id, Kind: KindNote, Project: caseID, Body: "b",
		Fields:  map[string]string{"priority": priority},
		Session: "boris", Seat: "boris",
	}); err != nil {
		t.Fatalf("writing note %s: %v", id, err)
	}
	if err := s.Link(tctx, id, LinkPartOf, caseID); err != nil {
		t.Fatalf("attaching %s: %v", id, err)
	}
}

// ⛔ SECTION 11, AND THE DERIVATION HAD NEVER LOOKED AT THE CONTAINER'S KIND.
//
// Section 39 rules that project.brief takes a container of kind project OR
// case, and Brief produced project shapes for whatever id it was handed. So
// row 11 - "up to attention_n notes part-of this case, priority desc then
// created_at desc" - could not be answered at all, whoever asked.
func TestACaseBriefAnswersSectionElevenInPriorityOrder(t *testing.T) {
	s := openStore(t, estate(t, caseID))
	caseRecord(t, s, "")
	// Written in an order that is neither the id order nor the answer, so a
	// derivation that simply returned insertion order would be caught.
	caseNote(t, s, "n-b", "low")
	caseNote(t, s, "n-a", "medium")
	caseNote(t, s, "n-c", "high")

	b, err := s.Brief(tctx, caseID)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}

	if b.Kind != KindCase {
		t.Errorf("Kind = %q, want %q - the brief did not read its container", b.Kind, KindCase)
	}
	if len(b.CaseNotes) != 3 {
		t.Fatalf("section 11 has %d notes, want 3", len(b.CaseNotes))
	}
	for i, want := range []string{"high", "medium", "low"} {
		if got := b.CaseNotes[i].Priority; got != want {
			var order []string
			for _, n := range b.CaseNotes {
				order = append(order, n.Priority)
			}
			t.Errorf("case_notes[%d] is %q, want %q - got %v", i, got, want, order)
			break
		}
	}

	state := map[Section]SectionState{}
	for _, st := range b.Sections {
		state[st.Section] = st.State
	}
	if state[SectionCaseNotes] != SectionComputed {
		t.Errorf("on a CASE brief section %q is %q, want %q",
			SectionCaseNotes, state[SectionCaseNotes], SectionComputed)
	}
}

// ⛔ A CAP IS A LIMIT ON WHAT IS SHOWN, NEVER A CLAIM ABOUT WHAT EXISTS.
// Section 39: "Never padded to N", the same phrasing it uses for next_up_n.
//
// The two halves are asserted separately because they fail separately: a cap
// that does not cap returns everything, and a cap that pads returns exactly N
// from a store that holds fewer.
func TestSectionElevenCapsAtAttentionNAndNeverPadsToIt(t *testing.T) {
	s := openStore(t, estate(t, caseID))
	caseRecord(t, s, "2")
	for _, id := range []string{"n-1", "n-2", "n-3", "n-4"} {
		caseNote(t, s, id, "high")
	}

	b, err := s.Brief(tctx, caseID)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b.CaseNotes) != 2 {
		t.Errorf("attention_n is 2 and section 11 returned %d notes", len(b.CaseNotes))
	}

	// The same case with fewer notes than the cap must return what exists.
	s2 := openStore(t, estate(t, caseID+"-thin"))
	caseRecord(t, s2, "10")
	caseNote(t, s2, "n-only", "high")
	b2, err := s2.Brief(tctx, caseID)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b2.CaseNotes) != 1 {
		t.Errorf("attention_n is 10 over one note and section 11 returned %d - "+
			"a padded list tells a reader there are exactly that many",
			len(b2.CaseNotes))
	}
}

// A malformed or absent cap is the default, never a refusal and never zero.
//
// ⛔ ZERO IS THE DANGEROUS ONE. A cap of zero renders an empty section on a
// store that is full, which reads as "there is nothing here" - and a section
// that IS computed carries no reason to correct that impression. One
// hand-written field must not be able to silence a section.
//
// The estate name is indexed rather than built from the value under test:
// estate names take lowercase letters, digits and hyphens only, so "3.5" as a
// path component is refused and the test fails for a reason that has nothing to
// do with what it is asking. Found by running it.
func TestAMalformedAttentionNFallsBackRatherThanSilencingTheSection(t *testing.T) {
	for i, bad := range []string{"", "0", "-3", "ten", "  ", "1e2"} {
		s := openStore(t, estate(t, fmt.Sprintf("cap-%d", i)))
		caseRecord(t, s, bad)
		caseNote(t, s, "n-1", "high")

		b, err := s.Brief(tctx, caseID)
		if err != nil {
			t.Fatalf("attention_n=%q: brief: %v", bad, err)
		}
		if len(b.CaseNotes) != 1 {
			t.Errorf("attention_n=%q silenced section 11: %d notes, want 1",
				bad, len(b.CaseNotes))
		}
	}
}

// ⛔ A PREFIX THAT PARSES IS ACCEPTED, AND THAT IS NOT A FALLBACK - IT IS A
// PARTIAL PARSE. Sscanf with %d reads "3.5" as 3 and stops at the dot, so the
// cap becomes 3 rather than the default 10.
//
// ASSERTED RATHER THAN CORRECTED, and the reasoning is that it is INHERITED
// behaviour, not new: next_up_n has read its field this way since the brief was
// written, and tightening it here would make two caps in one file disagree
// about the same malformed input. It is written down so the next reader finds a
// measurement rather than a surprise, and so that changing it is a decision
// about both caps at once.
func TestAPrefixThatParsesIsTakenAsTheCapRatherThanFallingBack(t *testing.T) {
	s := openStore(t, estate(t, "cap-prefix"))
	caseRecord(t, s, "2.9")
	for _, id := range []string{"n-1", "n-2", "n-3", "n-4"} {
		caseNote(t, s, id, "high")
	}

	b, err := s.Brief(tctx, caseID)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if len(b.CaseNotes) != 2 {
		t.Errorf("attention_n=%q gave %d notes, want 2 - Sscanf takes the "+
			"parsable prefix, so this is 2 and not the default 10", "2.9",
			len(b.CaseNotes))
	}
}

// ⛔ ON A PROJECT, SECTION 11 IS NOT APPLICABLE - NOT "not built yet".
//
// The wire has no state for that, so the reason string is the only place the
// distinction survives. This asserts the string says so, because it is the
// whole of the difference a caller can see. Reported to the team-lead; the
// enum is its file.
func TestSectionElevenOnAProjectSaysItDoesNotApplyRatherThanThatItIsMissing(t *testing.T) {
	s := openStore(t, estate(t, sectProject))
	project(t, s, "5")

	b, err := s.Brief(tctx, sectProject)
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if b.Kind != KindProject {
		t.Errorf("Kind = %q, want %q", b.Kind, KindProject)
	}
	if len(b.CaseNotes) != 0 {
		t.Errorf("a project brief carries %d case notes, want none", len(b.CaseNotes))
	}

	var got SectionStatus
	for _, st := range b.Sections {
		if st.Section == SectionCaseNotes {
			got = st
		}
	}
	if got.State != SectionNotComputed {
		t.Fatalf("section %q is %q on a project, want %q",
			SectionCaseNotes, got.State, SectionNotComputed)
	}
	if !strings.Contains(got.Reason, "does not apply") {
		t.Errorf("the reason does not say the section is inapplicable, so a "+
			"caller cannot tell it from a missing input: %q", got.Reason)
	}
}

// A container with no record at all: the kind is UNKNOWN, and saying "this is a
// project" would be an answer the store cannot support.
func TestAContainerWithNoRecordSaysItsKindIsUnknown(t *testing.T) {
	s := openStore(t, estate(t, "ghost"))

	b, err := s.Brief(tctx, "ghost")
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	if b.Kind != "" {
		t.Errorf("Kind = %q, want empty - there is no container record", b.Kind)
	}
	for _, st := range b.Sections {
		if st.Section != SectionCaseNotes {
			continue
		}
		if !strings.Contains(st.Reason, "unknown") {
			t.Errorf("with no container record the reason claims to know the "+
				"kind: %q", st.Reason)
		}
	}
}
