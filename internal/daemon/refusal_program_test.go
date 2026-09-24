package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/client"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// ⛔ A PROGRAM CAN REFUSE THE WAY RIG DOES. A handler that returns a
// *client.CallError has its Status - code, precondition, actual, fix and fix
// command - delivered to the caller unchanged, through the daemon's relay;
// any other error is INTERNAL. Before this every handler error was INTERNAL,
// so a program could not tell an agent "invalid argument, here is the fix".
func TestAProgramsRefusalReachesTheCallerUnchanged(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	decl := testDeclaration("shelf")
	find := proto.Clone(decl.GetCommands()[0]).(*rigv1.Command)
	find.Id, find.Title, find.Summary = "find", "Find", "Find a shelf"
	decl.Commands = append(decl.Commands, find)
	cmd := find.GetId()
	want := &rigv1.Status{
		Code: rigv1.Code_CODE_INVALID, Message: "shelf: no such shelf",
		Precondition: "the shelf exists", Actual: "there is no shelf 9",
		Fix: "list the shelves", FixCommand: "rig shelf list",
	}

	p := dial(t, sock)
	p.Handle(func(method string, payload []byte) (proto.Message, error) {
		switch method {
		case "shelf.ping":
			var req rigv1.PingRequest
			if err := proto.Unmarshal(payload, &req); err != nil {
				return nil, err
			}
			return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: "shelf", Version: "1.0"}, nil
		case "shelf." + cmd:
			return nil, &client.CallError{Method: method, Status: want}
		}
		return nil, errors.New("an undescribed failure")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.Hello(ctx, decl); err != nil {
		t.Fatal(err)
	}

	caller := dial(t, sock)
	err := caller.Call(ctx, "shelf."+cmd, &rigv1.CallRequest{}, &rigv1.CallResponse{})
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("the refusal arrived as %v, want a CallError", err)
	}
	if !proto.Equal(ce.Status, want) {
		t.Errorf("the refusal changed in transit:\ngot  %v\nwant %v", ce.Status, want)
	}

	// And an error the program did not describe is still INTERNAL.
	p.Handle(func(string, []byte) (proto.Message, error) { return nil, errors.New("disk on fire") })
	err = caller.Call(ctx, "shelf."+cmd, &rigv1.CallRequest{}, &rigv1.CallResponse{})
	if !errors.As(err, &ce) || ce.Code() != rigv1.Code_CODE_INTERNAL {
		t.Errorf("a plain error arrived as %v, want INTERNAL", err)
	}
}
