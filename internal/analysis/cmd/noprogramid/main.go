// Command noprogramid fails the build on a program's identity appearing in
// rig's own code (PLAN.md sections 5h, 5i, 29).
package main

import "github.com/borismilner/rig/internal/analysis"

func main() { analysis.Main(analysis.NoProgramID) }
