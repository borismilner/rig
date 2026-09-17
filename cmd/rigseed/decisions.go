// The decisions half of the seeder: DECISIONS.md into rig, through the CLI.
//
// ⛔ IT IS THE SAME PROGRAM AND NOT A SECOND ONE, AND THAT IS THE WHOLE
// ARGUMENT FOR THIS FILE EXISTING RATHER THAN A `rigdecseed`. `--check` is an
// answer rather than a second opinion only because the detector compares the
// store against the intents the seeder would write. Two seeders would need two
// detectors, and the day they disagreed nobody would know which was right -
// which is this project's most expensive recorded failure with the documents
// swapped for the programs that read them.
//
// ⛔ AND IT IS NOT A SECOND PARSER OF DECISIONS.md EITHER. Everything about
// that document's shape belongs in record.ParseDecisionsDocument; this file
// only decides what a parsed entry becomes in the store.
package main

import (
	"os"
	"strconv"

	"github.com/boris-milner/rig/internal/record"
)

// grainEntry is the one grain a decisions document states anything at.
//
// ⛔ ONE GRAIN AND NOT TWO, BECAUSE THE KIND ALREADY CARRIES THE DIFFERENCE.
// The backlog needs `row` and `heading` because the EVIDENCE differs - a row
// has a state cell and a heading has none. Here every entry is a markdown
// heading with prose under it and there is no second kind of evidence; what
// differs between a ruling and a standing section is what it IS, which the
// record's `kind` states. A second grain string would be `kind` spelled twice,
// and two spellings of one fact is how a projection starts disagreeing with
// itself.
const grainEntry = "entry"

// The fields one entry of the decisions document carries, named once because
// the seeder writes them and the detector reads them back.
const (
	fieldDocKey  = "doc-key"
	fieldDocLine = "doc-line"
	fieldDocDate = "doc-date"
	fieldLevel   = "level"
)

// readDecisions reads the whole decisions document, not only the entries it
// could import - for the reason readBacklog gives: an absence no instrument can
// report survives every review.
func readDecisions(path string) (record.DecisionParse, error) {
	f, err := os.Open(path)
	if err != nil {
		return record.DecisionParse{}, err
	}
	defer f.Close()
	return record.ParseDecisionsDocument(f)
}

// kindFor is the record kind one parsed entry becomes.
//
// ⛔ A STANDING SECTION IS A `note` AND NOT AN INVENTED `section` KIND. RULED
// by the team-lead, 2026-09-17. `note` is one of section 39's ten and its own
// doc comment says a note is a record attached to a project - which is exactly
// what "What rig is" or "Still open, and they are Boris's to call" is. The
// alternative was measured and refused: `kind` is not a closed set and `Put`
// refuses no value, so a kind nobody else knows about would be accepted
// silently and be wrong quietly, reachable only by a caller who already knew
// to ask for it. That is the defect section 12 was built to end, not one to
// re-open.
func kindFor(e record.DecisionEntry) string {
	if e.Kind == record.EntrySection {
		return record.KindNote
	}
	return record.KindDecision
}

// decisionIntent is the record one entry of the decisions document becomes.
//
// ⛔ NO `status`, AND THAT IS NOT THE HEADING-BORNE MISTAKE REPEATED. A work
// item without `status` is imported into invisibility because the brief's open
// list selects `status == "active"`; section 12 - which is where a decision is
// rendered - selects nothing and lists every record of the kind (`brief.go`,
// `governing`). So the field would change no derivation, and writing `active`
// on a ruling would assert that a settled decision is work somebody is holding.
// The document states a DATE, not a state, and the date is carried.
//
// ⛔ NO `description_short` EITHER. On a row it is the one-line form of a
// title that has no prose behind it; here the entry's own BODY is the
// statement and it is written as the record's body. A second copy of the title
// under a field name nothing reads is a field a later reader has to work out
// the meaning of.
func decisionIntent(o options, e record.DecisionEntry) intent {
	f := map[string]string{
		"title":      e.Title,
		"source":     o.decisions,
		fieldDocKey:  e.Key,
		fieldDocLine: strconv.Itoa(e.Line),
		fieldLevel:   strconv.Itoa(e.Level),
	}

	// ⛔ NO doc-date FIELD AT ALL WHERE THE DOCUMENT STATES NO DATE, RATHER
	// THAN AN EMPTY ONE. The parser refuses to inherit a date from the heading
	// above precisely because a fabricated date reads exactly like a measured
	// one; an empty string stored under `doc-date` is the same claim in a
	// thinner disguise - it says the document wrote a date and it was blank.
	// 338 of the 457 entries are in this case, so it is the common shape and
	// not an edge.
	if e.Date != "" {
		f[fieldDocDate] = e.Date
	}

	return intent{
		id:     e.Key,
		kind:   kindFor(e),
		grain:  grainEntry,
		title:  e.Title,
		body:   e.Body,
		fields: f,
		partOf: e.PartOf,
	}
}

// addDecisions extends the plan with everything the decisions document states.
//
// ⛔ IT TAKES THE `stated` SET RATHER THAN BUILDING ITS OWN, SO THE TWO
// DOCUMENTS CANNOT SILENTLY CLAIM ONE ID. The store keys on id alone and knows
// nothing about which document a record came from, so two puts against one id
// in one run supersede each other and whichever went last wins - a silent wrong
// answer decided by the order this function is called in. A backlog id is
// `B\d+` and a decision key is a title slug, so a collision is not expected;
// the point is that it would be REPORTED the day it happens rather than
// discovered as a record whose content nobody can account for.
func (p *plan) addDecisions(o options, dec record.DecisionParse, stated map[string]bool) {
	for _, e := range dec.Decisions {
		if stated[e.Key] {
			p.collided = append(p.collided, e.Key)
			continue
		}
		stated[e.Key] = true
		p.want = append(p.want, decisionIntent(o, e))
	}

	// Everything the decisions document addresses and no entry carries. It is
	// empty over rig's own document today, which is a fact about the document
	// rather than about this loop, and `{}` is printed either way.
	for _, u := range dec.Unimported {
		p.unimported = append(p.unimported, docUnimported{doc: o.decisions, Unimported: u})
	}
}
