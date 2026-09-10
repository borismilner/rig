package main

import (
	"context"
	"fmt"
	"time"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
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
