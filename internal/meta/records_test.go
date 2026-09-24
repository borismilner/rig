package meta_test

import (
	"context"
	"errors"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// holdsRecords is a Records an invoker can also be, so the type assertion under
// test has something real to find.
//
// A NAMED TYPE RATHER THAN AN ANONYMOUS CLOSURE, for Invoker's stated reason:
// a double that can say what it is in a stack trace is one somebody can find
// again.
// B77's three, on the double. ⛔ EACH RETURNS A DISTINGUISHABLE VALUE rather
// than a zero, because the reachability test below reads a value back on every
// tool: a double answering zeros makes "dispatched and wired" and "dispatched
// and silently empty" the same green.
func (h *holdsRecords) Retract(_ context.Context, id, reason string) (meta.RecordRetraction, error) {
	if h.err != nil {
		return meta.RecordRetraction{}, h.err
	}
	return meta.RecordRetraction{ID: id, Reason: reason, Seat: "a-seat"}, nil
}

func (h *holdsRecords) Delete(_ context.Context, id string, dryRun bool) (meta.RecordDeletion, error) {
	if h.err != nil {
		return meta.RecordDeletion{}, h.err
	}
	return meta.RecordDeletion{
		ID: id, Versions: 2, DryRun: dryRun,
		Edges: []meta.RecordEdge{{Src: "B91", Type: "part-of", Dst: id}},
	}, nil
}

func (h *holdsRecords) Replace(_ context.Context, old, replacement, reason string) (meta.RecordReplacement, meta.RecordRetraction, error) {
	if h.err != nil {
		return meta.RecordReplacement{}, meta.RecordRetraction{}, h.err
	}
	rep := meta.RecordReplacement{
		Old: old, New: replacement,
		Moved: []meta.RecordEdge{{Src: "B92", Type: "cites", Dst: old}},
	}
	ret := meta.RecordRetraction{
		ID: old, Reason: reason, ReplacedBy: replacement, Seat: "a-seat",
	}
	return rep, ret, nil
}

type holdsRecords struct {
	putSeen  meta.RecordPut
	rows     []meta.RecordRow
	refs     []meta.RecordRef
	stepSeen meta.ProgressStep
	linked   [3]string
	err      error

	// ⛔ THE READ ARGUMENTS ARE RECORDED, AND UNTIL 2026-09-17 THEY WERE NOT.
	// Query took its parameter and threw it away, so NO TEST IN THIS PACKAGE
	// COULD HAVE GONE RED on a read that dropped what the caller asked for -
	// whatever the real adapter did. Two defects lived behind that for their
	// whole life: `project_brief` could not find any project at all, and
	// `record_query` answered only when BOTH project and kind were supplied.
	// Both were in internal/daemon, and this double is why nothing here
	// noticed. The first surface left rig at section 50 move 8; the rule it
	// paid for is why the argument is still recorded here.
	//
	// A double that discards its arguments is not a simplification, it is a
	// check that cannot fail - the eighth recorded instance of that shape in
	// this project.
	querySeen struct {
		project string
		kind    string
		fields  map[string]string
		calls   int
	}
}

func (h *holdsRecords) Invoke(context.Context, kernel.Principal, string, string, []byte) ([]byte, error) {
	return nil, errors.New("not this test's concern")
}

func (h *holdsRecords) Put(_ context.Context, in meta.RecordPut) (meta.RecordRow, error) {
	h.putSeen = in
	if h.err != nil {
		return meta.RecordRow{}, h.err
	}
	return meta.RecordRow{
		ID: in.ID, Project: in.Project, Kind: in.Kind,
		Version: 1, Body: in.Body, Fields: in.Fields, Seat: "backend-1",
	}, nil
}

func (h *holdsRecords) Get(_ context.Context, id string, version uint64) (meta.RecordRow, error) {
	return meta.RecordRow{ID: id, Version: version, Seat: "backend-1"}, h.err
}

func (h *holdsRecords) Query(_ context.Context, project, kind string, fields map[string]string) ([]meta.RecordRow, error) {
	h.querySeen.project, h.querySeen.kind, h.querySeen.fields = project, kind, fields
	h.querySeen.calls++
	return h.rows, h.err
}

func (h *holdsRecords) History(context.Context, string) ([]meta.RecordRow, error) {
	return h.rows, h.err
}

func (h *holdsRecords) Link(_ context.Context, from, to, kind string) error {
	h.linked = [3]string{from, to, kind}
	return h.err
}

func (h *holdsRecords) Unlink(_ context.Context, from, to, kind string) error {
	h.linked = [3]string{from, to, kind}
	return h.err
}

func (h *holdsRecords) Refs(context.Context, string) ([]meta.RecordRef, error) {
	return h.refs, h.err
}

func (h *holdsRecords) Step(_ context.Context, in meta.ProgressStep) (meta.RecordRow, error) {
	h.stepSeen = in
	return meta.RecordRow{ID: "step-1", Kind: "progress", Seat: "backend-1"}, h.err
}

// TestTheContinuityRecordIsReachableFromTheAgentSurface is the whole point of
// the route, and it is written to go red against the build that shipped before
// it.
//
// ⛔ THE DEFECT IT PINS WAS MEASURED, NOT PREDICTED. Section 09's A0 survey,
// 2026-09-17, `[ran it]` on a live estate: `tools/call` for `record` and for
// `brief` both answered `-32602 unknown tool`, and `query` with subject
// `records` was byte-identical to an unrecognised subject. Seven tools and not
// one reached the record - so an agent with rig's MCP server was STRICTLY LESS
// CAPABLE than the same agent with a filesystem tool and the logbook, which is
// clause A failing on its own terms.
//
// ⛔ IT ASSERTS THE ANSWER IS RIGHT, NEVER MERELY THAT IT IS NOT EMPTY. A
// mutation setting every governing row's id to a constant "x" passed here in
// September because the rows were keyed on kind and nothing ever read an id's
// value. Every case below reads a value back.
func TestTheContinuityRecordIsReachableFromTheAgentSurface(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool meta.Tool
	}{
		{"put", meta.RecordPutTool},
		{"get", meta.RecordGetTool},
		{"query", meta.RecordQueryTool},
		{"history", meta.RecordHistoryTool},
		{"link", meta.RecordLinkTool},
		{"unlink", meta.RecordUnlinkTool},
		{"refs", meta.RecordRefsTool},
		{"step", meta.ProgressStepTool},

		// ⛔ B77's THREE. An agent that can WRITE a record and cannot withdraw
		// one has permanent mistakes, and the store accreting from a seat doing
		// its job correctly is the receipt the row was filed on.
		{"retract", meta.RecordRetractTool},
		{"delete", meta.RecordDeleteTool},
		{"replace", meta.RecordReplaceTool},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := meta.New(estate(t, kernel.CoverageFull), &holdsRecords{})
			got, err := s.Answer(context.Background(), agent(), meta.Request{Tool: tc.tool})
			if err != nil {
				t.Fatalf("%s is not dispatched at all: %v", tc.tool, err)
			}
			if got.Tool != tc.tool {
				t.Errorf("answer came back as %q, not %q", got.Tool, tc.tool)
			}
			if got.Record == nil {
				t.Fatalf("%s dispatched but carried no record payload", tc.tool)
			}
			// ⛔ AND IT MUST NOT REPORT THE RECORD AS UNAVAILABLE WHEN IT IS
			// RIGHT THERE. This is the arm that catches a handler wired to the
			// dispatch but not to the type assertion - which would answer, and
			// answer wrongly, and look fine.
			if contains(got.Unavailable, "this estate's continuity record") {
				t.Errorf("%s says the record is unavailable while holding one", tc.tool)
			}
		})
	}
}

// TestAnUnnamedEstateSaysItHasNoRecordRatherThanAnsweringEmpty is the negative
// arm, and it is the one that could most easily not go red.
//
// ⛔ SECTION 37 GIVES AN UNNAMED ESTATE NO CONTINUITY RECORD, so a daemon
// serving one satisfies Roster and Invoker and not Records. "No rows" and "this
// estate keeps none" are different facts, and a surface that renders them as
// the same bytes is section 5k's cardinal failure - a picture that implies a
// completeness it never had.
func TestAnUnnamedEstateSaysItHasNoRecordRatherThanAnsweringEmpty(t *testing.T) {
	// justInvokes satisfies Invoker and NOT Records, which is exactly the
	// configuration an unnamed estate presents.
	s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: production()})

	got, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.RecordQueryTool, Project: "rig"})
	if err != nil {
		t.Fatalf("record_query refused instead of reporting unavailable: %v", err)
	}
	if !contains(got.Unavailable, "this estate's continuity record") {
		t.Fatalf("an estate with no record answered without saying so; "+
			"Unavailable was %v and Record was %+v", got.Unavailable, got.Record)
	}
	if got.Record != nil {
		t.Errorf("it also invented a payload: %+v", *got.Record)
	}
}

// TestEveryRecordArgumentReachesTheRecords catches the wiring defect that would
// otherwise be invisible: a tool dispatched, answering, and dropping what the
// caller asked for.
//
// ⛔ A HANDLER THAT IGNORES ITS ARGUMENTS PASSES EVERY "did it answer" TEST.
// That is the shape of the `=` to `LIKE` mutation that survived a thorough
// suite here, because every case asked for something that exists.
func TestEveryRecordArgumentReachesTheRecords(t *testing.T) {
	h := &holdsRecords{}
	s := meta.New(estate(t, kernel.CoverageFull), h)

	if _, err := s.Answer(context.Background(), agent(), meta.Request{
		Tool: meta.RecordPutTool, RecordID: "b75", Project: "rig",
		Kind: "decision", Body: "the prose", IfVersion: 7,
		Fields: map[string]string{"title": "a title"},
	}); err != nil {
		t.Fatalf("record_put: %v", err)
	}
	if h.putSeen.ID != "b75" || h.putSeen.Project != "rig" ||
		h.putSeen.Kind != "decision" || h.putSeen.Body != "the prose" ||
		h.putSeen.IfVersion != 7 || h.putSeen.Fields["title"] != "a title" {
		t.Errorf("put lost an argument on the way through: %+v", h.putSeen)
	}

	if _, err := s.Answer(context.Background(), agent(), meta.Request{
		Tool: meta.RecordLinkTool, From: "a", To: "b", LinkKind: "part-of",
	}); err != nil {
		t.Fatalf("record_link: %v", err)
	}
	if h.linked != [3]string{"a", "b", "part-of"} {
		t.Errorf("link lost an argument: %v", h.linked)
	}

	if _, err := s.Answer(context.Background(), agent(), meta.Request{
		Tool: meta.ProgressStepTool, Item: "B75", Project: "rig",
		State: "done", Body: "shipped",
	}); err != nil {
		t.Fatalf("progress_step: %v", err)
	}
	if h.stepSeen.Item != "B75" || h.stepSeen.State != "done" ||
		h.stepSeen.Note != "shipped" || h.stepSeen.Project != "rig" {
		t.Errorf("step lost an argument: %+v", h.stepSeen)
	}

	// ⛔ THE READ, WHICH THIS TEST DID NOT COVER AND WHICH THE DOUBLE COULD
	// NOT HAVE FAILED. record_query is the surface an agent resumes through,
	// and it shipped a defect that reached the live door. It was not a wiring
	// fault here - it was in the adapter - but a double that discarded its
	// arguments is what made internal/meta unable to say anything about it.
	if _, err := s.Answer(context.Background(), agent(), meta.Request{
		Tool: meta.RecordQueryTool, Project: "rig", Kind: "decision",
		Fields: map[string]string{"owner": "read-path"},
	}); err != nil {
		t.Fatalf("record_query: %v", err)
	}
	if h.querySeen.calls != 1 {
		t.Errorf("record_query reached the records %d times, want 1",
			h.querySeen.calls)
	}
	if h.querySeen.project != "rig" || h.querySeen.kind != "decision" ||
		h.querySeen.fields["owner"] != "read-path" {
		t.Errorf("query lost an argument on the way through: %+v", h.querySeen)
	}

	// ⛔ AN EMPTY FILTER IS AN ARGUMENT TOO, AND IT IS THE ONE THAT BROKE.
	// Both defects were an empty string being treated as a value rather than
	// as "every". A route that silently substituted a default here would be
	// invisible to the row above, where nothing is empty.
	if _, err := s.Answer(context.Background(), agent(), meta.Request{
		Tool: meta.RecordQueryTool, Project: "rig",
	}); err != nil {
		t.Fatalf("record_query with no kind: %v", err)
	}
	if h.querySeen.project != "rig" || h.querySeen.kind != "" {
		t.Errorf("an omitted kind did not arrive as an empty one: %+v",
			h.querySeen)
	}

}

// TestEveryDispatchedToolIsNamedInTheRefusal keeps allToolNames honest.
//
// ⛔ IT EXISTS BECAUSE THE LIST IS HAND-KEPT AND SAYS SO. `briefSections()` is
// the same shape and its comment claimed the opposite - that it was derived
// from the store's struct - and two of its rows went stale behind that false
// claim. A comment cannot keep a list true; this can.
//
// The refusal for an unknown tool must name every tool that IS dispatched, so
// an agent that mistypes one is told what exists rather than only that it was
// wrong.
func TestEveryDispatchedToolIsNamedInTheRefusal(t *testing.T) {
	s := meta.New(estate(t, kernel.CoverageFull), &holdsRecords{})

	_, err := s.Answer(context.Background(), agent(), meta.Request{Tool: "no_such_tool"})
	if err == nil {
		t.Fatal("an unknown tool was accepted")
	}
	msg := err.Error()

	for _, tool := range []meta.Tool{
		meta.List, meta.Describe, meta.Invoke, meta.Query,
		meta.Announce, meta.SetActivity, meta.ListAgents,
		meta.RecordPutTool, meta.RecordGetTool, meta.RecordQueryTool,
		meta.RecordHistoryTool, meta.RecordLinkTool, meta.RecordUnlinkTool,
		meta.RecordRefsTool, meta.ProgressStepTool,
	} {
		if !containsSub(msg, string(tool)) {
			t.Errorf("the refusal does not name %q, so an agent that mistyped "+
				"it would never learn it exists; message was %q", tool, msg)
		}
	}
}

func containsSub(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// TestB77sThreeCarryTheirAnswersAndNotOnlyAPayload is the arm the loop above
// promises in its caption and does not actually run.
//
// ⛔ THE CAPTION SAYS "EVERY CASE BELOW READS A VALUE BACK" AND THE LOOP CHECKS
// ONLY THAT `Record` IS NON-NIL. That is the exact shape it warns about one
// paragraph earlier - a mutation setting every id to a constant passed there in
// September - so B77's three get the assertion rather than inheriting the gap.
// Fixing the loop for the other nine is a separate change against a pinned
// surface and is not smuggled in here.
func TestB77sThreeCarryTheirAnswersAndNotOnlyAPayload(t *testing.T) {
	s := meta.New(estate(t, kernel.CoverageFull), &holdsRecords{})
	ctx := context.Background()

	ret, err := s.Answer(ctx, agent(), meta.Request{
		Tool: meta.RecordRetractTool, RecordID: "B90", Reason: "filed twice",
	})
	if err != nil {
		t.Fatal(err)
	}
	switch got := ret.Record.Retraction; {
	case got == nil:
		t.Fatal("record_retract answered with no withdrawal, so an agent " +
			"cannot tell it from a call that did nothing")
	case got.ID != "B90" || got.Reason != "filed twice":
		t.Errorf("the withdrawal is %+v, want the id and reason that were sent", got)
	case got.Seat == "":
		t.Error("the withdrawal carries no seat - a retraction nobody can " +
			"attribute is a fact with no author")
	}

	del, err := s.Answer(ctx, agent(), meta.Request{
		Tool: meta.RecordDeleteTool, RecordID: "B90", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch got := del.Record.Deletion; {
	case got == nil:
		t.Fatal("record_delete answered with no account of what it took, " +
			"which is the whole obligation the verb carries")
	case got.Versions != 2:
		t.Errorf("the version count is %d, want the double's 2", got.Versions)
	case len(got.Edges) != 1:
		t.Errorf("the dropped edges are %+v, want the double's one", got.Edges)
	case !got.DryRun:
		t.Error("dry_run did not survive to the answer, so a caller who " +
			"previewed is told they deleted")
	}

	rep, err := s.Answer(ctx, agent(), meta.Request{
		Tool: meta.RecordReplaceTool, RecordID: "B90", NewID: "B91",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Record.Replacement; got == nil || got.Old != "B90" || got.New != "B91" {
		t.Fatalf("the replacement is %+v, want B90 replaced by B91", got)
	}
	if len(rep.Record.Replacement.Moved) != 1 {
		t.Errorf("the moved edges are %+v, want the double's one",
			rep.Record.Replacement.Moved)
	}
	// ⛔ BOTH HALVES TRAVEL. An agent told the edges moved and not told the
	// loser is withdrawn has been told half of what the verb did.
	if got := rep.Record.Retraction; got == nil || got.ReplacedBy != "B91" {
		t.Errorf("the replacement carried no withdrawal naming the survivor: %+v", got)
	}
}
