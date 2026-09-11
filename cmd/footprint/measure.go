package main

import (
	"context"
	"fmt"
	"time"
)

// idle measures a daemon nobody is talking to, against section 17's budgets.
func idle(ctx context.Context, o opts) error {
	e, err := start(ctx, o.binary)
	if err != nil {
		return err
	}
	defer e.stop()

	// The settle period is outside the measured window on purpose. Start-up
	// allocation and the first GC are not idle behaviour, and including them
	// makes the CPU number a function of how long you happened to watch.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(o.settle):
	}

	w, err := e.watch(ctx, o.quietFor)
	if err != nil {
		return err
	}

	fmt.Printf("idle, %s after a %s settle\n\n", fmt.Sprintf("%.0fs", w.Seconds), o.settle)
	fmt.Printf("  %-10s %14s", "resident", showSize(w.RSSBytes))
	var failures []string

	if o.maxRSS != "" {
		limit, err := parseSize(o.maxRSS)
		if err != nil {
			return err
		}
		fmt.Printf("   budget %s", showSize(limit))
		if w.RSSBytes > limit {
			fmt.Print("   OVER")
			failures = append(failures, fmt.Sprintf(
				"resident %s is over the %s budget", showSize(w.RSSBytes), showSize(limit)))
		}
	}
	fmt.Println()

	fmt.Printf("  %-10s %13.3f%%", "cpu", w.CPUPercent)
	if o.maxCPU > 0 {
		fmt.Printf("   budget %.3f%%", o.maxCPU)
		if w.CPUPercent > o.maxCPU {
			fmt.Print("   OVER")
			failures = append(failures, fmt.Sprintf(
				"cpu %.3f%% is over the %.3f%% budget", w.CPUPercent, o.maxCPU))
		}
	}
	fmt.Println()

	fmt.Printf("  %-10s %14.2f", "wakeups/s", w.WakeupsPerS)
	if o.maxWakeups > 0 {
		fmt.Printf("   budget %.2f", o.maxWakeups)
		if w.WakeupsPerS > o.maxWakeups {
			fmt.Print("   OVER")
			failures = append(failures, fmt.Sprintf(
				"wakeups %.2f/s is over the %.2f/s budget", w.WakeupsPerS, o.maxWakeups))
		}
	}
	fmt.Println()

	if len(failures) > 0 {
		fmt.Println()
		for _, f := range failures {
			fmt.Printf("  %s\n", f)
		}
		return fmt.Errorf("%d of section 17's idle budgets are exceeded", len(failures))
	}
	return nil
}

// scale measures what each registered program adds to the daemon.
func scale(ctx context.Context, o opts) error {
	want, err := counts(o.programs)
	if err != nil {
		return err
	}

	e, err := start(ctx, o.binary)
	if err != nil {
		return err
	}
	defer e.stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(o.settle):
	}

	base, err := take(e.pid())
	if err != nil {
		return err
	}
	fmt.Printf("scale, from a baseline of %s with no programs\n\n", showSize(base.RSSBytes))
	fmt.Printf("  %8s %14s %14s %16s\n", "programs", "resident", "over base", "per program")

	var worst float64
	var worstAt int
	for _, n := range want {
		if n > len(e.programs) {
			if err := e.addPrograms(ctx, o.program, n-len(e.programs)); err != nil {
				return err
			}
		}
		s, err := take(e.pid())
		if err != nil {
			return err
		}
		delta := s.RSSBytes - base.RSSBytes
		per := float64(delta)
		if n > 0 {
			per /= float64(n)
		}
		fmt.Printf("  %8d %14s %14s %16s\n",
			n, showSize(s.RSSBytes), showSize(delta), showSize(int64(per)))
		if n > 0 && per > worst {
			worst, worstAt = per, n
		}
	}

	if o.maxDelta == "" {
		return nil
	}
	limit, err := parseSize(o.maxDelta)
	if err != nil {
		return err
	}
	fmt.Printf("\n  worst per-program cost %s at %d programs, budget %s\n",
		showSize(int64(worst)), worstAt, showSize(limit))
	if int64(worst) > limit {
		return fmt.Errorf("a program costs %s, over the %s budget",
			showSize(int64(worst)), showSize(limit))
	}
	return nil
}
