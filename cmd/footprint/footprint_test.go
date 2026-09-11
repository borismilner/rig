package main

import (
	"os"
	"testing"
	"time"
)

func TestSizesAreBinaryNotDecimal(t *testing.T) {
	// Section 17 quotes MiB throughout. Reading 20MiB as 20 million bytes is
	// a 4.9% error, which is larger than several of the budget lines it would
	// be checking, and it would silently loosen every one of them.
	for _, c := range []struct {
		in   string
		want int64
	}{
		{"20MiB", 20 * 1 << 20},
		{"500KiB", 500 * 1 << 10},
		{"1GiB", 1 << 30},
		{"1024", 1024},
		{"20MB", 20_000_000},
	} {
		got, err := parseSize(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s parsed as %d, want %d", c.in, got, c.want)
		}
	}
}

func TestASizeThatIsNotASizeIsRefused(t *testing.T) {
	for _, in := range []string{"twenty", "20 gigabytes", "MiB"} {
		if _, err := parseSize(in); err == nil {
			t.Errorf("%q was accepted as a size", in)
		}
	}
}

func TestProgramCountsAreRead(t *testing.T) {
	got, err := counts("1,10,50")
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 10 || got[2] != 50 {
		t.Fatalf("read %v", got)
	}
	if _, err := counts("1,rabbit"); err == nil {
		t.Error("a non-number was accepted as a program count")
	}
	if _, err := counts(""); err == nil {
		t.Error("an empty list was accepted")
	}
}

// take() parses /proc, and the format is the kernel's rather than ours, so the
// test reads a real process: this one.
func TestAProcessCanBeSampled(t *testing.T) {
	s, err := take(os.Getpid())
	if err != nil {
		t.Fatalf("sampling this test's own process: %v", err)
	}
	if s.RSSBytes <= 0 {
		t.Errorf("resident size read as %d, which no running process has", s.RSSBytes)
	}
	if s.Switches <= 0 {
		t.Errorf("context switches read as %d; both counters are missing or "+
			"the status field names have changed", s.Switches)
	}
}

func TestSamplingAProcessThatIsNotThereFails(t *testing.T) {
	// A pid that cannot exist: the kernel's pid_max is well under this.
	if _, err := take(1 << 30); err == nil {
		t.Fatal("sampling a pid that does not exist reported no error, so a " +
			"dead daemon would be measured as zero rather than as a failure")
	}
}

// The rates are per second, and getting the divisor wrong scales every number
// by the length of the window - which still looks plausible.
func TestRatesArePerSecond(t *testing.T) {
	start := time.Now()
	first := sample{At: start, CPUTicks: 100, Switches: 1000, RSSBytes: 1}
	last := sample{At: start.Add(10 * time.Second), CPUTicks: 200, Switches: 1500, RSSBytes: 42}

	w := between(first, last)
	if w.Seconds != 10 {
		t.Fatalf("window is %v seconds", w.Seconds)
	}
	// 100 ticks at 100Hz is one second of CPU across ten seconds of wall time.
	if w.CPUPercent < 9.9 || w.CPUPercent > 10.1 {
		t.Errorf("cpu read as %.3f%%, want about 10%%", w.CPUPercent)
	}
	if w.WakeupsPerS != 50 {
		t.Errorf("wakeups read as %.2f/s, want 50", w.WakeupsPerS)
	}
	// RSS is the settled value, not a rate.
	if w.RSSBytes != 42 {
		t.Errorf("resident read as %d, want the last sample's 42", w.RSSBytes)
	}
}

// A zero-length window must not divide by zero and report an infinite rate.
func TestAZeroLengthWindowDoesNotDivideByZero(t *testing.T) {
	at := time.Now()
	w := between(sample{At: at, RSSBytes: 7}, sample{At: at, RSSBytes: 7})
	if w.CPUPercent != 0 || w.WakeupsPerS != 0 {
		t.Fatalf("a zero-length window reported cpu %v and wakeups %v",
			w.CPUPercent, w.WakeupsPerS)
	}
}
