package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// debArch maps Go's architecture names to Debian's.
//
// They agree on arm64 and disagree on everything else that matters, and a
// package built for the wrong architecture installs and then does not run.
func debArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	case "386":
		return "i386", nil
	case "arm":
		return "armhf", nil
	}
	return "", fmt.Errorf("no Debian architecture is known for GOARCH %q", goarch)
}

// debVersion turns `git describe` output into a version dpkg will accept.
//
// Policy: a version is [epoch:]upstream[-revision], and the UPSTREAM PART MUST
// START WITH A DIGIT. `git describe` gives v0.0.0-m0-79-gfb330d8, whose
// leading v breaks exactly that rule, and dpkg's refusal names the policy
// rather than the tag - so the v comes off here, where the reason can be
// written down.
//
// The hyphens are left alone: policy allows them in the upstream part as long
// as a revision follows, and the last one becomes the revision. Rewriting them
// would make the package version stop matching the tag it was built from,
// which is the one property anybody reads it for.
func debVersion(describe string) (string, error) {
	v := strings.TrimSpace(describe)
	if v == "" {
		return "", errors.New("--version is empty")
	}
	v = strings.TrimPrefix(v, "v")
	if v == "" || v[0] < '0' || v[0] > '9' {
		return "", fmt.Errorf("version %q does not start with a digit once "+
			"the leading v is removed, and dpkg refuses that: a Debian "+
			"upstream version has to begin with a number", describe)
	}
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '.', r == '+', r == '-', r == '~', r == ':':
		default:
			return "", fmt.Errorf("version %q contains %q, which dpkg does "+
				"not allow in a version", describe, r)
		}
	}
	return v, nil
}

// control is the package's own description of itself.
type control struct {
	Package       string
	Version       string
	Architecture  string
	Maintainer    string
	Description   string
	Summary       string
	InstalledSize int64 // bytes; written as KiB, which is what the field means
}

// String renders the control file.
//
// The blank-line rule is the one that bites: a continuation line in a
// description must start with a space, and an EMPTY line in it must be a lone
// full stop. A literally empty line ends the stanza, and dpkg then reports the
// rest of the description as a parse error somewhere else entirely.
func (c control) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Package: %s\n", c.Package)
	fmt.Fprintf(&b, "Version: %s\n", c.Version)
	fmt.Fprintf(&b, "Architecture: %s\n", c.Architecture)
	fmt.Fprintf(&b, "Maintainer: %s\n", c.Maintainer)
	fmt.Fprintf(&b, "Section: utils\n")
	fmt.Fprintf(&b, "Priority: optional\n")
	fmt.Fprintf(&b, "Installed-Size: %d\n", (c.InstalledSize+1023)/1024)
	fmt.Fprintf(&b, "Description: %s\n", c.Summary)
	for _, line := range strings.Split(c.Description, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString(" .\n")
			continue
		}
		fmt.Fprintf(&b, " %s\n", line)
	}
	return b.String()
}

// hostArch is the architecture of the binaries this build produced.
func hostArch() (string, error) { return debArch(runtime.GOARCH) }
