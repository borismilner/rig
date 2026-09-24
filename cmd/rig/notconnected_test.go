package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// B43. A WORD rig CANNOT RESOLVE AS A PROGRAM HAS TWO READINGS AND THE REFUSAL
// USED TO SERVE ONE OF THEM AS THOUGH IT WERE THE ONLY ONE.
//
// `rig mcp` on a build predating the verb falls through run()'s dispatch switch
// to the default branch, which reads the first token as a program name by
// design. "no program \"mcp\" is connected" is then TRUE and sends the reader to
// the daemon, the registry and its own spelling, none of which is where the
// answer is.
//
// WHY THIS IS A TEST AND NOT A DETECTION. The binary that needs the message is
// the one that does not have the verb, so it cannot recognise one; and a matched
// old rig and old rigd have no skew to compare, so no amount of skew detection
// reaches it either. The fix is to stop closing the reading rig cannot see, and
// what needs guarding is that all three sites keep doing so.
func TestEveryNotConnectedRefusalOffersTheSecondReading(t *testing.T) {
	// The zero-programs case is the one B43 was filed from: it is what an
	// operator on an old build actually sees.
	none := startFakeDaemon(t, &rigv1.ProgramsResponse{})
	// And the case where programs ARE connected, because the word is no less
	// likely to have been a subcommand just because something else is running.
	some := startFakeDaemon(t, &rigv1.ProgramsResponse{
		Programs: []*rigv1.Program{{Identity: &rigv1.Identity{Id: "ledger"}}},
	})

	for _, c := range []struct {
		name   string
		daemon *fakeDaemon
	}{
		{"nothing connected", none},
		{"something else connected", some},
	} {
		t.Run(c.name, func(t *testing.T) {
			conn, err := client.Dial(c.daemon.socket)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err = lookupProgram(ctx, conn, "mcp")
			if err == nil {
				t.Fatal("a word the daemon never listed was accepted as a program")
			}
			text := errorText(err)
			for _, want := range []string{"mcp", "subcommand", "rig help"} {
				if !strings.Contains(text, want) {
					t.Errorf("the refusal does not carry %q, so a caller on a "+
						"build older than the verb it typed is sent to look at "+
						"the daemon for a word that was never a program:\n%s",
						want, text)
				}
			}
		})
	}
}

// THE SITES ARE NOT ENUMERATED HERE, THEY ARE FOUND - AND THE GRANULARITY IS
// THE REFUSAL, NOT THE FUNCTION.
//
// A list of the sites would rot the first time a fourth is added, which is the
// defect this repository keeps paying for and the same argument
// refusal_verbs_test.go makes for parsing the dispatch switch.
//
// ⛔ THE FIRST VERSION OF THIS TEST CHECKED PER FUNCTION AND WAS USELESS FOR THE
// CASE IT MATTERED MOST. lookupProgram builds TWO refusals. Asking "does this
// FUNCTION mention maybeASubcommand" is answered yes by either one of them, so
// deleting the clause from one branch left the test green. Measured, not
// reasoned: the mutation was run and it passed.
//
// So the unit is the individual refusal EXPRESSION - one fmt.Errorf call, one
// jsonStatus literal. Each node that carries an "is connected" string must
// carry the second reading inside that same node.
func TestNoNotConnectedRefusalIsBuiltWithoutTheSecondReading(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	var checked []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			// One refusal is one expression: a call (fmt.Errorf) or a
			// composite literal (jsonStatus).
			switch n.(type) {
			case *ast.CallExpr, *ast.CompositeLit:
			default:
				return true
			}
			if !subtreeHasConnectedLiteral(n) {
				return true
			}
			// ONE REFUSAL IS THE OUTERMOST EXPRESSION THAT CARRIES IT, and
			// descent stops here. call.go's refusal is local(jsonStatus{...}):
			// the inner fmt.Sprintf holds the "is connected" string and the
			// sibling Fix field holds the clause, so a walk that kept
			// descending would report the inner call as a bare violation while
			// the refusal around it is correct. Measured: it did.
			where := name + ":" + strconv.Itoa(fset.Position(n.Pos()).Line)
			checked = append(checked, where)
			if !subtreeMentions(n, "maybeASubcommand") {
				t.Errorf("%s builds an \"is connected\" refusal and does not "+
					"carry maybeASubcommand IN THAT SAME EXPRESSION, so it "+
					"states the program reading as though it were the only "+
					"one. That is B43 arriving again at a site nobody was "+
					"watching. A sibling branch carrying the clause does not "+
					"cover this one", where)
			}
			return false
		})
	}

	// A WALK THAT FINDS NOTHING PASSES EVERY ASSERTION ABOVE. The message was
	// reworded or moved, and this test would go green reporting nothing.
	if len(checked) == 0 {
		t.Fatal("this walk found no \"is connected\" refusal anywhere in " +
			"cmd/rig, so it proved nothing. Either the wording moved and this " +
			"test must follow it, or the directory read did not run")
	}
	t.Logf("refusal sites carrying the second reading: %v", checked)
}

// subtreeHasConnectedLiteral reports whether any string literal under n is one
// of the not-connected refusals. It deliberately does not look at idents: a
// node that merely CALLS something which refuses is not itself the refusal.
func subtreeHasConnectedLiteral(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(m ast.Node) bool {
		lit, ok := m.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil &&
			strings.Contains(s, "is connected") {
			found = true
		}
		return true
	})
	return found
}

func subtreeMentions(n ast.Node, ident string) bool {
	found := false
	ast.Inspect(n, func(m ast.Node) bool {
		if id, ok := m.(*ast.Ident); ok && id.Name == ident {
			found = true
		}
		return true
	})
	return found
}
