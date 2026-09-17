// The specification half of the seeder: `plan/NN-*.md` into rig, through the
// CLI.
//
// ⛔ IT IS THE SAME PROGRAM AND NOT A THIRD ONE, for the reason decisions.go
// gives about being the second: `--check` is an answer rather than a second
// opinion only because the detector compares the store against the intents the
// seeder would write. Three seeders would need three detectors, and the day two
// of them disagreed nobody would know which was right.
//
// ⛔ AND IT IS NOT A PARSER. Everything about `plan/`'s shape is in
// record.ParsePlanDir, which calls the SAME `decisionTitle` and `decisionSlug`
// the decisions import keys on. This file only decides what a parsed heading
// becomes in the store.
//
// ⛔ THE FILE IS `plandoc.go` AND NOT `plan.go` BECAUSE `plan` IS ALREADY A TYPE
// IN THIS PACKAGE - the seeder's own intent set, with `plan.kinds()`,
// `plan.grains()` and `plan.report()`. A file named for the document would read
// as that type's file and is not it.
package main

import (
	"errors"
	"os"
	"path"
	"strconv"

	"github.com/boris-milner/rig/internal/record"
)

// grainPlanHeading is the grain a section file states a requirement at.
//
// ⛔ ITS OWN GRAIN BECAUSE THE EVIDENCE BEHIND IT IS NOT THE OTHERS'. A backlog
// row has a state cell; a decisions entry has a date and a position that makes
// it a ruling or a section; a plan heading has NEITHER, and its section number
// comes from a FILE NAME rather than from anything written in the document. A
// report that merged them would hide which grain moved, and B66 exists because
// a whole grain was invisible.
const grainPlanHeading = "plan-heading"

// ⛔ THE FIELD NAMES ARE THE ONES THE OTHER TWO DOCUMENTS ALREADY USE, AND NOT
// A SECOND SPELLING. `fieldSection` is declared in main.go for the ranked rows
// and `fieldLevel` and `fieldDocLine` in decisions.go; a `section` written here
// under its own constant would be one field name with two definitions, which is
// how a detector starts reading a field the seeder no longer writes.

// readPlan reads the whole plan directory, not only the headings it could
// import - for the reason readBacklog gives: an absence no instrument can
// report survives every review.
//
// ⛔ THE DIRECTORY IS OPENED AS AN fs.FS AND NOTHING JOINS A CALLER'S STRING
// ONTO A PATH INSIDE IT. `os.DirFS` roots the parser at the one directory it
// was pointed at, so a section file cannot name its way out of it, and the
// parser never sees an absolute path at all.
func readPlan(dir string) (record.PlanParse, error) {
	if dir == "" {
		return record.PlanParse{}, errors.New("--plan-dir is empty; it names the directory " +
			"holding the plan/NN-*.md section files")
	}
	return record.ParsePlanDir(os.DirFS(dir))
}

// planEntryIntent is the record one heading of one section file becomes.
//
// ⛔ NO `status`, AND THIS IS THE FIELD THAT ALREADY BIT ONCE. B66 records the
// seeder writing no status for a heading-borne backlog id, which imported B46 -
// Boris's own MVP acceptance test - into invisibility, because the brief's open
// list selects `status == "active"`. The backlog was MENDED rather than
// excused: its strikethrough IS a machine-readable closure mark, so a status
// could be read off the document. ⛔ `plan/` HAS NO SUCH MARK. A heading is
// neither open nor closed in anything the file says, so a `status` written here
// would be a fact this seeder invented, and section 12 - which is where a
// requirement renders - selects nothing and lists every record of the kind.
//
// ⛔ NO `date`, `priority` OR `supersedes` EITHER, and the test is the same one
// B66 states: a field a machine cannot compute from the document is a field a
// human would have to judge. Some headings state a date and most do not, so a
// `date` field would be populated on a minority and empty on the rest, which is
// indistinguishable from one that was lost.
func planEntryIntent(o options, e record.PlanEntry) intent {
	// ⛔ `source` IS THE PATH A READER WOULD TYPE, NOT THE PARSER'S BASE NAME.
	// The parser is rooted at the directory, so it only ever sees
	// `39-the-continuity-record.md`; a `source` field carrying that would not
	// resolve to a file from the repository root, and a file:line that is
	// precise and wrong is worse than none.
	source := path.Join(o.planDir, e.File)

	return intent{
		id:    e.Key,
		kind:  record.KindRequirement,
		grain: grainPlanHeading,
		title: e.Title,
		body:  e.Body,
		fields: map[string]string{
			"title":      e.Title,
			"source":     source,
			fieldSection: strconv.Itoa(e.Section),
			fieldLevel:   strconv.Itoa(e.Level),
			fieldDocLine: strconv.Itoa(e.Line),
		},
		partOf: e.PartOf,
	}
}

// addPlan extends the plan with everything the specification states.
//
// ⛔ IT TAKES THE `stated` SET RATHER THAN BUILDING ITS OWN, SO THREE DOCUMENTS
// CANNOT SILENTLY CLAIM ONE ID. The store keys on id alone and knows nothing
// about which document a record came from, so two puts against one id in one
// run supersede each other and whichever went last wins - a silent wrong answer
// decided by the order this function is called in. A backlog id is `B\d+`, a
// decision key is a title slug and a plan key starts with a section number, so
// a collision is not expected; the point is that it is REPORTED the day it
// happens rather than discovered as a record nobody can account for.
func (p *plan) addPlan(o options, pp record.PlanParse, stated map[string]bool) {
	for _, e := range pp.Entries {
		if stated[e.Key] {
			p.collided = append(p.collided, e.Key)
			continue
		}
		stated[e.Key] = true
		p.want = append(p.want, planEntryIntent(o, e))
	}

	// ⛔ THE THIRTEEN `#####` HEADINGS LAND HERE, AND THREE OF THEM ARE BORIS'S
	// OWN RULINGS. They are outside the grain he ruled, so they are NOT
	// imported and `--check` exits 2 over them until somebody rules - which is
	// B73's precedent exactly, where eleven ranked backlog rows were imported
	// at a coarser grain and the non-empty set was called the point. The code
	// that would widen the grain is one character; the ruling is Boris's.
	for _, u := range pp.Unimported {
		p.unimported = append(p.unimported, docUnimported{
			doc: path.Join(o.planDir, unimportedFile(pp, u)), Unimported: u,
		})
	}
}

// unimportedFile is the section file one unimported heading was read from.
//
// ⛔ THE PARSE DOES NOT CARRY IT ON `Unimported` AND THIS RECOVERS IT RATHER
// THAN LEAVING THE LINE NUMBER POINTING AT A DIRECTORY. `Unimported` is
// internal/record's shared type across three documents and gaining a file field
// for one of them is that package's change to make, not this one's. Its
// `Section` field holds the section NUMBER, which names exactly one file, so
// the file is recoverable from the entries this parse did carry.
func unimportedFile(pp record.PlanParse, u record.Unimported) string {
	for _, e := range pp.Entries {
		if strconv.Itoa(e.Section) == u.Section {
			return e.File
		}
	}
	// A section file whose every heading was irregular carries no entry to read
	// the name off. The section number is still the honest answer, and it is
	// said as a number rather than dressed up as a path.
	return "section " + u.Section
}
