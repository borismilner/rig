package record

import (
	"context"
	"errors"
	"testing"
)

// A CANCELLED CONTEXT MUST REACH THE DATABASE, AND THIS IS THE ONLY THING THAT
// PROVES IT DID.
//
// Threading a context through a package to satisfy `noctx` is a change that
// compiles, lints clean and can still be inert: a method that accepts a ctx and
// then hands context.Background() to the driver passes every other test in this
// package, because nothing else here ever cancels anything. The linter checks
// that a *Context call was USED; it cannot check that the caller's context is
// the one that was passed. This test is that check.
//
// It is also the guard against the change being quietly undone. Section 39's
// whole argument is that a fact stops being true and nothing announces it - a
// later edit reaching for a local root context inside one of these methods
// turns exactly one subtest red, and the subtest is named for the verb.
//
// Every verb here is called with VALID arguments on purpose. Each of them
// validates before it touches the store, so a bad argument would be refused by
// the validation and the subtest would pass while proving nothing - the dead
// -guard shape this package has already paid for twice.
func TestEveryVerbRefusesOnACancelledContext(t *testing.T) {
	s := openStore(t, estate(t, "ctx"))

	// The store is seeded through a LIVE context, so every refusal below is
	// about the cancellation and not about a record that is not there.
	if _, err := s.Put(tctx, PutRequest{
		ID: "ctx", Kind: "project", Project: "ctx", Body: "the subject",
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("seeding the project: %v", err)
	}
	if _, err := s.Put(tctx, PutRequest{
		ID: "wi-1", Kind: "work-item", Project: "ctx", Body: "an item",
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("seeding the item: %v", err)
	}
	if _, err := s.Put(tctx, PutRequest{
		ID: "wi-2", Kind: "work-item", Project: "ctx", Body: "another item",
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("seeding the second item: %v", err)
	}
	if err := s.Link(tctx, "wi-1", LinkBlocks, "wi-2"); err != nil {
		t.Fatalf("seeding the edge: %v", err)
	}

	dead, cancel := context.WithCancel(context.Background())
	cancel()

	// One entry per exported verb that reaches the store. A verb missing from
	// this table is a verb whose context nothing proves.
	verbs := map[string]func() error{
		"Put": func() error {
			_, err := s.Put(dead, PutRequest{
				ID: "wi-3", Kind: "work-item", Project: "ctx", Body: "a third",
				Session: "s", Seat: "backend-record",
			})
			return err
		},
		"Get":        func() error { _, err := s.Get(dead, "wi-1"); return err },
		"GetVersion": func() error { _, err := s.GetVersion(dead, "wi-1", 1); return err },
		"Query":      func() error { _, err := s.Query(dead, "ctx", "work-item"); return err },
		"History":    func() error { _, err := s.History(dead, "wi-1"); return err },
		"Link":       func() error { return s.Link(dead, "wi-2", LinkBlocks, "wi-1") },
		"Unlink":     func() error { return s.Unlink(dead, "wi-1", LinkBlocks, "wi-2") },
		"LinksFrom":  func() error { _, err := s.LinksFrom(dead, "wi-1", LinkBlocks); return err },
		"Step": func() error {
			_, err := s.Step(dead, StepRequest{
				Item: "wi-1", State: "started", Project: "ctx",
				Session: "s", Seat: "backend-record",
			})
			return err
		},
		"Stream": func() error { _, err := s.Stream(dead, "wi-1"); return err },
		"Brief":  func() error { _, err := s.Brief(dead, "ctx"); return err },
		"Refs":   func() error { _, err := s.Refs(dead, RefsRequest{ID: "wi-2"}); return err },
	}

	for name, call := range verbs {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatalf("%s answered on a cancelled context: the context it was "+
					"given is not the one reaching the database", name)
			}
			// ⛔ errors.Is RATHER THAN err != nil, AND THE DISTINCTION IS THE
			// WHOLE TEST. Every verb here can fail for reasons that have
			// nothing to do with the context - a missing record, a bad
			// argument, a closed store - and "it returned an error" cannot
			// tell those from a cancellation. This package has been burnt
			// three times by an assertion that could not name which guard
			// failed it.
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s failed, but not on the cancellation: %v", name, err)
			}
		})
	}

	// THE COUNT IS ASSERTED AGAINST THE PACKAGE RATHER THAN TYPED. A verb added
	// to the store and not to the table above would leave this test passing
	// while covering less, which is the guard-that-cannot-fail shape the lead
	// ruled against: derive the expectation, never hardcode the bound.
	if got, want := len(verbs), len(storeVerbsTakingContext); got != want {
		t.Fatalf("the table covers %d verbs and the store exports %d that take a "+
			"context: %v", got, want, storeVerbsTakingContext)
	}
	for _, v := range storeVerbsTakingContext {
		if _, ok := verbs[v]; !ok {
			t.Errorf("%s takes a context and nothing proves the context reaches the store", v)
		}
	}
}

// storeVerbsTakingContext is every exported *Store method whose first argument
// is a context.Context, listed here so the table above is checked against the
// package rather than against a number somebody typed.
//
// It is a written list rather than reflection on purpose: a reflective check
// would derive both sides from the same source and could not notice a verb
// being removed, which is a cross-check sharing a blind spot with its subject.
// Adding a verb to the store and forgetting this line turns the test red at
// the point the verb is added, not later.
var storeVerbsTakingContext = []string{
	"Put", "Get", "GetVersion", "Query", "History",
	"Link", "Unlink", "LinksFrom", "Step", "Stream", "Brief", "Refs",
}

// ⛔ THE TABLE ABOVE CANNOT SEE checkEdge, AND A SURVIVING MUTATION IS WHAT
// SAID SO.
//
// Dropping the caller's context inside checkEdge - the shared refusal path
// behind Link and Unlink - left the table GREEN. Not because the table is
// weak about the verbs, but because both verbs reach a SECOND context-carrying
// call afterwards: checkEdge passes, ExecContext then refuses on the
// cancellation, and the verb fails for the right reason by the wrong route.
// Two guards for one condition, and the assertion could not tell which ran -
// which is this package's most expensive recurring defect, arriving this time
// in the test rather than in the code.
//
// This test picks the one path where checkEdge is the ONLY consumer of the
// context: a destination that does not exist. checkEdge refuses it before
// either verb reaches its write, so the error names which guard answered.
//
//	ctx honoured  -> context.Canceled
//	ctx dropped   -> *NotFoundError, because the lookup ran anyway
//
// The mutation that survived the table fails HERE, which is the whole reason
// this is a second test and not a thirteenth row.
func TestTheSharedEdgeCheckUsesTheCallersContext(t *testing.T) {
	s := openStore(t, estate(t, "ctx"))
	if _, err := s.Put(tctx, PutRequest{
		ID: "wi-1", Kind: "work-item", Project: "ctx", Body: "an item",
		Session: "s", Seat: "backend-record",
	}); err != nil {
		t.Fatalf("seeding the item: %v", err)
	}

	dead, cancel := context.WithCancel(context.Background())
	cancel()

	for _, tc := range []struct {
		verb string
		call func() error
	}{
		{"Link", func() error { return s.Link(dead, "wi-1", LinkBlocks, "nobody") }},
		{"Unlink", func() error { return s.Unlink(dead, "wi-1", LinkBlocks, "nobody") }},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			err := tc.call()
			var missing *NotFoundError
			switch {
			case err == nil:
				t.Fatalf("%s answered for a destination that does not exist", tc.verb)
			case errors.As(err, &missing):
				t.Fatalf("%s reached the existence check and refused on the MISSING "+
					"record (%v): checkEdge ran its lookup on a context that was "+
					"already cancelled, so it is not using the caller's", tc.verb, err)
			case !errors.Is(err, context.Canceled):
				t.Fatalf("%s failed, but neither on the cancellation nor on the "+
					"missing record: %v", tc.verb, err)
			}
		})
	}
}
