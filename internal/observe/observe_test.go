package observe

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func attached(t *testing.T, opt Options) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "logs")
	s := New(opt)
	if err := s.Attach(dir); err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func messages(rs []Record) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Client+":"+r.Message)
	}
	return strings.Join(out, " ")
}

func TestRecordsBeforeAttachAreWrittenAndTheSeqResumesAcrossARestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	s := New(Options{})
	s.Append(Record{At: 1, Client: "rigd", Message: "early"})
	if err := s.Attach(dir); err != nil {
		t.Fatal(err)
	}
	s.Append(Record{At: 2, Client: "rigd", Message: "late"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "open")); !os.IsNotExist(err) {
		t.Fatalf("a clean close left the open marker: %v", err)
	}

	again := New(Options{})
	again.Append(Record{At: 3, Client: "rigd", Message: "next run"})
	if err := again.Attach(dir); err != nil {
		t.Fatal(err)
	}
	res, err := again.Query(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if got := messages(res.Records); got != "rigd:early rigd:late rigd:next run" {
		t.Fatalf("records = %q", got)
	}
	if seq := res.Records[2].Seq; seq != 3 {
		t.Fatalf("the next run's first seq = %d, want 3: it resumes after the disk", seq)
	}
	if len(res.Gaps) != 0 {
		t.Fatalf("a clean close is no gap: %+v", res.Gaps)
	}
	_ = again.Close()
}

func TestAnUncleanCloseIsAGapFromTheNewestRecordOnDisk(t *testing.T) {
	s, dir := attached(t, Options{})
	s.Append(Record{At: 100, Client: "rigd", Message: "before the crash"})
	s.flush()
	// No Close: the process died.

	now := time.Unix(0, 500)
	again := New(Options{Now: func() time.Time { return now }})
	if err := again.Attach(dir); err != nil {
		t.Fatal(err)
	}
	res, _ := again.Query(Query{})
	if len(res.Gaps) != 1 {
		t.Fatalf("gaps = %+v, want one", res.Gaps)
	}
	if g := res.Gaps[0]; g.Cause != CauseUncleanClose || g.From != 100 || g.To != 500 || g.Client != "*" {
		t.Fatalf("gap = %+v", g)
	}

	// Red control: the same store closed cleanly leaves no gap behind.
	_ = again.Close()
	third := New(Options{})
	if err := third.Attach(dir); err != nil {
		t.Fatal(err)
	}
	res, _ = third.Query(Query{})
	if len(res.Gaps) != 1 {
		t.Fatalf("a clean close added a gap: %+v", res.Gaps)
	}
	_ = third.Close()
}

func TestTheMergedViewFollowsTheRecordsClockNotArrival(t *testing.T) {
	s, _ := attached(t, Options{})
	defer func() { _ = s.Close() }()
	s.Append(Record{At: 30, Client: "a", Message: "a3"})
	s.Append(Record{At: 10, Client: "b", Message: "b1"})
	s.Append(Record{At: 20, Client: "a", Message: "a2"})
	res, _ := s.Query(Query{})
	if got := messages(res.Records); got != "b:b1 a:a2 a:a3" {
		t.Fatalf("order = %q", got)
	}
	res, _ = s.Query(Query{Clients: []string{"a"}})
	if got := messages(res.Records); got != "a:a2 a:a3" {
		t.Fatalf("client filter = %q", got)
	}
	res, _ = s.Query(Query{Allow: func(c string) bool { return c == "b" }})
	if got := messages(res.Records); got != "b:b1" {
		t.Fatalf("principal filter = %q", got)
	}
}

func TestFiltersLevelTimeGrepAndLimit(t *testing.T) {
	s := New(Options{})
	s.Append(Record{At: 1, Level: -4, Client: "rigd", Message: "debug line"})
	s.Append(Record{At: 2, Level: 0, Client: "rigd", Message: "info line", Attrs: map[string]string{"path": "/x/y"}})
	s.Append(Record{At: 3, Level: 4, Client: "rigd", Message: "warn line"})
	cases := []struct {
		q    Query
		want string
	}{
		{Query{}, "rigd:info line rigd:warn line"},
		{Query{MinLevel: -4}, "rigd:debug line rigd:info line rigd:warn line"},
		{Query{Since: 3}, "rigd:warn line"},
		{Query{Until: 2}, "rigd:info line"},
		{Query{Grep: regexp.MustCompile(`path=/x`)}, "rigd:info line"},
		{Query{MinLevel: -4, Limit: 2}, "rigd:info line rigd:warn line"},
		{Query{After: 2}, "rigd:warn line"},
	}
	for _, c := range cases {
		res, _ := s.Query(c.q)
		if got := messages(res.Records); got != c.want {
			t.Errorf("%+v: %q, want %q", c.q, got, c.want)
		}
	}
	if res, _ := s.Query(Query{MinLevel: -4, Limit: 2}); !res.Truncated {
		t.Error("a limited answer does not say truncated")
	}
}

func TestOlderThanTheRingIsReadFromTheSegments(t *testing.T) {
	s, _ := attached(t, Options{BufferBytes: 512})
	defer func() { _ = s.Close() }()
	for i := range 50 {
		s.Append(Record{At: int64(i + 1), Client: "rigd", Message: "line"})
	}
	if len(s.ring) >= 50 {
		t.Fatalf("the ring holds %d; the test needs it to have evicted", len(s.ring))
	}
	res, err := s.Query(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 50 || len(res.Gaps) != 0 {
		t.Fatalf("got %d records and gaps %+v, want 50 and none", len(res.Records), res.Gaps)
	}
	for i, r := range res.Records {
		if r.Seq != uint64(i+1) {
			t.Fatalf("record %d has seq %d: a duplicate or a hole between disk and ring", i, r.Seq)
		}
	}
}

func TestAnInMemoryStoreSaysWhatItsRingOverwrote(t *testing.T) {
	s := New(Options{BufferBytes: 512})
	for i := range 50 {
		s.Append(Record{At: int64(i + 1), Client: "rigd", Message: "line"})
	}
	res, _ := s.Query(Query{})
	if len(res.Gaps) != 1 || res.Gaps[0].Cause != CauseRingOverwrite {
		t.Fatalf("gaps = %+v", res.Gaps)
	}
	if g := res.Gaps[0]; g.From != 1 || int(g.Total)+len(res.Records) != 50 {
		t.Fatalf("gap %+v with %d records kept does not account for 50", g, len(res.Records))
	}
}

func TestATornLastLineIsSkippedNotFatal(t *testing.T) {
	s, dir := attached(t, Options{})
	s.Append(Record{At: 1, Client: "rigd", Message: "whole"})
	_ = s.Close()
	names, _ := segments(filepath.Join(dir, "segments"))
	f, err := os.OpenFile(filepath.Join(dir, "segments", names[0]), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"seq":2,"at":2,"cli`)
	_ = f.Close()

	again := New(Options{})
	if err := again.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = again.Close() }()
	res, _ := again.Query(Query{})
	if got := messages(res.Records); got != "rigd:whole" {
		t.Fatalf("records = %q", got)
	}
}

func TestTheFirstRecordArmsTheFlush(t *testing.T) {
	s, dir := attached(t, Options{FlushAfter: 20 * time.Millisecond})
	defer func() { _ = s.Close() }()
	s.Append(Record{At: 1, Client: "rigd", Message: "x"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		last, _ := lastOnDisk(filepath.Join(dir, "segments"))
		if last.Seq == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the record never reached a segment")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWaitWakesOnTheNextMatchingRecord(t *testing.T) {
	s := New(Options{})
	s.Append(Record{At: 1, Client: "rigd", Message: "old"})
	got := make(chan Result, 1)
	go func() {
		res, _ := s.Wait(context.Background(), Query{After: 1}, 5*time.Second)
		got <- res
	}()
	time.Sleep(20 * time.Millisecond)
	s.Append(Record{At: 2, Client: "rigd", Message: "new"})
	select {
	case res := <-got:
		if messages(res.Records) != "rigd:new" || res.Latest != 2 {
			t.Fatalf("woke with %q latest %d", messages(res.Records), res.Latest)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the wait never woke")
	}
	res, _ := s.Wait(context.Background(), Query{After: 2}, 10*time.Millisecond)
	if len(res.Records) != 0 {
		t.Fatalf("a timed-out wait answered %q", messages(res.Records))
	}
}

func TestTheTeeRunsUntilTheStoreIsDurableAndNotAfter(t *testing.T) {
	var stderr bytes.Buffer
	s := New(Options{})
	lv := new(slog.LevelVar)
	log := slog.New(NewHandler(s, "rigd", lv, &stderr))
	log.Info("an info line")
	log.Warn("before the store opens")
	if out := stderr.String(); !strings.Contains(out, "before the store opens") || strings.Contains(out, "an info line") {
		t.Fatalf("before attach, stderr = %q: want the warning and not the info", out)
	}
	if err := s.Attach(filepath.Join(t.TempDir(), "logs")); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	stderr.Reset()
	log.Error("after the store opens")
	if stderr.Len() != 0 {
		t.Fatalf("after attach, stderr = %q, want nothing", stderr.String())
	}
	res, _ := s.Query(Query{})
	if got := messages(res.Records); got != "rigd:an info line rigd:before the store opens rigd:after the store opens" {
		t.Fatalf("store = %q", got)
	}
}

func TestTheHandlerFlattensGroupsAndHonoursTheLevel(t *testing.T) {
	s := New(Options{})
	lv := new(slog.LevelVar)
	log := slog.New(NewHandler(s, "rigd", lv, nil))
	log.With("estate", "e").WithGroup("call").Info("hi", "verb", "x", slog.Group("arg", "n", 3))
	log.Debug("hidden")
	lv.Set(slog.LevelDebug)
	log.Debug("shown")
	res, _ := s.Query(Query{MinLevel: -4})
	if got := messages(res.Records); got != "rigd:hi rigd:shown" {
		t.Fatalf("records = %q", got)
	}
	a := res.Records[0].Attrs
	if a["estate"] != "e" || a["call.verb"] != "x" || a["call.arg.n"] != "3" {
		t.Fatalf("attrs = %v", a)
	}
}

func TestARecordIsClippedNotDropped(t *testing.T) {
	s := New(Options{})
	s.Append(Record{At: 1, Client: "rigd", Message: strings.Repeat("m", maxMessage+10)})
	res, _ := s.Query(Query{})
	if len(res.Records) != 1 || !strings.HasSuffix(res.Records[0].Message, "...(clipped)") {
		t.Fatal("an over-long message was not clipped")
	}
}

// BenchmarkHandler is what a rigd call site pays for one record: the
// handler and a ring slot. §8's 627 ns for the stdlib text handler is the
// ceiling (plan/49 decision 3). The encoding and the write are the flush's,
// off the caller's path, and BenchmarkSustained measures them.
func BenchmarkHandler(b *testing.B) {
	s := New(Options{})
	log := slog.New(NewHandler(s, "rigd", slog.LevelInfo, nil))
	b.ReportAllocs()
	for b.Loop() {
		log.Info("a call", "verb", "rig.ping", "ms", 3)
	}
}

// BenchmarkSustained is records per second to disk at saturation, where
// backpressure makes the callers pay for the encoding and the write too.
func BenchmarkSustained(b *testing.B) {
	s := New(Options{})
	if err := s.Attach(filepath.Join(b.TempDir(), "logs")); err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	log := slog.New(NewHandler(s, "rigd", slog.LevelInfo, nil))
	b.ReportAllocs()
	for b.Loop() {
		log.Info("a call", "verb", "rig.ping", "ms", 3)
	}
}
