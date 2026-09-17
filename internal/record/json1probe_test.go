package record

import "testing"

// TestJSONEachIsAvailable proves the driver carries JSON1 before anything is
// built on it. A field predicate that assumes json_each and does not have it
// fails at run time against a real store and nowhere in the test suite.
func TestJSONEachIsAvailable(t *testing.T) {
	s := openStore(t, "json1probe")
	var k, v string
	err := s.db.QueryRowContext(tctx,
		`SELECT je.key, je.value FROM json_each(?) je WHERE je.key = ?`,
		`{"status":"active","owner":"boris"}`, "owner").Scan(&k, &v)
	if err != nil {
		t.Fatalf("json_each unavailable in this driver: %v", err)
	}
	if k != "owner" || v != "boris" {
		t.Fatalf("json_each returned %q=%q, want owner=boris", k, v)
	}
	t.Logf("json_each present: %s=%s", k, v)

	// ⛔ THE NEGATIVE CONTROL, AND IT IS WHY THE GREEN ABOVE MEANS ANYTHING. A
	// probe that only ever asks for a key that IS there cannot tell json_each
	// from a function that returns the first row of anything.
	var junk string
	err = s.db.QueryRowContext(tctx,
		`SELECT je.value FROM json_each(?) je WHERE je.key = ?`,
		`{"status":"active","owner":"boris"}`, "absent").Scan(&junk)
	if err == nil {
		t.Fatalf("json_each matched a key that is not in the document, "+
			"returning %q - so this probe cannot tell a hit from a miss", junk)
	}
}
