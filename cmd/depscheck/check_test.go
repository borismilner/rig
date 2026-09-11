package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture writes a plan, a go.mod and a package.json and returns their paths.
func fixture(t *testing.T, stack, gomod, pkg string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	plan := filepath.Join(dir, "PLAN.md")
	body := "## 21. Something else\n\nnot the table\n\n## 22. Tech stack\n\n" +
		stack + "\n\n## 23. Milestone build order\n\nnot the table either\n"
	write(t, plan, body)

	mod := filepath.Join(dir, "go.mod")
	write(t, mod, gomod)

	npm := ""
	if pkg != "" {
		npm = filepath.Join(dir, "package.json")
		write(t, npm, pkg)
	}
	return plan, mod, npm
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

const oneRequire = "module example.com/x\n\ngo 1.27.1\n\nrequire (\n" +
	"\tgithub.com/santhosh-tekuri/jsonschema/v6 v6.0.3\n)\n"

func TestADependencyTheTableNamesPasses(t *testing.T) {
	plan, mod, _ := fixture(t,
		"| Schema | santhosh-tekuri/jsonschema (validate) | v6.0.3 |",
		oneRequire, "")
	problems, err := check(plan, mod, "")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("a named dependency at the named version was reported: %v", problems)
	}
}

func TestADependencyTheTableDoesNotNameIsReported(t *testing.T) {
	plan, mod, _ := fixture(t,
		"| Config | knadh/koanf/v2 | v2.3.6 |",
		oneRequire, "")
	problems, err := check(plan, mod, "")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "not named") {
		t.Fatalf("an unnamed dependency was not reported: %v", problems)
	}
}

// The asymmetry is the whole rule: a row with nothing pinned against it is a
// milestone that has not arrived, and must never fail the build.
func TestATableRowWithNothingInTheBuildIsNotAProblem(t *testing.T) {
	plan, mod, _ := fixture(t,
		"| Schema | santhosh-tekuri/jsonschema | v6.0.3 |\n"+
			"| Store | modernc.org/sqlite | v1.58.0 |\n"+
			"| CLI | spf13/cobra | v1.10.2 |",
		oneRequire, "")
	problems, err := check(plan, mod, "")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("rows for dependencies nobody has added yet were reported "+
			"as problems: %v", problems)
	}
}

func TestAVersionThatDisagreesWithTheTableIsReported(t *testing.T) {
	plan, mod, _ := fixture(t,
		"| Schema | santhosh-tekuri/jsonschema | v6.0.1 |",
		oneRequire, "")
	problems, err := check(plan, mod, "")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "v6.0.1") {
		t.Fatalf("a version disagreement was not reported: %v", problems)
	}
}

// The table abbreviates, and the abbreviations have to be accepted or every
// row becomes a false report.
func TestTheTablesAbbreviationsAreAccepted(t *testing.T) {
	for _, c := range []struct{ table, pinned string }{
		{"v1.36.x", "v1.36.11"},
		{"5.57", "5.57.0"},
		{"v3.0.0-beta.19", "v3.0.0-beta.19"},
	} {
		t.Run(c.table, func(t *testing.T) {
			gomod := "module example.com/x\n\ngo 1.27.1\n\nrequire (\n" +
				"\tgoogle.golang.org/protobuf " + c.pinned + "\n)\n"
			plan, mod, _ := fixture(t,
				"| Wire | google.golang.org/protobuf "+c.table+" | |",
				gomod, "")
			problems, err := check(plan, mod, "")
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if len(problems) != 0 {
				t.Fatalf("the table says %s and the pin is %s; reported %v",
					c.table, c.pinned, problems)
			}
		})
	}
}

// This is the false report that shipped in the first version of this checker.
//
// @tsconfig/svelte reached the Svelte row through the bare word `svelte` and
// was then compared against SVELTE's version, which is a different thing
// entirely. A neighbouring match may say "the table knows this area" and must
// never pin a version.
func TestANeighbouringMatchDoesNotGetItsNeighboursVersion(t *testing.T) {
	pkg := `{"devDependencies":{"@tsconfig/svelte":"5.0.8"}}`
	plan, mod, npm := fixture(t,
		"| Frontend | Svelte 5 + TypeScript + Vite | 5.57 / 7.0 / 8.2 |",
		oneRequire, pkg)
	problems, err := check(plan, mod, npm)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, p := range problems {
		if strings.Contains(p, "@tsconfig/svelte") {
			t.Fatalf("a neighbouring match was reported against the "+
				"neighbour's version: %s", p)
		}
	}
}

// The other false report from the first version: the table writes product
// names and a manifest writes package names.
func TestAProductNameInTheTableMatchesItsPackageName(t *testing.T) {
	pkg := `{"devDependencies":{"tailwindcss":"4.3.3"}}`
	plan, mod, npm := fixture(t,
		"| Frontend | Svelte 5 + TypeScript + Vite + Tailwind v4 | 5.57 / 7.0 / 8.2 / 4.3 |",
		oneRequire, pkg)
	problems, err := check(plan, mod, npm)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, p := range problems {
		if strings.Contains(p, "tailwindcss") {
			t.Fatalf("the table names Tailwind and the package is "+
				"tailwindcss; reported anyway: %s", p)
		}
	}
}

// A token must not match inside a longer package name.
func TestAShorterNameDoesNotMatchInsideALongerOne(t *testing.T) {
	pkg := `{"devDependencies":{"json-schema-to-typescript":"16.0.0"}}`
	plan, mod, npm := fixture(t,
		"| Schema | santhosh-tekuri/jsonschema (validate) | v6.0.3 |",
		oneRequire, npmOrEmpty(pkg))
	problems, err := check(plan, mod, npm)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, p := range problems {
		if strings.Contains(p, "json-schema-to-typescript") {
			found = true
		}
	}
	if !found {
		t.Fatal("json-schema-to-typescript was matched against jsonschema, " +
			"which is a different package")
	}
}

func npmOrEmpty(s string) string { return s }

// The boundary check, covered directly.
//
// The test above happens to report the same thing with or without it, which is
// no coverage at all - proved by mutation: removing the boundary check left it
// green. This one turns on it. The table names only `vite-plugin-svelte`, so a
// dependency called `vite` is NOT named, and a substring match would claim it
// was.
func TestATokenDoesNotMatchInsideALongerWord(t *testing.T) {
	pkg := `{"devDependencies":{"vite":"8.2.2"}}`
	plan, mod, npm := fixture(t,
		"| Frontend | vite-plugin-svelte | 7.3.0 |",
		oneRequire, pkg)
	problems, err := check(plan, mod, npm)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// The MESSAGE is the assertion, not merely that something was reported.
	// Without the boundary check `vite` matches inside `vite-plugin-svelte`,
	// takes that row, and gets reported against that row's version instead -
	// still a report, but the wrong one, and an assertion that only counted
	// reports passed either way.
	var got string
	for _, p := range problems {
		if strings.HasPrefix(p, "vite ") {
			got = p
		}
	}
	if got == "" {
		t.Fatalf("vite was not reported at all: %v", problems)
	}
	if !strings.Contains(got, "not named") {
		t.Fatalf("`vite` matched inside `vite-plugin-svelte` and was reported "+
			"against that row: %s", got)
	}
}

// Indirect requirements are nobody's decision here.
func TestIndirectRequirementsAreNotChecked(t *testing.T) {
	gomod := "module example.com/x\n\ngo 1.27.1\n\nrequire (\n" +
		"\tgithub.com/santhosh-tekuri/jsonschema/v6 v6.0.3\n)\n\n" +
		"require (\n\tgithub.com/nobody/chose-this v1.0.0 // indirect\n)\n"
	plan, mod, _ := fixture(t,
		"| Schema | santhosh-tekuri/jsonschema | v6.0.3 |", gomod, "")
	problems, err := check(plan, mod, "")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("an indirect requirement was reported: %v", problems)
	}
}

// A renumbered plan must stop the checker loudly rather than pass silently,
// which is what an empty section would do.
func TestARenumberedPlanFailsLoudlyInsteadOfPassing(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "PLAN.md")
	write(t, plan, "## 40. Tech stack\n\nthe table moved\n")
	mod := filepath.Join(dir, "go.mod")
	write(t, mod, oneRequire)

	if _, err := check(plan, mod, ""); err == nil {
		t.Fatal("the stack table was gone and the checker reported no problems")
	}
}
