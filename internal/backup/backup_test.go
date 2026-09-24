package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// at is the clock every test in this package uses.
//
// Fixed, so a stamped directory name is an ASSERTABLE STRING rather than
// something a test has to match with a pattern. Two tests that both want a
// distinct stamp add to it rather than reaching for time.Now.
var at = time.Date(2026, 9, 23, 14, 5, 6, 0, time.UTC)

// body is a record.db stand-in with enough structure that a one-byte flip
// cannot land on a repeated value.
func body(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	for i := range n {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// manifest is the manifest every test here starts from.
func manifest() Manifest {
	return Manifest{
		CreatedUnixNano: at.UnixNano(),
		Estate:          "brief",
		Format:          Format,
		Heads:           17,
		Records:         22,
		RigVersion:      "0.0.0-test",
		SchemaVersion:   2,
		Wire:            "1",
	}
}

// archiveOf writes a real archive through Write and answers where it is.
func archiveOf(t *testing.T, b []byte) (string, Manifest, Archive) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, RecordMember)
	if err := os.WriteFile(src, b, 0o600); err != nil {
		t.Fatalf("writing the source member: %v", err)
	}
	path := filepath.Join(dir, "brief-20260923T140506Z.tar.gz")
	got, err := Write(path, manifest(), []Source{{Name: RecordMember, Path: src}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	m, err := Read(path)
	if err != nil {
		t.Fatalf("Read of what Write just wrote: %v", err)
	}
	return path, m, got
}

// TestAnArchiveReadsBackAsExactlyWhatWasWritten is acceptance test 3's first
// half: members, sizes and digests match the manifest.
//
// The second half - heads matching a query on the RESTORED file - needs a
// store, so it is asserted where a store can be opened
// (internal/daemon/backup_test.go and the end-to-end run). What this layer can
// prove, and does, is that the bytes come back IDENTICAL and that every number
// the query would be checked against survived the round trip.
func TestAnArchiveReadsBackAsExactlyWhatWasWritten(t *testing.T) {
	want := body(t, 9001)
	path, m, got := archiveOf(t, want)

	if len(m.Members) != 1 {
		t.Fatalf("the archive declares %d members, want 1: %+v", len(m.Members), m.Members)
	}
	mem := m.Members[0]
	switch {
	case mem.Name != RecordMember:
		t.Errorf("member name %q, want %q", mem.Name, RecordMember)
	case mem.Bytes != int64(len(want)):
		t.Errorf("member is %d bytes, want %d", mem.Bytes, len(want))
	case mem.SHA256 != sum(want):
		t.Errorf("member hashes to %s, want %s", mem.SHA256, sum(want))
	}

	// Every manifest field the acceptance test compares, checked one by one so
	// a failure names WHICH number did not travel.
	src := manifest()
	for _, c := range []struct {
		field     string
		got, want any
	}{
		{"created_unix_nano", m.CreatedUnixNano, src.CreatedUnixNano},
		{"estate", m.Estate, src.Estate},
		{"format", m.Format, Format},
		{"heads", m.Heads, src.Heads},
		{"records", m.Records, src.Records},
		{"rig_version", m.RigVersion, src.RigVersion},
		{"schema_version", m.SchemaVersion, src.SchemaVersion},
		{"wire", m.Wire, src.Wire},
	} {
		if c.got != c.want {
			t.Errorf("manifest %s came back %v, want %v", c.field, c.got, c.want)
		}
	}

	// The archive's own digest, checked against the file rather than against
	// Write's own arithmetic: the number the CLI prints is the number a person
	// will run sha256sum against.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the archive back: %v", err)
	}
	if got.SHA256 != sum(raw) {
		t.Errorf("Write reported sha256 %s and the file on disk hashes to %s",
			got.SHA256, sum(raw))
	}
	if got.Bytes != int64(len(raw)) {
		t.Errorf("Write reported %d bytes and the file on disk is %d",
			got.Bytes, len(raw))
	}
	if got.Path != path {
		t.Errorf("Write reported path %q, want %q", got.Path, path)
	}

	// And the member bytes themselves come back, which is the only assertion
	// here that a restore actually depends on.
	dir := filepath.Join(t.TempDir(), "estates", "brief")
	if _, err := Restore(RestoreRequest{
		Archive: path, Dir: dir, SchemaCeiling: 2, At: at,
	}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	back, err := os.ReadFile(filepath.Join(dir, RecordMember))
	if err != nil {
		t.Fatalf("reading the restored member: %v", err)
	}
	if !slices.Equal(back, want) {
		t.Errorf("the restored member is %d bytes and the original was %d, and "+
			"they are not the same bytes", len(back), len(want))
	}
}

// TestTheArchiveCarriesTheManifestFirstAndNothingButItsMembers is acceptance
// clause 1: `tar tzf` lists manifest.json then record.db and nothing else.
//
// It reads the tar directly rather than through Read, because Read is the
// thing under test everywhere else and a test of the layout that went through
// it would be asserting the reader against itself.
func TestTheArchiveCarriesTheManifestFirstAndNothingButItsMembers(t *testing.T) {
	path, _, _ := archiveOf(t, body(t, 512))

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("the archive is not gzip: %v", err)
	}
	tr := tar.NewReader(gz)

	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading the archive's tar stream: %v", err)
		}
		names = append(names, h.Name)
		if h.Typeflag != tar.TypeReg {
			t.Errorf("member %s is tar type %q, want a regular file",
				h.Name, string(rune(h.Typeflag)))
		}
		if h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" {
			t.Errorf("member %s carries the writing user (uid %d, gid %d, %q/%q); "+
				"an archive of an estate says nothing about the machine that wrote it",
				h.Name, h.Uid, h.Gid, h.Uname, h.Gname)
		}
	}
	if want := []string{ManifestMember, RecordMember}; !slices.Equal(names, want) {
		t.Errorf("the archive lists %v, want exactly %v", names, want)
	}
}

// TestTheManifestsKeysAreSorted holds the "keys sorted" row of the manifest
// table.
//
// ⛔ IT READS THE EMITTED JSON, NOT THE STRUCT. encoding/json emits struct
// fields in DECLARATION order, so the sorting is a property of how the type is
// written and nothing in the compiler will notice a field added in the wrong
// place. This test is the only thing that will.
func TestTheManifestsKeysAreSorted(t *testing.T) {
	b, err := json.MarshalIndent(manifest(), "", "  ")
	if err != nil {
		t.Fatalf("marshalling the manifest: %v", err)
	}

	keys := topLevelKeys(t, b)
	if len(keys) == 0 {
		t.Fatalf("no keys were read out of the manifest, so this test proved "+
			"NOTHING:\n%s", b)
	}
	if !slices.IsSorted(keys) {
		t.Errorf("the manifest's keys come out as %v, which is not sorted.\n"+
			"Declare the new field in alphabetical order of its json tag: "+
			"encoding/json emits declaration order and nothing else enforces this.",
			keys)
	}

	// The member object is a manifest of its own and is sorted on the same
	// terms. Asserted separately so the failure names which object is wrong.
	var probe struct {
		Members []json.RawMessage `json:"members"`
	}
	m2 := manifest()
	m2.Members = []Member{{Bytes: 1, Name: RecordMember, SHA256: sum(nil)}}
	b2, err := json.Marshal(m2)
	if err != nil {
		t.Fatalf("marshalling a manifest with a member: %v", err)
	}
	if err := json.Unmarshal(b2, &probe); err != nil {
		t.Fatalf("reading the members back: %v", err)
	}
	if len(probe.Members) != 1 {
		t.Fatalf("want one member, got %d", len(probe.Members))
	}
	if mk := topLevelKeys(t, probe.Members[0]); !slices.IsSorted(mk) {
		t.Errorf("a member's keys come out as %v, which is not sorted", mk)
	}
}

// topLevelKeys reads an object's keys in the order they appear in the bytes.
func topLevelKeys(t *testing.T, b []byte) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(b)))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		t.Fatalf("this is not a JSON object: %v\n%s", err, b)
	}
	var keys []string
	depth := 0
	for dec.More() || depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("reading the object: %v", err)
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		case string:
			if depth == 0 {
				keys = append(keys, v)
				// Skip this key's value whole, so a nested object's keys are not
				// mistaken for the outer object's.
				var discard json.RawMessage
				if err := dec.Decode(&discard); err != nil {
					t.Fatalf("skipping the value of %q: %v", v, err)
				}
			}
		}
	}
	return keys
}

// TestWriteWillNotLandOnAnArchiveThatIsAlreadyThere is the one thing this
// package could do that would be worse than failing: destroying a backup while
// taking one.
func TestWriteWillNotLandOnAnArchiveThatIsAlreadyThere(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, RecordMember)
	if err := os.WriteFile(src, body(t, 64), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}
	path := filepath.Join(dir, "a.tar.gz")
	precious := []byte("an older backup nobody wants to lose")
	if err := os.WriteFile(path, precious, 0o600); err != nil {
		t.Fatalf("writing the incumbent: %v", err)
	}

	if _, err := Write(path, manifest(), []Source{{Name: RecordMember, Path: src}}); err == nil {
		t.Fatal("Write landed on an existing archive, and it must refuse")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the incumbent back: %v", err)
	}
	if !slices.Equal(got, precious) {
		t.Errorf("the existing archive was modified: it is now %q", got)
	}
}

// TestWriteRefusesAMemberItCouldNotReadBack keeps the closed set closed from
// the writing side too.
func TestWriteRefusesAMemberItCouldNotReadBack(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x")
	if err := os.WriteFile(src, body(t, 8), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}

	for _, c := range []struct {
		name    string
		sources []Source
	}{
		{"no members at all", nil},
		{"a name outside the set", []Source{{Name: "coord.db", Path: src}}},
		{"the manifest as a member", []Source{{Name: ManifestMember, Path: src}}},
		{"more members than exist", []Source{
			{Name: RecordMember, Path: src}, {Name: RecordMember, Path: src},
		}},
		{"a member with no file", []Source{{Name: RecordMember}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "a.tar.gz")
			if _, err := Write(path, manifest(), c.sources); err == nil {
				t.Fatalf("Write accepted %v", c.sources)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a refused Write left %s behind", path)
			}
		})
	}
}

// TestWriteRefusesAFormatItDoesNotWrite stops a caller declaring a format
// number this build's layout does not match.
func TestWriteRefusesAFormatItDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, RecordMember)
	if err := os.WriteFile(src, body(t, 8), 0o600); err != nil {
		t.Fatalf("writing the source: %v", err)
	}
	m := manifest()
	m.Format = Format + 1
	_, err := Write(filepath.Join(dir, "a.tar.gz"), m, []Source{{Name: RecordMember, Path: src}})
	var fe *FormatError
	if !errors.As(err, &fe) {
		t.Fatalf("Write returned %v, want a *FormatError", err)
	}
}

// TestTheChosenNameIsStampedAndRefusesAPathComponent covers decision 6's
// naming, including the half a reader is most likely to skip: the estate name
// becomes a path component and is checked HERE, not only by the caller.
func TestTheChosenNameIsStampedAndRefusesAPathComponent(t *testing.T) {
	got, err := ArchiveName("production", at)
	if err != nil {
		t.Fatalf("ArchiveName: %v", err)
	}
	if want := "production-20260923T140506Z.tar.gz"; got != want {
		t.Errorf("ArchiveName = %q, want %q", got, want)
	}

	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "../../etc", "a\x00b"} {
		if name, err := ArchiveName(bad, at); err == nil {
			t.Errorf("ArchiveName(%q) = %q, and it must be refused", bad, name)
		}
	}
}

// TestThisPackageLinksNothingButTheStandardLibrary is acceptance test 9's own
// half, and it is the reason `go list -deps ./cmd/rig | grep -c modernc`
// prints 0.
//
// ⛔ cmd/rig/layering_test.go CHECKS THE BINARY AND THIS CHECKS THE CAUSE.
// That test fails after the mistake is made anywhere in the client's import
// graph and names only the symptom; this one fails in the package where the
// import would be written, and its message says what to do instead. Both are
// owed: the client's graph can grow a store through a package this one knows
// nothing about.
func TestThisPackageLinksNothingButTheStandardLibrary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps . could not run, so this test proved NOTHING "+
			"and must not be read as a pass: %v\n%s", err, out)
	}
	deps := strings.Split(strings.TrimSpace(string(out)), "\n")

	// The positive control: an absence is also what an empty or misdirected
	// `go list` produces, so the first assertion is a pair of packages this
	// one certainly does list.
	//
	// `go list -deps` includes the named package itself, which is the second
	// control AND the one exception the loop below skips.
	const self = "github.com/borismilner/rig/internal/backup"
	for _, control := range []string{"archive/tar", self} {
		if !slices.Contains(deps, control) {
			t.Fatalf("the control package %q is not in `go list -deps .`, so the "+
				"command answered about something other than this package and "+
				"every assertion below is meaningless. %d lines came back:\n%s",
				control, len(deps), out)
		}
	}

	for _, d := range deps {
		if d == self {
			continue
		}
		// The standard library's convention: a module path has a dot in its
		// first element and a standard library path never does.
		first, _, _ := strings.Cut(d, "/")
		if !strings.Contains(first, ".") {
			continue
		}
		t.Errorf("internal/backup links %s, and it must link the standard "+
			"library ONLY.\n"+
			"       cmd/rig imports this package, so anything reachable from "+
			"here is in the client binary. internal/record would drag "+
			"modernc.org/sqlite in behind it.\n"+
			"       Whatever this package needs from there is passed in as data "+
			"(RestoreRequest.SchemaCeiling is the worked example).", d)
	}
}
