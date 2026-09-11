package main

import (
	"fmt"
	"strconv"
	"strings"
)

// units are the suffixes the Makefile writes, binary rather than decimal:
// section 17 quotes MiB throughout, and a 5% error from treating MiB as MB is
// larger than several of the budget lines it would be checking.
var units = []struct {
	suffix string
	scale  int64
}{
	{"GiB", 1 << 30},
	{"MiB", 1 << 20},
	{"KiB", 1 << 10},
	{"GB", 1e9},
	{"MB", 1e6},
	{"KB", 1e3},
	{"B", 1},
}

// parseSize reads "20MiB", "500KiB", "1024".
func parseSize(s string) (int64, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, nil
	}
	for _, u := range units {
		if !strings.HasSuffix(t, u.suffix) {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(t, u.suffix)), 64)
		if err != nil {
			return 0, fmt.Errorf("%q: %w", s, err)
		}
		return int64(n * float64(u.scale)), nil
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size: write bytes, or a suffix like 20MiB", s)
	}
	return n, nil
}

// showSize writes a size the way the budget does.
func showSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
