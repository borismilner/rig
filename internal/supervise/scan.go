package supervise

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Decision 0265: rig finds its programs in the directories its configuration
// names, and each binary declares how it is loaded. Everything in a scan
// directory is a program: rig runs each once to read it, so a directory
// holding anything else (~/.local/bin) is never one to scan.

// DefaultScan is programs.scan's default.
const DefaultScan = "~/.local/lib/rig/apps"

// ScanDirs splits programs.scan, colon-separated as PATH is, and resolves a
// leading ~/ against home. A directory that is not absolute after that, or
// that walks upward, is refused: it would make what rig runs depend on the
// directory rigd was started in.
func ScanDirs(setting, home string) ([]string, error) {
	var out []string
	for _, d := range strings.Split(setting, ":") {
		if d == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(d, "~/"); ok {
			if home == "" {
				return nil, fmt.Errorf("programs.scan: %q needs a home directory and there is none", d)
			}
			d = filepath.Join(home, rest)
		}
		if !filepath.IsAbs(d) || strings.Contains(d, "..") {
			return nil, fmt.Errorf("programs.scan: %q is not an absolute path", d)
		}
		out = append(out, filepath.Clean(d))
	}
	return out, nil
}

// ErrScanUnreadable is a scan directory that exists and could not be read,
// so what it holds is unknown rather than empty.
var ErrScanUnreadable = errors.New("a scan directory could not be read")

// Scan lists the programs in dirs, id to path: every executable regular
// file (a symlink to one counts), named by its file name. The first
// directory wins an id found twice, as PATH does, and the second is
// reported. A directory that does not exist holds nothing, which is the
// common case before anything is installed.
func Scan(dirs []string) (found map[string]string, problems []error) {
	found = map[string]string{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%w: %s: %w", ErrScanUnreadable, dir, err))
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(dir, name)
			fi, err := os.Stat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
				continue
			}
			if err := (Spec{ID: name, Path: path}).Validate(); err != nil {
				problems = append(problems, err)
				continue
			}
			if first, dup := found[name]; dup {
				problems = append(problems, fmt.Errorf("%s is shadowed by %s, found first", path, first))
				continue
			}
			found[name] = path
		}
	}
	return found, problems
}

// Merge is the declared set: programs.json's rows, with what the scan found.
//
// A row with a path is declared as it always was. A row without one is an
// OVERRIDE of the scanned program of that id (decision 0265): its args,
// health, budget and load mode win, and the path is the scan's. A scanned
// program with no row needs none. Its load mode is the binary's unless the
// row set autostart or on_call.
func Merge(rows []Spec, found map[string]string) (specs []Spec, problems []error) {
	overrides := map[string]Spec{}
	for _, r := range rows {
		if r.Path != "" {
			specs = append(specs, r)
			continue
		}
		overrides[r.ID] = r
	}
	explicit := map[string]bool{}
	for _, r := range specs {
		explicit[r.ID] = true // an explicit row is the program of that id
	}
	for id, path := range found {
		if explicit[id] {
			continue
		}
		spec, ok := overrides[id]
		if !ok {
			spec = Spec{ID: id, Health: DefaultHealth(), Budget: DefaultBudget()}
		}
		delete(overrides, id)
		spec.Path, spec.Scanned = path, true
		if !spec.Autostart && !spec.OnCall {
			spec.FromBinary, spec.OnCall = true, true
		}
		specs = append(specs, spec)
	}
	for id := range overrides {
		problems = append(problems, fmt.Errorf(
			"programs.json overrides %q, which no scan directory holds", id))
	}
	return specs, problems
}
