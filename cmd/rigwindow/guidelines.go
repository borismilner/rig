package main

import (
	"context"
	"errors"
	"time"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Guidelines is plan/55 requirement 29: what a program or an agent must be
// to work with rig, each rule dated, and when each program was built.
//
// Read from the running daemon's rig.guidelines, never from this binary, for
// the reason Capabilities gives (decision 0266).

// Guideline is one dated rule.
type Guideline struct {
	ID    string `json:"id"`
	Date  string `json:"date"`
	Who   string `json:"who"`
	Built bool   `json:"built"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Cite  string `json:"cite"`
}

// ProgramBuild is when one program's binary was built.
type ProgramBuild struct {
	Program string `json:"program"`
	// Day is the commit's local date, YYYY-MM-DD, the unit a rule is dated
	// in; "" when unknown.
	Day            string `json:"day"`
	CommitTime     string `json:"commitTime"`
	Modified       bool   `json:"modified"`
	UnknownBecause string `json:"unknownBecause"`
}

// Guidelines is the whole answer.
type Guidelines struct {
	Revision string         `json:"revision"`
	Rules    []Guideline    `json:"rules"`
	Programs []ProgramBuild `json:"programs"`
}

// Guidelines reads rig.guidelines.
func (RigService) Guidelines() (Guidelines, error) {
	c, err := client.Connect()
	if err != nil {
		return Guidelines{}, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &verbsv1.GuidelinesResponse{}
	if err := c.Call(ctx, "rig.guidelines", &verbsv1.GuidelinesRequest{}, resp); err != nil {
		var ce *client.CallError
		if errors.As(err, &ce) && ce.Code() == rigv1.Code_CODE_NOT_FOUND {
			// The window can be newer than the daemon (B89); say which.
			return Guidelines{}, errors.New("the running rigd is older than rig.guidelines, so it publishes none; deploy rig to read them")
		}
		return Guidelines{}, err
	}
	return guidelinesFrom(resp), nil
}

func guidelinesFrom(r *verbsv1.GuidelinesResponse) Guidelines {
	g := Guidelines{Revision: r.GetRevision(), Rules: []Guideline{}, Programs: []ProgramBuild{}}
	for _, x := range r.GetRules() {
		g.Rules = append(g.Rules, Guideline{
			ID: x.GetId(), Date: x.GetDate(), Who: x.GetWho(), Built: x.GetBuilt(),
			Title: x.GetTitle(), Body: x.GetBody(), Cite: x.GetCite(),
		})
	}
	for _, p := range r.GetPrograms() {
		b := ProgramBuild{
			Program: p.GetProgram(), CommitTime: p.GetCommitTime(),
			Modified: p.GetModified(), UnknownBecause: p.GetUnknownBecause(),
		}
		if t, err := time.Parse(time.RFC3339, b.CommitTime); err == nil {
			b.Day = t.Local().Format(time.DateOnly)
		} else if b.CommitTime != "" {
			b.UnknownBecause = "its commit time " + b.CommitTime + " is not RFC 3339"
			b.CommitTime = ""
		}
		g.Programs = append(g.Programs, b)
	}
	return g
}
