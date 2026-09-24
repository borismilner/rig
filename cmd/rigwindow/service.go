package main

import (
	"context"
	"fmt"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/internal/paths"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// readDeadline is short on purpose. A surface that hangs is worse than one that
// says it cannot reach rig: `rig ping` defaults to 5s against a 10s CallTimeout
// in the daemon, so an ordinary hang reaches a person as "context deadline
// exceeded" instead of rig's own message (a section 18 question nobody has
// settled). The window is drawing a rail, so it would rather be wrong for two
// seconds and say so.
const readDeadline = 2 * time.Second

// RigService is the window's only route to the daemon.
//
// It dials per call rather than holding a connection, which is what makes
// section 11's "the window survives a daemon restart" true for free: there is
// no socket to go stale, and the next repaint reconnects. Section 22's split
// pays for it - a unix socket dial is microseconds, and the alternative is
// reconnect logic that M6 is going to write properly anyway.
type RigService struct{}

// Program is one registered program, flattened for the rail.
//
// Deliberately NOT here: an identity hue. Section 11 gives a program one
// ownable hue and says a program without one leaves the shell achromatic, but
// rigv1.Identity carries id, name, version, icon and description and no hue at
// all - so every program is achromatic today, and not by choice. Deriving one
// from the id would fake a declaration the program never made. It is the first
// field the shell found missing from section 5e; the wire is not this session's
// to change.
type Program struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Icon         string   `json:"icon"`
	Description  string   `json:"description"`
	Coverage     string   `json:"coverage"`
	CoverageNote string   `json:"coverageNote"`
	Services     []string `json:"services"`
	Hosted       bool     `json:"hosted"`
	Commands     int      `json:"commands"`

	// Where the program serves its own HTML, empty if it serves none.
	// Section 11's three tiers turn on this one field: empty means rig draws
	// the pane from what was declared, and a value means the program draws it.
	// rig refuses anything but a loopback origin at registration, so by the
	// time a value reaches here it has already been checked.
	PaneURL string `json:"paneUrl"`
}

// Health is what the status strip renders.
//
// Detached is section 5g's fourth state: the window is up and the daemon is
// not. The strip has to be able to say that, because a rail with no programs
// in it looks identical to a rail whose daemon died.
type Health struct {
	Connected bool   `json:"connected"`
	Socket    string `json:"socket"`
	Detail    string `json:"detail"`
	Programs  int    `json:"programs"`
}

// Programs answers with what this client may see, which is the registry's own
// answer and not a filtered copy of it (section 14 decided visibility at
// connect time).
func (RigService) Programs() ([]Program, error) {
	c, err := client.Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.ProgramsResponse{}
	if err := c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{}, resp); err != nil {
		return nil, err
	}

	out := make([]Program, 0, len(resp.GetPrograms()))
	for _, p := range resp.GetPrograms() {
		id := p.GetIdentity()
		out = append(out, Program{
			ID:           id.GetId(),
			Name:         id.GetName(),
			Version:      id.GetVersion(),
			Icon:         id.GetIcon(),
			Description:  id.GetDescription(),
			Coverage:     coverageName(p.GetCoverage()),
			CoverageNote: p.GetCoverageNote(),
			Services:     p.GetServices(),
			Hosted:       p.GetHosted(),
			Commands:     len(p.GetCommands()),
			PaneURL:      p.GetPaneUrl(),
		})
	}
	return out, nil
}

// Health never returns an error: not reaching rig is a state the strip draws,
// not a failure the window has to handle.
func (RigService) Health() Health {
	h := Health{Socket: "unknown"}
	if s, err := paths.Socket(); err == nil {
		h.Socket = s
	}

	c, err := client.Connect()
	if err != nil {
		h.Detail = fmt.Sprintf("detached: %v", err)
		return h
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &rigv1.ProgramsResponse{}
	if err := c.Call(ctx, "rig.programs", &rigv1.ProgramsRequest{}, resp); err != nil {
		h.Detail = fmt.Sprintf("connected, but the registry did not answer: %v", err)
		return h
	}

	h.Connected = true
	h.Programs = len(resp.GetPrograms())
	return h
}

// Build is the four versions the binary carries, so the strip can show which
// window is running without a person going to a terminal.
func (RigService) Build() map[string]string {
	return map[string]string{
		"version": version,
		"wire":    wire,
		"commit":  sha,
		"built":   date,
	}
}

func coverageName(c rigv1.Coverage) string {
	switch c {
	case rigv1.Coverage_COVERAGE_FULL:
		return "full"
	case rigv1.Coverage_COVERAGE_PARTIAL:
		return "partial"
	default:
		return "unspecified"
	}
}

// Deployment is what is actually RUNNING, artefact by artefact, and whether the
// artefacts agree with each other.
//
// ⛔ IT EXISTS BECAUSE `make install` REPORTED SUCCESS OVER A WINDOW IT DOES NOT
// TOUCH, AND NOBODY COULD SEE IT FOR THIRTEEN HOURS (B89). The Makefile's
// install/install-window split is deliberate and right; the consequence nobody
// accounted for is that a person who deploys and then looks at the window has
// deployed the daemon and is looking at something else. Telling a stale window
// from a current one took four separate commands and a screenshot's footer.
//
// ⛔ SO THE PANEL OWES THIS BEFORE IT OWES A BUTTON. Boris asked for a
// management panel and said "redeployment should be trivial and automatic"
// (plan/11, B90). Two buttons would have produced B89 again, because somebody
// still has to know to press the second one. A redeploy control that cannot say
// what is currently running is a button that reports success over the same
// failure.
//
// WHAT IT DOES NOT DO: it does not compare against the SOURCE TREE. An installed
// window has no tree to read, and a "built" column derived from one would be
// absent on exactly the machine that needs it. This compares the artefacts that
// are running to each other, which is the comparison B89 actually needed.
type Deployment struct {
	// WindowVersion and its siblings are this binary's own ldflags stamp.
	WindowVersion string `json:"windowVersion"`
	WindowCommit  string `json:"windowCommit"`
	WindowBuilt   string `json:"windowBuilt"`

	// DaemonVersion comes over the socket from the daemon that is answering
	// right now, never from a file on disk.
	DaemonVersion string `json:"daemonVersion"`
	DaemonWire    string `json:"daemonWire"`
	WindowWire    string `json:"windowWire"`
	Epoch         uint64 `json:"epoch"`

	// Reached is false when the daemon did not answer. ⛔ IT IS NOT A SKEW:
	// "these disagree" and "I could not ask" are different facts and a panel
	// that renders them the same way is the B89 defect in a new place.
	Reached bool `json:"reached"`

	// Agree is only meaningful when Reached. Verdict says why in a person's
	// words, and it is the line the panel leads with.
	Agree   bool   `json:"agree"`
	Verdict string `json:"verdict"`
}

// Deployment never returns an error, for Health's reason: not reaching rig is a
// state to draw, not a failure to handle.
func (RigService) Deployment() Deployment {
	d := Deployment{
		WindowVersion: version,
		WindowCommit:  sha,
		WindowBuilt:   date,
		WindowWire:    wire,
	}

	est, ok := estateSnapshot()
	if !ok {
		d.Verdict = "rig is not answering, so the window cannot say what is deployed beside it."
		return d
	}

	d.Reached = true
	d.DaemonVersion = est.GetDaemonVersion()
	d.DaemonWire = est.GetWire()
	d.Epoch = est.GetEpoch()

	d.Agree, d.Verdict = deploymentVerdict(d.DaemonVersion, d.WindowVersion)
	return d
}

// deploymentVerdict decides whether two running artefacts agree, and says why.
//
// ⛔ IT IS A SEPARATE FUNCTION SO IT CAN BE TESTED WITHOUT A DAEMON. The
// comparison is the whole value of the panel and it would otherwise only ever
// run against whatever happened to be on the developer's machine, which is the
// arrangement that let B89 live for thirteen hours.
//
// ⛔ AN EMPTY DAEMON VERSION IS NOT A DISAGREEMENT. A daemon too old to stamp
// itself cannot be compared, and reporting that as a skew would send a person
// to reinstall a window that is already correct.
func deploymentVerdict(daemonVersion, windowVersion string) (bool, string) {
	switch daemonVersion {
	case "":
		return false, "this daemon does not report a version, so the two cannot be compared."
	case windowVersion:
		return true, "the daemon and the window are the same build."
	default:
		return false, "THE WINDOW AND THE DAEMON ARE DIFFERENT BUILDS. " +
			"`make install` does not install the window: run `make install-window` and restart the tray."
	}
}
