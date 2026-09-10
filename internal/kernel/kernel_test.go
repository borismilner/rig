package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// good is a declaration with every mandatory property said, so a test that
// wants one thing wrong can say only that thing.
func good(id string) kernel.Declaration {
	return kernel.Declaration{
		Identity:     kernel.Identity{ID: id, Name: id, Version: "1.0.0"},
		Coverage:     kernel.CoveragePartial,
		SemanticsGen: 1,
		Commands: []kernel.Command{{
			ID:           "reindex",
			Title:        "Reindex",
			Effects:      kernel.EffectsWritesFiles,
			Idempotent:   kernel.Yes,
			Sensitive:    []string{},
			Interactive:  kernel.No,
			Streams:      kernel.No,
			NeedsDisplay: kernel.No,
			Duration:     kernel.DurationSeconds,
			Confirms:     kernel.No,
			Shape:        kernel.ShapeUnary,
			Summary:      "Rebuild the index",
			Description:  "Walks the tree and rebuilds the index from scratch.",
			Returns:      "The number of items indexed.",
		}},
	}
}

func programPrincipal(id string) kernel.Principal {
	return kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: id, SessionID: "s-" + id, PID: 42,
	}
}

func agent(scopes ...string) kernel.Principal {
	return kernel.Principal{
		UID: 1000, Kind: kernel.KindAgent,
		ClientID: "an-agent", SessionID: "s-agent", PID: 43,
		Scopes: scopes, Scoped: len(scopes) > 0,
	}
}

func TestRegisterAndReadBack(t *testing.T) {
	k := kernel.New()
	p, err := k.Register(programPrincipal("pilot"), good("pilot"))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !p.Scoped {
		t.Fatal("registration is the only thing that sets Scoped, and it did not")
	}
	got, ok := k.See(p).Program("pilot")
	if !ok {
		t.Fatal("a program cannot see itself")
	}
	if got.Identity.Version != "1.0.0" || len(got.Commands) != 1 {
		t.Fatalf("read back the wrong declaration: %+v", got)
	}
}

func TestOnlyAProgramConnectionRegisters(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(agent(), good("pilot")); err == nil {
		t.Fatal("an agent connection registered a program")
	}
}

// Section 5e: no property has a default that carries a safety meaning, so
// every mandatory one has to have been said.
func TestEveryMandatoryPropertyIsRefusedWhenUnsaid(t *testing.T) {
	cases := map[string]func(*kernel.Command){
		"effects":       func(c *kernel.Command) { c.Effects = kernel.EffectsUnspecified },
		"idempotent":    func(c *kernel.Command) { c.Idempotent = kernel.Unsaid },
		"sensitive":     func(c *kernel.Command) { c.Sensitive = nil },
		"interactive":   func(c *kernel.Command) { c.Interactive = kernel.Unsaid },
		"streams":       func(c *kernel.Command) { c.Streams = kernel.Unsaid },
		"needs_display": func(c *kernel.Command) { c.NeedsDisplay = kernel.Unsaid },
		"duration":      func(c *kernel.Command) { c.Duration = kernel.DurationUnspecified },
		"confirms":      func(c *kernel.Command) { c.Confirms = kernel.Unsaid },
		"shape":         func(c *kernel.Command) { c.Shape = kernel.ShapeUnspecified },
		"summary":       func(c *kernel.Command) { c.Summary = "" },
		"description":   func(c *kernel.Command) { c.Description = "" },
		"returns":       func(c *kernel.Command) { c.Returns = "" },
	}
	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			d := good("pilot")
			break_(&d.Commands[0])
			err := d.Validate()
			if err == nil {
				t.Fatalf("%s was left unsaid and the declaration was accepted", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("the error does not name %s: %v", name, err)
			}
		})
	}
}

// An empty sensitive list is a declaration; an absent one is not.
func TestAnEmptySensitiveListIsSaidAndANilOneIsNot(t *testing.T) {
	d := good("pilot")
	d.Commands[0].Sensitive = []string{}
	if err := d.Validate(); err != nil {
		t.Fatalf("an empty sensitive list is a valid declaration: %v", err)
	}
	d.Commands[0].Sensitive = nil
	if err := d.Validate(); err == nil {
		t.Fatal("an absent sensitive list was accepted as an empty one")
	}
}

func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	d := good("pilot")
	d.Coverage = kernel.CoverageUnspecified
	d.SemanticsGen = 0
	d.Commands[0].Effects = kernel.EffectsUnspecified
	err := d.Validate()
	if err == nil {
		t.Fatal("wanted a refusal")
	}
	for _, want := range []string{"coverage", "semantics_gen", "effects"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal is missing %s: %v", want, err)
		}
	}
}

func TestRigsOwnNamespaceIsRefused(t *testing.T) {
	for _, id := range []string{"rig", "rigd"} {
		d := good(id)
		if err := d.Validate(); err == nil {
			t.Fatalf("%q was accepted as a program id", id)
		}
	}
}

func TestATakenIDIsRefusedToADifferentSession(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(programPrincipal("pilot"), good("pilot")); err != nil {
		t.Fatal(err)
	}
	other := programPrincipal("pilot")
	other.SessionID = "s-other"
	if _, err := k.Register(other, good("pilot")); err == nil {
		t.Fatal("two sessions hold the same program id")
	}
}

func TestDeregisterGivesTheIDBack(t *testing.T) {
	k := kernel.New()
	p, _ := k.Register(programPrincipal("pilot"), good("pilot"))
	k.Deregister(p.SessionID)
	other := programPrincipal("pilot")
	other.SessionID = "s-restarted"
	if _, err := k.Register(other, good("pilot")); err != nil {
		t.Fatalf("a restarted program could not take its own id back: %v", err)
	}
}

// Section 14: default deny. Two clients of one daemon are invisible to each
// other unless both opt in.
func TestAnUnprivilegedClientSeesNothingItIsNotScopedTo(t *testing.T) {
	k := kernel.New()
	mustRegister(t, k, "alpha")
	mustRegister(t, k, "beta")

	if got := k.See(agent("alpha")).Programs(); len(got) != 1 ||
		got[0].Identity.ID != "alpha" {
		t.Fatalf("a client scoped to alpha saw %v", ids(got))
	}
	if _, ok := k.See(agent("alpha")).Program("beta"); ok {
		t.Fatal("a client scoped to alpha read beta")
	}
}

// Section 14, run 2 of the three-run battery: a scoped answer where a
// complete one was owed fails as hard as a leak.
func TestAnIntrospectingClientSeesAllOfIt(t *testing.T) {
	k := kernel.New()
	mustRegister(t, k, "alpha")
	mustRegister(t, k, "beta")

	p := agent()
	p.Introspect = true
	got := k.See(p).Programs()
	if len(got) != 2 {
		t.Fatalf("an introspecting client saw %v, and a partial answer where a "+
			"complete one was owed fails as hard as a leak", ids(got))
	}
}

func TestAnUnscopedUnprivilegedClientSeesNoProgram(t *testing.T) {
	k := kernel.New()
	mustRegister(t, k, "alpha")
	if got := k.See(agent()).Programs(); len(got) != 0 {
		t.Fatalf("an unscoped client with no introspect saw %v", ids(got))
	}
}

func TestAProgramDoesNotReadAnother(t *testing.T) {
	k := kernel.New()
	alpha := mustRegister(t, k, "alpha")
	mustRegister(t, k, "beta")
	if _, ok := k.See(alpha).Program("beta"); ok {
		t.Fatal("alpha read beta's declaration")
	}
}

func TestCommandReadsThroughTheSameFilter(t *testing.T) {
	k := kernel.New()
	mustRegister(t, k, "alpha")
	if _, ok := k.See(agent("alpha")).Command("alpha", "reindex"); !ok {
		t.Fatal("a scoped client could not read its own command")
	}
	if _, ok := k.See(agent("beta")).Command("alpha", "reindex"); ok {
		t.Fatal("a command leaked past the program filter")
	}
}

func TestAPrincipalWithNoKindCannotAct(t *testing.T) {
	p := kernel.Principal{ClientID: "x", SessionID: "y"}
	if err := p.Valid(); err == nil {
		t.Fatal("a principal with the zero client kind was accepted")
	}
}

func TestAScopedPrincipalWithNoScopeIsRefused(t *testing.T) {
	p := agent()
	p.Scoped = true
	if err := p.Valid(); err == nil {
		t.Fatal("a scoped principal holding no scope would read nothing at all")
	}
}

func TestParseClientKindRefusesTheWildcardAndTheZero(t *testing.T) {
	if _, err := kernel.ParseClientKind("any"); err == nil {
		t.Fatal("any is a house rule's wildcard, not something a connection can be")
	}
	if _, err := kernel.ParseClientKind("unspecified"); err == nil {
		t.Fatal("the zero is not a client kind")
	}
	if k, err := kernel.ParseClientKind("schedule"); err != nil || k != kernel.KindSchedule {
		t.Fatalf("schedule did not parse: %v %v", k, err)
	}
}

func TestEffectsOrderForHouseRuleMatching(t *testing.T) {
	if !kernel.EffectsDestructive.AtLeastAsDangerousAs(kernel.EffectsReadOnly) {
		t.Fatal("destructive does not sort above read-only")
	}
	if kernel.EffectsReadOnly.AtLeastAsDangerousAs(kernel.EffectsDestructive) {
		t.Fatal("read-only sorts above destructive")
	}
}

func mustRegister(t *testing.T, k *kernel.Kernel, id string) kernel.Principal {
	t.Helper()
	p, err := k.Register(programPrincipal(id), good(id))
	if err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	return p
}

func ids(ps []kernel.Program) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Identity.ID)
	}
	return out
}
