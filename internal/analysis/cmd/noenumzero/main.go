// Command noenumzero fails the build on a proto enum whose zero value means
// something (PLAN.md section 21).
package main

import "github.com/boris-milner/rig/internal/analysis"

func main() { analysis.Main(analysis.NoEnumZero) }
