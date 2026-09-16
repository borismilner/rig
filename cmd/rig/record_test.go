package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
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

	put     func(PutArgs) (Record, error)
	get     func(string, uint64) (Record, error)
	query   func(string, string) ([]Record, error)
	history func(string) ([]Record, error)
	refs    func(RefsArgs) (Refs, error)
	step    func(StepArgs) (Record, error)
	brief   func(string) (Brief, error)
	linkErr error
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

func (f *fakeRecord) Query(_ context.Context, project, kind string) ([]Record, error) {
	f.calls = append(f.calls, "query")
	if f.query != nil {
		return f.query(project, kind)
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
	got := queryText("rig", "requirement", nil)

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

	for _, want := range []string{"backend-1", "record", "s-4f2"} {
		if !strings.Contains(got, want) {
			t.Errorf("the history does not carry %q, so it cannot answer who "+
				"wrote a version:\n%s", want, got)
		}
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
		{"a query missing its kind", []string{"record", "query", "rig"}, "usage"},
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
			_, err := a.Query(ctx, "rig", "note")
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
