package main

import (
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// A DEPTH DECIDES WHAT THE DAEMON SENT, SO IT DECIDES WHAT THIS CLIENT MAY
// ASSERT. Every empty rendering in appsJSON already carries a meaning: an
// empty `sensitive` is deliberately not null because null would read as "the
// program never considered the question", and an absent `args` means the
// command takes no arguments. The kernel's projection drops both below
// DEPTH_FULL and drops commands entirely at DEPTH_PROGRAMS, so at a shallow
// depth those renderings would assert a declaration nobody read.
//
// This is the tristate collapse this package already pins, in a form that is
// REACHABLE: the tristate one is unreachable because the kernel refuses an
// unsaid mandatory field, and this one is reachable the moment --depth exists.
func TestAShallowDepthOmitsWhatItDidNotFetchRatherThanRenderingItEmpty(t *testing.T) {
	// What the daemon actually sends back at each depth, mirroring
	// kernel.atDepth: DEPTH_PROGRAMS nils Commands, DEPTH_COMMANDS nils Args
	// and Sensitive on each command, DEPTH_FULL keeps everything.
	full := func() *rigv1.Program {
		return &rigv1.Program{
			Identity: &rigv1.Identity{Id: "fakeapp", Version: "1.2.0"},
			Coverage: rigv1.Coverage_COVERAGE_FULL,
			Commands: []*rigv1.Command{{
				Id:        "reindex",
				Args:      []byte(`{"type":"object"}`),
				Sensitive: &rigv1.SensitiveFields{Pointers: []string{"/token"}},
			}},
		}
	}

	for _, tc := range []struct {
		depth           rigv1.Depth
		program         *rigv1.Program
		wantCommandsKey bool
		wantDetailKeys  bool
	}{
		{
			depth: rigv1.Depth_DEPTH_PROGRAMS,
			program: func() *rigv1.Program {
				p := full()
				p.Commands = nil
				return p
			}(),
			wantCommandsKey: false,
		},
		{
			depth: rigv1.Depth_DEPTH_COMMANDS,
			program: func() *rigv1.Program {
				p := full()
				p.Commands[0].Args = nil
				p.Commands[0].Sensitive = nil
				return p
			}(),
			wantCommandsKey: true,
			wantDetailKeys:  false,
		},
		{
			depth:           rigv1.Depth_DEPTH_FULL,
			program:         full(),
			wantCommandsKey: true,
			wantDetailKeys:  true,
		},
	} {
		t.Run(tc.depth.String(), func(t *testing.T) {
			rows := appsJSON([]*rigv1.Program{tc.program}, tc.depth)
			if len(rows) != 1 {
				t.Fatalf("appsJSON rendered %d rows from one program", len(rows))
			}
			// Through the marshaller, so anything omitempty does is included
			// in what is measured rather than read off the map.
			var got map[string]any
			raw, err := json.Marshal(rows[0])
			if err != nil {
				t.Fatalf("the object did not render: %v", err)
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("the object did not round-trip: %v", err)
			}

			if _, ok := got["commands"]; ok != tc.wantCommandsKey {
				t.Errorf("at %s the commands key present=%v, want %v: an empty "+
					"list here says the program declares none, which is not "+
					"what a depth that did not ask for them means",
					tc.depth, ok, tc.wantCommandsKey)
			}
			if !tc.wantCommandsKey {
				return
			}

			cmds, ok := got["commands"].([]any)
			if !ok || len(cmds) != 1 {
				t.Fatalf("at %s the commands key is %T with %d entries, so the "+
					"fixture cannot say anything about the keys below it",
					tc.depth, got["commands"], len(cmds))
			}
			cmd, ok := cmds[0].(map[string]any)
			if !ok {
				t.Fatalf("at %s a command rendered as %T", tc.depth, cmds[0])
			}

			for _, key := range []string{"sensitive", "args"} {
				if _, ok := cmd[key]; ok != tc.wantDetailKeys {
					t.Errorf("at %s the %s key present=%v, want %v: the kernel's "+
						"projection drops it below DEPTH_FULL, so rendering it "+
						"reports what the projection removed as what the "+
						"program declared", tc.depth, key, ok, tc.wantDetailKeys)
				}
			}

			// The scalars a depth DOES fetch must still be there, or this
			// test would pass just as well against a renderer that dropped
			// everything.
			for _, key := range []string{"id", "summary", "effects", "idempotent"} {
				if _, ok := cmd[key]; !ok {
					t.Errorf("at %s the %s key is missing, and this depth does "+
						"fetch it", tc.depth, key)
				}
			}
		})
	}
}

// The count is a FUNCTION so it can be asserted without a live rigd. cmdApps
// needs one; this does not, and the fifteen functions that do are this
// package's documented coverage hole.
func TestTheCommandCountIsOmittedRatherThanZeroWhenNoCommandsWereFetched(t *testing.T) {
	twenty := &rigv1.Program{Identity: &rigv1.Identity{Id: "fakeapp"}}
	for range 20 {
		twenty.Commands = append(twenty.Commands, &rigv1.Command{Id: "c"})
	}

	// At DEPTH_PROGRAMS the daemon sends none of them, which is what the
	// empty program models.
	none := &rigv1.Program{Identity: &rigv1.Identity{Id: "fakeapp"}}

	if got := commandCountCell(none, rigv1.Depth_DEPTH_PROGRAMS); got != "" {
		t.Errorf("at DEPTH_PROGRAMS the count cell is %q, want empty: a program "+
			"with twenty commands would be reported as having none, which is a "+
			"false statement rather than a cheaper one", got)
	}
	if got := commandCountCell(twenty, rigv1.Depth_DEPTH_FULL); got != "20 commands, " {
		t.Errorf("at DEPTH_FULL the count cell is %q, want %q", got, "20 commands, ")
	}
	// The control: a program that genuinely has none still says so at a depth
	// that asked, or the omission above would be indistinguishable from it.
	if got := commandCountCell(none, rigv1.Depth_DEPTH_FULL); got != "0 commands, " {
		t.Errorf("at DEPTH_FULL a program with no commands renders %q, want %q - "+
			"the omission at a shallow depth must not swallow a real zero",
			got, "0 commands, ")
	}
}

// Section 5k: no surface may imply completeness. A shallow listing says what
// it left out; a full one has nothing to disclose and its output is unchanged.
func TestAShallowListingSaysWhatItWithheldAndAFullOneSaysNothing(t *testing.T) {
	for _, tc := range []struct {
		depth rigv1.Depth
		want  string
	}{
		{rigv1.Depth_DEPTH_PROGRAMS, "no commands were asked for"},
		{rigv1.Depth_DEPTH_COMMANDS, "without its arguments"},
		{rigv1.Depth_DEPTH_FULL, ""},
	} {
		got := withheldNote(tc.depth)
		switch {
		case tc.want == "" && got != "":
			t.Errorf("at %s the listing discloses %q, and nothing was withheld",
				tc.depth, got)
		case tc.want != "" && !strings.Contains(got, tc.want):
			t.Errorf("at %s the listing says %q, which does not say it withheld "+
				"%q", tc.depth, got, tc.want)
		}
	}
}

// THE FLAG'S SPELLINGS COME OFF THE WIRE'S OWN ENUM, AND THIS IS WHAT STOPS
// THEM DRIFTING.
//
// kernel.ParseDepth takes exactly these spellings and cmd/rig must not call
// it - the kernel links the JSON Schema validator, which is section 17's 1.45
// MB in the one binary the plan says must not pay it, and the layering test
// would fail naming santhosh-tekuri rather than the kernel. Walking the
// descriptor is the same contract without the dependency, and a depth added
// to the proto is picked up here with no edit.
func TestTheDepthSpellingsAreTheWireEnumsOwnAndExcludeTheZero(t *testing.T) {
	values := rigv1.Depth(0).Descriptor().Values()

	var want []string
	for i := range values.Len() {
		v := values.Get(i)
		if v.Number() == 0 {
			continue
		}
		want = append(want, enumLabel(string(v.Name()), "DEPTH_"))
	}

	// The positive control first: an absence is also what a walk that never
	// ran produces, so the enum has to be shown to have values at all.
	if len(want) == 0 {
		t.Fatal("the Depth descriptor yielded no non-zero values, so every " +
			"assertion below would pass against a walk that did nothing")
	}
	if got := depthSpellings(); !slices.Equal(got, want) {
		t.Errorf("--depth offers %v and the wire declares %v", got, want)
	}
	if slices.Contains(depthSpellings(), "unspecified") {
		t.Error("--depth offers the enum's zero: section 21 makes it mean " +
			"\"nothing was said\", so no surface may let a caller ask for it")
	}
}

func TestParseDepthRefusesTheZeroAndTakesAnAbsentFlagAsAbsent(t *testing.T) {
	// An absent flag is not a default. It sends the zero, and the daemon's
	// boundary restores the old wire's meaning - which is the one place that
	// knows the two disagree.
	got, err := parseDepth("")
	if err != nil || got != rigv1.Depth_DEPTH_UNSPECIFIED {
		t.Errorf("an absent --depth parsed as (%v, %v), want the zero and no "+
			"error: absence is what the compatibility rule is written against",
			got, err)
	}

	for _, name := range depthSpellings() {
		d, err := parseDepth(name)
		if err != nil {
			t.Errorf("--depth %s was refused: %v", name, err)
			continue
		}
		if d == rigv1.Depth_DEPTH_UNSPECIFIED {
			t.Errorf("--depth %s parsed as the zero", name)
		}
	}

	for _, bad := range []string{"unspecified", "DEPTH_FULL", "Full", "everything"} {
		if _, err := parseDepth(bad); err == nil {
			t.Errorf("--depth %s was accepted, and it is not a depth "+
				"the wire declares", bad)
		}
	}
}

// EVERY NON-BOOLEAN FLAG MUST BE IN valuedFlags, AND NOTHING COUPLED THE TWO
// UNTIL --depth.
//
// partition() splits flags from positionals before flag.Parse runs, so it has
// to know which flags eat the next argument, and valuedFlags is how it knows.
// Every flag on `apps` was a boolean until --depth, so the set was never
// exercised here and a missing entry is invisible to every other test in this
// package: `--depth full` reads `full` as a positional and dies with "flag
// needs an argument", which blames the flag rather than the set.
//
// This is the argv path, which nothing else in cmd/rig covers - the fifteen
// functions that need a live rigd are this package's documented coverage hole,
// and the defect this test pins was found by running the binary, not by a test.
// IT WALKS EVERY FLAG SET THIS PACKAGE BUILDS, NOT ONLY apps'. It walked only
// apps' until `rig estate` added the second one, and a test that walks one
// command's flags is blind to the next command by construction - which is the
// same shape as the defect it was written for: the coupling existed and
// nothing exercised it. Adding a verb with a valued flag and forgetting
// valuedFlags would have been invisible again.
func TestEveryFlagThatTakesAValueIsDeclaredToThePartitioner(t *testing.T) {
	appsFS, _, _, _ := appsFlagSet()
	estateFS, _, _ := estateFlagSet()
	sets := map[string]*flag.FlagSet{"apps": appsFS, "estate": estateFS}

	var checked int
	for verb, fs := range sets {
		fs.VisitAll(func(f *flag.Flag) {
			// A boolean is the only kind that does not consume its next
			// argument. flag's own BoolFlag marker is what the parser itself
			// reads, so it is what this asks rather than the name or the
			// default.
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
	// produces, and this test is entirely assertions about absence.
	if checked == 0 {
		t.Fatal("no non-boolean flag was examined, so this test would pass " +
			"against a VisitAll that visited nothing")
	}

	// THE CONTROL THAT MAKES THE LIST ABOVE MAINTAIN ITSELF, and the first
	// version of it did not work.
	//
	// A count-based control - "checked must be at least len(sets)" - passes
	// whether the map holds one flag set or both, because dropping a set drops
	// its flags from the count too. A MUTATION PROVED THAT: removing `estate`
	// from the map survived. So the control reads the SOURCE for every
	// constructor that builds a *flag.FlagSet and fails if one is not walked,
	// which is client/surface_test.go's technique applied to a second
	// enumeration that has to stay complete.
	for _, name := range flagSetConstructors(t) {
		if _, ok := sets[strings.TrimSuffix(name, "FlagSet")]; !ok {
			t.Errorf("%s builds a flag set and this test does not walk it, so "+
				"a valued flag on that verb is not coupled to valuedFlags and "+
				"its value will be handed to the positionals", name)
		}
	}
}

// flagSetConstructors names every top-level function in this package that
// returns a *flag.FlagSet, read off the source.
//
// Reflection cannot answer this: the constructors are unexported and nothing
// registers them anywhere, so the only record that a verb has flags is the
// function itself. Reading the source is what client/surface_test.go does for
// the stub's exported surface, and for the same reason - the thing being
// asserted is that an ENUMERATION is complete, and an enumeration cannot check
// its own completeness.
func flagSetConstructors(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Type.Results == nil {
				continue
			}
			for _, r := range fn.Type.Results.List {
				star, ok := r.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				sel, ok := star.X.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "FlagSet" {
					continue
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "flag" {
					out = append(out, fn.Name.Name)
				}
			}
		}
	}
	// The control on the control: a source walk that found nothing is
	// indistinguishable from a package with no flag sets in it.
	if len(out) == 0 {
		t.Fatal("no flag-set constructor was found in this package's source, " +
			"so this check would pass against a parse that read nothing")
	}
	return out
}

// The whole argv path for the flag, which is what actually broke.
func TestTheDepthFlagSurvivesPartitioningInEveryWrittenForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"space separated", []string{"list", "--depth", "programs"}},
		{"equals form", []string{"list", "--depth=programs"}},
		{"before the verb", []string{"--depth", "programs", "list"}},
		{"beside a boolean", []string{"list", "--depth", "programs", "--json"}},
		{"boolean first", []string{"list", "--json", "--depth", "programs"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs, _, _, depth := appsFlagSet()
			flags, positional := partition(tc.argv)
			if err := fs.Parse(flags); err != nil {
				t.Fatalf("%v did not parse: %v", tc.argv, err)
			}
			if *depth != "programs" {
				t.Errorf("%v gave --depth %q, want %q", tc.argv, *depth, "programs")
			}
			if !slices.Contains(positional, "list") {
				t.Errorf("%v lost the verb: positionals are %v", tc.argv, positional)
			}
			if slices.Contains(positional, "programs") {
				t.Errorf("%v left the depth's value in the positionals %v, which "+
					"is what a missing valuedFlags entry does", tc.argv, positional)
			}
		})
	}
}
