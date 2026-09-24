package supervise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ProgramsFile)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A MISSING FILE IS THE COMMON CASE AND NOT AN ERROR: an estate that declares
// no programs has nothing to supervise, and `rig up` on it says so.
func TestLoadTreatsAMissingFileAsNoPrograms(t *testing.T) {
	specs, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("a missing file was an error: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("got %d programs from a file that is not there", len(specs))
	}
}

func TestLoadReadsADeclarationWithItsOwnNumbers(t *testing.T) {
	path := write(t, `{"programs": [
	  {"id": "fakeapp", "path": "/usr/bin/fakeapp", "args": ["--name", "fakeapp"],
	   "env": ["CARGO_HOME"],
	   "health": {"interval": "2s", "idle": "10s", "register": "15s", "degraded": 2, "restart": 4},
	   "budget": {"restarts": 3, "window": "5m", "backoff": "500ms", "max_backoff": "30s"}}
	]}`)
	specs, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("got %d programs, want 1", len(specs))
	}
	s := specs[0]
	if s.ID != "fakeapp" || s.Path != "/usr/bin/fakeapp" || len(s.Args) != 2 {
		t.Fatalf("spec %+v", s)
	}
	if s.Health.Interval != 2*time.Second || s.Health.Idle != 10*time.Second ||
		s.Health.Register != 15*time.Second || s.Health.Degraded != 2 || s.Health.Restart != 4 {
		t.Errorf("health %+v", s.Health)
	}
	if s.Budget.Restarts != 3 || s.Budget.Window != 5*time.Minute ||
		s.Budget.Backoff != 500*time.Millisecond || s.Budget.MaxBackoff != 30*time.Second {
		t.Errorf("budget %+v", s.Budget)
	}
	// The timeout was not declared, so it is section 18's own default rather
	// than a zero that would mean "no timeout at all".
	if s.Health.Timeout != DefaultHealth().Timeout {
		t.Errorf("an undeclared timeout came out as %s, want the default", s.Health.Timeout)
	}
}

func TestLoadRefusesWhatCannotBeSupervisedHonestly(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"a relative executable",
			`{"programs": [{"id": "a", "path": "bin/app"}]}`,
			"not an absolute path",
		},
		{
			"a path that walks upward",
			`{"programs": [{"id": "a", "path": "/opt/../bin/app"}]}`,
			"walks upward",
		},
		{
			"an id that is a path",
			`{"programs": [{"id": "../a", "path": "/bin/app"}]}`,
			"a key",
		},
		{
			"a duration nobody can parse",
			`{"programs": [{"id": "a", "path": "/bin/app", "health": {"idle": "soon"}}]}`,
			"not a duration",
		},
		{
			"a negative duration",
			`{"programs": [{"id": "a", "path": "/bin/app", "budget": {"backoff": "-1s"}}]}`,
			"not a positive duration",
		},
		{
			"degraded above restart, so it would restart before degrading",
			`{"programs": [{"id": "a", "path": "/bin/app", "health": {"degraded": 9, "restart": 2}}]}`,
			"before it was ever degraded",
		},
		{
			"a field nobody reads, which is how a typo becomes a silent default",
			`{"programs": [{"id": "a", "path": "/bin/app", "helth": {}}]}`,
			"unknown field",
		},
		{"not json at all", `programs: []`, "invalid character"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(write(t, c.body))
			if err == nil {
				t.Fatal("accepted it")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestProgramsPathFollowsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/cfg")
	got, err := ProgramsPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/cfg/rig/programs.json" {
		t.Errorf("path %q", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/someone")
	got, err = ProgramsPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/someone/.config/rig/programs.json" {
		t.Errorf("path %q with no XDG_CONFIG_HOME, want the ~/.config fallback", got)
	}
}
