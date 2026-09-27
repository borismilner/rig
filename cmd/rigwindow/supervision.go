package main

import (
	"context"
	"strings"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Running is what the estate card says a supervised program is doing,
// read from rig.health (section 11: the cards must be useful and
// informative, Boris 2026-09-27). A program Rig does not supervise has no
// row, and the card says so rather than inventing a state for it.
type Running struct {
	ID string `json:"id"`

	// State is supervision's own word: starting, healthy, backoff, and
	// the rest. Since is when it entered that state, in unix milliseconds,
	// so the page can say "for 2 h" without a clock of its own.
	State    string `json:"state"`
	Since    int64  `json:"since"`
	Restarts uint32 `json:"restarts"`

	// Waiting and Parked are what the program last reported it is blocked
	// on, in its own words. Parked is the question a person has to answer.
	Waiting  string `json:"waiting"`
	Parked   string `json:"parked"`
	LastExit string `json:"lastExit"`
}

// Supervision never returns an error, for Health's reason: an estate Rig
// cannot be asked about is drawn from what Programs already said.
func (RigService) Supervision() []Running {
	c, err := client.Connect()
	if err != nil {
		return nil
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &verbsv1.HealthResponse{}
	if err := c.Call(ctx, "rig.health", &verbsv1.HealthRequest{}, resp); err != nil {
		return nil
	}
	out := make([]Running, 0, len(resp.GetPrograms()))
	for _, p := range resp.GetPrograms() {
		out = append(out, Running{
			ID:       p.GetId(),
			State:    stateWord(p.GetState()),
			Since:    time.Unix(0, p.GetSinceUnixNano()).UnixMilli(),
			Restarts: p.GetRestarts(),
			Waiting:  p.GetWaiting(),
			Parked:   p.GetParked(),
			LastExit: p.GetLastExit(),
		})
	}
	return out
}

// stateWord is the enum's own name in lower case, and empty for a state
// the daemon did not set, which the card leaves unsaid.
func stateWord(s verbsv1.ProgramState) string {
	if s == verbsv1.ProgramState_PROGRAM_STATE_UNSPECIFIED {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(s.String(), "PROGRAM_STATE_"))
}
