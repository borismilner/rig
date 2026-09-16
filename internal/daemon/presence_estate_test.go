package daemon

import (
	"testing"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// THE ADDRESSABLE IDENTITY IS (estate, seat, epoch, generation), AND THE
// ESTATE IS THE COMPONENT THAT WAS MISSING UNTIL THE CLAIM WAS ATTACKED.
//
// The epoch was added on the argument that a Seat lifted out of its RESPONSE
// must stay identifiable. The same argument one level up says a Seat lifted
// out of its ESTATE must too, and it did not: the epoch store is per estate -
// $XDG_STATE_HOME/rig/estates/<name>/ - so every estate counts its own epoch
// from 1, and two estates that have each restarted once are BOTH at epoch 2.
// The triple collides there, and nothing on the row said so.
//
// NAME THE CONSUMER FIRST, because "not unique" is only a defect if somebody
// can reach the collision. Section 11 is that somebody and it is Boris's own
// recorded requirement: section 37 runs two named estates at once, so the
// window carries TWO TRAYS ON ONE PANEL and holds both rosters at the same
// time. A reader on one connection never needs the estate - it reached exactly
// one and the estate is implied. A reader POOLING two does, and that reader is
// specified.
//
// THE UNNAMED ESTATE IS A CASE AND NOT AN OMISSION. It claims no name and
// opens no store, so it reports an empty estate beside epoch 0, and section 11
// gives it no tray at all - every test and every reproduction recipe starts
// one and none of them is a deployment. Empty beside zero is the coherent
// pair. EMPTY BESIDE A NON-ZERO EPOCH CANNOT HAPPEN: rigd derives both from
// one branch, where a name is what opens the store and the store is what bumps
// the epoch, and a named estate whose store will not open exits rather than
// serving without one.

// coherentIdentity checks the pair rule on a single served row.
//
// It is here rather than inline because it is the assertion that BITES when
// the field stops being served: drop the stamp and a named estate's rows keep
// their non-zero epoch while the name goes empty, which is precisely the pair
// the wire says cannot exist. A test that only checked "the name is what I
// configured" would catch the same mutation, but it would not say what is
// wrong with the row it caught - and a reader meeting this failure needs to
// know the row is incoherent, not merely unexpected.
func coherentIdentity(t *testing.T, where string, s *rigv1.Seat) {
	t.Helper()
	switch {
	case s.GetEstate() == "" && s.GetEpoch() != 0:
		t.Errorf("%s served seat %q with no estate but epoch %d. That pair "+
			"cannot be produced: an empty estate means no store, and no store "+
			"means no epoch. Either the estate stopped being carried onto the "+
			"row, or something is publishing an epoch it did not earn",
			where, s.GetSeat(), s.GetEpoch())
	case s.GetEstate() != "" && s.GetEpoch() == 0:
		t.Errorf("%s served seat %q in estate %q at epoch 0. A named estate "+
			"opens a store and the store bumps before it publishes, so a real "+
			"epoch is at least 1",
			where, s.GetSeat(), s.GetEstate())
	}
}

// TestEverySeatServedCarriesTheEstateItIsIn is the construction-site case, and
// it is deliberately the same shape as the epoch's because it is the same
// argument carried one level out.
//
// FOUR SITES, and the estate is set where an occupant is BORN rather than
// where a response is built. Stamping in the handlers was written and thrown
// away for the epoch: it leaves a fifth site free to forget, and the fifth
// site is the one nobody reviews. Carrying it on the occupant makes an
// estate-less Seat unconstructable, and this case is what proves the
// construction site is doing that work rather than three call sites happening
// to agree today.
//
// EVERY ROW IN ONE RESPONSE CARRIES THE SAME ESTATE, for the reason the epoch
// comment already gives about runs: a daemon serves exactly one estate, so a
// roster cannot hold two and a reader must never branch on the possibility.
// Anything deriving the estate per row is the thing this would catch.
func TestEverySeatServedCarriesTheEstateItIsIn(t *testing.T) {
	const (
		estate = "production"
		epoch  = 4
	)
	sock := upDaemonIn(t, estate, epoch)

	seated := dial(t, sock)
	// An unseated peer too. It holds no seat and carries generation 0, but it
	// is present in THIS estate, and an empty estate on its row would read as
	// the honest answer for an unnamed one rather than as a blank.
	loose := dial(t, sock)
	announce(t, loose, "", "a one-off session holding no seat", "reading")

	// SITE 1 and SITE 2: announce answers with the caller's own row and the
	// whole crew.
	got := announce(t, seated, "backend-1", "the seated peer", "working")
	if e := got.GetYou().GetEstate(); e != estate {
		t.Errorf("announce told the caller its estate is %q, want %q. A peer "+
			"cannot quote an identity it was never given", e, estate)
	}
	coherentIdentity(t, "announce (You)", got.GetYou())
	if n := len(got.GetCrew()); n != 2 {
		t.Fatalf("crew = %d, want 2", n)
	}
	for _, s := range got.GetCrew() {
		if e := s.GetEstate(); e != estate {
			t.Errorf("announce served seat %q with estate %q, want %q",
				s.GetSeat(), e, estate)
		}
		coherentIdentity(t, "announce (Crew)", s)
	}

	// SITE 3: activity answers with the caller's row, and it is the site a
	// per-response field would have missed entirely.
	act := &rigv1.ActivityResponse{}
	if err := seated.Call(ctx5(t), "rig.activity", &rigv1.ActivityRequest{
		Activity: "still working",
	}, act); err != nil {
		t.Fatal(err)
	}
	if e := act.GetYou().GetEstate(); e != estate {
		t.Errorf("rig.activity served a row with estate %q, want %q. This is "+
			"the call a peer makes most often and the one that would have "+
			"needed a third envelope field", e, estate)
	}
	coherentIdentity(t, "rig.activity (You)", act.GetYou())

	// SITE 4: the roster, which is what a third party reads.
	crew := roster(t, seated).GetCrew()
	if len(crew) != 2 {
		t.Fatalf("roster crew = %d, want 2", len(crew))
	}
	for _, s := range crew {
		if e := s.GetEstate(); e != estate {
			t.Errorf("rig.peers served seat %q (generation %d) with estate %q, "+
				"want %q", s.GetSeat(), s.GetGeneration(), e, estate)
		}
		coherentIdentity(t, "rig.peers (Crew)", s)
	}
}

// TestTheEstateIsWhatSeparatesTwoRostersAtTheSameEpoch is the case that RULES,
// and it is the collision itself rather than a description of it.
//
// TWO ESTATES, EACH HAVING RESTARTED ONCE, so both are at epoch 2 - which is
// not a contrivance but the ordinary state of section 37's own pair after a
// morning's work. The same seat name in both is likewise ordinary: seats are
// named for the ROLE, so `backend-1` in production and `backend-1` in
// development are exactly what a self-hosting estate pair produces.
//
// The reader is section 11's: one process, both connections open, both rosters
// in memory. For it, (seat, epoch, generation) is not an address - it names
// two different sessions and picks whichever was stored last.
func TestTheEstateIsWhatSeparatesTwoRostersAtTheSameEpoch(t *testing.T) {
	const epoch = 2 // each estate has restarted once. Both counters say 2.

	prod := dial(t, upDaemonIn(t, "production", epoch))
	dev := dial(t, upDaemonIn(t, "development", epoch))

	inProd := announce(t, prod, "backend-1", "the production tenancy", "working").GetYou()
	inDev := announce(t, dev, "backend-1", "the development tenancy", "working").GetYou()

	// THE COLLISION, asserted rather than assumed. `reference` is the identity
	// as it stood before this field, and using it here is the point: the two
	// rows are indistinguishable under it.
	was := reference{seat: inProd.GetSeat(), epoch: inProd.GetEpoch(), generation: inProd.GetGeneration()}
	other := reference{seat: inDev.GetSeat(), epoch: inDev.GetEpoch(), generation: inDev.GetGeneration()}
	if was != other {
		t.Fatalf("the two estates did not collide on (seat, epoch, generation): "+
			"%+v and %+v. This case exists to demonstrate that they DO, so "+
			"either the estates stopped counting epochs independently or this "+
			"test has drifted from what it was written to show", was, other)
	}

	// THE RULE. With the estate on the row they are two addresses, and a
	// pooling reader can hold both without either overwriting the other.
	if inProd.GetEstate() == inDev.GetEstate() {
		t.Fatalf("both rows report estate %q, so the full identity collides "+
			"too and a reader holding both rosters still cannot tell a "+
			"production seat from a development one. This is the failure the "+
			"field was added to close", inProd.GetEstate())
	}
	if e := inProd.GetEstate(); e != "production" {
		t.Errorf("the production roster served estate %q", e)
	}
	if e := inDev.GetEstate(); e != "development" {
		t.Errorf("the development roster served estate %q", e)
	}

	// AND NEITHER SEES THE OTHER, which is what makes the two rows a genuine
	// pair rather than one daemon answering twice. Section 37's isolation
	// precondition 1 is a separate mechanism, checked here only so far as this
	// case depends on it.
	for _, c := range []struct {
		where string
		crew  []*rigv1.Seat
	}{
		{"production", roster(t, prod).GetCrew()},
		{"development", roster(t, dev).GetCrew()},
	} {
		if n := len(c.crew); n != 1 {
			t.Fatalf("the %s roster holds %d rows, want 1: an estate that can "+
				"see into another one is not isolated and this case is "+
				"measuring the wrong thing", c.where, n)
		}
		if e := c.crew[0].GetEstate(); e != c.where {
			t.Errorf("the %s roster served a row belonging to estate %q",
				c.where, e)
		}
	}
}

// TestAnUnnamedEstateServesAnEmptyNameBesideEpochZero is the case the wire's
// "EMPTY IS A CASE AND NOT A MISSING VALUE" sentence is worth.
//
// An unnamed estate is not an edge: it is what every test in this repository
// and every reproduction recipe starts, and it is the shape a developer runs
// when they are not deploying anything. Its row says so honestly - no name, no
// store, no epoch - and the two blanks arrive TOGETHER, which is what lets a
// reader tell "this estate has no durable state" from "somebody dropped a
// field on the way out".
//
// The seated peer matters here. An unseated row is blank in several places
// already and proves little; a peer holding a seat at generation 1 with an
// empty estate beside it is the row a reader has to be able to interpret.
func TestAnUnnamedEstateServesAnEmptyNameBesideEpochZero(t *testing.T) {
	sock := upDaemonIn(t, "", 0)
	c := dial(t, sock)
	you := announce(t, c, "backend-1", "a seat in an estate with no name", "working").GetYou()

	if e := you.GetEstate(); e != "" {
		t.Errorf("an unnamed estate served estate %q. It claimed no name, so "+
			"there is none to report and anything here was invented", e)
	}
	if e := you.GetEpoch(); e != 0 {
		t.Errorf("an unnamed estate served epoch %d. It opens no store, and "+
			"the store is the only thing that issues an epoch", e)
	}
	if g := you.GetGeneration(); g != 1 {
		t.Fatalf("generation = %d, want 1. The blanks above are only "+
			"interesting on a row that is otherwise fully occupied", g)
	}

	// The pair holds on every site, including the roster a third party reads.
	coherentIdentity(t, "announce (You), unnamed estate", you)
	crew := roster(t, c).GetCrew()
	if len(crew) != 1 {
		t.Fatalf("roster crew = %d, want 1", len(crew))
	}
	coherentIdentity(t, "rig.peers (Crew), unnamed estate", crew[0])
}
