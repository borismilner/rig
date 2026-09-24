package main

import "testing"

func TestTheTagRules(t *testing.T) {
	for _, tc := range []struct {
		name, typ string
		ok        bool
	}{
		{"v0.0.0-m0", "tag", true},
		{"v0.1.0", "tag", true},
		{"v1.4.2", "tag", true},
		{"v1.5.0-rc.1", "tag", true},
		{"v0.3.0-m7", "tag", true},
		{"v0.1.0", "commit", false},      // lightweight
		{"v1.2", "tag", false},           // shorthand
		{"v2.0.0", "tag", false},         // no /v2 in the module path
		{"v1.0.0+build.5", "tag", false}, // build metadata
		{"v1.0.0-beta", "tag", false},    // not -mN or -rc.N
		{"v1.0.0-rc1", "tag", false},     // -rc.N, with the dot
		{"version-1", "tag", false},      // not semver at all
	} {
		err := checkTag(tc.name, tc.typ)
		if (err == nil) != tc.ok {
			t.Errorf("checkTag(%q, %q) = %v, want ok=%v", tc.name, tc.typ, err, tc.ok)
		}
	}
}
