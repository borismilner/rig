package analysis

import (
	"go/ast"
	"go/token"
)

// NoContextFree is section 3's rule as a gate: "No call anywhere is without a
// deadline. Enforced by a lint rule, not by review."
//
// A deadline is only ever established once, at the entry point, and then
// flows. Three checks hold that shape:
//
//  1. A root context has to be given a deadline where it is made.
//     context.Background() is allowed only as the argument of WithTimeout,
//     WithDeadline or signal.NotifyContext - the last because a process
//     lifetime is not a call. context.TODO() is refused outright: it means
//     the decision has not been taken, which is the thing being banned.
//  2. A context.Context parameter comes first and is called ctx, so a
//     function that has one cannot quietly not use it.
//  3. context.WithCancel needs a recorded reason, because it derives a
//     context that has no deadline of its own.
//
// What this does NOT check, because it needs types and dataflow that a
// syntactic pass does not have: that the context reaching a particular
// outbound call is one of the deadlined ones. Rule 1 is what makes that
// likely - there is no undeadlined root to reach it from - not what proves it.
var NoContextFree = Analyzer{
	Name: "nocontextfree",
	Doc:  "no call without a deadline (PLAN.md sections 3, 20)",
	Go: func(p *Pass, f *GoFile) {
		if !Product(f) {
			return
		}
		deadlined := deadlinedRoots(f)
		ast.Inspect(f.Syntax, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				checkRootContext(p, x, deadlined)
			case *ast.FuncDecl:
				checkCtxParam(p, x.Name.Name, x.Type)
			case *ast.FuncLit:
				checkCtxParam(p, "the function literal", x.Type)
			}
			return true
		})
	},
}

// wrappers give a root context a deadline, or a lifetime that is not a call.
var wrappers = map[string]bool{
	"context.WithTimeout":   true,
	"context.WithDeadline":  true,
	"signal.NotifyContext":  true,
	"context.WithoutCancel": true,
}

// deadlinedRoots collects the positions of the root contexts that are handed
// straight to a wrapper, so the check below can report only the loose ones.
func deadlinedRoots(f *GoFile) map[token.Pos]bool {
	ok := map[token.Pos]bool{}
	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		call, is := n.(*ast.CallExpr)
		if !is || len(call.Args) == 0 || !wrappers[qualified(call.Fun)] {
			return true
		}
		if arg, is := call.Args[0].(*ast.CallExpr); is {
			ok[arg.Pos()] = true
		}
		return true
	})
	return ok
}

func checkRootContext(p *Pass, call *ast.CallExpr, deadlined map[token.Pos]bool) {
	switch qualified(call.Fun) {
	case "context.TODO":
		p.Reportf(call.Pos(),
			"context.TODO() says the deadline has not been decided, which is "+
				"the decision section 3 requires: take a context from the caller, "+
				"or give this one a deadline where it is made")
	case "context.Background":
		if !deadlined[call.Pos()] {
			p.Reportf(call.Pos(),
				"context.Background() is used without a deadline: wrap it in "+
					"context.WithTimeout or context.WithDeadline here, or take the "+
					"context from the caller so the deadline flows in")
		}
	case "context.WithCancel":
		p.Reportf(call.Pos(),
			"context.WithCancel derives a context with no deadline of its own: "+
				"use context.WithTimeout, or record why this one cannot have a "+
				"deadline with a //rig:allow nocontextfree comment")
	}
}

// checkCtxParam holds section 3's shape: a context is the first argument and
// is called ctx.
func checkCtxParam(p *Pass, name string, ft *ast.FuncType) {
	if ft.Params == nil {
		return
	}
	pos := 0
	for _, field := range ft.Params.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		if qualified(field.Type) == "context.Context" {
			if pos != 0 {
				p.Reportf(field.Pos(),
					"%s takes a context.Context at argument %d: it goes first, so a "+
						"caller cannot miss that this function has a deadline to honour",
					name, pos+1)
			}
			for _, id := range field.Names {
				if id.Name != "ctx" && id.Name != "_" {
					p.Reportf(id.Pos(),
						"%s names its context %q: call it ctx, so a shadowed or "+
							"ignored deadline is visible at the call site",
						name, id.Name)
				}
			}
		}
		pos += n
	}
}

// qualified renders pkg.Name for a selector and Name for an identifier, and
// the empty string for anything else.
func qualified(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name + "." + x.Sel.Name
		}
	}
	return ""
}
