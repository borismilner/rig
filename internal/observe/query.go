package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

// Query is the read side's filters. Zero values are open: every time, every
// client, info and above.
type Query struct {
	Since, Until int64 // unix nanos
	Clients      []string
	MinLevel     int
	Grep         *regexp.Regexp
	After        uint64 // only records after this seq
	Limit        int    // newest kept; <= 0 is no limit
	MaxBytes     int    // the same, by size; <= 0 is no limit
	// Allow is the principal filter (decision 13); nil allows every client.
	Allow func(client string) bool
	// Kind reads one record kind: "" log records, KindCall, KindAudit.
	// `rig logs` reads only the first; a call record's reader is §15's
	// `rig history`, M5's.
	Kind string
}

// Result is a query's answer, time-ordered, oldest first.
type Result struct {
	Records   []Record
	Gaps      []Gap
	Latest    uint64
	Truncated bool
}

func (q *Query) match(r *Record) bool {
	switch {
	case r.Seq <= q.After,
		r.Kind != q.Kind,
		r.Level < q.MinLevel,
		q.Since != 0 && r.At < q.Since,
		q.Until != 0 && r.At > q.Until,
		len(q.Clients) > 0 && !slices.Contains(q.Clients, r.Client),
		q.Allow != nil && !q.Allow(r.Client):
		return false
	}
	return q.Grep == nil || q.Grep.MatchString(r.Message) || grepAttrs(q.Grep, r.Attrs)
}

func grepAttrs(re *regexp.Regexp, attrs map[string]string) bool {
	for k, v := range attrs {
		if re.MatchString(k + "=" + v) {
			return true
		}
	}
	return false
}

func (q *Query) overlaps(g *Gap) bool {
	return (q.Until == 0 || g.From <= q.Until) && (q.Since == 0 || g.To >= q.Since) &&
		(g.Client == "*" || (len(q.Clients) == 0 || slices.Contains(q.Clients, g.Client)) &&
			(q.Allow == nil || q.Allow(g.Client)))
}

// Query answers the records matching q: from the ring, and from the
// segments for anything older than the ring holds.
func (s *Store) Query(q Query) (Result, error) {
	res, _, err := s.query(&q)
	return res, err
}

func (s *Store) query(q *Query) (Result, <-chan struct{}, error) {
	// Attached, the ring evicts only what is on disk, so the segments
	// before the ring's oldest and the ring itself hold every record.
	s.mu.Lock()
	ring := slices.Clone(s.ring)
	dir, latest := s.dir, s.seq
	gaps := slices.Clone(s.gaps)
	open := s.openBandsLocked()
	if s.wake == nil && !s.closed {
		s.wake = make(chan struct{})
	}
	wake := s.wake
	s.mu.Unlock()

	var out []Record
	oldest := latest + 1
	if len(ring) > 0 {
		oldest = ring[0].Seq
	}
	if dir != "" {
		// Attached, the coverage log holds every gap, this run's included.
		var err error
		if gaps, err = readCoverage(filepath.Join(dir, "coverage.jsonl")); err != nil {
			return Result{}, wake, err
		}
		if q.After+1 < oldest {
			if out, err = scanDisk(filepath.Join(dir, "segments"), q, oldest); err != nil {
				return Result{}, wake, err
			}
		}
	}
	for i := range ring {
		if q.match(&ring[i]) {
			out = append(out, ring[i])
		}
	}
	res := Result{Latest: latest}
	gaps = append(gaps, open...)
	for i := range gaps {
		if q.overlaps(&gaps[i]) {
			res.Gaps = append(res.Gaps, gaps[i])
		}
	}
	// Time-ordered by the record's own clock, not by arrival (plan/49's
	// merged-view test); seq breaks ties.
	slices.SortStableFunc(out, func(a, b Record) int {
		if a.At != b.At {
			return cmpInt(a.At, b.At)
		}
		return cmpInt(int64(a.Seq), int64(b.Seq)) //nolint:gosec // seqs stay far below 2^63
	})
	res.Records, res.Truncated = keepNewest(out, q.Limit, q.MaxBytes)
	return res, wake, nil
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// keepNewest drops the oldest records past either bound.
func keepNewest(rs []Record, limit, maxBytes int) ([]Record, bool) {
	start := 0
	if limit > 0 && len(rs) > limit {
		start = len(rs) - limit
	}
	if maxBytes > 0 {
		total := 0
		for i := len(rs) - 1; i >= start; i-- {
			total += rs[i].size()
			if total > maxBytes {
				start = i + 1
				break
			}
		}
	}
	return rs[start:], start > 0
}

// scanDisk reads the matching records older than the ring's oldest from the
// segments that can hold any after the cursor.
func scanDisk(dir string, q *Query, ringOldest uint64) ([]Record, error) {
	names, err := segments(dir)
	if err != nil {
		return nil, err
	}
	var out []Record
	for i, name := range names {
		// The next segment's first seq bounds this one's last.
		if i+1 < len(names) && segmentFirst(names[i+1]) <= q.After+1 {
			continue
		}
		if segmentFirst(name) >= ringOldest {
			break
		}
		path := filepath.Join(dir, name)
		if q.Since != 0 {
			if st, err := os.Stat(path); err == nil && st.ModTime().UnixNano() < q.Since {
				continue // nothing in it was written after since
			}
		}
		err := scanSegment(path, func(r *Record) bool {
			if r.Seq >= ringOldest {
				return false
			}
			if q.match(r) {
				out = append(out, *r)
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readCoverage(path string) ([]Gap, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []Gap
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var g Gap
		if json.Unmarshal(sc.Bytes(), &g) == nil {
			out = append(out, g)
		}
	}
	return out, sc.Err()
}

// Wait answers as Query does once a record after q.After matches, or what
// there is when timeout passes or ctx ends.
func (s *Store) Wait(ctx context.Context, q Query, timeout time.Duration) (Result, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		res, wake, err := s.query(&q)
		if err != nil || len(res.Records) > 0 || wake == nil {
			return res, err
		}
		select {
		case <-wake:
		case <-timer.C:
			return res, nil
		case <-ctx.Done():
			return res, ctx.Err()
		}
	}
}
