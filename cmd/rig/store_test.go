package main

import "testing"

// --where takes JSON scalars as JSON and anything else as a string, so a
// plain word needs no inner quotes and "3" can still mean the string.
func TestWhereReadsAScalarOrAPlainString(t *testing.T) {
	for in, want := range map[string]string{
		"state eq done":        `"done"`,
		"cost ge 3":            `3`,
		`id eq "3"`:            `"3"`,
		"ok eq true":           `true`,
		"meta eq null":         `null`,
		"title eq two words":   `"two words"`,
		`note eq it's "quoted`: `"it's \"quoted"`,
	} {
		c, err := parseWhere(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if c.GetValue() != want {
			t.Errorf("%q value = %s, want %s", in, c.GetValue(), want)
		}
	}
	for _, bad := range []string{"state", "state eq", `m eq {"a":1}`, "l eq [1]", ""} {
		if _, err := parseWhere(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestStoreDeleteNeedsTheVersionRead(t *testing.T) {
	s := storeFlagSet()
	*s.program = "graft"
	if _, err := s.request("delete", []string{"runs", "r1"}); err == nil {
		t.Fatal("a delete with no --if-version was built")
	}
}
