package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// withPreamble is a declaration that fills every field the depths trim, so a
// test can tell "absent because the depth dropped it" from "absent because
// nothing set it". A fixture that left them empty would pass every trimming
// assertion without trimming anything.
func withPreamble(id string) kernel.Declaration {
	d := withCommands(id, map[string]kernel.Effects{
		"reindex": kernel.EffectsWritesFiles,
	})
	d.Preamble = "Read this before touching " + id + "."
	d.Coverage = kernel.CoveragePartial
	d.CoverageNote = "search is adopted; storage is not"
	for i := range d.Commands {
		d.Commands[i].Args = []byte(`{"type":"object"}`)
		d.Commands[i].Examples = []string{"rig " + id + " reindex --since 7d"}
		d.Commands[i].Preconditions = []string{id + ".index.path exists"}
		d.Commands[i].Sensitive = []string{"/token"}
		d.Commands[i].Description = "Rebuilds the index from the tree."
		d.Commands[i].Returns = "The number of items indexed."
		d.Commands[i].Summary = "Rebuild the index"
	}
	return d
}

func estateOf(t *testing.T, k *kernel.Kernel, p kernel.Principal, d kernel.Depth) []kernel.Program {
	t.Helper()
	got, err := k.See(p).Estate(d)
	if err != nil {
		t.Fatalf("estate at %s: %v", d, err)
	}
	return got
}

// TestSliceTwoDemo is M2 slice 2's demo, run rather than described:
//
//	"An introspecting client and a scoped one get different estates from the
//	 same call; describe on a program returns its preamble, describe on a
//	 command returns the full declaration with examples and effects."
//
// All three halves, in the order the demo states them.
func TestSliceTwoDemo(t *testing.T) {
	k := kernel.New()
	for _, id := range []string{"shelf", "grabbit"} {
		if _, err := k.Register(programPrincipal(id), withPreamble(id)); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	// One call, two principals. The introspecting one is an agent Boris runs
	// (section 2, section 14); the scoped one is a program that may see only
	// itself.
	seeing := caller(kernel.KindAgent)
	scoped := programPrincipal("shelf")
	scoped.Scoped = true
	scoped.Scopes = []string{"shelf"}

	whole := estateOf(t, k, seeing, kernel.DepthPrograms)
	partial := estateOf(t, k, scoped, kernel.DepthPrograms)

	if len(whole) != 2 {
		t.Fatalf("the introspecting client sees %d programs, want 2: %v",
			len(whole), ids(whole))
	}
	if len(partial) != 1 || partial[0].Identity.ID != "shelf" {
		t.Fatalf("the scoped client sees %v, want just shelf", ids(partial))
	}

	// Coverage travels with every program at every depth, because section 5k
	// forbids a surface implying completeness.
	for _, p := range whole {
		if p.Coverage != kernel.CoveragePartial {
			t.Errorf("%s lost its coverage in the estate: %v",
				p.Identity.ID, p.Coverage)
		}
		if p.CoverageNote == "" {
			t.Errorf("%s lost its coverage note", p.Identity.ID)
		}
	}

	// describe on a PROGRAM returns its preamble.
	shelf, ok := k.See(seeing).Program("shelf")
	if !ok {
		t.Fatal("describe found no shelf")
	}
	if !strings.Contains(shelf.Preamble, "Read this before touching shelf") {
		t.Fatalf("describe on a program did not return its preamble: %q",
			shelf.Preamble)
	}

	// describe on a COMMAND returns the full declaration, with the examples
	// and effects the demo names.
	cmd, ok := k.See(seeing).Command("shelf", "reindex")
	if !ok {
		t.Fatal("describe found no shelf.reindex")
	}
	if len(cmd.Examples) == 0 {
		t.Error("describe on a command returned no examples, which section 9 " +
			"calls the single highest-value field")
	}
	if cmd.Effects != kernel.EffectsWritesFiles {
		t.Errorf("describe on a command returned effects %v", cmd.Effects)
	}
	if len(cmd.Args) == 0 {
		t.Error("describe on a command returned no argument schema")
	}
	if len(cmd.Preconditions) == 0 {
		t.Error("describe on a command returned no preconditions")
	}
}

// TestTheEstateGetsCheaperWithDepthAndNeverWider is the property that makes
// depth safe to expose: a depth decides how much is said about a program,
// never whether the program is mentioned.
//
// A cheaper read that showed MORE of the estate would be a scope hole wearing
// a performance argument, and it is the mistake a second filter inside the
// depth path would make.
func TestTheEstateGetsCheaperWithDepthAndNeverWider(t *testing.T) {
	k := kernel.New()
	for _, id := range []string{"shelf", "grabbit"} {
		if _, err := k.Register(programPrincipal(id), withPreamble(id)); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	scoped := programPrincipal("shelf")
	scoped.Scoped = true
	scoped.Scopes = []string{"shelf"}

	for _, p := range []struct {
		name string
		who  kernel.Principal
		want []string
	}{
		{"introspecting", caller(kernel.KindAgent), []string{"grabbit", "shelf"}},
		{"scoped", scoped, []string{"shelf"}},
	} {
		t.Run(p.name, func(t *testing.T) {
			for _, d := range []kernel.Depth{
				kernel.DepthPrograms, kernel.DepthCommands, kernel.DepthFull,
			} {
				got := ids(estateOf(t, k, p.who, d))
				if strings.Join(got, ",") != strings.Join(p.want, ",") {
					t.Errorf("at depth %s the estate is %v, want %v", d, got, p.want)
				}
			}
		})
	}
}

// TestEachDepthDropsWhatTheNextOneCarries is the trimming itself, asserted as
// a difference rather than as a list of empty fields. Every field checked is
// non-empty in the fixture, so an assertion that something is absent is an
// assertion that the depth removed it.
func TestEachDepthDropsWhatTheNextOneCarries(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(programPrincipal("shelf"), withPreamble("shelf")); err != nil {
		t.Fatalf("register: %v", err)
	}
	who := caller(kernel.KindAgent)

	programs := estateOf(t, k, who, kernel.DepthPrograms)[0]
	commands := estateOf(t, k, who, kernel.DepthCommands)[0]
	full := estateOf(t, k, who, kernel.DepthFull)[0]

	// DepthPrograms: the estate and nothing about what it can do.
	if programs.Commands != nil {
		t.Errorf("depth %s carried %d commands", kernel.DepthPrograms,
			len(programs.Commands))
	}
	if programs.Preamble != "" {
		t.Errorf("depth %s carried the preamble", kernel.DepthPrograms)
	}
	if programs.Identity.ID == "" || programs.Coverage == kernel.CoverageUnspecified {
		t.Error("depth programs dropped something every depth must carry")
	}

	// DepthCommands: what a command is chosen BY, and nothing it is called
	// WITH.
	if len(commands.Commands) != 1 {
		t.Fatalf("depth %s carried %d commands, want 1", kernel.DepthCommands,
			len(commands.Commands))
	}
	c := commands.Commands[0]
	for _, f := range []struct {
		name  string
		empty bool
	}{
		{"args", len(c.Args) == 0},
		{"examples", len(c.Examples) == 0},
		{"preconditions", len(c.Preconditions) == 0},
		{"sensitive", len(c.Sensitive) == 0},
		{"description", c.Description == ""},
		{"returns", c.Returns == ""},
	} {
		if !f.empty {
			t.Errorf("depth %s carried %s, which is what the command is called "+
				"WITH rather than chosen by", kernel.DepthCommands, f.name)
		}
	}
	for _, f := range []struct {
		name string
		ok   bool
	}{
		{"id", c.ID != ""},
		{"title", c.Title != ""},
		{"summary", c.Summary != ""},
		{"effects", c.Effects != kernel.EffectsUnspecified},
	} {
		if !f.ok {
			t.Errorf("depth %s dropped %s, which is what a command is chosen by",
				kernel.DepthCommands, f.name)
		}
	}
	if commands.Preamble != "" {
		t.Errorf("depth %s carried the preamble", kernel.DepthCommands)
	}

	// DepthFull: everything the fixture set.
	fc := full.Commands[0]
	if full.Preamble == "" || len(fc.Args) == 0 || len(fc.Examples) == 0 ||
		len(fc.Preconditions) == 0 || fc.Description == "" || fc.Returns == "" {
		t.Errorf("depth %s dropped something: %+v", kernel.DepthFull, full)
	}
}

// TestAnAbsentDepthIsRefusedRatherThanDefaulted is section 21's enum-zero rule
// applied to a Go enum the proto gate does not reach.
//
// Both possible defaults are wrong in a way that is silent: the cheapest would
// hide every command from a caller that forgot to ask, and the most complete
// would defeat the context budget the whole mechanism exists for.
func TestAnAbsentDepthIsRefusedRatherThanDefaulted(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(programPrincipal("shelf"), withPreamble("shelf")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := k.See(caller(kernel.KindAgent)).Estate(kernel.Depth(0)); err == nil {
		t.Fatal("the zero depth was accepted")
	}
	if _, err := k.See(caller(kernel.KindAgent)).Estate(kernel.Depth(99)); err == nil {
		t.Fatal("a depth past the last one was accepted")
	}
}

// TestDepthRoundTripsThroughItsName is what a surface needs: every depth a
// caller can be shown is a depth it can ask for, and nothing else parses.
func TestDepthRoundTripsThroughItsName(t *testing.T) {
	for _, d := range []kernel.Depth{
		kernel.DepthPrograms, kernel.DepthCommands, kernel.DepthFull,
	} {
		got, err := kernel.ParseDepth(d.String())
		if err != nil {
			t.Errorf("%s does not parse back: %v", d, err)
		}
		if got != d {
			t.Errorf("%s parsed back as %s", d, got)
		}
	}
	for _, bad := range []string{"", "unspecified", "deep", "FULL"} {
		if _, err := kernel.ParseDepth(bad); err == nil {
			t.Errorf("%q parsed as a depth", bad)
		}
	}
}

// TestATrimmedReadCannotEditWhatAProgramDeclared locks the property, and the
// comment says which line provides it, because they are not the same thing.
//
// THE PROTECTION IS UPSTREAM, IN View.Programs' cloneCommands, NOT IN THE
// TRIM. Measured, not assumed: replacing commandsAtDepth's own copy with an
// alias of its argument leaves this test GREEN, because by then it is already
// operating on a private clone. So a green run here is evidence that a caller
// cannot reach the registry - which is the property worth locking - and it is
// NOT evidence that the trim copies anything. The copy in commandsAtDepth
// stays because a function that mutates its argument is a bad shape whether
// or not anyone can currently observe it, and it is recorded as untested
// rather than left to look covered.
func TestATrimmedReadCannotEditWhatAProgramDeclared(t *testing.T) {
	k := kernel.New()
	if _, err := k.Register(programPrincipal("shelf"), withPreamble("shelf")); err != nil {
		t.Fatalf("register: %v", err)
	}
	who := caller(kernel.KindAgent)

	// Take a trimmed read and scribble on it. Indexed rather than copied out:
	// a Program taken by value is a copy, so writing its string fields lands
	// nowhere and proves nothing. govet's unusedwrite caught exactly that in
	// the first version of this test, which had written to a local copy and
	// then asserted the registry was unchanged - true, and for the wrong
	// reason.
	// Only the slice-backed fields are scribbled on. A string field cannot
	// alias, so asserting that Preamble survived would be an assertion that
	// can never fail - which is the same defect as the two above, kept out
	// rather than written and explained.
	trimmed := estateOf(t, k, who, kernel.DepthFull)
	trimmed[0].Commands[0].Summary = "scribbled"
	trimmed[0].Commands[0].Effects = kernel.EffectsDestructive

	full := estateOf(t, k, who, kernel.DepthFull)[0]
	if full.Commands[0].Summary == "scribbled" {
		t.Error("a trimmed read shares the registry's command")
	}
	if full.Commands[0].Effects != kernel.EffectsWritesFiles {
		t.Errorf("a trimmed read changed what the program declared: %v",
			full.Commands[0].Effects)
	}
}
