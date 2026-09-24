package meta_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/internal/meta"
)

// EVERY ASSERTION IN THIS FILE GOES THROUGH MarshalAnswer, AND THAT IS THE
// TEST RATHER THAN A DETAIL OF IT.
//
// A field can sit on meta.Estate, be set by the daemon, and travel all the way
// through Answer while never reaching the agent, because the rendering is a
// separate struct with its own tags. That is the same defect one layer down
// from the one this file exists for, and it leaves every struct-level
// assertion green: the Go value is right and the object the agent reads does
// not have it. So nothing here looks at Answer.Identity.
//
// It is a separate file from estate_test.go because it asks a different
// question. That file asks WHICH estate this is; this one asks which
// INCARNATION of it.

// renderedIdentity answers query subject=estate and returns the estateIdentity
// object exactly as an agent receives it.
func renderedIdentity(t *testing.T, e meta.Estate) map[string]any {
	t.Helper()
	s := meta.New(estate(t, kernel.CoverageFull), &knowsWhere{where: e})

	got, err := s.Answer(context.Background(), agent(),
		meta.Request{Tool: meta.Query, Subject: meta.SubjectEstate})
	if err != nil {
		t.Fatalf("query subject=%s: %v", meta.SubjectEstate, err)
	}
	raw, err := meta.MarshalAnswer(got)
	if err != nil {
		t.Fatalf("marshalling the answer: %v", err)
	}
	var obj struct {
		Identity map[string]any `json:"estateIdentity"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("decoding the rendered answer: %v\n%s", err, raw)
	}
	if obj.Identity == nil {
		t.Fatalf("the rendered answer has no estateIdentity at all:\n%s", raw)
	}
	return obj.Identity
}

func restarted(e meta.Estate, epoch uint64) meta.Estate {
	e.Epoch = epoch
	return e
}

// TestARestartChangesExactlyOneRenderedKey is the defect, stated as the
// smallest thing that would have caught it.
//
// DEMONSTRATED 2026-09-16 on a real restart of one daemon over a shared
// XDG_STATE_HOME: every path an agent could read was BYTE-IDENTICAL before and
// after. The epoch had incremented - rigd's own log said epoch=1 then epoch=2 -
// and the only consumer of Store.Epoch outside internal/coord was that log
// line. So rig knew it had restarted and no agent could find out.
//
// The assertion is deliberately two-sided. That the epoch MOVED is the easy
// half and a test that only checked it would pass while the epoch quietly
// became the only thing anybody could rely on. That everything ELSE held still
// is the half that keeps this answer an identity: a restart is not a different
// estate, and an agent that re-derived its routing from a changed name or role
// would be worse off than one that could not tell at all.
func TestARestartChangesExactlyOneRenderedKey(t *testing.T) {
	before := renderedIdentity(t, restarted(production(), 1))
	after := renderedIdentity(t, restarted(production(), 2))

	if len(before) != len(after) {
		t.Fatalf("the two renderings have different shapes: %v vs %v", before, after)
	}
	var moved []string
	for k, v := range before {
		if v != after[k] {
			moved = append(moved, k)
		}
	}
	if len(moved) != 1 || moved[0] != "epoch" {
		t.Fatalf("a restart moved %v, want exactly [epoch]\n  before %v\n  after  %v",
			moved, before, after)
	}
	if before["epoch"] != float64(1) || after["epoch"] != float64(2) {
		t.Errorf("the epoch rendered as %v then %v, want 1 then 2",
			before["epoch"], after["epoch"])
	}
}

// TestEpochZeroIsRenderedRatherThanOmitted pins the no-omitempty rule that
// json.go states for this object, for the one field where breaking it is
// tempting.
//
// An unnamed estate opens no store and so has no epoch, and `omitempty` on a
// uint64 would silently drop the key for exactly that case. An agent reading
// the result cannot tell an absent key from a daemon too old to have it, so
// "this estate has no persistent state" would arrive looking like "this rig
// cannot tell you about restarts". They are different facts and the agent
// would act differently on each.
func TestEpochZeroIsRenderedRatherThanOmitted(t *testing.T) {
	unnamed := meta.Estate{Role: "unnamed", DaemonVersion: "2.0.0", Wire: "1", SemanticsGen: 1}
	got := renderedIdentity(t, unnamed)

	v, ok := got["epoch"]
	if !ok {
		t.Fatalf("an unnamed estate rendered no epoch key at all, which an "+
			"agent can only read as a daemon too old to have one: %v", got)
	}
	if v != float64(0) {
		t.Errorf("an unnamed estate rendered epoch %v, want 0", v)
	}
}

// TestTheRenderedEpochIsTheOneTheDaemonPublished is the plain wiring check:
// the number an agent reads is the number the store bumped, not a count of
// anything this package did.
func TestTheRenderedEpochIsTheOneTheDaemonPublished(t *testing.T) {
	const published = 47
	got := renderedIdentity(t, restarted(production(), published))
	if got["epoch"] != float64(published) {
		t.Errorf("the daemon published epoch %d and the agent was shown %v",
			published, got["epoch"])
	}
}
