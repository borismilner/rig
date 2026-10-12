package watch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type backend interface {
	Add(string) error
	Remove(string) error
	WatchList() []string
	Close() error
}

// LimitError refuses a watch before asking the kernel for a descriptor.
type LimitError struct {
	Used, Ceiling int
	Path          string
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("watch: %d directory watches already use the ceiling %d; refusing %s", e.Used, e.Ceiling, e.Path)
}

// Usage counts this module's watches, not other processes' inotify usage.
// KernelMaxWatches is the per-user ceiling; zero means it was unavailable.
type Usage struct {
	Watches          int `json:"watches"`
	Ceiling          int `json:"ceiling"`
	KernelMaxWatches int `json:"kernel_max_watches"`
	Entries          int `json:"entries"`
}

func kernelWatchLimit() int {
	b, err := os.ReadFile("/proc/sys/fs/inotify/max_user_watches")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func (w *Watcher) ignored(path string) bool {
	return path == w.state.path || path == w.state.path+"-wal" || path == w.state.path+"-shm" || path == w.state.path+"-journal"
}

func fileStamp(path string, info fs.FileInfo) (stamp, error) {
	s := stamp{Size: info.Size(), Modified: info.ModTime().UnixNano(), Mode: uint32(info.Mode())}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		if err != nil {
			return s, err
		}
		s.Link = link
	}
	return s, nil
}

// Refuse symlink ancestors rather than letting a lexical path cover a tree
// outside its declaration. The selected symlink itself is an ordinary entry.
func checkAncestors(path string) error {
	for p := filepath.Dir(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("watch: ancestor %s is a symlink", p)
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}

func existingDir(path string) (string, error) {
	for {
		info, err := os.Lstat(path)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("watch: ancestor %s is a symlink", path)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("watch: ancestor %s is not a directory", path)
			}
			return path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		path = parent
	}
}

// scan adds a directory before enumerating children. A complete baseline is
// returned only after every root succeeds; failed scans leave durable state
// intact and remove any subscriptions they added.
func (w *Watcher) scan(ctx context.Context, c Config) (entries map[string]map[string]stamp, wanted map[string]bool, err error) {
	before := map[string]bool{}
	for _, p := range w.backend.WatchList() {
		before[p] = true
	}
	wanted = map[string]bool{}
	added := []string{}
	defer func() {
		if err != nil {
			for _, p := range added {
				_ = w.backend.Remove(p)
			}
		}
	}()
	ceiling := c.MaxWatches
	if w.kernelLimit > 0 && w.kernelLimit < ceiling {
		ceiling = w.kernelLimit
	}
	add := func(path string) error {
		if wanted[path] {
			return nil
		}
		if !before[path] {
			used := len(before) + len(added)
			if used >= ceiling {
				return &LimitError{Used: used, Ceiling: ceiling, Path: path}
			}
			if e := w.backend.Add(path); e != nil {
				return fmt.Errorf("watch: cannot subscribe to %s (%d/%d): %w", path, used, ceiling, e)
			}
			added = append(added, path)
		}
		wanted[path] = true
		return nil
	}
	entries = map[string]map[string]stamp{}
	for _, g := range c.Groups {
		entries[g.ID] = map[string]stamp{}
	}
	roots := map[string]bool{}
	for _, g := range c.Groups {
		for _, p := range g.Paths {
			roots[scanRoot(p)] = true
		}
	}
	ordered := make([]string, 0, len(roots))
	for p := range roots {
		ordered = append(ordered, p)
	}
	slices.Sort(ordered)
	visited := map[string]bool{}
	count := 0
	for _, root := range ordered {
		if err = ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err = checkAncestors(root); err != nil {
			return nil, nil, err
		}
		var sentinel string
		sentinel, err = existingDir(filepath.Dir(root))
		if err != nil {
			return nil, nil, err
		}
		if err = add(sentinel); err != nil {
			return nil, nil, err
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if visited[path] {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			visited[path] = true
			if w.ignored(path) {
				return nil
			}
			if d.IsDir() {
				if e := add(path); e != nil {
					return e
				}
			}
			info, e := d.Info()
			if errors.Is(e, os.ErrNotExist) {
				return nil
			}
			if e != nil {
				return e
			}
			s, e := fileStamp(path, info)
			if e != nil {
				return e
			}
			for _, g := range c.Groups {
				if matches(g, path) {
					count++
					if count > c.MaxEntries {
						return fmt.Errorf("watch: snapshot exceeds max_entries %d", c.MaxEntries)
					}
					entries[g.ID][path] = s
				}
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return entries, wanted, nil
}

func (w *Watcher) prune(wanted map[string]bool) error {
	for _, p := range w.backend.WatchList() {
		if !wanted[p] {
			if err := w.backend.Remove(p); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
				return err
			}
		}
	}
	return nil
}

func diffEntries(old, next map[string]stamp) []delta {
	var out []delta
	for path, value := range next {
		if was, ok := old[path]; !ok || was != value {
			out = append(out, delta{path: path, value: &value})
		}
	}
	for path := range old {
		if _, ok := next[path]; !ok {
			out = append(out, delta{path: path})
		}
	}
	slices.SortFunc(out, func(a, b delta) int { return strings.Compare(a.path, b.path) })
	return out
}
