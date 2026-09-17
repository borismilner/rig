package record

import "testing"

// TestJSONEachIsAvailable proves the driver carries JSON1 before anything is
// built on it. A field predicate that assumes json_each and does not have it
// fails at run time against a real store and nowhere in the test suite.
// ⛔ AND IT GOES THROUGH estate(), WHICH IT DID NOT UNTIL 2026-09-17. openStore
// only OPENS; estate() is the half that sets XDG_STATE_HOME to a temp dir, so
// this probe was resolving the real one and every `go test ./internal/record/`
// on this machine wrote a 37K store into the developer's live
// ~/.local/state/rig/estates/. Measured, not reasoned: the directory
// `json1probe` was found there beside `production` and `development`, with the
// mtime of the test run that made it.
//
// ⛔ THE INJURY IS NOT THE STRAY DIRECTORY, IT IS THE NAME BEING A FREE
// PARAMETER ONE TYPO AWAY FROM `production`. Nothing between this call and the
// real store would have refused it. That is the same false containment
// COORDINATION.md records for XDG_RUNTIME_DIR and D-Bus - a private-looking
// setting that isolates a DIFFERENT resource from the one being written.
func TestJSONEachIsAvailable(t *testing.T) {
	s := openStore(t, estate(t, "json1probe"))
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
