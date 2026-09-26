package files

// The layout of the free files, plan/48 R28-R31 and R33: which kinds of file
// exist and where each goes under files/. It is one text file Boris edits
// (R30), never compiled into rig or a program.
//
// TWO COPIES, AND THE REASON IS RELAYOUT. layout.txt is the one he edits;
// layout.applied is the layout the files on disk are laid out by. files.place
// answers from the applied copy, so an edit changes nothing until relayout
// moves the files to match it; relayout then needs both, the old to find
// every file and the new to say where it goes.

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// The placeholders a place may use. Each fills one whole path segment.
const (
	VarSubject = "subject"
	VarType    = "type"
	VarProgram = "program"
)

// ProgramsKind is the one kind every layout must carry: files.root answers
// a program's own directory from it.
const ProgramsKind = "programs"

// Bounds on a layout file.
const (
	maxLayoutBytes = 64 << 10
	maxKinds       = 64
	maxSegments    = 8
	maxName        = 255
)

// DefaultLayout is written when there is no layout yet: R31 as Boris answered
// it.
const DefaultLayout = `# The layout of rig's free files (plan/48 R28-R31). Edit it, then run
# "rig files relayout" to move the files already written; until then rig
# keeps answering from the layout the files are laid out by.
#
# One kind per line: the kind, then where its files go under the free-files
# root, ending in "/". A place is directory names and placeholders, each
# placeholder a whole directory: {subject}, {type}, {program}. Each kind has its own top-level
# directory. The kind "programs" must be there and use {program}.

docs       docs/{subject}/
resources  resources/{type}/{subject}/
lessons    lessons/
programs   programs/{program}/
`

// Kind is one line of a layout.
type Kind struct {
	Name, Place string
	segs        []string // literal names, or "{var}"
}

func (k Kind) vars() []string {
	var out []string
	for _, s := range k.segs {
		if v, ok := placeholder(s); ok {
			out = append(out, v)
		}
	}
	return out
}

// Uses says the kind's place takes the named placeholder.
func (k Kind) Uses(v string) bool { return slices.Contains(k.vars(), v) }

func placeholder(seg string) (string, bool) {
	if len(seg) > 2 && seg[0] == '{' && seg[len(seg)-1] == '}' {
		return seg[1 : len(seg)-1], true
	}
	return "", false
}

// Layout is a parsed layout file.
type Layout struct {
	Kinds []Kind
	text  string
}

// Text is the layout as written.
func (l Layout) Text() string { return l.text }

// Kind is the named kind.
func (l Layout) Kind(name string) (Kind, bool) {
	for _, k := range l.Kinds {
		if k.Name == name {
			return k, true
		}
	}
	return Kind{}, false
}

func (l Layout) names() []string {
	out := make([]string, len(l.Kinds))
	for i, k := range l.Kinds {
		out[i] = k.Name
	}
	return out
}

// ParseLayout reads a layout file, refusing it whole on the first line it
// cannot use, with the line number.
func ParseLayout(text string) (Layout, error) {
	if len(text) > maxLayoutBytes {
		return Layout{}, invalid("a layout is at most %d bytes, got %d", maxLayoutBytes, len(text))
	}
	l := Layout{text: text}
	tops := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		k, err := parseKind(line)
		if err != nil {
			return Layout{}, invalid("layout line %d: %v", n, err)
		}
		if _, dup := l.Kind(k.Name); dup {
			return Layout{}, invalid("layout line %d: kind %q is named twice", n, k.Name)
		}
		if other, dup := tops[k.segs[0]]; dup {
			return Layout{}, invalid("layout line %d: kinds %q and %q share the top-level directory %q; "+
				"each kind has its own", n, other, k.Name, k.segs[0])
		}
		tops[k.segs[0]] = k.Name
		l.Kinds = append(l.Kinds, k)
		if len(l.Kinds) > maxKinds {
			return Layout{}, invalid("a layout has at most %d kinds", maxKinds)
		}
	}
	p, ok := l.Kind(ProgramsKind)
	if !ok || !slices.Equal(p.vars(), []string{VarProgram}) {
		return Layout{}, invalid("the layout must carry the kind %q with a place using {program} "+
			"and no other placeholder: a program's own directory comes from it", ProgramsKind)
	}
	return l, nil
}

func parseKind(line string) (Kind, error) {
	f := strings.Fields(line)
	if len(f) != 2 {
		return Kind{}, errors.New("a line is a kind and a place, two words")
	}
	if !namePattern.MatchString(f[0]) {
		return Kind{}, fmt.Errorf("kind %q is not a name: lowercase letters, digits, '.', '_' and '-'", f[0])
	}
	k := Kind{Name: f[0], Place: strings.Trim(f[1], "/")}
	k.segs = strings.Split(k.Place, "/")
	if len(k.segs) > maxSegments {
		return Kind{}, fmt.Errorf("place %q is deeper than %d directories", f[1], maxSegments)
	}
	seen := map[string]bool{}
	for i, s := range k.segs {
		v, isVar := placeholder(s)
		switch {
		case isVar && i == 0:
			return Kind{}, fmt.Errorf("place %q starts with a placeholder; its top-level directory is a name", f[1])
		case isVar && v != VarSubject && v != VarType && v != VarProgram:
			return Kind{}, fmt.Errorf("place %q uses {%s}; the placeholders are {subject}, {type} and {program}", f[1], v)
		case isVar && seen[v]:
			return Kind{}, fmt.Errorf("place %q uses {%s} twice", f[1], v)
		case isVar:
			seen[v] = true
		case !namePattern.MatchString(s):
			return Kind{}, fmt.Errorf("place %q has %q, which is neither a directory name "+
				"(lowercase letters, digits, '.', '_', '-') nor a placeholder", f[1], s)
		}
	}
	return k, nil
}

// Vars are the values a place's placeholders take.
type Vars struct{ Subject, Type, Program string }

func (v Vars) get(name string) string {
	switch name {
	case VarSubject:
		return v.Subject
	case VarType:
		return v.Type
	}
	return v.Program
}

// Dir is where a kind's files go for these values, relative to the area. A
// kind the layout does not name is refused with the kinds that exist (R33).
func (l Layout) Dir(kind string, v Vars) (string, error) {
	k, ok := l.Kind(kind)
	if !ok {
		return "", invalid("kind %q is not in the layout; the kinds are %s. Only Boris adds a kind, "+
			"by editing the layout", kind, strings.Join(l.names(), ", "))
	}
	var missing []string
	out := make([]string, len(k.segs))
	for i, s := range k.segs {
		name, isVar := placeholder(s)
		if !isVar {
			out[i] = s
			continue
		}
		val := v.get(name)
		switch {
		case val == "":
			missing = append(missing, name)
		case !namePattern.MatchString(val):
			return "", invalid("%s %q is not a name: lowercase letters, digits, '.', '_' and '-', "+
				"starting with a letter or digit, at most 64", name, val)
		}
		out[i] = val
	}
	if len(missing) > 0 {
		return "", invalid("kind %s goes to %s and needs %s", kind, k.Place, strings.Join(missing, " and "))
	}
	return strings.Join(out, "/"), nil
}

// Place is where a file of this kind and name goes, relative to the area.
// An empty name answers the directory.
func (l Layout) Place(kind string, v Vars, name string) (string, error) {
	dir, err := l.Dir(kind, v)
	if err != nil || name == "" {
		return dir, err
	}
	switch {
	case len(name) > maxName:
		return "", invalid("a file name is at most %d bytes", maxName)
	case name == "." || name == ".." || name == ".git" || strings.ContainsAny(name, "/\x00"):
		return "", invalid("file name %q is not one file name", name)
	}
	return path.Join(dir, name), nil
}

// match reads a path laid out by k: the placeholders' values, and the rest of
// the path under the kind's directory. ok is false for a path k does not lay
// out, such as a file sitting directly in docs/ when docs is docs/{subject}.
func (k Kind) match(rel string) (vals map[string]string, rest string, ok bool) {
	parts := strings.Split(rel, "/")
	if len(parts) <= len(k.segs) {
		return nil, "", false
	}
	vals = map[string]string{}
	for i, s := range k.segs {
		if name, isVar := placeholder(s); isVar {
			vals[name] = parts[i]
		} else if parts[i] != s {
			return nil, "", false
		}
	}
	return vals, strings.Join(parts[len(k.segs):], "/"), true
}

func (k Kind) render(vals map[string]string) string {
	out := make([]string, len(k.segs))
	for i, s := range k.segs {
		if name, isVar := placeholder(s); isVar {
			out[i] = vals[name]
		} else {
			out[i] = s
		}
	}
	return strings.Join(out, "/")
}

// Layouts holds the two copies of the layout in rig's internal area.
type Layouts struct {
	edited, applied string

	mu      sync.Mutex
	inForce Layout
}

// OpenLayouts reads the layout in dir, writing the default where there is
// none. The edited copy is recreated from the applied one if it was deleted.
func OpenLayouts(dir string) (*Layouts, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("files: creating %s: %w", dir, err)
	}
	ls := &Layouts{edited: filepath.Join(dir, "layout.txt"), applied: filepath.Join(dir, "layout.applied")}
	applied, err := os.ReadFile(ls.applied)
	if errors.Is(err, fs.ErrNotExist) {
		applied = []byte(DefaultLayout)
		if err := writeAtomic(ls.applied, applied); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("files: reading %s: %w", ls.applied, err)
	}
	if _, err := os.Stat(ls.edited); errors.Is(err, fs.ErrNotExist) {
		if err := writeAtomic(ls.edited, applied); err != nil {
			return nil, err
		}
	}
	l, err := ParseLayout(string(applied))
	if err != nil {
		return nil, fmt.Errorf("files: the applied layout %s: %w", ls.applied, err)
	}
	ls.inForce = l
	return ls, nil
}

// File is the layout Boris edits.
func (ls *Layouts) File() string { return ls.edited }

// InForce is the layout the files are laid out by.
func (ls *Layouts) InForce() Layout {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.inForce
}

// Edited is the layout as Boris last saved it.
func (ls *Layouts) Edited() (Layout, error) {
	b, err := os.ReadFile(ls.edited)
	if err != nil {
		return Layout{}, fmt.Errorf("files: reading the layout: %w", err)
	}
	return ParseLayout(string(b))
}

// apply makes l the layout in force.
func (ls *Layouts) apply(l Layout) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if err := writeAtomic(ls.applied, []byte(l.text)); err != nil {
		return err
	}
	ls.inForce = l
	return nil
}

func writeAtomic(p string, b []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("files: writing %s: %w", p, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("files: writing %s: %w", p, err)
	}
	return nil
}
