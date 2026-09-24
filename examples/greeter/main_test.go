package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/client/clienttest"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// How a program author tests against rig: a real daemon from clienttest,
// the program registered exactly as main does it, and its command called
// through rig from a second client.
func TestGreetThroughRig(t *testing.T) {
	clienttest.Start(t)
	prog, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer prog.Close()
	if err := register(prog); err != nil {
		t.Fatalf("rig refused the declaration: %v", err)
	}

	caller, err := client.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer caller.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out rigv1.CallResponse
	if err := caller.Call(ctx, "greeter.greet",
		&rigv1.CallRequest{Args: []byte(`{"name":"Boris"}`)}, &out); err != nil {
		t.Fatal(err)
	}
	if string(out.GetResult()) != `{"greeting":"hello, Boris"}` {
		t.Errorf("greet answered %s", out.GetResult())
	}

	err = caller.Call(ctx, "greeter.greet", &rigv1.CallRequest{Args: []byte(`{"name":""}`)}, &out)
	var ce *client.CallError
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INVALID || ce.Status.GetFix() == "" {
		t.Errorf("an empty name gave %v, want INVALID with a fix", err)
	}
}
