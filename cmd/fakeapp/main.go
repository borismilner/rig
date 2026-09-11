// Command fakeapp is the reference program.
//
// PLAN.md section 19: "fakeapp is the misbehaving reference program and ships
// in the repo. It hangs, crashes, leaks, floods, lies about its schema, ignores
// cancellation and returns garbage, each on a flag."
//
// At M0 it behaves, and answers ping. The misbehaviours arrive with the
// supervisor that is supposed to catch them (M6) - a flag that nothing asserts
// against is a flag that rots.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fakeapp: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "fakeapp", "the program id to announce")
	misbehave := flag.String("misbehave", "", "hang (M0 implements only this one)")
	flag.Parse()

	sock, err := paths.Socket()
	if err != nil {
		return err
	}
	c, err := client.Dial(sock)
	if err != nil {
		return err
	}
	defer c.Close()

	c.Handle(func(method string, payload []byte) (proto.Message, error) {
		if *misbehave == "hang" {
			// Longer than the daemon's CallTimeout, so rig answers DEADLINE
			// instead of waiting on this process.
			time.Sleep(time.Hour)
			return nil, nil
		}
		// The probe carries rig's own typed message; every declared command
		// carries CallRequest with the JSON its schema describes.
		if command(method) == "ping" {
			var req rigv1.PingRequest
			if err := proto.Unmarshal(payload, &req); err != nil {
				return nil, err
			}
			return &rigv1.PingResponse{
				Nonce:   req.GetNonce(),
				Program: *name,
				Version: version,
			}, nil
		}
		return answer(method, payload)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := c.Hello(ctx, declaration(*name))
	if err != nil {
		return err
	}
	fmt.Printf("%s up: wire %s, rigd %s, scoped %v\n",
		*name, resp.GetWire(), resp.GetDaemonVersion(), resp.GetScoped())

	<-c.Done()
	return c.Err()
}

// declaration is what fakeapp tells rig it is (PLAN.md section 5e).
//
// Written out by hand here because fakeapp is the reference program and the
// point is to see the whole shape. A real program generates this from its own
// command definitions: section 5e measured one rich archi command at 160
// non-blank lines of JSON, and section 3's 100-line budget is hand-written Go,
// not declaration.
//
// Every mandatory property is said. Leaving one out is not a smaller
// declaration, it is a refused one - section 5e bans a default that carries a
// safety meaning, so an absent effects is an error rather than read-only.
func declaration(id string) *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id:          id,
			Name:        "Fake App",
			Version:     version,
			Description: "The reference program the conformance suite drives.",
		},
		// Partial, and honestly so: fakeapp adopts nothing but the wire.
		// Section 5k's rule is that no surface may imply completeness.
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "the wire only: no config, no storage, no logs",
		SemanticsGen: 1,
		// THE PREAMBLE: section 9's "one document an agent reads first", and
		// the reference program had none until `rig describe` existed to
		// return it. The field has been declarable since M1 - accepted at
		// hello, validated, stored - and no message on any path could carry
		// it back, so nothing had a reason to fill it in.
		//
		// It says what a caller cannot work out from the command list, which
		// is the only thing worth spending an agent's first read on: what
		// this program is FOR, and the one trap in it.
		Preamble: "fakeapp exists to be driven, not to be useful. It is the " +
			"reference program the conformance suite runs against, so its " +
			"commands are shaped to exercise rig rather than to do work: " +
			"ping proves a round trip is your own, reindex takes a real " +
			"argument schema so the CLI has flags to build, and slow exists " +
			"to be interrupted.\n\n" +
			"Nothing it does touches anything outside its own process. " +
			"reindex declares that it writes files and writes none, which " +
			"is deliberate: a declaration is what rig acts on, and the " +
			"conformance suite needs a command whose declared effects it " +
			"can check rig honours without a program that actually destroys " +
			"something to check them against.",
		Commands: []*rigv1.Command{{
			Id:      "ping",
			Title:   "Ping",
			Effects: rigv1.Effects_EFFECTS_READ_ONLY,
			// Re-running is free, which is what lets rig retry it.
			Idempotent: rigv1.Tristate_TRISTATE_YES,
			// Present and empty: fakeapp considered the question and the
			// answer is none. An absent list would be refused.
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Confirms:     rigv1.Tristate_TRISTATE_NO,
			Shape:        rigv1.Shape_SHAPE_UNARY,
			Summary:      "Round-trip a nonce",
			Description: "Echoes the nonce it was given, so a caller can " +
				"prove the round trip was its own and not a cached reply.",
			Returns: "The nonce, this program's id and its version.",
		}, {
			Id:    "reindex",
			Title: "Reindex",
			// The argument schema. rig validates against THIS at the
			// boundary and builds the CLI's flags from it, so `--since` is
			// not written down anywhere in rig.
			Args: []byte(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["since"],
  "properties": {
    "since": {
      "type": "string",
      "pattern": "^[0-9]+[dhm]$",
      "description": "how far back to reindex, e.g. 7d"
    },
    "dry_run": {
      "type": "boolean",
      "description": "count what would be done and change nothing"
    },
    "workers": {"type": "integer", "minimum": 1, "maximum": 64}
  }
}`),
			Examples: []string{
				"rig fakeapp reindex --since 7d",
				"rig fakeapp reindex --since 24h --dry-run --json",
			},
			// Writes files rather than destructive: it rebuilds an index it
			// owns. A house rule naming destructive does not fire on it, and
			// that is the declaration doing its job.
			Effects:      rigv1.Effects_EFFECTS_WRITES_FILES,
			Idempotent:   rigv1.Tristate_TRISTATE_YES,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_SECONDS,
			Confirms:     rigv1.Tristate_TRISTATE_NO,
			Shape:        rigv1.Shape_SHAPE_UNARY,
			Summary:      "Rebuild the index over a window",
			Description: "Walks everything changed in the window and " +
				"rebuilds the index for it. Re-running is free.",
			Returns: "The window it used, how many items it indexed, and " +
				"whether it was a dry run.",
		}, {
			Id:    "purge",
			Title: "Purge",
			// No argument schema at all, which means it takes none - and rig
			// refuses a call that passes some rather than dropping them.
			Effects:      rigv1.Effects_EFFECTS_DESTRUCTIVE,
			Idempotent:   rigv1.Tristate_TRISTATE_YES,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Confirms:     rigv1.Tristate_TRISTATE_YES,
			Shape:        rigv1.Shape_SHAPE_UNARY,
			Summary:      "Delete the index",
			Description: "Deletes the index and everything derived from it. " +
				"Declared destructive, which is what a house rule matches on.",
			Returns: "Nothing.",
		}},
	}
}

// command is the part of <program>.<command> after the first dot.
func command(method string) string {
	if _, c, ok := strings.Cut(method, "."); ok {
		return c
	}
	return method
}

// answer runs a declared command over CallRequest/CallResponse.
//
// fakeapp does not re-validate the arguments: rig validated them against the
// declared schema at the boundary, and a program that checks again is a
// program with a second opinion about its own declaration.
func answer(method string, payload []byte) (proto.Message, error) {
	var req rigv1.CallRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, err
	}

	switch command(method) {
	case "reindex":
		var args struct {
			Since   string `json:"since"`
			DryRun  bool   `json:"dry_run"`
			Workers int    `json:"workers"`
		}
		if len(req.GetArgs()) > 0 {
			if err := json.Unmarshal(req.GetArgs(), &args); err != nil {
				return nil, err
			}
		}
		if args.Workers == 0 {
			args.Workers = 4
		}
		// A number that depends on the window, so the demo shows the
		// arguments actually arriving rather than a constant.
		indexed := len(args.Since) * 137
		if args.DryRun {
			indexed = 0
		}
		out, err := json.Marshal(map[string]any{
			"since":   args.Since,
			"indexed": indexed,
			"workers": args.Workers,
			"dry_run": args.DryRun,
		})
		if err != nil {
			return nil, err
		}
		return &rigv1.CallResponse{Result: out}, nil

	case "purge":
		return &rigv1.CallResponse{Result: []byte(`{"purged":true}`)}, nil
	}
	return nil, fmt.Errorf("fakeapp: no command %q", command(method))
}
