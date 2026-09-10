// Command noregistryhandle fails the build on a registry handle reachable
// from outside the kernel (PLAN.md section 14).
package main

import "github.com/boris-milner/rig/internal/analysis"

func main() { analysis.Main(analysis.NoRegistryHandle) }
