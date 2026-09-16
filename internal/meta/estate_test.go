package meta_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// knowsWhere is an invoker that can also say which estate it is, which is what
// the daemon is. The split matters: meta.New's invoker may be nil, so the two
// halves are separate interfaces and a surface with only the first must still
// answer query.
type knowsWhere struct {
	ranIt
	where meta.Estate
}

func (k *knowsWhere) EstateIdentity() meta.Estate { return k.where }

func production() meta.Estate {
	return meta.Estate{
		Name: "production", Role: "production",
		DaemonVersion: "2.0.0", Wire: "1", SemanticsGen: 1,
	}
}

func development() meta.Estate {
	return meta.Estate{
		Name: "development", Role: "development",
		DaemonVersion: "2.0.0", Wire: "1", SemanticsGen: 1,
	}
}

// TestAnAgentCanAskWhichEstateThisIs is section 37 row 0's clause 4 reduced to
// the thing it was blocked on.
//
// FOUND BY DEMONSTRATION, 2026-09-16, and the shape of the defect is what
// makes it worth a test rather than a line of the changelog. `rig.estate` was
// SPECIFIED (section 37 precondition 1), BUILT, and reachable from a terminal.
// The one caller that could not reach it was the agent - which section 37
// calls the motivating caller - because rig's declaration is held BESIDE the
// program map on purpose and `invoke` reaches the map. So the capability
// existed and its audience did not have it: an inversion, not a gap, and no
// count of shipped features would have shown it.
//
// It is asked through `query` rather than through a fifth tool or a second
// resource. query is section 9's tool for asking rig about rig, the four are
// four, and a second MCP resource was refused separately (B16: the SDK stamps
// every resource read publicly cacheable and a handler cannot opt out, which
// is the worst possible property for "which estate am I in").
func TestAnAgentCanAskWhichEstateThisIs(t *testing.T) {
	s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: production()})

	got, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.Query, Subject: meta.SubjectEstate})
	if err != nil {
		t.Fatalf("query subject=%s: %v", meta.SubjectEstate, err)
	}
	if got.Identity == nil {
		t.Fatalf("the agent surface still cannot say which estate this is; "+
			"query answered %+v", got)
	}
	if got.Identity.Name != "production" || got.Identity.Role != "production" {
		t.Errorf("wrong estate: %+v", *got.Identity)
	}

	// AND IT HAS TO REACH THE AGENT, NOT MERELY THE STRUCT. An agent reads
	// the rendered object; a field that exists in Go and not in the JSON is
	// the same defect one layer down.
	b, err := meta.MarshalAnswer(got)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatalf("rig rendered something that is not an object: %v", err)
	}
	id, ok := obj["estateIdentity"].(map[string]any)
	if !ok {
		t.Fatalf("the rendered answer carries no estate identity: %s", b)
	}
	if id["name"] != "production" || id["role"] != "production" {
		t.Errorf("the rendered estate identity is wrong: %v", id)
	}
}

// TestTwoEstatesWithTheSameProgramsAnswerDifferently is why the capability
// map's version could not have carried this, and it is the half that is easy
// to get wrong by reasoning.
//
// The version is a DIGEST OF CONTENT, deliberately - "comparable between two
// daemons", so it survives a restart and can be compared across boxes. The
// consequence is that two estates holding the same programs produce the SAME
// version. An agent trying to tell production from development by comparing
// digests gets a match in exactly the case that matters: two estates running
// the same software, which is what two estates of one project ARE.
func TestTwoEstatesWithTheSameProgramsAnswerDifferently(t *testing.T) {
	ask := func(e meta.Estate) meta.Answer {
		t.Helper()
		s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: e})
		got, err := s.Answer(context.Background(), agent(),
			meta.Request{Tool: meta.Query, Subject: meta.SubjectEstate})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		return got
	}
	prod, dev := ask(production()), ask(development())

	if prod.Identity == nil || dev.Identity == nil {
		t.Fatal("one of the two estates would not say which it is")
	}
	if prod.Identity.Name == dev.Identity.Name {
		t.Errorf("both estates answered %q", prod.Identity.Name)
	}

	// The negative control, and it is the point of the test rather than a
	// decoration: the thing an agent WOULD have compared is identical.
	list := func(e meta.Estate) string {
		t.Helper()
		s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: e})
		got, err := s.Answer(context.Background(), agent(),
			meta.Request{Tool: meta.List, Depth: kernel.DepthPrograms})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		return got.Version
	}
	if list(production()) != list(development()) {
		t.Skip("the two estates no longer hold identical registrations, so " +
			"this control proves nothing; rebuild it before trusting it")
	}
	t.Logf("same capability version in both estates, different identity: %s",
		list(production()))
}

// TestTheEstateIdentityIsUnavailableRatherThanInvented: a surface with no
// daemon under it says it cannot answer instead of making one up.
//
// Section 5h's recorded failure is a tool shipping as a stub with an invented
// data source, and section 5k's rule is that no surface may imply
// completeness. Unavailable is the mechanism for both, and this is the case it
// was built for - "a surface that can read the estate but not call into it is
// a real configuration, and it should say so".
func TestTheEstateIdentityIsUnavailableRatherThanInvented(t *testing.T) {
	for name, s := range map[string]*meta.Server{
		"no invoker at all":          meta.New(estate(t, kernel.CoverageFull), nil),
		"an invoker that cannot say": meta.New(estate(t, kernel.CoverageFull), &ranIt{}),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := s.Answer(context.Background(), agent(),
				meta.Request{Tool: meta.Query, Subject: meta.SubjectEstate})
			if err != nil {
				t.Fatalf("query refused instead of admitting: %v", err)
			}
			if got.Identity != nil {
				t.Fatalf("rig invented an estate identity: %+v", *got.Identity)
			}
			if !contains(got.Unavailable, "the estate's own identity") {
				t.Errorf("it could not answer and did not say so: %v", got.Unavailable)
			}
		})
	}
}

// TestAskingForRigIsRefusedWithTheRIGHTCAUSE pins the refusal's reasoning, not
// merely that there is one.
//
// A REFUSAL MUST NAME THE CAUSE IT ACTUALLY HAS. Asking to describe or invoke
// `rig` used to be answered `no program "rig" is visible to this caller`,
// which reads as a permissions problem: it sends an agent looking for a grant
// that does not exist, cannot be granted, and would be the wrong fix if it
// could. rig is not hidden from this caller; it is not a program.
//
// THE OTHER HALF IS THAT IT IS STILL REFUSED, and this test is the meta-layer
// companion to the daemon's TestRigIsNotAnInvokeTargetOnAnySurface. rig
// declares `down` EffectsDestructive. Routing rig through the program
// projection to "fix the asymmetry" would hand every MCP agent rig.down in the
// same change, with no line of the diff mentioning it. The estate question is
// answered by query instead, which adds no invoke target at all.
func TestAskingForRigIsRefusedWithTheRIGHTCAUSE(t *testing.T) {
	s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: production()})

	for _, r := range []meta.Request{
		{Tool: meta.Describe, Program: "rig"},
		{Tool: meta.Describe, Program: "rig", Command: "estate"},
		{Tool: meta.Invoke, Program: "rig", Command: "down"},
	} {
		_, err := s.Answer(context.Background(), agent(), r)
		if err == nil {
			t.Fatalf("%s reached rig through the program projection, so rig's "+
				"own commands are now an invoke target - including rig.down, "+
				"which is declared destructive", r.Tool)
		}
		if strings.Contains(err.Error(), "visible to this caller") {
			t.Errorf("%s blamed visibility for a structural refusal: %v", r.Tool, err)
		}
		if !strings.Contains(err.Error(), meta.SubjectEstate) {
			t.Errorf("%s refused without naming what does answer: %v", r.Tool, err)
		}
	}
}

// TestAMissingProgramDoesNotBlameScope is section 36's V20 applied to the
// refusal path rather than to the projection.
//
// V20: "absent and withheld are different facts and only one of them means
// ask for more access." THREE facts arrive at this refusal - unregistered,
// out of scope, and gone since the caller last looked - and meta cannot tell
// them apart, because there is no tombstone at M2. It used to pick the middle
// one and assert it.
//
// THE COST OF ASSERTING IT IS NOT AN IMPRECISE SENTENCE. It is the one case
// section 37's clause 2 measured against AgentBox and lost: a holder dies and
// the agent is told the same words it would hear for a permission boundary,
// which call for opposite actions. AgentBox's `shared` distinguishes
// owner_gone. Naming the ambiguity does not close that gap - only a tombstone
// does - but it stops rig stating the one answer that is actively misleading
// when the program has crashed.
func TestAMissingProgramDoesNotBlameScope(t *testing.T) {
	s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: production()})

	_, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.Describe, Program: "never-registered"})
	if err == nil {
		t.Fatal("describing a program that does not exist was not refused")
	}
	if strings.Contains(err.Error(), "is visible to this caller") {
		t.Errorf("rig asserted a scope cause it cannot know: %v", err)
	}
	for _, want := range []string{"unregistered", "scope", "gone"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q as a possibility: %v", want, err)
		}
	}
}
