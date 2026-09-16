package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// A REFUSAL THAT NAMES A VERB rig DOES NOT DISPATCH SERVES A FIX NOBODY CAN
// RUN, AND THE FAILURE THEN READS AS THE CALLER'S FAULT RATHER THAN THE
// MESSAGE'S.
//
// The case this exists for shipped. `internal/daemon/presence_serve.go` told a
// caller refused a held seat to "Run `rig peers` to see the roster". There is
// no `peers` verb: the word falls through the dispatch switch to the DEFAULT
// branch, which reads the first token as a PROGRAM name, so a caller following
// the advice was told its program did not exist. `rig apps` lists programs and
// not peers, so no existing command could have satisfied that advice either.
// It was deleted at 7ee7233 and nothing stopped the next one.
//
// `internal/kernel/refusal.go` already states the rule this enforces, about
// FixCommand: "Runnable as written, or empty. NEVER prose ... anything here
// that is not a real command is a defect and not a hint."
//
// WHY A TEST AND NOT A FOURTH ANALYZER, which is the shape this repository
// reaches for first. The three in `internal/analysis` run PER PACKAGE, and a
// package under `internal/` cannot see `cmd/rig`'s dispatch switch. An
// analyzer would need a second list of the verbs, which is a source of truth
// that rots exactly like the citation it is checking - the defect again rather
// than a fix for it. A test in this package reads the switch itself.
//
// THE SWITCH IS THE SOURCE OF TRUTH AND IT IS PARSED, NOT LISTED. Nothing here
// enumerates a verb. Adding one to `run` adds it here; removing one removes
// it. `complete.go` already calls `verbVersion` "one string in three places
// that MUST agree", and a fourth hand-maintained place is that bet lost again.
//
// TWO CARRIERS, BOTH PRECISE, AND THE PROSE ONE IS NOT THE INTERESTING HALF:
//
//	prose    a markdown-backticked `rig <verb>` inside any string literal
//	command  a FixCommand field, or `with`'s fourth argument, beginning "rig "
//
// WHAT IS DELIBERATELY OUT, EACH FOR A MEASURED REASON. A guard whose blind
// spots are written down beats one whose blind spots are discovered.
//
//   - COMMENTS. Five comments cite `rig peers run --lease=NAME`, a forward
//     reference to the M7 peers service, and every one is legitimate prose. A
//     comment is not an `ast.BasicLit`, so parsing rather than grepping draws
//     that line for free.
//   - A BARE LITERAL BEGINNING "rig ". Measured: "rig is a daemon and a
//     client", "rig remembers departures", "rig could not build", "rig is not
//     reachable". Those are sentences, not commands, and telling them apart by
//     shape is a heuristic this test refuses to carry. The two carriers above
//     are positions, not guesses.
//   - A VERB SPLIT ACROSS A CONCATENATION JOIN, or reached through a variable.
//     `"Run \x60rig " + "peers\x60"` is invisible here. Adjacent literal runs
//     ARE joined, which is what catches the real defect - its backticked token
//     sat inside one fragment of a chain that also concatenated `held.Seat` -
//     but constant folding through a variable is more machinery than the
//     defect, which is the analyzer argument over again.
//   - TEST FILES. Their strings reach no caller, and a case written to prove
//     this guard bites would otherwise trip it.
func TestNoRefusalCitesAVerbRigDoesNotDispatch(t *testing.T) {
	dispatched := dispatchedVerbs(t)

	var cited []citation
	files := 0
	for _, root := range []string{
		filepath.Join("..", "..", "internal"),
		filepath.Join("..", "..", "cmd"),
	} {
		c, n := citationsUnder(t, root)
		cited = append(cited, c...)
		files += n
	}

	// THE LIVE SET IS EMPTY TODAY AND THAT IS PRECISELY WHY THIS BLOCK EXISTS.
	// A walk that read NOTHING reports exactly what a walk that FOUND nothing
	// reports, and only the second is an answer. This repository has paid for
	// that class repeatedly - a size check that measured a leftover binary and
	// printed clean for five hours, a mutation whose sed never applied and
	// reported green. A check that cannot tell "nothing is wrong" from "the
	// check did not run" reads as a clean result.
	if files < 40 {
		t.Fatalf("the walk parsed %d non-test Go files under internal/ and cmd/, "+
			"which is too few to be this tree: it cannot report a citation it "+
			"never read", files)
	}

	// ONE DEFECT IS REPORTED ONCE. A citation inside a `+` chain is reached
	// four ways - as its own literal, and through each enclosing BinaryExpr -
	// and four copies of one finding is a failure report nobody reads to the
	// end. A dead verb in a file is one defect, so that is the key.
	seen := map[string]bool{}
	for _, c := range cited {
		if dispatched[c.verb] {
			continue
		}
		key := c.verb + "\x00" + strings.SplitN(c.pos, ":", 2)[0]
		if seen[key] {
			continue
		}
		seen[key] = true
		t.Errorf("%s: %s cites `rig %s`, and run's switch does not dispatch %q.\n"+
			"  the text: %s\n"+
			"  dispatched: %s\n"+
			"An unmatched word falls through to the default branch, which reads it "+
			"as a PROGRAM name - so the caller is told its program does not exist "+
			"and the failure reads as its own fault rather than this message's. If "+
			"the word is a program rather than a verb, it is still wrong here: cite "+
			"a command the caller can run, or none at all.",
			c.pos, c.carrier, c.verb, c.verb, strconv.Quote(c.text),
			strings.Join(sortedKeys(dispatched), " "))
	}
}

// THIS IS THE OTHER HALF AND IT IS NOT OPTIONAL. The guard above has an EMPTY
// live set - there is no backticked `rig <verb>` in any string literal in the
// tree - so it passes today whether it works or not. The only evidence that it
// still bites is running it against the exact bytes that were wrong.
func TestTheDeadVerbGuardStillBitesTheStringItWasBuiltFor(t *testing.T) {
	dispatched := dispatchedVerbs(t)

	// Verbatim from 7ee7233's diff, INCLUDING the concatenation, because the
	// literal in the tree was split across two operands and a scanner reading
	// one literal at a time is a different scanner from this one.
	const deleted = ". Run `rig peers` to see the roster, and announce " +
		"without a seat if you are not taking this one"

	if got := backtickedIn(deleted); !slices.Equal(got, []string{"peers"}) {
		t.Fatalf("the extractor read %v out of the refusal deleted at 7ee7233, "+
			"want [peers]: it can no longer see the string it exists for", got)
	}

	// ⛔ `peers` IS DISPATCHED NOW, AND THAT IS WHY THE RED INPUT BELOW IS A
	// SENTINEL RATHER THAN A PLAUSIBLE VERB.
	//
	// This self-check used to assert !dispatched["peers"], which was the exact
	// defect 7ee7233 deleted. B41 landed the verb and the assertion inverted:
	// the check went red and told its reader it could no longer prove anything.
	// That is the check working, and it is also the lesson - a known-red input
	// chosen from the real world expires the day the real world fixes it, and a
	// self-check that expires silently is the thing this whole file exists to
	// prevent.
	//
	// So the dispatch arm is pointed at a word that CANNOT become a verb. The
	// extractor arm above keeps the historical bytes, because what it proves -
	// that the scanner still reads a citation split across two operands - is a
	// fact about the scanner and does not rot when a verb lands.
	if !dispatched["peers"] {
		t.Error("`peers` is not dispatched, so the refusal deleted at 7ee7233 " +
			"would be a live defect again. B41 landed this verb; if it has been " +
			"removed, the citation it justified has to go with it.")
	}
	const neverAVerb = "verb-that-rig-will-never-have"
	if got := backtickedIn("run `rig " + neverAVerb + "` to fix it"); !slices.Equal(got, []string{neverAVerb}) {
		t.Fatalf("the extractor read %v out of a citation of a dead verb, want [%s]", got, neverAVerb)
	}
	if dispatched[neverAVerb] {
		t.Fatalf("%q is dispatched, which was supposed to be impossible. Pick "+
			"another sentinel - this arm is the only thing proving the guard "+
			"can still go red.", neverAVerb)
	}

	// THE COMMAND CARRIER, self-checked the same way. `rig peers` as a
	// FixCommand is the shape internal/kernel/refusal.go calls "a defect and
	// not a hint", so the guard must read it out of that position too.
	if got := commandVerb("rig peers"); got != "peers" {
		t.Fatalf("commandVerb read %q out of a dead fix command, want peers", got)
	}

	// TWO POSITIVE CONTROLS, so this is not a guard that fires on everything.
	if got := backtickedIn("`rig apps` lists programs, not peers"); !slices.Equal(got, []string{"apps"}) {
		t.Fatalf("the extractor read %v out of a VALID citation, want [apps]", got)
	}
	if got := commandVerb("rig describe " + "fakeapp"); got != "describe" {
		t.Fatalf("commandVerb read %q out of a valid fix command, want describe", got)
	}
	for _, v := range []string{"apps", "describe"} {
		if !dispatched[v] {
			t.Fatalf("%q is not dispatched, so either the switch parser is broken "+
				"or this control is stale", v)
		}
	}

	// AND A NEGATIVE CONTROL ON THE SHAPE THIS TEST REFUSES TO GUESS AT: prose
	// beginning "rig " is not a command and must yield nothing. Four such
	// sentences are in the tree.
	if got := commandVerb("rig is a daemon and a client"); got != "is" {
		t.Fatalf("commandVerb read %q; it reads the second word and nothing "+
			"cleverer, which is why prose is only ever reached through the two "+
			"carrier positions and never by scanning every literal", got)
	}
}

// citation is one place a verb is named in something a caller is shown.
type citation struct {
	verb    string
	pos     string // file:line
	carrier string // "a backticked citation" or "a fix command"
	text    string // the decoded string, for the failure message
}

// citedVerb matches a markdown-backticked citation and captures the FIRST word
// after "rig ", which is the token the dispatch switch matches on.
//
// THE BACKTICK IS LOAD-BEARING AND NOT DECORATION. Without it this matches
// `internal/mcpserver`'s preamble - a raw string beginning "rig is a
// coordination daemon" - and reports a verb called `is`. Backticks are how
// this tree writes a command it wants somebody to run, and a raw string's own
// delimiters are not part of its decoded text, so the two cannot be confused.
//
// A LEADING FLAG IS SKIPPED BY CONSTRUCTION: `[a-z]` does not match `-`, so
// "rig --json apps" yields nothing rather than a verb called `--json`. That is
// a gap in the SAFE direction - a citation this misses is one nobody is warned
// about, never a false accusation.
var citedVerb = regexp.MustCompile("`rig ([a-z][a-z0-9_-]*)")

func backtickedIn(decoded string) []string {
	var out []string
	for _, m := range citedVerb.FindAllStringSubmatch(decoded, -1) {
		out = append(out, m[1])
	}
	return out
}

// commandVerb reads the verb out of a string that is meant to be RUN. It reads
// the second word and nothing cleverer, which is safe only because it is
// reached from the two carrier positions and never by scanning every literal.
func commandVerb(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) < 2 || fields[0] != "rig" {
		return ""
	}
	return fields[1]
}

// citationsUnder parses every non-test Go file under root and returns what it
// found, plus the number of files it actually parsed - so a caller can tell a
// clean walk from one that never started.
func citationsUnder(t *testing.T, root string) ([]citation, int) {
	t.Helper()

	fset := token.NewFileSet()
	var found []citation
	files := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files++

		at := func(p token.Pos) string { return fset.Position(p).String() }

		ast.Inspect(f, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.BinaryExpr:
				// A MESSAGE SPLIT ACROSS `+` IS ONE STRING TO THE CALLER, AND
				// THE DEFECT LITERAL WAS EXACTLY THAT SHAPE. Adjacent literal
				// runs are joined so a citation straddling a join is seen; the
				// operands are scanned individually below as well, and a
				// duplicate finding is harmless where a missed one is not.
				if e.Op == token.ADD {
					for _, run := range literalRuns(e) {
						for _, v := range backtickedIn(run) {
							found = append(found, citation{v, at(e.Pos()), "a backticked citation", run})
						}
					}
				}

			case *ast.BasicLit:
				if e.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(e.Value)
				if err != nil {
					return true
				}
				for _, v := range backtickedIn(s) {
					found = append(found, citation{v, at(e.Pos()), "a backticked citation", s})
				}

			case *ast.KeyValueExpr:
				// `FixCommand: "rig apps list"` - a POSITION, not a guess.
				if k, ok := e.Key.(*ast.Ident); ok && k.Name == "FixCommand" {
					if v := commandVerb(leadingLiteral(e.Value)); v != "" {
						found = append(found, citation{v, at(e.Pos()), "a fix command", leadingLiteral(e.Value)})
					}
				}

			case *ast.CallExpr:
				// kernel's `with(precondition, actual, fix, fixCommand)`. The
				// fourth argument is the runnable one; refusal.go:46 is where
				// that contract is written.
				sel, ok := e.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "with" || len(e.Args) != 4 {
					return true
				}
				if v := commandVerb(leadingLiteral(e.Args[3])); v != "" {
					found = append(found, citation{v, at(e.Args[3].Pos()), "a fix command", leadingLiteral(e.Args[3])})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return found, files
}

// leadingLiteral returns the literal text an expression BEGINS with, so
// `"rig describe " + program` yields "rig describe ". The verb is always in
// the leading fragment of a fix command; what follows it is the argument.
func leadingLiteral(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.BinaryExpr:
			if x.Op != token.ADD {
				return ""
			}
			e = x.X
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				return ""
			}
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				return ""
			}
			return s
		default:
			return ""
		}
	}
}

// literalRuns flattens a `+` chain into the maximal runs of ADJACENT string
// literals, breaking at anything else. A run is a string the caller really
// sees end to end; joining ACROSS a variable would invent one nobody is shown.
func literalRuns(e *ast.BinaryExpr) []string {
	var parts []string // "" marks a break
	var walk func(ast.Expr)
	walk = func(n ast.Expr) {
		switch x := n.(type) {
		case *ast.ParenExpr:
			walk(x.X)
		case *ast.BinaryExpr:
			if x.Op != token.ADD {
				parts = append(parts, "")
				return
			}
			walk(x.X)
			walk(x.Y)
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				parts = append(parts, "")
				return
			}
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				parts = append(parts, "")
				return
			}
			parts = append(parts, s)
		default:
			parts = append(parts, "")
		}
	}
	walk(e)

	var runs []string
	var cur strings.Builder
	for _, p := range parts {
		if p == "" {
			if cur.Len() > 0 {
				runs = append(runs, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteString(p)
	}
	if cur.Len() > 0 {
		runs = append(runs, cur.String())
	}
	return runs
}

// dispatchedVerbs parses `run`'s switch in THIS package and returns every verb
// it matches. Nothing here is enumerated, on purpose - see the file comment.
func dispatchedVerbs(t *testing.T) map[string]bool {
	t.Helper()

	// `parser.ParseDir` would be the obvious call and it is DEPRECATED since
	// Go 1.25 - staticcheck fails the build on it. Its replacement is
	// `x/tools/go/packages`, which is not in go.mod and would be a new direct
	// dependency for one directory read. So this reads the directory itself,
	// which is what `citationsUnder` above already does: one technique in this
	// file rather than two, and no dependency.
	//
	// ParseDir's stated defect is that it ignores build tags. This inherits
	// that, and `cmd/rig` carries no build tag on any non-test file - checked,
	// not assumed. If one ever arrives, the anchor assertions below are what
	// go red.
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd/rig: %v", err)
	}
	var parsed []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if f.Name.Name != "main" {
			t.Fatalf("%s is package %s, not main: this test is in the wrong place",
				name, f.Name.Name)
		}
		parsed = append(parsed, f)
	}
	if len(parsed) == 0 {
		t.Fatal("no non-test Go files in cmd/rig: the directory read did not run")
	}

	// A case can name a CONSTANT rather than a literal - `case verbVersion:` -
	// so the package's string consts are resolved first.
	consts := map[string]string{}
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok || len(vs.Names) != len(vs.Values) {
				return true
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if s, err := strconv.Unquote(lit.Value); err == nil {
					consts[name.Name] = s
				}
			}
			return true
		})
	}

	verbs := map[string]bool{}
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "run" || fn.Recv != nil {
				return true
			}
			ast.Inspect(fn.Body, func(m ast.Node) bool {
				sw, ok := m.(*ast.SwitchStmt)
				if !ok || !isArgsZero(sw.Tag) {
					return true
				}
				for _, stmt := range sw.Body.List {
					cc, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					for _, expr := range cc.List {
						switch x := expr.(type) {
						case *ast.BasicLit:
							if s, err := strconv.Unquote(x.Value); err == nil {
								verbs[s] = true
							}
						case *ast.Ident:
							if s, ok := consts[x.Name]; ok {
								verbs[s] = true
							}
						}
					}
				}
				return true
			})
			return true
		})
	}

	// THE SILENT-ZERO TRAP AGAIN, ONE LEVEL IN. An empty map makes every
	// citation look undispatched, which at least fails loudly - but a map
	// missing ONE verb is a quiet false accusation, and a switch this test
	// failed to find at all would make the guard pass over nothing. The
	// anchors are verbs this CLI cannot lose without a specification change.
	if len(verbs) < 5 {
		t.Fatalf("parsed only %d verbs out of run's switch (%v): the switch moved "+
			"and this test is reading the wrong thing", len(verbs), sortedKeys(verbs))
	}
	for _, anchor := range []string{"apps", "estate", "describe", "version"} {
		if !verbs[anchor] {
			t.Fatalf("run's switch does not appear to dispatch %q, and it must. "+
				"Parsed: %v", anchor, sortedKeys(verbs))
		}
	}
	return verbs
}

// isArgsZero reports whether a switch tag is `args[0]`, which is the one
// dispatch switch in `run`. Pinning it stops a second switch added inside run
// for something else from quietly widening the set of verbs believed in here.
func isArgsZero(tag ast.Expr) bool {
	ix, ok := tag.(*ast.IndexExpr)
	if !ok {
		return false
	}
	id, ok := ix.X.(*ast.Ident)
	if !ok || id.Name != "args" {
		return false
	}
	lit, ok := ix.Index.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "0"
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
