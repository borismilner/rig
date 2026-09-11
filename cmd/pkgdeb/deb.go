package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5" //nolint:gosec // the .deb format's digest; see buildDeb
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// A .deb is an ar archive of exactly three members, in this order:
// debian-binary, control.tar.gz, data.tar.gz. dpkg reads them positionally, so
// the order is part of the format rather than a convention.
//
// It is written here rather than shelled out to dpkg-deb so the build does not
// need Debian tooling installed to produce a Debian package - which is the
// difference between a release that can be cut anywhere and one that can be
// cut on this laptop.

// file is one thing to put in the package.
type file struct {
	Path string // inside the package, without a leading slash
	Mode int64
	Body []byte
}

// buildDeb returns the bytes of a complete .deb.
func buildDeb(c control, files []file, when time.Time) ([]byte, error) {
	// Sorted so the same inputs give the same bytes: a package that differs
	// run to run cannot be compared against a published one.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	var size int64
	sums := &bytes.Buffer{}
	for _, f := range files {
		size += int64(len(f.Body))
		// md5 is not a security choice here. `md5sums` is a field of the
		// .deb format and `dpkg --verify` reads exactly this digest and no
		// other, so a stronger hash produces a file dpkg cannot use.
		//nolint:gosec // see above
		fmt.Fprintf(sums, "%x  %s\n", md5.Sum(f.Body), f.Path)
	}
	c.InstalledSize = size

	controlTar, err := tarGz([]file{
		{Path: "control", Mode: 0o644, Body: []byte(c.String())},
		{Path: "md5sums", Mode: 0o644, Body: sums.Bytes()},
	}, when, false)
	if err != nil {
		return nil, fmt.Errorf("control.tar.gz: %w", err)
	}
	dataTar, err := tarGz(files, when, true)
	if err != nil {
		return nil, fmt.Errorf("data.tar.gz: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("!<arch>\n")
	for _, m := range []struct {
		name string
		body []byte
	}{
		{"debian-binary", []byte("2.0\n")},
		{"control.tar.gz", controlTar},
		{"data.tar.gz", dataTar},
	} {
		if err := arMember(&out, m.name, m.body, when); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

// arMember writes one ar header and its payload.
func arMember(w io.Writer, name string, body []byte, when time.Time) error {
	// The ar header is fixed-width and space-padded, and every field is
	// decimal text except the mode, which is octal.
	h := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n",
		name, when.Unix(), 0, 0, 0o100644, len(body))
	if len(h) != 60 {
		return fmt.Errorf("ar header for %q came out %d bytes, not 60", name, len(h))
	}
	if _, err := io.WriteString(w, h); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	// Members are padded to an even length; the pad is not counted in size.
	if len(body)%2 == 1 {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

// tarGz builds one of the two inner archives.
//
// withDirs adds the directory entries the data archive needs: dpkg tracks
// directory ownership, and a data archive that names ./usr/bin/rig without
// naming ./usr/bin leaves the directory unowned by any package.
func tarGz(files []file, when time.Time, withDirs bool) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(zw)

	if withDirs {
		for _, dir := range dirsOf(files) {
			if err := tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     "./" + dir + "/",
				Mode:     0o755,
				ModTime:  when,
				Format:   tar.FormatGNU,
			}); err != nil {
				return nil, err
			}
		}
	}

	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     "./" + f.Path,
			Mode:     f.Mode,
			Size:     int64(len(f.Body)),
			ModTime:  when,
			Format:   tar.FormatGNU,
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.Body); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// dirsOf returns every ancestor directory of the given files, with no
// repeats, sorted so the archive is built the same way twice.
func dirsOf(files []file) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		parts := strings.Split(f.Path, "/")
		// The last element is the file name, so it is not a directory.
		for i := 1; i < len(parts); i++ {
			d := strings.Join(parts[:i], "/")
			if d != "" && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	sort.Strings(out)
	return out
}

// readBinary loads one built binary.
func readBinary(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w\n"+
			"       the package is built from what is already in build/; "+
			"run `make build` first", path, err)
	}
	return b, nil
}
