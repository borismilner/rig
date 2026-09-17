package record

import (
	"errors"
	"testing"
)

// fieldFixture writes six records across two projects and three kinds so that
// every one of the four field-bearing query shapes has something it must
// INCLUDE and something it must EXCLUDE.
//
// ⛔ THE EXCLUSIONS ARE THE POINT AND THEY ARE WHY THIS IS NOT THREE RECORDS. A
// predicate that is silently dropped returns a superset, and a superset built
// from a fixture with nothing to exclude is indistinguishable from a correct
// answer. Every case below asserts the whole SET, never a count: a count
// matching expectation is the least informative outcome available, and this
// package has a recorded instance of exactly that hiding a swap.
func fieldFixture(t *testing.T, s *Store) {
	t.Helper()
	write := func(id, kind, project string, fields map[string]string) {
		t.Helper()
		if _, err := s.Put(tctx, PutRequest{
			ID: id, Kind: kind, Project: project, Body: "b",
			Fields: fields, Session: "s", Seat: "test",
		}); err != nil {
			t.Fatalf("writing %s: %v", id, err)
		}
	}
	write("F1", "work-item", "alpha", map[string]string{"status": "active", "owner": "boris"})
	write("F2", "work-item", "alpha", map[string]string{"status": "closed", "owner": "boris"})
	write("F3", "work-item", "beta", map[string]string{"status": "active", "owner": "lead"})
	write("F4", "decision", "alpha", map[string]string{"status": "active"})
	write("F5", "decision", "beta", map[string]string{"status": "closed"})
	// F6 carries the field with an EMPTY value, which is the case the
	// empty-means-every rule must NOT swallow.
	write("F6", "work-item", "alpha", map[string]string{"status": "", "owner": "boris"})
}

func recIDs(t *testing.T, recs []Record) []string {
	t.Helper()
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

func sameIDs(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v, want %v", what, got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: got %v, want %v", what, got, want)
			return
		}
	}
}

// TestFindNarrowsByFieldInAllFourShapes is B65: section 39 defines record.query
// as "by kind, field and project" and the field predicate did not exist, so
// every field the specification's longest passage describes was write-only.
func TestFindNarrowsByFieldInAllFourShapes(t *testing.T) {
	s := openStore(t, estate(t, "findbyfield"))
	fieldFixture(t, s)

	for _, c := range []struct {
		what string
		f    QueryFilter
		want []string
	}{
		{
			// Field alone, across every project and kind. The order is
			// findOrder's - project, then kind, then id - so alpha/decision
			// precedes alpha/work-item and beta comes last. Asserted rather
			// than sorted, because the order IS part of the answer: an
			// unscoped read is only readable if it is grouped.
			"status=active everywhere",
			QueryFilter{Field: "status", Value: "active"},
			[]string{"F4", "F1", "F3"},
		},
		{
			// ⛔ EXCLUDES F3 (wrong project) AND F2/F6 (wrong value).
			"status=active in alpha",
			QueryFilter{Project: "alpha", Field: "status", Value: "active"},
			[]string{"F4", "F1"},
		},
		{
			// ⛔ EXCLUDES F4 (wrong kind) AND F2/F6 (wrong value).
			"status=active work-items in every project",
			QueryFilter{Kind: "work-item", Field: "status", Value: "active"},
			[]string{"F1", "F3"},
		},
		{
			// All three predicates, and each one is load-bearing: dropping the
			// project admits F3, dropping the kind admits F4, dropping the
			// field admits F2 and F6.
			"status=active work-items in alpha",
			QueryFilter{Project: "alpha", Kind: "work-item", Field: "status", Value: "active"},
			[]string{"F1"},
		},
		{
			// ⛔ THE EMPTY VALUE IS A REAL VALUE HERE, UNLIKE Project AND Kind.
			// This is the assertion that stops somebody "tidying" the
			// asymmetry away: if empty meant "every value", this would return
			// all six.
			"status is literally the empty string",
			QueryFilter{Field: "status", Value: ""},
			[]string{"F6"},
		},
		{
			// A field only some records carry.
			"owner=boris",
			QueryFilter{Field: "owner", Value: "boris"},
			[]string{"F1", "F2", "F6"},
		},
		{
			"a field no record carries answers empty, not everything",
			QueryFilter{Field: "priority", Value: "high"},
			nil,
		},
		{
			"a value no record has answers empty",
			QueryFilter{Field: "status", Value: "invented"},
			nil,
		},
	} {
		got, err := s.Find(tctx, c.f)
		if err != nil {
			t.Fatalf("%s: %v", c.what, err)
		}
		sameIDs(t, c.what, recIDs(t, got), c.want)
	}
}

// TestFindRefusesAValueWithNoFieldToMatchIt is the refusal, and the reason it
// is a refusal rather than a shrug is that the permissive reading WIDENS the
// answer: a caller whose field name expanded to nothing gets every record and
// no indication that its predicate vanished.
func TestFindRefusesAValueWithNoFieldToMatchIt(t *testing.T) {
	s := openStore(t, estate(t, "findvaluenofield"))
	fieldFixture(t, s)

	got, err := s.Find(tctx, QueryFilter{Value: "active"})
	if !errors.Is(err, ErrValueWithoutField) {
		t.Fatalf("Find with a value and no field returned %d records and err %v, "+
			"want ErrValueWithoutField - dropping the predicate silently is how "+
			"a narrow query becomes a full table read that looks like it worked",
			len(got), err)
	}
	if got != nil {
		t.Errorf("the refusal also returned %d records; it must return none", len(got))
	}
}

// TestTheFieldPredicateComposesWithTheIndexedOnes proves the four field-bearing
// constants are reached at all, by checking each returns something DIFFERENT
// from the unfiltered read.
//
// ⛔ THIS IS THE WATCHED-RED ARM. Without it, a switch that fell through to
// findEverything in all four field cases would pass every assertion above that
// happens to want the whole store, and this package's own history says that is
// the shape which survives review.
func TestTheFieldPredicateComposesWithTheIndexedOnes(t *testing.T) {
	s := openStore(t, estate(t, "findcompose"))
	fieldFixture(t, s)

	all, err := s.Find(tctx, QueryFilter{})
	if err != nil {
		t.Fatalf("unfiltered: %v", err)
	}
	if len(all) != 6 {
		t.Fatalf("the fixture is %d records, want 6 - the rest of this test "+
			"compares against that number", len(all))
	}

	for _, f := range []QueryFilter{
		{Field: "status", Value: "active"},
		{Project: "alpha", Field: "status", Value: "active"},
		{Kind: "work-item", Field: "status", Value: "active"},
		{Project: "alpha", Kind: "work-item", Field: "status", Value: "active"},
	} {
		got, err := s.Find(tctx, f)
		if err != nil {
			t.Fatalf("%s: %v", f.describe(), err)
		}
		if len(got) == len(all) {
			t.Errorf("%s returned all %d records, so its predicate did nothing - "+
				"the switch fell through to an unfiltered query",
				f.describe(), len(all))
		}
	}
}

// TestAFieldNameIsNeverInterpreted is the reason this predicate uses json_each
// rather than json_extract: a json_extract path has to be built by concatenating
// the caller's field name, and a name containing a dot or a quote then means
// something other than itself.
//
// ⛔ THIS TEST IS THE WHOLE ARGUMENT FOR THE QUERY SHAPE. If somebody moves the
// predicate to `json_extract(r.fields, '$.' || ?)` it goes red, which is exactly
// when it should.
func TestAFieldNameIsNeverInterpreted(t *testing.T) {
	s := openStore(t, estate(t, "findweirdkeys"))
	if _, err := s.Put(tctx, PutRequest{
		ID: "W1", Kind: "work-item", Project: "alpha", Body: "b",
		Fields: map[string]string{
			"a.b":      "dotted",
			"nested":   "flat",
			`quo"ted`:  "quoted",
			"$.escape": "dollar",
		},
		Session: "s", Seat: "test",
	}); err != nil {
		t.Fatalf("writing W1: %v", err)
	}
	// A record that WOULD be reached if `a.b` were treated as a path into a
	// nested object. It is flat here, so a path reading finds nothing and an
	// exact-key reading finds W1 - the two answers differ, which is what makes
	// this discriminating.
	if _, err := s.Put(tctx, PutRequest{
		ID: "W2", Kind: "work-item", Project: "alpha", Body: "b",
		Fields: map[string]string{"a": "not a container"}, Session: "s", Seat: "test",
	}); err != nil {
		t.Fatalf("writing W2: %v", err)
	}

	// W3 carries an ordinary field name that SQL's LIKE wildcards would reach
	// from a pattern, which is what makes the negative cases below bite.
	if _, err := s.Put(tctx, PutRequest{
		ID: "W3", Kind: "work-item", Project: "alpha", Body: "b",
		Fields: map[string]string{"owner": "boris"}, Session: "s", Seat: "test",
	}); err != nil {
		t.Fatalf("writing W3: %v", err)
	}

	for _, c := range []struct{ key, value, want string }{
		{"a.b", "dotted", "W1"},
		{`quo"ted`, "quoted", "W1"},
		{"$.escape", "dollar", "W1"},
	} {
		got, err := s.Find(tctx, QueryFilter{Field: c.key, Value: c.value})
		if err != nil {
			t.Fatalf("field %q: %v", c.key, err)
		}
		if len(got) != 1 || got[0].ID != c.want {
			t.Errorf("field %q=%q matched %v, want exactly [%s] - a field name "+
				"is compared as a string and is never parsed as a path",
				c.key, c.value, recIDs(t, got), c.want)
		}
	}

	// ⛔ THE NEGATIVE CASES, AND THEY WERE MISSING. A mutation swapping the
	// key comparison from `=` to `LIKE` SURVIVED the first version of this
	// test: every case above asks for a name that exists, and `LIKE 'a.b'`
	// matches 'a.b' perfectly well. What separates the two operators is a
	// pattern that matches NOTHING literally - so these ask for names no
	// record carries, which `=` answers empty and `LIKE` answers with W3.
	//
	// This is the "make every check fail on purpose" rule arriving one level
	// in: the assertions were all satisfiable by the broken code.
	for _, pattern := range []string{"own%", "o_ner", "%", "_____"} {
		got, err := s.Find(tctx, QueryFilter{Field: pattern, Value: "boris"})
		if err != nil {
			t.Fatalf("field %q: %v", pattern, err)
		}
		if len(got) != 0 {
			t.Errorf("field %q=boris matched %v, want nothing - no record has a "+
				"field CALLED %q, so a match means the key is being treated as a "+
				"LIKE pattern rather than compared as a string",
				pattern, recIDs(t, got), pattern)
		}
	}
}
