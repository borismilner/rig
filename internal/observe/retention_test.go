package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// small rotates segments and blob files at a few KiB for one test.
func small(t *testing.T) {
	t.Helper()
	seg, blob := segmentBytes, blobBytes
	segmentBytes, blobBytes = 4<<10, 8<<10
	t.Cleanup(func() { segmentBytes, blobBytes = seg, blob })
}

// reread is a fresh store over dir, so a query reads the disk and not the
// ring the writer still holds.
func reread(t *testing.T, dir string) *Store {
	t.Helper()
	r := New(Options{})
	if err := r.Attach(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func fill(s *Store, client string, n int, at int64) {
	for i := range n {
		s.Append(Record{At: at + int64(i), Client: client, Message: "a line of " + client, Attrs: map[string]string{"i": strings.Repeat("x", 40)}})
		if i%20 == 19 {
			s.flush() // rotation is decided per write
		}
	}
	s.flush()
}

// Decision 11: a closed segment is compressed once, and a compressed one
// reads back the same; the open segment is never touched.
func TestARotatedSegmentIsCompressedAndReadsTheSame(t *testing.T) {
	small(t)
	s, dir := attached(t, Options{RateRecords: 1 << 30, RateBurst: 1 << 30})
	defer func() { _ = s.Close() }()
	fill(s, "rigd", 400, 1)
	if err := s.Retain(); err != nil {
		t.Fatal(err)
	}
	segs, err := s.Segments()
	if err != nil || len(segs) < 3 {
		t.Fatalf("%d segments, %v", len(segs), err)
	}
	for _, g := range segs[:len(segs)-1] {
		if !g.Compressed {
			t.Fatalf("closed segment %d is still raw", g.ID)
		}
	}
	if last := segs[len(segs)-1]; last.Compressed && last.Open {
		t.Fatal("the open segment was compressed")
	}
	// Read from disk, not the ring.
	again := New(Options{})
	_ = s.Close()
	if err := again.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Close() }()
	res, err := again.Query(Query{Limit: 1000})
	if err != nil || len(res.Records) != 400 || res.Records[399].At != 400 {
		t.Fatalf("read back %d records, %v", len(res.Records), err)
	}
}

// age ages a segment's file by d.
func age(t *testing.T, g Segment, d time.Duration) {
	t.Helper()
	old := time.Now().Add(-d)
	if err := os.Chtimes(g.Path, old, old); err != nil {
		t.Fatal(err)
	}
}

// At a month a segment moves to archive/ and is still read; at three
// months it is unlinked, with a band, unless pinned.
func TestAgeArchivesThenUnlinksUnlessPinned(t *testing.T) {
	small(t)
	s, dir := attached(t, Options{RateRecords: 1 << 30, RateBurst: 1 << 30})
	defer func() { _ = s.Close() }()
	fill(s, "rigd", 400, 1)
	if err := s.Retain(); err != nil {
		t.Fatal(err)
	}
	segs, _ := s.Segments()
	oldest, pinned, middle := segs[0], segs[1], segs[2]
	age(t, oldest, 100*24*time.Hour)
	age(t, pinned, 100*24*time.Hour)
	age(t, middle, 40*24*time.Hour)
	if changed, err := s.Pin(pinned.ID, true); err != nil || !changed {
		t.Fatalf("pin: %v %v", changed, err)
	}
	if _, err := s.Pin(999_999, true); err == nil {
		t.Fatal("a segment that does not exist was pinned")
	}
	if err := s.Retain(); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Segments()
	byID := map[uint64]Segment{}
	for _, g := range after {
		byID[g.ID] = g
	}
	if _, kept := byID[oldest.ID]; kept {
		t.Fatal("a segment of 100 days was kept")
	}
	if g := byID[pinned.ID]; !g.Pinned || !g.Archived {
		t.Fatalf("the pinned segment is %+v, want archived and kept", g)
	}
	if g := byID[middle.ID]; !g.Archived {
		t.Fatalf("a segment of 40 days is %+v, want archived", g)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive")); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	res, _ := reread(t, dir).Query(Query{Limit: 1000})
	var band *Gap
	for i := range res.Gaps {
		if res.Gaps[i].Cause == CauseUnlinked {
			band = &res.Gaps[i]
		}
	}
	if band == nil || band.Total != pinned.ID-oldest.ID || band.From != 1 {
		t.Fatalf("the unlink band is %+v, want %d records from 1", band, pinned.ID-oldest.ID)
	}
	if res.Records[0].Seq != pinned.ID {
		t.Fatalf("the archive was not read: the oldest record is seq %d", res.Records[0].Seq)
	}

	s = reread(t, dir)
	if changed, _ := s.Pin(pinned.ID, false); !changed {
		t.Fatal("unpin changed nothing")
	}
	if err := s.Retain(); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.Segments(); after[0].ID == pinned.ID {
		t.Fatal("an unpinned segment of 100 days was kept")
	}
}

// THE CEILINGS (plan/49's test): a client at a flood cannot evict a quiet
// client's segment, because the rate ceiling holds it before the size
// ceiling sees it.
func TestAFloodCannotEvictAQuietClient(t *testing.T) {
	if evicted(t, Options{RateRecords: 100, RateBurst: 500}) {
		t.Fatal("the flood evicted the quiet client's segment")
	}
}

// RED CONTROL: the rate ceiling disabled, the same flood unlinks it.
func TestWithoutTheRateCeilingTheFloodEvictsTheQuietClient(t *testing.T) {
	if !evicted(t, Options{RateRecords: 1 << 30, RateBurst: 1 << 30}) {
		t.Fatal("with no rate ceiling the quiet client survived, so the test above proves nothing")
	}
}

func evicted(t *testing.T, opt Options) bool {
	t.Helper()
	small(t)
	clk := &clock{t: time.Now()}
	opt.Now, opt.RetainBytes, opt.BufferBytes = clk.now, 48<<10, 64<<20
	s, dir := attached(t, opt)
	fill(s, "quiet", 60, 1)
	fill(s, "noisy", 20_000, 1000)
	if err := s.Retain(); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	res, err := reread(t, dir).Query(Query{Clients: []string{"quiet"}})
	if err != nil {
		t.Fatal(err)
	}
	return len(res.Records) < 60
}

// A blob file goes once every record that could name it is gone.
func TestABlobFileGoesWithTheLastSegmentThatCouldNameIt(t *testing.T) {
	small(t)
	s, dir := attached(t, Options{RateRecords: 1 << 30, RateBurst: 1 << 30})
	defer func() { _ = s.Close() }()
	big := []byte(`{"pad":"` + strings.Repeat("y", 6000) + `"}`)
	for i := range 6 {
		s.AppendCall(Call{At: time.Now(), Caller: "t", Method: "p.c", Args: big, ArgsSize: len(big)})
		fill(s, "rigd", 60, int64(i*1000+1))
		time.Sleep(2 * time.Millisecond)
	}
	before := len(blobFiles(dir))
	if before < 3 {
		t.Fatalf("only %d blob files were written", before)
	}
	s.rmu.Lock() // no pass running while the ceiling is lowered
	s.opt.RetainBytes = 1
	s.rmu.Unlock()
	if err := s.Retain(); err != nil {
		t.Fatal(err)
	}
	after := blobFiles(dir)
	if len(after) >= before {
		t.Fatalf("%d blob files before and %v after, with every closed segment over the size ceiling", before, after)
	}
}
