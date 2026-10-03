#!/bin/sh
# Regenerates rig-self.json: rig's own commands as internal/daemon/self.go
# declares them, with each bridged verb's argument schema. The mockup reads
# it so the Capabilities panel shows rig's real words (plan/55 req 28).
# It works on a throwaway copy, so the tree is never touched.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
rsync -a --exclude .git "$root/" "$tmp/"
cat > "$tmp/internal/daemon/zz_dump_test.go" <<'GO'
package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestZZDumpSelf(t *testing.T) {
	inputs := map[string]any{}
	for _, v := range (&mcpCaller{}).Verbs() {
		inputs[v.Command] = v.Input
	}
	type cmd struct {
		ID, Title, Summary, Description, Returns, Effects string
		Args                                               any
	}
	var out []cmd
	for _, c := range selfDeclaration().Commands {
		var args any
		if in, ok := inputs[c.ID]; ok {
			args = in
		} else if len(c.Args) > 0 {
			_ = json.Unmarshal(c.Args, &args)
		}
		out = append(out, cmd{c.ID, c.Title, c.Summary, c.Description, c.Returns, fmt.Sprint(c.Effects), args})
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("DUMP"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
GO
(cd "$tmp" && DUMP="$here/rig-self.json" go test ./internal/daemon -run TestZZDumpSelf -count=1 >/dev/null)
echo "wrote $here/rig-self.json"
