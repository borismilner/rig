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
