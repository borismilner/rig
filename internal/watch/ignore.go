package watch

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Defaults remain active when a group supplies an empty or additional list.
var defaultIgnores = []string{
	".git", "node_modules", "build", "dist", "out", "target",
	".cache", "__pycache__", ".pytest_cache", ".mypy_cache",
	".ruff_cache", ".next", ".nuxt", ".parcel-cache", ".turbo",
}

func ignoredBy(g Group, name string) bool {
	for p := filepath.Clean(name); p != filepath.Dir(p); p = filepath.Dir(p) {
		base := filepath.Base(p)
		if slices.Contains(defaultIgnores, base) {
			return true
		}
		for _, pattern := range g.Ignore {
			if ok, _ := filepath.Match(pattern, base); ok {
				return true
			}
		}
	}
	return false
}

func accepts(g Group, name string) bool {
	return !ignoredBy(g, name) && matches(g, name)
}

func beneath(name, root string) bool {
	return name == root || strings.HasPrefix(name, root+string(filepath.Separator)) || root == string(filepath.Separator)
}

// A shared traversal is pruned only when no group needs it. Static glob
// prefixes and missing-root ancestors still need subscriptions.
func needed(c Config, name string) bool {
	for _, g := range c.Groups {
		if ignoredBy(g, name) {
			continue
		}
		for _, pattern := range g.Paths {
			root := scanRoot(pattern)
			if !ignoredBy(g, root) && (beneath(name, root) || beneath(root, name)) {
				return true
			}
		}
	}
	return false
}

func normaliseIgnores(g Group) ([]string, error) {
	if len(g.Ignore) > 256 {
		return nil, fmt.Errorf("watch %s: at most 256 ignore patterns", g.ID)
	}
	g.Ignore = slices.Clone(g.Ignore)
	for _, pattern := range g.Ignore {
		if pattern == "" || len(pattern) > 255 || strings.ContainsAny(pattern, "/\\\x00") || strings.Contains(pattern, "**") {
			return nil, fmt.Errorf("watch %s: ignore must be a basename glob without ** or escapes", g.ID)
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("watch %s: invalid ignore glob %q: %w", g.ID, pattern, err)
		}
	}
	slices.Sort(g.Ignore)
	g.Ignore = slices.Compact(g.Ignore)
	if len(g.Ignore) == 0 {
		g.Ignore = nil
	}
	return g.Ignore, nil
}
