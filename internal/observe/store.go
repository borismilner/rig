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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Defaults are decision 12's, which rigd reads from the logs.* settings.
const (
	DefaultBufferBytes = 2 << 20
	DefaultFlushAfter  = 250 * time.Millisecond
	DefaultFlushBytes  = 256 << 10

	// The per-client rate ceiling (decision 11), enforced before the size
	// ceiling so one noisy client cannot evict the estate's record. §15
	// gives no number; these are the seat's.
	DefaultRateRecords = 1000
	DefaultRateBurst   = 5000

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
	CauseSampled       = "sampled"
	CauseClientDropped = "dropped by the client"
)

// KindAudit marks an audit entry, which the rate ceiling never samples (§15).
const KindAudit = "audit"

// sampleWindow is how long one sampled band runs before it is written.
const sampleWindow = time.Second

// segmentBytes is where a segment rotates. Not a setting: retention is by
// total bytes (logs.retention.bytes), and this only decides how finely that
// is cut. A variable so a test can rotate without writing 16 MiB.
var segmentBytes int64 = 16 << 20

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
	RateRecords int // per client per second, past the burst
	RateBurst   int
	// The size and age ceilings (retention.go); zero takes the defaults.
	RetainBytes  int64
	ArchiveAfter time.Duration
	DeleteAfter  time.Duration
	Now          func() time.Time
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
	segID    uint64 // the open segment's first seq; under wmu
	segSize  int64
	writeErr error // the latest write failure, until one succeeds
	closed   bool

	blobs blobArea

	// retention: rmu is held across a pass, which never takes wmu.
	rmu         sync.Mutex
	retainAgain atomic.Bool
	retainErr   error // under mu
	retainTick  *time.Timer

	buckets map[string]*bucket // the rate ceiling's, per client
}

// bucket is one client's token bucket, and the sampled band open on it.
type bucket struct {
	tokens float64
	last   int64 // when tokens was last refilled, by the store's clock
	opened int64 // when the open band began, by the same clock
	band   Gap   // Total 0 when none is open
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
	if opt.RateRecords <= 0 {
		opt.RateRecords = DefaultRateRecords
	}
	if opt.RateBurst <= 0 {
		opt.RateBurst = DefaultRateBurst
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	return &Store{opt: opt, buckets: map[string]*bucket{}}
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
	s.kickRetain()
	s.armRetain()
	return s.WriteErr()
}

// armRetain checks age hourly between rotations, until Close.
func (s *Store) armRetain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.retainTick = time.AfterFunc(retainEvery, func() {
		s.kickRetain()
		s.armRetain()
	})
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
	last, err := lastOnDisk(dir)
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

// lastOnDisk is the newest whole record in the newest segment that has one,
// live or archived.
func lastOnDisk(dir string) (Record, error) {
	segs, err := listSegments(dir)
	if err != nil {
		return Record{}, err
	}
	for _, g := range slices.Backward(segs) {
		var last Record
		err := scanSegment(g.Path, func(r *Record) bool {
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

func segmentName(firstSeq uint64) string { return fmt.Sprintf("%020d.jsonl", firstSeq) }

// scanSegment calls fn for each whole record in a segment. A torn line, the
// tail of a write cut short by a crash, is skipped: the unclean-close gap
// already covers it.
func scanSegment(path string, fn func(*Record) bool) error {
	f, err := openSegment(path)
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
//
// It answers false for a record the client's rate ceiling sampled out.
func (s *Store) Append(r Record) bool {
	clip(&r)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	if r.Kind != KindAudit && !s.admitLocked(&r) {
		s.mu.Unlock()
		return false
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
	return true
}

// admitLocked is the rate ceiling: a token bucket per client, refilled by
// the store's clock rather than the record's, which a program writes. A
// record past it is counted into the client's sampled band, and the band is
// written once it has run a second, so retained/total stays true.
func (s *Store) admitLocked(r *Record) bool {
	now := s.opt.Now().UnixNano()
	b := s.buckets[r.Client]
	if b == nil {
		b = &bucket{tokens: float64(s.opt.RateBurst), last: now}
		s.buckets[r.Client] = b
	}
	b.tokens = min(float64(s.opt.RateBurst), b.tokens+float64(now-b.last)/1e9*float64(s.opt.RateRecords))
	b.last = now
	if b.band.Total > 0 && now-b.opened >= int64(sampleWindow) {
		s.closeBandLocked(b)
	}
	keep := b.tokens >= 1
	if keep {
		b.tokens--
	}
	switch {
	case !keep && b.band.Total == 0:
		b.opened = now
		b.band = Gap{From: r.At, To: r.At, Client: r.Client, Cause: CauseSampled}
	case b.band.Total == 0:
		return true
	}
	b.band.From, b.band.To = min(b.band.From, r.At), max(b.band.To, r.At)
	b.band.Total++
	if keep {
		b.band.Retained++
	}
	return keep
}

func (s *Store) closeBandLocked(b *bucket) {
	s.noteGapLocked(b.band)
	b.band = Gap{}
}

// settleLocked writes every band that has run its second and forgets the
// buckets that are full again, on the cold path: a flush, or Close.
func (s *Store) settleLocked(all bool) {
	now := s.opt.Now().UnixNano()
	for client, b := range s.buckets {
		if b.band.Total > 0 && (all || now-b.opened >= int64(sampleWindow)) {
			s.closeBandLocked(b)
		}
		full := b.tokens+float64(now-b.last)/1e9*float64(s.opt.RateRecords) >= float64(s.opt.RateBurst)
		if b.band.Total == 0 && full {
			delete(s.buckets, client)
		}
	}
}

// openBandsLocked are the sampled bands not yet written, so a read during
// a flood still states what it does not hold.
func (s *Store) openBandsLocked() []Gap {
	var out []Gap
	for _, b := range s.buckets {
		if b.band.Total > 0 {
			out = append(out, b.band)
		}
	}
	return out
}

// NoteGap records a loss the store did not see happen, such as records a
// program's own buffer dropped before they were sent.
func (s *Store) NoteGap(g Gap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.noteGapLocked(g)
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
	s.noteGapLocked(Gap{From: from, To: to, Client: "*", Cause: cause, Total: n})
}

func (s *Store) noteGapLocked(g Gap) {
	if s.dir != "" {
		s.appendCoverageLocked(g)
		return
	}
	if k := len(s.gaps) - 1; k >= 0 && s.gaps[k].Cause == g.Cause && s.gaps[k].Client == g.Client {
		s.gaps[k].From = min(s.gaps[k].From, g.From)
		s.gaps[k].To = max(s.gaps[k].To, g.To)
		s.gaps[k].Retained += g.Retained
		s.gaps[k].Total += g.Total
		return
	}
	s.gaps = append(s.gaps, g)
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
	s.settleLocked(false)
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
		s.seg, s.segID, s.segSize = f, batch[0].Seq, 0
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
		s.kickRetain() // compress the segment just closed, and apply the ceilings
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
	s.settleLocked(true)
	s.closed = true
	if s.retainTick != nil {
		s.retainTick.Stop()
	}
	if s.wake != nil {
		close(s.wake)
		s.wake = nil
	}
	s.mu.Unlock()

	s.flush()
	s.rmu.Lock() // a retention pass running finishes first
	defer s.rmu.Unlock()
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
