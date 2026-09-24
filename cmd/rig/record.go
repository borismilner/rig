package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/user"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/boris-milner/rig/client"
	// ALIASED, AND NOT BY PREFERENCE. `wire` is already a package-level
	// identifier here - main.go's `wire = "v1"`, the wire VERSION this build
	// speaks - and Go refuses an import whose name collides with one, in any
	// file of the package. The alias follows rigv1's spelling below so the
	// two rig packages read as a pair.
	rigwire "github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The continuity record at a prompt (PLAN.md section 39).
//
// THE GROUPING IS SECTION 39's AND IT IS A RULE RATHER THAN A LAYOUT: "ON THE
// CLI: rig record, rig progress, rig standard, rig brief. Four commands,
// because a surface an agent has to learn is a surface it gets wrong." So the
// seven read/write verbs hang off one word rather than becoming seven
// top-level verbs, and `rig progress` and `rig brief` are their own because
// they answer different questions - a stream and a derivation, not a record.
//
// ⛔ THREE OF SECTION 39's ELEVEN VERBS ARE DELIBERATELY ABSENT, AND THIS
// PARAGRAPH IS HERE SO THE NEXT READER DOES NOT "COMPLETE" THE SET.
//
//	standard.stamp   slice 6
//	standard.drift   slice 6
//	project.gate     slice 7
//
// Boris defined the MVP on 2026-09-16 as "being able to use rig to work on rig
// with respect to the project/case management", and section 39 ranks that as
// slices 1, 2 and 4 - the store and the nouns, progress and the brief, links
// and refs. The standards register and the read-before-write gate are neither,
// so a `rig standard` verb here would be a surface promising a capability the
// daemon does not have. Adding them is a change to section 39's build order,
// which is the lead's, not a gap in this file.
//
// WHERE THE WIRE IS. Everything down to the seam goes through RecordAPI and
// knows no protobuf exists; `wireRecord` at the BOTTOM of this file is the
// implementation against proto/, and it is the only part that does. That
// split is what let the CLI and the wire be built at the same time by two
// seats, and it is why every renderer, refusal and argument check above is
// exercised by a fake rather than by a daemon.

// ---- the contract with rigd ------------------------------------------------

// RecordAPI is everything the CLI needs from rigd for section 39's verbs.
//
// THE INTERFACE IS THE CONTRACT THAT LET TWO SEATS BUILD ONE CAPABILITY AT
// ONCE, and it outlived the reason it was written. It was agreed before either
// half existed, so the CLI did not wait on proto/rig/v1 - the lead's SECOND
// COLLISION in COORDINATION.md, a file this seat may not open. The wire has
// landed and `wireRecord` at the bottom of this file implements it.
//
// It is deliberately NOT a *client.Client wrapper, and that is the half worth
// keeping now the wire exists. A CLI built against the concrete client is a
// CLI whose every verb needs a live daemon to test, and cmd/rig already
// documents the fifteen functions in that hole. Against an interface, every
// renderer and every refusal below is exercised by a fake.
type RecordAPI interface {
	// Put creates a record or supersedes one, and returns what was written.
	Put(ctx context.Context, a PutArgs) (Record, error)

	// Get returns one record. VERSION 0 MEANS HEAD - it is not "version
	// zero", which no record has, because the first write is version 1.
	Get(ctx context.Context, id string, version uint64) (Record, error)

	// Query returns the head of every record matching the filters. This is
	// section 39's "indexed".
	Query(ctx context.Context, a QueryArgs) ([]Record, error)

	// History returns every version of one record, oldest first, with its
	// provenance.
	History(ctx context.Context, id string) ([]Record, error)

	// Link and Unlink write and remove a typed, directed edge. This is
	// section 39's "linked".
	Link(ctx context.Context, src, linkType, dst string) error
	Unlink(ctx context.Context, src, linkType, dst string) error

	// Retract, Delete and Replace are B77's three, and they are THREE
	// CAPABILITIES rather than three names for one. Boris's distinguishing
	// questions: does the id survive (retract yes, delete no), and does the
	// history survive (retract yes, delete no)?
	Retract(ctx context.Context, id, reason string) (Retraction, bool, error)
	Delete(ctx context.Context, id string, dryRun bool) (Deletion, error)
	Replace(ctx context.Context, old, replacement, reason string) (Replacement, error)

	// Refs is what points AT a record: the direction files cannot go, and
	// section 39's "correlated".
	//
	// IT TAKES A STRUCT, on Put's and Step's precedent. It was
	// `Refs(ctx, id string, depth int)` and `cross_project` arriving on the
	// wire would have made it a third positional bool - the parameter shape
	// where a caller swaps two arguments and the compiler says nothing.
	Refs(ctx context.Context, a RefsArgs) (Refs, error)

	// Step appends one step to a work item's stream and returns the step.
	//
	// IT TAKES A STRUCT AND RETURNS THE RECORD, and both halves were a
	// correction. The first shape of this contract was
	// `Step(ctx, item, note string) error`, which cannot drive
	// internal/record's own Step: that one refuses an empty state and an
	// empty project by name, and section 39 says a step IS "the work item,
	// one line of what, a state (started, blocked, done)". Returning only an
	// error also left this surface unable to echo what it wrote.
	Step(ctx context.Context, a StepArgs) (Record, error)

	// Brief is the derived answer to "what is going on here", over a project
	// or a case.
	Brief(ctx context.Context, project string) (Brief, error)
}

// Provenance is who wrote a version, when, and under which daemon.
//
// It mirrors internal/record.Provenance field for field. NONE OF IT IS
// OPTIONAL there, and the renderers below treat an empty one as a DEFECT to
// report rather than a value to print, for the reason section 21 gives about
// every zero on this wire: nothing was said is never a fact about anything.
type Provenance struct {
	Session   string
	Seat      string
	Epoch     uint64
	CreatedAt time.Time
}

// Record is one version of one record, as this client sees it.
//
// The field names are internal/record.Record's, read off that file rather than
// guessed, so a reader holding both open is not translating between two
// vocabularies for one noun. `Prov` is spelled as it is spelled there.
type Record struct {
	ID      string
	Version uint64
	Kind    string
	Project string
	Body    string
	Fields  map[string]string
	Prov    Provenance

	// Retraction is set when this record has been WITHDRAWN, and nil
	// otherwise. B77, ruled by Boris 2026-09-17.
	//
	// ⛔ ONLY `get` EVER CARRIES IT, AND THAT IS THE CONTRACT. A retracted
	// record leaves every brief and every query, so a Record that came out of
	// `query` could not be retracted and a renderer there has nothing to check.
	// `get` is the one verb that still answers about a withdrawn record, and it
	// has to say so or retract is invisible to every reader.
	Retraction *Retraction
}

// Retraction is a record withdrawn: what, why, by whom, and what replaced it.
type Retraction struct {
	ID     string
	Reason string

	// ReplacedBy is the survivor when a replace did the withdrawing, and empty
	// for a plain retract. It answers the question a reader HOLDING THE OLD ID
	// actually has, which is where the fact went rather than that it left.
	ReplacedBy string

	Prov Provenance
}

// Edge is one typed, directed link, as a value a report carries.
type Edge struct {
	Src  string
	Type string
	Dst  string
}

// String is the arrow form `rig record link` already teaches.
func (e Edge) String() string { return e.Src + " --" + e.Type + "--> " + e.Dst }

// Deletion is what a delete took. ⛔ THE ACCOUNT IS THE OUTPUT: Boris ruled
// that delete drops the edges rather than refusing, so the obligation moved
// from the verb to the report, and a verb that answers "deleted" and nothing
// else is B75's shape arriving through a verb whose job is to lose data.
type Deletion struct {
	ID       string
	Versions uint64
	Edges    []Edge
	DryRun   bool
}

// Replacement accounts for every inbound edge of the loser in one of three
// disjoint buckets, and carries the withdrawal that finished the job.
type Replacement struct {
	Old        string
	New        string
	Moved      []Edge
	Merged     []Edge
	Dropped    []Edge
	Retraction *Retraction
}

// PutArgs creates a record or supersedes one. It mirrors
// internal/record.PutRequest.
// QueryArgs is section 39's three filters for `record.query`: "by kind, field
// and project". EVERY ONE IS OPTIONAL.
//
// ⛔ IT IS A STRUCT RATHER THAN FOUR STRINGS FOR THE REASON RefsArgs ALREADY
// RECORDS: adjacent parameters of the same type can be swapped with no
// compiler complaint, and a swapped project and kind returns zero rows, which
// reads as "the store holds none of those" rather than as a typo. With a field
// and a value beside them that is four swappable strings, and two of them are
// arbitrary user text.
type QueryArgs struct {
	// Project and Kind narrow by container and by what the record IS.
	//
	// ⛔ EMPTY MEANS *EVERY* ONE, NOT "THE EMPTY ONE". A record cannot have
	// either empty - the store refuses both by name - so there is no value
	// the empty string could collide with. Both were once REQUIRED here, and
	// since `kind` is not a closed set, that made every census of the store
	// incomplete by construction: a record under an unguessed kind was
	// invisible to every question anybody could write.
	Project string
	Kind    string

	// Field and Value are the third filter, missing until BACKLOG.md B65.
	//
	// ⛔ AND THE EMPTY RULE ABOVE DOES NOT EXTEND TO Value. An empty Field
	// means no predicate; an empty Value means THE EMPTY STRING, because a
	// record may legitimately carry one and asking for it is a real question.
	// A Value with no Field is refused by the store rather than dropped,
	// because dropping it widens the answer silently.
	Field string
	Value string
}

type PutArgs struct {
	// ID is the record to write. Empty means the store mints one - UUIDv7,
	// section 39's id scheme - except for a `project` or a `case`, whose id
	// IS its slug and must be supplied.
	//
	// THAT EXCEPTION IS NOT RE-STATED AS A CHECK HERE, deliberately.
	// internal/record.generateID already refuses a slug-kinded put with no
	// id, in a sentence naming the kind, and a second copy of the rule in
	// this file is a second thing to keep in step with section 39.
	ID string

	// IfVersion is the version the caller believes is current.
	//
	// ⛔ ZERO MEANS CREATE. That is what makes the zero value dangerous at a
	// prompt: a caller who forgot the flag is indistinguishable from one
	// asking to create, so this surface refuses to send a zero it was not
	// told to send. recordPut is where that is enforced.
	IfVersion uint64

	Kind    string
	Project string
	Body    string
	Fields  map[string]string

	// ⛔ THERE IS NO Session, Seat OR Epoch HERE, AND THEIR ABSENCE IS THE
	// POINT. RULED 2026-09-16 and carried into the wire itself, so a request
	// message cannot express provenance at all.
	//
	// internal/record.PutRequest carries all three, and this struct
	// deliberately does NOT mirror it there: that one is the DAEMON's
	// interface to the store, and the daemon is where the three come from.
	// A client that can set its own session and seat can write a record
	// attributed to another seat - a forgery surface, on the one field
	// section 39 makes load-bearing. Section 39: "provenance timestamps are
	// the daemon's, never the client's"; the connection already knows all
	// three.
	//
	// So they are not optional on the client. They are absent, and there is
	// nothing here to ignore or to forge.
}

// StepArgs appends one step to a work item's stream.
//
// It mirrors internal/record.StepRequest MINUS its provenance, for the reason
// PutArgs states at length: the daemon stamps who and when, and a client that
// could name a seat could name somebody else's.
type StepArgs struct {
	// Item is the record id of the work item this step is about.
	Item string

	// State is started, blocked or done.
	//
	// ⛔ THIS CLIENT DOES NOT VALIDATE IT AND MUST NOT START. rigd refuses an
	// unknown state by name, quoting the value back, and a second copy of the
	// set here is a second thing to keep in step with section 39 - two
	// validators drift, one does not. The --state flag's help text names the
	// three because a caller has to be able to discover them; the REFUSAL is
	// the daemon's alone.
	State string

	// Note is what happened, in the seat's own words. Optional: a step with a
	// state and no note is still the signal that something moved.
	Note string

	// Project is the project or case the item belongs to.
	Project string
}

// RefsArgs asks what points AT a record. It mirrors
// internal/record.RefsRequest, minus the fields the daemon derives.
type RefsArgs struct {
	// ID is the record being asked about.
	ID string

	// Depth is how many hops to follow backwards.
	//
	// ⛔ ZERO MEANS THE DAEMON'S DEFAULT AND IS NOT "no hops". The store reads
	// it that way (internal/record.DefaultRefsDepth), the wire carries it
	// through untouched, and the answer reports the depth it actually served -
	// which is the only thing that makes a zero readable at the other end.
	// recordRefs is where a TYPED zero is refused, because only flag.Visit can
	// tell a typed one from an omitted one.
	Depth int

	// CrossProject opts in to leaving the record's own project.
	//
	// Section 39 makes the project predicate a performance rule - it prunes
	// the frontier at every hop rather than filtering the same work - and says
	// crossing is "asked for, never arrived at". So it is false by default and
	// there is nothing here that could cross by accident.
	CrossProject bool
}

// Ref is one edge, as `record.refs` reports it.
//
// ⛔ THE PROVISIONAL SHAPE IS GONE AND THIS ONE IS THE WIRE'S. The comment
// here used to say "PROVISIONAL UNTIL SLICE 2 ... when the wire lands, the
// lead's message is the contract and these follow it". `rigv1.Ref` landed
// carrying src, type, distance, kind, title and via, and this is that promise
// kept: field for field, with the wire's own words.
//
// ⛔ `Dst` IS DELETED RATHER THAN RENAMED, AND THE DIFFERENCE MATTERS. There
// is no `dst` on the wire and there never was one in the store either: `Via`
// IS the other end of this edge - internal/record/refs.go spells it "the
// record this one points at - the subject at depth 1". A field called Dst
// filled from Via would be right at distance 1 and quietly wrong past it,
// which is the worst available shape for a field a renderer prints.
type Ref struct {
	// Src and Type are this edge's near end and its label. The edge is
	// DIRECTED and both ends are printed, because "what points at this" is
	// only readable if the reader can see which end they are looking at.
	Src  string
	Type string

	// Via is the record this edge arrives at: the subject itself at distance
	// 1, and the record it was reached THROUGH beyond that.
	//
	// ⛔ IT IS WHAT MAKES A DISTANCE-3 ROW MEAN ANYTHING. A distance with no
	// via says how far and not through what, so the relationship the caller
	// asked about is unreadable exactly where the verb stops being obvious.
	Via string

	// Kind is the kind of the OTHER end, so a reader can tell a decision
	// citing a requirement from a work-item implementing one without a second
	// call per row.
	Kind string

	// Title is that record's own one line.
	//
	// ⛔ IT IS WHY THIS VERB IS ONE CALL RATHER THAN ONE PLUS N. The store
	// computes it on the way past; without it a reader holding a column of
	// UUIDv7s has to `record.get` every row to learn what any of them are,
	// which is section 9's context budget paying for a field that was already
	// in hand.
	Title string

	// Distance is how many hops this edge is from the record that was asked
	// about. 1 is a direct reference.
	//
	// ⛔ IT IS `Distance` AND NOT `Depth`, WHICH IS THE WIRE'S OWN CHOICE AND
	// IS LOAD-BEARING. `Refs.Depth` below is how far the walk LOOKED; this is
	// how far one edge IS. One word for two numbers in one answer is the
	// collapse this file spends paragraphs preventing in enums, and it would
	// arrive here through a struct field instead.
	Distance int
}

// Refs is the answer to "what points AT this record".
type Refs struct {
	// ID is the record that was asked about, echoed so an answer read out of
	// context still says what it is about.
	ID string

	// Depth is the depth that was ANSWERED, which is not always the depth
	// that was asked. It rides here for the reason appsJSON spends a
	// paragraph on the same field: a reader who did not type the flag cannot
	// otherwise tell a cheap question from an empty answer.
	Depth int

	// In are the edges pointing at ID. Empty is an ANSWER - nothing cites it
	// - and never a failure.
	In []Ref

	// Truncated is true when the walk hit its bound, so this answer is
	// PARTIAL.
	//
	// ⛔ DROPPING IT IS THE DEFECT THE FLAG EXISTS TO PREVENT, AND IT IS THE
	// ONE THIS REPOSITORY KEEPS CATCHING: a short list that looks complete.
	// The wire carries it, the store computes it, and a client that reads
	// neither reports "these are the edges" over "these are some of the
	// edges".
	Truncated bool

	// Cycles are the cycles among the records the walk reached, each naming
	// its items.
	//
	// ⛔ DETECTED, REPORTED, ORDERED AROUND, NEVER RESOLVED. Section 39 rules
	// it and the ruling reaches this client rather than stopping at the
	// store: rig does not pick an edge to break, because choosing which one
	// is wrong is a judgement about the work. So this holds the ITEMS and
	// nothing that could be read as a recommendation.
	Cycles [][]string
}

// ---- the seam --------------------------------------------------------------

// recordAPI opens the record surface and returns the function that releases
// it.
//
// A package-level var for two reasons and both are load-bearing. The lead
// replaces the one function below with the wire client, which is a one-line
// change to a file this seat owns rather than a merge across two. And every
// test in record_test.go, progress_test.go and brief_test.go swaps it for a
// fake, which is the only way this surface is testable at all before the wire
// exists.
var recordAPI = openRecordAPI

// openRecordAPI is the default, and it DIALS.
//
// ⛔ IT USED TO REFUSE, AND THE REFUSAL IS DELETED RATHER THAN NARROWED.
// `notWired()` said "rigd serves nothing to call" and enumerated all nine
// verbs as its precondition; rigd dispatches all nine (internal/daemon's
// record.go, declared in self.go, routed in daemon.go), so every clause of it
// is now false. A refusal that has stopped being true is worse than no
// refusal: it is a sentence a reader believes.
//
// THE FAILURE IT USED TO GUARD AGAINST IS NOW THE TRUE ONE. Reporting "is
// rigd running?" was wrong while the verbs were missing from a daemon that
// was running perfectly. With them served, a dial that fails IS a daemon that
// is not there, which is exactly what noDaemon says.
func openRecordAPI() (RecordAPI, func(), error) {
	c, err := connect()
	if err != nil {
		return nil, nil, noDaemon(err)
	}
	return wireRecord{c: c}, func() { _ = c.Close() }, nil
}

// ---- the flags -------------------------------------------------------------

// titleKey is section 39's spelling of the one noun several surfaces here
// echo, and it is ONE constant because it is ONE name.
//
// The work-item metadata table names `title` as a typed field - "one line" -
// and every rendering below is that field arriving by some route: a record's
// own `fields["title"]`, a ref's resolved title, a brief item's, a blocker's.
// They reach this package through three different messages and they are not
// three different words, which is what a second spelling here would assert.
const titleKey = "title"

// fieldFlag collects `--field key=value`, repeated.
//
// A REPEATED FLAG RATHER THAN ONE JSON BLOB, because section 39's whole
// argument for typed fields is that an agent reaches one by name rather than
// parsing prose, and `--fields '{"a":"b"}'` puts a parser back in front of the
// thing the parser was removed from. The blob form is what --body is for, and
// --body is one value.
//
// A DUPLICATE KEY IS REFUSED RATHER THAN OVERWRITTEN. Last-one-wins is the
// usual choice and it is silent: a caller who typed --field state=done twice
// with different values gets one of them and is never told which.
type fieldFlag struct {
	m     map[string]string
	order []string
}

// String is called by flag on a zero Value while printing defaults, so it must
// survive a nil receiver.
func (f *fieldFlag) String() string {
	if f == nil || len(f.order) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(f.order))
	for _, k := range f.order {
		pairs = append(pairs, k+"="+f.m[k])
	}
	return strings.Join(pairs, ",")
}

func (f *fieldFlag) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return fmt.Errorf("%q is not key=value: a typed field needs a name", s)
	}
	if f.m == nil {
		f.m = map[string]string{}
	}
	if old, dup := f.m[k]; dup {
		return fmt.Errorf("--field %s was given twice, as %q and %q: rig will "+
			"not pick one for you", k, old, v)
	}
	f.m[k] = v
	f.order = append(f.order, k)
	return nil
}

// values is what goes on the wire: nil when nothing was given, so a put that
// set no fields is distinguishable from one that set none on purpose.
func (f *fieldFlag) values() map[string]string {
	if f == nil || len(f.m) == 0 {
		return nil
	}
	return f.m
}

// recordFlags is one subcommand's flag set and everything it parsed.
//
// ONE SET PER SUBCOMMAND, CARRYING ONLY THE FLAGS THAT SUBCOMMAND TAKES, so
// `rig record get --kind requirement` is REFUSED by flag itself rather than
// silently ignored. A flag that is accepted and does nothing is the worst of
// the three answers: the caller believes it asked for something.
type recordFlags struct {
	fs      *flag.FlagSet
	asJSON  *bool
	timeout *time.Duration

	id           *string
	kind         *string
	project      *string
	body         *string
	bodyFile     *string
	fields       *fieldFlag
	field        *string
	value        *string
	ifVersion    *uint64
	version      *uint64
	depth        *int
	crossProject *bool
	reason       *string
	dryRun       *bool
}

// recordFlagSet builds the set for one subcommand, or the bare set for a word
// that is not one.
//
// THE UNKNOWN SUBCOMMAND STILL GETS --json, and that is the reason this
// function takes the name rather than each subcommand building its own set
// inline. Section 10 promises --json on every error; a dispatcher that refuses
// before any flag is parsed prints prose for the one caller most likely to be
// a script.
func recordFlagSet(sub string) *recordFlags {
	r := &recordFlags{fs: flag.NewFlagSet("record "+sub, flag.ContinueOnError)}
	r.asJSON = r.fs.Bool("json", false, "emit JSON")
	r.timeout = r.fs.Duration("timeout", defaultCallTimeout, "how long to wait")

	switch sub {
	case "put":
		r.id = r.fs.String("id", "", "the record to write; empty mints one")
		r.kind = r.fs.String("kind", "", "what the record is")
		r.project = r.fs.String("project", "", "the project or case it belongs to")
		r.body = r.fs.String("body", "", "the record's prose")
		r.bodyFile = r.fs.String("body-file", "",
			"a file to read the record's prose from; - is standard input")
		r.fields = &fieldFlag{}
		r.fs.Var(r.fields, "field", "a typed field, key=value; repeatable")
		r.ifVersion = r.fs.Uint64("if-version", 0,
			"the version you believe is current; 0 creates")
	case "get":
		r.version = r.fs.Uint64("version", 0, "a version to read; 0 is the head")
	case "retract", "replace":
		r.reason = r.fs.String("reason", "",
			"why it was withdrawn; empty is a real answer and is recorded as one")
	case "delete":
		// ⛔ THE DRY RUN IS THE SAFETY THIS VERB HAS INSTEAD OF A REFUSAL.
		// Boris ruled delete drops the edges rather than refusing while
		// anything cites the record, and ruled in the same turn that a dry run
		// must be able to show them first. This is that flag.
		r.dryRun = r.fs.Bool("dry-run", false,
			"print exactly what would be removed and write nothing")
	case "query":
		// ⛔ BOTH FILTERS ARE OPTIONAL AND AN ABSENT ONE MEANS *EVERY* VALUE.
		// Section 39 specifies record.query as "by kind, field and project" and
		// makes none of the three mandatory. This surface had made two of them
		// a required conjunction, and because `kind` is not a closed set, that
		// meant no census of the store could ever be complete: a record under
		// an unguessed kind was invisible to every question anybody could
		// write. Note the asymmetry it left behind - `record history` takes a
		// bare id.
		//
		// THE FLAGS EXIST BECAUSE THE POSITIONAL FORM CANNOT EXPRESS THE
		// INTERESTING CASE. `rig record query <project> <kind>` reads in one
		// order, so a lone positional can only be the project; asking for one
		// KIND across every project - which is how a caller finds out what
		// projects exist at all - has no positional spelling.
		r.project = r.fs.String("project", "",
			"the project to look in; unset means every project")
		r.kind = r.fs.String("kind", "",
			"the kind to look for; unset means every kind")

		// ⛔ THE THIRD FILTER, AND IT HAS NO POSITIONAL SPELLING ON PURPOSE.
		// A field query is a PAIR, and a positional pair would put two pieces
		// of arbitrary user text next to a project and a kind in one
		// unlabelled list - four strings whose order is the only thing saying
		// what they are. Section 39's own metadata table is what these select
		// on, and until B65 nothing could: every field it describes was
		// storable and not selectable.
		r.field = r.fs.String("field", "",
			"a field the record must carry; unset means no field predicate")
		r.value = r.fs.String("value", "",
			"the exact value --field must have; empty means the EMPTY STRING, "+
				"not every value")
	case "refs":
		// ⛔ ZERO MEANS "THE DAEMON'S DEFAULT", AND THE DEFAULT USED TO BE 1
		// HERE WHILE THE STORE'S WAS 4.
		//
		// internal/record.DefaultRefsDepth is 4 and the wire carries a zero
		// through to it untouched, deliberately - internal/daemon/record.go
		// says why at the same field: "two places deciding one default is two
		// places to disagree from". A `1` here was the second place. It was
		// not wrong when it was written, because there was no wire and so no
		// other default to disagree with; the wire landing is what made it
		// one.
		//
		// A caller who types nothing now gets what every other consumer of
		// record.refs gets, and the answer says which depth it served - which
		// is the field that makes the zero readable at all.
		r.depth = r.fs.Int("depth", 0,
			"how many hops to follow backwards; unset asks the daemon's default")
		// ⛔ WITHOUT THIS FLAG SECTION 39's RULING IS HALF IMPLEMENTED AT THE
		// PROMPT. That section makes project-scoping a PERFORMANCE rule rather
		// than a preference - the predicate prunes the frontier at every hop -
		// and says crossing is "asked for, never arrived at". The wire grew
		// `cross_project` so a caller COULD ask; a CLI with no flag to ask
		// with serves only the default, permanently and silently, which is
		// the same sentence one layer up.
		r.crossProject = r.fs.Bool("cross-project", false,
			"follow edges out of the record's own project")
	default:
		// A WORD THAT IS NOT A SUBCOMMAND GETS NO USAGE DUMP. flag prints the
		// set's own usage to stderr on a parse error, and a usage block for
		// `rig record stamp` would describe a subcommand that does not exist.
		// The dispatcher refuses the word itself and names the seven; that is
		// the answer, and the flag error underneath it is noise.
		r.fs.SetOutput(io.Discard)
	}
	return r
}

// flagNames is every flag a set declares, as a shell would complete them.
//
// WALKED RATHER THAN LISTED, which is the same argument refusal_verbs_test.go
// makes about the dispatch switch: a hand-kept completion list is a second
// source of truth that rots exactly like the thing it describes. Adding a flag
// to a record verb offers it immediately, and removing one stops offering it.
func flagNames(fs *flag.FlagSet) []string {
	var out []string
	fs.VisitAll(func(f *flag.Flag) { out = append(out, "--"+f.Name) })
	sort.Strings(out)
	return out
}

// wasSet reports whether the CALLER typed a flag, as opposed to the flag
// having its default.
//
// ⛔ THIS IS THE WHOLE if-version RULE AND IT CANNOT BE DONE ON THE VALUE.
// IfVersion 0 MEANS CREATE, so `--if-version=0` and no --if-version at all
// carry the same number and opposite intentions. flag.Visit walks only what
// was actually set, which is the one place that difference survives.
func (r *recordFlags) wasSet(name string) bool {
	seen := false
	r.fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

// ---- dispatch --------------------------------------------------------------

// recordSubcommands are the TEN, in the order the usage line prints them.
//
// ⛔ THE COUNT IS NOT IN THIS CAPTION ANY MORE THAN IT HAS TO BE, and the
// reason is this repository's most-recorded defect: it read "the seven" while
// B77 added three, at which point the sentence was asserting a number the slice
// no longer held. len(recordSubcommands) is the only honest count.
//
// Read by complete.go and by the usage text below, so adding one is a line
// here rather than three places that can disagree - the coupling complete.go
// calls "one string in three places that MUST agree".
var recordSubcommands = []string{
	"put", "get", "query", "history", "link", "unlink", "refs",
	"retract", "delete", "replace",
}

func recordUsage() string {
	return "usage: rig record <" + strings.Join(recordSubcommands, "|") + ">"
}

// cmdRecord is `rig record`.
func cmdRecord(args []string) (err error) {
	flags, positional := partition(args)

	sub := ""
	if len(positional) > 0 {
		sub = positional[0]
	}
	rf := recordFlagSet(sub)
	// ⛔ THE PARSE ERROR IS HELD RATHER THAN RETURNED, because an unknown
	// SUBCOMMAND outranks a bad flag. `rig record stamp --project rig` parsed
	// first reports "flag provided but not defined: -project", which is true
	// and useless: --project is defined on every subcommand that exists, and
	// the caller's actual mistake was the word before it. Measured by running
	// it.
	parseErr := rf.fs.Parse(flags)
	defer func() { err = inMode(err, *rf.asJSON) }()

	if sub == "" {
		return badArgumentf("%s", recordUsage())
	}
	if !slices.Contains(recordSubcommands, sub) {
		// Naming what WAS typed as well as what exists. `rig record put-all`
		// is a typo and `rig record stamp` is somebody completing section
		// 39's set from memory, and the two want different sentences - so the
		// list is printed rather than the caller being told to look it up.
		return badArgumentf("%q is not a record subcommand; they are %s",
			sub, strings.Join(recordSubcommands, ", "))
	}
	if parseErr != nil {
		return parseErr
	}

	// ⛔ NOTHING IS OPENED HERE. Each subcommand below checks its own arguments
	// FIRST and reaches the daemon only through withRecordAPI, which is the
	// order every other verb in this package already uses - peers and ping
	// both refuse a bad argument before they dial.
	//
	// IT WAS THE OTHER WAY ROUND AND RUNNING THE BINARY IS WHAT FOUND IT.
	// Opening here meant `rig record put --id X` with no --if-version answered
	// with the connection's failure instead of the refusal that names the id,
	// and the whole guard was unreachable. Every unit test passed, because a
	// test's fake opens successfully and the two orderings are then
	// indistinguishable.
	rest := positional[1:]
	switch sub {
	case "put":
		return recordPut(rf, rest)
	case "get":
		return recordGet(rf, rest)
	case "query":
		return recordQuery(rf, rest)
	case "history":
		return recordHistory(rf, rest)
	case "link":
		return recordLink(rf, rest, false)
	case "unlink":
		return recordLink(rf, rest, true)
	case "refs":
		return recordRefs(rf, rest)
	case "retract":
		return recordRetract(rf, rest)
	case "delete":
		return recordDelete(rf, rest)
	case "replace":
		return recordReplace(rf, rest)
	default:
		// Unreachable: the membership check above already refused every word
		// that is not one of the seven. It is here so that adding a
		// subcommand to recordSubcommands and forgetting to dispatch it fails
		// loudly rather than silently doing nothing.
		return badArgumentf("%q is listed as a record subcommand and nothing "+
			"dispatches it, which is a defect in rig rather than in what you "+
			"typed", sub)
	}
}

// withRecordAPI opens the record surface, runs one call against it and closes
// it.
//
// IT IS CALLED AFTER A SUBCOMMAND HAS CHECKED ITS ARGUMENTS, NEVER BEFORE.
// That ordering is the whole reason this is a function rather than four lines
// at the top of cmdRecord: a caller with a malformed command must be told what
// is wrong with it, not what is wrong with the daemon, and rig cannot know the
// second thing is even relevant until the first is settled.
//
// The deadline is built here, so no record verb can reach the wire without
// one.
func withRecordAPI(timeout time.Duration, call func(context.Context, RecordAPI) error) error {
	api, release, err := recordAPI()
	if err != nil {
		return err
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return call(ctx, api)
}

// ---- the body --------------------------------------------------------------

// maxBodyBytes is the largest body this surface will read from a file or from
// standard input.
//
// ⛔ THE CEILING IS THE WIRE'S RATHER THAN A TASTE. internal/wire.MaxFrameSize
// is 1 MiB and one put is ONE frame, so a body at the frame limit cannot fit:
// the same frame carries the id, the kind, the project and every typed field.
// 64 KiB is left for them, which is half the largest argv a shell on this
// machine will pass and so wider than the flags can be in practice.
//
// IT IS DERIVED AND NOT COPIED, because two places holding one ceiling is two
// places to disagree from - the reason internal/daemon/record.go already gives
// about the refs depth. Raising MaxFrameSize raises this with it.
//
// AND IT IS A BOUND ON AN ALLOCATION, not only a courtesy. The path comes from
// the caller, and reading whatever is at it into memory with no limit is the
// unchecked-bounds failure the standing rules name. The largest body this
// project has actually written is 3,502 bytes.
const maxBodyBytes = rigwire.MaxFrameSize - 64*1024

// bodyFileStdin is the --body-file value meaning standard input.
//
// `-` IS THE SPELLING EVERY OTHER TOOL USES for this, so it is the one a
// caller guesses first, and it cannot collide with a real path: a file called
// `-` is reachable as `./-` and is not a thing this project has.
const bodyFileStdin = "-"

// stdinSource is standard input, indirected so the rules below are testable.
//
// A test needs three different standard inputs - a pipe carrying prose, a pipe
// carrying nothing, and a character device standing in for a terminal - and
// the process only has one. Nothing else in this file reaches for os.Stdin.
var stdinSource = func() *os.File { return os.Stdin }

// putBody is where a record's prose comes from, and the whole of B75 is the
// second-to-last case below.
//
// ⛔ PROSE OFFERED ON A ROUTE RIG DOES NOT READ IS REFUSED, NOT DISCARDED, AND
// THE CHOICE WAS BETWEEN THOSE TWO. `echo prose | rig record put --kind
// working-note --project a0-survey` answered `created ... at version 1`, exited
// 0, and stored an empty body. A create that destroys what it was handed and
// reports success is the worse half of B75: the id is real, so nothing the
// caller can see says to look.
//
// CONSUMING IT SILENTLY WAS THE OTHER ANSWER AND IT WAS REJECTED FOR THREE
// REASONS, one of which is decisive:
//
//  1. It makes rig guess. Two hundred lines above, recordPut already refuses
//     an omitted --if-version, an empty --project and a --value with no
//     --field, each with the same sentence: rig will not guess which you
//     meant. A fourth behaviour that guesses contradicts the other three.
//  2. ⛔ IT WOULD BLOCK. A put whose standard input is an inherited pipe that
//     nobody ever writes to would wait for EOF forever, and a hung CLI reads
//     as a hung DAEMON. The refusal below reads the file MODE and never reads
//     a byte, so it cannot hang. This is the reason that decides it.
//  3. It needs a precedence rule for --body plus a pipe that nobody has asked
//     for.
//
// A refusal naming the flag is one keystroke from fixed. A silent success is
// not discoverable at all.
func putBody(rf *recordFlags) (string, error) {
	bodyNamed, fileNamed := rf.wasSet("body"), rf.wasSet("body-file")
	switch {
	case bodyNamed && fileNamed:
		// TWO BODIES AND NO RULE FOR PICKING ONE, which is the duplicate
		// --field refusal one noun over: last-one-wins is silent, and the
		// caller never learns which of the two they are reading back.
		return "", badArgumentf(
			"--body and --body-file both say what the record's prose is, and " +
				"rig will not pick one for you.\n" +
				"       --body <text>      the prose is here on the command line\n" +
				"       --body-file <path> the prose is in a file, or on " +
				"standard input as -")

	case fileNamed && *rf.bodyFile == "":
		// The same accident every other flag on this surface guards: a shell
		// variable that expanded to nothing. An empty path is not the current
		// directory and it is not standard input.
		return "", badArgumentf(
			"--body-file was given an empty path, which is what a shell " +
				"variable that expanded to nothing looks like.\n" +
				"       --body-file <path> read the prose from a file\n" +
				"       --body-file -      read it from standard input")

	case fileNamed && *rf.bodyFile == bodyFileStdin:
		return readBody(stdinSource(), "standard input")

	case fileNamed:
		// THE PATH IS THE CALLER'S OWN AND IS OPENED AS THEY TYPED IT. Nothing
		// is joined onto a base here, so there is no traversal to validate:
		// this process has exactly the caller's own privileges and they could
		// have read the file with `cat`. What IS checked is the SIZE, in
		// readBody, because an unbounded read of a caller-named path is an
		// allocation nobody limits.
		f, err := os.Open(*rf.bodyFile)
		if err != nil {
			// The error already names the path - "open /x/y: no such file or
			// directory" - so naming it again would print it twice.
			return "", badArgumentf("--body-file could not be read: %v", err)
		}
		defer func() { _ = f.Close() }()
		return readBody(f, *rf.bodyFile)

	case !bodyNamed && stdinIsOffering():
		// ⛔ THIS IS B75. Nothing is read; the mode alone says that something
		// is there, and something being there with no flag to route it is the
		// one case where creating the record is the wrong answer.
		//
		// AN EMPTY PIPE IS REFUSED TOO, AND THAT IS THE PRICE OF NOT READING.
		// `printf "" | rig record put ...` offered nothing and is refused
		// anyway, because telling an empty pipe from a full one means reading
		// it, and reading it is what can hang. The trade is a loud refusal
		// with three ways out against a silent hang, and the way out for a
		// caller who meant no body is one flag: --body ''. Measured by running
		// it - do not "fix" this by reading first.
		return "", badArgumentf(
			"rig record put was given no body, and standard input is not a "+
				"terminal - something is piped in and nothing here would "+
				"read it.\n"+
				"       rig will not create a record that silently drops "+
				"prose it was handed.\n"+
				"       %-18s use what is on standard input as the body\n"+
				"       %-18s give the prose on the command line\n"+
				"       %-18s create the record with an empty body, on purpose",
			"--body-file -", "--body <text>", "--body ''")
	}
	return *rf.body, nil
}

// stdinIsOffering reports whether standard input is a stream that could be
// carrying bytes nobody has asked rig to read.
//
// ⛔ THE TEST IS THE FILE MODE AND NOT `TIOCGWINSZ`. briefStyleFor asks the
// ioctl because it needs a WIDTH, and that call fails on /dev/null exactly as
// it fails on a pipe - correct for layout and wrong here, because
// `rig record put < /dev/null` offers nothing and must not be refused. A
// character device covers both a terminal and /dev/null; a pipe and a
// redirected regular file are the two shapes that carry bytes.
//
// A CLOSED DESCRIPTOR IS NOT AN OFFER EITHER. Stat fails on one, and the
// answer is false rather than a panic.
func stdinIsOffering() bool {
	st, err := stdinSource().Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice == 0
}

// readBody reads a body from a file or from standard input, bounded.
//
// ⛔ THE TRAILING NEWLINE IS DROPPED, AND THAT IS ABOUT TWO ROUTES STORING ONE
// VALUE. `echo prose |` ends in a newline because that is what a line is, and
// a file ends in one because that is what a text file is; `--body prose`
// cannot express a trailing newline at all. Left in, the same intent would
// store two bodies differing by an invisible byte, and the store is where that
// difference gets compared later by something that cannot see it either.
// Leading and interior whitespace is the caller's and is untouched.
func readBody(r io.Reader, name string) (string, error) {
	// ONE BYTE OVER THE CEILING IS READ ON PURPOSE: it is how a body AT the
	// limit is told from one OVER it without reading the rest of the file.
	raw, err := io.ReadAll(io.LimitReader(r, maxBodyBytes+1))
	if err != nil {
		return "", badArgumentf("the body could not be read from %s: %v", name, err)
	}
	// ⛔ INVALID UTF-8 IS REFUSED HERE SO THE MESSAGE IS ABOUT THE BODY.
	// protobuf refuses a string field that is not valid UTF-8, and left to it
	// the answer is "client: marshal rig.record.put: string field contains
	// invalid UTF-8" - true, and it names neither the body nor the file the
	// bytes came from. A caller who pointed --body-file at a binary has no
	// way from that sentence to what they typed. Measured by running it.
	if !utf8.Valid(raw) {
		return "", badArgumentf(
			"%s is not valid UTF-8, and a record's body is text.\n"+
				"       the first bad byte is at offset %d\n"+
				"       a file rig cannot read as text is a file, and a "+
				"record that points at it is the shape rig has for that",
			name, firstInvalidUTF8(raw))
	}
	if len(raw) > maxBodyBytes {
		return "", badArgumentf(
			"the body from %s is larger than %d bytes, which is more than "+
				"one record.put can carry: the whole request travels in a "+
				"single %d-byte frame and the id, the kind, the project and "+
				"every typed field share it.\n"+
				"       a record that large is a file, and a record that "+
				"points at the file is the shape rig has for it",
			name, maxBodyBytes, rigwire.MaxFrameSize)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

// firstInvalidUTF8 is the offset of the first byte that is not part of a valid
// rune, so a refusal can point at it rather than at the whole file.
//
// It is only ever called on bytes utf8.Valid has already rejected, so the
// final return is unreachable in practice - it is there because a function
// that answers "nowhere" with a plausible-looking 0 would be worse than one
// that answers with the length.
func firstInvalidUTF8(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			return i
		}
		i += size
	}
	return len(b)
}

// ---- put -------------------------------------------------------------------

// recordPut writes a record.
//
// ⛔ THE ENTIRE FUNCTION IS SHAPED BY ONE FACT: IfVersion 0 MEANS CREATE.
// internal/record/records.go says so at the field and internal/record's tests
// pin it. A caller who forgets --if-version therefore does not get a
// half-written supersede - they get a CREATE, which against an existing id is
// refused as a conflict. That refusal is correct and it is the wrong
// conversation: it tells the caller the id is taken when what happened is that
// they omitted a flag.
//
// So the flag is REQUIRED whenever an id is named, and the refusal NAMES THE
// ID, because the id is the only thing in the caller's head that connects the
// message to what they typed.
func recordPut(rf *recordFlags, rest []string) error {
	if len(rest) != 0 {
		return badArgumentf("usage: rig record put --kind <kind> --project " +
			"<project> [--id <id>] [--body <text> | --body-file <path>] " +
			"[--field k=v] [--if-version <n>]")
	}
	if *rf.kind == "" || *rf.project == "" {
		return badArgumentf("rig record put needs --kind and --project: a " +
			"record is what it is and what it belongs to, and neither has a " +
			"default that could be right")
	}

	idNamed := rf.wasSet("id")
	versionNamed := rf.wasSet("if-version")

	switch {
	case idNamed && !versionNamed:
		return badArgumentf(
			"rig record put --id %s did not say --if-version, and that reads "+
				"as a CREATE.\n"+
				"       IfVersion 0 means create, so an omitted flag and an "+
				"explicit create are the same request on the wire and rig "+
				"will not guess which you meant.\n"+
				"       --if-version <n>  supersede version n of %s\n"+
				"       --if-version 0    create %s, which is how a project "+
				"or a case is written, since its id is its slug",
			*rf.id, *rf.id, *rf.id)
	case !idNamed && versionNamed && *rf.ifVersion != 0:
		// The mirror image, and internal/record refuses it too: "a put
		// superseding a version needs the id it supersedes." Caught here so
		// the caller is told before anything is dialled.
		return badArgumentf(
			"--if-version %d names a version to supersede and no --id says "+
				"which record. A supersede needs the id it supersedes",
			*rf.ifVersion)
	}

	// THE BODY IS RESOLVED BEFORE ANYTHING IS DIALLED, which is this file's
	// stated ordering and not a preference: a caller with a malformed command
	// must be told what is wrong with it rather than what is wrong with the
	// daemon. It also means a body that cannot be read never mints an id.
	body, err := putBody(rf)
	if err != nil {
		return err
	}

	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		rec, err := api.Put(ctx, PutArgs{
			ID:        *rf.id,
			IfVersion: *rf.ifVersion,
			Kind:      *rf.kind,
			Project:   *rf.project,
			Body:      body,
			Fields:    rf.fields.values(),
		})
		if err != nil {
			return err
		}

		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(recordJSON(rec, time.Now()))
		}
		// WHICH IT WAS IS PRINTED, not left to be inferred from the number. A
		// caller who meant to supersede and created reads "wrote version 1" as
		// a success; "created" is the word that tells them otherwise.
		fmt.Printf("%s %s at version %d\n", putVerb(rec.Version), rec.ID, rec.Version)
		return nil
	})
}

// putVerb is what a put DID, in one word.
//
// Version 1 is a create because the first write is version 1 and nothing
// supersedes into it - the store's own numbering, not a convention this file
// invents.
func putVerb(version uint64) string {
	if version <= 1 {
		return "created"
	}
	return "superseded"
}

// ---- get -------------------------------------------------------------------

func recordGet(rf *recordFlags, rest []string) error {
	if len(rest) != 1 {
		return badArgumentf("usage: rig record get <id> [--version <n>]")
	}
	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		rec, err := api.Get(ctx, rest[0], *rf.version)
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(recordJSON(rec, time.Now()))
		}
		fmt.Print(recordText(rec, time.Now()))
		return nil
	})
}

// ---- query -----------------------------------------------------------------

// verbS is the "s" a VERB takes, which is the opposite of the one plural()
// gives a noun. One edge POINTS; two edges POINT.
//
// It is a second three-line function rather than a flag on plural() because
// the two are used in the same sentence and a boolean there reads as "plural
// or not" at exactly the moment the reader needs to know which word it is
// agreeing with.
func verbS(n int) string {
	if n == 1 {
		return "s"
	}
	return ""
}

const queryUsage = "usage: rig record query [<project> [<kind>]]\n" +
	"       rig record query [--project <p>] [--kind <k>] " +
	"[--field <f> --value <v>]\n" +
	"       every filter is optional and an omitted one means EVERY value, " +
	"so\n       `rig record query` on its own is the whole store.\n" +
	"       --field and --value go together: a value with no field is refused, " +
	"and\n       an empty --value means the EMPTY STRING, not every value"

// recordQuery lists records, filtered by nothing, by one thing or by both.
//
// ⛔ AN OMITTED FILTER MEANS EVERY VALUE AND AN EXPLICITLY EMPTY ONE IS
// REFUSED, AND THE DIFFERENCE IS THE WHOLE GUARD. `--project ""` is almost
// always a shell variable that expanded to nothing, and on the wire it is
// byte-identical to a caller that deliberately asked for every project - so
// the daemon cannot tell them apart and does not try. THIS is the layer that
// can: flag.Visit knows what was typed, which is the same distinction
// --if-version turns on two hundred lines above.
func recordQuery(rf *recordFlags, rest []string) error {
	if len(rest) > 2 {
		return badArgumentf("%s", queryUsage)
	}

	project, kind := *rf.project, *rf.kind
	for _, f := range []struct {
		name, value string
		positional  int
	}{{"project", project, 0}, {"kind", kind, 1}} {
		if rf.wasSet(f.name) && f.value == "" {
			// THE TWO WAYS OUT ARE COLUMN-ALIGNED, because they are read as a
			// pair and a ragged left edge makes the reader find the second
			// one rather than see it. %-18s is the width of the longer form.
			return badArgumentf(
				"--%s was given an empty value. An OMITTED --%s means every "+
					"%s, which is almost certainly what you want; an empty one "+
					"is what a shell variable that expanded to nothing looks "+
					"like, and rig will not guess which happened.\n"+
					"       %-18s ask for every %s\n"+
					"       %-18s ask for one",
				f.name, f.name, f.name,
				"(drop the flag)", f.name,
				"--"+f.name+" <name>")
		}
		if len(rest) > f.positional && rest[f.positional] == "" {
			// THE SAME ACCIDENT ONE ARGUMENT OVER. `rig record query "$P" note`
			// with P unset is a variable that expanded to nothing, and it
			// reaches here as a positional rather than as a flag - so the
			// guard above would never see it and the store would be asked for
			// every project.
			return badArgumentf(
				"the %s was given as an empty argument. Leave it out to ask "+
					"for every %s; rig will not read an argument that expanded "+
					"to nothing as a request for everything.\n"+
					"       %-18s ask for every %s\n"+
					"       %-18s ask for one",
				f.name, f.name,
				"rig record query", f.name,
				"--"+f.name+" <name>")
		}
		if len(rest) > f.positional && rf.wasSet(f.name) {
			return badArgumentf(
				"the %s was given twice, as %q and as --%s %q. One of them is "+
					"the one you meant and rig cannot tell which",
				f.name, rest[f.positional], f.name, f.value)
		}
	}
	if len(rest) > 0 {
		project = rest[0]
	}
	if len(rest) > 1 {
		kind = rest[1]
	}

	// ⛔ A --value WITH NO --field IS REFUSED HERE AS WELL AS IN THE STORE, AND
	// THE TWO REFUSALS ARE NOT REDUNDANT. The store's is the invariant and
	// catches every caller including the daemon's own; this one can say what
	// was TYPED, which the store cannot see. `--value "$S"` with S unset is a
	// shell variable that expanded to nothing, and it is byte-identical on the
	// wire to a deliberate ask for the empty string.
	field, value := *rf.field, *rf.value
	if field == "" && rf.wasSet("value") {
		return badArgumentf(
			"--value was given with no --field to match it against. rig will "+
				"not drop the predicate and answer with every record, because "+
				"that is a WIDER answer than you asked for and nothing would "+
				"say so.\n"+
				"       %-26s ask for one field's value\n"+
				"       %-26s ask for every record",
			"--field <name> --value <v>", "(drop both flags)")
	}
	if rf.wasSet("field") && field == "" {
		return badArgumentf(
			"--field was given an empty value, which is what a shell variable " +
				"that expanded to nothing looks like. An OMITTED --field means " +
				"no field predicate at all; rig will not guess which happened.\n" +
				"       (drop the flag)           ask without a field predicate\n" +
				"       --field <name>            narrow by a field")
	}

	a := QueryArgs{Project: project, Kind: kind, Field: field, Value: value}
	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		recs, err := api.Query(ctx, a)
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(recordsJSON(recs, time.Now()))
		}
		fmt.Print(queryText(a, recs, briefStyleFor(os.Stdout).Width))
		return nil
	})
}

// ---- history ---------------------------------------------------------------

func recordHistory(rf *recordFlags, rest []string) error {
	if len(rest) != 1 {
		return badArgumentf("usage: rig record history <id>")
	}
	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		recs, err := api.History(ctx, rest[0])
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(recordsJSON(recs, time.Now()))
		}
		fmt.Print(historyText(rest[0], recs, time.Now(),
			briefStyleFor(os.Stdout).Width))
		return nil
	})
}

// ---- link and unlink -------------------------------------------------------

// recordLink writes or removes one edge. One function for both, because the
// argument shape, the refusal and the confirmation differ by a single word and
// two copies would be two places for that word to be wrong.
func recordLink(rf *recordFlags, rest []string, remove bool) error {
	verb := "link"
	if remove {
		verb = "unlink"
	}
	if len(rest) != 3 {
		// THE ORDER IS THE EDGE'S DIRECTION AND THE USAGE LINE SAYS SO IN A
		// PICTURE. `rig record link A cites B` has to read as "A cites B" and
		// nothing but an arrow makes a reader check that.
		return badArgumentf("usage: rig record %s <src> <type> <dst>\n"+
			"       the edge is DIRECTED: src --type--> dst, so `rig record "+
			"%s a cites b` reads as \"a cites b\"", verb, verb)
	}
	src, linkType, dst := rest[0], rest[1], rest[2]

	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		var err error
		if remove {
			err = api.Unlink(ctx, src, linkType, dst)
		} else {
			err = api.Link(ctx, src, linkType, dst)
		}
		if err != nil {
			return err
		}

		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"src": src, "type": linkType, "dst": dst, "removed": remove,
			})
		}
		fmt.Printf("%s %s --%s--> %s\n", linkVerbPast(remove), src, linkType, dst)
		return nil
	})
}

func linkVerbPast(remove bool) string {
	if remove {
		return "removed"
	}
	return "linked"
}

// ---- refs ------------------------------------------------------------------

func recordRefs(rf *recordFlags, rest []string) error {
	if len(rest) != 1 {
		return badArgumentf("usage: rig record refs <id> [--depth <n>]")
	}
	// ⛔ THE REFUSAL IS ON WHAT WAS TYPED, NOT ON THE VALUE, AND THAT IS THE
	// SAME DISTINCTION --if-version TURNS ON. An unset --depth is now 0
	// meaning "the daemon's default"; a TYPED --depth 0 is a caller asking to
	// follow no edge at all, and its answer - nothing - could not be told from
	// a record nothing points at, which is the one answer this verb exists to
	// make readable. flag.Visit walks only what was actually set, which is the
	// one place that difference survives.
	if rf.wasSet("depth") && *rf.depth < 1 {
		return badArgumentf("--depth %d follows no edge at all, so its answer "+
			"could not be told from a record nothing points at. The smallest "+
			"useful depth is 1, and leaving --depth off asks the daemon for "+
			"its own default", *rf.depth)
	}
	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		refs, err := api.Refs(ctx, RefsArgs{
			ID:           rest[0],
			Depth:        *rf.depth,
			CrossProject: *rf.crossProject,
		})
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(refsJSON(refs))
		}
		fmt.Print(refsText(refs, briefStyleFor(os.Stdout).Width))
		return nil
	})
}

// ---- rendering -------------------------------------------------------------

// recordJSON is the object --json emits for one record.
//
// EVERY KEY IS PRESENT ON EVERY ANSWER, no omitempty anywhere, which is the
// argument peersJSON, estateJSON and meta's answerJSON all write down: an
// absent key reads as "this was never considered", and an empty body is the
// ANSWER for a record whose content is entirely in its typed fields.
//
// `fields` IS {} AND NEVER null WHEN A RECORD HAS NONE, for the reason `crew`
// is [] in peersJSON: a null is indistinguishable from a field this build
// failed to set, and a record with no typed fields is an ordinary record
// rather than a broken one.
func recordJSON(r Record, now time.Time) map[string]any {
	fields := map[string]string{}
	for k, v := range r.Fields {
		fields[k] = v
	}
	return map[string]any{
		"id":      r.ID,
		"version": r.Version,
		"kind":    r.Kind,
		"project": r.Project,
		"body":    r.Body,
		"fields":  fields,
		// Provenance is nested rather than flattened, because it is one fact
		// about one write and a reader merging it into the record's own
		// fields loses which is which the moment a kind declares a field
		// called `seat`.
		"provenance": map[string]any{
			"session": r.Prov.Session,
			"seat":    r.Prov.Seat,
			"epoch":   r.Prov.Epoch,
			// The timestamp is emitted unchanged AS WELL AS the age, because
			// a consumer computing against its own clock must not be forced
			// through this one - peersJSON's rule, and the same reason.
			"created_at":      provTime(r.Prov.CreatedAt),
			"created_age_s":   provAge(r.Prov.CreatedAt, now),
			"created_at_unix": provUnix(r.Prov.CreatedAt),
		},

		// ⛔ B77. null ON A LIVE RECORD AND AN OBJECT ON A WITHDRAWN ONE, and
		// the key is ALWAYS PRESENT - unlike the text form, which prints
		// nothing. A person skims and a `"retraction": null` on every record
		// would be noise; a consumer cannot tell a build that serves this from
		// one that does not unless the key is there to be read.
		"retraction": retractionJSON(r.Retraction, now),
	}
}

// retractionJSON is a withdrawal as an object, or nil.
func retractionJSON(r *Retraction, now time.Time) map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{
		"reason": r.Reason,
		// Emitted even when empty, because "" and absent are the same bytes in
		// JSON for a string and a consumer branching on presence would read a
		// plain retract as a malformed replace.
		"replaced_by": r.ReplacedBy,
		"retracted_by": map[string]any{
			"session":         r.Prov.Session,
			"seat":            r.Prov.Seat,
			"epoch":           r.Prov.Epoch,
			"created_at":      provTime(r.Prov.CreatedAt),
			"created_age_s":   provAge(r.Prov.CreatedAt, now),
			"created_at_unix": provUnix(r.Prov.CreatedAt),
		},
	}
}

// retractionNotice is the block `record get` prints over a withdrawn record,
// and nothing at all over a live one.
//
// ⛔ NOTHING AT ALL, rather than "not retracted". Every other section of this
// renderer prints its own absence, and the reason is the absent-versus-empty
// rule - but a retraction is not a section, it is a CONDITION. A line reading
// "retracted: no" on every record in the store would train a reader to skip the
// place the notice appears, which is the one outcome that makes the notice
// worthless on the day it is there.
func retractionNotice(r *Retraction, now time.Time) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n⛔ RETRACTED - this record is withdrawn.\n")
	fmt.Fprintf(&b, "  reason: %s\n", briefCell(r.Reason, "(none given)"))
	if r.ReplacedBy != "" {
		// ⛔ THE SURVIVOR IS NAMED AND THE COMMAND IS SPELLED OUT. A reader
		// holding the old id is asking where the fact went, and an id with no
		// verb beside it is one more thing to look up.
		fmt.Fprintf(&b, "  replaced by: %s - `rig record get %s`\n",
			r.ReplacedBy, r.ReplacedBy)
	}
	b.WriteString("  " + strings.TrimPrefix(provenanceLine(r.Prov, now), "written "))
	b.WriteString("  it is gone from every brief and every query. " +
		"Its history below is intact.\n")
	return b.String()
}

// recordsJSON is a list of records, and it is ALWAYS AN ARRAY.
//
// A query that matched nothing emits [], never null, and never an object
// wrapping a count. `rig record query rig requirement` on a project with no
// requirements is the ordinary first call, exactly as an empty roster is.
func recordsJSON(rs []Record, now time.Time) []map[string]any {
	out := make([]map[string]any, 0, len(rs))
	for _, r := range rs {
		out = append(out, recordJSON(r, now))
	}
	return out
}

// provTime renders a stamp, and A ZERO IS NOT THE UNIX EPOCH.
//
// Computed straight, an unset CreatedAt prints 0001-01-01T00:00:00Z, which
// reads as a date rather than as an absence. internal/record refuses to write
// a record without provenance, so a zero here means the daemon or this wire
// dropped it - a defect, and the string says which rather than handing the
// reader a year to interpret.
func provTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func provUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixNano()
}

// provAge reuses peers' ageSeconds, which returns -1 for a stamp that was
// never set. Deliberately the same function rather than a second one: two
// surfaces of one client disagreeing about what an age means is the class of
// defect section 10 exists to prevent.
func provAge(t, now time.Time) int64 {
	return ageSeconds(provUnix(t), now)
}

// recordText is one record in full, for a person.
//
// A LABELLED BLOCK RATHER THAN A ROW, because a record has a body and free
// text does not survive a column. The listing renderers below are the ones
// that use writeTable.
func recordText(r Record, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  version %d\n", r.ID, r.Version)
	fmt.Fprintf(&b, "%s in %s\n", r.Kind, r.Project)
	b.WriteString(provenanceLine(r.Prov, now))

	// ⛔ THE WITHDRAWAL GOES FIRST, BEFORE THE CONTENT IT QUALIFIES. B77's
	// contract is that `record.get` is the ONE verb still answering about a
	// retracted record, so everything below this line is a record that is gone
	// from every brief and every query - and a reader who took the fields as
	// live because the notice was at the bottom has been told the truth in an
	// order that made it useless.
	b.WriteString(retractionNotice(r.Retraction, now))

	// THE BODY AND THE FIELDS ARE LABELLED EVEN WHEN EMPTY. A record whose
	// content is entirely in its typed fields is normal - section 39 says so
	// in as many words, "never prose buried in a body an agent has to parse" -
	// so a missing body is a fact to state, not a section to drop. Dropped,
	// it reads as a renderer that ran out of things to print.
	b.WriteString("\nfields\n")
	if len(r.Fields) == 0 {
		b.WriteString("  (none)\n")
	} else {
		keys := make([]string, 0, len(r.Fields))
		width := 0
		for k := range r.Fields {
			keys = append(keys, k)
			width = max(width, len(k))
		}
		// Sorted, because a map's order is random and a record printed twice
		// must not look like two different records.
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, k, r.Fields[k])
		}
	}

	b.WriteString("\nbody\n")
	if strings.TrimSpace(r.Body) == "" {
		b.WriteString("  (none)\n")
		return b.String()
	}
	for _, line := range strings.Split(strings.TrimRight(r.Body, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

// provenanceLine is who wrote a version and when.
//
// ⛔ AN EMPTY SEAT IS A DEFECT AND RENDERS AS ONE. internal/record refuses a
// put without session and seat both - "a put needs its provenance: session and
// seat" - so a blank cell here cannot mean "nobody claimed it". It means
// something between the store and this renderer dropped it, and section 21's
// rule about the zero applies exactly: nothing was said is never a fact about
// anything.
//
// ⛔ THE SESSION IS DELIBERATELY NOT PRINTED, AND RESTORING IT WOULD BE A
// REGRESSION RATHER THAN A FIX. MEASURED 2026-09-17, sixty `rig record put`
// invocations at one terminal by one person in one act: SIXTY DISTINCT
// SESSIONS, one constant seat, one constant epoch. A session IS a connection
// today - `wire.proto` says so, "because serveSession answers SESSION_DEAD to
// every resume" - so the id is minted per INVOCATION, not per sitting.
//
// Printing it to a human asserts a grouping that does not exist: it sits in a
// sentence beside the seat, reads as the narrower "which sitting", and answers
// a "what did this session do" question with exactly one record every time.
// **That is a confidently wrong answer, not a coarse one.** `--json` keeps the
// field, because a parser wants raw identity with no claim attached and a
// human is the one being told something.
//
// ⛔ THE REOPEN CONDITION IS NAMED AND IT IS NOT A DATE: WHEN A SESSION
// OUTLIVES ITS CONNECTION - `wire.proto` puts that at M7, "when a resumed
// session starts carrying anything at all" - the field starts meaning what
// this sentence would imply and it comes back. Until then, seat and epoch are
// the two fields that identify an act and they are both here.
func provenanceLine(p Provenance, now time.Time) string {
	return fmt.Sprintf("written by %s, epoch %d, %s\n",
		displaySeat(p.Seat), p.Epoch, provWhen(p.CreatedAt, now))
}

// displaySeat renders a seat FOR A HUMAN. The stored value is never changed
// and `--json` never sees this function.
//
// ⛔ RULED BY BORIS 2026-09-17 OFF A LIVE TRANSCRIPT. `terminal:boris-milner`
// is derived from the socket's peer credentials and is therefore unforgeable -
// `internal/daemon/record.go` mints it as `"terminal:" + u.Username` from
// `user.LookupId(p.UID)` - and that derivation is the whole value of the
// field. So the STORE keeps it and only the rendering changes.
//
// ⛔ A DISPLAY NAME IS SET, NOT COMPUTED, AND THAT IS THE POINT. The first
// shape proposed was "cut at the first hyphen", which renders `boris-milner`
// as `boris` and is a rule inferred from one example: `mary-jane` becomes
// `mary`, and a name with no hyphen renders whole, so the rule is INVISIBLE
// until it meets a name shaped like the one it was written for. That is the
// same defect as a backlog parser that trims markers off the front of a cell
// because the cell it was written for began with them.
//
// ⛔ `RIG_DISPLAY_NAME` IS §6's FIRST KEY ARRIVING AHEAD OF §6. That section
// specifies a seven-layer resolution - /etc, ~/.config, declared defaults,
// per-app toml, `RIG_*`, flags, runtime override - with schema-declared keys
// and `rig config origin`. NONE OF IT IS BUILT: there is not one `RIG_*` key
// anywhere in `cmd/` or `internal/` today. This sits exactly on the declared
// `environment (RIG_*)` layer, so it is that system's first key rather than a
// contradiction of it, and it must not grow into a config system here.
//
// ⛔ IT SUBSTITUTES ONLY FOR THIS CALLER'S OWN SEAT, AND THAT BOUND IS LOAD
// BEARING RATHER THAN CAUTIOUS. `record history` renders OTHER seats in the
// same column - `backend-1`, `record`, another person's terminal - and a
// display name applied to all of them would print this user's name over
// another writer's work. **On a provenance surface, misattribution is the
// worst available failure**, and it would be invisible on a single-user
// machine, which is the only kind this was tested on.
//
// UNSET OR EMPTY FALLS BACK TO STRIPPING THE KIND PREFIX, never to the raw
// seat: an unset reader sees `boris-milner`, which is always correct. That
// fallback is the path every machine but this one takes, so it is the half
// that is tested hardest.
func displaySeat(seat string) string {
	if seat == "" {
		return provWord(seat)
	}
	if name := os.Getenv("RIG_DISPLAY_NAME"); name != "" && seat == selfSeat() {
		return name
	}
	// A seat with no kind prefix is left whole. `backend-1` is a seat name,
	// not a namespaced one, and cutting on a colon that is not there must not
	// invent an empty string.
	if _, rest, found := strings.Cut(seat, ":"); found && rest != "" {
		return rest
	}
	return seat
}

// selfSeat is how THIS caller's own writes are stamped by the daemon, so
// displaySeat can tell "my record" from "somebody else's".
//
// It is a package var rather than a call so a test can state which seat it is
// pretending to be - the same reason `skewOut` is one. Computed lazily,
// because a CLI that never renders a seat should not pay for a user lookup.
var selfSeat = func() string {
	if cachedSelfSeat == "" {
		u, err := user.Current()
		if err != nil || u.Username == "" {
			// ⛔ NO SEAT RATHER THAN A GUESS. If this lookup fails, nothing
			// matches and every seat renders stripped - which is the correct
			// answer, not a degraded one.
			cachedSelfSeat = "\x00none"
		} else {
			cachedSelfSeat = "terminal:" + u.Username
		}
	}
	return cachedSelfSeat
}

var cachedSelfSeat string

// provWord renders a provenance string, in a shape no real value can take when
// it is missing.
func provWord(s string) string {
	if s == "" {
		return "(not said)"
	}
	return s
}

func provWhen(t, now time.Time) string {
	if t.IsZero() {
		return "at no recorded time"
	}
	// peersAgeCell, deliberately: "2m" must mean the same thing on every
	// surface this binary prints. Its name says peers because peers is where
	// it was first needed, and renaming it is a change to a file this seat
	// does not own.
	return fmt.Sprintf("%s (%s ago)", t.UTC().Format(time.RFC3339),
		peersAgeCell(provUnix(t), now))
}

// queryScope names what was asked for, in the words a caller would use.
//
// ⛔ AN EMPTY FILTER IS A DIFFERENT SENTENCE, NOT A BLANK IN THE SAME ONE.
// Both filters became optional, and the old wording formatted straight through
// them: no kind and no project printed "no  records in .", which reads as a
// broken command rather than as an answer about the whole store.
// THE COUNT IS A PARAMETER BECAUSE THE NOUN AGREES WITH IT. `1 records in rig`
// is the same defect as refsText's "1 edge point at B9", caught here before it
// shipped rather than after; an empty answer takes n=0, which is plural in
// English ("no records in rig").
func queryScope(a QueryArgs, n int) string {
	noun := "record" + plural(n)
	var scope string
	switch {
	case a.Project == "" && a.Kind == "":
		scope = noun + ", across all projects"
	case a.Project == "":
		scope = a.Kind + " " + noun + ", across all projects"
	case a.Kind == "":
		scope = noun + " in " + a.Project
	default:
		scope = a.Kind + " " + noun + " in " + a.Project
	}

	// ⛔ THE FIELD PREDICATE IS NAMED IN THE ANSWER, AND THE EMPTY RESULT IS
	// WHY. "no work-item records in rig" and "no work-item records in rig with
	// status=\"closed\"" are different facts, and a reader who narrowed by a
	// field and got nothing cannot otherwise tell which of the three filters
	// emptied the answer. The value is QUOTED because an empty one is a real
	// query: `with status=` reads as a truncated line, `with status=""` reads
	// as the question that was asked.
	if a.Field != "" {
		scope += " with " + a.Field + "=" + strconv.Quote(a.Value)
	}
	return scope
}

// queryText is a listing of the records a filter matched.
func queryText(a QueryArgs, rs []Record, width int) string {
	var b strings.Builder
	scope := queryScope(a, len(rs))
	project, kind := a.Project, a.Kind

	// AN EMPTY RESULT IS A SENTENCE, NOT A BLANK TABLE, and it names what was
	// asked. peersText's rule: a bare header over nothing reads as a broken
	// command, and the reader of an empty answer is usually somebody who
	// expected rows.
	if len(rs) == 0 {
		fmt.Fprintf(&b, "no %s.\n", scope)
		b.WriteString("A row appears when something calls record.put with " +
			"that kind and project;\nboth are matched exactly, so a kind " +
			"spelled differently is a different kind.\n")
		if project != "" || kind != "" || a.Field != "" {
			// ⛔ THE WAY OUT IS PRINTED, because the reader of an empty answer
			// has no way to tell a store with nothing in it from a filter that
			// does not match anything - and until both filters became
			// optional, there was no command that could tell them either.
			b.WriteString("`rig record query` with no filter lists the whole " +
				"store, which is how\na kind or a project spelled differently " +
				"is found.\n")
		}
		return b.String()
	}

	// ⛔ THE PROJECT COLUMN APPEARS ONLY WHEN IT VARIES. Printed always it is a
	// column of one repeated word on every scoped call, which is every call
	// this CLI made before the filters became optional; omitted always, an
	// unscoped listing is a pile of ids from projects the reader cannot tell
	// apart.
	header := []string{"ID", "VERSION", "KIND", "SUMMARY"}
	if project == "" {
		header = []string{"ID", "PROJECT", "VERSION", "KIND", "SUMMARY"}
	}
	rows := make([][]string, 0, len(rs))
	for _, r := range rs {
		row := []string{r.ID}
		if project == "" {
			row = append(row, r.Project)
		}
		row = append(row,
			"v"+strconv.FormatUint(r.Version, 10),
			r.Kind,
			recordSummary(r),
		)
		rows = append(rows, row)
	}
	queryTable(&b, header, rows, width)
	fmt.Fprintf(&b, "\n%d %s.\n", len(rs), scope)
	return b.String()
}

// queryTable lays the listing out AND FITS IT TO THE TERMINAL, under the house
// rule rather than a rule of its own.
//
// ⛔ MEASURED, NOT SUSPECTED. `rig record query` with no filter answered 568
// lines on 2026-09-17 whose longest was 413 columns, against a store whose
// widest id is 161 characters and whose commonest id - a backlog item, 70 of
// them - is three. writeTable sizes every column to its widest cell, so all
// 568 rows were padded out to 161 before their project was printed, and a
// terminal showed a quarter of the result.
//
// ⛔ THE RULE IS brief.go's AND IT WAS RULED FOR BOTH TABLES ON 2026-09-17.
// Three clauses, in the order they apply:
//
//  1. PIN the key column so the prose pays. briefFitAround takes the width out
//     of the widest column that is NOT pinned. The id is pinned: measured at
//     80 columns against the live store, five of six governing ids rendered
//     the identical stub because the cut fell inside a shared prefix. An
//     elided title still reads; an elided id identifies nothing and cannot be
//     pasted into `rig record get`, which is the one thing the column is for.
//  2. CHOOSE THE LAYOUT at a default width when there is no terminal. A layout
//     chosen for a default takes no bytes away, so a pipe's consumer still
//     reads every one.
//  3. CUT a cell only at a REAL terminal. st.Width and not the default, which
//     is the difference between the two numbers below - passing the default to
//     the fit is what put an ellipsis into a pipe on brief.go's first run of
//     this code.
//
// AND THE LAYOUT IS ALL-OR-NOTHING FOR THE WHOLE TABLE, as briefGoverningRows
// already does it: either every id sits in the column, or the id steps out of
// the table for every row and is printed whole beneath it. A per-row choice
// was this file's first answer and it is a SECOND RULE - two shapes of row in
// one table, and a reader scanning a column cannot tell which kind of line
// they are on.
func queryTable(b *strings.Builder, header []string, rows [][]string, width int) {
	keyedTable(b, header, rows, width,
		"An id here is longer than the page, so each is printed whole on its "+
			"own line below its row: a cut id cannot be pasted into "+
			"`rig record get`, and these ids share prefixes long enough that "+
			"cutting would make several of them identical.")
}

// keyedTable is the ruled width rule for any table whose FIRST column is a key.
//
// ⛔ IT IS ONE ANSWER BECAUSE A SECOND ONE WAS ALREADY THE DEFECT. `queryTable`
// and `briefGoverningRows` reached the same three clauses independently, and a
// THIRD copy was owed the day `record refs` needed it - at which point the rule
// would have been stated three times and enforced nowhere. `briefGoverningRows`
// stays where it is: its heading is bespoke and its table is two columns after
// the key, so folding it in would cost more than it saves.
//
// The three clauses, in the order they apply:
//
//  1. PIN the key column so the prose pays. briefFitAround takes the width out
//     of the widest column that is NOT pinned. Measured at 80 columns against
//     the live store, five of six governing ids rendered the identical stub
//     because the cut fell inside a shared prefix. An elided title still
//     reads; an elided id identifies nothing and cannot be pasted into
//     `rig record get`, which is the one thing the column is for.
//  2. CHOOSE THE LAYOUT at a default width when there is no terminal. A layout
//     chosen for a default takes no bytes away, so a pipe's consumer still
//     reads every one.
//  3. CUT a cell only at a REAL terminal. st.Width and not the default -
//     passing the default to the fit is what put an ellipsis into a pipe on
//     brief.go's first run of this code.
//
// AND THE LAYOUT IS ALL-OR-NOTHING FOR THE WHOLE TABLE: either every key sits
// in the column, or the key steps out for EVERY row and is printed whole
// beneath it. A per-row choice was this file's first answer and it is a SECOND
// RULE - two shapes of row in one table, and a reader scanning a column cannot
// tell which kind of line they are on.
//
// `note` is what the step-out form says before it, because a reader meeting the
// two-line shape has to be told it is a layout rather than a defect. It differs
// per table, which is why it is an argument and not a constant here.
func keyedTable(b *strings.Builder, header []string, rows [][]string, width int, note string) {
	if len(header) == 0 {
		return
	}
	// ⛔ TWO NUMBERS, AND CONFLATING THEM IS THE DEFECT CLAUSE 3 NAMES.
	// `budget` chooses the layout and falls back to a default; `width` cuts,
	// and is zero at a pipe so that nothing is cut there.
	budget := width
	if budget <= 0 {
		budget = briefDefaultWrap
	}
	st := briefStyle{Width: width}

	widest := utf8.RuneCountInString(header[0])
	for _, r := range rows {
		if n := utf8.RuneCountInString(r[0]); n > widest {
			widest = n
		}
	}

	// The narrowest the inline form can be without cutting a key: the keys at
	// full width, every middle column at full width, and the last column
	// squeezed to the floor below which a cell is an ellipsis and a letter.
	inline := widest + 2 + briefMinCell
	for i := 1; i < len(header)-1; i++ {
		w := utf8.RuneCountInString(header[i])
		for _, r := range rows {
			if n := utf8.RuneCountInString(r[i]); n > w {
				w = n
			}
		}
		inline += w + 2
	}
	if inline <= budget {
		// ⛔ THE PIN IS THE INVARIANT AND THE CONDITION ABOVE IS ITS PROOF, so
		// do not read the pin as the only thing holding the key whole.
		// `inline` already reserves the key at FULL width with every other
		// column at its floor, so briefFitAround runs out of deficit before
		// the key could become the column it takes from - measured by
		// mutation: removing the pin changed no output. It stays because it
		// states WHICH column is the key, and because a later change to
		// `inline` would otherwise make the key payable with nothing saying
		// it had.
		briefTableAround(b, st, header, rows, map[int]bool{0: true})
		return
	}

	// The key steps out. Every other column keeps its table, so the listing is
	// still scannable down a column; only the key leaves it.
	b.WriteString(briefWrap(note, budget) + "\n")

	// ⛔ THE REMAINING COLUMNS ARE LAID OUT HERE RATHER THAN THROUGH
	// briefTableAround, and it is the duplication briefGoverningRows already
	// accepted for the same reason: the key line has to be indented to the
	// first column, and a renderer that computes its widths privately cannot
	// be asked what they came out as. Recomputing them beside it would be two
	// places deciding one layout.
	cols := make([]int, len(header)-1)
	for i := range cols {
		cols[i] = utf8.RuneCountInString(header[i+1])
		for _, r := range rows {
			if n := utf8.RuneCountInString(r[i+1]); n > cols[i] {
				cols[i] = n
			}
		}
	}
	briefFitAround(cols, st.Width, nil)

	line := func(cells []string) string {
		var out strings.Builder
		for i, cell := range cells {
			cell = briefElide(cell, cols[i])
			// The last column is never padded, for writeTable's reason: a
			// summary otherwise drags trailing spaces across the terminal.
			if i == len(cells)-1 {
				out.WriteString(cell)
				break
			}
			out.WriteString(cell)
			if pad := cols[i] - utf8.RuneCountInString(cell); pad > 0 {
				out.WriteString(strings.Repeat(" ", pad))
			}
			out.WriteString("  ")
		}
		return out.String()
	}

	// ⛔ THE KEY IS INDENTED TO THE SECOND COLUMN AND NEVER PASSED THROUGH
	// briefElide. It is the one cell in this listing that must survive whole,
	// and the indent is what keeps it reading as part of the row above rather
	// than as a row of its own.
	indent := strings.Repeat(" ", cols[0]+2)
	b.WriteString(st.strong(line(header[1:])) + "\n")
	for _, r := range rows {
		b.WriteString(line(r[1:]) + "\n" + indent + r[0] + "\n")
	}
}

// recordSummary is the one line a listing shows for a record.
//
// IT PREFERS THE TYPED FIELDS OVER THE BODY, and that is section 39's own
// ordering rather than a rendering taste: `title` and `description_short` are
// named in the work-item metadata table as "one line" and "one line, for lists
// and briefs", which is this column exactly. The body is the fallback because
// a `note` or a `decision` may carry nothing else.
func recordSummary(r Record) string {
	for _, k := range []string{titleKey, "description_short"} {
		if v := strings.TrimSpace(r.Fields[k]); v != "" {
			return firstLine(v)
		}
	}
	if v := strings.TrimSpace(r.Body); v != "" {
		return firstLine(v)
	}
	// NOT AN EMPTY CELL. A record with no title and no body is a real thing -
	// a link target, or one written by a caller that only set typed fields -
	// and a blank cell reads as a renderer that failed.
	return "(no title, no body)"
}

// firstLine is the first line of a value, marked when there was more.
//
// The marker matters: a summary column silently showing one line of a
// twelve-line body teaches a reader that the record is one line long.
func firstLine(s string) string {
	line, rest, cut := strings.Cut(s, "\n")
	if cut && strings.TrimSpace(rest) != "" {
		return line + " ..."
	}
	return line
}

// historyText is every version of one record, oldest first.
//
// THE PROVENANCE IS THE POINT OF THE VERB, not a decoration on it. Section 39:
// "record.history - every version of one record with its provenance", and the
// reason it exists at all is that "a quotation attributed to Boris that cannot
// be traced to a transcript is a paraphrase until proved otherwise" becomes a
// field rather than an investigation. A history without a seat and a session
// per row would have answered the wrong question.
func historyText(id string, rs []Record, now time.Time, width int) string {
	var b strings.Builder

	// An id with no versions cannot happen through the store - History
	// returns NotFoundError rather than an empty list - so an empty answer
	// here is a wire or a daemon that lost them, and it says so rather than
	// printing a bare header.
	if len(rs) == 0 {
		fmt.Fprintf(&b, "%s has no versions at all, which no record can "+
			"have: the first write is version 1.\n", id)
		b.WriteString("Read this as a defect between rigd and this client, " +
			"not as a record with no history.\n")
		return b.String()
	}

	rows := make([][]string, 0, len(rs))
	for _, r := range rs {
		rows = append(rows, []string{
			"v" + strconv.FormatUint(r.Version, 10),
			displaySeat(r.Prov.Seat),
			// ⛔ NO SESSION COLUMN. See provenanceLine for the measurement and
			// the reopen condition. THIS TABLE IS WHERE THE DEFECT WAS
			// VISIBLE: one person editing one record twice, forty-four seconds
			// apart, rendered as two sessions in a column standing beside the
			// truthful SEAT one - so the reader had a lying column and an
			// honest column side by side and nothing said which was which.
			strconv.FormatUint(r.Prov.Epoch, 10),
			peersAgeCell(provUnix(r.Prov.CreatedAt), now),
			recordSummary(r),
		})
	}
	// ⛔ THE SUMMARY IS PROSE AND writeTable SIZED THE COLUMN TO THE LONGEST
	// ONE, so a single wordy version set the width of every row and the table
	// ran off the page. The house rule instead: the VERSION key is pinned, the
	// summary pays for any squeeze, and a cell is cut only at a real terminal.
	//
	// ⛔ THE PIN CANNOT BIND HERE AND IS STILL WRITTEN. `v12` is four columns
	// and can never be the widest, so briefFitAround would never choose it -
	// but which column is the KEY is a fact about this table rather than about
	// today's data, and the day an id-shaped column arrives the rule has to be
	// already stated. The step-out form is unreachable for the same reason and
	// is not special-cased: an unreachable branch that is correct costs
	// nothing, and a table that grows a long key later gets the right layout
	// with no second reading of this function.
	keyedTable(&b, []string{"VERSION", "SEAT", "EPOCH", "AGE", "SUMMARY"},
		rows, width,
		"A version key here is longer than the page, so each is printed "+
			"whole on its own line below its row.")
	fmt.Fprintf(&b, "\n%s, %d version%s, oldest first.\n", id, len(rs),
		plural(len(rs)))
	return b.String()
}

// refsJSON is the object --json emits for `record refs`.
//
// THE ROW'S KEYS ARE THE WIRE'S OWN FIELD NAMES - src, type, distance, kind,
// title, via - which is section 10's rule about the proto being the contract.
//
// ⛔ THE LIST IS EMITTED TWICE, UNDER `refs` AND UNDER `in`, AND THE
// DUPLICATION IS A MEASURED REPAIR RATHER THAN AN OVERSIGHT.
//
// One thing had three names - the Go field is Refs.Refs, the wire field is
// `refs`, the text header is SRC, and this object said `in` - and no test
// compared them. What that cost, on 2026-09-17: at least three separate
// specialists swept the whole store with `rig record refs --json` keyed on
// `refs`, got a clean `0` from every record, and that 0 MATCHED A STALE FIGURE
// IN THEIR BRIEF. The wrong instrument confirmed itself, and the run's
// most-repeated number was wrong because of it. It was caught by a positive
// control, not by anybody doubting the number.
//
// So `refs` is the primary key and matches the wire, which is what an
// unprepared caller reaches for; `in` is kept because the seats that DID learn
// it are outside this repository and a rename would break them silently, in
// exactly the direction that produced the defect. THE TWO ARE THE SAME SLICE
// and TestTheJSONKeysAreTheWiresOwnFieldNames pins that they cannot drift.
// Dropping `in` belongs to the next wire-breaking change, not to this one.
func refsJSON(r Refs) map[string]any {
	in := make([]map[string]any, 0, len(r.In))
	for _, e := range r.In {
		in = append(in, map[string]any{
			"src":      e.Src,
			"type":     e.Type,
			"via":      e.Via,
			"kind":     e.Kind,
			titleKey:   e.Title,
			"distance": e.Distance,
		})
	}
	cycles := make([][]string, 0, len(r.Cycles))
	for _, c := range r.Cycles {
		if c == nil {
			// [] rather than null, for the reason every other list in this
			// package is: a null cannot be told from a field this build
			// failed to set.
			c = []string{}
		}
		cycles = append(cycles, c)
	}
	return map[string]any{
		"id": r.ID,
		// The depth that was ANSWERED. A reader who did not type --depth
		// cannot otherwise tell a shallow answer from a record nothing cites.
		"depth": r.Depth,
		// THE SAME SLICE UNDER BOTH NAMES. See the comment above; `refs` is
		// the wire's word and `in` is this client's, and a caller that reaches
		// for either must not be told a record is uncited when it is not.
		"refs": in,
		"in":   in,
		// ⛔ WITHOUT THIS KEY A PARTIAL ANSWER IS INDISTINGUISHABLE FROM A
		// COMPLETE ONE, which is the single defect this capability exists to
		// prevent. It is present on EVERY answer rather than only when true,
		// because a key that appears only when something is wrong is a key
		// nobody's parser has a branch for at the moment it first appears.
		"truncated": r.Truncated,
		"cycles":    cycles,
	}
}

// refsText is what points AT a record.
//
// SECTION 39 CALLS THIS THE DIRECTION FILES CANNOT GO, and the rendering has
// to make the direction visible or the verb is just a second listing. Every
// row prints the edge whole - src, type, dst - rather than only the other end,
// because a reader scanning a column of ids has no way to know which side of
// the arrow they are on.
func refsText(r Refs, width int) string {
	var b strings.Builder

	// NOTHING POINTING AT A RECORD IS AN ANSWER AND A LOUD ONE. It is what a
	// reader checking whether a requirement is cited anywhere is looking for,
	// and a blank table would read as a failed call.
	if len(r.In) == 0 {
		// ⛔ THE STRONGEST CLAIM THIS VERB MAKES IS NOT MADE WHEN IT WOULD
		// BE FALSE. "Nothing points at this" is what a reader checking whether
		// a requirement is cited anywhere came for, and a truncated walk is
		// precisely the case where something DOES point at it - a record past
		// the depth bound, or one the project scope crossed and left out. The
		// qualifier two lines below is a correction, and a correction arriving
		// after the claim is how a reader keeps the claim.
		if r.Truncated {
			fmt.Fprintf(&b, "no record within %d hop%s points at %s that this "+
				"answer may show.\n", r.Depth, plural(r.Depth), r.ID)
			b.WriteString("THIS IS NOT AN ABSENCE. The walk was cut short, " +
				"so something outside\nit may well point here. Read the " +
				"qualifier below before concluding\nthe record is uncited.\n")
		} else {
			fmt.Fprintf(&b, "nothing points at %s within %d hop%s.\n",
				r.ID, r.Depth, plural(r.Depth))
			b.WriteString("This is an answer, not a failure: an uncited " +
				"record is exactly what\n`record.refs` exists to make " +
				"visible.\n")
		}
		// ⛔ BOTH QUALIFIERS PRINT ON THE EMPTY PATH TOO, AND THIS IS WHERE
		// THEY MATTER MOST. The sentence above is the strongest claim this
		// verb makes - nothing points at this - and a truncated walk or a
		// cycle is exactly what would make it false. Today's store cannot
		// report a cycle with no edges, which is a fact about the store and
		// not a licence for this renderer to drop a field the wire carries:
		// the asymmetry is how a later change lands silently.
		b.WriteString(refsTruncationLine(r))
		b.WriteString(refsCycleBlock(r.Cycles))
		return b.String()
	}

	rows := make([][]string, 0, len(r.In))
	for _, e := range r.In {
		rows = append(rows, []string{
			e.Src,
			refKindCell(e.Kind),
			e.Type,
			// THE EDGE IS PRINTED WHOLE AND `VIA` IS ITS FAR END. At distance
			// 1 that is the subject itself, which is why the column is
			// labelled VIA rather than DST: past the first hop the reader is
			// looking at what this edge arrives at on the way, and a column
			// called DST would claim every row points straight at the
			// subject.
			e.Via,
			strconv.Itoa(e.Distance),
		})
	}
	// ⛔ SRC IS AN ID AND writeTable PADDED THE COLUMN TO THE LONGEST ONE. The
	// live store's longest id is 161 characters - a doc-key slug derived from
	// two headings - so one such row started every other row's KIND cell at
	// column 163 and the table was unreadable for the 550 rows that did not
	// share it.
	//
	// ⛔ AND AN ELIDED SRC IS WORSE THAN A WIDE ONE, WHICH IS WHY THE KEY IS
	// PINNED RATHER THAN FITTED. These ids share long prefixes, so a cut falls
	// inside the shared part and several rows render as the same stub - an id
	// that cannot be told from its neighbour and cannot be pasted into
	// `rig record get` has stopped doing the one job the column has.
	keyedTable(&b, []string{"SRC", "KIND", "TYPE", "VIA", "HOPS"}, rows, width,
		"A src id here is longer than the page, so each is printed whole on "+
			"its own line below its row: a cut id cannot be pasted into "+
			"`rig record get`, and these ids share prefixes long enough that "+
			"cutting would make several of them identical.")
	b.WriteString(refsTitleBlock(r.In, width))
	// THE VERB AGREES WITH THE SUBJECT. It read "1 edge point at B9" until
	// S5 reported it; plural() gives the noun its s and the verb needs the
	// opposite one.
	fmt.Fprintf(&b, "\n%d edge%s point%s at %s within %d hop%s.\n",
		len(r.In), plural(len(r.In)), verbS(len(r.In)), r.ID,
		r.Depth, plural(r.Depth))
	b.WriteString(refsTruncationLine(r))
	b.WriteString(refsCycleBlock(r.Cycles))
	return b.String()
}

// refsTitleBlock names each record the table printed as an id, so a reader is
// not left holding a column of UUIDv7s.
//
// ⛔ IT IS A BLOCK RATHER THAN A SIXTH COLUMN, AND THAT IS A LEGIBILITY
// DECISION RATHER THAN A TASTE. A title is free prose of any length and the
// five columns above are all short tokens; folded in, one long title sets the
// width for every row and the direction the table exists to show stops being
// scannable. Printed underneath, the ids stay aligned and the titles are read
// only by somebody who needs them.
//
// A REF WITH NO TITLE IS SKIPPED RATHER THAN PRINTED BLANK. The store computes
// a title for every record that has one and the field is genuinely empty for a
// record with neither; a row saying `01927-src  (none)` teaches a reader that
// the block is broken.
// ⛔ THE ID AND THE TITLE GO ON SEPARATE LINES AND THE TITLE IS WRAPPED, AND
// THIS BLOCK WAS THE LAST OVERRUNNING THING ON THE PAGE ONCE THE TABLE WAS
// RULED. Measured against a copy of the production store with six 161-column
// doc-key ids citing one record: the table came down to the rule and these
// lines still rendered at 263, because `id + two spaces + title` is unbounded
// in both halves at once.
//
// One line each is what the width rule already says everywhere else: the ID IS
// NEVER CUT because a cut id cannot be pasted into `rig record get`, and the
// TITLE IS PROSE so it wraps under the indent. Putting the title on the id's
// own line would mean choosing which of the two pays, and the answer is that
// neither has to.
func refsTitleBlock(in []Ref, width int) string {
	seen := map[string]bool{}
	var b strings.Builder
	for _, e := range in {
		if e.Title == "" || seen[e.Src] {
			continue
		}
		seen[e.Src] = true
		// The id whole, on its own line, indented like the table's rows.
		fmt.Fprintf(&b, "  %s\n", e.Src)
		// ⛔ THE TITLE WRAPS AT A DEFAULT WHEN THERE IS NO TERMINAL, WHICH IS
		// NOT THE SAME AS BEING CUT. briefWrapUnits inserts newlines and
		// discards nothing, so a pipe's consumer still reads every byte -
		// the distinction briefStyle's zero value exists to keep.
		b.WriteString(briefWrapUnits(strings.Fields(firstLine(e.Title)),
			"      ", "      ", width))
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n" + b.String()
}

// refsTruncationLine says the answer is PARTIAL, and it is the one line here
// that changes what a reader may conclude from everything above it.
//
// ⛔ IT PRINTS NOTHING WHEN THE ANSWER IS COMPLETE, DELIBERATELY. A line
// reading "complete" on every call trains a reader to skip the place the real
// warning appears - the argument briefBlockedSection makes about cycles, and
// the same one.
func refsTruncationLine(r Refs) string {
	if !r.Truncated {
		return ""
	}
	// ⛔ BOTH WAYS OUT ARE PRINTED, BECAUSE THERE ARE NOW TWO REASONS THE FLAG
	// IS SET AND THE WIRE CARRIES ONE BOOL FOR BOTH. Either the walk stopped
	// at its bound with somewhere still to go, or the project scope kept a
	// record it CROSSED out of the list. Naming only --depth would send a
	// caller to spend a deeper traversal on an answer a deeper traversal
	// cannot change.
	return fmt.Sprintf("\n⛔ TRUNCATED: this is a PARTIAL answer - there is "+
		"more than it shows.\nEither the walk stopped at %d hop%s with "+
		"somewhere still to go, or the\nproject scope kept a record it "+
		"crossed out of the list.\n"+
		"       --depth <n>      follow further\n"+
		"       --cross-project  keep the records the scope left out\n",
		r.Depth, plural(r.Depth))
}

// refsCycleBlock reports the cycles the walk crossed and REFUSES TO RESOLVE
// THEM.
//
// Section 39: detected, reported, ordered around, never resolved. rig does not
// pick an edge to break, because choosing which one is wrong is a judgement
// about the work - so this names the items and carries nothing a reader could
// take as a recommendation. briefBlockedSection says the same thing about the
// `blocks` graph; this one is about the reference graph, and a reader meeting
// either must read the same sentence.
func refsCycleBlock(cycles [][]string) string {
	if len(cycles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nCYCLES crossed while walking backwards\n")
	for _, c := range cycles {
		if len(c) == 0 {
			// A cycle with no items is a report that reports nothing, and it
			// must not render as a decorative warning. Naming the items is
			// the entire requirement.
			b.WriteString("  a cycle was found and its items did not reach " +
				"this client\n")
			continue
		}
		// The first item is repeated at the end, because a cycle written as a
		// flat list reads as a chain and the reader has to be told it closes.
		b.WriteString("  " + strings.Join(append(append([]string{}, c...), c[0]),
			" -> ") + "\n")
	}
	b.WriteString("rig does not pick an edge to break: which reference is " +
		"the wrong one is a\njudgement about the work.\n")
	return b.String()
}

// refKindCell renders the kind of the other end, and an unknown one is a FACT
// rather than a blank - the same argument peersSeatCell makes about an
// unseated peer. Every record has a kind; internal/record refuses a put
// without one. So a blank means it did not reach this renderer.
func refKindCell(kind string) string {
	if kind == "" {
		return "(not said)"
	}
	return kind
}

// ---- the wire ---------------------------------------------------------------

// wireRecord is RecordAPI against a live rigd.
//
// ⛔ IT IS THE ONLY THING IN THIS PACKAGE THAT KNOWS SECTION 39 HAS A
// PROTOBUF, and that is what the RecordAPI interface bought: every renderer,
// every refusal and every argument check above is exercised by a fake, and
// this file is the one place a wire change can reach.
//
// EVERY METHOD GOES THROUGH call(), never through c.Call directly.
// TestEveryWireCallInThisPackageGoesThroughCall walks this package for exactly
// that, and the reason is that call() is where a client.CallError becomes
// rig's refusal type - so a record verb refusing renders the same as `rig
// ping` refusing, which is section 10's promise.
//
// THE METHOD NAMES CARRY THE `rig.` PREFIX. rigd splits a method on its FIRST
// dot into program and command (internal/daemon's splitMethod), so
// "rig.record.put" reaches the `rig` program's `record.put` command. A name
// without the prefix would be read as a PROGRAM called record, and the refusal
// would be about a program that is not connected.
type wireRecord struct{ c *client.Client }

func (w wireRecord) Put(ctx context.Context, a PutArgs) (Record, error) {
	// ⛔ NO Session, Seat OR Epoch IS SENT AND THE REQUEST MESSAGE CANNOT
	// CARRY THEM. PutArgs says why at length: a client that can name a seat
	// can name somebody else's, on the one field section 39 makes
	// load-bearing. The daemon stamps all three from the connection.
	resp := &rigv1.RecordPutResponse{}
	if err := call(ctx, w.c, "rig.record.put", &rigv1.RecordPutRequest{
		Id:        a.ID,
		IfVersion: a.IfVersion,
		Kind:      a.Kind,
		Project:   a.Project,
		Body:      a.Body,
		Fields:    a.Fields,
	}, resp); err != nil {
		return Record{}, err
	}
	return recordFromWire(resp.GetRecord()), nil
}

func (w wireRecord) Get(ctx context.Context, id string, version uint64) (Record, error) {
	// VERSION 0 IS SENT AS 0 AND MEANS HEAD. Not "version zero", which no
	// record has - the first write is version 1 - so there is no value this
	// field could carry that a zero collides with.
	resp := &rigv1.RecordGetResponse{}
	if err := call(ctx, w.c, "rig.record.get", &rigv1.RecordGetRequest{
		Id:      id,
		Version: version,
	}, resp); err != nil {
		return Record{}, err
	}
	return recordFromWire(resp.GetRecord()), nil
}

// Query walks every page and answers the whole set, because B116 made an
// answer arrive in more than one frame.
//
// ⛔ THE PAGES ARE THE WIRE'S AND THEY STOP HERE. Nothing above this
// function knows a page exists: there is no flag, no cursor in QueryArgs and
// no change to what the renderers are handed, which is what lets `cmd/rigseed`
// and every other caller stay exactly as they were. Section 50, "B116, the
// answer is paged".
//
// Against a daemon built before B116 the answer carries no `next`, so this is
// one call - the loop costs nothing and needs no version check.
func (w wireRecord) Query(ctx context.Context, a QueryArgs) ([]Record, error) {
	var (
		out   []Record
		after string
	)
	for {
		resp := &rigv1.RecordQueryResponse{}
		if err := call(ctx, w.c, "rig.record.query", &rigv1.RecordQueryRequest{
			Project: a.Project,
			Kind:    a.Kind,
			Field:   a.Field,
			Value:   a.Value,
			After:   after,
		}, resp); err != nil {
			return nil, err
		}
		out = append(out, recordsFromWire(resp.GetRecords())...)

		next := resp.GetNext()
		if next == "" {
			return out, nil
		}

		// ⛔ A CURSOR THAT DOES NOT MOVE IS REFUSED RATHER THAN FOLLOWED.
		// A daemon that answers the same cursor twice - or a page with no
		// records and a cursor still set - is an unbounded loop in this
		// process, and a client that hangs on a malformed answer is worse
		// than one that says what it got. The daemon guarantees neither can
		// happen; this is the caller not taking that on trust.
		if next == after {
			return nil, fmt.Errorf(
				"rig answered the same page cursor twice for %s, so the "+
					"listing cannot advance. This is a rig defect, not a "+
					"bad argument: report the query that produced it",
				queryScope(a, len(out)))
		}
		after = next
	}
}

func (w wireRecord) History(ctx context.Context, id string) ([]Record, error) {
	resp := &rigv1.RecordHistoryResponse{}
	if err := call(ctx, w.c, "rig.record.history",
		&rigv1.RecordHistoryRequest{Id: id}, resp); err != nil {
		return nil, err
	}
	return recordsFromWire(resp.GetVersions()), nil
}

func (w wireRecord) Link(ctx context.Context, src, linkType, dst string) error {
	// THE TYPE IS NOT VALIDATED HERE. The set is closed at section 39's eight
	// names and the STORE refuses an unknown one, quoting the value back and
	// listing the valid ones - and unlike a step's state, a link type crosses
	// this wire as a STRING, so the caller's own word survives to the refusal
	// that names it. That is the whole difference between this method and
	// Step below, and it is why one pre-validates and the other does not.
	return call(ctx, w.c, "rig.record.link", &rigv1.RecordLinkRequest{
		Src: src, Type: linkType, Dst: dst,
	}, &rigv1.RecordLinkResponse{})
}

func (w wireRecord) Unlink(ctx context.Context, src, linkType, dst string) error {
	return call(ctx, w.c, "rig.record.unlink", &rigv1.RecordUnlinkRequest{
		Src: src, Type: linkType, Dst: dst,
	}, &rigv1.RecordUnlinkResponse{})
}

func (w wireRecord) Retract(ctx context.Context, id, reason string) (Retraction, bool, error) {
	var resp rigv1.RecordRetractResponse
	if err := call(ctx, w.c, "rig.record.retract",
		&rigv1.RecordRetractRequest{Id: id, Reason: reason}, &resp); err != nil {
		return Retraction{}, false, err
	}
	r := retractionFromWire(resp.GetRetraction())
	if r == nil {
		// ⛔ A SUCCESSFUL RETRACT THAT CARRIES NO WITHDRAWAL IS A SKEW AND NOT
		// AN ANSWER. The zero value would read as "withdrawn, and nobody said
		// anything about it", which is the reassuring direction: the caller
		// would believe the record is gone from every list on the strength of
		// a message that said nothing.
		return Retraction{}, false, fmt.Errorf(
			"rig: the daemon reported %s retracted and sent no withdrawal, so "+
				"there is nothing to say who withdrew it or why", id)
	}
	return *r, resp.GetAlready(), nil
}

func (w wireRecord) Delete(ctx context.Context, id string, dryRun bool) (Deletion, error) {
	var resp rigv1.RecordDeleteResponse
	if err := call(ctx, w.c, "rig.record.delete",
		&rigv1.RecordDeleteRequest{Id: id, DryRun: dryRun}, &resp); err != nil {
		return Deletion{}, err
	}
	// ⛔ dry_run IS READ BACK OFF THE ANSWER RATHER THAN CARRIED FROM THE
	// REQUEST. What the caller ASKED for and what the daemon DID are two facts,
	// and a renderer that printed the request's flag would say "nothing was
	// written" on the strength of its own intention.
	return Deletion{
		ID: resp.GetId(), Versions: resp.GetVersions(),
		Edges: edgesFromWire(resp.GetEdges()), DryRun: resp.GetDryRun(),
	}, nil
}

func (w wireRecord) Replace(ctx context.Context, old, replacement, reason string) (Replacement, error) {
	var resp rigv1.RecordReplaceResponse
	if err := call(ctx, w.c, "rig.record.replace", &rigv1.RecordReplaceRequest{
		Old: old, New: replacement, Reason: reason,
	}, &resp); err != nil {
		return Replacement{}, err
	}
	return Replacement{
		Old: resp.GetOld(), New: resp.GetNew(),
		Moved:      edgesFromWire(resp.GetMoved()),
		Merged:     edgesFromWire(resp.GetMerged()),
		Dropped:    edgesFromWire(resp.GetDropped()),
		Retraction: retractionFromWire(resp.GetRetraction()),
	}, nil
}

func retractionFromWire(r *rigv1.Retraction) *Retraction {
	if r == nil {
		return nil
	}
	return &Retraction{
		ID: r.GetId(), Reason: r.GetReason(), ReplacedBy: r.GetReplacedBy(),
		Prov: provFromWire(r.GetProv()),
	}
}

func edgesFromWire(es []*rigv1.Edge) []Edge {
	if len(es) == 0 {
		return nil
	}
	out := make([]Edge, 0, len(es))
	for _, e := range es {
		out = append(out, Edge{Src: e.GetSrc(), Type: e.GetType(), Dst: e.GetDst()})
	}
	return out
}

func (w wireRecord) Refs(ctx context.Context, a RefsArgs) (Refs, error) {
	// THE DEPTH IS NOT CLAMPED ON THE WAY OUT. A depth above the store's
	// maximum is REFUSED BY NAME there, because a caller that asked for 8, got
	// 5 and was not told has a partial answer that looks complete - the one
	// failure this capability exists to prevent. A client that clamped would
	// be the thing hiding it.
	//
	// The int is bounded before the conversion because an unchecked negative
	// becomes an enormous positive, and this field is read as "how many hops"
	// - the one direction in which a wrong number looks plausible. A negative
	// cannot arrive: recordRefs refuses a typed depth below 1. The guard is
	// what lets a reader see that without holding that function open.
	req := &rigv1.RecordRefsRequest{Id: a.ID, CrossProject: a.CrossProject}
	if a.Depth > 0 && a.Depth <= math.MaxUint32 {
		req.Depth = uint32(a.Depth)
	}
	resp := &rigv1.RecordRefsResponse{}
	if err := call(ctx, w.c, "rig.record.refs", req, resp); err != nil {
		return Refs{}, err
	}
	out := Refs{
		ID: resp.GetId(),
		// The depth ANSWERED, which is not always the depth asked - and after
		// the default-meaning zero above, is usually not even a number this
		// caller sent.
		Depth:     int(resp.GetDepth()),
		Truncated: resp.GetTruncated(),
	}
	for _, r := range resp.GetRefs() {
		out.In = append(out.In, Ref{
			Src:      r.GetSrc(),
			Type:     r.GetType(),
			Via:      r.GetVia(),
			Kind:     r.GetKind(),
			Title:    r.GetTitle(),
			Distance: int(r.GetDistance()),
		})
	}
	for _, c := range resp.GetCycles() {
		out.Cycles = append(out.Cycles, c.GetItems())
	}
	return out, nil
}

func (w wireRecord) Step(ctx context.Context, a StepArgs) (Record, error) {
	state, err := stepStateOnTheWire(a.State)
	if err != nil {
		return Record{}, err
	}
	// ⛔ THE PROJECT IS NOT SENT AND THE REQUEST MESSAGE HAS NO FIELD FOR IT.
	// The daemon DERIVES it from the item, because a step's project is a fact
	// about the item rather than a choice the caller makes. StepArgs.Project
	// is still parsed and still refused when empty - by rigd, which needs the
	// item to exist before it can say anything about a project at all.
	resp := &rigv1.ProgressStepResponse{}
	if err := call(ctx, w.c, "rig.progress.step", &rigv1.ProgressStepRequest{
		Item:  a.Item,
		State: state,
		Note:  a.Note,
	}, resp); err != nil {
		return Record{}, err
	}
	return recordFromWire(resp.GetStep()), nil
}

func (w wireRecord) Brief(ctx context.Context, project string) (Brief, error) {
	resp := &rigv1.ProjectBriefResponse{}
	if err := call(ctx, w.c, "rig.project.brief",
		&rigv1.ProjectBriefRequest{Project: project}, resp); err != nil {
		return Brief{}, err
	}
	return briefFromWire(resp), nil
}

// ---- the step state, and why this client refuses one it cannot spell -------

// stepStateOnTheWire turns the caller's word into the enum, and REFUSES A WORD
// THIS BUILD HAS NO VALUE FOR.
//
// ⛔ THIS OVERTURNS progress.go's "the client does not pre-validate", AND THE
// ENUM IS WHY. That rule was right and its premise has stopped being true.
// It said rigd "refuses an unknown state by name and quotes the value back",
// so a second validator here could only drift. But `state` crosses this wire
// as an ENUM: `--state banana` has no value, arrives as UNSPECIFIED, and
// internal/daemon maps UNSPECIFIED to the empty string - so the store refuses
// `"" is not a step state` and QUOTES THE WRONG VALUE BACK. The caller's word
// cannot survive the enum, which makes this client the last place it exists.
//
// ⛔ AND IT IS NOT A SECOND SOURCE OF TRUTH, WHICH IS THE HALF THAT MAKES IT
// SAFE. The set is WALKED off the enum's own descriptor rather than written
// down here, so a fourth state added to the proto is offered by this build the
// same day, and one removed stops being offered. A hand-kept table would have
// been the thing progress.go was right to refuse.
//
// An EMPTY --state is passed through rather than refused, deliberately: rigd
// already answers that one in a sentence naming the flag, and there is no
// caller word to lose.
func stepStateOnTheWire(word string) (rigv1.StepState, error) {
	if word == "" {
		return rigv1.StepState_STEP_STATE_UNSPECIFIED, nil
	}
	values := rigv1.StepState_STEP_STATE_UNSPECIFIED.Descriptor().Values()
	var known []string
	for i := range values.Len() {
		v := values.Get(i)
		label := enumLabel(string(v.Name()), "STEP_STATE_")
		// THE ZERO IS NOT OFFERED AND NOT ACCEPTED. Section 21: its meaning is
		// "nothing was said", so a caller spelling it would be asking for an
		// unset field, which is what leaving the flag off already does.
		if v.Number() == 0 {
			continue
		}
		if label == word {
			return rigv1.StepState(v.Number()), nil
		}
		known = append(known, label)
	}
	return rigv1.StepState_STEP_STATE_UNSPECIFIED, badArgumentf(
		"%q is not a step state; this build knows %s.\n"+
			"       rig refuses it here rather than sending it, because the "+
			"state travels as an enum: a word with no value arrives at rigd "+
			"as nothing at all, and the refusal you would get back quotes an "+
			"empty string instead of what you typed",
		word, strings.Join(known, ", "))
}

// ---- the wire's nouns, in this client's vocabulary -------------------------

// recordFromWire is one record. A NIL MESSAGE YIELDS A ZERO RECORD rather than
// a panic, and the renderers above are built to report a zero as a defect -
// provenanceLine says "(not said)" and provTime returns "" for a stamp that
// never arrived. A daemon that replies with an empty payload is a bug, and the
// surface that shows it must not be a crash.
func recordFromWire(r *rigv1.Record) Record {
	return Record{
		ID:      r.GetId(),
		Version: r.GetVersion(),
		Kind:    r.GetKind(),
		Project: r.GetProject(),
		Body:    r.GetBody(),
		Fields:  r.GetFields(),
		Prov:    provFromWire(r.GetProv()),

		// B77. Absent on every record that is not withdrawn, which is every
		// record any verb but `get` can return.
		Retraction: retractionFromWire(r.GetRetraction()),
	}
}

func recordsFromWire(rs []*rigv1.Record) []Record {
	if len(rs) == 0 {
		// nil rather than an empty slice, because every renderer above
		// branches on len() and recordsJSON already builds the [] a consumer
		// sees. Two ways to spell "none" in one package is one too many.
		return nil
	}
	out := make([]Record, 0, len(rs))
	for _, r := range rs {
		out = append(out, recordFromWire(r))
	}
	return out
}

// provFromWire is who wrote a version and when.
//
// ⛔ A ZERO TIMESTAMP STAYS THE ZERO time.Time AND IS NEVER time.Unix(0, 0).
// The Unix epoch renders as 1970, which reads as a date rather than as an
// absence, and provTime's whole job is to tell those apart. internal/record
// refuses to write a record without provenance, so a zero arriving here is a
// defect between the store and this client - which is a thing to report, not a
// year to hand the reader.
func provFromWire(p *rigv1.Provenance) Provenance {
	out := Provenance{
		Session: p.GetSession(),
		Seat:    p.GetSeat(),
		Epoch:   p.GetEpoch(),
	}
	if n := p.GetAtUnixNano(); n != 0 {
		out.CreatedAt = time.Unix(0, n).UTC()
	}
	return out
}

// ---- B77: retract, delete and replace --------------------------------------

// recordRetract withdraws a record. The id and the history survive.
func recordRetract(rf *recordFlags, rest []string) error {
	if len(rest) != 1 {
		return badArgumentf("usage: rig record retract <id> [--reason <why>]\n" +
			"       the record leaves every brief and every query; its id and " +
			"every version of it stay, and `rig record get` still answers")
	}
	id := rest[0]

	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		r, already, err := api.Retract(ctx, id, *rf.reason)
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"id": r.ID, "reason": r.Reason, "already": already,
				"retracted_by": map[string]any{
					"session": r.Prov.Session, "seat": r.Prov.Seat,
					"epoch": r.Prov.Epoch, "at": provTime(r.Prov.CreatedAt),
				},
			})
		}
		// ⛔ "ALREADY" IS A DIFFERENT SENTENCE AND NOT A QUIETER ONE. The
		// caller's reason was NOT recorded, and a line reading "retracted" over
		// somebody else's withdrawal would leave them believing theirs was.
		if already {
			fmt.Printf("%s was ALREADY retracted, and this call changed "+
				"nothing.\n", id)
			fmt.Printf("  withdrawn by %s (%s) at %s\n",
				briefCell(r.Prov.Seat, "(no seat)"),
				briefCell(r.Prov.Session, "(no session)"),
				provTime(r.Prov.CreatedAt))
			fmt.Printf("  reason: %s\n", briefCell(r.Reason, "(none given)"))
			if *rf.reason != "" && *rf.reason != r.Reason {
				fmt.Printf("  ⛔ YOUR REASON WAS NOT RECORDED: %q\n", *rf.reason)
			}
			return nil
		}
		fmt.Printf("retracted %s\n", id)
		fmt.Printf("  reason: %s\n", briefCell(r.Reason, "(none given)"))
		fmt.Printf("  it leaves every brief and query; `rig record get %s` "+
			"still explains it\n", id)
		return nil
	})
}

// recordDelete removes a record, its versions and every edge touching it.
//
// ⛔ THE OUTPUT IS THE OBLIGATION. Boris ruled 2026-09-17, against the lead's
// recommendation and with the cost stated, that delete DROPS the edges rather
// than refusing while anything cites the record - so the account of what it
// took is not a courtesy. A destructive verb that answers `deleted` and nothing
// else is B75's shape, success reported over data loss.
func recordDelete(rf *recordFlags, rest []string) error {
	if len(rest) != 1 {
		return badArgumentf("usage: rig record delete <id> [--dry-run]\n" +
			"       DESTRUCTIVE: the record, every version of it and every " +
			"edge touching it go.\n" +
			"       --dry-run prints exactly that list and writes nothing")
	}
	id := rest[0]

	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		d, err := api.Delete(ctx, id, *rf.dryRun)
		if err != nil {
			return err
		}
		if *rf.asJSON {
			edges := make([]map[string]any, 0, len(d.Edges))
			for _, e := range d.Edges {
				edges = append(edges, map[string]any{
					"src": e.Src, "type": e.Type, "dst": e.Dst,
				})
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"id": d.ID, "versions": d.Versions,
				"edges": edges, "dry_run": d.DryRun,
			})
		}

		lead := "deleted"
		if d.DryRun {
			lead = "DRY RUN - nothing was written. This would delete"
		}
		fmt.Printf("%s %s\n", lead, d.ID)
		fmt.Printf("  %d version(s) of the record\n", d.Versions)

		// ⛔ THE EDGES ARE LISTED AND NOT COUNTED, AND THE EMPTY CASE SAYS SO.
		// "0 edges" printed as nothing at all is indistinguishable from a
		// renderer that forgot the section, which is the absent-versus-empty
		// failure this whole project is built against.
		if len(d.Edges) == 0 {
			fmt.Printf("  no edges: nothing pointed at it and it pointed at nothing\n")
			return nil
		}
		fmt.Printf("  %d edge(s) dropped, in both directions:\n", len(d.Edges))
		for _, e := range d.Edges {
			fmt.Printf("    %s\n", e)
		}
		// ⛔ THE COST HE ACCEPTED, SAID OUT LOUD AT THE MOMENT IT IS PAID.
		// Deleting a parent leaves its children with no record that they ever
		// had one, and the next brief renders them as top-level items as though
		// that were intended. He took that cost knowingly; the caller standing
		// here may not have.
		fmt.Printf("  ⛔ nothing records that these edges existed. A child " +
			"whose parent went reads as top-level from here on.\n")
		return nil
	})
}

// recordReplace puts a DIFFERENT record in the place of an existing one.
func recordReplace(rf *recordFlags, rest []string) error {
	if len(rest) != 2 {
		return badArgumentf("usage: rig record replace <old> <new> [--reason <why>]\n" +
			"       the duplicate case: every edge pointing at <old> moves onto " +
			"<new>,\n" +
			"       and <old> is withdrawn pointing at it. A supersede cannot " +
			"express this,\n" +
			"       because a supersede keeps the id")
	}
	oldID, newID := rest[0], rest[1]

	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		r, err := api.Replace(ctx, oldID, newID, *rf.reason)
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"old": r.Old, "new": r.New,
				"moved":   edgesJSON(r.Moved),
				"merged":  edgesJSON(r.Merged),
				"dropped": edgesJSON(r.Dropped),
				"retraction": map[string]any{
					"reason": retractionReason(r.Retraction),
				},
			})
		}

		fmt.Printf("%s replaces %s\n", r.New, r.Old)
		// ⛔ THREE BUCKETS, EACH PRINTED EVEN WHEN EMPTY, AND THAT IS THE WHOLE
		// REPORT. They are disjoint and exhaustive over the loser's inbound
		// edges: a replace that printed only what moved would leave a caller
		// believing every edge survived, which is false for the two other
		// buckets and is the same reassuring lie the delete report refuses.
		edgeBucket("moved onto "+r.New, r.Moved,
			"nothing pointed at "+r.Old)
		edgeBucket("merged - "+r.New+" already had", r.Merged, "none")
		edgeBucket("DROPPED - would have been a self-edge on "+r.New,
			r.Dropped, "none")
		fmt.Printf("  %s is withdrawn: %s\n", r.Old, retractionReason(r.Retraction))
		fmt.Printf("  `rig record get %s` still says where the fact went\n", r.Old)
		return nil
	})
}

// edgeBucket prints one bucket of a replacement's account, INCLUDING when it
// is empty: a section that vanishes reads as a renderer that forgot it.
func edgeBucket(label string, es []Edge, empty string) {
	if len(es) == 0 {
		fmt.Printf("  %s: %s\n", label, empty)
		return
	}
	fmt.Printf("  %s (%d):\n", label, len(es))
	for _, e := range es {
		fmt.Printf("    %s\n", e)
	}
}

func edgesJSON(es []Edge) []map[string]any {
	out := make([]map[string]any, 0, len(es))
	for _, e := range es {
		out = append(out, map[string]any{"src": e.Src, "type": e.Type, "dst": e.Dst})
	}
	return out
}

// retractionReason is what a withdrawal says, or a named absence.
func retractionReason(r *Retraction) string {
	if r == nil {
		// Reachable only from a daemon that served a replacement without the
		// withdrawal that finished it, which is a skew rather than a state.
		return "(the daemon sent no withdrawal with this replacement)"
	}
	return briefCell(r.Reason, "(no reason given)")
}
