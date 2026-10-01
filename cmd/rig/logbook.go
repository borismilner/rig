package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/borismilner/rig/internal/logbook"
)

// rig logbook, plan/51: the split logbook's documents, read and written as
// plain files with no daemon. rig being down never stops an agent (R3).

const logbookUsage = `usage: rig logbook <cmd> [--dir D] ...
  grep [-i] [-F] [-l] <pattern> [<doc>...]  search the parts, and what waits below the marker
  show <doc> <NNNN|key>   one entry, by its number or the start of its title (B107)
  line <doc> <N>          where line N of the whole document lives now
  cat <doc>               the whole document, as it read before the split
  add <doc> [--title T]   a new entry from stdin; the index is rebuilt
  index [<doc>...]        move what sits below the marker into parts, then rebuild
  check [<doc>...]        exit 1 when an index is stale
  open [--all] [<doc>...] the work items not closed, one line each
  brief                   what a resuming session needs first, in a few KB
  split <doc>             once: a document becomes parts and an index`

func cmdLogbook(args []string) (err error) {
	fs := flag.NewFlagSet("logbook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "emit JSON")
	dir := fs.String("dir", "", "the folder the documents are in")
	title := fs.String("title", "", "add: the entry's heading, when stdin has none")
	fold := fs.Bool("i", false, "grep: ignore case")
	fixed := fs.Bool("F", false, "grep: the pattern is a fixed string")
	names := fs.Bool("l", false, "grep: print only the files that match")
	all := fs.Bool("all", false, "open: closed items too")
	flags, positional := partition(args)
	if err := fs.Parse(flags); err != nil {
		return badArgumentf("%v\n%s", err, logbookUsage)
	}
	defer func() { err = inMode(err, *asJSON) }()
	if len(positional) == 0 {
		return badArgumentf(logbookUsage)
	}
	lb := logbookIn{dir: *dir, asJSON: *asJSON}
	sub, rest := positional[0], positional[1:]
	switch {
	case sub == "grep" && len(rest) >= 1:
		return lb.grep(rest[0], rest[1:], *fold, *fixed, *names)
	case sub == "show" && len(rest) == 2:
		return lb.show(rest[0], rest[1])
	case sub == "line" && len(rest) == 2:
		return lb.line(rest[0], rest[1])
	case sub == "cat" && len(rest) == 1:
		return lb.cat(rest[0])
	case sub == "add" && len(rest) == 1:
		return lb.add(rest[0], *title, os.Stdin)
	case sub == "index" || sub == "check":
		return lb.index(rest, sub == "check")
	case sub == "open":
		return lb.open(rest, *all)
	case sub == "brief" && len(rest) == 0:
		return lb.brief()
	case sub == "split" && len(rest) == 1:
		return lb.split(rest[0])
	}
	return badArgumentf(logbookUsage)
}

type logbookIn struct {
	dir    string
	asJSON bool
}

// home is where a document named without a path lives: --dir, else the
// logbook's notes for the project this shell is in
// ($RIG_LOGBOOK or ~/me/projects/logbook, then projects/<git root's name>).
func (lb logbookIn) home() (string, error) {
	if lb.dir != "" {
		return lb.dir, nil
	}
	root := os.Getenv("RIG_LOGBOOK")
	if root == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(h, "me", "projects", "logbook")
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if splitHere(wd) {
		return wd, nil // run from inside the notes themselves
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return filepath.Join(root, "projects", filepath.Base(d)), nil
		}
		if d == filepath.Dir(d) {
			return "", errors.New("not inside a git checkout, so there is no project to find notes for; pass --dir")
		}
	}
}

// splitHere is true when dir holds a split document.
func splitHere(dir string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		// A regular file only: a project's checkout links DECISIONS.md in,
		// and that makes it a project, not the notes folder.
		if ext := filepath.Ext(e.Name()); e.Type().IsRegular() && (ext == ".md" || ext == ".txt") &&
			logbook.Open(filepath.Join(dir, e.Name())).Generated() {
			return true
		}
	}
	return false
}

// doc resolves a name. A path that exists is taken as it is, so the command
// works from inside the logbook too; a bare name is looked up at home.
func (lb logbookIn) doc(name string) (logbook.Doc, error) {
	if lb.dir == "" {
		if _, err := os.Stat(name); err == nil {
			return logbook.Open(name), nil
		}
	}
	if strings.ContainsRune(name, filepath.Separator) && lb.dir == "" {
		return logbook.Doc{}, badArgumentf("%s: no such file", name)
	}
	h, err := lb.home()
	if err != nil {
		return logbook.Doc{}, err
	}
	d := logbook.Open(filepath.Join(h, filepath.Base(name)))
	if _, err := os.Stat(d.Index); err != nil && !d.Split() {
		return logbook.Doc{}, badArgumentf("%s: not in %s", name, h)
	}
	return d, nil
}

// docs is the named documents, or every split one at home.
func (lb logbookIn) docs(names []string) ([]logbook.Doc, error) {
	var out []logbook.Doc
	for _, n := range names {
		d, err := lb.doc(n)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if len(names) > 0 {
		return out, nil
	}
	h, err := lb.home()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(h)
	if err != nil {
		return nil, fmt.Errorf("no logbook notes at %s; pass --dir", h)
	}
	for _, e := range ents {
		if ext := filepath.Ext(e.Name()); e.IsDir() || (ext != ".md" && ext != ".txt") {
			continue
		}
		if d := logbook.Open(filepath.Join(h, e.Name())); d.Generated() {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no split document in %s", h)
	}
	return out, nil
}

// shown is a path the way the caller can open it: relative when it is under
// the working directory, absolute otherwise.
func shown(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return abs
}

func (lb logbookIn) emit(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }

type logbookHit struct {
	Doc     string `json:"doc"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	DocLine int    `json:"doc_line"`
	Text    string `json:"text"`
}

func (lb logbookIn) grep(pattern string, names []string, fold, fixed, onlyNames bool) error {
	if fixed {
		pattern = regexp.QuoteMeta(pattern)
	}
	if fold {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return badArgumentf("the pattern: %v", err)
	}
	docs, err := lb.docs(names)
	if err != nil {
		return err
	}
	var hits []logbookHit
	for _, d := range docs {
		leaves, err := d.Leaves()
		if err != nil {
			return err
		}
		base := filepath.Dir(d.Index)
		last := 1
		for _, l := range leaves {
			hits = scan(hits, re, d.Name, filepath.Join(base, l.Path), l.Text, 1, l.Line)
			last = l.Line + logbook.LineCount(l.Text)
		}
		pending, err := d.Pending()
		if err != nil {
			return err
		}
		// Below the marker: not yet in a part, so its file is the index. Its
		// place in the document is after the last part.
		if pending != "" {
			text := fileText(d.Index)
			head := strings.Count(text[:len(text)-len(pending)], "\n") + 1
			hits = scan(hits, re, d.Name, d.Index, pending, head, last)
		}
	}
	if lb.asJSON {
		if hits == nil {
			hits = []logbookHit{}
		}
		return lb.emit(hits)
	}
	seen := map[string]bool{}
	for _, h := range hits {
		switch {
		case !onlyNames:
			fmt.Printf("%s:%d:%s\n", shown(h.Path), h.Line, h.Text)
		case !seen[h.Path]:
			seen[h.Path] = true
			fmt.Println(shown(h.Path))
		}
	}
	if len(hits) == 0 {
		return fmt.Errorf("no line matches %q", pattern)
	}
	return nil
}

func scan(hits []logbookHit, re *regexp.Regexp, doc, path, text string, line, docLine int) []logbookHit {
	for i, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if re.MatchString(l) {
			hits = append(hits, logbookHit{Doc: doc, Path: path, Line: line + i, DocLine: docLine + i, Text: l})
		}
	}
	return hits
}

func fileText(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func (lb logbookIn) show(name, key string) error {
	d, err := lb.doc(name)
	if err != nil {
		return err
	}
	leaves, err := d.Leaves()
	if err != nil {
		return err
	}
	var found []logbook.Leaf
	if n, err := strconv.Atoi(key); err == nil && n >= 0 && n <= 9999 {
		want := fmt.Sprintf("%s/%04d-", d.Stem, n)
		for _, l := range leaves {
			if strings.HasPrefix(l.Path, want) {
				found = append(found, l)
			}
		}
	}
	for _, match := range []func(string) bool{
		func(t string) bool { return t == key || strings.HasPrefix(t, key+" ") },
		func(t string) bool { return strings.Contains(strings.ToLower(t), strings.ToLower(key)) },
	} {
		if len(found) > 0 {
			break
		}
		for _, l := range leaves {
			if match(l.Title) {
				found = append(found, l)
			}
		}
	}
	base := filepath.Dir(d.Index)
	if lb.asJSON {
		out := []map[string]any{}
		for _, l := range found {
			out = append(out, map[string]any{"path": filepath.Join(base, l.Path), "title": l.Title, "doc_line": l.Line, "text": l.Text})
		}
		return lb.emit(out)
	}
	switch len(found) {
	case 0:
		return fmt.Errorf("%s: no entry numbered or titled %q", d.Name, key)
	case 1:
		fmt.Printf("%s\n\n%s", shown(filepath.Join(base, found[0].Path)), found[0].Text)
		return nil
	}
	fmt.Printf("%d entries match %q; name one by its number:\n", len(found), key)
	for _, l := range found {
		fmt.Printf("  %s  %s\n", shown(filepath.Join(base, l.Path)), l.Title)
	}
	return nil
}

func (lb logbookIn) line(name, num string) error {
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 {
		return badArgumentf("a line is a number from 1; got %q", num)
	}
	d, err := lb.doc(name)
	if err != nil {
		return err
	}
	leaf, at, err := d.At(n)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(d.Index), leaf.Path)
	text := strings.Split(leaf.Text, "\n")[at-1]
	if lb.asJSON {
		return lb.emit(logbookHit{Doc: d.Name, Path: path, Line: at, DocLine: n, Text: text})
	}
	fmt.Printf("%s:%d:%s\n", shown(path), at, text)
	return nil
}

func (lb logbookIn) cat(name string) error {
	d, err := lb.doc(name)
	if err != nil {
		return err
	}
	parts, err := d.Parts()
	if err != nil {
		return err
	}
	pending, err := d.Pending()
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(logbook.Join(parts) + pending)
	return err
}

func (lb logbookIn) add(name, title string, in io.Reader) error {
	d, err := lb.doc(name)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(in, logbook.Big*4+1))
	if err != nil {
		return err
	}
	if len(b) > logbook.Big*4 {
		return badArgumentf("an entry is at most %d KB; split it into entries", logbook.Big*4/1024)
	}
	entry := string(b)
	if title != "" && !strings.HasPrefix(entry, "#") {
		entry = "## " + title + "\n\n" + entry
	}
	if !strings.HasPrefix(entry, "## ") {
		return badArgumentf("an entry starts with a `## ` heading; give one on stdin or with --title")
	}
	path, err := d.Add(entry)
	if err != nil {
		return err
	}
	if _, err := d.Reindex(); err != nil {
		return fmt.Errorf("%s written, but the index was not: %w", shown(path), err)
	}
	if lb.asJSON {
		return lb.emit(map[string]string{"path": path})
	}
	fmt.Println(shown(path))
	return nil
}

func (lb logbookIn) index(names []string, check bool) error {
	docs, err := lb.docs(names)
	if err != nil {
		return err
	}
	type row struct {
		Doc   string `json:"doc"`
		Moved int    `json:"moved_in,omitempty"`
		Stale bool   `json:"stale,omitempty"`
	}
	var rows []row
	stale := 0
	for _, d := range docs {
		r := row{Doc: d.Name}
		if check {
			if r.Stale, err = d.Stale(); err != nil {
				return err
			}
			if r.Stale {
				stale++
			}
		} else if r.Moved, err = d.Reindex(); err != nil {
			return err
		}
		rows = append(rows, r)
	}
	if lb.asJSON {
		if err := lb.emit(rows); err != nil {
			return err
		}
	} else {
		for _, r := range rows {
			switch {
			case r.Stale:
				fmt.Printf("%s: stale, run rig logbook index %s\n", r.Doc, r.Doc)
			case check:
			case r.Moved > 0:
				fmt.Printf("%s: index rebuilt, %d entries moved in from below the marker\n", r.Doc, r.Moved)
			default:
				fmt.Printf("%s: index rebuilt\n", r.Doc)
			}
		}
	}
	if stale > 0 {
		return fmt.Errorf("%d of %d indexes are stale", stale, len(rows))
	}
	return nil
}

func (lb logbookIn) split(name string) error {
	d, err := lb.doc(name)
	if err != nil {
		return err
	}
	before, parts, err := d.SplitFile()
	if err != nil {
		return err
	}
	if lb.asJSON {
		return lb.emit(map[string]any{"doc": d.Name, "bytes": before, "parts": parts, "dir": d.Dir})
	}
	fmt.Printf("%s: %d bytes -> %s/ (%d top-level parts)\n", d.Name, before, d.Stem, parts)
	return nil
}

// clip cuts s to n characters, marking the cut.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-3]) + "..."
	}
	return s
}

// items are the work items of the named documents, or of every split one.
func (lb logbookIn) items(names []string, all bool) ([]logbook.Item, error) {
	docs, err := lb.docs(names)
	if err != nil {
		return nil, err
	}
	var out []logbook.Item
	for _, d := range docs {
		leaves, err := d.Leaves()
		if err != nil {
			return nil, err
		}
		for _, it := range logbook.Items(leaves) {
			if all || it.Open {
				out = append(out, it)
			}
		}
	}
	return out, nil
}

func (lb logbookIn) open(names []string, all bool) error {
	items, err := lb.items(names, all)
	if err != nil {
		return err
	}
	if lb.asJSON {
		if items == nil {
			items = []logbook.Item{}
		}
		return lb.emit(items)
	}
	unclassified := 0
	for _, it := range items {
		word := it.Word
		if word == "" {
			word, unclassified = "?", unclassified+1
		}
		fmt.Printf("%-6s %-10s %s\n", it.ID, word, clip(it.Title, 70))
	}
	fmt.Printf("%d items; rig logbook show <doc> <id> reads one\n", len(items))
	if unclassified > 0 {
		fmt.Printf("%d marked ? have no state word: start the state with one of %s\n",
			unclassified, strings.Join(logbook.States, ", "))
	}
	return nil
}

// brief is what a resuming session reads first: the handoffs, what each
// document holds and its newest entries, what waits below a marker, and the
// open work, so the whole files are read only when a line here points there.
func (lb logbookIn) brief() error {
	docs, err := lb.docs(nil)
	if err != nil {
		return err
	}
	home := filepath.Dir(docs[0].Index)
	type docBrief struct {
		Doc     string   `json:"doc"`
		Entries int      `json:"entries"`
		KB      int      `json:"kb"`
		Newest  []string `json:"newest"`
		Pending bool     `json:"pending"`
		Stale   bool     `json:"stale"`
	}
	var briefs []docBrief
	var open []logbook.Item
	for _, d := range docs {
		leaves, err := d.Leaves()
		if err != nil {
			return err
		}
		b := docBrief{Doc: d.Name, Entries: len(leaves)}
		for _, l := range leaves {
			b.KB += len(l.Text)
		}
		b.KB /= 1024
		for _, l := range leaves[max(0, len(leaves)-4):] {
			b.Newest = append(b.Newest, l.Title)
		}
		p, err := d.Pending()
		if err != nil {
			return err
		}
		b.Pending = p != ""
		if b.Stale, err = d.Stale(); err != nil {
			return err
		}
		briefs = append(briefs, b)
		for _, it := range logbook.Items(leaves) {
			if it.Open {
				open = append(open, it)
			}
		}
	}
	handoffs, _ := filepath.Glob(filepath.Join(home, "HANDOFF*.md"))
	type handoff struct {
		Path    string `json:"path"`
		KB      int    `json:"kb"`
		Written string `json:"written"`
	}
	var hs []handoff
	for _, h := range handoffs {
		if st, err := os.Stat(h); err == nil {
			hs = append(hs, handoff{h, int(st.Size() / 1024), st.ModTime().Format("2006-01-02 15:04")})
		}
	}
	slices.SortFunc(hs, func(a, b handoff) int { return strings.Compare(b.Written, a.Written) })
	if lb.asJSON {
		if open == nil {
			open = []logbook.Item{}
		}
		return lb.emit(map[string]any{"dir": home, "handoffs": hs, "documents": briefs, "open": open})
	}
	fmt.Printf("notes: %s\n\nhandoffs, newest first:\n", shown(home))
	for _, h := range hs {
		fmt.Printf("  %s  %s  %d KB\n", h.Written, filepath.Base(h.Path), h.KB)
	}
	for _, b := range briefs {
		fmt.Printf("\n%s: %d entries, %d KB", b.Doc, b.Entries, b.KB)
		if b.Pending {
			fmt.Print("; entries wait below the marker")
		}
		if b.Stale {
			fmt.Print("; index STALE, run rig logbook index")
		}
		fmt.Println("\n  newest:")
		for _, t := range b.Newest {
			fmt.Println("    " + clip(t, 90))
		}
	}
	fmt.Printf("\nopen work, in document order (%d; rig logbook open for states):\n", len(open))
	for _, it := range open {
		fmt.Printf("  %-6s %-9s %s\n", it.ID, it.Word, clip(it.Title, 70))
	}
	return nil
}
