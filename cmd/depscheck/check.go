package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// dep is one thing the build depends on.
type dep struct {
	Name    string // as the manifest writes it
	Version string // as the manifest pins it
	Where   string // which manifest, for the message
}

// section pulls one numbered section out of the plan.
//
// By heading rather than by line number, because the plan is edited constantly
// and a line number in a checker is a checker that breaks on an unrelated edit.
func section(plan, heading string) (string, error) {
	body, err := os.ReadFile(plan)
	if err != nil {
		return "", err
	}
	text := string(body)
	start := strings.Index(text, heading)
	if start < 0 {
		return "", fmt.Errorf("%s has no %q heading: either the plan was "+
			"renumbered, in which case this checker needs the new heading, or "+
			"the stack table is gone, which is a bigger problem", plan, heading)
	}
	rest := text[start+len(heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end], nil
	}
	return rest, nil
}

// goDeps reads the DIRECT requirements of a go.mod.
//
// Indirect requirements are deliberately skipped: they are chosen by the
// modules that need them, not by anybody here, so a section 22 row for one
// would be recording a decision nobody made.
func goDeps(path string) ([]dep, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []dep
	inBlock := false
	for _, line := range strings.Split(string(body), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "require ("):
			inBlock = true
			continue
		case t == ")":
			inBlock = false
			continue
		}
		if !inBlock || t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		if strings.Contains(t, "// indirect") {
			continue
		}
		f := strings.Fields(t)
		if len(f) < 2 {
			continue
		}
		out = append(out, dep{Name: f[0], Version: f[1], Where: path})
	}
	return out, nil
}

// npmDeps reads both dependency blocks of a package.json.
func npmDeps(path string) ([]dep, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []dep
	for _, block := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name, version := range block {
			out = append(out, dep{Name: name, Version: version, Where: path})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// majorSuffix matches the /v2, /v6 a Go module path carries.
var majorSuffix = regexp.MustCompile(`/v\d+$`)

// alias maps a package name to the name the stack table writes.
//
// The table is written in PRODUCT names and a manifest pins PACKAGE names, and
// the two are not always the same string. Bridging that gap by guessing - by
// stripping a "css" suffix, say - is how a checker starts matching the wrong
// row, so each bridge is written down here and can be argued with.
var alias = map[string]string{
	"tailwindcss": "Tailwind",
}

// token is something to look for in the stack table.
//
// Exact means the table names this dependency and not merely something near
// it, which is the only case where comparing versions is meaningful. A
// fallback token is good enough to say "the table knows about this area" and
// NOT good enough to attribute a version: @tsconfig/svelte reached the Svelte
// row through the word `svelte` and was then reported against Svelte's own
// version, which is the wrong row and a false report.
type token struct {
	value string
	exact bool
}

// tokens is what to look for, most specific first.
func tokens(d dep) []token {
	out := []token{{d.Name, true}}
	if a, ok := alias[d.Name]; ok {
		out = append(out, token{a, true})
	}

	if strings.HasPrefix(d.Name, "@") {
		// A scoped npm package: @scope/base. Scope before base, because a base
		// is often a word as generic as `test`.
		if i := strings.Index(d.Name, "/"); i > 0 {
			out = append(out,
				token{strings.TrimPrefix(d.Name[:i], "@"), false},
				token{d.Name[i+1:], false})
		}
		return out
	}
	if strings.Contains(d.Name, "/") {
		bare := majorSuffix.ReplaceAllString(d.Name, "")
		if bare != d.Name {
			out = append(out, token{bare, true})
		}
		parts := strings.Split(bare, "/")
		if len(parts) >= 2 {
			// owner/repo, which is how prose names a Go module and how every
			// row in the stack table writes one. It identifies the module
			// exactly; it is a shorter spelling, not a neighbour.
			out = append(out, token{strings.Join(parts[len(parts)-2:], "/"), true})
		}
		// The bare repo name on its own is NOT exact: `sqlite` or `jsonschema`
		// could be any of several rows.
		out = append(out, token{parts[len(parts)-1], false})
	}
	return out
}

// versionish matches the version strings the stack table writes.
var versionish = regexp.MustCompile(`v?\d+\.\d+(\.\d+)?(-[A-Za-z0-9.]+)?`)

// rowFor returns the table line a token was found on.
func rowFor(text, token string) string {
	lower := strings.ToLower(text)
	i := strings.Index(lower, strings.ToLower(token))
	if i < 0 {
		return ""
	}
	start := strings.LastIndex(text[:i], "\n") + 1
	end := strings.Index(text[i:], "\n")
	if end < 0 {
		return text[start:]
	}
	return text[start : i+end]
}

// found reports whether a token appears as a token rather than inside a longer
// name: `jsonschema` must not match `json-schema-to-typescript`.
func found(text, token string) bool {
	lower := strings.ToLower(text)
	t := strings.ToLower(token)
	from := 0
	for {
		i := strings.Index(lower[from:], t)
		if i < 0 {
			return false
		}
		i += from
		before := byte(' ')
		if i > 0 {
			before = lower[i-1]
		}
		after := byte(' ')
		if i+len(t) < len(lower) {
			after = lower[i+len(t)]
		}
		if !wordByte(before) && !wordByte(after) {
			return true
		}
		from = i + len(t)
	}
}

// wordByte says whether a byte continues a package name.
func wordByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	case b == '-', b == '_', b == '.':
		return true
	}
	return false
}

// check compares every pinned dependency against the stack table.
//
// It returns two lists, and the second one exists because the first cannot be
// read without it. `problems` is what disagrees with the table. `unenforced`
// is every dependency whose VERSION this checker did not actually compare -
// because the row names several dependencies and cannot say which version
// belongs to which, because the name only matched a neighbour, or because the
// row deliberately pins nothing.
//
// Without that second list a clean run reads as "every version is pinned",
// and it never meant that: it means nothing DISAGREED, which is also what a
// comparison that never happened looks like. Measured on this repository on
// 2026-09-11, before the list existed: of seventeen direct dependencies, this
// checker compared FOUR versions and never compared the other thirteen - and
// it printed one line saying everything was named.
func check(plan, gomod, npm string) (problems, unenforced []string, err error) {
	stack, err := section(plan, "## 22. Tech stack")
	if err != nil {
		return nil, nil, err
	}

	deps, err := goDeps(gomod)
	if err != nil {
		return nil, nil, err
	}
	if npm != "" {
		more, err := npmDeps(npm)
		if err != nil {
			return nil, nil, err
		}
		deps = append(deps, more...)
	}

	for _, d := range deps {
		var hit token
		for _, t := range tokens(d) {
			if found(stack, t.value) {
				hit = t
				break
			}
		}
		if hit.value == "" {
			problems = append(problems, fmt.Sprintf(
				"%s %s (%s) is not named in the stack table",
				d.Name, d.Version, d.Where))
			continue
		}
		if !hit.exact {
			// Named through a neighbour. Enough to say the area is a decision
			// somebody made; not enough to pin a version on.
			unenforced = append(unenforced, fmt.Sprintf(
				"%s %s (%s): the table names it only through %q, which is a "+
					"neighbouring name, so no version was compared",
				d.Name, d.Version, d.Where, hit.value))
			continue
		}

		row := rowFor(stack, hit.value)
		want := versionish.FindAllString(row, -1)
		if len(want) == 0 {
			// A row that pins no version is a decision about the choice and
			// not about the version. Nothing to disagree with.
			unenforced = append(unenforced, fmt.Sprintf(
				"%s %s (%s): its row pins no version, which is a decision "+
					"about the choice rather than the version - by design",
				d.Name, d.Version, d.Where))
			continue
		}
		if len(want) > 1 {
			// The row carries more than one number and nothing says which is
			// this dependency's, so ANY of them is accepted. That is not a
			// pin. It happens when one row names several dependencies, and
			// also when a row's prose carries a number that is not a version
			// at all - the wire row's "+9.80 MiB" measurement is read as a
			// permitted version, so protobuf at v9.80.0 passes.
			unenforced = append(unenforced, fmt.Sprintf(
				"%s %s (%s): its row offers %s, and any of them is accepted",
				d.Name, d.Version, d.Where, strings.Join(want, " / ")))
		}
		if !agrees(d.Version, want) {
			problems = append(problems, fmt.Sprintf(
				"%s is pinned at %s (%s) but the stack table's row says %s",
				d.Name, d.Version, d.Where, strings.Join(want, " / ")))
		}
	}
	return problems, unenforced, nil
}

// agrees accepts a pin the table covers, including the abbreviations it uses:
// a table saying v1.36.x covers v1.36.11, and one saying 5.57 covers 5.57.0.
func agrees(pinned string, want []string) bool {
	got := strings.TrimPrefix(strings.TrimSpace(pinned), "v")
	got = strings.TrimLeft(got, "^~>=< ")
	for _, w := range want {
		w = strings.TrimPrefix(w, "v")
		if got == w || strings.HasPrefix(got, w+".") || strings.HasPrefix(got, w+"-") {
			return true
		}
	}
	return false
}
