package daemon

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// TestSoakReturnsToBaseline drives a real daemon over its socket for a while
// - connections opened and closed, records written and read, programs
// registered and gone - and asserts that afterwards the goroutine count is
// back to its baseline and the heap has not grown past a bound. A leak that
// costs one goroutine or a few kilobytes per call is invisible in a unit test
// and fatal in a daemon that runs for weeks; this is the test that sees it.
//
// Off by default, because it is measured in minutes: RIG_SOAK=3m runs it for
// three minutes. `go test -run TestSoak ./internal/daemon` with RIG_SOAK set.
func TestSoakReturnsToBaseline(t *testing.T) {
	d, err := time.ParseDuration(os.Getenv("RIG_SOAK"))
	if err != nil || d <= 0 {
		t.Skip("set RIG_SOAK to a duration, e.g. RIG_SOAK=3m, to run the soak")
	}
	sock := upRecordDaemon(t)

	settle := func() (int, uint64) {
		var ms runtime.MemStats
		for range 3 {
			runtime.GC()
			time.Sleep(100 * time.Millisecond)
		}
		runtime.ReadMemStats(&ms)
		return runtime.NumGoroutine(), ms.HeapAlloc
	}

	// Warm up first, so caches and pools the store fills once are in the
	// baseline rather than counted as growth.
	// Clients are dialled and closed here directly, not through the test
	// helpers: those register a t.Cleanup per connection, so every client of
	// every round would stay referenced until the test ended and the heap
	// would measure the test, not the daemon (the first run of this soak did
	// exactly that, 8 GB over three minutes).
	round := func(i int) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, err := client.Dial(sock)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.Call(ctx, "rig.announce", &verbsv1.AnnounceRequest{
			Seat: "soak-" + strconv.Itoa(i%4), Purpose: "soak", Activity: "soaking",
		}, &verbsv1.AnnounceResponse{})
		id := "S" + strconv.Itoa(i%200)
		_ = c.Call(ctx, "rig.record.put", &verbsv1.RecordPutRequest{
			Id: id, Kind: "note", Project: "soak", Body: strconv.Itoa(i),
		}, &verbsv1.RecordPutResponse{})
		_ = c.Call(ctx, "rig.record.get", &verbsv1.RecordGetRequest{Id: id}, &verbsv1.RecordGetResponse{})
		_ = c.Call(ctx, "rig.record.query", &verbsv1.RecordQueryRequest{Project: "soak", Limit: 50},
			&verbsv1.RecordQueryResponse{})
		_ = c.Call(ctx, "rig.peers", &verbsv1.PeersRequest{}, &verbsv1.PeersResponse{})
		_ = c.Close()

		p, err := client.Dial(sock)
		if err != nil {
			t.Fatal(err)
		}
		name := "soak" + strconv.Itoa(i%50)
		p.Handle(func(string, []byte) (proto.Message, error) {
			return &rigv1.PingResponse{Program: name}, nil
		})
		_, _ = p.Hello(ctx, testDeclaration(name))
		_ = p.Close()
	}
	for i := range 200 {
		round(i)
	}
	g0, h0 := settle()

	n, end := 0, time.Now().Add(d)
	for time.Now().Before(end) {
		round(n)
		n++
	}
	g1, h1 := settle()
	t.Logf("%d rounds in %s: goroutines %d -> %d, heap %d KiB -> %d KiB",
		n, d, g0, g1, h0>>10, h1>>10)

	if g1 > g0+2 {
		t.Errorf("goroutines grew from %d to %d over %d rounds", g0, g1, n)
	}
	// Heap: generous, because the record store legitimately holds more rows
	// (200 ids, each with more versions). A leak per round grows without that
	// bound; 64 MiB over the baseline is far past what the rows explain.
	if h1 > h0+64<<20 {
		t.Errorf("heap grew from %d KiB to %d KiB over %d rounds", h0>>10, h1>>10, n)
	}
}
