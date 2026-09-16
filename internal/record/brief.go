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

	// Kind is the container's own kind, `project` or `case`, and it decides
	// which sections mean anything. Empty when the container has no record.
	Kind string
}

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
	// coarseCitations already reads it with ->>, so this costs no schema change.
	//
	// A VALUE THAT IS NOT A JSON ARRAY YIELDS NO TAGS RATHER THAN AN ERROR. A
	// brief is a read of whatever is in the store, and one malformed field on
	// one record must not refuse the whole answer.
	Tags []string

	// TargetDate is row 2's optional date, carried as the string that was
	// stored. Section 39 names the field and no format; parsing it here would
	// invent one and refuse every record that disagreed.
	TargetDate string

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
		Tags:             decodeTags(it.Fields["tags"]),
		TargetDate:       it.Fields["target_date"],
		Semver:           it.Fields["semver"],
	}
}

// decodeTags reads the JSON array a tags field holds.
//
// ⛔ A MALFORMED VALUE IS NO TAGS, NEVER AN ERROR, and that is deliberate
// rather than lazy. The alternative is a brief that refuses to answer because
// one record out of forty-five has a hand-written tags field - which would make
// the whole derivation hostage to the worst row in the store, in a system whose
// entire argument is that it answers from what is actually there.
func decodeTags(raw string) []string {
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
// matches and read as items with no tags. Pairs with decodeTags.
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
	ID       string
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

// Brief derives the answer to "what is going on here" for one project.
func (s *Store) Brief(ctx context.Context, project string) (Brief, error) {
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
	container, cerr := s.Get(ctx, project)
	if cerr != nil && !errors.As(cerr, new(*NotFoundError)) {
		return Brief{}, cerr
	}
	if cerr == nil {
		b.Kind = container.Kind
	}
	led := newSectionLedger(b.Kind)

	items, err := s.Query(ctx, project, "work-item")
	if err != nil {
		return Brief{}, err
	}
	latest, err := s.latestSteps(ctx, project)
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
	noted, err := s.itemsWithANote(ctx, project)
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

	edges, err := s.blocksAmong(ctx, active)
	if err != nil {
		return Brief{}, err
	}

	order, cycles := topoSort(active, edges)
	b.Cycles = cycles

	blockedBy, err := s.blockedBy(ctx, items, latest, active)
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
		if len(b.NextUp) < n && len(blockedBy[id]) == 0 {
			b.NextUp = append(b.NextUp, active[id])
			continue
		}
		b.Open = append(b.Open, active[id])
	}

	for id, on := range blockedBy {
		sort.Strings(on)
		b.Blocked = append(b.Blocked, Blockage{Item: id, Title: active[id].Title, BlockedBy: on})
	}
	sort.Slice(b.Blocked, func(i, j int) bool { return b.Blocked[i].Item < b.Blocked[j].Item })

	if b.CoarseCitations, err = s.coarseCitations(ctx, project); err != nil {
		return Brief{}, err
	}

	// SECTION 3. The subjects are the project itself and every item in the OPEN
	// list - never the next-up ones, which carry only HasNote.
	subjects := map[string]bool{project: true}
	for _, it := range b.Open {
		subjects[it.ID] = true
	}
	if b.Notes, err = s.notesAbout(ctx, project, subjects); err != nil {
		return Brief{}, err
	}
	led.did(SectionNotes)

	// SECTION 10.
	if b.Features, b.Stages, err = s.features(ctx, project); err != nil {
		return Brief{}, err
	}
	led.did(SectionFeatures)

	// SECTION 11, and only for a case. A project's notes are section 3 above;
	// this is the case's own attention list, capped and ordered by importance.
	if b.Kind == KindCase {
		if b.CaseNotes, err = s.caseNotes(ctx, container); err != nil {
			return Brief{}, err
		}
		led.did(SectionCaseNotes)
	}

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
// `edges` is blocksAmong(active) with BOTH ends filtered.
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
func (s *Store) blockedBy(ctx context.Context, items []Record,
	latest map[string]Record, active map[string]ItemState,
) (map[string][]string, error) {
	blockedBy := map[string][]string{}
	for _, it := range items {
		if step, ok := latest[it.ID]; ok && step.Fields["state"] == "done" {
			continue
		}
		dsts, err := s.LinksFrom(ctx, it.ID, LinkBlocks)
		if err != nil {
			return nil, err
		}
		for _, d := range dsts {
			if _, ok := active[d]; ok {
				blockedBy[d] = append(blockedBy[d], it.ID)
			}
		}
	}
	return blockedBy, nil
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

// blocksAmong returns the blocks edges with BOTH ends in the active set.
//
// Restricted deliberately: an edge from a done item is not a live dependency,
// and carrying it would put finished work back into the ordering.
func (s *Store) blocksAmong(ctx context.Context, active map[string]ItemState) (map[string][]string, error) {
	out := map[string][]string{}
	for id := range active {
		dsts, err := s.LinksFrom(ctx, id, LinkBlocks)
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

// stronglyConnected finds every cycle among the given nodes, by Tarjan.
//
// A COMPONENT OF ONE IS NOT A CYCLE and is dropped: those are the items merely
// DOWNSTREAM of a cycle, which are stuck but are not themselves the problem.
// Naming them in the cycle report would send a reader to the wrong edge. A
// self-loop cannot appear here because Link refuses one.
func stronglyConnected(nodes map[string]bool, edges map[string][]string) [][]string {
	var (
		index   = map[string]int{}
		low     = map[string]int{}
		onStack = map[string]bool{}
		stack   []string
		next    int
		out     [][]string
		strong  func(string)
		ordered = make([]string, 0, len(nodes))
	)
	for id := range nodes {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)

	strong = func(v string) {
		index[v] = next
		low[v] = next
		next++
		stack = append(stack, v)
		onStack[v] = true

		succ := append([]string(nil), edges[v]...)
		sort.Strings(succ)
		for _, w := range succ {
			if !nodes[w] {
				continue
			}
			if _, seen := index[w]; !seen {
				strong(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}

		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			if len(comp) > 1 {
				sort.Strings(comp)
				out = append(out, comp)
			}
		}
	}

	for _, v := range ordered {
		if _, seen := index[v]; !seen {
			strong(v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// notesAbout collects section 3's notes: every note part-of one of `subjects`.
//
// ONE QUERY FOR EVERY NOTE IN THE PROJECT, FILTERED IN GO. The alternative is
// an IN clause built from the subject set, which is a query whose text changes
// with the data - unprepareable, and a different plan on every call. The set is
// the project plus its open items, so the difference is a handful of rows.
//
// ⛔ SCOPED ON THE DESTINATION, like the has-note flag, because section 39 rules
// that links MAY cross a project boundary. A note written elsewhere and attached
// here is exactly the question this section exists to surface.
//
// ⛔ ORDERED BY PRIORITY, THEN created_at DESCENDING, THEN id - AND THIS
// COMMENT USED TO NAME A MECHANISM THE QUERY DID NOT HAVE. It read "ordered by
// priority then id" above an ORDER BY n.id, wrong from the first draft rather
// than drifted, and no test ever watched it: ordering by id alone IS
// deterministic, so every assertion here passed against a caption describing a
// sort that was not happening.
//
// Section 39 binds neither order here - row 3 says a note is rendered in full
// and says nothing about sequence - so this one is the seat's, taken with the
// lead. It is deliberately the SAME sort a case's attention_n notes get eleven
// rows later in section 39, because two adjacent note lists ordering
// differently is precisely the drift the one-definition rule exists to stop.
//
// ⛔ AND THE SORT RUNS HERE RATHER THAN IN THE ORDER BY, WHICH IS THE ONE THING
// THE RANK CANNOT DELEGATE. SQLite can only reach the raw string out of the
// fields blob, so a SQL ordering is alphabetical whichever way it is pointed.
// The only SQL form that honours the rank is a CASE WHEN, and that puts a
// second copy of the vocabulary in a string literal no Go test can reach -
// which is the drift the ruling names, arriving through the door opened to
// implement it. The query keeps ORDER BY n.id for a stable read.
func (s *Store) notesAbout(ctx context.Context, project string, subjects map[string]bool) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.body, n.fields, l.dst,
		       n.session, n.seat, n.epoch, n.created_at
		FROM links l
		JOIN records n ON n.id = l.src
		JOIN heads hn ON hn.id = n.id AND hn.version = n.version
		JOIN records d ON d.id = l.dst
		JOIN heads hd ON hd.id = d.id AND hd.version = d.version
		WHERE l.type = ? AND n.kind = ? AND d.project = ?
		ORDER BY n.id`, LinkPartOf, KindNote, project)
	if err != nil {
		return nil, fmt.Errorf("record: reading the notes in %s: %w", project, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Note
	for rows.Next() {
		var (
			n           Note
			fields      string
			epoch, nano int64
		)
		if err := rows.Scan(&n.ID, &n.Body, &fields, &n.About,
			&n.Prov.Session, &n.Prov.Seat, &epoch, &nano); err != nil {
			return nil, err
		}
		if !subjects[n.About] {
			continue
		}
		if n.Prov.Epoch, err = fromColumn("epoch", epoch); err != nil {
			return nil, err
		}
		n.Prov.CreatedAt = unixNano(nano)
		var f map[string]string
		if err := json.Unmarshal([]byte(fields), &f); err == nil {
			n.Priority = f["priority"]
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortNotes(out)
	return out, nil
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
// ⛔ IT REUSES notesAbout RATHER THAN WRITING A SECOND QUERY, and the subject
// set is the case itself. Section 39 is explicit that row 11 needs NO new verb
// and no new kind - "a note part-of a case IS the mechanism" - and the same
// argument reaches the read side: a second query over the same two tables is a
// second place for the part-of scoping to be got wrong.
//
// ⛔ NEVER PADDED TO attention_n. Section 39 uses that phrase for next_up_n and
// it binds here for the same reason: a list padded to its cap tells a reader
// there are exactly that many, and a cap is a limit on what is SHOWN rather
// than a claim about what EXISTS.
func (s *Store) caseNotes(ctx context.Context, container Record) ([]Note, error) {
	notes, err := s.notesAbout(ctx, container.ID, map[string]bool{container.ID: true})
	if err != nil {
		return nil, err
	}
	if n := capFrom(container, "attention_n", DefaultAttentionN); len(notes) > n {
		notes = notes[:n]
	}
	return notes, nil
}

// features answers section 10: the features at stage `building`, and the count
// at every stage.
//
// BOTH FROM ONE READ. The counts cover every feature and the list covers one
// stage, so deriving the counts from the list would report a project with three
// shipped features as having none. They are two answers about the same rows and
// this reads those rows once.
func (s *Store) features(ctx context.Context, project string) ([]Feature, []StageCount, error) {
	recs, err := s.Query(ctx, project, KindFeature)
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
