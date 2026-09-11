// Command footprint measures what the daemon costs when nothing is asking it
// for anything, and what each registered program adds.
//
// Section 17's budgets - 20 MiB resident, 0.1% of a core, one wakeup a second
// - had no instrument. `make bench-idle` and `make bench-scale` both called
// this binary and neither could run, so every number in section 17 was a
// design target that nothing checked.
//
// IT BRINGS UP ITS OWN DAEMON IN ITS OWN RUNTIME DIRECTORY. internal/paths
// requires XDG_RUNTIME_DIR and section 5f refuses a second daemon in one of
// them, so measuring against whatever happens to be running would both disturb
// a live estate and measure a daemon with somebody else's programs attached.
// A private directory is the only way the number means what it says.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		binary   = flag.String("binary", "build/rigd", "the daemon to measure")
		program  = flag.String("program-binary", "build/fakeapp", "the program to register in scale mode")
		quietFor = flag.Duration("quiet-for", 60*time.Second, "how long to watch an idle daemon")
		settle   = flag.Duration("settle", 3*time.Second, "how long to let it start before the window opens")

		maxRSS     = flag.String("max-rss", "", "fail over this resident size, e.g. 20MiB")
		maxCPU     = flag.Float64("max-cpu", 0, "fail over this percent of one core")
		maxWakeups = flag.Float64("max-wakeups", 0, "fail over this many wakeups a second")

		programs = flag.String("programs", "", "scale mode: register this many programs, e.g. 1,10,50")
		maxDelta = flag.String("max-delta", "", "scale mode: fail if one program costs more than this")
	)
	flag.Parse()

	// os.Exit is called from main and nowhere else, so the deadline's cancel
	// is not skipped by an exit that jumps over it - the measurement's child
	// processes are killed by the context, and an exit that leaked it would
	// leave a daemon holding a runtime directory.
	if err := measure(opts{
		binary:     *binary,
		program:    *program,
		quietFor:   *quietFor,
		settle:     *settle,
		maxRSS:     *maxRSS,
		maxCPU:     *maxCPU,
		maxWakeups: *maxWakeups,
		programs:   *programs,
		maxDelta:   *maxDelta,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "footprint: %v\n", err)
		os.Exit(1)
	}
}

// measure owns the deadline, so main can exit without jumping over a defer.
//
// The whole run is deadlined at the entry point and every child process is
// killed with it, so a daemon that refuses to die cannot outlive the
// measurement and keep its runtime directory.
func measure(o opts) error {
	budget := o.settle + o.quietFor + 2*time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return run(ctx, o)
}

type opts struct {
	binary, program    string
	quietFor, settle   time.Duration
	maxRSS             string
	maxCPU, maxWakeups float64
	programs, maxDelta string
}

func run(ctx context.Context, o opts) error {
	if o.programs != "" {
		return scale(ctx, o)
	}
	return idle(ctx, o)
}

// counts reads "1,10,50".
func counts(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("--programs %q: %w", s, err)
		}
		if n < 0 {
			return nil, fmt.Errorf("--programs %q: negative", s)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--programs %q names no counts", s)
	}
	return out, nil
}
