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

	"github.com/boris-milner/rig/client"
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

// kindKey is the JSON key for a record's kind, for titleKey's reason and for
// one more: B64 added two section-12 objects that each carry a kind, which took
// the bare literal past goconst's ceiling of six in this package. ⛔ THE FLAG
// NAMES AND THE POSITIONAL LABEL SPELLED "kind" ARE DELIBERATELY NOT THIS
// CONSTANT. They are a different thing that happens to share a spelling - a
// flag is part of a command's surface and a JSON key is part of an answer's -
// and collapsing them would mean a rename of one silently renaming the other.
const kindKey = "kind"

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
	fields       *fieldFlag
	field        *string
	value        *string
	ifVersion    *uint64
	version      *uint64
	depth        *int
	crossProject *bool
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
		r.fields = &fieldFlag{}
		r.fs.Var(r.fields, "field", "a typed field, key=value; repeatable")
		r.ifVersion = r.fs.Uint64("if-version", 0,
			"the version you believe is current; 0 creates")
	case "get":
		r.version = r.fs.Uint64("version", 0, "a version to read; 0 is the head")
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

// recordSubcommands are the seven, in the order the usage line prints them.
//
// Read by complete.go and by the usage text below, so adding one is a line
// here rather than three places that can disagree - the coupling complete.go
// calls "one string in three places that MUST agree".
var recordSubcommands = []string{
	"put", "get", "query", "history", "link", "unlink", "refs",
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
			"<project> [--id <id>] [--body <text>] [--field k=v] " +
			"[--if-version <n>]")
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

	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		rec, err := api.Put(ctx, PutArgs{
			ID:        *rf.id,
			IfVersion: *rf.ifVersion,
			Kind:      *rf.kind,
			Project:   *rf.project,
			Body:      *rf.body,
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
		fmt.Print(queryText(a, recs))
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
		fmt.Print(historyText(rest[0], recs, time.Now()))
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
		fmt.Print(refsText(refs))
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
	}
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
func queryText(a QueryArgs, rs []Record) string {
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
	writeTable(&b, header, rows)
	fmt.Fprintf(&b, "\n%d %s.\n", len(rs), scope)
	return b.String()
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
func historyText(id string, rs []Record, now time.Time) string {
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
	writeTable(&b, []string{"VERSION", "SEAT", "EPOCH", "AGE", "SUMMARY"}, rows)
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
func refsText(r Refs) string {
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
	writeTable(&b, []string{"SRC", "KIND", "TYPE", "VIA", "HOPS"}, rows)
	b.WriteString(refsTitleBlock(r.In))
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
func refsTitleBlock(in []Ref) string {
	seen := map[string]bool{}
	var b strings.Builder
	for _, e := range in {
		if e.Title == "" || seen[e.Src] {
			continue
		}
		seen[e.Src] = true
		fmt.Fprintf(&b, "  %s  %s\n", e.Src, firstLine(e.Title))
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

func (w wireRecord) Query(ctx context.Context, a QueryArgs) ([]Record, error) {
	resp := &rigv1.RecordQueryResponse{}
	if err := call(ctx, w.c, "rig.record.query", &rigv1.RecordQueryRequest{
		Project: a.Project,
		Kind:    a.Kind,
		Field:   a.Field,
		Value:   a.Value,
	}, resp); err != nil {
		return nil, err
	}
	return recordsFromWire(resp.GetRecords()), nil
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
