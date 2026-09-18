// Backlog parsing: a markdown work-item table read as records.
//
// ⛔ THIS LIVED IN acceptance_test.go UNTIL rig PROMOTED IT, AND THE REASON IT
// MOVED IS THE REASON IT MUST NOT BE COPIED. Seeding rig's real backlog through
// the CLI is the last step of the MVP, and the only thing that could read
// BACKLOG.md was a test helper - so anything written in cmd/ to drive that
// seeding would have been a SECOND parser of one document. Two parsers of one
// file that drift is not a hypothetical here: this package's own history is a
// row regex, two cross-checks and a grep that all required a pipe this document
// does not always have, agreeing on a number that was wrong.
//
// It is promoted UNCHANGED. The answers it gives about rig's backlog today -
// 45 rows, 9 closed, 36 open, 4 claiming a terminal state unstruck, 1 malformed
// - are pinned in backlogpin_test.go, landed in its own commit BEFORE this file
// existed, against both a fixed fixture and the live document.

package record

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// backlogRowAnywhere is the SECOND instrument, and it exists to disagree.
//
// It is a multiline match over the whole file rather than a line scan, so it
// shares no code path with the scanner - not the buffer bound, not the loop,
// not the split. The guard asserts the two agree as SETS.
//
// ⛔ IT EARNED ITS KEEP ON ITS FIRST RUN. It found B21, which FOUR separate
// hand-counts had missed - the old regex, two python cross-checks, and the
// `grep -cE '^\| B[0-9]+ \|'` that produced the 44 reported to the lead and
// into READINESS.txt. Every one of them required a pipe after the id, and B21's
// row does not have one. THE TRUE ROW COUNT IS 45, NOT 44.
//
// ⛔ IT NOW MATCHES THE WIDER ID-SHAPED TOKEN RATHER THAN THE LEGAL ID, AND
// THAT IS THE HALF THAT WAS MISSING. It used to end at `(?:\b|[^a-z0-9])`,
// which is a boundary `B60-2` satisfies after `B60` - so it read `B60`, the
// scanner read `B60`, and the two instruments agreed on a truncation. An id
// this parser cannot read has to reach BOTH instruments in the shape the
// document wrote it, or the disagreement they exist to produce cannot happen.
// Measured 2026-09-17 over the live document: same 68 ids, zero shapes only
// one of them finds, so the widening costs nothing today and is the only
// reason `B60-2` is reportable at all.
var backlogRowAnywhere = regexp.MustCompile(`(?m)^\|[^|\n]*?\b(B\d+[A-Za-z0-9._-]*)`)

// BacklogItem is one row of a backlog table, read rather than interpreted.
type BacklogItem struct {
	ID    string
	Title string

	// Done is the DOCUMENT'S OWN closure mark: the title struck through.
	//
	// ⛔ THIS IS EVIDENCE RATHER THAN INFERENCE. B7's State cell says it in as
	// many words - "done, struck not deleted" - so the file states its own
	// convention. Striking a title is a deliberate act somebody performed;
	// scanning the row's prose for the word "done" is a lottery, and the
	// pattern this replaced lost B44 to a comma, B15 to the word CLOSED and
	// B26 to a full stop.
	Done bool

	// ClaimsDone is a row whose STATE cell leads with a terminal disposition
	// while its title is NOT struck.
	//
	// ⛔ COUNTED AND REPORTED, NEVER FOLDED INTO Done. cells[2] and cells[5]
	// are different axes: the item cell carries whether the WORK is closed,
	// the state cell carries how the FINDING was disposed of. B9 is struck and
	// done while its state says "argued", and that is not a contradiction.
	// Whether a row that claims done in prose without being struck is closed
	// is a question about the document, and this parser does not own the
	// document - so it hands the fact up rather than deciding.
	ClaimsDone bool

	// Struck records WHICH of Done's two clauses closed this row: the
	// strikethrough, or a terminal lead in the item cell.
	//
	// ⛔ IT EXISTS BECAUSE THE LABEL LIED. A report printed "%d closed (title
	// struck)" over a set that is not all struck - 8 of 9 - so two seats
	// measured honestly and got different numbers. The label asserted a
	// one-clause test the code does not run, which is the same family as a
	// bound that cannot notice itself going stale: a caption is a claim, and an
	// unchecked one goes wrong exactly where nobody looks.
	Struck bool

	// Malformed is a row whose markdown is irregular enough that the id cell
	// and the item cell ran together. Named in the answer so the document's
	// owner can fix it; never a reason to drop the row.
	//
	// ⛔ SECTION 39'S MIGRATION RULING IS IMPORT EVERYTHING AND FLAG WHAT IS
	// IRREGULAR. A row a parser quietly tidies is a row nobody ever fixes.
	//
	// ⛔ IT NO LONGER MEANS "DO NOT TRUST THIS ROW'S TITLE". It did: the title
	// was read from cells[2], which on a shifted row is the EVIDENCE cell, so
	// B21's title in rig's own backlog was the word "evidence". That is fixed -
	// the title is taken from the reconstructed cell - and the fix is its own
	// commit rather than smuggled into the promotion, because it CHANGES AN
	// ANSWER and the pin existed precisely to make that visible.
	//
	// The flag still means the DOCUMENT is irregular and somebody should mend
	// the row. rig imports it either way: section 39's migration ruling is
	// import everything and flag what is irregular.
	Malformed bool

	// RuledClosed is the THIRD closure convention this document uses, and it is
	// reported rather than folded into Done. A row closed by a RULING carries a
	// tick in its ID cell and a terminal word leading its state cell, with its
	// title unstruck and its item cell open - because no work was finished, a
	// decision was taken. B55 and B56 are both this shape. Collapsing it into
	// Done would assert that a ruling completed the work; dropping it is what
	// has seeded B15, B24, B25 and B44 as OPEN since the parser existed.
	RuledClosed bool

	// Disposition is WHICH terminal word closed the row - DONE, CLOSED,
	// REJECTED or RETRACTED - and it is empty on a row that is not terminal.
	//
	// ⛔ THE PARSE HAS ALWAYS DISTINGUISHED THE FOUR AND HAS ALWAYS THROWN THE
	// WORD AWAY. `terminalDispositions` is a set lookup whose answer is a
	// bool, so a consumer could learn that a row is closed and never how. The
	// store's best available statement was `closed`, which is true of all four
	// - and coarsening is honest where inverting is not, but B19 is RETRACTED
	// as falsified and a retraction is not a completion. The word is the fact
	// that matters for trust, so it is carried rather than re-derived by
	// whoever needs it next.
	//
	// ⛔ AND IT IS NOT AN EXPORT OF SOMETHING ALREADY COMPUTED. On a STRUCK row
	// the terminal word is a SECOND bold run - B19's item cell is
	// `~~**title**~~ **RETRACTED 2026-09-16 ...**` - and `boldLead` returns the
	// FIRST run, which is the title. `Done` was reached through
	// `HasPrefix(item, "~~")` and never through the word at all, so recovering
	// it meant reading a part of the cell nothing had read.
	//
	// ⛔ IT CANNOT MOVE ANY BOOLEAN ABOVE, BY CONSTRUCTION. It is computed FROM
	// them rather than beside them: empty unless Done or ClaimsDone is already
	// true, and the search is bounded to the run that clause already decided
	// on. An empty Disposition on a closed row means the DOCUMENT named no
	// word there - reported, never guessed.
	Disposition string

	// PartOf is the id this row is a sub-task of, where the DOCUMENT states it
	// structurally. Section 39:1027 - "A sub-task IS a work-item, `part-of` its
	// parent."
	//
	// ⛔ IT COMES FROM THE SUB-LETTER AND FROM NOTHING ELSE. Heading enclosure
	// was the other candidate, it is what the specification recommended, and
	// running it over the live document produced TWENTY-FOUR edges the
	// document never states. The measurement and the reasoning are on `partOf`
	// below; the short form is that where a row SITS is a fact about the file
	// and `part-of` is a claim about the work.
	//
	// ⛔ DERIVED HERE, WIRED TO NOTHING YET. No edge is emitted from it: that is
	// `cmd/rigseed`'s act and this file does not own it. It is carried so the
	// seeder needs no second derivation of a relation the document states.
	PartOf string

	// State is the STATE CELL'S OWN PROSE, whole, and it is the only place a
	// reader learns what the row is actually about.
	//
	// ⛔ IT WAS PARSED AND THROWN AWAY. `Done`, `ClaimsDone` and `Disposition`
	// are all read OUT of this cell and the cell itself was dropped, so a
	// record carried three booleans derived from thousands of characters that
	// reached no store. Measured on the live production store 2026-09-17: B62's
	// `title`, `description_short` and `body` are the SAME 161-character
	// string, which is why clicking a row in the window shows nothing new.
	//
	// ⛔ BORIS, 2026-09-17, ON EXACTLY THAT: "clicking them doesn't show the
	// full description. The information must be stored in a way that helps the
	// human reviewers." This field is what there is to store.
	State string

	// Owner is who the row belongs to, from the table's `Seat` or `Adopter`
	// column. Section 39 declares `owner` and nothing has ever written it.
	Owner string

	// Section is the nearest enclosing heading, as a slug: the document's own
	// grouping of its rows. The requirement it answers is `plan/11`, Boris's
	// fourth 2026-09-17 ruling, and the two-bucket measurement over rig's own
	// backlog is in `plan/39`.
	//
	// ⛔ IT IS NOT `PartOf` AND MUST NEVER BE PROMOTED TO ONE. `Under`'s
	// comment below carries the measurement: heading enclosure as a `part-of`
	// edge produced twenty-four edges this document never states. Where a row
	// SITS is a fact about the file; `part-of` is a claim about the work.
	//
	// Same field, same rule, as `Unimported.Section` - both read
	// `sectionStack`, which is that type's reason for existing.
	Section string

	// SectionTitle is the same heading's TEXT, for a reader rather than for a
	// key. `FriendlyTag` shortens it; the raw slug cannot be shortened without
	// guessing.
	SectionTitle string
}

// cells splits a markdown table row and trims every cell. The table is
// | # | Item | Evidence | Adopter | State |, so cells[1] is the id, cells[2]
// the item and cells[5] the state.
//
// ⛔ A PIPE INSIDE A BACKTICK CODE SPAN IS NOT A CELL BOUNDARY, AND THIS USED
// TO BE `strings.Split(line, "|")`, WHICH SAID IT WAS. A row whose item cell
// quotes a regex or a shell alternation - `a|b` - split into one cell too many,
// and every cell after it shifted LEFT. The state cell then read as the adopter
// cell, so the row's terminal disposition was decided by a column that never
// held one. B8 in rig's own backlog is the live instance: its ClaimsDone was
// being read from "**PRESENCE** proposed, **the lead** holds the shape".
//
// ⛔ IT MOVED NO PINNED FIGURE THE DAY IT WAS FIXED, AND THAT IS WHY IT IS
// WORTH SAYING OUT LOUD. Neither cell's bold lead happened to be a terminal
// word, so the wrong answer and the right answer agreed. The defect was
// invisible to every count in this package and would have surfaced the first
// time somebody wrote a backticked pipe into a row that closes.
func cells(line string) []string {
	var out []string
	var cur strings.Builder
	inCode := false
	for _, r := range line {
		switch {
		case r == '`':
			inCode = !inCode
			cur.WriteRune(r)
		case r == '|' && !inCode:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, strings.TrimSpace(cur.String()))
}

// titleOf takes the item cell's title: its FIRST BOLDED RUN, which is how every
// row in this table writes one, struck or not.
//
// ⛔ IT USED TO TRIM THE MARKERS OFF THE FRONT AND CUT AT THE NEXT ONE, WHICH
// ASSUMED THE CELL STARTS WITH THEM. B21 in rig's own backlog starts with a
// tick, so trimming matched nothing and the cut landed at the tick: its title
// read "✅". The rows are handwritten and a decoration before the bold is
// ordinary, so the title is located rather than trimmed to.
//
// The fallback matters for the same reason: a cell with no bold at all still
// has to yield something, and the whole trimmed cell is the honest answer.
func titleOf(cell string) string {
	if b := strings.TrimSpace(boldLead(cell)); b != "" {
		return b
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(cell, "~~"), "**"))
}

// boldRuns returns every bolded run in a cell, in order. A run left open at
// the end of the cell is still a run: B8 in the fixture never closes its bold
// before the pipe and its title has to come from somewhere.
func boldRuns(cell string) []string {
	var out []string
	for {
		i := strings.Index(cell, "**")
		if i < 0 {
			return out
		}
		rest := cell[i+2:]
		j := strings.Index(rest, "**")
		if j < 0 {
			return append(out, rest)
		}
		out = append(out, rest[:j])
		cell = rest[j+2:]
	}
}

// boldLead returns a cell's first bolded run, which is how every row in this
// table states its disposition.
func boldLead(cell string) string {
	if r := boldRuns(cell); len(r) > 0 {
		return r[0]
	}
	return ""
}

// afterStrike returns the part of an item cell the strikethrough does NOT
// cover, which on a closed row is where the document puts the terminal word.
//
// ⛔ A CELL THAT OPENS A STRIKE AND NEVER CLOSES IT RETURNS NOTHING, not the
// remainder. Everything after an unclosed `~~` is inside the strike as far as
// a reader is concerned, so treating it as commentary after the title would
// read a struck title's own words as the row's disposition.
func afterStrike(cell string) string {
	i := strings.Index(cell, "~~")
	if i < 0 {
		return cell
	}
	rest := cell[i+2:]
	j := strings.Index(rest, "~~")
	if j < 0 {
		return ""
	}
	return rest[j+2:]
}

// closingWord returns the first terminal disposition leading a bold run in s,
// and "" when no run leads with one.
func closingWord(s string) string {
	for _, r := range boldRuns(s) {
		if w := firstWord(r); terminalDispositions[w] {
			return w
		}
	}
	return ""
}

// terminalDispositions are the bold leads that mean the work is over.
var terminalDispositions = map[string]bool{
	"DONE": true, "CLOSED": true, "REJECTED": true, "RETRACTED": true,
}

// ownerOf reads a NAME out of an owner cell, or nothing.
//
// ⛔ THE CELL IS PROSE AS OFTEN AS IT IS A NAME, AND STORING IT RAW MAKES THE
// FIELD USELESS FOR THE THING IT WAS ASKED FOR. Measured 2026-09-17 over rig's
// own 106 work items: 42 read `team-lead`, and the rest include a 400-character
// paragraph about a seat name that decayed. Boris asked for rows "grouped or at
// least tagged so the user can see what relates to what", and an owner field
// holding a paragraph groups exactly one row with itself.
//
// ⛔ AND AN OWNER IT CANNOT READ IS LEFT ABSENT RATHER THAN GUESSED. A wrong
// owner sends a reader to the wrong seat and looks authoritative doing it; an
// absent one is a fact about the document that record.query can find. The cut
// is a LENGTH: a name is short, and anything long is the cell explaining itself.
func ownerOf(cell string) string {
	name := strings.TrimSpace(cell)
	// A bold run leads most of these cells and is where the document puts the
	// name when the cell goes on to explain itself.
	if b := boldLead(name); b != "" {
		name = b
	}
	name = strings.TrimSpace(strings.Trim(name, decoration+"`"))

	// The name ends where the cell starts qualifying it.
	//
	// ⛔ A BARE HYPHEN IS NOT A SEPARATOR HERE AND CUTTING ON ONE IS A BUG THIS
	// FUNCTION SHIPPED AND HAD MEASURED BACK AT IT: `team-lead` came out as
	// `team` and `backend-record` as `backend`, which is 52 of 106 rows filed
	// under seats that do not exist. Every seat name in this project is
	// hyphenated. Only a SPACED dash separates a name from its explanation.
	if i := strings.IndexAny(name, ",(:;."); i > 0 {
		name = strings.TrimSpace(name[:i])
	}
	for _, dash := range []string{" - ", " \u2013 ", " \u2014 "} {
		if i := strings.Index(name, dash); i > 0 {
			name = strings.TrimSpace(name[:i])
		}
	}
	name = strings.TrimSpace(strings.Trim(name, decoration+"`"))

	// ⛔ A PLACEHOLDER IS NOT AN OWNER. The document writes `-` for a row
	// nobody has taken, and storing that would make "unowned" look like a seat
	// called "-".
	switch {
	case name == "" || name == "-":
		return ""
	case utf8.RuneCountInString(name) > ownerNameMax:
		// Still a sentence: the cell is explaining rather than naming.
		return ""
	}
	return name
}

// ownerNameMax is the longest thing this parser will call a name.
//
// ⛔ A LENGTH AND NOT A VOCABULARY. A closed set of seat names would need
// maintaining in this file and would silently drop the day a seat is added.
// Section 39 closes the LINK-TYPE set deliberately; nobody has closed the set
// of owners, so a length is the honest test.
const ownerNameMax = 24

// firstWord returns a cell's first word, upper-cased and stripped of the
// punctuation these rows attach to it - "DONE," and "DONE." both mean DONE.
func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " ,.;:"); i > 0 {
		s = s[:i]
	}
	return strings.ToUpper(s)
}

// backlogID matches a backlog id ANYWHERE IN A FIRST CELL, decoration and all.
//
// ⛔ IT USED TO BE ANCHORED AT THE CELL'S START AND TO END AT A WORD BOUNDARY,
// AND BOTH HALVES WERE WRONG ABOUT THIS DOCUMENT.
//
//   - `^\|\s*(B\d+)` cannot see `| ⛔ **B47** |`. From B46g onward every row is
//     filed with a decoration before its id.
//   - `(B\d+)\b` cannot see `| B46a |`. There is NO word boundary between `6`
//     and `a` - both are word characters - so a sub-lettered id fails on a row
//     carrying no decoration at all.
//
// THE LEGAL SHAPES ARE `B<digits>` AND `B<digits><one lowercase letter>`, and
// that is evidence rather than preference: B46a, B46b, B46c, B46e, B46f and
// B46g are rows, and no ROW uses any other shape.
//
// ⛔ THE SENTENCE THAT STOOD HERE SAID "B46d DOES NOT [EXIST]" AND B46d EXISTS.
// It is a `###` heading at `BACKLOG.md:176`, "B46d - THE CYCLE IS CONSTRUCTED,
// AND THE RELAY SAID OTHERWISE". The claim was true of this table's ROWS and
// was offered as evidence about the DOCUMENT, which is the wider thing it does
// not cover - and the pattern's design was never what was wrong, because
// `backlogID` matched `B46d` perfectly well the whole time. **The COMMENT was
// repaired rather than the code, because there was no code defect to repair.**
// What was missing is that nothing could REPORT a heading-borne id, and
// `ParseBacklogDocument` now does.
//
// ⛔ AND THE OTHER SENTENCE THAT STOOD HERE WAS FALSE IN THE OTHER DIRECTION:
// "the next person who invents `B60-2` gets a red from the set guard below
// rather than a silent drop". ⛔ **MEASURED 2026-09-17: `B60-2` READ AS `B60`,
// AND SO DID THE SET GUARD.** Both instruments truncated at the same place, so
// they agreed, and agreement is what they do wrong. B60 is now an OCCUPIED row,
// so the truncated id hit the duplicate rule and the invented row vanished
// without a trace; had it come FIRST it would have taken B60's id and title
// instead, which is a silent mis-id and worse than a drop.
//
// **The CODE was repaired for that one**, because the comment was describing
// the behaviour anyone would want. An id-shaped token is now located with
// `backlogIDish`, which is deliberately WIDER than any legal id, and tested
// against `backlogIDLegal`. A token that fails is never read as a prefix: it is
// reported as an `irregular-id`, so it appears in what the parse says it did
// not take and a pin over that set goes red.
//
// ⛔ REPORTED AND NOT REFUSED, WHICH IS THIS FILE'S OWN RULING APPLIED TO
// ITSELF. Section 39's migration rule is import everything and flag what is
// irregular; killing the parse on one malformed id cell would contradict the
// same sentence `Malformed` is justified by.
var (
	backlogIDish   = regexp.MustCompile(`\bB\d+[A-Za-z0-9._-]*`)
	backlogIDLegal = regexp.MustCompile(`^B\d+[a-z]?$`)
)

// mdHeading splits a markdown heading into its level and its text.
var mdHeading = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

// tableRule matches one cell of a markdown header separator - `---`, `:--`,
// `--:`. A separator is a row of a table and carries no id, so a detector for
// "a row in a work-item table with no id" that does not exclude it reports
// three false positives against rig's own backlog.
var tableRule = regexp.MustCompile(`^:?-{2,}:?$`)

// decoration is what a first cell may carry around its id without meaning
// anything: the two status glyphs this document uses, bold and strike markers,
// and space. Anything LEFT once these are stripped is absorbed content, which
// is how a shifted row is told from a decorated one.
const decoration = "⛔✅*~ \t"

// backlogTables are the header shapes that introduce work items, keyed by the
// role each column plays.
//
// ⛔ THIS EXISTS BECAUSE `BACKLOG.md` HAS THREE TABLES WITH BACKLOG IDS IN
// THEIR FIRST CELL AND THE PARSER COULD ONLY READ ONE. Measured 2026-09-17:
//
//	| # | Work | Seat | State |                      4 cols   19 rows, ALL INVISIBLE
//	| # | Item | Evidence | Adopter | State |        5 cols   45 rows, the ones read
//	| Row | Adopter, as a ROLE |                     2 cols    5 rows, NOT work items
//
// The nineteen were not scattered and they were not two defects. They were the
// entire contents of ONE TABLE added with a different header, and the two id
// conventions inside it are conventions rather than causes. Finding that took
// asking which TABLES carry ids, not running a second instrument over first
// cells - two instruments that both read first cells agree, and agreeing is
// what they do wrong.
//
// ⛔ AND THE COLUMN ROLES MOVE BETWEEN THE TWO SHAPES, WHICH IS WHY THIS IS A
// TABLE OF ROLES AND NOT A LOOSER REGEX. State is the 5th cell in one and the
// 4th in the other. A parser that loosened the id pattern alone would have made
// nineteen rows appear and read every one of their terminal states from the
// trailing empty string - `len(c) < 6` passes on a 4-column row, because six is
// six - reporting `ClaimsDone: false` for all of them, silently and for ever.
// B55 and B56 are both in that table and both are closed by a ruling.
var backlogTables = []struct {
	header []string
	id     int
	item   int
	state  int

	// owner is the column naming WHO the row belongs to, and it was read by
	// nothing until 2026-09-17.
	//
	// ⛔ BORIS RULED HUMAN-FRIENDLY FIELDS OBLIGATORY (plan/11) AND `owner` IS
	// ONE SECTION 39 ALREADY DECLARES. Both of these tables have carried the
	// answer in a column of their own since they were written - `Adopter` in
	// one, `Seat` in the other - and the parser walked past it, so every record
	// in the store says nothing about whose work it is. That is the cheapest
	// half of "grouped or at least tagged so the user can see what relates to
	// what": the document already grouped them.
	owner int
}{
	{header: []string{"#", "Item", "Evidence", "Adopter", "State"}, id: 1, item: 2, state: 5, owner: 4},
	{header: []string{"#", "Work", "Seat", "State"}, id: 1, item: 2, state: 4, owner: 3},
}

// tableFor returns the column roles for a header row, and whether it is a work
// item table at all. A header it does not recognise is not an error: the
// document holds tables about other things and they are allowed to mention a
// backlog id.
func tableFor(line string) (id, item, state, owner int, ok bool) {
	c := cells(line)
	for _, t := range backlogTables {
		if len(c) != len(t.header)+2 {
			continue
		}
		match := true
		for i, want := range t.header {
			if !strings.EqualFold(c[i+1], want) {
				match = false
				break
			}
		}
		if match {
			return t.id, t.item, t.state, t.owner, true
		}
	}
	return 0, 0, 0, 0, false
}

// UnimportedKind names WHY something the document addresses is in no record.
type UnimportedKind string

const (
	// UnimportedHeading is an id the document carries in a HEADING rather than
	// in a row. B46 - the MVP acceptance test - is one, and so is B46d.
	UnimportedHeading UnimportedKind = "heading"

	// UnimportedRowWithoutID is a row INSIDE a recognised work-item table whose
	// id cell names no id. The eleven rows of the critical path are these, and
	// the document calls them Ordered.
	UnimportedRowWithoutID UnimportedKind = "row-without-id"

	// UnimportedIrregularID is a first cell naming something id-SHAPED that is
	// not a legal id - `B60-2`. Never read as the prefix it starts with.
	UnimportedIrregularID UnimportedKind = "irregular-id"

	// UnimportedOtherTable is an id in the first cell of a table that is not a
	// work-item table. The parser is right not to import it; what was wrong is
	// that nothing could tell that decision apart from never having seen it.
	UnimportedOtherTable UnimportedKind = "other-table"
)

// Unimported is one thing the document addresses that no record carries.
//
// ⛔ IT EXISTS BECAUSE AN ABSENCE NO INSTRUMENT CAN REPORT SURVIVES EVERY
// REVIEW. `ParseBacklog` answered 68 with no error over a document that
// addresses 70 ids and holds eleven rows it calls Ordered, and BOTH of this
// package's instruments keyed on the same id shape - so the whole class was
// invisible by construction rather than by accident. Widening the aperture
// first would have been the wrong repair in the wrong order: a parser that
// SOMETIMES catches a heading is undetectable, where one that never does is at
// least consistent. So the blindness goes first, and what to do about the
// aperture is then a decision somebody can take on evidence.
type Unimported struct {
	Kind UnimportedKind

	// ID is the id the document names, where it names one. Empty for a row
	// whose id cell holds no id, and empty for an irregular shape - which is
	// not an id and must not be filed as one.
	ID string

	// Label is what the document wrote where the id goes: the first cell
	// verbatim, the id-shaped token verbatim, or the heading's own text.
	//
	// ⛔ ON THE ELEVEN ORDERED ROWS IT IS THE RANK - `0`, `6a`, `9` - AND THAT
	// IS THE ONLY PLACE THE DOCUMENT'S DECLARED ORDER SURVIVES. Section 39
	// offers `blocks`, which asserts a dependency nobody stated, and
	// `priority`, which has three buckets for eleven ranks. A total order has
	// no faithful home in the model, so the rank is REPORTED and no mapping is
	// invented. That is a ruling and this parser does not own it.
	Label string

	// Line is the 1-based line the thing sits on, so a report resolves to a
	// place in the document rather than to the document.
	Line int

	// Cells is the row's content cells, verbatim and in the document's order,
	// for anything this grain found in a TABLE ROW. The outer empty strings a
	// markdown row's leading and trailing pipes produce are not cells and are
	// not here, so Cells[0] is the first cell a reader sees.
	//
	// ⛔ IT EXISTS BECAUSE THE GRAIN KEPT ONE CELL AND THREW THE ROW AWAY.
	// Measured over the eleven ordered rows of rig's own critical path on
	// 2026-09-17: 34 bytes kept - Label, the rank - and 4,656 bytes dropped.
	// The live consequence reached the production store: all eleven records
	// were titled `0`..`9` and `6a`, body and all, because the rank was the
	// only thing that survived the parse.
	//
	// ⛔ AND IT IS VERBATIM, WHICH IS THE POINT RATHER THAN A SHORTCUT. The
	// alternative was to project every cell into a named field, and that needs
	// a ruling about what a rank-keyed row MEANS that Label's own comment says
	// this parser does not own. Cells asserts nothing: it is the evidence, and
	// a consumer that wants a column can name it without this file guessing
	// which columns the next table will have.
	//
	// Empty for a heading, which has no cells. `Body` is the heading's
	// equivalent and the two never both apply.
	Cells []string

	// Title is the title of the work, with the document's decoration removed:
	// a heading's own text less its id, or a row's item cell through the same
	// titleOf every imported row's title goes through.
	//
	// ⛔ IT IS A SECOND FIELD RATHER THAN A CLEANED-UP Label, AND THE REASON
	// IS THAT Label's CONTRACT IS TO BE VERBATIM. A report that says "the
	// document wrote this here" has to quote what the document wrote. The
	// defect was real and reached a live store: B46 was seeded with the title
	// `⛔ B46 - THE MVP ACCEPTANCE TEST, AND IT EXISTED IN NO DOCUMENT AT
	// ALL`, decoration and id included, where every row grain title goes
	// through titleOf. Two grains rendering one document two ways is the
	// thing this file exists to stop.
	//
	// ⛔ AND IT IS DERIVED HERE RATHER THAN LEFT TO THE CONSUMER FOR EXACTLY
	// THAT REASON. Handing over Cells alone would leave a seeder to locate the
	// bold run itself - a SECOND title derivation over one document, which is
	// the B46 failure verbatim one kind over.
	//
	// Empty where this parser does not know which text is the title: a row of
	// a table whose shape it does not recognise has no item column to read,
	// and an irregular token in a heading is not an id to strip.
	Title string

	// Struck is the document's own strikethrough mark: on a heading, its text
	// struck through; on a row, a struck item cell.
	//
	// ⛔ IT IS THE DOCUMENT'S OWN CLOSURE CONVENTION READ AT A SECOND GRAIN,
	// NOT A NEW ONE. B7's state cell says it in as many words - "done, struck
	// not deleted" - and BacklogItem.Struck reads exactly this mark on a row.
	// Applying a stated convention to a heading is reading the document; it is
	// the alternative, inferring a parent's state from its children's, that
	// would be a seat judging.
	//
	// ⛔ AND NOTHING IN rig's OWN BACKLOG IS STRUCK AT THIS GRAIN TODAY -
	// no heading, and none of the eleven ordered rows - so the true branch is
	// covered by a synthetic test and by nothing in the live document. Said
	// out loud because a pin that only ever sees one side of a predicate is
	// half a pin.
	Struck bool

	// Under is the enclosing id-bearing heading, where there is one.
	//
	// ⛔ IT IS DELIBERATELY NOT CALLED `PartOf`, AND THE NAME IS THE FINDING.
	// Position under a heading is a fact about the FILE; `part-of` is a claim
	// about the WORK, and in this document they come apart - see the note on
	// `partOf` below, where heading enclosure produced twenty-four edges
	// nobody stated. Under says where the thing sits and asserts nothing else.
	Under string

	// Section is the nearest enclosing heading of ANY level, id-bearing or
	// not, as a slug.
	//
	// ⛔ IT IS NOT A WIDER `Under` AND THE TWO ARE DIFFERENT FACTS. Under is
	// the nearest heading that CARRIES AN ID, and its doc comment above says
	// why that narrowness is load-bearing. Section says WHERE THE THING SITS
	// in the document and names no id at all. Both are true of the same
	// Unimported and neither can be derived from the other: `## The critical
	// path to the gate` has no id, so the eleven ordered rows beneath it have
	// an empty Under and a Section of `the-critical-path-to-the-gate`.
	//
	// ⛔ IT EXISTS BECAUSE NOTHING SAID WHICH TABLE THE ELEVEN CAME FROM, AND
	// AN ID MINTED FROM THE BARE RANK WOULD COLLIDE SILENTLY. Label on those
	// rows is `0`, `6a`, `9` - unique in this document today, and unique only
	// by luck. The day a second unnumbered table appears, a seeder keyed on
	// the rank alone overwrites eleven records and nothing reports it. Section
	// is the qualifier that makes the key safe, which is why it is a SLUG: it
	// becomes part of a record id, and a raw heading carries the document's
	// decoration, its markdown emphasis and its punctuation.
	//
	// ⛔ IT IS EXCLUSIVE OF THE THING IT DESCRIBES, EXACTLY AS Under IS. A
	// heading's Section is its PARENT heading, never itself - `heading` reads
	// the stack before it pushes its own frame, and a field that meant one
	// thing on a row and another on a heading would be worse than either
	// answer.
	//
	// ⛔ AND IT IS POPULATED ON ALL SEVEN KINDS, INCLUDING THE THREE
	// `decisions.go` REPORTS. It was empty on those three for one commit, and
	// that is recorded here rather than forgotten: Unimported is SHARED
	// between two grains, so a field one grain adds is a field the other grain
	// silently lacks, and a field populated on four kinds and empty on three
	// is indistinguishable from one that was lost. Both files now take the
	// enclosing heading from `sectionStack`, so there is one rule and not two.
	Section string

	// SectionTitle is the same heading's TEXT, for a reader rather than for a
	// key, and it is populated wherever `Section` is. `FriendlyTag` turns it
	// into something short enough to read in a filter.
	SectionTitle string

	// Body is the PROSE beneath a heading-borne id, down to the next heading
	// of any level, with the runs of blank lines a removed table leaves
	// collapsed to one. Empty for everything that is not a heading, and for a
	// heading with no prose under it.
	//
	// ⛔ IT EXISTS BECAUSE A HEADING'S RECORD ASSERTED ITS OWN TITLE AS ITS
	// BODY. `cmd/rigseed` sets `body: title` and says so; that is a consumer
	// papering over an absence with a stand-in, which is the same shape as
	// `title = u.ID`, and under `## ⛔ B46` the prose it stands in for IS the
	// statement of the MVP acceptance test.
	//
	// ⛔ AND IT IS PROSE RATHER THAN EVERY LINE, WHICH IS WHERE IT PARTS
	// COMPANY WITH DecisionEntry.Body - measured, not preferred. A decisions
	// heading encloses paragraphs; a backlog heading encloses TABLES. Over
	// rig's own document on 2026-09-17, `## ⛔ B46` holds 65,187 bytes to the
	// next heading and 695 of them are prose: the other 64,492 are work-item
	// rows THIS SAME PARSE already returns, as Items or as Unimported. Copying
	// them in would duplicate the row grain inside the heading grain, grow
	// without bound as the table grows, and bury the 695 bytes that are
	// carried nowhere else.
	//
	// ⛔ AND THE SPLIT IS NOT A NEW RULE. `read` already partitions every line
	// into table - a leading `|` - and prose, and decides the parse on it.
	// Body reuses that partition rather than inventing a second reading of
	// what a table is.
	Body string
}

// BacklogParse is one pass over a backlog document: what it imported, and
// everything else the document addresses.
type BacklogParse struct {
	// Items is the row grain, and it is exactly what ParseBacklog returns.
	Items []BacklogItem

	// Unimported is in DOCUMENT order, because on eleven of these that order
	// is the only thing the document states about them.
	Unimported []Unimported
}

// headingFrame is one level of the heading stack. carry is the nearest
// id-bearing heading at or above this level, which is what a row underneath is
// part-of. section is this frame's OWN heading, slugged, and it is tracked for
// every heading rather than only for the id-bearing ones.
//
// ⛔ THE TWO FIELDS CANNOT BE ONE. carry SKIPS a heading that states no id, so
// it survives the pop as the grandparent's value; section never skips, because
// its whole job is to name the heading a thing literally sits under. `## The
// critical path to the gate` is the heading where they differ and it is the
// heading the eleven ordered rows are in.
type headingFrame struct {
	level   int
	carry   string
	section string

	// sectionTitle is the heading's own TEXT, kept BESIDE the slug because a
	// slug cannot be shortened well and the text can - see `FriendlyTag`.
	//
	// ⛔ IT IS NOT A SECOND SLUG RULE, WHICH IS THE DRIFT THIS TYPE EXISTS TO
	// STOP. `section` stays the key component: ranked-row ids are built from
	// it and shortening it would RENAME eleven live records. Both are taken
	// from the same text in the same call. Bar and reasoning: `plan/39`.
	sectionTitle string
}

// sectionStack is the heading stack, and BOTH grains in this package keep one.
//
// ⛔ IT IS A TYPE RATHER THAN A POP LOOP IN EACH FILE BECAUSE THE RULE IS ONE
// RULE. `Section` means the same thing on an Unimported the backlog grain
// reports and on one the decisions grain reports, and two files each running
// their own three-line pop loop is how the two meanings drift apart - which is
// the drift this package spends its time correcting.
type sectionStack []headingFrame

// popTo drops every frame at or below level, so the top is then the frame
// ENCLOSING a heading about to be entered at that level.
//
// ⛔ CALLED BEFORE THE HEADING'S OWN FRAME IS PUSHED, WHICH IS WHAT MAKES
// carry AND section EXCLUSIVE OF THE HEADING ITSELF. A heading sits under its
// PARENT heading; it does not enclose itself.
func (st *sectionStack) popTo(level int) {
	for len(*st) > 0 && (*st)[len(*st)-1].level >= level {
		*st = (*st)[:len(*st)-1]
	}
}

// carry is the nearest id-bearing heading at or above the top of the stack.
func (st sectionStack) carry() string {
	if n := len(st); n > 0 {
		return st[n-1].carry
	}
	return ""
}

// section is the nearest heading of ANY level, slugged. Empty only where the
// document has not opened a heading yet.
func (st sectionStack) section() string {
	if n := len(st); n > 0 {
		return st[n-1].section
	}
	return ""
}

// sectionTitle is the same heading's TEXT, for a reader rather than for a key.
func (st sectionStack) sectionTitle() string {
	if n := len(st); n > 0 {
		return st[n-1].sectionTitle
	}
	return ""
}

// backlogScan is one pass's state. It is a type rather than a pile of locals
// because the heading stack, the table roles and the accounting maps all have
// to survive a line, and a single function holding all of them was already at
// the edge of being unreadable before any of this was added.
type backlogScan struct {
	items      []BacklogItem
	unimported []Unimported

	// seen is the ids that became records; accounted is every id-shaped token
	// the parse can EXPLAIN, whether it became a record or not. The guard
	// reconciles the whole-file instrument against accounted, so "I decided
	// against this" and "I never saw this" stop being the same silence.
	seen      map[string]bool
	accounted map[string]bool

	heads sectionStack

	// bodyAt is the index in unimported of the heading whose prose is being
	// collected, and -1 when no heading is open for one. It is an INDEX rather
	// than a pointer because unimported is appended to while the body is being
	// read, and a pointer into a slice that grows is a pointer into the old
	// array.
	bodyAt  int
	bodyBuf []string

	idCol, itemCol, stateCol, ownerCol int
	inTable                            bool

	// fences decides whether a `#` line is a heading or a shell comment
	// inside a ```sh block. It is stepped by EVERY line, including the ones
	// read() returns on early: a machine that sees only some lines cannot
	// know which block it is in.
	fences fenceScan
	line   int
}

// ParseBacklog reads a backlog document and returns one item per work-item row.
//
// ⛔ IT IS THE NARROW ANSWER AND IT DOES NOT SAY WHAT IT DROPPED. Call
// ParseBacklogDocument for that. This form is kept because its answer over
// rig's own backlog is pinned in two places and seeds the live store, and
// moving a set and adding a capability in one change is how a repair becomes
// unreviewable.
func ParseBacklog(r io.Reader) ([]BacklogItem, error) {
	p, err := ParseBacklogDocument(r)
	return p.Items, err
}

// ParseBacklogDocument reads a backlog document and returns BOTH what became a
// record and everything else the document addresses.
//
// It refuses rather than guessing on two conditions, and both have cost this
// project a wrong number: a row inside a work-item table that is too short to
// be that table's shape, and a disagreement between the line scanner and a
// whole-file scan that shares no code path with it.
//
// Everything else it declines to import it REPORTS. A row in a table this
// parser does not recognise, a row inside one that carries no id, an id shape
// it cannot read, an id the document states in a heading: each is named in
// Unimported with its line, never dropped and never refused. Refusing would
// kill the parse on rows that are real; dropping is the defect this whole
// mechanism exists to end.
func ParseBacklogDocument(r io.Reader) (BacklogParse, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return BacklogParse{}, err
	}
	text := string(raw)

	s := &backlogScan{seen: map[string]bool{}, accounted: map[string]bool{}, bodyAt: -1}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		s.line++
		if err := s.read(sc.Text()); err != nil {
			return BacklogParse{}, err
		}
	}
	if err := sc.Err(); err != nil {
		return BacklogParse{}, err
	}
	// ⛔ THE LAST HEADING'S BODY IS CLOSED BY THE END OF THE DOCUMENT AND BY
	// NOTHING ELSE, so it has to be closed here. Closing it inside reconcile
	// would put it after reconcile's own filter, which rewrites the slice this
	// index points into.
	s.closeBody()
	if fence := s.fences.unclosed(); fence != "" {
		return BacklogParse{}, fmt.Errorf("a fenced block opened with %q is never closed, "+
			"so every heading after it was read as code; this parse declines to answer "+
			"rather than report the rest of the document as absent", fence)
	}
	if err := s.reconcile(text); err != nil {
		return BacklogParse{}, err
	}
	return BacklogParse{Items: s.items, Unimported: s.unimported}, nil
}

// read takes one line of the document.
func (s *backlogScan) read(line string) error {
	// ⛔ THE FENCE MACHINE IS STEPPED BEFORE ANY EARLY RETURN, AND ITS ANSWER
	// REACHES ONE DECISION ONLY: whether a `#` line is a heading. It
	// deliberately does NOT change the table partition - a `|` row inside a
	// fenced block is an example rather than a work item, and declining it
	// here would make reconcile accuse the parse of dropping an id. That is a
	// separate question with its own answer owed; it is measured at zero
	// occurrences today. DECISIONS.md, 2026-09-18.
	marker, code := s.fences.read(line)
	fenced := marker || code

	// ⛔ A BLANK LINE DOES NOT END A TABLE IN THIS DOCUMENT, AND STANDARD
	// MARKDOWN SAYS IT DOES. `BACKLOG.md` uses blank lines to GROUP rows
	// visually inside one table - five of them inside the five-column table
	// alone - and a parser that took the markdown rule literally kept the
	// header for thirteen rows and then lost it, reading the remaining
	// thirty-two as rows of no table at all. Measured: 32 rows where 64 was
	// the answer, and the pin caught it on the first run.
	//
	// ⛔ RE-MEASURED 2026-09-17 AND THE FIGURE HOLDS, WITH ONE REFINEMENT WORTH
	// MORE THAN THE FIGURE. The 32 are the CONTIGUOUS RUN B14-B45, not a
	// scatter, which is what a lost header looks like and a bad regex does
	// not. And the variant reading returns its wrong answer WITH NO ERROR,
	// because the lost rows land where nothing reads them.
	//
	// ⛔ AND THIS RULE IS ABOUT THIS DOCUMENT, NOT ABOUT MARKDOWN. It is
	// correct for `BACKLOG.md` and it is a guess anywhere else. The rule is
	// what a reader sees: the roles persist until a line of PROSE, which is
	// where a new heading or a new table begins.
	if strings.TrimSpace(line) == "" {
		// A blank line is kept for the body, where it is the only thing
		// separating one paragraph from the next.
		s.bodyLine(line)
		return nil
	}
	if !strings.HasPrefix(line, "|") {
		s.inTable = false
		// ⛔ THE SAME PARTITION THAT DECIDES THE PARSE DECIDES THE BODY, which
		// is why Body is not a second reading of what a table is: anything
		// reaching here is not a table row, and anything that is not a heading
		// either is the prose under the heading above it.
		if fenced || !s.heading(line) {
			s.bodyLine(line)
		}
		return nil
	}
	if i, it, st, ow, ok := tableFor(line); ok {
		s.idCol, s.itemCol, s.stateCol, s.ownerCol = i, it, st, ow
		s.inTable = true
		return nil
	}
	return s.row(line)
}

// heading tracks the heading stack and reports an id the document states in a
// heading rather than in a row.
//
// ⛔ THE HEADING GRAIN IS REPORTED AND IS NOT SEEDED, AND THE REASON IS
// OWNERSHIP RATHER THAN DOUBT. An exact whole-set equality between the store
// and the document IS computable for a heading-borne id, so by the projection
// criterion it EARNS a record. Writing one is `cmd/rigseed`'s act and the
// acceptance pin that would move is in another file; this parser derives the
// id, its text and its parent, and hands them over ready to use.
//
// It returns whether the line was a heading at all, because the caller has to
// know: a line that is neither a table row nor a heading is body prose.
func (s *backlogScan) heading(line string) bool {
	m := mdHeading.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	// ⛔ BEFORE ANYTHING IS APPENDED. A heading of any level ends the previous
	// heading's body, and closing after the append would file this heading's
	// body onto this heading.
	s.closeBody()
	level, text := len(m[1]), strings.TrimSpace(m[2])
	s.heads.popTo(level)
	parent := s.heads.carry()
	// ⛔ READ BEFORE THE PUSH, WHICH IS WHAT MAKES Section EXCLUSIVE OF THE
	// HEADING ITSELF - exactly as `parent` above is. A heading sits under its
	// PARENT heading; it does not enclose itself, and a field that meant one
	// thing on a row and another on a heading would be worse than either.
	parentSection := s.heads.section()
	parentSectionTitle := s.heads.sectionTitle()
	carry := parent
	if tok := backlogIDish.FindString(text); tok != "" {
		// An irregular id is labelled by its TOKEN wherever it is found, so
		// one Kind means one thing. A heading's own text is only the Label
		// when the heading carries a legal id and the whole line is the
		// thing being reported.
		u := Unimported{
			Kind: UnimportedIrregularID, Label: tok, Line: s.line,
			Under: parent, Section: parentSection,
			SectionTitle: parentSectionTitle,
		}
		if backlogIDLegal.MatchString(tok) {
			u = Unimported{
				Kind: UnimportedHeading, ID: tok, Label: text,
				Title: headingTitle(text, tok), Struck: struckThrough(text),
				Line: s.line, Under: parent, Section: parentSection,
				SectionTitle: parentSectionTitle,
			}
			carry = tok
		}
		s.accounted[tok] = true
		s.unimported = append(s.unimported, u)
		// Only a heading that carries a legal id becomes a record, so only
		// that one has a body anybody can read. An irregular token in a
		// heading is not an id and is reported as a shape, not as a thing.
		if u.Kind == UnimportedHeading {
			s.bodyAt = len(s.unimported) - 1
		}
	}
	s.heads = append(s.heads, headingFrame{
		level: level, carry: carry,
		section:      sectionSlug(text),
		sectionTitle: decisionTitle(text),
	})
	return true
}

// bodyLine takes one line of prose for the heading whose body is open, and
// drops it where none is.
func (s *backlogScan) bodyLine(line string) {
	if s.bodyAt < 0 {
		return
	}
	s.bodyBuf = append(s.bodyBuf, line)
}

// closeBody files the collected prose on the heading it belongs to and opens
// no new one. It is safe to call with nothing open, which is what makes the
// two call sites - every heading, and the end of the document - the only two
// places that have to know about it.
func (s *backlogScan) closeBody() {
	if s.bodyAt >= 0 && s.bodyAt < len(s.unimported) {
		s.unimported[s.bodyAt].Body = proseBody(s.bodyBuf)
	}
	s.bodyAt, s.bodyBuf = -1, nil
}

// proseBody joins a heading's prose lines, collapsing every run of blank lines
// to one.
//
// ⛔ THE COLLAPSE IS BECAUSE THE TABLES ARE GONE, NOT BECAUSE THE DOCUMENT IS
// UNTIDY. Removing a table from between two paragraphs leaves the blank line
// before it and the blank line after it adjacent, so a verbatim join would put
// a hole in the prose exactly where a reader would look for the table.
func proseBody(lines []string) string {
	out := make([]string, 0, len(lines))
	gap := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			gap = true
			continue
		}
		if gap && len(out) > 0 {
			out = append(out, "")
		}
		gap = false
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// sectionSlug is the key component for one heading, and it is THE SAME RULE
// the decisions grain keys its headings by rather than a second one.
//
// ⛔ NAMING IT HERE IS THE WHOLE POINT. `decisionSlug` and `decisionTitle` sit
// in decisions.go, in this package, so the backlog grain can call them without
// copying anything - and two slug rules over one repository's headings is the
// drift this package spends its time correcting. The names are
// decisions-flavoured for what is now a package-wide job; moving them to a
// neutral home is a rename in a file this grain does not own, so it is
// reported rather than taken.
func sectionSlug(text string) string {
	return decisionSlug(decisionTitle(text))
}

// row takes one table row.
func (s *backlogScan) row(line string) error {
	c := cells(line)
	if len(c) < 3 {
		return nil
	}

	// ⛔ THE TOKEN IS LOCATED WITH A PATTERN WIDER THAN ANY LEGAL ID, THEN
	// TESTED. Finding the id with the legal pattern is what let `B60-2` be read
	// as `B60`: the match succeeded on a prefix and nothing downstream could
	// know the cell had said something else.
	tok := backlogIDish.FindString(c[1])
	switch {
	case tok == "":
		// ⛔ A ROW INSIDE A RECOGNISED WORK-ITEM TABLE THAT YIELDS NO ID IS THE
		// CLASS NEITHER INSTRUMENT COULD SEE. The eleven rows of the critical
		// path are inside `| # | Work | Seat | State |`, which this parser
		// READS - they are not lost to an unknown header, they are lost at the
		// id. The whole-file scan cannot help, because there is no id to find.
		if s.inTable && !separatorRow(c) {
			u := Unimported{
				Kind:  UnimportedRowWithoutID,
				Label: strings.TrimSpace(c[1]),
				Cells: contentCells(c),
				Line:  s.line, Under: s.heads.carry(), Section: s.heads.section(),
				SectionTitle: s.heads.sectionTitle(),
			}
			s.readWorkCell(&u, c)
			s.unimported = append(s.unimported, u)
		}
		return nil
	case !backlogIDLegal.MatchString(tok):
		s.accounted[tok] = true
		u := Unimported{
			Kind: UnimportedIrregularID, Label: tok, Cells: contentCells(c),
			Line: s.line, Under: s.heads.carry(), Section: s.heads.section(),
			SectionTitle: s.heads.sectionTitle(),
		}
		s.readWorkCell(&u, c)
		s.unimported = append(s.unimported, u)
		return nil
	}

	id := tok
	s.accounted[id] = true
	if !s.inTable {
		// The adopter table at the foot of rig's backlog puts five backlog ids
		// in two-column rows. Refusing them would kill the parse on rows that
		// were never work items; dropping them silently is what left the set
		// guard unable to tell them from rows that went missing.
		//
		// ⛔ NO Title AND NO Struck, AND THE REASON IS IN THE KIND'S NAME.
		// This is a table whose header this parser does not recognise, so it
		// has no item column: there is no cell it can call the title without
		// guessing which column the next such table will put it in. Cells
		// carries the row whole and asserts nothing.
		s.unimported = append(s.unimported, Unimported{
			Kind: UnimportedOtherTable, ID: id,
			Label: strings.TrimSpace(c[1]), Cells: contentCells(c),
			Line: s.line, Under: s.heads.carry(), Section: s.heads.section(),
			SectionTitle: s.heads.sectionTitle(),
		})
		return nil
	}
	if s.seen[id] {
		return nil
	}
	s.seen[id] = true

	if len(c) <= s.stateCol {
		return fmt.Errorf("%s has %d cells and this table's state column is %d: %.120s",
			id, len(c)-2, s.stateCol, line)
	}
	s.items = append(s.items, s.item(id, c))
	return nil
}

// contentCells is a row's cells without the two empty strings its leading and
// trailing pipes produce, copied so that nothing a caller does to the result
// can reach back into the parse.
//
// ⛔ THE BOUNDS ARE CHECKED RATHER THAN ASSUMED. Every caller today has passed
// `len(c) < 3` first, which is a fact about the callers and not about this
// function; a row with no content cells at all returns none.
func contentCells(c []string) []string {
	if len(c) < 3 {
		return nil
	}
	out := make([]string, len(c)-2)
	copy(out, c[1:len(c)-1])
	return out
}

// readWorkCell fills in the fields that need this table's ITEM column - the
// title and the document's strikethrough - for a row this grain is reporting
// rather than importing.
//
// ⛔ IT IS THE SAME DERIVATION AN IMPORTED ROW GETS AND NOT A SECOND ONE.
// `item` reads titleOf and `HasPrefix(item, "~~")` off the same cell; a row
// that lost its id is still a row of the same table, written by the same hand,
// and a grain that derived its title differently would be the two-readings
// defect this file exists to stop.
//
// It does nothing where the column roles are unknown - outside a recognised
// table - or where the row is too short to have the cell, because a title
// invented from whichever cell happens to be last is worse than none.
func (s *backlogScan) readWorkCell(u *Unimported, c []string) {
	if !s.inTable || s.itemCol < 1 || s.itemCol >= len(c) {
		return
	}
	item := c[s.itemCol]
	u.Title = titleOf(item)
	u.Struck = strings.HasPrefix(item, "~~")
}

// item builds the record for one well-formed row of a recognised table.
func (s *backlogScan) item(id string, c []string) BacklogItem {
	// ⛔ ONE ROW IS MISSING THE PIPE AFTER ITS ID, SO ITS ID CELL ABSORBED THE
	// FRONT OF ITS ITEM CELL - INCLUDING THE CLOSURE MARKER.
	//
	// B21, and it is why the second instrument exists. Reconstructing the item
	// cell is the honest repair: the row is real, it is closed, and every count
	// anyone ran missed it because every one required the pipe.
	//
	// ⛔ DECORATION IS NOT ABSORBED CONTENT AND THE TEST IS WHAT SURVIVES
	// STRIPPING IT. `⛔ **B47**` leaves nothing and is an ordinary row;
	// `B21 ✅ **CLOSED ...` leaves a sentence and is a shifted one. The ORIGINAL
	// remainder is what gets prepended, markers and all, because the title is
	// located inside it.
	item, malformed := c[s.itemCol], false
	rest := strings.TrimSpace(strings.Replace(c[s.idCol], id, " ", 1))
	if strings.Trim(rest, decoration) != "" {
		item, malformed = rest+" "+item, true
	}

	it := BacklogItem{
		ID: id,
		// ⛔ THE TITLE COMES FROM THE RECONSTRUCTED ITEM CELL, NOT FROM cells[2].
		// On a row missing the pipe after its id every cell has shifted left, so
		// cells[2] is the EVIDENCE cell - B21 in rig's own backlog read its title
		// as the word "evidence" for as long as this parser has existed. `item`
		// is already the repaired cell, and for a well-formed row it IS the item
		// cell, so the normal path is untouched.
		Title: titleOf(item),
		// The document's own closure mark, in the item cell: a struck title, or a
		// terminal bold lead where the strikethrough would be.
		Struck:    strings.HasPrefix(item, "~~"),
		Done:      strings.HasPrefix(item, "~~") || terminalDispositions[firstWord(boldLead(item))],
		Malformed: malformed,
		PartOf:    partOf(id),
	}
	it.ClaimsDone = !it.Done && terminalDispositions[firstWord(boldLead(c[s.stateCol]))]
	it.State = strings.TrimSpace(c[s.stateCol])
	// ⛔ BOUNDS-CHECKED RATHER THAN ASSUMED. `stateCol` is checked by the
	// caller and `ownerCol` is not, and a table shape added later with no owner
	// column would index past the row. An absent owner is an honest empty
	// string; a panic in a document parser is not.
	if s.ownerCol > 0 && s.ownerCol < len(c) {
		it.Owner = ownerOf(c[s.ownerCol])
	}
	// ⛔ THE THIRD CLOSURE CONVENTION, REPORTED RATHER THAN COLLAPSED INTO THE
	// OTHER TWO. This document closes a row three ways: a struck title, a
	// terminal bold lead in the item cell, and - for a row closed by a RULING
	// rather than by work - a tick in the ID cell with a terminal word leading
	// the state cell. The first two are `Done`. The third is not, and folding it
	// in would assert that a ruling finished the work. Naming it is what lets a
	// seeder decide; guessing is what produced the four rows that have seeded
	// OPEN since B15.
	// Read from the same stack the other grain uses, never re-derived. A row is
	// not a heading, so the top of the stack is the heading enclosing it.
	it.Section = s.heads.section()
	it.SectionTitle = s.heads.sectionTitle()
	it.RuledClosed = it.ClaimsDone && strings.Contains(c[s.idCol], "✅")
	it.Disposition = dispositionOf(it, item, c[s.stateCol])
	return it
}

// dispositionOf recovers WHICH terminal word closed a row, from whichever part
// of the row the clause that closed it already read.
//
// ⛔ THE STRUCK CASE IS THE ONE THAT IS NOT A LOOKUP. `Done` on a struck row
// was decided by `HasPrefix(item, "~~")` and never touched a word at all; the
// word sits in a SECOND bold run after the strike closes, which `boldLead`
// cannot reach. Bounding the search to `afterStrike` is what stops a struck
// TITLE containing the word DONE from being read as the row's disposition.
func dispositionOf(it BacklogItem, item, state string) string {
	switch {
	case it.Done && it.Struck:
		return closingWord(afterStrike(item))
	case it.Done:
		return closingWord(item)
	case it.ClaimsDone:
		return closingWord(state)
	}
	return ""
}

// partOf is the id a row is a sub-task of, and it comes from the SUB-LETTER
// alone: B46a is part-of B46, and a row whose id carries no letter is part-of
// nothing.
//
// ⛔ HEADING ENCLOSURE WAS TRIED AND IS FALSIFIED BY THE DOCUMENT, WHICH IS
// WHY IT IS NOT HERE. The specification this parser was repaired against
// recommends it - "a row under `## B46` is enclosed by it. Exact, derivable,
// checkable" - and that was reasoned rather than run. ⛔ MEASURED 2026-09-17:
// the table under `## ⛔ B46` has accumulated TWENTY-FOUR rows that are not
// B46's children, B61 ("section 6's configuration system is unbuilt") among
// them. Deciding which of them belongs needs a human reading the prose, which
// is precisely what the projection criterion forbids.
//
// The sub-letter survives because it is INSIDE the id. A row can be moved to
// another section of a file somebody is still editing and its parent does not
// change; a row's position cannot make that promise.
func partOf(id string) string {
	if n := len(id); n > 1 && id[n-1] >= 'a' && id[n-1] <= 'z' {
		return id[:n-1]
	}
	return ""
}

// separatorRow reports whether every content cell is a header rule.
func separatorRow(c []string) bool {
	for _, x := range c[1 : len(c)-1] {
		if !tableRule.MatchString(strings.TrimSpace(x)) {
			return false
		}
	}
	return true
}

// reconcile is the guard, run once the whole document has been read.
//
// ⛔ IT IS AN EQUALITY BETWEEN TWO INSTRUMENTS, NOT A BOUND.
//
// This was `if len(items) < 30 { ... "expected the real file's ~44" }`. Over a
// 44-row file it read 36, and 36 >= 30, so it passed and the message naming 44
// never printed. A BOUND CANNOT NOTICE ITSELF GOING STALE - it is the same
// defect as a check that cannot tell "nothing is wrong" from "the check did not
// run".
//
// So the file is scanned a second time by a pattern sharing no code path with
// the scanner, and the two must agree AS SETS. A count equality would still
// hide a swap; the set difference names the rows that went missing, which is
// the failure that actually happened.
//
// ⛔ AND THE SECOND INSTRUMENT KNOWS NOTHING ABOUT HEADERS, WHICH IS THE
// ASSUMPTION IT NO LONGER SHARES. It finds every id-shaped token in every first
// cell; the scanner decides what each one was. So a token it finds must be
// either imported or EXPLAINED, and "explained" is now a thing a caller can
// read rather than a private map that existed to keep this check quiet.
//
// ⛔ WHAT IT STILL CANNOT DO, STATED SO NOBODY READS MORE INTO A GREEN THAN IS
// THERE: the second instrument reads FIRST CELLS, so it can no more see a
// heading-borne id or an id-less row than the scanner can. Those two classes
// are found by ONE instrument each, and the only real check on them is a pin
// over the reported set against the live document. A second regex over the same
// lines would be two instruments sharing an assumption, which is the failure
// this whole guard descends from rather than a defence against it.
func (s *backlogScan) reconcile(text string) error {
	// A heading id that ALSO became a record is not unimported. Today no
	// heading id is a row, so this removes nothing; it exists so that the day
	// one is both, the report says so rather than accusing the parse.
	kept := s.unimported[:0]
	for _, u := range s.unimported {
		if u.Kind == UnimportedHeading && s.seen[u.ID] {
			continue
		}
		kept = append(kept, u)
	}
	s.unimported = kept

	var missing []string
	for _, m := range backlogRowAnywhere.FindAllStringSubmatch(text, -1) {
		if s.accounted[m[1]] {
			continue
		}
		s.accounted[m[1]] = true
		missing = append(missing, m[1])
	}
	if len(missing) > 0 {
		// ⛔ THE SET, NOT A DERIVED COUNT. An earlier draft of this message
		// reported "and accounted for N further ids" as
		// len(accounted)-len(seen)-len(missing), which undercounts: an id in
		// the adopter table is usually a record as well, so it is counted
		// once. A number in an error message is a claim like any other, and
		// this file has spent its whole history on captions that were not
		// checked. The ids are the evidence; the arithmetic added nothing.
		return fmt.Errorf("the row scanner read %d work items and could explain "+
			"neither importing nor declining %d further ids the whole-file scan "+
			"found in a first cell: it never saw %v", len(s.items), len(missing), missing)
	}
	return nil
}

// decorationNoStrike is `decoration` WITHOUT the tilde.
//
// ⛔ IT EXISTS BECAUSE `decoration` EATS THE MARK IT IS BEING USED TO FIND.
// `decoration` lists `~` so that a struck title trims to its words; trimming
// with it before testing for `~~` therefore always answers false, silently and
// for every input. Caught by the test the first time it ran, which is the only
// reason it is not in the store.
const decorationNoStrike = "⛔✅* \t"

// struckThrough is whether a heading's text is struck through, which is this
// document's own mark for a closed thing.
func struckThrough(text string) bool {
	return strings.HasPrefix(strings.Trim(text, decorationNoStrike), "~~")
}

// headingTitle is a heading's title: its text with the document's decoration,
// its strikethrough, its own id and the separator after the id removed.
//
// ⛔ THE ID IS REMOVED BECAUSE IT IS ALREADY THE RECORD'S ID. A row's id
// cell and its item cell are different cells, so a row title never repeats the
// id; a heading states both in one line, and carrying it through leaves every
// heading-borne record titled with its own name twice.
func headingTitle(text, id string) string {
	t := strings.Trim(text, decoration)
	t = strings.TrimSpace(strings.TrimPrefix(t, "~~"))
	t = strings.TrimSpace(strings.TrimPrefix(t, id))
	// The separators this document uses between a heading's id and its title.
	// A heading that uses none simply keeps its text.
	for _, sep := range []string{"-", "–", "—", ":", "."} {
		if rest := strings.TrimPrefix(t, sep); rest != t {
			t = strings.TrimSpace(rest)
			break
		}
	}
	t = strings.TrimSuffix(strings.TrimSpace(t), "~~")
	return strings.Trim(strings.TrimSpace(t), decoration)
}
