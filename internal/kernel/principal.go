// Package kernel is rig's mechanisms: the registry, the principal and the
// scope, and later the invoker, supervision, config resolution and the
// capability check (PLAN.md section 5i).
//
// It holds no domain type. No Severity, no LogRecord, no Lease: shared
// vocabulary lives in the service that owns it, because otherwise the cheapest
// way to share anything is always to move it in here.
package kernel

import (
	"fmt"
	"slices"
)

// ClientKind is what is on the other end of a call.
//
// Section 13a: the enum ships at M1, before the surfaces it names exist,
// because house rules live in the kernel's invoker and the point of that
// placement is that a surface added in 2028 is covered by a rule written in
// 2026 - which only holds if the vocabulary can already name it.
//
// The last three are not connections. A scheduled fire, a bus-triggered
// invocation and a rig:// URL each mint a principal at the point of
// invocation, and without them the rule the owner most needs - "the scheduler
// may not run destructive commands unattended" - cannot be written at all.
type ClientKind uint8

const (
	// KindUnspecified is the zero, and it means nothing (section 21). A
	// principal that never said what it is fails the connect-time check
	// rather than defaulting to the least dangerous kind.
	KindUnspecified ClientKind = iota
	KindAgent
	KindTerminal
	KindWindow
	KindScript
	KindProgram
	KindSchedule
	KindBus
	KindURL
)

var kindNames = map[ClientKind]string{
	KindUnspecified: "unspecified",
	KindAgent:       "agent",
	KindTerminal:    "terminal",
	KindWindow:      "window",
	KindScript:      "script",
	KindProgram:     "program",
	KindSchedule:    "schedule",
	KindBus:         "bus",
	KindURL:         "url",
}

func (k ClientKind) String() string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return fmt.Sprintf("ClientKind(%d)", uint8(k))
}

// Valid reports whether a kind was actually declared.
func (k ClientKind) Valid() bool {
	_, ok := kindNames[k]
	return ok && k != KindUnspecified
}

// ParseClientKind reads the name a house rule writes.
//
// "any" is a rule's wildcard, not something a connection can be, so it is not
// a ClientKind and is not parsed here.
func ParseClientKind(s string) (ClientKind, error) {
	for k, n := range kindNames {
		if n == s && k != KindUnspecified {
			return k, nil
		}
	}
	return KindUnspecified, fmt.Errorf("kernel: %q is not a client kind", s)
}

// Principal is who is calling, decided when the connection is made and fixed
// for its life (section 14).
//
// There is deliberately no Operate field. Estate-wide action is an
// authorisation decision per call, not a grant: an interactive caller is asked
// and the answer authorises that one call, an unattended caller is named in a
// house rule. Section 14 deleted the credential, and with it the file that had
// to be distributed to callers that provably could not read it.
type Principal struct {
	// UID is the unix user. One daemon per uid, so this is the same for every
	// principal a daemon ever sees; it is carried anyway because the isolation
	// argument is stated over it.
	UID int

	// Kind is what is on the other end.
	Kind ClientKind

	// ClientID names this client across reconnects; SessionID names one
	// connection. A reconnecting client keeps the first and gets a new second.
	ClientID  string
	SessionID string

	// PID is the caller's process, carried because an elevation prompt has to
	// name it (section 14) - a question that says only "stop shelf?" is
	// answered by whoever is looking at the screen.
	PID int

	// Scoped is the one boolean left on this struct, and registration is the
	// only thing that sets it. A connection that completed the program
	// handshake is a program, and is scoped for the life of that connection.
	Scoped bool

	// Scopes are the sets this principal belongs to. A scoped principal reads
	// only what its scopes carry; the crew a peers announce returns is a scope
	// rather than a special case in the kernel.
	Scopes []string

	// Introspect is decided at connect time, never presented and never
	// distributed (section 14). It is the one door out of the default-deny
	// view, and it is what an agent Boris runs holds.
	Introspect bool
}

// Valid reports whether a principal is complete enough to act.
func (p Principal) Valid() error {
	if !p.Kind.Valid() {
		return fmt.Errorf("kernel: principal has no client kind")
	}
	if p.ClientID == "" {
		return fmt.Errorf("kernel: principal has no client id")
	}
	if p.SessionID == "" {
		return fmt.Errorf("kernel: principal has no session id")
	}
	if p.Scoped && len(p.Scopes) == 0 {
		return fmt.Errorf("kernel: principal %q is scoped and holds no scope, "+
			"which would read nothing at all", p.ClientID)
	}
	return nil
}

// InScope reports whether this principal may see something carrying scope s.
//
// An introspecting principal is in every scope. That is the whole of the door
// out of default deny, stated once, here, rather than as a condition repeated
// at each view - which is how the seventh view leaks.
func (p Principal) InScope(s string) bool {
	if p.Introspect {
		return true
	}
	if !p.Scoped {
		// An unscoped, unprivileged client reads what is unscoped: its own
		// calls and the programs it may reach.
		return s == ""
	}
	return slices.Contains(p.Scopes, s)
}

// String renders a principal for a log line or an elevation prompt.
func (p Principal) String() string {
	return fmt.Sprintf("%s/%s (session %s, pid %d)", p.Kind, p.ClientID, p.SessionID, p.PID)
}
