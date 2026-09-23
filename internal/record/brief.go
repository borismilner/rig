package record

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// DefaultNextUpN is how many work items a brief surfaces as "next up" when the
// project record does not override it. Section 39: "Default 5."
const DefaultNextUpN = 5

// DefaultAttentionN is how many notes a CASE's brief surfaces when the case
// record does not override it. Section 39: "Default 10, override per case."
//
// ⛔ NEVER PADDED TO N, the same phrasing section 39 uses for next_up_n. A cap
// is how many the brief will show, not how many it claims exist.
const DefaultAttentionN = 10

// Brief is the derived answer to "what is going on here".
//
// EVERY FIELD IS COMPUTED AT READ TIME AND NOTHING HERE IS STORED. Section 39
// is explicit that auto-completion is derived and never stamped, and the same
// argument covers the whole brief: a stamp with no witness is asserted rather
// than evidenced, and a derivation cannot go stale because it is recomputed
// every time. This is also why no seat writes a sentence of prose to produce
// it - that is the acceptance test, not a style preference.
type Brief struct {
	Project string

	// Open is every active item that is not done, with its latest step.
	// ⛔ DISJOINT FROM NextUp BY CONSTRUCTION - section 39's defect D2: an item
	// appearing in both lists got two incompatible rules for how its notes
	// render. Every item has exactly one rendering because it is in exactly
	// one list.
	Open []ItemState

	// NextUp is the first NextUpN items in execution order.
	NextUp []ItemState

	// Blocked is what cannot start, and on whom.
	//
	// ⛔ A BLOCKER IN HERE MAY BE IN NEITHER Open NOR NextUp. Ruled rig
	// fee7580: an item is blocked when a `blocks` edge points at it from any
	// item whose latest step is not `done`, active or not - so a blocker whose
	// status is `idea` is named here and appears in no other list. A renderer
	// must not assume it can resolve a blocker id against the other two.
	Blocked []Blockage

	// Cycles are the blocks cycles, each naming its items.
	//
	// DETECTED, REPORTED, ORDERED AROUND, NEVER RESOLVED. rig does not pick an
	// edge to break: choosing which blocks edge is the wrong one is a
	// judgement about the work, which is domain logic and section 29's first
	// non-goal. A cyclic backlog is a real state of a real project and it is
	// one Boris would want SURFACED.
	Cycles [][]string

	// CoarseCitations is the count of citations that resolve only to a whole
	// section rather than to one record.
	//
	// RULED BY BORIS 2026-09-16: the migration imports all 4,206 citations and
	// flags the coarse ones rather than dropping them. He accepted the named
	// risk - "a flagged count that nobody ever burns down" - on the condition
	// that the brief SURFACES it. So this field is not decoration, and a brief
	// that omits it has not implemented his ruling.
	CoarseCitations int

	// Notes is SECTION 3: every note part-of the project itself, or part-of a
	// work-item in the OPEN list.
	//
	// ⛔ OPEN, NOT NextUp, AND THAT IS THE FIX FOR A CONTRADICTION SECTION 39
	// ALREADY RESOLVED. Rows 2 and 3 gave the same item two incompatible
	// renderings - a next-up item's notes as a flag, an open item's in full -
	// and the lists were made disjoint so every item has exactly one. Carrying
	// a next-up item's notes here would put that contradiction back.
	Notes []Note

	// Features is SECTION 10: the features at stage `building`.
	//
	// RULED BY BORIS 2026-09-16 with the other ten: "Cover all of them."
	// Section 39's own note on this row is that a brief without features
	// cannot drive the overview the GUI paragraph specifies.
	Features []Feature

	// Stages is section 10's counts per stage, one row each, in lifecycle
	// order. It counts EVERY feature, not only the building ones, which is why
	// it is not derivable from Features above.
	Stages []StageCount

	// Sections is the state of ALL ELEVEN of section 39's brief sections,
	// every time, whether or not each one can be answered.
	//
	// ⛔ IT IS HERE AND NOT AT THE WIRE BECAUSE THE REASON IS THE DERIVATION'S
	// KNOWLEDGE. The daemon decided these until 2026-09-16 and could not help
	// going stale: it reported NOT_COMPUTED for notes and features, blaming a
	// derivation that had already landed, and a reader was sent to build what
	// existed. See sections.go for the ledger and why a struct-derived answer
	// would be wrong.
	//
	// WITHHELD_BY_VIEW IS NOT SET HERE. A view is a property of who is asking
	// and the store does not know who is asking; the daemon applies it over
	// the top. The state is declared in this package so both ends share one
	// vocabulary.
	Sections []SectionStatus

	// CaseNotes is SECTION 11: up to attention_n notes part-of this CASE,
	// priority descending then created_at descending.
	//
	// ⛔ SEPARATE FROM Notes AND NOT A SUBSET OF IT. Row 3 renders every note
	// on a project and its open items, uncapped; row 11 is a case's attention
	// list, capped and ordered by importance. The wire keeps them apart for the
	// same reason. EMPTY ON A PROJECT BRIEF, where the section does not apply -
	// read Sections to tell that from a case with no notes.
	CaseNotes []Note

	// Governing is SECTION 12: every decision, requirement and artefact
	// recorded against this project.
	//
	// ⛔ B64. Three of section 39's ten kinds had no section, so a decision put
	// into rig could be read back only by a caller who already knew its id, or
	// who already knew to ask record.query for kind=decision. Nothing told a
	// reader they were there at all - which is the same "reachable only if you
	// already know" failure the brief exists to end, one level up from the one
	// it was built for.
	//
	// ⛔ EACH ROW CARRIES ITS OWN KIND AND THAT IS THE POINT OF THE SHAPE. One
	// section holding three kinds is not a fold: a fold loses which kind a row
	// is, and a decision that renders as a note has been hidden rather than
	// surfaced.
	Governing []GoverningRecord

	// Closed is SECTION 13: every work item this brief's open lists drop
	// because something CLOSED it, each row carrying that word.
	//
	// ⛔ RULED BY BORIS 2026-09-17, B68, AND THE OPEN LIST DOES NOT CHANGE.
	// 14 of 80 work items were invisible because the derivation rendered only
	// `status == "active"`, and absence was the store's only way of saying a
	// thing was finished. He was shown three shapes and chose this one: a
	// section of its own, because marking the closed rows inside the open list
	// would grow the one part of the brief he has already called hard to read.
	//
	// ⛔ CLOSED IS A FAMILY AND NOT A WORD. `closed` and `closed-by-ruling`
	// are both in the live store, so the membership test is what the status
	// is NOT - see itemClosingWord.
	Closed []ClosedItem

	// ClosedCounts is how many closed items carry each word, word ascending.
	//
	// It is the census the ruling asks the section to show, and it is what
	// makes the family visible as a family rather than as a column a reader
	// has to tally by eye.
	ClosedCounts []WordCount

	// Unlisted is how many work items are in NEITHER the open lists NOR the
	// closed one.
	//
	// ⛔ IT EXISTS BECAUSE THE CLOSED SECTION CREATES A WRONG INFERENCE AND
	// NOTHING ELSE CORRECTS IT. Before this section there was one list and no
	// reason to add anything up; with two, a reader takes open + closed for
	// the total. Measured 2026-09-17 in the live production store: 11 of 95
	// work items carry no `status` field at all, written by rigseed out of a
	// backlog table with no status column, so the sum is wrong by 11.
	//
	// ⛔ A COUNT AND NOT A LIST, AND THAT IS THE HONEST LIMIT. `idea` means the
	// item has not been picked up and an absent status means nobody said - two
	// facts that are neither open work nor closed work, and rendering them as
	// either would be this brief's own reassuring-lie failure. The count says
	// they are there; record.query says which.
	//
	// ⛔ uint64 AND NOT int, AND IT IS THE WIRE'S TYPE ON PURPOSE. A signed
	// count here means the daemon converts on every brief, and a conversion
	// gosec is right to flag: nothing in the type stops a negative arriving,
	// and int -> uint64 turns one into a number larger than the store could
	// ever hold. The bound belongs where the count is DERIVED, not at the
	// crossing - the same argument GoverningCounts and StageCount already make
	// by being uint64 all the way down.
	Unlisted uint64

	// GoverningCounts is how many of each governing kind exist, in the
	// vocabulary's order.
	//
	// SEPARATELY DERIVED FROM Governing FOR SECTION 10's REASON, restated
	// because the pair is the same pair: the list is capped by nothing today
	// and the counts are a total, so deriving one from the other works right up
	// until somebody adds a cap, at which point the counts quietly start
	// describing the capped list instead of the store.
	GoverningCounts []KindCount

	// Kind is the container's own kind, `project` or `case`, and it decides
	// which sections mean anything. Empty when the container has no record.
	Kind string

	// ContainerFound is whether the id this brief is about has a record in
	// this store AT ALL.
	//
	// ⛔ FALSE IS NOT "A PROJECT WITH NOTHING IN IT", AND B76 IS WHAT HAPPENS
	// WHEN THE TWO ARE ONE ANSWER. Every section below is written to
	// distinguish "nothing to report" from "this build cannot answer"; the
	// CONTAINER made no such distinction, so a typo in a slug rendered a
	// complete, confident brief reporting sections 1-4 computed and nothing to
	// do. An agent resuming on the wrong slug was told, in rig's own voice,
	// that its project was clear.
	//
	// ⛔ IT IS A FIELD AND NOT `Kind == ""` REPEATED AT EVERY CALL SITE. The
	// emptiness of Kind already carried this fact and three consumers had to
	// know the rule to read it - which is the same "reachable only if you
	// already know" failure section 12 exists to end. ContainerMissing below
	// is the one place the rule is written.
	//
	// ⛔ A FALSE HERE DOES NOT EMPTY THE BRIEF, AND MUST NOT. Records carry
	// their own `project` field, so work items, decisions and notes can exist
	// under an id that has no container record - `a0-survey` in the live
	// production store is exactly that, measured 2026-09-17. Suppressing the
	// sections would replace one wrong answer with another.
	ContainerFound bool

	// Title, Status and Semver are the rest of the container's own metadata,
	// read from the same record Kind comes from.
	//
	// ⛔ THE DERIVATION READ THE CONTAINER AND KEPT ONLY THE KIND, AND THE
	// STORE HELD THE REST THE WHOLE TIME. Measured 2026-09-17 against the
	// seeded production store: the project record carried title and status and
	// `rig brief rig` printed "(not said) (no status)" over "(no title)" with
	// every row beneath it correct. This is the phase-1 capture-fidelity gate
	// failing on the one project rig holds - itself.
	//
	// ⛔ IT WAS TWO DEFECTS AND NEITHER HALF EXPLAINED THE SCREEN ALONE. The
	// wire carries four header fields and the daemon set none of them,
	// INCLUDING the Kind this package already computed, so the emptiness was
	// reachable from either side and fixing one alone would have changed
	// nothing visible. The daemon half landed at rig af7715d.
	//
	// EMPTY WHEN THE CONTAINER HAS NO RECORD, and a container whose record has
	// no title reads identically. That is deliberate: a missing title is a fact
	// about the project, an unread one is a defect in the pipeline, and giving
	// the defect its own rendering would teach every reader that there are two
	// normal kinds of blank. The pipeline defect is caught by a
	// descriptor-coverage guard over the wire, not on a human's screen.
	Title  string
	Status string

	// DescriptionShort and DescriptionLong are the container's own description,
	// and BOTH ARE SECTION 39'S OWN FIELD NAMES rather than anything invented
	// here: section 39's field table gives `description_short` ("one line, for
	// lists and briefs") and `description_long` ("the prose `body` was going to
	// carry - typed and separate rather than one blob") to `project` as well as
	// to `work-item`, and section 39's case table repeats both for `case`.
	//
	// ⛔ THEY EXIST BECAUSE BORIS ASKED THE WINDOW FOR SOMETHING IT COULD NOT
	// RENDER. 2026-09-17: "Each project should start with the name of the
	// project, the overall state something similar to what there is now, some
	// description of the project to remind what it is about." The name is
	// Title and the state is the counts below; the REMINDER had nothing behind
	// it. plan/11 records the finding as "a STORAGE gap and not a layout one".
	//
	// ⛔ NOTHING NEW WAS ADDED TO THE STORE TO CARRY THEM, and that is plan/11's
	// order being followed rather than a shortcut: "TRY THE EXISTING FIELDS
	// FIRST", with dedicated fields authorised only if the existing ones will
	// not do. A Record already has a free-form Fields map, so both names were
	// storable the whole time and the gap was ONLY ever this derivation, the
	// wire, and the absence of a writer. The authorisation to add a dedicated
	// field was not needed and so was not used.
	//
	// EMPTY IS HONEST AND IS NOT A DEFECT. A project whose record carries no
	// description reads empty, exactly as a missing title does, for the reason
	// already given above: giving the pipeline defect its own rendering would
	// teach every reader that there are two normal kinds of blank.
	DescriptionShort string
	DescriptionLong  string

	// Semver is empty on a case BECAUSE NOTHING WROTE ONE, not because this
	// derivation suppresses it. Section 39 rules that a case has no semver -
	// "a case does not ship, so it has no version to advance" - and the wire
	// says field 12 is empty on a case. Reading the field satisfies both
	// without a rule, since no case record carries one.
	//
	// ⛔ SUPPRESSING IT BY KIND WAS REFUSED, and the refusal is recorded rather
	// than left as an absence. It is a narrowing no lead ruled, and it would
	// make the brief hide a field a record genuinely carried - against this
	// package's own B21 finding, that reporting what the document says beats
	// inventing what it meant. If a case must ever hide a semver it wrote, that
	// is a ruling and not a tweak.
	Semver string
}

// ContainerMissing is the ONE statement of B76's condition in this package.
//
// It is a method rather than a comparison spelled out at each reader, because
// the comparison is not self-explaining: `!ContainerFound` reads as a fact
// about the STORE, and what a caller needs to decide is whether the ANSWER it
// is holding describes anything at all.
//
// ⛔ A BRIEF WHOSE CONTAINER IS MISSING IS STILL A BRIEF AND IS STILL
// ACCURATE. Its lists are the records that name this project, and there may be
// many. What it cannot do is claim the project exists, and a caller that
// reports it as an ordinary result has made exactly B76's mistake.
func (b Brief) ContainerMissing() bool { return !b.ContainerFound }

// ItemState is a work item and the last thing that happened to it.
//
// ⛔ IT CARRIES SECTION 39 ROW 2's COMPACT CARD, AND ROW 2 SPECIFIES NINE
// FIELDS. Until 2026-09-16 this struct held five, so the brief's headline list -
// the one thing a reader looks at first - answered five of nine and the CLI
// renderer had Owner and Priority fields it could never fill.
//
// THE CARD IS NEVER description_long, and that is row 2 in as many words: "a
// list meant to be scanned in one pass fails its own readability requirement
// the moment it carries a paragraph per row; the long form is one record.get
// away." The same rule governs Note, below.
type ItemState struct {
	ID    string
	Title string

	// DescriptionShort is row 2's one line for lists and briefs.
	DescriptionShort string

	// Priority is the item's own ordering signal, distinct from the topological
	// order: section 39 breaks ties in next-up BY priority, and a blocks edge is
	// a hard dependency where priority is relative importance.
	Priority string

	// Status is the RECORD's status - `idea` or `active` - and it is NOT State.
	//
	// ⛔ THE TWO ARE DIFFERENT FIELDS ANSWERING DIFFERENT QUESTIONS AND SECTION
	// 39 IS EXPLICIT ABOUT IT. Status says whether the item has been picked up;
	// `idea` means no progress stream is expected yet. State is the latest
	// step's own state - started, blocked, done - and section 39 refuses to keep
	// a second completion field in sync with it. Collapsing them is defect D5
	// arriving from the other direction: an item with status `active` and a
	// `done` latest step is finished, and an item with status `idea` and no
	// steps has not begun. One field cannot say both.
	Status string

	// Owner is which seat is currently driving it.
	Owner string

	// Tags are row 2's free-form grouping.
	//
	// STORED AS A JSON ARRAY IN THE FIELDS BLOB, and that is a decision of this
	// seat's rather than a reading of section 39, which specifies tags as
	// "free-form grouping, QUERYABLE like any typed field" and never says how a
	// list lives in a map[string]string. It was taken on measurement rather than
	// taste, against the real driver:
	//
	//	json_each(json(fields ->> 'tags')) WHERE value = 'ops'   -> 1
	//	                                   WHERE value = 'op'    -> 0
	//	fields ->> 'flat' LIKE '%ops%'                           -> 1
	//	fields ->> 'flat' LIKE '%dev%'                           -> 1  ⛔
	//
	// The last row is why. A comma-joined string cannot be queried without
	// matching inside another tag - searching `dev` finds `devops` - so the flat
	// form satisfies rendering and silently fails the requirement that makes
	// tags worth having. The fields blob is already JSON in SQLite and
	// CoarseCitations already reads it with ->>, so this costs no schema change.
	//
	// A VALUE THAT IS NOT A JSON ARRAY YIELDS NO TAGS RATHER THAN AN ERROR. A
	// brief is a read of whatever is in the store, and one malformed field on
	// one record must not refuse the whole answer.
	Tags []string

	// TargetDate is row 2's optional date, carried as the string that was
	// stored. Section 39 names the field and no format; parsing it here would
	// invent one and refuse every record that disagreed.
	TargetDate string

	// ItemType is `task`, `bug`, `idea` - the classification WITHIN work-item.
	//
	// ⛔ IT IS NOT `Kind`. Kind separates a work-item from a decision from a
	// requirement; this separates work-items from each other. Boris asked for
	// it on 2026-09-17 with "appropriate icons", and section 39's field table
	// carries the full statement.
	//
	// ⛔ EMPTY IS A REAL ANSWER AND THE COMMON ONE. Nothing may infer a type
	// from a row's wording: BACKLOG.md says nothing about whether a row is a
	// bug, and a type guessed from prose is a seat composing content. An
	// untyped item says untyped.
	ItemType string

	// Semver is on the card because Boris's word for the metadata was
	// "exhaustive" and the brief is where metadata is seen - one short string
	// answering "how far along is this".
	Semver string

	// HasNote is whether a NOTE-kinded record is attached to this item.
	//
	// ⛔ IT IS NOT "Note != \"\"", AND THE TWO ARE DIFFERENT THINGS. Note below
	// is the latest PROGRESS STEP's own words, which every item with a stream
	// has. This flag is row 2's "whether a note is attached" - a `note` record
	// linked part-of the item, which is Boris's comment and question mechanism.
	// An item can have a busy progress stream and no note at all, and a note
	// with no steps since it was written. Row 3 renders those notes in full for
	// the OPEN list; the compact card carries only the flag, and the text is one
	// record.refs away.
	HasNote bool

	// State is the latest step's state, or "" when the stream is empty.
	// An active item with no steps has been picked up and not yet reported on.
	State string

	// Since is when that step was recorded. Zero when there are no steps.
	Since time.Time

	// Note is the latest step's own words.
	Note string
}

// card fills section 39 row 2's compact card from the item record itself.
//
// Every field is read through the same Fields map the store already carries, so
// a record written without one of them produces an empty string rather than a
// refusal: the brief answers for the store as it is, and a migration that has
// not filled target_date yet is not an error condition.
func card(it Record) ItemState {
	return ItemState{
		ID:               it.ID,
		Title:            it.Fields["title"],
		DescriptionShort: it.Fields["description_short"],
		Priority:         it.Fields["priority"],
		Status:           it.Fields["status"],
		Owner:            it.Fields["owner"],
		Tags:             DecodeTags(it.Fields["tags"]),
		TargetDate:       it.Fields["target_date"],
		ItemType:         it.Fields["item_type"],
		Semver:           it.Fields["semver"],
	}
}

// closedItems is section 13: every work item the open lists drop because
// something closed it, the per-word census, and how many items are in NEITHER
// list.
//
// ⛔ THE ACTIVE SET IS THE INPUT, NOT A SECOND COPY OF ITS PREDICATE. Deciding
// membership here by re-testing `status` and the latest step would be the same
// rule written twice, and the day one half moves an item leaves both lists or
// appears in both. "Not open" is the only definition that cannot drift.
//
// ⛔ AND THE CENSUS IS ACCUMULATED BESIDE THE LIST RATHER THAN FROM IT, for the
// reason GoverningCounts records: a count taken from a list starts describing
// the list the day somebody caps it.
func closedItems(items []Record, latest map[string]Record,
	active map[string]ItemState,
) ([]ClosedItem, []WordCount, uint64) {
	closed := make([]ClosedItem, 0, len(items))
	counts := map[string]uint64{}
	var unlisted uint64
	for _, it := range items {
		if _, open := active[it.ID]; open {
			continue
		}
		word := itemClosingWord(it.Fields["status"], latest[it.ID].Fields["state"])
		if word == "" {
			unlisted++
			continue
		}
		closed = append(closed, ClosedItem{
			ID: it.ID, Title: it.Fields["title"], Word: word,
		})
		counts[word]++
	}
	sort.Slice(closed, func(i, j int) bool { return closed[i].ID < closed[j].ID })

	words := make([]WordCount, 0, len(counts))
	for w, n := range counts {
		words = append(words, WordCount{Word: w, Count: n})
	}
	sort.Slice(words, func(i, j int) bool { return words[i].Word < words[j].Word })

	if len(closed) == 0 {
		closed = nil
	}
	if len(words) == 0 {
		words = nil
	}
	return closed, words, unlisted
}

// itemClosingWord is WHAT CLOSED an item, or empty when nothing did.
//
// ⛔ THE FAMILY IS DEFINED BY EXCLUSION AND THIS IS THE WHOLE OF B68. The live
// store holds `closed` AND `closed-by-ruling`; a predicate written against the
// literal `"closed"` drops the second silently, which is the invisibility the
// section was ruled to end. So every status that is not one of the three
// NON-closing words is a closing word, including ones nobody has written yet.
//
// ⛔ THE THREE EXCLUSIONS EACH SAY SOMETHING DIFFERENT AND NONE OF THEM SAYS
// FINISHED. `active` is open work. `idea` is section 39's word for "has not
// been picked up". An ABSENT status is nobody having said - 11 of the live
// store's 95 work items, written by rigseed out of a backlog table with no
// status column. Filing any of the three under a heading reading CLOSED would
// state something no document says; they are counted in Brief.Unlisted instead.
//
// ⛔ THE STATUS WORD WINS OVER THE STEP, because it is what a person wrote. An
// item stepped `done` whose status still reads `active` is closed by its
// progress stream and B68's own row names that case - absence is how this store
// already expresses closure for the progress-stepped items.
func itemClosingWord(status, step string) string {
	switch status {
	case "", "active", "idea":
		if step == "done" {
			return "done"
		}
		return ""
	default:
		return status
	}
}

// DecodeTags reads the JSON array a tags field holds.
//
// ⛔ A MALFORMED VALUE IS NO TAGS, NEVER AN ERROR, and that is deliberate
// rather than lazy. The alternative is a brief that refuses to answer because
// one record out of forty-five has a hand-written tags field - which would make
// the whole derivation hostage to the worst row in the store, in a system whose
// entire argument is that it answers from what is actually there.
//
// ⛔ EXPORTED BECAUSE THE UNEXPORTED HALF LET A DEFECT SHIP FOR THE LIFE OF
// THIS FIELD: `EncodeTags` was exported and this was not, so a writer could
// not check its own write with the reader's code. **A tolerant reader needs
// its writer to be able to run it**, or the tolerance hides the writer's bug.
// The measurement is in `DECISIONS.md`, 2026-09-18.
func DecodeTags(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EncodeTags renders a tag list for a record's tags field.
//
// IT EXISTS SO THE ENCODING HAS ONE DEFINITION. A caller that joins tags with a
// comma and a caller that writes JSON produce a store where half the tags are
// queryable, and nothing would report it - the flat rows simply return no
// matches and read as items with no tags. Pairs with DecodeTags.
func EncodeTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return ""
	}
	return string(b)
}

// Age is how long the item has been sitting at its latest step.
//
// Section 39: "a stale step beside a live session is the signal that a seat is
// stuck." Zero for an item with no steps, because "infinitely stale" would
// sort it above every real signal.
func (i ItemState) Age(asOf time.Time) time.Duration {
	if i.Since.IsZero() {
		return 0
	}
	return asOf.Sub(i.Since)
}

// Blockage is one item and everything it is waiting on.
type Blockage struct {
	Item      string
	Title     string
	BlockedBy []string
}

// Note is section 39 row 3's note, RENDERED IN FULL.
//
// "never summarised - this is Boris's comment/question mechanism, and an agent
// that skips it has not read the item." So the body is carried whole. That is
// the opposite of the compact card's rule and both are deliberate: the card is
// a list to scan, this is a question somebody asked and is waiting on.
type Note struct {
	ID string

	// Title is what the document called this note, and it is what a reader
	// needs FIRST.
	//
	// ⛔ IT WAS DERIVED AND THEN DROPPED, WHICH IS WHY THE WINDOW SHOWED PROSE
	// WHERE A NAME BELONGS. Every one of the eight notes in Boris's store
	// carries `fields["title"]` - "Naming", "What rig is" - and this struct had
	// no slot for it, so the window listed each note by the first 90 characters
	// of its BODY. One of his is 1,951 bytes and one is empty, so that list
	// read as a truncated paragraph and a blank line. Boris, 2026-09-18: "fix
	// the rig functionality". B97.
	Title string

	Body     string
	Priority string

	// About is the record this note is part-of - the project itself, or one of
	// its work-items. A note with no About has nothing to be read against.
	About string

	Prov Provenance
}

// Feature is section 39 row 10's feature, for the features-at-stage list.
type Feature struct {
	ID    string
	Title string
	Stage string
}

// StageCount is how many features sit at one stage.
//
// ⛔ A SLICE AND NOT A MAP, AND THE REASON IS DETERMINISM RATHER THAN STYLE.
// Go randomises map iteration order, so a map would produce a different
// response ordering on every call and a golden test over the rendered brief
// would flake. The team-lead asked for this shape by name for exactly that.
type StageCount struct {
	Stage string
	Count uint64
}

// GoverningRecord is one row of section 12: what governs this project.
//
// ⛔ Kind IS NOT OPTIONAL AND IS NOT DECORATION. It is what makes one section
// carrying three kinds different from a fold. A reader has to be able to tell a
// ruling from a requirement from a file that exists, because they are answers
// to three different questions, and a row that cannot say which it is has
// hidden the distinction rather than rendered it.
type GoverningRecord struct {
	ID    string
	Kind  string
	Title string
}

// KindCount is how many records of one kind exist.
//
// A SLICE AND NOT A MAP, for StageCount's reason above: Go randomises map
// iteration, so a map renders differently on every call and a golden test over
// the brief would flake.
type KindCount struct {
	Kind  string
	Count uint64
}

// ClosedItem is one work item that is NOT open, and THE WORD THAT CLOSED IT.
//
// ⛔ THE WORD IS THE WHOLE ROW AND IT IS WHAT BORIS RULED, 2026-09-17: the
// brief gains a section carrying the closed rows "with the word that closed
// them". `closed` and `closed-by-ruling` are both in the live store, and a
// section that printed one heading over both would have hidden the second
// inside the first - the fold that GoverningRecord.Kind already refuses one
// section up.
//
// ⛔ IT IS NOT ItemState. The compact card carries nine fields for work that is
// live - a state, an age, a latest note, a priority to rank it by - and every
// one of them answers a question nobody asks about finished work. A reader of
// this section is asking what happened to B55, which is three strings.
type ClosedItem struct {
	ID    string
	Title string

	// Word is what closed it: the record's own status when it carries a
	// closing one, and `done` when the thing that closed it is the progress
	// stream. EMPTY IS IMPOSSIBLE HERE by construction - an item with no
	// closing word is not in this list at all, it is in Brief.Unlisted.
	Word string
}

// WordCount is how many closed items carry one closing word.
//
// ⛔ SEPARATELY DERIVED FROM THE LIST, for the reason GoverningCounts and
// StageCount both record: a total derived from a list starts describing the
// list the day somebody caps it, and the cap is the change nobody remembers
// was load-bearing.
//
// A SLICE AND NOT A MAP, for StageCount's reason: Go randomises map iteration.
type WordCount struct {
	Word  string
	Count uint64
}

// governingKinds is section 12's vocabulary, IN THE ORDER OF CONSEQUENCE:
// what was ruled, what is required, what exists.
//
// ⛔ THE ORDER IS THE SAME ARGUMENT featureStages MAKES AND IT IS WORTH
// RESTATING RATHER THAN CROSS-REFERENCING, BECAUSE THE ALPHABETICAL ANSWER IS
// SO MUCH CHEAPER TO REACH FOR. artefact/decision/requirement reads as nothing.
// decision/requirement/artefact reads as a project: here is what was settled,
// here is what it must do, here is what came out.
//
// ⛔ IT IS CLOSED, AND UNLIKE featureStages A VALUE OUTSIDE IT IS NOT REPORTED
// HERE. That is not an inconsistency: a stage outside the vocabulary is a typo
// in a field on a record that IS a feature, so the brief shows it. A kind
// outside this set is a different kind entirely and already has its own section
// or none - appending it would make section 12 a dump of everything the other
// eleven do not claim, which is the failure the negative half of
// TestSectionTwelveCarriesTheGoverningKindsAndNothingElse exists to catch.
var governingKinds = []string{KindDecision, KindRequirement, KindArtefact}

// featureStages is section 39's closed stage vocabulary, IN LIFECYCLE ORDER.
//
// The order is the point: counts rendered planned/building/shipped/deprecated
// read as a pipeline, where alphabetical (building, deprecated, planned,
// shipped) reads as nothing. A stage outside this set still gets a row - the
// brief reports the store as it is - and sorts after the known ones by name,
// so an unrecognised value is visible rather than dropped.
var featureStages = []string{"planned", "building", "shipped", "deprecated"}

// priorities is section 39's priority vocabulary, MOST IMPORTANT FIRST.
//
// RULED BY THE LEAD 2026-09-16, not by Boris, and cheap to overturn: it is one
// table in one file. Section 39 had used priority as an ordering signal since
// 2026-09-15 in two places and had never anywhere said what its values are.
// This seat held section 11 rather than guessing a rank, which is why there is
// a ruling here instead of a reading.
//
// ⛔ ONE DEFINITION, USED BY BOTH SORTS. That is the ruling rather than a
// preference - two rank tables drift, and the lead argued it from this
// package's own EncodeTags comment six hundred lines up. The two sites section
// 39 binds are next-up's tie-break and a case's attention_n notes.
//
// ⛔ AND THE SORT THAT SLIPS THROUGH IS NOT THE ONE SECTION 39 NAMES. Over raw
// strings h < l < m, so ASCENDING is high, low, medium - which agrees with this
// rank on any two values and disagrees only once medium is present. Descending
// is medium, low, high, which any two-value test catches. Section 39's word is
// "descending", so the direction a literal implementation reaches for is the
// safe one, and the dangerous one is a leftover sort.Strings that nobody would
// think to write down. That is why priority_test.go seeds three values and an
// empty one rather than two.
var priorities = []string{"high", "medium", "low"}

// priorityRank is where a priority sorts. Lower comes first.
//
// ⛔ AN UNRECOGNISED OR EMPTY VALUE RANKS AFTER ALL THREE AND IS NEVER DROPPED.
// Dropping is how a work item disappears from the one list that exists to
// surface it. This is deliberately the same shape as an unrecognised feature
// stage above - the lead ruled the two together so they cannot disagree about
// the unknown case, because two rules in one package that disagree about it is
// how the next reader learns the wrong one.
func priorityRank(p string) int {
	for i, known := range priorities {
		if p == known {
			return i
		}
	}
	return len(priorities)
}

// sortByPriority orders a set of ready work-item ids: priority first, then id.
//
// ⛔ IT REPLACES A sort.Strings THAT READ AS A CHOICE AND WAS NOT ONE. Section
// 39 binds this: "a topological sort over the blocks graph among status: active
// work-items, unresolved dependencies excluded, TIES BROKEN BY priority." The
// spec has said so since the section was written; the code ordered by id alone
// and ItemState.Priority was filled and read by nothing.
//
// id remains the FINAL tie-break, so the same store still answers the same way
// twice - which is what makes a golden test over the brief possible at all.
func sortByPriority(ids []string, active map[string]ItemState) {
	sort.SliceStable(ids, func(i, j int) bool {
		ri, rj := priorityRank(active[ids[i]].Priority), priorityRank(active[ids[j]].Priority)
		if ri != rj {
			return ri < rj
		}
		return ids[i] < ids[j]
	})
}

// BriefStore is everything the brief derivation reads, and nothing else.
//
// ⛔ IT EXISTS BECAUSE THE DERIVATION IS LEAVING AND THE STORE IS NOT. Section
// 43 measured the one real coupling in this package: twelve methods on *Store
// were DEFINED in the two planning files and four of them reached `s.db`
// directly, so the application had grown into the platform's private state.
// Section 50's move 2 (B100) states that finding in code - the four raw reads
// are the store's and are exported here by name, and everything below this
// declaration is a plain function over the interface.
//
// ⛔ IT IS A STEPPING STONE AND NOT THE END STATE, section 50 decision 2. When
// the planner is its own program it implements this interface over
// `record.query` on the socket rather than over a *Store, and the seven
// methods here are the list of what that client has to answer.
//
// EVERY METHOD IS EXPORTED ON PURPOSE: an interface with an unexported method
// can only ever be satisfied from inside this package, which is exactly the
// coupling being removed.
type BriefStore interface {
	// Get reads one record at its head version.
	Get(ctx context.Context, id string) (Record, error)
	// Query reads every head record of one kind in one project.
	Query(ctx context.Context, project, kind string) ([]Record, error)
	// LinksFrom reads the destinations of one link type out of one record.
	LinksFrom(ctx context.Context, src, typ string) ([]string, error)
	// LatestSteps is the newest progress step on every item, by item id.
	LatestSteps(ctx context.Context, project string) (map[string]Record, error)
	// ItemsWithANote is the set of records in a project carrying a note.
	ItemsWithANote(ctx context.Context, project string) (map[string]bool, error)
	// CoarseCitations counts citations that resolve only to a section.
	CoarseCitations(ctx context.Context, project string) (int, error)
	// NotesAbout reads every note part-of a record in one project.
	NotesAbout(ctx context.Context, project string, subjects map[string]bool) ([]Note, error)
}

// containerFor reads the container record and copies its own metadata onto
// the brief, answering the record itself for the derivations that need it.
//
// ⛔ A MISSING CONTAINER IS NOT AN ERROR AND THAT IS DELIBERATE. The brief
// answers for the store as it is, and nextUpN has always fallen back to the
// default rather than refusing - but it is no longer silently a PROJECT either:
// Kind stays empty and the section ledger uses that to say section 11 cannot be
// placed rather than guessing.
//
// ⛔ EXTRACTED FROM Brief BECAUSE B64's TWELFTH SECTION PUT IT AT gocyclo 26
// AGAINST A CEILING OF 25. The ceiling was not raised: the same trade
// serveRecord records about serveSelf, which crossed on ONE inline case. A
// derivation that is one branch under a limit is one section away from being
// over it, and the limit is doing its job when that is what forces the split.
func containerFor(ctx context.Context, src BriefStore, project string, b *Brief) (Record, error) {
	container, err := src.Get(ctx, project)
	if err != nil {
		if !errors.As(err, new(*NotFoundError)) {
			return Record{}, err
		}
		// ⛔ B76. THE ONLY PLACE IN THE DERIVATION THAT KNOWS THE CONTAINER IS
		// ABSENT, AND IT USED TO RETURN THAT KNOWLEDGE AS A ZERO VALUE. The
		// brief carried on and answered every section, and nothing downstream
		// could tell a mistyped slug from a project with no work.
		return Record{}, nil
	}
	b.ContainerFound = true
	b.Kind = container.Kind
	b.Title = container.Fields["title"]
	b.Status = container.Fields["status"]
	b.Semver = container.Fields["semver"]
	b.DescriptionShort = container.Fields["description_short"]
	b.DescriptionLong = container.Fields["description_long"]
	return container, nil
}

// Brief derives the answer to "what is going on here" for one project.
//
// ⛔ THE METHOD IS THE STORE'S AND THE DERIVATION IS NOT. The signature is
// frozen - internal/daemon and cmd/rig both call it - and the body is one
// line so that deriveBrief below can be compiled against BriefStore rather
// than against this package's concrete store. That is the whole of B100.
func (s *Store) Brief(ctx context.Context, project string) (Brief, error) {
	return deriveBrief(ctx, s, project)
}

func deriveBrief(ctx context.Context, src BriefStore, project string) (Brief, error) {
	if project == "" {
		return Brief{}, errors.New("record: a brief needs a project")
	}
	b := Brief{Project: project}

	// ⛔ THE CONTAINER IS READ ONCE AND ITS KIND DECIDES THE SHAPE. Section 39
	// rules that project.brief takes a container of kind project OR case, and
	// this derivation had never looked: it produced project shapes for whatever
	// id it was handed. A MISSING CONTAINER IS NOT AN ERROR - the brief answers
	// for the store as it is, and nextUpN has always fallen back to the default
	// rather than refusing - but it is no longer silently a project either.
	container, err := containerFor(ctx, src, project, &b)
	if err != nil {
		return Brief{}, err
	}
	led := newSectionLedger(b.Kind)

	items, err := src.Query(ctx, project, "work-item")
	if err != nil {
		return Brief{}, err
	}
	latest, err := src.LatestSteps(ctx, project)
	if err != nil {
		return Brief{}, err
	}

	// THE ACTIVE SET, AND BOTH HALVES OF THE PREDICATE ARE REQUIRED.
	//
	// Section 39 is explicit that completion lives in the progress stream and
	// NOT in `status`, so `status == active` on its own keeps a finished item
	// in next-up forever and ships a list that never empties. That was defect
	// D5 and this is the corrected reading.
	// ONE QUERY FOR THE WHOLE PROJECT, NOT ONE PER ITEM. The flag is a set
	// membership test, and asking it per item would be forty-five round trips
	// to answer forty-five booleans - the shape section 39's own traversal
	// numbers exist to keep out of the brief.
	noted, err := src.ItemsWithANote(ctx, project)
	if err != nil {
		return Brief{}, err
	}

	active := map[string]ItemState{}
	for _, it := range items {
		if it.Fields["status"] != "active" {
			continue
		}
		st := card(it)
		st.HasNote = noted[it.ID]
		if step, ok := latest[it.ID]; ok {
			st.State = step.Fields["state"]
			st.Since = step.Prov.CreatedAt
			st.Note = step.Body
			if st.State == "done" {
				continue
			}
		}
		active[it.ID] = st
	}

	edges, err := blockEdges(ctx, src, active)
	if err != nil {
		return Brief{}, err
	}

	order, cycles := topoSort(active, edges)
	b.Cycles = cycles

	blockers, err := blockersOf(ctx, src, items, latest, active)
	if err != nil {
		return Brief{}, err
	}

	// NEXT UP IS WHAT CAN BE STARTED NOW, so a blocked item is never in it -
	// including one whose blocker is in neither list. The lists stay disjoint
	// and together still carry every active item.
	n := nextUpN(container)
	led.did(SectionNextUp)
	led.did(SectionOpen)
	led.did(SectionBlocked)
	for _, id := range order {
		if len(b.NextUp) < n && len(blockers[id]) == 0 {
			b.NextUp = append(b.NextUp, active[id])
			continue
		}
		b.Open = append(b.Open, active[id])
	}

	for id, on := range blockers {
		sort.Strings(on)
		b.Blocked = append(b.Blocked, Blockage{Item: id, Title: active[id].Title, BlockedBy: on})
	}
	sort.Slice(b.Blocked, func(i, j int) bool { return b.Blocked[i].Item < b.Blocked[j].Item })

	if b.CoarseCitations, err = src.CoarseCitations(ctx, project); err != nil {
		return Brief{}, err
	}

	// SECTION 3. The subjects are the project itself and every item in the OPEN
	// list - never the next-up ones, which carry only HasNote.
	subjects := map[string]bool{project: true}
	for _, it := range b.Open {
		subjects[it.ID] = true
	}
	if b.Notes, err = src.NotesAbout(ctx, project, subjects); err != nil {
		return Brief{}, err
	}
	led.did(SectionNotes)

	// SECTION 10.
	if b.Features, b.Stages, err = features(ctx, src, project); err != nil {
		return Brief{}, err
	}
	led.did(SectionFeatures)

	// SECTION 12, B64.
	if b.Governing, b.GoverningCounts, err = governing(ctx, src, project); err != nil {
		return Brief{}, err
	}
	led.did(SectionGoverning)

	// SECTION 11, and only for a case. A project's notes are section 3 above;
	// this is the case's own attention list, capped and ordered by importance.
	if b.Kind == KindCase {
		if b.CaseNotes, err = caseNotes(ctx, src, container); err != nil {
			return Brief{}, err
		}
		led.did(SectionCaseNotes)
	}

	// SECTION 13, B68, RULED BY BORIS 2026-09-17. A SEPARATE PASS OVER THE
	// SAME ITEMS, AND THAT IS THE RULING EXPRESSED IN CODE: he chose a section
	// of its own precisely so the open list would not change, so the active-set
	// loop above is untouched and this reads what that loop declined to keep.
	b.Closed, b.ClosedCounts, b.Unlisted = closedItems(items, latest, active)
	led.did(SectionClosed)

	// ⛔ THE SECTION STATES ARE THE DERIVATION'S AND THIS IS WHERE THEY LAND.
	// They lived in the daemon as a hand-kept list until now, and its own
	// comment claimed the answer was derived from this struct - it was not, and
	// two rows went stale within a day of internal/record growing Notes and
	// Features. Only the code that computes a section knows whether it did.
	//
	// statuses() REFUSES rather than answering ten of eleven, which is why this
	// can return an error at the very end of a derivation that has otherwise
	// succeeded. That is a programming error being made loud, not a data
	// condition: a brief whose section list is incomplete is indistinguishable
	// from one whose missing section is fine.
	if b.Sections, err = led.statuses(); err != nil {
		return Brief{}, err
	}
	return b, nil
}

// ⛔ WHAT IS BLOCKED, AND ON WHOM - AND THE BLOCKERS COME FROM OUTSIDE THE
// ACTIVE SET ON PURPOSE. Ruled by the team-lead, rig fee7580, section 39.
//
// The ordering above runs over the active set and must: a topological sort
// has to be over the nodes being ordered. THE BLOCKED DETERMINATION IS A
// DIFFERENT QUESTION and had silently inherited the same filter, because
// `edges` is blockEdges(active) with BOTH ends filtered.
//
// The defect that hid inside it: the filter is right for a blocker whose
// latest step is `done` - finished work is not a live dependency - and
// WRONG for every other kind. An item whose status is `idea` is also
// outside the active set, `idea` being section 39's word for "has not been
// picked up", so the brief called an item ready while the thing it waits on
// had not been started. That is the mirror of the never-empties defect
// section 39 already corrected, and the code comment here could not see it
// because it only ever reasoned about `done`.
//
// Extracted from Brief when it crossed the house cyclomatic bound: it is a
// question with its own name and its own ruling, and it reads better beside
// them than inside a derivation that does eleven other things.
func blockersOf(ctx context.Context, src BriefStore, items []Record,
	latest map[string]Record, active map[string]ItemState,
) (map[string][]string, error) {
	blockers := map[string][]string{}
	for _, it := range items {
		if step, ok := latest[it.ID]; ok && step.Fields["state"] == "done" {
			continue
		}
		dsts, err := src.LinksFrom(ctx, it.ID, LinkBlocks)
		if err != nil {
			return nil, err
		}
		for _, d := range dsts {
			if _, ok := active[d]; ok {
				blockers[d] = append(blockers[d], it.ID)
			}
		}
	}
	return blockers, nil
}

// nextUpN reads the container's override, falling back to the default.
//
// It takes the record rather than re-reading it: Brief already holds the
// container, and two reads of one row is two chances for them to disagree.
func nextUpN(container Record) int {
	return capFrom(container, "next_up_n", DefaultNextUpN)
}

// capFrom reads a positive integer field, falling back to def.
//
// ⛔ A MALFORMED OR ABSENT VALUE IS THE DEFAULT, NEVER A REFUSAL, and zero or
// negative is malformed. A cap of zero would render an empty list that means
// "nothing here" while the store is full, which is the reassuring lie this
// brief exists to refuse - and a hand-written field should not be able to
// silence a section.
func capFrom(container Record, field string, def int) int {
	var n int
	if _, err := fmt.Sscanf(container.Fields[field], "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}

// blockEdges returns the blocks edges with BOTH ends in the active set.
//
// Restricted deliberately: an edge from a done item is not a live dependency,
// and carrying it would put finished work back into the ordering.
func blockEdges(ctx context.Context, src BriefStore, active map[string]ItemState) (map[string][]string, error) {
	out := map[string][]string{}
	for id := range active {
		dsts, err := src.LinksFrom(ctx, id, LinkBlocks)
		if err != nil {
			return nil, err
		}
		for _, d := range dsts {
			if _, ok := active[d]; ok {
				out[id] = append(out[id], d)
			}
		}
	}
	return out, nil
}

// topoSort orders the active set so that a blocker comes before what it blocks,
// and returns every cycle it could not order.
//
// ⛔ THE CYCLE IS REPORTED AND THE REST IS STILL ORDERED. A derivation that
// refused to answer because the graph has a cycle would be the other failure
// section 39 names - "swallowing it silently is the failure; so is refusing to
// answer." Items in a cycle are appended after the orderable ones, in a stable
// order, so the brief is still usable while the cycle is visible.
func topoSort(active map[string]ItemState, edges map[string][]string) ([]string, [][]string) {
	indeg := map[string]int{}
	for id := range active {
		indeg[id] = 0
	}
	for _, dsts := range edges {
		for _, d := range dsts {
			indeg[d]++
		}
	}

	var ready []string
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sortByPriority(ready, active)

	var order []string
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)

		var freed []string
		for _, d := range edges[id] {
			indeg[d]--
			if indeg[d] == 0 {
				freed = append(freed, d)
			}
		}
		sortByPriority(freed, active)
		ready = append(ready, freed...)
		sortByPriority(ready, active)
	}

	if len(order) == len(active) {
		return order, nil
	}

	// WHAT IS LEFT IS EXACTLY THE ITEMS IN OR DOWNSTREAM OF A CYCLE.
	remaining := map[string]bool{}
	for id := range active {
		remaining[id] = true
	}
	for _, id := range order {
		delete(remaining, id)
	}
	cycles := stronglyConnected(remaining, edges)

	// The cyclic remainder is ordered the same way, deliberately. It is still
	// next-up's list and section 39's tie-break does not stop applying because
	// the graph has a cycle - a reader looking at stuck work wants the
	// important stuck work first, exactly as above.
	rest := make([]string, 0, len(remaining))
	for id := range remaining {
		rest = append(rest, id)
	}
	sortByPriority(rest, active)
	return append(order, rest...), cycles
}

// sortNotes puts the important notes first: priority, then recency, then id.
//
// ⛔ ONE SORT FOR BOTH NOTE LISTS, AND THAT IS THE POINT OF IT BEING A
// FUNCTION. Section 39 states this ordering for a case's attention_n notes
// (row 11) and states nothing for a project's notes (row 3); using it for both
// is this seat's choice, taken with the lead, because two adjacent note lists
// ordering differently is the drift the one-rank ruling exists to stop. A
// reader who learns the order from one list must not be wrong about the other.
//
// "importance leads, recency is the tie-break" is section 39's own phrasing.
// id comes last so two notes written in the same millisecond still order the
// same way twice - the clock is millisecond-resolution and tests freeze it, so
// that tie is real rather than theoretical. It is the argument lateststeps.go
// makes at length about MAX(id).
func sortNotes(notes []Note) {
	sort.SliceStable(notes, func(i, j int) bool {
		a, b := notes[i], notes[j]
		if ra, rb := priorityRank(a.Priority), priorityRank(b.Priority); ra != rb {
			return ra < rb
		}
		if !a.Prov.CreatedAt.Equal(b.Prov.CreatedAt) {
			return a.Prov.CreatedAt.After(b.Prov.CreatedAt)
		}
		return a.ID < b.ID
	})
}

// caseNotes answers section 11: up to attention_n notes part-of this case.
//
// ⛔ IT REUSES NotesAbout RATHER THAN WRITING A SECOND QUERY, and the subject
// set is the case itself. Section 39 is explicit that row 11 needs NO new verb
// and no new kind - "a note part-of a case IS the mechanism" - and the same
// argument reaches the read side: a second query over the same two tables is a
// second place for the part-of scoping to be got wrong.
//
// ⛔ NEVER PADDED TO attention_n. Section 39 uses that phrase for next_up_n and
// it binds here for the same reason: a list padded to its cap tells a reader
// there are exactly that many, and a cap is a limit on what is SHOWN rather
// than a claim about what EXISTS.
func caseNotes(ctx context.Context, src BriefStore, container Record) ([]Note, error) {
	notes, err := src.NotesAbout(ctx, container.ID, map[string]bool{container.ID: true})
	if err != nil {
		return nil, err
	}
	if n := capFrom(container, "attention_n", DefaultAttentionN); len(notes) > n {
		notes = notes[:n]
	}
	return notes, nil
}

// governing answers section 12: every decision, requirement and artefact in
// this project, and how many of each.
//
// ⛔ ONE QUERY PER KIND RATHER THAN ONE UNFILTERED QUERY FILTERED IN Go, AND
// THAT IS DELIBERATE. Query takes an exact kind, so asking for everything and
// discarding what does not match would read the whole project - every
// work-item, every progress step - to return three kinds. On rig's own store
// that is 85 rows read to answer about zero. The loop is over a CLOSED
// three-element vocabulary, so it is three statements and not an unbounded fan.
//
// ⛔ AND IT IS WHY THE COUNTS ARE FREE. Each query already returns the whole set
// for its kind, so the count is len() of something already in hand - there is
// no second read, and no opportunity for the list and the count to be answers
// about two different moments.
func governing(ctx context.Context, src BriefStore, project string) ([]GoverningRecord, []KindCount, error) {
	var rows []GoverningRecord
	var counts []KindCount

	for _, kind := range governingKinds {
		recs, err := src.Query(ctx, project, kind)
		if err != nil {
			return nil, nil, err
		}
		// ⛔ NO ROW FOR A KIND WITH NOTHING IN IT. A zero is a different claim
		// from an absence: "0 decisions" asserts the project was asked and has
		// none, which is exactly what section 12's COMPUTED state already says
		// for the whole section. Rendering both says it twice and invites a
		// reader to wonder which one is load-bearing.
		if len(recs) == 0 {
			continue
		}
		counts = append(counts, KindCount{Kind: kind, Count: uint64(len(recs))})
		for _, r := range recs {
			rows = append(rows, GoverningRecord{
				ID: r.ID, Kind: r.Kind, Title: r.Fields["title"],
			})
		}
	}

	// Query orders by project, kind and id, so the rows within one kind arrive
	// sorted and the kinds arrive in the loop's order. Nothing more is needed
	// and a re-sort here would be a second ordering rule nobody could see.
	return rows, counts, nil
}

// features answers section 10: the features at stage `building`, and the count
// at every stage.
//
// BOTH FROM ONE READ. The counts cover every feature and the list covers one
// stage, so deriving the counts from the list would report a project with three
// shipped features as having none. They are two answers about the same rows and
// this reads those rows once.
func features(ctx context.Context, src BriefStore, project string) ([]Feature, []StageCount, error) {
	recs, err := src.Query(ctx, project, KindFeature)
	if err != nil {
		return nil, nil, err
	}

	var building []Feature
	counts := map[string]uint64{}
	for _, r := range recs {
		stage := r.Fields["stage"]
		counts[stage]++
		if stage == StageBuilding {
			building = append(building, Feature{
				ID: r.ID, Title: r.Fields["title"], Stage: stage,
			})
		}
	}

	// Lifecycle order first, then anything unrecognised by name. A stage the
	// vocabulary does not know is REPORTED rather than dropped: the brief
	// answers for the store as it is, and a typo in a stage field should be
	// visible in the one place somebody is looking.
	seen := map[string]bool{}
	var stages []StageCount
	for _, st := range featureStages {
		if n, ok := counts[st]; ok {
			stages = append(stages, StageCount{Stage: st, Count: n})
		}
		seen[st] = true
	}
	var rest []string
	for st := range counts {
		if !seen[st] {
			rest = append(rest, st)
		}
	}
	sort.Strings(rest)
	for _, st := range rest {
		stages = append(stages, StageCount{Stage: st, Count: counts[st]})
	}
	return building, stages, nil
}
