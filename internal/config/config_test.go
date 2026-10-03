package config

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func rig(t *testing.T) *Schema {
	t.Helper()
	s, err := RigSchema()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rig.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRigsSchemaDeclaresItsKeys(t *testing.T) {
	var names []string
	for _, k := range rig(t).Keys() {
		names = append(names, k.Name+" "+k.Env+" "+k.Flag+" "+k.Apply)
	}
	want := []string{
		"dashboard.notifications.keep.days RIG_DASHBOARD_NOTIFICATIONS_KEEP_DAYS dashboard-notifications-keep-days live",
		"display.name RIG_DISPLAY_NAME display-name live", "log.level RIG_LOG_LEVEL log-level live",
		"logs.archive.days RIG_LOGS_ARCHIVE_DAYS logs-archive-days restart",
		"logs.buffer.bytes RIG_LOGS_BUFFER_BYTES logs-buffer-bytes restart",
		"logs.call.payload.cap RIG_LOGS_CALL_PAYLOAD_CAP logs-call-payload-cap restart",
		"logs.delete.days RIG_LOGS_DELETE_DAYS logs-delete-days restart",
		"logs.flush.bytes RIG_LOGS_FLUSH_BYTES logs-flush-bytes restart",
		"logs.flush.ms RIG_LOGS_FLUSH_MS logs-flush-ms restart",
		"logs.rate.burst RIG_LOGS_RATE_BURST logs-rate-burst restart",
		"logs.rate.records RIG_LOGS_RATE_RECORDS logs-rate-records restart",
		"logs.retention.bytes RIG_LOGS_RETENTION_BYTES logs-retention-bytes restart",
		"programs.scan RIG_PROGRAMS_SCAN programs-scan restart",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("keys %q", names)
	}
}

// Every adjacent pair of populated layers: the higher wins and the lower is
// recorded as a loser, in order.
func TestEachLayerBeatsTheOneBelowIt(t *testing.T) {
	sys := write(t, `log.level = "error"`)
	usr := write(t, "[log]\nlevel = \"warn\"\n")
	src := Sources{
		SystemFile: sys, UserFile: usr,
		Environ: []string{"RIG_LOG_LEVEL=info"},
		Flags:   map[string]string{"log.level": "debug"},
	}
	stack := []Source{
		{"default", "", "info"},
		{"system", sys, "error"},
		{"user", usr, "warn"},
		{"env", "", "info"},
		{"flag", "", "debug"},
		{"runtime", "", "error"},
	}
	// Strip layers from the top down: each time the next one down wins.
	strip := []func(*Sources){
		func(s *Sources) { s.SystemFile = "" },
		func(s *Sources) { s.UserFile = "" },
		func(s *Sources) { s.Environ = nil },
		func(s *Sources) { s.Flags = nil },
	}
	for top := len(stack) - 1; top >= 0; top-- {
		cut := src
		for i := top; i < len(strip); i++ {
			strip[i](&cut)
		}
		r := Load(rig(t), cut)
		if top == len(stack)-1 {
			if _, _, err := r.Set(map[string]string{"log.level": `"error"`}); err != nil {
				t.Fatal(err)
			}
		}
		got := r.Get("log.level")[0]
		if got.Winner != stack[top] {
			t.Errorf("top layer %s: winner %+v", stack[top].Layer, got.Winner)
		}
		if !slices.Equal(got.Losers, stack[:top]) {
			t.Errorf("top layer %s: losers %+v, want %+v", stack[top].Layer, got.Losers, stack[:top])
		}
	}
}

// A change set with one bad value applies nothing, and says which key, which
// layer and why.
func TestAChangeSetIsRefusedWhole(t *testing.T) {
	r := Load(rig(t), Sources{})
	_, _, err := r.Set(map[string]string{"log.level": `"loud"`, "display.name": `"x"`})
	var ref *RefusalError
	if !errors.As(err, &ref) || ref.Key != "log.level" || ref.Layer != "runtime" || !strings.Contains(ref.Reason, "loud") {
		t.Fatalf("refused with %v", err)
	}
	if r.String("display.name") != "" || r.String("log.level") != "info" {
		t.Fatal("a refused change set moved a key")
	}
	_, _, err = r.Set(map[string]string{"log.levle": `"warn"`})
	if !errors.As(err, &ref) || ref.Key != "log.levle" {
		t.Fatalf("an unknown key: %v", err)
	}
	out, moved, err := r.Set(map[string]string{"log.level": `"warn"`, "display.name": `"B"`})
	if err != nil || out["log.level"] != "applied" || !slices.Equal(moved, []string{"display.name", "log.level"}) {
		t.Fatalf("set: %v %v %v", out, moved, err)
	}
	if _, moved, _ := r.Set(map[string]string{"log.level": `"warn"`}); len(moved) != 0 {
		t.Fatalf("setting the same value again moved %v", moved)
	}
}

// An unknown key is an orphan and the file still loads; a bad value is a
// problem and takes no part; neither stops rigd.
func TestOrphansAndProblemsAreReportedNotFatal(t *testing.T) {
	usr := write(t, "log.levle = \"warn\"\ndisplay.name = \"B\"\nlog.level = \"loud\"\n")
	r := Load(rig(t), Sources{UserFile: usr, Environ: []string{
		"RIG_DISPLAY_NAM=x", "RIG_ROOT=/tmp/x", "PATH=/bin", "RIG_DISPLAY_NAME=from-env",
	}})
	if got := r.Orphans(); !slices.Equal(got, []string{"env::RIG_DISPLAY_NAM", "user:" + usr + ":log.levle"}) {
		t.Fatalf("orphans %q", got)
	}
	if p := r.Problems(); len(p) != 1 || !strings.Contains(p[0], "log.level") || !strings.Contains(p[0], "loud") {
		t.Fatalf("problems %q", p)
	}
	if r.String("log.level") != "info" || r.String("display.name") != "from-env" {
		t.Fatalf("resolved %q %q", r.String("log.level"), r.String("display.name"))
	}
	broken := Load(rig(t), Sources{UserFile: write(t, "log.level = ")})
	if p := broken.Problems(); len(p) != 1 || !strings.Contains(p[0], "not TOML") {
		t.Fatalf("a broken file: %q", p)
	}
}

func TestFlagsAreSpelledFromTheSchema(t *testing.T) {
	fs := flag.NewFlagSet("rigd", flag.ContinueOnError)
	given := rig(t).Flags(fs)
	if err := fs.Parse([]string{"--log-level=warn"}); err != nil {
		t.Fatal(err)
	}
	if got := given(); len(got) != 1 || got["log.level"] != "warn" {
		t.Fatalf("given %v", got)
	}
}

func TestTheSnapshotIsTheResolvedDocument(t *testing.T) {
	r := Load(rig(t), Sources{Environ: []string{"RIG_DISPLAY_NAME=B \"q\""}})
	p := filepath.Join(t.TempDir(), "config", "resolved.toml")
	if err := r.WriteSnapshot(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# display.name from env\ndisplay.name = \"B \\\"q\\\"\"", "log.level = \"info\""} {
		if !strings.Contains(string(b), want) {
			t.Errorf("snapshot lacks %q:\n%s", want, b)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".resolved-*")); len(left) != 0 {
		t.Fatalf("a temporary file was left: %v", left)
	}
}

// Every RIG_* name in the tree is a key's spelling or listed as not a
// setting, so the orphan check cannot cry wolf over rig's own plumbing.
func TestEveryRigVariableInTheTreeIsAKeyOrListed(t *testing.T) {
	keys := map[string]bool{}
	for _, k := range rig(t).Keys() {
		keys[k.Env] = true
	}
	re := regexp.MustCompile(`"(RIG_[A-Z][A-Z_]*)"`)
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "frontend") {
			if err == nil {
				return filepath.SkipDir
			}
			return err
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if !keys[m[1]] && !NotSettings[m[1]] {
				t.Errorf("%s reads %s, which is neither a key nor in NotSettings", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntReadsAnIntegerKeyFromEveryLayer(t *testing.T) {
	s := MustRigSchema()
	if got := Load(s, Sources{}).Int("logs.flush.ms"); got != 250 {
		t.Fatalf("default = %d, want 250", got)
	}
	r := Load(s, Sources{Environ: []string{"RIG_LOGS_FLUSH_MS=40"}})
	if got := r.Int("logs.flush.ms"); got != 40 {
		t.Fatalf("env = %d, want 40; problems %v", got, r.Problems())
	}
	r = Load(s, Sources{Environ: []string{"RIG_LOGS_FLUSH_MS=5"}})
	if got := r.Int("logs.flush.ms"); got != 250 || len(r.Problems()) != 1 {
		t.Fatalf("an out-of-range value = %d with problems %v, want the default and one problem", got, r.Problems())
	}
	if got := r.Int("log.level"); got != 0 {
		t.Fatalf("a string key read as an int = %d", got)
	}
}
