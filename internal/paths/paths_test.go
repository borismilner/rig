package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeDirIsRequiredNotGuessed(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if _, err := RuntimeDir(); err == nil {
		t.Fatal("an unset XDG_RUNTIME_DIR was accepted; rig must not guess a runtime directory")
	}
}

// The kernel reports an over-long socket path only as EINVAL, which names
// neither the path nor the limit. This was found by running the daemon under a
// long scratch directory, not by reading the code.
func TestOverLongSocketPathIsNamed(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/"+strings.Repeat("x", 200))
	_, err := Socket()
	if err == nil {
		t.Fatal("a socket path over the kernel limit was accepted")
	}
	for _, want := range []string{"over the", "kernel limit", "XDG_RUNTIME_DIR"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestSocketAndPIDFileSitTogether(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	s, err := Socket()
	if err != nil {
		t.Fatal(err)
	}
	p, err := PIDFile()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/run/user/1000/rig/rigd.sock"; s != want {
		t.Fatalf("socket is %s, want %s", s, want)
	}
	if want := "/run/user/1000/rig/rigd.pid"; p != want {
		t.Fatalf("pidfile is %s, want %s", p, want)
	}
}

// THE CRUX OF SECTION 37 PRECONDITION 6, AND THE ONE TEST THAT WOULD HAVE
// CAUGHT THE OBVIOUS WRONG IMPLEMENTATION.
//
// Two estates differ EXACTLY in their runtime directory. A name claim that
// moves with XDG_RUNTIME_DIR refuses nothing: production and development would
// each take an uncontested claim inside their own tree, both would call
// themselves production, and the mechanism would look implemented while
// enforcing nothing at all.
func TestTheEstateClaimDoesNotMoveWithTheRuntimeDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")

	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	first, err := EstateLock("production")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/a-second-estate")
	second, err := EstateLock("production")
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatalf("the claim for one name resolved to two paths:\n  %s\n  %s\n"+
			"two estates differ exactly in XDG_RUNTIME_DIR, so a claim that "+
			"moves with it can never refuse a duplicate name", first, second)
	}
	if strings.Contains(first, "/run/user/1000") || strings.Contains(first, "a-second-estate") {
		t.Fatalf("the claim %s sits under a runtime directory", first)
	}
}

func TestTwoEstateNamesDoNotShareAClaim(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	prod, err := EstateLock("production")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := EstateLock("development")
	if err != nil {
		t.Fatal(err)
	}
	if prod == dev {
		t.Fatalf("production and development share the claim %s", prod)
	}
}

// The name becomes a path component, so this is a correctness requirement and
// not a style rule.
func TestAnEstateNameCannotEscapeItsDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	for _, name := range []string{
		"", "..", ".", "../../etc/passwd", "a/b", "/absolute",
		"Production", "has space", "x\x00y", "-leading", "9leading",
		strings.Repeat("a", MaxEstateNameLen+1),
	} {
		p, err := EstateLock(name)
		if err == nil {
			t.Errorf("estate name %q was accepted and resolved to %s", name, p)
		}
	}
	for _, name := range []string{"production", "development", "dev-2", "a"} {
		if _, err := EstateLock(name); err != nil {
			t.Errorf("estate name %q was refused: %v", name, err)
		}
	}
}

func TestStateHomeIsHonouredAndDefaultedBySpec(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/somewhere/state")
	d, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/somewhere/state/rig"; d != want {
		t.Fatalf("StateDir is %s, want %s", d, want)
	}

	// Unlike XDG_RUNTIME_DIR, the basedir spec DEFINES a default for this one,
	// so deriving it is not the act RuntimeDir refuses to perform.
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	d, err = StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/home/u/.local/state/rig"; d != want {
		t.Fatalf("defaulted StateDir is %s, want %s", d, want)
	}
}

// SECTION 37 PRECONDITION 2'S OWN SENTENCE, AS A TEST: "an unnamed estate gets
// no persistent state at all, and that is the answer rather than an omission".
//
// It is worth a test of its own because the wrong implementation is the
// comfortable one. Falling back to the shared root for an unnamed estate would
// compile, would look like a convenience, and would put two ephemeral estates -
// every test in this repository - into one store keyed by nothing. The refusal
// is the mechanism, so it is asserted rather than assumed.
func TestAnUnnamedEstateGetsNoPersistentState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	p, err := EstateStateDir("")
	if err == nil {
		t.Fatalf("an unnamed estate was given the persistent state directory %s", p)
	}
	for _, want := range []string{"no persistent state", "named"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should say %q, so a reader learns it is the "+
				"answer rather than a bug; got: %v", want, err)
		}
	}
}

// The same crux as TestTheEstateClaimDoesNotMoveWithTheRuntimeDir, and it
// matters MORE here, not less.
//
// A claim that moves with XDG_RUNTIME_DIR refuses nothing, which is loud the
// first time two estates take the same name. A STATE directory that moved with
// it would be silent in both directions for as long as it lasted: development
// would write into its own tree believing it was production's, and production
// would find its records missing rather than corrupted. Section 37 names that
// asymmetry - "the failure it prevents is silent in both directions".
func TestTheStateSubtreeDoesNotMoveWithTheRuntimeDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")

	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	first, err := EstateStateDir("production")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/a-second-estate")
	second, err := EstateStateDir("production")
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatalf("one estate's state resolved to two directories:\n  %s\n  %s\n"+
			"two estates differ exactly in XDG_RUNTIME_DIR, so state that moves "+
			"with it is keyed by the runtime directory and not by the estate",
			first, second)
	}
	if strings.Contains(first, "/run/user/1000") || strings.Contains(first, "a-second-estate") {
		t.Fatalf("the state directory %s sits under a runtime directory, which "+
			"is cleared at logout", first)
	}
}

func TestTwoEstatesDoNotShareAStateSubtree(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	prod, err := EstateStateDir("production")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := EstateStateDir("development")
	if err != nil {
		t.Fatal(err)
	}
	if prod == dev {
		t.Fatalf("production and development share the state directory %s", prod)
	}
	if strings.HasPrefix(prod, dev+"/") || strings.HasPrefix(dev, prod+"/") {
		t.Fatalf("one estate's state is nested inside the other's:\n  %s\n  %s", prod, dev)
	}
}

// THE SHARED ROOT KEEPS EXACTLY ONE TENANT (section 37, precondition 2).
//
// Anything at $XDG_STATE_HOME/rig is by construction visible to both estates,
// which is exactly the property the cross-estate name claim needs and exactly
// the property state must not have. So state is strictly BELOW the root, and
// the claim is the only thing the root is for.
func TestEstateStateSitsBelowTheSharedRootRatherThanAtIt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	root, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	state, err := EstateStateDir("production")
	if err != nil {
		t.Fatal(err)
	}
	if state == root {
		t.Fatalf("an estate's state IS the shared root %s, so both estates "+
			"would see it; the root's one tenant is the name claim", root)
	}
	if !strings.HasPrefix(state, root+"/") {
		t.Fatalf("the state directory %s is not under the state root %s", state, root)
	}
}

// The claim file and the state directory are siblings with the same stem -
// `production.pid` beside `production/` - and they must not be able to collide.
//
// THE COLLISION IS REACHABLE ONLY THROUGH A NAME CONTAINING A DOT, so that is
// what this pins. An estate called `production.pid` would resolve its state
// directory onto another estate's claim file. ValidEstateName already refuses a
// dot, and this asserts it stays refused for the reason that now depends on it.
//
// It also pins the property presence's estate listing rests on: everything in
// `estates/` is either `<name>.pid` or `<name>/`, and the two are told apart by
// a suffix no valid name can carry.
func TestTheClaimFileAndTheStateDirectoryCannotCollide(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")

	claim, err := EstateLock("production")
	if err != nil {
		t.Fatal(err)
	}
	state, err := EstateStateDir("production")
	if err != nil {
		t.Fatal(err)
	}
	if claim == state {
		t.Fatalf("the claim file and the state directory are the same path: %s", claim)
	}
	if filepath.Dir(claim) != filepath.Dir(state) {
		t.Fatalf("the claim and the state subtree are not siblings:\n  %s\n  %s\n"+
			"one `estates` directory is the established idiom and a second tree "+
			"beside it is what precondition 2 says not to open", claim, state)
	}

	// The only way a state directory could land on a claim file.
	for _, name := range []string{"production.pid", "a.b", "x."} {
		if p, err := EstateStateDir(name); err == nil {
			t.Errorf("estate name %q was accepted and resolved to %s; a dot lets "+
				"one estate's state directory land on another's claim file", name, p)
		}
	}
}

// Every name the claim refuses, the state subtree refuses identically. Two
// validators drifting apart is how a name becomes legal for one and not the
// other, and the first symptom would be a daemon that claims a name and then
// cannot open its own store.
func TestTheClaimAndTheStateSubtreeRefuseTheSameNames(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	for _, name := range []string{
		"", "..", ".", "../../etc/passwd", "a/b", "/absolute",
		"Production", "has space", "x\x00y", "-leading", "9leading",
		strings.Repeat("a", MaxEstateNameLen+1),
	} {
		_, lockErr := EstateLock(name)
		_, stateErr := EstateStateDir(name)
		if (lockErr == nil) != (stateErr == nil) {
			t.Errorf("name %q: the claim and the state subtree disagree "+
				"(claim err %v, state err %v)", name, lockErr, stateErr)
		}
	}
	for _, name := range []string{"production", "development", "dev-2", "a"} {
		_, lockErr := EstateLock(name)
		_, stateErr := EstateStateDir(name)
		if lockErr != nil || stateErr != nil {
			t.Errorf("name %q: claim err %v, state err %v", name, lockErr, stateErr)
		}
	}
}
