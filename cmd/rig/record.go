package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
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
// WHAT THIS FILE IS NOT. It is not the wire. Every call below goes through
// RecordAPI, which the lead implements against proto/ as one deliberate
// change; nothing here knows a protobuf exists, which is what let the CLI and
// the wire be built at the same time by two seats.

// ---- the contract with rigd ------------------------------------------------

// RecordAPI is everything the CLI needs from rigd for section 39's verbs.
//
// ⛔ THE LEAD IMPLEMENTS THIS AND THIS SEAT DOES NOT. The interface is the
// contract between the CLI and the wire, agreed before either existed, so the
// two proceed in parallel: proto/rig/v1 is the lead's SECOND COLLISION
// (COORDINATION.md) and a CLI that waited for it would have waited for a file
// it may not open.
//
// It is deliberately NOT a *client.Client wrapper. A CLI built against the
// concrete client is a CLI whose every verb needs a live daemon to test, and
// cmd/rig already documents the fifteen functions in that hole. Against an
// interface, every renderer and every refusal below is exercised by a fake.
type RecordAPI interface {
	// Put creates a record or supersedes one, and returns what was written.
	Put(ctx context.Context, a PutArgs) (Record, error)

	// Get returns one record. VERSION 0 MEANS HEAD - it is not "version
	// zero", which no record has, because the first write is version 1.
	Get(ctx context.Context, id string, version uint64) (Record, error)

	// Query returns the head of every record of a kind in a project. This is
	// section 39's "indexed".
	Query(ctx context.Context, project, kind string) ([]Record, error)

	// History returns every version of one record, oldest first, with its
	// provenance.
	History(ctx context.Context, id string) ([]Record, error)

	// Link and Unlink write and remove a typed, directed edge. This is
	// section 39's "linked".
	Link(ctx context.Context, src, linkType, dst string) error
	Unlink(ctx context.Context, src, linkType, dst string) error

	// Refs is what points AT a record: the direction files cannot go, and
	// section 39's "correlated".
	Refs(ctx context.Context, id string, depth int) (Refs, error)

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

// Ref is one edge, as `record.refs` reports it.
//
// ⛔ PROVISIONAL UNTIL SLICE 2. Refs and Ref are shaped by what this renderer
// needs, because slice 1 puts no wire under them: section 39 specifies the
// verb and its meaning and not its response message. When the wire lands, the
// lead's message is the contract and these follow it.
type Ref struct {
	// Src, Type and Dst are the edge as the store holds it. The edge is
	// DIRECTED and both ends are printed, because "what points at this" is
	// only readable if the reader can see which end they are looking at.
	Src  string
	Type string
	Dst  string

	// Kind is the kind of the OTHER end, so a reader can tell a decision
	// citing a requirement from a work-item implementing one without a second
	// call per row.
	Kind string

	// Depth is how many hops this edge is from the record that was asked
	// about. 1 is a direct reference.
	Depth int
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

// openRecordAPI is the default, and today it refuses.
//
// ⛔ IT SAYS THE WIRE IS MISSING RATHER THAN PRETENDING TO DIAL. The
// alternative - connect() and then fail - would report "is rigd running?" for
// a daemon that is running perfectly and simply does not serve these methods
// yet, which sends the reader to start a daemon that is already up. A refusal
// that names the wrong cause is worse than one that names none.
func openRecordAPI() (RecordAPI, func(), error) {
	return nil, nil, notWired()
}

// codeNoRecordWire is rig's own code for "this build has the verb and nothing
// to call". It is spelled with codeLocal's prefix, so it is disjoint from
// every CODE_* the daemon can send by construction rather than by review.
const codeNoRecordWire = codeLocal + "NO_RECORD_WIRE"

// notWired is the refusal every record verb gives until the wire lands.
//
// FixCommand is EMPTY ON PURPOSE. internal/kernel/refusal.go's rule is
// "runnable as written, or empty. NEVER prose", and there is no command a
// caller can run that puts these methods on the wire.
func notWired() error {
	return local(jsonStatus{
		Code: codeNoRecordWire,
		Message: "rig does not have the record verbs on the wire yet: the CLI " +
			"is built and rigd serves nothing to call",
		Precondition: "rigd answers record.put, record.get, record.query, " +
			"record.history, record.link, record.unlink, record.refs, " +
			"progress.step and project.brief",
		Actual: "this build of rig can ask and this daemon cannot answer",
		Fix: "the record verbs reach the wire as one deliberate change " +
			"(PLAN.md section 39, slice 1). Until then these verbs cannot answer",
	})
}

// ---- the flags -------------------------------------------------------------

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

	id        *string
	kind      *string
	project   *string
	body      *string
	fields    *fieldFlag
	ifVersion *uint64
	version   *uint64
	depth     *int
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
	case "refs":
		r.depth = r.fs.Int("depth", 1, "how many hops to follow backwards")
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

func recordQuery(rf *recordFlags, rest []string) error {
	if len(rest) != 2 {
		return badArgumentf("usage: rig record query <project> <kind>")
	}
	project, kind := rest[0], rest[1]
	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		recs, err := api.Query(ctx, project, kind)
		if err != nil {
			return err
		}
		if *rf.asJSON {
			return json.NewEncoder(os.Stdout).Encode(recordsJSON(recs, time.Now()))
		}
		fmt.Print(queryText(project, kind, recs))
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
	if *rf.depth < 1 {
		// Depth 0 would answer "nothing", which is indistinguishable from a
		// record nothing cites - the one answer this verb exists to make
		// readable.
		return badArgumentf("--depth %d follows no edge at all, so its answer "+
			"could not be told from a record nothing points at. The smallest "+
			"useful depth is 1", *rf.depth)
	}
	return withRecordAPI(*rf.timeout, func(ctx context.Context, api RecordAPI) error {
		refs, err := api.Refs(ctx, rest[0], *rf.depth)
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
// ⛔ AN EMPTY SESSION OR SEAT IS A DEFECT AND RENDERS AS ONE. internal/record
// refuses a put without both - "a put needs its provenance: session and seat"
// - so a blank cell here cannot mean "nobody claimed it". It means something
// between the store and this renderer dropped it, and section 21's rule about
// the zero applies exactly: nothing was said is never a fact about anything.
func provenanceLine(p Provenance, now time.Time) string {
	return fmt.Sprintf("written by %s in session %s, epoch %d, %s\n",
		provWord(p.Seat), provWord(p.Session), p.Epoch,
		provWhen(p.CreatedAt, now))
}

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

// queryText is a listing of one kind in one project.
func queryText(project, kind string, rs []Record) string {
	var b strings.Builder

	// AN EMPTY RESULT IS A SENTENCE, NOT A BLANK TABLE, and it names both
	// halves of what was asked. peersText's rule: a bare header over nothing
	// reads as a broken command, and the reader of an empty answer is usually
	// somebody who expected rows.
	if len(rs) == 0 {
		fmt.Fprintf(&b, "no %s records in %s.\n", kind, project)
		b.WriteString("A row appears when something calls record.put with " +
			"that kind and project;\nboth are matched exactly, so a kind " +
			"spelled differently is a different kind.\n")
		return b.String()
	}

	rows := make([][]string, 0, len(rs))
	for _, r := range rs {
		rows = append(rows, []string{
			r.ID,
			"v" + strconv.FormatUint(r.Version, 10),
			r.Kind,
			recordSummary(r),
		})
	}
	writeTable(&b, []string{"ID", "VERSION", "KIND", "SUMMARY"}, rows)
	fmt.Fprintf(&b, "\n%d %s record%s in %s.\n", len(rs), kind, plural(len(rs)),
		project)
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
	for _, k := range []string{"title", "description_short"} {
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
			provWord(r.Prov.Seat),
			provWord(r.Prov.Session),
			strconv.FormatUint(r.Prov.Epoch, 10),
			peersAgeCell(provUnix(r.Prov.CreatedAt), now),
			recordSummary(r),
		})
	}
	writeTable(&b, []string{"VERSION", "SEAT", "SESSION", "EPOCH", "AGE", "SUMMARY"}, rows)
	fmt.Fprintf(&b, "\n%s, %d version%s, oldest first.\n", id, len(rs),
		plural(len(rs)))
	return b.String()
}

// refsJSON is the object --json emits for `record refs`.
func refsJSON(r Refs) map[string]any {
	in := make([]map[string]any, 0, len(r.In))
	for _, e := range r.In {
		in = append(in, map[string]any{
			"src":   e.Src,
			"type":  e.Type,
			"dst":   e.Dst,
			"kind":  e.Kind,
			"depth": e.Depth,
		})
	}
	return map[string]any{
		"id": r.ID,
		// The depth that was ANSWERED. A reader who did not type --depth
		// cannot otherwise tell a shallow answer from a record nothing cites.
		"depth": r.Depth,
		"in":    in,
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
		fmt.Fprintf(&b, "nothing points at %s within %d hop%s.\n",
			r.ID, r.Depth, plural(r.Depth))
		b.WriteString("This is an answer, not a failure: an uncited record " +
			"is exactly what\n`record.refs` exists to make visible.\n")
		return b.String()
	}

	rows := make([][]string, 0, len(r.In))
	for _, e := range r.In {
		rows = append(rows, []string{
			e.Src,
			refKindCell(e.Kind),
			e.Type,
			e.Dst,
			strconv.Itoa(e.Depth),
		})
	}
	writeTable(&b, []string{"SRC", "KIND", "TYPE", "DST", "HOPS"}, rows)
	fmt.Fprintf(&b, "\n%d edge%s point at %s within %d hop%s.\n",
		len(r.In), plural(len(r.In)), r.ID, r.Depth, plural(r.Depth))
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
