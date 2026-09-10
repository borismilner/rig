package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

func TestAPaneURLIsAcceptedOnlyOnLoopback(t *testing.T) {
	good := []string{
		"",                           // declares no pane
		"http://127.0.0.1:7431/pane", // what a program's own server prints
		"http://localhost:7431/pane", // resolved by the webview, not by rig
		"https://localhost/pane",     // a program with its own certificate
		"http://[::1]:7431/pane",     // ipv6 loopback
		"http://127.0.0.2:7431/",     // loopback without being the literal
	}
	for _, raw := range good {
		d := good1(raw)
		if err := d.Validate(); err != nil {
			t.Fatalf("pane_url %q was refused: %v", raw, err)
		}
	}

	bad := map[string]string{
		"someone else's machine": "http://192.168.1.10:7431/pane",
		"a public origin":        "https://example.com/pane",
		"no scheme":              "127.0.0.1:7431/pane",
		"a file":                 "file:///tmp/pane.html",
		"a unix socket":          "unix:///run/user/1000/pane.sock",
		"javascript":             "javascript:alert(1)",
		"no host":                "http:///pane",
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			err := good1(raw).Validate()
			if err == nil {
				t.Fatalf("%q was accepted", raw)
			}
			if !strings.Contains(err.Error(), "pane_url") {
				t.Fatalf("the refusal does not name the field: %v", err)
			}
		})
	}
}

func TestAPaneURLIsRefusedAtRegistrationNotAtRender(t *testing.T) {
	// Section 5h's R7: a refusal at render happens in front of the user.
	k := kernel.New()
	_, err := k.Register(programPrincipal("shelf"), good1("https://example.com/pane"))
	if err == nil {
		t.Fatal("a program declaring an off-machine pane was registered")
	}
	if _, ok := k.See(caller(kernel.KindAgent)).Program("shelf"); ok {
		t.Fatal("the refused declaration is in the registry anyway")
	}
}

func TestAValidPaneURLReachesAReader(t *testing.T) {
	// The window is a reader like any other, so the field has to survive the
	// projection rather than stopping at the registry.
	k := kernel.New()
	p, err := k.Register(programPrincipal("shelf"), good1("http://127.0.0.1:7431/pane"))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := k.See(p).Program("shelf")
	if !ok {
		t.Fatal("the program cannot see itself")
	}
	if got.PaneURL != "http://127.0.0.1:7431/pane" {
		t.Fatalf("a reader got pane_url %q", got.PaneURL)
	}
}

// good1 is a valid declaration for "shelf" carrying one pane url.
func good1(pane string) kernel.Declaration {
	d := good("shelf")
	d.PaneURL = pane
	return d
}
