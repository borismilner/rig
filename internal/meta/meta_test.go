package meta_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// ranIt records what it was asked to run, so a test can prove the meta layer
// routed rather than answered.
type ranIt struct {
	program, command string
	args             []byte
	result           []byte
	err              error

	// who is what the floor would authorise on. It is recorded because the
	// interface once did not carry it at all, and a test that does not look
	// at it cannot tell a principal that arrived from one that was dropped.
	who kernel.Principal
}

func (r *ranIt) Invoke(_ context.Context, who kernel.Principal,
	program, command string, args []byte,
) ([]byte, error) {
	r.who = who
	r.program, r.command, r.args = program, command, args
	return r.result, r.err
}

func declaring(id string, coverage kernel.Coverage) kernel.Declaration {
	return kernel.Declaration{
		Identity:     kernel.Identity{ID: id, Name: id, Version: "1.0"},
		Coverage:     coverage,
		CoverageNote: "search is adopted; storage is not",
		SemanticsGen: 1,
		Preamble:     "Read this before touching " + id + ".",
		Commands: []kernel.Command{{
			ID: "reindex", Title: "Reindex",
			Effects: kernel.EffectsWritesFiles, Idempotent: kernel.Yes,
			Sensitive: []string{}, Interactive: kernel.No, Streams: kernel.No,
			NeedsDisplay: kernel.No, Duration: kernel.DurationSeconds,
			Confirms: kernel.No, Shape: kernel.ShapeUnary,
			Summary: "Rebuild the index", Description: "Rebuilds it.",
			Returns:  "A count.",
			Examples: []string{"rig " + id + " reindex --since 7d"},
		}},
	}
}

func estate(t *testing.T, coverage kernel.Coverage) *kernel.Kernel {
	t.Helper()
	k := kernel.New()
	p := kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "shelf", SessionID: "s-shelf", PID: 1,
	}
	if _, err := k.Register(p, declaring("shelf", coverage)); err != nil {
		t.Fatalf("register: %v", err)
	}
	return k
}

func agent() kernel.Principal {
	return kernel.Principal{
		UID: 1000, Kind: kernel.KindAgent,
		ClientID: "an-agent", SessionID: "s-agent", PID: 2, Introspect: true,
	}
}

// TestSliceThreeDemo is M2 slice 3's demo with the transport taken out:
//
//	"An agent runs a real command in a real program ... having written
//	 nothing - and is told, in the same answer, that its picture of that
//	 program is partial."
//
// IN THE SAME ANSWER is the assertion. Not in a second call, not in a
// resource it might read, not in a log line. A surface that answers a
// question without saying what it does not know is implying completeness,
// which section 5k forbids.
func TestSliceThreeDemo(t *testing.T) {
	k := estate(t, kernel.CoveragePartial)
	ran := &ranIt{result: []byte(`{"indexed":1204}`)}
	s := meta.New(k, ran)

	// The agent has written nothing: it discovers the command and calls it
	// through the same four tools.
	listed, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.List, Depth: kernel.DepthCommands})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed.Estate) != 1 || len(listed.Estate[0].Commands) != 1 {
		t.Fatalf("list did not find the command: %+v", listed.Estate)
	}

	got, err := s.Answer(context.Background(), agent(), meta.Request{
		Tool:    meta.Invoke,
		Program: "shelf",
		Command: "reindex",
		Args:    []byte(`{"since":"7d"}`),
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	// It really ran, and it ran the thing that was asked for.
	if ran.program != "shelf" || ran.command != "reindex" {
		t.Fatalf("the meta layer answered instead of routing: %s.%s",
			ran.program, ran.command)
	}
	if string(ran.args) != `{"since":"7d"}` {
		t.Fatalf("the arguments were changed on the way through: %s", ran.args)
	}
	if string(got.Result) != `{"indexed":1204}` {
		t.Fatalf("the program's result was not returned: %s", got.Result)
	}

	// AND IN THE SAME ANSWER.
	if len(got.Partial) != 1 {
		t.Fatalf("the answer that ran the command did not say the picture is "+
			"partial: %+v", got.Partial)
	}
	if got.Partial[0].Program != "shelf" ||
		got.Partial[0].Coverage != kernel.CoveragePartial {
		t.Errorf("the warning names the wrong thing: %+v", got.Partial[0])
	}
	if got.Partial[0].Note == "" {
		t.Error("the warning says a picture is partial without saying which part")
	}
}

// TestFullCoverageSaysNothing is the other half of Partial being useful. An
// empty Partial has to mean "nothing here is incomplete" rather than "nobody
// looked", or an agent cannot act on either.
func TestFullCoverageSaysNothing(t *testing.T) {
	s := meta.New(estate(t, kernel.CoverageFull), &ranIt{result: []byte(`{}`)})
	for _, r := range []meta.Request{
		{Tool: meta.List, Depth: kernel.DepthCommands},
		{Tool: meta.Describe, Program: "shelf"},
		{Tool: meta.Invoke, Program: "shelf", Command: "reindex"},
		{Tool: meta.Query},
	} {
		got, err := s.Answer(context.Background(), agent(), r)
		if err != nil {
			t.Fatalf("%s: %v", r.Tool, err)
		}
		if len(got.Partial) != 0 {
			t.Errorf("%s warned about a program with full coverage: %+v",
				r.Tool, got.Partial)
		}
	}
}

// TestEveryToolCarriesTheWarning is the reason there is ONE Answer type.
//
// Four result types would let one tool quietly stop carrying Partial, and the
// one that stopped would be whichever was added last. Section 5k's rule is
// about every surface, so it is asserted over every tool rather than over the
// one the demo happens to use.
func TestEveryToolCarriesTheWarning(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), &ranIt{result: []byte(`{}`)})
	for _, r := range []meta.Request{
		{Tool: meta.List, Depth: kernel.DepthPrograms},
		{Tool: meta.List, Depth: kernel.DepthFull},
		{Tool: meta.Describe, Program: "shelf"},
		{Tool: meta.Describe, Program: "shelf", Command: "reindex"},
		{Tool: meta.Invoke, Program: "shelf", Command: "reindex"},
		{Tool: meta.Query},
		{Tool: meta.Query, Subject: "something nobody has built"},
	} {
		name := string(r.Tool)
		if r.Command != "" {
			name += "/command"
		}
		if r.Subject != "" {
			name += "/subject"
		}
		t.Run(name, func(t *testing.T) {
			got, err := s.Answer(context.Background(), agent(), r)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if len(got.Partial) == 0 {
				t.Error("this tool answered without saying the picture is partial")
			}
			if got.Tool != r.Tool {
				t.Errorf("the answer says it came from %s", got.Tool)
			}
		})
	}
}

// TestQuerySaysWhatItCannotRead is section 5k's coverage principle applied to
// rig itself.
//
// query reaches logs, traces, the call log, config provenance, schedule
// history and the audit log, and at M2 every one of those lands at M4 or
// later. The failure this prevents is section 5h's recorded one: a tool
// shipping as a stub with an invented data source. It ships scoped, and it
// says what it does not cover.
func TestQuerySaysWhatItCannotRead(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), nil)
	got, err := s.Answer(context.Background(), agent(), meta.Request{Tool: meta.Query})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for _, want := range []string{
		"logs", "traces", "the call log", "config provenance",
		"schedule history", "the audit log",
	} {
		if !contains(got.Unavailable, want) {
			t.Errorf("query does not admit it cannot read %q: %v",
				want, got.Unavailable)
		}
	}
	// And it answers with what it DOES have, rather than refusing.
	if len(got.Estate) == 0 {
		t.Error("query refused instead of answering with the registry")
	}
}

// TestQueryDoesNotRefuseASubjectItCannotReach: a refusal would tell an agent
// the subject does not exist. Saying where to look next is the difference.
func TestQueryDoesNotRefuseASubjectItCannotReach(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), nil)
	got, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.Query, Subject: "why did the nightly reindex fail"})
	if err != nil {
		t.Fatalf("query refused a subject it cannot reach: %v", err)
	}
	if len(got.Unavailable) == 0 {
		t.Error("query answered a question it cannot answer and said nothing")
	}
}

// TestTheScopeFilterIsTheKernelsOwn: the meta layer must not be a second
// filter. A surface that re-derives who may see what is how the 2026-09-10
// sweep found a grant reaching a caller kind nobody was thinking about.
func TestTheScopeFilterIsTheKernelsOwn(t *testing.T) {
	k := estate(t, kernel.CoveragePartial)
	other := kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "grabbit", SessionID: "s-grabbit", PID: 3,
		Scoped: true, Scopes: []string{"grabbit"},
	}
	s := meta.New(k, &ranIt{result: []byte(`{}`)})

	got, err := s.Answer(context.Background(), other,
		meta.Request{Tool: meta.List, Depth: kernel.DepthCommands})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got.Estate) != 0 {
		t.Fatalf("a scoped program saw %v", got.Estate)
	}

	// describe and invoke report a program it may not see as MISSING, not as
	// refused: "you may not see shelf" tells the caller that shelf exists.
	for _, r := range []meta.Request{
		{Tool: meta.Describe, Program: "shelf"},
		{Tool: meta.Invoke, Program: "shelf", Command: "reindex"},
	} {
		_, err := s.Answer(context.Background(), other, r)
		if err == nil {
			t.Fatalf("%s reached a program outside the caller's scope", r.Tool)
		}
		if !strings.Contains(err.Error(), "no program") &&
			!strings.Contains(err.Error(), "not visible") {
			t.Errorf("%s leaked that shelf exists: %v", r.Tool, err)
		}
	}
}

// TestInvokeRefusesWhenNothingCanRunIt: a surface that can read the estate
// but not call into it is a real configuration, and it should say so rather
// than panic.
func TestInvokeRefusesWhenNothingCanRunIt(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), nil)
	_, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.Invoke, Program: "shelf", Command: "reindex"})
	if err == nil {
		t.Fatal("invoke succeeded with no invoker")
	}
	if !strings.Contains(err.Error(), "no invoker") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// TestThereAreFourTools, and a fifth name is refused rather than guessed at.
func TestThereAreFourTools(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), &ranIt{})
	_, err := s.Answer(context.Background(), agent(), meta.Request{Tool: "explain"})
	if !errors.Is(err, meta.ErrNoSuchTool) {
		t.Fatalf("a fifth tool was accepted: %v", err)
	}
	for _, name := range []string{"list", "describe", "invoke", "query"} {
		if _, err := s.Answer(context.Background(), agent(), meta.Request{
			Tool: meta.Tool(name), Program: "shelf", Command: "reindex",
			Depth: kernel.DepthPrograms,
		}); errors.Is(err, meta.ErrNoSuchTool) {
			t.Errorf("%q is not one of the four", name)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestTheRefusalsSayWhatIsMissing covers the paths the demo never walks. Each
// is a real caller mistake, and a refusal that does not name what was wrong
// sends an agent round the loop again with the same request.
func TestTheRefusalsSayWhatIsMissing(t *testing.T) {
	s := meta.New(estate(t, kernel.CoveragePartial), &ranIt{result: []byte(`{}`)})

	for _, c := range []struct {
		name string
		req  meta.Request
		want string
	}{
		{"describe with no program", meta.Request{Tool: meta.Describe}, "needs a program"},
		{"invoke with no program", meta.Request{Tool: meta.Invoke, Command: "reindex"}, "needs a program"},
		{"invoke with no command", meta.Request{Tool: meta.Invoke, Program: "shelf"}, "needs a program and a command"},
		{"describe an unknown program", meta.Request{Tool: meta.Describe, Program: "nope"}, "no program"},
		{"describe an unknown command", meta.Request{Tool: meta.Describe, Program: "shelf", Command: "nope"}, "declares no command"},
		{"list at no depth", meta.Request{Tool: meta.List}, "no depth"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := s.Answer(context.Background(), agent(), c.req)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the refusal does not say what is wrong:\n got %v\nwant %q",
					err, c.want)
			}
		})
	}
}

// TestUnspecifiedCoverageIsReportedAsIncomplete: registration refuses it, so
// seeing one means something bypassed registration. Treating it as complete
// would be the one case where silence is a lie rather than an absence.
func TestUnspecifiedCoverageIsReportedAsIncomplete(t *testing.T) {
	got := meta.PartialForTest(kernel.Program{
		Identity: kernel.Identity{ID: "ghost"},
		Coverage: kernel.CoverageUnspecified,
	})
	if len(got) != 1 {
		t.Fatalf("a program with unspecified coverage was treated as complete: %+v", got)
	}
	if got[0].Coverage != kernel.CoverageUnspecified {
		t.Errorf("the warning renames the coverage: %v", got[0].Coverage)
	}
}
