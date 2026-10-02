package observe

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Retention (plan/49 decision 11): §7's ladder over §15's three ceilings,
// rate first (admitLocked), then size, then age. A closed segment is
// compressed once, at maximal force, when it rotates; at ArchiveAfter it
// moves to archive/; at DeleteAfter it is unlinked unless pinned. Every
// unlink is a band in the coverage log.

// Decision 12's retention defaults, the logs.retention.bytes,
// logs.archive.days and logs.delete.days settings.
const (
	DefaultRetainBytes  = 500 << 20
	DefaultArchiveAfter = 30 * 24 * time.Hour
	DefaultDeleteAfter  = 90 * 24 * time.Hour

	// retainEvery is how often age is checked between rotations: hourly,
	// so the idle daemon's wakeups stay inside §17's budget.
	retainEvery = time.Hour

	CauseUnlinked = "segment unlinked"
)

const zstSuffix = ".zst"

// Segment is one segment file, as `rig logs coverage` lists it.
type Segment struct {
	ID         uint64 `json:"id"` // its first seq, and the name `rig logs pin` takes
	Path       string `json:"-"`
	Bytes      int64  `json:"bytes"`
	Modified   int64  `json:"modified"` // unix nanos of its last write
	Compressed bool   `json:"compressed"`
	Archived   bool   `json:"archived"`
	Pinned     bool   `json:"pinned"`
	Open       bool   `json:"open"` // the segment being written
}

// listSegments is every segment under the logs directory, live and
// archived, oldest first. A segment caught mid-compression is listed once,
// as its finished form.
func listSegments(dir string) ([]Segment, error) {
	byID := map[uint64]Segment{}
	for _, sub := range []string{"segments", "archive"} {
		ents, err := os.ReadDir(filepath.Join(dir, sub))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			name := e.Name()
			compressed := strings.HasSuffix(name, ".jsonl"+zstSuffix)
			if !e.Type().IsRegular() || !compressed && !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			id, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSuffix(name, zstSuffix), ".jsonl"), 10, 64)
			if err != nil {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue // unlinked as it was listed
			}
			if prev, dup := byID[id]; dup && prev.Compressed {
				continue
			}
			byID[id] = Segment{
				ID: id, Path: filepath.Join(dir, sub, name), Bytes: info.Size(),
				Modified: info.ModTime().UnixNano(), Compressed: compressed, Archived: sub == "archive",
			}
		}
	}
	out := slices.Collect(func(yield func(Segment) bool) {
		for _, s := range byID {
			if !yield(s) {
				return
			}
		}
	})
	slices.SortFunc(out, func(a, b Segment) int { return cmpInt(int64(a.ID), int64(b.ID)) }) //nolint:gosec // seqs stay far below 2^63
	return out, nil
}

// openSegment reads a segment whichever form it is in.
func openSegment(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil || !strings.HasSuffix(path, zstSuffix) {
		return f, err
	}
	d, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &zstReader{d: d, f: f}, nil
}

type zstReader struct {
	d *zstd.Decoder
	f *os.File
}

func (z *zstReader) Read(p []byte) (int, error) { return z.d.Read(p) }

func (z *zstReader) Close() error {
	z.d.Close()
	return z.f.Close()
}

// compress replaces a closed segment with its zstd form, once, at maximal
// force. The finished file takes the segment's modification time, since
// age is counted from the last record written, not from the compression.
func compress(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := path + zstSuffix + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w, err := zstd.NewWriter(out, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithEncoderConcurrency(1))
	if err == nil {
		_, err = io.Copy(w, bufio.NewReader(in))
		err = errors.Join(err, w.Close())
	}
	err = errors.Join(err, out.Close())
	if err == nil {
		err = os.Chtimes(tmp, info.ModTime(), info.ModTime())
	}
	if err == nil {
		err = os.Rename(tmp, path+zstSuffix)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Remove(path)
}

// firstAt is the time of a segment's first whole record, 0 if it has none.
func firstAt(path string) int64 {
	var at int64
	_ = scanSegment(path, func(r *Record) bool {
		at = r.At
		return false
	})
	return at
}

// pins is the set of pinned segment ids, one per line in logs/pins.
func readPins(dir string) (map[uint64]bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, "pins"))
	if errors.Is(err, fs.ErrNotExist) {
		return map[uint64]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[uint64]bool{}
	for line := range strings.Lines(string(b)) {
		if id, err := strconv.ParseUint(strings.TrimSpace(line), 10, 64); err == nil {
			out[id] = true
		}
	}
	return out, nil
}

func writePins(dir string, pins map[uint64]bool) error {
	var b strings.Builder
	for _, id := range slices.Sorted(func(yield func(uint64) bool) {
		for id := range pins {
			if !yield(id) {
				return
			}
		}
	}) {
		fmt.Fprintf(&b, "%d\n", id)
	}
	tmp := filepath.Join(dir, "pins.tmp")
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "pins"))
}

// ErrNoSegment is a pin naming no segment the store holds.
var ErrNoSegment = errors.New("observe: no such segment")

// Pin keeps a segment past the age and size ceilings (§7's minimal pin);
// pin false releases it. It answers whether anything changed.
func (s *Store) Pin(id uint64, pin bool) (bool, error) {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	dir := s.Dir()
	if dir == "" {
		return false, errors.New("observe: the store has no directory")
	}
	segs, err := listSegments(dir)
	if err != nil {
		return false, err
	}
	if !slices.ContainsFunc(segs, func(g Segment) bool { return g.ID == id }) && pin {
		return false, ErrNoSegment
	}
	pins, err := readPins(dir)
	if err != nil {
		return false, err
	}
	if pins[id] == pin {
		return false, nil
	}
	if pin {
		pins[id] = true
	} else {
		delete(pins, id)
	}
	return true, writePins(dir, pins)
}

// openSegmentID is the id of the segment being written, 0 between segments.
func (s *Store) openSegmentID() uint64 {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.seg == nil {
		return 0
	}
	return s.segID
}

// Segments lists the store's segments, live and archived, oldest first.
// It flushes first, so what the ring still holds is counted where it will
// be written rather than missing from the answer.
func (s *Store) Segments() ([]Segment, error) {
	dir := s.Dir()
	if dir == "" {
		return nil, nil
	}
	s.flush()
	segs, err := listSegments(dir)
	if err != nil {
		return nil, err
	}
	pins, err := readPins(dir)
	if err != nil {
		return nil, err
	}
	open := s.openSegmentID()
	for i := range segs {
		segs[i].Pinned = pins[segs[i].ID]
		segs[i].Open = segs[i].ID == open && !segs[i].Compressed
	}
	return segs, nil
}

// Retain applies the size and age ceilings now: compresses any closed
// segment still raw, archives, unlinks, and drops blob files no remaining
// segment can refer to.
//
// Under its own lock, rmu, and never wmu: compressing 16 MiB at maximal
// force takes seconds, and a writer waiting that long would make every
// caller in backpressure wait too. The writer only ever touches its open
// segment, and that is the one segment this never touches.
func (s *Store) Retain() error {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	return s.retainLocked()
}

// kickRetain runs retention off the caller's path; a kick while one runs
// makes it run once more, so a rotation is never missed.
func (s *Store) kickRetain() {
	if !s.rmu.TryLock() {
		s.retainAgain.Store(true)
		return
	}
	go func() {
		defer s.rmu.Unlock()
		for {
			s.retainAgain.Store(false)
			err := s.retainLocked()
			s.mu.Lock()
			s.retainErr = err
			s.mu.Unlock()
			if !s.retainAgain.Load() {
				return
			}
		}
	}()
}

// RetainErr is the latest retention failure, nil once a pass succeeds.
func (s *Store) RetainErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retainErr
}

func (s *Store) retainLocked() error {
	dir := s.Dir()
	if dir == "" {
		return nil
	}
	segs, err := listSegments(dir)
	if err != nil {
		return err
	}
	// After the listing, so a segment opened since is not in it, and one
	// open when it was listed is still skipped.
	openID := s.openSegmentID()
	pins, err := readPins(dir)
	if err != nil {
		return err
	}
	now := s.opt.Now()
	var errs []error
	live := segs[:0:0]
	for _, g := range segs {
		if g.ID == openID && !g.Compressed {
			live = append(live, g) // the open one: never compressed, moved or unlinked
			continue
		}
		if !g.Compressed {
			if err := compress(g.Path); err != nil {
				errs = append(errs, err)
			} else {
				g.Path += zstSuffix
				g.Compressed = true
				if info, err := os.Stat(g.Path); err == nil {
					g.Bytes = info.Size()
				}
			}
		}
		age := now.Sub(time.Unix(0, g.Modified))
		if !g.Archived && age >= s.archiveAfter() {
			to := filepath.Join(dir, "archive", filepath.Base(g.Path))
			if err := os.MkdirAll(filepath.Dir(to), 0o700); err == nil && os.Rename(g.Path, to) == nil {
				g.Path, g.Archived = to, true
			}
		}
		live = append(live, g)
	}

	// Age, then size, oldest first; a pinned segment is never unlinked.
	var total int64
	for _, g := range live {
		total += g.Bytes
	}
	total += blobBytesOn(dir)
	for i := 0; i < len(live); i++ {
		g := live[i]
		open := g.ID == openID && !g.Compressed
		aged := now.Sub(time.Unix(0, g.Modified)) >= s.deleteAfter()
		if open || pins[g.ID] || !aged && total <= s.retainBytes() {
			continue
		}
		var next uint64
		if i+1 < len(live) {
			next = live[i+1].ID
		} else {
			s.mu.Lock()
			next = s.onDisk + 1
			s.mu.Unlock()
		}
		from := firstAt(g.Path)
		if err := os.Remove(g.Path); err != nil {
			errs = append(errs, err)
			continue
		}
		total -= g.Bytes
		s.mu.Lock()
		s.appendCoverageLocked(Gap{From: from, To: g.Modified, Client: "*", Cause: CauseUnlinked, Total: next - g.ID})
		s.mu.Unlock()
		live = slices.Delete(live, i, i+1)
		i--
	}
	errs = append(errs, s.dropBlobs(dir, live))
	return errors.Join(errs...)
}

func (s *Store) retainBytes() int64 {
	if s.opt.RetainBytes > 0 {
		return s.opt.RetainBytes
	}
	return DefaultRetainBytes
}

func (s *Store) archiveAfter() time.Duration {
	if s.opt.ArchiveAfter > 0 {
		return s.opt.ArchiveAfter
	}
	return DefaultArchiveAfter
}

func (s *Store) deleteAfter() time.Duration {
	if s.opt.DeleteAfter > 0 {
		return s.opt.DeleteAfter
	}
	return DefaultDeleteAfter
}

func blobFiles(dir string) []string {
	ents, err := os.ReadDir(filepath.Join(dir, "blobs"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".blob") {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out
}

func blobBytesOn(dir string) int64 {
	var n int64
	for _, name := range blobFiles(dir) {
		if info, err := os.Stat(filepath.Join(dir, "blobs", name)); err == nil {
			n += info.Size()
		}
	}
	return n
}

// dropBlobs unlinks the blob files every remaining segment is newer than. A
// blob file holds the payloads of the records written from its creation to
// the next file's, so it is dead once the next file predates the oldest
// record still kept.
func (s *Store) dropBlobs(dir string, live []Segment) error {
	files := blobFiles(dir)
	oldest := int64(1<<63 - 1)
	if len(live) > 0 {
		if at := firstAt(live[0].Path); at != 0 {
			oldest = at
		}
	}
	s.blobs.mu.Lock()
	current := s.blobs.name
	s.blobs.mu.Unlock()
	var errs []error
	for i := 0; i+1 < len(files); i++ {
		next, err := strconv.ParseInt(strings.TrimSuffix(files[i+1], ".blob"), 10, 64)
		if err != nil || files[i] == current || next > oldest {
			continue
		}
		if err := os.Remove(filepath.Join(dir, "blobs", files[i])); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
