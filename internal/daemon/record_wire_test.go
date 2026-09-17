package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/instance"
	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/record"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The record verbs over the real wire (PLAN.md section 39).
//
// EVERY TEST HERE DIALS A UNIX SOCKET AND SPEAKS FRAMES. The point of the
// exercise is that section 39's verbs are REACHABLE under the names the
// specification promises - `rig.record.put` and not some flattened spelling -
// and reachability is the one property a unit test on a handler cannot show.

// upRecordDaemon stands a NAMED estate up, which is what gives it a store.
//
// The estate name is the whole precondition: record.Open resolves
// paths.EstateStateDir(name), so an unnamed estate has nowhere to put a record
// and the verbs refuse. XDG_STATE_HOME is redirected so the test never touches
// the developer's own record.
func upRecordDaemon(t *testing.T) string {
	const estate = "recordwire"
	t.Helper()
	// Short, deliberately: sun_path is 108 bytes and t.TempDir under a long
	// TMPDIR silently exceeds it, failing as EINVAL rather than as a path
	// length. The same trap is recorded in upDaemonLogged.
	dir, err := os.MkdirTemp("", "rigr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	sock := filepath.Join(dir, "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{
		Version: "test", Wire: "v1", Lock: lock, Estate: estate, Epoch: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.records == nil {
		t.Fatalf("estate %q opened no record store, so nothing below tests the wire", estate)
	}
	t.Cleanup(func() { _ = d.records.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock
}

func recordCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// seated dials and announces, because a write needs an attributable author.
func seated(t *testing.T, sock, seat string) *client.Client {
	t.Helper()
	c := dial(t, sock)
	if err := c.Call(recordCtx(t), "rig.announce", &rigv1.AnnounceRequest{
		Seat: seat, Purpose: "exercising the record verbs", Activity: "testing",
	}, &rigv1.AnnounceResponse{}); err != nil {
		t.Fatalf("rig.announce(%q): %v", seat, err)
	}
	return c
}

// TestTheRecordVerbsAreReachableUnderSection39sOwnNames is the demonstration.
//
// It drives a work item start to finish - put, step, brief - which is section
// 39's slice-2 sentence, over the wire rather than over the package.
func TestTheRecordVerbsAreReachableUnderSection39sOwnNames(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	// The project, whose id IS its slug - section 39's id-scheme exception.
	var proj rigv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Id: "rig", Kind: "project", Project: "rig", Body: "rig itself",
	}, &proj); err != nil {
		t.Fatalf("rig.record.put(project): %v", err)
	}
	if got := proj.GetRecord().GetVersion(); got != 1 {
		t.Errorf("a first write landed at version %d, not 1", got)
	}

	// ⛔ PROVENANCE IS THE DAEMON'S AND THE REQUEST HAS NOWHERE TO PUT IT.
	// RecordPutRequest carries no session, seat or epoch field at all, so this
	// assertion is about what the daemon FILLED rather than what it copied.
	prov := proj.GetRecord().GetProv()
	if prov.GetSeat() != "team-lead" {
		t.Errorf("the record was attributed to %q, not to the announcing seat", prov.GetSeat())
	}
	if prov.GetEpoch() != 6 {
		t.Errorf("the epoch was %d, not the daemon's 6", prov.GetEpoch())
	}
	if prov.GetSession() == "" {
		t.Error("no session was stamped, so the write cannot be traced to a session")
	}
	if prov.GetAtUnixNano() == 0 {
		t.Error("no timestamp was stamped")
	}

	// A work item, then a step against it.
	var item rigv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Kind: "work-item", Project: "rig", Body: "put the record verbs on the wire",
		Fields: map[string]string{"title": "the wire surface", "status": "active"},
	}, &item); err != nil {
		t.Fatalf("rig.record.put(work-item): %v", err)
	}
	id := item.GetRecord().GetId()
	if id == "" {
		t.Fatal("an empty id was not minted")
	}

	var step rigv1.ProgressStepResponse
	if err := c.Call(ctx, "rig.progress.step", &rigv1.ProgressStepRequest{
		Item: id, State: rigv1.StepState_STEP_STATE_STARTED, Note: "proto landed",
	}, &step); err != nil {
		t.Fatalf("rig.progress.step: %v", err)
	}
	if got := step.GetStep().GetVersion(); got != 1 {
		t.Errorf("a step landed at version %d; a stream is append-only so every step is 1", got)
	}

	// The brief derives it back. NOTHING HERE WAS STORED AS PROSE.
	var brief rigv1.ProjectBriefResponse
	if err := c.Call(ctx, "rig.project.brief", &rigv1.ProjectBriefRequest{
		Project: "rig",
	}, &brief); err != nil {
		t.Fatalf("rig.project.brief: %v", err)
	}
	if len(brief.GetNextUp()) == 0 {
		t.Fatal("the brief derived no next-up item from a started work item")
	}
	found := false
	for _, it := range brief.GetNextUp() {
		if it.GetId() == id {
			found = true
			if it.GetState() != rigv1.StepState_STEP_STATE_STARTED {
				t.Errorf("the item came back in state %v, not the step that was written", it.GetState())
			}
			if it.GetSinceUnixNano() == 0 {
				t.Error("an item with a step came back with a zero Since, which means 'never stepped'")
			}
		}
	}
	if !found {
		t.Errorf("the item just stepped is not in next-up: %+v", brief.GetNextUp())
	}
}

// TestATerminalWritesUnderASeatTheDaemonMinted is the provenance rule with its
// teeth in, and it replaces a test that pinned the opposite.
//
// ⛔ THE TEST IT REPLACES ASSERTED THAT AN UNANNOUNCED WRITE IS REFUSED, AND IT
// PASSED FOR AS LONG AS THE MVP WAS IMPOSSIBLE. `record.put`, `record.link`,
// `record.unlink` and `progress.step` demanded an announced seat, and there is
// no `rig announce` at a terminal - so section 39's demonstration, seeding
// rig's own backlog THROUGH THE CLI, could not be performed from any surface.
// A green test guarded the hole. Ruled 2026-09-17: the daemon names the caller.
//
// What must stay true is the part that was never about announcing: the seat is
// the DAEMON'S and the caller cannot supply it.
func TestATerminalWritesUnderASeatTheDaemonMinted(t *testing.T) {
	sock := upRecordDaemon(t)
	c := dial(t, sock) // deliberately NOT seated - this is a terminal

	var put rigv1.RecordPutResponse
	if err := c.Call(recordCtx(t), "rig.record.put", &rigv1.RecordPutRequest{
		Id: "T1", Kind: "note", Project: "rig", Body: "who wrote this?",
	}, &put); err != nil {
		t.Fatalf("a terminal could not write, so the CLI demonstration is "+
			"unreachable again: %v", err)
	}

	seat := put.GetRecord().GetProv().GetSeat()
	if !strings.HasPrefix(seat, "terminal:") {
		t.Errorf("the record's seat is %q, and a terminal's write must be "+
			"attributed to a terminal seat the daemon minted", seat)
	}
	if u, err := user.LookupId(strconv.Itoa(os.Getuid())); err == nil &&
		u.Username != "" && seat != "terminal:"+u.Username {
		t.Errorf("the seat is %q and the daemon is one per unix user, so it "+
			"should name %q", seat, "terminal:"+u.Username)
	}
	if put.GetRecord().GetProv().GetSession() == "" {
		t.Error("the record names a seat and no session, so two invocations " +
			"by the same user are indistinguishable")
	}
}

// TestAnAnnouncedSeatBeatsTheTerminalFallback.
//
// ⛔ THE FALLBACK MUST BE A FALLBACK. If it ever ran first, every seated peer's
// work would be filed under one shared `terminal:` name and the roster's whole
// point - who is doing what - would be silently gone from the record while
// every test that only checks "a seat is set" stayed green.
func TestAnAnnouncedSeatBeatsTheTerminalFallback(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")

	var put rigv1.RecordPutResponse
	if err := c.Call(recordCtx(t), "rig.record.put", &rigv1.RecordPutRequest{
		Id: "T2", Kind: "note", Project: "rig", Body: "seated",
	}, &put); err != nil {
		t.Fatalf("rig.record.put: %v", err)
	}
	if got := put.GetRecord().GetProv().GetSeat(); got != "team-lead" {
		t.Errorf("a seated connection wrote under %q, not its announced seat", got)
	}
}

// TestEachHalfOfTheTerminalGuardIsReachableOnItsOwn measures terminalSeat
// directly, and it exists because the wire test above CANNOT.
//
// ⛔ THE WIRE TEST PINS THE CONJUNCTION AND NEITHER MEMBER, WHICH IS ONLY
// VISIBLE FROM A MUTATION PASS. Measured: deleting `c.scoped.Load()` leaves it
// GREEN, and deleting `p.Kind != KindTerminal` leaves it GREEN; only deleting
// BOTH turns it red. The two conditions are independently sufficient against a
// registered program, so over the wire each one hides whether the other can
// fire at all - a guard that survives every mutation aimed at it is not
// protecting anything that has been demonstrated.
//
// They are kept as two because they are two different facts that can change
// apart: `scoped` is section 14's authorisation boolean, set by the program
// handshake, and `Kind` is what the kernel minted at accept. This table is the
// smaller unit of measurement that can tell them apart, so each row below is a
// mutation target the wire test could not offer.
func TestEachHalfOfTheTerminalGuardIsReachableOnItsOwn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scoped bool
		kind   kernel.ClientKind
		want   bool // a seat is minted
	}{
		{"an unannounced terminal", false, kernel.KindTerminal, true},
		{"a SCOPED connection that is still KindTerminal", true, kernel.KindTerminal, false},
		{"a registered program", false, kernel.KindProgram, false},
		{"an agent", false, kernel.KindAgent, false},
		{"a principal that never said what it is", false, kernel.KindUnspecified, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &conn{}
			c.scoped.Store(tc.scoped)
			p := kernel.Principal{UID: os.Getuid(), Kind: tc.kind}

			got := terminalSeat(c, p)
			if (got != "") != tc.want {
				t.Fatalf("terminalSeat returned %q, and this caller %s be named",
					got, map[bool]string{true: "must", false: "must NOT"}[tc.want])
			}
			if tc.want && !strings.HasPrefix(got, "terminal:") {
				t.Errorf("the minted seat is %q and must be marked as a terminal's, "+
					"so a reader of the record can tell it from an announced seat", got)
			}
		})
	}
}

// TestARegisteredProgramIsStillRefusedByName.
//
// ⛔ THE TERMINAL FALLBACK IS NOT A GENERAL AMNESTY, AND THIS IS THE EDGE THAT
// SAYS SO. A registered program has an identity of its own, so writing its work
// down as a terminal's would be a lie the store could never afterwards tell
// from the truth. It announces or it does not write - and unlike a terminal it
// CAN, which is why the refusal still names `announce` and is still correct.
func TestARegisteredProgramIsStillRefusedByName(t *testing.T) {
	sock := upRecordDaemon(t)
	c := program(t, sock, "fakeapp") // scoped by the handshake, never announced

	err := c.Call(recordCtx(t), "rig.record.put", &rigv1.RecordPutRequest{
		Id: "T3", Kind: "note", Project: "rig", Body: "who wrote this?",
	}, &rigv1.RecordPutResponse{})
	if err == nil {
		t.Fatal("a registered program wrote a record without announcing, so " +
			"the terminal fallback has become a general amnesty")
	}
	if !strings.Contains(err.Error(), "seat") {
		t.Errorf("the refusal does not mention a seat: %v", err)
	}

	var refusal *client.CallError
	if !errors.As(err, &refusal) {
		t.Fatalf("the refusal arrived unstructured: %T %v", err, err)
	}
	if !strings.Contains(refusal.Status.GetFix(), "announce") {
		t.Errorf("the fix does not tell the caller to announce: %q", refusal.Status.GetFix())
	}
}

// TestALostCompareAndSwapIsAConflictAndNotJustInvalid.
//
// The two are different instructions to a caller: a conflict is retryable
// after a re-read and an invalid argument is not. A caller that cannot tell
// them apart retries the one that never succeeds.
func TestALostCompareAndSwapIsAConflictAndNotJustInvalid(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	var first rigv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Kind: "note", Project: "rig", Body: "one",
	}, &first); err != nil {
		t.Fatal(err)
	}
	id := first.GetRecord().GetId()

	// Move it to version 2, so the caller below is holding a stale 1.
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Id: id, IfVersion: 1, Kind: "note", Project: "rig", Body: "two",
	}, &rigv1.RecordPutResponse{}); err != nil {
		t.Fatal(err)
	}

	err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Id: id, IfVersion: 1, Kind: "note", Project: "rig", Body: "also two",
	}, &rigv1.RecordPutResponse{})
	if err == nil {
		t.Fatal("a stale compare-and-swap overwrote a newer version")
	}
	var refusal *client.CallError
	if !errors.As(err, &refusal) {
		t.Fatalf("the refusal arrived unstructured: %T %v", err, err)
	}
	if refusal.Code() != rigv1.Code_CODE_CONFLICT {
		t.Errorf("a lost compare-and-swap answered %v, so a caller cannot tell it from a bad argument",
			refusal.Code())
	}
}

// TestVersionZeroMeansHeadAndNotVersionZero.
//
// No record has version 0 - the first write is 1 - so the zero cannot collide
// with a version anybody meant, and reading it as HEAD is what makes the
// common call require no field at all.
func TestVersionZeroMeansHeadAndNotVersionZero(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	var first rigv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Kind: "note", Project: "rig", Body: "one",
	}, &first); err != nil {
		t.Fatal(err)
	}
	id := first.GetRecord().GetId()
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Id: id, IfVersion: 1, Kind: "note", Project: "rig", Body: "two",
	}, &rigv1.RecordPutResponse{}); err != nil {
		t.Fatal(err)
	}

	var head rigv1.RecordGetResponse
	if err := c.Call(ctx, "rig.record.get", &rigv1.RecordGetRequest{Id: id}, &head); err != nil {
		t.Fatalf("rig.record.get at head: %v", err)
	}
	if got := head.GetRecord().GetBody(); got != "two" {
		t.Errorf("version 0 answered %q, so it was read as a version rather than as HEAD", got)
	}

	var old rigv1.RecordGetResponse
	if err := c.Call(ctx, "rig.record.get",
		&rigv1.RecordGetRequest{Id: id, Version: 1}, &old); err != nil {
		t.Fatalf("rig.record.get at version 1: %v", err)
	}
	if got := old.GetRecord().GetBody(); got != "one" {
		t.Errorf("version 1 answered %q", got)
	}

	// And history carries both, oldest first, each with its own provenance.
	var hist rigv1.RecordHistoryResponse
	if err := c.Call(ctx, "rig.record.history",
		&rigv1.RecordHistoryRequest{Id: id}, &hist); err != nil {
		t.Fatalf("rig.record.history: %v", err)
	}
	if len(hist.GetVersions()) != 2 {
		t.Fatalf("history holds %d versions, not 2", len(hist.GetVersions()))
	}
	if hist.GetVersions()[0].GetVersion() != 1 {
		t.Error("history is not oldest-first")
	}
}

// TestAnUnnamedEstateServesTheRecordFromAnEphemeralStore.
//
// ⛔ THIS TEST REPLACES TestAnUnnamedEstateRefusesTheRecordVerbsAndSaysWhy,
// WHICH PINNED THE OPPOSITE CONTRACT, AND THE REPLACEMENT IS RECORDED RATHER
// THAN QUIET. The old behaviour was reasoned and was not a bug: section 37
// gives an unnamed estate no PERSISTENT state, so the daemon carried a nil
// store and every record verb refused with the cause named.
//
// ⛔ WHAT CHANGED IS THAT THE COST WAS FINALLY COUNTED. Section 09's A0
// survey, 2026-09-17, `[ran it]`: a third estate NAME is refused by rigd, an
// unnamed estate had no store, and a second daemon on a named estate is B72 -
// every door shut, so every write an agent made landed in one of the two
// estates on the human's screen. Four lead generations wrote nothing for that
// survey and the reason was that the instrument did not exist.
//
// ⛔ SECTION 37's RULE IS NOT WEAKENED AND THIS TEST IS WHERE THAT IS PROVED.
// The scratch store lives in the RUNTIME directory rather than the state
// directory, so it cannot survive one, and no third estate arrives by the back
// door. Persistent state still needs a name; only ephemeral state does not.
func TestAnUnnamedEstateServesTheRecordFromAnEphemeralStore(t *testing.T) {
	sock, _ := upDaemon(t, nil) // unnamed
	c := dial(t, sock)

	var got rigv1.ProjectBriefResponse
	if err := c.Call(recordCtx(t), "rig.project.brief",
		&rigv1.ProjectBriefRequest{Project: "rig"}, &got); err != nil {
		t.Fatalf("an unnamed estate still cannot serve the record, so an agent "+
			"has nowhere to write that is not somebody's live estate: %v", err)
	}

	// ⛔ AND IT MUST ACCEPT A WRITE, NOT ONLY A READ. A read-only sandbox is
	// not a sandbox: the survey's whole difficulty was the WRITE half, and a
	// brief that answers on an empty store would pass an assertion about
	// reading while leaving the measured gap exactly where it was.
	var put rigv1.RecordPutResponse
	if err := c.Call(recordCtx(t), "rig.record.put", &rigv1.RecordPutRequest{
		Id: "scratch-1", Kind: "note", Project: "rig", Body: "written to scratch",
	}, &put); err != nil {
		t.Fatalf("the scratch store refused a write: %v", err)
	}

	var back rigv1.RecordGetResponse
	if err := c.Call(recordCtx(t), "rig.record.get",
		&rigv1.RecordGetRequest{Id: "scratch-1"}, &back); err != nil {
		t.Fatalf("what was written could not be read back: %v", err)
	}
	if body := back.GetRecord().GetBody(); body != "written to scratch" {
		t.Errorf("the scratch store lost the body: got %q", body)
	}
}

// TestNoServedRequestFieldIsSilentlyDropped walks every record request message
// and fails on a field no handler reads.
//
// ⛔ IT EXISTS BECAUSE ONE ALREADY SHIPPED. `ProgressStepRequest` carried
// `string evidence = 4` for one commit: section 39 names evidence in its
// definition of a step, so it went on the wire, and nothing ever read it -
// record.StepRequest has no such field and serveProgressStep never called
// GetEvidence. A caller could set it, get a SUCCESS, and lose what it sent.
//
// WHY THAT IS WORSE THAN AN UNDECLARED VERB, which is the failure section 39
// spends paragraphs refusing for standard.stamp and project.gate: an
// undeclared verb REFUSES, so the caller finds out in one call. A served field
// that is dropped ANSWERS. The rule that selfDeclaration() is the contract
// does not reach inside a message the daemon already serves, and this test is
// what covers that gap.
//
// THE CHECK IS THE DESCRIPTOR AGAINST THIS FILE'S OWN LIST, deliberately, and
// not a walk of the handler source. A source walk would have to parse Go to
// decide whether a GetX() result reaches the store, which is the second source
// of truth this repository keeps refusing to build. A hand-kept list fails
// LOUDLY on the one event that matters - somebody adds a field - and the
// failure names the field and tells them what to do about it.
func TestNoServedRequestFieldIsSilentlyDropped(t *testing.T) {
	// read is every field the daemon actually takes off the request and passes
	// to internal/record. Adding a field here is a claim that record.go reads
	// it; adding one to the proto without adding it here is the defect.
	read := map[string][]string{
		"RecordPutRequest":     {"id", "if_version", "kind", "project", "body", "fields"},
		"RecordGetRequest":     {"id", "version"},
		"RecordQueryRequest":   {"project", "kind", "field", "value"},
		"RecordHistoryRequest": {"id"},
		"RecordLinkRequest":    {"src", "type", "dst"},
		"RecordUnlinkRequest":  {"src", "type", "dst"},
		"ProgressStepRequest":  {"item", "state", "note"},
		"ProjectBriefRequest":  {"project"},
	}

	files := (&rigv1.ProgressStepRequest{}).ProtoReflect().Descriptor().ParentFile()
	msgs := files.Messages()
	seen := 0
	for i := range msgs.Len() {
		md := msgs.Get(i)
		want, ok := read[string(md.Name())]
		if !ok {
			continue
		}
		seen++
		isRead := map[string]bool{}
		for _, f := range want {
			isRead[f] = true
		}
		fields := md.Fields()
		for j := range fields.Len() {
			name := string(fields.Get(j).Name())
			if !isRead[name] {
				t.Errorf("%s.%s is ON THE WIRE and NO HANDLER READS IT: a caller "+
					"that sets it gets a success and loses the value. Either read it "+
					"in serveRecord and add it to this test's list, or `reserved` the "+
					"field number the way ProgressStepRequest reserves 4",
					md.Name(), name)
			}
		}
	}
	if seen != len(read) {
		t.Fatalf("checked %d served request messages, expected %d - the "+
			"descriptor walk found fewer messages than this test names, so it "+
			"proved nothing about the ones it missed", seen, len(read))
	}
}

// TestEveryBriefSectionReportsItsOwnState is the response-side half of the
// guard above, and it is the one that would have caught the cut.
//
// ⛔ THE FAILURE IT EXISTS FOR ALREADY HAPPENED TWICE. `project.brief` is
// specified with ELEVEN sections and shipped with four; separately,
// `must_read` and `must_read_cleared` have been on the response since the wire
// landed with nothing writing either, so a caller reading an empty set would
// conclude the project requires nothing. Boris ruled all eleven in on
// 2026-09-16 - "Cover all of them" - and the mechanism that makes that true is
// SectionState, not a longer message.
//
// THE REQUEST-SIDE GUARD CANNOT SEE ANY OF THIS. It walks request messages
// against the fields serveRecord READS; a response field nothing WRITES is the
// same defect pointed the other way, and it needs its own walk. Raised by the
// backend-record seat, which found `must_read` independently and correctly
// refused to fix a file it does not own.
//
// ⛔ REWRITTEN FOR B64, AND IT NOW TESTS THE REAL PATH. It used to iterate a
// hand-kept briefSections() in this package - a list whose own comment said
// every row was "a claim that expires", and three of them did. That list is
// gone; the store's ledger marks each section at the code that earns it and the
// daemon maps them. So this walks the MAPPER, which is the only thing left in
// this package that can be wrong about a section.
func TestEveryBriefSectionReportsItsOwnState(t *testing.T) {
	// EVERY section in the enum must be reported EXACTLY ONCE. A section that
	// is simply absent is the original defect: the caller cannot tell it from
	// a section with nothing in it.
	vals := rigv1.BriefSection(0).Descriptor().Values()
	want := map[rigv1.BriefSection]bool{}
	for i := range vals.Len() {
		if n := rigv1.BriefSection(vals.Get(i).Number()); n != 0 {
			want[n] = true
		}
	}
	if len(want) != 13 {
		t.Fatalf("BriefSection carries %d sections and this wire serves 13 - "+
			"section 39's eleven, the governing section B64 added and the "+
			"closed section B68 did. If the specification changed, this "+
			"number moves with it deliberately", len(want))
	}

	// ⛔ THE STORE'S TWELVE, TRANSCRIBED, AS A SECOND INSTRUMENT. Feeding this
	// mapper a list derived from the mapper's own switch would agree with it
	// whatever it said - the cross-check-with-the-same-blind-spot this seam has
	// already paid for. internal/record does not export its section list, and
	// writing the names out here is the point rather than a workaround: a
	// section added on the store side reaches this wire only when a person adds
	// an arm, and this is where they find out.
	from := []record.SectionStatus{
		{Section: record.SectionOpen, State: record.SectionComputed},
		{Section: record.SectionNextUp, State: record.SectionComputed},
		{Section: record.SectionNotes, State: record.SectionComputed},
		{Section: record.SectionBlocked, State: record.SectionComputed},
		{Section: record.SectionDrift, State: record.SectionNotComputed, Reason: "r"},
		{Section: record.SectionMustRead, State: record.SectionNotComputed, Reason: "r"},
		{Section: record.SectionProjectionBehind, State: record.SectionNotComputed, Reason: "r"},
		{Section: record.SectionPending, State: record.SectionNotComputed, Reason: "r"},
		{Section: record.SectionLocalOnly, State: record.SectionNotComputed, Reason: "r"},
		{Section: record.SectionFeatures, State: record.SectionComputed},
		{Section: record.SectionCaseNotes, State: record.SectionNotComputed, Reason: "r"},
		{Section: record.SectionGoverning, State: record.SectionComputed},
		{Section: record.SectionClosed, State: record.SectionComputed},
	}
	got, err := sectionStatuses(from)
	if err != nil {
		t.Fatalf("mapping the store's sections onto the wire: %v", err)
	}

	// ⛔ AND THE MAPPER MUST REFUSE WHAT IT CANNOT NAME, watched going red here
	// rather than assumed from reading the default arm. A section travelling as
	// UNSPECIFIED is section 21's rule broken in the quietest possible way.
	if _, err := sectionStatuses([]record.SectionStatus{
		{Section: record.Section("invented"), State: record.SectionComputed},
	}); err == nil {
		t.Error("the mapper accepted a section this wire has no member for and " +
			"would have served it as UNSPECIFIED, which reads as an unset field")
	}

	seen := map[rigv1.BriefSection]int{}
	for _, st := range got {
		seen[st.GetSection()]++

		// ⛔ A REASON IS MANDATORY WHENEVER A SECTION CANNOT ANSWER. Without it
		// NOT_COMPUTED is just a quieter absence: the caller learns the section
		// is unavailable and not which input it is waiting on.
		if st.GetState() != rigv1.SectionState_SECTION_STATE_COMPUTED &&
			strings.TrimSpace(st.GetReason()) == "" {
			t.Errorf("%v is %v with NO REASON: the caller is told a section is "+
				"unavailable and not what it waits on, which is an absence with "+
				"extra steps", st.GetSection(), st.GetState())
		}
		if st.GetState() == rigv1.SectionState_SECTION_STATE_UNSPECIFIED {
			t.Errorf("%v reports UNSPECIFIED, so an unset field has decoded as a "+
				"decision - section 21's rule, and the reason every enum here has "+
				"an explicit zero", st.GetSection())
		}
	}
	for s := range want {
		switch seen[s] {
		case 1:
		case 0:
			t.Errorf("%v is in the enum and the mapper does NOT report it. "+
				"A missing section reads as covered, which is how seven of eleven "+
				"were shipped absent", s)
		default:
			t.Errorf("%v is reported %d times; a caller reading the first gets a "+
				"different answer from one reading the last", s, seen[s])
		}
	}
}

// TestTheBriefNeverAnswersWithoutItsSectionStates pins the one property the
// whole ruling rests on, over the real wire.
//
// An all-zero BriefHealth is the correct encoding of a healthy project AND of
// a projection that does not exist yet. ⛔ SO A BRIEF THAT ARRIVES WITHOUT
// `sections` IS INDISTINGUISHABLE FROM ONE REPORTING PERFECT HEALTH, which is
// section 39's recorded injury verbatim: a brief that "reported a
// healthy-looking project that was not backed up".
func TestTheBriefNeverAnswersWithoutItsSectionStates(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "brief-sections")

	var resp rigv1.ProjectBriefResponse
	if err := c.Call(recordCtx(t), "rig.project.brief",
		&rigv1.ProjectBriefRequest{Project: "rig"}, &resp); err != nil {
		t.Fatalf("rig.project.brief: %v", err)
	}
	if len(resp.GetSections()) != 13 {
		t.Fatalf("the brief answered with %d section states, want 13: an empty "+
			"health block is then indistinguishable from a healthy project",
			len(resp.GetSections()))
	}
	if resp.GetHealth() == nil {
		t.Error("health is nil on the wire, so sections 7-9 are absent rather " +
			"than reported - the shape section 39 added rows 7-9 to prevent")
	}
	// The must-read gate is the one with a recorded history of reading as
	// "nothing is required" while being unbuilt.
	for _, st := range resp.GetSections() {
		if st.GetSection() != rigv1.BriefSection_BRIEF_SECTION_MUST_READ {
			continue
		}
		if st.GetState() == rigv1.SectionState_SECTION_STATE_COMPUTED &&
			len(resp.GetMustRead()) == 0 {
			t.Error("must_read is reported COMPUTED and is empty; if the mark is " +
				"now built this is right, and if it is not, an empty set is being " +
				"served as though it meant the project requires nothing")
		}
	}
}

// TestTheBriefCarriesTheNotesAndFeaturesTheDerivationComputes is the guard for
// the half of this seam that has now failed in BOTH directions.
//
// ⛔ THE DEFECT IT CATCHES: internal/record grew Notes, Features and Stages, and
// serveProjectBrief mapped none of the three. `project.brief` answered, it was
// well-formed, and it was short - while briefSections() went on reporting NOTES
// and FEATURES as NOT_COMPUTED with a reason blaming a derivation that had
// already landed. A caller could not tell a project with no notes from a daemon
// that never looked.
//
// ⛔ AND IT IS THE MIRROR OF THE `must_read` DEFECT, WHICH IS WHY BOTH HALVES
// ARE ASSERTED HERE RATHER THAN IN TWO TESTS. That one was a WIRE FIELD NOTHING
// WROTE - read it, always empty. This is a DAEMON THAT DID NOT READ THREE
// PACKAGE FIELDS THAT EXIST. Same seam, opposite direction, and ownership in
// this repository is by FILE - so the join between two files belongs to nobody
// and only a reader holding both sides at once finds either.
//
// THE SECTION STATE IS ASSERTED BESIDE THE PAYLOAD DELIBERATELY. A section that
// carries rows while reporting NOT_COMPUTED, or reports COMPUTED while carrying
// nothing it was given, is the same lie told from either end.
func TestTheBriefCarriesTheNotesAndFeaturesTheDerivationComputes(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	put := func(what string, req *rigv1.RecordPutRequest) string {
		t.Helper()
		var resp rigv1.RecordPutResponse
		if err := c.Call(ctx, "rig.record.put", req, &resp); err != nil {
			t.Fatalf("rig.record.put(%s): %v", what, err)
		}
		return resp.GetRecord().GetId()
	}

	put("project", &rigv1.RecordPutRequest{
		Id: "rig", Kind: "project", Project: "rig", Body: "rig itself",
	})

	// Section 3: a note, attached to the project by a `part-of` edge. The
	// derivation scopes this list on the edge's DESTINATION, so the link is not
	// decoration - without it the note is not in the section at all.
	note := put("note", &rigv1.RecordPutRequest{
		Kind: "note", Project: "rig", Body: "the close path panics on a real WM_DELETE_WINDOW",
		Fields: map[string]string{"priority": "high"},
	})
	if err := c.Call(ctx, "rig.record.link", &rigv1.RecordLinkRequest{
		Src: note, Type: "part-of", Dst: "rig",
	}, &rigv1.RecordLinkResponse{}); err != nil {
		t.Fatalf("rig.record.link(note -> project): %v", err)
	}

	// Section 10: features, and the STAGE is what the section is about. Two
	// stages so the counts cannot pass by carrying a single row.
	put("feature building", &rigv1.RecordPutRequest{
		Kind: "feature", Project: "rig", Body: "the continuity record",
		Fields: map[string]string{"title": "record", "stage": "building"},
	})
	put("feature shipped", &rigv1.RecordPutRequest{
		Kind: "feature", Project: "rig", Body: "the estate verb",
		Fields: map[string]string{"title": "estate", "stage": "shipped"},
	})

	var brief rigv1.ProjectBriefResponse
	if err := c.Call(ctx, "rig.project.brief",
		&rigv1.ProjectBriefRequest{Project: "rig"}, &brief); err != nil {
		t.Fatalf("rig.project.brief: %v", err)
	}

	// ---- section 3 -----------------------------------------------------
	if len(brief.GetNotes()) != 1 {
		t.Fatalf("the brief carries %d notes and the derivation computed 1: "+
			"the daemon is not reading record.Brief.Notes, so the section "+
			"answers short rather than refusing", len(brief.GetNotes()))
	}
	got := brief.GetNotes()[0]
	if got.GetId() != note {
		t.Errorf("the note carries id %q, not %q", got.GetId(), note)
	}
	if got.GetPriority() != "high" {
		t.Errorf("the note's priority is %q, not \"high\" - it is the ordering "+
			"signal section 11 sorts on, so a dropped one is a wrong list later",
			got.GetPriority())
	}
	// ⛔ `about` IS WHAT MAKES THE LIST READABLE. The section is scoped on the
	// link's destination, so a note on the project and a note on a work item
	// arrive in ONE slice and nothing else separates them.
	if got.GetAbout() != "rig" {
		t.Errorf("the note says it is about %q, not \"rig\": without it a caller "+
			"cannot tell a note on the project from a note on an item", got.GetAbout())
	}
	if got.GetProv().GetSeat() != "team-lead" {
		t.Errorf("the note's provenance names %q, not the announcing seat",
			got.GetProv().GetSeat())
	}

	// ---- section 10 ----------------------------------------------------
	// Only `building` features are listed; every stage is COUNTED. Asserting
	// both is what stops a mutation that returns all features from passing.
	if len(brief.GetFeatures()) != 1 {
		t.Fatalf("the brief lists %d features and one is at stage `building`: "+
			"section 39 lists the building ones and counts them all",
			len(brief.GetFeatures()))
	}
	if s := brief.GetFeatures()[0].GetStage(); s != "building" {
		t.Errorf("the listed feature is at stage %q, not \"building\"", s)
	}
	counts := map[string]uint64{}
	for _, sc := range brief.GetFeatureStages() {
		counts[sc.GetStage()] = sc.GetCount()
	}
	if counts["building"] != 1 || counts["shipped"] != 1 {
		t.Errorf("the stage counts are %v and two features were written, one "+
			"per stage: a short count reads as a smaller project", counts)
	}

	// ---- the section states, which must agree with the payload ----------
	state := map[rigv1.BriefSection]*rigv1.BriefSectionStatus{}
	for _, st := range brief.GetSections() {
		state[st.GetSection()] = st
	}
	for _, s := range []rigv1.BriefSection{
		rigv1.BriefSection_BRIEF_SECTION_NOTES,
		rigv1.BriefSection_BRIEF_SECTION_FEATURES,
	} {
		st, ok := state[s]
		if !ok {
			t.Errorf("%v is absent from the section states entirely", s)
			continue
		}
		if st.GetState() != rigv1.SectionState_SECTION_STATE_COMPUTED {
			t.Errorf("%v reports %v with reason %q, and this brief just carried "+
				"its rows: a section that answers while reporting NOT_COMPUTED "+
				"sends a reader to build what already exists",
				s, st.GetState(), st.GetReason())
		}
	}
}

// TestRecordRefsCarriesEveryFieldTheStoreComputes is B57's guard, and the three
// fields it asserts hardest are the three the wire could not carry until the
// change that added this test.
//
// ⛔ THE DEFECT: RecordRefsRequest and Ref were written into the proto BEFORE
// anything dispatched them, so nothing ever compared them to
// internal/record.Ref. The store computes six fields per edge and the wire
// carried three - `kind`, `title` and `via` were absent - and the request could
// not express CrossProject at all. A caller would have received a well-formed
// answer with half of it silently dropped.
//
// ⛔ WHY `kind`, `title` AND `via` ARE ASSERTED AND NOT JUST `src`: without the
// first two a reader needs one record.get PER ROW to know what it is looking
// at, which is section 9's context budget paying for fields already computed;
// without `via` a distance-2 edge says how far and not through what, and the
// relationship the caller asked about is unreadable.
func TestRecordRefsCarriesEveryFieldTheStoreComputes(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	put := func(what string, req *rigv1.RecordPutRequest) string {
		t.Helper()
		var resp rigv1.RecordPutResponse
		if err := c.Call(ctx, "rig.record.put", req, &resp); err != nil {
			t.Fatalf("rig.record.put(%s): %v", what, err)
		}
		return resp.GetRecord().GetId()
	}
	link := func(src, typ, dst string) {
		t.Helper()
		if err := c.Call(ctx, "rig.record.link", &rigv1.RecordLinkRequest{
			Src: src, Type: typ, Dst: dst,
		}, &rigv1.RecordLinkResponse{}); err != nil {
			t.Fatalf("rig.record.link(%s -%s-> %s): %v", src, typ, dst, err)
		}
	}

	put("project", &rigv1.RecordPutRequest{
		Id: "rig", Kind: "project", Project: "rig", Body: "rig itself",
	})
	subject := put("subject", &rigv1.RecordPutRequest{
		Kind: "work-item", Project: "rig", Body: "the CLI seam",
		Fields: map[string]string{"title": "the seam", "status": "active"},
	})
	citing := put("decision", &rigv1.RecordPutRequest{
		Kind: "decision", Project: "rig", Body: "land all nine verbs at once",
		Fields: map[string]string{"title": "nine at once"},
	})
	link(citing, "cites", subject)

	var refs rigv1.RecordRefsResponse
	if err := c.Call(ctx, "rig.record.refs",
		&rigv1.RecordRefsRequest{Id: subject}, &refs); err != nil {
		t.Fatalf("rig.record.refs: %v", err)
	}

	// ⛔ THE DEPTH ANSWERED, WHICH THE CALLER DID NOT ASK FOR. A caller that
	// sent no depth cannot otherwise tell a cheap question from an empty
	// answer, and record.Refs does not report it - the daemon derives it from
	// the package's own exported default, so a zero here means that derivation
	// is gone.
	if refs.GetDepth() == 0 {
		t.Error("the answer does not say which depth it served, so a caller " +
			"who sent no depth cannot tell a bounded walk from an empty one")
	}
	if refs.GetId() != subject {
		t.Errorf("the answer is about %q, not %q", refs.GetId(), subject)
	}
	if len(refs.GetRefs()) != 1 {
		t.Fatalf("refs carries %d edges and exactly one record cites the "+
			"subject: refs answers what points AT a record", len(refs.GetRefs()))
	}

	got := refs.GetRefs()[0]
	for _, tc := range []struct{ field, got, want, why string }{
		{"src", got.GetSrc(), citing, "the citing record's id"},
		{"type", got.GetType(), "cites", "the link type of the edge"},
		{
			"kind", got.GetKind(), "decision",
			"without it a reader cannot tell a decision citing a requirement " +
				"from a work-item implementing one, and pays a record.get per row",
		},
		{
			"title", got.GetTitle(), "nine at once",
			"without it every row needs a second call to be readable at all",
		},
		{
			"via", got.GetVia(), subject,
			"without it a distance says how far and not through what",
		},
	} {
		if tc.got != tc.want {
			t.Errorf("Ref.%s is %q, want %q - %s", tc.field, tc.got, tc.want, tc.why)
		}
	}
	if got.GetDistance() != 1 {
		t.Errorf("a direct citation is at distance %d, not 1", got.GetDistance())
	}

	// ---- cross_project, which had no wire field at all ------------------
	// ⛔ SECTION 39 MAKES SCOPING A PERFORMANCE REQUIREMENT AND SAYS CROSSING
	// IS "ASKED FOR, NEVER ARRIVED AT". A wire that cannot ask serves only the
	// default, so this pair - refused by default, served when asked - is the
	// whole of that ruling and neither half proves it alone.
	put("other project", &rigv1.RecordPutRequest{
		Id: "shelf", Kind: "project", Project: "shelf", Body: "another project",
	})
	foreign := put("foreign citation", &rigv1.RecordPutRequest{
		Kind: "decision", Project: "shelf", Body: "shelf depends on the record",
		Fields: map[string]string{"title": "from shelf"},
	})
	link(foreign, "cites", subject)

	var scoped rigv1.RecordRefsResponse
	if err := c.Call(ctx, "rig.record.refs",
		&rigv1.RecordRefsRequest{Id: subject}, &scoped); err != nil {
		t.Fatalf("rig.record.refs(scoped): %v", err)
	}
	if n := len(scoped.GetRefs()); n != 1 {
		t.Errorf("the default walk returned %d edges and should have stayed "+
			"inside the subject's own project, which is one: crossing is "+
			"asked for, never arrived at", n)
	}

	var crossed rigv1.RecordRefsResponse
	if err := c.Call(ctx, "rig.record.refs",
		&rigv1.RecordRefsRequest{Id: subject, CrossProject: true}, &crossed); err != nil {
		t.Fatalf("rig.record.refs(cross_project): %v", err)
	}
	seen := map[string]bool{}
	for _, r := range crossed.GetRefs() {
		seen[r.GetSrc()] = true
	}
	if !seen[foreign] {
		t.Errorf("cross_project=true did not reach the citation in another "+
			"project: the request field is on the wire but nothing carries it "+
			"to the store, so section 39's opt-in is unreachable (got %d edges)",
			len(crossed.GetRefs()))
	}
	if !seen[citing] {
		t.Error("cross_project=true LOST the same-project citation: opting in " +
			"to crossing widens the walk, it does not move it")
	}
}

// TestTheBriefCarriesItsContainersOwnHeaderFields pins the four header fields
// against the defect that produced them: the derivation read the container
// record, kept its metadata, and this function sent none of it, so the first
// brief rig ever gave of itself said "(not said) (no status)" over "(no title)"
// while every row beneath it was correct.
//
// WHY IT IS A SEPARATE TEST FROM THE CLI'S DESCRIPTOR GUARD, WHICH ALREADY
// EXISTS AND CANNOT COVER THIS. cmd/rig's briefWireFieldsNotRendered proves the
// client RENDERS every field the wire declares. It cannot prove the daemon SETS
// one, because a field the daemon never populates arrives byte-identical to a
// project that genuinely has no title. The two guards are on opposite sides of
// the same join and neither implies the other - which is exactly how this field
// spent one wire revision declared, filled by the package, and mapped by
// nobody.
//
// AND IT IS WRITTEN TO GO RED FIELD BY FIELD. Four distinct assertions naming
// four distinct values, rather than one comparison of a whole struct: deleting
// any single mapping line in briefResponse must fail this test and name the
// field that went missing. A struct-equality assertion would have gone red too
// and would have said only "the header differs".
func TestTheBriefCarriesItsContainersOwnHeaderFields(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	// The container carries all four. `semver` is read rather than suppressed
	// by kind: section 39 rules a case has no semver, and no case record writes
	// one, so reading the field satisfies the rule without a second rule.
	var proj rigv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Id: "rig", Kind: "project", Project: "rig", Body: "rig itself",
		Fields: map[string]string{
			"title":  "rig",
			"status": "active",
			"semver": "0.1.0",
		},
	}, &proj); err != nil {
		t.Fatalf("rig.record.put(project): %v", err)
	}

	var brief rigv1.ProjectBriefResponse
	if err := c.Call(ctx, "rig.project.brief", &rigv1.ProjectBriefRequest{
		Project: "rig",
	}, &brief); err != nil {
		t.Fatalf("rig.project.brief: %v", err)
	}

	for _, tc := range []struct {
		field string
		got   string
		want  string
	}{
		{"kind", brief.GetKind(), "project"},
		{"title", brief.GetTitle(), "rig"},
		{"status", brief.GetStatus(), "active"},
		{"semver", brief.GetSemver(), "0.1.0"},
	} {
		if tc.got != tc.want {
			t.Errorf("the brief's %s came back %q, want %q - the store holds it "+
				"and this response dropped it, which is the defect this test pins",
				tc.field, tc.got, tc.want)
		}
	}
}

// ⛔ B76 CROSSES THE WIRE AS A FACT, AND BOTH ANSWERS ARE ASSERTED.
//
// `record.Brief.ContainerFound` has existed since rig 072aea4 and this
// response dropped it, so cmd/rig re-derived the condition from an empty
// `kind`. That inference was correct only because fields 9-12 happen to be
// served: against any daemon older than rig af7715d every brief arrives with
// an empty kind and every brief reads as a missing container.
//
// ⛔ THE FOUND CASE IS HALF THE TEST AND IS NOT A COURTESY. A handler that
// hard-coded TRISTATE_NO would pass a missing-container assertion on its own,
// and a handler that never set the field would pass nothing while looking
// like it passed - which is the third row.
//
// ⛔ THE THIRD ROW IS WHAT MAKES THE OTHER TWO MEAN ANYTHING: UNSPECIFIED IS
// UNREACHABLE FROM THIS DAEMON. The zero is reserved for a peer that does not
// carry field 22, so if this end ever spends it, a reader loses the ability to
// tell "I did not look" from "I looked and found nothing" - which is the exact
// distinction B76 is about.
func TestTheBriefStatesWhetherTheContainerExists(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	var put rigv1.RecordPutResponse
	if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
		Id: "rig", Kind: "project", Project: "rig", Body: "rig itself",
	}, &put); err != nil {
		t.Fatalf("rig.record.put(project): %v", err)
	}

	for _, tc := range []struct {
		name    string
		project string
		want    rigv1.Tristate
	}{
		{"a container that exists", "rig", rigv1.Tristate_TRISTATE_YES},
		{
			"an id nothing was created under", "zzz-no-such-project-42",
			rigv1.Tristate_TRISTATE_NO,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var brief rigv1.ProjectBriefResponse
			if err := c.Call(ctx, "rig.project.brief",
				&rigv1.ProjectBriefRequest{Project: tc.project}, &brief); err != nil {
				t.Fatalf("rig.project.brief: %v", err)
			}
			got := brief.GetContainerFound()
			if got == rigv1.Tristate_TRISTATE_UNSPECIFIED {
				t.Fatalf("container_found came back UNSPECIFIED for %q. This "+
					"daemon HAS read the container, so the zero is a value it "+
					"must never spend: it is reserved for a peer that does not "+
					"carry field 22, and a reader cannot tell that from a "+
					"container that is genuinely absent", tc.project)
			}
			if got != tc.want {
				t.Errorf("container_found for %q came back %v, want %v",
					tc.project, got, tc.want)
			}
		})
	}
}

// ⛔ AN EMPTY project OR kind ON THE WIRE MEANS EVERY ONE, AND A WRONG ONE
// STILL MEANS NOTHING.
//
// Both halves are asserted because protojson omits an empty string exactly as
// it omits an unserved field: absent and empty are the same bytes. "Empty
// means every" is only safe if a non-empty value that matches nothing answers
// nothing rather than everything, and a test that checked only the empty case
// would pass against a handler that ignored the filters entirely.
//
// WHAT IT UNBLOCKS, stated so the test is not read as symmetry for its own
// sake: nine record verbs are served and not one of them enumerates projects.
// `record.query` with an empty project and kind `project` is that question,
// and the window's project tab has no other way to ask it.
func TestAnEmptyQueryFilterMeansEveryValueOverTheWire(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	for _, r := range []struct{ id, kind, project string }{
		{"rig", "project", "rig"},
		{"standards", "project", "standards"},
		{"rig-req-1", "requirement", "rig"},
		{"std-odd-1", "a-kind-nobody-would-guess", "standards"},
	} {
		if err := c.Call(ctx, "rig.record.put", &rigv1.RecordPutRequest{
			Id: r.id, Kind: r.kind, Project: r.project, Body: "",
			Fields: map[string]string{"title": r.id},
		}, &rigv1.RecordPutResponse{}); err != nil {
			t.Fatalf("rig.record.put(%s): %v", r.id, err)
		}
	}

	query := func(project, kind string) []*rigv1.Record {
		var resp rigv1.RecordQueryResponse
		if err := c.Call(ctx, "rig.record.query", &rigv1.RecordQueryRequest{
			Project: project, Kind: kind,
		}, &resp); err != nil {
			t.Fatalf("rig.record.query(%q, %q): %v", project, kind, err)
		}
		return resp.GetRecords()
	}

	if got := query("", ""); len(got) != 4 {
		t.Errorf("an unfiltered query returned %d records, want all 4: an "+
			"empty filter on the wire means every value, and without it no "+
			"census of the store can be complete", len(got))
	}

	// THE PROJECT ENUMERATION, which is the live need.
	projects := query("", "project")
	if len(projects) != 2 {
		t.Fatalf("asking for kind `project` across every project returned %d, "+
			"want 2. This is the only way anything can find out what projects "+
			"exist", len(projects))
	}
	seen := map[string]bool{}
	for _, p := range projects {
		seen[p.GetId()] = true
	}
	if !seen["rig"] || !seen["standards"] {
		t.Errorf("the project enumeration returned %v, want rig and standards", seen)
	}

	if got := query("rig", ""); len(got) != 2 {
		t.Errorf("every kind in project rig returned %d records, want 2", len(got))
	}

	// ⛔ THE KIND NOBODY WOULD GUESS. `kind` is not a closed set, so a census
	// that enumerates the kinds it knows about misses this record and reports
	// a complete-looking answer. It is reachable only because the kind filter
	// became optional.
	odd := false
	for _, r := range query("", "") {
		if r.GetKind() == "a-kind-nobody-would-guess" {
			odd = true
		}
	}
	if !odd {
		t.Error("a record under an unguessed kind was not reachable without " +
			"naming that kind, which is the defect the optional filter closes")
	}

	// ⛔ THE SECOND MUTATION. A wrong non-empty value must answer nothing.
	if got := query("no-such-project", ""); len(got) != 0 {
		t.Errorf("a project that does not exist returned %d records, want 0. "+
			"A mistyped filter must answer nothing, never everything", len(got))
	}
	if got := query("", "no-such-kind"); len(got) != 0 {
		t.Errorf("a kind that does not exist returned %d records, want 0", len(got))
	}
	if got := query("rig", "a-kind-nobody-would-guess"); len(got) != 0 {
		t.Errorf("a kind that exists only in another project returned %d "+
			"records for rig, want 0: the filters are AND-ed, not OR-ed", len(got))
	}
}

// TestTheBriefCarriesTheGoverningRecordsOverTheWire is B64's wire half, and it
// is the acceptance test stated end to end: a decision written through
// record.put comes back out of project.brief WITHOUT the caller knowing its id.
//
// ⛔ DECISION 6 BINDS EVERY STRING FIELD THIS ADDS - `id`, `kind` and `title` on
// GoverningRecord, and `kind` on KindCount. protojson omits the empty string,
// so absent and unserved are the same bytes and an empty-value mutation asserts
// nothing about liveness. Each field is therefore asserted against a WRONG
// NON-EMPTY VALUE it could not hold by accident, not merely against "not
// empty".
//
// ⛔ AND THE NEGATIVE HALF IS WHY THIS IS A TEST RATHER THAN A DEMONSTRATION. A
// daemon that put every record of the project into `governing` would satisfy
// every positive assertion here. The work-item below is the assertion that
// cannot be passed that way.
func TestTheBriefCarriesTheGoverningRecordsOverTheWire(t *testing.T) {
	sock := upRecordDaemon(t)
	c := seated(t, sock, "team-lead")
	ctx := recordCtx(t)

	// ⛔ IT RETURNS NOTHING, AND THAT IS THE TEST'S WHOLE PREMISE RATHER THAN
	// tidiness. The sibling helpers in this file hand back the new id because
	// their subjects need linking; here, keeping an id would let the test reach
	// a record the way a caller COULD BEFORE B64, and it would then pass
	// against the store as it was. A helper that cannot return an id cannot
	// accidentally be used that way.
	put := func(what string, req *rigv1.RecordPutRequest) {
		t.Helper()
		var resp rigv1.RecordPutResponse
		if err := c.Call(ctx, "rig.record.put", req, &resp); err != nil {
			t.Fatalf("rig.record.put(%s): %v", what, err)
		}
	}

	put("project", &rigv1.RecordPutRequest{
		Id: "rig", Kind: "project", Project: "rig", Body: "rig itself",
	})
	put("decision", &rigv1.RecordPutRequest{
		Kind: "decision", Project: "rig",
		Body:   "the retention ladder governs observability, never records",
		Fields: map[string]string{"title": "the ladder governs observability"},
	})
	put("requirement", &rigv1.RecordPutRequest{
		Kind: "requirement", Project: "rig", Body: "storage does not reset",
		Fields: map[string]string{"title": "storage survives a release"},
	})
	put("artefact", &rigv1.RecordPutRequest{
		Kind: "artefact", Project: "rig", Body: "the attack synthesis",
		Fields: map[string]string{"title": "SYNTHESIS.md"},
	})
	put("work-item", &rigv1.RecordPutRequest{
		Id: "B1", Kind: "work-item", Project: "rig", Body: "an ordinary row",
		Fields: map[string]string{"title": "not governing", "status": "active"},
	})

	var resp rigv1.ProjectBriefResponse
	if err := c.Call(ctx, "rig.project.brief",
		&rigv1.ProjectBriefRequest{Project: "rig"}, &resp); err != nil {
		t.Fatalf("rig.project.brief: %v", err)
	}

	byKind := map[string]*rigv1.GoverningRecord{}
	for _, g := range resp.GetGoverning() {
		byKind[g.GetKind()] = g

		// ⛔ THE ID IS RESOLVED, NOT MERELY CHECKED FOR EMPTINESS, AND THIS
		// ASSERTION EXISTS BECAUSE THE WEAKER ONE LET A MUTATION THROUGH.
		// The first version of this test asserted only `id != ""`, and setting
		// every row's Id to a constant "x" SURVIVED it: the rows were keyed on
		// kind, so nothing ever looked at an id's value. That is generation
		// 11's own finding arriving in the test written to carry its lesson -
		// every assertion was satisfiable by the broken code.
		//
		// Fetching it is the assertion that cannot be satisfied that way, and
		// it is also the property a reader actually needs: an id in the brief
		// is worth having only if it resolves. A wrong id fails here whatever
		// it is, including one copied from a neighbouring row.
		var got rigv1.RecordGetResponse
		if err := c.Call(ctx, "rig.record.get",
			&rigv1.RecordGetRequest{Id: g.GetId()}, &got); err != nil {
			t.Errorf("the brief offers %s id %q and record.get cannot resolve "+
				"it (%v) - an id that does not fetch is worse than no id, "+
				"because a reader will spend a call finding out",
				g.GetKind(), g.GetId(), err)
			continue
		}
		if k := got.GetRecord().GetKind(); k != g.GetKind() {
			t.Errorf("the brief says %q is a %s and the store says it is a %s - "+
				"the row's id and its kind are describing different records",
				g.GetId(), g.GetKind(), k)
		}
	}

	// The positive half, with the wrong-non-empty-value assertion DECISION 6
	// requires: the title is checked against what was stored, not against "".
	for _, want := range []struct{ kind, title string }{
		{"decision", "the ladder governs observability"},
		{"requirement", "storage survives a release"},
		{"artefact", "SYNTHESIS.md"},
	} {
		g, ok := byKind[want.kind]
		if !ok {
			t.Errorf("a %s was written through record.put and the brief does "+
				"not carry it - it is reachable only by an id the caller would "+
				"have to have kept, which is B64", want.kind)
			continue
		}
		if g.GetTitle() != want.title {
			t.Errorf("the %s's title is %q, want %q - an empty title would be "+
				"the same bytes as an unserved field, so this asserts the value "+
				"and not its presence", want.kind, g.GetTitle(), want.title)
		}
	}

	// ⛔ THE NEGATIVE HALF.
	if g, ok := byKind["work-item"]; ok {
		t.Errorf("the brief serves work-item %q in `governing`; sections 1 and "+
			"2 already carry it, and a daemon dumping every record passes every "+
			"assertion above", g.GetId())
	}
	if n := len(resp.GetGoverning()); n != 3 {
		t.Errorf("`governing` carries %d rows, want exactly 3", n)
	}

	counts := map[string]uint64{}
	for _, c := range resp.GetGoverningCounts() {
		if c.GetKind() == "" {
			t.Error("a count arrived with NO kind, so a reader is told a number " +
				"and not what it counts")
		}
		counts[c.GetKind()] = c.GetCount()
	}
	for _, k := range []string{"decision", "requirement", "artefact"} {
		if counts[k] != 1 {
			t.Errorf("governing_counts says %d %s, want 1", counts[k], k)
		}
	}
	if counts["work-item"] != 0 {
		t.Errorf("governing_counts counts work-items (%d)", counts["work-item"])
	}
}
