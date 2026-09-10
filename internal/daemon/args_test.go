package daemon

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

func TestDeclaredArgumentsAreCheckedAtTheBoundary(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	dangerous(t, sock)
	c := dial(t, sock)

	var resp rigv1.CallResponse
	if err := c.Call(ctx5(t), "shelf.reindex",
		&rigv1.CallRequest{Args: []byte(`{"since":"7d"}`)}, &resp); err != nil {
		t.Fatalf("valid arguments were refused: %v", err)
	}
	if !strings.Contains(string(resp.GetResult()), "shelf.reindex") {
		t.Fatalf("the program answered %q", resp.GetResult())
	}

	bad := map[string]string{
		"unknown field": `{"since":"7d","untl":"now"}`,
		"wrong type":    `{"since":7}`,
		"missing":       `{}`,
		"nothing":       ``,
	}
	for name, args := range bad {
		t.Run(name, func(t *testing.T) {
			err := c.Call(ctx5(t), "shelf.reindex",
				&rigv1.CallRequest{Args: []byte(args)}, &resp)
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !strings.Contains(err.Error(), "INVALID") {
				t.Fatalf("a schema failure arrived as %v", err)
			}
			if !strings.Contains(err.Error(), "reindex") {
				t.Fatalf("the error does not name the command: %v", err)
			}
		})
	}
}

func TestACommandNobodyDeclaredNeverReachesTheProgram(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	dangerous(t, sock)

	var resp rigv1.CallResponse
	err := dial(t, sock).Call(ctx5(t), "shelf.nosuch", &rigv1.CallRequest{}, &resp)
	if err == nil {
		t.Fatal("an undeclared command was routed to the program")
	}
	if !strings.Contains(err.Error(), "declares no command") {
		t.Fatalf("the refusal does not say what is wrong: %v", err)
	}
}

func TestAuthorisationIsDecidedBeforeTheArgumentsAreLookedAt(t *testing.T) {
	// A caller a house rule refuses must not learn whether its arguments
	// would have been accepted. The order is the whole assertion: send
	// arguments that are BOTH unauthorised and invalid, and the answer has to
	// be the refusal.
	sock, d := upDaemon(t, nil)
	dangerous(t, sock)
	if err := d.kernel.SetRules([]kernel.Rule{{
		ID: "deny-destructive", Caller: kernel.AnyCaller(),
		Effects: kernel.EffectsDestructive, Action: kernel.ActionDeny,
	}}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	var resp rigv1.CallResponse
	err := dial(t, sock).Call(ctx5(t), "shelf.reindex",
		&rigv1.CallRequest{Args: []byte(`{"nonsense":true}`)}, &resp)
	if err == nil {
		t.Fatal("allowed")
	}
	if !strings.Contains(err.Error(), "DENIED") {
		t.Fatalf("the schema was checked first: %v", err)
	}
	if strings.Contains(err.Error(), "schema") {
		t.Fatalf("the refusal leaked the schema failure: %v", err)
	}
}

func TestTheProbeIsExemptFromArgumentValidation(t *testing.T) {
	// rig.ping's payload is rig's own typed message, not declared JSON, and
	// PingRequest is wire-compatible with CallRequest - a nonce decodes as
	// `args`. Without the exemption by name, every liveness probe would be
	// validated as JSON and fail. See ProbeCommand.
	sock, _ := upDaemon(t, nil)
	program(t, sock, "grabbit")

	resp := &rigv1.PingResponse{}
	if err := dial(t, sock).Call(ctx5(t), "grabbit.ping",
		&rigv1.PingRequest{Nonce: []byte("not json")}, resp); err != nil {
		t.Fatalf("the probe was refused: %v", err)
	}
	if string(resp.GetNonce()) != "not json" {
		t.Fatalf("the probe came back with %q", resp.GetNonce())
	}
}
