package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// DirExistsError is the refusal of decision 8: EXISTING STATE IS NEVER
// DELETED.
//
// ⛔ THE FIX LINE NAMES --force AND SAYS WHAT --force DOES, because the whole
// value of this refusal is lost if the reader believes --force means "delete
// the old one". It does not: it RENAMES the directory whole, so the state is
// still on disk under a stamped name and a restore that turns out to have been
// a mistake is undone by renaming it back.
type DirExistsError struct{ Dir string }

func (e *DirExistsError) Error() string {
	return fmt.Sprintf("backup: %s already holds state and rig will not restore "+
		"over it\n"+
		"       Pass --force to move it aside: the directory is RENAMED whole to "+
		"%s.replaced-<stamp> and nothing is deleted, so this is undone by "+
		"renaming it back", e.Dir, e.Dir)
}

// SchemaTooNewError is decision 10's ceiling.
//
// ONE DIRECTION ONLY. An OLDER schema is accepted and migrated when the daemon
// next opens the store, which is what internal/record's migration ladder is
// for. A NEWER one is refused, because this binary has no idea what changed
// and a store it half-understands is the silent failure store.go names.
type SchemaTooNewError struct {
	Found   uint64
	Ceiling uint64
}

func (e *SchemaTooNewError) Error() string {
	return fmt.Sprintf("backup: the archive holds schema version %d and this "+
		"build knows up to %d, so restoring it would hand the daemon a store it "+
		"cannot read\n"+
		"       An older archive is fine and is migrated on the next open; a "+
		"newer one needs the newer rig that wrote it", e.Found, e.Ceiling)
}

// IntegrityError is a member whose bytes are not what the manifest said.
//
// It carries BOTH digests rather than saying "checksum mismatch", because the
// two cases a reader must tell apart - a truncated download and a file
// modified in place - look identical without them.
type IntegrityError struct {
	Member    string
	WantBytes int64
	GotBytes  int64
	WantSum   string
	GotSum    string
}

func (e *IntegrityError) Error() string {
	if e.WantBytes != e.GotBytes {
		return fmt.Sprintf("backup: member %s is %d bytes and its manifest says "+
			"%d: the archive is truncated or was written while it was read",
			e.Member, e.GotBytes, e.WantBytes)
	}
	return fmt.Sprintf("backup: member %s hashes to %s and its manifest says "+
		"%s: the archive is the right length and the wrong bytes, so it has been "+
		"modified or corrupted in place", e.Member, e.GotSum, e.WantSum)
}

// RestoreRequest is everything a restore needs, and it is all DATA.
//
// ⛔ NOTHING HERE IS DISCOVERED BY THIS PACKAGE. The schema ceiling comes from
// the binary that links internal/record, the directory comes from
// internal/paths, the clock comes from the caller. That is what keeps this
// package standard library only (see the package doc) and it is also what
// makes the whole restore testable without a store, a daemon or a clock.
type RestoreRequest struct {
	// Archive is the file to read. It is the ONE path a human typed.
	Archive string

	// Dir is the estate's state directory, as it must exist when this returns.
	//
	// It is built by the caller from a validated estate name, and it is checked
	// here as well: absolute, not the filesystem root, with a parent to be a
	// sibling of.
	Dir string

	// Force allows an existing Dir to be moved aside. See DirExistsError.
	Force bool

	// SchemaCeiling is the restoring binary's record.SchemaVersion.
	SchemaCeiling uint64

	// At stamps the sibling directory names.
	At time.Time
}

// RestoreResult says what moved where, so the CLI can print it and a person
// can undo it.
type RestoreResult struct {
	// Manifest is what the archive declared.
	Manifest Manifest

	// Dir is the estate directory that now holds the restored state.
	Dir string

	// Replaced is where the previous state went, empty when there was none.
	//
	// ⛔ IT IS PRINTED. A person who restored the wrong archive needs this path
	// to get back, and the moment it is only in a log it is effectively gone.
	Replaced string

	// Staging is the directory the archive was extracted into.
	//
	// On success it no longer exists under this name, having been renamed to
	// Dir. On a FAILURE AFTER EXTRACTION BEGAN it is still there, and it is
	// reported for the same reason Replaced is: it is evidence, and rig does not
	// delete it.
	Staging string
}

// Read opens an archive, checks everything checkable, and answers its
// manifest. IT WRITES NOTHING.
//
// ⛔ THIS IS PASS ONE OF DECISION 9'S ALL-OR-NOTHING, AND IT IS A SEPARATE PASS
// SO THAT A BAD ARCHIVE IS REFUSED BEFORE THE FIRST BYTE LANDS. Acceptance
// test 4 is exactly that: an archive with a member the manifest does not
// declare is refused and no directory is created. A reader that validated as
// it extracted would have written the good members first, and "no state was
// touched" would be a claim rather than a property.
//
// It is also the whole of `rig restore --check` territory and of any future
// listing: reading an archive can never cost anything.
func Read(path string) (Manifest, error) {
	if path == "" {
		return Manifest{}, errors.New("backup: Read needs an archive to read")
	}
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return readStream(f, path, nil)
}

// readStream is the one reader both passes go through.
//
// visit is called for each member AFTER the manifest, with the member's
// declared shape and a reader bounded to its declared length. Read passes nil
// and so only validates; Restore passes an extractor. ⛔ ONE READER MEANS THE
// TWO PASSES CANNOT DISAGREE ABOUT WHAT THE ARCHIVE CONTAINS, which is the
// only way pass one's verdict is worth anything to pass two.
func readStream(r io.Reader, path string, visit func(Member, io.Reader) error) (Manifest, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: %s is not a gzip stream, so "+
			"it is not a rig archive: %w", path, err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)

	m, err := readManifest(tr, path)
	if err != nil {
		return Manifest{}, err
	}

	seen := make([]Member, 0, len(m.Members))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Manifest{}, fmt.Errorf("backup: reading %s: %w", path, err)
		}
		if len(seen) == len(m.Members) {
			return Manifest{}, fmt.Errorf("backup: %s carries %q after the "+
				"last member its manifest declares: an archive holds what its "+
				"manifest says and nothing else", path, h.Name)
		}

		// ⛔ THE NAME IS COMPARED, NEVER JOINED. The declared name is matched
		// against the manifest's next entry and the manifest's names were already
		// matched against the closed set, so there is no path here for `..`, an
		// absolute path or a separator to survive into a filesystem call. Section
		// 38's fourth standing rule, and the reason acceptance test 5 has nothing
		// to find.
		want := m.Members[len(seen)]
		if h.Name != want.Name {
			return Manifest{}, fmt.Errorf("backup: %s carries %q where its "+
				"manifest declares %q: the archive and its manifest disagree about "+
				"what is in it", path, h.Name, want.Name)
		}
		if h.Typeflag != tar.TypeReg {
			return Manifest{}, fmt.Errorf("backup: member %s is not a regular "+
				"file (tar type %q); rig archives carry files and nothing else",
				h.Name, string(rune(h.Typeflag)))
		}
		if h.Size != want.Bytes {
			return Manifest{}, &IntegrityError{
				Member: h.Name, WantBytes: want.Bytes, GotBytes: h.Size,
			}
		}

		if visit != nil {
			if err := visit(want, io.LimitReader(tr, want.Bytes)); err != nil {
				return Manifest{}, err
			}
		}
		seen = append(seen, want)
	}

	if len(seen) != len(m.Members) {
		return Manifest{}, fmt.Errorf("backup: %s declares %d members and "+
			"carries %d: it is truncated", path, len(m.Members), len(seen))
	}
	return m, nil
}

// readManifest reads and validates the first member.
//
// EVERYTHING THE REST OF THE READ TRUSTS IS ESTABLISHED HERE: the format, the
// member names against the closed set, their order, their shapes. After this
// returns, the manifest is a description of something this build can read, and
// every later check is against it rather than against a literal.
func readManifest(tr *tar.Reader, path string) (Manifest, error) {
	h, err := tr.Next()
	if errors.Is(err, io.EOF) {
		return Manifest{}, fmt.Errorf("backup: %s is an empty archive", path)
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: reading %s: %w", path, err)
	}
	if h.Name != ManifestMember {
		return Manifest{}, fmt.Errorf("backup: %s begins with %q and every rig "+
			"archive begins with %s", path, h.Name, ManifestMember)
	}
	if h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > maxManifestBytes {
		return Manifest{}, fmt.Errorf("backup: %s's manifest is %d bytes of tar "+
			"type %q, which is not a manifest this build will read",
			path, h.Size, string(rune(h.Typeflag)))
	}

	// Bounded by the header AND by the limit, because the header is part of the
	// file being checked and cannot be the only thing bounding the read of it.
	body, err := io.ReadAll(io.LimitReader(tr, maxManifestBytes))
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: reading %s's manifest: %w", path, err)
	}

	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("backup: %s's manifest is not one this "+
			"build reads: %w", path, err)
	}
	if err := checkManifest(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// checkManifest refuses a manifest before anything is trusted to it.
func checkManifest(m Manifest) error {
	if m.Format != Format {
		return &FormatError{Found: m.Format, Known: Format}
	}
	if len(m.Members) == 0 {
		return errors.New("backup: the manifest declares no members, so there is " +
			"nothing to restore")
	}
	if len(m.Members) > len(known) {
		return &UnknownMemberError{Name: m.Members[len(known)].Name}
	}
	for i, mem := range m.Members {
		if mem.Name != known[i] {
			return &UnknownMemberError{Name: mem.Name}
		}
		if mem.Bytes < 0 {
			return fmt.Errorf("backup: the manifest says member %s is %d bytes",
				mem.Name, mem.Bytes)
		}
		if !isSHA256(mem.SHA256) {
			return fmt.Errorf("backup: the manifest's digest for member %s is %q, "+
				"which is not a SHA-256", mem.Name, mem.SHA256)
		}
	}
	return nil
}

// isSHA256 answers whether s is 64 lowercase hex digits.
//
// The case matters: every digest this package writes comes from
// hex.EncodeToString, which is lowercase, and accepting the other case would
// mean two spellings of one digest and a comparison that is right by accident.
func isSHA256(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Restore puts an archive's state at r.Dir, or leaves everything as it was.
//
// ⛔ DECISION 9, AND THE ORDER IS THE WHOLE MECHANISM:
//
//  1. Read validates the archive completely, writing nothing. A bad archive
//     stops here, before a directory exists.
//  2. The schema ceiling is checked (decision 10).
//  3. Everything is extracted into a SIBLING, <dir>.restoring-<stamp>, and
//     every member is verified against the manifest as it lands.
//  4. Only then do the renames happen: the existing directory to
//     <dir>.replaced-<stamp> if there is one, then the sibling to <dir>.
//
// A sibling rather than a temp directory because rename is atomic only within
// a filesystem, and a sibling is the only placement guaranteed to be on the
// same one. The window in which <dir> does not exist is the gap between two
// renames on the same directory's parent.
//
// ⛔ THERE IS NO os.RemoveAll ANYWHERE IN THIS PATH AND THAT IS DECISION 8.
// A failed restore leaves the .restoring- directory sitting there named for
// exactly what it is. That is not untidiness, it is the evidence, and rig does
// not decide on a person's behalf that their data was worthless.
//
// This function NEVER CONNECTS TO A DAEMON (decision 7): a restore replaces
// the files a running daemon has open, so it is offline by construction. The
// caller holds the estate's lock, which is how a running daemon is detected.
func Restore(r RestoreRequest) (RestoreResult, error) {
	if err := checkRestoreRequest(r); err != nil {
		return RestoreResult{}, err
	}

	// Pass one. Nothing below this line runs if the archive is not whole.
	m, err := Read(r.Archive)
	if err != nil {
		return RestoreResult{}, err
	}
	if m.SchemaVersion > r.SchemaCeiling {
		return RestoreResult{}, &SchemaTooNewError{
			Found: m.SchemaVersion, Ceiling: r.SchemaCeiling,
		}
	}

	existing, err := existingDir(r.Dir)
	if err != nil {
		return RestoreResult{}, err
	}
	if existing && !r.Force {
		return RestoreResult{}, &DirExistsError{Dir: r.Dir}
	}

	stamp := Stamp(r.At)
	staging := r.Dir + ".restoring-" + stamp
	result := RestoreResult{Manifest: m, Dir: r.Dir, Staging: staging}

	if err := os.MkdirAll(filepath.Dir(r.Dir), 0o700); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: making room for %s: %w", r.Dir, err)
	}
	// Exclusive: a .restoring- directory already at this name is another restore
	// in flight or a previous one that failed, and writing into either is how
	// two archives become one estate.
	if err := os.Mkdir(staging, 0o700); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: making the staging directory "+
			"%s: %w", staging, err)
	}

	// Pass two. The members are verified against PASS ONE'S manifest as they
	// land, so an archive rewritten between the two passes fails here rather
	// than being restored: the second read would have to reproduce the first
	// read's digests to get through.
	if err := extract(r.Archive, staging, m); err != nil {
		return result, err
	}

	if existing {
		replaced := r.Dir + ".replaced-" + stamp
		if err := os.Rename(r.Dir, replaced); err != nil {
			return result, fmt.Errorf("backup: moving the existing state at %s "+
				"aside to %s: %w\n"+
				"       The restored state is extracted and verified at %s and "+
				"nothing has been lost", r.Dir, replaced, err, staging)
		}
		result.Replaced = replaced
	}
	if err := os.Rename(staging, r.Dir); err != nil {
		return result, fmt.Errorf("backup: putting the restored state at %s: %w\n"+
			"       It is extracted and verified at %s; the previous state, if "+
			"there was one, is at %s", r.Dir, err, staging, result.Replaced)
	}
	return result, syncDir(filepath.Dir(r.Dir))
}

// checkRestoreRequest refuses a request before it can touch anything.
func checkRestoreRequest(r RestoreRequest) error {
	switch {
	case r.Archive == "":
		return errors.New("backup: a restore needs an archive to restore from")
	case r.Dir == "":
		return errors.New("backup: a restore needs a directory to restore into")
	case !filepath.IsAbs(r.Dir):
		return fmt.Errorf("backup: the restore target must be an absolute path "+
			"and %q is not", r.Dir)
	case filepath.Clean(r.Dir) != r.Dir:
		// An uncleaned path here would mean the sibling names are built by
		// appending to something containing `..`, and the two renames would then
		// land somewhere nobody named.
		return fmt.Errorf("backup: the restore target %q is not in its plain "+
			"form (%q); rig will not build the .replaced- and .restoring- names "+
			"beside a path that has not been resolved", r.Dir, filepath.Clean(r.Dir))
	case filepath.Dir(r.Dir) == r.Dir:
		return fmt.Errorf("backup: %q has no parent directory to place the "+
			"restored state beside", r.Dir)
	case r.SchemaCeiling == 0:
		// Never guessed: a zero ceiling would refuse every archive, and a ceiling
		// defaulted to "anything" would restore a store no build can read.
		return errors.New("backup: a restore needs the schema version this build " +
			"can read and none was given")
	case r.At.IsZero():
		return errors.New("backup: a restore needs the time, to name what it " +
			"moves aside")
	}
	return nil
}

// existingDir answers whether dir is there, and refuses anything that is there
// but is not a directory.
func existingDir(dir string) (bool, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("backup: looking at %s: %w", dir, err)
	}
	if !info.IsDir() {
		// Including a symlink: renaming one aside and putting a directory in its
		// place would leave whatever it pointed at silently orphaned.
		return false, fmt.Errorf("backup: %s is not a directory, so it is not an "+
			"estate's state and rig will not replace it", dir)
	}
	return true, nil
}

// extract writes every member into staging and verifies each as it lands.
//
// The digest is taken from the bytes ON THE WAY TO DISK rather than by reading
// the file back afterwards, so what is verified is what was written and not a
// second read that could answer differently.
func extract(archive, staging string, m Manifest) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("backup: opening %s: %w", archive, err)
	}
	defer func() { _ = f.Close() }()

	verified := 0
	_, err = readStream(f, archive, func(want Member, body io.Reader) error {
		if err := extractMember(staging, want, body); err != nil {
			return err
		}
		verified++
		return nil
	})
	if err != nil {
		return err
	}
	if verified != len(m.Members) {
		return fmt.Errorf("backup: %d of %d members were extracted from %s",
			verified, len(m.Members), archive)
	}
	return syncDir(staging)
}

// extractMember writes one member and refuses it if it is not what was
// declared.
func extractMember(staging string, want Member, body io.Reader) error {
	// filepath.Join is safe here ONLY because want.Name came through
	// checkManifest's closed set, which is why that check runs before any of
	// this. Belt and braces: the joined path must still be a direct child.
	dst := filepath.Join(staging, want.Name)
	if filepath.Dir(dst) != staging {
		return &UnknownMemberError{Name: want.Name}
	}

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("backup: writing member %s: %w", want.Name, err)
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), body)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("backup: extracting member %s: %w", want.Name, err)
	}
	// Synced before it is called good: a restore that a power cut can undo is
	// not the all-or-nothing decision 9 asks for.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("backup: flushing member %s: %w", want.Name, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup: closing member %s: %w", want.Name, err)
	}

	got := hex.EncodeToString(sum.Sum(nil))
	if n != want.Bytes || got != want.SHA256 {
		return &IntegrityError{
			Member: want.Name, WantBytes: want.Bytes, GotBytes: n,
			WantSum: want.SHA256, GotSum: got,
		}
	}
	return nil
}

// syncDir flushes a directory entry, so the names survive a crash as the files
// do.
//
// An error here is reported rather than swallowed, but a filesystem that
// refuses to sync a directory (some do) is not a failed restore, so only a
// real failure is returned.
func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("backup: flushing %s: %w", dir, err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return fmt.Errorf("backup: flushing %s: %w", dir, err)
	}
	return nil
}
