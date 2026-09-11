package main

import "testing"

// The numbers this command prints are the evidence PLAN.md section 4's
// transport decisions rest on, and README quotes them. A formatter that
// rounds one of them away is a measurement bug wearing a display bug's
// clothes.
func TestASubNanosecondCostIsNotPrintedAsZero(t *testing.T) {
	for _, tc := range []struct {
		name string
		ns   float64
		want string
	}{
		// The one that was wrong. A direct Go call measured 0.79 ns and the
		// table said "0 ns", which reads as free - and free is exactly the
		// claim the daemon architecture is arguing against.
		{"a direct Go call", 0.79, "0.79 ns"},
		{"faster still", 0.4, "0.40 ns"},
		{"exactly zero is still zero", 0, "0.00 ns"},

		// Everything above a nanosecond keeps the precision it had.
		{"shared memory", 133, "133 ns"},
		{"tens of nanoseconds keep a decimal", 12.5, "12.5 ns"},
		{"one-way socket write", 574, "574 ns"},
		{"a socket round trip", 6430, "6.43 µs"},
		{"a big payload", 18000, "18.0 µs"},

		// The boundaries, which the old chain got wrong in the other
		// direction: it used > rather than >=, so exactly 10000 ns printed as
		// "10000 ns" instead of "10.0 µs".
		{"exactly a microsecond", 1000, "1.00 µs"},
		{"exactly ten microseconds", 10000, "10.0 µs"},
		{"exactly a hundred nanoseconds", 100, "100 ns"},
		{"exactly ten nanoseconds", 10, "10.0 ns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := perOp(tc.ns); got != tc.want {
				t.Errorf("perOp(%v) = %q, want %q", tc.ns, got, tc.want)
			}
		})
	}
}
