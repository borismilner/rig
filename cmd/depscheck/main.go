// Command depscheck fails if the build depends on something the plan does not
// name, or pins it at a version the plan disagrees with.
//
// PLAN.md section 22 is the stack table, and it is a decision record rather
// than a lockfile: most of its rows are milestones that have not arrived, so
// a name in the table with nothing in go.mod is normal and is NOT reported.
// The check runs the other way.
//
// THE ASYMMETRY IS THE WHOLE RULE. The plan may run ahead of the code; the
// code may not run ahead of the plan. A dependency that appears in a build
// without a row in section 22 is the case section 22 exists to prevent - it
// is how a library arrives unmeasured, unattributed to a binary, and unpinned
// by anybody's decision.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	plan := flag.String("plan", "PLAN.md", "the plan whose stack table is the contract")
	mod := flag.String("gomod", "go.mod", "the Go module file")
	npm := flag.String("npm", "frontend/package.json", "the frontend manifest, or empty to skip")
	flag.Parse()

	problems, err := check(*plan, *mod, *npm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "depscheck: %v\n", err)
		os.Exit(2)
	}
	if len(problems) == 0 {
		fmt.Println("depscheck: every pinned dependency is named in the stack table")
		return
	}

	fmt.Fprintf(os.Stderr, "depscheck: %d dependencies disagree with the stack table\n\n", len(problems))
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "  %s\n", p)
	}
	fmt.Fprintf(os.Stderr, "\nEither add the row to the stack table, or take the dependency out.\n"+
		"A row is a decision: which binary carries it, what it costs, and what it\n"+
		"replaces. PLAN.md is owned by the plan role, under the `rig-plan` lock.\n")
	os.Exit(1)
}
