// Package paths resolves rig's runtime locations.
//
// One socket for the whole daemon at $XDG_RUNTIME_DIR/rig/rigd.sock, mode 0600,
// and the pidfile beside it (PLAN.md section 5f). There is deliberately no
// per-program path: a socket named after a program turns the runtime directory
// into a second copy of the registry that ls enumerates and stat polls for
// presence, and a mode bit is a uid instrument being asked to enforce a client
// boundary (section 14).
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MaxSocketPath is the longest usable unix socket path.
//
// Linux's sockaddr_un.sun_path is 108 bytes including the terminating NUL, so
// 107 is the real ceiling. Exceeding it fails bind and connect with EINVAL -
// "invalid argument" - which names neither the path nor the limit, so the
// check is here and the error says what is wrong.
const MaxSocketPath = 107

// RuntimeDir is $XDG_RUNTIME_DIR/rig.
//
// XDG_RUNTIME_DIR is required, not defaulted. The spec says it is owned by the
// user, mode 0700 and cleared at logout; /tmp guarantees none of that, and a
// silent fallback would put the socket somewhere with different security
// properties than the ones section 14 reasons about.
func RuntimeDir() (string, error) {
	d := os.Getenv("XDG_RUNTIME_DIR")
	if d == "" {
		return "", errors.New("paths: XDG_RUNTIME_DIR is not set; rig will not " +
			"guess a runtime directory, because the socket's security depends on its mode and owner")
	}
	return filepath.Join(d, "rig"), nil
}

// Socket is the daemon's own socket path, which every ordinary client speaks.
func Socket() (string, error) { return socketNamed("rigd.sock") }

// MCPSocket is the MCP surface's own socket (PLAN.md sections 9, 13a, 14).
//
// A SECOND SOCKET, AND SECTION 14'S CALLER TABLE IS THE REASON RATHER THAN
// CONVENIENCE. The alternatives were ruled out on mechanism, not preference:
// stdio is one process per agent and rigd is a singleton, and folding MCP into
// the HTTP surface merges two rows the table exists to keep apart - an HTTP
// client gets "its bearer principal's scopes, never introspect", while MCP
// connecting as an unregistered socket client gets everything. A second socket
// is the only shape that keeps MCP a distinct caller row.
//
// It reaches exactly the set of processes the first socket reaches: same 0600
// mode, same 0700 runtime directory, same uid. So it widens nothing, and the
// isolation argument section 14 makes over the first socket is the same
// argument here.
func MCPSocket() (string, error) { return socketNamed("rigd-mcp.sock") }

// socketNamed joins a socket name to the runtime directory and refuses a path
// the kernel would reject.
//
// The length check is here rather than at each call site because the kernel
// reports this as a bare EINVAL, naming neither the path nor the limit, and a
// second socket is a second chance to spend an afternoon on it.
func socketNamed(name string) (string, error) {
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(d, name)
	if len(p) > MaxSocketPath {
		return "", fmt.Errorf(
			"paths: socket path is %d bytes, over the %d-byte kernel limit: %s\n"+
				"       set XDG_RUNTIME_DIR to something shorter; the kernel reports "+
				"this only as EINVAL, which names neither the path nor the limit",
			len(p), MaxSocketPath, p)
	}
	return p, nil
}

// PIDFile is the single-instance lock path (section 5f).
func PIDFile() (string, error) {
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "rigd.pid"), nil
}

// MaxEstateNameLen bounds an estate name, because the name becomes a path
// component. It is generous: section 37 allows two names and both are words.
const MaxEstateNameLen = 64

// ValidEstateName refuses anything that would not be a safe single path
// component.
//
// The name is joined into a filename, so this is a correctness requirement and
// not a style rule: a name containing a separator, a "..", a NUL or a space is
// a claim on a path the caller did not mean to make. Lowercase letters, digits
// and hyphens, starting with a letter, is total and leaves nothing to reason
// about.
//
// IT DELIBERATELY DOES NOT ENFORCE SECTION 37'S TWO NAMES. "At most two named
// estates, production and development" is that section's rule, and hardcoding
// the pair here would be writing an unreviewed policy into the mechanism that
// enforces a different precondition. Precondition 6 is "a named estate refuses
// a name already held"; the enum is not it.
func ValidEstateName(name string) error {
	if name == "" {
		return errors.New("paths: an estate name may not be empty; an estate " +
			"started without a name claims no name and collides with nothing")
	}
	if len(name) > MaxEstateNameLen {
		return fmt.Errorf("paths: estate name is %d bytes, over the %d-byte limit: %q",
			len(name), MaxEstateNameLen, name)
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9', r == '-':
			if i == 0 {
				return fmt.Errorf("paths: estate name %q must start with a lowercase letter", name)
			}
		default:
			return fmt.Errorf("paths: estate name %q may use lowercase letters, "+
				"digits and hyphens only; it becomes a path component", name)
		}
	}
	return nil
}

// StateDir is $XDG_STATE_HOME/rig, and it is deliberately NOT under
// XDG_RUNTIME_DIR.
//
// THIS IS THE WHOLE MECHANISM OF THE NAMED-ESTATE CLAIM, so the reasoning is
// here rather than at the call site. Two estates differ exactly in their
// runtime directory (sections 5f and 37), so a name claim placed beside the
// socket would collide with NOTHING: production and development would each take
// an uncontested claim inside their own tree and both would call themselves
// production. A claim only refuses a duplicate if both estates can see it, and
// the one place the runtime directory does not reach is outside the runtime
// directory.
//
// DEFAULTING THIS IS NOT THE ACT RuntimeDir REFUSES TO PERFORM, and the
// difference is in the spec rather than in taste. The XDG basedir spec defines
// a default for XDG_STATE_HOME ($HOME/.local/state) and defines NONE for
// XDG_RUNTIME_DIR - which is why RuntimeDir refuses to guess and this does not.
// Nothing here is a socket, so no client boundary rides on its mode (section
// 14): it is a lock file whose only requirement is that it is the user's own.
//
// A claim surviving a reboot is harmless and deliberately not swept. The lock
// lives on the open descriptor, so it is released by the process dying; the
// file's contents are never trusted, exactly as the pidfile's are not.
func StateDir() (string, error) {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "rig"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("paths: XDG_STATE_HOME is unset and the home "+
			"directory could not be resolved, so the estate-name claim has "+
			"nowhere to live: %w", err)
	}
	return filepath.Join(home, ".local", "state", "rig"), nil
}

// EstateLock is the claim path for a NAMED estate (section 37, precondition 6).
//
// An estate started without a name never calls this and claims nothing, which
// is section 37's ephemeral-estates clause working by construction rather than
// by exemption: every test in this repository starts an unnamed estate in a
// temporary runtime directory and none of them touch this path.
func EstateLock(name string) (string, error) {
	if err := ValidEstateName(name); err != nil {
		return "", err
	}
	d, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "estates", name+".pid"), nil
}

// EstateStateDir is the PERSISTENT STATE subtree for a NAMED estate (section
// 37, precondition 2): $XDG_STATE_HOME/rig/estates/<name>/.
//
// THE SHARED ROOT KEEPS EXACTLY ONE TENANT, AND THAT IS A RULE RATHER THAN AN
// OBSERVATION. StateDir's own comment above says what the root is for: a name
// claim refuses a duplicate only if both estates can see it, and the one place
// the runtime directory does not reach is outside the runtime directory.
// Anything placed at that root is therefore visible to BOTH estates by
// construction - which is precisely the property the claim needs and precisely
// the property everything else must not have. So the claim keeps the root and
// every other persistent thing rig holds goes under this function's result.
//
// The subtree is not invented here. paths.EstateLock is already
// $XDG_STATE_HOME/rig/estates/<name>.pid, so `estates` is the established
// idiom and the estate name is the established key; this extends both rather
// than opening a second tree beside them. The claim file and the state
// directory are SIBLINGS with the same stem, `production.pid` beside
// `production/`, and they cannot collide because ValidEstateName refuses a
// name containing a dot.
//
// AN UNNAMED ESTATE GETS NO PERSISTENT STATE AT ALL, AND THAT IS THE ANSWER
// RATHER THAN AN OMISSION. Its name is the empty string, ValidEstateName
// refuses the empty string, and so there is no subtree to key it to. This
// falls out of section 37's ephemeral-estates clause rather than contradicting
// it: an ephemeral estate that persisted across runs would be a third estate
// arriving by the back door. Every test in this repository starts an unnamed
// estate in a temporary runtime directory, so the refusal is what keeps them
// working - a test that needs state gives itself a name, or does without.
//
// It resolves a path and creates nothing, exactly as every other function
// here does. The mkdir belongs to whoever is about to write, under the lock it
// took, which is the ordering instance.Acquire already establishes.
// EstateScratchDir is the EPHEMERAL record subtree for an UNNAMED estate:
// $XDG_RUNTIME_DIR/rig/record/.
//
// ⛔ IT EXISTS BECAUSE AN AGENT HAD NOWHERE TO WRITE, AND THAT WAS MEASURED
// RATHER THAN SUPPOSED. Section 09's A0 survey, 2026-09-17, `[ran it]`: a third
// estate NAME is refused by rigd; an unnamed estate starts and had no record
// store at all; and a second daemon on production or development is B72. Every
// door closed, so every write an agent made landed in one of the two estates on
// the human's screen. ⛔ **FOUR LEAD GENERATIONS WROTE NOTHING FOR THAT SURVEY,
// AND THE REASON WAS NOT RELUCTANCE - THE INSTRUMENT DID NOT EXIST.** A seat
// told to measure the record verbs had to choose between not measuring them and
// writing into production.
//
// ⛔ IT DOES NOT WEAKEN EstateStateDir's RULE, IT KEYS ON A DIFFERENT THING.
// That function's rule is that PERSISTENT state needs a NAME, and the reason is
// exact: an ephemeral estate that persisted across runs would be a third estate
// arriving by the back door. This keys on the RUNTIME DIRECTORY instead, which
// is already the machine-wide singleton for the socket, the pidfile and the
// flock - so it extends the established key rather than opening a second tree,
// and it cannot outlive the runtime directory the operating system clears.
// Ephemeral state in an ephemeral place; persistent state still needs a name.
//
// ⛔ AND THE PRODUCT MUST SAY WHICH ONE A CALLER IS TALKING TO. A scratch
// store that looked durable would be worse than none: an agent would write its
// working notes into something that vanishes and be told, correctly, that the
// write succeeded. Store.Ephemeral carries it and the surfaces render it.
//
// It resolves a path and creates nothing, exactly as every other function here.
func EstateScratchDir() (string, error) {
	d, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "rig", "record"), nil
}

func EstateStateDir(name string) (string, error) {
	if err := ValidEstateName(name); err != nil {
		return "", fmt.Errorf("paths: an estate has no persistent state until "+
			"it is named, and this name cannot be used: %w", err)
	}
	d, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "estates", name), nil
}
