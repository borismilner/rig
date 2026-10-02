package observe

import (
	"sync"
	"testing"
	"time"
)

// clock is a test clock the store refills its buckets by.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// THE RATE CEILING (decision 11): a flood is sampled to the client's
// ceiling, a quiet client loses nothing, and the band says retained/total
// so the counts stay true.
func TestAFloodIsSampledToTheCeilingAndTheBandCountsIt(t *testing.T) {
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	s, _ := attached(t, Options{RateRecords: 100, RateBurst: 500, Now: clk.now})
	at := clk.now().UnixNano()
	kept := 0
	for i := range 10_000 {
		if s.Append(Record{At: at + int64(i), Client: "noisy", Message: "flood"}) {
			kept++
		}
		if i%100 == 99 {
			s.Append(Record{At: at + int64(i), Client: "quiet", Message: "heartbeat"})
		}
	}
	// Half a second of flood: the burst, plus nothing refilled.
	if kept != 500 {
		t.Fatalf("the ceiling kept %d of 10000, want the burst of 500", kept)
	}
	clk.add(2 * time.Second)
	if !s.Append(Record{At: clk.now().UnixNano(), Client: "noisy", Message: "after"}) {
		t.Fatal("a record after the flood, with the bucket refilled, was sampled")
	}

	res, err := s.Query(Query{Clients: []string{"quiet"}})
	if err != nil || len(res.Records) != 100 {
		t.Fatalf("the quiet client kept %d of 100: %v", len(res.Records), err)
	}
	res, err = s.Query(Query{Clients: []string{"noisy"}})
	if err != nil {
		t.Fatal(err)
	}
	var band *Gap
	for i := range res.Gaps {
		if res.Gaps[i].Cause == CauseSampled {
			band = &res.Gaps[i]
		}
	}
	if band == nil || band.Client != "noisy" || band.Total != 9_500 || band.Retained != 0 {
		t.Fatalf("the sampled band is %+v, want noisy 0/9500", band)
	}
	if len(res.Records) != 501 {
		t.Fatalf("noisy reads %d records, want 501", len(res.Records))
	}
}

// RED CONTROL: with a ceiling too high to bind, the same flood is kept
// whole, so the test above measures the ceiling and not something else.
func TestWithoutTheCeilingTheFloodIsKept(t *testing.T) {
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	s, _ := attached(t, Options{RateRecords: 1 << 30, RateBurst: 1 << 30, Now: clk.now, BufferBytes: 64 << 20})
	kept := 0
	for i := range 10_000 {
		if s.Append(Record{At: int64(i + 1), Client: "noisy", Message: "flood"}) {
			kept++
		}
	}
	if kept != 10_000 {
		t.Fatalf("kept %d", kept)
	}
}

// A band still open is part of a read, so a view during a flood is not
// told it holds everything; and an audit entry is never sampled.
func TestAnOpenBandIsReadAndAnAuditIsNeverSampled(t *testing.T) {
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	s := New(Options{RateRecords: 1, RateBurst: 1, Now: clk.now})
	s.Append(Record{At: 1, Client: "rigd", Message: "one"})
	if s.Append(Record{At: 2, Client: "rigd", Message: "two"}) {
		t.Fatal("past a burst of one, a second record was kept")
	}
	res, _ := s.Query(Query{})
	if len(res.Gaps) != 1 || res.Gaps[0].Total != 1 || res.Gaps[0].Cause != CauseSampled {
		t.Fatalf("the open band read as %+v", res.Gaps)
	}
	for range 10 {
		if !s.Append(Record{At: 3, Client: "rigd", Kind: KindAudit, Message: "read"}) {
			t.Fatal("an audit entry was sampled")
		}
	}
}

// A principal reading only its own records is not shown another client's
// bands either: a band names its client.
func TestAGapOfAnotherClientIsFilteredWithItsRecords(t *testing.T) {
	s := New(Options{RateRecords: 1, RateBurst: 1})
	s.Append(Record{At: 1, Client: "shelf"})
	s.Append(Record{At: 2, Client: "shelf"})
	res, _ := s.Query(Query{Allow: func(c string) bool { return c == "graft" }})
	if len(res.Gaps) != 0 {
		t.Fatalf("graft was shown %+v", res.Gaps)
	}
}
