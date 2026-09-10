// Command nocontextfree fails the build on a call that could run without a
// deadline (PLAN.md sections 3, 20).
package main

import "github.com/boris-milner/rig/internal/analysis"

func main() { analysis.Main(analysis.NoContextFree) }
