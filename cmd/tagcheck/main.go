// Command tagcheck fails when a release tag breaks the rules a program pins
// against (PLAN.md section 28, integration gap 9).
//
// WHAT GETS TAGGED. The repository is one Go module,
// github.com/borismilner/rig, and a tag versions all of it: the daemon, the
// CLI, the client a program links and the proto files a program in any other
// language generates from. So one tag is the one thing a program pins.
//
// THE FORMAT, for every tag that starts with "v":
//
//   - canonical semver, vMAJOR.MINOR.PATCH, as golang.org/x/mod/semver reads
//     it (no "v1.2" shorthand, no build metadata, which the Go toolchain
//     drops and so cannot pin);
//   - major v0 or v1: the module path carries no /vN suffix, and Go refuses a
//     v2+ tag on a module path without one, so such a tag could never be
//     fetched;
//   - a pre-release only as -mN (a milestone, section 23) or -rc.N;
//   - annotated, because section 28 records a milestone's demo in the tag's
//     message, and a lightweight tag has no message.
//
// Tags that do not start with "v" are not release tags and are listed, not
// judged. A clone with no tags at all passes and says so, because CI's
// shallow checkout often has none.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// prerelease is the only pre-release shapes a release tag may carry.
var prerelease = regexp.MustCompile(`^-(m\d+|rc\.\d+)$`)

// checkTag judges one tag. objectType is git's: "tag" for an annotated tag,
// "commit" for a lightweight one.
func checkTag(name, objectType string) error {
	if !semver.IsValid(name) || semver.Canonical(name) != name {
		return fmt.Errorf("%s is not canonical semver vMAJOR.MINOR.PATCH (build metadata is not allowed: Go drops it, so it cannot be pinned)", name)
	}
	if m := semver.Major(name); m != "v0" && m != "v1" {
		return fmt.Errorf("%s has major %s, but the module path github.com/borismilner/rig has no /%s suffix, so Go can never fetch it", name, m, m)
	}
	if pre := semver.Prerelease(name); pre != "" && !prerelease.MatchString(pre) {
		return fmt.Errorf("%s has pre-release %q; a release tag's pre-release is -mN (a milestone) or -rc.N", name, pre)
	}
	if objectType != "tag" {
		return fmt.Errorf("%s is a lightweight tag; release tags are annotated, so the message can record what was released", name)
	}
	return nil
}

func main() { os.Exit(run()) }

// run is main's body, returning the exit code so the deferred cancel runs.
func run() int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "for-each-ref", "refs/tags",
		"--format=%(refname:short) %(objecttype)").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tagcheck: listing tags:", err)
		return 2
	}
	var checked, bad int
	var other []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, typ, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if !strings.HasPrefix(name, "v") {
			other = append(other, name)
			continue
		}
		checked++
		if err := checkTag(name, typ); err != nil {
			fmt.Fprintln(os.Stderr, "tagcheck: "+err.Error())
			bad++
		}
	}
	if len(other) > 0 {
		fmt.Printf("tagcheck: not release tags, not judged: %s\n", strings.Join(other, ", "))
	}
	if bad > 0 {
		return 1
	}
	if checked == 0 {
		fmt.Println("tagcheck: no release tags in this clone, nothing to check")
		return 0
	}
	fmt.Printf("tagcheck: %d release tag(s), all pinnable\n", checked)
	return 0
}
