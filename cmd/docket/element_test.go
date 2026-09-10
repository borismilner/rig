package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The declaration and the page are two halves of one claim written in two
// files. main.go tells rig which elements this program uses; page.go is where
// it uses them. Nothing connects them, and the failure is silent in the
// direction that matters: DROP an element from the page and the declaration
// still names it, so rig is told about a dependency that no longer exists and
// every check downstream of R3 is checking a fiction.
//
// The other direction is caught for us. Adding an element to the page without
// declaring it is what R7 refuses at registration - but only for a name rig
// does not serve, and every name in the kit is one rig serves. So an undeclared
// rigPanel would sail through the handshake and be missing from the one field
// section 5e added to carry it.
//
// This is the test that connects them, and it reads the page rather than a
// second list, because a second list is the thing it exists to prevent.
//
// It reads the kit.js import ONLY. page.go also imports rigPane from pane.js,
// and that is correctly absent from the declaration: pane.js is the transport
// every embedded page needs and it is not an element. Section 5h's split is
// exactly this - pane.js alone is the embedded tier, pane.js plus kit.js is
// the kit tier - so a test that counted rigPane as an element would be
// contradicting the thing the kit's own file layout was arranged to say.
var kitImport = regexp.MustCompile(`import\s*\{([^}]*)\}\s*from\s*"/kit/kit\.js"`)

func TestTheDeclaredElementsAreTheOnesThePageImports(t *testing.T) {
	src, err := os.ReadFile("page.go")
	if err != nil {
		t.Fatalf("reading the page this declaration describes: %v", err)
	}

	m := kitImport.FindSubmatch(src)
	if m == nil {
		t.Fatal("page.go no longer imports from /kit/kit.js in a shape this " +
			"test can read. If the page stopped using the kit, the declaration " +
			"in main.go has to stop naming elements; if only the import's " +
			"shape changed, fix the pattern here.")
	}

	var imported []string
	for _, name := range strings.Split(string(m[1]), ",") {
		if name = strings.TrimSpace(name); name != "" {
			imported = append(imported, name)
		}
	}
	sort.Strings(imported)

	declared := append([]string(nil), declaration("docket", "").GetElements()...)
	sort.Strings(declared)

	if strings.Join(imported, ",") != strings.Join(declared, ",") {
		t.Errorf("the declaration and the page disagree about which elements "+
			"this program uses.\n  page.go imports: %v\n  main.go declares: %v\n"+
			"Section 5h R3 makes the declaration the one authority, so it has to "+
			"be the truth about the page and not a guess at it.", imported, declared)
	}
}

// Every declared name must also be one rig serves, which is R7's own check.
// Asserting it here as well is not duplication: the kernel proves the RULE
// holds, and this proves THIS PROGRAM satisfies it, without needing a daemon.
// A fake application that cannot register is not a fake application.
func TestEveryDeclaredElementIsOneRigServes(t *testing.T) {
	served := map[string]bool{"rigPanel": true, "rigTable": true, "rigToolbar": true}
	for _, name := range declaration("docket", "").GetElements() {
		if !served[name] {
			t.Errorf("declares %q, which rig does not serve: registration would "+
				"be refused and this program would not start (section 5h R7)", name)
		}
	}
}
