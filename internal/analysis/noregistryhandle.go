package analysis

import (
	"go/ast"
	"path/filepath"
	"strings"
)

// KernelDir is the package path prefix that is the kernel.
//
// It does not exist yet: the registry is M1 (PLAN.md section 23). The
// analyzer ships now because the rule it enforces is the reason section 14
// was rewritten, and a rule with no gate behind it is the failure mode that
// section said it was correcting.
const KernelDir = "internal/kernel"

// handleTypes are the names of the registry handle.
//
// Section 14: "Every registry read is a method on a principal, the principal
// is in the type, and an unscoped read is unrepresentable rather than
// discouraged." Unrepresentable means the handle itself never leaves the
// kernel - what leaves is a principal-scoped view.
var handleTypes = map[string]bool{
	"Registry":       true,
	"RegistryHandle": true,
}

// NoRegistryHandle is section 14's rule as a gate: no registry handle outside
// the kernel.
//
// Three checks:
//
//  1. A package outside the kernel names the kernel's handle type.
//  2. A package outside the kernel declares a registry handle of its own,
//     which is the same leak by a second route.
//  3. The kernel exports a function or method that returns the handle. An
//     exported accessor is how the handle gets out in the first place, and
//     section 14 bans the whole class rather than an allowlist of views.
var NoRegistryHandle = Analyzer{
	Name: "noregistryhandle",
	Doc:  "no registry handle outside the kernel (PLAN.md section 14)",
	Go: func(p *Pass, f *GoFile) {
		if !Product(f) {
			return
		}
		if inKernel(f) {
			checkKernelAccessors(p, f)
			return
		}
		checkForeignDecl(p, f)
		checkKernelSelector(p, f)
	},
	Whole: func(p *Pass) {
		for _, f := range p.GoAll {
			if inKernel(f) {
				return
			}
		}
		p.Notef("no %s package in this tree, so nothing was checked. "+
			"The registry lands at M1 (PLAN.md section 23) and this gate "+
			"cannot fail until it does", KernelDir)
	},
}

func inKernel(f *GoFile) bool {
	return under(filepath.ToSlash(f.Path), KernelDir)
}

// checkForeignDecl catches a package outside the kernel declaring its own
// registry handle.
func checkForeignDecl(p *Pass, f *GoFile) {
	for _, d := range f.Syntax.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || !handleTypes[ts.Name.Name] {
				continue
			}
			p.Reportf(ts.Pos(),
				"package %s declares the registry handle %s: the handle lives in "+
					"%s and never leaves it, because an unscoped read has to be "+
					"unrepresentable rather than discouraged",
				f.Pkg, ts.Name.Name, KernelDir)
		}
	}
}

// checkKernelSelector catches a package outside the kernel naming the
// kernel's handle type through its import.
func checkKernelSelector(p *Pass, f *GoFile) {
	kernelAliases := map[string]string{}
	for _, imp := range f.Syntax.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !strings.Contains(path, KernelDir) {
			continue
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		kernelAliases[name] = path
	}
	if len(kernelAliases) == 0 {
		return
	}
	ast.Inspect(f.Syntax, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !handleTypes[sel.Sel.Name] {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if path, ok := kernelAliases[id.Name]; ok {
			p.Reportf(sel.Pos(),
				"%s.%s names the registry handle from %s: reach the registry "+
					"through a principal-scoped view, which is a method on the "+
					"principal and carries the scope in its type",
				id.Name, sel.Sel.Name, path)
		}
		return true
	})
}

// checkKernelAccessors catches the kernel handing the handle out.
func checkKernelAccessors(p *Pass, f *GoFile) {
	InFunc(f, func(name string, fd *ast.FuncDecl) {
		if !fd.Name.IsExported() || fd.Type.Results == nil {
			return
		}
		for _, r := range fd.Type.Results.List {
			if t, ok := handleResult(r.Type); ok {
				p.Reportf(r.Pos(),
					"exported %s returns the registry handle %s: an exported accessor "+
						"is how the handle leaves the kernel, and section 14 bans the "+
						"class rather than filtering a list of views",
					name, t)
			}
		}
	})
}

func handleResult(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.StarExpr:
		return handleResult(x.X)
	case *ast.Ident:
		if handleTypes[x.Name] {
			return x.Name, true
		}
	case *ast.SelectorExpr:
		if handleTypes[x.Sel.Name] {
			return x.Sel.Name, true
		}
	}
	return "", false
}
