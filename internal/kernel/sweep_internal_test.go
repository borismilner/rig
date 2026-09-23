package kernel

import (
	"testing"
	"time"
)

// TestExpiredDeparturesAreActuallyDropped is the one test in this package that
// reaches inside, and the reason is that the property it checks is invisible
// from outside ON PURPOSE.
//
// Departed reports an expired departure as absent without deleting it, so no
// black-box test can tell a swept map from one that grows for the life of the
// daemon: both answer "no" to every question. A mutation that stops sweep
// deleting therefore passes the whole external suite, which is how this test
// came to be written.
//
// It matters because zero cost at rest is a REQUIREMENT here rather than a
// nice property: an estate where nothing ever dies pays nothing, and one with
// churn pays for the window and no longer. Without the sweep the second half
// is false and nothing says so.
func TestExpiredDeparturesAreActuallyDropped(t *testing.T) {
	k := New()
	k.RememberDeparturesFor(time.Millisecond)

	for _, id := range []string{"shelf", "pilot", "satchel"} {
		p, err := k.Register(Principal{
			UID: 1000, Kind: KindProgram,
			ClientID: id, SessionID: "s-" + id, PID: 42,
		}, decl(id))
		if err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
		k.Deregister(p.SessionID)
	}
	if got := len(k.registry.departed); got != 3 {
		t.Fatalf("three programs left and the registry holds %d departures", got)
	}

	// Past the window, and then one more departure to do the sweeping: the
	// sweep runs on write, which is what keeps an idle daemon free of it.
	time.Sleep(10 * time.Millisecond)
	p, err := k.Register(Principal{
		UID: 1000, Kind: KindProgram,
		ClientID: "late", SessionID: "s-late", PID: 42,
	}, decl("late"))
	if err != nil {
		t.Fatalf("register late: %v", err)
	}
	k.Deregister(p.SessionID)

	if got := len(k.registry.departed); got != 1 {
		t.Fatalf("after the window passed and another program left, the "+
			"registry still holds %d departures, want 1; expired records are "+
			"unreadable but never freed, so a churning estate grows without "+
			"bound", got)
	}
	if _, held := k.registry.departed["late"]; !held {
		t.Error("the sweep dropped the departure that had just happened")
	}
}

// decl is the smallest declaration Register accepts.
func decl(id string) Declaration {
	return Declaration{
		Identity:     Identity{ID: id, Name: id, Version: "1.0.0"},
		Coverage:     CoveragePartial,
		SemanticsGen: 1,
		Commands: []Command{{
			ID: "reindex", Title: "Reindex",
			Effects: EffectsWritesFiles, Idempotent: Yes,
			Sensitive: []string{}, Interactive: No, Streams: No,
			NeedsDisplay: No, Duration: DurationSeconds, Confirms: No,
			Shape: ShapeUnary, Summary: "Rebuild the index",
			Description: "Walks the tree and rebuilds the index from scratch.",
			Returns:     "The number of items indexed.",
		}},
	}
}
