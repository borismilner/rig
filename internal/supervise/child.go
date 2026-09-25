package supervise

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultStopGrace is how long a child gets after SIGTERM before SIGKILL.
//
// PLAN.md section 18, Shutdown: "the lifecycle notice first, then SIGTERM,
// grace period, then SIGKILL." Three seconds is what cmd/rigwindow measured
// and has used since it was written; this package is where that number now
// lives, so the tray and the daemon cannot drift apart on it.
const DefaultStopGrace = 3 * time.Second

// Spec is one program rig may start. It is what the declared-program file
// deserialises into, and what the tray hands in for its own child.
type Spec struct {
	// ID is the program id it registers under. It is a KEY, never a path:
	// nothing in this package joins it to a directory.
	ID string

	// Path is the executable. Absolute, and checked before a launch.
	Path string

	// Args are the arguments after argv[0].
	Args []string

	// Dir is the working directory. Empty means the daemon's own.
	Dir string

	// Env is the EXTRA environment names this program is allowed to inherit,
	// on top of the allowlist in PLAN.md section 18. Section 18: "the
	// allowlist is config ... so a program needing one more variable declares
	// it rather than getting the parent's whole environment back."
	Env []string

	// Health is how this program's health is judged.
	Health HealthPolicy

	// Budget is its restart budget.
	Budget Budget
}

// Validate refuses a spec that could not be launched safely, before anything
// is started. Section 38's craft rule: no caller path joined unvalidated.
func (s Spec) Validate() error {
	switch {
	case s.ID == "":
		return errors.New("a supervised program needs an id")
	case strings.ContainsAny(s.ID, "/\\ \t\n"):
		return fmt.Errorf("program id %q: an id is a key, so it may not contain a path separator or a space", s.ID)
	case s.Path == "":
		return fmt.Errorf("program %q: no executable declared", s.ID)
	case !strings.HasPrefix(s.Path, "/"):
		// Relative would resolve against whatever directory rigd happens to
		// be in, which is not a thing a declaration should depend on, and
		// PATH lookup would let a directory earlier in PATH decide what rig
		// starts.
		return fmt.Errorf("program %q: %q is not an absolute path", s.ID, s.Path)
	case strings.Contains(s.Path, ".."):
		return fmt.Errorf("program %q: %q walks upward, which a declared path never needs to", s.ID, s.Path)
	}
	for _, name := range s.Env {
		if name == "" || strings.ContainsAny(name, "=\x00") {
			return fmt.Errorf("program %q: %q is not an environment variable name", s.ID, name)
		}
	}
	return nil
}

// inherited is PLAN.md section 18's environment allowlist, exactly.
//
// The section's own table says what breaks without each, and the reason the
// list exists at all: "rig builds the child's environment rather than
// inheriting its own ... a constructed environment silently governs the
// estate and an empty one breaks four subsystems."
//
// ⛔ NO GRANT IS HERE AND NONE CAN BE, which is also section 18: "no grant
// travels in it, because no grant exists as a variable" - introspect is
// decided from the connection (section 14). There is nothing to scrub.
var inherited = []string{
	// go-keyring reaches the secret store, and section 12's D-Bus toast
	// fallback stays alive.
	"DBUS_SESSION_BUS_ADDRESS",
	// Every needs_display command, including section 5e's own worked example.
	"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY",
	// A program that shells out to git over ssh, instead of hanging on a
	// passphrase nobody can answer.
	"SSH_AUTH_SOCK",
	// Timestamps and collation that match every surface that renders them.
	"TZ", "LANG", "LC_ALL",
	// Without these nothing resolves: a program cannot find its own socket,
	// config or cache.
	"HOME", "PATH", "XDG_RUNTIME_DIR",
}

// inheritedPrefixes are the wildcard rows of the same table: XDG_*_HOME, and
// RIG_* for the program's own registration handles.
var inheritedPrefixes = []string{"XDG_", "RIG_"}

// Environment builds the child's environment from the parent's, section 18's
// allowlist, the program's own declared extras, and the handles rig sets.
//
// It takes the parent environment rather than reading os.Environ so the test
// can state the whole input, which is the only way to assert that everything
// else is DROPPED rather than merely that the allowlist survives.
func Environment(spec Spec, parent []string, handles map[string]string) []string {
	allow := make(map[string]bool, len(inherited)+len(spec.Env))
	for _, n := range inherited {
		allow[n] = true
	}
	for _, n := range spec.Env {
		allow[n] = true
	}

	out := make([]string, 0, len(allow)+len(handles))
	seen := make(map[string]bool, len(allow)+len(handles))
	for _, kv := range parent {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || seen[name] {
			continue
		}
		if !allow[name] && !hasAnyPrefix(name, inheritedPrefixes) {
			continue
		}
		// A handle rig sets below wins over an inherited one of the same
		// name: the child's RIG_SOCKET must be the socket rig is serving,
		// never whatever the parent happened to carry.
		if _, set := handles[name]; set {
			continue
		}
		seen[name] = true
		out = append(out, kv)
	}
	for name, value := range handles {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name+"="+value)
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// Exit is how a child ended, in the form every surface needs.
//
// PLAN.md section 23's M6 demo asks for process.exited "with its code where
// rig is the parent and the literal unknown where it is not". rig is always
// the parent here, so Code is always real; Signal carries the other half,
// because a program killed by SIGKILL has no exit code and reporting 0 or -1
// would be a lie either way.
type Exit struct {
	Code   int
	Signal string
	At     time.Time
}

// String renders an exit the way the history shows it.
func (e Exit) String() string {
	if e.Signal != "" {
		return "killed by " + e.Signal
	}
	return fmt.Sprintf("exit %d", e.Code)
}

// OK reports whether this was a clean exit.
func (e Exit) OK() bool { return e.Code == 0 && e.Signal == "" }

// Process is one running child. The supervisor can ask it to go, and read how
// it went from the channel Start returned.
//
// The interface exists so a test can supervise something that is not a
// process at all - the state machine above is the part worth testing without
// a fork per case.
type Process interface {
	// Stop asks the child to leave: SIGTERM, then SIGKILL after the grace.
	Stop(grace time.Duration)

	// PID is the child's process id, for the history and for a human who
	// wants to look at it. Zero if it never started.
	PID() int
}

// Starter launches one child and reports how it ended.
//
// exited closes when the process has ended, however it ended, and the Exit is
// readable from the returned Process's owner through Wait below.
type Starter func(spec Spec, handles map[string]string) (proc Process, exited <-chan Exit, err error)

// Start is the real Starter: it launches the executable the spec names, with
// the environment section 18 specifies and nothing else.
func Start(spec Spec, handles map[string]string) (Process, <-chan Exit, error) {
	if err := spec.Validate(); err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(spec.Path); err != nil {
		return nil, nil, fmt.Errorf("program %q: %w", spec.ID, err)
	}
	//rig:allow nocontextfree: a supervised program runs until it exits or the supervisor stops it, so its end is the exit channel rather than a deadline
	cmd := exec.CommandContext(context.Background(), spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = Environment(spec, os.Environ(), handles)
	// The child's output is rig's output until section 8's ingest exists.
	// Section 5g: "rig collects nothing it did not see", and the journal is
	// where a crashed program's last words are read today.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	proc, exited, err := StartCommand(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("starting %q: %w", spec.ID, err)
	}
	return proc, exited, nil
}

// StartCommand is the mechanism under Start with none of its policy: it
// starts a command the caller built, reaps it, and reports its exit, and the
// Process it returns stops it SIGTERM-then-SIGKILL.
//
// ⛔ IT EXISTS FOR THE TRAY, WHOSE CHILD IS NOT A SUPERVISED PROGRAM. The
// window is the user's own GUI in the user's session and inherits the whole
// session environment: section 18's allowlist would drop XMODIFIERS (the input
// method, so no Hebrew in the window) and GTK_MODULES, measured on the live
// unit 2026-09-25. A program rig supervises always goes through Start.
func StartCommand(cmd *exec.Cmd) (Process, <-chan Exit, error) {
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	exited := make(chan Exit, 1)
	c := &childProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		close(c.done)
		exited <- exitOf(err)
		close(exited)
	}()
	return c, exited, nil
}

// exitOf turns what exec.Wait said into the Exit every surface reads.
func exitOf(err error) Exit {
	e := Exit{At: time.Now()}
	if err == nil {
		return e
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		// Not an exit status at all: the wait itself failed. Say so rather
		// than inventing a code.
		e.Code = -1
		e.Signal = "unknown: " + err.Error()
		return e
	}
	e.Code = ee.ExitCode()
	//nolint:misspell // Signaled is the stdlib spelling of syscall.WaitStatus.
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		e.Signal = ws.Signal().String()
	}
	return e
}

type childProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
}

// Ended closes once the child has been reaped.
func (c *childProcess) Ended() <-chan struct{} { return c.done }

func (c *childProcess) PID() int {
	if c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// Stop is SIGTERM, then SIGKILL after the grace (section 18, Shutdown).
//
// ⛔ IT SIGNALS ONE PID, THE ONE RIG FORKED. Section 18: "Programs rig did not
// start are never killed", and the narrowest reading of that is also the
// safest implementation - no process group, no name matching, no scan of the
// process table. A program that leaves its own children behind is its own
// defect and rig does not clean it up by killing a group it does not own.
func (c *childProcess) Stop(grace time.Duration) {
	if c.cmd.Process == nil {
		return
	}
	c.once.Do(func() {
		_ = c.cmd.Process.Signal(syscall.SIGTERM)
		go func() {
			select {
			case <-c.done:
			case <-time.After(grace):
				_ = c.cmd.Process.Kill()
			}
		}()
	})
}
