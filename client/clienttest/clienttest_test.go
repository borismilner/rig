package clienttest_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/client/clienttest"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// The whole of what a program author does with this package: start rig,
// register, and call a command through rig the way the CLI would.
func TestAProgramRegistersAndIsCalledThroughARealDaemon(t *testing.T) {
	clienttest.Start(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	prog, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer prog.Close()
	prog.Handle(func(method string, payload []byte) (proto.Message, error) {
		switch method {
		case "echo.ping":
			var req rigv1.PingRequest
			if err := proto.Unmarshal(payload, &req); err != nil {
				return nil, err
			}
			return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: "echo", Version: "1"}, nil
		case "echo.say":
			var req rigv1.CallRequest
			if err := proto.Unmarshal(payload, &req); err != nil {
				return nil, err
			}
			return &rigv1.CallResponse{Result: req.GetArgs()}, nil
		}
		return nil, errors.New("no such command")
	})
	hello, err := prog.Hello(ctx, &rigv1.Declaration{
		Identity: &rigv1.Identity{Id: "echo", Name: "Echo", Version: "1"},
		Coverage: rigv1.Coverage_COVERAGE_PARTIAL, CoverageNote: "a test", SemanticsGen: 1,
		Commands: []*rigv1.Command{{
			Id: "say", Title: "Say", Summary: "Echo the arguments", Description: "Echoes.",
			Returns: "The arguments.", Args: []byte(`{"type":"object"}`),
			Effects: rigv1.Effects_EFFECTS_READ_ONLY, Idempotent: rigv1.Tristate_TRISTATE_YES,
			Sensitive: &rigv1.SensitiveFields{}, Interactive: rigv1.Tristate_TRISTATE_NO,
			Streams: rigv1.Tristate_TRISTATE_NO, NeedsDisplay: rigv1.Tristate_TRISTATE_NO,
			Duration: rigv1.Duration_DURATION_INSTANT, Confirms: rigv1.Tristate_TRISTATE_NO,
			Shape: rigv1.Shape_SHAPE_UNARY,
		}},
	})
	if err != nil {
		t.Fatalf("hello: %v", err)
	}
	if hello.GetWire() != "v1" || len(hello.GetMethods()) == 0 {
		t.Errorf("hello answered wire %q with %d methods", hello.GetWire(), len(hello.GetMethods()))
	}

	caller, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer caller.Close()
	var out rigv1.CallResponse
	if err := caller.Call(ctx, "echo.say", &rigv1.CallRequest{Args: []byte(`{"x":1}`)}, &out); err != nil {
		t.Fatalf("calling the program through rig: %v", err)
	}
	var got map[string]int
	if err := json.Unmarshal(out.GetResult(), &got); err != nil || got["x"] != 1 {
		t.Errorf("the program's answer came back as %s", out.GetResult())
	}
}
