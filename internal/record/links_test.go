package record

import (
	"errors"
	"strings"
	"testing"
)

func TestALinkIsWrittenOnceAndAssertingItAgainIsNotAnError(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	a := workItem(t, s, "wi-a", "first")
	b := workItem(t, s, "wi-b", "second")

	if err := s.Link(tctx, a, LinkBlocks, b); err != nil {
		t.Fatalf("linking: %v", err)
	}
	// IDEMPOTENT BY CONTRACT. The caller asked for the edge to exist, and it
	// does. A second assertion of the same fact is not new information.
	if err := s.Link(tctx, a, LinkBlocks, b); err != nil {
		t.Fatalf("re-asserting the same edge should be a no-op, got: %v", err)
	}

	// ⛔ A SECOND EDGE OF A DIFFERENT TYPE, OR THIS ASSERTION IS NOT ABOUT
	// FILTERING. With only one edge present, linksFrom returns the same answer
	// whether it honours the type or ignores it - a mutation that deleted the
	// type predicate survived this test in exactly that form.
	c := workItem(t, s, "wi-c", "third")
	if err := s.Link(tctx, a, LinkCites, c); err != nil {
		t.Fatalf("linking a second type: %v", err)
	}

	out, err := s.linksFrom(tctx, a, LinkBlocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != b {
		t.Fatalf("blocks edges from %s are %v, want exactly [%s] - the cites edge leaked in", a, out, b)
	}
	cites, err := s.linksFrom(tctx, a, LinkCites)
	if err != nil {
		t.Fatal(err)
	}
	if len(cites) != 1 || cites[0] != c {
		t.Fatalf("cites edges from %s are %v, want exactly [%s]", a, cites, c)
	}

	if err := s.Unlink(tctx, a, LinkBlocks, b); err != nil {
		t.Fatalf("unlinking: %v", err)
	}
	if err := s.Unlink(tctx, a, LinkBlocks, b); err != nil {
		t.Fatalf("unlinking an edge that is already gone should be a no-op, got: %v", err)
	}
	out, err = s.linksFrom(tctx, a, LinkBlocks)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("edges from %s are %v after unlink, want none", a, out)
	}
}

// ⛔ AN UNKNOWN LINK TYPE IS THE QUIET FAILURE, WHICH IS WHY IT IS REFUSED.
// Section 39 names eight. A typo writes an edge that exists, satisfies its
// insert, and is found by nothing - every derivation queries by type, so a
// `block` edge is not a wrong answer, it is silence.
func TestAnUnknownLinkTypeIsRefusedAndQuotedBack(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	a := workItem(t, s, "wi-a", "first")
	b := workItem(t, s, "wi-b", "second")

	err := s.Link(tctx, a, "block", b)
	if err == nil {
		t.Fatal("a misspelt link type was accepted; nothing would ever find that edge")
	}
	if !strings.Contains(err.Error(), `"block"`) {
		t.Fatalf("the refusal does not quote the bad type back: %v", err)
	}
	if !strings.Contains(err.Error(), LinkBlocks) {
		t.Fatalf("the refusal does not name a valid type the caller could have meant: %v", err)
	}
}

func TestLinkingToARecordThatDoesNotExistNamesWhichEndIsMissing(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	a := workItem(t, s, "wi-a", "first")

	var nf *NotFoundError
	err := s.Link(tctx, a, LinkBlocks, "wi-ghost")
	if !errors.As(err, &nf) {
		t.Fatalf("linking to a missing record should be NotFound, got %T: %v", err, err)
	}
	if nf.ID != "wi-ghost" {
		t.Fatalf("the refusal names %q, want the end that was missing", nf.ID)
	}

	err = s.Link(tctx, "wi-ghost", LinkBlocks, a)
	if !errors.As(err, &nf) {
		t.Fatalf("linking FROM a missing record should be NotFound, got %T: %v", err, err)
	}
	if nf.ID != "wi-ghost" {
		t.Fatalf("the refusal names %q, want the source that was missing", nf.ID)
	}
}

// A record blocking itself is a one-node cycle. Section 39 requires a cycle be
// REPORTED and never resolved, but that is about cycles a real dependency graph
// grows. This one is a typo every time, and refusing it at the edge costs
// nothing where reporting it costs a caller a confused brief.
func TestARecordCannotBlockItself(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	a := workItem(t, s, "wi-a", "first")

	if err := s.Link(tctx, a, LinkBlocks, a); err == nil {
		t.Fatal("a record was allowed to block itself")
	}
}

// ⛔ THE STREAM INVARIANT SURVIVES THE NEW VERBS, OR THE NEW VERBS BREAK IT.
// Put already refuses to write or rewrite a progress record. If Unlink can
// detach a step from its item, the step becomes a record nothing can find, and
// the invariant Step enforces is undone through a different door.
func TestUnlinkCannotDetachAProgressStepFromItsItem(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	item := workItem(t, s, "wi-b41", "the CLI roster verb")

	step, err := s.Step(tctx, StepRequest{
		Item: item, State: "started", Project: "rig",
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Unlink(tctx, step.ID, LinkPartOf, item); err == nil {
		t.Fatal("a progress step was detached from its item; it is now invisible to every derivation")
	}
	stream, err := s.stream(tctx, item)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream) != 1 {
		t.Fatalf("the stream has %d steps after the refused unlink, want 1", len(stream))
	}
}

func TestLinkRefusesAnEmptyEndByName(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	a := workItem(t, s, "wi-a", "first")

	for _, c := range []struct{ src, typ, dst, want string }{
		{"", LinkBlocks, a, "source"},
		{a, "", a, "is not a link type"},
		{a, LinkBlocks, "", "destination"},
	} {
		err := s.Link(tctx, c.src, c.typ, c.dst)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("Link(%q,%q,%q) should be refused naming the %s, got: %v", c.src, c.typ, c.dst, c.want, err)
		}
	}
}
