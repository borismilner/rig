package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// userHZ is the kernel's clock tick, which /proc/<pid>/stat counts CPU in.
//
// 100 on every Linux this runs on. It is a constant rather than a getconf
// call because reading it wrong scales every CPU number by a whole factor,
// and a wrong factor that still looks plausible is worse than a broken build.
const userHZ = 100

// sample is one reading of a process, taken from /proc.
type sample struct {
	At       time.Time
	RSSBytes int64
	CPUTicks int64 // utime + stime
	Switches int64 // voluntary + nonvoluntary context switches
}

// take reads one sample for a pid.
func take(pid int) (sample, error) {
	s := sample{At: time.Now()}

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return s, fmt.Errorf("reading stat: %w", err)
	}
	// The comm field is parenthesised and may contain spaces, so fields are
	// counted from after the last ')' rather than from the start.
	endComm := strings.LastIndex(string(stat), ")")
	if endComm < 0 {
		return s, fmt.Errorf("stat for pid %d has no comm field", pid)
	}
	f := strings.Fields(string(stat)[endComm+1:])
	// After comm, field 1 is state; utime is 12 and stime 13 counting state
	// as 1, which is index 11 and 12 here.
	if len(f) < 13 {
		return s, fmt.Errorf("stat for pid %d has %d fields after comm", pid, len(f))
	}
	utime, err := strconv.ParseInt(f[11], 10, 64)
	if err != nil {
		return s, fmt.Errorf("utime: %w", err)
	}
	stime, err := strconv.ParseInt(f[12], 10, 64)
	if err != nil {
		return s, fmt.Errorf("stime: %w", err)
	}
	s.CPUTicks = utime + stime

	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return s, fmt.Errorf("reading status: %w", err)
	}
	for _, line := range strings.Split(string(status), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch name {
		case "VmRSS":
			kb, err := strconv.ParseInt(strings.Fields(value)[0], 10, 64)
			if err != nil {
				return s, fmt.Errorf("VmRSS: %w", err)
			}
			s.RSSBytes = kb * 1024
		case "voluntary_ctxt_switches", "nonvoluntary_ctxt_switches":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return s, fmt.Errorf("%s: %w", name, err)
			}
			s.Switches += n
		}
	}
	return s, nil
}

// window is what two samples say happened between them.
type window struct {
	Seconds     float64
	RSSBytes    int64   // at the END of the window: what it settled at
	CPUPercent  float64 // of one core
	WakeupsPerS float64
}

// between reports the rates across two samples.
func between(first, last sample) window {
	secs := last.At.Sub(first.At).Seconds()
	if secs <= 0 {
		return window{RSSBytes: last.RSSBytes}
	}
	return window{
		Seconds:     secs,
		RSSBytes:    last.RSSBytes,
		CPUPercent:  float64(last.CPUTicks-first.CPUTicks) / userHZ / secs * 100,
		WakeupsPerS: float64(last.Switches-first.Switches) / secs,
	}
}
