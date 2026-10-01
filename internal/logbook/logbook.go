// Package logbook splits a big logbook document into one file per entry and
// keeps the old path as a generated index (plan/51, ruled by Boris
// 2026-10-01: the files stay the truth, rig only makes them cheap to use).
//
// A document is an ordered list of parts, one file each, named NNNN-slug.md.
// Concatenating the parts in name order gives back the document byte for
// byte. A part over Big characters becomes a directory of its own parts,
// split at its next heading level, then at table rows, then at paragraphs.
//
// Everything here works on plain files and needs no daemon (plan/51 R3).
package logbook

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Big is the size, in characters, over which a part is split further.
const Big = 24 * 1024

// maxDepth bounds the nesting: level 2 is a document's `## ` sections.
const maxDepth = 5

// Part is one piece of a document: text, or a directory of parts.
type Part struct {
	Name string // NNNN-slug, without .md
	Text string // set when Sub is nil
	Sub  []Part
}

// Dir reports whether p is a directory of parts.
func (p Part) Dir() bool { return p.Sub != nil }

// Join concatenates parts back into the document they came from.
func Join(parts []Part) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Dir() {
			b.WriteString(Join(p.Sub))
		} else {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func size(s string) int { return utf8.RuneCountInString(s) }

// lines splits s after every newline, keeping them.
func lines(s string) []string {
	var out []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// cut splits text before every line for which heading says yes. The first
// piece is what precedes the first heading, possibly empty.
func cut(text string, heading func(ls []string, i int) bool) []string {
	ls := lines(text)
	var bounds []int
	for i := range ls {
		if heading(ls, i) {
			bounds = append(bounds, i)
		}
	}
	if len(bounds) == 0 {
		return []string{text}
	}
	bounds = append(append([]int{0}, bounds...), len(ls))
	out := make([]string, 0, len(bounds)-1)
	for k := 0; k+1 < len(bounds); k++ {
		out = append(out, strings.Join(ls[bounds[k]:bounds[k+1]], ""))
	}
	return out
}

var (
	ruleRE   = regexp.MustCompile(`^-{20,}\n$`)
	sepRE    = regexp.MustCompile(`^\|\s*:?-{3,}`)
	bannerRE = regexp.MustCompile(`^\s*[-=]{20,}\s*$`)
	hashRE   = regexp.MustCompile(`^#+\s*`)
	dropRE   = regexp.MustCompile("[`*_\\[\\]()~⛔✅]")
	nonSlug  = regexp.MustCompile(`[^a-z0-9]+`)
)

// headingAt is a heading of the given level, by format: markdown's `## `,
// or a .txt file's banner (a dashed rule, a title line, a dashed rule).
func headingAt(ext string, level int) func([]string, int) bool {
	if ext == ".txt" {
		if level != 2 {
			return func([]string, int) bool { return false }
		}
		return func(ls []string, i int) bool {
			return i+2 < len(ls) && ruleRE.MatchString(ls[i]) &&
				strings.HasSuffix(ls[i+1], "\n") && ls[i+1] != "\n" && ruleRE.MatchString(ls[i+2])
		}
	}
	mark := strings.Repeat("#", level) + " "
	return func(ls []string, i int) bool { return strings.HasPrefix(ls[i], mark) }
}

// rows splits before every table row after the first table's separator, so
// a row wrapped by a stray line still gets its own piece. Piece 0 is what
// precedes the first row; a later table's header and any prose ride with
// the row before them.
func rows(text string) []string {
	ls := lines(text)
	var cuts []int
	inTable := false
	for i, ln := range ls {
		if sepRE.MatchString(ln) && i > 0 && strings.HasPrefix(ls[i-1], "|") {
			inTable = true
			continue
		}
		header := i+1 < len(ls) && sepRE.MatchString(ls[i+1])
		if inTable && strings.HasPrefix(ln, "|") && !header {
			cuts = append(cuts, i)
		}
	}
	if len(cuts) < 2 {
		return []string{text}
	}
	bounds := append(append([]int{0}, cuts...), len(ls))
	var out []string
	for k := 0; k+1 < len(bounds); k++ {
		if bounds[k] < bounds[k+1] {
			out = append(out, strings.Join(ls[bounds[k]:bounds[k+1]], ""))
		}
	}
	return out
}

// paragraphs packs blank-line-separated paragraphs into pieces of about
// Big/2 characters: the last resort for prose with no headings or tables.
func paragraphs(text string) []string {
	var paras []string
	start := 0
	for i := 2; i <= len(text); i++ {
		if text[i-2:i] == "\n\n" {
			paras = append(paras, text[start:i])
			start = i
		}
	}
	paras = append(paras, text[start:])
	var out []string
	cur := ""
	for _, p := range paras {
		if cur != "" && size(cur)+size(p) > Big/2 {
			out = append(out, cur)
			cur = ""
		}
		cur += p
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// Title is a part's first meaningful line: a heading without its hashes, or
// a table row's first two cells.
func Title(part string) string {
	first := ""
	for l := range strings.SplitSeq(part, "\n") {
		l = strings.TrimSuffix(l, "\r")
		if strings.TrimSpace(l) != "" && !bannerRE.MatchString(l) {
			first = strings.TrimSpace(l)
			break
		}
	}
	if strings.HasPrefix(first, "|") {
		var keep []string
		for k, c := range strings.Split(strings.Trim(first, "|"), "|") {
			if c = strings.TrimSpace(c); k < 2 && c != "" {
				keep = append(keep, c)
			}
		}
		return strings.Join(keep, " - ")
	}
	return hashRE.ReplaceAllString(first, "")
}

// Slug is a title as a file name: lower case, dashes, at most 60 bytes.
func Slug(title string) string {
	t := dropRE.ReplaceAllString(strings.ToLower(title), "")
	t = strings.Trim(nonSlug.ReplaceAllString(t, "-"), "-")
	if len(t) > 60 {
		t = t[:60]
	}
	if t = strings.TrimRight(t, "-"); t == "" {
		return "part"
	}
	return t
}

func partName(n int, text string) string {
	if n == 0 {
		return "0000-about"
	}
	return fmt.Sprintf("%04d-%s", n, Slug(Title(text)))
}

// Plan splits a document into parts. ext is its extension, which picks the
// heading form.
func Plan(text, ext string) []Part { return plan(text, 2, ext) }

func plan(text string, level int, ext string) []Part {
	var pieces []string
	for _, p := range cut(text, headingAt(ext, level)) {
		if p != "" {
			pieces = append(pieces, p)
		}
	}
	if level > 2 && len(pieces) == 1 {
		pieces = rows(text)
	}
	if level > 2 && len(pieces) == 1 && size(text) > Big {
		pieces = paragraphs(text)
	}
	out := make([]Part, 0, len(pieces))
	for n, p := range pieces {
		name := partName(n, p)
		if size(p) > Big && level < maxDepth {
			if sub := plan(p, level+1, ext); len(sub) > 1 {
				out = append(out, Part{Name: name, Sub: sub})
				continue
			}
		}
		out = append(out, Part{Name: name, Text: p})
	}
	return out
}

var numbered = regexp.MustCompile(`^\d{4}-`)

// Load reads a parts directory back, in name order.
func Load(dir string) ([]Part, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Part
	for _, e := range ents { // ReadDir sorts by name
		if !numbered.MatchString(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		switch {
		case e.IsDir():
			sub, err := Load(p)
			if err != nil {
				return nil, err
			}
			out = append(out, Part{Name: e.Name(), Sub: sub})
		case strings.HasSuffix(e.Name(), ".md") && e.Type().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			out = append(out, Part{Name: strings.TrimSuffix(e.Name(), ".md"), Text: string(b)})
		}
	}
	if out == nil {
		out = []Part{}
	}
	return out, nil
}

// write lays parts out under dir.
func write(dir string, parts []Part) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range parts {
		path := filepath.Join(dir, p.Name)
		if p.Dir() {
			if err := write(path, p.Sub); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(path+".md", []byte(p.Text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Doc is one split document: its index file and the parts directory beside
// it, named after the file in lower case (DECISIONS.md -> decisions/).
type Doc struct {
	Index string // the path of the index file
	Name  string // its base name
	Stem  string // the parts directory's base name
	Dir   string // the parts directory
}

// Open names a document by the path of its index file.
//
// A symlink is followed first: a project links DECISIONS.md into its logbook
// folder, and the parts sit beside the file, not beside the link.
func Open(path string) Doc {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	name := filepath.Base(path)
	stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	return Doc{Index: path, Name: name, Stem: stem, Dir: filepath.Join(filepath.Dir(path), stem)}
}

// Split is true when the document has been split.
func (d Doc) Split() bool {
	st, err := os.Stat(d.Dir)
	return err == nil && st.IsDir()
}

// ErrNotSplit is a document that has no parts directory yet.
var ErrNotSplit = errors.New("not split")

// Parts loads the document's parts.
func (d Doc) Parts() ([]Part, error) {
	if !d.Split() {
		return nil, fmt.Errorf("%s: %w; rig logbook split %s first", d.Name, ErrNotSplit, d.Name)
	}
	return Load(d.Dir)
}

// SplitFile turns the document into parts and its file into the index. It
// proves the parts reassemble to the original twice, in memory and from
// disk, and touches nothing when the first proof fails.
func (d Doc) SplitFile() (before, parts int, err error) {
	if d.Split() {
		return 0, 0, fmt.Errorf("%s/ exists: %s is already split; use index", d.Stem, d.Name)
	}
	b, err := os.ReadFile(d.Index)
	if err != nil {
		return 0, 0, err
	}
	text := string(b)
	ps := Plan(text, filepath.Ext(d.Name))
	if Join(ps) != text {
		return 0, 0, fmt.Errorf("the parts do not reassemble to %s: nothing written", d.Name)
	}
	if err := write(d.Dir, ps); err != nil {
		return 0, 0, err
	}
	back, err := Load(d.Dir)
	if err != nil {
		return 0, 0, err
	}
	if Join(back) != text {
		return 0, 0, fmt.Errorf("the parts on disk do not reassemble to %s: %s/ left for inspection, %s untouched", d.Name, d.Stem, d.Name)
	}
	return len(b), len(ps), os.WriteFile(d.Index, []byte(IndexText(d, back)), 0o644)
}

// The index's first line, and the marker at its end. Below the marker an
// agent may append an entry the old way; Reindex moves it into a part.
const (
	headFmt = "<!-- generated by rig logbook from %s/ - edit the parts, not this index -->"
	markFmt = "<!-- logbook: add an entry below this line, or as a new file in %s/; then run rig logbook index %s -->"
)

// markRE finds the marker this package writes and the one the first,
// Python, version wrote, so an append below either is never lost.
var (
	markRE = regexp.MustCompile(`(?m)^<!-- (logbook|logsplit): add an entry below this line.*-->$`)
	headRE = regexp.MustCompile(`^<!-- generated by (rig logbook|tools/logsplit\.py) from `)
)

// IndexText is the index file for parts.
func IndexText(d Doc, parts []Part) string {
	out := []string{fmt.Sprintf(headFmt, d.Stem), ""}
	if len(parts) > 0 && strings.HasPrefix(parts[0].Name, "0000-about") && !parts[0].Dir() && parts[0].Text != "" {
		out = append(out, strings.TrimRight(parts[0].Text, "\n"), "")
	}
	out = append(out, fmt.Sprintf("**One file per entry, in `%s/`.** Read the entry you need; "+
		"`rig logbook grep` or `grep -r` the folder; `rig logbook cat %s` gives the whole "+
		"document. Sizes are in KB.", d.Stem, d.Name), "")
	var walk func(ps []Part, rel string, depth int)
	walk = func(ps []Part, rel string, depth int) {
		for _, p := range ps {
			if depth == 0 && strings.HasPrefix(p.Name, "0000-about") && !p.Dir() {
				continue // shown inline above
			}
			text := p.Text
			link := rel + "/" + p.Name + ".md"
			if p.Dir() {
				text, link = Join(p.Sub), rel+"/"+p.Name+"/"
			}
			t := Title(text)
			if size(t) > 113 {
				t = string([]rune(t)[:110]) + "..."
			}
			out = append(out, fmt.Sprintf("%s- [%s](%s) %s (%.1f)",
				strings.Repeat("  ", depth), p.Name[:4], link, t, float64(size(text))/1024))
			if p.Dir() {
				walk(p.Sub, rel+"/"+p.Name, depth+1)
			}
		}
	}
	walk(parts, d.Stem, 0)
	out = append(out, "", fmt.Sprintf(markFmt, d.Stem, d.Name), "")
	return strings.Join(out, "\n")
}

// Reindex moves whatever was appended below the index's marker into new
// parts, then rewrites the index. It answers how many entries it moved in.
func (d Doc) Reindex() (int, error) {
	if !d.Split() {
		return 0, fmt.Errorf("%s: %w; rig logbook split %s first", d.Name, ErrNotSplit, d.Name)
	}
	b, err := os.ReadFile(d.Index)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	text := string(b)
	added := 0
	if loc := markRE.FindStringIndex(text); loc != nil {
		if added, err = d.addAll(strings.Trim(text[loc[1]:], "\n")); err != nil {
			return 0, err
		}
	} else if text != "" && !headRE.MatchString(text) {
		return 0, fmt.Errorf("%s has no marker and is not a generated index: nothing touched", d.Name)
	}
	parts, err := Load(d.Dir)
	if err != nil {
		return 0, err
	}
	// An entry appended while this ran would be lost to the rewrite, so it
	// is moved in too. The window is small, not closed.
	if now, err := os.ReadFile(d.Index); err == nil && string(now) != text {
		if !strings.HasPrefix(string(now), text) {
			return added, fmt.Errorf("%s was edited while it was being indexed: what was below its marker "+
				"is already in %s/, so delete it from the index by hand before running index again", d.Name, d.Stem)
		}
		more, err := d.addAll(strings.Trim(string(now)[len(text):], "\n"))
		if added += more; err != nil {
			return added, err
		}
		if parts, err = Load(d.Dir); err != nil {
			return added, err
		}
	}
	return added, os.WriteFile(d.Index, []byte(IndexText(d, parts)), 0o644)
}

// addAll writes each `## ` section of tail as a new part at the end.
func (d Doc) addAll(tail string) (int, error) {
	if tail == "" {
		return 0, nil
	}
	added := 0
	for _, p := range cut(tail+"\n", headingAt(".md", 2)) {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, err := d.Add(p); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// Add writes entry as the next part at the top level and answers its path.
// It never reads an existing part, only the directory's names.
func (d Doc) Add(entry string) (string, error) {
	if strings.TrimSpace(entry) == "" {
		return "", errors.New("an empty entry")
	}
	if !strings.HasSuffix(entry, "\n\n") {
		entry = strings.TrimRight(entry, "\n") + "\n\n"
	}
	ents, err := os.ReadDir(d.Dir)
	if err != nil {
		return "", err
	}
	n := 1
	for _, e := range ents {
		if numbered.MatchString(e.Name()) {
			var k int
			if _, err := fmt.Sscanf(e.Name()[:4], "%d", &k); err == nil && k >= n {
				n = k + 1
			}
		}
	}
	if n > 9999 {
		return "", fmt.Errorf("%s/ is full at 9999 entries", d.Stem)
	}
	path := filepath.Join(d.Dir, fmt.Sprintf("%04d-%s.md", n, Slug(Title(entry))))
	// O_EXCL: two writers racing to the same name never overwrite each other.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(entry); err != nil {
		_ = f.Close()
		return "", err
	}
	return path, f.Close()
}

// Stale is true when the index on disk is not what Reindex would write.
func (d Doc) Stale() (bool, error) {
	parts, err := d.Parts()
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(d.Index)
	if err != nil {
		return false, err
	}
	return string(b) != IndexText(d, parts), nil
}

// Leaf is one part file, with the line the document's text would have
// given its first line.
type Leaf struct {
	Path  string // relative to the document's directory, e.g. decisions/0086-x.md
	Title string
	Text  string
	Line  int // 1-based line of its first line in the whole document
}

// Leaves flattens parts into files, in document order.
func (d Doc) Leaves() ([]Leaf, error) {
	parts, err := d.Parts()
	if err != nil {
		return nil, err
	}
	var out []Leaf
	line := 1
	var walk func(ps []Part, rel string)
	walk = func(ps []Part, rel string) {
		for _, p := range ps {
			if p.Dir() {
				walk(p.Sub, rel+"/"+p.Name)
				continue
			}
			out = append(out, Leaf{Path: rel + "/" + p.Name + ".md", Title: Title(p.Text), Text: p.Text, Line: line})
			line += strings.Count(p.Text, "\n")
		}
	}
	walk(parts, d.Stem)
	return out, nil
}

// At answers the part file holding line n of the whole document, and the
// line within it: how an old "DECISIONS.md line 5889" citation resolves.
func (d Doc) At(n int) (Leaf, int, error) {
	leaves, err := d.Leaves()
	if err != nil {
		return Leaf{}, 0, err
	}
	i, _ := slices.BinarySearchFunc(leaves, n, func(l Leaf, n int) int { return l.Line - n })
	if i == len(leaves) || leaves[i].Line > n {
		i--
	}
	if i < 0 || n-leaves[i].Line >= LineCount(leaves[i].Text) {
		return Leaf{}, 0, fmt.Errorf("%s has no line %d", d.Name, n)
	}
	return leaves[i], n - leaves[i].Line + 1, nil
}

// LineCount is how many lines text has, a last line without a newline
// counted.
func LineCount(text string) int {
	n := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		n++
	}
	return n
}

// Pending is what sits below the index's marker, appended the old way and
// not yet moved into a part. Every reader includes it, so an entry is
// findable the moment it is written, before anyone runs index.
func (d Doc) Pending() (string, error) {
	b, err := os.ReadFile(d.Index)
	if err != nil {
		return "", err
	}
	text := string(b)
	loc := markRE.FindStringIndex(text)
	if loc == nil {
		return "", nil
	}
	return strings.TrimLeft(text[loc[1]:], "\n"), nil
}

// Generated is true when the document's file is an index this package, or
// the Python tool before it, wrote. A file beside a folder of the same name
// is not enough: PLAN.md and plan/ are not a logbook document.
func (d Doc) Generated() bool {
	f, err := os.Open(d.Index)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 128)
	n, _ := io.ReadFull(f, head)
	return d.Split() && headRE.Match(head[:n])
}

// Item is one work item: a table row or a heading whose title starts with
// an id like B107.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Word  string `json:"word"`  // the state word the item starts with; empty when unclassified
	State string `json:"state"` // the prose after it, as written
	Path  string `json:"path"`
	Open  bool   `json:"open"`
}

// States are the words an item's state starts with (BACKLOG.md "States").
// The last three close it.
var States = []string{"proposed", "argued", "owned", "held", "deferred", "moved", "done", "rejected", "superseded"}

var (
	itemIDRE = regexp.MustCompile(`^(B\d+[a-z]?)\b`)
	markupRE = regexp.MustCompile("[*`⛔✅~]+")
	spacesRE = regexp.MustCompile(`\s+`)
	wordRE   = regexp.MustCompile("\\|\\s*`(" + strings.Join(States, "|") + ")` · ")
	headWord = regexp.MustCompile("(?m)^State: `(" + strings.Join(States, "|") + ")`$")
	closedRE = regexp.MustCompile(`^(done|rejected|superseded)$`)
)

func plain(s string) string {
	return strings.TrimSpace(spacesRE.ReplaceAllString(markupRE.ReplaceAllString(s, ""), " "))
}

// Items are the work items in leaves, in document order, each id once. A
// row's state starts with one of States in backticks and a heading's is a
// "State: `word`" line. An item without one is unclassified and counts as
// open: hiding it would lose it.
func Items(leaves []Leaf) []Item {
	var out []Item
	seen := map[string]bool{}
	for _, l := range leaves {
		first := ""
		for ln := range strings.SplitSeq(l.Text, "\n") {
			if strings.TrimSpace(ln) != "" && !bannerRE.MatchString(ln) {
				first = strings.TrimSpace(ln)
				break
			}
		}
		var it Item
		if strings.HasPrefix(first, "|") {
			cells := strings.Split(strings.Trim(first, "|"), "|")
			id := itemIDRE.FindString(plain(cells[0]))
			if id == "" || len(cells) < 2 {
				continue
			}
			// The stamp is found in the row rather than in a counted cell:
			// a pipe inside a code span splits a row wrongly.
			state := cells[len(cells)-1]
			if ms := wordRE.FindAllStringSubmatchIndex(first, -1); ms != nil {
				m := ms[len(ms)-1]
				it.Word, state = first[m[2]:m[3]], strings.TrimSuffix(strings.TrimSpace(first[m[1]:]), "|")
			}
			it.ID, it.Title, it.State = id, plain(cells[1]), plain(state)
		} else {
			t := plain(hashRE.ReplaceAllString(first, ""))
			id := itemIDRE.FindString(t)
			if id == "" || !strings.HasPrefix(first, "#") {
				continue
			}
			it = Item{ID: id, Title: strings.TrimLeft(strings.TrimPrefix(t, id), " -:")}
			if m := headWord.FindStringSubmatch(l.Text); m != nil {
				it.Word = m[1]
			}
		}
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		it.Path = l.Path
		it.Open = !closedRE.MatchString(it.Word)
		out = append(out, it)
	}
	return out
}
