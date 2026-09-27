package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// The commands storeworker declares. rig routes `rig storeworker <command>`
// at a terminal, and an agent's invoke, to handle below; the page calls the
// same actions directly.

type command struct {
	id, title, summary, description, returns, args, example string
	effects                                                 rigv1.Effects
	idempotent, confirms                                    rigv1.Tristate
}

var commands = []command{
	{
		id: "assign", title: "Assign a run", summary: "Store a run and queue it for the worker",
		description: "Stores the run and the day's count in one transaction, then queues it. A prompt over 400 characters costs more than the budget and waits for an answer; a prompt containing the word fail fails.",
		returns:     "The run id, its state and its cost.",
		args:        `{"type":"object","properties":{"title":{"type":"string"},"prompt":{"type":"string"},"reply_to":{"type":"string","description":"a seat to mail when the run ends"}},"required":["title"]}`,
		example:     `rig storeworker assign --args '{"title":"summarise the inbox","prompt":"three lines"}'`,
		effects:     rigv1.Effects_EFFECTS_WRITES_FILES, idempotent: rigv1.Tristate_TRISTATE_NO, confirms: rigv1.Tristate_TRISTATE_NO,
	},
	{
		id: "runs", title: "List runs", summary: "List runs newest first, one state or all",
		description: "Queries the runs collection, newest first, and counts every state.",
		returns:     "The runs, how many match, and a count per state.",
		args:        `{"type":"object","properties":{"state":{"type":"string","enum":["queued","running","waiting","done","failed","denied"]},"limit":{"type":"integer","minimum":1,"maximum":200}}}`,
		example:     `rig storeworker runs --args '{"state":"waiting"}'`,
		effects:     rigv1.Effects_EFFECTS_READ_ONLY, idempotent: rigv1.Tristate_TRISTATE_YES, confirms: rigv1.Tristate_TRISTATE_NO,
	},
	{
		id: "answer", title: "Answer a waiting run", summary: "Say yes or no to a run waiting over budget",
		description: "A run over the budget waits for this answer. Yes runs it; no stops it.",
		returns:     "The run and the answer given.",
		args:        `{"type":"object","properties":{"run":{"type":"string"},"yes":{"type":"boolean"}},"required":["run","yes"]}`,
		example:     `rig storeworker answer --args '{"run":"r0927-101500-000","yes":true}'`,
		effects:     rigv1.Effects_EFFECTS_WRITES_FILES, idempotent: rigv1.Tristate_TRISTATE_NO, confirms: rigv1.Tristate_TRISTATE_NO,
	},
	{
		id: "export", title: "Export the store", summary: "Write every collection as text and commit it",
		description: "Calls rig's store.export for this program: one JSON Lines file per collection, committed to the exports repository.",
		returns:     "The export directory, the commit, and how many collections.",
		args:        `{"type":"object","properties":{}}`,
		example:     `rig storeworker export`,
		effects:     rigv1.Effects_EFFECTS_WRITES_FILES, idempotent: rigv1.Tristate_TRISTATE_YES, confirms: rigv1.Tristate_TRISTATE_NO,
	},
	{
		id: "restore", title: "Restore the store", summary: "Replace the store with its last export",
		description: "Calls rig's store.import for this program. rig snapshots the store first, and the answer names the snapshot. It asks first: without yes it only says what it would replace.",
		returns:     "The snapshot of the store as it was, and how many collections were replaced.",
		args:        `{"type":"object","properties":{"yes":{"type":"boolean","description":"replace the store; without it, nothing changes"}}}`,
		example:     `rig storeworker restore --args '{"yes":true}'`,
		effects:     rigv1.Effects_EFFECTS_DESTRUCTIVE, idempotent: rigv1.Tristate_TRISTATE_NO, confirms: rigv1.Tristate_TRISTATE_YES,
	},
}

func (a *app) declaration(paneURL string) *rigv1.Declaration {
	d := &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id: a.id, Name: "Storeworker", Version: version,
			Description: "A fake graft: it runs assignments, and each tab of its pane shows one part of rig at work.",
		},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "a demonstration: store, queue, leases, files, knowledge base, mail, toasts and health, no config or logs",
		SemanticsGen: 1,
		PaneUrl:      paneURL,
		Preamble:     "storeworker runs fake assignments. Assign one, list runs, answer a run waiting over budget, export or restore its store.",
	}
	for _, c := range commands {
		d.Commands = append(d.Commands, &rigv1.Command{
			Id: c.id, Title: c.title, Summary: c.summary, Description: c.description, Returns: c.returns,
			Args: []byte(c.args), Examples: []string{c.example},
			Effects: c.effects, Idempotent: c.idempotent, Confirms: c.confirms,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Shape:        rigv1.Shape_SHAPE_UNARY,
		})
	}
	return d
}

// handle answers what rig routes here: the liveness probe and the commands.
func (a *app) handle(method string, payload []byte) (proto.Message, error) {
	_, cmd, _ := strings.Cut(method, ".")
	if cmd == "ping" {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: a.id, Version: version}, nil
	}
	var req rigv1.CallRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	a.logInbound(cmd)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := a.dispatch(ctx, cmd, req.GetArgs())
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &rigv1.CallResponse{Result: b}, nil
}

// dispatch runs one command from its JSON arguments.
func (a *app) dispatch(ctx context.Context, cmd string, raw []byte) (any, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	decode := func(v any) error {
		if err := json.Unmarshal(raw, v); err != nil {
			return invalid("the arguments are not a JSON object: "+err.Error(), "{}")
		}
		return nil
	}
	switch cmd {
	case "assign":
		var args assignArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return a.assign(ctx, args)
	case "runs":
		var args runsArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return a.runs(ctx, args)
	case "answer":
		var args answerArgs
		if err := decode(&args); err != nil {
			return nil, err
		}
		return a.answer(ctx, args)
	case "export":
		return a.exportStore(ctx)
	case "restore":
		var args struct {
			Yes bool `json:"yes"`
		}
		if err := decode(&args); err != nil {
			return nil, err
		}
		if !args.Yes {
			return nil, &client.CallError{Method: "storeworker.restore", Status: &rigv1.Status{
				Code:         rigv1.Code_CODE_DENIED,
				Message:      "storeworker: restore replaces the whole store with its last export, and needs yes to go ahead",
				Precondition: "you said yes",
				Fix:          "run it again with yes",
				FixCommand:   `rig storeworker restore --args '{"yes":true}'`,
			}}
		}
		return a.restore(ctx)
	}
	return nil, &client.CallError{Method: "storeworker." + cmd, Status: &rigv1.Status{
		Code: rigv1.Code_CODE_NOT_FOUND, Message: "storeworker: no command " + cmd,
	}}
}

// logInbound records a command rig routed here, so the page shows a call
// that came IN through rig beside the ones this program made.
func (a *app) logInbound(cmd string) {
	a.logRow(callRow{
		At: time.Now().Format("15:04:05"), Verb: "routed in",
		Note: "rig delivered storeworker." + cmd,
	})
}
