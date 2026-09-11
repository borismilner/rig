// Command pkgdeb builds the .deb from the binaries already in build/.
//
// WHAT THIS DELIBERATELY IS NOT. Section 23 puts packaging at M15: the .deb,
// a desktop entry, a signed update channel and self-update. None of that is
// here, and none of it should be pulled forward - M15's demo is a fresh
// machine to a working rig in one command, and a half-built update channel
// would make that harder to get right rather than easier.
//
// What is here is the smallest thing that makes `make package` produce a real,
// installable artefact: the two binaries, in /usr/bin, with a control file
// dpkg accepts. The autostart unit is M6's (section 5l) and is not here either.
//
// The archive is written directly rather than shelled out to dpkg-deb, so a
// release can be cut on a machine with no Debian tooling installed.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	var (
		version = flag.String("version", "", "the version to package, as `git describe` gives it")
		out     = flag.String("out", "dist/", "directory to write the .deb into")
		bindir  = flag.String("bindir", "build/", "directory to take the built binaries from")
	)
	flag.Parse()

	path, err := run(*version, *out, *bindir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pkgdeb: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(path)
}

// shipped is what the package installs. Two binaries, and section 2's split is
// the reason there are two: rigd holds the daemon, rig links none of it.
var shipped = []struct{ binary, installAs string }{
	{"rigd", "usr/bin/rigd"},
	{"rig", "usr/bin/rig"},
}

func run(version, out, bindir string) (string, error) {
	v, err := debVersion(version)
	if err != nil {
		return "", err
	}
	arch, err := hostArch()
	if err != nil {
		return "", err
	}

	var files []file
	for _, s := range shipped {
		body, err := readBinary(filepath.Join(bindir, s.binary))
		if err != nil {
			return "", err
		}
		files = append(files, file{Path: s.installAs, Mode: 0o755, Body: body})
	}

	c := control{
		Package:      "rig",
		Version:      v,
		Architecture: arch,
		Maintainer:   "Boris Milner <boris@minimus.io>",
		Summary:      "One place a program says what it can do",
		Description: "rig is a daemon and a client. A program declares its\n" +
			"commands once, at connect, and gets a CLI, a window pane and the\n" +
			"other surfaces from that one declaration.\n" +
			"\n" +
			"This package installs the daemon and the client. It does not\n" +
			"install an autostart unit or a desktop entry.",
	}

	// A fixed timestamp, so the same inputs give the same bytes. Build time in
	// an archive makes two packages built from one commit differ, and then
	// nothing can be compared against a published artefact.
	when := time.Unix(0, 0).UTC()

	body, err := buildDeb(c, files, when)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", out, err)
	}
	path := filepath.Join(out, fmt.Sprintf("rig_%s_%s.deb", v, arch))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}
