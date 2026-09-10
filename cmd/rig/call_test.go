package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// reindexDecl is a declared command with one of each flag-able type, so a
// test can prove the flags are typed by the DECLARATION rather than by the
// shape of what was typed.
func reindexDecl() *rigv1.Command {
	return &rigv1.Command{
		Id: "reindex",
		Args: []byte(`{
      "type": "object",
      "additionalProperties": false,
      "required": ["since"],
      "properties": {
        "since":   {"type": "string"},
        "dry_run": {"type": "boolean"},
        "workers": {"type": "integer"},
        "ratio":   {"type": "number"},
        "either":  {"type": ["string", "integer"]},
        "nested":  {"type": "object"}
      }
    }`),
	}
}

func TestOwnFlagsAreTakenOutAndTheCommandsAreLeft(t *testing.T) {
	own, rest, err := splitOwnFlags([]string{
		"--since", "7d", "--json", "--timeout", "9s", "--dry-run",
	})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if !own.asJSON {
		t.Fatal("--json was not taken")
	}
	if own.timeout != 9*time.Second {
		t.Fatalf("--timeout came out as %s", own.timeout)
	}
	// The declared flags have to survive intact, values included: rig cannot
	// know that --since takes one until it has read the registry.
	want := "--since 7d --dry-run"
	if got := strings.Join(rest, " "); got != want {
		t.Fatalf("the command's own flags came out as %q, want %q", got, want)
	}
}

func TestOwnFlagsInEveryFormTheyCanBeWritten(t *testing.T) {
	cases := map[string]struct {
		argv    []string
		asJSON  bool
		timeout time.Duration
		args    string
	}{
		"equals":      {argv: []string{"--json=true", "--timeout=2s"}, asJSON: true, timeout: 2 * time.Second},
		"json false":  {argv: []string{"--json=false"}, timeout: 30 * time.Second},
		"bare json":   {argv: []string{"--json"}, asJSON: true, timeout: 30 * time.Second},
		"single dash": {argv: []string{"-json"}, asJSON: true, timeout: 30 * time.Second},
		"args":        {argv: []string{"--args", `{"since":"1d"}`}, timeout: 30 * time.Second, args: `{"since":"1d"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			own, rest, err := splitOwnFlags(tc.argv)
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			if own.asJSON != tc.asJSON || own.timeout != tc.timeout || own.args != tc.args {
				t.Fatalf("got json=%v timeout=%s args=%q",
					own.asJSON, own.timeout, own.args)
			}
			if len(rest) != 0 {
				t.Fatalf("leftovers: %v", rest)
			}
		})
	}
}

func TestEverythingAfterADoubleDashBelongsToTheCommand(t *testing.T) {
	_, rest, err := splitOwnFlags([]string{"--json", "--", "--timeout", "--args"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	// Past `--`, rig's own flag names are the command's business.
	if strings.Join(rest, " ") != "--timeout --args" {
		t.Fatalf("rest is %v", rest)
	}
}

func TestAnOwnFlagWithNoValueSaysSo(t *testing.T) {
	for _, argv := range [][]string{{"--timeout"}, {"--args"}} {
		if _, _, err := splitOwnFlags(argv); err == nil {
			t.Fatalf("%v was accepted", argv)
		}
	}
	if _, _, err := splitOwnFlags([]string{"--timeout", "soon"}); err == nil {
		t.Fatal("an unparseable duration was accepted")
	}
}

func TestFlagsAreTypedByTheDeclarationNotByTheirText(t *testing.T) {
	own, rest, err := splitOwnFlags([]string{
		"--since", "7d", "--workers", "8", "--ratio", "0.5", "--dry-run",
	})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	args, err := buildArgs(reindexDecl(), own, rest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(args, &got); err != nil {
		t.Fatalf("the arguments are not JSON: %v", err)
	}
	// "8" is a number because the schema says integer, and "7d" is a string
	// because the schema says string. Nothing here guesses from the text.
	if got["workers"] != float64(8) {
		t.Fatalf("workers came out as %#v", got["workers"])
	}
	if got["since"] != "7d" {
		t.Fatalf("since came out as %#v", got["since"])
	}
	if got["ratio"] != 0.5 {
		t.Fatalf("ratio came out as %#v", got["ratio"])
	}
	// A declared dry_run is typed as --dry-run, because nobody types an
	// underscore, and the JSON carries the declared name.
	if got["dry_run"] != true {
		t.Fatalf("dry_run came out as %#v", got["dry_run"])
	}
}

func TestABooleanFlagCanStillBeGivenAValue(t *testing.T) {
	own, rest, _ := splitOwnFlags([]string{"--since", "7d", "--dry-run=false"})
	args, err := buildArgs(reindexDecl(), own, rest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(args), `"dry_run":false`) {
		t.Fatalf("arguments came out as %s", args)
	}
}

func TestAFlagThatWasNotDeclaredIsRefusedWithTheOnesThatWere(t *testing.T) {
	own, rest, _ := splitOwnFlags([]string{"--since", "7d", "--untl", "now"})
	_, err := buildArgs(reindexDecl(), own, rest)
	if err == nil {
		t.Fatal("an undeclared flag was accepted")
	}
	// The error is the whole point: whoever typed it needs the list, not a
	// complaint.
	for _, want := range []string{"--untl", "--since", "--dry-run", "--workers"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not mention %s: %v", want, err)
		}
	}
}

func TestATypeWithNoFlagFormPointsAtArgs(t *testing.T) {
	for _, argv := range [][]string{
		{"--nested", "{}"},    // an object
		{"--either", "7"},     // declared as two types
		{"--workers", "many"}, // not a whole number
		{"--ratio", "x"},      // not a number
		{"--since"},           // no value
	} {
		own, rest, err := splitOwnFlags(argv)
		if err != nil {
			continue // rejected earlier, which is also a refusal
		}
		if _, err := buildArgs(reindexDecl(), own, rest); err == nil {
			t.Fatalf("%v was accepted", argv)
		}
	}
}

func TestArgsIsExactAndCannotBeMixedWithFlags(t *testing.T) {
	own, rest, _ := splitOwnFlags([]string{"--args", `{"since":"3d"}`})
	args, err := buildArgs(reindexDecl(), own, rest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if string(args) != `{"since":"3d"}` {
		t.Fatalf("--args was not passed through verbatim: %s", args)
	}

	own, rest, _ = splitOwnFlags([]string{"--args", `{"since":"3d"}`, "--dry-run"})
	if _, err := buildArgs(reindexDecl(), own, rest); err == nil {
		t.Fatal("--args and a flag were merged rather than refused")
	}

	own, rest, _ = splitOwnFlags([]string{"--args", `{since`})
	if _, err := buildArgs(reindexDecl(), own, rest); err == nil {
		t.Fatal("--args accepted something that is not JSON")
	}
}

func TestACommandThatDeclaresNoArgumentsRefusesFlags(t *testing.T) {
	purge := &rigv1.Command{Id: "purge"}

	own, rest, _ := splitOwnFlags([]string{"--since", "7d"})
	if _, err := buildArgs(purge, own, rest); err == nil {
		t.Fatal("a command with no declared arguments accepted a flag")
	}

	// With no flags it is a call with no arguments, which is fine.
	own, rest, _ = splitOwnFlags(nil)
	args, e := buildArgs(purge, own, rest)
	if e != nil {
		t.Fatalf("a bare call was refused: %v", e)
	}
	if len(args) != 0 {
		t.Fatalf("a bare call sent %s", args)
	}
}

func TestAPositionalArgumentIsRefusedRatherThanIgnored(t *testing.T) {
	own, rest, _ := splitOwnFlags([]string{"7d"})
	if _, err := buildArgs(reindexDecl(), own, rest); err == nil {
		t.Fatal("a positional argument was silently dropped")
	}
}

func TestHumanRenderingDoesNotLoseAnything(t *testing.T) {
	out := human(map[string]any{
		"since": "7d", "indexed": float64(274), "dry_run": false,
		"missing": nil, "list": []any{"a", "b"},
	})
	for _, want := range []string{
		"since", "7d", "indexed", "274", "dry_run",
		"false", "missing", "-", "a, b",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q is missing from:\n%s", want, out)
		}
	}
}

func TestAResultThatIsNotJSONIsReportedRatherThanSwallowed(t *testing.T) {
	// Both modes, because a defect that depends on who is looking is worse
	// than the defect.
	for _, asJSON := range []bool{true, false} {
		if err := printResult("shelf", "reindex", []byte("not json"), asJSON); err == nil {
			t.Fatalf("asJSON=%v accepted a non-JSON result", asJSON)
		}
	}
	if err := printResult("shelf", "reindex", nil, true); err != nil {
		t.Fatalf("an empty result was an error: %v", err)
	}
	if err := printResult("shelf", "reindex", []byte(`{"ok":true}`), true); err != nil {
		t.Fatalf("a good result was an error: %v", err)
	}
}

func TestAFlagGivenTwiceIsRefused(t *testing.T) {
	own, rest, _ := splitOwnFlags([]string{"--since", "1d", "--since", "2d"})
	_, err := buildArgs(reindexDecl(), own, rest)
	if err == nil {
		t.Fatal("the second --since silently won")
	}
	if !strings.Contains(err.Error(), "twice") {
		t.Fatalf("the error does not say what happened: %v", err)
	}
}

func TestRigsOwnFlagsWinACollisionAndArgsIsTheWayOut(t *testing.T) {
	// A command that declares a property called `json`. rig's flag wins,
	// because --json means the same thing on every surface of rig - so the
	// declared one is unreachable as sugar and has to go through --args.
	collide := &rigv1.Command{
		Id: "export",
		Args: []byte(`{"type":"object","properties":{
      "json": {"type": "boolean"},
      "timeout": {"type": "string"}
    }}`),
	}

	own, rest, err := splitOwnFlags([]string{"--json", "--timeout", "5s"})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	// Both were taken as rig's own, so nothing is left for the command.
	if !own.asJSON || own.timeout != 5*time.Second {
		t.Fatalf("rig's own flags did not win: json=%v timeout=%s",
			own.asJSON, own.timeout)
	}
	args, err := buildArgs(collide, own, rest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("a colliding flag leaked into the arguments: %s", args)
	}

	// And the way out works.
	own, rest, _ = splitOwnFlags([]string{"--args", `{"json":true,"timeout":"5s"}`})
	args, err = buildArgs(collide, own, rest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(string(args), `"json":true`) {
		t.Fatalf("--args did not carry the declared json property: %s", args)
	}
}
