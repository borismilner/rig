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
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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

	f := map[string]string{
		"title":      e.Title,
		"source":     source,
		fieldSection: strconv.Itoa(e.Section),
		fieldLevel:   strconv.Itoa(e.Level),
		fieldDocLine: strconv.Itoa(e.Line),
	}

	// ⛔ THE SHORT DESCRIPTION IS THE DOCUMENT'S OWN LEAD AND NOT A CUT OF THE
	// TITLE. `shortOf(title)` is what a backlog row gets, and on this grain it
	// would be the GAP 1 defect restated: 116 of these titles SHOUT and 103 run
	// past 70 characters, so a cut of one is a second copy of a thing the
	// reader has already read. This project's house style leads a block with
	// its fact in bold, so the lead IS the one-line form the document wrote.
	// Where there is no lead to read - a table, a fence, an empty body - the
	// field is ABSENT, which is the answer `headingIntent` already gives.
	if short := docLead(e.Body); short != "" {
		f["description_short"] = short
	}

	// ⛔ THE SECTION TAG, SPELLED THE ONE WAY `sectioned` SPELLS IT. Until this
	// commit a requirement record carried NO tags at all, so the one grain the
	// whole plan import exists for was the only one with nothing to group by -
	// `plan/11`'s fourth 2026-09-17 ruling reached the backlog and the
	// decisions and stopped at the document it was about.
	//
	// ⛔ AND NO GRAIN TAG BESIDE IT. `tagHeadingBorne` and `tagRankOnly` mark
	// what is IRREGULAR about a row; nothing about a plan heading is, and a tag
	// every record of a kind carries groups nothing.
	if tag := record.FriendlyTag(e.SectionTitle); tag != "" {
		f[fieldTags] = record.EncodeTags([]string{tagSection + tag})
	}

	return intent{
		id:     e.Key,
		kind:   record.KindRequirement,
		grain:  grainPlanHeading,
		title:  e.Title,
		body:   e.Body,
		fields: f,
		partOf: e.PartOf,
	}
}

// docLead is the one-line form a section file already wrote for one heading.
//
// ⛔ IT READS THE DOCUMENT AND COMPOSES NOTHING. `plan/11` lets a seat compose
// a description where that is a real benefit; this is the cheaper half of the
// same job, and it is the rule GAP 1 settled for the project record - a writer
// fills these from what the document already says. 214 of the 379 headings
// open with a bold lead and 36 open with Boris's own words in a blockquote, so
// the lead is READ in 66% of cases rather than invented in any.
//
// The rule, in order:
//
//  1. a table or a fenced block         -> nothing; the field is ABSENT
//  2. a blockquote                      -> the quote, usually his own words
//  3. a list                            -> its first item
//  4. a paragraph that is only a label  -> the block it introduces
//  5. anything else                     -> its opening sentences, to the budget
//
// ⛔ RULE 4 IS WORTH ITS LINES AND WAS MEASURED, NOT GUESSED. 48 of the 214
// bold leads end in a colon - `**BORIS, 2026-09-17, verbatim:**` and its
// siblings - and 38 of those introduce a blockquote. Without the rule, one
// record in eight would carry a label where its statement belongs.
func docLead(body string) string {
	lines := strings.Split(body, "\n")
	for {
		lines = dropBlank(lines)
		if len(lines) == 0 {
			return ""
		}

		head := strings.TrimSpace(lines[0])
		if strings.HasPrefix(head, "|") ||
			strings.HasPrefix(head, "```") || strings.HasPrefix(head, "~~~") {
			return ""
		}
		if strings.HasPrefix(head, ">") {
			return fillSentences(stripEmphasis(quoteBlock(lines)))
		}

		para := paragraph(lines)
		if listMarker.MatchString(head) {
			para = listItem(lines)
		}

		// A label alone in its paragraph introduces the block under it, so read
		// that instead. A label with its statement beside it is left whole -
		// `**Measured:** today they are the same thing` reads correctly.
		if isBareLabel(para) {
			lines = nextBlock(lines)
			continue
		}
		return fillSentences(stripEmphasis(para))
	}
}

// isBareLabel reports whether a paragraph is nothing but a bold run ending in
// a colon - `**BORIS, 2026-09-17, verbatim:**` - which introduces the block
// under it rather than saying anything itself.
func isBareLabel(para string) bool {
	m := leadBold.FindStringSubmatch(para)
	if len(m) != 2 || !strings.HasSuffix(strings.TrimSpace(m[1]), ":") {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(para, m[0])) == ""
}

// leadBold matches a paragraph's opening bold run, past this document's
// decoration marks. The capture is the text inside it.
var leadBold = regexp.MustCompile(`^[⛔✅!]*\s*\*\*(.+?)\*\*`)

// listMarker is a bullet or a numbered item at the start of a line.
var listMarker = regexp.MustCompile(`^([-*+]|\d+[.)])\s+`)

// dropBlank skips leading blank lines.
func dropBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}

// paragraph is the leading run of non-blank lines, joined the way a reader
// reads them - as one line.
func paragraph(lines []string) string {
	var out []string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			break
		}
		out = append(out, strings.TrimSpace(l))
	}
	return strings.Join(out, " ")
}

// nextBlock is what is left after the leading paragraph and the blank lines
// under it.
func nextBlock(lines []string) []string {
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
		i++
	}
	return lines[i:]
}

// quoteBlock is the leading run of blockquote lines with their markers off.
func quoteBlock(lines []string) string {
	var out []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, ">") {
			break
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(t, ">")))
	}
	return strings.Join(out, " ")
}

// stripEmphasis removes the markdown a plain-text field must not carry.
//
// ⛔ THE LINK RULE KEEPS THE TEXT AND DROPS THE TARGET, because a
// `description_short` is read in a list where a URL is noise and the words in
// the brackets are the sentence.
func stripEmphasis(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdBold.ReplaceAllString(s, "$1")
	s = mdItalic.ReplaceAllString(s, "$1")
	s = mdCode.ReplaceAllString(s, "$1")
	return strings.Join(strings.Fields(s), " ")
}

var (
	mdLink   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdItalic = regexp.MustCompile(`\*(.+?)\*`)
	mdCode   = regexp.MustCompile("`([^`]+)`")
)

// listItem is the first item of a list, marker off, with its continuation
// lines but NOT the item after it.
func listItem(lines []string) string {
	var out []string
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			break
		}
		if i == 0 {
			out = append(out, strings.TrimSpace(listMarker.ReplaceAllString(t, "")))
			continue
		}
		if listMarker.MatchString(t) {
			break
		}
		out = append(out, t)
	}
	return strings.Join(out, " ")
}

// fillSentences is the opening of a passage, cut at a sentence end rather than
// mid-word wherever one fits the budget.
//
// ⛔ IT TAKES MORE THAN ONE SENTENCE ON PURPOSE, and the reason is measured.
// This document's house style leads a block with a bold fact, and where that
// fact is one word - `**Presence.** What AgentBox does today, kept because it
// works.` - a single-sentence rule yields `Presence.`, which is a label rather
// than a description. Filling to the budget yields the whole line and still
// stops at a sentence end.
//
// ⛔ AND A SENTENCE ENDS AT PUNCTUATION FOLLOWED BY A SPACE, so `§37.` and
// `2026-09-17.` inside a sentence do not end it early. A first sentence that
// alone overruns the budget falls back to `shortOf`'s word-boundary cut, which
// is the one cut rule this seeder has.
func fillSentences(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	out := ""
	for _, end := range sentenceEnds(s) {
		if utf8.RuneCountInString(s[:end]) > shortWidth {
			break
		}
		out = s[:end]
	}
	if out == "" {
		return shortOf(s)
	}
	return shortOf(out)
}

// sentenceEnds is every offset in s just past a sentence end, and finally the
// end of s itself - so a passage with no terminal punctuation is still offered
// whole.
func sentenceEnds(s string) []int {
	var out []int
	for _, m := range sentenceEnd.FindAllStringIndex(s, -1) {
		out = append(out, m[0]+1)
	}
	return append(out, len(s))
}

var sentenceEnd = regexp.MustCompile(`[.!?]["')\]]?\s`)

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
