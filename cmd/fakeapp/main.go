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
	"flag"
	"fmt"
	"os"
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

	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		if *misbehave == "hang" {
			// Longer than the daemon's CallTimeout, so rig answers DEADLINE
			// instead of waiting on this process.
			time.Sleep(time.Hour)
			return nil, nil
		}
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{
			Nonce:   req.GetNonce(),
			Program: *name,
			Version: version,
		}, nil
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
		}},
	}
}
