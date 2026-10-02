package coord

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

var one = json.RawMessage(`{"n":1}`)

// ⛔ FIRST WRITER WINS: version 0 claims a key nobody has, and a second
// claim is told who got there first rather than overwriting it.
func TestAClaimFromEmptyWinsOnce(t *testing.T) {
	s := openStore(t, estate(t, "shared"))
	won, applied, err := s.SharedSet("claims.chunk-1", one, 0, "seat-a", NoWitness(), "seat-a")
	if err != nil || !applied || won.Version == 0 {
		t.Fatalf("first claim: %+v %v %v", won, applied, err)
	}
	lost, applied, err := s.SharedSet("claims.chunk-1", json.RawMessage(`{"n":2}`), 0, "seat-b", NoWitness(), "seat-b")
	if err != nil || applied {
		t.Fatalf("second claim applied=%v err=%v", applied, err)
	}
	if lost.Owner != "seat-a" || lost.Version != won.Version || string(lost.Value) != `{"n":1}` {
		t.Fatalf("the refusal shows %+v, want seat-a's claim", lost)
	}
}

func TestAWriteAtAnOldVersionIsRefused(t *testing.T) {
	s := openStore(t, estate(t, "shared"))
	v1, _, _ := s.SharedSet("count", one, 0, "", NoWitness(), "seat-a")
	v2, applied, _ := s.SharedSet("count", json.RawMessage(`2`), v1.Version, "", NoWitness(), "seat-b")
	if !applied || v2.By != "seat-b" {
		t.Fatalf("update at the read version: %+v %v", v2, applied)
	}
	if _, applied, _ := s.SharedSet("count", json.RawMessage(`3`), v1.Version, "", NoWitness(), "seat-a"); applied {
		t.Fatal("a write at a version somebody moved past landed")
	}
	if _, applied, _ := s.SharedSet("absent", one, 7, "", NoWitness(), "seat-a"); applied {
		t.Fatal("a write at version 7 created a key that did not exist")
	}
}

// ⛔ A VERSION IS NEVER REUSED. AgentBox restarts a deleted key at 1, so a
// writer still holding the old claim's version lands on the new claim.
func TestAKeyMadeAgainNeverRepeatsAVersion(t *testing.T) {
	s := openStore(t, estate(t, "shared"))
	old, _, _ := s.SharedSet("claims.x", one, 0, "seat-a", NoWitness(), "seat-a")
	if _, applied, _ := s.SharedDelete("claims.x", old.Version); !applied {
		t.Fatal("delete at the read version refused")
	}
	again, _, _ := s.SharedSet("claims.x", one, 0, "seat-b", NoWitness(), "seat-b")
	if again.Version == old.Version {
		t.Fatalf("the new claim reused version %d", old.Version)
	}
	if _, applied, _ := s.SharedSet("claims.x", one, old.Version, "seat-a", NoWitness(), "seat-a"); applied {
		t.Fatal("a stale writer overwrote the new claim")
	}
}

func TestADeleteNamesTheVersionItRead(t *testing.T) {
	s := openStore(t, estate(t, "shared"))
	v, _, _ := s.SharedSet("k", one, 0, "", NoWitness(), "seat-a")
	if _, _, err := s.SharedDelete("k", 0); err == nil {
		t.Fatal("a delete without a version was taken")
	}
	took, _, _ := s.SharedSet("k", one, v.Version, "seat-b", NoWitness(), "seat-b")
	cur, applied, err := s.SharedDelete("k", v.Version)
	if err != nil || applied || cur.Version != took.Version {
		t.Fatalf("a delete erased a takeover: %+v %v %v", cur, applied, err)
	}
}

// A family read is by prefix, and claims. never reaches claimsx.
func TestAFamilyIsItsPrefixAndNothingElse(t *testing.T) {
	s := openStore(t, estate(t, "shared"))
	for _, k := range []string{"claims.a", "claims.b", "claimsx", "other"} {
		_, _, _ = s.SharedSet(k, one, 0, "", NoWitness(), "seat-a")
	}
	got, more, err := s.SharedList("claims.")
	if err != nil || more || len(got) != 2 || got[0].Key != "claims.a" || got[1].Key != "claims.b" {
		t.Fatalf("got %+v more %v err %v", got, more, err)
	}
	all, _, _ := s.SharedList("")
	if len(all) != 4 {
		t.Fatalf("every key: %d", len(all))
	}
}

func TestTheTableRefusesANewKeyAtItsCap(t *testing.T) {
	s := openStore(t, estate(t, "shared"))
	var last Shared
	for i := range MaxSharedKeys {
		last, _, _ = s.SharedSet(fmt.Sprintf("k%04d", i), one, 0, "", NoWitness(), "seat-a")
	}
	if _, _, err := s.SharedSet("one-more", one, 0, "", NoWitness(), "seat-a"); !errors.Is(err, ErrSharedFull) {
		t.Fatalf("past the cap: %v", err)
	}
	// An existing key still updates: the cap stops growth, not work.
	if _, applied, err := s.SharedSet(last.Key, one, last.Version, "", NoWitness(), "seat-a"); err != nil || !applied {
		t.Fatalf("updating at the cap: %v %v", applied, err)
	}
	got, more, _ := s.SharedList("")
	if len(got) != MaxSharedList || !more {
		t.Fatalf("a full read answered %d, more %v", len(got), more)
	}
}

func TestAnOwnersClaimsAreFoundAndSurviveAReopen(t *testing.T) {
	name := estate(t, "shared")
	s := openStore(t, name)
	w := Witness{Kind: PID, Pid: 42, StartTicks: 7, BootID: "b"}
	_, _, _ = s.SharedSet("claims.a", one, 0, "seat-a", w, "seat-a")
	_, _, _ = s.SharedSet("claims.b", one, 0, "seat-b", w, "seat-b")
	_, _, _ = s.SharedSet("note", one, 0, "", w, "seat-a")
	_ = s.Close()

	again := openStore(t, name)
	got, err := again.SharedOwnedBy("seat-a")
	if err != nil || len(got) != 1 || got[0].Key != "claims.a" || got[0].Witness != w {
		t.Fatalf("got %+v err %v", got, err)
	}
	note, _, _ := again.SharedGet("note")
	if note.Owner != "" || note.Witness != (Witness{}) || note.By != "seat-a" {
		t.Fatalf("an unowned write kept a witness: %+v", note)
	}
}

// A version 2 file, as every estate holds today, gains the table in place.
func TestAVersionTwoStoreGainsTheSharedTable(t *testing.T) {
	name := estate(t, "shared")
	s := openStore(t, name)
	path := s.Path()
	_ = s.Close()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"DROP TABLE shared", "DROP TABLE shared_seq", "PRAGMA user_version = 2"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	again := openStore(t, name)
	if _, applied, err := again.SharedSet("k", one, 0, "", NoWitness(), "seat-a"); err != nil || !applied {
		t.Fatalf("after the step: %v %v", applied, err)
	}
}
