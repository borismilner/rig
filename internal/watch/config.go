// Package watch turns estate-local filesystem changes into debounced events.
// It uses rig's SQLite opener and accepts the daemon's bus and coverage hooks.
package watch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	DefaultDebounce   = 15 * time.Minute
	DefaultMaxWatches = 8192
	DefaultMaxEntries = 100000
)

// Group collects paths under one quiet-period deadline. Paths are absolute;
// glob syntax is filepath.Match's (*, ?, [...]), without recursive **.
// A directory, including one matched by a glob, includes its descendants.
type Group struct {
	ID          string   `json:"id"`
	Paths       []string `json:"paths"`
	DebounceMS  int64    `json:"debounce_ms"`
	Description string   `json:"description,omitempty"`
}

// Config is a complete replacement, also used by future runtime declarations.
// MaxWatches counts this instance's directory watches, including sentinels.
// MaxEntries bounds the sum of snapshot entries across all groups.
type Config struct {
	Groups     []Group `json:"watches"`
	MaxWatches int     `json:"max_watches"`
	MaxEntries int     `json:"max_entries"`
}

var groupID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ReadConfig decodes a JSON config file and rejects unknown fields or trailing
// values. The daemon may instead resolve its own config and call Configure.
func ReadConfig(r io.Reader) (Config, error) {
	raw, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil {
		return Config{}, fmt.Errorf("watch config: %w", err)
	}
	if len(raw) > 1<<20 {
		return Config{}, fmt.Errorf("watch config: exceeds 1 MiB")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return Config{}, fmt.Errorf("watch config: expected one JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var c Config
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("watch config: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("watch config: expected one JSON object")
	}
	return normalize(c)
}

func normalize(c Config) (Config, error) {
	if c.MaxWatches == 0 {
		c.MaxWatches = DefaultMaxWatches
	}
	if c.MaxEntries == 0 {
		c.MaxEntries = DefaultMaxEntries
	}
	if c.MaxWatches < 1 || c.MaxEntries < 1 {
		return c, fmt.Errorf("watch: limits must be positive")
	}
	if len(c.Groups) > 256 {
		return c, fmt.Errorf("watch: at most 256 groups")
	}
	c.Groups = slices.Clone(c.Groups)
	seen := map[string]bool{}
	for i, g := range c.Groups {
		if !groupID.MatchString(g.ID) || seen[g.ID] {
			return c, fmt.Errorf("watch: invalid or duplicate id %q", g.ID)
		}
		seen[g.ID] = true
		if len(g.Paths) == 0 || len(g.Paths) > 256 {
			return c, fmt.Errorf("watch %s: requires 1-256 paths", g.ID)
		}
		if len(g.Description) > 4096 {
			return c, fmt.Errorf("watch %s: description exceeds 4096 bytes", g.ID)
		}
		if g.DebounceMS == 0 {
			g.DebounceMS = DefaultDebounce.Milliseconds()
		}
		if g.DebounceMS < 1 || g.DebounceMS > (365*24*time.Hour).Milliseconds() {
			return c, fmt.Errorf("watch %s: debounce_ms must be between 1 and one year", g.ID)
		}
		g.Paths = slices.Clone(g.Paths)
		for j, p := range g.Paths {
			if !filepath.IsAbs(p) || len(p) > 4096 || strings.ContainsAny(p, "\x00\\") || strings.Contains(p, "**") {
				return c, fmt.Errorf("watch %s: path %q must be absolute, without ** or escapes", g.ID, p)
			}
			if _, err := filepath.Match(p, ""); err != nil {
				return c, fmt.Errorf("watch %s: invalid glob %q: %w", g.ID, p, err)
			}
			g.Paths[j] = filepath.Clean(p)
		}
		slices.Sort(g.Paths)
		g.Paths = slices.Compact(g.Paths)
		c.Groups[i] = g
	}
	slices.SortFunc(c.Groups, func(a, b Group) int { return strings.Compare(a.ID, b.ID) })
	return c, nil
}

func matches(g Group, name string) bool {
	for _, pattern := range g.Paths {
		for p := name; ; p = filepath.Dir(p) {
			ok, _ := filepath.Match(pattern, p)
			if ok {
				return true
			}
			if p == filepath.Dir(p) {
				break
			}
		}
	}
	return false
}

func scanRoot(pattern string) string {
	if i := strings.IndexAny(pattern, "*?["); i >= 0 {
		return filepath.Dir(pattern[:i] + "x")
	}
	return pattern
}
