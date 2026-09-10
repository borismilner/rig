package kernel

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// A program's pane url is checked at registration, and only loopback passes
// (PLAN.md sections 11, 5h R7).
//
// Whatever a program puts in this field is what a webview inside rig loads.
// That makes it a capability question rather than a rendering one: a program
// that could name any origin could point rig's own window at anything, and
// nothing else in the declaration reaches that far. So the check is here,
// where a bad value is a refused registration the program's author sees -
// not at render, in front of the user, which is what R7 exists to prevent.

// loopbackHosts is the whole allowlist.
//
// "localhost" is included because that is what a program's own dev server
// prints, and it is resolved by the webview rather than by rig - so it is
// allowed by NAME here and the resolution is the browser's business. The two
// literals are the addresses it is expected to resolve to.
var loopbackHosts = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
	"::1":       true,
}

// validatePaneURL reports why a pane url cannot be used, or returns nil.
func validatePaneURL(raw string) error {
	if raw == "" {
		return nil // declares no pane, which is the generated tier
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("pane_url %q is not a url: %w", raw, err)
	}
	switch u.Scheme {
	case "http", "https":
	case "":
		return fmt.Errorf("pane_url %q has no scheme: rig will not guess one, "+
			"because the guess decides what the window loads", raw)
	default:
		return fmt.Errorf("pane_url %q uses scheme %q: a pane is served over "+
			"http or https, and a unix socket would need the window to proxy it",
			raw, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("pane_url %q names no host", raw)
	}
	if loopbackHosts[strings.ToLower(host)] {
		return nil
	}
	// An address that is loopback but not one of the two literals - 127.0.0.2,
	// or ::ffff:127.0.0.1 - is still this machine, and refusing it would be
	// refusing something safe for looking unfamiliar.
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("pane_url %q points at %q: a pane must be served on "+
		"loopback, because this is the origin rig's own window will load",
		raw, host)
}
