package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// declaring connects a program whose declaration names these kit elements,
// and reports the error rather than failing, because half these cases expect
// one.
func declaring(t *testing.T, sock, name string, elements []string) error {
	t.Helper()
	c := dial(t, sock)
	c.Handle(func(_ string, payload []byte) (proto.Message, error) {
		var req rigv1.PingRequest
		if err := proto.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return &rigv1.PingResponse{Nonce: req.GetNonce(), Program: name, Version: "1.0"}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := testDeclaration(name)
	d.Elements = elements
	_, err := c.Hello(ctx, d)
	return err
}

// The declared element list survives the wire in both directions: it is a
// registration declaration (5h R3), so a reader gets it the way it gets
// services.
func TestTheDeclaredElementListRoundTrips(t *testing.T) {
	sock := up(t)
	if err := declaring(t, sock, "pullreport", []string{"rigTable", "rigToolbar"}); err != nil {
		t.Fatalf("a declaration naming served elements was refused: %v", err)
	}

	var resp rigv1.ProgramsResponse
	if err := dial(t, sock).Call(ctx5(t), "rig.programs",
		&rigv1.ProgramsRequest{}, &resp); err != nil {
		t.Fatalf("programs: %v", err)
	}
	if len(resp.GetPrograms()) != 1 {
		t.Fatalf("want one program, got %d", len(resp.GetPrograms()))
	}
	got := resp.GetPrograms()[0].GetElements()
	if len(got) != 2 || got[0] != "rigTable" || got[1] != "rigToolbar" {
		t.Fatalf("the element list came back as %v", got)
	}
}

// R7 over the wire, which is where it has to hold: the refusal is at
// REGISTRATION, so the program never connects and never gets to serve a page
// that would have failed at render.
func TestAnUnservedElementRefusesTheHandshakeOverTheWire(t *testing.T) {
	sock := up(t)
	err := declaring(t, sock, "clinic", []string{"rigTable", "rigAccordion"})
	if err == nil {
		t.Fatal("a program naming an element rig does not serve completed the handshake")
	}
	if !strings.Contains(err.Error(), "rigAccordion") {
		t.Fatalf("the refusal does not name the element: %v", err)
	}

	// And it is not registered, so nothing downstream can reach a program
	// whose page rig cannot draw.
	var resp rigv1.ProgramsResponse
	if err := dial(t, sock).Call(ctx5(t), "rig.programs",
		&rigv1.ProgramsRequest{}, &resp); err != nil {
		t.Fatalf("programs: %v", err)
	}
	if len(resp.GetPrograms()) != 0 {
		t.Fatalf("a refused program is in the registry: %v", resp.GetPrograms())
	}
}
