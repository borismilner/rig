package analysis

import (
	"go/ast"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// InHouse is every program rig serves, from PLAN.md section 25's migration
// order, plus the one it supersedes (section 2).
//
// The list is here rather than read out of PLAN.md because a lint rule that
// parses the plan fails the moment the plan is reworded, and because a name
// leaving this list is a decision worth seeing in a diff.
var InHouse = []string{
	"agentbox",
	"archi",
	"dedup",
	"devtool",
	"dispatch",
	"grabbit",
	"graft",
	"nudge",
	"romsort",
	"shelf",
	"sigs",
	"snapper",
}

// Reserved is rig's own namespace: the ids no program may take, which the
// daemon therefore has to name in order to refuse them (section 5f). The empty
// string is here too - testing for it is a presence check, not a branch on
// which program is calling.
var Reserved = map[string]bool{"": true, "rig": true, "rigd": true}

// identityName matches a variable or field that holds a program's identity.
// Comparing one of these against a literal is the shape of the bug even when
// the name on the right is not one rig knows yet.
var identityName = regexp.MustCompile(`(?i)(program|progid|appid|app_id)`)

// NoProgramID is section 29's rule as a gate: "rig does not run business
// logic. Ever. A program id appearing in rig's code is a bug."
//
// Two checks, because a name list alone only catches the programs that exist
// today:
//
//  1. A string literal naming an in-house program, whole or as the first
//     dotted segment of a method name.
//  2. A comparison or switch that tests a program-identity expression against
//     any string literal at all.
//
// The rule that makes this worth enforcing is section 5h's: a service or
// surface that needs to know a program's identity to work is not a service or
// surface, it is business logic in the wrong place.
var NoProgramID = Analyzer{
	Name: "noprogramid",
	Doc:  "no program id in rig code (PLAN.md sections 5h, 5i, 29)",
	Go: func(p *Pass, f *GoFile) {
		if !Product(f) {
			return
		}
		known := map[string]bool{}
		for _, n := range InHouse {
			known[n] = true
		}
		ast.Inspect(f.Syntax, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if name, ok := programLiteral(x, known); ok {
					p.Reportf(x.Pos(),
						"%q names the in-house program %q: rig runs no business logic, "+
							"so a program's identity reaches it through the registry, never through its source",
						litValue(x), name)
				}
			case *ast.BinaryExpr:
				checkIdentityCompare(p, x)
			case *ast.SwitchStmt:
				checkIdentitySwitch(p, x)
			}
			return true
		})
	},
}

func litValue(l *ast.BasicLit) string {
	v, err := strconv.Unquote(l.Value)
	if err != nil {
		return l.Value
	}
	return v
}

// programLiteral reports whether a string literal names an in-house program,
// either outright or as the first segment of a dotted method name such as
// "shelf.reindex". A literal that merely starts with the same word - an error
// prefix like "dispatch: %w" - is not a match, because it has no dot.
func programLiteral(l *ast.BasicLit, known map[string]bool) (string, bool) {
	if l.Kind.String() != "STRING" {
		return "", false
	}
	v := litValue(l)
	if known[v] {
		return v, true
	}
	if i := strings.Index(v, "."); i > 0 && known[v[:i]] {
		return v[:i], true
	}
	return "", false
}

func checkIdentityCompare(p *Pass, x *ast.BinaryExpr) {
	if x.Op.String() != "==" && x.Op.String() != "!=" {
		return
	}
	for _, pair := range [][2]ast.Expr{{x.X, x.Y}, {x.Y, x.X}} {
		if name, ok := identityExpr(pair[0]); ok {
			if lit, ok := pair[1].(*ast.BasicLit); ok && lit.Kind.String() == "STRING" {
				if Reserved[litValue(lit)] {
					return
				}
				p.Reportf(x.Pos(),
					"%s is compared against the literal %q: rig may route on a program's "+
						"identity but must never branch on which program it is",
					name, litValue(lit))
				return
			}
		}
	}
}

func checkIdentitySwitch(p *Pass, x *ast.SwitchStmt) {
	if x.Tag == nil {
		return
	}
	name, ok := identityExpr(x.Tag)
	if !ok {
		return
	}
	for _, stmt := range x.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, e := range cc.List {
			if lit, ok := e.(*ast.BasicLit); ok && lit.Kind.String() == "STRING" {
				if Reserved[litValue(lit)] {
					continue
				}
				p.Reportf(lit.Pos(),
					"a switch on %s has the case %q: rig may route on a program's "+
						"identity but must never branch on which program it is",
					name, litValue(lit))
			}
		}
	}
}

// identityExpr reports whether an expression names a program's identity, by
// the name of its last identifier: program, programID, f.Program, appID.
func identityExpr(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if identityName.MatchString(x.Name) {
			return x.Name, true
		}
	case *ast.SelectorExpr:
		if identityName.MatchString(x.Sel.Name) {
			return exprString(x), true
		}
	case *ast.CallExpr:
		// f.Program() and f.GetProgram() read as the field does.
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && len(x.Args) == 0 {
			if identityName.MatchString(sel.Sel.Name) {
				return exprString(sel) + "()", true
			}
		}
	case *ast.IndexExpr:
		return identityExpr(x.X)
	}
	return "", false
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	}
	return "the expression"
}

// KnownPrograms is InHouse, sorted, for a message that has to list them.
func KnownPrograms() []string {
	out := append([]string(nil), InHouse...)
	sort.Strings(out)
	return out
}
