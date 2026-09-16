package record

import (
	"strings"
	"testing"
	"time"
)

func requirement(t *testing.T, s *Store, id, title, body string) string {
	t.Helper()
	w, err := s.Put(PutRequest{
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
		if err := s.Link(src, LinkCites, subject); err != nil {
			t.Fatal(err)
		}
	}

	// THE GREP, run over the same corpus, so the miss is demonstrated rather
	// than asserted.
	var grepped []string
	for _, id := range []string{quoting, paraphrase} {
		rec, err := s.Get(id)
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

	got, err := s.Refs(RefsRequest{ID: subject})
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

	_, err := s.Refs(RefsRequest{ID: subject, Depth: MaxRefsDepth + 1})
	if err == nil {
		t.Fatal("a depth above the cap was accepted; a silently clamped traversal returns " +
			"a partial answer that looks complete")
	}
	// It must be the CAP talking. A NotFoundError here would mean the bound was
	// never checked and the lookup simply missed.
	if !strings.Contains(err.Error(), "above the cap") {
		t.Fatalf("refused with %q, which is not the depth check", err)
	}
	if _, err := s.Refs(RefsRequest{ID: subject, Depth: MaxRefsDepth}); err != nil {
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
		if err := s.Link(cur, LinkCites, prev); err != nil {
			t.Fatal(err)
		}
		prev = cur
	}

	shallow, err := s.Refs(RefsRequest{ID: subject, Depth: 2})
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

	full, err := s.Refs(RefsRequest{ID: subject, Depth: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Refs) != 4 {
		t.Fatalf("depth 4 reached %d records, want the whole chain: %+v", len(full.Refs), full.Refs)
	}
	if full.Truncated {
		t.Fatal("a traversal that reached the end of the graph reported itself truncated")
	}
}

// ⛔ A TRAVERSAL IS PROJECT-SCOPED BY DEFAULT AND CROSSING IS ASKED FOR.
func TestATraversalStaysInOneProjectUntilItIsAskedToLeave(t *testing.T) {
	name := estate(t, "development")
	s := openStore(t, name)

	subject := requirement(t, s, "r39", "the continuity record", "")
	inside := requirement(t, s, "r37", "two estates", "")
	outside, err := s.Put(PutRequest{
		ID: "std-1", Kind: "requirement", Project: "standards", Body: "",
		Fields:  map[string]string{"title": "every project stamps its checks"},
		Session: "record", Seat: "backend-record", Epoch: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{inside, outside.ID} {
		if err := s.Link(src, LinkCites, subject); err != nil {
			t.Fatal(err)
		}
	}

	scoped, err := s.Refs(RefsRequest{ID: subject})
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Refs) != 1 || scoped.Refs[0].ID != inside {
		t.Fatalf("the default traversal returned %+v; it must stay inside the subject's "+
			"own project", scoped.Refs)
	}

	crossed, err := s.Refs(RefsRequest{ID: subject, CrossProject: true})
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
		if err := s.Link(e[0], LinkCites, e[1]); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan Refs, 1)
	go func() {
		got, err := s.Refs(RefsRequest{ID: a, Depth: MaxRefsDepth})
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
			out, err := s.LinksFrom(src, LinkCites)
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
