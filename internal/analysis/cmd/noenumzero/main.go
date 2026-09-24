// Command noenumzero fails the build on a proto enum whose zero value means
// something (PLAN.md section 21).
package main

import "github.com/borismilner/rig/internal/analysis"

func main() { analysis.Main(analysis.NoEnumZero) }
