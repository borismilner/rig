// Package observe is section 49's log store: one per estate, owned by rigd.
//
// Records are held in a bounded ring and written to JSON-lines segments under
// the estate's logs directory: write(2) armed by the first record, never
// fsync (§15). The store is files and rigd is its only writer, so nothing
// here imports SQLite, bbolt or the wire (plan/49 acceptance 6).
package observe

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Defaults are decision 12's, which rigd reads from the logs.* settings.
const (
	DefaultBufferBytes = 2 << 20
	DefaultFlushAfter  = 250 * time.Millisecond
	DefaultFlushBytes  = 256 << 10

	// segmentBytes is where a segment rotates. Not a setting: retention is
	// by total bytes (logs.retention.bytes, slice 4), and this only decides
	// how finely that is cut.
	segmentBytes = 16 << 20

	// The record caps keep one record well inside a 1 MiB frame and a
	// bufio line.
	maxMessage   = 16 << 10
	maxAttrValue = 4 << 10
	maxAttrs     = 64
	maxLine      = 1 << 20

	// Causes a gap band names.
	CauseUncleanClose  = "unclean close"
	CauseRingOverwrite = "ring overwrite"
	CauseWriteFailed   = "write failed"
)

// Record is one log record. Attrs are flattened, groups joined with ".".
type Record struct {
	Seq     uint64            `json:"seq"`
	At      int64             `json:"at"`
	Level   int               `json:"level"`
	Client  string            `json:"client"`
	Message string            `json:"msg"`
	Attrs   map[string]string `json:"attrs,omitempty"`
	Kind    string            `json:"kind,omitempty"` // "" a log record, KindCall a call
}

// size is what a record costs the ring, roughly what it costs encoded.
func (r *Record) size() int {
	n := 64 + len(r.Client) + len(r.Message)
	for k, v := range r.Attrs {
		n += len(k) + len(v) + 8
	}
	return n
}

// clip holds a record to the caps, saying so in the record rather than
// dropping it.
func clip(r *Record) {
	if len(r.Message) > maxMessage {
		r.Message = r.Message[:maxMessage] + "...(clipped)"
	}
	if len(r.Attrs) > maxAttrs {
		keys := slices.Sorted(mapKeys(r.Attrs))
		for _, k := range keys[maxAttrs:] {
			delete(r.Attrs, k)
		}
		r.Attrs["observe.clipped_attrs"] = strconv.Itoa(len(keys) - maxAttrs)
	}
	for k, v := range r.Attrs {
		if len(v) > maxAttrValue && clipsAttr(r, k) {
			r.Attrs[k] = v[:maxAttrValue] + "...(clipped)"
		}
	}
}

func mapKeys(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// Gap is a stretch the store knows it does not hold whole (decision 10).
type Gap struct {
	From     int64  `json:"from"`
	To       int64  `json:"to"`
	Client   string `json:"client"`
	Cause    string `json:"cause"`
	Retained uint64 `json:"retained"`
	Total    uint64 `json:"total"`
}

// Options are the store's knobs; zero values take the defaults.
type Options struct {
	BufferBytes int
	FlushAfter  time.Duration
	FlushBytes  int
	PayloadCap  int // a call payload inline up to this, else in the blob area
	Now         func() time.Time
}

// Store is the estate's log store. It starts in memory; Attach gives it a
// directory, and only then is anything written.
type Store struct {
	opt Options

	wmu sync.Mutex // the writer: held across encoding and write(2); see flush

	mu        sync.Mutex
	seq       uint64
	ring      []Record // newest records, oldest first
	ringBytes int
	// onDisk is the highest seq written to a segment. Once attached, every
	// ring record after it is pending, and the ring never evicts one
	// unwritten while the disk takes writes.
	onDisk       uint64
	pendingBytes int
	gaps         []Gap // losses this run, ring overwrites before Attach
	wake         chan struct{}

	dir      string // "" until Attach
	buf      []byte // the flush's encoding, reused
	torn     bool   // the last write was cut short mid-line
	timer    *time.Timer
	kicked   bool // a flush past FlushBytes is already on its way
	seg      *os.File
	segSize  int64
	writeErr error // the latest write failure, until one succeeds
	closed   bool

	blobs blobArea
}

// New is a store in memory, holding what rigd logs before its estate's
// directory is known.
func New(opt Options) *Store {
	if opt.BufferBytes <= 0 {
		opt.BufferBytes = DefaultBufferBytes
	}
	if opt.FlushAfter <= 0 {
		opt.FlushAfter = DefaultFlushAfter
	}
	if opt.FlushBytes <= 0 {
		opt.FlushBytes = DefaultFlushBytes
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	return &Store{opt: opt}
}

// Durable says whether the store writes to disk. Decision 14's tee runs
// until it does.
func (s *Store) Durable() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir != ""
}

// Dir is the logs directory, "" while the store is in memory.
func (s *Store) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir
}

// Attach opens the store under dir. The sequence resumes after the newest
// record on disk, and the records held in memory until now are renumbered
// after it and written first.
//
// An "open" marker left by the last run is an unclean close: whatever that
// run had not written is gone, and the gap is logged from its newest record
// on disk to now.
func (s *Store) Attach(dir string) error {
	if err := s.attach(dir); err != nil {
		return err
	}
	s.flush()
	return s.WriteErr()
}

func (s *Store) attach(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir != "" {
		return errors.New("observe: the store is already attached at " + s.dir)
	}
	segs := filepath.Join(dir, "segments")
	if err := os.MkdirAll(segs, 0o700); err != nil {
		return err
	}
	last, err := lastOnDisk(segs)
	if err != nil {
		return err
	}
	marker := filepath.Join(dir, "open")
	if _, err := os.Stat(marker); err == nil {
		// The gap ends where this run's records begin, so none of them
		// reads as inside it.
		to := s.opt.Now().UnixNano()
		if len(s.ring) > 0 {
			to = min(to, s.ring[0].At)
		}
		s.gaps = append(s.gaps, Gap{From: last.At, To: to, Client: "*", Cause: CauseUncleanClose})
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		return err
	}
	s.dir = dir
	for i := range s.ring {
		s.ring[i].Seq += last.Seq
	}
	s.seq += last.Seq
	s.onDisk, s.pendingBytes = last.Seq, s.ringBytes
	for i := range s.gaps {
		s.appendCoverageLocked(s.gaps[i])
	}
	return nil
}

// lastOnDisk is the newest whole record in the newest segment that has one.
func lastOnDisk(dir string) (Record, error) {
	names, err := segments(dir)
	if err != nil {
		return Record{}, err
	}
	for _, name := range slices.Backward(names) {
		var last Record
		err := scanSegment(filepath.Join(dir, name), func(r *Record) bool {
			last = *r
			return true
		})
		if err != nil {
			return Record{}, err
		}
		if last.Seq != 0 {
			return last, nil
		}
	}
	return Record{}, nil
}

// segments lists the segment files, oldest first. A name is the first seq
// it holds, zero-padded, so the names sort as the sequence does.
func segments(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".jsonl") {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out, nil
}

func segmentName(firstSeq uint64) string { return fmt.Sprintf("%020d.jsonl", firstSeq) }

func segmentFirst(name string) uint64 {
	n, _ := strconv.ParseUint(strings.TrimSuffix(name, ".jsonl"), 10, 64)
	return n
}

// scanSegment calls fn for each whole record in a segment. A torn line, the
// tail of a write cut short by a crash, is skipped: the unclean-close gap
// already covers it.
func scanSegment(path string, fn func(*Record) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) != nil || r.Seq == 0 {
			continue
		}
		if !fn(&r) {
			return nil
		}
	}
	if errors.Is(sc.Err(), bufio.ErrTooLong) {
		return nil
	}
	return sc.Err()
}

// Append adds a record and assigns its seq. It does not touch the disk: a
// flush past FlushBytes or FlushAfter runs on its own goroutine. The one
// exception is backpressure, when a whole ring is waiting on a flush that
// has not kept up: then this caller writes, rather than the record be lost.
func (s *Store) Append(r Record) {
	clip(&r)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.seq++
	r.Seq = s.seq
	size := r.size()
	s.ring = append(s.ring, r)
	s.ringBytes += size
	if s.dir != "" {
		s.pendingBytes += size
		switch {
		case s.pendingBytes >= s.opt.FlushBytes && !s.kicked:
			s.kicked = true
			if s.timer != nil {
				s.timer.Stop()
			}
			s.timer = time.AfterFunc(0, s.flush)
		case s.timer == nil:
			s.timer = time.AfterFunc(s.opt.FlushAfter, s.flush)
		}
	}
	behind := s.evictLocked()
	if s.wake != nil {
		close(s.wake)
		s.wake = nil
	}
	s.mu.Unlock()
	if behind {
		s.flush()
		s.mu.Lock()
		s.evictLocked()
		s.mu.Unlock()
	}
}

// evictLocked holds the ring to its bound, down to three quarters so the
// copy runs once per quarter of the ring rather than at every append.
//
// Attached, a record is evicted only once it is on disk, and the ring may
// grow to twice its bound waiting for a flush; past that, behind asks the
// caller to flush. Only a disk refusing writes loses records here, as a gap.
// In memory, every eviction is a loss.
func (s *Store) evictLocked() (behind bool) {
	if s.ringBytes <= s.opt.BufferBytes {
		return false
	}
	failing := s.writeErr != nil
	target, n := s.opt.BufferBytes/4*3, 0
	for s.ringBytes > target && n < len(s.ring)-1 {
		if s.dir != "" && s.ring[n].Seq > s.onDisk && (!failing || s.ringBytes <= 2*s.opt.BufferBytes) {
			behind = !failing && s.ringBytes > 2*s.opt.BufferBytes
			break
		}
		s.ringBytes -= s.ring[n].size()
		n++
	}
	if n == 0 {
		return behind
	}
	switch {
	case s.dir == "":
		s.noteLossLocked(CauseRingOverwrite, s.ring[0].At, s.ring[n-1].At, uint64(n))
	case s.ring[n-1].Seq > s.onDisk:
		k := 0
		for k < n && s.ring[k].Seq <= s.onDisk {
			k++
		}
		s.noteLossLocked(CauseWriteFailed, s.ring[k].At, s.ring[n-1].At, uint64(n-k)) //nolint:gosec // k <= n
		s.onDisk = s.ring[n-1].Seq
		s.pendingBytes = 0
		for _, r := range s.ring[n:] {
			s.pendingBytes += r.size()
		}
	}
	// In place: the array is reused, so a full ring allocates nothing.
	k := copy(s.ring, s.ring[n:])
	clear(s.ring[k:])
	s.ring = s.ring[:k]
	return behind
}

// noteLossLocked records a loss. In memory it widens this run's last gap of
// the same cause, or opens one; attached, it is a line in the coverage log.
func (s *Store) noteLossLocked(cause string, from, to int64, n uint64) {
	if s.dir != "" {
		s.appendCoverageLocked(Gap{From: from, To: to, Client: "*", Cause: cause, Total: n})
		return
	}
	if k := len(s.gaps) - 1; k >= 0 && s.gaps[k].Cause == cause {
		s.gaps[k].To = to
		s.gaps[k].Total += n
		return
	}
	s.gaps = append(s.gaps, Gap{From: from, To: to, Client: "*", Cause: cause, Total: n})
}

// flush encodes the ring's records after onDisk and writes them to the
// current segment, rotating when it is full. Never fsync: §15's trade, a
// crash costs at most one flush interval.
//
// TWO LOCKS, so a caller never waits on the disk. wmu is held across the
// encoding and the write(2) and owns the segment; mu is held only to copy
// what is pending and to record what was written. The order is always wmu,
// then mu.
func (s *Store) flush() {
	s.wmu.Lock()
	defer s.wmu.Unlock()

	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.kicked = false
	if s.dir == "" || len(s.ring) == 0 || s.ring[len(s.ring)-1].Seq <= s.onDisk {
		s.mu.Unlock()
		return
	}
	from := max(0, len(s.ring)-int(s.seq-s.onDisk)) //nolint:gosec // bounded by the ring's length
	batch := slices.Clone(s.ring[from:])
	segs := filepath.Join(s.dir, "segments")
	s.mu.Unlock()

	err := s.write(segs, batch)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeErr = err
	if err != nil {
		return
	}
	if last := batch[len(batch)-1].Seq; last > s.onDisk {
		s.onDisk = last
	}
	s.pendingBytes = 0
	for i := len(s.ring) - 1; i >= 0 && s.ring[i].Seq > s.onDisk; i-- {
		s.pendingBytes += s.ring[i].size()
	}
}

// write is the I/O half of flush, under wmu alone.
func (s *Store) write(segs string, batch []Record) error {
	s.buf = s.buf[:0]
	if s.torn {
		s.buf = append(s.buf, '\n')
	}
	for i := range batch {
		b, err := json.Marshal(&batch[i])
		if err != nil {
			continue
		}
		s.buf = append(append(s.buf, b...), '\n')
	}
	if s.seg == nil {
		f, err := os.OpenFile(filepath.Join(segs, segmentName(batch[0].Seq)),
			os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		s.seg, s.segSize = f, 0
	}
	n, err := s.seg.Write(s.buf)
	s.segSize += int64(n)
	// A short write leaves a torn line; the next write starts a fresh one,
	// and the reader skips the torn one.
	s.torn = err != nil && n > 0
	if cap(s.buf) > 4*s.opt.FlushBytes {
		s.buf = nil
	}
	if err == nil && s.segSize >= segmentBytes {
		err = s.seg.Close()
		s.seg = nil
	}
	return err
}

func (s *Store) appendCoverageLocked(g Gap) {
	b, err := json.Marshal(g)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "coverage.jsonl"),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

// WriteErr is the latest write failure, nil once a write succeeds again.
func (s *Store) WriteErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}

// Close writes what is pending and removes the open marker, so the next run
// knows this one closed cleanly. Records appended after Close are dropped.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.wake != nil {
		close(s.wake)
		s.wake = nil
	}
	s.mu.Unlock()

	s.flush()
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return nil
	}
	err := s.writeErr
	if s.seg != nil {
		err = errors.Join(err, s.seg.Close())
		s.seg = nil
	}
	s.blobs.mu.Lock()
	err = errors.Join(err, s.blobs.close())
	s.blobs.mu.Unlock()
	if err == nil {
		err = os.Remove(filepath.Join(s.dir, "open"))
	}
	return err
}
