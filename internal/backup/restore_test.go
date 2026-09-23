package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// rawEntry is a tar member built BY HAND, including the ones this package
// would never write.
//
// ⛔ THE TESTS BELOW CANNOT GO THROUGH Write, AND THAT IS THE POINT. Write
// refuses every name outside the closed set, so an archive carrying `../x`
// does not exist until a test builds one. Acceptance test 4 says the
// adversarial inputs ARE the control, and this type is how they are produced.
type rawEntry struct {
	name     string
	body     []byte
	typeflag byte
	link     string
}

// craft writes an archive with exactly the manifest and entries given, with no
// checking of any kind.
func craft(t *testing.T, m Manifest, entries []rawEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crafted.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating the crafted archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshalling the crafted manifest: %v", err)
	}
	all := append([]rawEntry{{name: ManifestMember, body: body}}, entries...)
	for _, e := range all {
		flag := e.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		size := int64(len(e.body))
		if flag != tar.TypeReg {
			size = 0
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: e.name, Typeflag: flag, Mode: 0o600, Size: size, Linkname: e.link,
		}); err != nil {
			t.Fatalf("writing the crafted header for %q: %v", e.name, err)
		}
		if size > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatalf("writing the crafted body for %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing the crafted tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing the crafted gzip: %v", err)
	}
	return path
}

// craftValid builds a WELL-FORMED archive whose manifest is then mutated,
// which is how a test changes exactly one thing.
func craftValid(t *testing.T, b []byte, mutate func(*Manifest)) string {
	t.Helper()
	m := manifest()
	m.Members = []Member{{Bytes: int64(len(b)), Name: RecordMember, SHA256: sum(b)}}
	if mutate != nil {
		mutate(&m)
	}
	return craft(t, m, []rawEntry{{name: RecordMember, body: b}})
}

// target is a fresh estate directory path that does NOT exist yet.
func target(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "estates", "brief")
}

// assertNothingWasWritten is the assertion acceptance test 4 is actually
// about.
//
// A refusal that still created the directory would pass a test that only
// checked the error, and a half-restored estate is the thing decision 9
// exists to prevent. So every refusal below asserts the ABSENCE of all three
// names this package can create.
func assertNothingWasWritten(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused restore created %s", dir)
	}
	for _, pattern := range []string{dir + ".restoring-*", dir + ".replaced-*"} {
		found, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("globbing %s: %v", pattern, err)
		}
		if len(found) > 0 {
			t.Errorf("a refused restore left %v behind, and it was refused "+
				"BEFORE any byte should have been written", found)
		}
	}
}

// TestAnArchiveCarryingAnythingOutsideTheKnownSetIsRefusedBeforeAByteIsWritten
// is acceptance test 4, and the adversarial inputs are its control.
func TestAnArchiveCarryingAnythingOutsideTheKnownSetIsRefusedBeforeAByteIsWritten(t *testing.T) {
	good := body(t, 300)

	for _, c := range []struct {
		name    string
		archive func(t *testing.T) string
	}{
		{
			"a member named ../x",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 3, Name: "../x", SHA256: sum([]byte("abc"))}),
					[]rawEntry{{name: "../x", body: []byte("abc")}})
			},
		},
		{
			"an absolute member name",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 3, Name: "/etc/passwd", SHA256: sum([]byte("abc"))}),
					[]rawEntry{{name: "/etc/passwd", body: []byte("abc")}})
			},
		},
		{
			"a member named record.db-wal",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 3, Name: "record.db-wal", SHA256: sum([]byte("abc"))}),
					[]rawEntry{{name: "record.db-wal", body: []byte("abc")}})
			},
		},
		{
			"a member named coord.db",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 3, Name: "coord.db", SHA256: sum([]byte("abc"))}),
					[]rawEntry{{name: "coord.db", body: []byte("abc")}})
			},
		},
		{
			"a nested member name",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 3, Name: "a/record.db", SHA256: sum([]byte("abc"))}),
					[]rawEntry{{name: "a/record.db", body: []byte("abc")}})
			},
		},
		{
			"the tar carries a name the manifest does not declare",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: int64(len(good)), Name: RecordMember, SHA256: sum(good)}),
					[]rawEntry{{name: "../x", body: good}})
			},
		},
		{
			"an extra entry after the last declared member",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: int64(len(good)), Name: RecordMember, SHA256: sum(good)}),
					[]rawEntry{
						{name: RecordMember, body: good},
						{name: "../../etc/cron.d/x", body: []byte("* * * * * root sh")},
					})
			},
		},
		{
			"no manifest first",
			func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "nomanifest.tar.gz")
				f, err := os.Create(path)
				if err != nil {
					t.Fatalf("creating: %v", err)
				}
				gz := gzip.NewWriter(f)
				tw := tar.NewWriter(gz)
				if err := tw.WriteHeader(&tar.Header{
					Name: RecordMember, Typeflag: tar.TypeReg, Mode: 0o600,
					Size: int64(len(good)),
				}); err != nil {
					t.Fatalf("writing the header: %v", err)
				}
				if _, err := tw.Write(good); err != nil {
					t.Fatalf("writing the body: %v", err)
				}
				for _, closer := range []func() error{tw.Close, gz.Close, f.Close} {
					if err := closer(); err != nil {
						t.Fatalf("closing: %v", err)
					}
				}
				return path
			},
		},
		{
			"a symlink where the member should be",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 0, Name: RecordMember, SHA256: sum(nil)}),
					[]rawEntry{{name: RecordMember, typeflag: tar.TypeSymlink, link: "/etc/passwd"}})
			},
		},
		{
			"a directory where the member should be",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{Bytes: 0, Name: RecordMember, SHA256: sum(nil)}),
					[]rawEntry{{name: RecordMember, typeflag: tar.TypeDir}})
			},
		},
		{
			"the same member declared twice",
			func(t *testing.T) string {
				mem := Member{Bytes: int64(len(good)), Name: RecordMember, SHA256: sum(good)}
				return craft(t, withMembers(mem, mem), []rawEntry{
					{name: RecordMember, body: good}, {name: RecordMember, body: good},
				})
			},
		},
		{
			"no members at all",
			func(t *testing.T) string { return craft(t, manifest(), nil) },
		},
		{
			"a digest that is not a sha256",
			func(t *testing.T) string {
				return craftValid(t, good, func(m *Manifest) { m.Members[0].SHA256 = "NOTADIGEST" })
			},
		},
		{
			"a digest in the wrong case",
			func(t *testing.T) string {
				return craftValid(t, good, func(m *Manifest) {
					m.Members[0].SHA256 = upper(sum(good))
				})
			},
		},
		{
			"a format this build does not read",
			func(t *testing.T) string {
				return craftValid(t, good, func(m *Manifest) { m.Format = Format + 1 })
			},
		},
		{
			"a declared length the tar does not carry",
			func(t *testing.T) string {
				return craftValid(t, good, func(m *Manifest) { m.Members[0].Bytes++ })
			},
		},
		{
			"a declared member the tar is missing",
			func(t *testing.T) string {
				return craft(t, withMembers(Member{
					Bytes: int64(len(good)), Name: RecordMember, SHA256: sum(good),
				}), nil)
			},
		},
		{
			"not a gzip stream at all",
			func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "plain.tar.gz")
				if err := os.WriteFile(path, []byte("this is not an archive"), 0o600); err != nil {
					t.Fatalf("writing: %v", err)
				}
				return path
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			archive := c.archive(t)

			// Read is asserted as well as Restore, because Read is what a future
			// `rig restore --check` and any listing would go through, and a
			// refusal that only happens on the write path is not the property
			// decision 9 claims.
			if m, err := Read(archive); err == nil {
				t.Errorf("Read accepted it and answered %+v", m)
			}

			dir := target(t)
			if _, err := Restore(RestoreRequest{
				Archive: archive, Dir: dir, SchemaCeiling: 2, At: at,
			}); err == nil {
				t.Fatal("Restore accepted it")
			}
			assertNothingWasWritten(t, dir)
		})
	}
}

// withMembers is a manifest carrying exactly the members given.
func withMembers(members ...Member) Manifest {
	m := manifest()
	m.Members = members
	return m
}

// upper is strings.ToUpper for a hex digest, written here so the test file
// does not reach for a package to do one thing.
func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'f' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

// TestOneFlippedByteInsideTheArchiveRefusesTheRestore is acceptance test 3's
// red control, run as a test.
//
// ⛔ THE FLIP DOES NOT CHANGE THE LENGTH, so nothing about the archive's SHAPE
// is wrong: the name, the type and the declared size all still agree with the
// manifest. Only the digest can catch it, which is exactly what makes this the
// control for the digest assertion rather than for the shape assertions test 4
// covers.
//
// The target directory must not exist afterwards. The staging directory MUST,
// and that is decision 8: the bytes are kept, named for what they are, and rig
// does not decide on a person's behalf that they were worthless.
func TestOneFlippedByteInsideTheArchiveRefusesTheRestore(t *testing.T) {
	good := body(t, 4096)
	flipped := slices.Clone(good)
	flipped[2048] ^= 0x01

	// The manifest describes the GOOD bytes; the tar carries the flipped ones.
	m := manifest()
	m.Members = []Member{{Bytes: int64(len(good)), Name: RecordMember, SHA256: sum(good)}}
	archive := craft(t, m, []rawEntry{{name: RecordMember, body: flipped}})

	// Pass one passes: the archive's shape is beyond reproach.
	if _, err := Read(archive); err != nil {
		t.Fatalf("Read refused the archive on its shape, so this test is no "+
			"longer about the digest: %v", err)
	}

	dir := target(t)
	res, err := Restore(RestoreRequest{
		Archive: archive, Dir: dir, SchemaCeiling: 2, At: at,
	})
	var ie *IntegrityError
	if !errors.As(err, &ie) {
		t.Fatalf("Restore returned %v, want an *IntegrityError", err)
	}
	if ie.Member != RecordMember || ie.GotSum != sum(flipped) || ie.WantSum != sum(good) {
		t.Errorf("the refusal is %+v, and it must name the member and both "+
			"digests", ie)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s exists after a refused restore", dir)
	}
	if res.Staging == "" {
		t.Fatal("the result names no staging directory, so nobody can find the " +
			"bytes that were extracted")
	}
	if _, err := os.Stat(res.Staging); err != nil {
		t.Errorf("the staging directory %s is gone: a failed restore keeps what "+
			"it extracted, decision 8. %v", res.Staging, err)
	}
}

// TestRestoreOverExistingStateRefusesWithoutForceAndMovesItAsideWithIt is
// acceptance test 6, and its red control is the byte-identity assertion rather
// than mere presence.
func TestRestoreOverExistingStateRefusesWithoutForceAndMovesItAsideWithIt(t *testing.T) {
	fresh := body(t, 2048)
	archive := craftValid(t, fresh, nil)

	dir := target(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("making the existing estate: %v", err)
	}
	// ⛔ THE WAL IS WHY THE DIRECTORY MOVES AS ONE. A stale record.db-wal beside
	// a fresh record.db is replayed into it on the next open, so this test
	// carries one and asserts it does NOT survive into the restored estate.
	was := map[string][]byte{
		RecordMember:     body(t, 1024),
		"record.db-wal":  body(t, 777),
		"record.db-shm":  body(t, 64),
		"coord.db":       body(t, 128),
		"last-announced": []byte("0.4.0\n"),
	}
	for name, b := range was {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatalf("writing the existing %s: %v", name, err)
		}
	}

	// Without --force: refused, and NOTHING moved.
	_, err := Restore(RestoreRequest{Archive: archive, Dir: dir, SchemaCeiling: 2, At: at})
	var de *DirExistsError
	if !errors.As(err, &de) {
		t.Fatalf("Restore returned %v, want a *DirExistsError", err)
	}
	for name, want := range was {
		assertBytes(t, filepath.Join(dir, name), want)
	}
	if found, _ := filepath.Glob(dir + ".*"); len(found) > 0 {
		t.Errorf("a refused restore left %v beside the estate", found)
	}

	// With --force: the whole directory is renamed aside and the new state lands.
	res, err := Restore(RestoreRequest{
		Archive: archive, Dir: dir, Force: true, SchemaCeiling: 2,
		At: at.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Restore --force: %v", err)
	}
	if want := dir + ".replaced-20260923T140606Z"; res.Replaced != want {
		t.Errorf("the previous state went to %q, want %q", res.Replaced, want)
	}

	// The red control the row asks for: byte-identical, not merely present.
	for name, want := range was {
		assertBytes(t, filepath.Join(res.Replaced, name), want)
	}
	assertBytes(t, filepath.Join(dir, RecordMember), fresh)

	// And the restored estate carries the archive's members and NOTHING ELSE:
	// a surviving record.db-wal would be replayed into the restored database.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the restored estate: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{RecordMember}; !slices.Equal(names, want) {
		t.Errorf("the restored estate holds %v, want exactly %v: anything else "+
			"is state from before the restore", names, want)
	}
	if _, err := os.Stat(res.Staging); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the staging directory %s is still there after a successful "+
			"restore", res.Staging)
	}
}

// assertBytes fails unless path holds exactly want.
func assertBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("reading %s: %v", path, err)
		return
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s is %d bytes and should be the same %d bytes it was",
			path, len(got), len(want))
	}
}

// TestASchemaNewerThanThisBuildIsRefusedAndAnOlderOneIsNot is acceptance test
// 7, and the two numbers are its control.
func TestASchemaNewerThanThisBuildIsRefusedAndAnOlderOneIsNot(t *testing.T) {
	const ceiling = 2
	good := body(t, 256)

	t.Run("newer is refused", func(t *testing.T) {
		archive := craftValid(t, good, func(m *Manifest) { m.SchemaVersion = ceiling + 1 })
		dir := target(t)
		_, err := Restore(RestoreRequest{
			Archive: archive, Dir: dir, SchemaCeiling: ceiling, At: at,
		})
		var se *SchemaTooNewError
		if !errors.As(err, &se) {
			t.Fatalf("Restore returned %v, want a *SchemaTooNewError", err)
		}
		if se.Found != ceiling+1 || se.Ceiling != ceiling {
			t.Errorf("the refusal says found %d, ceiling %d; want %d and %d",
				se.Found, se.Ceiling, ceiling+1, ceiling)
		}
		// ⛔ THE CEILING IS CHECKED BEFORE ANYTHING IS CREATED. A restore that
		// staged the files and then noticed the schema would leave a directory
		// full of a store nothing on this machine can open.
		assertNothingWasWritten(t, dir)
	})

	for _, version := range []uint64{ceiling, ceiling - 1} {
		t.Run(fmt.Sprintf("schema %d is accepted and migrated later", version), func(t *testing.T) {
			archive := craftValid(t, good, func(m *Manifest) { m.SchemaVersion = version })
			dir := target(t)
			res, err := Restore(RestoreRequest{
				Archive: archive, Dir: dir, SchemaCeiling: ceiling, At: at,
			})
			if err != nil {
				t.Fatalf("schema %d was refused and it must be migrated on the "+
					"daemon's next open instead: %v", version, err)
			}
			if res.Manifest.SchemaVersion != version {
				t.Errorf("the result reports schema %d, want %d",
					res.Manifest.SchemaVersion, version)
			}
			assertBytes(t, filepath.Join(dir, RecordMember), good)
		})
	}
}

// TestARestoreRequestIsCheckedBeforeItCanTouchAnything covers the arguments
// that would make the two renames land somewhere nobody named.
func TestARestoreRequestIsCheckedBeforeItCanTouchAnything(t *testing.T) {
	archive := craftValid(t, body(t, 32), nil)
	base := t.TempDir()

	for _, c := range []struct {
		name string
		req  RestoreRequest
	}{
		{"no archive", RestoreRequest{Dir: base + "/e", SchemaCeiling: 2, At: at}},
		{"no directory", RestoreRequest{Archive: archive, SchemaCeiling: 2, At: at}},
		{"a relative directory", RestoreRequest{
			Archive: archive, Dir: "estates/brief", SchemaCeiling: 2, At: at,
		}},
		{"a directory that has not been cleaned", RestoreRequest{
			Archive: archive, Dir: base + "/estates/../estates/brief",
			SchemaCeiling: 2, At: at,
		}},
		{"the filesystem root", RestoreRequest{
			Archive: archive, Dir: "/", SchemaCeiling: 2, At: at,
		}},
		{"no schema ceiling", RestoreRequest{
			Archive: archive, Dir: base + "/e", At: at,
		}},
		{"no clock", RestoreRequest{
			Archive: archive, Dir: base + "/e", SchemaCeiling: 2,
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Restore(c.req); err == nil {
				t.Fatalf("Restore accepted %+v", c.req)
			}
			if c.req.Dir != "" && filepath.IsAbs(c.req.Dir) && c.req.Dir != "/" {
				assertNothingWasWritten(t, filepath.Clean(c.req.Dir))
			}
		})
	}
}

// TestRestoreRefusesAnEstatePathThatIsNotADirectory stops a symlink being
// renamed aside and a directory put in its place, which would orphan whatever
// it pointed at.
func TestRestoreRefusesAnEstatePathThatIsNotADirectory(t *testing.T) {
	archive := craftValid(t, body(t, 32), nil)
	base := t.TempDir()

	dir := filepath.Join(base, "afile")
	if err := os.WriteFile(dir, []byte("not an estate"), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, err := Restore(RestoreRequest{
		Archive: archive, Dir: dir, Force: true, SchemaCeiling: 2, At: at,
	}); err == nil {
		t.Fatal("Restore replaced a plain file")
	}
	assertBytes(t, dir, []byte("not an estate"))

	link := filepath.Join(base, "alink")
	if err := os.Symlink(base, link); err != nil {
		t.Fatalf("making the symlink: %v", err)
	}
	if _, err := Restore(RestoreRequest{
		Archive: archive, Dir: link, Force: true, SchemaCeiling: 2, At: at,
	}); err == nil {
		t.Fatal("Restore replaced a symlink")
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink at %s is no longer a symlink: %v %v", link, fi, err)
	}
}

// TestTwoRestoresCannotShareOneStagingDirectory stops two archives becoming
// one estate.
func TestTwoRestoresCannotShareOneStagingDirectory(t *testing.T) {
	archive := craftValid(t, body(t, 64), nil)
	dir := target(t)
	staging := dir + ".restoring-" + Stamp(at)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatalf("making the staging directory: %v", err)
	}
	marker := filepath.Join(staging, "from-the-other-restore")
	if err := os.WriteFile(marker, []byte("mine"), 0o600); err != nil {
		t.Fatalf("writing the marker: %v", err)
	}

	if _, err := Restore(RestoreRequest{
		Archive: archive, Dir: dir, SchemaCeiling: 2, At: at,
	}); err == nil {
		t.Fatal("Restore wrote into a staging directory that was already there")
	}
	assertBytes(t, marker, []byte("mine"))
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s exists after a refused restore", dir)
	}
}
