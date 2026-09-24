// Command greeter is the smallest complete rig program, and the one
// docs/programs.md walks through.
//
// It declares one command, greet, which takes a name, and refuses an empty
// one with a code and a fix the caller can act on. Run it beside rigd and
// then:
//
//	rig greeter greet --args '{"name":"Boris"}'
//
// It imports only rig's public client and the program-door proto package,
// which is all any program needs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

const (
	id      = "greeter"
	version = "0.1.0"
)

func main() {
	c, err := client.Connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "greeter:", err)
		os.Exit(1)
	}
	defer c.Close()
	if err := register(c); err != nil {
		fmt.Fprintln(os.Stderr, "greeter:", err)
		os.Exit(1)
	}
	fmt.Println("greeter: registered")
	<-c.Done()
}

// register sets the handler and says hello. The handler is set first, so a
// request cannot arrive with nothing to answer it.
func register(c *client.Client) error {
	c.Handle(handle)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.Hello(ctx, declaration())
	return err
}

// handle answers the two methods rig routes here: the liveness probe and the
// one declared command.
func handle(method string, payload []byte) (proto.Message, error) {
	_, command, _ := strings.Cut(method, ".")
	switch command {
	case "ping":
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: id, Version: version}, nil
	case "greet":
		var req rigv1.CallRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.GetArgs(), &args); err != nil || args.Name == "" {
			// A refusal the caller can act on, not an INTERNAL error.
			return nil, &client.CallError{Method: method, Status: &rigv1.Status{
				Code:         rigv1.Code_CODE_INVALID,
				Message:      "greeter: greet needs a name",
				Precondition: "the arguments carry a non-empty name",
				Actual:       "no name was given",
				Fix:          "pass a name",
				FixCommand:   `rig greeter greet --args '{"name":"you"}'`,
			}}
		}
		out, err := json.Marshal(map[string]string{"greeting": "hello, " + args.Name})
		if err != nil {
			return nil, err
		}
		return &rigv1.CallResponse{Result: out}, nil
	}
	return nil, &client.CallError{Method: method, Status: &rigv1.Status{
		Code: rigv1.Code_CODE_NOT_FOUND, Message: "greeter: no command " + command,
	}}
}

// declaration is everything rig learns about this program. Every property of
// a command is said explicitly: rig refuses a zero, because a zero would mean
// nobody decided.
func declaration() *rigv1.Declaration {
	return &rigv1.Declaration{
		Identity: &rigv1.Identity{
			Id: id, Name: "Greeter", Version: version,
			Description: "The smallest complete rig program.",
		},
		Coverage:     rigv1.Coverage_COVERAGE_PARTIAL,
		CoverageNote: "the wire only: no config, no storage, no logs",
		SemanticsGen: 1,
		Commands: []*rigv1.Command{{
			Id: "greet", Title: "Greet", Summary: "Say hello to someone",
			Description: "Greets the name it is given.",
			Returns:     "A greeting.",
			Args: []byte(`{"type":"object","properties":{"name":{"type":"string"}},` +
				`"required":["name"]}`),
			Examples:     []string{`rig greeter greet --args '{"name":"Boris"}'`},
			Effects:      rigv1.Effects_EFFECTS_READ_ONLY,
			Idempotent:   rigv1.Tristate_TRISTATE_YES,
			Sensitive:    &rigv1.SensitiveFields{},
			Interactive:  rigv1.Tristate_TRISTATE_NO,
			Streams:      rigv1.Tristate_TRISTATE_NO,
			NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration:     rigv1.Duration_DURATION_INSTANT,
			Confirms:     rigv1.Tristate_TRISTATE_NO,
			Shape:        rigv1.Shape_SHAPE_UNARY,
		}},
	}
}
