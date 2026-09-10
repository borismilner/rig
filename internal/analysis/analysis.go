// Package analysis is the machinery behind rig's house analyzers.
//
// PLAN.md section 22 names three of them - no program id in rig code, no
// registry handle outside the kernel, no meaningful enum zero - and section 3
// adds a fourth, that no call is without a deadline. Each is a tiny main under
// cmd/, and this package is what they share: finding the files, parsing them,
// honouring suppression directives and reporting.
//
// # Why this walks the filesystem instead of loading packages
//
// go/packages would give type information, and would cost a dependency on
// golang.org/x/tools that section 22's stack table does not carry. It would
// also see one build configuration at a time, and section 5i requires these to
// run "under every tag set CI builds". A filesystem walk parses every file
// whatever its build tags, so one run covers every tag set rather than the
// default one. The price is that these checks are syntactic: they read names
// and literals, not types. Section 5i's layering analyzer needs more than that
// - it has to reject an untyped service locator - and it is an M1 item, so the
// question of paying for type information is deferred to it rather than
// answered here.
package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Analyzer is one house rule.
//
// A rule implements whichever hooks it needs. Go runs once per parsed Go file,
// Proto once per .proto file, and Whole once at the end for anything that
// needs to see the tree as a whole.
type Analyzer struct {
	Name  string
	Doc   string
	Go    func(*Pass, *GoFile)
	Proto func(*Pass, *ProtoFile)
	Whole func(*Pass)
}

// GoFile is one parsed Go source file.
type GoFile struct {
	Path      string // as given on the command line's root, cleaned
	Pkg       string // package clause
	Syntax    *ast.File
	Test      bool // _test.go, or package foo_test
	Generated bool // carries the "Code generated ... DO NOT EDIT." line
}

// ProtoFile is one .proto file, unparsed: the analyzers that read protos want
// the text, not a schema.
type ProtoFile struct {
	Path string
	Text string
}

// Pass is one run of one analyzer over one file set.
type Pass struct {
	Fset   *token.FileSet
	GoAll  []*GoFile
	Protos []*ProtoFile

	name  string
	diags []Diagnostic
	notes []string
}

// Diagnostic is one violation, at one place.
type Diagnostic struct {
	Pos token.Position
	Msg string
}

// Reportf records a violation at a parsed Go position.
func (p *Pass) Reportf(pos token.Pos, format string, args ...any) {
	p.diags = append(p.diags, Diagnostic{
		Pos: p.Fset.Position(pos),
		Msg: fmt.Sprintf(format, args...),
	})
}

// ReportAt records a violation at a place with no token.Pos behind it - a line
// in a .proto file, say.
func (p *Pass) ReportAt(file string, line int, format string, args ...any) {
	p.diags = append(p.diags, Diagnostic{
		Pos: token.Position{Filename: file, Line: line},
		Msg: fmt.Sprintf(format, args...),
	})
}

// Notef records something the reader should know that is not a failure.
//
// A gate that cannot fail is section 5i's complaint about the previous
// modularity rules, and an analyzer whose subject does not exist yet is
// exactly that. Saying so on stderr is the difference between a vacuous pass
// and a silent one.
func (p *Pass) Notef(format string, args ...any) {
	p.notes = append(p.notes, fmt.Sprintf(format, args...))
}

// directive matches a recorded exemption, written as a comment on the
// offending line or the line above it:
//
//	//rig:allow ANALYZER: REASON
//
// The reason is not optional. An exemption with no reason is the nod section
// 5i says a rule must cost more than, and a stale one is reported like a
// violation so an exemption cannot outlive what it excused. (The placeholders
// above are upper case so this comment is not itself a directive.)
var directive = regexp.MustCompile(`//\s*rig:allow\s+([a-z]+)\s*:\s*(\S.*)$`)

type allow struct {
	line   int
	name   string
	reason string
	used   bool
}

// Main is the whole of an analyzer's command: parse the arguments, load, run,
// report, exit.
func Main(a Analyzer) {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"./..."}
	}
	code, err := Run(a, roots, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", a.Name, err)
		os.Exit(2)
	}
	os.Exit(code)
}

// Run is Main without the exiting, so the analyzers are testable.
func Run(a Analyzer, roots []string, stdout, stderr io.Writer) (int, error) {
	p, allows, err := load(roots)
	if err != nil {
		return 0, err
	}
	p.name = a.Name

	if a.Go != nil {
		for _, f := range p.GoAll {
			a.Go(p, f)
		}
	}
	if a.Proto != nil {
		for _, f := range p.Protos {
			a.Proto(p, f)
		}
	}
	if a.Whole != nil {
		a.Whole(p)
	}

	kept := suppress(p.diags, allows, a.Name)

	for _, n := range p.notes {
		fmt.Fprintf(stderr, "%s: note: %s\n", a.Name, n)
	}
	stale := staleAllows(allows, a.Name, p.examined())
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].Pos.Filename != kept[j].Pos.Filename {
			return kept[i].Pos.Filename < kept[j].Pos.Filename
		}
		return kept[i].Pos.Line < kept[j].Pos.Line
	})
	for _, d := range kept {
		fmt.Fprintf(stdout, "%s:%d:%d: %s\n", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Msg)
	}
	for _, s := range stale {
		fmt.Fprintf(stdout, "%s: no %s violation here, so the exemption is stale: %s\n",
			s.where, a.Name, s.reason)
	}
	if n := len(kept) + len(stale); n > 0 {
		fmt.Fprintf(stderr, "%s: %d problem(s)\n", a.Name, n)
		return 1, nil
	}
	return 0, nil
}

type staleAllow struct {
	where  string
	reason string
}

// examined names the files an analyzer could have reported on, so a
// directive in a file it skips - a test, a generated file - is not called
// stale for excusing nothing.
func (p *Pass) examined() map[string]bool {
	skip := map[string]bool{}
	for _, f := range p.GoAll {
		if !Product(f) {
			skip[f.Path] = true
		}
	}
	return skip
}

func staleAllows(allows map[string][]*allow, name string, skip map[string]bool) []staleAllow {
	var out []staleAllow
	files := make([]string, 0, len(allows))
	for f := range allows {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		if skip[f] {
			continue
		}
		for _, al := range allows[f] {
			if al.name == name && !al.used {
				out = append(out, staleAllow{fmt.Sprintf("%s:%d", f, al.line), al.reason})
			}
		}
	}
	return out
}

// suppress drops the diagnostics a directive covers, and marks that directive
// used so a stale one can be reported.
func suppress(diags []Diagnostic, allows map[string][]*allow, name string) []Diagnostic {
	var kept []Diagnostic
	for _, d := range diags {
		covered := false
		for _, al := range allows[d.Pos.Filename] {
			if al.name != name {
				continue
			}
			// On the offending line, or on the line above it.
			if al.line == d.Pos.Line || al.line == d.Pos.Line-1 {
				al.used = true
				covered = true
			}
		}
		if !covered {
			kept = append(kept, d)
		}
	}
	return kept
}

var skipDir = map[string]bool{
	".git": true, "build": true, "dist": true,
	"node_modules": true, "vendor": true, "testdata": true,
}

func load(roots []string) (*Pass, map[string][]*allow, error) {
	p := &Pass{Fset: token.NewFileSet()}
	allows := map[string][]*allow{}
	seen := map[string]bool{}

	for _, r := range roots {
		dir := strings.TrimSuffix(strings.TrimSuffix(r, "..."), "/")
		if dir == "" {
			dir = "."
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				base := filepath.Base(path)
				if path != dir && (skipDir[base] || strings.HasPrefix(base, ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			path = filepath.Clean(path)
			if seen[path] {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go":
				seen[path] = true
				return p.addGo(path, allows)
			case ".proto":
				seen[path] = true
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				p.Protos = append(p.Protos, &ProtoFile{Path: path, Text: string(b)})
				collectAllows(path, string(b), allows)
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return p, allows, nil
}

var generatedLine = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

func (p *Pass) addGo(path string, allows map[string][]*allow) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(b)
	f, err := parser.ParseFile(p.Fset, path, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	gf := &GoFile{
		Path:   path,
		Pkg:    f.Name.Name,
		Syntax: f,
		Test: strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(f.Name.Name, "_test"),
	}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if generatedLine.MatchString(c.Text) {
				gf.Generated = true
			}
		}
	}
	p.GoAll = append(p.GoAll, gf)
	// From the comments, not from the lines: a fixture in a test can hold the
	// text of a directive inside a string literal, and a raw line scan reads
	// that as an exemption for a violation that is not there.
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if !strings.HasPrefix(c.Text, "//") {
				continue
			}
			m := directive.FindStringSubmatch(c.Text)
			if m == nil {
				continue
			}
			allows[path] = append(allows[path], &allow{
				line:   p.Fset.Position(c.Slash).Line,
				name:   m[1],
				reason: strings.TrimSpace(m[2]),
			})
		}
	}
	return nil
}

// collectAllows reads directives out of a .proto file, where a line scan is
// enough: a directive there is always a comment.
func collectAllows(path, src string, allows map[string][]*allow) {
	for i, line := range strings.Split(src, "\n") {
		m := directive.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		allows[path] = append(allows[path], &allow{
			line:   i + 1,
			name:   m[1],
			reason: strings.TrimSpace(m[2]),
		})
	}
}

// Product reports whether a file is rig's own hand-written code.
//
// Everything under internal/ and cmd/ counts, which is deliberately wider than
// the two shipped binaries: section 5i wants these rules applied to every
// module, and a benchmark or a build tool that hard-codes a program's identity
// is the same bug in a cheaper place. Three exclusions:
//
//   - Generated files. The generator is the thing to fix, not its output.
//   - This package. It has to name the rules it enforces, including the list
//     of programs, and cannot be subject to them.
//   - Tests. Section 20 has the pilot running the real shelf binary in CI, so
//     a test naming a program is the plan working, not a violation.
func Product(f *GoFile) bool {
	if f.Generated || f.Test {
		return false
	}
	p := filepath.ToSlash(f.Path)
	if under(p, "internal/analysis") {
		return false
	}
	return under(p, "internal") || under(p, "cmd")
}

// under reports whether a slash-separated path has dir as a whole segment
// prefix somewhere in it, so the answer does not depend on whether the walk
// started at the module root or above it.
func under(path, dir string) bool {
	return strings.HasPrefix(path, dir+"/") || strings.Contains(path, "/"+dir+"/")
}

// InFunc walks every function and method body in a file, naming each.
func InFunc(f *GoFile, fn func(name string, decl *ast.FuncDecl)) {
	for _, d := range f.Syntax.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		fn(fd.Name.Name, fd)
	}
}
