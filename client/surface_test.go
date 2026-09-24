package client_test

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/borismilner/rig/client"
)

// The stub's public surface does not grow without a decision recorded in the
// plan (PLAN.md section 3). This is that assertion: every exported symbol in
// the package, compared against the enumeration in surface.go.
//
// It reads the source rather than using reflection, because reflection cannot
// see a type that is exported and never instantiated, and it is the export
// that costs, not the use.
func TestThePublicSurfaceIsTheOneThatWasDecided(t *testing.T) {
	got := exported(t, ".")
	want := append([]string(nil), client.Surface...)
	slices.Sort(want)
	slices.Sort(got)

	if slices.Equal(got, want) {
		return
	}
	var added, gone []string
	for _, s := range got {
		if !slices.Contains(want, s) {
			added = append(added, s)
		}
	}
	for _, s := range want {
		if !slices.Contains(got, s) {
			gone = append(gone, s)
		}
	}
	t.Fatalf("the client stub's public surface changed.\n"+
		"  added:   %v\n  removed: %v\n"+
		"Section 3: every symbol here is a future rebuild that cannot be "+
		"avoided, so a change costs a decision recorded in PLAN.md, not an "+
		"edit to client.Surface. A symbol appearing in BOTH lists is the "+
		"same name with a different shape, which breaks a linking program "+
		"exactly as surely as deleting it.", added, gone)
}

// exported lists every exported symbol a linking program can name, WITH the
// shape it names it at: top-level declarations, methods on exported types,
// and the exported fields of exported structs.
//
// THE NAME IS NOT THE CONTRACT, and rendering the shape is the whole point of
// this function. A program links Client.Call(ctx, method, in, out) error;
// change it to return (proto.Message, error) instead and every program that
// links the stub stops compiling, while the name Client.Call is still there
// and a surface check that compared names would pass. The same holds for
// CallError's fields: cmd/rig builds one by naming Method and Status, so they
// are as much the contract as any method is.
//
// PARAMETER NAMES ARE DELIBERATELY NOT RENDERED. Go has no named arguments,
// so renaming a parameter breaks nobody, and locking one would spend a
// recorded decision on an edit that costs nothing. What is rendered is the
// types, in the form the source writes them, which is what a caller must
// satisfy.
func exported(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
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
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if !x.Name.IsExported() {
					continue
				}
				if x.Recv == nil {
					out = append(out, x.Name.Name+signature(fset, x.Type))
					continue
				}
				if recv := receiver(x); recv != "" {
					out = append(out,
						recv+"."+x.Name.Name+signature(fset, x.Type))
				}
			case *ast.GenDecl:
				out = append(out, exportedSpecs(fset, x)...)
			}
		}
	}
	return out
}

func exportedSpecs(fset *token.FileSet, g *ast.GenDecl) []string {
	var out []string
	for _, spec := range g.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			out = append(out, s.Name.Name+kind(fset, s.Type))
			out = append(out, fields(fset, s.Name.Name, s.Type)...)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				// Surface is the enumeration, not a member of it.
				if n.IsExported() && n.Name != "Surface" {
					out = append(out, n.Name)
				}
			}
		}
	}
	return out
}

// kind renders what a type IS, not only that it exists. Turning a struct into
// an interface, or changing what a func type takes, breaks every linking
// program while the name stays exactly where it was.
func kind(fset *token.FileSet, e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StructType:
		return " struct"
	case *ast.InterfaceType:
		return " interface"
	case *ast.FuncType:
		return " func" + signature(fset, t)
	default:
		return " " + render(fset, e)
	}
}

// fields lists the exported fields of an exported struct.
//
// An exported field is a symbol a linking program names directly, and nothing
// was checking them: cmd/rig constructs client.CallError{Method: ..., Status:
// ...} today, so retyping or renaming either one is a breaking change that
// the name-only check waved through.
func fields(fset *token.FileSet, owner string, e ast.Expr) []string {
	st, ok := e.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}
	var out []string
	for _, f := range st.Fields.List {
		typ := render(fset, f.Type)
		if len(f.Names) == 0 {
			// An embedded field is named by its own type, and promotes
			// everything that type carries, so it is the widest kind of
			// export there is.
			if name := embedded(typ); token.IsExported(name) {
				out = append(out, owner+"."+name+" "+typ)
			}
			continue
		}
		for _, n := range f.Names {
			if n.IsExported() {
				out = append(out, owner+"."+n.Name+" "+typ)
			}
		}
	}
	return out
}

// embedded is the field name Go gives an embedded type: the type's own name,
// without the pointer star or the package qualifier.
func embedded(typ string) string {
	typ = strings.TrimPrefix(typ, "*")
	if i := strings.LastIndex(typ, "."); i >= 0 {
		typ = typ[i+1:]
	}
	return typ
}

// signature renders a function's shape, parameter names stripped.
func signature(fset *token.FileSet, ft *ast.FuncType) string {
	in := "(" + strings.Join(paramTypes(fset, ft.Params), ", ") + ")"
	res := paramTypes(fset, ft.Results)
	switch len(res) {
	case 0:
		return in
	case 1:
		return in + " " + res[0]
	default:
		return in + " (" + strings.Join(res, ", ") + ")"
	}
}

// paramTypes renders one field list's types, once per name, so that the
// grouped form `in, out proto.Message` is two parameters rather than one.
func paramTypes(fset *token.FileSet, fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var out []string
	for _, f := range fl.List {
		typ := render(fset, f.Type)
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			out = append(out, typ)
		}
	}
	return out
}

// render prints one type expression in the form the source wrote it.
func render(fset *token.FileSet, e ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, e); err != nil {
		// Returning the error as the rendering makes an unprintable type a
		// visible diff rather than a silently empty entry that matches
		// nothing and is blamed on the enumeration.
		return "unprintable: " + err.Error()
	}
	return b.String()
}

func receiver(f *ast.FuncDecl) string {
	if len(f.Recv.List) == 0 {
		return ""
	}
	t := f.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	id, ok := t.(*ast.Ident)
	if !ok || !id.IsExported() {
		return ""
	}
	return id.Name
}
