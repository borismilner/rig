package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RootEnv names the environment variable that moves rig's storage root.
const RootEnv = "RIG_ROOT"

// Areas are the three storage areas of one estate (plan/48, R14-R21).
//
// ⛔ THEY ARE SIBLINGS AND NEVER NEST, which is R16: rig's internal storage is
// kept apart from the files programs write freely, and an export is a third
// structure apart from both. A git repository over Files or Exports can then
// never reach a database file, whatever a program writes.
type Areas struct {
	// Internal is rig's own: databases and configuration. Never in git.
	Internal string
	// Files holds one free directory per program plus the shared one. A git
	// repository that rig commits on an interval.
	Files string
	// Exports holds the text exports of the store. A git repository that rig
	// commits when an export is written.
	Exports string
}

// Root is where a NAMED estate keeps its storage: the flag if one was given,
// else RIG_ROOT, else ~/.rig (plan/48, R14 and R15).
//
// The flag is the caller's to pass: rigd reads --root and hands it here, so a
// command line always beats the environment. An empty flag means "not given".
//
// A relative root is refused rather than resolved against the working
// directory, because a daemon started from two different directories would
// then keep two different stores under one name without saying so.
func Root(flag string) (string, error) {
	if flag != "" {
		return CheckRoot(flag, "--root")
	}
	if env := os.Getenv(RootEnv); env != "" {
		return CheckRoot(env, RootEnv)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("paths: %s is unset and the home directory could not "+
			"be resolved, so rig has no default root: %w", RootEnv, err)
	}
	return filepath.Join(home, ".rig"), nil
}

// ScratchRoot is where an UNNAMED estate keeps its storage: the flag if one
// was given, else a directory under the runtime directory.
//
// ⛔ RIG_ROOT IS DELIBERATELY NOT READ HERE. Every test in this repository
// starts an unnamed estate with the developer's environment, and one exported
// RIG_ROOT would otherwise put all of them inside the live store. An unnamed
// estate is disposable (section 37), so it gets a root only by being told one
// on its own command line.
func ScratchRoot(flag string) (string, error) {
	if flag != "" {
		return CheckRoot(flag, "--root")
	}
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "root"), nil
}

// CheckRoot accepts an absolute path and returns it cleaned. source names
// where the value came from, so the refusal says which setting to fix.
func CheckRoot(dir, source string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("paths: %s must be an absolute path, got %q", source, dir)
	}
	return filepath.Clean(dir), nil
}

// EstateAreas are the areas of a named estate under root:
// <root>/estates/<name>/{internal,files,exports}.
//
// One subtree per estate because section 37 forbids two estates sharing state,
// and the name is checked because it becomes a path element.
func EstateAreas(root, name string) (Areas, error) {
	if err := ValidEstateName(name); err != nil {
		return Areas{}, fmt.Errorf("paths: an estate has no storage areas until "+
			"it is named, and this name cannot be used: %w", err)
	}
	if root == "" {
		return Areas{}, errors.New("paths: no root to put the estate's areas under")
	}
	return areasUnder(filepath.Join(root, "estates", name)), nil
}

// ScratchAreas are the areas of an unnamed estate: directly under its root,
// since it has no name to key a subtree to.
func ScratchAreas(root string) (Areas, error) {
	if root == "" {
		return Areas{}, errors.New("paths: no root to put the scratch areas under")
	}
	return areasUnder(root), nil
}

func areasUnder(dir string) Areas {
	return Areas{
		Internal: filepath.Join(dir, "internal"),
		Files:    filepath.Join(dir, "files"),
		Exports:  filepath.Join(dir, "exports"),
	}
}
