// Command gate answers one question the repo-wide gates cannot: is what I
// touched clean.
//
// ⛔ THE PROBLEM IS SHARED-TREE ATTRIBUTION, NOT SPEED. `make ci` and
// `make lint` run over the whole module, so on a tree several seats share, a
// seat cannot tell its own red from a peer's, and the only way to find out has
// been to stand up a detached worktree and re-run. Measured at ~15 minutes on
// one seat, paid independently by four seats across two days. BACKLOG.md B84.
//
// ⛔ AND IT MUST NEVER BE MISTAKEN FOR THE GATE. A scoped answer that reads
// like a full one is worse than no answer: it is this project's own
// missing-row failure wearing a green tick. Every run prints what it did NOT
// cover, by name, and the exit code says nothing about those.
//
// ⛔ A SKIP IS NOT A PASS, AND `go test` PRINTS `ok` FOR BOTH. The pin tests in
// internal/record read documents through gitignored symlinks; without the
// logbook beside rig they SKIP, and the skip reads as `ok` - the house defect
// inside the very instrument used to attribute a red. Every skipped test is
// named here.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// notCovered is what a scoped run cannot answer, named so the report can say
// so rather than leaving the reader to assume. Each is a `make ci` step that
// reads the whole module by construction.
var notCovered = []struct{ target, why string }{
	{"lint-house", "the four house analyzers walk the filesystem, not packages"},
	{"schema-check", "the committed schema is one artefact for the whole module"},
	{"deps-check", "the dependency stack is the module's, not a package's"},
	{"theme-gate", "the palette is one artefact"},
	{"a peer's packages", "only what you named, or what git reports dirty"},
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "gate: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	race bool
	lint bool
}

func run(ctx context.Context) error {
	var o options
	flag.BoolVar(&o.race, "race", true, "run the tests under the race detector, as ci does")
	flag.BoolVar(&o.lint, "lint", true, "run golangci-lint over the same packages")
	flag.Parse()

	pkgs, from, err := scope(ctx, flag.Args())
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Printf("gate: nothing to gate - %s reports no Go file.\n", from)
		fmt.Println("      This is an answer, not a pass: name a package to gate it anyway.")
		return nil
	}

	fmt.Printf("gating %d package(s), from %s:\n", len(pkgs), from)
	for _, p := range pkgs {
		fmt.Println("  " + p)
	}
	fmt.Println()

	var failed []string
	if out, err := unformatted(ctx, pkgs); err != nil {
		return err
	} else if len(out) > 0 {
		failed = append(failed, "gofmt")
		fmt.Println("⛔ UNFORMATTED:")
		for _, f := range out {
			fmt.Println("   " + f)
		}
	} else {
		fmt.Println("✅ gofmt      clean")
	}

	if err := step(ctx, "go", append([]string{"vet"}, pkgs...)...); err != nil {
		failed = append(failed, "vet")
		fmt.Println("⛔ VET        failed (output above)")
	} else {
		fmt.Println("✅ vet        clean")
	}

	res, err := runTests(ctx, o.race, pkgs)
	if err != nil {
		return err
	}
	if len(res.failed) > 0 {
		failed = append(failed, "test")
	}
	res.report(o.race, pkgs)

	if o.lint {
		switch err := step(ctx, "golangci-lint", append([]string{"run"}, pkgs...)...); {
		case err == nil:
			fmt.Println("✅ lint       0 issues")
		case errors.Is(err, exec.ErrNotFound):
			// ⛔ NOT RUN IS NOT CLEAN. COORDINATION.md records a third outcome
			// beside clean and dirty - golangci-lint can DECLINE to run - and a
			// seat reading only the exit code calls either one a result.
			fmt.Println("⚠️  lint       NOT RUN: golangci-lint is not installed. This is not a clean lint.")
		default:
			failed = append(failed, "lint")
			fmt.Println("⛔ LINT       failed or declined to run (output above)")
		}
	}

	fmt.Println()
	fmt.Println("NOT COVERED by this run, and the exit code says nothing about them:")
	for _, n := range notCovered {
		fmt.Printf("  %-16s %s\n", n.target, n.why)
	}
	fmt.Println()
	if len(failed) > 0 {
		return fmt.Errorf("%s red on the packages you named: %s",
			plural(len(failed), "step is", "steps are"), strings.Join(failed, ", "))
	}
	fmt.Println("✅ what you touched is clean. `make ci` is still the gate.")
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// scope answers the package patterns to gate and where they came from. Named
// arguments win; otherwise the working tree's own dirty Go files decide, which
// is the question a seat is actually asking.
func scope(ctx context.Context, args []string) (pkgs []string, from string, err error) {
	if len(args) > 0 {
		return args, "the arguments you gave", nil
	}
	// ⛔ `--untracked-files=all` OR A NEW PACKAGE IS INVISIBLE TO ITS OWN GATE.
	// Plain `--porcelain` collapses an untracked DIRECTORY to one entry ending
	// in `/`, so the first run of this tool reported "nothing to gate" against
	// a tree holding the tool. Found by running it rather than by reading it.
	out, err := output(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, "", fmt.Errorf("asking git what is dirty: %w", err)
	}
	return dirsFromPorcelain(out, exists), "the Go files git reports dirty", nil
}

// exists is dirsFromPorcelain's real answer to "is this file still there".
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dirsFromPorcelain reads `git status --porcelain` and answers the package
// patterns holding a dirty Go file. It takes its own stat so the parse can be
// tested without a repository.
func dirsFromPorcelain(out string, stat func(string) bool) []string {
	dirs := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		// `XY path`, and a rename carries `old -> new`. The new name is what
		// exists on disk, so it is the one that can be gated.
		path := strings.TrimSpace(line[3:])
		if i := strings.LastIndex(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		path = strings.Trim(path, `"`)
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if !stat(path) {
			continue // deleted: there is no package left to gate
		}
		dirs["./"+filepath.Dir(path)] = true
	}
	pkgs := make([]string, 0, len(dirs))
	for d := range dirs {
		pkgs = append(pkgs, d)
	}
	sort.Strings(pkgs)
	return pkgs
}

// unformatted lists the files gofmt would change, over the same packages.
func unformatted(ctx context.Context, pkgs []string) ([]string, error) {
	args := []string{"-l"}
	for _, p := range pkgs {
		args = append(args, strings.TrimPrefix(p, "./"))
	}
	out, err := output(ctx, "gofmt", args...)
	if err != nil {
		return nil, fmt.Errorf("running gofmt: %w", err)
	}
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

type testRun struct {
	passed  int
	failed  []string
	skipped []string
	// withTests is every package that emitted at least one test event, so a
	// package holding NO TEST AT ALL can be named. `go test` prints
	// `? pkg [no test files]` for it, and the summary line of a scoped run
	// would otherwise read exactly like a package that passed.
	withTests map[string]bool
}

// runTests runs the packages and reads the JSON stream, because the line
// `go test` prints for a package that skipped every test is `ok`.
func runTests(ctx context.Context, race bool, pkgs []string) (testRun, error) {
	args := []string{"test", "-json", "-count=1"}
	if race {
		args = append(args, "-race")
	}
	args = append(args, pkgs...)

	cmd := exec.CommandContext(ctx, "go", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return testRun{}, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return testRun{}, fmt.Errorf("starting the tests: %w", err)
	}

	res := testRun{withTests: map[string]bool{}}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var ev struct {
			Action  string `json:"Action"`
			Package string `json:"Package"`
			Test    string `json:"Test"`
			Output  string `json:"Output"`
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue // a non-JSON line is a build error, and Stderr carries it
		}
		if ev.Test == "" {
			continue // a package-level verdict, which the test lines already say
		}
		res.withTests[ev.Package] = true
		switch ev.Action {
		case "pass":
			res.passed++
		case "fail":
			res.failed = append(res.failed, ev.Package+"."+ev.Test)
		case "skip":
			res.skipped = append(res.skipped, ev.Package+"."+ev.Test)
		}
	}
	// A non-zero exit is the failures, which are already counted.
	_ = cmd.Wait()
	return res, sc.Err()
}

func (r testRun) report(race bool, pkgs []string) {
	how := "test"
	if race {
		how = "test -race"
	}
	switch {
	case len(r.failed) > 0:
		fmt.Printf("⛔ %-10s %d passed, %d FAILED\n", how, r.passed, len(r.failed))
		for _, f := range r.failed {
			fmt.Println("   " + f)
		}
	default:
		fmt.Printf("✅ %-10s %d passed, 0 failed\n", how, r.passed)
	}
	if bare := r.untested(pkgs); len(bare) > 0 {
		fmt.Printf("⚠️  %d package(s) hold NO TEST AT ALL, which is not a pass either:\n", len(bare))
		for _, p := range bare {
			fmt.Println("   " + p)
		}
	}
	if len(r.skipped) > 0 {
		// ⛔ NAMED, NEVER COUNTED ALONE. A skipped pin test is the shape of a
		// check that cannot go red, and `ok` is what go test prints for it.
		fmt.Printf("⚠️  %d test(s) SKIPPED, and a skip is not a pass:\n", len(r.skipped))
		for _, s := range r.skipped {
			fmt.Println("   " + s)
		}
	}
}

// untested is the packages that were gated and emitted no test event.
func (r testRun) untested(pkgs []string) []string {
	var bare []string
	for _, p := range pkgs {
		dir := strings.TrimPrefix(p, "./")
		found := false
		for seen := range r.withTests {
			if seen == dir || strings.HasSuffix(seen, "/"+dir) {
				found = true
				break
			}
		}
		if !found {
			bare = append(bare, p)
		}
	}
	sort.Strings(bare)
	return bare
}

func step(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}
