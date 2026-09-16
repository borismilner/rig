package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/instance"
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

// TestAWriteFromAnUnannouncedConnectionIsRefusedByName is the provenance rule
// with its teeth in.
//
// The refusal has to name `announce`, because the caller has NO field it could
// set instead - that is the whole design - and a refusal reporting a missing
// session would send it looking for one.
func TestAWriteFromAnUnannouncedConnectionIsRefusedByName(t *testing.T) {
	sock := upRecordDaemon(t)
	c := dial(t, sock) // deliberately NOT seated

	err := c.Call(recordCtx(t), "rig.record.put", &rigv1.RecordPutRequest{
		Kind: "note", Project: "rig", Body: "who wrote this?",
	}, &rigv1.RecordPutResponse{})
	if err == nil {
		t.Fatal("an unattributable record was accepted")
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

// TestAnUnnamedEstateRefusesTheRecordVerbsAndSaysWhy.
//
// nil is a correct state, not a hole - and the refusal has to separate "this
// estate keeps no record" from "the store broke", because the second would
// send a reader hunting a fault that does not exist.
func TestAnUnnamedEstateRefusesTheRecordVerbsAndSaysWhy(t *testing.T) {
	sock, _ := upDaemon(t, nil) // unnamed
	c := dial(t, sock)

	err := c.Call(recordCtx(t), "rig.project.brief",
		&rigv1.ProjectBriefRequest{Project: "rig"}, &rigv1.ProjectBriefResponse{})
	if err == nil {
		t.Fatal("an unnamed estate served a record verb")
	}
	if !strings.Contains(err.Error(), "unnamed") {
		t.Errorf("the refusal does not name the cause: %v", err)
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
		"RecordQueryRequest":   {"project", "kind"},
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
