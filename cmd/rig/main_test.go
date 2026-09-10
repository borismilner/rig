package main

import (
	"slices"
	"testing"
)

// Section 10 promises --json on everything. Go's flag package stops at the
// first positional, so this is the test that keeps the promise independent of
// argument order.
func TestPartitionAcceptsFlagsAnywhere(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         []string
		wantFlags  []string
		wantPosArg []string
	}{
		{
			"flag after positional",
			[]string{"fakeapp", "--json"},
			[]string{"--json"},
			[]string{"fakeapp"},
		},
		{
			"flag before positional",
			[]string{"--json", "fakeapp"},
			[]string{"--json"},
			[]string{"fakeapp"},
		},
		{
			"valued flag after",
			[]string{"fakeapp", "--timeout", "9s"},
			[]string{"--timeout", "9s"},
			[]string{"fakeapp"},
		},
		{
			"valued flag with equals",
			[]string{"fakeapp", "--timeout=9s"},
			[]string{"--timeout=9s"},
			[]string{"fakeapp"},
		},
		{
			"bool flag does not eat the positional",
			[]string{"--json", "fakeapp"},
			[]string{"--json"},
			[]string{"fakeapp"},
		},
		{
			"double dash ends flags",
			[]string{"--json", "--", "-weird-name"},
			[]string{"--json"},
			[]string{"-weird-name"},
		},
		{"nothing", nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotFlags, gotPos := partition(tc.in)
			if !slices.Equal(gotFlags, tc.wantFlags) {
				t.Errorf("flags = %q, want %q", gotFlags, tc.wantFlags)
			}
			if !slices.Equal(gotPos, tc.wantPosArg) {
				t.Errorf("positional = %q, want %q", gotPos, tc.wantPosArg)
			}
		})
	}
}
