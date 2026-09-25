package supervise

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestValidateRefusesAnythingThatCouldNotBeLaunchedSafely(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"no id", Spec{Path: "/bin/true"}, "needs an id"},
		{"an id with a separator", Spec{ID: "a/b", Path: "/bin/true"}, "a key"},
		{"an id with a space", Spec{ID: "a b", Path: "/bin/true"}, "a key"},
		{"no path", Spec{ID: "a"}, "no executable"},
		{"a relative path", Spec{ID: "a", Path: "bin/true"}, "not an absolute path"},
		{"a path that walks up", Spec{ID: "a", Path: "/opt/../bin/true"}, "walks upward"},
		{"an env entry with a value in it", Spec{ID: "a", Path: "/bin/true", Env: []string{"A=1"}}, "not an environment variable name"},
		{"an empty env name", Spec{ID: "a", Path: "/bin/true", Env: []string{""}}, "not an environment variable name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.spec.Validate()
			if err == nil {
				t.Fatalf("accepted %+v", c.spec)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q, want it to mention %q", err, c.want)
			}
		})
	}
	if err := (Spec{ID: "a", Path: "/bin/true", Env: []string{"CARGO_HOME"}}).Validate(); err != nil {
		t.Errorf("a valid spec was refused: %v", err)
	}
}

// ⛔ THE ASSERTION THAT MATTERS IS THE NEGATIVE ONE. Section 18 builds the
// child's environment rather than inheriting it, so a test that only checks
// the allowlist SURVIVES would pass against a plain os.Environ.
func TestEnvironmentDropsEverythingNotOnTheAllowlist(t *testing.T) {
	parent := []string{
		"HOME=/home/b", "PATH=/usr/bin", "DISPLAY=:0", "TZ=Asia/Jerusalem",
		"XDG_RUNTIME_DIR=/run/user/1000", "XDG_CONFIG_HOME=/home/b/.config",
		"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/bus", "SSH_AUTH_SOCK=/run/ssh",
		"LANG=en_IL.UTF-8", "LC_ALL=C", "WAYLAND_DISPLAY=wayland-0", "XAUTHORITY=/home/b/.Xauth",
		"RIG_WIRE=1",
		// None of these may reach a child.
		"AWS_SECRET_ACCESS_KEY=hunter2", "GITHUB_TOKEN=ghp_x", "EDITOR=vim",
		"SUDO_ASKPASS=/usr/bin/x", "LD_PRELOAD=/tmp/evil.so",
	}
	got := Environment(Spec{ID: "a", Path: "/bin/true", Env: []string{"EDITOR"}}, parent,
		map[string]string{"RIG_SOCKET": "/run/rig.sock", "RIG_PROGRAM_ID": "a"})

	names := map[string]string{}
	for _, kv := range got {
		name, value, _ := strings.Cut(kv, "=")
		names[name] = value
	}
	for _, want := range []string{
		"HOME", "PATH", "DISPLAY", "TZ", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME",
		"DBUS_SESSION_BUS_ADDRESS", "SSH_AUTH_SOCK", "LANG", "LC_ALL",
		"WAYLAND_DISPLAY", "XAUTHORITY", "RIG_WIRE",
	} {
		if _, ok := names[want]; !ok {
			t.Errorf("%s was dropped, and section 18's table says what breaks without it", want)
		}
	}
	for _, banned := range []string{"AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "SUDO_ASKPASS", "LD_PRELOAD"} {
		if _, ok := names[banned]; ok {
			t.Errorf("%s reached the child: the environment is BUILT, not inherited", banned)
		}
	}
	// A declared extra is the section's own escape hatch, and it is the only one.
	if names["EDITOR"] != "vim" {
		t.Error("a declared extra variable did not reach the child")
	}
	if names["RIG_SOCKET"] != "/run/rig.sock" || names["RIG_PROGRAM_ID"] != "a" {
		t.Error("rig's own handles did not reach the child")
	}
}

// A handle rig sets must beat an inherited variable of the same name: the
// child's socket is the one rig is serving, never whatever the parent carried.
func TestRigsOwnHandlesBeatAnInheritedValue(t *testing.T) {
	got := Environment(Spec{ID: "a", Path: "/bin/true"},
		[]string{"RIG_SOCKET=/run/stale.sock", "HOME=/home/b"},
		map[string]string{"RIG_SOCKET": "/run/live.sock"})
	if !slices.Contains(got, "RIG_SOCKET=/run/live.sock") {
		t.Errorf("environment %v, want the live socket", got)
	}
	if slices.Contains(got, "RIG_SOCKET=/run/stale.sock") {
		t.Error("the stale socket survived: a child would dial the wrong daemon")
	}
}

func TestExitSaysWhetherItWasASignal(t *testing.T) {
	if got := (Exit{Code: 0}).String(); got != "exit 0" {
		t.Errorf("String() = %q", got)
	}
	if got := (Exit{Code: -1, Signal: "killed"}).String(); got != "killed by killed" {
		t.Errorf("String() = %q", got)
	}
	if !(Exit{}).OK() {
		t.Error("a clean exit did not read as OK")
	}
	if (Exit{Code: 0, Signal: "terminated"}).OK() {
		t.Error("a signalled exit read as OK: a killed process did not finish")
	}
}

func TestStartRefusesABadSpecAndAMissingExecutable(t *testing.T) {
	if _, _, err := Start(Spec{ID: "a", Path: "relative"}, nil); err == nil {
		t.Error("Start accepted a relative path")
	}
	missing := filepath.Join(t.TempDir(), "nothing")
	if _, _, err := Start(Spec{ID: "a", Path: missing}, nil); err == nil {
		t.Error("Start accepted an executable that is not there")
	}
}

// The real launcher, once, against a script: it runs, it reports its exit, and
// the environment it saw is the built one.
func TestStartRunsAChildAndReportsHowItEnded(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")
	script := filepath.Join(dir, "child.sh")
	body := "#!/bin/sh\nenv > " + out + "\nexit 3\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPERVISE_TEST_SECRET", "must-not-travel")

	_, exited, err := Start(Spec{ID: "child", Path: script}, map[string]string{"RIG_PROGRAM_ID": "child"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case e := <-exited:
		if e.Code != 3 {
			t.Errorf("exit %+v, want code 3", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the child never ended")
	}
	seen, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the child wrote no environment: %v", err)
	}
	if strings.Contains(string(seen), "SUPERVISE_TEST_SECRET") {
		t.Error("the parent's own variables reached a supervised child")
	}
	if !strings.Contains(string(seen), "RIG_PROGRAM_ID=child") {
		t.Error("the child was not told which program it is")
	}
}

// Stop is SIGTERM then SIGKILL, and the SIGKILL half is what this checks: a
// child that ignores SIGTERM still leaves.
func TestStopKillsAChildThatIgnoresSIGTERM(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "stubborn.sh")
	body := "#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 0.05; done\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	proc, exited, err := Start(Spec{ID: "stubborn", Path: script}, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if proc.PID() <= 0 {
		t.Fatal("a started child has no pid")
	}
	proc.Stop(200 * time.Millisecond)
	select {
	case e := <-exited:
		if e.Signal == "" {
			t.Errorf("exit %+v, want it killed by a signal", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a child that ignores SIGTERM was never killed")
	}
}

// StopAll returns only once its children are gone, a stubborn one included:
// rigd exits straight after it, and a SIGKILL still pending in a goroutine
// dies with rigd and leaves the child running with nobody to kill it.
func TestStopAllLeavesNoChildBehind(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "stubborn.sh")
	body := "#!/bin/sh\ntrap '' TERM\nwhile true; do sleep 0.05; done\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	sup := New(Options{StopGrace: 200 * time.Millisecond})
	if err := sup.Declare([]Spec{{ID: "stubborn", Path: script}}); err != nil {
		t.Fatal(err)
	}
	st, err := sup.Up("stubborn")
	if err != nil || st[0].PID <= 0 {
		t.Fatalf("up: %v %+v", err, st)
	}
	pid := st[0].PID
	sup.StopAll()
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("pid %d is still there after StopAll returned (kill 0: %v)", pid, err)
	}
}
