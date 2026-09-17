package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The fake is the whole reason this surface is testable. Every record verb
// needs a live rigd, and cmd/rig already documents the fifteen functions in
// that hole - so the CLI is built against RecordAPI and the tests drive it
// through this.
type fakeRecord struct {
	// calls is what was invoked, in order, so a test can assert that a
	// refusal happened BEFORE the wire was reached. "it refused" and "it
	// refused without calling" are different claims and only the second one
	// says the guard is in the client.
	calls []string

	lastPut  PutArgs
	lastStep StepArgs
	lastGet  struct {
		id      string
		version uint64
	}
	lastRefs RefsArgs

	put   func(PutArgs) (Record, error)
	get   func(string, uint64) (Record, error)
	query func(QueryArgs) ([]Record, error)
	// lastFilter is the whole filter Query was handed, so a test can assert
	// that the flags reached the API rather than only that it was called.
	// Named apart from the `queried` helper below, which returns only the
	// project and the kind and predates the field predicate.
	lastFilter QueryArgs
	history    func(string) ([]Record, error)
	refs       func(RefsArgs) (Refs, error)
	step       func(StepArgs) (Record, error)
	brief      func(string) (Brief, error)
	linkErr    error
}

func (f *fakeRecord) Put(_ context.Context, a PutArgs) (Record, error) {
	f.calls = append(f.calls, "put")
	f.lastPut = a
	if f.put != nil {
		return f.put(a)
	}
	return Record{
		ID: "01927-put", Version: a.IfVersion + 1, Kind: a.Kind,
		Project: a.Project, Body: a.Body, Fields: a.Fields, Prov: prov(),
	}, nil
}

func (f *fakeRecord) Get(_ context.Context, id string, version uint64) (Record, error) {
	f.calls = append(f.calls, "get")
	f.lastGet.id, f.lastGet.version = id, version
	if f.get != nil {
		return f.get(id, version)
	}
	return record(), nil
}

func (f *fakeRecord) Query(_ context.Context, a QueryArgs) ([]Record, error) {
	f.calls = append(f.calls, "query")
	f.lastFilter = a
	if f.query != nil {
		return f.query(a)
	}
	return nil, nil
}

func (f *fakeRecord) History(_ context.Context, id string) ([]Record, error) {
	f.calls = append(f.calls, "history")
	if f.history != nil {
		return f.history(id)
	}
	return []Record{record()}, nil
}

func (f *fakeRecord) Link(_ context.Context, _, _, _ string) error {
	f.calls = append(f.calls, "link")
	return f.linkErr
}

func (f *fakeRecord) Unlink(_ context.Context, _, _, _ string) error {
	f.calls = append(f.calls, "unlink")
	return f.linkErr
}

func (f *fakeRecord) Refs(_ context.Context, a RefsArgs) (Refs, error) {
	f.calls = append(f.calls, "refs")
	f.lastRefs = a
	if f.refs != nil {
		return f.refs(a)
	}
	return Refs{ID: a.ID, Depth: a.Depth}, nil
}

func (f *fakeRecord) Step(_ context.Context, a StepArgs) (Record, error) {
	f.calls = append(f.calls, "step")
	f.lastStep = a
	if f.step != nil {
		return f.step(a)
	}
	return step(), nil
}

func (f *fakeRecord) Brief(_ context.Context, project string) (Brief, error) {
	f.calls = append(f.calls, "brief")
	if f.brief != nil {
		return f.brief(project)
	}
	return Brief{Project: project, Kind: "project"}, nil
}

// serving swaps the seam for the fake and puts it back, so one test's API
// cannot leak into the next.
func serving(t *testing.T, f *fakeRecord) *fakeRecord {
	t.Helper()
	saved := recordAPI
	recordAPI = func() (RecordAPI, func(), error) { return f, func() {}, nil }
	t.Cleanup(func() { recordAPI = saved })
	return f
}

// prov is a filled provenance. `now` and `ago` are peers_test.go's fixed
// clock, reused rather than redeclared: two clocks in one package is two
// things to keep in step.
func prov() Provenance {
	return Provenance{
		Session:   "s-4f2",
		Seat:      "cli",
		Epoch:     7,
		CreatedAt: now.Add(-90 * time.Second),
	}
}

func record(mut ...func(*Record)) Record {
	r := Record{
		ID:      "01927-abc",
		Version: 3,
		Kind:    "requirement",
		Project: "rig",
		Body:    "the grain is the heading",
		Fields:  map[string]string{"title": "the grain", "status": "active"},
		Prov:    prov(),
	}
	for _, m := range mut {
		m(&r)
	}
	return r
}

func step(mut ...func(*Record)) Record {
	r := Record{
		ID:      "01927-step",
		Version: 1,
		Kind:    "progress",
		Project: "rig",
		Body:    "gated in a detached worktree",
		Fields:  map[string]string{"state": "done", "item": "01927-item"},
		Prov:    prov(),
	}
	for _, m := range mut {
		m(&r)
	}
	return r
}

// captureStdout is renderUsage's technique pointed at the other stream. The
// record verbs write their answer to stdout, and asserting on what a renderer
// returned would miss a verb that computed the right thing and printed
// something else.
func captureStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	runErr := f()
	os.Stdout = saved
	if err := w.Close(); err != nil {
		t.Fatalf("closing the pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading the pipe: %v", err)
	}
	return string(out), runErr
}

// ---- ⛔ IfVersion 0 MEANS CREATE -------------------------------------------

// ⛔ THE ONE REFUSAL THIS FILE EXISTS FOR.
//
// internal/record: a put with IfVersion 0 CREATES, and a create against an id
// that already exists is a conflict rather than an overwrite. So a caller who
// forgets --if-version is byte-for-byte a caller asking to create, and the
// store's refusal then tells them the id is taken when what happened is that
// they omitted a flag.
//
// The guard has to be in the client, before anything is dialled, and it has to
// NAME THE ID - the id is the only thing connecting the message to what the
// caller typed.
func TestAPutNamingAnIdWithNoIfVersionIsRefusedBeforeItIsSent(t *testing.T) {
	f := serving(t, &fakeRecord{})

	err := run([]string{
		"record", "put", "--id", "01927-abc",
		"--kind", "requirement", "--project", "rig",
	})

	if err == nil {
		t.Fatal("a put naming an id with no --if-version was ACCEPTED. On the " +
			"wire that is a create, so it would be refused as a taken id and " +
			"the caller would never learn they omitted a flag")
	}
	if len(f.calls) != 0 {
		t.Errorf("the put reached the API as %v: the guard has to refuse "+
			"BEFORE anything is sent, or the caller gets the store's "+
			"already-exists message instead of this one", f.calls)
	}
	if !strings.Contains(err.Error(), "01927-abc") {
		t.Errorf("the refusal does not name the id:\n%s\nThe id is the only "+
			"thing in the caller's head that connects this message to what "+
			"they typed", err)
	}
	if !strings.Contains(err.Error(), "--if-version") {
		t.Errorf("the refusal does not name the flag that fixes it:\n%s", err)
	}
}

// AND AN EXPLICIT --if-version 0 IS A CREATE AND MUST GO THROUGH. It is the
// only way to write a `project` or a `case`, whose id is its slug rather than
// a minted UUIDv7. A guard that refused every id without a version would have
// made those two kinds unwritable.
func TestAnExplicitIfVersionZeroCreatesAtTheIdThatWasNamed(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if _, err := captureStdout(t, func() error {
		return run([]string{
			"record", "put", "--id", "rig", "--kind", "project",
			"--project", "rig", "--if-version", "0",
		})
	}); err != nil {
		t.Fatalf("an explicit --if-version 0 was refused: %v", err)
	}
	if !slices.Contains(f.calls, "put") {
		t.Fatal("an explicit create never reached the API")
	}
	if f.lastPut.ID != "rig" || f.lastPut.IfVersion != 0 {
		t.Errorf("the put carried id %q at version %d, want id \"rig\" at 0",
			f.lastPut.ID, f.lastPut.IfVersion)
	}
}

// THE MIRROR IMAGE: a version with no id names nothing to supersede.
// internal/record refuses it too; catching it here means the caller is told
// before anything is dialled.
func TestASupersedeWithNoIdIsRefused(t *testing.T) {
	f := serving(t, &fakeRecord{})

	err := run([]string{
		"record", "put", "--if-version", "4",
		"--kind", "requirement", "--project", "rig",
	})

	if err == nil {
		t.Fatal("--if-version 4 with no --id was accepted; there is no record " +
			"for it to supersede")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
}

// A PUT WITH NEITHER IS A PLAIN CREATE and must not be caught by the guard.
// The id is minted by the store, which is section 39's UUIDv7 scheme.
func TestAPutWithNoIdAndNoVersionCreatesWithoutBeingRefused(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if _, err := captureStdout(t, func() error {
		return run([]string{"record", "put", "--kind", "note", "--project", "rig"})
	}); err != nil {
		t.Fatalf("a plain create was refused: %v", err)
	}
	if f.lastPut.ID != "" {
		t.Errorf("the put carried id %q; an empty id is what asks the store to "+
			"mint one", f.lastPut.ID)
	}
}

// WHICH IT WAS IS PRINTED, NOT LEFT TO BE INFERRED FROM THE NUMBER. A caller
// who meant to supersede and created reads "version 1" as a success.
func TestAPutSaysWhetherItCreatedOrSuperseded(t *testing.T) {
	if got := putVerb(1); got != "created" {
		t.Errorf("version 1 rendered as %q: the first write is version 1 and "+
			"nothing supersedes into it", got)
	}
	if got := putVerb(2); got != "superseded" {
		t.Errorf("version 2 rendered as %q", got)
	}
	if putVerb(1) == putVerb(2) {
		t.Fatal("a create and a supersede print the same word, so the one " +
			"mistake this surface is shaped around is invisible in its output")
	}
}

// ---- ⛔ PROVENANCE IS NOT THE CLIENT'S TO SEND -----------------------------

// ⛔ A FORGERY SURFACE, RULED OUT AT THE TYPE. A client that can set its own
// session and seat can write a record attributed to another seat, on the one
// field section 39 makes load-bearing - "a quotation attributed to Boris that
// cannot be traced is a paraphrase until proved otherwise" is a PROVENANCE
// check, and it is worth nothing if the client fills it in.
//
// The field cannot come back by accident: this reads the struct rather than
// the call sites, so adding one anywhere fails here.
func TestTheClientCannotSendProvenanceAtAll(t *testing.T) {
	for _, tc := range []struct {
		what string
		typ  reflect.Type
	}{
		{"PutArgs", reflect.TypeOf(PutArgs{})},
		{"StepArgs", reflect.TypeOf(StepArgs{})},
	} {
		for _, forbidden := range []string{"Session", "Seat", "Epoch", "CreatedAt"} {
			if _, found := tc.typ.FieldByName(forbidden); found {
				t.Errorf("%s has a %s field. Provenance is the DAEMON's - the "+
					"connection already knows the session, the seat and the "+
					"epoch - and a client that can name one can name somebody "+
					"else's. internal/record.PutRequest carries these because "+
					"it is the daemon's interface to the store, not a "+
					"client's.", tc.what, forbidden)
			}
		}
	}
}

// ---- the subcommands -------------------------------------------------------

// ⛔ THE THREE VERBS THAT ARE DELIBERATELY ABSENT, ASSERTED SO THE COMMENT
// SAYING SO CANNOT BE THE ONLY THING HOLDING THE LINE.
//
// section 39 names ELEVEN verbs. standard.stamp and standard.drift are slice
// 6, project.gate is slice 7, and Boris's MVP is slices 1, 2 and 4. Somebody
// completing the set from memory is the predictable move, and a surface
// promising a capability rigd does not have is worse than a missing one.
func TestTheVerbsOffTheMVPAreNotOfferedAnywhere(t *testing.T) {
	for _, absent := range []string{"stamp", "drift", "gate"} {
		if slices.Contains(recordSubcommands, absent) {
			t.Errorf("`record %s` is a subcommand. standard.stamp, "+
				"standard.drift and project.gate are slices 6 and 7, off the "+
				"MVP: adding one is a change to section 39's build order, "+
				"which is the lead's", absent)
		}
	}
	if slices.Contains(staticVerbs, "standard") {
		t.Error("`rig standard` is offered. The standards register is slice " +
			"6, and rigd serves none of it")
	}
}

// AN UNKNOWN SUBCOMMAND NAMES THE ONES THAT EXIST, because the two callers who
// reach it want different things: `rig record put-all` is a typo, and
// `rig record stamp` is somebody completing section 39's set from memory.
func TestAnUnknownRecordSubcommandListsTheRealOnes(t *testing.T) {
	f := serving(t, &fakeRecord{})

	err := run([]string{"record", "stamp", "--project", "rig"})
	if err == nil {
		t.Fatal("`rig record stamp` was accepted")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
	for _, sub := range recordSubcommands {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("the refusal does not name %q, so a caller cannot see "+
				"what does exist:\n%s", sub, err)
		}
	}
}

// A FLAG ANOTHER SUBCOMMAND TAKES IS REFUSED, NOT IGNORED. An accepted flag
// that does nothing is the worst of the three answers: the caller believes it
// asked for something.
func TestASubcommandRefusesAFlagItDoesNotTake(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if err := run([]string{"record", "get", "01927-abc", "--kind", "requirement"}); err == nil {
		t.Fatal("`rig record get --kind` was accepted; --kind belongs to put " +
			"and does nothing here")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
}

// VERSION 0 IS THE HEAD, and it is what an unflagged get must send. Version
// zero is not a thing any record has: the first write is version 1.
func TestAGetWithNoVersionAsksForTheHead(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if _, err := captureStdout(t, func() error {
		return run([]string{"record", "get", "01927-abc"})
	}); err != nil {
		t.Fatal(err)
	}
	if f.lastGet.version != 0 {
		t.Errorf("an unflagged get asked for version %d, want 0 (the head)",
			f.lastGet.version)
	}
}

// ---- the typed fields ------------------------------------------------------

// A DUPLICATE KEY IS REFUSED RATHER THAN OVERWRITTEN. Last-one-wins is silent:
// a caller who typed --field state=done twice with different values gets one
// of them and is never told which.
func TestADuplicateFieldIsRefusedRatherThanSilentlyOverwritten(t *testing.T) {
	f := &fieldFlag{}
	if err := f.Set("state=started"); err != nil {
		t.Fatal(err)
	}
	err := f.Set("state=done")
	if err == nil {
		t.Fatal("--field state was accepted twice; one of the two values is " +
			"silently gone and the caller is never told which")
	}
	if !strings.Contains(err.Error(), "started") || !strings.Contains(err.Error(), "done") {
		t.Errorf("the refusal does not quote both values back:\n%s", err)
	}
}

func TestAFieldWithoutAValueIsRefused(t *testing.T) {
	for _, bad := range []string{"state", "=done", ""} {
		f := &fieldFlag{}
		if err := f.Set(bad); err == nil {
			t.Errorf("--field %q was accepted; a typed field needs a name and "+
				"a value", bad)
		}
	}
}

// NO FIELDS SENDS nil, NOT AN EMPTY MAP. The two are the same to the store
// today and they are not the same claim, and values() is where that is
// decided rather than at three call sites.
func TestNoFieldsSendsNothingRatherThanAnEmptyMap(t *testing.T) {
	if got := (&fieldFlag{}).values(); got != nil {
		t.Errorf("an unused --field sent %v, want nil", got)
	}
	var nilFlag *fieldFlag
	if got := nilFlag.values(); got != nil {
		t.Errorf("a nil fieldFlag sent %v, want nil", got)
	}
	if got := nilFlag.String(); got != "" {
		t.Errorf("a nil fieldFlag rendered %q; flag calls String on a zero "+
			"Value while printing defaults", got)
	}
}

// ---- rendering: the record -------------------------------------------------

// `fields` IS {} AND NEVER null. peersJSON's argument, on this object: a null
// is indistinguishable from a field this build failed to set, and a record
// carrying no typed fields is an ordinary record rather than a broken one.
func TestARecordWithNoFieldsEmitsAnObjectRatherThanNull(t *testing.T) {
	b, err := json.Marshal(recordJSON(record(func(r *Record) { r.Fields = nil }), now))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"fields":{}`) {
		t.Errorf("want an empty fields object, got: %s", b)
	}
}

// A QUERY THAT MATCHED NOTHING IS [] AND NEVER null, for the same reason.
func TestAnEmptyQueryEmitsAnEmptyArrayRatherThanNull(t *testing.T) {
	b, err := json.Marshal(recordsJSON(nil, now))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Errorf("an empty query emitted %s, want []", b)
	}
}

// AN EMPTY BODY IS A FACT AND IS LABELLED. Section 39 puts a record's content
// in its TYPED FIELDS - "never prose buried in a body an agent has to parse" -
// so a record with no body is normal. Dropped, the section reads as a
// renderer that ran out of things to print.
func TestARecordWithNoBodyAndNoFieldsSaysSoRatherThanPrintingNothing(t *testing.T) {
	got := recordText(record(func(r *Record) {
		r.Body = ""
		r.Fields = nil
	}), now)

	if !strings.Contains(got, "body") || !strings.Contains(got, "fields") {
		t.Fatalf("a record with no body and no fields dropped the sections "+
			"entirely:\n%s", got)
	}
	if strings.Count(got, "(none)") != 2 {
		t.Errorf("want both empty sections stated in words, got:\n%s", got)
	}
}

// ⛔ AN ABSENT SEAT OR SESSION IS A DEFECT AND RENDERS AS ONE.
// internal/record REFUSES a put without both, so a blank cell here cannot mean
// "nobody claimed it" - it means something between the store and this renderer
// dropped it. Section 21's rule: nothing was said is never a fact about
// anything, and it must render in a shape a real value cannot take.
func TestMissingProvenanceRendersAsADefectAndNotAsABlank(t *testing.T) {
	got := provenanceLine(Provenance{}, now)

	if strings.Contains(got, "  ") {
		t.Errorf("an empty provenance left a gap where a name goes, which "+
			"reads as a seat with no name:\n%q", got)
	}
	if !strings.Contains(got, "(not said)") {
		t.Errorf("an empty provenance did not say the value was missing:\n%q", got)
	}
	if !strings.Contains(got, "no recorded time") {
		t.Errorf("an unset timestamp did not say so:\n%q", got)
	}
	// And the shape must be one a real seat name cannot take, or it reads as
	// a seat called "not said".
	if !strings.Contains(got, "(") {
		t.Errorf("the missing value is not parenthesised, so it could be "+
			"mistaken for a real name:\n%q", got)
	}
}

// A ZERO TIMESTAMP IS NOT A DATE. Rendered straight it prints 0001-01-01,
// which a reader takes for a value; the age it produces is decades.
func TestAnUnsetTimestampIsNotRenderedAsADate(t *testing.T) {
	if got := provTime(time.Time{}); got != "" {
		t.Errorf("an unset stamp rendered as %q, which reads as a date", got)
	}
	if got := provAge(time.Time{}, now); got != -1 {
		t.Errorf("provAge of an unset stamp = %d, want -1: a real age cannot "+
			"be negative, so -1 cannot be mistaken for a measurement", got)
	}
}

// THE TIMESTAMP TRAVELS AS WELL AS THE AGE. peersJSON's rule: a consumer
// computing against its own clock must not be forced through this one.
func TestTheJSONRowCarriesBothTheStampAndTheAge(t *testing.T) {
	obj := recordJSON(record(), now)
	p, ok := obj["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("no provenance object: %v", obj)
	}
	for _, key := range []string{
		"session", "seat", "epoch", "created_at",
		"created_age_s", "created_at_unix",
	} {
		if _, ok := p[key]; !ok {
			t.Errorf("the provenance object has no %q, so a consumer reading "+
				"`rig record get --json` cannot see it at all", key)
		}
	}
}

// ---- rendering: the listings -----------------------------------------------

// AN EMPTY QUERY IS A SENTENCE, NOT A BLANK TABLE, and it names BOTH halves of
// what was asked - the project and the kind - because a kind spelled
// differently is a different kind and that is the likeliest cause.
func TestAnEmptyQueryIsASentenceNamingWhatWasAsked(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig", Kind: "requirement"}, nil, 0)

	if strings.Contains(got, "ID") && strings.Contains(got, "VERSION") {
		t.Errorf("an empty query printed a table header, which reads as a "+
			"broken command:\n%s", got)
	}
	if !strings.Contains(got, "rig") || !strings.Contains(got, "requirement") {
		t.Errorf("an empty query did not name the project and the kind it "+
			"asked about:\n%s", got)
	}
}

// THE SUMMARY COLUMN PREFERS THE TYPED FIELDS, and that is section 39's own
// ordering rather than a taste: `title` and `description_short` are named in
// the work-item metadata table as "one line" and "one line, for lists and
// briefs", which is this column exactly.
func TestTheSummaryPrefersTheTypedTitleOverTheBody(t *testing.T) {
	got := recordSummary(record(func(r *Record) {
		r.Fields = map[string]string{"title": "the typed title"}
		r.Body = "the body"
	}))
	if got != "the typed title" {
		t.Errorf("the summary rendered %q, want the typed title: section 39 "+
			"puts a record's one-line description in a FIELD so an agent "+
			"reaches it by name rather than parsing prose", got)
	}
}

// A RECORD WITH NOTHING TO SUMMARISE IS NOT A BLANK CELL. It is a real thing -
// a link target, or one written with only typed fields - and an empty cell
// reads as a renderer that failed.
func TestARecordWithNoTitleAndNoBodySaysSoInTheCell(t *testing.T) {
	got := recordSummary(Record{})
	if strings.TrimSpace(got) == "" {
		t.Fatal("a record with no title and no body rendered as an empty cell")
	}
	if !strings.HasPrefix(got, "(") {
		t.Errorf("the cell is %q, which could be mistaken for a title", got)
	}
}

// A TRUNCATED SUMMARY SAYS IT WAS TRUNCATED. A column silently showing one
// line of a twelve-line body teaches a reader the record is one line long.
func TestAMultiLineSummaryIsMarkedAsTruncated(t *testing.T) {
	got := firstLine("the first line\nand a second")
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a truncated summary rendered as %q with no marker", got)
	}
	if plain := firstLine("just the one"); plain != "just the one" {
		t.Errorf("a single-line value was marked as truncated: %q", plain)
	}
	// A trailing newline is not more content, and marking it would put "..."
	// on every value that happens to end in one.
	if got := firstLine("one line\n"); got != "one line" {
		t.Errorf("a trailing newline was treated as more content: %q", got)
	}
}

// THE HISTORY IS THE PROVENANCE. Section 39: "record.history - every version
// of one record WITH ITS PROVENANCE", and the reason the verb exists is that
// tracing a quotation becomes a field rather than an investigation. A history
// without a seat and a session per row answered the wrong question.
func TestTheHistoryCarriesWhoWroteEachVersion(t *testing.T) {
	got := historyText("01927-abc", []Record{
		record(func(r *Record) { r.Version = 1; r.Prov.Seat = "backend-1" }),
		record(func(r *Record) { r.Version = 2; r.Prov.Seat = "record" }),
	}, now)

	for _, want := range []string{"backend-1", "record"} {
		if !strings.Contains(got, want) {
			t.Errorf("the history does not carry %q, so it cannot answer who "+
				"wrote a version:\n%s", want, got)
		}
	}

	// ⛔ THE SESSION COLUMN IS GONE AND THIS TABLE IS WHERE IT DID THE DAMAGE.
	// One person editing one record twice, forty-four seconds apart, rendered
	// as two sessions in a column standing beside the truthful SEAT one - a
	// lying column and an honest one side by side, with nothing saying which
	// was which. Measured live at 3d1c05c before the ruling.
	if strings.Contains(got, "s-4f2") || strings.Contains(got, "SESSION") {
		t.Errorf("the history still carries the session: it is minted per "+
			"invocation, so the column reads as \"which sitting\" and answers "+
			"with one row per write:\n%s", got)
	}
}

// ---- refs ------------------------------------------------------------------

// NOTHING POINTING AT A RECORD IS AN ANSWER AND A LOUD ONE. An uncited
// requirement is exactly what record.refs exists to make visible, and a blank
// table would read as a failed call.
func TestARecordNothingPointsAtGetsASentenceAndNotABlank(t *testing.T) {
	got := refsText(Refs{ID: "01927-abc", Depth: 2})

	if strings.Contains(got, "SRC") {
		t.Errorf("an empty refs answer printed a table header:\n%s", got)
	}
	if !strings.Contains(got, "01927-abc") {
		t.Errorf("an empty refs answer did not name the record:\n%s", got)
	}
	if !strings.Contains(got, "2 hops") {
		t.Errorf("an empty refs answer did not say how far it looked, so it "+
			"cannot be told from a shallow question:\n%s", got)
	}
}

// THE EDGE IS DIRECTED AND THE ROW PRINTS IT WHOLE. A reader scanning a column
// of ids has no way to know which side of the arrow they are on, and "what
// points AT this" is the whole verb.
func TestARefRowPrintsBothEndsAndTheType(t *testing.T) {
	got := refsText(Refs{ID: "01927-dst", Depth: 1, In: []Ref{
		{
			Src: "01927-src", Type: "cites", Via: "01927-dst",
			Kind: "decision", Title: "the priority ruling", Distance: 1,
		},
	}})

	// ⛔ `via` IS THE FAR END, NOT A DECORATION ON THE ROW. At distance 1 it is
	// the subject itself, which is the case asserted here; past that it is the
	// record this edge was reached THROUGH, and a row without it says how far
	// and not through what.
	for _, want := range []string{
		"01927-src", "cites", "01927-dst", "decision", "the priority ruling",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the row does not carry %q:\n%s", want, got)
		}
	}
}

// A DEPTH THAT FOLLOWS NO EDGE IS REFUSED, because its answer - nothing -
// cannot be told from a record nothing points at, which is the one answer this
// verb exists to make readable.
func TestRefsRefusesADepthThatFollowsNoEdge(t *testing.T) {
	f := serving(t, &fakeRecord{})

	if err := run([]string{"record", "refs", "01927-abc", "--depth", "0"}); err == nil {
		t.Fatal("--depth 0 was accepted; its empty answer is indistinguishable " +
			"from a record nothing cites")
	}
	if len(f.calls) != 0 {
		t.Errorf("it reached the API as %v", f.calls)
	}
}

// THE DEPTH THAT WAS ANSWERED RIDES IN THE OBJECT. appsJSON spends a paragraph
// on the same field: a reader who did not type the flag cannot otherwise tell
// a cheap question from an empty answer.
func TestTheRefsObjectCarriesTheDepthAndAnArrayThatIsNeverNull(t *testing.T) {
	b, err := json.Marshal(refsJSON(Refs{ID: "x", Depth: 3}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"depth":3`) {
		t.Errorf("the object does not carry the depth it answered: %s", b)
	}
	if !strings.Contains(string(b), `"in":[]`) {
		t.Errorf("want an empty array, got: %s", b)
	}
}

// ---- the link verbs --------------------------------------------------------

// link AND unlink PRINT DIFFERENT WORDS. One function serves both, which is
// exactly the shape where a copied confirmation line reports the wrong
// operation, and a caller who unlinked and read "linked" has no other signal.
func TestLinkAndUnlinkDoNotReportEachOthersOperation(t *testing.T) {
	f := serving(t, &fakeRecord{})

	linked, err := captureStdout(t, func() error {
		return run([]string{"record", "link", "a", "cites", "b"})
	})
	if err != nil {
		t.Fatal(err)
	}
	unlinked, err := captureStdout(t, func() error {
		return run([]string{"record", "unlink", "a", "cites", "b"})
	})
	if err != nil {
		t.Fatal(err)
	}

	if linked == unlinked {
		t.Fatalf("link and unlink printed the same line (%q), so a caller "+
			"cannot tell which one ran", linked)
	}
	if !slices.Equal(f.calls, []string{"link", "unlink"}) {
		t.Errorf("the API saw %v, want link then unlink: the two verbs are one "+
			"function and a swapped branch is invisible from the output alone",
			f.calls)
	}
}

// THE USAGE LINE DRAWS THE DIRECTION. `rig record link A cites B` has to read
// as "A cites B", and nothing but an arrow makes a reader check that.
func TestTheLinkUsageShowsWhichWayTheEdgePoints(t *testing.T) {
	serving(t, &fakeRecord{})

	err := run([]string{"record", "link", "a", "cites"})
	if err == nil {
		t.Fatal("a two-argument link was accepted")
	}
	if !strings.Contains(err.Error(), "-->") {
		t.Errorf("the usage line does not draw the edge's direction:\n%s", err)
	}
}

// ---- the seam --------------------------------------------------------------

// ⛔ THE SEAM NOW BLAMES THE DAEMON, AND THAT IS THE REVERSAL RATHER THAN A
// RELAXATION.
//
// This test replaces TestTheUnwiredRefusalNamesTheWireAndNotAStoppedDaemon,
// which asserted the OPPOSITE: while the record verbs were missing from the
// wire, reporting "is rigd running?" sent a reader to start a daemon that was
// already up, so the seam refused with its own code and named the wire.
//
// ⛔ EVERY CLAUSE OF THAT REFUSAL IS NOW FALSE. rigd dispatches all nine verbs,
// so "rigd serves nothing to call" is untrue and its precondition - that rigd
// answers those nine - is MET. A refusal that has stopped being true is worse
// than no refusal, because it is a sentence a reader believes. notWired() and
// codeNoRecordWire are deleted rather than narrowed, and this is what took
// their place: with the verbs served, a seam that cannot open IS a daemon that
// is not there, which is the one cause left to name.
func TestTheSeamRefusesWithTheDaemonsOwnCodeWhenNothingIsListening(t *testing.T) {
	// A runtime dir with no socket in it, so the dial certainly fails. Without
	// this the test reads whatever daemon the developer's shell happens to
	// point at, which is a result that depends on the machine.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	_, _, err := openRecordAPI()
	if err == nil {
		t.Fatal("the seam opened against a runtime directory with no daemon " +
			"in it, so this test proved NOTHING about what it refuses with")
	}

	obj, _, structured := shape(err)
	if !structured {
		t.Fatal("the seam's refusal is not a structured error, so --json " +
			"cannot carry it")
	}
	if obj.Code != codeNoDaemon {
		t.Errorf("the seam refused with %q. With all nine verbs served, a "+
			"dial that fails is a daemon that is not running, and any other "+
			"code sends the reader to look for a gap that was closed",
			obj.Code)
	}
	// internal/kernel/refusal.go: "Runnable as written, or empty. NEVER
	// prose." This one HAS a command, which is the other half of the
	// reversal - there was no command that put the verbs on the wire, and
	// there is one that starts a daemon.
	if obj.FixCommand == "" {
		t.Error("the refusal offers no command, and starting rigd is exactly " +
			"the thing a caller can run to fix this")
	}
}

// ⛔ A MALFORMED COMMAND IS REFUSED BEFORE ANYTHING IS OPENED, AND THIS IS THE
// TEST THAT RUNNING THE BINARY PRODUCED.
//
// Every other test here installs a fake, and a fake opens successfully - so
// "check the arguments, then open" and "open, then check the arguments" are
// indistinguishable to all of them. Against the REAL seam they are not: with
// the wire missing, opening first answered `rig record put --id X` with the
// wire's failure and the whole --if-version guard was unreachable. Found by
// running the binary; every unit test was green.
//
// So this one runs with the seam left at its default, and it is the ONLY test
// in the file that does. What it asserts is an ORDER, and the only way to see
// an order is to make the second step fail.
//
// ⛔ THE SECOND STEP FAILS FOR A DIFFERENT REASON NOW AND THE TEST IS
// STRONGER FOR IT. It used to fail because the wire was missing, which was a
// property of the build; it now fails because nothing is listening on the
// runtime directory below, which is a property this test SETS UP. The old
// version would have quietly stopped asserting anything the moment the wire
// landed - a green that could no longer go red.
func TestABadCommandIsRefusedBeforeTheDaemonIsReachedFor(t *testing.T) {
	// No serving() here, deliberately: recordAPI is the real one, which now
	// DIALS - so anything that opens first reaches this empty runtime
	// directory and cannot produce an argument error.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{
			"a put with an id and no version",
			[]string{"record", "put", "--id", "01927-abc", "--kind", "note", "--project", "rig"},
			"01927-abc",
		},
		{"a put with no kind", []string{"record", "put", "--project", "rig"}, "--kind"},
		{"a get with no id", []string{"record", "get"}, "usage"},
		// ⛔ `record query rig` IS NOW VALID - every kind in project rig -
		// and this row used to assert the opposite. The bad shapes that
		// remain are a third positional, which the usage line has no
		// meaning for, and a flag typed with an empty value, which is a
		// shell variable that expanded to nothing rather than a request
		// for everything.
		{"a query with a third positional", []string{"record", "query", "rig", "note", "extra"}, "usage"},
		{"a query whose project expanded to nothing", []string{"record", "query", "--project", ""}, "--project"},
		{"a query whose kind expanded to nothing", []string{"record", "query", "--kind", ""}, "--kind"},
		{"a query given its project twice", []string{"record", "query", "rig", "--project", "other"}, "twice"},
		{"a history with no id", []string{"record", "history"}, "usage"},
		{"a link with two arguments", []string{"record", "link", "a", "cites"}, "usage"},
		{"refs at depth nothing", []string{"record", "refs", "x", "--depth", "0"}, "--depth"},
		{"a step with no item", []string{"progress", "step"}, "usage"},
		{"a brief with no container", []string{"brief"}, "usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.argv)
			if err == nil {
				t.Fatalf("%v was accepted", tc.argv)
			}
			obj, _, _ := shape(err)
			if obj.Code == codeNoDaemon {
				t.Fatalf("%v was answered with the DAEMON's failure rather "+
					"than with what is wrong with the command. rig opened the "+
					"record surface before checking its arguments, so the "+
					"caller is told to go start rigd when the mistake is "+
					"theirs - and every refusal in this file is unreachable "+
					"to anybody without a daemon.\ngot: %s", tc.argv, err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not mention %q:\n%s", tc.want, err)
			}
		})
	}
}

// EVERY RECORD VERB GOES THROUGH THE SEAM, so none of them can be wired up
// beside it later and escape both the fake and the refusal above.
func TestEveryRecordVerbReachesTheSeam(t *testing.T) {
	for _, argv := range [][]string{
		{"record", "get", "x"},
		{"record", "query", "rig", "note"},
		{"record", "history", "x"},
		{"record", "link", "a", "b", "c"},
		{"record", "unlink", "a", "b", "c"},
		{"record", "refs", "x"},
		{"progress", "step", "x", "--project", "rig", "--state", "done"},
		{"brief", "rig"},
	} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			f := serving(t, &fakeRecord{})
			if _, err := captureStdout(t, func() error { return run(argv) }); err != nil {
				t.Fatalf("%v: %v", argv, err)
			}
			if len(f.calls) == 0 {
				t.Errorf("%v reached no API call, so it answered from "+
					"somewhere other than rigd", argv)
			}
		})
	}
}

// ---- the partitioner, walked over THIS seat's flag sets --------------------

// EVERY NON-BOOLEAN FLAG MUST BE IN valuedFlags, AND NOTHING ELSE COUPLES THE
// TWO FOR THESE VERBS.
//
// depth_test.go carries the canonical walk and it is the LEAD's file, so it
// does not yet see the sets below. A missing entry does not fail loudly:
// partition() hands the value to the positionals and the command dies with
// "flag needs an argument", which blames the flag rather than the map.
//
// THIS IS A DUPLICATE ON PURPOSE AND IT IS MEANT TO DIE. The lead is extending
// the canonical walk to cover these sets; when that commit lands, delete this
// test rather than keeping two walks that can disagree. A gap is worse than a
// duplicate, in that order.
func TestEveryRecordFlagThatTakesAValueIsDeclaredToThePartitioner(t *testing.T) {
	sets := map[string]*flag.FlagSet{
		"brief":         briefFlagSet().fs,
		"progress step": progressFlagSet().fs,
	}
	for _, sub := range recordSubcommands {
		sets["record "+sub] = recordFlagSet(sub).fs
	}

	var checked int
	for verb, fs := range sets {
		fs.VisitAll(func(f *flag.Flag) {
			bf, ok := f.Value.(interface{ IsBoolFlag() bool })
			if ok && bf.IsBoolFlag() {
				return
			}
			checked++
			if !valuedFlags[f.Name] {
				t.Errorf("rig %s --%s takes a value and valuedFlags does not "+
					"list it, so partition() hands its value to the positionals "+
					"and the command fails with \"flag needs an argument\"",
					verb, f.Name)
			}
		})
	}

	// The positive control: an absence is also what a walk that never ran
	// produces, and every assertion above is about absence.
	if checked == 0 {
		t.Fatal("no non-boolean flag was examined, so this test would pass " +
			"against a VisitAll that visited nothing")
	}
	// And the second control, which the canonical walk had to learn: a count
	// alone passes whether the map holds one set or nine, because dropping a
	// set drops its flags from the count too.
	if len(sets) != len(recordSubcommands)+2 {
		t.Fatalf("the walk covers %d flag sets, want one per record "+
			"subcommand plus progress and brief", len(sets))
	}
}

// ---- completion ------------------------------------------------------------

// THE COMPLETION IS WALKED OFF THE FLAG SETS, not listed beside them.
// complete.go calls this coupling "one string in three places that MUST
// agree", and a hand-kept list is the fourth.
func TestTheCompletionOffersWhatTheFlagSetsActuallyDeclare(t *testing.T) {
	got := candidates([]string{"record", "put"})

	for _, want := range []string{"--id", "--if-version", "--kind", "--field"} {
		if !slices.Contains(got, want) {
			t.Errorf("`rig record put <TAB>` does not offer %q: %v", want, got)
		}
	}
	// --args belongs to a DECLARED COMMAND and would arrive if `record` fell
	// through to flagsOf, which asks the registry about a program of that
	// name.
	if slices.Contains(got, "--args") {
		t.Errorf("`rig record put <TAB>` offers --args, which only a "+
			"program's declared command takes: %v", got)
	}
	// And a flag that belongs to another subcommand must not be offered here,
	// because the subcommand would refuse it.
	if slices.Contains(got, "--version") {
		t.Errorf("`rig record put <TAB>` offers --version, which is get's: %v", got)
	}
}

func TestTheRecordSubcommandsAreOfferedAfterTheVerb(t *testing.T) {
	got := candidates([]string{"record"})
	if !slices.Equal(got, recordSubcommands) {
		t.Errorf("`rig record <TAB>` offered %v, want %v", got, recordSubcommands)
	}
	// A FRESH SLICE EVERY CALL. complete.go says callers of candidates()
	// append to what they are given, and handing out the package's own slice
	// lets one completion mutate the dispatcher's list.
	//
	// WRITING THROUGH AN INDEX RATHER THAN APPENDING, and the difference is
	// the test working at all: append to a slice with no spare capacity
	// allocates a new array, so an append-based check passes against a
	// function that hands out its own slice. golangci-lint found the first
	// version - it flagged the append as ineffectual, which is exactly what
	// made it prove nothing.
	got[0] = "mutated"
	if slices.Contains(recordSubcommands, "mutated") {
		t.Fatal("the completion handed out the dispatcher's own slice, so a " +
			"caller writing through it changes what `rig record` accepts")
	}
}

// ---- the wire, asserted against a fake daemon -------------------------------

// wiredTo is a RecordAPI over a fake daemon, so a test can read the frame this
// client actually SENT.
//
// It builds wireRecord directly rather than going through openRecordAPI,
// deliberately: openRecordAPI reaches paths.Socket() and runs the skew check,
// and neither is the thing under test here. buildskew_test.go's
// TestARealVerbWarnsOnSkew is what covers that path, once, for every verb.
func wiredTo(t *testing.T, reply proto.Message) (RecordAPI, *fakeDaemon) {
	t.Helper()
	d := startFakeDaemon(t, reply)
	c, err := client.Dial(d.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return wireRecord{c: c}, d
}

// wireCtx is a bounded context for a call to the fake. Bare Background() is
// what the nocontextfree analyzer refuses, and a deadline here is also what
// stops a broken read loop hanging the suite instead of failing it.
func wireCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// ⛔ EVERY METHOD NAME CARRIES THE `rig.` PREFIX, AND GETTING ONE WRONG DOES
// NOT LOOK LIKE A TYPO.
//
// rigd splits a method on its FIRST dot into program and command, so
// "record.put" without the prefix is read as a PROGRAM called `record` and the
// caller is refused with "no program \"record\" is connected" - a sentence
// that sends them to the daemon, the registry and their own spelling, none of
// which is where the answer is. B43 is the recorded instance of exactly that
// reading, on a different verb.
//
// NOTHING ELSE IN THIS PACKAGE CAN CATCH IT. A renderer cannot see what was
// asked for, and the fake replies to any method at all, so all nine of these
// pass their own unit tests with every name misspelled.
func TestEveryRecordMethodNamesItselfWithTheRigPrefix(t *testing.T) {
	for _, tc := range []struct {
		want string
		call func(context.Context, RecordAPI) error
	}{
		{"rig.record.put", func(ctx context.Context, a RecordAPI) error {
			_, err := a.Put(ctx, PutArgs{Kind: "note", Project: "rig"})
			return err
		}},
		{"rig.record.get", func(ctx context.Context, a RecordAPI) error {
			_, err := a.Get(ctx, "x", 0)
			return err
		}},
		{"rig.record.query", func(ctx context.Context, a RecordAPI) error {
			_, err := a.Query(ctx, QueryArgs{Project: "rig", Kind: "note"})
			return err
		}},
		{"rig.record.history", func(ctx context.Context, a RecordAPI) error {
			_, err := a.History(ctx, "x")
			return err
		}},
		{"rig.record.link", func(ctx context.Context, a RecordAPI) error {
			return a.Link(ctx, "a", "cites", "b")
		}},
		{"rig.record.unlink", func(ctx context.Context, a RecordAPI) error {
			return a.Unlink(ctx, "a", "cites", "b")
		}},
		{"rig.record.refs", func(ctx context.Context, a RecordAPI) error {
			_, err := a.Refs(ctx, RefsArgs{ID: "x"})
			return err
		}},
		{"rig.progress.step", func(ctx context.Context, a RecordAPI) error {
			_, err := a.Step(ctx, StepArgs{Item: "x", State: "done", Project: "rig"})
			return err
		}},
		{"rig.project.brief", func(ctx context.Context, a RecordAPI) error {
			_, err := a.Brief(ctx, "rig")
			return err
		}},
	} {
		t.Run(tc.want, func(t *testing.T) {
			// An EMPTY reply message, which marshals to zero bytes and so
			// decodes into whichever response type the method expects. The
			// answer is not what this test is about; the envelope is.
			api, d := wiredTo(t, &rigv1.RecordGetResponse{})
			if err := tc.call(wireCtx(t), api); err != nil {
				t.Fatalf("%s: %v", tc.want, err)
			}
			frames := d.frames(t)
			if len(frames) != 1 {
				t.Fatalf("the daemon was sent %d frames, want 1", len(frames))
			}
			if got := frames[0].GetMethod(); got != tc.want {
				t.Errorf("the call went out as %q, want %q. rigd splits on the "+
					"FIRST dot, so a missing `rig.` is read as a PROGRAM of "+
					"that name and the caller is told it is not connected",
					got, tc.want)
			}
		})
	}
}

// ⛔ AN UNSET --depth ASKS THE DAEMON'S DEFAULT, AND A TYPED ONE ASKS FOR
// ITSELF.
//
// The CLI used to default this flag to 1 while internal/record.DefaultRefsDepth
// is 4 - two places deciding one default, which is the defect
// internal/daemon/record.go writes a comment about at the same field. A zero on
// the wire is carried through to the store untouched, so the zero is how a
// client says "yours".
//
// THE CONTROL IS WHAT MAKES THIS MEAN ANYTHING: a second call at an explicit
// depth must travel differently, or the test passes against a client that
// never sets the field at all.
func TestAnUnsetDepthAsksTheDaemonForItsOwnDefault(t *testing.T) {
	api, d := wiredTo(t, &rigv1.RecordRefsResponse{})
	ctx := wireCtx(t)

	if _, err := api.Refs(ctx, RefsArgs{ID: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Refs(ctx, RefsArgs{ID: "x", Depth: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Refs(ctx, RefsArgs{ID: "x", CrossProject: true}); err != nil {
		t.Fatal(err)
	}

	sent := refsRequests(t, d)
	if len(sent) != 3 {
		t.Fatalf("the daemon was sent %d refs requests, want 3", len(sent))
	}
	if got := sent[0].GetDepth(); got != 0 {
		t.Errorf("an unset --depth travelled as %d. A zero is how this wire "+
			"says \"the daemon's default\", and any other number is a second "+
			"default in a client the store cannot see", got)
	}
	if got := sent[1].GetDepth(); got != 3 {
		t.Errorf("--depth 3 travelled as %d, so the field is not travelling "+
			"at all and the assertion above proves nothing", got)
	}
	// ⛔ SECTION 39 MAKES CROSSING "asked for, never arrived at", so the
	// default must be false on the wire and not merely unset in the flag set.
	if sent[0].GetCrossProject() {
		t.Error("a refs call with no --cross-project asked to leave the " +
			"record's own project")
	}
	if !sent[2].GetCrossProject() {
		t.Error("--cross-project did not reach the wire, so the flag is " +
			"accepted and does nothing - which is the worst of the three " +
			"possible answers")
	}
}

func refsRequests(t *testing.T, d *fakeDaemon) []*rigv1.RecordRefsRequest {
	t.Helper()
	var out []*rigv1.RecordRefsRequest
	for _, f := range d.frames(t) {
		req := &rigv1.RecordRefsRequest{}
		if err := proto.Unmarshal(f.GetPayload(), req); err != nil {
			t.Fatalf("a refs request did not decode: %v", err)
		}
		out = append(out, req)
	}
	return out
}

// ⛔ A STEP STATE THIS BUILD CANNOT SPELL IS REFUSED HERE, NAMING WHAT WAS
// TYPED, AND NOTHING IS SENT.
//
// progress.go's rule was that this client never pre-validates, because rigd
// "refuses an unknown state by name and quotes the value back". THE ENUM
// BROKE THAT PREMISE: `banana` has no value, arrives as UNSPECIFIED, and the
// daemon maps UNSPECIFIED to the empty string - so the store's refusal reads
// `"" is not a step state` and the caller's own word is gone. This client is
// the last place it exists.
func TestAStepStateWithNoValueOnTheWireIsRefusedBeforeItIsSent(t *testing.T) {
	api, d := wiredTo(t, &rigv1.ProgressStepResponse{})

	_, err := api.Step(wireCtx(t), StepArgs{Item: "x", State: "banana", Project: "rig"})
	if err == nil {
		t.Fatal("a state with no value on the wire was sent, and it arrives " +
			"at rigd as nothing at all")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("the refusal does not quote back what was typed, which is "+
			"the whole reason it happens here rather than at rigd:\n%s", err)
	}
	// The three it DOES know, so a caller learns the set from the refusal
	// rather than from the flag's help text they have already misread once.
	for _, want := range []string{"started", "blocked", "done"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not offer %q:\n%s", want, err)
		}
	}
	// ⛔ AND THE ZERO IS NOT OFFERED. Section 21: its meaning is "nothing was
	// said", so a caller spelling it would be asking for an unset field.
	if strings.Contains(err.Error(), "unspecified") {
		t.Errorf("the refusal offers the enum's ZERO as a state somebody "+
			"could type:\n%s", err)
	}
	if n := len(d.frames(t)); n != 0 {
		t.Errorf("%d frames reached the daemon; a state rig cannot spell must "+
			"not be sent", n)
	}
}

// THE THREE SPELLINGS ARE WALKED OFF THE ENUM RATHER THAN WRITTEN DOWN, so a
// fourth state added to the proto is accepted by this build the same day.
//
// This is the half that makes the refusal above safe rather than a second
// source of truth - which is what progress.go was right to refuse.
func TestEveryStepStateTheWireDeclaresIsAcceptedByName(t *testing.T) {
	values := rigv1.StepState_STEP_STATE_UNSPECIFIED.Descriptor().Values()
	seen := 0
	for i := range values.Len() {
		v := values.Get(i)
		if v.Number() == 0 {
			continue
		}
		seen++
		word := enumLabel(string(v.Name()), "STEP_STATE_")
		got, err := stepStateOnTheWire(word)
		if err != nil {
			t.Errorf("the wire declares %q and this client refuses it: %v", word, err)
			continue
		}
		if got.Number() != v.Number() {
			t.Errorf("%q mapped to %v, want %v", word, got.Number(), v.Number())
		}
	}
	// The positive control. An empty descriptor walk would pass every
	// assertion above without checking anything, which is the "nothing is
	// wrong" and "the check did not run" collapse this repository keeps
	// catching.
	if seen != 3 {
		t.Fatalf("the walk found %d non-zero step states, want 3: it answered "+
			"about something other than StepState and every assertion above "+
			"is meaningless", seen)
	}
}

// ---- refs: the two fields that stop a partial answer looking complete -------

// ⛔ A TRUNCATED ANSWER SAYS SO, IN THE TEXT AND IN THE OBJECT.
//
// It is the single defect this capability exists to prevent: a short list that
// reads as the whole answer. The store computes the flag and the wire carries
// it, so a client that drops it is the only thing between a reader and the
// truth.
func TestATruncatedRefsAnswerSaysItIsPartial(t *testing.T) {
	full := refsText(Refs{ID: "x", Depth: 2, In: []Ref{
		{Src: "a", Type: "cites", Via: "x", Kind: "decision", Distance: 1},
	}})
	if strings.Contains(strings.ToUpper(full), "TRUNCATED") {
		t.Errorf("a COMPLETE answer announced a truncation, so the assertion "+
			"below cannot tell the two apart:\n%s", full)
	}

	cut := refsText(Refs{ID: "x", Depth: 2, Truncated: true, In: []Ref{
		{Src: "a", Type: "cites", Via: "x", Kind: "decision", Distance: 1},
	}})
	if !strings.Contains(strings.ToUpper(cut), "TRUNCATED") {
		t.Errorf("a PARTIAL answer rendered exactly like a complete one:\n%s", cut)
	}

	// AND ON AN EMPTY ANSWER TOO, which is the case where it matters most: a
	// reader is being told nothing points at this record, and the walk may
	// simply not have got there.
	empty := refsText(Refs{ID: "x", Depth: 1, Truncated: true})
	if !strings.Contains(strings.ToUpper(empty), "TRUNCATED") {
		t.Errorf("an empty PARTIAL answer read as \"nothing points at this\", "+
			"which is a different claim:\n%s", empty)
	}
}

// A CYCLE IS NAMED AND NEVER RESOLVED. Section 39: detected, reported, ordered
// around, never resolved - rig does not pick an edge to break, because
// choosing which one is wrong is a judgement about the work.
func TestRefsNamesACycleAndOffersNoEdgeToBreak(t *testing.T) {
	got := refsText(Refs{ID: "x", Depth: 3, Cycles: [][]string{{"a", "b", "c"}}})

	for _, want := range []string{"a -> b -> c -> a"} {
		if !strings.Contains(got, want) {
			t.Errorf("the cycle is not named as a closed loop, so it reads as "+
				"a chain:\n%s", got)
		}
	}
	if !strings.Contains(got, "does not pick an edge to break") {
		t.Errorf("the cycle report does not say that rig refuses to resolve "+
			"it:\n%s", got)
	}
}

// BOTH FIELDS RIDE IN THE OBJECT ON EVERY ANSWER, not only when they are
// interesting. A key that appears only when something is wrong is a key
// nobody's parser has a branch for at the moment it first appears.
func TestTheRefsObjectAlwaysCarriesTruncatedAndCycles(t *testing.T) {
	b, err := json.Marshal(refsJSON(Refs{ID: "x", Depth: 4}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"truncated":false`, `"cycles":[]`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the object does not carry %s: %s", want, b)
		}
	}

	// THE ROW'S KEYS ARE THE WIRE'S OWN WORDS, and `dst` is not one of them -
	// there is no such field, and a key of that name would claim every row
	// points straight at the subject.
	b, err = json.Marshal(refsJSON(Refs{ID: "x", Depth: 4, In: []Ref{
		{Src: "a", Type: "cites", Via: "m", Kind: "decision", Title: "t", Distance: 2},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"via":"m"`, `"title":"t"`, `"distance":2`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the row does not carry %s: %s", want, b)
		}
	}
	if strings.Contains(string(b), `"dst"`) {
		t.Errorf("the row carries a `dst` key and the wire has no such "+
			"field: %s", b)
	}
}

// ---- provenance off the wire ------------------------------------------------

// ⛔ A ZERO STAMP IS NOT 1970. Computed straight through time.Unix, an unset
// timestamp renders as a date rather than as an absence, and provTime's whole
// job is telling those apart. internal/record refuses to write a record
// without provenance, so a zero here is a defect between the store and this
// client - a thing to report, not a year to hand the reader.
func TestAProvenanceWithNoStampDoesNotBecomeTheUnixEpoch(t *testing.T) {
	p := provFromWire(&rigv1.Provenance{Session: "s", Seat: "cli", Epoch: 7})
	if !p.CreatedAt.IsZero() {
		t.Errorf("an unset stamp became %s, which renders as a date",
			p.CreatedAt)
	}
	if got := provTime(p.CreatedAt); got != "" {
		t.Errorf("provTime rendered the absent stamp as %q", got)
	}

	// The control: a real stamp must survive, or the assertion above passes
	// against a converter that drops the field entirely.
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	p = provFromWire(&rigv1.Provenance{AtUnixNano: at.UnixNano()})
	if !p.CreatedAt.Equal(at) {
		t.Errorf("a real stamp arrived as %s, want %s", p.CreatedAt, at)
	}
}

// ⛔ A TYPED FIELD REACHES THE WIRE WITH ITS SPELLING UNCHANGED, AND THAT IS
// WHAT MAKES A CROSS-SEAT FIELD NAME A CONTRACT AT ALL.
//
// `must_read` is the ruled example (PLAN.md section 39, rig 891b89f): the CLI
// writes the field and the brief's derivation reads it, and TWO SPELLINGS FAIL
// SILENTLY - an empty must-read set renders as "this project requires nothing",
// which is the reassuring lie the read-before-write gate exists to refuse.
//
// ⛔ SO THE PROPERTY UNDER TEST IS THAT rig IS A FAITHFUL CARRIER AND NEVER A
// HELPFUL ONE. A client that lowercased, trimmed, or turned a dash into an
// underscore would make the caller's spelling and the derivation's spelling
// agree BY ACCIDENT on some inputs and not others, which is worse than never
// agreeing: the contract would hold until the first field whose normalisation
// differs.
//
// ⛔ IT RUNS FROM ARGV AND THE FIRST VERSION DID NOT, WHICH IS WHY IT IS
// WRITTEN THIS WAY. That version called the API with a PutArgs built in the
// test, and a mutation that lowercased every key inside fieldFlag.Set SURVIVED
// it - because the flag parser is exactly the code such a test skips. The
// carriage being tested spans argv, fieldFlag, PutArgs and the wire, and a
// measurement that starts halfway along cannot see the half in front of it.
//
// A MISSPELLING IS CARRIED, NOT CAUGHT, AND THAT IS A FINDING RATHER THAN AN
// ASSERTION OF CORRECTNESS. Nothing in this client knows section 39's field
// vocabulary, so `must_reed` is a valid field name here and everywhere else it
// reaches. This test pins the carriage; what pins the SPELLING does not exist
// on either end yet.
func TestATypedFieldReachesTheWireWithItsSpellingUnchanged(t *testing.T) {
	// ⛔ THE KEYS ARE ORDERED AND EACH ONE IS A DIFFERENT NORMALISATION BAIT.
	// A client that "helpfully" tidied any of them would agree with the
	// derivation on some fields and not others.
	pairs := []struct{ key, value string }{
		// The ruled contract name, exactly as section 39 spells it.
		{"must_read", "true"},
		// ⛔ THE MISSPELLING, CARRIED RATHER THAN CAUGHT. It is here so that
		// the day rig learns to refuse an unknown field, this test fails and
		// somebody reads this comment.
		{"must_reed", "true"},
		// A dash and a capital: the two shapes a normaliser reaches for.
		{"Must-Read", "true"},
		{"description_short", "one line, for lists and briefs"},
	}

	argv := []string{"record", "put", "--kind", "work-item", "--project", "rig"}
	for _, p := range pairs {
		argv = append(argv, "--field", p.key+"="+p.value)
	}

	f := serving(t, &fakeRecord{})
	if _, err := captureStdout(t, func() error { return run(argv) }); err != nil {
		t.Fatalf("%v: %v", argv, err)
	}

	got := f.lastPut.Fields
	for _, p := range pairs {
		v, ok := got[p.key]
		if !ok {
			t.Errorf("the field %q did not survive argv under that key. A "+
				"client that renames a field breaks every cross-seat contract "+
				"resting on the name, and it breaks them silently. what "+
				"arrived: %v", p.key, got)
			continue
		}
		if v != p.value {
			t.Errorf("%q travelled as %q, want %q", p.key, v, p.value)
		}
	}
	// ⛔ AND NOTHING WAS ADDED. A client that supplied a default for a field
	// the caller omitted would put a value in the store that nobody typed,
	// attributed to the seat that typed the rest.
	if len(got) != len(pairs) {
		t.Errorf("%d fields arrived and %d were typed: %v", len(got), len(pairs), got)
	}
}

// AND THE SAME SPELLINGS SURVIVE THE WIRE ITSELF, which is the second half of
// the carriage and a different mechanism: the first test ends at PutArgs, this
// one reads the bytes the daemon was sent.
func TestATypedFieldReachesTheDaemonUnderTheKeyItWasTyped(t *testing.T) {
	api, d := wiredTo(t, &rigv1.RecordPutResponse{})

	sent := map[string]string{"must_read": "true", "Must-Read": "true"}
	if _, err := api.Put(wireCtx(t), PutArgs{
		Kind: "work-item", Project: "rig", Fields: sent,
	}); err != nil {
		t.Fatal(err)
	}

	frames := d.frames(t)
	if len(frames) != 1 {
		t.Fatalf("the daemon was sent %d frames, want 1", len(frames))
	}
	req := &rigv1.RecordPutRequest{}
	if err := proto.Unmarshal(frames[0].GetPayload(), req); err != nil {
		t.Fatal(err)
	}
	for k, want := range sent {
		if got := req.GetFields()[k]; got != want {
			t.Errorf("%q reached the daemon as %q, want %q: %v",
				k, got, want, req.GetFields())
		}
	}
}

// ---- the real seam, through argv, against a socket -------------------------

// atAFakeDaemon points this process's runtime directory at a fake daemon
// serving one reply, so a verb that DIALS reaches it.
//
// ⛔ IT EXISTS BECAUSE EVERY OTHER TEST HERE SWAPS THE SEAM AND SO NEVER RUNS
// IT. `serving()` replaces recordAPI with a fake, which is what makes the
// renderers testable - and it means openRecordAPI, connect(), the dial, the
// deadline and the release are covered by nothing. That is this package's
// documented coverage hole, in the three verbs this seat owns.
//
// The directory is made with os.MkdirTemp rather than t.TempDir for the reason
// buildskew_test.go gives at the same call: a unix socket path caps near 108
// bytes and a Go test's own temp path can approach it.
func atAFakeDaemon(t *testing.T, reply proto.Message) *fakeDaemon {
	t.Helper()
	runtime, err := os.MkdirTemp("", "rigseam")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtime) })
	t.Setenv("XDG_RUNTIME_DIR", runtime)

	sock, err := paths.Socket()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}

	// The skew line is stderr noise here and nothing under test: the fake
	// answers rig.estate with whatever message it was given, so the check
	// always has something to complain about. Redirected so a failure message
	// in this file is readable.
	saved := skewOut
	skewOut = io.Discard
	t.Cleanup(func() { skewOut = saved })

	return serveFakeDaemonAt(t, sock, reply)
}

// ⛔ `rig record get` DIALS A SOCKET AND RENDERS WHAT CAME BACK OFF IT.
//
// Every other rendering test in this file hands recordText a Record the test
// built. This one starts at argv and ends at stdout with a real wire in the
// middle, so it is the only thing covering openRecordAPI, the dial, the
// unmarshal and the provenance conversion together.
func TestRecordGetDialsARealSocketAndRendersWhatCameBack(t *testing.T) {
	at := time.Date(2026, 9, 16, 21, 0, 0, 0, time.UTC)
	d := atAFakeDaemon(t, &rigv1.RecordGetResponse{Record: &rigv1.Record{
		Id:      "01927-abc",
		Version: 3,
		Kind:    "requirement",
		Project: "rig",
		Body:    "the grain is the heading",
		Fields:  map[string]string{titleKey: "the grain"},
		Prov: &rigv1.Provenance{
			Session: "sess-4f2", Seat: "terminal:someone", Epoch: 7,
			AtUnixNano: at.UnixNano(),
		},
	}})

	out, err := captureStdout(t, func() error {
		return run([]string{"record", "get", "01927-abc"})
	})
	if err != nil {
		t.Fatalf("rig record get against the fake: %v", err)
	}

	for _, want := range []string{
		"01927-abc", "version 3", "requirement", "rig",
		"the grain is the heading",
		// ⛔ THE PROVENANCE IS THE HALF A FAKE SEAM CANNOT EXERCISE. It comes
		// off `Provenance.at_unix_nano`, an int64 that has to become a
		// time.Time without passing through the Unix epoch on the way.
		//
		// The SESSION is deliberately absent from this list and asserted
		// absent below: the timestamp is what exercises the conversion, and
		// the session was never what this test was about.
		//
		// ⛔ `someone`, NOT `terminal:someone`, AND THE PREFIX GOING MISSING
		// HERE IS THE POINT. This runs from argv through a real socket, so it
		// is the end-to-end proof that displaySeat is on the rendering path
		// rather than only on a formatter a unit test can reach. The STORED
		// value still has its prefix - the object below is where that is
		// asserted.
		"someone", "2026-09-16T21:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rendered record does not carry %q:\n%s", want, out)
		}
	}

	// ⛔ AND THE SESSION IS NOT RENDERED TO A HUMAN. This runs from argv
	// through a real socket, so it is the end-to-end proof of the ruling
	// rather than a unit check on one formatter: whatever the daemon sent,
	// the reader does not see a session id. `--json` keeps it, and
	// TestTheRecordObjectStillCarriesTheSessionForAParser is that half.
	if strings.Contains(out, "sess-4f2") {
		t.Errorf("the human rendering carries the session, which is minted "+
			"per invocation and groups nothing:\n%s", out)
	}
	// And the kind prefix is not shown either. Asserted as an absence beside
	// the presence above, because "someone" is a substring of
	// "terminal:someone" - a Contains check alone would pass on the old
	// output and prove nothing.
	if strings.Contains(out, "terminal:someone") {
		t.Errorf("the human rendering still carries the kind prefix:\n%s", out)
	}

	// AND THE VERB ASKED FOR WHAT IT WAS TOLD TO. The skew check dials first,
	// so the record call is the one that is not rig.estate.
	var asked int
	for _, f := range d.frames(t) {
		if f.GetMethod() == "rig.record.get" {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("the daemon was sent %d rig.record.get frames, want 1: %v",
			asked, d.frames(t))
	}
}

// ⛔ THE BRIEF RENDERS FROM WIRE BYTES RATHER THAN FROM A STRUCT A TEST BUILT,
// which is the only way briefFromWire and the renderer are exercised as one
// thing. Every other brief test builds a Brief and hands it to a renderer, so
// a mapping that dropped a field and a renderer that ignored one look the same.
func TestTheBriefRendersWhatArrivedOnTheWire(t *testing.T) {
	atAFakeDaemon(t, &rigv1.ProjectBriefResponse{
		Project: "rig",
		Kind:    "project",
		Title:   "the swiss knife and the record under it",
		Status:  "active",
		Semver:  "0.4.1",
		NextUp: []*rigv1.ItemState{{
			Id: "01927-n", Title: "the seam",
			State:         rigv1.StepState_STEP_STATE_STARTED,
			SinceUnixNano: time.Now().Add(-2 * time.Minute).UnixNano(),
			Note:          "dialling now",
		}},
		Open:     []*rigv1.ItemState{{Id: "01927-o", Title: "the coverage hole"}},
		Features: []*rigv1.Feature{{Id: "01927-f", Title: "the record", Stage: "building"}},
		Sections: []*rigv1.BriefSectionStatus{
			{
				Section: rigv1.BriefSection_BRIEF_SECTION_DRIFT,
				State:   rigv1.SectionState_SECTION_STATE_NOT_COMPUTED,
				Reason:  "the standards register is slice 6",
			},
		},
	})

	out, err := captureStdout(t, func() error { return run([]string{"brief", "rig"}) })
	if err != nil {
		t.Fatalf("rig brief against the fake: %v", err)
	}

	for _, want := range []string{
		"the swiss knife and the record under it", "active", "v0.4.1",
		// next-up, with the three columns the realigned BriefItem carries.
		"01927-n", "started", "dialling now",
		// open, as its OWN section rather than folded in.
		"ALSO OPEN", "01927-o",
		// features, and a not-computed section with the daemon's own reason.
		"FEATURES", "the record", "building",
		"the standards register is slice 6",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the brief does not carry %q:\n%s", want, out)
		}
	}
	// ⛔ AND THE OPEN ITEM IS NOT ALSO IN NEXT-UP. Disjointness, asserted on
	// the path where the mapping could break it rather than on a struct.
	if n := strings.Count(out, "01927-o"); n != 1 {
		t.Errorf("the open item appears %d times, so the two lists were "+
			"merged somewhere between the wire and the page:\n%s", n, out)
	}
}

// ⛔ THE SEAM'S RELEASE ACTUALLY CLOSES THE CONNECTION.
//
// withRecordAPI defers it on every verb, and nothing checked that it does
// anything. A release that leaked would be invisible to every test in this
// package and to a person running one command, and would show up only as a
// daemon accumulating connections under an agent that calls rig in a loop.
func TestTheSeamsReleaseClosesTheConnection(t *testing.T) {
	atAFakeDaemon(t, &rigv1.RecordGetResponse{})

	api, release, err := openRecordAPI()
	if err != nil {
		t.Fatalf("the seam did not open against a live socket: %v", err)
	}
	if _, err := api.Get(wireCtx(t), "01927-abc", 0); err != nil {
		t.Fatalf("the seam opened and could not call: %v", err)
	}

	release()

	// The control is the successful call above: without it, a seam that never
	// worked at all would pass this assertion trivially.
	if _, err := api.Get(wireCtx(t), "01927-abc", 0); err == nil {
		t.Error("a call succeeded AFTER release, so the release does not " +
			"close the connection and every record verb leaks one")
	}
}

// ⛔ A FIELD ADDED TO `Ref` ON THE WIRE AND NEVER RENDERED HERE IS THE THING
// THAT ROTS, AND THIS IS THE TEST THAT NOTICES.
//
// It is `TestTheJSONObjectCoversEveryFieldOfStatus` applied one message over,
// deliberately rather than by coincidence: that test exists because
// `jsonStatus` is hand-written, and `refsJSON`'s per-edge object is hand-written
// for exactly the same reason. Section 38 says not to reinvent what already
// works, and the technique already works here.
//
// ⛔ IT IS ALSO A FORCING FUNCTION FOR A DEFECT THIS SEAT HAS REPORTED AND MAY
// NOT FIX. Measured 2026-09-17 against a two-project store, client and daemon
// both at `6c768c4`: `record refs --cross-project` returns the foreign edge and
// NOTHING IN THE ANSWER SAYS WHICH ROWS CROSSED. No layer carries `project` on
// a ref - not `internal/record.Ref`, not `rigv1.Ref`, not this package's - even
// though `internal/record/refs.go` reads a record's project to make the pruning
// decision and then drops it. The wire is not this seat's to change, so the
// repair is the lead's and the store seat's.
//
// **When that field lands, this test goes RED and names it**, which is the
// whole point: the renderer cannot be the reason a fix to the seam is invisible
// to a reader. A comment listing the fields would have rotted on the same
// commit it was written.
func TestEveryFieldTheWireCarriesOnARefIsRendered(t *testing.T) {
	var onTheWire []string
	fields := (&rigv1.Ref{}).ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		onTheWire = append(onTheWire, string(fields.Get(i).Name()))
	}

	// Rendered through refsJSON rather than off the struct, because the struct
	// is not what a caller reads. A field present on `Ref` and absent from the
	// object is the same silent drop as a field absent from both.
	one := refsJSON(Refs{In: []Ref{{}}})["in"].([]map[string]any)
	if len(one) != 1 {
		t.Fatalf("refsJSON rendered %d edges from one, so this test is "+
			"measuring the wrong thing", len(one))
	}
	var rendered []string
	for k := range one[0] {
		rendered = append(rendered, k)
	}

	slices.Sort(onTheWire)
	slices.Sort(rendered)
	if !slices.Equal(onTheWire, rendered) {
		t.Errorf("--json does not render the Ref the wire carries\n"+
			" wire: %v\n json: %v\n"+
			"a field added to Ref must be rendered here, or the answer "+
			"silently drops it. If the new field is `project`, this is the "+
			"cross-project gap closing and the renderer owes a column as well "+
			"as a key", onTheWire, rendered)
	}
}

// ⛔ OPTING IN TO CROSSING WIDENS THE WALK; IT DOES NOT MOVE IT - AND THIS IS
// THE RENDERER'S HALF OF THAT CLAIM.
//
// `internal/daemon/record_wire_test.go` pins it at the wire: a cross-project
// answer must still carry the SAME-project citation. **That says nothing about
// whether this package prints it.** A renderer that replaced its rows instead
// of extending them would satisfy every test on the daemon side and lose a row
// on the way to the reader - the seam failure this project keeps finding,
// because ownership is by file and the join between two files is nobody's.
//
// Runs from argv through a real socket, so the flag parser, the dial, the
// unmarshal and the renderer are all on the path. A test that started at
// `refsText` would skip the three of those most likely to be wrong.
func TestACrossProjectAnswerStillRendersTheEdgeThatDidNotCross(t *testing.T) {
	const subject = "01927-subject"
	near := &rigv1.Ref{
		Src: "01927-near", Type: "cites", Via: subject,
		Kind: "decision", Title: "a decision inside alpha", Distance: 1,
	}
	// The foreign end. ⛔ NOTHING ON THIS MESSAGE CAN SAY IT IS FOREIGN: the
	// only reason this fixture can tell the two apart at all is the prose of
	// the title, which is the defect stated as a construction rather than as a
	// complaint. A reader whose records are titled less helpfully gets two
	// identical rows.
	far := &rigv1.Ref{
		Src: "01927-far", Type: "cites", Via: subject,
		Kind: "decision", Title: "a decision inside beta", Distance: 1,
	}
	atAFakeDaemon(t, &rigv1.RecordRefsResponse{
		Id: subject, Depth: 4, Refs: []*rigv1.Ref{near, far},
	})

	out, err := captureStdout(t, func() error {
		return run([]string{"record", "refs", subject, "--cross-project"})
	})
	if err != nil {
		t.Fatalf("record refs --cross-project: %v", err)
	}

	// ⛔ THE TABLE ROWS, NOT THE WHOLE OUTPUT, AND THE DIFFERENCE IS NOT
	// PEDANTRY - IT IS THE ONLY VERSION OF THIS TEST THAT WORKS.
	//
	// Written first as `strings.Contains(out, "01927-near")`, it SURVIVED two
	// mutations that deleted a row from the table outright (`r.In[:1]` and
	// `r.In[1:]`). `refsTitleBlock` prints every id again underneath, so the
	// id is in the output whether or not the row is, and the assertion was
	// measuring the title block while claiming to measure the table. **It read
	// as coverage and was none** - the wrong-unit defect, caught by the
	// mutation rather than by review.
	rows := tableRows(out)
	for _, want := range []string{"01927-near", "01927-far"} {
		found := false
		for _, r := range rows {
			if strings.Contains(r, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no TABLE ROW carries %s.\n"+
				"Crossing widens the walk and must not move it: the near edge "+
				"and the foreign one are both answers.\nrows=%q\n%s",
				want, rows, out)
		}
	}
	if len(rows) != 2 {
		t.Errorf("the table has %d rows, want 2 - a row was added or lost "+
			"without either id going missing\nrows=%q", len(rows), rows)
	}
	if !strings.Contains(out, "2 edges point at") {
		t.Errorf("the count does not say two, so the summary disagrees with "+
			"the table above it\n%s", out)
	}
}

// tableRows returns the body lines of the one table `refsText` writes: the
// lines after the SRC header and before the blank line that ends it.
//
// It exists because an assertion over the WHOLE of that output cannot tell a
// row in the table from the same id repeated in the title block underneath -
// measured, by two mutations that deleted a row and were not noticed.
func tableRows(out string) []string {
	var rows []string
	started := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "SRC") {
			started = true
			continue
		}
		if !started {
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		rows = append(rows, line)
	}
	return rows
}

// ⛔ `--json` KEEPS THE SESSION, AND THIS IS THE HALF THAT STOPS THE RULING
// BEING OVER-APPLIED.
//
// Ruled 2026-09-17: the human surfaces stop printing `prov.session` because to
// a reader it asserts a grouping that does not exist - sixty puts at one
// terminal in one act wrote sixty distinct sessions, measured. **The object
// does not stop carrying it**, and the split is deliberate rather than an
// oversight in either direction: a parser wants raw identity with no claim
// attached, and it is the only surface from which the invocation id is still
// recoverable.
//
// Without this test the ruling reads as "the session is noise" and the next
// seat tidying the object removes it too - which would delete the field from
// the last place it survives, in the name of a decision that was only ever
// about prose.
func TestTheRecordObjectStillCarriesTheSessionForAParser(t *testing.T) {
	obj := recordJSON(Record{
		ID: "01927-abc", Version: 3, Kind: "requirement", Project: "rig",
		Prov: Provenance{
			Session: "sess-4f2", Seat: "terminal:someone", Epoch: 7,
		},
	}, now)

	prov, ok := obj["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("the object has no provenance block, so this test is "+
			"measuring the wrong thing: %#v", obj)
	}
	if prov["session"] != "sess-4f2" {
		t.Errorf("--json dropped the session (got %#v). A human is told less "+
			"than a parser here ON PURPOSE: the prose made a claim the field "+
			"cannot support, and the object makes no claim at all", prov["session"])
	}
	// The seat and epoch are the two that DO identify an act, so an object
	// that lost either would be worse than one that lost the session.
	if prov["seat"] != "terminal:someone" || prov["epoch"] != uint64(7) {
		t.Errorf("--json dropped a field that actually identifies the writer: %#v", prov)
	}
}

// ⛔ THE UNSET PATH IS THE ONE EVERY MACHINE BUT THIS ONE TAKES, SO IT IS
// TESTED FIRST AND HARDEST.
//
// `RIG_DISPLAY_NAME` is a name a person sets. The machine this was built on is
// the only machine in the world where it is set, so a test suite that only
// covered the set case would pass here forever and ship a broken default
// everywhere else.
func TestASeatWithNoDisplayNameSetRendersWithItsKindPrefixStripped(t *testing.T) {
	t.Setenv("RIG_DISPLAY_NAME", "")

	for _, tc := range []struct{ seat, want, why string }{
		{
			"terminal:boris-milner", "boris-milner",
			"the unix username, whole - NOT cut at a hyphen, which is a rule " +
				"inferred from one name and wrong about names in general",
		},
		{
			"terminal:mary-jane", "mary-jane",
			"the name that would have exposed a first-hyphen cut, kept here " +
				"deliberately so the rejected rule cannot come back unnoticed",
		},
		{"agent:some-worker", "some-worker", "any kind prefix, not just terminal:"},
		{
			"backend-1", "backend-1",
			"a seat with NO prefix is left whole: cutting on a colon that is " +
				"not there must not invent an empty string",
		},
		{
			"terminal:", "terminal:",
			"a prefix with nothing after it is not a name, so the raw value " +
				"is the honest answer rather than an empty cell",
		},
	} {
		if got := displaySeat(tc.seat); got != tc.want {
			t.Errorf("displaySeat(%q) = %q, want %q - %s", tc.seat, got, tc.want, tc.why)
		}
	}
}

// ⛔ THE DISPLAY NAME SUBSTITUTES FOR THIS CALLER'S OWN SEAT AND FOR NOTHING
// ELSE. THIS IS THE MISATTRIBUTION GUARD AND IT IS THE MOST IMPORTANT TEST IN
// THIS FILE.
//
// `record history` renders other seats in the same column - `backend-1`,
// `record`, another person's terminal. A display name applied to all of them
// would print this user's name over another writer's work, on the one surface
// whose entire job is saying who wrote something. **It would also be invisible
// on a single-user machine, which is the only kind this was developed on.**
func TestADisplayNameNeverRendersOverAnotherSeatsWork(t *testing.T) {
	saved := selfSeat
	t.Cleanup(func() { selfSeat = saved })
	selfSeat = func() string { return "terminal:boris-milner" }
	t.Setenv("RIG_DISPLAY_NAME", "boris")

	if got := displaySeat("terminal:boris-milner"); got != "boris" {
		t.Errorf("displaySeat on THIS caller's own seat = %q, want the display "+
			"name %q", got, "boris")
	}

	// Everything below is somebody else's work.
	for _, other := range []string{
		"terminal:mary-jane", // another person at another terminal
		"backend-1",          // an agent seat
		"record",             // another agent seat
		"terminal:root",      // the same kind, a different user
	} {
		got := displaySeat(other)
		if got == "boris" {
			t.Errorf("displaySeat(%q) rendered THIS user's display name over "+
				"another writer's work: provenance that misattributes is "+
				"worse than provenance that is ugly", other)
		}
		if got == "" {
			t.Errorf("displaySeat(%q) rendered empty", other)
		}
	}
}

// An unset variable and a variable set to the empty string are the same
// intention, and only one of them is what `os.Getenv` returns for both.
func TestAnEmptyDisplayNameIsTreatedAsUnsetRatherThanAsAName(t *testing.T) {
	saved := selfSeat
	t.Cleanup(func() { selfSeat = saved })
	selfSeat = func() string { return "terminal:boris-milner" }

	t.Setenv("RIG_DISPLAY_NAME", "")
	if got := displaySeat("terminal:boris-milner"); got != "boris-milner" {
		t.Errorf("an empty RIG_DISPLAY_NAME rendered %q; it must fall back to "+
			"the stripped seat, never to an empty author", got)
	}
}

// ⛔ `--json` NEVER SEES displaySeat. A parser wants the derived, unforgeable
// identity; a display name is a preference of the person reading. Rendering it
// into the object would put an unverifiable string where the only verifiable
// one belongs.
func TestTheObjectCarriesTheStoredSeatAndNeverTheDisplayName(t *testing.T) {
	saved := selfSeat
	t.Cleanup(func() { selfSeat = saved })
	selfSeat = func() string { return "terminal:boris-milner" }
	t.Setenv("RIG_DISPLAY_NAME", "boris")

	obj := recordJSON(Record{
		ID: "01927-abc", Kind: "requirement", Project: "rig",
		Prov: Provenance{Session: "s-1", Seat: "terminal:boris-milner", Epoch: 7},
	}, now)
	prov := obj["provenance"].(map[string]any)
	if prov["seat"] != "terminal:boris-milner" {
		t.Errorf("--json rendered %q, want the STORED seat: the object is the "+
			"only surface from which the derived identity is still "+
			"recoverable", prov["seat"])
	}
}

// recordWireRendering declares, per `rigv1.Record` field, the key or keys
// `recordJSON` emits for it. A wire field absent from this map is expected to
// be rendered under its own name.
//
// ⛔ IT IS A DECLARED MAP RATHER THAN THE SET EQUALITY USED ON `Ref`, because
// this object does not render its wire one-for-one and the departure is
// deliberate: `prov` is emitted as `provenance`, spelled out because `Prov` is
// an abbreviation internal/record chose and a JSON consumer is not reading
// that file.
//
// The cost of a rename map is that it can rot in three directions rather than
// one - a wire field can vanish, a rendered key can vanish, and the map itself
// can name something that no longer exists on either side. All three are red
// below, because a table that only checks the direction its author was
// thinking about is the comment it replaced.
var recordWireRendering = map[string][]string{
	"prov": {"provenance"},
}

// recordProvWireRendering is the same declaration for the NESTED object.
//
// ⛔ ONE WIRE FIELD BECOMES THREE KEYS, AND THAT IS THE WHOLE REASON THIS
// GUARD COULD NOT BE THE ONE ON `Ref`. `at_unix_nano` is emitted unchanged as
// `created_at_unix`, formatted as `created_at`, and differenced against the
// caller's clock as `created_age_s` - peersJSON's rule, so a consumer
// computing against its own clock is not forced through this one.
var recordProvWireRendering = map[string][]string{
	"at_unix_nano": {"created_at", "created_age_s", "created_at_unix"},
}

// recordWireFieldsNotRendered names every field on `rigv1.Record` that
// `recordJSON` deliberately does not emit, WITH THE REASON.
//
// ⛔ IT IS EMPTY, AND EMPTY IS A RESULT HERE RATHER THAN AN OVERSIGHT: every
// field the wire carries on a record reaches the reader. It stays declared so
// that the day one does not, the answer is written down beside the field
// instead of in a commit message nobody greps.
var recordWireFieldsNotRendered = map[string]string{}

// recordProvWireFieldsNotRendered is the same for `rigv1.Provenance`, and is
// empty for the same reason.
var recordProvWireFieldsNotRendered = map[string]string{}

// ⛔ EVERY FIELD THE WIRE CARRIES ON A RECORD IS RENDERED, OR SAYS WHY NOT -
// AND SO IS EVERY FIELD OF ITS PROVENANCE.
//
// `recordJSON` is the third hand-written JSON object in this package and the
// most exposed: `rig record get`, `rig record query`, `rig record history` and
// `rig progress step` all answer through it, so one dropped field is four
// surfaces losing it at once. It is the object a consumer parses, which is why
// the walk compares against the EMITTED KEYS rather than against `Record`'s Go
// fields - a value read into the struct and never emitted is still dropped,
// and comparing against the struct would hide exactly that.
//
// ⛔ `rig progress step --json` NEEDS NO GUARD OF ITS OWN AND MUST NOT GROW
// ONE. It encodes `recordJSON(step, …)`: a step IS a record, under a comment in
// progress.go saying so on purpose. This test covers it by construction, and a
// second walk over the same function would be a second thing to keep in step.
func TestEveryFieldTheWireCarriesOnARecordIsRenderedOrSaysWhyNot(t *testing.T) {
	obj := recordJSON(Record{}, now)

	// ⛔ THE POSITIVE CONTROL, AND IT IS FIRST BECAUSE EVERY ASSERTION BELOW
	// IS AN ABSENCE. A walk that renders nothing and a walk that renders
	// everything correctly both produce an empty list of complaints; this is
	// the only line that separates them.
	prov, nested := obj["provenance"].(map[string]any)
	if !nested || len(obj) == 0 || len(prov) == 0 {
		t.Fatalf("recordJSON emitted %d top-level keys and a `provenance` of "+
			"%T - this test measures ABSENCES and cannot report anything "+
			"useful about an object it did not get. Fix the instrument "+
			"before reading its result", len(obj), obj["provenance"])
	}

	wireCoverage(t, "rigv1.Record", (&rigv1.Record{}).ProtoReflect().Descriptor().Fields(),
		obj, recordWireRendering, recordWireFieldsNotRendered)
	wireCoverage(t, "rigv1.Provenance", (&rigv1.Provenance{}).ProtoReflect().Descriptor().Fields(),
		prov, recordProvWireRendering, recordProvWireFieldsNotRendered)
}

// wireCoverage is the instrument behind the guard above, and it is written
// once because it walks two messages.
//
// ⛔ IT CHECKS FOUR DIRECTIONS, NOT ONE, AND THE LAST TWO ARE WHAT A RENAME MAP
// COSTS. `TestEveryFieldTheWireCarriesOnARefIsRendered` can use `slices.Equal`
// and get every direction free: an unrendered field, an invented key and a
// stale name are all one inequality. A declared map buys the ability to
// describe a rename and gives that up, so each direction is asserted by hand:
//
//   - a wire field rendered under NO key, with no reason recorded
//   - a wire field rendered AND excused, which means one of the two is stale
//   - a wire field PARTIALLY rendered, which only an expanding rename can be
//   - an entry in either table naming a field the wire no longer carries
//   - an emitted key NOTHING on the wire accounts for
//
// The fourth is the `//rig:allow` property: an exemption must not outlive what
// it excused. The fifth is the one a rename map silently loses, and losing it
// would let this object grow a key that answers to nothing.
func wireCoverage(t *testing.T, message string, fields protoreflect.FieldDescriptors,
	emitted map[string]any, renamed map[string][]string, excused map[string]string,
) {
	// ⛔ `t.Helper()` IS DELIBERATELY ABSENT, AND PUTTING IT BACK BLINDS THE
	// MUTATION HARNESS. It re-attributes a failure to the CALLER's line, and
	// this function holds SIX assertions that mean six different things. With
	// it, every one of them reports as whichever `wireCoverage(...)` call was
	// running - so three mutations aimed at three separate clauses came back
	// as two line numbers, and those two were the two MESSAGES being walked,
	// not two assertions.
	//
	// Measured here, 2026-09-17, by the mutation pass on this guard's own
	// first green. It is COORDINATION.md's "count the distinct assertions your
	// mutations turn red, not the mutations" arriving from a direction nobody
	// had recorded: not N mutations tripping one assertion, but N assertions
	// REPORTING AS one, so the instrument cannot tell whether they are
	// distinct. A helper is the right shape here - it walks two messages - and
	// the caller's line is strictly less informative than the assertion's,
	// because every message below already names which message it was reading.

	onTheWire := map[string]bool{}
	accountedFor := map[string]bool{}

	for i := range fields.Len() {
		name := string(fields.Get(i).Name())
		onTheWire[name] = true

		keys := renamed[name]
		if keys == nil {
			keys = []string{name}
		}
		var missing []string
		for _, k := range keys {
			if _, ok := emitted[k]; ok {
				accountedFor[k] = true
				continue
			}
			missing = append(missing, k)
		}
		why, isExcused := excused[name]

		switch {
		case len(missing) == 0 && isExcused:
			t.Errorf("%s.%s is BOTH rendered and excused (%q). One of the two "+
				"is stale, and an excuse beside a rendering is how a reader "+
				"learns to distrust the table", message, name, why)
		case len(missing) == 0:
			// Rendered under every key it declares. Nothing owed.
		case len(missing) < len(keys):
			// ⛔ ONLY AN EXPANDING RENAME REACHES THIS, and it is the half of
			// the rot a set comparison would have called a plain absence.
			// `at_unix_nano` becomes three keys; losing one of them leaves a
			// field that still looks carried from the wire's side.
			t.Errorf("%s.%s is PARTIALLY rendered: declared as %v, and %v is "+
				"missing.\nOne wire field expanding into several is exactly "+
				"where a drop hides - the field still appears rendered "+
				"because SOME of its keys are there", message, name, keys, missing)
		case isExcused:
			// Deliberately unrendered, with the reason written down.
		default:
			t.Errorf("the wire carries %s.%s and `recordJSON` emits nothing "+
				"for it, and no reason is recorded.\n"+
				"`rig record get`, `query`, `history` and `progress step` all "+
				"answer through this object, so a field dropped here is lost "+
				"on four surfaces at once.\n"+
				"Render it, declare its rename in the rendering table, or "+
				"excuse it WITH a reason.", message, name)
		}
	}

	for name, why := range excused {
		if !onTheWire[name] {
			t.Errorf("a reason is recorded for %s.%s (%q), which is not a "+
				"field on that message any more. Delete the row: an exemption "+
				"must not outlive what it excused", message, name, why)
		}
	}
	for name, keys := range renamed {
		if !onTheWire[name] {
			t.Errorf("the rendering table maps %s.%s to %v, and that field is "+
				"gone from the wire. A rename outliving its field is the same "+
				"defect as an excuse outliving one, and it is worse here: it "+
				"keeps %v accounted for, so the key it names can never be "+
				"reported as unexplained", message, name, keys, keys)
		}
	}
	for k := range emitted {
		if !accountedFor[k] {
			t.Errorf("`recordJSON` emits %q under %s and NOTHING ON THE WIRE "+
				"ACCOUNTS FOR IT.\nA key a consumer can read but the wire "+
				"cannot explain is a second source of truth: it either "+
				"belongs to a field that was removed, or it is invented here "+
				"and the reader has no way to learn what feeds it", k, message)
		}
	}
}

// ---- query: both filters are optional --------------------------------------

// queried runs `rig record query ...` against a fake and returns the project
// and kind the CLI decided to ask for. It asserts nothing, so a mutation pass
// attributes every failure to the case that made it.
func queried(t *testing.T, argv ...string) (string, string, error) {
	t.Helper()
	var gotProject, gotKind string
	serving(t, &fakeRecord{query: func(a QueryArgs) ([]Record, error) {
		gotProject, gotKind = a.Project, a.Kind
		return nil, nil
	}})
	err := run(append([]string{"record", "query"}, argv...))
	return gotProject, gotKind, err
}

// ⛔ AN OMITTED FILTER REACHES THE DAEMON AS AN EMPTY STRING, WHICH MEANS
// EVERY VALUE.
//
// This is the whole census fix at the prompt. `kind` is not a closed set, so
// while both filters were required positionally, a record written under an
// unguessed kind was invisible to every question anybody could write, and no
// answer about what the store holds could be complete. It cost one attack
// specialist 13 round trips and a hedge on a finding that should have been
// flat.
//
// `plan/39` specifies record.query as "by kind, field and project" and never
// says all three are mandatory: this surface invented the conjunction.
func TestBothQueryFiltersAreOptionalAndAbsentMeansEvery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		argv          []string
		project, kind string
	}{
		{"nothing at all is the whole store", nil, "", ""},
		{"one positional is the project", []string{"rig"}, "rig", ""},
		{"two positionals still work", []string{"rig", "note"}, "rig", "note"},
		{"a kind with no project", []string{"--kind", "project"}, "", "project"},
		{"a project by flag", []string{"--project", "rig"}, "rig", ""},
		{"both by flag", []string{"--project", "rig", "--kind", "note"}, "rig", "note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, k, err := queried(t, tc.argv...)
			if err != nil {
				t.Fatalf("%v was refused: %v", tc.argv, err)
			}
			if p != tc.project {
				t.Errorf("asked the daemon for project %q, want %q", p, tc.project)
			}
			if k != tc.kind {
				t.Errorf("asked the daemon for kind %q, want %q", k, tc.kind)
			}
		})
	}
}

// ⛔ `--kind project` WITH NO PROJECT IS THE WINDOW's PROJECT TAB, AND IT IS
// THE ONE QUESTION NO VERB COULD ANSWER.
//
// Nine record verbs are served - put, get, query, history, link, unlink, refs,
// progress.step, project.brief - and not one of them enumerates projects. The
// window's alternative was to hard-code "rig", which is a lie the moment a
// second project exists. The filter has to reach the daemon with the project
// EMPTY for this to work, so the case is pinned on its own rather than left to
// the table above.
func TestAskingForEveryProjectIsHowTheProjectsAreEnumerated(t *testing.T) {
	p, k, err := queried(t, "--kind", "project")
	if err != nil {
		t.Fatal(err)
	}
	if p != "" {
		t.Errorf("the project filter reached the daemon as %q; it must be "+
			"empty, or the answer is scoped to one project and cannot name "+
			"the others", p)
	}
	if k != "project" {
		t.Errorf("the kind filter reached the daemon as %q, want %q", k, "project")
	}
}

// ⛔ AN EXPLICITLY EMPTY FLAG IS REFUSED, AND THAT IS THE ONLY DEFENCE THERE
// IS AGAINST THE WIRE's AMBIGUITY.
//
// protojson omits an empty string exactly as it omits an unserved field, so
// `project: ""` meaning "every project" is byte-identical to a caller that
// forgot to set it. The daemon cannot tell them apart and does not try. The
// CLI can, through flag.Visit - the same distinction --if-version turns on -
// and a typed empty value is what a shell variable that expanded to nothing
// looks like.
func TestAFlagTypedEmptyIsRefusedRatherThanReadAsEverything(t *testing.T) {
	// ⛔ `field` IS IN THIS LIST NOW. It is the third filter and it has the
	// same accident: `--field "$F"` with F unset expands to nothing and is
	// byte-identical to a deliberate ask for no predicate at all.
	for _, flagName := range []string{"project", "kind", "field"} {
		t.Run(flagName, func(t *testing.T) {
			var reached bool
			serving(t, &fakeRecord{query: func(QueryArgs) ([]Record, error) {
				reached = true
				return nil, nil
			}})
			err := run([]string{"record", "query", "--" + flagName, ""})
			if err == nil {
				t.Fatalf("--%s \"\" was accepted and read as every %s. An "+
					"omitted flag already says that; an empty one is a "+
					"variable that expanded to nothing", flagName, flagName)
			}
			if !strings.Contains(err.Error(), "--"+flagName) {
				t.Errorf("the refusal does not name --%s:\n%s", flagName, err)
			}
			if reached {
				t.Errorf("--%s \"\" reached the daemon before it was refused", flagName)
			}
		})
	}
}

// A FILTER GIVEN TWICE IS REFUSED RATHER THAN SILENTLY PREFERRED.
func TestAQueryFilterGivenBothWaysIsRefused(t *testing.T) {
	_, _, err := queried(t, "rig", "note", "--kind", "decision")
	if err == nil {
		t.Fatal("`record query rig note --kind decision` was accepted; one of " +
			"the two kinds would have been silently dropped")
	}
	if !strings.Contains(err.Error(), "twice") {
		t.Errorf("the refusal does not say the filter was given twice:\n%s", err)
	}
}

// ---- query: the listing says what it was asked --------------------------

// ⛔ AN UNFILTERED LISTING IS A DIFFERENT SENTENCE, NOT A BLANK IN THE SAME
// ONE. The old wording formatted straight through both filters, so no kind
// and no project printed "no  records in .", which reads as a broken command.
func TestAnEmptyUnfilteredQueryIsStillASentence(t *testing.T) {
	got := queryText(QueryArgs{}, nil, 0)
	if strings.Contains(got, "in .") || strings.Contains(got, "no  ") {
		t.Errorf("an unfiltered empty query formatted its missing filters "+
			"into the sentence:\n%s", got)
	}
	if !strings.Contains(got, "no records") {
		t.Errorf("an unfiltered empty query does not say that no records "+
			"were found:\n%s", got)
	}
}

// ⛔ A FILTERED EMPTY ANSWER NAMES THE WAY OUT, because a reader cannot
// otherwise tell an empty store from a filter that matches nothing - and
// until both filters became optional there was no command that could tell
// them either.
func TestAFilteredEmptyQueryNamesTheUnfilteredOne(t *testing.T) {
	got := queryText(QueryArgs{Project: "rig", Kind: "requirement"}, nil, 0)
	if !strings.Contains(got, "rig record query") {
		t.Errorf("an empty filtered answer does not name the unfiltered "+
			"query, which is the only way to find a kind spelled "+
			"differently:\n%s", got)
	}

	unfiltered := queryText(QueryArgs{}, nil, 0)
	if strings.Contains(unfiltered, "rig record query` with no filter") {
		t.Errorf("the unfiltered answer advised itself:\n%s", unfiltered)
	}
}

// ⛔ THE PROJECT COLUMN APPEARS EXACTLY WHEN THE PROJECT VARIES. Always, and
// every scoped call prints one repeated word; never, and an unscoped listing
// is a pile of ids from projects the reader cannot tell apart.
func TestTheProjectColumnAppearsOnlyWhenTheProjectWasNotFiltered(t *testing.T) {
	rs := []Record{
		record(func(r *Record) { r.ID = "a"; r.Project = "rig" }),
		record(func(r *Record) { r.ID = "b"; r.Project = "standards" }),
	}

	unscoped := queryText(QueryArgs{}, rs, 0)
	if !strings.Contains(unscoped, "PROJECT") {
		t.Errorf("an unscoped listing has no PROJECT column, so two records "+
			"from two projects render identically:\n%s", unscoped)
	}
	if !strings.Contains(unscoped, "standards") {
		t.Errorf("an unscoped listing does not print the projects:\n%s", unscoped)
	}

	scoped := queryText(QueryArgs{Project: "rig"}, rs[:1], 0)
	if strings.Contains(scoped, "PROJECT") {
		t.Errorf("a listing scoped to one project printed a PROJECT column of "+
			"one repeated word:\n%s", scoped)
	}
}

// THE NOUN AGREES WITH THE COUNT. `1 records in rig` is the same defect as
// `1 edge point at B9`, caught here rather than shipped.
func TestTheListingFooterAgreesWithItsCount(t *testing.T) {
	one := queryText(QueryArgs{Project: "rig"}, []Record{record()}, 0)
	if !strings.Contains(one, "1 record in rig") {
		t.Errorf("a listing of one says:\n%s", one)
	}
	two := queryText(QueryArgs{Project: "rig"}, []Record{record(), record()}, 0)
	if !strings.Contains(two, "2 records in rig") {
		t.Errorf("a listing of two says:\n%s", two)
	}
}

// ---- the names, across the layers ------------------------------------------

// ⛔ ONE THING WITH THREE NAMES AND NO TEST COMPARING THEM COST A WHOLE ATTACK
// RUN ITS MOST-REPEATED NUMBER.
//
// The Go field is `Refs.Refs`, the wire field is `refs`, the text header is
// `SRC`, and this client's JSON said `in`. On 2026-09-17 at least three
// specialists swept the store with `rig record refs --json` keyed on `refs`,
// got a clean `0` from every record, and that 0 matched a stale figure in
// their brief - so the wrong instrument confirmed itself. It was caught by a
// positive control, not by anybody doubting the number.
//
// THE TEST IS THE DURABLE HALF. It walks the WIRE's own descriptors and
// requires every field name to be a key this client emits, so the next
// divergence goes red at the moment somebody adds a field rather than months
// later in somebody's shell loop. `in` is allowed as an EXTRA key, never as a
// replacement, and the case below pins it to the same slice.
func TestTheJSONKeysAreTheWiresOwnFieldNames(t *testing.T) {
	got := refsJSON(Refs{
		ID: "r39", Depth: 4, Truncated: true,
		In:     []Ref{{Src: "d-1", Type: "cites", Via: "r39", Kind: "decision", Title: "t", Distance: 1}},
		Cycles: [][]string{{"a", "b"}},
	})

	top := (&rigv1.RecordRefsResponse{}).ProtoReflect().Descriptor().Fields()
	for i := range top.Len() {
		name := string(top.Get(i).Name())
		if _, ok := got[name]; !ok {
			t.Errorf("the wire calls a field %q and `record refs --json` emits "+
				"no such key. A caller keyed on the wire's own name reads the "+
				"absence as zero, which is the defect this test exists for. "+
				"Keys emitted: %v", name, emittedKeys(got))
		}
	}

	rows, ok := got["refs"].([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("the `refs` key is %T, want a list of one row", got["refs"])
	}
	row := rigv1.Ref{}
	rf := row.ProtoReflect().Descriptor().Fields()
	for i := range rf.Len() {
		name := string(rf.Get(i).Name())
		if _, ok := rows[0][name]; !ok {
			t.Errorf("the wire calls a Ref field %q and the emitted row has no "+
				"such key. Row keys: %v", name, emittedKeys(rows[0]))
		}
	}
}

// ⛔ `in` IS AN ALIAS AND MUST CARRY THE SAME SLICE AS `refs`. Two keys that
// can disagree is the original defect with one more place to hide.
func TestTheInAliasCannotDriftFromRefs(t *testing.T) {
	got := refsJSON(Refs{
		ID: "r39", Depth: 4,
		In: []Ref{{Src: "d-1", Type: "cites", Via: "r39", Distance: 1}},
	})

	refs, err := json.Marshal(got["refs"])
	if err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(got["in"])
	if err != nil {
		t.Fatal(err)
	}
	if string(refs) != string(in) {
		t.Fatalf("`refs` and `in` disagree:\n  refs: %s\n  in:   %s", refs, in)
	}

	// AND NEITHER IS null ON AN EMPTY ANSWER, because a null is what a sweep
	// reads as "this was never considered" - which is precisely how the
	// original mis-keyed sweep produced a confident zero.
	empty := refsJSON(Refs{ID: "r39", Depth: 4})
	for _, key := range []string{"refs", "in"} {
		b, err := json.Marshal(empty[key])
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "[]" {
			t.Errorf("an answer with no edges emitted %q: %s, want []", key, b)
		}
	}
}

// ⛔ THE TEXT HEADER IS THE THIRD NAME AND IT IS THE WIRE's TOO. `SRC` is
// `src` upper-cased, which is the rule this table already follows; a header
// that invented its own word would put a fourth name on one thing.
func TestTheTextHeadersAreTheWiresOwnFieldNames(t *testing.T) {
	out := refsText(Refs{
		ID: "r39", Depth: 4,
		In: []Ref{{Src: "d-1", Type: "cites", Via: "r39", Kind: "decision", Distance: 1}},
	})
	for _, want := range []string{"SRC", "TYPE", "VIA", "KIND"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refs table has no %s column:\n%s", want, out)
		}
	}
}

// emittedKeys names what a rendered JSON object actually carries, so a failure
// above says what WAS emitted rather than only what was missing.
//
// Named apart from refusal_verbs_test.go's sortedKeys, which is the same shape
// over map[string]bool. Two names is correct here: one thing with two names is
// this file's subject, and two THINGS sharing one name is the other half of
// the same mistake.
func emittedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ⛔ THE REFS FOOTER's VERB AGREES WITH ITS SUBJECT. It printed "1 edge point
// at B9" and S5 reported it as a micro-defect on a human surface.
//
// It is a separate test from the listing footer because they are separate
// sentences in separate renderers: a mutation pass proved it, killing the
// listing's agreement and leaving the refs table's alone.
func TestTheRefsFooterVerbAgreesWithItsSubject(t *testing.T) {
	one := refsText(Refs{ID: "B9", Depth: 4, In: []Ref{
		{Src: "d-1", Type: "part-of", Via: "B9", Distance: 1},
	}})
	if !strings.Contains(one, "1 edge points at B9") {
		t.Errorf("a single edge reads:\n%s", one)
	}

	two := refsText(Refs{ID: "B9", Depth: 4, In: []Ref{
		{Src: "d-1", Type: "part-of", Via: "B9", Distance: 1},
		{Src: "d-2", Type: "part-of", Via: "B9", Distance: 1},
	}})
	if !strings.Contains(two, "2 edges point at B9") {
		t.Errorf("two edges read:\n%s", two)
	}
}

// ⛔ A TRUNCATED ANSWER NAMES BOTH WAYS OUT, because the flag now has two
// sources and the wire carries one bool for both. Naming only --depth sends a
// caller to spend a deeper traversal on an answer a deeper traversal cannot
// change: a record dropped by the project scope comes back with
// --cross-project and never with a bigger number.
func TestATruncatedAnswerNamesBothWaysOut(t *testing.T) {
	got := refsText(Refs{ID: "r39", Depth: 4, Truncated: true})
	for _, want := range []string{"--depth", "--cross-project"} {
		if !strings.Contains(got, want) {
			t.Errorf("a truncated answer does not offer %s:\n%s", want, got)
		}
	}
}

// ⛔ A TRUNCATED EMPTY ANSWER MUST NOT SAY "NOTHING POINTS AT THIS".
//
// That sentence is the strongest claim the verb makes, and a truncated walk
// is exactly the case where it is false: a record past the depth bound, or
// one the project scope crossed and left out, DOES point at the subject. The
// truncation qualifier arrives two lines later, and a correction after the
// claim is how a reader keeps the claim.
//
// This is the defect the traversal repair was about, one layer up, and it was
// found by an adversarial pass over the diff rather than by a test.
func TestATruncatedEmptyAnswerDoesNotClaimTheRecordIsUncited(t *testing.T) {
	cut := refsText(Refs{ID: "r39", Depth: 4, Truncated: true})
	if strings.Contains(cut, "nothing points at") {
		t.Errorf("a truncated empty answer still claims nothing points at the "+
			"record:\n%s", cut)
	}
	if !strings.Contains(strings.ToUpper(cut), "TRUNCATED") {
		t.Errorf("a truncated empty answer does not say so:\n%s", cut)
	}

	// AND THE UNTRUNCATED ONE STILL MAKES THE CLAIM, because an uncited
	// record is what this verb exists to make visible and hedging every
	// answer would destroy the one it is for.
	whole := refsText(Refs{ID: "r39", Depth: 4})
	if !strings.Contains(whole, "nothing points at r39") {
		t.Errorf("a complete empty answer no longer states the absence:\n%s", whole)
	}
}

// ⛔ AN EMPTY POSITIONAL IS THE SAME ACCIDENT AS AN EMPTY FLAG.
//
// `rig record query "$P" note` with P unset reaches the parser as a
// positional, so the flag guard never sees it and the store would be asked
// for every project. Found by an adversarial pass over the diff.
func TestAnEmptyPositionalIsRefusedTheSameWayAnEmptyFlagIs(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"an empty project", []string{""}, "project"},
		{"an empty kind", []string{"rig", ""}, "kind"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reached bool
			serving(t, &fakeRecord{query: func(QueryArgs) ([]Record, error) {
				reached = true
				return nil, nil
			}})
			err := run(append([]string{"record", "query"}, tc.argv...))
			if err == nil {
				t.Fatalf("%v was accepted and read as every %s", tc.argv, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not name the %s:\n%s", tc.want, err)
			}
			if reached {
				t.Errorf("%v reached the daemon before it was refused", tc.argv)
			}
		})
	}
}

// TestTheFieldPredicateReachesTheAPI is B65 at the prompt: the store gained a
// field filter and the question is whether anything a person types can reach
// it.
//
// ⛔ IT ASSERTS THE WHOLE FILTER, NOT THAT Query WAS CALLED. A flag wired to
// the wrong struct member, or dropped between the flag set and QueryArgs,
// still calls Query - and every existing query test would stay green, because
// they only ever looked at the project and the kind.
func TestTheFieldPredicateReachesTheAPI(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want QueryArgs
	}{
		{
			"field and value alone",
			[]string{"--field", "status", "--value", "closed"},
			QueryArgs{Field: "status", Value: "closed"},
		},
		{
			"all three filters at once",
			[]string{"rig", "work-item", "--field", "owner", "--value", "boris"},
			QueryArgs{Project: "rig", Kind: "work-item", Field: "owner", Value: "boris"},
		},
		{
			// ⛔ AN EMPTY --value IS A REAL QUERY AND MUST SURVIVE THE TRIP.
			// It is the one place the empty-means-every rule does NOT apply,
			// and a layer that "helpfully" drops it turns "records whose
			// status is empty" into "records with any status".
			"an explicitly empty value is carried, not dropped",
			[]string{"--field", "closure_note", "--value", ""},
			QueryArgs{Field: "closure_note", Value: ""},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeRecord{}
			serving(t, f)
			if err := run(append([]string{"record", "query"}, tc.argv...)); err != nil {
				t.Fatalf("rig record query %v: %v", tc.argv, err)
			}
			if f.lastFilter != tc.want {
				t.Errorf("the CLI asked for %+v, want %+v", f.lastFilter, tc.want)
			}
		})
	}
}

// TestAValueWithNoFieldIsRefusedAtThePrompt is the CLI half of the refusal.
// The store refuses it too, and the two are not redundant: only this layer can
// tell a value that was TYPED from one the daemon simply received, because on
// the wire an unset field and an empty one are the same bytes.
func TestAValueWithNoFieldIsRefusedAtThePrompt(t *testing.T) {
	var reached bool
	serving(t, &fakeRecord{query: func(QueryArgs) ([]Record, error) {
		reached = true
		return nil, nil
	}})
	err := run([]string{"record", "query", "--value", "closed"})
	if err == nil {
		t.Fatal("--value with no --field was accepted. Dropping the predicate " +
			"answers with EVERY record, which is wider than what was asked and " +
			"nothing in the output would say so")
	}
	if !strings.Contains(err.Error(), "--field") {
		t.Errorf("the refusal is %q and does not name --field, so it does not "+
			"say how to fix it", err)
	}
	if reached {
		t.Error("the refusal still called the API; it must refuse before the round trip")
	}
}
