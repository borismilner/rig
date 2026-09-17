package record

import "fmt"

// Section is one of section 39's eleven brief sections.
//
// ⛔ NAMED RATHER THAN NUMBERED, AND THAT IS DELIBERATE EVEN THOUGH THE WIRE
// NUMBERS THEM 1..11. A Section that happened to equal its wire number invites
// a cast, and a cast is a mapping nobody can see going wrong: add a section on
// one side and the numbers silently mean different things. Strings force the
// daemon to write the correspondence out, where a switch over a closed set can
// be read and tested. It also matches this package's own idiom - KindNote,
// LinkPartOf and the stage vocabulary are all string constants.
//
// THE STORE DOES NOT IMPORT THE WIRE AND MUST NOT. internal/record is below
// proto/; the daemon maps. That is why this type exists at all rather than the
// derivation returning rigv1 values.
type Section string

const (
	SectionOpen             Section = "open"
	SectionNextUp           Section = "next_up"
	SectionNotes            Section = "notes"
	SectionBlocked          Section = "blocked"
	SectionDrift            Section = "drift"
	SectionMustRead         Section = "must_read"
	SectionProjectionBehind Section = "projection_behind"
	SectionPending          Section = "pending"
	SectionLocalOnly        Section = "local_only"
	SectionFeatures         Section = "features"
	SectionCaseNotes        Section = "case_notes"

	// SectionGoverning is the TWELFTH, and it is the only one section 39 did
	// not specify. B64: three of section 39's ten kinds - decision,
	// requirement, artefact - had no section, so a decision put into rig was
	// reachable only by a caller who already knew to ask for kind=decision.
	//
	// ⛔ TWELVE RATHER THAN A FOLD INTO AN EXISTING ROW, AND THE ARGUMENT IS
	// MECHANICAL. Boris delegated the choice - "I don't care twelfth section or
	// fold - I trust you to make the best decision" - and statuses() below is
	// what decides it: a section added without both halves makes every brief
	// FAIL, loudly, including the one the window polls. A half-done fold has no
	// such property; it renders, and a decision arrives disguised as a note.
	// The failure mode that cannot be missed beats the one that cannot be seen.
	//
	// ⛔ IT DOES NOT CONTRADICT "Cover all of them". That ruling is that no
	// section may be SILENTLY ABSENT. A twelfth that declares its own state
	// obeys it; a fold that hides one kind inside another's row is the thing it
	// was made against.
	SectionGoverning Section = "governing"
)

// briefSections is section 39's eleven, in the section's own order, plus the
// twelfth that B64 added.
//
// EVERY BRIEF ANSWERS ALL ELEVEN. Boris, 2026-09-16, asked directly whether the
// four that shipped were enough: "Cover all of them." A section that cannot be
// answered says so; it is not omitted, because absence hides the capability gap
// and an empty list lies in the reassuring direction.
var briefSections = []Section{
	SectionOpen, SectionNextUp, SectionNotes, SectionBlocked,
	SectionDrift, SectionMustRead, SectionProjectionBehind,
	SectionPending, SectionLocalOnly, SectionFeatures, SectionCaseNotes,
	SectionGoverning,
}

// SectionState is whether a section's answer means anything.
type SectionState string

const (
	// SectionComputed - derived. An empty list now means there is nothing.
	SectionComputed SectionState = "computed"

	// SectionNotComputed - the input this section derives from does not exist
	// yet, and Reason names WHICH.
	SectionNotComputed SectionState = "not_computed"

	// SectionWithheldByView - the derivation ran and this caller is not being
	// shown it.
	//
	// ⛔ THE DERIVATION NEVER EMITS THIS AND THAT IS A DESIGN DECISION, NOT AN
	// OMISSION. A view is a property of who is asking, and the store does not
	// know who is asking - section 39's view table drops the must-read set from
	// the HUMAN view because it is not a decision he makes. So this state is
	// the daemon's to apply, over the top of whatever the derivation returned.
	// It is declared here so that the two vocabularies are the same one and the
	// daemon is not inventing a value the store has never heard of.
	//
	// IT MUST NEVER BE COLLAPSED INTO SectionNotComputed. Reporting a view rule
	// as a missing input makes the human view read as a degraded agent view.
	SectionWithheldByView SectionState = "withheld_by_view"
)

// SectionStatus is one section's state, and why when it is not computed.
type SectionStatus struct {
	Section Section
	State   SectionState

	// Reason is why, in the caller's terms, and it is REQUIRED whenever State
	// is not SectionComputed.
	//
	// ⛔ IT CANNOT BE A CONSTANT AND IT CANNOT LIVE AT THE WIRE. The derivation
	// is the only thing that knows which input is missing: "the standards
	// register does not exist" is a fact about what rig can hold, and a section
	// this derivation simply does not collect yet is a different fact with the
	// same state. A caller told only NOT_COMPUTED learns nothing it can act on.
	Reason string
}

// sectionsWaitingOn is why each unanswerable section is unanswerable.
//
// ⛔ A SECTION IS IN EXACTLY ONE OF THIS MAP AND THE DERIVATION'S OWN MARKS,
// AND briefStatuses REFUSES ANY BRIEF WHERE THAT IS NOT TRUE. That is the whole
// mechanism: the states used to live in the daemon as a hand-kept list whose
// comment claimed it derived itself from record.Brief's struct, and two rows
// went stale within a day when this package grew Notes and Features. Moving the
// list here does not by itself stop it going stale - what stops it is that a
// section can no longer be silently unaccounted for.
//
// ⛔ AND THE STRUCT-DERIVED VERSION IS WRONG, WHICH IS WHY THIS IS NOT CLEVER.
// Whether a section is computABLE is a property of the code; whether its slice
// is populated is a property of the data. A project with genuinely no notes
// returns an empty Notes, so "non-empty means computed" would report that
// project's notes as NOT_COMPUTED - the empty-reads-as-missing defect arriving
// inside the mechanism built to separate the two.
var sectionsWaitingOn = map[Section]string{
	SectionDrift: "the standards register does not exist. standard.stamp and " +
		"standard.drift are slice 7 and are deliberately off this wire, so " +
		"nothing can be behind a standard rig cannot yet hold",

	SectionMustRead: "neither half of the must-read gate is built: the SET is " +
		"records marked in the store, and the MARK is per-session state keyed " +
		"on the session Token. Both are ruled for slice 2 in PLAN.md section " +
		"39. Until they exist an empty must_read set means UNKNOWN, never " +
		"\"this project requires nothing\"",

	SectionProjectionBehind: sectionsWaitOnProjection,
	SectionPending:          sectionsWaitOnProjection,
	SectionLocalOnly:        sectionsWaitOnProjection,
}

// sectionNotForThisContainer is why a section does not apply to the container
// being briefed, which is NOT the same fact as an input being missing.
//
// ⛔ THE WIRE HAS NO STATE FOR "not applicable" AND THAT IS THE EMPTY-VERSUS-
// ABSENT ARGUMENT ARRIVING INSIDE THE MECHANISM BUILT TO SETTLE IT. Section 39
// rules that project.brief takes a container of kind project OR case, so
// section 11 is answered for one and meaningless for the other. A caller gets
// not_computed either way and cannot tell "will never be answered for this
// container" from "will be answered when slice 7 lands". The distinction
// survives only in this string. Reported to the team-lead; the enum is its file.
func sectionNotForThisContainer(kind string) string {
	switch kind {
	case "":
		return "section 39 row 11 is a case's attention_n notes, and this " +
			"container has no record in the store, so its kind is unknown and " +
			"the brief cannot say whether the section applies"
	default:
		return "section 39 row 11 is a case's attention_n notes and this " +
			"brief is for a " + kind + ". It is not missing an input: it does " +
			"not apply to this container. A brief for a case answers it"
	}
}

// sectionsWaitOnProjection is the one reason sections 7, 8 and 9 share.
//
// Written once rather than three times: three copies of one sentence drift into
// three slightly different sentences, and a reader comparing them looks for a
// distinction that was never meant.
const sectionsWaitOnProjection = "the git projection does not exist yet, so " +
	"there is nothing to be behind. PLAN.md section 39's spine, and it is what " +
	"makes the degraded path readable at all"

// sectionLedger records which sections a derivation actually answered.
//
// ⛔ THE MARK GOES AT THE CODE THAT EARNS IT, not in a table at the end. A
// table at the end is the hand-kept list again, one file further down: it can
// be written before the derivation exists and it can outlive it. Marking at the
// call site means the claim and the work are deleted together.
type sectionLedger struct {
	answered map[Section]bool

	// waiting is this brief's reasons, not a package-level constant, because
	// ⛔ WHETHER A SECTION IS WAITING DEPENDS ON THE CONTAINER. Section 11 is
	// derived for a case and inapplicable to a project, so a single global
	// table would make a case brief report it as both derived and waiting -
	// which statuses() refuses, correctly.
	waiting map[Section]string
}

// newSectionLedger opens a ledger for a container of the given kind.
func newSectionLedger(kind string) *sectionLedger {
	w := make(map[Section]string, len(sectionsWaitingOn)+1)
	for k, v := range sectionsWaitingOn {
		w[k] = v
	}
	if kind != KindCase {
		w[SectionCaseNotes] = sectionNotForThisContainer(kind)
	}
	return &sectionLedger{answered: map[Section]bool{}, waiting: w}
}

// answered records that this derivation produced section s.
func (l *sectionLedger) did(s Section) { l.answered[s] = true }

// statuses closes the ledger into the eleven statuses a brief carries.
//
// ⛔ IT REFUSES RATHER THAN GUESSING, AND THE REFUSAL IS THE POINT. A section
// that is neither marked answered nor given a reason is a programming error -
// somebody added a row to briefSections and wired neither half - and the two
// failure shapes it prevents are the ones this whole mechanism exists for: a
// section silently absent, and a NOT_COMPUTED whose reason is blank. Both read
// as "nothing to see here".
//
// A section marked BOTH ways is refused too. It means the derivation answers
// something a table still says it is waiting on, which is exactly how the
// daemon's rows went stale - except that this time it cannot ship.
func (l *sectionLedger) statuses() ([]SectionStatus, error) {
	out := make([]SectionStatus, 0, len(briefSections))
	for _, s := range briefSections {
		why, waiting := l.waiting[s]
		switch {
		case l.answered[s] && waiting:
			return nil, fmt.Errorf("record: section %q is both derived and "+
				"listed as waiting on %q - the derivation answers it, so the "+
				"row saying it is waiting is stale and must go", s, why)
		case l.answered[s]:
			out = append(out, SectionStatus{Section: s, State: SectionComputed})
		case waiting:
			out = append(out, SectionStatus{
				Section: s, State: SectionNotComputed, Reason: why,
			})
		default:
			// ⛔ THE COUNTS ARE DERIVED, NOT WRITTEN. This message read "ten
			// of eleven ... the eleventh" until B64 made it twelve, at which
			// point it was a caption asserting a number the code no longer
			// held - this repository's most-recorded defect class, produced
			// here by the change that added the twelfth section. A sentence
			// that cannot go stale is cheaper than one more instance.
			return nil, fmt.Errorf("record: section %q has no state - it is "+
				"neither derived nor listed as waiting on anything, so a brief "+
				"would answer %d of %d and say nothing about the %dth",
				s, len(briefSections)-1, len(briefSections), len(briefSections))
		}
	}
	return out, nil
}
