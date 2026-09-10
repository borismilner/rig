package client_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/boris-milner/rig/client"
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
		"avoided, so growth costs a decision recorded in PLAN.md, not an "+
		"edit to client.Surface.", added, gone)
}

// exported lists every exported symbol a linking program can name: top-level
// declarations, and methods on exported types. Surface itself is the
// enumeration and is not part of what it enumerates.
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
					out = append(out, x.Name.Name)
					continue
				}
				if recv := receiver(x); recv != "" {
					out = append(out, recv+"."+x.Name.Name)
				}
			case *ast.GenDecl:
				out = append(out, exportedSpecs(x)...)
			}
		}
	}
	return out
}

func exportedSpecs(g *ast.GenDecl) []string {
	var out []string
	for _, spec := range g.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if s.Name.IsExported() {
				out = append(out, s.Name.Name)
			}
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
