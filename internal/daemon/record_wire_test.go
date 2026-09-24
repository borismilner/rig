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

	// ⛔ project.brief IS STILL DIALLED AND IT ANSWERS A REFUSAL. plan/50
	// decision 4: the derivation left for the docket program, section 21
	// forbids the verb vanishing from a shipped wire version, so the arm stays
	// declared and refuses in the caller's terms. THIS ASSERTION IS THE
	// EVIDENCE THE ARM STILL ANSWERS - decision 4 asks for it by name - and it
	// is what separates a moved verb from a deleted one: a deleted verb gets
	// daemon.go's CODE_NOT_FOUND "no such method".
	//
	// ⛔ THE REQUEST IS DELIBERATELY THE WRONG MESSAGE. The arm unmarshals
	// nothing, so a record.get body must produce exactly the same refusal as a
	// brief body would; sending the brief's own request type here would also
	// fail plan/50 acceptance B, whose grep must print nothing outside proto/.
	if err := c.Call(ctx, "rig.project.brief",
		&rigv1.RecordGetRequest{Id: "rig"}, &rigv1.RecordGetResponse{}); err == nil {
		t.Fatal("rig.project.brief answered a brief, so the derivation is still here")
	} else {
		var refusal *client.CallError
		if !errors.As(err, &refusal) {
			t.Fatalf("the refusal arrived unstructured: %T %v", err, err)
		}
		if got := refusal.Status.GetCode(); got != rigv1.Code_CODE_DENIED {
			t.Errorf("the moved verb answered %v; NOT_FOUND is how an undeclared "+
				"verb answers and a caller must be able to tell the two apart", got)
		}
		if !strings.Contains(refusal.Status.GetFix(), "docket") {
			t.Errorf("the fix does not name the program that owns the brief: %q",
				refusal.Status.GetFix())
		}
		if refusal.Status.GetPrecondition() == "" || refusal.Status.GetActual() == "" {
			t.Errorf("the refusal is not structured: precondition %q actual %q",
				refusal.Status.GetPrecondition(), refusal.Status.GetActual())
		}
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

	// The READ half. It was `project.brief` until plan/50 move 8 retired that
	// arm; `record.query` is the same probe against the same store, and it is a
	// verb rig still carries.
	var got rigv1.RecordQueryResponse
	if err := c.Call(recordCtx(t), "rig.record.query",
		&rigv1.RecordQueryRequest{Project: "rig"}, &got); err != nil {
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
		"RecordQueryRequest":   {"project", "kind", "field", "value", "limit", "after"},
		"RecordHistoryRequest": {"id"},
		"RecordLinkRequest":    {"src", "type", "dst"},
		"RecordUnlinkRequest":  {"src", "type", "dst"},
		"ProgressStepRequest":  {"item", "state", "note"},

		// ⛔ THE BRIEF'S REQUEST IS NOT NAMED HERE AND ITS ABSENCE IS THE
		// POINT. plan/50 move 8 left the arm declared and answering a refusal
		// that unmarshals nothing, so every field on that message is now a
		// field no handler reads - by ruling, not by defect. This walk only
		// checks the messages it names, and naming it would assert the
		// opposite of what decision 4 decided. The seven brief messages stay
		// in wire.proto until the next major.
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
