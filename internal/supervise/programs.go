package supervise

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// The declared programs, on disk.
//
// ⛔ ONE FILE, `encoding/json`, NO LAYERING, AND THAT IS A DECISION RATHER
// THAN A SHORTCUT. PLAN.md section 22 pins knadh/koanf/v2 as rig's config
// library and section 23's cherry-pick table defers the config LAYERS to M4,
// so there is no resolver to read this through yet. The two ways to go wrong
// here were adopting koanf early - growing rigd for a file that has no layers
// and pre-empting M4's own design - or inventing a second config mechanism
// that M4 would later have to discover.
//
// So this is written down as neither: the M4 resolver's FIRST CONSUMER. When
// section 11's layers land, this reader is deleted and `Load` becomes a call
// into the resolver. The file format below is the schema M4 inherits.
//
// It is also why the footprint ruling is satisfied: `encoding/json` is already
// linked by rigd through the MCP surface, so this costs a struct.

// ProgramsFile is the name under the config directory.
const ProgramsFile = "programs.json"

// declared is the file's schema. It is not Spec: Spec holds time.Duration,
// and a declaration written by a person holds "5s".
type declared struct {
	Programs []declaredProgram `json:"programs"`
}

type declaredProgram struct {
	ID   string   `json:"id"`
	Path string   `json:"path"`
	Args []string `json:"args,omitempty"`
	Dir  string   `json:"dir,omitempty"`
	Env  []string `json:"env,omitempty"`

	Health declaredHealth `json:"health"`
	Budget declaredBudget `json:"budget"`
}

type declaredHealth struct {
	Interval string `json:"interval,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
	Idle     string `json:"idle,omitempty"`
	Register string `json:"register,omitempty"`
	Degraded int    `json:"degraded,omitempty"`
	Restart  int    `json:"restart,omitempty"`
}

type declaredBudget struct {
	Restarts   int    `json:"restarts,omitempty"`
	Window     string `json:"window,omitempty"`
	Backoff    string `json:"backoff,omitempty"`
	MaxBackoff string `json:"max_backoff,omitempty"`
}

// ProgramsPath is where the declared programs are read from:
// $XDG_CONFIG_HOME/rig/programs.json, falling back to ~/.config as the XDG
// base directory specification says to.
func ProgramsPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no XDG_CONFIG_HOME and no home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "rig", ProgramsFile), nil
}

// Load reads the declared programs.
//
// A MISSING FILE IS NOT AN ERROR AND THAT IS THE COMMON CASE: an estate that
// declares no programs is an estate with nothing to supervise, and `rig up`
// on it should say so rather than fail. Anything else - unreadable, malformed,
// a program that could not be launched safely - IS an error, named with the
// path, because a supervisor that silently supervises less than it was told to
// is the failure section 18 exists to prevent.
func Load(path string) ([]Spec, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var f declared
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	specs := make([]Spec, 0, len(f.Programs))
	for i, d := range f.Programs {
		spec, err := d.spec()
		if err != nil {
			return nil, fmt.Errorf("%s: program %d: %w", path, i+1, err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// LoadDefault reads the declared programs from ProgramsPath.
func LoadDefault() ([]Spec, error) {
	path, err := ProgramsPath()
	if err != nil {
		return nil, err
	}
	return Load(path)
}

func (d declaredProgram) spec() (Spec, error) {
	h := DefaultHealth()
	if err := durations([]duration{
		{"interval", &h.Interval, d.Health.Interval},
		{"timeout", &h.Timeout, d.Health.Timeout},
		{"idle", &h.Idle, d.Health.Idle},
		{"register", &h.Register, d.Health.Register},
	}); err != nil {
		return Spec{}, err
	}
	if d.Health.Degraded > 0 {
		h.Degraded = d.Health.Degraded
	}
	if d.Health.Restart > 0 {
		h.Restart = d.Health.Restart
	}
	if h.Degraded > h.Restart {
		return Spec{}, fmt.Errorf(
			"health: degraded at %d failures and restart at %d, so the program would be restarted before it was ever degraded",
			h.Degraded, h.Restart)
	}

	b := DefaultBudget()
	if err := durations([]duration{
		{"window", &b.Window, d.Budget.Window},
		{"backoff", &b.Backoff, d.Budget.Backoff},
		{"max_backoff", &b.MaxBackoff, d.Budget.MaxBackoff},
	}); err != nil {
		return Spec{}, err
	}
	if d.Budget.Restarts > 0 {
		b.Restarts = d.Budget.Restarts
	}

	spec := Spec{
		ID: d.ID, Path: d.Path, Args: d.Args, Dir: d.Dir, Env: d.Env,
		Health: h, Budget: b,
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// duration is one declared duration: the field's name in the file, where the
// parsed value goes, and what the person wrote.
type duration struct {
	field string
	into  *time.Duration
	text  string
}

// durations parses every written duration in declaration order, so a file with
// two bad values always names the same one and the error is reproducible.
func durations(in []duration) error {
	for _, d := range in {
		if d.text == "" {
			continue
		}
		v, err := time.ParseDuration(d.text)
		if err != nil {
			return fmt.Errorf("%s: %q is not a duration: %w", d.field, d.text, err)
		}
		if v <= 0 {
			return fmt.Errorf("%s: %q is not a positive duration", d.field, d.text)
		}
		*d.into = v
	}
	return nil
}
