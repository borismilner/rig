package paths

import (
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
