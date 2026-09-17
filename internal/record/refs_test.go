package record

import (
	"strings"
	"testing"
	"time"
)

func requirement(t *testing.T, s *Store, id, title, body string) string {
	t.Helper()
	w, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "requirement", Project: "rig", Body: body,
		Fields:  map[string]string{"title": title, "status": "active"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatalf("writing requirement %s: %v", id, err)
	}
	return w.ID
}

// ⛔ SECTION 39's SLICE-4 DEMONSTRATION: "what cites this requirement" answers
// with a copy that a grep for the obvious phrase MISSES.
//
// The fixture is section 39's own - the struck-quotation residue of 2026-09-12,
// where a correction landed at the place it was argued and the OTHER mentions
// of the same fact were left behind. A grep finds the ones that quote the
// wording. The edge finds the one that paraphrases it, which is the copy that
// goes stale silently and the whole reason the reverse direction exists.
func TestWhatCitesThisFindsTheCopyAGrepForTheWordingMisses(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	subject := requirement(t, s, "r37-row3", "row 3 keeps its position", "")
	quoting := requirement(t, s, "d-quote", "the summary row",
		"Row 3 keeps its position and the record follows it.")
	// ⛔ THE ONE A GREP MISSES. It states the same fact and shares no phrase.
	paraphrase := requirement(t, s, "d-para", "the tension-7 note",
		"The ordering above is unchanged and the register trails it.")

	for _, src := range []string{quoting, paraphrase} {
		if err := s.Link(tctx, src, LinkCites, subject); err != nil {
			t.Fatal(err)
		}
	}

	// THE GREP, run over the same corpus, so the miss is demonstrated rather
	// than asserted.
	var grepped []string
	for _, id := range []string{quoting, paraphrase} {
		rec, err := s.Get(tctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(rec.Body), "keeps its position") {
			grepped = append(grepped, id)
		}
	}
	if len(grepped) != 1 || grepped[0] != quoting {
		t.Fatalf("the grep found %v; this fixture is only a demonstration if it finds "+
			"exactly the quoting copy", grepped)
	}

	got, err := s.Refs(tctx, RefsRequest{ID: subject})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Refs) != 2 {
		t.Fatalf("refs found %d citations, want both: %+v", len(got.Refs), got.Refs)
	}
	var found bool
	for _, r := range got.Refs {
		if r.ID == paraphrase {
			found = true
			if r.Type != LinkCites || r.Depth != 1 || r.Via != subject {
				t.Fatalf("the paraphrase is %+v; want a cites edge at depth 1 via the subject", r)
			}
		}
	}
	if !found {
		t.Fatalf("refs missed the paraphrase, which is the only copy a grep cannot reach: %+v", got.Refs)
	}
	if got.Truncated {
		t.Fatal("a two-node answer inside the default depth reported itself truncated")
	}
}

// ⛔ A DEPTH ABOVE THE CAP IS REFUSED BY NAME, NEVER CLAMPED. R3.2.
func TestARefsDepthAboveTheCapIsRefusedRatherThanQuietlyClamped(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)
	subject := requirement(t, s, "r37", "two estates", "")

	_, err := s.Refs(tctx, RefsRequest{ID: subject, Depth: MaxRefsDepth + 1})
	if err == nil {
		t.Fatal("a depth above the cap was accepted; a silently clamped traversal returns " +
			"a partial answer that looks complete")
	}
	// It must be the CAP talking. A NotFoundError here would mean the bound was
	// never checked and the lookup simply missed.
	if !strings.Contains(err.Error(), "above the cap") {
		t.Fatalf("refused with %q, which is not the depth check", err)
	}
	if _, err := s.Refs(tctx, RefsRequest{ID: subject, Depth: MaxRefsDepth}); err != nil {
		t.Fatalf("the cap itself was refused: %v", err)
	}
}

// ⛔ TRUNCATION IS REPORTED. A partial answer that looks complete is the failure
// this capability exists to prevent.
func TestATraversalStoppedByTheBoundSaysSo(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	// A chain: c4 -> c3 -> c2 -> c1 -> subject, so depth 2 cannot reach c3.
	subject := requirement(t, s, "r0", "the subject", "")
	prev := subject
	for _, id := range []string{"c1", "c2", "c3", "c4"} {
		cur := requirement(t, s, id, id, "")
		if err := s.Link(tctx, cur, LinkCites, prev); err != nil {
			t.Fatal(err)
		}
		prev = cur
	}

	shallow, err := s.Refs(tctx, RefsRequest{ID: subject, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(shallow.Refs) != 2 {
		t.Fatalf("depth 2 reached %d records, want 2: %+v", len(shallow.Refs), shallow.Refs)
	}
	if !shallow.Truncated {
		t.Fatal("the traversal stopped at its bound with two records still to reach " +
			"and did not say so - the caller cannot tell this from a complete answer")
	}

	full, err := s.Refs(tctx, RefsRequest{ID: subject, Depth: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Refs) != 4 {
		t.Fatalf("depth 4 reached %d records, want the whole chain: %+v", len(full.Refs), full.Refs)
	}
	// ⛔ THIS IS THE ASSERTION THE RULING PINS, plan/39 at rig 891b89f. The
	// walk reaches depth 4 of a bound of 4 - it HITS THE CAP - and has nowhere
	// left to go, so the answer is complete and the flag must be false. A
	// literal reading of R3.2's old wording ("hits the cap returns truncated")
	// would have made this true, and the specification moved rather than the
	// code.
	if full.Truncated {
		t.Fatal("a traversal that reached the end of the graph AT its bound " +
			"reported itself truncated - the cap alone is not the test, and a " +
			"caller told its whole answer is partial spends another traversal " +
			"for nothing")
	}
}

// ⛔ A TRAVERSAL IS PROJECT-SCOPED BY DEFAULT AND CROSSING IS ASKED FOR.
func TestATraversalStaysInOneProjectUntilItIsAskedToLeave(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	subject := requirement(t, s, "r39", "the continuity record", "")
	inside := requirement(t, s, "r37", "two estates", "")
	outside, err := s.Put(tctx, PutRequest{
		ID: "std-1", Kind: "requirement", Project: "standards", Body: "",
		Fields:  map[string]string{"title": "every project stamps its checks"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{inside, outside.ID} {
		if err := s.Link(tctx, src, LinkCites, subject); err != nil {
			t.Fatal(err)
		}
	}

	scoped, err := s.Refs(tctx, RefsRequest{ID: subject})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Refs) != 1 || scoped.Refs[0].ID != inside {
		t.Fatalf("the default traversal returned %+v; it must stay inside the subject's "+
			"own project", scoped.Refs)
	}

	crossed, err := s.Refs(tctx, RefsRequest{ID: subject, CrossProject: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(crossed.Refs) != 2 {
		t.Fatalf("the cross-project traversal returned %+v, want both ends", crossed.Refs)
	}
}

// ⛔ A CYCLE IS REPORTED AND THE TRAVERSAL TERMINATES, AND THE TERMINATION IS
// THE HALF THAT MATTERS.
//
// A recursive UNION dedups on (node, depth) and depth keeps incrementing, so on
// a cycle the dedup NEVER fires. Measured: 0->1->2->0 with the bound dropped ran
// past 20 seconds and was killed. The failure is a HANG, not a wrong answer,
// which is why this test carries its own clock: a hang otherwise reports as an
// infrastructure problem rather than as a defect.
func TestACycleIsReportedAndTheTraversalDoesNotHang(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	a := requirement(t, s, "a", "a", "")
	b := requirement(t, s, "b", "b", "")
	c := requirement(t, s, "c", "c", "")
	for _, e := range [][2]string{{a, b}, {b, c}, {c, a}} {
		if err := s.Link(tctx, e[0], LinkCites, e[1]); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan Refs, 1)
	go func() {
		got, err := s.Refs(tctx, RefsRequest{ID: a, Depth: MaxRefsDepth})
		if err != nil {
			t.Error(err)
			close(done)
			return
		}
		done <- got
	}()

	select {
	case got, ok := <-done:
		if !ok {
			t.Fatal("the traversal errored")
		}
		if len(got.Cycles) != 1 {
			t.Fatalf("cycles are %v, want the one a->b->c->a closes", got.Cycles)
		}
		want := []string{"a", "b", "c"}
		if len(got.Cycles[0]) != 3 {
			t.Fatalf("the cycle names %v, want %v", got.Cycles[0], want)
		}
		// ⛔ AND EVERY RECORD APPEARS ONCE. This is what the visited set buys,
		// and nothing asserted it until a mutation deleting that guard survived
		// this test - the depth bound had terminated the walk, so the only
		// visible damage was duplicates nobody was looking at.
		seen := map[string]int{}
		for _, ref := range got.Refs {
			seen[ref.ID]++
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("%s appears %d times in a cyclic graph's answer; the walk is "+
					"revisiting nodes and the frontier grows at every hop", id, n)
			}
		}

		// AND NOTHING WAS RESOLVED: every edge survives.
		for _, src := range want {
			out, err := s.LinksFrom(tctx, src, LinkCites)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != 1 {
				t.Fatalf("rig broke an edge out of %s to resolve the cycle: %v", src, out)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the traversal did not terminate on a cyclic graph in 10s - the visited " +
			"set is not doing its job, and this is the hang a recursive UNION produces")
	}
}

// foreign writes a record in a project that is NOT the one `requirement`
// uses, so a fixture can put a hop outside the subject's project.
func foreign(t *testing.T, s *Store, id, title string) string {
	t.Helper()
	w, err := s.Put(tctx, PutRequest{
		ID: id, Kind: "requirement", Project: "standards", Body: "",
		Fields:  map[string]string{"title": title, "status": "active"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatalf("writing foreign record %s: %v", id, err)
	}
	return w.ID
}

// ⛔ THE SCOPE FILTERS THE ANSWER, IT DOES NOT PRUNE THE WALK - AND THIS TEST
// IS THE DEFECT IT WAS WRITTEN FOR, KEPT AS A FIXTURE.
//
// Demonstrated before the fix: `C(rig) <-cites- B(standards) <-cites- A(rig)`
// answered 0 refs with truncated=false. A-home is in the subject's OWN
// project, and `record.refs` said nothing points at C. The filter ran per hop,
// above the visited mark and above the frontier append, so the entire subtree
// behind B was unreachable and moreBeyond applied the same filter and could not
// raise the flag either.
//
// Both halves are asserted separately on purpose: a fix that returns A but
// forgets that B was dropped is a scoped answer claiming completeness, which is
// the same defect one level down.
func TestASameProjectRecordBehindAForeignHopIsStillFound(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	subject := requirement(t, s, "C-subject", "the subject", "")
	middle := foreign(t, s, "B-foreign", "the foreign middle")
	home := requirement(t, s, "A-home", "same project as the subject", "")

	if err := s.Link(tctx, middle, LinkCites, subject); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(tctx, home, LinkCites, middle); err != nil {
		t.Fatal(err)
	}

	scoped, err := s.Refs(tctx, RefsRequest{ID: subject})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Refs) != 1 {
		t.Fatalf("the default traversal returned %d refs %+v; %s is in the "+
			"subject's own project and is reachable only through %s, so a walk "+
			"that prunes at the hop rather than at the answer loses it",
			len(scoped.Refs), scoped.Refs, home, middle)
	}
	if scoped.Refs[0].ID != home {
		t.Fatalf("the default traversal returned %s, want %s: the foreign hop "+
			"is walked THROUGH and kept out of the answer, not returned",
			scoped.Refs[0].ID, home)
	}
	if scoped.Refs[0].Depth != 2 {
		t.Errorf("%s came back at depth %d, want 2: the foreign hop it was "+
			"reached through still costs a hop, and a depth that skipped it "+
			"would claim a direct edge that does not exist",
			home, scoped.Refs[0].Depth)
	}
	if scoped.Refs[0].Via != middle {
		t.Errorf("%s came back via %q, want %s: Via names the record actually "+
			"crossed even when the scope keeps it out of the list, because it "+
			"is the only explanation the answer carries for the truncation",
			home, scoped.Refs[0].Via, middle)
	}
	if !scoped.Truncated {
		t.Errorf("the default traversal dropped %s from the answer and "+
			"reported truncated=false; a filtered answer that claims "+
			"completeness is the failure this whole capability exists to "+
			"prevent", middle)
	}

	crossed, err := s.Refs(tctx, RefsRequest{ID: subject, CrossProject: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(crossed.Refs) != 2 {
		t.Fatalf("the cross-project traversal returned %+v, want both %s and %s",
			crossed.Refs, middle, home)
	}
	if crossed.Truncated {
		t.Errorf("the cross-project traversal returned the whole graph and " +
			"reported itself truncated; nothing was filtered and nothing lies " +
			"beyond the horizon, so there is no more to show")
	}
}

// ⛔ A SCOPE-FILTERED ANSWER IS NEVER REPORTED COMPLETE, EVEN WHEN THE HORIZON
// IS CLEAN.
//
// The two sources of Truncated are OR-ed, and this is the fixture that tells
// them apart: the walk is bounded at 1, the filter drops a record at hop 1, and
// moreBeyond then correctly finds nothing past the frontier. An assignment at
// the bound instead of an OR clears the flag the filter set, and the caller is
// told its partial answer is whole.
func TestAScopeFilteredAnswerIsNeverReportedComplete(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	subject := requirement(t, s, "r39", "the continuity record", "")
	inside := requirement(t, s, "r37", "two estates", "")
	outside := foreign(t, s, "std-1", "every project stamps its checks")
	for _, src := range []string{inside, outside} {
		if err := s.Link(tctx, src, LinkCites, subject); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Refs(tctx, RefsRequest{ID: subject, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Refs) != 1 || got.Refs[0].ID != inside {
		t.Fatalf("the default traversal returned %+v, want only %s: the scope "+
			"still decides what comes back", got.Refs, inside)
	}
	if !got.Truncated {
		t.Fatal("the bound was reached with nothing beyond it, so the horizon " +
			"check is right to say false - but a record was dropped by the " +
			"scope at hop 1 and the flag must survive that. It was OR-ed for " +
			"this case and an assignment at the bound wipes it")
	}
}

// ⛔ THE HORIZON CHECK ASKS WHETHER THERE IS UNVISITED GRAPH, NOT WHETHER THERE
// IS UNVISITED GRAPH IN THIS PROJECT.
//
// moreBeyond used to repeat the hop loop's project filter, which made it blind
// in exactly the way the hop loop was: the nodes it refused to count are the
// ones the subject's own project can be hiding behind. Here the bound stops one
// hop short of a foreign neighbour and nothing was filtered out of the answer,
// so the flag can only come from the horizon.
func TestTheHorizonCountsAForeignNeighbourAsSomewhereStillToGo(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	subject := requirement(t, s, "r39", "the continuity record", "")
	inside := requirement(t, s, "r37", "two estates", "")
	beyond := foreign(t, s, "std-1", "every project stamps its checks")

	if err := s.Link(tctx, inside, LinkCites, subject); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(tctx, beyond, LinkCites, inside); err != nil {
		t.Fatal(err)
	}

	got, err := s.Refs(tctx, RefsRequest{ID: subject, Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Refs) != 1 || got.Refs[0].ID != inside {
		t.Fatalf("the bounded traversal returned %+v, want only %s", got.Refs, inside)
	}
	if !got.Truncated {
		t.Fatalf("the walk stopped at its bound with %s one hop past the "+
			"frontier and reported itself complete. %s is in another project, "+
			"which is the whole point: what lies BEHIND it was never looked at "+
			"and can be the subject's own project", beyond, beyond)
	}
}
