package daemon

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
)

// A program is told who calls it, as rig knows the caller: its seat, its
// kind and its process. What a client claims about itself is overwritten,
// so a program may decide on it what a caller may read.
func TestAProgramIsToldWhoCallsIt(t *testing.T) {
	sock := upRecordDaemon(t)
	got := make(chan *rigv1.Caller, 1)
	app := dial(t, sock)
	app.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.CallRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		got <- req.GetCaller()
		return &rigv1.CallResponse{Result: []byte(`{}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	decl := testDeclaration("app")
	who := proto.CloneOf(decl.GetCommands()[0])
	who.Id, who.Title, who.Args = "who", "Who calls", []byte(`{"type":"object"}`)
	decl.Commands = append(decl.Commands, who)
	if _, err := app.Hello(ctx, decl); err != nil {
		t.Fatal(err)
	}

	you := seated(t, sock, "backend-1")
	forged := &rigv1.Caller{Kind: "program", Program: "chief", Seat: "boris", Pid: 1}
	if err := you.Call(ctx, "app.who", &rigv1.CallRequest{Args: []byte(`{}`), Caller: forged}, &rigv1.CallResponse{}); err != nil {
		t.Fatal(err)
	}
	c := <-got
	if c.GetSeat() != "backend-1" || c.GetProgram() != "" || c.GetPid() != int32(os.Getpid()) || c.GetKind() == "" || c.GetKind() == "program" {
		t.Fatalf("the program was told it was called by %v, want seat backend-1 from pid %d", c, os.Getpid())
	}
}
