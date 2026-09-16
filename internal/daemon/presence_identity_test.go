package daemon

import "testing"

// TWO PEERS MUST BE TWO ROWS, AND THE WAY THAT BREAKS IS NOT THE WAY ANYONE
// LOOKS FOR.
//
// presence keys its roster on an `*occupancy` token, which each door allocates
// one of per accepted connection. The token carries no data - presence needs
// an identity and never dereferences it - so the obvious shape is an empty
// struct, and the obvious shape is WRONG.
//
// GO GIVES EVERY HEAP ALLOCATION OF A ZERO-SIZED TYPE THE SAME ADDRESS.
// `&occupancy{}` on two connections returns one pointer (runtime.zerobase), a
// map keyed on it holds ONE entry for both, and every peer on the estate
// collapses into a single row that each of them overwrites in turn.
//
// IT WAS WRITTEN THAT WAY AND THE SUITE CAUGHT IT - five wire cases went red
// at once, all reporting "crew = 1, want 2". This case exists because that is
// the wrong sentence to debug from: it reads as a roster bug, it is really a
// property of the language, and the fix is one byte of padding that a later
// reader has every reason to think is dead weight.
//
// So this is the case that fails LEGIBLY when somebody tidies the padding
// away, and the mutation is exactly that: delete the `_ [1]byte`.
func TestTwoOccupanciesAreTwoRows(t *testing.T) {
	p := newPresence("production", 1)

	// Allocated the way a door allocates them - on the heap, one per
	// connection. Stack variables happen to survive this bug and would hide
	// it, which is why these are pointers from `new` rather than `var`.
	first, second := &occupancy{}, &occupancy{}

	if first == second {
		t.Fatal("two freshly allocated occupancy tokens are the SAME pointer. " +
			"occupancy is zero-sized again, so every connection on this " +
			"estate shares one roster row: presence sees one peer where " +
			"there are two, the seat refusal cannot fire because there is " +
			"nothing to refuse against, and two sessions both believe they " +
			"hold the seat - which is the failure the whole mechanism exists " +
			"to make visible")
	}

	if _, err := p.announce(first, "backend-1", "the cutover", "writing"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.announce(second, "backend-3", "lifecycle", "installing"); err != nil {
		t.Fatal(err)
	}

	if n := len(p.crew()); n != 2 {
		t.Fatalf("crew = %d after two peers announced into two seats, want 2. "+
			"One row for two occupants means the token is not an identity", n)
	}
}

// TestASeatHeldThroughOneTokenIsRefusedThroughAnother is the consequence
// spelled out, because the case above can be read as bookkeeping.
//
// The refusal is what stops two sessions believing they are the same seat. It
// works by finding a DIFFERENT token already holding the name - so if the
// tokens are not distinct, the refusal does not merely weaken, it becomes
// unreachable, and the intruder is served a successful announce.
func TestASeatHeldThroughOneTokenIsRefusedThroughAnother(t *testing.T) {
	p := newPresence("production", 1)

	holder, intruder := &occupancy{}, &occupancy{}
	if _, err := p.announce(holder, "team-lead", "sequencing the cutover", "writing"); err != nil {
		t.Fatal(err)
	}

	_, err := p.announce(intruder, "team-lead", "also sequencing", "writing")
	if err == nil {
		t.Fatal("a second token took a held seat and was GRANTED it. With one " +
			"token per connection this is two sessions both told they are " +
			"team-lead, and neither ever finds out")
	}
	var held *seatHeldError
	if !asSeatHeld(err, &held) {
		t.Fatalf("refused with %v, want a seatHeldError naming the holder. "+
			"Section 9: the refusal has to carry the state actually found", err)
	}
	if held.Purpose != "sequencing the cutover" {
		t.Fatalf("the refusal names purpose %q, want the holder's", held.Purpose)
	}
}
