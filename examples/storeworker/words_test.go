package main

import (
	"encoding/json"
	"regexp"
	"testing"
)

// Every word a tab lists is explained in WORDS (plan/48, R40), so no tab
// shows "not explained yet".
func TestEveryWordATabUsesIsExplained(t *testing.T) {
	explained := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^  "([^"]+)": "`).FindAllStringSubmatch(page, -1) {
		explained[m[1]] = true
	}
	lists := regexp.MustCompile(`terms: (\[[^\]]*\])`).FindAllStringSubmatch(page, -1)
	if len(explained) < 50 || len(lists) < 12 {
		t.Fatalf("found %d words and %d tab lists; the page's shape changed under this test", len(explained), len(lists))
	}
	for _, l := range lists {
		var terms []string
		if err := json.Unmarshal([]byte(l[1]), &terms); err != nil {
			t.Fatal(err)
		}
		for _, w := range terms {
			if !explained[w] {
				t.Errorf("a tab uses %q, and WORDS does not explain it", w)
			}
		}
	}
}
