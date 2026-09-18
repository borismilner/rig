package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// all is the stat a test uses when every named file is present.
func all(string) bool { return true }

// ⛔ THE UNTRACKED-DIRECTORY LINE IS THE FIRST CASE, BECAUSE IT IS THE ONE
// THAT BIT. Plain `git status --porcelain` collapses a new package to a single
// entry ending in `/`, so the tool's first run reported "nothing to gate"
// against a tree holding the tool itself. The fix is `--untracked-files=all`,
// which reports the FILES - and this test is what stops the flag being dropped
// as noise, because the failure is silent and looks like a clean tree.
func TestANewPackagesFilesAreGatedAndItsBareDirectoryIsNot(t *testing.T) {
	for _, c := range []struct {
		name string
		out  string
		want string
	}{
		{
			name: "the collapsed directory, which names no Go file",
			out:  "?? cmd/gate/\n",
			want: "",
		},
		{
			name: "the same tree under --untracked-files=all",
			out:  "?? cmd/gate/main.go\n?? cmd/gate/main_test.go\n",
			want: "./cmd/gate",
		},
		{
			name: "staged, unstaged and both, deduplicated by package",
			out:  "M  internal/record/plan.go\n M internal/record/section.go\nMM internal/record/backlog.go\n",
			want: "./internal/record",
		},
		{
			name: "a rename is gated where the file now is",
			out:  "R  internal/old/a.go -> internal/new/a.go\n",
			want: "./internal/new",
		},
		{
			name: "everything that is not a Go file",
			out:  " M README.md\n?? notes.txt\n M Makefile\n",
			want: "",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(dirsFromPorcelain(c.out, all), " ")
			if got != c.want {
				t.Errorf("dirsFromPorcelain(%q) = %q; want %q", c.out, got, c.want)
			}
		})
	}
}

// A deleted file leaves no package to gate, and asking go to build one is an
// error the seat did not cause.
func TestADeletedFileIsNotGated(t *testing.T) {
	gone := func(string) bool { return false }
	if got := dirsFromPorcelain(" D internal/record/plan.go\n", gone); len(got) != 0 {
		t.Errorf("dirsFromPorcelain = %v; a file that is not on disk has no package", got)
	}
}

// ⛔ A PACKAGE WITH NO TEST MUST BE NAMED, because `go test` prints nothing for
// it and the summary line then reads exactly like a package that passed.
func TestAPackageThatRanNoTestIsNamed(t *testing.T) {
	r := testRun{withTests: map[string]bool{"github.com/boris-milner/rig/internal/record": true}}
	got := r.untested([]string{"./internal/record", "./cmd/gate"})
	if strings.Join(got, " ") != "./cmd/gate" {
		t.Errorf("untested = %v; want only ./cmd/gate", got)
	}
	// POSITIVE CONTROL: with neither package reporting, both are named - so the
	// assertion above is about the matching and not about the list being empty.
	if got := (testRun{withTests: map[string]bool{}}).untested([]string{"./a", "./b"}); len(got) != 2 {
		t.Errorf("untested = %v; want both", got)
	}
}

// ⛔ AND THE FLAG ITSELF, AGAINST A REAL REPOSITORY. The parse test above
// cannot see `--untracked-files=all` being dropped, because it is fed
// porcelain output rather than a repository - a mutation that removed the flag
// left it green. This one runs git, so the defect that started all of this
// cannot come back silently.
func TestScopeSeesAWholeNewPackageInARealRepository(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("no usable git here: %v: %s", err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "brandnew"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "brandnew", "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	back, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(back) })

	pkgs, from, err := scope(context.Background(), nil)
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	if strings.Join(pkgs, " ") != "./cmd/brandnew" {
		t.Errorf("scope = %v (from %s); an untracked package was invisible to its own gate", pkgs, from)
	}
}
