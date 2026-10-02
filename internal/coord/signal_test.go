package coord

import (
	"database/sql"
	"testing"
)

func all(*Signal) bool { return true }

func TestASignalSurvivesAReopen(t *testing.T) {
	name := estate(t, "sig")
	s := openStore(t, name)
	if err := s.PutSignal(Signal{Seq: 7, Kind: "signal.a", At: 1, Source: "seat:x", To: "seat:y", Payload: `{"n":1}`}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	again := openStore(t, name)
	got, more, err := again.SignalsAfter(0, 10, all)
	if err != nil || more || len(got) != 1 {
		t.Fatalf("got %v more %v err %v", got, more, err)
	}
	if g := got[0]; g.Seq != 7 || g.To != "seat:y" || g.Payload != `{"n":1}` {
		t.Fatalf("got %+v", g)
	}
}

// A kind keeps its newest MaxSignalsPerKind, and the trim records how far it
// went, per kind: a quiet kind's rows never hide a busy kind's trim.
func TestRetentionIsPerKindAndRecordsWhatItTook(t *testing.T) {
	s := openStore(t, estate(t, "sig"))
	if err := s.PutSignal(Signal{Seq: 1, Kind: "signal.quiet", At: 10, Source: "seat:x"}); err != nil {
		t.Fatal(err)
	}
	for i := uint64(2); i <= MaxSignalsPerKind+11; i++ {
		if err := s.PutSignal(Signal{Seq: i, Kind: "signal.busy", At: 10, Source: "seat:x"}); err != nil {
			t.Fatal(err)
		}
	}
	trimmed, err := s.SignalsTrimmed()
	if err != nil {
		t.Fatal(err)
	}
	if trimmed["signal.busy"] != 11 || trimmed["signal.quiet"] != 0 {
		t.Fatalf("trimmed = %v, want busy through 11 and quiet untouched", trimmed)
	}
	got, _, _ := s.SignalsAfter(0, MaxSignalBatch, func(sig *Signal) bool { return sig.Kind == "signal.busy" })
	if len(got) != MaxSignalsPerKind || got[0].Seq != 12 {
		t.Fatalf("kept %d, oldest %d", len(got), got[0].Seq)
	}
}

func TestOldSignalsAgeOut(t *testing.T) {
	s := openStore(t, estate(t, "sig"))
	_ = s.PutSignal(Signal{Seq: 1, Kind: "signal.a", At: 100, Source: "seat:x"})
	_ = s.PutSignal(Signal{Seq: 2, Kind: "signal.b", At: 200, Source: "seat:x"})
	// A post ages out its own kind.
	_ = s.PutSignal(Signal{Seq: 3, Kind: "signal.a", At: 100 + SignalMaxAge + 1, Source: "seat:x"})
	// The start sweep takes a kind nobody posts to.
	if err := s.TrimSignalsBefore(201); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.SignalsAfter(0, 10, all)
	if len(got) != 1 || got[0].Seq != 3 {
		t.Fatalf("kept %+v, want only seq 3", got)
	}
	trimmed, _ := s.SignalsTrimmed()
	if trimmed["signal.a"] != 1 || trimmed["signal.b"] != 2 {
		t.Fatalf("trimmed = %v", trimmed)
	}
}

func TestABatchStopsAtItsLimitAndSaysSo(t *testing.T) {
	s := openStore(t, estate(t, "sig"))
	for i := uint64(1); i <= 5; i++ {
		_ = s.PutSignal(Signal{Seq: i, Kind: "signal.a", At: 1, Source: "seat:x"})
	}
	got, more, _ := s.SignalsAfter(1, 2, all)
	if len(got) != 2 || !more || got[0].Seq != 2 {
		t.Fatalf("got %v more %v", got, more)
	}
}

// A version 1 file, as every estate holds today, gains the tables in place.
func TestAVersionOneStoreGainsTheSignalTables(t *testing.T) {
	name := estate(t, "sig")
	s := openStore(t, name)
	path := s.Path()
	_ = s.Close()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"DROP TABLE signals", "DROP TABLE signals_trimmed", "DROP TABLE shared", "DROP TABLE shared_seq", "PRAGMA user_version = 1"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	again := openStore(t, name)
	if err := again.PutSignal(Signal{Seq: 1, Kind: "signal.a", At: 1, Source: "seat:x"}); err != nil {
		t.Fatalf("after the step: %v", err)
	}
}
