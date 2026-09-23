// Package backup is rig's own archive of an estate's persistent state:
// PLAN.md section 46.
//
// ONE ARCHIVE, TWO HALVES, AND THEY RUN IN DIFFERENT PROCESSES. `rig backup`
// asks the daemon, because section 22 puts modernc.org/sqlite in rigd alone
// and cmd/rig/layering_test.go is the gate that keeps it there; the daemon
// takes the snapshot and writes the file. `rig restore` is OFFLINE and runs in
// the CLI, because a restore is file placement and verification and the CLI
// links no store at all.
//
// ⛔ SO THIS PACKAGE IS STANDARD LIBRARY ONLY, AND THAT IS A GATE RATHER THAN
// A PREFERENCE. cmd/rig imports it. An import of internal/record here would
// drag the SQLite driver into the client binary and turn
// TestTheClientLinksNeitherTheDaemonNorItsValidator red, which is the point of
// that test. Anything this package needs to know about the record store - the
// schema ceiling a restore refuses above, the member's name - arrives as data
// from the caller that does know.
//
// archive/tar inside compress/gzip, both standard library, decided in section
// 46 row 3: the whole estate is under 7 MB, so klauspost/compress/zstd would
// need a section 22 row and a footprint measurement for a benefit these sizes
// do not produce, and mholt/archiver pulls every format in to get one.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Format is the archive format this build writes and reads.
//
// IT IS BUMPED ONLY WHEN A READER OF THE OLD FORMAT COULD NOT READ THE NEW
// ONE. Adding a member is not that: decision 4 makes an unknown member name a
// REFUSAL, so an older binary meeting a newer archive stops rather than
// restoring half of it, and that is the protection the number would otherwise
// be spent on.
const Format = 1

// The member names, and the set is CLOSED.
//
// ⛔ A MEMBER LIST THAT CAN GROW SILENTLY IS WHAT DECISION 4 REFUSES. Section
// 07's table marks four kinds of state worth keeping and only the record store
// exists in code today; the config tree is three flags, the audit log and the
// notification centre are unbuilt. Each joins this set on the day it exists,
// which is additive, and until then a name outside the set is refused before
// any byte is extracted - so an older binary refuses a newer archive instead
// of writing part of it.
const (
	// ManifestMember is the first member of every archive.
	ManifestMember = "manifest.json"

	// RecordMember is the record store's snapshot.
	//
	// ⛔ IT IS A SEPARATE CONSTANT FROM record.DBName AND THE TWO ARE PINNED BY
	// A TEST, not by this comment. This package may not import internal/record
	// (see the package doc), so the name is carried here and
	// internal/daemon/backup_test.go - which imports both - asserts they are
	// equal. That is the same shape as cmd/rig reading daemon.CallTimeout in a
	// test so two numbers cannot drift in silence.
	RecordMember = "record.db"
)

// known is every member name a reader accepts, after the manifest, IN ARCHIVE
// ORDER.
var known = []string{RecordMember}

// maxManifestBytes bounds the first member.
//
// A bound rather than a trusted length: the manifest is read before anything
// about the archive has been checked, so it is the one member whose size is
// not yet known from a trustworthy source. 1 MiB is four orders of magnitude
// over what a manifest of two members needs.
const maxManifestBytes = 1 << 20

// Manifest describes an archive, and EVERY VALUE IN IT IS SET BY THE WRITER.
//
// Nothing here is read off a request: decision 6 gives the request no fields
// at all, because anything a client could write into one would be a second
// place for the client and the daemon to disagree.
//
// ⛔ THE FIELDS ARE DECLARED IN ALPHABETICAL ORDER OF THEIR JSON TAGS, WHICH IS
// HOW "keys sorted" IS MET. encoding/json emits struct fields in declaration
// order, so the order is chosen here rather than by a map - and a map would
// lose every type in the process. TestTheManifestsKeysAreSorted is what keeps
// it true when a field is added.
type Manifest struct {
	// CreatedUnixNano is when the snapshot was taken, on the daemon's clock.
	CreatedUnixNano int64 `json:"created_unix_nano"`

	// Estate is the estate the daemon was running.
	Estate string `json:"estate"`

	// Format is the archive format. See Format.
	Format int `json:"format"`

	// Heads is the head count in the snapshot.
	//
	// ⛔ IT IS THE NUMBER THE ACCEPTANCE TEST COMPARES, because record.query
	// answers heads while Records counts every superseded version too.
	Heads uint64 `json:"heads"`

	// Members are the members after the manifest, in archive order.
	Members []Member `json:"members"`

	// Records is the row count in the snapshot.
	Records uint64 `json:"records"`

	// RigVersion is what the writing daemon prints for --version.
	RigVersion string `json:"rig_version"`

	// SchemaVersion is the snapshot's own PRAGMA user_version, read from the
	// file rather than from the writing binary's constant.
	SchemaVersion uint64 `json:"schema_version"`

	// Wire is the wire major the writing daemon served.
	Wire string `json:"wire"`
}

// Member is one file in the archive.
type Member struct {
	Bytes  int64  `json:"bytes"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Source is a file on disk to carry as a named member.
type Source struct {
	Name string
	Path string
}

// Archive is the file Write produced.
type Archive struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// UnknownMemberError is a name outside the closed set.
//
// It is its own type because it is the refusal decision 4 exists to produce:
// an OLDER binary meeting a NEWER archive. The message says which side is old,
// so a reader is not sent looking for a corrupt file.
type UnknownMemberError struct{ Name string }

func (e *UnknownMemberError) Error() string {
	return fmt.Sprintf("backup: the archive carries a member named %q and this "+
		"build knows only %s\n"+
		"       rig refuses an archive it cannot read WHOLE rather than "+
		"restoring the part it understands. This is very likely a newer rig's "+
		"archive: run that build's rig restore",
		e.Name, strings.Join(append([]string{ManifestMember}, known...), ", "))
}

// FormatError is an archive format this build does not read.
type FormatError struct{ Found, Known int }

func (e *FormatError) Error() string {
	return fmt.Sprintf("backup: the archive is format %d and this build reads "+
		"format %d: the format is bumped only when an old reader could not "+
		"read the new layout, so this one cannot be read here",
		e.Found, e.Known)
}

// Stamp is the UTC stamp rig puts in every name it chooses.
//
// Seconds, no separators inside the time, and a trailing Z so the string says
// it is UTC. It is sortable as text, which is the only property a directory
// listing of archives needs.
func Stamp(at time.Time) string { return at.UTC().Format("20060102T150405Z") }

// ArchiveName is the file name for an estate's archive, and the caller never
// supplies one.
//
// DECISION 6: the daemon chooses the name and the CLI prints it. An empty
// request has nothing to validate, which is the cheapest way to satisfy
// section 38's rule that no caller path is joined unvalidated.
//
// The estate name still becomes a path component here, so it is checked. rigd
// refuses a name outside a closed set long before this runs; that is the
// caller's guarantee and not this function's, and a function that joins a
// value into a filename checks it itself or the guarantee lives nowhere.
func ArchiveName(estate string, at time.Time) (string, error) {
	if err := safeComponent(estate); err != nil {
		return "", err
	}
	return estate + "-" + Stamp(at) + ".tar.gz", nil
}

// safeComponent refuses anything that would not be one safe path component.
func safeComponent(s string) error {
	switch {
	case s == "":
		return errors.New("backup: an empty name cannot be part of a file name")
	case s == "." || s == "..":
		return fmt.Errorf("backup: %q is not a name, it is a directory reference", s)
	case strings.ContainsAny(s, `/\`+"\x00"):
		return fmt.Errorf("backup: %q contains a path separator, so it is a "+
			"claim on a path nobody meant to make", s)
	}
	return nil
}

// Write creates the archive at path and answers what it wrote.
//
// ⛔ THE FILE IS CREATED EXCLUSIVELY. An archive that silently landed on top of
// an existing one would destroy a backup in the act of taking one, which is
// the single worst thing this package could do. The name carries a
// second-resolution UTC stamp, so the only way to collide is two backups in
// one second, and that is a refusal rather than an overwrite.
//
// The manifest is written FIRST, so a reader knows what to expect before it
// meets it. Each source is hashed as it is copied, and the manifest is built
// from those hashes rather than from a separate pass, so what the manifest
// says and what the archive carries cannot come from two different reads of
// the same file.
func Write(path string, m Manifest, sources []Source) (Archive, error) {
	if path == "" {
		return Archive{}, errors.New("backup: Write needs a path to write to")
	}
	if m.Format != Format {
		return Archive{}, &FormatError{Found: m.Format, Known: Format}
	}
	if err := checkSources(sources); err != nil {
		return Archive{}, err
	}

	// Every member is hashed and measured BEFORE the file is created, so a
	// source that cannot be read leaves no half-written archive behind.
	m.Members = make([]Member, 0, len(sources))
	for _, s := range sources {
		mem, err := describe(s)
		if err != nil {
			return Archive{}, err
		}
		m.Members = append(m.Members, mem)
	}

	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Archive{}, fmt.Errorf("backup: encoding the manifest: %w", err)
	}
	body = append(body, '\n')

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Archive{}, fmt.Errorf("backup: creating %s: %w", path, err)
	}

	// The whole file's digest is taken as it is written rather than by reading
	// the file back: a second read can answer about different bytes, which is
	// exactly the class of defect this package exists to close.
	sum := sha256.New()
	counted := &countingWriter{w: io.MultiWriter(f, sum)}

	if err := writeBody(counted, body, sources, m.Members); err != nil {
		_ = f.Close()
		// The partial file is REMOVED, and this is the one removal in the
		// package. It is removing something this call created moments ago and
		// nothing else has ever seen, which is not the "existing state is never
		// deleted" rule decision 8 states about a restore.
		_ = os.Remove(path)
		return Archive{}, err
	}
	if err := f.Close(); err != nil {
		return Archive{}, fmt.Errorf("backup: closing %s: %w", path, err)
	}

	return Archive{
		Path:   path,
		Bytes:  counted.n,
		SHA256: hex.EncodeToString(sum.Sum(nil)),
	}, nil
}

// writeBody is the tar-inside-gzip half, split out so the caller above owns
// the file's lifetime and this owns the stream's.
func writeBody(w io.Writer, manifest []byte, sources []Source, members []Member) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	if err := writeEntry(tw, ManifestMember, int64(len(manifest)),
		strings.NewReader(string(manifest))); err != nil {
		return err
	}
	for i, s := range sources {
		src, err := os.Open(s.Path)
		if err != nil {
			return fmt.Errorf("backup: reading member %s from %s: %w", s.Name, s.Path, err)
		}
		err = writeEntry(tw, s.Name, members[i].Bytes, src)
		_ = src.Close()
		if err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("backup: closing the archive's tar stream: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("backup: closing the archive's gzip stream: %w", err)
	}
	return nil
}

// writeEntry writes one member, with a header carrying nothing that varies
// between machines.
//
// NO MODE BITS BEYOND 0600, no uid, no gid, no name: a tar carrying the
// writing user's identity is a fact about the machine rather than about the
// estate, and rig has no use for it on the way back in. The modification time
// is left at the zero value for the same reason - the manifest carries when
// the snapshot was taken, and two places for one fact is one place to
// disagree.
func writeEntry(tw *tar.Writer, name string, size int64, body io.Reader) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeReg,
		Mode:     0o600,
		Size:     size,
	}); err != nil {
		return fmt.Errorf("backup: writing the header for %s: %w", name, err)
	}
	written, err := io.Copy(tw, body)
	if err != nil {
		return fmt.Errorf("backup: writing %s into the archive: %w", name, err)
	}
	if written != size {
		// tar refuses this itself; the message here says WHICH member changed
		// under the writer, which tar's cannot.
		return fmt.Errorf("backup: %s was %d bytes when it was measured and %d "+
			"when it was copied: it is being written while it is archived",
			name, size, written)
	}
	return nil
}

// checkSources refuses a member set this package could not read back.
//
// The order matters as much as the set: the manifest lists members "in archive
// order", so a writer that emitted them in another order would produce an
// archive whose own manifest described a different file.
func checkSources(sources []Source) error {
	if len(sources) == 0 {
		return errors.New("backup: an archive with no members is not a backup " +
			"of anything; say what to carry")
	}
	if len(sources) > len(known) {
		return fmt.Errorf("backup: %d members were offered and this build knows "+
			"%d", len(sources), len(known))
	}
	for i, s := range sources {
		if s.Name != known[i] {
			return &UnknownMemberError{Name: s.Name}
		}
		if s.Path == "" {
			return fmt.Errorf("backup: member %s has no file to read", s.Name)
		}
	}
	return nil
}

// describe measures and hashes one source.
func describe(s Source) (Member, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return Member{}, fmt.Errorf("backup: reading member %s from %s: %w",
			s.Name, s.Path, err)
	}
	defer func() { _ = f.Close() }()

	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return Member{}, fmt.Errorf("backup: hashing member %s: %w", s.Name, err)
	}
	return Member{Name: s.Name, Bytes: n, SHA256: hex.EncodeToString(sum.Sum(nil))}, nil
}

// countingWriter counts what went through it, so the archive's own size is
// measured at the moment it is written rather than stat'd afterwards.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
